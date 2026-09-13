// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package config

// DefaultStarterConfigTemplate is the canonical starter configuration
// auto-initialized when nacho-flow boots on a clean environment without an existing config.yaml.
const DefaultStarterConfigTemplate = `# =============================================================================
# 🌮 NACHO FLOW CONFIGURATION
# Agent Supervisor & Model Dispatcher
# =============================================================================

port: 8000
host: "127.0.0.1" # Bind address (default: 127.0.0.1 for local isolation, 0.0.0.0 for LAN access)

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
    type: "cloud"
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
  # Canonical question heuristics, mode switch heuristics, and error signatures
  # are automatically loaded from data/agents/*.json via agentregistry.
  # Use these lists only to define custom user overrides/extensions.
  # question_heuristics: []
  # mode_switch_heuristics: []
  # error_signatures: []

# =============================================================================
# 🎸 CYCLE KILLER (Qu'est-ce que c'est?)
# In-Flight Stream Defense that murders infinite loops & monologue traps in <3s
# =============================================================================
cycle_killer:
  enabled: true                     # Master switch for all in-flight stream defense
  phrase_length: 6                  # Default sliding n-gram window size (words) across lanes
  budget_max_repeats: 5             # Default repeat threshold once lane token budget is exceeded
  model_cooldown_seconds: 120       # 🧊 Model Cooldown: skip cycle-killed model on this session for 2m
  retry_floor: 3                    # 📈 Auto-Escalation: jump session retries to 3 on severed streams
  max_retries: 1                    # Stage 1 local retries with [SYSTEM OVERRIDE] before cloud escalation

  thinking_lane:
    max_tokens: 4096                # Max reasoning tokens before budget repetition check
    max_repeats: 6                  # Fast-kill repetition loop threshold (Type 1)

  content_lane:
    max_tokens: 6144                # Max non-tool content before budget repetition check
    max_repeats: 8                  # Fast-kill repetition loop threshold (Type 1)

  tool_lane:
    max_tokens: 8192                # Max streaming tool call arguments before repetition enforcement
    max_write_tokens: 32768         # Max size for Category A file writes (zero-alloc fast path)
    phrase_length: 4                # Tighter n-gram window to catch repeating 4-word shell commands
    max_repeats: 8                  # Fast-kill repetition loop threshold (Type 1)

kickstart:
  enabled: true                     # ⚡ Master switch for cross-turn idle session resuscitation
  threshold: 5                      # Jolt with [SYSTEM OVERRIDE] after N consecutive idle turns (0 = off)
  max_count: 10                     # 🛑 Kickstart Cap: force-escalate to default tier after N kickstarts
  max_failures: 3                   # 🔌 Circuit Breaker: suppress injection after N consecutive model failures
  write_only: true                  # Only count file writes / commands as progress (ignores read-only tools)
  # custom_write_tools: []          # Optional extension (agentregistry automatically provides standard write tools)

# =============================================================================
# 🗜️ NACHO TOKEN SAVER (NTS)
# Wire-speed, zero-allocation in-place tool output compaction engine
# =============================================================================
nts:
  enabled: true                     # Master switch for tool output compaction
  strip_ansi: true                  # Pass 1: Strip ANSI & OSC escape sequences
  resolve_cr: true                  # Pass 2: Overwrite carriage returns from progress spinners
  deduplicate_lines: true           # Pass 3: Collapse repeated consecutive lines (>3 times)
  dedup_threshold: 3                # Consecutive duplicate threshold before collapse
  strip_boilerplate: true           # Pass 4: Strip IDE tool boilerplate notices
  normalize_whitespace: true        # Pass 5: Collapse multiple empty lines
  preserve_file_reads: true         # 🛡️ Dual-lane immunity for read_file / view_file
  preserve_file_writes: true        # 🛡️ Dual-lane immunity for write_to_file / apply_diff
  preserve_cache_control: true      # 🛡️ Dual-lane immunity for prompt cache breakpoints
  compact_stale_file_reads: true    # 📦 Evict superseded historical file reads
  stale_read_depth: 3              # 📚 Keep the 3 most recent reads of each file/range to prevent amnesia

# =============================================================================
# 🚦 ORDERED DYNAMIC ROUTING TIERS (FIRST MATCH WINS)
# =============================================================================
tiers:
  # ---------------------------------------------------------------------------
  # ⚡ KICKSTART ESCALATION: Break read-only idle loops with Gemini Flash
  # ---------------------------------------------------------------------------
  - name: "Kickstart Escalation (Gemini 3.8 Flash)"
    provider: "openrouter"
    model: "google/gemini-3.8-flash"
    when: "SessionKickstarted && Retries < 3"

  # ---------------------------------------------------------------------------
  # 👁️ VISION ESCAPEMENT: Multimodal screenshot turns route to Gemini Flash
  # ---------------------------------------------------------------------------
  - name: "Tier: Multimodal Vision (Gemini 3.8 Flash)"
    provider: "openrouter"
    model: "google/gemini-3.8-flash"
    when: "HasImages && Retries < 2"

  # ---------------------------------------------------------------------------
  # TIER 1: Local GPU Workhorse (100% Free VRAM Offload)
  # ---------------------------------------------------------------------------
  - name: "Tier 1: Local GPU Workhorse"
    provider: "ollama"
    model: "gemma4:12b-it-qat"
    when: "Tokens < 20000 && Retries < 2"
    strip_images: false
    max_context: 32000

  # ---------------------------------------------------------------------------
  # TIER 2: Flagship Agent Coder (Qwen3 Coder Plus — $0.65 / $3.25 per 1M)
  # ---------------------------------------------------------------------------
  - name: "Tier 2: Flagship Agent Coder (Qwen3 Coder Plus)"
    provider: "openrouter"
    model: "qwen/qwen3-coder-plus"
    when: "Tokens < 160000 && Retries < 2"

  # ---------------------------------------------------------------------------
  # TIER 3: Debug & Reasoning Workhorse (Gemini 3.8 Flash — $0.75 / $3.75 per 1M)
  # ---------------------------------------------------------------------------
  - name: "Tier 3: Debug & Reasoning Workhorse (Gemini 3.8 Flash)"
    provider: "openrouter"
    model: "google/gemini-3.8-flash"
    when: "Tokens < 260000 && Retries < 5"

  # ---------------------------------------------------------------------------
  # TIER 4: Large Context Synthesis (Gemini 3.1 Pro)
  # ---------------------------------------------------------------------------
  - name: "Tier 4: Large Context Synthesis (Gemini 3.1 Pro)"
    provider: "openrouter"
    model: "google/gemini-3.1-pro-preview"
    when: "Retries < 7"

  # ---------------------------------------------------------------------------
  # TIER 5: Frontier Powerhouse (Claude Sonnet 5 — $2.00/$10.00 per 1M)
  # ---------------------------------------------------------------------------
  - name: "Tier 5: Frontier Powerhouse (Claude Sonnet 5)"
    provider: "openrouter"
    model: "anthropic/claude-sonnet-5"
    when: "Retries < 9"

  # ---------------------------------------------------------------------------
  # TIER 6: Claude Opus 5 — SPICY DIRECTIVE ONLY (unreachable by routing)
  # Access via: X-Spicy-Model: anthropic/claude-opus-5 or Fairy Dust Checkpoints
  # ---------------------------------------------------------------------------
  - name: "Tier 6: Opus On-Demand (Spicy Only)"
    provider: "openrouter"
    model: "anthropic/claude-opus-5"
    when: "false"

# =============================================================================
# 🛡️ DEFAULT TIER: Cost-Safe Catch-All (Claude Sonnet 5)
# =============================================================================
default_tier:
  name: "Default: Cost-Safe Catch-All (Claude Sonnet 5)"
  provider: "openrouter"
  model: "anthropic/claude-sonnet-5"
  when: "true"

# =============================================================================
# FAIRY DUSTING - Proactive Frontier Quality Checkpoints
# =============================================================================
fairy_dust:
  enabled: true
  entries:
    # Tactical Code Review (Gemini 3.8 Flash)
    - name: "Tactical Code Review"
      model: "google/gemini-3.8-flash"
      provider: "openrouter"
      frequency: 15
      max_per_session: 5
      priority: 10
      prompt: >
        [QUALITY CHECKPOINT - Tactical Review] You are a senior code reviewer
        consulted mid-flight. Analyze the current codebase for: (1) logic bugs
        and incorrect calculations, (2) compilation and type errors, (3) test failures
        or broken assertions. Fix any issues immediately with tool calls. If
        everything looks correct, confirm and continue the current task.

    # Strategic Architecture Review — SPEC TRACEABILITY AUDIT
    - name: "Strategic Architecture Review"
      # model: "anthropic/claude-opus-5"
      model: "google/gemini-3.8-flash" # swap in opus 5 for tough jobs
      provider: "openrouter"
      frequency: 60
      max_per_session: 1
      priority: 100
      prompt: >
        [QUALITY CHECKPOINT - Architecture Review] You are the lead architect
        consulted for a strategic review. Evaluate: (1) Is the agent solving the
        RIGHT problem? Compare current work against the original requirements.
        (2) Is the overall architecture sound, or has it drifted into unnecessary
        complexity? (3) Are there systemic issues (wrong patterns, missing
        abstractions, repeated mistakes) that tactical fixes won't solve? If you
        identify strategic drift, restructure the approach. If the trajectory is
        correct, confirm the direction and continue.
`
