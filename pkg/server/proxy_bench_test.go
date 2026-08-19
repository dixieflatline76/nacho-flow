package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/router"
	"github.com/dixieflatline76/nacho-flow/pkg/strategy"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
)

func BenchmarkProxy_ChatCompletions_EndToEnd(b *testing.B) {
	// Mock upstream returning immediate minimal OpenAI completion response
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"bench-cmpl","choices":[{"message":{"role":"assistant","content":"pong"}}]}`))
	}))
	defer mockUpstream.Close()

	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"mock": {
				BaseURL: mockUpstream.URL,
			},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Local ROCm Tier",
				Model:    "qwen2.5-coder:14b",
				Provider: "mock",
				When:     "Tokens < 16000 && !HasImages && !HasTools",
			},
			{
				Name:     "Cloud Agentic Tier",
				Model:    "qwen3-coder",
				Provider: "mock",
				When:     "Tokens >= 16000 || HasTools",
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Fallback",
			Model:    "fallback-model",
			Provider: "mock",
		},
	}

	evaluator, err := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier)
	if err != nil {
		b.Fatalf("failed to create evaluator: %v", err)
	}

	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	oracle := telemetry.NewPricingOracle()
	tracker := telemetry.NewStatsTracker(50000)
	defer tracker.Close()

	srv := NewServerWithTelemetry(cfg, evaluator, classifier, sanitizer, oracle, tracker, nil)

	payload := `{"model": "nacho-hybrid", "messages": [{"role": "user", "content": "How do I optimize mutex contention in high concurrency Go?"}]}`

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(payload))
			srv.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				b.Errorf("expected status 200, got %d", rec.Code)
			}
		}
	})
}
