package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

type Server struct {
	config     *contract.Config
	evaluator  contract.Evaluator
	classifier contract.Classifier
	sanitizer  contract.Sanitizer
}

func NewServer(cfg *contract.Config, eval contract.Evaluator, class contract.Classifier, san contract.Sanitizer) *Server {
	return &Server{
		config:     cfg,
		evaluator:  eval,
		classifier: class,
		sanitizer:  san,
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Health check endpoint
	if r.URL.Path == "/health" || r.URL.Path == "/v1/health" {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok","service":"nacho-flow"}`))
		return
	}

	// Models listing endpoint (OpenAI compatible)
	if r.URL.Path == "/v1/models" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"object":"list","data":[{"id":"nacho-hybrid","object":"model","owned_by":"spicerack.dev"}]}`))
		return
	}

	// Only process chat completions / completions endpoints for routing
	if !strings.HasSuffix(r.URL.Path, "/chat/completions") && !strings.HasSuffix(r.URL.Path, "/completions") {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	r.Body.Close()

	// 1. Classify request metadata
	reqCtx, err := s.classifier.Classify(body)
	if err != nil {
		log.Printf("[nacho-flow] Warning: Failed to classify payload: %v", err)
	}

	// 2. Evaluate 1..N tiers using expr engine
	targetTier, err := s.evaluator.SelectTier(reqCtx)
	if err != nil {
		log.Printf("[nacho-flow] Error evaluating tier: %v, falling back to default", err)
		targetTier = s.config.DefaultTier
	}

	log.Printf("[nacho-flow] Routing -> Tier: '%s' | Model: '%s' | Provider: '%s' | Tokens: ~%d | Images: %v | Tools: %v",
		targetTier.Name, targetTier.Model, targetTier.Provider, reqCtx.Tokens, reqCtx.HasImages, reqCtx.HasTools)

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

	// 4. Sanitize history images if target model lacks vision or strip_images is enabled
	hasVision := strings.Contains(strings.ToLower(targetTier.Model), "vision") || strings.Contains(strings.ToLower(targetTier.Model), "flash") || targetTier.Provider == "openrouter"
	if targetTier.StripImages {
		hasVision = false
	}
	body, _ = s.sanitizer.SanitizePayload(body, hasVision)

	// 5. Determine target provider URL
	targetBaseURL, exists := s.config.Providers[targetTier.Provider]
	if !exists || targetBaseURL == "" {
		targetBaseURL = s.config.Providers["openrouter"]
	}

	targetURL, err := url.Parse(targetBaseURL)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid target URL for provider %s: %v", targetTier.Provider, err), http.StatusInternalServerError)
		return
	}

	// 6. Reverse Proxy Setup
	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	// Custom Director to modify outgoing request headers and body
	originalDirector := proxy.Director
	proxy.Director = func(outReq *http.Request) {
		originalDirector(outReq)
		outReq.Host = targetURL.Host
		outReq.URL.Scheme = targetURL.Scheme
		outReq.URL.Host = targetURL.Host
		outReq.URL.Path = singleJoiningSlash(targetURL.Path, r.URL.Path)

		// Set API Key header
		openRouterKey := s.config.OpenRouterKey
		if openRouterKey == "ENV_OPENROUTER_API_KEY" || openRouterKey == "" {
			openRouterKey = os.Getenv("OPENROUTER_API_KEY")
		}

		if targetTier.Provider == "openrouter" && openRouterKey != "" {
			outReq.Header.Set("Authorization", "Bearer "+openRouterKey)
			outReq.Header.Set("HTTP-Referer", "https://spicerack.dev")
			outReq.Header.Set("X-Title", "nacho-flow")
		}

		// Re-assign body
		outReq.Body = io.NopCloser(bytes.NewReader(body))
		outReq.ContentLength = int64(len(body))
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
