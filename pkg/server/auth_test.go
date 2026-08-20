package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/router"
	"github.com/dixieflatline76/nacho-flow/pkg/strategy"
)

func createAuthTestServer(authToken string) (*Server, *httptest.Server) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl-test","choices":[{"message":{"content":"authenticated response"}}]}`))
	}))

	cfg := &contract.Config{
		Port:      8000,
		AuthToken: authToken,
		Providers: map[string]contract.ProviderConfig{
			"test_prov": {
				BaseURL: upstream.URL,
				APIKey:  "real-cloud-api-key",
				Type:    "cloud",
			},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Tier1",
				Model:    "test-model",
				Provider: "test_prov",
				When:     "true",
			},
		},
		DefaultTier: contract.Tier{
			Name:     "DefaultTier",
			Model:    "test-model",
			Provider: "test_prov",
			When:     "true",
		},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier)
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()

	srv := NewServer(cfg, evaluator, classifier, sanitizer)
	return srv, upstream
}

func TestServer_Auth_ValidBearerToken_Allowed(t *testing.T) {
	srv, upstream := createAuthTestServer("sk-my-secret-key")
	defer upstream.Close()

	payload := []byte(`{"model":"nacho-hybrid","messages":[{"role":"user","content":"hello"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer sk-my-secret-key")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200 OK for valid bearer token, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestServer_Auth_ValidXAPIKeyHeader_Allowed(t *testing.T) {
	srv, upstream := createAuthTestServer("sk-my-secret-key")
	defer upstream.Close()

	payload := []byte(`{"model":"nacho-hybrid","messages":[{"role":"user","content":"hello"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(payload))
	req.Header.Set("X-API-Key", "sk-my-secret-key")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200 OK for valid X-API-Key header, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestServer_Auth_InvalidBearerToken_Unauthorized(t *testing.T) {
	srv, upstream := createAuthTestServer("sk-my-secret-key")
	defer upstream.Close()

	payload := []byte(`{"model":"nacho-hybrid","messages":[{"role":"user","content":"hello"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer invalid-wrong-token")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("Expected status 401 Unauthorized for invalid token, got %d", w.Code)
	}

	if !bytes.Contains(w.Body.Bytes(), []byte("invalid_api_key")) {
		t.Errorf("Expected response body to contain 'invalid_api_key', got: %s", w.Body.String())
	}
}

func TestServer_Auth_MissingAuthorization_Unauthorized(t *testing.T) {
	srv, upstream := createAuthTestServer("sk-my-secret-key")
	defer upstream.Close()

	payload := []byte(`{"model":"nacho-hybrid","messages":[{"role":"user","content":"hello"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(payload))

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("Expected status 401 Unauthorized for missing token, got %d", w.Code)
	}
}

func TestServer_Auth_UnsetToken_OpenMode(t *testing.T) {
	srv, upstream := createAuthTestServer("") // Auth disabled (localhost zero friction)
	defer upstream.Close()

	payload := []byte(`{"model":"nacho-hybrid","messages":[{"role":"user","content":"hello"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer sk-dummy") // dummy client key

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200 OK when auth is unset, got %d", w.Code)
	}
}

func TestServer_HealthCheck_AlwaysPublic(t *testing.T) {
	srv, upstream := createAuthTestServer("sk-my-secret-key")
	defer upstream.Close()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected /health to remain public (200 OK) even when AuthToken is set, got %d", w.Code)
	}
}
