package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
	"github.com/dixieflatline76/nacho-flow/pkg/tuner"
)

func TestAPI_TelemetryClients(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "traffic.jsonl")
	srv.trafficLogPath = logPath

	tl, err := telemetry.NewTrafficLogger(logPath, 100)
	if err != nil {
		t.Fatalf("failed to create traffic logger: %v", err)
	}

	tl.Emit(telemetry.TurnRecord{
		Timestamp: time.Now().UTC(),
		SessionID: "s-cline",
		Tokens:    1000,
		ClientID:  "cline",
	})
	tl.Emit(telemetry.TurnRecord{
		Timestamp: time.Now().UTC(),
		SessionID: "s-zoo",
		Tokens:    5000,
		ClientID:  "zoo",
	})
	_ = tl.Close()

	// 1. GET /api/v1/telemetry/clients
	req := httptest.NewRequest(http.MethodGet, contract.PathAPITelemetryClients, nil)
	req.Header.Set("Authorization", "Bearer test-secret-token")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /api/v1/telemetry/clients, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Clients []string       `json:"clients"`
		Counts  map[string]int `json:"counts"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse JSON response: %v", err)
	}

	if len(resp.Clients) == 0 || resp.Clients[0] != "all" {
		t.Errorf("expected 'all' as first client, got %v", resp.Clients)
	}

	hasCline := false
	hasZoo := false
	for _, c := range resp.Clients {
		if c == "cline" {
			hasCline = true
		}
		if c == "zoo" {
			hasZoo = true
		}
	}
	if !hasCline || !hasZoo {
		t.Errorf("expected cline and zoo in clients list, got %v", resp.Clients)
	}

	if resp.Counts["cline"] != 1 {
		t.Errorf("expected 1 count for cline, got %d", resp.Counts["cline"])
	}
	if resp.Counts["zoo"] != 1 {
		t.Errorf("expected 1 count for zoo, got %d", resp.Counts["zoo"])
	}

	// 2. CORS OPTIONS preflight
	reqOpts := httptest.NewRequest(http.MethodOptions, contract.PathAPITelemetryClients, nil)
	wOpts := httptest.NewRecorder()
	srv.ServeHTTP(wOpts, reqOpts)
	if wOpts.Code != http.StatusOK {
		t.Errorf("expected 200 OK for OPTIONS, got %d", wOpts.Code)
	}

	// 3. Method not allowed
	reqPost := httptest.NewRequest(http.MethodPost, contract.PathAPITelemetryClients, nil)
	reqPost.Header.Set("Authorization", "Bearer test-secret-token")
	wPost := httptest.NewRecorder()
	srv.ServeHTTP(wPost, reqPost)
	if wPost.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 Method Not Allowed for POST, got %d", wPost.Code)
	}
}

func TestAPI_Tune_WithClientID(t *testing.T) {
	srv, ringBuffer, _ := setupTestServer(t)

	// Emit records with specific client IDs
	for i := 0; i < 5; i++ {
		ringBuffer.Emit(telemetry.TurnRecord{
			SessionID: "s-cline-1",
			Tokens:    1200,
			IsLocal:   true,
			ClientID:  "cline",
		})
		ringBuffer.Emit(telemetry.TurnRecord{
			SessionID: "s-zoo-1",
			Tokens:    6000,
			IsLocal:   true,
			ClientID:  "zoo",
		})
	}

	// 1. Query parameter: ?client_id=cline
	reqQuery := httptest.NewRequest(http.MethodPost, contract.PathAPITune+"?client_id=cline", nil)
	reqQuery.Header.Set("Authorization", "Bearer test-secret-token")
	wQuery := httptest.NewRecorder()
	srv.ServeHTTP(wQuery, reqQuery)

	if wQuery.Code != http.StatusOK {
		t.Fatalf("expected 200 OK with ?client_id=cline, got %d: %s", wQuery.Code, wQuery.Body.String())
	}

	var resQuery tuner.TuningResult
	if err := json.Unmarshal(wQuery.Body.Bytes(), &resQuery); err != nil {
		t.Fatalf("failed to decode response JSON: %v", err)
	}
	if len(resQuery.Tiers) == 0 {
		t.Errorf("expected tuned tiers in response")
	}

	// 2. JSON Body: {"client_id": "zoo"}
	bodyJSON := bytes.NewBufferString(`{"client_id": "zoo"}`)
	reqBody := httptest.NewRequest(http.MethodPost, contract.PathAPITune, bodyJSON)
	reqBody.Header.Set("Authorization", "Bearer test-secret-token")
	reqBody.Header.Set("Content-Type", "application/json")
	wBody := httptest.NewRecorder()
	srv.ServeHTTP(wBody, reqBody)

	if wBody.Code != http.StatusOK {
		t.Fatalf("expected 200 OK with body client_id, got %d: %s", wBody.Code, wBody.Body.String())
	}

	var resBody tuner.TuningResult
	if err := json.Unmarshal(wBody.Body.Bytes(), &resBody); err != nil {
		t.Fatalf("failed to decode body response JSON: %v", err)
	}
	if len(resBody.Tiers) == 0 {
		t.Errorf("expected tuned tiers in response")
	}
}
