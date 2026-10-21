import * as vscode from 'vscode';
import * as path from 'path';
import { parseConfigSymbolIndex, SourceRange, ProviderSymbolDef, ConfigSymbolIndex } from './config-symbol-index';
import { validateNachoConfigYaml } from './yaml-validator';
import { discoverModels, resolveEffectiveBaseUrl } from './model-discovery';

export const REQUEST_CONTEXT_DOCS: Record<string, string> = {
	Tokens: 'Total estimated input tokens in the current request turn.',
	RawTokens: 'Exact raw token count of the prompt before compaction or NTS.',
	Retries: 'Number of consecutive retry attempts for the current turn.',
	IsRetry: 'True if this turn is an immediate retry of a failed request.',
	MessageCount: 'Total number of messages in the session conversation history.',
	HasImages: 'True if any user message contains image content blocks.',
	HasTools: 'True if the client sent tool/function declarations.',
	InteractiveTool: 'Name of interactive tool being invoked (e.g. `write_to_file`, `execute_command`).',
	Keywords: 'List of detected task keywords (e.g. `[\'debug\', \'refactor\', \'test\']`).',
	Prompt: 'Raw text of the most recent user prompt.',
	CleanPrompt: 'Sanitized prompt text with system boilerplates stripped.',
	HasToolProgress: 'True if recent turns executed successful tool invocations.',
	HasWriteProgress: 'True if recent turns modified files or generated artifacts.',
	HasShellWrite: 'True if terminal commands were executed.',
	HasTestProgress: 'True if test runners were executed.',
	HasTestPass: 'True if test execution succeeded.',
	HasTestFail: 'True if test execution failed.',
	HasActiveRepair: 'True if active error repair latch is engaged.',
	HasWriteCapability: 'True if the model has file-write tool declarations.',
	NoKickstart: 'Directive disabling Read Slap / Kickstart for this turn.',
	NoCycleKiller: 'Directive disabling loop and repetition defense.',
	NoShield: 'Directive disabling gateway rate/error shield.',
	RawModeEnabled: 'True if raw passthrough mode is active.',
	HistoryErrors: 'Count of consecutive error responses in conversation history.',
	CycleRetries: 'Number of times Cycle Killer was triggered.',
	CycleBreakerTriggered: 'True if loop repetition or monologue was detected.',
	CycleBreakerReason: 'Reason string for loop detection (e.g. `ngram_repetition`, `reasoning_monologue`).',
	CycleContentTokens: 'Tokens spent in content lane during the current turn.',
	CycleThinkingTokens: 'Tokens spent in reasoning/thinking lane during the current turn.',
	CycleToolTokens: 'Tokens spent in tool argument streaming lane.',
	SessionReadSlapped: 'True if Read Slap nudged the agent this session.',
	SessionReadSlapCount: 'Number of Read Slap nudges applied in this session.',
	SessionThoughtSlapCount: 'Number of Thought Slap reasoning limit recoveries in this session.',
	ThoughtSlapAttempts: 'Number of Thought Slap attempts on the active turn.',
	ThoughtSlapOutcome: 'Outcome status of Thought Slap (`recovered_tool_call`, `recovered_content`, `exhausted`).',
	ThoughtSlapTier: 'Tier name executing the Thought Slap rescue.',
	ForcedTier: 'Explicit tier override set via `#nacho:tier=NAME` directive.',
	ForcedModel: 'Explicit model override set via `#nacho:model=NAME` directive.',
	IsModelCoolingDown: 'Helper function: `IsModelCoolingDown(\'model-name\')` returns true if cooling down.',
	ClientID: 'Client identifier (e.g. `\'cline\'`, `\'zoo-code\'`, `\'cursor\'`).',
	NTSTokensSaved: 'Tokens saved via Neural Token Saver on this session.',
	NTSBytesSaved: 'Bytes saved via Neural Token Saver on this session.',
	FairyDusted: 'True if Fairy Dust reviewer injection was triggered.',
	FairyDustCount: 'Number of Fairy Dust quality checkpoints triggered in this session.'
};

export interface ConfigDocItem {
	title: string;
	desc: string;
	detail?: string;
}

