/**
 * ConfigSymbolIndex: Relational symbol graph parser for Nacho Flow YAML configurations.
 * Extracts definitions (providers, tiers), cross-references, and exact source ranges
 * for diagnostics, autocomplete, QuickPicks, F12 navigation, and F2 renaming.
 * Zero external dependencies.
 */

export interface SourceRange {
	startLine: number;
	startCharacter: number;
	endLine: number;
	endCharacter: number;
}

export interface ProviderSymbolDef {
	id: string;
	idRange: SourceRange;
	blockRange: SourceRange;
	type?: string;
	typeRange?: SourceRange;
	baseUrl?: string;
	baseUrlRange?: SourceRange;
	apiKey?: string;
}

export interface TierSymbolDef {
	name?: string;
	nameValueRange: SourceRange;
	blockRange: SourceRange;
	provider?: string;
	providerValueRange?: SourceRange;
	model?: string;
	modelValueRange?: SourceRange;
	reasoningEffort?: string;
	when?: string;
	whenValueRange?: SourceRange;
	isDefaultTier: boolean;
}

export interface ProviderUsageRef {
	providerId: string;
	valueRange: SourceRange;
	context: 'tier' | 'default_tier' | 'fairy_dust';
	ownerName?: string;
}

export interface RescueTierUsageRef {
	tierName: string;
	valueRange: SourceRange;
	context: 'global_thought_slap' | 'tier_thought_slap';
	ownerTierName?: string;
}

export interface FairyDustEntrySymbolDef {
	name?: string;
	nameValueRange?: SourceRange;
	blockRange: SourceRange;
	provider?: string;
	providerValueRange?: SourceRange;
	model?: string;
	modelValueRange?: SourceRange;
}

export interface ConfigSymbolIndex {
	providers: Map<string, ProviderSymbolDef>;
	tiers: Map<string, TierSymbolDef>;
	defaultTier?: TierSymbolDef;
	fairyDustEntries: Map<string, FairyDustEntrySymbolDef>;
	providerUsages: ProviderUsageRef[];
	rescueTierUsages: RescueTierUsageRef[];
	getRenameEdits(oldName: string, newName?: string): SourceRange[];
}

/**
 * Extracts inner value and precise SourceRange, stripping surrounding quotes
 * and trailing `#` comments.
 */
export function extractInnerValueRange(
	lineText: string,
	lineNum: number,
	valStartIndex: number
): { value: string; range: SourceRange } {
	// Find end before trailing comment (accounting for quotes)
	let inQuote: '"' | "'" | null = null;
	let commentIndex = -1;

	for (let i = valStartIndex; i < lineText.length; i++) {
		const char = lineText[i];
		if (!inQuote && (char === '"' || char === "'")) {
			inQuote = char;
		} else if (inQuote && char === inQuote) {
			inQuote = null;
		} else if (!inQuote && char === '#') {
			commentIndex = i;
			break;
		}
	}

	const rawValSubstring = commentIndex !== -1
		? lineText.slice(valStartIndex, commentIndex)
		: lineText.slice(valStartIndex);

	// Strip leading whitespace
	const leadingWs = rawValSubstring.match(/^\s*/)?.[0].length || 0;
	// Strip trailing whitespace
	const trailingWs = rawValSubstring.match(/\s*$/)?.[0].length || 0;

	let startCol = valStartIndex + leadingWs;
	let endCol = valStartIndex + rawValSubstring.length - trailingWs;

	let innerText = lineText.slice(startCol, endCol);

	// Strip surrounding quotes
	if (
		(innerText.startsWith('"') && innerText.endsWith('"') && innerText.length >= 2) ||
		(innerText.startsWith("'") && innerText.endsWith("'") && innerText.length >= 2)
	) {
		startCol += 1;
		endCol -= 1;
		innerText = innerText.slice(1, -1);
	}

	return {
		value: innerText,
		range: {
			startLine: lineNum,
			startCharacter: startCol,
			endLine: lineNum,
			endCharacter: endCol
		}
	};
}

/**
 * Parses raw YAML text into a complete ConfigSymbolIndex.
 */
