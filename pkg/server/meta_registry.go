package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/provider"
	"github.com/dixieflatline76/nacho-flow/pkg/router"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
)

// MetaEnv provides access to runtime configuration and daemon telemetry for meta directives.
type MetaEnv struct {
	Config         *contract.Config
	Stats          *telemetry.StatsTracker
	Oracle         *telemetry.PricingOracle
	Providers      *provider.Registry
	StartTime      time.Time
	DaemonVersion  string
	SessionTracker *router.SessionTracker
	SessionKey     string
}

// MetaCommand defines the strategy interface for a local zero-cost chat directive.
type MetaCommand interface {
	Name() string
	Description() string
	Execute(ctx context.Context, reqCtx contract.RequestContext, env MetaEnv) (string, error)
}

// MetaRegistry manages and dispatches local meta commands.
type MetaRegistry struct {
	commands    map[string]MetaCommand
	commandList []MetaCommand
}

// NewMetaRegistry initializes the default meta directive registry.
func NewMetaRegistry() *MetaRegistry {
	reg := &MetaRegistry{
		commands:    make(map[string]MetaCommand),
		commandList: make([]MetaCommand, 0),
	}

	reg.Register(&HelpCommandHandler{registry: reg})
	reg.Register(&TiersCommandHandler{})
	reg.Register(&StatusCommandHandler{})
	reg.Register(&DealsCommandHandler{})
	reg.Register(&TogglesCommandHandler{})
	reg.Register(&ResetCommandHandler{})

	// Aliases
	togglesCmd := &TogglesCommandHandler{}
	reg.commands["guardrails"] = togglesCmd
	reg.commands["guardrail"] = togglesCmd
	reg.commands["features"] = togglesCmd
	reg.commands["feature"] = togglesCmd
	reg.commands["toggle"] = togglesCmd
	reg.commands["clear"] = &ResetCommandHandler{}

	// Standalone toggle commands
	reg.commands["kickstart-off"] = &KickstartOffCommandHandler{}
	reg.commands["kickstart-on"] = &KickstartOnCommandHandler{}
	reg.commands["cyclekiller-off"] = &CycleKillerOffCommandHandler{}
	reg.commands["cyclekiller-on"] = &CycleKillerOnCommandHandler{}
	reg.commands["shield-off"] = &ShieldOffCommandHandler{}
	reg.commands["shield-on"] = &ShieldOnCommandHandler{}
	reg.commands["raw-on"] = &RawOnCommandHandler{}
	reg.commands["raw-off"] = &RawOffCommandHandler{}
	reg.commands["fairydust-off"] = &FairyDustOffCommandHandler{}
	reg.commands["fairydust-on"] = &FairyDustOnCommandHandler{}

	return reg
}

// Register adds a MetaCommand to the registry.
func (r *MetaRegistry) Register(cmd MetaCommand) {
	r.commands[strings.ToLower(cmd.Name())] = cmd
	r.commandList = append(r.commandList, cmd)
}

// Execute evaluates the meta directive and returns formatted markdown.
func (r *MetaRegistry) Execute(ctx context.Context, reqCtx contract.RequestContext, env MetaEnv) (string, error) {
	cmdName := strings.ToLower(reqCtx.MetaDirective)
	cmd, exists := r.commands[cmdName]
	if !exists {
		unknown := &UnknownCommandHandler{registry: r}
		return unknown.Execute(ctx, reqCtx, env)
	}
	return cmd.Execute(ctx, reqCtx, env)
}

