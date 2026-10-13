// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/provider"
	"github.com/dixieflatline76/nacho-flow/pkg/router/shield"
)

// maxGlobalTurnCalls enforces a hard safety cap on total upstream calls per turn.
const maxGlobalTurnCalls = 4

var thoughtSlapNoticeChunkBytes = func() []byte {
	escaped, _ := json.Marshal("\n\n> " + contract.ThoughtSlapNoticeText + "\n\n")
	return fmt.Appendf(nil, "data: {\"choices\":[{\"index\":0,\"delta\":{\"reasoning_content\":%s}}]}\n\n", string(escaped))
}()

// thoughtSlapNoticeChunk returns the client-visible reasoning notice emitted before Thought Slap recovery calls.
func thoughtSlapNoticeChunk() []byte {
	return thoughtSlapNoticeChunkBytes
}

// findRescueTier looks up a named rescue tier from the active server configuration.
func findRescueTier(tierName string, cfg *contract.Config) (contract.Tier, bool) {
	if tierName == "" || cfg == nil {
		return contract.Tier{}, false
	}
	if cfg.DefaultTier.Name == tierName {
		return cfg.DefaultTier, true
	}
	for _, t := range cfg.Tiers {
		if t.Name == tierName {
			return t, true
		}
	}
	return contract.Tier{}, false
}

// accumulateUsage sums token and cost metrics from attempt into total.
func accumulateUsage(total *StreamUsage, attempt StreamUsage) {
	total.PromptTokens += attempt.PromptTokens
	total.CompletionTokens += attempt.CompletionTokens
	total.TotalTokens += attempt.TotalTokens
	total.Cost += attempt.Cost
	if attempt.PromptTokensDetails != nil {
		if total.PromptTokensDetails == nil {
			total.PromptTokensDetails = &PromptTokensDetails{}
		}
		total.PromptTokensDetails.CachedTokens += attempt.PromptTokensDetails.CachedTokens
	}
}

// buildExhaustedTerminal constructs the final terminal SSE byte sequence when thought slap is exhausted or errors.
func buildExhaustedTerminal(totalUsage StreamUsage) []byte {
	var b strings.Builder
	b.WriteString(honestTruncationNoticeChunk)
	b.WriteString("data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"length\"}]}\n\n")

	prompt := totalUsage.PromptTokens
	comp := totalUsage.CompletionTokens
	tot := totalUsage.TotalTokens
	if tot == 0 && (prompt > 0 || comp > 0) {
		tot = prompt + comp
	}

	usageJSON, err := json.Marshal(map[string]any{
		"prompt_tokens":     prompt,
		"completion_tokens": comp,
		"total_tokens":      tot,
	})
	if err == nil {
		b.WriteString("data: {\"choices\":[],\"usage\":")
		b.Write(usageJSON)
		b.WriteString("}\n\n")
	}

	b.WriteString("data: [DONE]\n\n")
	return []byte(b.String())
}

// executeThoughtSlapUpstream executes a recovery request against the specified rescue or target tier.
// After HTTP/SSE headers are sent, no errors may write HTTP headers. All failures must be returned as errors.
func (s *Server) executeThoughtSlapUpstream(
	ctx context.Context,
	r *http.Request,
	recTier contract.Tier,
	recBody []byte,
	reqLogger *slog.Logger,
	w http.ResponseWriter,
) (*http.Response, provider.LLMProvider, error) {
	reg := s.GetRegistry()
	if reg == nil {
		return nil, nil, fmt.Errorf("provider registry not available")
	}
	prov, exists := reg.Get(recTier.Provider)
	if !exists {
		return nil, nil, fmt.Errorf("provider not found: %s", recTier.Provider)
	}
	if !s.allowProvider(prov) {
		return nil, nil, fmt.Errorf("provider circuit breaker open: %s", recTier.Provider)
	}

	preparedBody := s.preparePayload(&recTier, prov, recBody)
	outReq, err := s.buildOutboundRequest(ctx, r, prov, recTier.Model, preparedBody)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build outbound request: %w", err)
	}
	dumpOutbound(outReq, reqLogger)

	// Keepalive comments during recovery TTFT (Decision D6): emit ': keepalive\n\n' every 10s while waiting
	var stopKeepalive chan struct{}
	var keepaliveDone chan struct{}
	if w != nil {
		stopKeepalive = make(chan struct{})
		keepaliveDone = make(chan struct{})
		go func() {
			defer close(keepaliveDone)
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-stopKeepalive:
					return
				case <-ticker.C:
					_, _ = w.Write([]byte(": keepalive\n\n"))
					if flusher, ok := w.(http.Flusher); ok {
						flusher.Flush()
					}
				}
			}
		}()
	}

	resp, err := s.transport.RoundTrip(outReq)

	if stopKeepalive != nil {
		close(stopKeepalive)
		<-keepaliveDone
	}

	if err != nil {
		s.recordProviderFailure(prov)
		s.recordProxyError()
		return nil, nil, fmt.Errorf("upstream round trip failed: %w", err)
	}
	if resp.StatusCode >= 500 {
		s.recordProviderFailure(prov)
		s.recordProxyError()
		_ = resp.Body.Close()
		return nil, nil, fmt.Errorf("upstream returned 5xx status: %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, nil, fmt.Errorf("upstream returned non-200 status: %d", resp.StatusCode)
	}

	return resp, prov, nil
}

