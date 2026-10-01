// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/agentregistry"
	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/provider"
)

// streamBufferBundle pools peek and stream transfer byte buffers to guarantee zero heap allocations.
type streamBufferBundle struct {
	peekBuf []byte    // capacity 4096, reset with [:0]
	readBuf []byte    // capacity 4096
	tempBuf [512]byte // 512-byte scratch reader
}

func (b *streamBufferBundle) reset() {
	b.peekBuf = b.peekBuf[:0]
}

var streamBufferPool = sync.Pool{
	New: func() any {
		return &streamBufferBundle{
			peekBuf: make([]byte, 0, 4096),
			readBuf: make([]byte, 4096),
		}
	},
}

type fastMessage struct {
	Role             string          `json:"role"`
	Content          string          `json:"content"`
	ReasoningContent string          `json:"reasoning_content,omitempty"`
	Reasoning        string          `json:"reasoning,omitempty"`
	Reason           string          `json:"reason,omitempty"`
	ToolCalls        json.RawMessage `json:"tool_calls,omitempty"`
}

type fastChoice struct {
	Index        int             `json:"index"`
	Message      fastMessage     `json:"message"`
	FinishReason string          `json:"finish_reason"`
	Logprobs     json.RawMessage `json:"logprobs,omitempty"`
}

type fastChatCompletionResponse struct {
	ID                string          `json:"id"`
	Object            string          `json:"object"`
	Created           int64           `json:"created"`
	Model             string          `json:"model"`
	Choices           []fastChoice    `json:"choices"`
	Usage             json.RawMessage `json:"usage,omitempty"`
	SystemFingerprint string          `json:"system_fingerprint,omitempty"`
}

func (s *Server) forwardWithFallback(
	w http.ResponseWriter,
	r *http.Request,
	reqCtx contract.RequestContext,
	targetTier contract.Tier,
	body []byte,
	startTime time.Time,
	reqLogger *slog.Logger,
) {
	isFallback := false
	s.dispatchTier(w, r, reqCtx, targetTier, body, startTime, reqLogger, isFallback)
}

