package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync"
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

func TestServer_Auth_ValidApiKeyLowerHeader_Allowed(t *testing.T) {
	srv, upstream := createAuthTestServer("sk-my-secret-key")
	defer upstream.Close()

	payload := []byte(`{"model":"nacho-hybrid","messages":[{"role":"user","content":"hello"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(payload))
	req.Header.Set("api-key", "sk-my-secret-key")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200 OK for valid api-key header, got %d", w.Code)
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

func TestServer_Auth_CaseSensitivity_Unauthorized(t *testing.T) {
	srv, upstream := createAuthTestServer("sk-MySecretKey")
	defer upstream.Close()

	payload := []byte(`{"model":"nacho-hybrid","messages":[{"role":"user","content":"hello"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer sk-mysecretkey") // wrong casing

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("Expected status 401 for case-mismatched token, got %d", w.Code)
	}
}

func TestServer_Auth_MalformedAuthHeader_Unauthorized(t *testing.T) {
	srv, upstream := createAuthTestServer("sk-my-secret-key")
	defer upstream.Close()

	testCases := []string{
		"Bearer",
		"Bearer ",
		"Basic dXNlcjpwYXNz",
		"Token sk-my-secret-key",
		"",
	}

	for _, tc := range testCases {
		payload := []byte(`{"model":"nacho-hybrid","messages":[{"role":"user","content":"hello"}]}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(payload))
		if tc != "" {
			req.Header.Set("Authorization", tc)
		}

		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected 401 for auth header '%s', got %d", tc, w.Code)
		}
	}
}

func TestServer_Auth_StatsEndpoint_Protected(t *testing.T) {
	srv, upstream := createAuthTestServer("sk-my-secret-key")
	defer upstream.Close()

	// 1. Without auth -> 401
	req := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401 Unauthorized for /v1/stats without auth, got %d", w.Code)
	}

	// 2. With valid auth -> 200
	reqValid := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	reqValid.Header.Set("Authorization", "Bearer sk-my-secret-key")
	wValid := httptest.NewRecorder()
	srv.ServeHTTP(wValid, reqValid)
	if wValid.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for /v1/stats with valid auth, got %d", wValid.Code)
	}
}

func TestServer_Auth_ModelsEndpoint_Protected(t *testing.T) {
	srv, upstream := createAuthTestServer("sk-my-secret-key")
	defer upstream.Close()

	// 1. Without auth -> 401
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401 Unauthorized for /v1/models without auth, got %d", w.Code)
	}

	// 2. With valid auth -> 200
	reqValid := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	reqValid.Header.Set("Authorization", "Bearer sk-my-secret-key")
	wValid := httptest.NewRecorder()
	srv.ServeHTTP(wValid, reqValid)
	if wValid.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for /v1/models with valid auth, got %d", wValid.Code)
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

func TestServer_Auth_HighConcurrency_Race(t *testing.T) {
	srv, upstream := createAuthTestServer("sk-concurrent-key")
	defer upstream.Close()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		// Worker 1: Valid auth
		go func() {
			defer wg.Done()
			payload := []byte(`{"model":"nacho-hybrid","messages":[{"role":"user","content":"ping"}]}`)
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(payload))
			req.Header.Set("Authorization", "Bearer sk-concurrent-key")
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Errorf("Expected 200 OK under concurrency, got %d", w.Code)
			}
		}()

		// Worker 2: Invalid auth
		go func() {
			defer wg.Done()
			payload := []byte(`{"model":"nacho-hybrid","messages":[{"role":"user","content":"ping"}]}`)
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(payload))
			req.Header.Set("Authorization", "Bearer bad-key")
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("Expected 401 Unauthorized under concurrency, got %d", w.Code)
			}
		}()
	}
	wg.Wait()
}
