package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/agentregistry"
	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/nts"
	"github.com/dixieflatline76/nacho-flow/pkg/provider"
	"github.com/dixieflatline76/nacho-flow/pkg/router"
	"github.com/dixieflatline76/nacho-flow/pkg/router/shield"
	"github.com/dixieflatline76/nacho-flow/pkg/store"
	"github.com/dixieflatline76/nacho-flow/pkg/strategy"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
	"github.com/dixieflatline76/nacho-flow/pkg/tuner"
)

type runtimeState struct {
	config         *contract.Config
	evaluator      contract.Evaluator
	registry       *provider.Registry
	ntsTransformer *nts.Transformer
}

type Server struct {
	state                    atomic.Pointer[runtimeState]
	classifier               contract.Classifier
	sanitizer                contract.Sanitizer
	oracle                   *telemetry.PricingOracle
	tracker                  *telemetry.StatsTracker
	sessionTracker           *router.SessionTracker
	metaRegistry             *MetaRegistry
	shieldMgr                *shield.ShieldManager
	logger                   *slog.Logger
	transport                *http.Transport
	ringBuffer               *telemetry.RingBufferSink
	eventBroker              *telemetry.EventBroker
	tuner                    tuner.OptimizationStrategy
	tuningRunner             TuningRunner
	trafficLogger            *telemetry.TrafficLogger
	diskStore                *store.DiskStore
	ntsTransformer           *nts.Transformer
	trafficLogPath           string
	configPath               string
	lastDiskWriteUnixNano    atomic.Int64
	startTime                time.Time
	mementoState             *runtimeState
	watchdogMu               sync.Mutex
	watchdogActive           bool
	watchdogErrors           atomic.Int32
	directivePath            string
	exitFunc                 func(code int)
	warnedZeroUsageProviders sync.Map
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

// SetRingBuffer attaches a ring buffer sink to the server.
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

func (s *Server) SetLastDiskWriteUnixNano(nano int64) {
	s.lastDiskWriteUnixNano.Store(nano)
}

func (s *Server) GetLastDiskWriteUnixNano() int64 {
	return s.lastDiskWriteUnixNano.Load()
}

func (s *Server) SetTuner(t tuner.OptimizationStrategy) {
	s.tuner = t
}

func (s *Server) SetTuningRunner(runner TuningRunner) {
	s.tuningRunner = runner
}

func (s *Server) SetTrafficLogger(tl *telemetry.TrafficLogger) {
	s.trafficLogger = tl
}

func (s *Server) SetDiskStore(ds *store.DiskStore) {
	s.diskStore = ds
}

func (s *Server) SetTrafficLogPath(path string) {
	s.trafficLogPath = path
}

func (s *Server) SetExitFunc(fn func(code int)) {
	s.exitFunc = fn
}

func (s *Server) SetDirectivePath(path string) {
	s.directivePath = path
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
	// Error signatures: pull canonical error signatures from agentregistry
	// and merge any user-configured custom overrides if specified.
	errorSignatures := agentregistry.DefaultRegistry().ErrorSignaturesList()
	if len(cfg.AgentShield.ErrorSignatures) > 0 {
		errorSignatures = mergeUniqueSlices(errorSignatures, cfg.AgentShield.ErrorSignatures)
	}
	if classWithSigs, ok := class.(interface{ SetErrorSignatures([]string) }); ok {
		classWithSigs.SetErrorSignatures(errorSignatures)
	}
	// Kickstart write tools: pull canonical zero-allocation write tools from agentregistry
	// and merge any user-configured custom overrides if specified.
	writeTools := agentregistry.DefaultRegistry().WriteToolsList()
	var custom []string
	if len(cfg.Kickstart.CustomWriteTools) > 0 {
		custom = cfg.Kickstart.CustomWriteTools
	} else if len(cfg.Kickstart.WriteTools) > 0 {
		custom = cfg.Kickstart.WriteTools
	} else if len(cfg.CycleKiller.WriteTools) > 0 {
		custom = cfg.CycleKiller.WriteTools
	} else if len(cfg.CycleKiller.KickstartWriteTools) > 0 {
		custom = cfg.CycleKiller.KickstartWriteTools
	} else if len(cfg.CycleBreaker.WriteTools) > 0 {
		custom = cfg.CycleBreaker.WriteTools
	} else if len(cfg.CycleBreaker.KickstartWriteTools) > 0 {
		custom = cfg.CycleBreaker.KickstartWriteTools
	}
	if len(custom) > 0 {
		writeTools = mergeUniqueSlices(writeTools, custom)
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
	for i := range cfg.Tiers {
		ResolveTierVision(&cfg.Tiers[i], oracle)
	}
	ResolveTierVision(&cfg.DefaultTier, oracle)
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
		questions := mergeUniqueSlices(agentregistry.DefaultRegistry().QuestionHeuristicsList(), cfg.AgentShield.QuestionHeuristics)
		modes := mergeUniqueSlices(agentregistry.DefaultRegistry().ModeHeuristicsList(), cfg.AgentShield.ModeSwitchHeuristics)
		shieldMgr = shield.NewShieldManager(questions, modes)
	}

	ntsTr := BuildNTSTransformer(cfg.NTS)

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
		tuner:          tuner.NewMinConflictsOptimizer(tuner.DefaultTuningPolicy()),
		ntsTransformer: ntsTr,
		startTime:      time.Now(),
	}
	srv.tuningRunner = NewInProcessTuningRunner(srv)

	srv.state.Store(&runtimeState{
		config:         cfg,
		evaluator:      eval,
		registry:       reg,
		ntsTransformer: ntsTr,
	})

	return srv
}

