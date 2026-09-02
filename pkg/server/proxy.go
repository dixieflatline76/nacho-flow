package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/provider"
	"github.com/dixieflatline76/nacho-flow/pkg/router"
	"github.com/dixieflatline76/nacho-flow/pkg/router/shield"
	"github.com/dixieflatline76/nacho-flow/pkg/store"
	"github.com/dixieflatline76/nacho-flow/pkg/strategy"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
	"github.com/dixieflatline76/nacho-flow/pkg/tuner"
)

type runtimeState struct {
	config    *contract.Config
	evaluator contract.Evaluator
	registry  *provider.Registry
}

type Server struct {
	state          atomic.Pointer[runtimeState]
	classifier     contract.Classifier
	sanitizer      contract.Sanitizer
	oracle         *telemetry.PricingOracle
	tracker        *telemetry.StatsTracker
	sessionTracker *router.SessionTracker
	metaRegistry   *MetaRegistry
	shieldMgr      *shield.ShieldManager
	logger         *slog.Logger
	transport      *http.Transport
	ringBuffer     *telemetry.RingBufferSink
	eventBroker    *telemetry.EventBroker
	tuner          *tuner.CostPenaltyOptimizer
	diskStore      *store.DiskStore
	trafficLogPath string
	configPath     string
	startTime      time.Time
	mementoState   *runtimeState
	watchdogMu     sync.Mutex
	watchdogActive bool
	watchdogErrors atomic.Int32
}

// GetConfig returns the current active configuration atomically.
func (s *Server) GetConfig() *contract.Config {
	if st := s.state.Load(); st != nil && st.config != nil {
		return st.config
	}
	return &contract.Config{}
}

// GetEvaluator returns the current active tier evaluator atomically.
func (s *Server) GetEvaluator() contract.Evaluator {
	if st := s.state.Load(); st != nil && st.evaluator != nil {
		return st.evaluator
	}
	return nil
}

// GetRegistry returns the current active provider registry atomically.
func (s *Server) GetRegistry() *provider.Registry {
	if st := s.state.Load(); st != nil && st.registry != nil {
		return st.registry
	}
	return nil
}

// SetRingBuffer attaches a ring buffer sink for /api/v1/routes.
func (s *Server) SetRingBuffer(rb *telemetry.RingBufferSink) {
	s.ringBuffer = rb
}

// SetEventBroker attaches an SSE event broker for /api/v1/events.
func (s *Server) SetEventBroker(eb *telemetry.EventBroker) {
	s.eventBroker = eb
}

// SetConfigPath sets the path to config.yaml on disk.
func (s *Server) SetConfigPath(path string) {
	s.configPath = path
}

// SetTuner sets the optimizer for /api/v1/tune.
func (s *Server) SetTuner(t *tuner.CostPenaltyOptimizer) {
	s.tuner = t
}

// SetDiskStore sets the persistence disk store for stats snapshots.
func (s *Server) SetDiskStore(ds *store.DiskStore) {
	s.diskStore = ds
}

// SetTrafficLogPath sets the path to the traffic.jsonl log.
func (s *Server) SetTrafficLogPath(path string) {
	s.trafficLogPath = path
}

func (s *Server) armWatchdog(memento *runtimeState, duration time.Duration) {
	if memento == nil {
		return
	}
	s.watchdogMu.Lock()
	s.mementoState = memento
	s.watchdogActive = true
	s.watchdogErrors.Store(0)
	s.watchdogMu.Unlock()

	go func() {
		time.Sleep(duration)
		s.watchdogMu.Lock()
		defer s.watchdogMu.Unlock()
		s.watchdogActive = false
		s.mementoState = nil
	}()
}

func (s *Server) recordProxyError() {
	s.watchdogMu.Lock()
	defer s.watchdogMu.Unlock()
	if !s.watchdogActive || s.mementoState == nil {
		return
	}
	if s.watchdogErrors.Add(1) >= 3 {
		s.logger.Warn("Watchdog triggered: 3 consecutive upstream failures after config reload. Rolling back to previous configuration snapshot.")
		s.state.Store(s.mementoState)
		s.watchdogActive = false
		s.mementoState = nil
	}
}

func (s *Server) recordProxySuccess() {
	s.watchdogErrors.Store(0)
}

// NewServer creates a Server with default telemetry, registry, and logging components.
func NewServer(cfg *contract.Config, eval contract.Evaluator, class contract.Classifier, san contract.Sanitizer) *Server {
	return NewServerWithTelemetryAndRegistry(cfg, eval, class, san, nil, nil, nil, nil)
}

// NewServerWithTelemetry creates a Server with injected telemetry and logger.
func NewServerWithTelemetry(
	cfg *contract.Config,
	eval contract.Evaluator,
	class contract.Classifier,
	san contract.Sanitizer,
	oracle *telemetry.PricingOracle,
	tracker *telemetry.StatsTracker,
	logger *slog.Logger,
) *Server {
	return NewServerWithTelemetryAndRegistry(cfg, eval, class, san, oracle, tracker, nil, logger)
}