// executeThoughtSlapRecoveryStream coordinates the Thought Slap recovery loop for streaming turns.
func (s *Server) executeThoughtSlapRecoveryStream(
	w http.ResponseWriter,
	r *http.Request,
	reqCtx *contract.RequestContext,
	targetTier contract.Tier,
	initialNormalizer *StreamNormalizer,
	body []byte,
	tsCfg contract.ThoughtSlapConfig,
	sBuf *streamBufferBundle,
	reqLogger *slog.Logger,
) (StreamUsage, int, float64) {
	initialUsage, _ := initialNormalizer.GetUsage()
	totalUsage := initialUsage
	var extraTokens int
	var extraCost float64

	attempts := 0
	currentNormalizer := initialNormalizer
	outcome := contract.ThoughtSlapOutcomeExhausted

	sessionKey := extractSessionKey(r)
	if s.sessionTracker != nil {
		s.sessionTracker.RecordThoughtSlap(sessionKey)
		reqCtx.SessionThoughtSlapCount = s.sessionTracker.GetThoughtSlapCount(sessionKey)
	}

	recTier := targetTier
	if tsCfg.RescueTier != "" {
		if found, ok := findRescueTier(tsCfg.RescueTier, s.GetConfig()); ok {
			recTier = found
		} else {
			reqLogger.Warn("Thought slap rescue tier not found, falling back to current tier",
				slog.String("rescue_tier", tsCfg.RescueTier),
				slog.String("current_tier", targetTier.Name),
			)
		}
	}
	reqCtx.ThoughtSlapTier = recTier.Name

	prompt := tsCfg.PromptOverride
	if prompt == "" {
		prompt = contract.DefaultThoughtSlapPrompt
	}

	for currentNormalizer.TerminalHeld() && attempts < tsCfg.MaxAttempts {
		totalCalls := 1 + reqCtx.CycleRetries + attempts
		if totalCalls >= maxGlobalTurnCalls {
			reqLogger.Warn("Thought slap aborted: reached global turn call hard cap",
				slog.Int("total_calls", totalCalls),
				slog.Int("attempts", attempts),
			)
			outcome = contract.ThoughtSlapOutcomeExhausted
			break
		}
		if r.Context().Err() != nil {
			outcome = contract.ThoughtSlapOutcomeClientCancelled
			break
		}

		reqLogger.Warn("Kickstart thought slap: empty truncated turn, requesting action",
			slog.String("tier", recTier.Name),
			slog.String("model", recTier.Model),
			slog.Int("attempt", attempts+1),
		)

		// 1. Emit thought slap notice reasoning chunk
		_, _ = w.Write(thoughtSlapNoticeChunk())
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}

		// 2. Inject correction prompt into original request body
		recBody := injectCorrectionPrompt(body, prompt)

		// 3. Upstream recovery request
		resp, prov, err := s.executeThoughtSlapUpstream(r.Context(), r, recTier, recBody, reqLogger, w)
		if err != nil {
			reqLogger.Error("Thought slap upstream request failed", slog.Any("error", err))
			if r.Context().Err() != nil {
				outcome = contract.ThoughtSlapOutcomeClientCancelled
			} else {
				outcome = contract.ThoughtSlapOutcomeUpstreamError
			}
			break
		}

		// 4. Wrap upstream response stream in new normalizer
		bodyReader := s.wrapResponseStream(resp, prov)
		recNorm := NewStreamNormalizer(bodyReader)
		recNorm.SetFeatures(reqCtx.Features)
		var recCb *shield.CycleBreaker
		if reqCtx.HasTools && !reqCtx.NoCycleKiller {
			recCb = resolveCycleBreaker(recTier, s.GetConfig())
			if recCb != nil && recCb.IsEnabled() {
				recNorm.SetCycleBreaker(recCb)
			}
		}
		recNorm.SetDeferTerminal(true)
		recNorm.SetSuppressPrelude(true)

		// 5. Stream recovery normalizer directly to client
		for {
			n, readErr := recNorm.Read(sBuf.readBuf)
			if violated, reason := recNorm.CheckCycleViolation(); violated && recCb != nil && recCb.IsEnabled() {
				reqLogger.Warn("Cycle breaker tripped during thought slap recovery",
					slog.String("reason", reason),
					slog.String("tier", recTier.Name),
				)
				reqCtx.CycleBreakerTriggered = true
				reqCtx.CycleBreakerReason = reason
				break
			}
			if n > 0 {
				_, _ = w.Write(sBuf.readBuf[:n])
				if flusher, ok := w.(http.Flusher); ok {
					flusher.Flush()
				}
			}
			if readErr != nil {
				break
			}
		}

		// 6. Accumulate token usage
		attemptUsage, _ := recNorm.GetUsage()
		extraTokens += attemptUsage.TotalTokens
		extraCost += attemptUsage.Cost
		accumulateUsage(&totalUsage, attemptUsage)

		attempts++

		// Close previous normalizer if swapped
		if currentNormalizer != initialNormalizer {
			_ = currentNormalizer.Close()
		}

		if recNorm.HasEmittedToolCalls() {
			outcome = contract.ThoughtSlapOutcomeRecoveredToolCall
			currentNormalizer = recNorm
			break
		} else if recNorm.HasNonWhitespaceContent() {
			outcome = contract.ThoughtSlapOutcomeRecoveredContent
			currentNormalizer = recNorm
			break
		}

		currentNormalizer = recNorm
	}

	reqCtx.ThoughtSlapAttempts = attempts
	reqCtx.ThoughtSlapOutcome = outcome
	reqCtx.ThoughtSlapExtraTokens = extraTokens
	reqCtx.ThoughtSlapExtraCost = extraCost

	switch outcome {
	case contract.ThoughtSlapOutcomeRecoveredToolCall:
		reqLogger.Info("Thought slap recovery succeeded",
			slog.String("outcome", outcome),
			slog.Int("attempts", attempts),
			slog.String("tier", reqCtx.ThoughtSlapTier),
		)
		reqCtx.EmptyTruncatedTurn = false
		reqCtx.UpstreamFinishReason = "tool_calls"
	case contract.ThoughtSlapOutcomeRecoveredContent:
		reqLogger.Info("Thought slap recovery succeeded",
			slog.String("outcome", outcome),
			slog.Int("attempts", attempts),
			slog.String("tier", reqCtx.ThoughtSlapTier),
		)
		reqCtx.EmptyTruncatedTurn = false
		reqCtx.UpstreamFinishReason = "stop"
	default:
		reqLogger.Info("Thought slap recovery exhausted or failed",
			slog.String("outcome", outcome),
			slog.Int("attempts", attempts),
		)
		reqCtx.EmptyTruncatedTurn = true
		reqCtx.UpstreamFinishReason = "length"
		// Emit Phase 1 truthful terminal notice + finish_reason "length" + summed usage + [DONE]
		terminalBytes := buildExhaustedTerminal(totalUsage)
		_, _ = w.Write(terminalBytes)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}

	if currentNormalizer != nil && currentNormalizer != initialNormalizer {
		_ = currentNormalizer.Close()
	}

	return totalUsage, extraTokens, extraCost
}
