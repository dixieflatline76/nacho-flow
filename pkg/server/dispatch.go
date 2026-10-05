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
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/agentregistry"
	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/provider"
	"github.com/dixieflatline76/nacho-flow/pkg/zeroalloc"
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

	// 2. Prepare payload (model rewrite, reasoning effort, stream options, sanitization)
	preparedBody := s.preparePayload(&targetTier, targetProvider, body)

	// 3. Build outbound request via polymorphic provider adapter (Open-Closed Pattern)
	outReq, err := s.buildOutboundRequest(r.Context(), r, targetProvider, targetTier.Model, preparedBody)
	if err != nil {
		reqLogger.Error("Failed to build outbound upstream request", slog.Any("error", err))
		http.Error(w, fmt.Sprintf("Invalid target URL or request for provider %s: %v", targetTier.Provider, err), http.StatusInternalServerError)
		return
	}

	dumpOutbound(outReq, reqLogger)

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
		s.handleStreaming(w, r, reqCtx, targetTier, targetProvider, resp, body, startTime, reqLogger, isFallback, activeFeatures, reqCtx.NTSTokensSaved, reqCtx.NTSBytesSaved)
	} else {
		s.handleNonStreaming(w, r, reqCtx, targetTier, targetProvider, resp, body, startTime, reqLogger, isFallback, activeFeatures, reqCtx.NTSTokensSaved, reqCtx.NTSBytesSaved)
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
// vision sanitization, and local token scrubbing with zero deep interface{} allocations.
func (s *Server) preparePayload(
	targetTier *contract.Tier,
	targetProvider provider.LLMProvider,
	body []byte,
) []byte {
	preparedBody := body

	// Fast zero-alloc top-level JSON field editing.
	// Only modified fields (model, reasoning_effort, stream_options) are touched;
	// tools, messages, seeds, and key order remain 100% byte-identical to the client's payload.
	if edited := modifyTopLevelPayload(body, targetTier); len(edited) > 0 {
		preparedBody = edited
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

	return preparedBody
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

func modifyTopLevelPayload(body []byte, targetTier *contract.Tier) []byte {
	// Find top-level object bounds
	startObj := 0
	for startObj < len(body) && (body[startObj] == ' ' || body[startObj] == '\t' || body[startObj] == '\n' || body[startObj] == '\r') {
		startObj++
	}
	if startObj >= len(body) || body[startObj] != '{' {
		return body
	}

	closeObj := len(body) - 1
	for closeObj > startObj && (body[closeObj] == ' ' || body[closeObj] == '\t' || body[closeObj] == '\n' || body[closeObj] == '\r') {
		closeObj--
	}
	if closeObj <= startObj || body[closeObj] != '}' {
		return body
	}

	var edits []zeroalloc.Edit

	modelRepl := zeroalloc.AppendEscapedJSONString(nil, targetTier.Model)
	var reRepl []byte
	if targetTier.ReasoningEffort != "" {
		reRepl = zeroalloc.AppendEscapedJSONString(nil, targetTier.ReasoningEffort)
	}

	foundModel := false
	foundRE := false
	isStream := false
	type span struct{ start, end int }
	var streamOptionsSpans []span

	// Scan top-level members to locate existing keys (handling duplicate keys by recording all occurrences)
	i := startObj + 1
	for i < closeObj {
		for i < closeObj && (body[i] == ' ' || body[i] == '\t' || body[i] == '\n' || body[i] == '\r') {
			i++
		}
		if i >= closeObj || body[i] == '}' {
			break
		}
		if body[i] == ',' {
			i++
			continue
		}
		if body[i] != '"' {
			return body // invalid top-level object structure
		}

		kStart := i
		kEnd, okStr := zeroalloc.SkipValue(body, i)
		if !okStr || kEnd > closeObj {
			return body
		}
		i = kEnd

		for i < closeObj && (body[i] == ' ' || body[i] == '\t' || body[i] == '\n' || body[i] == '\r') {
			i++
		}
		if i >= closeObj || body[i] != ':' {
			return body
		}
		i++ // skip ':'

		for i < closeObj && (body[i] == ' ' || body[i] == '\t' || body[i] == '\n' || body[i] == '\r') {
			i++
		}
		if i >= closeObj {
			return body
		}

		vStart := i
		vEnd, okVal := zeroalloc.SkipValue(body, i)
		if !okVal || vEnd > closeObj {
			return body
		}
		i = vEnd

		key := string(body[kStart+1 : kEnd-1])
		switch key {
		case "model":
			foundModel = true
			edits = append(edits, zeroalloc.Edit{Start: vStart, End: vEnd, Repl: modelRepl})
		case "reasoning_effort":
			if targetTier.ReasoningEffort != "" {
				foundRE = true
				edits = append(edits, zeroalloc.Edit{Start: vStart, End: vEnd, Repl: reRepl})
			}
		case "stream":
			valBytes := bytes.TrimSpace(body[vStart:vEnd])
			if bytes.Equal(valBytes, []byte("true")) {
				isStream = true
			}
		case "stream_options":
			streamOptionsSpans = append(streamOptionsSpans, span{start: vStart, end: vEnd})
		}
	}

	// Handle stream_options when streaming
	if isStream {
		if len(streamOptionsSpans) > 0 {
			for _, sp := range streamOptionsSpans {
				// Trim whitespace to see if it's an object
				s := sp.start
				for s < sp.end && (body[s] == ' ' || body[s] == '\t' || body[s] == '\n' || body[s] == '\r') {
					s++
				}
				e := sp.end - 1
				for e > s && (body[e] == ' ' || body[e] == '\t' || body[e] == '\n' || body[e] == '\r') {
					e--
				}
				if s < sp.end && body[s] == '{' && body[e] == '}' {
					// Check if include_usage exists inside stream_options object
					_, iuVStart, iuVEnd, okIU := zeroalloc.ObjectField(body, s, sp.end, "include_usage")
					if okIU {
						edits = append(edits, zeroalloc.Edit{Start: iuVStart, End: iuVEnd, Repl: []byte("true")})
					} else {
						// Insert before '}'
						hasElems := false
						for k := s + 1; k < e; k++ {
							if body[k] != ' ' && body[k] != '\t' && body[k] != '\n' && body[k] != '\r' {
								hasElems = true
								break
							}
						}
						var ins []byte
						if hasElems {
							ins = append(ins, ',')
						}
						ins = append(ins, `"include_usage":true`...)
						edits = append(edits, zeroalloc.Edit{Start: e, End: e, Repl: ins})
					}
				} else {
					// Not an object (e.g. null) -> replace with {"include_usage":true}
					edits = append(edits, zeroalloc.Edit{Start: sp.start, End: sp.end, Repl: []byte(`{"include_usage":true}`)})
				}
			}
		}
	}

	// Check if top-level has any existing members
	hasTopLevelElements := false
	for k := startObj + 1; k < closeObj; k++ {
		if body[k] != ' ' && body[k] != '\t' && body[k] != '\n' && body[k] != '\r' {
			hasTopLevelElements = true
			break
		}
	}

	// Insert missing top-level keys before closeObj
	var topIns []byte
	if !foundModel {
		if hasTopLevelElements || len(topIns) > 0 {
			topIns = append(topIns, ',')
		}
		topIns = append(topIns, `"model":`...)
		topIns = append(topIns, modelRepl...)
	}
	if targetTier.ReasoningEffort != "" && !foundRE {
		if hasTopLevelElements || len(topIns) > 0 {
			topIns = append(topIns, ',')
		}
		topIns = append(topIns, `"reasoning_effort":`...)
		topIns = append(topIns, reRepl...)
	}
	if isStream && len(streamOptionsSpans) == 0 {
		if hasTopLevelElements || len(topIns) > 0 {
			topIns = append(topIns, ',')
		}
		topIns = append(topIns, `"stream_options":{"include_usage":true}`...)
	}
	if len(topIns) > 0 {
		edits = append(edits, zeroalloc.Edit{Start: closeObj, End: closeObj, Repl: topIns})
	}

	// Sort edits by Start offset
	sort.Slice(edits, func(a, b int) bool {
		return edits[a].Start < edits[b].Start
	})

	// Calculate extra capacity needed if any edits expand
	extraCap := 0
	for _, e := range edits {
		spanLen := e.End - e.Start
		if len(e.Repl) > spanLen {
			extraCap += len(e.Repl) - spanLen
		}
	}

	// Safety Rule 4: never modify body; allocate a fresh destination buffer
	// so the caller's body remains 100% pristine for fallback tiers and cycle recovery.
	dst := make([]byte, 0, len(body)+extraCap)
	out, ok := zeroalloc.SpliceInto(dst, body, edits)
	if !ok {
		return body
	}
	return out
}

var dumpCounter uint64

// dumpOutbound captures the raw outbound request body to disk if NACHO_DUMP_OUTBOUND is set.
// It never logs headers to prevent accidental leakage of API keys.
func dumpOutbound(outReq *http.Request, reqLogger *slog.Logger) {
	dumpDir := os.Getenv("NACHO_DUMP_OUTBOUND")
	if dumpDir == "" || outReq == nil || outReq.GetBody == nil {
		return
	}

	rc, err := outReq.GetBody()
	if err != nil {
		reqLogger.Warn("Failed to get outbound request body for dump", slog.Any("error", err))
		return
	}
	defer rc.Close()

	bodyBytes, err := io.ReadAll(rc)
	if err != nil {
		reqLogger.Warn("Failed to read outbound request body for dump", slog.Any("error", err))
		return
	}

	if err := os.MkdirAll(dumpDir, 0700); err != nil {
		reqLogger.Warn("Failed to create outbound dump directory", slog.String("dir", dumpDir), slog.Any("error", err))
		return
	}

	cnt := atomic.AddUint64(&dumpCounter, 1)
	filename := fmt.Sprintf("outbound_%d_%06d.json", time.Now().UnixMilli(), cnt)
	filePath := filepath.Join(dumpDir, filename)

	if err := os.WriteFile(filePath, bodyBytes, 0600); err != nil {
		reqLogger.Warn("Failed to write outbound dump file", slog.String("file", filePath), slog.Any("error", err))
		return
	}
}