export const CONFIG_KEY_DOCS: Record<string, ConfigDocItem> = {
	// Top-level config
	version: {
		title: '📄 `version` *(Schema Version)*',
		desc: 'Configuration schema version format (e.g. `"1.1.0"`).'
	},
	port: {
		title: '🔌 `port` *(Gateway Port)*',
		desc: 'TCP port on which the Nacho Flow local proxy gateway listens for agent requests (default: `8000`).'
	},
	host: {
		title: '🌐 `host` *(Bind Address)*',
		desc: 'Network interface bind address (e.g. `"127.0.0.1"` for local isolation, `"0.0.0.0"` for LAN access).'
	},
	bind_address: {
		title: '🌐 `bind_address` *(Bind Address)*',
		desc: 'Network interface bind address (alias for `host`).'
	},
	providers: {
		title: '🔌 `providers` *(Upstream Providers)*',
		desc: 'Dictionary of declared inference backends: local GPU engines (`llamacpp`, `vllm`, `sglang`, `ollama`) and cloud APIs (`anthropic`, `openrouter`, `openai`, `deepseek`).'
	},
	tiers: {
		title: '⚡ `tiers` *(Dynamic Routing Tiers)*',
		desc: 'Ordered waterfall routing tiers evaluated sequentially from top to bottom. The first tier whose `when:` condition matches is selected to dispatch the request.'
	},
	default_tier: {
		title: '🛡️ `default_tier` *(Fallback Catch-All Tier)*',
		desc: 'Final fallback tier used when no waterfall tier `when:` condition matches. Ensures request continuity without failing.'
	},
	router: {
		title: '🔀 `router` *(Router Directives)*',
		desc: 'In-flight directive parsing configurations (e.g. `#nacho:tier=NAME`, `#nacho:raw=true`).'
	},

	// Deals (Heatseeker)
	deals: {
		title: '🔥 `deals` *(Heatseeker Live Model Deals)*',
		desc: 'Autonomous spot price and discount monitoring engine. Detects temporary or permanent promotional pricing on frontier and coding models.',
		detail: '**Subkeys:** `enabled`, `alert_threshold_pct`, `min_coding_index`, `require_tools`, `exclude_batch`, `exclude_free`'
	},
	alert_threshold_pct: {
		title: '📉 `alert_threshold_pct` *(Deal Discount Threshold)*',
		desc: 'Minimum discount percentage required to alert or recommend a deal (default: `30.0`%).'
	},
	min_coding_index: {
		title: '📊 `min_coding_index` *(Minimum Coding Benchmark Index)*',
		desc: 'Minimum benchmark coding index required for deal eligibility (default: `40.0`). Prevents cheap but poor-quality models from being recommended.'
	},
	require_tools: {
		title: '🛠️ `require_tools` *(Require Tool-Calling Capability)*',
		desc: 'When `true`, excludes deal models that do not declare tool-calling capabilities (default: `true`).'
	},
	exclude_batch: {
		title: '⏱️ `exclude_batch` *(Exclude Batch Endpoints)*',
		desc: 'When `true`, filters out non-interactive asynchronous batch endpoints from deal consideration (default: `true`).'
	},
	exclude_free: {
		title: '🚫 `exclude_free` *(Exclude Free Tiers)*',
		desc: 'When `true`, filters out heavily rate-limited or congested free endpoints (default: `true`).'
	},

	// Agent Shield
	agent_shield: {
		title: '🛡️ `agent_shield` *(Agentic Tool Fallback Shield)*',
		desc: 'In-flight gateway shield that detects and intercepts broken tool invocations, repetitive question loops, and unrecoverable CLI error cycles before wasting tokens.',
		detail: '**Subkeys:** `enabled`, `tail_buffer_bytes`, `question_heuristics`, `mode_switch_heuristics`, `error_signatures`'
	},
	tail_buffer_bytes: {
		title: '📏 `tail_buffer_bytes` *(Shield Inspection Window)*',
		desc: 'Rolling byte window size inspected at stream tails for error signatures and loop patterns (default: `256` bytes).'
	},
	question_heuristics: {
		title: '❓ `question_heuristics` *(Question Loop Heuristics)*',
		desc: 'List of regular expressions that detect when an agent is asking redundant questions instead of proceeding with tool work.'
	},
	mode_switch_heuristics: {
		title: '🔄 `mode_switch_heuristics` *(Mode Switch Heuristics)*',
		desc: 'List of regex patterns detecting abrupt, unauthorized mode switch attempts by autonomous agents.'
	},
	error_signatures: {
		title: '⚠️ `error_signatures` *(Unrecoverable Error Signatures)*',
		desc: 'List of error substrings and status codes triggering immediate fallback intervention.'
	},

	// Cycle Killer / Cycle Breaker
	cycle_killer: {
		title: '💀 `cycle_killer` *(In-Flight Stream Repetition Defense)*',
		desc: 'Real-time n-gram stream monitor that detects infinite loops, repeated phrases, and monologue traps, terminating stuck streams in `<3s`.',
		detail: '**Subkeys:** `enabled`, `phrase_length`, `budget_max_repeats`, `model_cooldown_seconds`, `retry_floor`, `max_retries`, `correction_prompt`'
	},
	cycle_breaker: {
		title: '💀 `cycle_breaker` *(Cycle Breaker Config)*',
		desc: 'Alias for `cycle_killer`. Real-time repetition and reasoning monologue defense.'
	},
	phrase_length: {
		title: '🔤 `phrase_length` *(Repetition Window / N-gram Size)*',
		desc: 'Size of the sliding word/token window analyzed for consecutive identical repeats (default: `6` words).'
	},
	budget_max_repeats: {
		title: '🔁 `budget_max_repeats` *(Max Budgeted Repeats)*',
		desc: 'Maximum repeat threshold tolerated once the lane cooperative token budget is exceeded (default: `20`).'
	},
	model_cooldown_seconds: {
		title: '⏱️ `model_cooldown_seconds` *(Model Cooldown Duration)*',
		desc: 'Duration in seconds to place a model on cooldown after triggering cycle killer, preventing immediate re-entry into broken behavior (default: `120`s).'
	},
	retry_floor: {
		title: '🪜 `retry_floor` *(Retry Floor Before Escalation)*',
		desc: 'Minimum number of retries before escalating from local model to frontier cloud model (default: `3`).'
	},
	max_retries: {
		title: '🔄 `max_retries` *(Cycle Killer Max Retries)*',
		desc: 'Number of Stage 1 local retries with `[SYSTEM OVERRIDE]` prompt injection before escalating to cloud (default: `1`).'
	},
	correction_prompt: {
		title: '💬 `correction_prompt` *(Cycle Killer Correction Prompt)*',
		desc: 'System prompt injected into conversation when a cycle is broken to nudge the agent out of the loop.'
	},

	// Neural Token Saver (NTS)
	nts: {
		title: '🧠 `nts` *(Neural Token Saver)*',
		desc: 'In-place prompt compaction engine that strips redundant boilerplate, ANSI colors, progress spinners, and historical reasoning, saving `30-70%` of context tokens.',
		detail: '**Passes:** ANSI Stripping, CR Overwrites, Whitespace Normalization, Tool Output Deduplication, Test Collapse, Reasoning Pruning'
	},
	strip_ansi: {
		title: '🎨 `strip_ansi` *(Pass 1: Strip Terminal Escapes)*',
		desc: 'Strips ANSI and OSC escape sequences from terminal command outputs, reducing token clutter (default: `true`).'
	},
	resolve_cr: {
		title: '🔄 `resolve_cr` *(Pass 2: Overwrite Carriage Returns)*',
		desc: 'Overwrites carriage returns (`\\r`) from progress spinners and progress bars, keeping only the final line state (default: `true`).'
	},
	normalize_whitespace: {
		title: '🧹 `normalize_whitespace` *(Pass 3: Collapse Whitespace)*',
		desc: 'Collapses 3+ consecutive empty lines into 2 and normalizes trailing spaces (default: `true`).'
	},
	preserve_file_reads: {
		title: '📖 `preserve_file_reads` *(Dual-Lane Immunity for File Reads)*',
		desc: 'Protects contents of `read_file` and `view_file` tool responses from compaction disruption, ensuring agents see full source files (default: `true`).'
	},
	preserve_file_writes: {
		title: '✍️ `preserve_file_writes` *(Dual-Lane Immunity for File Writes)*',
		desc: 'Protects file modification and artifact outputs from compaction (default: `true`).'
	},
	preserve_cache_control: {
		title: '⚡ `preserve_cache_control` *(Prompt Cache Immunity)*',
		desc: 'Preserves prompt cache breakpoints and boundaries for Anthropic and OpenAI caching (default: `true`).'
	},
	strip_historical_reasoning: {
		title: '✂️ `strip_historical_reasoning` *(Historical Thought Pruning)*',
		desc: 'Strips `<thought>` or `thinking` blocks from earlier turns while keeping current turns intact, freeing massive amounts of context (default: `true`).'
	},
	keep_recent_reasoning_turns: {
		title: '🧠 `keep_recent_reasoning_turns` *(Recent Thought Memory Retention)*',
		desc: 'Number of most recent assistant turns to preserve reasoning blocks intact to prevent agent amnesia (default: `3`).'
	},
	deduplicate_reminders: {
		title: '📋 `deduplicate_reminders` *(Environment Reminder Deduplication)*',
		desc: 'Deduplicates repeated `<environment_details>` reminder tables across turns (default: `true`).'
	},
	reminders_refresh_interval: {
		title: '🔄 `reminders_refresh_interval` *(Reminders Re-emit Interval)*',
		desc: 'Interval in turns at which full reminder tables are refreshed into context (default: `10`).'
	},
	collapse_verbose_tests: {
		title: '🧪 `collapse_verbose_tests` *(Multi-Language Test Collapse)*',
		desc: 'Collapses verbose test runner outputs (Go test, Cargo, Pytest, Jest) to show summary failures while hiding passing noise (default: `true`).'
	},
	deduplicate_tool_outputs: {
		title: '📑 `deduplicate_tool_outputs` *(Generic Tool Deduplication)*',
		desc: 'Replaces identical repeated tool outputs (>200 B) with references to the previous turn output (default: `true`).'
	},
	min_duplicate_output_bytes: {
		title: '📏 `min_duplicate_output_bytes` *(Duplicate Output Minimum Size)*',
		desc: 'Minimum output length in bytes for duplicate detection and referencing (default: `200` bytes).'
	},
	compact_stale_file_reads: {
		title: '📁 `compact_stale_file_reads` *(Stale File Read Compaction)*',
		desc: 'Replaces older file reads with references when a newer read of the same file has occurred.'
	},
	stale_read_depth: {
		title: '⏳ `stale_read_depth` *(Stale Read Turn Depth)*',
		desc: 'Number of turns before an unreferenced file read is considered stale (default: `5`).'
	},

	// Fairy Dust
	fairy_dust: {
		title: '✨ `fairy_dust` *(Autonomous Reviewer Injection)*',
		desc: 'Injects an independent frontier model prompt checkpoint into long-running agent loops at designated turn intervals to evaluate plan alignment and prevent rabbit holes.',
		detail: '**Subkeys:** `enabled`, `entries`, `frequency`, `max_per_session`, `priority`, `prompt`'
	},
	entries: {
		title: '📝 `entries` *(Fairy Dust Reviewer Entries)*',
		desc: 'Array of reviewer model configurations, each with its own model, provider, prompt, and frequency.'
	},
	frequency: {
		title: '⏱️ `frequency` *(Turn Frequency)*',
		desc: 'Number of turns between proactive fairy dust reviewer injections (e.g. `10` = review every 10 turns).'
	},
	max_per_session: {
		title: '🛑 `max_per_session` *(Max Injections Per Session)*',
		desc: 'Upper bound on total reviewer injections allowed in a single agent session.'
	},
	priority: {
		title: '🥇 `priority` *(Reviewer Priority)*',
		desc: 'Evaluation priority ordering when multiple fairy dust reviewers are declared.'
	},
	prompt_override: {
		title: '💬 `prompt_override` *(Custom Reviewer Prompt)*',
		desc: 'Custom reviewer instructions overriding the default sanity check / architecture review prompt.'
	},

	// Kickstart
	kickstart: {
		title: '👢 `kickstart` *(Idle Loop & Truncation Defense)*',
		desc: 'Cross-turn defense managing Read Slap (for idle file reading loops) and Thought Slap (for reasoning token exhaustion).',
		detail: '**Subkeys:** `read_slap`, `thought_slap`'
	},
	read_slap: {
		title: '👋 `read_slap` *(Idle Read Loop Nudge)*',
		desc: 'Detects when an agent repeatedly reads files without executing edits or commands, and injects an actionable nudge.'
	},
	thought_slap: {
		title: '⚡ `thought_slap` *(Reasoning Token Limit Recovery)*',
		desc: 'Catches requests where reasoning models hit max token limits mid-thought and rescues them with a continuation tier.'
	},
	threshold: {
		title: '📏 `threshold` *(Slap Trigger Threshold)*',
		desc: 'Number of consecutive idle turns before Read Slap engages (default: `3`).'
	},
	max_slaps: {
		title: '🛑 `max_slaps` *(Max Slap Interventions)*',
		desc: 'Maximum number of slaps allowed per session before escalating (default: `2`).'
	},
	rescue_tier: {
		title: '🚑 `rescue_tier` *(Thought Slap Rescue Tier)*',
		desc: 'Name of the tier configured to rescue a truncated reasoning request.'
	},

	// Tier properties
	provider: {
		title: '🔌 `provider` *(Tier Provider)*',
		desc: 'ID of the provider defined under `providers:` that serves this tier.'
	},
	model: {
		title: '🧠 `model` *(Tier Model)*',
		desc: 'Model identifier dispatched to the upstream provider (e.g. `"qwen3.8-27b"`, `"claude-3-7-sonnet-20250219"`).'
	},
	reasoning_effort: {
		title: '🤔 `reasoning_effort` *(Reasoning Effort Budget)*',
		desc: 'Reasoning effort passed to reasoning/thinking models: `"low"`, `"medium"`, `"high"`, or `"max"`.'
	},
	when: {
		title: '⚡ `when` *(Tier Activation Condition)*',
		desc: 'Boolean expression evaluated against `RequestContext`. First tier whose `when:` returns true wins.',
		detail: '**Examples:** `Tokens < 60000 && Retries < 2`, `HasImages || HasTools`'
	},
	thinking_lane: {
		title: '🧠 `thinking_lane` *(Thinking Stream Inspection Lane)*',
		desc: 'Configuration for monitoring and defending model reasoning/thinking streams against loops and monologues.',
		detail: '**Subkeys:** `max_tokens`, `phrase_length`, `max_repeats`, `budget_max_repeats`, `repetition_window`, `repetition_threshold`'
	},
	reasoning_lane: {
		title: '🧠 `reasoning_lane` *(Reasoning Lane Config)*',
		desc: 'Alias for `thinking_lane`.'
	},
	content_lane: {
		title: '📝 `content_lane` *(Content Stream Inspection Lane)*',
		desc: 'Configuration for monitoring prose/content output streams against loops and runaway generation.',
		detail: '**Subkeys:** `max_tokens`, `phrase_length`, `max_repeats`, `budget_max_repeats`, `repetition_window`, `repetition_threshold`'
	},
	tool_lane: {
		title: '🔧 `tool_lane` *(Tool Stream Inspection Lane)*',
		desc: 'Configuration for inspecting tool call arguments and streaming payloads.',
		detail: '**Subkeys:** `max_tokens`, `max_write_tokens`, `write_tools`, `phrase_length`, `max_repeats`'
	},
	max_tokens: {
		title: '🔢 `max_tokens` *(Maximum Tokens Limit)*',
		desc: 'Upper token limit for generation or lane inspection before intervention.'
	},
	max_write_tokens: {
		title: '✍️ `max_write_tokens` *(Maximum File Write Tokens)*',
		desc: 'Upper token ceiling permitted for file write tool operations (default: `32768`).'
	},
	max_repeats: {
		title: '🔁 `max_repeats` *(Maximum Phrase Repeats)*',
		desc: 'Maximum count of identical phrase repetitions permitted before cycle breaker terminates stream.'
	},
	repetition_window: {
		title: '🪟 `repetition_window` *(Repetition Window Size)*',
		desc: 'Sliding n-gram token window examined for repeated phrases (alias for phrase_length).'
	},
	repetition_threshold: {
		title: '🛑 `repetition_threshold` *(Repetition Trigger Threshold)*',
		desc: 'Threshold count of repeated n-grams triggering cycle intervention.'
	},
	budget_repetition_threshold: {
		title: '🪙 `budget_repetition_threshold` *(Cooperative Budget Repeat Threshold)*',
		desc: 'Threshold of repeated phrases permitted while within token budget.'
	},
	prompt: {
		title: '💬 `prompt` *(System Override or Reviewer Prompt)*',
		desc: 'Custom prompt text injected during reviewer checks or cycle recovery.'
	},
	strip_images: {
		title: '🖼️ `strip_images` *(Strip Base64 Images)*',
		desc: 'When `true`, strips base64 encoded images from conversation history sent to this tier, preserving context tokens (default: `false`).'
	},
	has_vision: {
		title: '👁️ `has_vision` *(Explicit Vision Capability)*',
		desc: 'Explicit boolean override declaring whether this tier model supports vision/images.'
	},
	strip_tools: {
		title: '🔨 `strip_tools` *(Strip Tool Schemas)*',
		desc: 'When `true`, removes tool definitions and function calling history from requests sent to this tier.'
	},
	has_tools: {
		title: '🛠️ `has_tools` *(Explicit Tool-Calling Capability)*',
		desc: 'Explicit boolean override declaring whether this tier model supports function/tool calling.'
	},
	max_context: {
		title: '📦 `max_context` *(Maximum Context Window)*',
		desc: 'Maximum context window length in tokens allocated for this tier.'
	},
	raw: {
		title: '⚡ `raw` *(Raw Pass-Through Mode)*',
		desc: 'When `true`, passes incoming requests directly to the model unmutated without normalizers or rewriting.'
	},
	shield: {
		title: '🛡️ `shield` *(Tier-Level Shield Override)*',
		desc: 'Tier-level toggle enabling or disabling Agentic Tool Fallback Shield.'
	},
	normalizer: {
		title: '🔄 `normalizer` *(Stream Normalizer Toggle)*',
		desc: 'Tier-level toggle enabling or disabling stream response normalizers.'
	},
	normalizers: {
		title: '🔀 `normalizers` *(Normalizer Strategies)*',
		desc: 'Fine-grained normalizer sub-strategy configuration (markdown, bare_json, react, think).'
	},
	deduplicate_lines: {
		title: '📑 `deduplicate_lines` *(Deduplicate Consecutive Lines)*',
		desc: 'Strips duplicate consecutive lines from tool and terminal outputs (default: `true`).'
	},
	dedup_threshold: {
		title: '📏 `dedup_threshold` *(Line Deduplication Threshold)*',
		desc: 'Number of consecutive identical lines triggering deduplication collapse (default: `3`).'
	},
	strip_boilerplate: {
		title: '🧹 `strip_boilerplate` *(Strip Terminal Boilerplate)*',
		desc: 'Strips common repetitive terminal headers and package manager boilerplates.'
	}
};

