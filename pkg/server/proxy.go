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
	logger         *slog.Logger
	transport      *http.Transport
	ringBuffer     *telemetry.RingBufferSink
	eventBroker    *telemetry.EventBroker
	tuner          *tuner.CostPenaltyOptimizer
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

	srv := &Server{
		classifier:     class,
		sanitizer:      san,
		oracle:         oracle,
		tracker:        tracker,
		sessionTracker: router.NewSessionTracker(5 * time.Minute),
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
		w.WriteHeader(http.StatusOK)
		resp := fmt.Sprintf(`{"status":"ok","service":"%s","version":"%s"}`, contract.AppName, contract.Version)
		_, _ = w.Write([]byte(resp))
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

	// 2. Track session retries for auto-escalation
	sessionKey := r.Header.Get("x-session-id")
	if sessionKey == "" {
		sessionKey = r.Header.Get("session-id")
	}
	if sessionKey == "" {
		sessionKey = r.RemoteAddr
	}
	promptHash := router.HashPrompt(reqCtx.Prompt)
	if s.sessionTracker == nil {
		s.sessionTracker = router.NewSessionTracker(5 * time.Minute)
	}
	retries, isRetry := s.sessionTracker.RecordTurn(sessionKey, promptHash)
	reqCtx.Retries = retries
	reqCtx.IsRetry = isRetry

	// 3. Evaluate 1..N tiers using expr engine
	targetTier, err := s.GetEvaluator().SelectTier(reqCtx)
	if err != nil {
		reqLogger.Error("Error evaluating tier, falling back to default", slog.Any("error", err))
		targetTier = s.GetConfig().DefaultTier
	}

	reqLogger.Info("Routing request",
		slog.String("tier", targetTier.Name),
		slog.String("model", targetTier.Model),
		slog.String("provider", targetTier.Provider),
		slog.Int("tokens", reqCtx.Tokens),
		slog.Bool("has_images", reqCtx.HasImages),
		slog.Bool("has_tools", reqCtx.HasTools),
		slog.Int("retries", reqCtx.Retries),
	)

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

	fullTargetURL := singleJoiningSlash(targetURL.String(), r.URL.Path)
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
		peekBuf := make([]byte, 4096)
		n, readErr := normalizer.Read(peekBuf)

		// Check quality defect on stream: immediate [DONE] on local provider
		trimmedPeek := strings.TrimSpace(string(peekBuf[:n]))
		if targetProvider.IsLocal() && !isFallback && (trimmedPeek == "data: [DONE]" || (n == 0 && readErr == io.EOF)) {
			_ = normalizer.Close()
			reqLogger.Warn("Local provider returned empty stream, failing over to cloud fallback tier",
				slog.String("tier", targetTier.Name),
			)
			if defaultTier.Provider != "" && targetTier.Name != defaultTier.Name {
				s.dispatchTier(w, r, reqCtx, defaultTier, body, startTime, reqLogger, true)
				return
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

		if n > 0 {
			_, _ = w.Write(peekBuf[:n])
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
		if readErr == nil {
			_, _ = io.Copy(w, normalizer)
		}
		_ = normalizer.Close()

		s.recordTelemetry(targetTier, targetProvider, reqCtx, resp.StatusCode, startTime, isFallback, reqLogger)
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

	// Normalize non-streaming tools & reasoning
	if resp.StatusCode == http.StatusOK && strings.Contains(resp.Header.Get(contract.HeaderContentType), contract.ContentTypeJSON) {
		if hasCandidateToolTokens(bodyBytes) || bytes.Contains(bodyBytes, []byte("reasoning")) {
			var completionResp fastChatCompletionResponse
			if json.Unmarshal(bodyBytes, &completionResp) == nil && len(completionResp.Choices) > 0 {
				firstChoice := &completionResp.Choices[0]
				modified := false

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

				if reqCtx.HasTools && len(firstChoice.Message.ToolCalls) == 0 && firstChoice.Message.Content != "" {
					cleanedText, extractedCalls, parsed := router.NormalizeMarkdownToolCalls(firstChoice.Message.Content)
					if parsed && len(extractedCalls) > 0 {
						firstChoice.Message.Content = cleanedText
						firstChoice.FinishReason = "tool_calls"
						rawCallsJSON, _ := json.Marshal(extractedCalls)
						firstChoice.Message.ToolCalls = rawCallsJSON
						modified = true
					}
				}

				if modified {
					if newJSON, err := marshalNoEscapeHTML(completionResp); err == nil {
						bodyBytes = newJSON
					}
				}

				// Calibrate Adaptive Token Estimator if usage prompt tokens reported
				if len(completionResp.Usage) > 0 {
					var usageData struct {
						PromptTokens int `json:"prompt_tokens"`
					}
					if json.Unmarshal(completionResp.Usage, &usageData) == nil && usageData.PromptTokens > 0 {
						if calibrator, ok := s.classifier.(interface{ GetEstimator() *router.TokenEstimator }); ok {
							calibrator.GetEstimator().Calibrate(usageData.PromptTokens, len(body))
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

	s.recordTelemetry(targetTier, targetProvider, reqCtx, resp.StatusCode, startTime, isFallback, reqLogger)
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
	statusCode int,
	startTime time.Time,
	isFallback bool,
	reqLogger *slog.Logger,
) {
	latency := float64(time.Since(startTime).Milliseconds())
	isLocal := targetProvider.IsLocal()
	var costSaved float64

	if reqCtx.Tokens > 0 {
		baselineRatePerM := contract.DefaultBenchmarkPricePerMillion
		if baselinePricing, found := s.oracle.GetPrice("openrouter", contract.DefaultBenchmarkModel); found && baselinePricing.PromptCostPerMillion > 0 {
			baselineRatePerM = baselinePricing.PromptCostPerMillion
		}
		if isLocal {
			costSaved = (float64(reqCtx.Tokens) / 1_000_000.0) * baselineRatePerM
		} else {
			if tierPricing, found := s.oracle.GetPrice(targetTier.Provider, targetTier.Model); found {
				if baselineRatePerM > tierPricing.PromptCostPerMillion {
					costSaved = (float64(reqCtx.Tokens) / 1_000_000.0) * (baselineRatePerM - tierPricing.PromptCostPerMillion)
				}
			}
		}
	}

	tierNum := 1
	switch {
	case strings.Contains(strings.ToLower(targetTier.Name), "tier 2") || strings.Contains(strings.ToLower(targetTier.Name), "coder"):
		tierNum = 2
	case strings.Contains(strings.ToLower(targetTier.Name), "tier 3") || strings.Contains(strings.ToLower(targetTier.Name), "reasoning"):
		tierNum = 3
	case strings.Contains(strings.ToLower(targetTier.Name), "tier 4") || strings.Contains(strings.ToLower(targetTier.Model), "vision"):
		tierNum = 4
	case isLocal:
		tierNum = 1
	default:
		tierNum = 2
	}

	s.tracker.Record(telemetry.Observation{
		Tier:       tierNum,
		TierName:   targetTier.Name,
		Model:      targetTier.Model,
		Provider:   targetTier.Provider,
		Tokens:     reqCtx.Tokens,
		CostSaved:  costSaved,
		IsLocal:    isLocal,
		IsFallback: isFallback,
		LatencyMs:  latency,
		Keywords:   reqCtx.Keywords,
		HasImages:  reqCtx.HasImages,
		HasTools:   reqCtx.HasTools,
		StatusCode: statusCode,
		IsRetry:    reqCtx.IsRetry,
	})

	reqLogger.Info("Completed proxy request",
		slog.String("tier", targetTier.Name),
		slog.String("model", targetTier.Model),
		slog.Int("tokens", reqCtx.Tokens),
		slog.Float64("latency_ms", latency),
		slog.Int("status", statusCode),
		slog.Bool("is_fallback", isFallback),
		slog.Bool("is_retry", reqCtx.IsRetry),
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