// Dispatch executes the meta directive, applies anti-abuse debounce, and renders response via Presenter.
func (r *MetaRegistry) Dispatch(
	w http.ResponseWriter,
	rReq *http.Request,
	reqCtx contract.RequestContext,
	env MetaEnv,
	sessionTracker *router.SessionTracker,
	isStream bool,
) {
	sessionKey := extractSessionKey(rReq)

	// 1. Anti-abuse rate debounce (2-second window for identical meta directives)
	if sessionTracker != nil && sessionTracker.ShouldDebounceMeta(sessionKey, reqCtx.MetaDirective, 2*time.Second) {
		content := fmt.Sprintf("🌮 **Nacho Flow**\n\n*Directive `@nacho:%s` recently served. Please wait a moment before resending.*", reqCtx.MetaDirective)
		r.present(w, content, isStream)
		return
	}

	// 2. Execute meta command
	content, err := r.Execute(rReq.Context(), reqCtx, env)
	if err != nil {
		content = fmt.Sprintf("🌮 **Nacho Flow: Directive Error**\n\nFailed to execute `@nacho:%s`: %v", reqCtx.MetaDirective, err)
	}

	// 3. Present formatted response
	r.present(w, content, isStream)
}

func (r *MetaRegistry) present(w http.ResponseWriter, content string, isStream bool) {
	id := fmt.Sprintf("nacho-meta-%d", time.Now().UnixNano())
	if isStream {
		presenter := &SSEMetaPresenter{}
		_ = presenter.WriteResponse(w, content, id)
	} else {
		presenter := &JSONMetaPresenter{}
		_ = presenter.WriteResponse(w, content, id)
	}
}

// HelpCommandHandler generates a dynamic help menu of all registered directives.
type HelpCommandHandler struct {
	registry *MetaRegistry
}

func (h *HelpCommandHandler) Name() string { return "help" }
func (h *HelpCommandHandler) Description() string {
	return "Show available HotSauce directives and usage"
}
func (h *HelpCommandHandler) Execute(ctx context.Context, reqCtx contract.RequestContext, env MetaEnv) (string, error) {
	var sb strings.Builder
	sb.WriteString("🌶️ **Nacho Flow: HotSauce Directives**\n\n")
	sb.WriteString("Splash a directive tag onto any prompt to override routing or query gateway metadata ($0.00 / 0ms):\n\n")

	sb.WriteString("**Heat Levels (Routing Overrides):**\n")
	sb.WriteString("• `@nacho:local` — 🟢 Mild: Force routing to local GPU ($0.00)\n")
	sb.WriteString("• `@nacho:cloud` — 🟡 Medium: Force routing to cloud workhorse tier\n")
	sb.WriteString("• `@nacho:frontier` — 🟠 Extra Hot: Force routing to frontier cloud tier (Claude Sonnet 5 / GPT-4o)\n")
	sb.WriteString("• `@nacho:reasoning` — 🔥 Inferno: Force routing to deep reasoning tier (DeepSeek-R1 / o1)\n")
	sb.WriteString("• `@nacho:tier=\"Tier Name\"` — 🌶️ Custom: Force routing to a specific named tier\n")
	sb.WriteString("• `@nacho:model=\"model-id\"` — 🌶️ Chef's Special: Force routing to a specific model ID\n\n")

	sb.WriteString("**Zero-Cost Meta Commands:**\n")
	for _, cmd := range h.registry.commandList {
		sb.WriteString(fmt.Sprintf("• `@nacho:%s` — %s\n", cmd.Name(), cmd.Description()))
	}
	sb.WriteString("\n*Directives are automatically stripped before forwarding to upstream LLMs.*")

	return sb.String(), nil
}

// TiersCommandHandler renders all active tiers and models from config.yaml.
type TiersCommandHandler struct{}

