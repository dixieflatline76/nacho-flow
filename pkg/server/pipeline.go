package server

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/router"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
)

func resolveFeatureFlags(reqCtx contract.RequestContext, targetTier contract.Tier, globalShieldEnabled bool) router.FeatureFlag {
	// If explicit in-prompt directive was parsed (and not default), it takes highest precedence
	if reqCtx.Features != 0 && reqCtx.Features != uint16(router.FeatureDefaultAll) {
		return router.FeatureFlag(reqCtx.Features)
	}

	// Next precedence: Tier Policy in config.yaml
	if targetTier.Raw != nil && *targetTier.Raw {
		return router.FeatureRawPassThrough
	}

	flags := router.FeatureDefaultAll
	if !globalShieldEnabled || reqCtx.NoShield {
		flags = flags.MaskOut(router.FeatureShieldEnabled | router.FeatureShieldFollowup | router.FeatureShieldModeSwitch)
	}

	if targetTier.Shield != nil {
		if !*targetTier.Shield {
			flags = flags.MaskOut(router.FeatureShieldEnabled | router.FeatureShieldFollowup | router.FeatureShieldModeSwitch)
		} else {
			flags = flags | router.FeatureShieldEnabled
		}
	}

	if targetTier.Normalizer != nil && !*targetTier.Normalizer {
		flags = flags.MaskOut(router.FeatureToolNormalizer | router.FeatureNormMarkdown | router.FeatureNormBareJSON | router.FeatureNormReAct)
	}

	if targetTier.Normalizers != nil {
		if targetTier.Normalizers.Enabled != nil && !*targetTier.Normalizers.Enabled {
			flags = flags.MaskOut(router.FeatureToolNormalizer | router.FeatureNormMarkdown | router.FeatureNormBareJSON | router.FeatureNormReAct)
		}
		if targetTier.Normalizers.Markdown != nil && !*targetTier.Normalizers.Markdown {
			flags = flags.MaskOut(router.FeatureNormMarkdown)
		}
		if targetTier.Normalizers.BareJSON != nil && !*targetTier.Normalizers.BareJSON {
			flags = flags.MaskOut(router.FeatureNormBareJSON)
		}
		if targetTier.Normalizers.ReAct != nil && !*targetTier.Normalizers.ReAct {
			flags = flags.MaskOut(router.FeatureNormReAct)
		}
		if targetTier.Normalizers.Think != nil && !*targetTier.Normalizers.Think {
			flags = flags.MaskOut(router.FeatureThinkNormalizer | router.FeatureThinkSanitize)
		}
	}

	return flags
}

// ResolveTierVision determines whether a tier's model supports multimodal image inputs.
// Priority order:
// 1. If StripImages is true -> ResolvedHasVision = false (explicit stripping always wins).
// 2. If HasVision is explicitly specified in YAML (*bool) -> use that value.
// 3. If PricingOracle metadata reports SupportsVision -> use that.
// 4. Fallback for canonical frontier multimodal families (gemini, claude, gpt-4o, *-vl) when uncataloged/offline.
// 5. Default to false (safe text-only mode).
func ResolveTierVision(t *contract.Tier, oracle *telemetry.PricingOracle) {
	if t.StripImages {
		t.ResolvedHasVision = false
		return
	}
	if t.HasVision != nil {
		t.ResolvedHasVision = *t.HasVision
		return
	}
	if oracle != nil {
		if meta, ok := oracle.GetModelMetadata(t.Provider, t.Model); ok {
			t.ResolvedHasVision = meta.SupportsVision
			return
		}
	}
	m := strings.ToLower(t.Model)
	if strings.Contains(m, "gemini") || strings.Contains(m, "claude") || strings.Contains(m, "gpt-4o") || strings.Contains(m, "-vl") {
		t.ResolvedHasVision = true
		return
	}
	t.ResolvedHasVision = false
}

