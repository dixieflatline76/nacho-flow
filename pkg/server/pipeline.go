package server

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/agentregistry"
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

	// 0. Wire-speed ingress context compaction (NTS)
	body, ntsTokensSaved, ntsBytesSaved := s.compactIngress(r, body)

	// 1. Classify request metadata & client detection
	reqCtx := s.classifyRequest(r, body, ntsTokensSaved, ntsBytesSaved, reqLogger)
	sessionKey := extractSessionKey(r)
	if s.sessionTracker == nil {
		s.sessionTracker = router.NewSessionTracker(5 * time.Minute)
	}

	// 2. Intercept local meta directives (@nacho:help, @nacho:tiers, etc.)
	if s.handleMetaDirectives(w, r, reqCtx, sessionKey, body) {
		return
	}

	// 3. Apply session guardrail directives (raw-on, shield-off, kickstart-off)
	s.applySessionGuardrails(sessionKey, reqCtx)

	cfg := s.GetConfig()

	// 4. Track session turns & detect Read Slap semantic stalls
	s.evaluateTurnAndReadSlap(sessionKey, reqCtx, cfg, reqLogger)

	// 5. Evaluate proactive Fairy Dust checkpoints
	s.evaluateFairyDust(sessionKey, reqCtx, cfg, reqLogger)

	// 6. Select execution tier with Anti-Latching Breaker & Fairy Dust overrides
	targetTier := s.resolveTargetTier(sessionKey, reqCtx, cfg, reqLogger)

	// 7. Check circuit breaker for forced provider directives
	if s.isForcedProviderBlocked(w, reqCtx, targetTier, body, reqLogger) {
		return
	}

	s.logRoutingDecision(sessionKey, targetTier, reqCtx, r, reqLogger)

	// 8. Apply prompt corrections (Read Slap & Fairy Dust injection)
	body = s.applyPromptInjections(sessionKey, reqCtx, cfg, body, reqLogger)

	// 9. Upstream proxy dispatch with automated tier fallback
	s.forwardWithFallback(w, r, *reqCtx, targetTier, body, startTime, reqLogger)
}

// compactIngress applies wire-speed NTS context compaction at ingress before classification.
// Returns the potentially transformed body slice along with saved token and byte metrics.
func (s *Server) compactIngress(r *http.Request, body []byte) ([]byte, int, int) {
	var ntsTokensSaved, ntsBytesSaved int
	if ntsTr := s.GetNTSTransformer(); ntsTr != nil {
		if strings.Contains(r.URL.Path, "messages") || r.Header.Get("anthropic-version") != "" {
			if transformed, res, err := ntsTr.TransformAnthropic(body); err == nil && !res.Bypassed {
				body = transformed
				ntsTokensSaved = res.TokensSaved
				ntsBytesSaved = res.BytesSaved
			}
		} else {
			if transformed, res, err := ntsTr.TransformOpenAI(body); err == nil && !res.Bypassed {
				body = transformed
				ntsTokensSaved = res.TokensSaved
				ntsBytesSaved = res.BytesSaved
			}
		}
	}
	return body, ntsTokensSaved, ntsBytesSaved
}

// classifyRequest parses the request body AST, attaches NTS metrics, and detects the client agent harness.
func (s *Server) classifyRequest(r *http.Request, body []byte, ntsTokensSaved, ntsBytesSaved int, reqLogger *slog.Logger) *contract.RequestContext {
	reqCtx, err := s.classifier.Classify(body)
	if err != nil {
		reqLogger.Warn("Failed to classify payload", slog.Any("error", err))
	}
	reqCtx.NTSTokensSaved = ntsTokensSaved
	reqCtx.NTSBytesSaved = ntsBytesSaved
	reqCtx.RawTokens = reqCtx.Tokens + ntsTokensSaved
	reqCtx.ClientID = agentregistry.DefaultRegistry().DetectClient(r.Header, body)

	// Honor config toggle to disable in-prompt directives
	if s.GetConfig().Router.EnableInPromptDirectives != nil && !*s.GetConfig().Router.EnableInPromptDirectives {
		reqCtx.IsMetaDirective = false
		reqCtx.ForcedTier = ""
		reqCtx.ForcedModel = ""
		reqCtx.MetaDirective = ""
		reqCtx.MetaDirectiveRaw = ""
		reqCtx.CleanPrompt = reqCtx.Prompt
	}
	return &reqCtx
}