func (t *TiersCommandHandler) Name() string { return "tiers" }
func (t *TiersCommandHandler) Description() string {
	return "List your configured routing tiers and models"
}
func (t *TiersCommandHandler) Execute(ctx context.Context, reqCtx contract.RequestContext, env MetaEnv) (string, error) {
	if env.Config == nil || len(env.Config.Tiers) == 0 {
		return "🌮 **Configured Tiers**\n\nNo tiers currently configured in `config.yaml`.", nil
	}

	var sb strings.Builder
	sb.WriteString("🌮 **Your Configured Tiers**\n\n")

	for i, tier := range env.Config.Tiers {
		sb.WriteString(fmt.Sprintf("• **Tier %d: %s** — `%s` (`%s`)\n", i+1, tier.Name, tier.Model, tier.Provider))
	}

	if env.Config.DefaultTier.Name != "" {
		sb.WriteString(fmt.Sprintf("\n• **Default Fallback: %s** — `%s` (`%s`)\n",
			env.Config.DefaultTier.Name, env.Config.DefaultTier.Model, env.Config.DefaultTier.Provider))
	}

	sb.WriteString("\n*Use `@nacho:tier=\"<Tier Name>\"` to force a specific tier for this turn.*")
	return sb.String(), nil
}

// StatusCommandHandler renders live uptime, circuit breaker states, and session telemetry.
type StatusCommandHandler struct{}

func (s *StatusCommandHandler) Name() string { return "status" }
func (s *StatusCommandHandler) Description() string {
	return "View daemon health, circuit breakers, and savings"
}
func (s *StatusCommandHandler) Execute(ctx context.Context, reqCtx contract.RequestContext, env MetaEnv) (string, error) {
	uptime := "N/A"
	if !env.StartTime.IsZero() {
		uptime = time.Since(env.StartTime).Round(time.Second).String()
	}

	version := env.DaemonVersion
	if version == "" {
		version = contract.Version
	}

	var sb strings.Builder
	sb.WriteString("🌮 **Nacho Flow Daemon Status**\n\n")
	sb.WriteString(fmt.Sprintf("• **Version & Uptime**: %s (running %s)\n", version, uptime))

	if env.Stats != nil {
		snap := env.Stats.GetStats()
		pct := snap.CostReductionPct
		sb.WriteString(fmt.Sprintf("• **Turn Telemetry**: %d requests ($%.2f saved | %.1f%% net cost reduction)\n",
			snap.TotalRequests, snap.EstimatedCostSavedUSD, pct))
	}

	if env.Oracle != nil {
		pricing := env.Oracle.GetAllPricing()
		sb.WriteString(fmt.Sprintf("• **Pricing Oracle**: 🟢 ONLINE (%d models indexed)\n", len(pricing)))
	}

	return sb.String(), nil
}

// DealsCommandHandler renders active spot deals from the Pricing Oracle.
type DealsCommandHandler struct{}

func (d *DealsCommandHandler) Name() string { return "deals" }
func (d *DealsCommandHandler) Description() string {
	return "View drop-in tier replacements (Heat Seeker)"
}
func (d *DealsCommandHandler) Execute(ctx context.Context, reqCtx contract.RequestContext, env MetaEnv) (string, error) {
	if env.Oracle == nil {
		return "🔥 **Heat Seeker**\n\nPricing oracle is not active.", nil
	}

	dealsCfg := contract.DealsConfig{
		Enabled:           true,
		AlertThresholdPct: 30.0,
		MinCodingIndex:    40.0,
	}
	if env.Config != nil && env.Config.Deals.Enabled {
		dealsCfg = env.Config.Deals
	}

	deals := env.Oracle.GetDeals(dealsCfg, contract.DefaultBenchmarkPricePerMillion, 5)
	if len(deals) == 0 {
		return "🔥 **Heat Seeker**\n\nNo drop-in tier replacements currently detected.", nil
	}

	var sb strings.Builder
	sb.WriteString("🔥 **Heat Seeker**\n\n")
	for i, deal := range deals {
		if i >= 5 {
			break
		}
		cost := fmt.Sprintf("$%.2f/1M", deal.CompletionCostPerM)
		if deal.IsFree {
			cost = "FREE"
		}
		sb.WriteString(fmt.Sprintf("• **%s** (`%s`) — %s (%.0f%% discount vs benchmark)\n",
			deal.Name, deal.ModelID, cost, deal.DiscountPct))
	}
	sb.WriteString("\n*Run `nacho-flow deals` in terminal for full catalog analysis.*")
	return sb.String(), nil
}

