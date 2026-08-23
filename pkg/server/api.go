package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/config"
	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/provider"
	"github.com/dixieflatline76/nacho-flow/pkg/strategy"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
	"gopkg.in/yaml.v3"
)

func (s *Server) setCORS(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-API-Key, api-key")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return true
	}
	return false
}

// handleAPIInfo serves GET /api/v1/info (Public).
func (s *Server) handleAPIInfo(w http.ResponseWriter, r *http.Request) {
	if s.setCORS(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
	w.WriteHeader(http.StatusOK)

	resp := map[string]interface{}{
		"service": contract.AppName,
		"version": contract.Version,
		"features": []string{
			"sse_telemetry",
			"ring_buffer_routes",
			"config_hot_reload",
			"circuit_breakers",
			"pricing_oracle",
			"async_tuner",
		},
		"uptime_seconds": int(time.Since(s.startTime).Seconds()),
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// handleAPIEvents serves GET /api/v1/events (SSE Stream).
func (s *Server) handleAPIEvents(w http.ResponseWriter, r *http.Request) {
	if s.setCORS(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set(contract.HeaderContentType, contract.ContentTypeEventStream)
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	if s.eventBroker == nil {
		return
	}

	eventChan := s.eventBroker.Subscribe(100)
	defer s.eventBroker.Unsubscribe(eventChan)

	keepAliveTicker := time.NewTicker(15 * time.Second)
	defer keepAliveTicker.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-keepAliveTicker.C:
			if _, err := fmt.Fprintf(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case evt, ok := <-eventChan:
			if !ok {
				return
			}
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Type, string(evt.Data)); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// handleAPIRoutes serves GET /api/v1/routes (Ring Buffer History).
func (s *Server) handleAPIRoutes(w http.ResponseWriter, r *http.Request) {
	if s.setCORS(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limit := 50
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if parsed, err := strconv.Atoi(lStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
	w.WriteHeader(http.StatusOK)

	var routes []telemetry.TurnRecord
	var totalTracked int64
	capacity := telemetry.DefaultRingBufferCapacity
	if s.ringBuffer != nil {
		routes = s.ringBuffer.GetRecent(limit)
		totalTracked = s.ringBuffer.TotalTracked()
		capacity = s.ringBuffer.Capacity()
	} else {
		routes = []telemetry.TurnRecord{}
	}

	resp := map[string]interface{}{
		"total_tracked":   totalTracked,
		"buffer_capacity": capacity,
		"routes":          routes,
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// handleAPICircuits serves GET /api/v1/circuits and POST /api/v1/circuits/reset.
func (s *Server) handleAPICircuits(w http.ResponseWriter, r *http.Request) {
	if s.setCORS(w, r) {
		return
	}

	if r.Method == http.MethodGet {
		w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
		w.WriteHeader(http.StatusOK)

		reg := s.GetRegistry()
		var circuits []provider.CircuitInfo
		if reg != nil {
			circuits = reg.GetCircuitsStatus()
		} else {
			circuits = []provider.CircuitInfo{}
		}
		resp := map[string]interface{}{
			"circuits": circuits,
		}
		_ = json.NewEncoder(w).Encode(resp)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// handleAPICircuitsReset serves POST /api/v1/circuits/reset.
func (s *Server) handleAPICircuitsReset(w http.ResponseWriter, r *http.Request) {
	if s.setCORS(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Provider string `json:"provider"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	if req.Provider == "" {
		req.Provider = r.URL.Query().Get("provider")
	}

	reg := s.GetRegistry()
	if reg != nil {
		reg.ResetCircuit(req.Provider)
	}

	if s.eventBroker != nil {
		_ = s.eventBroker.PublishJSON(telemetry.EventCircuitStateChanged, telemetry.CircuitEventData{
			Provider: req.Provider,
			State:    "closed",
			Failures: 0,
		})
	}

	w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"message": fmt.Sprintf("Circuit breaker for '%s' reset to closed", req.Provider),
	})
}

// handleAPIPricing serves GET /api/v1/pricing.
func (s *Server) handleAPIPricing(w http.ResponseWriter, r *http.Request) {
	if s.setCORS(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
	w.WriteHeader(http.StatusOK)

	var prices map[string]telemetry.ModelPricing
	if s.oracle != nil {
		prices = s.oracle.GetAllPricing()
	} else {
		prices = make(map[string]telemetry.ModelPricing)
	}

	resp := map[string]interface{}{
		"benchmark_model":             contract.DefaultBenchmarkModel,
		"benchmark_price_per_million": contract.DefaultBenchmarkPricePerMillion,
		"pricing":                     prices,
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// handleAPIConfig serves GET /api/v1/config and PUT /api/v1/config.
func (s *Server) handleAPIConfig(w http.ResponseWriter, r *http.Request) {
	if s.setCORS(w, r) {
		return
	}

	if r.Method == http.MethodGet {
		w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
		w.WriteHeader(http.StatusOK)

		sanitized := config.SanitizeConfig(s.GetConfig())
		_ = json.NewEncoder(w).Encode(sanitized)
		return
	}

	if r.Method == http.MethodPut || r.Method == http.MethodPost {
		s.handleConfigUpdate(w, r)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func (s *Server) handleConfigUpdate(w http.ResponseWriter, r *http.Request) {
	var incoming contract.Config
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&incoming); err != nil {
		// Fallback try YAML
		_ = r.Body.Close()
		w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{
				"type":    "invalid_payload",
				"message": fmt.Sprintf("failed to parse JSON configuration: %v", err),
			},
		})
		return
	}

	// 1. Merge masked secrets with current active secrets in memory
	merged := config.MergeSecrets(s.GetConfig(), &incoming)

	// 2. Validate provider definitions and base URLs
	if len(merged.Providers) == 0 {
		w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{
				"type":    "invalid_config",
				"message": "configuration must have at least one provider defined",
			},
		})
		return
	}

	for id, p := range merged.Providers {
		if strings.TrimSpace(p.BaseURL) == "" {
			w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{
					"type":    "invalid_config",
					"message": fmt.Sprintf("provider '%s' is missing required base_url", id),
				},
			})
			return
		}
	}

	// Validate tier references
	for _, tier := range merged.Tiers {
		if _, exists := merged.Providers[tier.Provider]; !exists {
			w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{
					"type":    "invalid_config",
					"message": fmt.Sprintf("tier '%s' references unknown provider '%s'", tier.Name, tier.Provider),
				},
			})
			return
		}
	}

	// 3. Pre-compile AST expr rules to verify syntax
	newEval, err := strategy.NewExprEvaluator(merged.Tiers, merged.DefaultTier)
	if err != nil {
		w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{
				"type":    "invalid_expr",
				"message": err.Error(),
			},
		})
		return
	}

	// Dry run mode check
	if r.URL.Query().Get("dry_run") == "true" {
		w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"dry_run": true,
			"message": "Configuration and routing expressions validated successfully",
		})
		return
	}

	// 4. Create memento and update disk file if configPath is set
	var backupFile string
	if s.configPath != "" {
		if data, readErr := os.ReadFile(s.configPath); readErr == nil {
			timestamp := time.Now().Format("20060102T150405")
			backupFile = filepath.Clean(fmt.Sprintf("%s.bak.%s", s.configPath, timestamp))
			_ = os.WriteFile(backupFile, data, 0600)
		}

		if yamlBytes, marshalErr := yaml.Marshal(merged); marshalErr == nil {
			dir := filepath.Dir(s.configPath)
			tmpFile := filepath.Clean(filepath.Join(dir, fmt.Sprintf("config.tmp.%d.yaml", os.Getpid())))
			if writeErr := os.WriteFile(tmpFile, yamlBytes, 0600); writeErr == nil {
				_ = os.Rename(tmpFile, s.configPath)
			}
		}
	}

	// 5. Atomic State Pointer Swap (RCU)
	newReg := provider.NewRegistryFromConfig(merged)
	mementoState := s.state.Load()
	s.state.Store(&runtimeState{
		config:    merged,
		evaluator: newEval,
		registry:  newReg,
	})

	// 6. Arm Watchdog for Auto-Rollback (if next proxy requests fail consecutively)
	s.armWatchdog(mementoState, 30*time.Second)

	// 7. Broadcast config update event over SSE
	if s.eventBroker != nil {
		_ = s.eventBroker.PublishJSON(telemetry.EventConfigUpdated, telemetry.ConfigEventData{
			Timestamp: time.Now().Format(time.RFC3339),
			Version:   contract.Version,
		})
	}

	w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	resp := map[string]interface{}{
		"status":  "ok",
		"message": "Configuration validated and atomically hot-reloaded",
	}
	if backupFile != "" {
		resp["backup_file"] = backupFile
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// handleAPITune serves POST /api/v1/tune.
func (s *Server) handleAPITune(w http.ResponseWriter, r *http.Request) {
	if s.setCORS(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.tuner == nil {
		w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{
				"type":    "tuner_unavailable",
				"message": "auto-tuning optimizer is not initialized on this server",
			},
		})
		return
	}

	var records []telemetry.TurnRecord
	if s.ringBuffer != nil {
		records = s.ringBuffer.GetRecent(500)
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	result, err := s.tuner.OptimizeWithContext(ctx, records, s.GetConfig())
	if err != nil {
		w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{
				"type":    "tuning_error",
				"message": err.Error(),
			},
		})
		return
	}

	w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(result)
}
