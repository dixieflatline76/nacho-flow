package server

import (
	"log/slog"
	"strings"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/provider"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
)

func (s *Server) recordTelemetry(
	targetTier contract.Tier,
	targetProvider provider.LLMProvider,
	reqCtx contract.RequestContext,
	usage StreamUsage,
	statusCode int,
	startTime time.Time,
	isFallback bool,
	reqLogger *slog.Logger,
	ntsTokensSaved int,
	ntsBytesSaved int,
) {
	latency := float64(time.Since(startTime).Milliseconds())
	isLocal := targetProvider.IsLocal()

	promptTokens := reqCtx.Tokens
	if usage.PromptTokens > 0 {
		promptTokens = usage.PromptTokens
	}
	completionTokens := usage.CompletionTokens
	totalTokens := usage.TotalTokens
	if totalTokens == 0 || usage.PromptTokens == 0 {
		totalTokens = promptTokens + completionTokens
	}

	// Baseline rates are resolved internally by PricingOracle.resolveBenchmarkRates().
	const baselineRatePerM = 0.0

	cachedTokens := 0
	if usage.PromptTokensDetails != nil {
		cachedTokens = usage.PromptTokensDetails.CachedTokens
	}
	upstreamCost := usage.Cost

	costSpent, costSaved := s.oracle.CalculateFinancials(targetTier.Provider, targetTier.Model, isLocal, promptTokens, completionTokens, cachedTokens, upstreamCost, baselineRatePerM)

	tierNum := 2
	if isLocal {
		tierNum = 1
	} else {
		switch {
		case strings.Contains(strings.ToLower(targetTier.Name), "tier 3") || strings.Contains(strings.ToLower(targetTier.Name), "reasoning") || strings.Contains(strings.ToLower(targetTier.Name), "rescue") || strings.Contains(strings.ToLower(targetTier.Name), "sonnet"):
			tierNum = 3
		case strings.Contains(strings.ToLower(targetTier.Name), "tier 4") || strings.Contains(strings.ToLower(targetTier.Name), "vision") || strings.Contains(strings.ToLower(targetTier.Model), "vision"):
			tierNum = 4
		default:
			tierNum = 2
		}
	}

	s.tracker.Record(telemetry.Observation{
		Tier:                      tierNum,
		TierName:                  targetTier.Name,
		Model:                     targetTier.Model,
		Provider:                  targetTier.Provider,
		Tokens:                    totalTokens,
		CostSpent:                 costSpent,
		CostSaved:                 costSaved,
		IsLocal:                   isLocal,
		IsFallback:                isFallback,
		LatencyMs:                 latency,
		Keywords:                  reqCtx.Keywords,
		HasImages:                 reqCtx.HasImages,
		HasTools:                  reqCtx.HasTools,
		StatusCode:                statusCode,
		IsRetry:                   reqCtx.IsRetry,
		ForcedTier:                reqCtx.ForcedTier,
		ForcedModel:               reqCtx.ForcedModel,
		DirectiveUsed:             reqCtx.MetaDirectiveRaw,
		CycleBreakerTriggered:     reqCtx.CycleBreakerTriggered,
		CycleBreakerReason:        reqCtx.CycleBreakerReason,
		CycleContentTokens:        reqCtx.CycleContentTokens,
		CycleMaxNgramFreq:         reqCtx.CycleMaxNgramFreq,
		CycleThinkingTokens:       reqCtx.CycleThinkingTokens,
		CycleMaxThinkingNgramFreq: reqCtx.CycleMaxThinkingNgramFreq,
		CycleToolTokens:           reqCtx.CycleToolTokens,
		CycleMaxToolNgramFreq:     reqCtx.CycleMaxToolNgramFreq,
		HasShellWrite:             reqCtx.HasShellWrite,
		SessionKickstarted:        reqCtx.SessionKickstarted,
		CachedTokens:              cachedTokens,
		UpstreamCost:              upstreamCost,
		FairyDusted:               reqCtx.FairyDusted,
		FairyDustEntry:            reqCtx.FairyDustEntry,
		NTSTokensSaved:            ntsTokensSaved,
		NTSBytesSaved:             ntsBytesSaved,
		SessionKey:                reqCtx.SessionKey,
		RootPromptHash:            reqCtx.RootPromptHash,
		Retries:                   reqCtx.Retries,
		HasWriteCapability:        reqCtx.HasWriteCapability,
		HasWriteProgress:          reqCtx.HasWriteProgress,
		HasTestPass:               reqCtx.HasTestPass,
		HasTestFail:               reqCtx.HasTestFail,
	})

	reqLogger.Info("Completed proxy request",
		slog.String("session_key", reqCtx.SessionKey),
		slog.String("tier", targetTier.Name),
		slog.String("model", targetTier.Model),
		slog.Int("tokens", totalTokens),
		slog.Float64("latency_ms", latency),
		slog.Int("status", statusCode),
		slog.Bool("is_fallback", isFallback),
		slog.Bool("is_retry", reqCtx.IsRetry),
		slog.Bool("cycle_breaker_triggered", reqCtx.CycleBreakerTriggered),
		slog.Int("cycle_content_tokens", reqCtx.CycleContentTokens),
		slog.Int("cycle_max_ngram_freq", reqCtx.CycleMaxNgramFreq),
		slog.Int("cycle_thinking_tokens", reqCtx.CycleThinkingTokens),
		slog.Int("cycle_max_thinking_ngram_freq", reqCtx.CycleMaxThinkingNgramFreq),
		slog.Int("cycle_tool_tokens", reqCtx.CycleToolTokens),
		slog.Int("cycle_max_tool_ngram_freq", reqCtx.CycleMaxToolNgramFreq),
		slog.Bool("session_kickstarted", reqCtx.SessionKickstarted),
		slog.Int("session_kickstart_count", reqCtx.SessionKickstartCount),
		slog.Bool("fairy_dusted", reqCtx.FairyDusted),
		slog.String("fairy_dust_entry", reqCtx.FairyDustEntry),
		slog.Int("fairy_dust_count", reqCtx.FairyDustCount),
	)
}