// TogglesCommandHandler renders active session guardrails and toggles.
type TogglesCommandHandler struct{}

func (t *TogglesCommandHandler) Name() string { return "toggles" }
func (t *TogglesCommandHandler) Description() string {
	return "View active session guardrails and toggles"
}
func (t *TogglesCommandHandler) Execute(ctx context.Context, reqCtx contract.RequestContext, env MetaEnv) (string, error) {
	var g router.SessionGuardrails
	if env.SessionTracker != nil && env.SessionKey != "" {
		g = env.SessionTracker.GetGuardrails(env.SessionKey)
	}

	var sb strings.Builder
	sb.WriteString("🌮 **Nacho Flow Session Toggles & Guardrails**\n\n")

	// 1. Kickstart
	if g.KickstartDisabled {
		sb.WriteString("• **HotSauce Kickstart**: ⏸️ OFF (Stall auto-escalation disabled)\n")
	} else {
		sb.WriteString("• **HotSauce Kickstart**: 🟢 ON  (Auto-suspends in read-only / plan mode)\n")
	}
	sb.WriteString("  ↳ Control: `@nacho:kickstart-off` | `@nacho:kickstart-on`\n\n")

	// 2. Cycle Killer
	if g.CycleKillerDisabled {
		sb.WriteString("• **Cycle Killer**:       ⏸️ OFF (Infinite loop aborts disabled)\n")
	} else {
		sb.WriteString("• **Cycle Killer**:       🟢 ON  (Aborts infinite loops & stream repetition)\n")
	}
	sb.WriteString("  ↳ Control: `@nacho:cyclekiller-off` | `@nacho:cyclekiller-on`\n\n")

	// 3. Fallback Shield
	if g.ShieldDisabled {
		sb.WriteString("• **Fallback Shield**:    ⏸️ OFF (Trailing text questions stream as prose)\n")
	} else {
		sb.WriteString("• **Fallback Shield**:    🟢 ON  (Synthesizes tool calls for trailing text)\n")
	}
	sb.WriteString("  ↳ Control: `@nacho:shield-off` | `@nacho:shield-on`\n\n")

	// 4. Raw Pass-Through
	if g.RawModeEnabled {
		sb.WriteString("• **Raw Pass-Through**:   🟢 ON  (Normalizers and formatters bypassed)\n")
	} else {
		sb.WriteString("• **Raw Pass-Through**:   ⚪ OFF (Normalizers and formatting active)\n")
	}
	sb.WriteString("  ↳ Control: `@nacho:raw-on` | `@nacho:raw-off`\n\n")

	// 5. Fairy Dusting
	if g.FairyDustDisabled {
		sb.WriteString("• **Fairy Dusting**:      ⏸️ OFF (Strategic review escalation disabled)\n")
	} else {
		sb.WriteString("• **Fairy Dusting**:      🟢 ON  (Strategic model escalation enabled)\n")
	}
	sb.WriteString("  ↳ Control: `@nacho:fairydust-off` | `@nacho:fairydust-on`\n\n")

	sb.WriteString("*Tip: Use `@nacho:reset` to restore all session toggles to defaults.*")
	return sb.String(), nil
}

// ResetCommandHandler resets session retry counters, cooldowns, and guardrails.
type ResetCommandHandler struct{}

func (r *ResetCommandHandler) Name() string { return "reset" }
func (r *ResetCommandHandler) Description() string {
	return "Reset session counters and restore default toggles"
}
func (r *ResetCommandHandler) Execute(ctx context.Context, reqCtx contract.RequestContext, env MetaEnv) (string, error) {
	if env.SessionTracker != nil && env.SessionKey != "" {
		env.SessionTracker.ResetSession(env.SessionKey)
	}
	return "🔄 **Session Reset**\n\nAll session retry counters, model cooldowns, and guardrail toggles have been reset to defaults.", nil
}

// Standalone toggle command handlers
type KickstartOffCommandHandler struct{}