export function parseConfigSymbolIndex(text: string): ConfigSymbolIndex {
	const providers = new Map<string, ProviderSymbolDef>();
	const tiers = new Map<string, TierSymbolDef>();
	let defaultTier: TierSymbolDef | undefined;
	const fairyDustEntries = new Map<string, FairyDustEntrySymbolDef>();
	const providerUsages: ProviderUsageRef[] = [];
	const rescueTierUsages: RescueTierUsageRef[] = [];

	if (!text || !text.trim()) {
		return {
			providers,
			tiers,
			defaultTier,
			fairyDustEntries,
			providerUsages,
			rescueTierUsages,
			getRenameEdits: () => []
		};
	}

	const lines = text.split(/\r?\n/);

	type TopSection = 'none' | 'providers' | 'tiers' | 'default_tier' | 'kickstart' | 'fairy_dust' | 'other';
	let currentSection: TopSection = 'none';

	let currentProviderDef: Partial<ProviderSymbolDef> | null = null;
	let currentTierDef: Partial<TierSymbolDef> | null = null;
	let currentFairyDustEntry: Partial<FairyDustEntrySymbolDef> | null = null;
	let fairyDustEntryCounter = 0;
	let currentTierOwnerName = '';
	let currentFairyDustOwnerName = '';

	const finishCurrentProvider = (endLine: number) => {
		if (currentProviderDef && currentProviderDef.id) {
			currentProviderDef.blockRange = {
				startLine: currentProviderDef.blockRange?.startLine ?? currentProviderDef.idRange!.startLine,
				startCharacter: 0,
				endLine,
				endCharacter: (lines[endLine] || '').length
			};
			providers.set(currentProviderDef.id, currentProviderDef as ProviderSymbolDef);
			currentProviderDef = null;
		}
	};

	const finishCurrentTier = (endLine: number) => {
		if (currentTierDef && (currentTierDef.name || currentTierDef.isDefaultTier)) {
			currentTierDef.blockRange = {
				startLine: currentTierDef.blockRange?.startLine ?? currentTierDef.nameValueRange!.startLine,
				startCharacter: 0,
				endLine,
				endCharacter: (lines[endLine] || '').length
			};
			if (currentTierDef.isDefaultTier) {
				defaultTier = currentTierDef as TierSymbolDef;
			} else if (currentTierDef.name) {
				tiers.set(currentTierDef.name, currentTierDef as TierSymbolDef);
			}
			currentTierDef = null;
		}
	};

	const finishCurrentFairyDustEntry = (endLine: number) => {
		if (currentFairyDustEntry) {
			currentFairyDustEntry.blockRange = {
				startLine: currentFairyDustEntry.blockRange?.startLine ?? (currentFairyDustEntry.nameValueRange ? currentFairyDustEntry.nameValueRange.startLine : 0),
				startCharacter: 0,
				endLine,
				endCharacter: (lines[endLine] || '').length
			};
			const key = currentFairyDustEntry.name || `entry_${++fairyDustEntryCounter}`;
			fairyDustEntries.set(key, currentFairyDustEntry as FairyDustEntrySymbolDef);
			currentFairyDustEntry = null;
		}
	};

	for (let lineIdx = 0; lineIdx < lines.length; lineIdx++) {
		const rawLine = lines[lineIdx];
		const trimmed = rawLine.trim();

		// Check for top-level keys (no indent)
		const topKeyMatch = rawLine.match(/^([a-zA-Z0-9_-]+)\s*:/);
		if (topKeyMatch) {
			finishCurrentProvider(Math.max(0, lineIdx - 1));
			finishCurrentTier(Math.max(0, lineIdx - 1));
			finishCurrentFairyDustEntry(Math.max(0, lineIdx - 1));

			const key = topKeyMatch[1];
			if (key === 'providers') currentSection = 'providers';
			else if (key === 'tiers') currentSection = 'tiers';
			else if (key === 'default_tier') {
				currentSection = 'default_tier';
				const keyStart = rawLine.indexOf('default_tier');
				currentTierDef = {
					nameValueRange: {
						startLine: lineIdx,
						startCharacter: keyStart >= 0 ? keyStart : 0,
						endLine: lineIdx,
						endCharacter: keyStart >= 0 ? keyStart + 'default_tier'.length : rawLine.length
					},
					isDefaultTier: true,
					blockRange: { startLine: lineIdx, startCharacter: 0, endLine: lineIdx, endCharacter: rawLine.length }
				};
			} else if (key === 'kickstart') currentSection = 'kickstart';
			else if (key === 'fairy_dust') currentSection = 'fairy_dust';
			else currentSection = 'other';

			continue;
		}

		if (currentSection === 'providers') {
			// Sub-key in providers: e.g. "  llamacpp_local:"
			const provHeaderMatch = rawLine.match(/^(\s{2}|\t)([a-zA-Z0-9_-]+)\s*:/);
			if (provHeaderMatch) {
				finishCurrentProvider(Math.max(0, lineIdx - 1));
				const pId = provHeaderMatch[2];
				const startCol = rawLine.indexOf(pId);
				currentProviderDef = {
					id: pId,
					idRange: {
						startLine: lineIdx,
						startCharacter: startCol,
						endLine: lineIdx,
						endCharacter: startCol + pId.length
					},
					blockRange: {
						startLine: lineIdx,
						startCharacter: 0,
						endLine: lineIdx,
						endCharacter: rawLine.length
					}
				};
				continue;
			}

			if (currentProviderDef) {
				const baseUrlMatch = rawLine.match(/base_url\s*:\s*(.*)/);
				if (baseUrlMatch) {
					const valStart = rawLine.indexOf(baseUrlMatch[1]);
					const { value, range } = extractInnerValueRange(rawLine, lineIdx, valStart);
					currentProviderDef.baseUrl = value;
					currentProviderDef.baseUrlRange = range;
				}

				const typeMatch = rawLine.match(/type\s*:\s*(.*)/);
				if (typeMatch) {
					const valStart = rawLine.indexOf(typeMatch[1]);
					const { value, range } = extractInnerValueRange(rawLine, lineIdx, valStart);
					currentProviderDef.type = value;
					currentProviderDef.typeRange = range;
				}

				const apiKeyMatch = rawLine.match(/api_key\s*:\s*(.*)/);
				if (apiKeyMatch) {
					const valStart = rawLine.indexOf(apiKeyMatch[1]);
					const { value } = extractInnerValueRange(rawLine, lineIdx, valStart);
					currentProviderDef.apiKey = value;
				}
			}
		} else if (currentSection === 'tiers') {
			// Tier item start: e.g. "  - name: ..." or "  -"
			const tierItemMatch = rawLine.match(/^(\s{2}|\t)-\s*(.*)/);
			if (tierItemMatch) {
				finishCurrentTier(Math.max(0, lineIdx - 1));

				// Look back for leading comment lines
				let blockStartLine = lineIdx;
				for (let backIdx = lineIdx - 1; backIdx >= 0; backIdx--) {
					const prevTrimmed = lines[backIdx].trim();
					if (prevTrimmed.startsWith('#')) {
						blockStartLine = backIdx;
					} else {
						break;
					}
				}

				currentTierDef = {
					isDefaultTier: false,
					blockRange: {
						startLine: blockStartLine,
						startCharacter: 0,
						endLine: lineIdx,
						endCharacter: rawLine.length
					}
				};

				const restOfLine = tierItemMatch[2];
				if (restOfLine.startsWith('name:')) {
					const valPart = restOfLine.slice(5);
					const valStart = rawLine.indexOf(valPart);
					const { value, range } = extractInnerValueRange(rawLine, lineIdx, valStart);
					currentTierDef.name = value;
					currentTierDef.nameValueRange = range;
					currentTierOwnerName = value;
				}
				continue;
			}

			if (currentTierDef) {
				const nameMatch = rawLine.match(/^\s+name\s*:\s*(.*)/);
				if (nameMatch && !currentTierDef.name) {
					const valStart = rawLine.indexOf(nameMatch[1]);
					const { value, range } = extractInnerValueRange(rawLine, lineIdx, valStart);
					currentTierDef.name = value;
					currentTierDef.nameValueRange = range;
					currentTierOwnerName = value;
				}

				const provMatch = rawLine.match(/^\s+provider\s*:\s*(.*)/);
				if (provMatch) {
					const valStart = rawLine.indexOf(provMatch[1]);
					const { value, range } = extractInnerValueRange(rawLine, lineIdx, valStart);
					currentTierDef.provider = value;
					currentTierDef.providerValueRange = range;
					providerUsages.push({
						providerId: value,
						valueRange: range,
						context: 'tier',
						ownerName: currentTierOwnerName || currentTierDef.name
					});
				}

				const modelMatch = rawLine.match(/^\s+model\s*:\s*(.*)/);
				if (modelMatch) {
					const valStart = rawLine.indexOf(modelMatch[1]);
					const { value, range } = extractInnerValueRange(rawLine, lineIdx, valStart);
					currentTierDef.model = value;
					currentTierDef.modelValueRange = range;
				}

				const effortMatch = rawLine.match(/^\s+reasoning_effort\s*:\s*(.*)/);
				if (effortMatch) {
					const valStart = rawLine.indexOf(effortMatch[1]);
					const { value } = extractInnerValueRange(rawLine, lineIdx, valStart);
					currentTierDef.reasoningEffort = value;
				}

				const whenMatch = rawLine.match(/^\s+when\s*:\s*(.*)/);
				if (whenMatch) {
					const valStart = rawLine.indexOf(whenMatch[1]);
					const { value, range } = extractInnerValueRange(rawLine, lineIdx, valStart);
					currentTierDef.when = value;
					currentTierDef.whenValueRange = range;
				}

				const rescueTierMatch = rawLine.match(/^\s+rescue_tier\s*:\s*(.*)/);
				if (rescueTierMatch) {
					const valStart = rawLine.indexOf(rescueTierMatch[1]);
					const { value, range } = extractInnerValueRange(rawLine, lineIdx, valStart);
					rescueTierUsages.push({
						tierName: value,
						valueRange: range,
						context: 'tier_thought_slap',
						ownerTierName: currentTierOwnerName || currentTierDef.name
					});
				}
			}
		} else if (currentSection === 'default_tier') {
			if (currentTierDef) {
				const nameMatch = rawLine.match(/^\s+name\s*:\s*(.*)/);
				if (nameMatch) {
					const valStart = rawLine.indexOf(nameMatch[1]);
					const { value, range } = extractInnerValueRange(rawLine, lineIdx, valStart);
					currentTierDef.name = value;
					currentTierDef.nameValueRange = range;
				}

				const provMatch = rawLine.match(/^\s+provider\s*:\s*(.*)/);
				if (provMatch) {
					const valStart = rawLine.indexOf(provMatch[1]);
					const { value, range } = extractInnerValueRange(rawLine, lineIdx, valStart);
					currentTierDef.provider = value;
					currentTierDef.providerValueRange = range;
					providerUsages.push({
						providerId: value,
						valueRange: range,
						context: 'default_tier',
						ownerName: currentTierDef.name || 'default_tier'
					});
				}

				const modelMatch = rawLine.match(/^\s+model\s*:\s*(.*)/);
				if (modelMatch) {
					const valStart = rawLine.indexOf(modelMatch[1]);
					const { value, range } = extractInnerValueRange(rawLine, lineIdx, valStart);
					currentTierDef.model = value;
					currentTierDef.modelValueRange = range;
				}

				const effortMatch = rawLine.match(/^\s+reasoning_effort\s*:\s*(.*)/);
				if (effortMatch) {
					const valStart = rawLine.indexOf(effortMatch[1]);
					const { value } = extractInnerValueRange(rawLine, lineIdx, valStart);
					currentTierDef.reasoningEffort = value;
				}

				const rescueTierMatch = rawLine.match(/^\s+rescue_tier\s*:\s*(.*)/);
				if (rescueTierMatch) {
					const valStart = rawLine.indexOf(rescueTierMatch[1]);
					const { value, range } = extractInnerValueRange(rawLine, lineIdx, valStart);
					rescueTierUsages.push({
						tierName: value,
						valueRange: range,
						context: 'tier_thought_slap',
						ownerTierName: currentTierDef.name || 'Default Catch-All'
					});
				}
			}
		} else if (currentSection === 'kickstart') {
			const rescueTierMatch = rawLine.match(/^\s+rescue_tier\s*:\s*(.*)/);
			if (rescueTierMatch) {
				const valStart = rawLine.indexOf(rescueTierMatch[1]);
				const { value, range } = extractInnerValueRange(rawLine, lineIdx, valStart);
				rescueTierUsages.push({
					tierName: value,
					valueRange: range,
					context: 'global_thought_slap'
				});
			}
		} else if (currentSection === 'fairy_dust') {
			const fdItemMatch = rawLine.match(/^(\s{4}|\t\t)-\s*(.*)/);
			if (fdItemMatch) {
				finishCurrentFairyDustEntry(Math.max(0, lineIdx - 1));

				let blockStartLine = lineIdx;
				for (let backIdx = lineIdx - 1; backIdx >= 0; backIdx--) {
					const prevTrimmed = lines[backIdx].trim();
					if (prevTrimmed.startsWith('#')) {
						blockStartLine = backIdx;
					} else {
						break;
					}
				}

				const dashCol = rawLine.indexOf('-');
				currentFairyDustEntry = {
					nameValueRange: {
						startLine: lineIdx,
						startCharacter: dashCol >= 0 ? dashCol : 0,
						endLine: lineIdx,
						endCharacter: rawLine.length
					},
					blockRange: {
						startLine: blockStartLine,
						startCharacter: 0,
						endLine: lineIdx,
						endCharacter: rawLine.length
					}
				};

				const restOfLine = fdItemMatch[2];
				const nameOnFirstLine = restOfLine.match(/^name\s*:\s*(.*)/);
				if (nameOnFirstLine) {
					const valStart = rawLine.indexOf(nameOnFirstLine[1]);
					const { value, range } = extractInnerValueRange(rawLine, lineIdx, valStart);
					currentFairyDustEntry.name = value;
					currentFairyDustEntry.nameValueRange = range;
					currentFairyDustOwnerName = value;
				}
			}

			const nameMatch = rawLine.match(/^(?:\s{6}|\t{3})name\s*:\s*(.*)/);
			if (nameMatch && currentFairyDustEntry && !currentFairyDustEntry.name) {
				const valStart = rawLine.indexOf(nameMatch[1]);
				const { value, range } = extractInnerValueRange(rawLine, lineIdx, valStart);
				currentFairyDustEntry.name = value;
				currentFairyDustEntry.nameValueRange = range;
				currentFairyDustOwnerName = value;
			}

			const provMatch = rawLine.match(/^(?:\s{6}|\t{3}|(?:\s{4}|\t\t)-\s*)provider\s*:\s*(.*)/);
			if (provMatch) {
				const valStart = rawLine.indexOf(provMatch[1]);
				const { value, range } = extractInnerValueRange(rawLine, lineIdx, valStart);
				if (currentFairyDustEntry) {
					currentFairyDustEntry.provider = value;
					currentFairyDustEntry.providerValueRange = range;
				}
				providerUsages.push({
					providerId: value,
					valueRange: range,
					context: 'fairy_dust',
					ownerName: currentFairyDustEntry?.name || currentFairyDustOwnerName || 'Fairy Dust Entry'
				});
			}

			const modelMatch = rawLine.match(/^(?:\s{6}|\t{3}|(?:\s{4}|\t\t)-\s*)model\s*:\s*(.*)/);
			if (modelMatch && currentFairyDustEntry) {
				const valStart = rawLine.indexOf(modelMatch[1]);
				const { value, range } = extractInnerValueRange(rawLine, lineIdx, valStart);
				currentFairyDustEntry.model = value;
				currentFairyDustEntry.modelValueRange = range;
			}
		}
	}

	finishCurrentProvider(lines.length - 1);
	finishCurrentTier(lines.length - 1);
	finishCurrentFairyDustEntry(lines.length - 1);

	const index: ConfigSymbolIndex = {
		providers,
		tiers,
		defaultTier,
		fairyDustEntries,
		providerUsages,
		rescueTierUsages,
		getRenameEdits: (oldName: string) => {
			const ranges: SourceRange[] = [];
			const prov = providers.get(oldName);
			if (prov) {
				ranges.push(prov.idRange);
				for (const u of providerUsages) {
					if (u.providerId === oldName) {
						ranges.push(u.valueRange);
					}
				}
				return ranges;
			}
			const tier = tiers.get(oldName) || (defaultTier?.name === oldName ? defaultTier : undefined);
			if (tier) {
				ranges.push(tier.nameValueRange);
				for (const r of rescueTierUsages) {
					if (r.tierName === oldName) {
						ranges.push(r.valueRange);
					}
				}
				return ranges;
			}
			return ranges;
		}
	};

	return index;
}