/**
 * Checks whether a text document is a Nacho Flow configuration file.
 */
export function isNachoConfigDocument(document: {
	fileName?: string;
	uri?: any;
	languageId?: string;
	getText?: () => string;
}): boolean {
	const fsPath = document.uri?.fsPath || document.fileName || '';
	const normalized = fsPath.replace(/\\/g, '/').toLowerCase();
	const base = path.posix.basename(normalized);

	if (!base.endsWith('.yaml') && !base.endsWith('.yml')) {
		return false;
	}

	if (/^profile.*\.ya?ml$/i.test(base) || /config.*\.ya?ml$/i.test(base)) {
		return true;
	}

	if (normalized.includes('/.nacho/') || normalized.includes('/profiles/')) {
		return true;
	}

	if (typeof document.getText === 'function') {
		const text = document.getText();
		if (text.includes('providers:') && text.includes('tiers:')) {
			return true;
		}
	}

	return false;
}

export function toVsCodeRange(range: SourceRange): vscode.Range {
	return new vscode.Range(range.startLine, range.startCharacter, range.endLine, range.endCharacter);
}

/**
 * Replaces a scalar value range quote-safely: preserves existing quotes or wraps unquoted scalars in double quotes.
 */
export function replaceScalarRange(
	doc: vscode.TextDocument,
	editBuilder: vscode.TextEditorEdit,
	range: SourceRange,
	newValue: string
): void {
	const lineText = doc.lineAt(range.startLine).text;
	const charBefore = range.startCharacter > 0 ? lineText[range.startCharacter - 1] : '';
	const charAfter = range.endCharacter < lineText.length ? lineText[range.endCharacter] : '';
	const isQuoted = (charBefore === '"' && charAfter === '"') || (charBefore === "'" && charAfter === "'");
	if (isQuoted) {
		editBuilder.replace(toVsCodeRange(range), newValue);
	} else {
		editBuilder.replace(toVsCodeRange(range), `"${newValue}"`);
	}
}