func (k *KickstartOffCommandHandler) Name() string { return "kickstart-off" }
func (k *KickstartOffCommandHandler) Description() string {
	return "Suspend Kickstart stalled-turn detection"
}
func (k *KickstartOffCommandHandler) Execute(ctx context.Context, reqCtx contract.RequestContext, env MetaEnv) (string, error) {
	if env.SessionTracker != nil && env.SessionKey != "" {
		env.SessionTracker.SetKickstartDisabled(env.SessionKey, true)
	}
	return "🌮 **Kickstart Suspended**\n\nStalled-turn detection is now paused for this session. Use `@nacho:kickstart-on` to resume.", nil
}

type KickstartOnCommandHandler struct{}

func (k *KickstartOnCommandHandler) Name() string { return "kickstart-on" }
func (k *KickstartOnCommandHandler) Description() string {
	return "Re-enable Kickstart stalled-turn detection"
}
func (k *KickstartOnCommandHandler) Execute(ctx context.Context, reqCtx contract.RequestContext, env MetaEnv) (string, error) {
	if env.SessionTracker != nil && env.SessionKey != "" {
		env.SessionTracker.SetKickstartDisabled(env.SessionKey, false)
	}
	return "🌮 **Kickstart Active**\n\nStalled-turn detection is now re-enabled for this session.", nil
}

type CycleKillerOffCommandHandler struct{}

func (c *CycleKillerOffCommandHandler) Name() string { return "cyclekiller-off" }
func (c *CycleKillerOffCommandHandler) Description() string {
	return "Suspend Cycle Killer infinite loop aborts"
}
func (c *CycleKillerOffCommandHandler) Execute(ctx context.Context, reqCtx contract.RequestContext, env MetaEnv) (string, error) {
	if env.SessionTracker != nil && env.SessionKey != "" {
		env.SessionTracker.SetCycleKillerDisabled(env.SessionKey, true)
	}
	return "🌮 **Cycle Killer Disabled**\n\nRepetitive loop and stream runaway defense is paused for this session. Use `@nacho:cyclekiller-on` to resume.", nil
}

type CycleKillerOnCommandHandler struct{}

func (c *CycleKillerOnCommandHandler) Name() string { return "cyclekiller-on" }
func (c *CycleKillerOnCommandHandler) Description() string {
	return "Re-enable Cycle Killer infinite loop aborts"
}
func (c *CycleKillerOnCommandHandler) Execute(ctx context.Context, reqCtx contract.RequestContext, env MetaEnv) (string, error) {
	if env.SessionTracker != nil && env.SessionKey != "" {
		env.SessionTracker.SetCycleKillerDisabled(env.SessionKey, false)
	}
	return "🌮 **Cycle Killer Active**\n\nRepetitive loop and stream runaway defense is active.", nil
}

type ShieldOffCommandHandler struct{}

func (s *ShieldOffCommandHandler) Name() string { return "shield-off" }
func (s *ShieldOffCommandHandler) Description() string {
	return "Disable Fallback Shield interactive tool synthesis"
}
func (s *ShieldOffCommandHandler) Execute(ctx context.Context, reqCtx contract.RequestContext, env MetaEnv) (string, error) {
	if env.SessionTracker != nil && env.SessionKey != "" {
		env.SessionTracker.SetShieldDisabled(env.SessionKey, true)
	}
	return "🌮 **Fallback Shield Disabled**\n\nInteractive tool call synthesis is paused. Responses with trailing questions will stream as plain text.", nil
}

type ShieldOnCommandHandler struct{}

func (s *ShieldOnCommandHandler) Name() string { return "shield-on" }
func (s *ShieldOnCommandHandler) Description() string {
	return "Re-enable Fallback Shield interactive tool synthesis"
}
func (s *ShieldOnCommandHandler) Execute(ctx context.Context, reqCtx contract.RequestContext, env MetaEnv) (string, error) {
	if env.SessionTracker != nil && env.SessionKey != "" {
		env.SessionTracker.SetShieldDisabled(env.SessionKey, false)
	}
	return "🌮 **Fallback Shield Active**\n\nInteractive tool call synthesis is active.", nil
}

