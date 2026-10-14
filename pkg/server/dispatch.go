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
	if edited := modifyTopLevelPayload(body, targetTier, targetProvider); len(edited) > 0 {
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

// deleteMemberSpan calculates the byte range [delStart, delEnd) to cleanly delete a JSON object member
// [kStart, vEnd) along with its associated comma delimiter, avoiding syntax errors.
func deleteMemberSpan(body []byte, kStart, vEnd, startObj, closeObj int) (int, int) {
	// 1. Prefer deleting preceding comma if it exists (for members after the first)
	p := kStart - 1
	for p > startObj && (body[p] == ' ' || body[p] == '\t' || body[p] == '\n' || body[p] == '\r') {
		p--
	}
	if p > startObj && body[p] == ',' {
		return p, vEnd
	}

	// 2. If first member, delete up to and including succeeding comma
	n := vEnd
	for n < closeObj && (body[n] == ' ' || body[n] == '\t' || body[n] == '\n' || body[n] == '\r') {
		n++
	}
	if n < closeObj && body[n] == ',' {
		return kStart, n + 1
	}

	// 3. Sole member in object: remove member bounds
	return kStart, vEnd
}

type memberSpan struct {
	kStart int
	kEnd   int
	vStart int
	vEnd   int
}

var (
	jsonNone                     = []byte(`"none"`)
	jsonEmptyStr                 = []byte(`""`)
	jsonNull                     = []byte(`null`)
	jsonEffortKey                = []byte("effort")
	jsonTrue                     = []byte("true")
	jsonIncludeUsageObj          = []byte(`{"include_usage":true}`)
	jsonIncludeUsageKey          = []byte(`"include_usage":true`)
	jsonIncludeUsageKeyWithComma = []byte(`,"include_usage":true`)
)

// isEffortNone returns true if the raw JSON value represents an empty or "none" reasoning effort.
func isEffortNone(val []byte) bool {
	val = bytes.TrimSpace(val)
	return bytes.Equal(val, jsonNone) || bytes.Equal(val, jsonEmptyStr) || bytes.Equal(val, jsonNull)
}

// isNestedReasoningSoleEffort checks if [objStart, objEnd) contains "effort": "none" and whether it is the sole key.
func isNestedReasoningSoleEffort(body []byte, objStart, objEnd int) (soleEffort bool, hasEffortNone bool, effortKStart, effortVEnd int) {
	kStart, valStart, valEnd, ok := zeroalloc.ObjectField(body, objStart, objEnd, "effort")
	if !ok || !isEffortNone(body[valStart:valEnd]) {
		return false, false, 0, 0
	}
	hasEffortNone = true
	effortKStart = kStart
	effortVEnd = valEnd

	soleEffort = true
	i := objStart + 1
	for i < objEnd-1 {
		for i < objEnd-1 && (body[i] == ' ' || body[i] == '\t' || body[i] == '\n' || body[i] == '\r' || body[i] == ',') {
			i++
		}
		if i >= objEnd-1 || body[i] == '}' {
			break
		}
		if body[i] != '"' {
			break
		}
		ks := i
		ke, okStr := zeroalloc.SkipValue(body, i)
		if !okStr {
			break
		}
		if !bytes.Equal(body[ks+1:ke-1], jsonEffortKey) {
			soleEffort = false
			break
		}
		for ke < objEnd-1 && (body[ke] == ' ' || body[ke] == '\t' || body[ke] == '\n' || body[ke] == '\r') {
			ke++
		}
		if ke < objEnd-1 && body[ke] == ':' {
			ke++
		}
		ve, okVal := zeroalloc.SkipValue(body, ke)
		if !okVal {
			break
		}
		i = ve
	}
	return soleEffort, hasEffortNone, effortKStart, effortVEnd
}

// coalesceDeleteSpans merges contiguous or overlapping deleted member spans in sorted order.
// Returns the number of unique spans remaining in spans[:count] without allocating.
func coalesceDeleteSpans(body []byte, spans []memberSpan) int {
	if len(spans) <= 1 {
		return len(spans)
	}
	for a := 1; a < len(spans); a++ {
		for b := a; b > 0 && spans[b].kStart < spans[b-1].kStart; b-- {
			spans[b], spans[b-1] = spans[b-1], spans[b]
		}
	}
	w := 0
	for i := 1; i < len(spans); i++ {
		prevEnd := spans[w].vEnd
		nextStart := spans[i].kStart
		isContiguous := true
		for k := prevEnd; k < nextStart; k++ {
			c := body[k]
			if c != ' ' && c != '\t' && c != '\n' && c != '\r' && c != ',' {
				isContiguous = false
				break
			}
		}
		if isContiguous {
			if spans[i].vEnd > spans[w].vEnd {
				spans[w].vEnd = spans[i].vEnd
			}
		} else {
			w++
			spans[w] = spans[i]
		}
	}
	return w + 1
}

var scratchPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 256)
		return &b
	},
}