func BuildNTSTransformer(cfg contract.NTSConfig) *nts.Transformer {
	ntsCfg := nts.DefaultConfig()
	if cfg.Enabled != nil {
		ntsCfg.Enabled = *cfg.Enabled
	}
	if !ntsCfg.Enabled {
		return nil
	}
	if cfg.StripANSI != nil {
		ntsCfg.StripANSI = *cfg.StripANSI
	}
	if cfg.ResolveCR != nil {
		ntsCfg.ResolveCR = *cfg.ResolveCR
	}
	if cfg.DeduplicateLines != nil {
		ntsCfg.DeduplicateLines = *cfg.DeduplicateLines
	}
	if cfg.DedupThreshold > 0 {
		ntsCfg.DedupThreshold = cfg.DedupThreshold
	}
	if cfg.StripBoilerplate != nil {
		ntsCfg.StripBoilerplate = *cfg.StripBoilerplate
	}
	if cfg.NormalizeWhitespace != nil {
		ntsCfg.NormalizeWhitespace = *cfg.NormalizeWhitespace
	}
	if cfg.PreserveFileReads != nil {
		ntsCfg.PreserveFileReads = *cfg.PreserveFileReads
	}
	if cfg.PreserveFileWrites != nil {
		ntsCfg.PreserveFileWrites = *cfg.PreserveFileWrites
	}
	if cfg.PreserveCacheControl != nil {
		ntsCfg.PreserveCacheControl = *cfg.PreserveCacheControl
	}
	if cfg.CompactStaleFileReads != nil {
		ntsCfg.CompactStaleFileReads = *cfg.CompactStaleFileReads
	}
	if cfg.StaleReadDepth > 0 {
		ntsCfg.StaleReadDepth = cfg.StaleReadDepth
	}
	return nts.NewTransformer(ntsCfg)
}

func (s *Server) GetNTSTransformer() *nts.Transformer {
	if st := s.state.Load(); st != nil {
		return st.ntsTransformer
	}
	return nil
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
		case contract.PathAPIDirective:
			s.handleAPIDirective(w, r)
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

	s.handleCompletions(w, r, startTime, reqLogger)
}