type RawOnCommandHandler struct{}

func (r *RawOnCommandHandler) Name() string        { return "raw-on" }
func (r *RawOnCommandHandler) Description() string { return "Enable Raw Pass-Through mode" }
func (r *RawOnCommandHandler) Execute(ctx context.Context, reqCtx contract.RequestContext, env MetaEnv) (string, error) {
	if env.SessionTracker != nil && env.SessionKey != "" {
		env.SessionTracker.SetRawModeEnabled(env.SessionKey, true)
	}
	return "🌮 **Raw Pass-Through Active**\n\nStream normalizers and formatters are bypassed for this session.", nil
}

type RawOffCommandHandler struct{}

func (r *RawOffCommandHandler) Name() string        { return "raw-off" }
func (r *RawOffCommandHandler) Description() string { return "Disable Raw Pass-Through mode" }
func (r *RawOffCommandHandler) Execute(ctx context.Context, reqCtx contract.RequestContext, env MetaEnv) (string, error) {
	if env.SessionTracker != nil && env.SessionKey != "" {
		env.SessionTracker.SetRawModeEnabled(env.SessionKey, false)
	}
	return "🌮 **Raw Pass-Through Disabled**\n\nStream normalizers and formatters are active.", nil
}

type FairyDustOffCommandHandler struct{}

func (f *FairyDustOffCommandHandler) Name() string { return "fairydust-off" }
func (f *FairyDustOffCommandHandler) Description() string {
	return "Suspend Fairy Dust strategic model escalations"
}
func (f *FairyDustOffCommandHandler) Execute(ctx context.Context, reqCtx contract.RequestContext, env MetaEnv) (string, error) {
	if env.SessionTracker != nil && env.SessionKey != "" {
		env.SessionTracker.SetFairyDustDisabled(env.SessionKey, true)
	}
	return "🌮 **Fairy Dusting Disabled**\n\nStrategic model escalations are paused for this session.", nil
}

type FairyDustOnCommandHandler struct{}

func (f *FairyDustOnCommandHandler) Name() string { return "fairydust-on" }
func (f *FairyDustOnCommandHandler) Description() string {
	return "Re-enable Fairy Dust strategic model escalations"
}
func (f *FairyDustOnCommandHandler) Execute(ctx context.Context, reqCtx contract.RequestContext, env MetaEnv) (string, error) {
	if env.SessionTracker != nil && env.SessionKey != "" {
		env.SessionTracker.SetFairyDustDisabled(env.SessionKey, false)
	}
	return "🌮 **Fairy Dusting Active**\n\nStrategic model escalations are active.", nil
}

// UnknownCommandHandler formats a friendly suggestion for unrecognized directives.
type UnknownCommandHandler struct {
	registry *MetaRegistry
}