// handleMetaDirectives intercepts local administrative commands (@nacho:help, @nacho:tiers, etc.).
// Returns true if the request was handled and the caller should return immediately.
func (s *Server) handleMetaDirectives(w http.ResponseWriter, r *http.Request, reqCtx *contract.RequestContext, sessionKey string, body []byte) bool {
	if !reqCtx.IsMetaDirective {
		return false
	}
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
	s.metaRegistry.Dispatch(w, r, *reqCtx, env, s.sessionTracker, isStream)
	return true
}

// applySessionGuardrails processes in-prompt guardrail toggles (e.g., kickstart-off, raw-on)
// and updates reqCtx feature flags and guardrail overrides accordingly.
func (s *Server) applySessionGuardrails(sessionKey string, reqCtx *contract.RequestContext) {
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
}

// evaluateTurnAndReadSlap tracks session retries, calculates genuine forward progress
// (distinguishing writes/tests from errors), and flags Read Slap when agents idle in loops.
func (s *Server) evaluateTurnAndReadSlap(
	sessionKey string,
	reqCtx *contract.RequestContext,
	cfg *contract.Config,
	reqLogger *slog.Logger,
) {
	reqCtx.SessionKey = sessionKey
	promptHash := reqCtx.PromptHash
	if promptHash == 0 && reqCtx.Prompt != "" {
		promptHash = router.HashPrompt(reqCtx.Prompt)
	}
	rootPromptHash := reqCtx.RootPromptHash
	if rootPromptHash == 0 {
		rootPromptHash = promptHash
	}

	// Pass tool progress signal from classifier to session tracker.
	// In agent loop mode, genuine forward progress requires verified test passes,
	// or workspace actions conducted in an error-free state.
	// An unverified write during active test failure is an attempted fix, NOT verified progress.
	turnProgress := reqCtx.HasToolProgress && !reqCtx.HasTestFail
	writeOnly := cfg.Kickstart.ReadSlap.WriteOnly
	if writeOnly {
		turnProgress = reqCtx.HasTestProgress || ((reqCtx.HasWriteProgress || reqCtx.HasShellWrite) && !reqCtx.HasTestFail)
		if reqCtx.HasTools && !reqCtx.HasWriteCapability && !reqCtx.HasTestFail {
			turnProgress = turnProgress || reqCtx.HasToolProgress
		}
	} else if reqCtx.HasTestProgress {
		turnProgress = true
	}
	retries, isRetry := s.sessionTracker.RecordTurnTask(
		sessionKey,
		rootPromptHash,
		promptHash,
		turnProgress,
		reqCtx.HasTools,
		reqCtx.MessageCount,
	)
	reqCtx.CoolingDownModels = s.sessionTracker.GetCoolingDownModels(sessionKey)

	// In-history errors OVERRIDE session tracker when they detect real failures
	if reqCtx.HistoryErrors > retries {
		retries = reqCtx.HistoryErrors
		isRetry = true
	}
	reqCtx.Retries = retries
	reqCtx.IsRetry = isRetry

	// Read Slap: detect semantic stall/idle loop across consecutive turns
	readSlapEnabled := cfg.Kickstart.ReadSlap.Enabled == nil || *cfg.Kickstart.ReadSlap.Enabled
	readSlapThreshold := cfg.Kickstart.ReadSlap.Threshold
	if readSlapThreshold <= 0 && readSlapEnabled {
		readSlapThreshold = 5
	}
	if readSlapEnabled && readSlapThreshold > 0 && !reqCtx.NoKickstart {
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
		maxFailures := cfg.Kickstart.ReadSlap.MaxFailures
		if maxFailures <= 0 {
			maxFailures = 3
		}
		kickstartCount, isKickstarted := s.sessionTracker.RecordKickstartState(sessionKey, kickstartProgress, readSlapThreshold, maxFailures)
		if isKickstarted {
			reqCtx.SessionReadSlapped = true
			reqCtx.SessionReadSlapCount = kickstartCount
			reqCtx.SessionKickstarted = true
			reqCtx.SessionKickstartCount = kickstartCount
			reqLogger.Warn("Read Slap: agent idling without tool progress",
				slog.Int("read_slap_count", kickstartCount),
				slog.Int("read_slap_threshold", readSlapThreshold),
				slog.Bool("write_only", writeOnly),
				slog.Bool("has_test_pass", reqCtx.HasTestPass),
				slog.Bool("has_test_fail", reqCtx.HasTestFail),
				slog.String("session_key", sessionKey),
			)
		}

		// Read Slap max slaps cap: force-escalate to default tier when count exceeds limit
		maxSlaps := cfg.Kickstart.ReadSlap.MaxSlaps
		if maxSlaps <= 0 {
			maxSlaps = 10
		}
		if maxSlaps > 0 && isKickstarted && kickstartCount >= maxSlaps {
			reqLogger.Warn("Read Slap: max slaps exceeded, force-escalating to default tier",
				slog.Int("read_slap_count", kickstartCount),
				slog.Int("max_slaps", maxSlaps),
			)
			reqCtx.ForcedTier = "cloud"
		}
	}
}

