package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/config"
	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/provider"
	"github.com/dixieflatline76/nacho-flow/pkg/safeio"
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
		if r.URL.Query().Get("format") == "yaml" || strings.Contains(r.Header.Get("Accept"), "yaml") {
			w.Header().Set(contract.HeaderContentType, "application/x-yaml")
			w.WriteHeader(http.StatusOK)
			if s.configPath != "" {
				configDir := filepath.Dir(s.configPath)
				configFile := filepath.Base(s.configPath)
				if sbd, err := safeio.NewSafeBoundedDir(configDir); err == nil {
					if rawBytes, readErr := sbd.ReadFile(configFile); readErr == nil {
						_, _ = w.Write(rawBytes)
						return
					}
				}
			}
			publicDTO := config.ToPublicDTO(s.GetConfig())
			yamlData, _ := publicDTO.MarshalYAML()
			yamlBytes, _ := yaml.Marshal(yamlData)
			_, _ = w.Write(yamlBytes)
			return
		}

		w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
		w.WriteHeader(http.StatusOK)

		publicDTO := config.ToPublicDTO(s.GetConfig())
		jsonBytes, _ := publicDTO.MarshalJSON()
		_, _ = w.Write(jsonBytes)
		return
	}

	if r.Method == http.MethodPut || r.Method == http.MethodPost {
		s.handleConfigUpdate(w, r)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// ApplyConfig atomically validates, merges secrets, compiles expressions, and swaps runtime state.
// ApplyConfig atomically updates in-memory routing rules and optionally saves to disk.
// If rawYAML bytes are provided, they will be written directly to disk to preserve comments.
func (s *Server) ApplyConfig(incoming *contract.Config, persistDisk bool, rawYAML ...[]byte) (string, error) {
	var backupFile string

	// 1. Merge with existing secrets if incoming has masked placeholders
	merged := config.MergeSecrets(s.GetConfig(), incoming)

	// 2. Validate providers
	if len(merged.Providers) == 0 {
		return "", fmt.Errorf("configuration must have at least one provider defined")
	}

	for id, p := range merged.Providers {
		if strings.TrimSpace(p.BaseURL) == "" {
			return "", fmt.Errorf("provider '%s' is missing required base_url", id)
		}
	}

	// Validate tier references
	for _, tier := range merged.Tiers {
		if _, exists := merged.Providers[tier.Provider]; !exists {
			return "", fmt.Errorf("tier '%s' references unknown provider '%s'", tier.Name, tier.Provider)
		}
	}

	// 3. Pre-compile AST expr rules to verify syntax
	newEval, err := strategy.NewExprEvaluator(merged.Tiers, merged.DefaultTier)
	if err != nil {
		return "", fmt.Errorf("invalid routing expression: %w", err)
	}

	// 4. Create memento and update disk file if persistDisk is true
	if persistDisk && s.configPath != "" {
		configDir := filepath.Dir(s.configPath)
		configFile := filepath.Base(s.configPath)
		if sbd, err := safeio.NewSafeBoundedDir(configDir); err == nil {
			if data, readErr := sbd.ReadFile(configFile); readErr == nil {
				timestamp := time.Now().Format("20060102T150405")
				backupName := fmt.Sprintf("%s.bak.%s", configFile, timestamp)
				if writeErr := sbd.WriteFile(backupName, data, 0600); writeErr == nil {
					backupFile, _ = sbd.ResolveSafePath(backupName)
				}
			}

			var yamlBytes []byte
			if len(rawYAML) > 0 && len(rawYAML[0]) > 0 {
				yamlBytes = rawYAML[0]
			} else {
				yamlBytes, _ = config.SerializeConfigYAML(merged)
			}

			if len(yamlBytes) > 0 {
				_ = sbd.AtomicWrite(configFile, yamlBytes, 0600)
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

	// 5b. Dynamically reconfigure PricingOracle providers upon hot-reload
	if s.oracle != nil {
		for id, p := range merged.Providers {
			idLower := strings.ToLower(id)
			if idLower == contract.ProviderOpenRouter || strings.Contains(strings.ToLower(p.BaseURL), contract.ProviderOpenRouter) {
				var interval time.Duration
				if p.PricingSyncInterval != "" {
					interval, _ = time.ParseDuration(p.PricingSyncInterval)
				}
				s.oracle.RegisterProvider(
					telemetry.NewOpenRouterPricingProviderWithURL(p.BaseURL, p.APIKey),
					interval,
				)
			}
		}
	}

	// 6. Arm Watchdog for Auto-Rollback (if next proxy requests fail consecutively)
	s.armWatchdog(mementoState, 30*time.Second)

	// 7. Broadcast config update event over SSE
	if s.eventBroker != nil {
		_ = s.eventBroker.PublishJSON(telemetry.EventConfigUpdated, telemetry.ConfigEventData{
			Timestamp: time.Now().Format(time.RFC3339),
			Version:   contract.Version,
		})
	}

	return backupFile, nil
}

// ReloadConfigFromDisk reads and re-evaluates the config.yaml file from disk.
func (s *Server) ReloadConfigFromDisk() error {
	if s.configPath == "" {
		return fmt.Errorf("no config path configured")
	}
	loadedCfg, err := config.LoadConfig(s.configPath)
	if err != nil {
		return fmt.Errorf("failed to read config from disk: %w", err)
	}
	_, err = s.ApplyConfig(loadedCfg, false)
	return err
}

func (s *Server) handleConfigUpdate(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{
				"type":    "invalid_payload",
				"message": "failed to read request body",
			},
		})
		return
	}

	var incoming contract.Config
	if jsonErr := json.Unmarshal(bodyBytes, &incoming); jsonErr != nil {
		if yamlErr := yaml.Unmarshal(bodyBytes, &incoming); yamlErr != nil {
			w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{
					"type":    "invalid_payload",
					"message": fmt.Sprintf("failed to parse configuration: json: %v, yaml: %v", jsonErr, yamlErr),
				},
			})
			return
		}
	}

	if r.URL.Query().Get("dry_run") == "true" {
		merged := config.MergeSecrets(s.GetConfig(), &incoming)
		if _, evalErr := strategy.NewExprEvaluator(merged.Tiers, merged.DefaultTier); evalErr != nil {
			w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{
					"type":    "invalid_expr",
					"message": evalErr.Error(),
				},
			})
			return
		}
		w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"dry_run": true,
			"message": "Configuration and routing expressions validated successfully",
		})
		return
	}

	backupFile, applyErr := s.ApplyConfig(&incoming, true, bodyBytes)
	if applyErr != nil {
		w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{
				"type":    "invalid_config",
				"message": applyErr.Error(),
			},
		})
		return
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