func (u *UnknownCommandHandler) Execute(ctx context.Context, reqCtx contract.RequestContext, env MetaEnv) (string, error) {
	candidates := []string{
		"local", "cloud", "frontier", "reasoning",
		"help", "tiers", "status", "deals", "toggles", "reset",
		"kickstart-off", "kickstart-on",
		"cyclekiller-off", "cyclekiller-on",
		"shield-off", "shield-on",
		"raw-on", "raw-off",
		"fairydust-off", "fairydust-on",
	}
	for _, cmd := range u.registry.commandList {
		candidates = append(candidates, cmd.Name())
	}

	raw := reqCtx.MetaDirectiveRaw
	if raw == "" {
		raw = "@nacho:" + reqCtx.MetaDirective
	}

	rawKeyword := strings.TrimPrefix(raw, "@nacho:")
	closest := router.FindClosestDirective(rawKeyword, candidates, 2)

	var sb strings.Builder
	sb.WriteString("🌮 **Nacho Flow Directives**\n\n")
	if closest != "" {
		sb.WriteString(fmt.Sprintf("You typed `%s` — did you mean `@nacho:%s`?\n\n", raw, closest))
	} else {
		sb.WriteString(fmt.Sprintf("You typed `%s` — that is not a recognized directive.\n\n", raw))
	}

	sb.WriteString("**Available Directives:**\n")
	sb.WriteString("• `@nacho:local` — Force local GPU ($0.00)\n")
	sb.WriteString("• `@nacho:cloud` — Force cloud fallback tier\n")
	sb.WriteString("• `@nacho:reasoning` — Force deep reasoning tier\n")
	sb.WriteString("• `@nacho:tier=\"Tier Name\"` — Force specific named tier\n")
	sb.WriteString("• `@nacho:model=\"model-id\"` — Force specific model ID\n")
	sb.WriteString("• `@nacho:help` — Show full directives guide\n")
	sb.WriteString("• `@nacho:tiers` — List your configured tiers\n")
	sb.WriteString("• `@nacho:status` — View daemon health and metrics\n")
	sb.WriteString("• `@nacho:deals` — View active spot market deals\n\n")
	sb.WriteString("*Remove the directive tag and resend your prompt to use normal smart routing.*")

	return sb.String(), nil
}

// RenderCircuitBlocked returns a zero-cost chat alert when a forced tier provider circuit is open.
func RenderCircuitBlocked(tierName, providerName string) string {
	return fmt.Sprintf("🌮 **Nacho Flow: Circuit Alert**\n\n"+
		"The provider for tier **%s** (`%s`) is currently **unavailable** (Circuit Breaker: OPEN).\n"+
		"Your forced directive was blocked to prevent a request failure.\n\n"+
		"**Recommended Actions:**\n"+
		"• Run with `@nacho:cloud` to route to your cloud fallback tier.\n"+
		"• Check the health of your local service (`%s`).", tierName, providerName, providerName)
}

// JSONMetaPresenter serializes OpenAI chat.completion JSON for non-streaming clients.
type JSONMetaPresenter struct{}

func (p *JSONMetaPresenter) WriteResponse(w http.ResponseWriter, content string, id string) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	resp := map[string]interface{}{
		"id":      id,
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   "nacho-flow",
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     0,
			"completion_tokens": 0,
			"total_tokens":      0,
		},
	}

	return json.NewEncoder(w).Encode(resp)
}

// SSEMetaPresenter serializes OpenAI chat.completion.chunk SSE stream for streaming clients.
type SSEMetaPresenter struct{}

func (p *SSEMetaPresenter) WriteResponse(w http.ResponseWriter, content string, id string) error {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, hasFlusher := w.(http.Flusher)

	// Chunk 1: Role delta
	chunk1 := map[string]interface{}{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   "nacho-flow",
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"delta": map[string]interface{}{
					"role": "assistant",
				},
				"finish_reason": nil,
			},
		},
	}
	b1, _ := json.Marshal(chunk1)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", b1)
	if hasFlusher {
		flusher.Flush()
	}

	// Chunk 2: Content delta
	chunk2 := map[string]interface{}{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   "nacho-flow",
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"delta": map[string]interface{}{
					"content": content,
				},
				"finish_reason": nil,
			},
		},
	}
	b2, _ := json.Marshal(chunk2)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", b2)
	if hasFlusher {
		flusher.Flush()
	}

	// Chunk 3: Stop finish reason
	chunk3 := map[string]interface{}{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   "nacho-flow",
		"choices": []map[string]interface{}{
			{
				"index":         0,
				"delta":         map[string]interface{}{},
				"finish_reason": "stop",
			},
		},
	}
	b3, _ := json.Marshal(chunk3)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", b3)

	// Done sentinel
	_, _ = fmt.Fprintf(w, "data: [DONE]\n\n")
	if hasFlusher {
		flusher.Flush()
	}

	return nil
}