// NewServerWithTelemetryAndRegistry creates a Server with injected registry and telemetry components.
func NewServerWithTelemetryAndRegistry(
	cfg *contract.Config,
	eval contract.Evaluator,
	class contract.Classifier,
	san contract.Sanitizer,
	oracle *telemetry.PricingOracle,
	tracker *telemetry.StatsTracker,
	reg *provider.Registry,
	logger *slog.Logger,
) *Server {
	if cfg == nil {
		cfg = &contract.Config{}
	}
	if class == nil {
		class = router.NewClassifier()
	}
	if classWithSigs, ok := class.(interface{ SetErrorSignatures([]string) }); ok {
		classWithSigs.SetErrorSignatures(cfg.AgentShield.ErrorSignatures)
	}
	writeTools := cfg.CycleKiller.KickstartWriteTools
	if len(writeTools) == 0 && len(cfg.CycleBreaker.KickstartWriteTools) > 0 {
		writeTools = cfg.CycleBreaker.KickstartWriteTools
	}
	if classWithWriteTools, ok := class.(interface{ SetKickstartWriteTools([]string) }); ok {
		classWithWriteTools.SetKickstartWriteTools(writeTools)
	}
	if san == nil {
		san = router.NewSanitizer()
	}
	if eval == nil {
		eval, _ = strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier)
	}
	if oracle == nil {
		oracle = telemetry.NewPricingOracle()
	}
	if tracker == nil {
		tracker = telemetry.NewStatsTracker(1000)
	}
	if reg == nil {
		reg = provider.NewRegistryFromConfig(cfg)
	}
	if logger == nil {
		logger = slog.Default()
	}

	// High-throughput connection pool to prevent socket thread exhaustion under massive concurrency
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 60 * time.Second,
		}).DialContext,
		MaxIdleConns:          10000,
		MaxIdleConnsPerHost:   2000,
		MaxConnsPerHost:       0,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	var shieldMgr *shield.ShieldManager
	if cfg.AgentShield.Enabled == nil || *cfg.AgentShield.Enabled {
		if len(cfg.AgentShield.QuestionHeuristics) > 0 || len(cfg.AgentShield.ModeSwitchHeuristics) > 0 {
			shieldMgr = shield.NewShieldManager(cfg.AgentShield.QuestionHeuristics, cfg.AgentShield.ModeSwitchHeuristics)
		} else {
			shieldMgr = shield.NewDefaultShieldManager()
		}
	}

	srv := &Server{
		classifier:     class,
		sanitizer:      san,
		oracle:         oracle,
		tracker:        tracker,
		sessionTracker: router.NewSessionTracker(5 * time.Minute),
		metaRegistry:   NewMetaRegistry(),
		shieldMgr:      shieldMgr,
		logger:         logger,
		transport:      transport,
		tuner:          tuner.NewCostPenaltyOptimizer(),
		startTime:      time.Now(),
	}

	srv.state.Store(&runtimeState{
		config:    cfg,
		evaluator: eval,
		registry:  reg,
	})

	return srv
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Health check endpoint (always public for load balancers & monitoring)
	if r.URL.Path == contract.PathHealth || r.URL.Path == contract.PathV1Health {
		w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
		w.Header().Set("Server", contract.AppName+"/"+contract.Version)
		w.Header().Set("X-Nacho-Flow", "true")
		w.WriteHeader(http.StatusOK)
		uptime := ""
		if !s.startTime.IsZero() {
			uptime = time.Since(s.startTime).String()
		}
		respMap := map[string]interface{}{
			"status":  "ok",
			"app":     contract.AppName,
			"service": contract.AppName,
			"version": contract.Version,
		}
		if uptime != "" {
			respMap["uptime"] = uptime
		}
		_ = json.NewEncoder(w).Encode(respMap)
		return
	}

	// Discovery endpoint (public)
	if r.URL.Path == contract.PathAPIInfo {
		s.handleAPIInfo(w, r)
		return
	}

	// CORS Preflight for any management route
	if strings.HasPrefix(r.URL.Path, "/api/v1/") && r.Method == http.MethodOptions {
		if s.setCORS(w, r) {
			return
		}
	}

	// 0. Inbound Client Authentication (Dual-layer security for LAN / Tailscale)
	if s.GetConfig().AuthToken != "" && !s.authenticateClient(r) {
		w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"Invalid or missing gateway API key","type":"auth_error","code":"invalid_api_key"}}`))
		return
	}

	// Management REST API Endpoints (v0.6.0+)
	if strings.HasPrefix(r.URL.Path, "/api/v1/") {
		switch r.URL.Path {
		case contract.PathAPIEvents:
			s.handleAPIEvents(w, r)
			return
		case contract.PathAPIRoutes:
			s.handleAPIRoutes(w, r)
			return
		case contract.PathAPICircuits:
			s.handleAPICircuits(w, r)
			return
		case contract.PathAPICircuitsReset:
			s.handleAPICircuitsReset(w, r)
			return
		case contract.PathAPIPricing:
			s.handleAPIPricing(w, r)
			return
		case contract.PathAPIConfig:
			s.handleAPIConfig(w, r)
			return
		case contract.PathAPITune:
			s.handleAPITune(w, r)
			return
		case contract.PathAPIDeals:
			s.handleAPIDeals(w, r)
			return
		case contract.PathAPIStatsReset:
			s.handleAPIStatsReset(w, r)
			return
		case contract.PathAPIStatsRecalculate:
			s.handleAPIStatsRecalculate(w, r)
			return
		default:
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
	}

	// Expose /v1/stats endpoint
	if r.URL.Path == contract.PathStats {
		s.tracker.ServeHTTP(w, r)
		return
	}

	// Models listing endpoint (OpenAI compatible)
	if r.URL.Path == contract.PathModels {
		w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"nacho-hybrid","object":"model","owned_by":"spicebox.dev"}]}`))
		return
	}

	// Only process chat completions / completions endpoints for routing
	if !strings.HasSuffix(r.URL.Path, contract.PathChatCompletions) && !strings.HasSuffix(r.URL.Path, contract.PathCompletions) {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	startTime := time.Now()
	reqID := fmt.Sprintf("req-%d", startTime.UnixNano())
	reqLogger := s.logger.With(
		slog.String("request_id", reqID),
		slog.String("client_ip", r.RemoteAddr),
	)

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

	// 2. Intercept local meta directives (@nacho:help, @nacho:tiers, @nacho:status, @nacho:deals, @nacho:<typo>)
	if reqCtx.IsMetaDirective {
		isStream := strings.Contains(string(body), `"stream":true`) || strings.Contains(string(body), `"stream": true`)
		env := MetaEnv{
			Config:        s.GetConfig(),
			Stats:         s.tracker,
			Oracle:        s.oracle,
			Providers:     s.GetRegistry(),
			StartTime:     s.startTime,
			DaemonVersion: contract.Version,
		}
		if s.metaRegistry == nil {
			s.metaRegistry = NewMetaRegistry()
		}
		s.metaRegistry.Dispatch(w, r, reqCtx, env, s.sessionTracker, isStream)
		return
	}

	// 3. Track session retries for auto-escalation
	sessionKey := extractSessionKey(r)
	reqCtx.SessionKey = sessionKey
	promptHash := router.HashPrompt(reqCtx.Prompt)
	if s.sessionTracker == nil {
		s.sessionTracker = router.NewSessionTracker(5 * time.Minute)
	}

	// Pass tool progress signal from classifier to session tracker
	retries, isRetry := s.sessionTracker.RecordTurn(sessionKey, promptHash, reqCtx.HasToolProgress)
	reqCtx.CoolingDownModels = s.sessionTracker.GetCoolingDownModels(sessionKey)

	// Kickstart: detect semantic stall/idle loop across consecutive turns
	cfg := s.GetConfig()
	kickstartThreshold := cfg.CycleKiller.KickstartThreshold
	if kickstartThreshold == 0 && cfg.CycleBreaker.KickstartThreshold > 0 {
		kickstartThreshold = cfg.CycleBreaker.KickstartThreshold
	}
	if kickstartThreshold > 0 {
		kickstartProgress := reqCtx.HasToolProgress
		if cfg.CycleKiller.KickstartWriteOnly || cfg.CycleBreaker.KickstartWriteOnly {
			kickstartProgress = reqCtx.HasWriteProgress
		}
		kickstartCount, isKickstarted := s.sessionTracker.RecordKickstartState(sessionKey, kickstartProgress, kickstartThreshold)
		if isKickstarted {
			reqCtx.SessionKickstarted = true
			reqCtx.SessionKickstartCount = kickstartCount
			reqLogger.Warn("Kickstart: agent idling without tool progress",
				slog.Int("kickstart_count", kickstartCount),
				slog.Int("kickstart_threshold", kickstartThreshold),
				slog.Bool("write_only", cfg.CycleKiller.KickstartWriteOnly || cfg.CycleBreaker.KickstartWriteOnly),
				slog.String("session_key", sessionKey),
			)
		}

		// Kickstart max cap: force-escalate to default tier when kickstart count exceeds limit
		maxKS := cfg.CycleKiller.KickstartMaxCount
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
	if fdCfg.Enabled != nil && *fdCfg.Enabled && len(fdCfg.Entries) > 0 {
		// 1. Record write progress (single global counter, increments only on write turns)
		writeCount := s.sessionTracker.RecordWriteProgress(sessionKey, reqCtx.HasWriteProgress)

		// 2. Check each entry; collect the highest-priority candidate that triggers
		type fdCandidate struct {
			entry    contract.FairyDustEntry
			count    int
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
		slog.Int("history_errors", reqCtx.HistoryErrors),
		slog.Any("cooling_down_models", reqCtx.CoolingDownModels),
		slog.String("user_agent", r.Header.Get("User-Agent")),
	)

	// Kickstart: inject prompt when idle loop detected
	if reqCtx.SessionKickstarted {
		kickstartPrompt := contract.DefaultKickstartPrompt
		if cfg.CycleKiller.KickstartPrompt != "" {
			kickstartPrompt = cfg.CycleKiller.KickstartPrompt
		} else if cfg.CycleBreaker.KickstartPrompt != "" {
			kickstartPrompt = cfg.CycleBreaker.KickstartPrompt
		}
		body = injectCorrectionPrompt(body, kickstartPrompt)
		reqLogger.Info("Kickstart: injected prompt",
			slog.Int("kickstart_count", reqCtx.SessionKickstartCount),
		)
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

func (s *Server) forwardWithFallback(
	w http.ResponseWriter,
	r *http.Request,
	reqCtx contract.RequestContext,
	targetTier contract.Tier,
	body []byte,
	startTime time.Time,
	reqLogger *slog.Logger,
) {
	isFallback := false
	s.dispatchTier(w, r, reqCtx, targetTier, body, startTime, reqLogger, isFallback)
}

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
	if !globalShieldEnabled {
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

func (s *Server) dispatchTier(
	w http.ResponseWriter,
	r *http.Request,
	reqCtx contract.RequestContext,
	targetTier contract.Tier,
	body []byte,
	startTime time.Time,
	reqLogger *slog.Logger,
	isFallback bool,
) {
	globalShield := s.GetConfig().AgentShield.Enabled == nil || *s.GetConfig().AgentShield.Enabled
	activeFeatures := resolveFeatureFlags(reqCtx, targetTier, globalShield)
	reqCtx.Features = uint16(activeFeatures)
	// Check Circuit Breaker before connecting
	reg := s.GetRegistry()
	var targetProvider provider.LLMProvider
	var exists bool
	if reg != nil {
		targetProvider, exists = reg.Get(targetTier.Provider)
	}

	defaultTier := s.GetConfig().DefaultTier
	if exists && !s.allowProvider(targetProvider) {
		reqLogger.Warn("Target provider circuit breaker OPEN, bypassing to default tier",
			slog.String("provider", targetTier.Provider),
			slog.String("tier", targetTier.Name),
		)
		if !isFallback && defaultTier.Provider != "" && targetTier.Name != defaultTier.Name {
			s.dispatchTier(w, r, reqCtx, defaultTier, body, startTime, reqLogger, true)
			return
		}
	}

	if !exists {
		reqLogger.Error("Target provider not found in registry", slog.String("provider", targetTier.Provider))
		if !isFallback && defaultTier.Provider != "" && targetTier.Name != defaultTier.Name {
			s.dispatchTier(w, r, reqCtx, defaultTier, body, startTime, reqLogger, true)
			return
		}
		http.Error(w, fmt.Sprintf("Provider not found: %s", targetTier.Provider), http.StatusBadGateway)
		return
	}

	// Prepare payload for target model
	preparedBody := body
	var rawPayload map[string]interface{}
	if err := json.Unmarshal(body, &rawPayload); err == nil {
		rawPayload["model"] = targetTier.Model
		if targetTier.ReasoningEffort != "" {
			rawPayload["reasoning_effort"] = targetTier.ReasoningEffort
		}
		if reencoded, err := json.Marshal(rawPayload); err == nil {
			preparedBody = reencoded
		}
	}

	hasVision := strings.Contains(strings.ToLower(targetTier.Model), "vision") || strings.Contains(strings.ToLower(targetTier.Model), "flash") || targetTier.Provider == "openrouter"
	if targetTier.StripImages {
		hasVision = false
	}
	preparedBody, _ = s.sanitizer.SanitizePayload(preparedBody, hasVision)

	targetURL, err := url.Parse(targetProvider.BaseURL())
	if err != nil {
		reqLogger.Error("Invalid target URL for provider", slog.String("provider", targetTier.Provider), slog.Any("error", err))
		http.Error(w, fmt.Sprintf("Invalid target URL for provider %s: %v", targetTier.Provider, err), http.StatusInternalServerError)
		return
	}

	reqPath := r.URL.Path
	targetBasePath := strings.TrimRight(targetURL.Path, "/")
	if strings.HasSuffix(targetBasePath, "/v1") && strings.HasPrefix(reqPath, "/v1/") {
		reqPath = strings.TrimPrefix(reqPath, "/v1")
	}
	fullTargetURL := singleJoiningSlash(targetURL.String(), reqPath)
	// #nosec G704 - upstream proxy URL is constructed from validated registered provider base URL
	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, fullTargetURL, bytes.NewReader(preparedBody))
	if err != nil {
		reqLogger.Error("Failed to build upstream request", slog.Any("error", err))
		http.Error(w, "Internal proxy error", http.StatusInternalServerError)
		return
	}

	// Copy headers
	for k, vv := range r.Header {
		for _, v := range vv {
			outReq.Header.Add(k, v)
		}
	}
	outReq.Host = targetURL.Host

	// Provider Capability: AuthProvider
	if auth, ok := targetProvider.(provider.AuthProvider); ok {
		if apiKey := auth.GetAPIKey(); apiKey != "" {
			outReq.Header.Set(contract.HeaderAuthorization, "Bearer "+apiKey)
		}
	}
	// Provider Capability: HeaderProvider
	if hdr, ok := targetProvider.(provider.HeaderProvider); ok {
		for k, v := range hdr.GetHeaders() {
			outReq.Header.Set(k, v)
		}
	}
	outReq.Header.Set(contract.HeaderContentLength, strconv.Itoa(len(preparedBody)))

	resp, err := s.transport.RoundTrip(outReq)
	if err != nil || (resp != nil && resp.StatusCode >= 500) {
		s.recordProviderFailure(targetProvider)
		s.recordProxyError()
		if resp != nil {
			_ = resp.Body.Close()
		}
		reqLogger.Warn("Upstream provider failure",
			slog.String("tier", targetTier.Name),
			slog.String("provider", targetTier.Provider),
			slog.Any("error", err),
		)
		if !isFallback && defaultTier.Provider != "" && targetTier.Name != defaultTier.Name {
			reqLogger.Info("Retrying with default fallback tier", slog.String("fallback_tier", defaultTier.Name))
			s.dispatchTier(w, r, reqCtx, defaultTier, body, startTime, reqLogger, true)
			return
		}
		http.Error(w, fmt.Sprintf("Proxy error: %v", err), http.StatusBadGateway)
		return
	}

	isStreaming := strings.Contains(resp.Header.Get(contract.HeaderContentType), contract.ContentTypeEventStream)

	if isStreaming {
		normalizer := NewStreamNormalizer(resp.Body)
		normalizer.SetFeatures(reqCtx.Features)
		if activeFeatures.Has(router.FeatureShieldEnabled) && reqCtx.InteractiveTool != "" {
			normalizer.SetShield(reqCtx.InteractiveTool, s.shieldMgr)
		}
		var cb *shield.CycleBreaker
		if reqCtx.HasTools {
			cb = resolveCycleBreaker(targetTier, s.GetConfig())
			if cb != nil && cb.IsEnabled() {
				normalizer.SetCycleBreaker(cb)
			}
		}

		peekBytes := make([]byte, 0, 4096)
		tempBuf := make([]byte, 512)
		var readErr error
		for len(peekBytes) < 2048 {
			n, err := normalizer.Read(tempBuf)
			if n > 0 {
				peekBytes = append(peekBytes, tempBuf[:n]...)
			}
			if err != nil {
				readErr = err
				break
			}
			if violated, _ := normalizer.CheckCycleViolation(); violated {
				break
			}
		}

		// Check quality defect on stream: immediate [DONE] on local provider
		trimmedPeek := strings.TrimSpace(string(peekBytes))
		if targetProvider.IsLocal() && !isFallback && (trimmedPeek == "data: [DONE]" || (len(peekBytes) == 0 && readErr == io.EOF)) {
			_ = normalizer.Close()
			reqLogger.Warn("Local provider returned empty stream, failing over to cloud fallback tier",
				slog.String("tier", targetTier.Name),
			)
			if defaultTier.Provider != "" && targetTier.Name != defaultTier.Name {
				s.dispatchTier(w, r, reqCtx, defaultTier, body, startTime, reqLogger, true)
				return
			}
		}

		// Check Cycle Breaker violation on stream peek (Phase 1)
		if cb != nil && cb.IsEnabled() && !isFallback {
			if violated, reason := normalizer.CheckCycleViolation(); violated {
				_ = normalizer.Close()
				reqCtx.CycleBreakerTriggered = true
				reqCtx.CycleBreakerReason = reason
				reqLogger.Warn("Cycle killer (qu'est-ce que c'est?): Runaway monologue intercepted",
					slog.String("reason", reason),
					slog.String("tier", targetTier.Name),
					slog.Int("cycle_retries", reqCtx.CycleRetries),
				)

				if reqCtx.CycleRetries < cb.MaxRetries() {
					// Stage 1: Local self-correction retry ($0.00) with [SYSTEM OVERRIDE] prompt
					reqCtx.CycleRetries++
					injectedBody := injectCorrectionPrompt(body, cb.CorrectionPrompt())
					s.dispatchTier(w, r, reqCtx, targetTier, injectedBody, startTime, reqLogger, false)
					return
				}

				if s.sessionTracker != nil {
					cooldown, floor := s.resolveCycleKillParams()
					s.sessionTracker.RecordCycleKill(extractSessionKey(r), targetTier.Model, cooldown, floor)
				}

				// Stage 2: Cloud Failover after local retry exhaustion
				if defaultTier.Provider != "" && targetTier.Name != defaultTier.Name {
					reqLogger.Warn("Local cycle retries exhausted, escalating to cloud fallback tier",
						slog.String("fallback_tier", defaultTier.Name),
						slog.String("fallback_model", defaultTier.Model),
					)
					s.dispatchTier(w, r, reqCtx, defaultTier, body, startTime, reqLogger, true)
					return
				}
			}
		}

		s.recordProviderSuccess(targetProvider)
		s.recordProxySuccess()

		// Copy headers to client
		for k, vv := range resp.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.Header().Set(contract.HeaderNachoRouterTier, targetTier.Name)
		w.Header().Set(contract.HeaderSpiceRouterTier, targetTier.Name)
		w.Header().Set(contract.HeaderNachoTargetModel, targetTier.Model)
		w.Header().Set(contract.HeaderSpiceTargetModel, targetTier.Model)
		w.WriteHeader(resp.StatusCode)

		if len(peekBytes) > 0 {
			_, _ = w.Write(peekBytes)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
		if readErr == nil {
			buf := make([]byte, 4096)
			for {
				n, err := normalizer.Read(buf)
				// Phase 2: Active Mid-Stream Circuit Severing (Check BEFORE writing to swallow degenerate chunks)
				if violated, reason := normalizer.CheckCycleViolation(); violated && cb != nil && cb.IsEnabled() {
					reqLogger.Warn("Cycle killer (qu'est-ce que c'est?): Severing runaway stream",
						slog.String("reason", reason),
						slog.String("tier", targetTier.Name),
					)
					reqCtx.CycleBreakerTriggered = true
					reqCtx.CycleBreakerReason = reason
					if s.sessionTracker != nil {
						cooldown, floor := s.resolveCycleKillParams()
						s.sessionTracker.RecordCycleKill(extractSessionKey(r), targetTier.Model, cooldown, floor)
					}
					_ = normalizer.Close()
					finishChunk := "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
					_, _ = w.Write([]byte(finishChunk))
					if flusher, ok := w.(http.Flusher); ok {
						flusher.Flush()
					}
					break
				}
				if n > 0 {
					_, _ = w.Write(buf[:n])
					if flusher, ok := w.(http.Flusher); ok {
						flusher.Flush()
					}
				}
				if err != nil {
					break
				}
			}
		}
		usage, _ := normalizer.GetUsage()
		if cb != nil {
			reqCtx.CycleProseTokens = cb.ProseTokens()
			reqCtx.CycleMaxNgramFreq = cb.MaxNgramFreq()
			reqCtx.CycleThinkingTokens = cb.ThinkingTokens()
			reqCtx.CycleMaxThinkingNgramFreq = cb.MaxThinkingNgramFreq()
		}
		_ = normalizer.Close()

		s.recordTelemetry(targetTier, targetProvider, reqCtx, usage, resp.StatusCode, startTime, isFallback, reqLogger)
		return
	}

	// Non-streaming response
	bodyBytes, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		s.recordProxyError()
		http.Error(w, "Failed to read upstream response", http.StatusBadGateway)
		return
	}

	// Quality Validation: Empty content from local provider -> Transparent Fallback
	if targetProvider.IsLocal() && !isFallback && isDefectiveEmptyContent(bodyBytes) {
		reqLogger.Warn("Local provider returned defective empty content, failing over to cloud fallback tier",
			slog.String("tier", targetTier.Name),
		)
		if defaultTier.Provider != "" && targetTier.Name != defaultTier.Name {
			s.dispatchTier(w, r, reqCtx, defaultTier, body, startTime, reqLogger, true)
			return
		}
	}

	s.recordProviderSuccess(targetProvider)
	s.recordProxySuccess()

	var nonStreamUsage StreamUsage

	// Normalize non-streaming tools & reasoning
	if resp.StatusCode == http.StatusOK && strings.Contains(resp.Header.Get(contract.HeaderContentType), contract.ContentTypeJSON) {
		if activeFeatures != router.FeatureRawPassThrough {
			var completionResp fastChatCompletionResponse
			if json.Unmarshal(bodyBytes, &completionResp) == nil {
				if len(completionResp.Usage) > 0 {
					_ = json.Unmarshal(completionResp.Usage, &nonStreamUsage)
				}
				if len(completionResp.Choices) > 0 {
					firstChoice := &completionResp.Choices[0]
					modified := false

					if activeFeatures.Has(router.FeatureThinkNormalizer) {
						reasoningText := firstChoice.Message.ReasoningContent
						if reasoningText == "" {
							reasoningText = firstChoice.Message.Reasoning
						}
						if reasoningText == "" {
							reasoningText = firstChoice.Message.Reason
						}
						if reasoningText != "" && !strings.Contains(firstChoice.Message.Content, "<think>") {
							firstChoice.Message.Content = "<think>\n" + reasoningText + "\n</think>\n\n" + firstChoice.Message.Content
							firstChoice.Message.ReasoningContent = ""
							firstChoice.Message.Reasoning = ""
							firstChoice.Message.Reason = ""
							modified = true
						}
					}

					if activeFeatures.Has(router.FeatureToolNormalizer) && reqCtx.HasTools && len(firstChoice.Message.ToolCalls) == 0 && firstChoice.Message.Content != "" {
						cleanedText, extractedCalls, parsed := router.NormalizeMarkdownToolCalls(firstChoice.Message.Content)
						if parsed && len(extractedCalls) > 0 {
							firstChoice.Message.Content = cleanedText
							firstChoice.FinishReason = "tool_calls"
							rawCallsJSON, _ := json.Marshal(extractedCalls)
							firstChoice.Message.ToolCalls = rawCallsJSON
							modified = true
						} else if activeFeatures.Has(router.FeatureShieldEnabled) && reqCtx.InteractiveTool != "" && s.shieldMgr != nil {
							if synthCall, ok := s.shieldMgr.EvaluateAndSynthesize(firstChoice.Message.Content, reqCtx.InteractiveTool); ok && synthCall != nil {
								firstChoice.FinishReason = "tool_calls"
								rawCallsJSON, _ := json.Marshal([]interface{}{synthCall})
								firstChoice.Message.ToolCalls = rawCallsJSON
								modified = true
							}
						}
					}

					if len(firstChoice.Message.ToolCalls) == 0 && firstChoice.Message.Content != "" && reqCtx.HasTools && !isFallback {
						if cb := resolveCycleBreaker(targetTier, s.GetConfig()); cb != nil && cb.IsEnabled() {
							if triggered, reason := cb.ProcessDelta(firstChoice.Message.Content, false); triggered {
								reqCtx.CycleBreakerTriggered = true
								reqCtx.CycleBreakerReason = reason
								reqLogger.Warn("Cycle killer (qu'est-ce que c'est?): Runaway monologue intercepted",
									slog.String("reason", reason),
									slog.String("tier", targetTier.Name),
									slog.Int("cycle_retries", reqCtx.CycleRetries),
								)
							}
							reqCtx.CycleProseTokens = cb.ProseTokens()
							reqCtx.CycleMaxNgramFreq = cb.MaxNgramFreq()
							reqCtx.CycleThinkingTokens = cb.ThinkingTokens()
							reqCtx.CycleMaxThinkingNgramFreq = cb.MaxThinkingNgramFreq()
							if reqCtx.CycleBreakerTriggered {
								if reqCtx.CycleRetries < cb.MaxRetries() {
									reqCtx.CycleRetries++
									injectedBody := injectCorrectionPrompt(body, cb.CorrectionPrompt())
									s.dispatchTier(w, r, reqCtx, targetTier, injectedBody, startTime, reqLogger, false)
									return
								}
								if s.sessionTracker != nil {
									cooldown, floor := s.resolveCycleKillParams()
									s.sessionTracker.RecordCycleKill(extractSessionKey(r), targetTier.Model, cooldown, floor)
								}
								if defaultTier.Provider != "" && targetTier.Name != defaultTier.Name {
									reqLogger.Warn("Local cycle retries exhausted, escalating to cloud fallback tier",
										slog.String("fallback_tier", defaultTier.Name),
										slog.String("fallback_model", defaultTier.Model),
									)
									s.dispatchTier(w, r, reqCtx, defaultTier, body, startTime, reqLogger, true)
									return
								}
							}
						}
					}

					if modified {
						if newJSON, err := marshalNoEscapeHTML(completionResp); err == nil {
							bodyBytes = newJSON
						}
					}

					// Calibrate Adaptive Token Estimator if usage prompt tokens reported
					if nonStreamUsage.PromptTokens > 0 {
						if calibrator, ok := s.classifier.(interface{ GetEstimator() *router.TokenEstimator }); ok {
							calibrator.GetEstimator().Calibrate(nonStreamUsage.PromptTokens, len(body))
						}
					}
				}
			}
		}
	}

	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.Header().Set(contract.HeaderNachoRouterTier, targetTier.Name)
	w.Header().Set(contract.HeaderSpiceRouterTier, targetTier.Name)
	w.Header().Set(contract.HeaderNachoTargetModel, targetTier.Model)
	w.Header().Set(contract.HeaderSpiceTargetModel, targetTier.Model)
	w.Header().Set(contract.HeaderContentLength, strconv.Itoa(len(bodyBytes)))
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(bodyBytes)

	s.recordTelemetry(targetTier, targetProvider, reqCtx, nonStreamUsage, resp.StatusCode, startTime, isFallback, reqLogger)
}

func (s *Server) allowProvider(p provider.LLMProvider) bool {
	if cbProvider, ok := p.(provider.CircuitBreakerProvider); ok {
		return cbProvider.CircuitBreaker().AllowRequest()
	}
	return true
}

func (s *Server) recordProviderFailure(p provider.LLMProvider) {
	if cbProvider, ok := p.(provider.CircuitBreakerProvider); ok {
		cbProvider.CircuitBreaker().RecordFailure()
	}
}

func (s *Server) recordProviderSuccess(p provider.LLMProvider) {
	if cbProvider, ok := p.(provider.CircuitBreakerProvider); ok {
		cbProvider.CircuitBreaker().RecordSuccess()
	}
}

func isDefectiveEmptyContent(bodyBytes []byte) bool {
	var raw map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &raw); err != nil {
		return false
	}
	choices, ok := raw["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return true
	}
	firstChoice, ok := choices[0].(map[string]interface{})
	if !ok {
		return true
	}
	msg, ok := firstChoice["message"].(map[string]interface{})
	if !ok {
		return true
	}
	content, _ := msg["content"].(string)
	tools, hasTools := msg["tool_calls"].([]interface{})
	reasoning, _ := msg["reasoning_content"].(string)
	if reasoning == "" {
		reasoning, _ = msg["reasoning"].(string)
	}
	return strings.TrimSpace(content) == "" && (!hasTools || len(tools) == 0) && strings.TrimSpace(reasoning) == ""
}

func (s *Server) recordTelemetry(
	targetTier contract.Tier,
	targetProvider provider.LLMProvider,
	reqCtx contract.RequestContext,
	usage StreamUsage,
	statusCode int,
	startTime time.Time,
	isFallback bool,
	reqLogger *slog.Logger,
) {
	latency := float64(time.Since(startTime).Milliseconds())
	isLocal := targetProvider.IsLocal()

	promptTokens := reqCtx.Tokens
	if usage.PromptTokens > 0 {
		promptTokens = usage.PromptTokens
	}
	completionTokens := usage.CompletionTokens
	totalTokens := usage.TotalTokens
	if totalTokens == 0 {
		totalTokens = promptTokens + completionTokens
	}

	// Baseline rates are resolved internally by PricingOracle.resolveBenchmarkRates().
	const baselineRatePerM = 0.0

	cachedTokens := 0
	if usage.PromptTokensDetails != nil {
		cachedTokens = usage.PromptTokensDetails.CachedTokens
	}
	upstreamCost := usage.Cost

	costSpent, costSaved := s.oracle.CalculateFinancials(targetTier.Provider, targetTier.Model, isLocal, promptTokens, completionTokens, cachedTokens, upstreamCost, baselineRatePerM)

	tierNum := 2
	if isLocal {
		tierNum = 1
	} else {
		switch {
		case strings.Contains(strings.ToLower(targetTier.Name), "tier 3") || strings.Contains(strings.ToLower(targetTier.Name), "reasoning") || strings.Contains(strings.ToLower(targetTier.Name), "rescue") || strings.Contains(strings.ToLower(targetTier.Name), "sonnet"):
			tierNum = 3
		case strings.Contains(strings.ToLower(targetTier.Name), "tier 4") || strings.Contains(strings.ToLower(targetTier.Name), "vision") || strings.Contains(strings.ToLower(targetTier.Model), "vision"):
			tierNum = 4
		default:
			tierNum = 2
		}
	}

	s.tracker.Record(telemetry.Observation{
		Tier:                  tierNum,
		TierName:              targetTier.Name,
		Model:                 targetTier.Model,
		Provider:              targetTier.Provider,
		Tokens:                totalTokens,
		CostSpent:             costSpent,
		CostSaved:             costSaved,
		IsLocal:               isLocal,
		IsFallback:            isFallback,
		LatencyMs:             latency,
		Keywords:              reqCtx.Keywords,
		HasImages:             reqCtx.HasImages,
		HasTools:              reqCtx.HasTools,
		StatusCode:            statusCode,
		IsRetry:               reqCtx.IsRetry,
		ForcedTier:            reqCtx.ForcedTier,
		ForcedModel:           reqCtx.ForcedModel,
		DirectiveUsed:             reqCtx.MetaDirectiveRaw,
		CycleBreakerTriggered:     reqCtx.CycleBreakerTriggered,
		CycleBreakerReason:        reqCtx.CycleBreakerReason,
		CycleProseTokens:          reqCtx.CycleProseTokens,
		CycleMaxNgramFreq:         reqCtx.CycleMaxNgramFreq,
		CycleThinkingTokens:       reqCtx.CycleThinkingTokens,
		CycleMaxThinkingNgramFreq: reqCtx.CycleMaxThinkingNgramFreq,
		SessionKickstarted:        reqCtx.SessionKickstarted,
		CachedTokens:              cachedTokens,
		UpstreamCost:              upstreamCost,
		FairyDusted:               reqCtx.FairyDusted,
		FairyDustEntry:            reqCtx.FairyDustEntry,
	})

	reqLogger.Info("Completed proxy request",
		slog.String("session_key", reqCtx.SessionKey),
		slog.String("tier", targetTier.Name),
		slog.String("model", targetTier.Model),
		slog.Int("tokens", totalTokens),
		slog.Float64("latency_ms", latency),
		slog.Int("status", statusCode),
		slog.Bool("is_fallback", isFallback),
		slog.Bool("is_retry", reqCtx.IsRetry),
		slog.Bool("cycle_breaker_triggered", reqCtx.CycleBreakerTriggered),
		slog.Int("cycle_prose_tokens", reqCtx.CycleProseTokens),
		slog.Int("cycle_max_ngram_freq", reqCtx.CycleMaxNgramFreq),
		slog.Int("cycle_thinking_tokens", reqCtx.CycleThinkingTokens),
		slog.Int("cycle_max_thinking_ngram_freq", reqCtx.CycleMaxThinkingNgramFreq),
		slog.Bool("session_kickstarted", reqCtx.SessionKickstarted),
		slog.Int("session_kickstart_count", reqCtx.SessionKickstartCount),
		slog.Bool("fairy_dusted", reqCtx.FairyDusted),
		slog.String("fairy_dust_entry", reqCtx.FairyDustEntry),
		slog.Int("fairy_dust_count", reqCtx.FairyDustCount),
	)
}

func singleJoiningSlash(a, b string) string {
	aslashes := strings.HasSuffix(a, "/")
	bslashes := strings.HasPrefix(b, "/")
	switch {
	case aslashes && bslashes:
		return a + b[1:]
	case !aslashes && !bslashes:
		return a + "/" + b
	}
	return a + b
}

// authenticateClient checks if the incoming request carries a valid Bearer token or API key.
func (s *Server) authenticateClient(r *http.Request) bool {
	expected := s.GetConfig().AuthToken
	if expected == "" {
		return true
	}

	authHeader := r.Header.Get(contract.HeaderAuthorization)
	if strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == expected {
			return true
		}
	}

	// Also check X-API-Key or api-key headers
	if r.Header.Get(contract.HeaderXAPIKey) == expected || r.Header.Get(contract.HeaderAPIKey) == expected {
		return true
	}

	return false
}

type fastMessage struct {
	Role             string          `json:"role"`
	Content          string          `json:"content"`
	ReasoningContent string          `json:"reasoning_content,omitempty"`
	Reasoning        string          `json:"reasoning,omitempty"`
	Reason           string          `json:"reason,omitempty"`
	ToolCalls        json.RawMessage `json:"tool_calls,omitempty"`
}

type fastChoice struct {
	Index        int             `json:"index"`
	Message      fastMessage     `json:"message"`
	FinishReason string          `json:"finish_reason"`
	Logprobs     json.RawMessage `json:"logprobs,omitempty"`
}

type fastChatCompletionResponse struct {
	ID                string          `json:"id"`
	Object            string          `json:"object"`
	Created           int64           `json:"created"`
	Model             string          `json:"model"`
	Choices           []fastChoice    `json:"choices"`
	Usage             json.RawMessage `json:"usage,omitempty"`
	SystemFingerprint string          `json:"system_fingerprint,omitempty"`
}

func hasCandidateToolTokens(b []byte) bool {
	return bytes.Contains(b, []byte("<tool_call>")) ||
		bytes.Contains(b, []byte("[TOOL_CALLS]")) ||
		bytes.Contains(b, []byte("<function=")) ||
		bytes.Contains(b, []byte("<|python_tag|>")) ||
		bytes.Contains(b, []byte("<invoke")) ||
		bytes.Contains(b, []byte("Action:")) ||
		bytes.Contains(b, []byte("```"))
}

func resolveCycleBreaker(tier contract.Tier, cfg *contract.Config) *shield.CycleBreaker {
	var cbCfg contract.CycleBreakerConfig
	if cfg != nil {
		cbCfg = cfg.CycleKiller
		if cbCfg.Enabled == nil && cfg.CycleBreaker.Enabled != nil {
			cbCfg = cfg.CycleBreaker
		}
	}

	// Support both cycle_killer and cycle_breaker on tier level
	tierCb := tier.CycleKiller
	if tierCb == nil {
		tierCb = tier.CycleBreaker
	}

	if tierCb != nil {
		if tierCb.Enabled != nil {
			cbCfg.Enabled = tierCb.Enabled
		}
		if tierCb.MaxProseTokens > 0 {
			cbCfg.MaxProseTokens = tierCb.MaxProseTokens
		}
		if tierCb.RepetitionWindow > 0 {
			cbCfg.RepetitionWindow = tierCb.RepetitionWindow
		}
		if tierCb.RepetitionThreshold > 0 {
			cbCfg.RepetitionThreshold = tierCb.RepetitionThreshold
		}
		if tierCb.MaxRetries > 0 {
			cbCfg.MaxRetries = tierCb.MaxRetries
		}
		if tierCb.CorrectionPrompt != "" {
			cbCfg.CorrectionPrompt = tierCb.CorrectionPrompt
		}
	}
	if cbCfg.Enabled != nil && !*cbCfg.Enabled {
		return nil
	}
	return shield.NewCycleBreaker(&cbCfg)
}

func (s *Server) resolveCycleKillParams() (time.Duration, int) {
	cfg := s.GetConfig()
	cooldownSec := cfg.CycleKiller.ModelCooldownSeconds
	if cooldownSec == 0 && cfg.CycleBreaker.ModelCooldownSeconds > 0 {
		cooldownSec = cfg.CycleBreaker.ModelCooldownSeconds
	}
	cooldown := router.DefaultModelCooldown
	if cooldownSec > 0 {
		cooldown = time.Duration(cooldownSec) * time.Second
	}

	floor := cfg.CycleKiller.RetryFloor
	if floor == 0 && cfg.CycleBreaker.RetryFloor > 0 {
		floor = cfg.CycleBreaker.RetryFloor
	}
	if floor <= 0 {
		floor = 3
	}
	return cooldown, floor
}

func injectCorrectionPrompt(body []byte, prompt string) []byte {
	if prompt == "" {
		prompt = contract.CycleBreakerDefaultCorrectionPrompt
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return body
	}

	messages, ok := payload["messages"].([]interface{})
	if !ok {
		return body
	}

	overrideMsg := map[string]interface{}{
		"role":    "user",
		"content": prompt,
	}
	payload["messages"] = append(messages, overrideMsg)

	reencoded, err := json.Marshal(payload)
	if err != nil {
		return body
	}
	return reencoded
}

// extractSessionKey extracts a consistent session identifier from HTTP headers or client IP.
// If x-session-id or session-id headers are absent, it extracts the host IP from RemoteAddr
// or proxy headers (X-Forwarded-For, X-Real-IP) rather than raw IP:port so that ephemeral
// client TCP ports do not break cross-turn retry counting and kickstart state.
func extractSessionKey(r *http.Request) string {
	if s := r.Header.Get("x-session-id"); s != "" {
		return s
	}
	if s := r.Header.Get("session-id"); s != "" {
		return s
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if ip := strings.TrimSpace(parts[0]); ip != "" {
			return ip
		}
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		if ip := strings.TrimSpace(xri); ip != "" {
			return ip
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
