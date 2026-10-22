import { SourceRange, parseConfigSymbolIndex } from './config-symbol-index';

export type ValidationSeverity = 'Error' | 'Warning' | 'Information' | 'Hint';

export interface QuickFixAction {
	title: string;
	replacement: string;
	range: SourceRange;
}

export interface YamlValidationIssue {
	ruleId: string;
	message: string;
	severity: ValidationSeverity;
	range: SourceRange;
	quickFixes?: QuickFixAction[];
}

const VALID_PROVIDER_TYPES = new Set(['openai_compatible', 'anthropic', 'openrouter']);

const ALLOWED_REQUEST_CONTEXT_VARS = new Set([
	'Tokens',
	'RawTokens',
	'Retries',
	'IsRetry',
	'MessageCount',
	'HasImages',
	'HasTools',
	'InteractiveTool',
	'Keywords',
	'Prompt',
	'CleanPrompt',
	'HasToolProgress',
	'HasWriteProgress',
	'HasShellWrite',
	'HasTestProgress',
	'HasTestPass',
	'HasTestFail',
	'HasActiveRepair',
	'HasWriteCapability',
	'NoKickstart',
	'NoCycleKiller',
	'NoShield',
	'RawModeEnabled',
	'HistoryErrors',
	'CycleRetries',
	'CycleBreakerTriggered',
	'CycleBreakerReason',
	'CycleContentTokens',
	'CycleThinkingTokens',
	'CycleToolTokens',
	'SessionReadSlapped',
	'SessionReadSlapCount',
	'SessionThoughtSlapCount',
	'ThoughtSlapAttempts',
	'ThoughtSlapOutcome',
	'ThoughtSlapTier',
	'ForcedTier',
	'ForcedModel',
	'IsModelCoolingDown',
	'FairyDusted',
	'FairyDustCount',
	'ClientID',
	'NTSTokensSaved',
	'NTSBytesSaved'
]);

const DSL_KEYWORDS = new Set([
	'true',
	'false',
	'null',
	'nil',
	'and',
	'or',
	'not',
	'in',
	'matches',
	'contains',
	'startsWith',
	'endsWith',
	// expr-lang built-ins
	'any',
	'all',
	'none',
	'one',
	'filter',
	'map',
	'count',
	'len',
	'lower',
	'upper',
	'trim',
	'split'
]);

function maskStringLiterals(expr: string): string {
	const chars = expr.split('');
	let inSingle = false;
	let inDouble = false;

	for (let i = 0; i < chars.length; i++) {
		const c = chars[i];
		if (c === '\\') {
			chars[i] = ' ';
			continue;
		}
		if (c === "'" && !inDouble) {
			inSingle = !inSingle;
			chars[i] = ' ';
			continue;
		}
		if (c === '"' && !inSingle) {
			inDouble = !inDouble;
			chars[i] = ' ';
			continue;
		}
		if (inSingle || inDouble) {
			chars[i] = ' ';
		}
	}
	return chars.join('');
}

function levenshteinDistance(s1: string, s2: string): number {
	const m = s1.length;
	const n = s2.length;
	const dp: number[][] = Array.from({ length: m + 1 }, () => Array(n + 1).fill(0));

	for (let i = 0; i <= m; i++) dp[i][0] = i;
	for (let j = 0; j <= n; j++) dp[0][j] = j;

	for (let i = 1; i <= m; i++) {
		for (let j = 1; j <= n; j++) {
			if (s1[i - 1].toLowerCase() === s2[j - 1].toLowerCase()) {
				dp[i][j] = dp[i - 1][j - 1];
			} else {
				dp[i][j] = 1 + Math.min(dp[i - 1][j], dp[i][j - 1], dp[i - 1][j - 1]);
			}
		}
	}
	return dp[m][n];
}

function findClosestMatch(target: string, candidates: string[]): string | undefined {
	if (candidates.length === 0) return undefined;
	let bestMatch = candidates[0];
	let bestDist = levenshteinDistance(target, bestMatch);

	for (let i = 1; i < candidates.length; i++) {
		const dist = levenshteinDistance(target, candidates[i]);
		if (dist < bestDist) {
			bestDist = dist;
			bestMatch = candidates[i];
		}
	}
	return bestMatch;
}

