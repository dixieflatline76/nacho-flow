// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/provider"
	"github.com/dixieflatline76/nacho-flow/pkg/router"
	"github.com/dixieflatline76/nacho-flow/pkg/router/shield"
)

// isDefectiveEmptyContent performs a zero-map-allocation inspection of the response body to detect
// hollow local model generations (e.g. empty choices array, whitespace content, or zero tool calls)
// without allocating dynamic map[string]interface{} trees on the heap.
func isDefectiveEmptyContent(bodyBytes []byte) bool {
	if !json.Valid(bodyBytes) {
		return false
	}
	var top struct {
		Choices []json.RawMessage `json:"choices"`
	}
	if err := json.Unmarshal(bodyBytes, &top); err != nil {
		return true
	}
	if len(top.Choices) == 0 {
		return true
	}
	firstChoice := bytes.TrimSpace(top.Choices[0])
	if len(firstChoice) == 0 || firstChoice[0] != '{' {
		return true
	}
	var choice struct {
		Message json.RawMessage `json:"message"`
	}
	if err := json.Unmarshal(firstChoice, &choice); err != nil {
		return true
	}
	firstMsg := bytes.TrimSpace(choice.Message)
	if len(firstMsg) == 0 || firstMsg[0] != '{' {
		return true
	}
	var msg struct {
		Content          string            `json:"content"`
		ReasoningContent string            `json:"reasoning_content"`
		Reasoning        string            `json:"reasoning"`
		ToolCalls        []json.RawMessage `json:"tool_calls"`
	}
	if err := json.Unmarshal(firstMsg, &msg); err != nil {
		return true
	}
	reasoning := msg.ReasoningContent
	if reasoning == "" {
		reasoning = msg.Reasoning
	}
	hasTools := len(msg.ToolCalls) > 0
	return strings.TrimSpace(msg.Content) == "" && !hasTools && strings.TrimSpace(reasoning) == ""
}

// handleNonStreaming executes unary response reading, protocol translation, quality verification,
// thinking-tag normalization, and cycle breaker evaluation.
func (s *Server) handleNonStreaming(
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

	bodyBytes, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		s.recordProxyError()
		http.Error(w, "Failed to read upstream response", http.StatusBadGateway)
		return
	}

	var transErr error
	bodyBytes, transErr = s.translateResponseBody(resp.StatusCode, bodyBytes, targetProvider)
	if transErr != nil {
		reqLogger.Error("Failed to translate upstream protocol response", slog.Any("error", transErr))
		http.Error(w, "Failed to parse upstream response", http.StatusBadGateway)
		return
	}
	if _, ok := targetProvider.(provider.ProtocolProvider); ok {
		w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
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

	copyStreamHeaders(w.Header(), resp.Header)
	w.Header().Set(contract.HeaderNachoRouterTier, targetTier.Name)
	w.Header().Set(contract.HeaderSpiceRouterTier, targetTier.Name)
	w.Header().Set(contract.HeaderNachoTargetModel, targetTier.Model)
	w.Header().Set(contract.HeaderSpiceTargetModel, targetTier.Model)
	w.Header().Set(contract.HeaderContentLength, strconv.Itoa(len(bodyBytes)))
	w.WriteHeader(resp.StatusCode)
	// #nosec G705 - proxying upstream JSON response payload directly to client
	_, _ = w.Write(bodyBytes)

	s.recordTelemetry(targetTier, targetProvider, reqCtx, nonStreamUsage, resp.StatusCode, startTime, isFallback, reqLogger, ntsTokensSaved, ntsBytesSaved)
}
