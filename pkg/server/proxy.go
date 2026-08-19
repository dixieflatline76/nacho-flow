package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/provider"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
)

type Server struct {
	config     *contract.Config
	evaluator  contract.Evaluator
	classifier contract.Classifier
	sanitizer  contract.Sanitizer
	oracle     *telemetry.PricingOracle
	tracker    *telemetry.StatsTracker
	registry   *provider.Registry
	logger     *slog.Logger
	transport  *http.Transport
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

	return &Server{
		config:     cfg,
		evaluator:  eval,
		classifier: class,
		sanitizer:  san,
		oracle:     oracle,
		tracker:    tracker,
		registry:   reg,
		logger:     logger,
		transport:  transport,
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 0. Expose /v1/stats endpoint
	if r.URL.Path == "/v1/stats" {
		s.tracker.ServeHTTP(w, r)
		return
	}

	// Health check endpoint
	if r.URL.Path == "/health" || r.URL.Path == "/v1/health" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","service":"nacho-flow"}`))
		return
	}

	// Models listing endpoint (OpenAI compatible)
	if r.URL.Path == "/v1/models" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"nacho-hybrid","object":"model","owned_by":"spicebox.dev"}]}`))
		return
	}

	// Only process chat completions / completions endpoints for routing
	if !strings.HasSuffix(r.URL.Path, "/chat/completions") && !strings.HasSuffix(r.URL.Path, "/completions") {
		http.Error(w, "Not found", http.StatusNotFound)
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

	// 2. Evaluate 1..N tiers using expr engine
	targetTier, err := s.evaluator.SelectTier(reqCtx)
	if err != nil {
		reqLogger.Error("Error evaluating tier, falling back to default", slog.Any("error", err))
		targetTier = s.config.DefaultTier
	}

	reqLogger.Info("Routing request",
		slog.String("tier", targetTier.Name),
		slog.String("model", targetTier.Model),
		slog.String("provider", targetTier.Provider),
		slog.Int("tokens", reqCtx.Tokens),
		slog.Bool("has_images", reqCtx.HasImages),
		slog.Bool("has_tools", reqCtx.HasTools),
	)

	// 3. Rewrite model ID in payload
	var rawPayload map[string]interface{}
	if err := json.Unmarshal(body, &rawPayload); err == nil {
		rawPayload["model"] = targetTier.Model

		// Inject reasoning effort if specified
		if targetTier.ReasoningEffort != "" {
			rawPayload["reasoning_effort"] = targetTier.ReasoningEffort
		}

		if reencoded, err := json.Marshal(rawPayload); err == nil {
			body = reencoded
		}
	}

	// 4. Resolve target provider from Registry
	targetProvider, exists := s.registry.Get(targetTier.Provider)
	if !exists {
		reqLogger.Error("Target provider not found in registry", slog.String("provider", targetTier.Provider))
		http.Error(w, fmt.Sprintf("Provider not found: %s", targetTier.Provider), http.StatusBadGateway)
		return
	}

	// 5. Sanitize history images if target model lacks vision or strip_images is enabled
	hasVision := strings.Contains(strings.ToLower(targetTier.Model), "vision") || strings.Contains(strings.ToLower(targetTier.Model), "flash") || targetTier.Provider == "openrouter"
	if targetTier.StripImages {
		hasVision = false
	}
	body, _ = s.sanitizer.SanitizePayload(body, hasVision)

	// 6. Parse target provider URL
	targetURL, err := url.Parse(targetProvider.BaseURL())
	if err != nil {
		reqLogger.Error("Invalid target URL for provider", slog.String("provider", targetTier.Provider), slog.Any("error", err))
		http.Error(w, fmt.Sprintf("Invalid target URL for provider %s: %v", targetTier.Provider, err), http.StatusInternalServerError)
		return
	}

	// 7. Calculate tier classification & potential USD savings
	isLocal := targetProvider.IsLocal()
	var costSaved float64
	if isLocal && reqCtx.Tokens > 0 {
		costSaved = (float64(reqCtx.Tokens) / 1_000_000.0) * 4.50
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

	// 8. Reverse Proxy Setup with shared high-concurrency Transport
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.Transport = s.transport

	// Custom Director using dynamic capability assertions (Zero hardcoded providers!)
	originalDirector := proxy.Director
	proxy.Director = func(outReq *http.Request) {
		originalDirector(outReq)
		outReq.Host = targetURL.Host
		outReq.URL.Scheme = targetURL.Scheme
		outReq.URL.Host = targetURL.Host
		outReq.URL.Path = singleJoiningSlash(targetURL.Path, r.URL.Path)

		// Capability: AuthProvider (Inject Bearer token)
		if auth, ok := targetProvider.(provider.AuthProvider); ok {
			if apiKey := auth.GetAPIKey(); apiKey != "" {
				outReq.Header.Set("Authorization", "Bearer "+apiKey)
			}
		}

		// Capability: HeaderProvider (Inject custom headers e.g. Referrers, Org IDs)
		if hdr, ok := targetProvider.(provider.HeaderProvider); ok {
			for k, v := range hdr.GetHeaders() {
				outReq.Header.Set(k, v)
			}
		}

		// Re-assign body
		outReq.Body = io.NopCloser(bytes.NewReader(body))
		outReq.ContentLength = int64(len(body))
	}

	proxy.ModifyResponse = func(resp *http.Response) error {
		latency := float64(time.Since(startTime).Milliseconds())
		resp.Header.Set("x-nacho-router-tier", targetTier.Name)
		resp.Header.Set("x-spice-router-tier", targetTier.Name)
		resp.Header.Set("x-nacho-target-model", targetTier.Model)
		resp.Header.Set("x-spice-target-model", targetTier.Model)

		s.tracker.Record(telemetry.Observation{
			Tier:       tierNum,
			TierName:   targetTier.Name,
			Model:      targetTier.Model,
			Provider:   targetTier.Provider,
			Tokens:     reqCtx.Tokens,
			CostSaved:  costSaved,
			IsLocal:    isLocal,
			IsFallback: false,
			LatencyMs:  latency,
			Keywords:   reqCtx.Keywords,
			HasImages:  reqCtx.HasImages,
			HasTools:   reqCtx.HasTools,
			StatusCode: resp.StatusCode,
		})

		reqLogger.Info("Completed proxy request",
			slog.String("tier", targetTier.Name),
			slog.String("model", targetTier.Model),
			slog.Int("tokens", reqCtx.Tokens),
			slog.Float64("latency_ms", latency),
			slog.Int("status", resp.StatusCode),
		)
		return nil
	}

	proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, proxyErr error) {
		latency := float64(time.Since(startTime).Milliseconds())
		reqLogger.Error("Proxy upstream error",
			slog.String("tier", targetTier.Name),
			slog.String("target_url", targetURL.String()),
			slog.Any("error", proxyErr),
			slog.Float64("latency_ms", latency),
		)
		http.Error(rw, fmt.Sprintf("Proxy error: %v", proxyErr), http.StatusBadGateway)
	}

	proxy.ServeHTTP(w, r)
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