// dispatchTier is the central orchestrator responsible for circuit-breaker validation,
// outbound request construction, network round-trip, and delegating to streaming or unary response handlers.
func (s *Server) dispatchTier(
	w http.ResponseWriter,
	r *http.Request,
	reqCtx contract.RequestContext,
	targetTier contract.Tier,
	body []byte,
	startTime time.Time,
	reqLogger *slog.Logger,
	isFallback bool,
) {
	globalShield := s.GetConfig().AgentShield.Enabled == nil || *s.GetConfig().AgentShield.Enabled
	activeFeatures := resolveFeatureFlags(reqCtx, targetTier, globalShield)
	reqCtx.Features = uint16(activeFeatures)

	// 1. Check provider registry & Circuit Breaker status
	reg := s.GetRegistry()
	var targetProvider provider.LLMProvider
	var exists bool
	if reg != nil {
		targetProvider, exists = reg.Get(targetTier.Provider)
	}

	defaultTier := s.GetConfig().DefaultTier
	if exists && !s.allowProvider(targetProvider) {
		reqLogger.Warn("Target provider circuit breaker OPEN, bypassing to default tier",
			slog.String("provider", targetTier.Provider),
			slog.String("tier", targetTier.Name),
		)
		if !isFallback && defaultTier.Provider != "" && targetTier.Name != defaultTier.Name {
			s.dispatchTier(w, r, reqCtx, defaultTier, body, startTime, reqLogger, true)
			return
		}
	}

	if !exists {
		reqLogger.Error("Target provider not found in registry", slog.String("provider", targetTier.Provider))
		if !isFallback && defaultTier.Provider != "" && targetTier.Name != defaultTier.Name {
			s.dispatchTier(w, r, reqCtx, defaultTier, body, startTime, reqLogger, true)
			return
		}
		http.Error(w, fmt.Sprintf("Provider not found: %s", targetTier.Provider), http.StatusBadGateway)
		return
	}

	// 2. Prepare payload (model rewrite, reasoning effort, stream options, sanitization, NTS)
	preparedBody, ntsTokensSaved, ntsBytesSaved := s.preparePayload(r, &targetTier, targetProvider, body)

	// 3. Build outbound request via polymorphic provider adapter (Open-Closed Pattern)
	outReq, err := s.buildOutboundRequest(r.Context(), r, targetProvider, targetTier.Model, preparedBody)
	if err != nil {
		reqLogger.Error("Failed to build outbound upstream request", slog.Any("error", err))
		http.Error(w, fmt.Sprintf("Invalid target URL or request for provider %s: %v", targetTier.Provider, err), http.StatusInternalServerError)
		return
	}

	// 4. Execute network round-trip via connection pool
	resp, err := s.transport.RoundTrip(outReq)
	if err != nil || (resp != nil && resp.StatusCode >= 500) {
		s.recordProviderFailure(targetProvider)
		s.recordProxyError()
		if resp != nil {
			_ = resp.Body.Close()
		}
		reqLogger.Warn("Upstream provider failure",
			slog.String("tier", targetTier.Name),
			slog.String("provider", targetTier.Provider),
			slog.Any("error", err),
		)
		if !isFallback && defaultTier.Provider != "" && targetTier.Name != defaultTier.Name {
			reqLogger.Info("Retrying with default fallback tier", slog.String("fallback_tier", defaultTier.Name))
			s.dispatchTier(w, r, reqCtx, defaultTier, body, startTime, reqLogger, true)
			return
		}
		http.Error(w, fmt.Sprintf("Proxy error: %v", err), http.StatusBadGateway)
		return
	}

	// 5. Delegate to specialized stage handler (Streaming vs Unary)
	isStreaming := strings.Contains(resp.Header.Get(contract.HeaderContentType), contract.ContentTypeEventStream)
	if isStreaming {
		s.handleStreaming(w, r, reqCtx, targetTier, targetProvider, resp, body, startTime, reqLogger, isFallback, activeFeatures, ntsTokensSaved, ntsBytesSaved)
	} else {
		s.handleNonStreaming(w, r, reqCtx, targetTier, targetProvider, resp, body, startTime, reqLogger, isFallback, activeFeatures, ntsTokensSaved, ntsBytesSaved)
	}
}