func (s *Server) handleCompletions(w http.ResponseWriter, r *http.Request, startTime time.Time, reqLogger *slog.Logger) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		reqLogger.Error("Failed to read request body", slog.Any("error", err))
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	_ = r.Body.Close()

	// 1. Classify request metadata
	reqCtx, err := s.classifier.Classify(body)
	if err != nil {
		reqLogger.Warn("Failed to classify payload", slog.Any("error", err))
	}

	// Honor config toggle to disable in-prompt directives
	if s.GetConfig().Router.EnableInPromptDirectives != nil && !*s.GetConfig().Router.EnableInPromptDirectives {
		reqCtx.IsMetaDirective = false
		reqCtx.ForcedTier = ""
		reqCtx.ForcedModel = ""
		reqCtx.MetaDirective = ""
		reqCtx.MetaDirectiveRaw = ""
		reqCtx.CleanPrompt = reqCtx.Prompt
	}

	sessionKey := extractSessionKey(r)
	if s.sessionTracker == nil {
		s.sessionTracker = router.NewSessionTracker(5 * time.Minute)
	}

	// 2. Intercept local meta directives (@nacho:help, @nacho:tiers, @nacho:status, @nacho:deals, @nacho:toggles, @nacho:reset, @nacho:<standalone_toggle>, @nacho:<typo>)
	if reqCtx.IsMetaDirective {
		isStream := strings.Contains(string(body), `"stream":true`) || strings.Contains(string(body), `"stream": true`)
		env := MetaEnv{
			Config:         s.GetConfig(),
			Stats:          s.tracker,
			Oracle:         s.oracle,
			Providers:      s.GetRegistry(),
			StartTime:      s.startTime,
			DaemonVersion:  contract.Version,
			SessionTracker: s.sessionTracker,
			SessionKey:     sessionKey,
		}
		if s.metaRegistry == nil {
			s.metaRegistry = NewMetaRegistry()
		}
		s.metaRegistry.Dispatch(w, r, reqCtx, env, s.sessionTracker, isStream)
		return
	}

	// 2.5 Handle embedded session guardrail toggles
	if reqCtx.CleanPrompt != reqCtx.Prompt {
		info, _ := router.ExtractDirective(reqCtx.Prompt)
		switch info.Directive {
		case "kickstart-off":
			s.sessionTracker.SetKickstartDisabled(sessionKey, true)
		case "kickstart-on":
			s.sessionTracker.SetKickstartDisabled(sessionKey, false)
		case "cyclekiller-off":
			s.sessionTracker.SetCycleKillerDisabled(sessionKey, true)
		case "cyclekiller-on":
			s.sessionTracker.SetCycleKillerDisabled(sessionKey, false)
		case "shield-off":
			s.sessionTracker.SetShieldDisabled(sessionKey, true)
		case "shield-on":
			s.sessionTracker.SetShieldDisabled(sessionKey, false)
		case "raw-on":
			s.sessionTracker.SetRawModeEnabled(sessionKey, true)
		case "raw-off":
			s.sessionTracker.SetRawModeEnabled(sessionKey, false)
		case "fairydust-off":
			s.sessionTracker.SetFairyDustDisabled(sessionKey, true)
		case "fairydust-on":
			s.sessionTracker.SetFairyDustDisabled(sessionKey, false)
		}
	}

	// 2.6 Apply persisted session guardrails to request context
	guardrails := s.sessionTracker.GetGuardrails(sessionKey)
	reqCtx.NoKickstart = guardrails.KickstartDisabled
	reqCtx.NoCycleKiller = guardrails.CycleKillerDisabled
	reqCtx.NoShield = guardrails.ShieldDisabled
	reqCtx.RawModeEnabled = guardrails.RawModeEnabled

	if guardrails.RawModeEnabled {
		reqCtx.Features = uint16(router.FeatureRawPassThrough)
	} else if guardrails.ShieldDisabled {
		reqCtx.Features = uint16(router.FeatureFlag(reqCtx.Features).MaskOut(router.FeatureShieldEnabled | router.FeatureShieldFollowup | router.FeatureShieldModeSwitch))
	}

	// 3. Track session retries for auto-escalation
	reqCtx.SessionKey = sessionKey
	promptHash := router.HashPrompt(reqCtx.Prompt)

	cfg := s.GetConfig()

	// Pass tool progress signal from classifier to session tracker.
	// In write-only mode, genuine forward progress requires write progress or passing tests,
	// preventing read-only command loops from resetting retries while tests fail.
	turnProgress := reqCtx.HasToolProgress
	writeOnly := cfg.Kickstart.WriteOnly || cfg.CycleKiller.KickstartWriteOnly || cfg.CycleBreaker.KickstartWriteOnly
	if writeOnly {
		turnProgress = reqCtx.HasWriteProgress || reqCtx.HasShellWrite || reqCtx.HasTestProgress
		if reqCtx.HasTools && !reqCtx.HasWriteCapability {
			turnProgress = turnProgress || reqCtx.HasToolProgress
		}
	}
	retries, isRetry := s.sessionTracker.RecordTurn(sessionKey, promptHash, turnProgress)
	reqCtx.CoolingDownModels = s.sessionTracker.GetCoolingDownModels(sessionKey)

	// Kickstart: detect semantic stall/idle loop across consecutive turns
	kickstartThreshold := cfg.Kickstart.Threshold
	if kickstartThreshold == 0 {
		kickstartThreshold = cfg.CycleKiller.KickstartThreshold
	}
	if kickstartThreshold == 0 && cfg.CycleBreaker.KickstartThreshold > 0 {
		kickstartThreshold = cfg.CycleBreaker.KickstartThreshold
	}
	if kickstartThreshold > 0 && !reqCtx.NoKickstart {
		kickstartProgress := reqCtx.HasToolProgress
		if writeOnly {
			// Legitimate progress requires concrete write activity OR a clean passing test suite.
			// Failing tests require code edits to fix and do NOT prevent kickstart accumulation.
			kickstartProgress = reqCtx.HasWriteProgress || reqCtx.HasShellWrite || reqCtx.HasTestProgress
		}
		// Part A Guard: Auto-suspend when agent has tools but zero write tools (Plan Mode)
		if writeOnly && reqCtx.HasTools && !reqCtx.HasWriteCapability {
			kickstartProgress = true
		}
		maxFailures := cfg.Kickstart.MaxFailures
		if maxFailures == 0 {
			maxFailures = cfg.CycleKiller.KickstartMaxFailures + cfg.CycleBreaker.KickstartMaxFailures
		}
		kickstartCount, isKickstarted := s.sessionTracker.RecordKickstartState(sessionKey, kickstartProgress, kickstartThreshold, maxFailures)
		if isKickstarted {
			reqCtx.SessionKickstarted = true
			reqCtx.SessionKickstartCount = kickstartCount
			reqLogger.Warn("Kickstart: agent idling without tool progress",
				slog.Int("kickstart_count", kickstartCount),
				slog.Int("kickstart_threshold", kickstartThreshold),
				slog.Bool("write_only", writeOnly),
				slog.Bool("has_test_pass", reqCtx.HasTestPass),
				slog.Bool("has_test_fail", reqCtx.HasTestFail),
				slog.String("session_key", sessionKey),
			)
		}

		// Kickstart max cap: force-escalate to default tier when kickstart count exceeds limit
		maxKS := cfg.Kickstart.MaxCount
		if maxKS == 0 {
			maxKS = cfg.CycleKiller.KickstartMaxCount
		}
		if maxKS == 0 {
			maxKS = cfg.CycleBreaker.KickstartMaxCount
		}
		if maxKS > 0 && isKickstarted && kickstartCount >= maxKS {
			reqLogger.Warn("Kickstart: max count exceeded, force-escalating to default tier",
				slog.Int("kickstart_count", kickstartCount),
				slog.Int("kickstart_max_count", maxKS),
			)
			reqCtx.ForcedTier = "cloud"
		}
	}

	// Fairy Dust: periodic proactive frontier model quality checkpoints
	fdCfg := cfg.FairyDust
	if fdCfg.Enabled != nil && *fdCfg.Enabled && len(fdCfg.Entries) > 0 && !guardrails.FairyDustDisabled {
		// 1. Record write progress (single global counter, increments only on write turns)
		writeCount := s.sessionTracker.RecordWriteProgress(sessionKey, reqCtx.HasWriteProgress || reqCtx.HasShellWrite)

		// 2. Check each entry; collect the highest-priority candidate that triggers
		type fdCandidate struct {
			entry contract.FairyDustEntry
			count int
		}
		var winner *fdCandidate
		if reqCtx.HasWriteProgress && writeCount > 0 {
			for _, entry := range fdCfg.Entries {
				if entry.Frequency <= 0 || entry.Model == "" {
					continue
				}
				maxFD := entry.MaxPerSession
				if maxFD <= 0 {
					maxFD = 5
				}
				entryCount, shouldTrigger := s.sessionTracker.CheckFairyDust(
					sessionKey, entry.Name, entry.Frequency, maxFD,
				)
				if shouldTrigger {
					if winner == nil || entry.Priority > winner.entry.Priority {
						entryCopy := entry
						winner = &fdCandidate{entry: entryCopy, count: entryCount}
					}
				}
			}
		}

		// 3. Apply winning entry
		if winner != nil {
			reqCtx.FairyDusted = true
			reqCtx.FairyDustEntry = winner.entry.Name
			reqCtx.FairyDustCount = winner.count
			reqCtx.NoCycleKiller = true
			reqCtx.NoKickstart = true
			reqCtx.NoShield = true
			reqLogger.Info("Fairy Dust: quality checkpoint triggered",
				slog.String("fairy_dust_entry", winner.entry.Name),
				slog.Int("fairy_dust_count", winner.count),
				slog.Int("write_progress_count", writeCount),
				slog.String("fairy_dust_model", winner.entry.Model),
				slog.Int("fairy_dust_priority", winner.entry.Priority),
				slog.String("session_key", sessionKey),
			)
		}
	}

	// In-history errors OVERRIDE session tracker when they detect real failures
	if reqCtx.HistoryErrors > retries {
		retries = reqCtx.HistoryErrors
		isRetry = true
	}

	reqCtx.Retries = retries
	reqCtx.IsRetry = isRetry

	// 4. Evaluate 1..N tiers using expr engine
	targetTier, err := s.GetEvaluator().SelectTier(reqCtx)
	if err != nil {
		reqLogger.Error("Error evaluating tier, falling back to default", slog.Any("error", err))
		targetTier = s.GetConfig().DefaultTier
	}

	// 4.5: Escalation budget — prevent runaway frontier costs
	defaultTierName := s.GetConfig().DefaultTier.Name
	if targetTier.Name == defaultTierName {
		budgetExhausted := s.sessionTracker.RecordEscalation(sessionKey)
		if budgetExhausted {
			// Force de-escalation: pick the first cloud tier that isn't the default
			for _, tier := range s.GetConfig().Tiers {
				if tier.Name != defaultTierName {
					targetTier = tier
					break
				}
			}
			reqLogger.Warn("Escalation budget exhausted, de-escalating",
				slog.String("fallback_tier", targetTier.Name),
				slog.String("fallback_model", targetTier.Model))
		}
	} else {
		s.sessionTracker.ResetEscalation(sessionKey)
	}

	// 4.6: Fairy Dust — override tier with winning entry's frontier model
	if reqCtx.FairyDusted {
		reqCtx.NoCycleKiller = true
		reqCtx.NoKickstart = true
		reqCtx.NoShield = true
		for _, entry := range cfg.FairyDust.Entries {
			if entry.Name == reqCtx.FairyDustEntry {
				provider := entry.Provider
				if provider == "" {
					provider = s.GetConfig().DefaultTier.Provider
				}
				targetTier = contract.Tier{
					Name:     fmt.Sprintf("Fairy Dust: %s #%d (%s)", entry.Name, reqCtx.FairyDustCount, entry.Model),
					Model:    entry.Model,
					Provider: provider,
				}
				break
			}
		}
	}

	// 5. If forced directive is used, check provider circuit breaker (Strict Fallback Bypass)
	if reqCtx.ForcedTier != "" || reqCtx.ForcedModel != "" {
		targetProvider, found := s.GetRegistry().Get(targetTier.Provider)
		if found && !s.allowProvider(targetProvider) {
			reqLogger.Warn("Forced tier provider circuit is OPEN, blocking request without fallback",
				slog.String("tier", targetTier.Name),
				slog.String("provider", targetTier.Provider))
			alertMsg := RenderCircuitBlocked(targetTier.Name, targetTier.Provider)
			isStream := strings.Contains(string(body), `"stream":true`) || strings.Contains(string(body), `"stream": true`)
			if isStream {
				sse := &SSEMetaPresenter{}
				_ = sse.WriteResponse(w, alertMsg, fmt.Sprintf("circuit-blocked-%d", time.Now().UnixNano()))
			} else {
				jsonP := &JSONMetaPresenter{}
				_ = jsonP.WriteResponse(w, alertMsg, fmt.Sprintf("circuit-blocked-%d", time.Now().UnixNano()))
			}
			return
		}
	}

	reqLogger.Info("Routing request",
		slog.String("session_key", sessionKey),
		slog.String("tier", targetTier.Name),
		slog.String("model", targetTier.Model),
		slog.String("provider", targetTier.Provider),
		slog.Int("tokens", reqCtx.Tokens),
		slog.Bool("has_images", reqCtx.HasImages),
		slog.Bool("has_tools", reqCtx.HasTools),
		slog.Int("retries", reqCtx.Retries),
		slog.Bool("has_tool_progress", reqCtx.HasToolProgress),
		slog.Bool("has_write_progress", reqCtx.HasWriteProgress),
		slog.Bool("has_shell_write", reqCtx.HasShellWrite),
		slog.Bool("has_test_progress", reqCtx.HasTestProgress),
		slog.Bool("has_test_pass", reqCtx.HasTestPass),
		slog.Bool("has_test_fail", reqCtx.HasTestFail),
		slog.Int("history_errors", reqCtx.HistoryErrors),
		slog.Any("cooling_down_models", reqCtx.CoolingDownModels),
		slog.String("user_agent", r.Header.Get("User-Agent")),
	)

	// Kickstart: inject prompt when idle loop detected
	if reqCtx.SessionKickstarted && !reqCtx.NoKickstart {
		kickstartPrompt := contract.DefaultKickstartPrompt
		if cfg.Kickstart.Prompt != "" {
			kickstartPrompt = cfg.Kickstart.Prompt
		} else if cfg.CycleKiller.KickstartPrompt != "" {
			kickstartPrompt = cfg.CycleKiller.KickstartPrompt
		} else if cfg.CycleBreaker.KickstartPrompt != "" {
			kickstartPrompt = cfg.CycleBreaker.KickstartPrompt
		}
		body = injectCorrectionPrompt(body, kickstartPrompt)
		reqLogger.Info("Kickstart: injected prompt",
			slog.Int("kickstart_count", reqCtx.SessionKickstartCount),
		)
		// Record this injection as a potential failure. If the model produces no tool
		// calls this turn, the next request will have HistoryErrors > 0, and
		// RecordKickstartFailure will have already incremented the circuit breaker counter.
		// This prevents MODEL_NO_TOOLS_USED death spirals.
		s.sessionTracker.RecordKickstartFailure(sessionKey)
	}

	// Fairy Dust: inject checkpoint prompt from winning entry
	if reqCtx.FairyDusted {
		fdPrompt := contract.DefaultFairyDustPrompt
		for _, entry := range cfg.FairyDust.Entries {
			if entry.Name == reqCtx.FairyDustEntry && entry.Prompt != "" {
				fdPrompt = entry.Prompt
				break
			}
		}
		body = injectCorrectionPrompt(body, fdPrompt)
		reqLogger.Info("Fairy Dust: injected checkpoint prompt",
			slog.String("fairy_dust_entry", reqCtx.FairyDustEntry),
			slog.Int("fairy_dust_count", reqCtx.FairyDustCount),
		)
	}

	s.forwardWithFallback(w, r, reqCtx, targetTier, body, startTime, reqLogger)
}