// handleAPIStatsReset serves POST /api/v1/stats/reset.
func (s *Server) handleAPIStatsReset(w http.ResponseWriter, r *http.Request) {
	if s.setCORS(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.tracker != nil {
		s.tracker.Reset()
	}

	if s.ringBuffer != nil {
		s.ringBuffer.Reset()
	}

	if s.diskStore != nil && s.tracker != nil {
		_ = s.diskStore.Save(s.tracker.GetStats())
	}

	if s.eventBroker != nil && s.tracker != nil {
		_ = s.eventBroker.PublishJSON(telemetry.EventStats, s.tracker.GetStats())
	}

	w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"message": "Telemetry stats reset to zero",
	})
}

// handleAPIStatsRecalculate serves POST /api/v1/stats/recalculate.
func (s *Server) handleAPIStatsRecalculate(w http.ResponseWriter, r *http.Request) {
	if s.setCORS(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := s.trafficLogPath
	if path == "" {
		path = "logs/traffic.jsonl"
	}

	records, err := telemetry.ReadRecords(path, 0)
	if err != nil {
		w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{
				"type":    "recalculate_error",
				"message": err.Error(),
			},
		})
		return
	}

	benchmarkCost := contract.DefaultBenchmarkPricePerMillion

	if s.tracker != nil {
		s.tracker.RecalculateFromRecords(records, s.oracle, benchmarkCost)
	}

	if s.ringBuffer != nil {
		s.ringBuffer.Reset()
		start := 0
		if len(records) > 500 {
			start = len(records) - 500
		}
		for _, rec := range records[start:] {
			s.ringBuffer.Emit(rec)
		}
	}

	if s.diskStore != nil && s.tracker != nil {
		_ = s.diskStore.Save(s.tracker.GetStats())
	}

	if s.eventBroker != nil && s.tracker != nil {
		_ = s.eventBroker.PublishJSON(telemetry.EventStats, s.tracker.GetStats())
	}

	w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":            "ok",
		"message":           "Telemetry stats recalculated from traffic log",
		"records_processed": len(records),
	})
}