// buildOutboundRequest constructs an upstream HTTP request using the provider's protocol adapter,
// with safe fallback for legacy or test providers.
func (s *Server) buildOutboundRequest(ctx context.Context, r *http.Request, p provider.LLMProvider, model string, body []byte) (*http.Request, error) {
	if protoProvider, ok := p.(provider.ProtocolProvider); ok {
		return protoProvider.BuildUpstreamRequest(ctx, r, model, body)
	}

	// Fallback for custom test providers that only implement LLMProvider
	targetURL, err := url.Parse(p.BaseURL())
	if err != nil {
		return nil, fmt.Errorf("invalid target URL for provider %s: %w", p.ID(), err)
	}

	reqPath := r.URL.Path
	targetBasePath := strings.TrimRight(targetURL.Path, "/")
	if strings.HasSuffix(targetBasePath, "/v1") && strings.HasPrefix(reqPath, "/v1/") {
		reqPath = strings.TrimPrefix(reqPath, "/v1")
	}
	fullTargetURL := singleJoiningSlash(targetURL.String(), reqPath)

	// #nosec G704 - outgoing proxy request to configured upstream provider endpoint
	outReq, err := http.NewRequestWithContext(ctx, r.Method, fullTargetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	for k, vv := range r.Header {
		for _, v := range vv {
			outReq.Header.Add(k, v)
		}
	}
	outReq.Host = targetURL.Host

	if p.IsLocal() {
		outReq.Header.Del(contract.HeaderAuthorization)
	} else if auth, ok := p.(provider.AuthProvider); ok {
		if apiKey := auth.GetAPIKey(); apiKey != "" {
			outReq.Header.Set(contract.HeaderAuthorization, "Bearer "+apiKey)
		}
	}
	if hdr, ok := p.(provider.HeaderProvider); ok {
		for k, v := range hdr.GetHeaders() {
			outReq.Header.Set(k, v)
		}
	}
	outReq.Header.Set(contract.HeaderContentLength, strconv.Itoa(len(body)))
	return outReq, nil
}

// preparePayload performs model rewriting, reasoning effort injection, stream options enforcement,
// vision sanitization, local token scrubbing, and NTS token compression with zero deep interface{} allocations.
func (s *Server) preparePayload(
	r *http.Request,
	targetTier *contract.Tier,
	targetProvider provider.LLMProvider,
	body []byte,
) ([]byte, int, int) {
	preparedBody := body

	// Fast top-level JSON modification using map[string]json.RawMessage.
	// This avoids expanding the entire messages/tools array into thousands of heap-allocated interface{} maps.
	var rawPayload map[string]json.RawMessage
	if err := json.Unmarshal(body, &rawPayload); err == nil {
		if modelBytes, err := json.Marshal(targetTier.Model); err == nil {
			rawPayload["model"] = modelBytes
		}
		if targetTier.ReasoningEffort != "" {
			if reBytes, err := json.Marshal(targetTier.ReasoningEffort); err == nil {
				rawPayload["reasoning_effort"] = reBytes
			}
		}
		var isStream bool
		if streamRaw, ok := rawPayload["stream"]; ok {
			_ = json.Unmarshal(streamRaw, &isStream)
		}
		if isStream {
			opts := make(map[string]json.RawMessage)
			if optsRaw, ok := rawPayload["stream_options"]; ok && len(optsRaw) > 0 {
				_ = json.Unmarshal(optsRaw, &opts)
			}
			opts["include_usage"] = json.RawMessage("true")
			if reOpts, err := json.Marshal(opts); err == nil {
				rawPayload["stream_options"] = reOpts
			}
		}
		if reencoded, err := json.Marshal(rawPayload); err == nil {
			preparedBody = reencoded
		}
	}

	ResolveTierVision(targetTier, s.oracle)
	hasVision := targetTier.ResolvedHasVision
	if targetTier.StripImages {
		hasVision = false
	}
	if s.sanitizer != nil {
		preparedBody, _ = s.sanitizer.SanitizePayload(preparedBody, hasVision)
	}

	if targetProvider != nil && targetProvider.IsLocal() {
		if reg := agentregistry.DefaultRegistry(); reg != nil && reg.HasControlTokenMarker(preparedBody) {
			preparedBody = reg.StripControlTokensInPlace(preparedBody)
		}
	}

	var ntsTokensSaved, ntsBytesSaved int
	if ntsTr := s.GetNTSTransformer(); ntsTr != nil {
		if strings.Contains(r.URL.Path, "messages") || r.Header.Get("anthropic-version") != "" {
			if transformed, res, err := ntsTr.TransformAnthropic(preparedBody); err == nil && !res.Bypassed {
				preparedBody = transformed
				ntsTokensSaved = res.TokensSaved
				ntsBytesSaved = res.BytesSaved
			}
		} else {
			if transformed, res, err := ntsTr.TransformOpenAI(preparedBody); err == nil && !res.Bypassed {
				preparedBody = transformed
				ntsTokensSaved = res.TokensSaved
				ntsBytesSaved = res.BytesSaved
			}
		}
	}

	return preparedBody, ntsTokensSaved, ntsBytesSaved
}

func (s *Server) wrapResponseStream(resp *http.Response, p provider.LLMProvider) io.ReadCloser {
	if protoProvider, ok := p.(provider.ProtocolProvider); ok {
		return protoProvider.WrapResponseStream(resp)
	}
	return resp.Body
}

func (s *Server) translateResponseBody(statusCode int, body []byte, p provider.LLMProvider) ([]byte, error) {
	if protoProvider, ok := p.(provider.ProtocolProvider); ok {
		return protoProvider.TranslateResponseBody(statusCode, body)
	}
	return body, nil
}