export function validateNachoConfigYaml(text: string): YamlValidationIssue[] {
	const issues: YamlValidationIssue[] = [];
	if (!text || !text.trim()) {
		return issues;
	}

	const lines = text.split(/\r?\n/);
	const index = parseConfigSymbolIndex(text);

	// 1. SYNTAX_INDENT: Tab indentation & unclosed quotes check
	for (let lineIdx = 0; lineIdx < lines.length; lineIdx++) {
		const rawLine = lines[lineIdx];
		const leadingWhitespace = rawLine.match(/^[ \t]*/)?.[0] || '';
		if (leadingWhitespace.includes('\t')) {
			const tabCol = rawLine.indexOf('\t');
			issues.push({
				ruleId: 'SYNTAX_INDENT',
				message: 'YAML prohibits tab characters for indentation; convert tabs to spaces.',
				severity: 'Error',
				range: {
					startLine: lineIdx,
					startCharacter: tabCol,
					endLine: lineIdx,
					endCharacter: tabCol + 1
				},
				quickFixes: [
					{
						title: 'Convert tabs to spaces',
						replacement: rawLine.replace(/\t/g, '  '),
						range: {
							startLine: lineIdx,
							startCharacter: 0,
							endLine: lineIdx,
							endCharacter: rawLine.length
						}
					}
				]
			});
		}
	}

	// 2. MISSING_PROVIDERS check (Guardrail 1)
	const hasExplicitProvidersKey = lines.some((l) => /^providers\s*:\s*(#.*)?$/.test(l));
	const hasTiers = index.tiers.size > 0 || Boolean(index.defaultTier);

	if (index.providers.size === 0) {
		if (hasExplicitProvidersKey) {
			const pLineIdx = lines.findIndex((l) => /^providers\s*:\s*(#.*)?$/.test(l));
			issues.push({
				ruleId: 'MISSING_PROVIDERS',
				message: "At least one provider must be defined in 'providers'.",
				severity: 'Error',
				range: {
					startLine: pLineIdx !== -1 ? pLineIdx : 0,
					startCharacter: 0,
					endLine: pLineIdx !== -1 ? pLineIdx : 0,
					endCharacter: lines[pLineIdx !== -1 ? pLineIdx : 0].length
				}
			});
		} else if (hasTiers) {
			issues.push({
				ruleId: 'MISSING_PROVIDERS',
				message: "Configuration defines tiers but is missing required 'providers:' block.",
				severity: 'Error',
				range: {
					startLine: 0,
					startCharacter: 0,
					endLine: 0,
					endCharacter: lines[0]?.length || 0
				}
			});
		}
	}

	// 3. Provider validation (type, local, base_url)
	for (const provider of index.providers.values()) {
		if (provider.type === 'local') {
			issues.push({
				ruleId: 'REMOVED_PROVIDER_TYPE',
				message: `provider "${provider.id}": type "local" was removed; use type: openai_compatible with local: true`,
				severity: 'Error',
				range: provider.typeRange || provider.idRange
			});
			continue;
		}
		if (provider.type === 'cloud') {
			issues.push({
				ruleId: 'REMOVED_PROVIDER_TYPE',
				message: `provider "${provider.id}": type "cloud" was removed; use type: openai_compatible, or type: openrouter for OpenRouter`,
				severity: 'Error',
				range: provider.typeRange || provider.idRange
			});
			continue;
		}

		if (!provider.type || !VALID_PROVIDER_TYPES.has(provider.type)) {
			const range = provider.typeRange || provider.idRange;
			issues.push({
				ruleId: 'INVALID_PROVIDER_TYPE',
				message: `provider "${provider.id}": type is required and must be one of openai_compatible, anthropic, openrouter`,
				severity: 'Error',
				range,
				quickFixes: Array.from(VALID_PROVIDER_TYPES).map((t) => ({
					title: `Change to "${t}"`,
					replacement: t,
					range
				}))
			});
			continue;
		}

		if (provider.local && provider.type !== 'openai_compatible') {
			issues.push({
				ruleId: 'INVALID_PROVIDER_LOCAL',
				message: `provider "${provider.id}": local is only valid for type openai_compatible`,
				severity: 'Error',
				range: provider.localRange || provider.idRange
			});
		}

		if (provider.type === 'openai_compatible' && (!provider.baseUrl || !provider.baseUrl.trim())) {
			issues.push({
				ruleId: 'MISSING_BASE_URL',
				message: `provider "${provider.id}": base_url is required for type openai_compatible`,
				severity: 'Error',
				range: provider.idRange,
				quickFixes: [
					{
						title: 'Add base_url: "http://127.0.0.1:8080/v1"',
						replacement: 'http://127.0.0.1:8080/v1',
						range: provider.idRange
					}
				]
			});
		}

		if (provider.baseUrl && provider.baseUrl.trim()) {
			const urlStr = provider.baseUrl.trim();
			try {
				const parsed = new URL(urlStr);
				if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
					issues.push({
						ruleId: 'INVALID_BASE_URL',
						message: `provider "${provider.id}": base_url must be an absolute http(s) URL`,
						severity: 'Error',
						range: provider.baseUrlRange || provider.idRange
					});
				}
			} catch {
				issues.push({
					ruleId: 'INVALID_BASE_URL',
					message: `provider "${provider.id}": base_url must be an absolute http(s) URL`,
					severity: 'Error',
					range: provider.baseUrlRange || provider.idRange
				});
			}
		}
	}

	// 4. UNKNOWN_PROVIDER_REF
	const definedProviderIds = Array.from(index.providers.keys());
	for (const usage of index.providerUsages) {
		if (!index.providers.has(usage.providerId)) {
			const closest = findClosestMatch(usage.providerId, definedProviderIds);
			const quickFixes: QuickFixAction[] = [];
			if (closest) {
				quickFixes.push({
					title: `Change to "${closest}"`,
					replacement: closest,
					range: usage.valueRange
				});
			}

			issues.push({
				ruleId: 'UNKNOWN_PROVIDER_REF',
				message: `Unknown provider reference '${usage.providerId}' in ${usage.context}${usage.ownerName ? ` (${usage.ownerName})` : ''}.`,
				severity: 'Error',
				range: usage.valueRange,
				quickFixes
			});
		}
	}

	// 5. UNKNOWN_RESCUE_TIER
	const definedTierNames = Array.from(index.tiers.keys());
	if (index.defaultTier?.name) {
		definedTierNames.push(index.defaultTier.name);
	}

	for (const rescue of index.rescueTierUsages) {
		const exists = definedTierNames.includes(rescue.tierName);
		if (!exists) {
			const closest = findClosestMatch(rescue.tierName, definedTierNames);
			const quickFixes: QuickFixAction[] = [];
			if (closest) {
				quickFixes.push({
					title: `Change to "${closest}"`,
					replacement: closest,
					range: rescue.valueRange
				});
			}

			issues.push({
				ruleId: 'UNKNOWN_RESCUE_TIER',
				message: `Thought slap rescue_tier references unknown tier '${rescue.tierName}'.`,
				severity: 'Error',
				range: rescue.valueRange,
				quickFixes
			});
		}
	}

	// 6. REMOVED_LEGACY_KEY check
	for (let lineIdx = 0; lineIdx < lines.length; lineIdx++) {
		const rawLine = lines[lineIdx];

		if (rawLine.includes('SessionKickstarted')) {
			const col = rawLine.indexOf('SessionKickstarted');
			issues.push({
				ruleId: 'REMOVED_LEGACY_KEY',
				message: "DSL variable 'SessionKickstarted' was removed in v1.5; rename to 'SessionReadSlapped'.",
				severity: 'Error',
				range: {
					startLine: lineIdx,
					startCharacter: col,
					endLine: lineIdx,
					endCharacter: col + 'SessionKickstarted'.length
				},
				quickFixes: [
					{
						title: "Rename to 'SessionReadSlapped'",
						replacement: 'SessionReadSlapped',
						range: {
							startLine: lineIdx,
							startCharacter: col,
							endLine: lineIdx,
							endCharacter: col + 'SessionKickstarted'.length
						}
					}
				]
			});
		}

		if (rawLine.includes('SessionKickstartCount')) {
			const col = rawLine.indexOf('SessionKickstartCount');
			issues.push({
				ruleId: 'REMOVED_LEGACY_KEY',
				message: "DSL variable 'SessionKickstartCount' was removed in v1.5; rename to 'SessionReadSlapCount'.",
				severity: 'Error',
				range: {
					startLine: lineIdx,
					startCharacter: col,
					endLine: lineIdx,
					endCharacter: col + 'SessionKickstartCount'.length
				},
				quickFixes: [
					{
						title: "Rename to 'SessionReadSlapCount'",
						replacement: 'SessionReadSlapCount',
						range: {
							startLine: lineIdx,
							startCharacter: col,
							endLine: lineIdx,
							endCharacter: col + 'SessionKickstartCount'.length
						}
					}
				]
			});
		}

		if (/^\s*(enabled|threshold|max_count|max_failures|write_only)\s*:\s*/.test(rawLine)) {
			// Check if we are inside kickstart: but not kickstart.read_slap:
			// Look backwards for context
			for (let back = lineIdx - 1; back >= 0; back--) {
				const prev = lines[back];
				if (/^\s*read_slap\s*:\s*/.test(prev) || /^\s*thought_slap\s*:\s*/.test(prev)) {
					break; // legitimately nested
				}
				if (/^kickstart\s*:\s*/.test(prev)) {
					const match = rawLine.match(/^\s*([a-zA-Z_-]+)\s*:/);
					if (match) {
						const k = match[1];
						const col = rawLine.indexOf(k);
						issues.push({
							ruleId: 'REMOVED_LEGACY_KEY',
							message: `Config uses removed key 'kickstart.${k}'; move to 'kickstart.read_slap.${k}'. See CHANGELOG v1.5.`,
							severity: 'Error',
							range: {
								startLine: lineIdx,
								startCharacter: col,
								endLine: lineIdx,
								endCharacter: col + k.length
							}
						});
					}
					break;
				}
				if (/^[a-zA-Z_-]+\s*:\s*/.test(prev)) {
					break; // left kickstart section
				}
			}
		}
	}

	// 7. MISSING_TIER_MODEL, INVALID_WHEN_EXPR & UNREACHABLE_TIER
	let catchAllReached = false;
	let catchAllTierName = '';

	const resolveMissingModelRange = (modelRange: SourceRange | undefined, fallbackRange: SourceRange): SourceRange => {
		if (modelRange) {
			const mLine = modelRange.startLine;
			const rawMLine = lines[mLine] || '';
			const firstNonWs = rawMLine.search(/\S/);
			return {
				startLine: mLine,
				startCharacter: firstNonWs !== -1 ? firstNonWs : modelRange.startCharacter,
				endLine: mLine,
				endCharacter: Math.max(rawMLine.length, modelRange.endCharacter)
			};
		}
		return fallbackRange;
	};

	for (const tier of index.tiers.values()) {
		const tierLabel = tier.name || 'unnamed';
		if (!tier.model || !tier.model.trim()) {
			issues.push({
				ruleId: 'MISSING_TIER_MODEL',
				message: `tier '${tierLabel}' is missing required 'model'`,
				severity: 'Error',
				range: resolveMissingModelRange(tier.modelValueRange, tier.nameValueRange)
			});
		}

		if (catchAllReached) {
			issues.push({
				ruleId: 'UNREACHABLE_TIER',
				message: `Tier '${tierLabel}' is unreachable because prior tier '${catchAllTierName}' is a catch-all (when: true).`,
				severity: 'Warning',
				range: tier.nameValueRange
			});
		}

		if (tier.when) {
			const exprStr = tier.when.trim();
			const whenRange = tier.whenValueRange || tier.nameValueRange;

			if (exprStr === 'true') {
				catchAllReached = true;
				catchAllTierName = tierLabel;
			}

			// Trailing operator check
			if (/(&&|\|\||<|>|<=|>=|==|!=|\+|\-|\*|\/|!)\s*$/.test(exprStr)) {
				issues.push({
					ruleId: 'INVALID_WHEN_EXPR',
					message: `Tier '${tierLabel}' when expression ends with incomplete trailing operator.`,
					severity: 'Error',
					range: whenRange
				});
			}

			// Unbalanced parens
			let parenDepth = 0;
			for (const char of exprStr) {
				if (char === '(') parenDepth++;
				if (char === ')') parenDepth--;
				if (parenDepth < 0) break;
			}
			if (parenDepth !== 0) {
				issues.push({
					ruleId: 'INVALID_WHEN_EXPR',
					message: `Tier '${tierLabel}' when expression has unbalanced parentheses.`,
					severity: 'Error',
					range: whenRange
				});
			}

			// Check identifiers
			const maskedExpr = maskStringLiterals(exprStr);
			const idRegex = /[a-zA-Z_][a-zA-Z0-9_]*/g;
			let match: RegExpExecArray | null;
			while ((match = idRegex.exec(maskedExpr)) !== null) {
				const id = match[0];
				if (!DSL_KEYWORDS.has(id) && !ALLOWED_REQUEST_CONTEXT_VARS.has(id)) {
					const closestVar = findClosestMatch(id, Array.from(ALLOWED_REQUEST_CONTEXT_VARS));
					const quickFixes: QuickFixAction[] = [];
					if (closestVar && whenRange) {
						quickFixes.push({
							title: `Change to "${closestVar}"`,
							replacement: closestVar,
							range: {
								startLine: whenRange.startLine,
								startCharacter: whenRange.startCharacter + match.index,
								endLine: whenRange.startLine,
								endCharacter: whenRange.startCharacter + match.index + id.length
							}
						});
					}

					issues.push({
						ruleId: 'INVALID_WHEN_EXPR',
						message: `Tier '${tierLabel}' when expression references unknown variable '${id}'.`,
						severity: 'Error',
						range: whenRange,
						quickFixes
					});
				}
			}
		}
	}

	if (index.defaultTier) {
		const dt = index.defaultTier;
		if ((dt.name || dt.provider || dt.modelValueRange) && (!dt.model || !dt.model.trim())) {
			const msg = dt.name
				? `default_tier '${dt.name}' is missing required 'model'`
				: `default_tier is missing required 'model'`;
			issues.push({
				ruleId: 'MISSING_TIER_MODEL',
				message: msg,
				severity: 'Error',
				range: resolveMissingModelRange(dt.modelValueRange, dt.nameValueRange || dt.blockRange)
			});
		}
	}

	return issues;
}