function isPositionInRange(pos: vscode.Position, range: SourceRange): boolean {
	if (pos.line < range.startLine || pos.line > range.endLine) return false;
	if (pos.line === range.startLine && pos.character < range.startCharacter) return false;
	if (pos.line === range.endLine && pos.character > range.endCharacter) return false;
	return true;
}

function getActivePosition(editor: vscode.TextEditor): vscode.Position {
	return editor.selection?.active || (editor.selection as any)?.start || new vscode.Position(0, 0);
}

export interface RouteTargetArgs {
	tierName?: string;
	isDefaultTier?: boolean;
	fairyDustEntryName?: string;
	range?: SourceRange;
}

export interface RouteTarget {
	label: string;
	lensRange: SourceRange;
	blockRange: SourceRange;
	provider?: string;
	providerValueRange?: SourceRange;
	model?: string;
	modelValueRange?: SourceRange;
	commandArgs: RouteTargetArgs;
}

export function getAllRouteTargets(index: ConfigSymbolIndex): RouteTarget[] {
	const targets: RouteTarget[] = [];
	for (const [name, tier] of index.tiers) {
		targets.push({
			label: name,
			lensRange: tier.nameValueRange,
			blockRange: tier.blockRange,
			provider: tier.provider,
			providerValueRange: tier.providerValueRange,
			model: tier.model,
			modelValueRange: tier.modelValueRange,
			commandArgs: { tierName: name }
		});
	}
	if (index.defaultTier) {
		const dt = index.defaultTier;
		targets.push({
			label: dt.name || 'default_tier',
			lensRange: dt.nameValueRange || dt.blockRange,
			blockRange: dt.blockRange,
			provider: dt.provider,
			providerValueRange: dt.providerValueRange,
			model: dt.model,
			modelValueRange: dt.modelValueRange,
			commandArgs: { isDefaultTier: true, tierName: dt.name || 'default_tier' }
		});
	}
	for (const [name, entry] of index.fairyDustEntries) {
		targets.push({
			label: entry.name || name,
			lensRange: entry.nameValueRange || entry.blockRange,
			blockRange: entry.blockRange,
			provider: entry.provider,
			providerValueRange: entry.providerValueRange,
			model: entry.model,
			modelValueRange: entry.modelValueRange,
			commandArgs: { fairyDustEntryName: name }
		});
	}
	return targets;
}

export function resolveRouteTarget(
	index: ConfigSymbolIndex,
	args?: RouteTargetArgs,
	cursorLine?: number
): RouteTarget | undefined {
	const targets = getAllRouteTargets(index);
	if (args?.isDefaultTier) {
		return targets.find((t) => t.commandArgs.isDefaultTier);
	}
	if (args?.fairyDustEntryName) {
		return targets.find((t) => t.commandArgs.fairyDustEntryName === args.fairyDustEntryName);
	}
	if (args?.tierName) {
		return targets.find((t) => t.commandArgs.tierName === args.tierName);
	}
	if (typeof cursorLine === 'number') {
		return targets.find((t) => cursorLine >= t.blockRange.startLine && cursorLine <= t.blockRange.endLine);
	}
	return undefined;
}

function findUnquotedCommentStart(lineText: string, startIndex: number): number {
	let inSingle = false;
	let inDouble = false;
	for (let i = startIndex; i < lineText.length; i++) {
		const ch = lineText[i];
		if (ch === "'" && !inDouble) {
			inSingle = !inSingle;
		} else if (ch === '"' && !inSingle) {
			inDouble = !inDouble;
		} else if (ch === '#' && !inSingle && !inDouble) {
			return i;
		}
	}
	return lineText.length;
}

function applyScalarCompletionRange(
	item: vscode.CompletionItem,
	rawValue: string,
	lineText: string,
	lineNumber: number,
	cursorChar: number,
	keyName: string,
	preferQuotedWhenEmpty: boolean
): void {
	const keyRegex = new RegExp(`^(\\s*(?:-\\s*)?${keyName}\\s*:\\s*)`);
	const match = lineText.match(keyRegex);
	if (!match || cursorChar < match[1].length) {
		return;
	}
	const valStart = match[1].length;
	const commentStart = findUnquotedCommentStart(lineText, valStart);
	const rawSection = lineText.slice(valStart, commentStart);
	const trimmedVal = rawSection.trimEnd();
	const valEnd = valStart + trimmedVal.length;
	const hasOpeningQuote = trimmedVal.startsWith('"') || trimmedVal.startsWith("'");

	if (hasOpeningQuote) {
		const q = trimmedVal[0];
		const hasClosingQuote = trimmedVal.length >= 2 && trimmedVal.endsWith(q);
		if (cursorChar > valStart && (!hasClosingQuote || cursorChar < valEnd)) {
			const innerStart = valStart + 1;
			const innerEnd = hasClosingQuote ? valEnd - 1 : Math.max(valEnd, cursorChar);
			const prefix = lineText.slice(innerStart, Math.max(innerStart, cursorChar));
			(item as any).range = new vscode.Range(lineNumber, innerStart, lineNumber, innerEnd);
			item.insertText = rawValue;
			(item as any).filterText = prefix ? `${prefix} ${rawValue}` : rawValue;
			return;
		}
		const replaceEnd = Math.max(valEnd, cursorChar);
		const prefix = lineText.slice(valStart, Math.max(valStart, cursorChar));
		(item as any).range = new vscode.Range(lineNumber, valStart, lineNumber, replaceEnd);
		item.insertText = `${q}${rawValue}${q}`;
		(item as any).filterText = prefix ? `${prefix} ${rawValue}` : rawValue;
		return;
	}

	const replaceEnd = Math.max(valEnd, cursorChar);
	const prefix = lineText.slice(valStart, Math.max(valStart, cursorChar));
	(item as any).range = new vscode.Range(lineNumber, valStart, lineNumber, replaceEnd);
	item.insertText = preferQuotedWhenEmpty && trimmedVal.length === 0 ? `"${rawValue}"` : rawValue;
	(item as any).filterText = prefix ? `${prefix} ${rawValue}` : rawValue;
}

/**
 * Completion provider for provider IDs, provider types, rescue tier names, live models, reasoning effort, and DSL variables.
 */
