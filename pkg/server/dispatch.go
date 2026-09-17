package server

import (
	"bytes"
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
	"github.com/dixieflatline76/nacho-flow/pkg/router"
	"github.com/dixieflatline76/nacho-flow/pkg/router/shield"
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

func isDefectiveEmptyContent(bodyBytes []byte) bool {
	var raw map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &raw); err != nil {
		return false
	}
	choices, ok := raw["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return true
	}
	firstChoice, ok := choices[0].(map[string]interface{})
	if !ok {
		return true
	}
	msg, ok := firstChoice["message"].(map[string]interface{})
	if !ok {
		return true
	}
	content, _ := msg["content"].(string)
	tools, hasTools := msg["tool_calls"].([]interface{})
	reasoning, _ := msg["reasoning_content"].(string)
	if reasoning == "" {
		reasoning, _ = msg["reasoning"].(string)
	}
	return strings.TrimSpace(content) == "" && (!hasTools || len(tools) == 0) && strings.TrimSpace(reasoning) == ""
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
	// Check Circuit Breaker before connecting
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

	// Prepare payload for target model
	preparedBody := body
	var rawPayload map[string]interface{}
	if err := json.Unmarshal(body, &rawPayload); err == nil {
		rawPayload["model"] = targetTier.Model
		if targetTier.ReasoningEffort != "" {
			rawPayload["reasoning_effort"] = targetTier.ReasoningEffort
		}
		if isStream, _ := rawPayload["stream"].(bool); isStream {
			if opts, ok := rawPayload["stream_options"].(map[string]interface{}); ok {
				opts["include_usage"] = true
			} else {
				rawPayload["stream_options"] = map[string]interface{}{
					"include_usage": true,
				}
			}
		}
		if reencoded, err := json.Marshal(rawPayload); err == nil {
			preparedBody = reencoded
		}
	}

	ResolveTierVision(&targetTier, s.oracle)
	hasVision := targetTier.ResolvedHasVision
	if targetTier.StripImages {
		hasVision = false
	}
	preparedBody, _ = s.sanitizer.SanitizePayload(preparedBody, hasVision)

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

	targetURL, err := url.Parse(targetProvider.BaseURL())
	if err != nil {
		reqLogger.Error("Invalid target URL for provider", slog.String("provider", targetTier.Provider), slog.Any("error", err))
		http.Error(w, fmt.Sprintf("Invalid target URL for provider %s: %v", targetTier.Provider, err), http.StatusInternalServerError)
		return
	}

	reqPath := r.URL.Path
	targetBasePath := strings.TrimRight(targetURL.Path, "/")
	if strings.HasSuffix(targetBasePath, "/v1") && strings.HasPrefix(reqPath, "/v1/") {
		reqPath = strings.TrimPrefix(reqPath, "/v1")
	}
	fullTargetURL := singleJoiningSlash(targetURL.String(), reqPath)
	// #nosec G704 - upstream proxy URL is constructed from validated registered provider base URL
	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, fullTargetURL, bytes.NewReader(preparedBody))
	if err != nil {
		reqLogger.Error("Failed to build upstream request", slog.Any("error", err))
		http.Error(w, "Internal proxy error", http.StatusInternalServerError)
		return
	}

	// Copy headers
	for k, vv := range r.Header {
		for _, v := range vv {
			outReq.Header.Add(k, v)
		}
	}
	outReq.Host = targetURL.Host

	// Provider Capability: AuthProvider
	if auth, ok := targetProvider.(provider.AuthProvider); ok {
		if apiKey := auth.GetAPIKey(); apiKey != "" {
			outReq.Header.Set(contract.HeaderAuthorization, "Bearer "+apiKey)
		}
	}
	// Provider Capability: HeaderProvider
	if hdr, ok := targetProvider.(provider.HeaderProvider); ok {
		for k, v := range hdr.GetHeaders() {
			outReq.Header.Set(k, v)
		}
	}
	outReq.Header.Set(contract.HeaderContentLength, strconv.Itoa(len(preparedBody)))

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

	isStreaming := strings.Contains(resp.Header.Get(contract.HeaderContentType), contract.ContentTypeEventStream)

	if isStreaming {
		normalizer := NewStreamNormalizer(resp.Body)
		normalizer.SetFeatures(reqCtx.Features)
		if activeFeatures.Has(router.FeatureShieldEnabled) && reqCtx.InteractiveTool != "" {
			normalizer.SetShield(reqCtx.InteractiveTool, s.shieldMgr)
		}
		var cb *shield.CycleBreaker
		if reqCtx.HasTools && !reqCtx.NoCycleKiller {
			cb = resolveCycleBreaker(targetTier, s.GetConfig())
			if cb != nil && cb.IsEnabled() {
				normalizer.SetCycleBreaker(cb)
			}
		}

		sBuf := streamBufferPool.Get().(*streamBufferBundle)
		defer func() {
			sBuf.reset()
			streamBufferPool.Put(sBuf)
		}()

		var readErr error
		for len(sBuf.peekBuf) < 2048 {
			n, err := normalizer.Read(sBuf.tempBuf[:])
			if n > 0 {
				sBuf.peekBuf = append(sBuf.peekBuf, sBuf.tempBuf[:n]...)
			}
			if err != nil {
				readErr = err
				break
			}
			if violated, _ := normalizer.CheckCycleViolation(); violated {
				break
			}
		}

		// Check quality defect on stream: immediate [DONE] on local provider
		trimmedPeek := strings.TrimSpace(string(sBuf.peekBuf))
		if targetProvider.IsLocal() && !isFallback && (trimmedPeek == "data: [DONE]" || (len(sBuf.peekBuf) == 0 && readErr == io.EOF)) {
			_ = normalizer.Close()
			reqLogger.Warn("Local provider returned empty stream, failing over to cloud fallback tier",
				slog.String("tier", targetTier.Name),
			)
			if defaultTier.Provider != "" && targetTier.Name != defaultTier.Name {
				s.dispatchTier(w, r, reqCtx, defaultTier, body, startTime, reqLogger, true)
				return
			}
		}

		// Check Cycle Breaker violation on stream peek (Phase 1)
		if cb != nil && cb.IsEnabled() && !isFallback {
			if violated, reason := normalizer.CheckCycleViolation(); violated {
				reqCtx.CycleBreakerTriggered = true
				reqCtx.CycleBreakerReason = reason
				reqCtx.CycleContentTokens = cb.ContentTokens()
				reqCtx.CycleMaxNgramFreq = cb.MaxNgramFreq()
				reqCtx.CycleThinkingTokens = cb.ThinkingTokens()
				reqCtx.CycleMaxThinkingNgramFreq = cb.MaxThinkingNgramFreq()
				reqCtx.CycleToolTokens = cb.ToolTokens()
				reqCtx.CycleMaxToolNgramFreq = cb.MaxToolNgramFreq()
				_ = normalizer.Close()
				reqLogger.Warn("Cycle killer (qu'est-ce que c'est?): Runaway monologue intercepted",
					slog.String("reason", reason),
					slog.String("tier", targetTier.Name),
					slog.Int("cycle_retries", reqCtx.CycleRetries),
				)

				if reqCtx.CycleRetries < cb.MaxRetries() {
					// Stage 1: Local self-correction retry ($0.00) with [SYSTEM OVERRIDE] prompt
					reqCtx.CycleRetries++
					injectedBody := injectCorrectionPrompt(body, cb.CorrectionPrompt())
					s.dispatchTier(w, r, reqCtx, targetTier, injectedBody, startTime, reqLogger, false)
					return
				}

				if s.sessionTracker != nil {
					cooldown, floor := s.resolveCycleKillParams()
					s.sessionTracker.RecordCycleKill(extractSessionKey(r), targetTier.Model, cooldown, floor)
				}

				// Stage 2: Cloud Failover after local retry exhaustion
				if defaultTier.Provider != "" && targetTier.Name != defaultTier.Name {
					reqLogger.Warn("Local cycle retries exhausted, escalating to cloud fallback tier",
						slog.String("fallback_tier", defaultTier.Name),
						slog.String("fallback_model", defaultTier.Model),
					)
					s.dispatchTier(w, r, reqCtx, defaultTier, body, startTime, reqLogger, true)
					return
				}

				// Stage 3: Sever stream immediately with loop notice if no cloud fallback tier is configured
				w.Header().Set(contract.HeaderContentType, contract.ContentTypeEventStream)
				w.Header().Set(contract.HeaderNachoRouterTier, targetTier.Name)
				w.Header().Set(contract.HeaderSpiceRouterTier, targetTier.Name)
				w.Header().Set(contract.HeaderNachoTargetModel, targetTier.Model)
				w.Header().Set(contract.HeaderSpiceTargetModel, targetTier.Model)
				w.WriteHeader(http.StatusOK)

				isToolViolation := normalizer.HasActiveToolCall() || strings.HasPrefix(reason, "tool_") || strings.HasPrefix(reason, "write_")
				if isToolViolation {
					errPayload, _ := json.Marshal(map[string]any{
						"error": map[string]any{
							"message": fmt.Sprintf("Nacho Flow cycle breaker: model (%s) got stuck in a %s during tool call execution.", targetTier.Model, formatCycleKillReason(reason)),
							"type":    "cycle_killer_error",
							"code":    "tool_cycle_detected",
						},
					})
					noticeText := fmt.Sprintf("\n\n> 🌮 **Nacho Flow • Tool Loop Intercepted**\n> The model (`%s`) got stuck in a %s during tool call execution. Stream was severed to protect your token budget.\n> \n> 💡 **Next Steps:**\n> • Reply `continue` or click **Retry** — Nacho Flow will automatically escalate to a higher tier for this turn.\n> • Or override directly with HotSauce: `@nacho:cloud` or `@nacho:frontier`.\n", targetTier.Model, formatCycleKillReason(reason))
					escapedNotice, _ := json.Marshal(noticeText)
					noticeChunk := fmt.Sprintf("data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":%s}}]}\n\n", string(escapedNotice))
					_, _ = w.Write(fmt.Appendf(nil, "data: %s\n\n%sdata: [DONE]\n\n", errPayload, noticeChunk))
				} else {
					noticeText := fmt.Sprintf("\n\n> 🌮 **Nacho Flow • Loop Detected**\n> The model (`%s`) got stuck in a %s. Generation was stopped to protect your token budget.\n> \n> 💡 **Next Steps:**\n> • Reply `continue` or click **Retry** — Nacho Flow will automatically escalate to a higher tier for this turn.\n> • Or override directly with HotSauce: `@nacho:cloud` or `@nacho:frontier`.\n", targetTier.Model, formatCycleKillReason(reason))
					escapedNotice, _ := json.Marshal(noticeText)
					noticeChunk := fmt.Sprintf("data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":%s}}]}\n\n", string(escapedNotice))
					finishChunk := "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
					_, _ = w.Write([]byte(noticeChunk))
					_, _ = w.Write([]byte(finishChunk))
				}
				if flusher, ok := w.(http.Flusher); ok {
					flusher.Flush()
				}
				usage, _ := normalizer.GetUsage()
				s.recordTelemetry(targetTier, targetProvider, reqCtx, usage, http.StatusOK, startTime, isFallback, reqLogger, ntsTokensSaved, ntsBytesSaved)
				return
			}
		}

		s.recordProviderSuccess(targetProvider)
		s.recordProxySuccess()

		// Copy headers to client
		for k, vv := range resp.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.Header().Set(contract.HeaderNachoRouterTier, targetTier.Name)
		w.Header().Set(contract.HeaderSpiceRouterTier, targetTier.Name)
		w.Header().Set(contract.HeaderNachoTargetModel, targetTier.Model)
		w.Header().Set(contract.HeaderSpiceTargetModel, targetTier.Model)
		w.WriteHeader(resp.StatusCode)

		if len(sBuf.peekBuf) > 0 {
			_, _ = w.Write(sBuf.peekBuf)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
		if readErr == nil {
			for {
				n, err := normalizer.Read(sBuf.readBuf)
				// Phase 2: Active Mid-Stream Circuit Severing (Check BEFORE writing to swallow degenerate chunks)
				if violated, reason := normalizer.CheckCycleViolation(); violated && cb != nil && cb.IsEnabled() {
					reqLogger.Warn("Cycle killer (qu'est-ce que c'est?): Severing runaway stream",
						slog.String("reason", reason),
						slog.String("tier", targetTier.Name),
					)
					reqCtx.CycleBreakerTriggered = true
					reqCtx.CycleBreakerReason = reason
					cooldown, floor := s.resolveCycleKillParams()
					if s.sessionTracker != nil {
						s.sessionTracker.RecordCycleKill(extractSessionKey(r), targetTier.Model, cooldown, floor)
					}
					reqCtx.CycleContentTokens = cb.ContentTokens()
					reqCtx.CycleMaxNgramFreq = cb.MaxNgramFreq()
					reqCtx.CycleThinkingTokens = cb.ThinkingTokens()
					reqCtx.CycleMaxThinkingNgramFreq = cb.MaxThinkingNgramFreq()
					reqCtx.CycleToolTokens = cb.ToolTokens()
					reqCtx.CycleMaxToolNgramFreq = cb.MaxToolNgramFreq()
					isToolViolation := normalizer.HasActiveToolCall() || strings.HasPrefix(reason, "tool_") || strings.HasPrefix(reason, "write_")
					_ = normalizer.Close()

					if isToolViolation {
						errPayload, _ := json.Marshal(map[string]any{
							"error": map[string]any{
								"message": fmt.Sprintf("Nacho Flow cycle breaker: model (%s) got stuck in a %s during tool call execution.", targetTier.Model, formatCycleKillReason(reason)),
								"type":    "cycle_killer_error",
								"code":    "tool_cycle_detected",
							},
						})
						noticeText := fmt.Sprintf("\n\n> 🌮 **Nacho Flow • Tool Loop Intercepted**\n> The model (`%s`) got stuck in a %s during tool call execution. Stream was severed to protect your token budget.\n> \n> 💡 **Next Steps:**\n> • Reply `continue` or click **Retry** — Nacho Flow will automatically escalate to a higher tier for this turn.\n> • Or override directly with HotSauce: `@nacho:cloud` or `@nacho:frontier`.\n", targetTier.Model, formatCycleKillReason(reason))
						escapedNotice, _ := json.Marshal(noticeText)
						noticeChunk := fmt.Sprintf("data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":%s}}]}\n\n", string(escapedNotice))
						_, _ = w.Write(fmt.Appendf(nil, "data: %s\n\n%sdata: [DONE]\n\n", errPayload, noticeChunk))
					} else {
						noticeText := fmt.Sprintf("\n\n> 🌮 **Nacho Flow • Loop Detected**\n> The model (`%s`) got stuck in a %s. Generation was stopped to protect your token budget.\n> \n> 💡 **Next Steps:**\n> • Reply `continue` or click **Retry** — Nacho Flow will automatically escalate to a higher tier for this turn.\n> • Or override directly with HotSauce: `@nacho:cloud` or `@nacho:frontier`.\n", targetTier.Model, formatCycleKillReason(reason))
						escapedNotice, _ := json.Marshal(noticeText)
						noticeChunk := fmt.Sprintf("data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":%s}}]}\n\n", string(escapedNotice))
						finishChunk := "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
						_, _ = w.Write([]byte(noticeChunk))
						_, _ = w.Write([]byte(finishChunk))
					}
					if flusher, ok := w.(http.Flusher); ok {
						flusher.Flush()
					}
					break
				}
				if n > 0 {
					_, _ = w.Write(sBuf.readBuf[:n])
					if flusher, ok := w.(http.Flusher); ok {
						flusher.Flush()
					}
				}
				if err != nil {
					break
				}
			}
		}
		usage, hasUsage := normalizer.GetUsage()
		if usage.PromptTokens > 0 {
			if calibrator, ok := s.classifier.(interface{ GetEstimator() *router.TokenEstimator }); ok {
				calibrator.GetEstimator().Calibrate(usage.PromptTokens, len(body))
			}
		}
		if resp.StatusCode == http.StatusOK && !hasUsage {
			if _, loaded := s.warnedZeroUsageProviders.LoadOrStore(targetTier.Provider, true); !loaded {
				reqLogger.Warn("Streaming response completed with zero usage tokens reported by provider",
					slog.String("provider", targetTier.Provider),
					slog.String("model", targetTier.Model),
				)
			}
		}
		if cb != nil && !reqCtx.CycleBreakerTriggered {
			reqCtx.CycleContentTokens = cb.ContentTokens()
			reqCtx.CycleMaxNgramFreq = cb.MaxNgramFreq()
			reqCtx.CycleThinkingTokens = cb.ThinkingTokens()
			reqCtx.CycleMaxThinkingNgramFreq = cb.MaxThinkingNgramFreq()
			reqCtx.CycleToolTokens = cb.ToolTokens()
			reqCtx.CycleMaxToolNgramFreq = cb.MaxToolNgramFreq()
		}
		_ = normalizer.Close()

		s.recordTelemetry(targetTier, targetProvider, reqCtx, usage, resp.StatusCode, startTime, isFallback, reqLogger, ntsTokensSaved, ntsBytesSaved)
		return
	}

	// Non-streaming response
	bodyBytes, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		s.recordProxyError()
		http.Error(w, "Failed to read upstream response", http.StatusBadGateway)
		return
	}

	// Quality Validation: Empty content from local provider -> Transparent Fallback
	if targetProvider.IsLocal() && !isFallback && isDefectiveEmptyContent(bodyBytes) {
		reqLogger.Warn("Local provider returned defective empty content, failing over to cloud fallback tier",
			slog.String("tier", targetTier.Name),
		)
		if defaultTier.Provider != "" && targetTier.Name != defaultTier.Name {
			s.dispatchTier(w, r, reqCtx, defaultTier, body, startTime, reqLogger, true)
			return
		}
	}

	s.recordProviderSuccess(targetProvider)
	s.recordProxySuccess()

	var nonStreamUsage StreamUsage

	// Normalize non-streaming tools & reasoning
	if resp.StatusCode == http.StatusOK && strings.Contains(resp.Header.Get(contract.HeaderContentType), contract.ContentTypeJSON) {
		if activeFeatures != router.FeatureRawPassThrough {
			var completionResp fastChatCompletionResponse
			if json.Unmarshal(bodyBytes, &completionResp) == nil {
				if len(completionResp.Usage) > 0 {
					_ = json.Unmarshal(completionResp.Usage, &nonStreamUsage)
				}
				if len(completionResp.Choices) > 0 {
					firstChoice := &completionResp.Choices[0]
					modified := false

					if activeFeatures.Has(router.FeatureThinkNormalizer) {
						reasoningText := firstChoice.Message.ReasoningContent
						if reasoningText == "" {
							reasoningText = firstChoice.Message.Reasoning
						}
						if reasoningText == "" {
							reasoningText = firstChoice.Message.Reason
						}
						if reasoningText != "" && !strings.Contains(firstChoice.Message.Content, "<think>") {
							firstChoice.Message.Content = "<think>\n" + reasoningText + "\n</think>\n\n" + firstChoice.Message.Content
							firstChoice.Message.ReasoningContent = ""
							firstChoice.Message.Reasoning = ""
							firstChoice.Message.Reason = ""
							modified = true
						}
					}

					if activeFeatures.Has(router.FeatureToolNormalizer) && reqCtx.HasTools && len(firstChoice.Message.ToolCalls) == 0 && firstChoice.Message.Content != "" {
						cleanedText, extractedCalls, parsed := router.NormalizeMarkdownToolCalls(firstChoice.Message.Content)
						if parsed && len(extractedCalls) > 0 {
							firstChoice.Message.Content = cleanedText
							firstChoice.FinishReason = "tool_calls"
							rawCallsJSON, _ := json.Marshal(extractedCalls)
							firstChoice.Message.ToolCalls = rawCallsJSON
							modified = true
						} else if activeFeatures.Has(router.FeatureShieldEnabled) && reqCtx.InteractiveTool != "" && s.shieldMgr != nil {
							if synthCall, ok := s.shieldMgr.EvaluateAndSynthesize(firstChoice.Message.Content, reqCtx.InteractiveTool); ok && synthCall != nil {
								firstChoice.FinishReason = "tool_calls"
								rawCallsJSON, _ := json.Marshal([]interface{}{synthCall})
								firstChoice.Message.ToolCalls = rawCallsJSON
								modified = true
							}
						}
					}

					if len(firstChoice.Message.ToolCalls) == 0 && firstChoice.Message.Content != "" && reqCtx.HasTools && !reqCtx.NoCycleKiller && !isFallback {
						if cb := resolveCycleBreaker(targetTier, s.GetConfig()); cb != nil && cb.IsEnabled() {
							if triggered, reason := cb.ProcessDelta(firstChoice.Message.Content, false); triggered {
								reqCtx.CycleBreakerTriggered = true
								reqCtx.CycleBreakerReason = reason
								reqLogger.Warn("Cycle killer (qu'est-ce que c'est?): Runaway monologue intercepted",
									slog.String("reason", reason),
									slog.String("tier", targetTier.Name),
									slog.Int("cycle_retries", reqCtx.CycleRetries),
								)
							}
							reqCtx.CycleContentTokens = cb.ContentTokens()
							reqCtx.CycleMaxNgramFreq = cb.MaxNgramFreq()
							reqCtx.CycleThinkingTokens = cb.ThinkingTokens()
							reqCtx.CycleMaxThinkingNgramFreq = cb.MaxThinkingNgramFreq()
							reqCtx.CycleToolTokens = cb.ToolTokens()
							reqCtx.CycleMaxToolNgramFreq = cb.MaxToolNgramFreq()
							cbMaxRetries := cb.MaxRetries()
							cbCorrectionPrompt := cb.CorrectionPrompt()
							shield.PutCycleBreaker(cb)

							if reqCtx.CycleBreakerTriggered {
								if reqCtx.CycleRetries < cbMaxRetries {
									reqCtx.CycleRetries++
									injectedBody := injectCorrectionPrompt(body, cbCorrectionPrompt)
									s.dispatchTier(w, r, reqCtx, targetTier, injectedBody, startTime, reqLogger, false)
									return
								}
								if s.sessionTracker != nil {
									cooldown, floor := s.resolveCycleKillParams()
									s.sessionTracker.RecordCycleKill(extractSessionKey(r), targetTier.Model, cooldown, floor)
								}
								if defaultTier.Provider != "" && targetTier.Name != defaultTier.Name {
									reqLogger.Warn("Local cycle retries exhausted, escalating to cloud fallback tier",
										slog.String("fallback_tier", defaultTier.Name),
										slog.String("fallback_model", defaultTier.Model),
									)
									s.dispatchTier(w, r, reqCtx, defaultTier, body, startTime, reqLogger, true)
									return
								}
							}
						}
					}

					if modified {
						if newJSON, err := marshalNoEscapeHTML(completionResp); err == nil {
							bodyBytes = newJSON
						}
					}

					// Calibrate Adaptive Token Estimator if usage prompt tokens reported
					if nonStreamUsage.PromptTokens > 0 {
						if calibrator, ok := s.classifier.(interface{ GetEstimator() *router.TokenEstimator }); ok {
							calibrator.GetEstimator().Calibrate(nonStreamUsage.PromptTokens, len(body))
						}
					}
				}
			}
		}
	}

	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.Header().Set(contract.HeaderNachoRouterTier, targetTier.Name)
	w.Header().Set(contract.HeaderSpiceRouterTier, targetTier.Name)
	w.Header().Set(contract.HeaderNachoTargetModel, targetTier.Model)
	w.Header().Set(contract.HeaderSpiceTargetModel, targetTier.Model)
	w.Header().Set(contract.HeaderContentLength, strconv.Itoa(len(bodyBytes)))
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(bodyBytes)

	s.recordTelemetry(targetTier, targetProvider, reqCtx, nonStreamUsage, resp.StatusCode, startTime, isFallback, reqLogger, ntsTokensSaved, ntsBytesSaved)
}
