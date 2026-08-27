// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package config

// DefaultStarterConfigTemplate is the canonical starter configuration
// auto-initialized when nacho-flow boots on a clean environment without an existing config.yaml.
const DefaultStarterConfigTemplate = `# =============================================================================
# 🌮 NACHO FLOW CONFIGURATION
# Intelligent Semantic AI Gateway & Multi-Tier Cost Optimizer
# =============================================================================

port: 8000

# =============================================================================
# 🔌 LLM PROVIDERS
# =============================================================================
providers:
  # ---------------------------------------------------------------------------
  # 1. Local GPU Provider (Ollama / vLLM / SGLang)
  # - Cost: $0.00 / 1M Tokens (100% Free Local Compute)
  # ---------------------------------------------------------------------------
  ollama:
    base_url: "http://127.0.0.1:11434"
    type: "local"

  # ---------------------------------------------------------------------------
  # 2. OpenRouter Cloud Gateway
  # - Role: Global routing to 300+ frontier and open-weight cloud models.
  # - Secret: Resolves ENV_OPENROUTER_API_KEY from environment.
  # ---------------------------------------------------------------------------
  openrouter:
    base_url: "https://openrouter.ai/api/v1"
    api_key: "ENV_OPENROUTER_API_KEY"
    headers:
      HTTP-Referer: "https://github.com/dixieflatline76/nacho-flow"
      X-Title: "nacho-flow"

# =============================================================================
# 🔥 HEATSEEKER LIVE MODEL DEALS
# =============================================================================
deals:
  enabled: true
  alert_threshold_pct: 30.0
  min_coding_index: 40.0
  require_tools: true

# =============================================================================
# 🛡️ AGENTIC TOOL FALLBACK SHIELD
# =============================================================================
agent_shield:
  enabled: true
  tail_buffer_bytes: 256
  question_heuristics:
    - "are you satisfied"
    - "would you like"
    - "should i"
    - "do you approve"
    - "please confirm"
    - "let me know if"
    - "how would you like to proceed"
  mode_switch_heuristics:
    - "switch to code mode"
    - "switch to architect mode"
    - "ready to implement"

# =============================================================================
# 🚦 ORDERED DYNAMIC ROUTING TIERS (FIRST MATCH WINS)
# =============================================================================
tiers:
  # ---------------------------------------------------------------------------
  # TIER 1: Local GPU Free (Gemma 4 12B QAT) - $0.00 Cost
  # ---------------------------------------------------------------------------
  - name: "Tier 1: Local GPU Free (Gemma 4 12B QAT)"
    provider: "ollama"
    model: "gemma4:12b-it-qat"
    when: "Tokens < 16000 && Retries == 0"
    strip_images: false
    max_context: 64000

  # ---------------------------------------------------------------------------
  # TIER 2: Frontier-Grade Cloud Workhorse (Gemini 3.7 Flash Thinking)
  # ---------------------------------------------------------------------------
  - name: "Tier 2: Cloud Workhorse (Gemini 3.7 Flash)"
    provider: "openrouter"
    model: "google/gemini-3.7-flash"
    when: "Retries < 1"

  # ---------------------------------------------------------------------------
  # TIER 3: Deep Cloud Reasoner (DeepSeek R1)
  # ---------------------------------------------------------------------------
  - name: "Tier 3: Deep Reasoner (DeepSeek R1)"
    provider: "openrouter"
    model: "deepseek/deepseek-r1"
    when: "Retries == 1 || 'reason' in Keywords || 'algorithm' in Keywords || 'architect' in Keywords"

# =============================================================================
# 🛡️ DEFAULT TIER: Frontier Powerhouse (Claude 3.5 Sonnet)
# =============================================================================
default_tier:
  name: "Tier 4: Frontier Powerhouse (Claude 3.5 Sonnet)"
  provider: "openrouter"
  model: "anthropic/claude-3.5-sonnet"
  when: "true"
`