export class NachoCompletionItemProvider implements vscode.CompletionItemProvider {
	async provideCompletionItems(
		document: vscode.TextDocument,
		position: vscode.Position
	): Promise<vscode.CompletionItem[]> {
		if (!isNachoConfigDocument(document)) return [];
		const lineText = document.lineAt(position.line).text;
		const lineUntilCursor = lineText.slice(0, position.character);
		const fullText = document.getText();
		const index = parseConfigSymbolIndex(fullText);

		// 0. type: completions (provider type: openrouter, anthropic, openai_compatible)
		if (/^\s*(?:-\s*)?type\s*:/.test(lineUntilCursor)) {
			const providerTypes: Array<{ id: string; detail: string; doc: string }> = [
				{
					id: 'openrouter',
					detail: 'OpenRouter unified cloud gateway (vendor/model IDs)',
					doc: 'Routes requests to OpenRouter (`https://openrouter.ai/api/v1`) using canonical `vendor/model` identifiers and live OpenRouter pricing/benchmark curation.'
				},
				{
					id: 'anthropic',
					detail: 'Direct Anthropic Messages API (claude-* wire IDs)',
					doc: 'Routes requests directly to Anthropic (`https://api.anthropic.com/v1/messages`) using native `x-api-key` headers and hyphenated `claude-*` wire model IDs.'
				},
				{
					id: 'openai_compatible',
					detail: 'OpenAI-compatible endpoint (Ollama, vLLM, llama.cpp, OpenAI)',
					doc: 'Routes requests to any `/v1/chat/completions` OpenAI-compatible server (local Ollama/vLLM/llama.cpp when `local: true`, or cloud OpenAI-compatible endpoints).'
				}
			];
			return providerTypes.map((pt) => {
				const item = new vscode.CompletionItem(pt.id, vscode.CompletionItemKind.EnumMember);
				item.detail = pt.detail;
				item.documentation = new vscode.MarkdownString(pt.doc);
				applyScalarCompletionRange(item, pt.id, lineText, position.line, position.character, 'type', false);
				return item;
			});
		}

		// 1. provider: completions
		if (/^\s*(?:-\s*)?provider\s*:/.test(lineUntilCursor)) {
			const items: vscode.CompletionItem[] = [];
			for (const [id, def] of index.providers) {
				const item = new vscode.CompletionItem(id, vscode.CompletionItemKind.Reference);
				item.detail = `${def.type || 'provider'} • ${def.baseUrl || 'no base url'}`;
				item.documentation = new vscode.MarkdownString(`**Nacho Flow Provider Definition**\n- Base URL: \`${def.baseUrl}\`\n- Type: \`${def.type}\``);
				applyScalarCompletionRange(item, id, lineText, position.line, position.character, 'provider', false);
				items.push(item);
			}
			return items;
		}

		// 2. rescue_tier: completions
		if (/^\s*(?:-\s*)?rescue_tier\s*:/.test(lineUntilCursor)) {
			const items: vscode.CompletionItem[] = [];
			const quoteMatch = lineUntilCursor.match(/rescue_tier\s*:\s*(["'])/);
			const alreadyHasQuote = !!quoteMatch;

			const tierNames = Array.from(index.tiers.keys());
			if (index.defaultTier?.name && !tierNames.includes(index.defaultTier.name)) {
				tierNames.push(index.defaultTier.name);
			}

			for (const name of tierNames) {
				const tierDef = index.tiers.get(name) || index.defaultTier;
				const label = alreadyHasQuote ? name : `"${name}"`;
				const item = new vscode.CompletionItem(label, vscode.CompletionItemKind.Reference);
				item.detail = `Rescue Tier • Provider: ${tierDef?.provider || 'unknown'}`;
				item.insertText = label;
				applyScalarCompletionRange(item, name, lineText, position.line, position.character, 'rescue_tier', true);
				items.push(item);
			}
			return items;
		}

		// 3. model: completions (with live discovery)
		if (/^\s*(?:-\s*)?model\s*:/.test(lineUntilCursor)) {
			const targetProviderId = resolveRouteTarget(index, undefined, position.line)?.provider;

			if (targetProviderId) {
				const provDef = index.providers.get(targetProviderId);
				const effectiveBaseUrl = resolveEffectiveBaseUrl(provDef);
				if (effectiveBaseUrl) {
					try {
						const disc = await discoverModels(effectiveBaseUrl, {
							apiKey: provDef?.apiKey,
							providerType: provDef?.type
						});
						if (disc.online && disc.models.length > 0) {
							const quoteMatch = lineUntilCursor.match(/model\s*:\s*(["'])/);
							const alreadyHasQuote = !!quoteMatch;
							return disc.models.map((m) => {
								const label = alreadyHasQuote ? m : `"${m}"`;
								const item = new vscode.CompletionItem(label, vscode.CompletionItemKind.Value);
								item.detail = `Discovered on ${targetProviderId} (${disc.latencyMs}ms)`;
								item.insertText = label;
								applyScalarCompletionRange(item, m, lineText, position.line, position.character, 'model', true);
								return item;
							});
						}
					} catch {
						// Fall through
					}
				}
			}
		}

		// 4. reasoning_effort: completions
		if (/^\s*(?:-\s*)?reasoning_effort\s*:/.test(lineUntilCursor)) {
			return ['low', 'medium', 'high', 'max'].map((effort) => {
				const item = new vscode.CompletionItem(effort, vscode.CompletionItemKind.EnumMember);
				item.detail = `Reasoning effort level: ${effort}`;
				applyScalarCompletionRange(item, effort, lineText, position.line, position.character, 'reasoning_effort', false);
				return item;
			});
		}

		// 5. when: completions (DSL variables + keywords)
		if (lineUntilCursor.includes('when:')) {
			const items: vscode.CompletionItem[] = [];
			for (const [varName, doc] of Object.entries(REQUEST_CONTEXT_DOCS)) {
				const item = new vscode.CompletionItem(varName, vscode.CompletionItemKind.Variable);
				item.detail = 'RequestContext variable';
				item.documentation = new vscode.MarkdownString(doc);
				items.push(item);
			}

			const keywords = ['any', 'all', 'none', 'one', 'filter', 'map', 'count', 'len', 'in', 'matches', 'contains', 'startsWith', 'endsWith', 'and', 'or', 'not', 'true', 'false'];
			for (const kw of keywords) {
				const item = new vscode.CompletionItem(kw, vscode.CompletionItemKind.Keyword);
				item.detail = 'DSL built-in';
				items.push(item);
			}

			return items;
		}

		return [];
	}
}

/**
 * Hover provider for RequestContext DSL variables, provider definitions, and tier references.
 */
export class NachoHoverProvider implements vscode.HoverProvider {
	provideHover(document: vscode.TextDocument, position: vscode.Position): vscode.Hover | null {
		if (!isNachoConfigDocument(document)) return null;
		const range = document.getWordRangeAtPosition(position, /[a-zA-Z0-9_]+/);
		if (!range) return null;

		const word = document.getText(range);

		// 1. RequestContext variable hover
		if (REQUEST_CONTEXT_DOCS[word]) {
			const md = new vscode.MarkdownString();
			md.appendMarkdown(`### \`${word}\` *(Nacho Flow RequestContext)*\n\n`);
			md.appendMarkdown(`${REQUEST_CONTEXT_DOCS[word]}\n\n`);
			md.appendMarkdown('*Used in tier `when:` condition expressions.*');
			return new vscode.Hover(md, range);
		}

		// 1b. Config key documentation hover (fairy_dust, agent_shield, cycle_killer, nts, deals, etc.)
		if (CONFIG_KEY_DOCS[word]) {
			const info = CONFIG_KEY_DOCS[word];
			const md = new vscode.MarkdownString();
			md.appendMarkdown(`### ${info.title}\n\n`);
			md.appendMarkdown(`${info.desc}\n\n`);
			if (info.detail) {
				md.appendMarkdown(`${info.detail}\n`);
			}
			return new vscode.Hover(md, range);
		}

		const fullText = document.getText();
		const index = parseConfigSymbolIndex(fullText);

		// 2. Provider definition hover
		const provDef = index.providers.get(word);
		if (provDef && isPositionInRange(position, provDef.idRange)) {
			const usages = index.providerUsages.filter((u) => u.providerId === word);
			const md = new vscode.MarkdownString();
			md.appendMarkdown(`### 🔌 Provider: \`${word}\`\n\n`);
			md.appendMarkdown(`- **Type:** \`${provDef.type || 'unknown'}\`\n`);
			md.appendMarkdown(`- **Base URL:** \`${provDef.baseUrl || 'none'}\`\n`);
			md.appendMarkdown(`- **References:** ${usages.length} tier(s)\n`);
			return new vscode.Hover(md, range);
		}

		// 3. Provider usage hover
		const provUsage = index.providerUsages.find((u) => u.providerId === word && isPositionInRange(position, u.valueRange));
		if (provUsage) {
			const target = index.providers.get(word);
			const md = new vscode.MarkdownString();
			md.appendMarkdown(`### 🔌 Provider Reference: \`${word}\`\n\n`);
			if (target) {
				md.appendMarkdown(`- **Resolves to:** \`${target.baseUrl}\` (${target.type || 'provider'})\n`);
			} else {
				md.appendMarkdown(`⚠️ **Unresolved:** Provider \`${word}\` is not defined in \`providers:\`.\n`);
			}
			return new vscode.Hover(md, range);
		}

		// 4. Rescue tier usage hover
		const rescueUsage = index.rescueTierUsages.find((u) => isPositionInRange(position, u.valueRange));
		if (rescueUsage) {
			const targetTier = index.tiers.get(rescueUsage.tierName) || (index.defaultTier?.name === rescueUsage.tierName ? index.defaultTier : undefined);
			const md = new vscode.MarkdownString();
			md.appendMarkdown(`### 🚑 Rescue Tier Reference: \`${rescueUsage.tierName}\`\n\n`);
			if (targetTier) {
				md.appendMarkdown(`- **Provider:** \`${targetTier.provider || 'unknown'}\`\n`);
				md.appendMarkdown(`- **Model:** \`${targetTier.model || 'default'}\`\n`);
			} else {
				md.appendMarkdown(`⚠️ **Unresolved:** Tier \`${rescueUsage.tierName}\` is not defined in \`tiers:\`.\n`);
			}
			return new vscode.Hover(md, toVsCodeRange(rescueUsage.valueRange));
		}

		return null;
	}
}

/**
 * Definition provider jumping from provider: or rescue_tier: usages to their definitions.
 */
export class NachoDefinitionProvider implements vscode.DefinitionProvider {
	provideDefinition(document: vscode.TextDocument, position: vscode.Position): vscode.Definition | null {
		if (!isNachoConfigDocument(document)) return null;
		const fullText = document.getText();
		const index = parseConfigSymbolIndex(fullText);

		// Provider usage -> Provider definition
		for (const usage of index.providerUsages) {
			if (isPositionInRange(position, usage.valueRange)) {
				const def = index.providers.get(usage.providerId);
				if (def) {
					return new vscode.Location(document.uri, toVsCodeRange(def.idRange));
				}
			}
		}

		// Rescue tier usage -> Tier definition
		for (const usage of index.rescueTierUsages) {
			if (isPositionInRange(position, usage.valueRange)) {
				const tierDef = index.tiers.get(usage.tierName);
				if (tierDef) {
					return new vscode.Location(document.uri, toVsCodeRange(tierDef.nameValueRange));
				}
				if (index.defaultTier && index.defaultTier.name === usage.tierName) {
					return new vscode.Location(document.uri, toVsCodeRange(index.defaultTier.nameValueRange));
				}
			}
		}

		return null;
	}
}

/**
 * Reference provider finding all usages of a provider ID or tier name.
 */
export class NachoReferenceProvider implements vscode.ReferenceProvider {
	provideReferences(document: vscode.TextDocument, position: vscode.Position): vscode.Location[] {
		if (!isNachoConfigDocument(document)) return [];
		const fullText = document.getText();
		const index = parseConfigSymbolIndex(fullText);
		const locations: vscode.Location[] = [];

		// Check if position is on a provider ID (definition or usage)
		let targetProviderId: string | undefined;
		for (const [id, def] of index.providers) {
			if (isPositionInRange(position, def.idRange)) {
				targetProviderId = id;
				break;
			}
		}
		if (!targetProviderId) {
			for (const usage of index.providerUsages) {
				if (isPositionInRange(position, usage.valueRange)) {
					targetProviderId = usage.providerId;
					break;
				}
			}
		}

		if (targetProviderId) {
			const def = index.providers.get(targetProviderId);
			if (def) {
				locations.push(new vscode.Location(document.uri, toVsCodeRange(def.idRange)));
			}
			for (const usage of index.providerUsages) {
				if (usage.providerId === targetProviderId) {
					locations.push(new vscode.Location(document.uri, toVsCodeRange(usage.valueRange)));
				}
			}
			return locations;
		}

		// Check if position is on a tier name (definition or rescue_tier usage)
		let targetTierName: string | undefined;
		for (const [name, def] of index.tiers) {
			if (isPositionInRange(position, def.nameValueRange)) {
				targetTierName = name;
				break;
			}
		}
		if (!targetTierName && index.defaultTier && isPositionInRange(position, index.defaultTier.nameValueRange)) {
			targetTierName = index.defaultTier.name;
		}
		if (!targetTierName) {
			for (const usage of index.rescueTierUsages) {
				if (isPositionInRange(position, usage.valueRange)) {
					targetTierName = usage.tierName;
					break;
				}
			}
		}

		if (targetTierName) {
			const tierDef = index.tiers.get(targetTierName) || (index.defaultTier?.name === targetTierName ? index.defaultTier : undefined);
			if (tierDef) {
				locations.push(new vscode.Location(document.uri, toVsCodeRange(tierDef.nameValueRange)));
			}
			for (const usage of index.rescueTierUsages) {
				if (usage.tierName === targetTierName) {
					locations.push(new vscode.Location(document.uri, toVsCodeRange(usage.valueRange)));
				}
			}
			return locations;
		}

		return [];
	}
}

/**
 * Rename provider providing atomic F2 rename across the configuration file for provider IDs and tier names.
 */
export class NachoRenameProvider implements vscode.RenameProvider {
	prepareRename(
		document: vscode.TextDocument,
		position: vscode.Position
	): { range: vscode.Range; placeholder: string } {
		const fullText = document.getText();
		const index = parseConfigSymbolIndex(fullText);

		// Provider
		for (const [id, def] of index.providers) {
			if (isPositionInRange(position, def.idRange)) {
				return { range: toVsCodeRange(def.idRange), placeholder: id };
			}
		}
		for (const usage of index.providerUsages) {
			if (isPositionInRange(position, usage.valueRange)) {
				return { range: toVsCodeRange(usage.valueRange), placeholder: usage.providerId };
			}
		}

		// Tier name
		for (const [name, def] of index.tiers) {
			if (isPositionInRange(position, def.nameValueRange)) {
				return { range: toVsCodeRange(def.nameValueRange), placeholder: name };
			}
		}
		if (index.defaultTier && index.defaultTier.name && isPositionInRange(position, index.defaultTier.nameValueRange)) {
			return { range: toVsCodeRange(index.defaultTier.nameValueRange), placeholder: index.defaultTier.name };
		}
		for (const usage of index.rescueTierUsages) {
			if (isPositionInRange(position, usage.valueRange)) {
				return { range: toVsCodeRange(usage.valueRange), placeholder: usage.tierName };
			}
		}

		throw new Error('You cannot rename this element. Place cursor on a provider ID or tier name.');
	}

	provideRenameEdits(
		document: vscode.TextDocument,
		position: vscode.Position,
		newName: string
	): vscode.WorkspaceEdit {
		if (!isNachoConfigDocument(document)) return new vscode.WorkspaceEdit();
		const { placeholder: oldName } = this.prepareRename(document, position);
		const fullText = document.getText();
		const index = parseConfigSymbolIndex(fullText);

		const ranges = index.getRenameEdits(oldName, newName);
		const edit = new vscode.WorkspaceEdit();
		for (const r of ranges) {
			edit.replace(document.uri, toVsCodeRange(r), newName);
		}
		return edit;
	}
}

/**
 * CodeAction provider offering QuickFixes from YAML validation issues and QuickPick helpers.
 */
export class NachoCodeActionProvider implements vscode.CodeActionProvider {
	provideCodeActions(
		document: vscode.TextDocument,
		range: vscode.Range | vscode.Selection,
		_context: vscode.CodeActionContext
	): vscode.CodeAction[] {
		if (!isNachoConfigDocument(document)) return [];
		const fullText = document.getText();
		const issues = validateNachoConfigYaml(fullText);
		const actions: vscode.CodeAction[] = [];

		for (const issue of issues) {
			const issueVsRange = toVsCodeRange(issue.range);
			if (!issueVsRange.intersection(range) && !issueVsRange.contains(range.start)) {
				continue;
			}

			if (issue.quickFixes) {
				for (const qf of issue.quickFixes) {
					const action = new vscode.CodeAction(qf.title, vscode.CodeActionKind.QuickFix);
					action.edit = new vscode.WorkspaceEdit();
					action.edit.replace(document.uri, toVsCodeRange(qf.range), qf.replacement);
					action.isPreferred = true;
					actions.push(action);
				}
			}

			if (issue.ruleId === 'UNKNOWN_PROVIDER_REF') {
				const pickAction = new vscode.CodeAction('💡 Pick from defined providers...', vscode.CodeActionKind.QuickFix);
				pickAction.command = {
					title: 'Pick Provider',
					command: 'nacho-flow.pickProvider',
					arguments: [{ range: issue.range }]
				};
				actions.push(pickAction);
			}

			if (issue.ruleId === 'UNKNOWN_RESCUE_TIER') {
				const pickTierAction = new vscode.CodeAction('💡 Pick from defined tiers...', vscode.CodeActionKind.QuickFix);
				pickTierAction.command = {
					title: 'Pick Rescue Tier',
					command: 'nacho-flow.pickRescueTier',
					arguments: [{ range: issue.range }]
				};
				actions.push(pickTierAction);
			}
		}

		return actions;
	}
}

/**
 * CodeLens provider showing reference counts, ping endpoint, and tier helpers.
 */
export class NachoCodeLensProvider implements vscode.CodeLensProvider {
	provideCodeLenses(document: vscode.TextDocument): vscode.CodeLens[] {
		if (!isNachoConfigDocument(document)) return [];
		const fullText = document.getText();
		const index = parseConfigSymbolIndex(fullText);
		const lenses: vscode.CodeLens[] = [];

		// Providers
		for (const [id, def] of index.providers) {
			const range = toVsCodeRange(def.idRange);
			const usages = index.providerUsages.filter((u) => u.providerId === id);
			const refCount = usages.length;
			const refTitle = refCount === 0 ? '⚠️ 0 references (Unused)' : `${refCount} reference${refCount === 1 ? '' : 's'}`;

			lenses.push(
				new vscode.CodeLens(range, {
					title: refTitle,
					command: 'nacho-flow.findProviderReferences',
					arguments: [id]
				})
			);

			const effectiveUrl = resolveEffectiveBaseUrl(def);
			if (effectiveUrl) {
				lenses.push(
					new vscode.CodeLens(range, {
						title: '🔌 Ping Endpoint',
						command: 'nacho-flow.pingProviderEndpoint',
						arguments: [effectiveUrl, id]
					})
				);
			}
		}

		// Route Targets (Tiers, Default Tier, Fairy Dust Entries)
		for (const target of getAllRouteTargets(index)) {
			const range = toVsCodeRange(target.lensRange);

			lenses.push(
				new vscode.CodeLens(range, {
					title: '🔌 Switch Provider',
					command: 'nacho-flow.pickProvider',
					arguments: [target.commandArgs]
				})
			);

			lenses.push(
				new vscode.CodeLens(range, {
					title: '🧠 Pick Model',
					command: 'nacho-flow.pickModel',
					arguments: [target.commandArgs]
				})
			);
		}

		// Rescue Tiers (Thought Slap)
		for (const usage of index.rescueTierUsages) {
			const range = toVsCodeRange(usage.valueRange);

			lenses.push(
				new vscode.CodeLens(range, {
					title: '🚑 Pick Rescue Tier',
					command: 'nacho-flow.pickRescueTier',
					arguments: [{ range: usage.valueRange }]
				})
			);
		}

		return lenses;
	}
}

/**
 * DocumentLink provider making provider base_urls clickable.
 */
export class NachoDocumentLinkProvider implements vscode.DocumentLinkProvider {
	provideDocumentLinks(document: vscode.TextDocument): vscode.DocumentLink[] {
		if (!isNachoConfigDocument(document)) return [];
		const fullText = document.getText();
		const index = parseConfigSymbolIndex(fullText);
		const links: vscode.DocumentLink[] = [];

		for (const [, def] of index.providers) {
			if (def.baseUrl && def.baseUrlRange) {
				try {
					const uri = vscode.Uri.parse(def.baseUrl);
					links.push(new vscode.DocumentLink(toVsCodeRange(def.baseUrlRange), uri));
				} catch {
					// Invalid URI, skip
				}
			}
		}

		return links;
	}
}

/**
 * Registers all Nacho Flow YAML language features and commands.
 */
export function registerYamlLanguageFeatures(
	context: vscode.ExtensionContext,
	diagnosticCollection?: vscode.DiagnosticCollection
): vscode.Disposable {
	const disposables: vscode.Disposable[] = [];

	const documentSelector: vscode.DocumentSelector = [
		{ language: 'yaml' },
		{ pattern: '**/*config*.yaml' },
		{ pattern: '**/profile*.yaml' }
	];

	// Register Language Providers
	disposables.push(
		vscode.languages.registerCompletionItemProvider(
			documentSelector,
			new NachoCompletionItemProvider(),
			':', ' ', '"', "'", '.', ','
		),
		vscode.languages.registerHoverProvider(documentSelector, new NachoHoverProvider()),
		vscode.languages.registerDefinitionProvider(documentSelector, new NachoDefinitionProvider()),
		vscode.languages.registerReferenceProvider(documentSelector, new NachoReferenceProvider()),
		vscode.languages.registerRenameProvider(documentSelector, new NachoRenameProvider()),
		vscode.languages.registerCodeActionsProvider(documentSelector, new NachoCodeActionProvider()),
		vscode.languages.registerCodeLensProvider(documentSelector, new NachoCodeLensProvider()),
		vscode.languages.registerDocumentLinkProvider(documentSelector, new NachoDocumentLinkProvider())
	);

	// Register Interactive QuickPick Commands
	disposables.push(
		vscode.commands.registerCommand('nacho-flow.pickProvider', async (args?: RouteTargetArgs) => {
			const editor = vscode.window.activeTextEditor;
			if (!editor || !isNachoConfigDocument(editor.document)) return;

			const fullText = editor.document.getText();
			const index = parseConfigSymbolIndex(fullText);
			const providerKeys = Array.from(index.providers.keys());
			if (providerKeys.length === 0) {
				vscode.window.showInformationMessage('No providers defined in this configuration.');
				return;
			}

			const items = providerKeys.map((pId) => {
				const def = index.providers.get(pId)!;
				const usages = index.providerUsages.filter((u) => u.providerId === pId).length;
				return {
					label: `$(server) ${pId}`,
					description: `${def.type || 'provider'} • ${def.baseUrl || 'no base url'} (${usages} tier usages)`,
					providerId: pId
				};
			});

			const activePos = getActivePosition(editor);
			const target = resolveRouteTarget(index, args, activePos.line);

			const selected = await vscode.window.showQuickPick(items, {
				placeHolder: target?.label ? `Select provider for ${target.label}` : 'Select a defined provider'
			});

			if (!selected) return;

			// If args.range provided (e.g. from Code Action quickFix on unknown provider)
			if (args?.range) {
				await editor.edit((editBuilder) => {
					replaceScalarRange(editor.document, editBuilder, args.range!, selected.providerId);
				});
				return;
			}

			if (target?.providerValueRange) {
				await editor.edit((editBuilder) => {
					replaceScalarRange(editor.document, editBuilder, target.providerValueRange!, selected.providerId);
				});
				return;
			}

			// Else insert/replace at cursor position
			await editor.edit((editBuilder) => {
				editBuilder.replace(editor.selection, `"${selected.providerId}"`);
			});
		}),

		vscode.commands.registerCommand('nacho-flow.pickRescueTier', async (args?: { range?: SourceRange }) => {
			const editor = vscode.window.activeTextEditor;
			if (!editor || !isNachoConfigDocument(editor.document)) return;

			const fullText = editor.document.getText();
			const index = parseConfigSymbolIndex(fullText);
			const tierNames = Array.from(index.tiers.keys());
			if (index.defaultTier?.name && !tierNames.includes(index.defaultTier.name)) {
				tierNames.push(index.defaultTier.name);
			}

			if (tierNames.length === 0) {
				vscode.window.showInformationMessage('No tiers defined in this configuration.');
				return;
			}

			const items = tierNames.map((name) => {
				const tier = index.tiers.get(name) || index.defaultTier;
				return {
					label: `$(symbol-event) ${name}`,
					description: `Provider: ${tier?.provider || 'unknown'} • Model: ${tier?.model || 'default'}`,
					tierName: name
				};
			});

			const selected = await vscode.window.showQuickPick(items, {
				placeHolder: 'Select a defined tier to act as rescue_tier'
			});

			if (!selected) return;

			// If args.range provided (e.g. from Code Action quickFix on unknown rescue tier)
			if (args?.range) {
				await editor.edit((editBuilder) => {
					replaceScalarRange(editor.document, editBuilder, args.range!, selected.tierName);
				});
				return;
			}

			// Check cursor position across rescue tier usages
			const activePos = getActivePosition(editor);
			const line = activePos.line;
			for (const usage of index.rescueTierUsages) {
				if (line === usage.valueRange.startLine && usage.valueRange) {
					await editor.edit((editBuilder) => {
						replaceScalarRange(editor.document, editBuilder, usage.valueRange, selected.tierName);
					});
					return;
				}
			}

			await editor.edit((editBuilder) => {
				editBuilder.replace(editor.selection, `"${selected.tierName}"`);
			});
		}),

		vscode.commands.registerCommand('nacho-flow.pickModel', async (args?: RouteTargetArgs) => {
			const editor = vscode.window.activeTextEditor;
			if (!editor || !isNachoConfigDocument(editor.document)) return;

			const fullText = editor.document.getText();
			const index = parseConfigSymbolIndex(fullText);

			const target = resolveRouteTarget(index, args, getActivePosition(editor).line);

			if (!target || !target.provider) {
				vscode.window.showWarningMessage('No tier or fairy dust entry with a configured provider found at cursor.');
				return;
			}

			const provDef = index.providers.get(target.provider);
			const effectiveBaseUrl = resolveEffectiveBaseUrl(provDef);
			if (!provDef || !effectiveBaseUrl) {
				vscode.window.showWarningMessage(`Provider '${target.provider}' has no base_url configured.`);
				return;
			}

			const disc = await discoverModels(effectiveBaseUrl, {
				apiKey: provDef.apiKey,
				providerType: provDef.type
			});
			if (!disc.online || disc.models.length === 0) {
				vscode.window.showErrorMessage(`Failed to discover models from ${effectiveBaseUrl}: ${disc.error || 'No models returned'}`);
				return;
			}

			const items = disc.models.map((m) => ({
				label: `$(symbol-misc) ${m}`,
				description: `Discovered on ${target.provider} (${disc.latencyMs}ms)`,
				modelId: m
			}));

			const selected = await vscode.window.showQuickPick(items, {
				placeHolder: `Select model for ${target.label || 'default_tier'}`
			});

			if (!selected) return;

			if (target.modelValueRange) {
				await editor.edit((editBuilder) => {
					replaceScalarRange(editor.document, editBuilder, target.modelValueRange!, selected.modelId);
				});
			} else {
				await editor.edit((editBuilder) => {
					editBuilder.replace(editor.selection, `"${selected.modelId}"`);
				});
			}
		}),

		vscode.commands.registerCommand('nacho-flow.pingProviderEndpoint', async (baseUrl?: string, providerId?: string) => {
			let provDef: ProviderSymbolDef | undefined;
			const editor = vscode.window.activeTextEditor;
			if (editor && isNachoConfigDocument(editor.document)) {
				const index = parseConfigSymbolIndex(editor.document.getText());
				if (providerId) {
					provDef = index.providers.get(providerId);
				}
				if (!provDef && baseUrl) {
					provDef = Array.from(index.providers.values()).find((p) => p.baseUrl === baseUrl || resolveEffectiveBaseUrl(p) === baseUrl);
				}
				if (!provDef && !baseUrl && !providerId) {
					const firstProv = Array.from(index.providers.values())[0];
					baseUrl = resolveEffectiveBaseUrl(firstProv);
					providerId = firstProv?.id;
					provDef = firstProv;
				}
			}

			if (!baseUrl && provDef) {
				baseUrl = resolveEffectiveBaseUrl(provDef);
			}
			if (!providerId && provDef) {
				providerId = provDef.id;
			}

			if (!baseUrl) {
				vscode.window.showWarningMessage('No provider base_url available to ping.');
				return;
			}

			const disc = await discoverModels(baseUrl, {
				forceRefresh: true,
				apiKey: provDef?.apiKey,
				providerType: provDef?.type
			});
			if (disc.online) {
				const count = disc.models.length;
				vscode.window.showInformationMessage(
					`🔌 [${providerId || 'Provider'}] Online (${disc.latencyMs}ms) — ${count} model${count === 1 ? '' : 's'} available: ${disc.models.slice(0, 3).join(', ')}${count > 3 ? '...' : ''}`
				);
			} else {
				vscode.window.showErrorMessage(
					`🔌 [${providerId || 'Provider'}] Offline: ${disc.error || 'Connection failed'}`
				);
			}
		}),

		vscode.commands.registerCommand('nacho-flow.findProviderReferences', async (providerId?: string) => {
			let editor = vscode.window.activeTextEditor;
			if (!editor || !isNachoConfigDocument(editor.document)) {
				const visible = vscode.window.visibleTextEditors.find((e) => isNachoConfigDocument(e.document));
				if (visible) {
					editor = visible;
				}
			}
			if (!editor) return;

			const index = parseConfigSymbolIndex(editor.document.getText());
			let pId = providerId;
			if (!pId) {
				const activePos = getActivePosition(editor);
				const line = activePos.line;
				for (const [id, def] of index.providers) {
					if (line >= def.blockRange.startLine && line <= def.blockRange.endLine) {
						pId = id;
						break;
					}
				}
			}
			if (!pId) {
				pId = Array.from(index.providers.keys())[0];
			}
			if (!pId) return;

			const provDef = index.providers.get(pId);
			if (!provDef) return;

			const usages = index.providerUsages.filter((u) => u.providerId === pId);
			if (usages.length === 0) {
				vscode.window.showInformationMessage(`Provider '${pId}' is unused (0 references).`);
				return;
			}

			// Interactive reference picker QuickPick
			const items = usages.map((u) => ({
				label: `$(symbol-event) ${u.ownerName || u.context} (Line ${u.valueRange.startLine + 1})`,
				description: `Context: ${u.context}`,
				usage: u
			}));

			if (items.length === 1) {
				const loc = items[0].usage.valueRange;
				const pos = new vscode.Position(loc.startLine, loc.startCharacter);
				editor.selection = new (vscode.Selection || vscode.Range)(pos, new vscode.Position(loc.endLine, loc.endCharacter)) as any;
				if (typeof editor.revealRange === 'function') {
					editor.revealRange(toVsCodeRange(loc), vscode.TextEditorRevealType?.InCenter);
				}
			} else {
				const pick = await vscode.window.showQuickPick(items, {
					placeHolder: `Select reference for provider '${pId}' to jump to:`
				});
				if (pick) {
					const loc = pick.usage.valueRange;
					const pos = new vscode.Position(loc.startLine, loc.startCharacter);
					editor.selection = new (vscode.Selection || vscode.Range)(pos, new vscode.Position(loc.endLine, loc.endCharacter)) as any;
					if (typeof editor.revealRange === 'function') {
						editor.revealRange(toVsCodeRange(loc), vscode.TextEditorRevealType?.InCenter);
					}
				}
			}

			// Also trigger showReferences if available
			const locations = [
				new vscode.Location(editor.document.uri, toVsCodeRange(provDef.idRange)),
				...usages.map((u) => new vscode.Location(editor.document.uri, toVsCodeRange(u.valueRange)))
			];
			const pos = new vscode.Position(provDef.idRange.startLine, provDef.idRange.startCharacter);
			try {
				await vscode.commands.executeCommand('editor.action.showReferences', editor.document.uri, pos, locations);
			} catch (_) {}
		})
	);

	// Wire real-time diagnostics
	if (diagnosticCollection) {
		const updateDiagnostics = (doc: vscode.TextDocument) => {
			if (!isNachoConfigDocument(doc)) return;
			const issues = validateNachoConfigYaml(doc.getText());
			const diagnostics = issues.map((issue) => {
				const range = toVsCodeRange(issue.range);
				const severity = issue.severity === 'Error'
					? vscode.DiagnosticSeverity.Error
					: issue.severity === 'Warning'
					? vscode.DiagnosticSeverity.Warning
					: vscode.DiagnosticSeverity.Information;
				return new vscode.Diagnostic(range, issue.message, severity);
			});
			diagnosticCollection.set(doc.uri, diagnostics);
		};

		disposables.push(
			vscode.workspace.onDidOpenTextDocument(updateDiagnostics),
			vscode.workspace.onDidChangeTextDocument((e) => updateDiagnostics(e.document)),
			vscode.workspace.onDidCloseTextDocument((doc) => diagnosticCollection.delete(doc.uri))
		);

		if (vscode.window.activeTextEditor?.document) {
			updateDiagnostics(vscode.window.activeTextEditor.document);
		}
	}

	const disposableGroup = vscode.Disposable.from(...disposables);
	context.subscriptions.push(disposableGroup);
	return disposableGroup;
}
