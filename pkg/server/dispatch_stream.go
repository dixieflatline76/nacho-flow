// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/provider"
	"github.com/dixieflatline76/nacho-flow/pkg/router"
	"github.com/dixieflatline76/nacho-flow/pkg/router/shield"
)

// handleStreaming processes an active SSE stream through the normalization, quality verification,
// and cycle-breaking pipeline with zero heap allocation on the hot token path.
func (s *Server) handleStreaming(
	w http.ResponseWriter,
	r *http.Request,
	reqCtx contract.RequestContext,
	targetTier contract.Tier,
	targetProvider provider.LLMProvider,
	resp *http.Response,
	body []byte,
	startTime time.Time,
	reqLogger *slog.Logger,
	isFallback bool,
	activeFeatures router.FeatureFlag,
	ntsTokensSaved int,
	ntsBytesSaved int,
) {
	defaultTier := s.GetConfig().DefaultTier
	bodyReader := s.wrapResponseStream(resp, targetProvider)
	normalizer := NewStreamNormalizer(bodyReader)
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

	// Quality defect check: empty stream on local provider -> failover to cloud fallback
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

	// Cycle Breaker peek check
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
				reqCtx.CycleRetries++
				injectedBody := injectCorrectionPrompt(body, cb.CorrectionPrompt())
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

			// Sever stream immediately with loop notice
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

	copyStreamHeaders(w.Header(), resp.Header)
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
}