func modifyTopLevelPayload(body []byte, targetTier *contract.Tier, targetProvider provider.LLMProvider) []byte {
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

	decl := provider.DefaultWireDeclarator
	if wdp, ok := targetProvider.(provider.WireDeclaratorProvider); ok {
		if d := wdp.WireDeclarator(); d != nil {
			decl = d
		}
	}

	var editsBuf [8]zeroalloc.Edit
	edits := editsBuf[:0]

	var scratchPtr *[]byte
	var scratch []byte
	var modelRepl []byte
	var reRepl []byte
	if targetTier.ReasoningEffort != "" {
		scratchPtr = scratchPool.Get().(*[]byte)
		scratch = (*scratchPtr)[:0]
		sStart := len(scratch)
		scratch = zeroalloc.AppendEscapedJSONString(scratch, targetTier.ReasoningEffort)
		reRepl = scratch[sStart:]
	}

	foundModel := false
	var reSpansBuf [4]memberSpan
	reSpans := reSpansBuf[:0]
	var reasoningSpansBuf [4]memberSpan
	reasoningSpans := reasoningSpansBuf[:0]
	isStream := false
	type span struct{ start, end int }
	var streamOptionsSpansBuf [4]span
	streamOptionsSpans := streamOptionsSpansBuf[:0]

	totalMembers := 0

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
		totalMembers++

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

		switch string(body[kStart+1 : kEnd-1]) {
		case "model":
			foundModel = true
			if vEnd-vStart >= 2 && body[vStart] == '"' && body[vEnd-1] == '"' && string(body[vStart+1:vEnd-1]) == targetTier.Model {
				// Model already matches target tier; zero edits, zero allocations
			} else {
				if modelRepl == nil {
					if scratchPtr == nil {
						scratchPtr = scratchPool.Get().(*[]byte)
						scratch = (*scratchPtr)[:0]
					}
					sStart := len(scratch)
					scratch = zeroalloc.AppendEscapedJSONString(scratch, targetTier.Model)
					modelRepl = scratch[sStart:]
				}
				edits = append(edits, zeroalloc.Edit{Start: vStart, End: vEnd, Repl: modelRepl})
			}
		case "reasoning_effort":
			reSpans = append(reSpans, memberSpan{kStart: kStart, kEnd: kEnd, vStart: vStart, vEnd: vEnd})
		case "reasoning":
			reasoningSpans = append(reasoningSpans, memberSpan{kStart: kStart, kEnd: kEnd, vStart: vStart, vEnd: vEnd})
		case "stream":
			valBytes := bytes.TrimSpace(body[vStart:vEnd])
			if bytes.Equal(valBytes, jsonTrue) {
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
						edits = append(edits, zeroalloc.Edit{Start: iuVStart, End: iuVEnd, Repl: jsonTrue})
					} else {
						// Insert before '}'
						hasElems := false
						for k := s + 1; k < e; k++ {
							if body[k] != ' ' && body[k] != '\t' && body[k] != '\n' && body[k] != '\r' {
								hasElems = true
								break
							}
						}
						repl := jsonIncludeUsageKey
						if hasElems {
							repl = jsonIncludeUsageKeyWithComma
						}
						edits = append(edits, zeroalloc.Edit{Start: e, End: e, Repl: repl})
					}
				} else {
					// Not an object (e.g. null) -> replace with {"include_usage":true}
					edits = append(edits, zeroalloc.Edit{Start: sp.start, End: sp.end, Repl: jsonIncludeUsageObj})
				}
			}
		}
	}

	// 1. Evaluate reasoning normalization first so any deleted top-level members are accounted for.
	numDeletedTopMembers := 0
	var missingReasoningIns []byte

	if targetTier.ReasoningEffort != "" {
		if decl.Dialect == provider.ReasoningDialectNested {
			orValStart := len(scratch)
			scratch = append(scratch, `{"effort":`...)
			scratch = append(scratch, reRepl...)
			scratch = append(scratch, '}')
			orVal := scratch[orValStart:]

			if len(reasoningSpans) > 0 {
				// Replace the value of the first "reasoning" key
				edits = append(edits, zeroalloc.Edit{
					Start: reasoningSpans[0].vStart,
					End:   reasoningSpans[0].vEnd,
					Repl:  orVal,
				})
				// Collect spans to delete: duplicate reasoning keys + flat reasoning_effort keys
				var delBuf [8]memberSpan
				delSpans := delBuf[:0]
				for _, sp := range reasoningSpans[1:] {
					delSpans = append(delSpans, sp)
				}
				if decl.StripFlatOnNested {
					for _, sp := range reSpans {
						delSpans = append(delSpans, sp)
					}
				}
				if len(delSpans) > 0 {
					numDeletedTopMembers += len(delSpans)
					n := coalesceDeleteSpans(body, delSpans)
					for _, sp := range delSpans[:n] {
						dStart, dEnd := deleteMemberSpan(body, sp.kStart, sp.vEnd, startObj, closeObj)
						edits = append(edits, zeroalloc.Edit{Start: dStart, End: dEnd, Repl: nil})
					}
				}
			} else if len(reSpans) > 0 {
				// Inbound request had flat "reasoning_effort" but no "reasoning".
				// Mutate the first "reasoning_effort" key-value directly into "reasoning": {"effort": ...}
				orMemberStart := len(scratch)
				scratch = append(scratch, `"reasoning":`...)
				scratch = append(scratch, orVal...)
				orMember := scratch[orMemberStart:]

				edits = append(edits, zeroalloc.Edit{
					Start: reSpans[0].kStart,
					End:   reSpans[0].vEnd,
					Repl:  orMember,
				})
				// Delete any duplicate "reasoning_effort" fields
				if len(reSpans) > 1 {
					numDeletedTopMembers += len(reSpans) - 1
					n := coalesceDeleteSpans(body, reSpans[1:])
					for _, sp := range reSpans[1 : 1+n] {
						dStart, dEnd := deleteMemberSpan(body, sp.kStart, sp.vEnd, startObj, closeObj)
						edits = append(edits, zeroalloc.Edit{Start: dStart, End: dEnd, Repl: nil})
					}
				}
			} else {
				// Neither was present: queue nested reasoning for insertion before closeObj
				orInsStart := len(scratch)
				scratch = append(scratch, `"reasoning":`...)
				scratch = append(scratch, orVal...)
				missingReasoningIns = scratch[orInsStart:]
			}
		} else {
			// Standard Cloud / Local: flat "reasoning_effort"
			if len(reSpans) > 0 {
				for _, sp := range reSpans {
					edits = append(edits, zeroalloc.Edit{
						Start: sp.vStart,
						End:   sp.vEnd,
						Repl:  reRepl,
					})
				}
			} else {
				flatInsStart := len(scratch)
				scratch = append(scratch, `"reasoning_effort":`...)
				scratch = append(scratch, reRepl...)
				missingReasoningIns = scratch[flatInsStart:]
			}
		}
	} else if decl.ForbidsEffortNone {
		// targetTier.ReasoningEffort == "" and provider forbids "none":
		// Check inbound client spans individually.
		var delBuf [8]memberSpan
		delSpans := delBuf[:0]

		for _, sp := range reSpans {
			if isEffortNone(body[sp.vStart:sp.vEnd]) {
				delSpans = append(delSpans, sp)
			}
		}

		for _, sp := range reasoningSpans {
			sole, hasNone, effKStart, effVEnd := isNestedReasoningSoleEffort(body, sp.vStart, sp.vEnd)
			if hasNone {
				if sole {
					delSpans = append(delSpans, sp)
				} else {
					// Delete only the "effort": "none" member inside the "reasoning" object
					dStart, dEnd := deleteMemberSpan(body, effKStart, effVEnd, sp.vStart, sp.vEnd)
					edits = append(edits, zeroalloc.Edit{Start: dStart, End: dEnd, Repl: nil})
				}
			}
		}

		if len(delSpans) > 0 {
			numDeletedTopMembers += len(delSpans)
			n := coalesceDeleteSpans(body, delSpans)
			for _, sp := range delSpans[:n] {
				dStart, dEnd := deleteMemberSpan(body, sp.kStart, sp.vEnd, startObj, closeObj)
				edits = append(edits, zeroalloc.Edit{Start: dStart, End: dEnd, Repl: nil})
			}
		}
	}

	hasRemainingElements := totalMembers > numDeletedTopMembers

	// Insert missing top-level keys before closeObj
	if !foundModel && targetTier.Model != "" || len(missingReasoningIns) > 0 || (isStream && len(streamOptionsSpans) == 0) {
		if scratchPtr == nil {
			scratchPtr = scratchPool.Get().(*[]byte)
			scratch = (*scratchPtr)[:0]
		}
	}

	topInsStart := len(scratch)
	if !foundModel && targetTier.Model != "" {
		if hasRemainingElements || len(scratch) > topInsStart {
			scratch = append(scratch, ',')
		}
		scratch = append(scratch, `"model":`...)
		if modelRepl != nil {
			scratch = append(scratch, modelRepl...)
		} else {
			scratch = zeroalloc.AppendEscapedJSONString(scratch, targetTier.Model)
		}
	}

	if len(missingReasoningIns) > 0 {
		if hasRemainingElements || len(scratch) > topInsStart {
			scratch = append(scratch, ',')
		}
		scratch = append(scratch, missingReasoningIns...)
	}

	if isStream && len(streamOptionsSpans) == 0 {
		if hasRemainingElements || len(scratch) > topInsStart {
			scratch = append(scratch, ',')
		}
		scratch = append(scratch, `"stream_options":{"include_usage":true}`...)
	}

	if len(scratch) > topInsStart {
		edits = append(edits, zeroalloc.Edit{
			Start: closeObj,
			End:   closeObj,
			Repl:  scratch[topInsStart:],
		})
	}

	if len(edits) == 0 {
		if scratchPtr != nil {
			scratchPool.Put(scratchPtr)
		}
		return body
	}

	// Sort edits by Start offset in-place without interface boxing
	for a := 1; a < len(edits); a++ {
		for b := a; b > 0 && edits[b].Start < edits[b-1].Start; b-- {
			edits[b], edits[b-1] = edits[b-1], edits[b]
		}
	}

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
	if scratchPtr != nil {
		scratchPool.Put(scratchPtr)
	}
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

	cleanDir := filepath.Clean(dumpDir)
	// #nosec G703 - diagnostic dump directory explicitly configured via environment variable
	if err := os.MkdirAll(cleanDir, 0700); err != nil {
		reqLogger.Warn("Failed to create outbound dump directory", slog.String("dir", cleanDir), slog.Any("error", err))
		return
	}

	cnt := atomic.AddUint64(&dumpCounter, 1)
	filename := fmt.Sprintf("outbound_%d_%06d.json", time.Now().UnixMilli(), cnt)
	filePath := filepath.Join(cleanDir, filename)

	// #nosec G703 - diagnostic dump file path constructed from clean directory and internal counter
	if err := os.WriteFile(filePath, bodyBytes, 0600); err != nil {
		reqLogger.Warn("Failed to write outbound dump file", slog.String("file", filePath), slog.Any("error", err))
		return
	}
}
