// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
)

// DirectiveRequest represents an incoming command directive via POST /api/v1/directive.
type DirectiveRequest struct {
	Action  string                 `json:"action"`
	Payload map[string]interface{} `json:"payload,omitempty"`
}

// DirectiveResponse represents the outcome of a dispatched directive.
type DirectiveResponse struct {
	Status          string                 `json:"status"`
	Action          string                 `json:"action"`
	RequiresRestart bool                   `json:"requires_restart"`
	Message         string                 `json:"message,omitempty"`
	Details         map[string]interface{} `json:"details,omitempty"`
}

// handleAPIDirective serves POST /api/v1/directive (Unified Directive Control Plane).
func (s *Server) handleAPIDirective(w http.ResponseWriter, r *http.Request) {
	if s.setCORS(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req DirectiveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "Invalid request body, expected JSON DirectiveRequest",
		})
		return
	}

	req.Action = strings.ToUpper(strings.TrimSpace(req.Action))

	switch req.Action {
	case contract.DirectiveActionPurgeAllLogs:
		s.handlePurgeAllLogsDirective(w, req)
	case contract.DirectiveActionResetCircuits:
		s.handleResetCircuitsDirective(w, req)
	case contract.DirectiveActionRecalculateStats:
		s.handleRecalculateStatsDirective(w, req)
	default:
		w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": fmt.Sprintf("Unknown directive action '%s'", req.Action),
		})
	}
}

func (s *Server) getDirectiveFilePath() (string, error) {
	if s.directivePath != "" {
		return s.directivePath, nil
	}
	return contract.GetDirectiveFilePath()
}

func (s *Server) handlePurgeAllLogsDirective(w http.ResponseWriter, req DirectiveRequest) {
	directivePath, err := s.getDirectiveFilePath()
	if err != nil {
		w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": fmt.Sprintf("Failed to resolve directive path: %v", err),
		})
		return
	}

	statsPath := ""
	if s.diskStore != nil && s.diskStore.FilePath() != "" {
		statsPath = s.diskStore.FilePath()
	} else if s.directivePath != "" {
		statsPath = filepath.Join(filepath.Dir(s.directivePath), contract.DefaultStatsFileName)
	} else {
		userConfigDir, err := contract.GetUserConfigDir()
		if err != nil || userConfigDir == "" {
			userConfigDir = "."
		}
		statsPath = filepath.Join(userConfigDir, contract.AppName, contract.DefaultStatsFileName)
	}

	envelope := map[string]interface{}{
		"action":     contract.DirectiveActionPurgeAllLogs,
		"created_at": time.Now().UTC().Format(time.RFC3339),
		"targets": map[string]interface{}{
			"stats_path": statsPath,
			"log_dir":    "logs",
			"log_files":  []string{contract.DefaultTrafficLogFileName, contract.DefaultRouterLogFileName},
		},
	}

	raw, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": fmt.Sprintf("Failed to marshal directive envelope: %v", err),
		})
		return
	}

	dir := filepath.Dir(directivePath)
	if mkErr := os.MkdirAll(dir, 0750); mkErr != nil {
		w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": fmt.Sprintf("Failed to create directive directory: %v", mkErr),
		})
		return
	}

	tmpPath := fmt.Sprintf("%s.tmp.%d", directivePath, time.Now().UnixNano())
	if err := os.WriteFile(tmpPath, raw, 0600); err != nil {
		w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": fmt.Sprintf("Failed to write temp directive file: %v", err),
		})
		return
	}

	if err := os.Rename(tmpPath, directivePath); err != nil {
		_ = os.Remove(tmpPath)
		w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": fmt.Sprintf("Failed to commit directive file: %v", err),
		})
		return
	}

	resp := DirectiveResponse{
		Status:          "acknowledged",
		Action:          contract.DirectiveActionPurgeAllLogs,
		RequiresRestart: true,
		Message:         "Purge directive recorded. Daemon shutting down for cold execution.",
		Details: map[string]interface{}{
			"directive_file": directivePath,
		},
	}

	w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	// Schedule process termination after brief delay to flush HTTP frames
	exitFn := s.exitFunc
	if exitFn == nil {
		exitFn = os.Exit
	}
	go func() {
		time.Sleep(100 * time.Millisecond)
		exitFn(0)
	}()
}

func (s *Server) handleResetCircuitsDirective(w http.ResponseWriter, req DirectiveRequest) {
	provider := ""
	if req.Payload != nil {
		if p, ok := req.Payload["provider"].(string); ok {
			provider = p
		}
	}

	reg := s.GetRegistry()
	if reg != nil {
		reg.ResetCircuit(provider)
	}

	if s.eventBroker != nil {
		_ = s.eventBroker.PublishJSON(telemetry.EventCircuitStateChanged, telemetry.CircuitEventData{
			Provider: provider,
			State:    "closed",
			Failures: 0,
		})
	}

	resp := DirectiveResponse{
		Status:          "success",
		Action:          contract.DirectiveActionResetCircuits,
		RequiresRestart: false,
		Message:         fmt.Sprintf("Circuit breaker for '%s' reset to closed", provider),
	}

	w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleRecalculateStatsDirective(w http.ResponseWriter, req DirectiveRequest) {
	path := s.trafficLogPath
	if path == "" {
		path = "logs/traffic.jsonl"
	}

	records, err := telemetry.ReadRecords(path, 0)
	if err != nil {
		w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
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

	resp := DirectiveResponse{
		Status:          "success",
		Action:          contract.DirectiveActionRecalculateStats,
		RequiresRestart: false,
		Message:         "Stats successfully recalculated from logs.",
	}

	w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