// evaluateFairyDust checks write milestones against configured Fairy Dust entries,
// picking the highest-priority qualifying frontier model review checkpoint.
func (s *Server) evaluateFairyDust(
	sessionKey string,
	reqCtx *contract.RequestContext,
	cfg *contract.Config,
	reqLogger *slog.Logger,
) {
	fdCfg := cfg.FairyDust
	guardrails := s.sessionTracker.GetGuardrails(sessionKey)
	if fdCfg.Enabled == nil || !*fdCfg.Enabled || len(fdCfg.Entries) == 0 || guardrails.FairyDustDisabled {
		return
	}

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

// resolveTargetTier orchestrates tier evaluation via expression matching, enforces the
// anti-latching de-escalation circuit breaker, and applies Fairy Dust quality tier overrides.
func (s *Server) resolveTargetTier(
	sessionKey string,
	reqCtx *contract.RequestContext,
	cfg *contract.Config,
	reqLogger *slog.Logger,
) contract.Tier {
	targetTier, err := s.GetEvaluator().SelectTier(*reqCtx)
	if err != nil {
		reqLogger.Error("Error evaluating tier, falling back to default", slog.Any("error", err))
		targetTier = cfg.DefaultTier
	}

	targetTier = s.enforceAntiLatchingBreaker(sessionKey, targetTier, cfg, reqLogger)
	targetTier = s.applyFairyDustTierOverride(reqCtx, targetTier, cfg)
	return targetTier
}

// enforceAntiLatchingBreaker prevents sessions from permanently latching onto the expensive
// default tier when accumulated retries lock out economical workhorse tiers.
//
// Pre-conditions:
//   - targetTier has been evaluated via expr engine or default fallback.
//   - reqCtx.Retries and reqCtx.HistoryErrors have been updated.
//   - cfg contains DefaultTier and configured Tiers slice.
//
// Post-conditions:
//   - If targetTier == DefaultTier and escalation turns >= MaxEscalationTurns (default: 3),
//     targetTier is forcibly overridden to an economical non-default cloud tier to allow
//     forward progress to reset retries to 0.
//   - If targetTier != DefaultTier, the escalation budget counter is reset to 0.
func (s *Server) enforceAntiLatchingBreaker(
	sessionKey string,
	targetTier contract.Tier,
	cfg *contract.Config,
	reqLogger *slog.Logger,
) contract.Tier {
	defaultTierName := cfg.DefaultTier.Name
	if targetTier.Name == defaultTierName {
		budgetExhausted := s.sessionTracker.RecordEscalation(sessionKey)
		if budgetExhausted {
			// Force de-escalation: pick the first cloud tier that isn't the default
			for _, tier := range cfg.Tiers {
				if tier.Name != defaultTierName {
					targetTier = tier
					break
				}
			}
			reqLogger.Warn("Escalation budget exhausted, de-escalating",
				slog.String("fallback_tier", targetTier.Name),
				slog.String("fallback_model", targetTier.Model),
			)
		}
	} else {
		s.sessionTracker.ResetEscalation(sessionKey)
	}
	return targetTier
}

// applyFairyDustTierOverride replaces targetTier with the winning Fairy Dust frontier model
// when a checkpoint turn triggers.
func (s *Server) applyFairyDustTierOverride(
	reqCtx *contract.RequestContext,
	targetTier contract.Tier,
	cfg *contract.Config,
) contract.Tier {
	if !reqCtx.FairyDusted {
		return targetTier
	}
	reqCtx.NoCycleKiller = true
	reqCtx.NoKickstart = true
	reqCtx.NoShield = true
	for _, entry := range cfg.FairyDust.Entries {
		if entry.Name == reqCtx.FairyDustEntry {
			provider := entry.Provider
			if provider == "" {
				provider = cfg.DefaultTier.Provider
			}
			return contract.Tier{
				Name:            fmt.Sprintf("Fairy Dust: %s #%d (%s)", entry.Name, reqCtx.FairyDustCount, entry.Model),
				Model:           entry.Model,
				Provider:        provider,
				ReasoningEffort: entry.ReasoningEffort,
			}
		}
	}
	return targetTier
}

// isForcedProviderBlocked enforces circuit breaker blocks when a forced provider directive is used.
// Returns true if the circuit is OPEN and an error was written to w, indicating the caller must return.
func (s *Server) isForcedProviderBlocked(
	w http.ResponseWriter,
	reqCtx *contract.RequestContext,
	targetTier contract.Tier,
	body []byte,
	reqLogger *slog.Logger,
) bool {
	if reqCtx.ForcedTier == "" && reqCtx.ForcedModel == "" {
		return false
	}
	targetProvider, found := s.GetRegistry().Get(targetTier.Provider)
	if found && !s.allowProvider(targetProvider) {
		reqLogger.Warn("Forced tier provider circuit is OPEN, blocking request without fallback",
			slog.String("tier", targetTier.Name),
			slog.String("provider", targetTier.Provider),
		)
		alertMsg := RenderCircuitBlocked(targetTier.Name, targetTier.Provider)
		isStream := strings.Contains(string(body), `"stream":true`) || strings.Contains(string(body), `"stream": true`)
		if isStream {
			sse := &SSEMetaPresenter{}
			_ = sse.WriteResponse(w, alertMsg, fmt.Sprintf("circuit-blocked-%d", time.Now().UnixNano()))
		} else {
			jsonP := &JSONMetaPresenter{}
			_ = jsonP.WriteResponse(w, alertMsg, fmt.Sprintf("circuit-blocked-%d", time.Now().UnixNano()))
		}
		return true
	}
	return false
}

// logRoutingDecision emits standard structured slog telemetry for the selected tier and context.
func (s *Server) logRoutingDecision(
	sessionKey string,
	targetTier contract.Tier,
	reqCtx *contract.RequestContext,
	r *http.Request,
	reqLogger *slog.Logger,
) {
	reqLogger.Info("Routing request",
		slog.String("session_key", sessionKey),
		slog.String("tier", targetTier.Name),
		slog.String("model", targetTier.Model),
		slog.String("provider", targetTier.Provider),
		slog.Int("tokens", reqCtx.Tokens),
		slog.Int("raw_tokens", reqCtx.RawTokens),
		slog.Int("nts_tokens_saved", reqCtx.NTSTokensSaved),
		slog.Int("nts_bytes_saved", reqCtx.NTSBytesSaved),
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
}

// applyPromptInjections injects Read Slap idle-loop warnings and Fairy Dust quality checkpoint
// prompts into the request payload. Returns the updated body slice.
func (s *Server) applyPromptInjections(
	sessionKey string,
	reqCtx *contract.RequestContext,
	cfg *contract.Config,
	body []byte,
	reqLogger *slog.Logger,
) []byte {
	// Read Slap: inject prompt when idle loop detected
	if (reqCtx.SessionReadSlapped || reqCtx.SessionKickstarted) && !reqCtx.NoKickstart {
		prompt := contract.DefaultReadSlapPrompt
		if cfg.Kickstart.ReadSlap.PromptOverride != "" {
			prompt = cfg.Kickstart.ReadSlap.PromptOverride
		}
		body = injectCorrectionPrompt(body, prompt)
		reqLogger.Info("Read Slap: injected prompt",
			slog.Int("read_slap_count", reqCtx.SessionReadSlapCount),
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
	return body
}
