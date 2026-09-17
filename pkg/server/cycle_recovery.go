package server

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/router"
	"github.com/dixieflatline76/nacho-flow/pkg/router/shield"
)

func hasCandidateToolTokens(b []byte) bool {
	return bytes.Contains(b, []byte("<tool_call>")) ||
		bytes.Contains(b, []byte("[TOOL_CALLS]")) ||
		bytes.Contains(b, []byte("<function=")) ||
		bytes.Contains(b, []byte("<|python_tag|>")) ||
		bytes.Contains(b, []byte("<invoke")) ||
		bytes.Contains(b, []byte("Action:")) ||
		bytes.Contains(b, []byte("```"))
}

func resolveCycleBreaker(tier contract.Tier, cfg *contract.Config) *shield.CycleBreaker {
	var cbCfg contract.CycleBreakerConfig
	if cfg != nil {
		cbCfg = cfg.CycleKiller
		if cbCfg.Enabled == nil && cfg.CycleBreaker.Enabled != nil {
			cbCfg = cfg.CycleBreaker
		}
	}

	// Support both cycle_killer and cycle_breaker on tier level
	tierCb := tier.CycleKiller
	if tierCb == nil {
		tierCb = tier.CycleBreaker
	}

	if tierCb != nil {
		if tierCb.Enabled != nil {
			cbCfg.Enabled = tierCb.Enabled
		}
		if tierCb.MaxContentTokens > 0 {
			cbCfg.MaxContentTokens = tierCb.MaxContentTokens
		}
		if tierCb.ContentLane.MaxTokens > 0 {
			cbCfg.ContentLane = tierCb.ContentLane
		}
		if tierCb.RepetitionWindow > 0 {
			cbCfg.RepetitionWindow = tierCb.RepetitionWindow
		}
		if tierCb.RepetitionThreshold > 0 {
			cbCfg.RepetitionThreshold = tierCb.RepetitionThreshold
		}
		if tierCb.MaxRetries > 0 {
			cbCfg.MaxRetries = tierCb.MaxRetries
		}
		if tierCb.CorrectionPrompt != "" {
			cbCfg.CorrectionPrompt = tierCb.CorrectionPrompt
		}
	}
	if cbCfg.Enabled != nil && !*cbCfg.Enabled {
		return nil
	}
	return shield.NewCycleBreaker(&cbCfg)
}

func (s *Server) resolveCycleKillParams() (time.Duration, int) {
	cfg := s.GetConfig()
	cooldownSec := cfg.CycleKiller.ModelCooldownSeconds
	if cooldownSec == 0 && cfg.CycleBreaker.ModelCooldownSeconds > 0 {
		cooldownSec = cfg.CycleBreaker.ModelCooldownSeconds
	}
	cooldown := router.DefaultModelCooldown
	if cooldownSec > 0 {
		cooldown = time.Duration(cooldownSec) * time.Second
	}

	floor := cfg.CycleKiller.RetryFloor
	if floor == 0 && cfg.CycleBreaker.RetryFloor > 0 {
		floor = cfg.CycleBreaker.RetryFloor
	}
	if floor <= 0 {
		floor = 3
	}
	return cooldown, floor
}

func formatCycleKillReason(reason string) string {
	switch reason {
	case "ngram_repetition_loop_detected", "content_repetition_loop_detected":
		return "repetitive content loop detected"
	case "thinking_repetition_loop_detected":
		return "repetitive reasoning loop detected"
	case "content_budget_exceeded_with_repetition", "prose_budget_exceeded_with_repetition":
		return "content token budget exceeded with repetition"
	case "thinking_budget_exceeded_with_repetition":
		return "thinking token budget exceeded with repetition"
	default:
		return strings.ReplaceAll(reason, "_", " ")
	}
}

func injectCorrectionPrompt(body []byte, prompt string) []byte {
	if prompt == "" {
		prompt = contract.CycleBreakerDefaultCorrectionPrompt
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return body
	}

	messages, ok := payload["messages"].([]interface{})
	if !ok {
		return body
	}

	overrideMsg := map[string]interface{}{
		"role":    "user",
		"content": prompt,
	}
	payload["messages"] = append(messages, overrideMsg)

	reencoded, err := json.Marshal(payload)
	if err != nil {
		return body
	}
	return reencoded
}
