import * as vscode from 'vscode';
import * as path from 'path';
import { parseConfigSymbolIndex, SourceRange } from './config-symbol-index';
import { validateNachoConfigYaml } from './yaml-validator';
import { discoverModels } from './model-discovery';

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
	const base = path.basename(fsPath).toLowerCase();

	if (!base.endsWith('.yaml') && !base.endsWith('.yml')) {
		return false;
	}

	if (/^profile.*\.ya?ml$/i.test(base) || /config.*\.ya?ml$/i.test(base)) {
		return true;
	}

	const normalized = fsPath.replace(/\\/g, '/').toLowerCase();
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

function isPositionInRange(pos: vscode.Position, range: SourceRange): boolean {
	if (pos.line < range.startLine || pos.line > range.endLine) return false;
	if (pos.line === range.startLine && pos.character < range.startCharacter) return false;
	if (pos.line === range.endLine && pos.character > range.endCharacter) return false;
	return true;
}

/**
 * Completion provider for provider IDs, rescue tier names, live models, reasoning effort, and DSL variables.
 */
export class NachoCompletionItemProvider implements vscode.CompletionItemProvider {
	async provideCompletionItems(
		document: vscode.TextDocument,
		position: vscode.Position
	): Promise<vscode.CompletionItem[]> {
		const lineText = document.lineAt(position.line).text;
		const lineUntilCursor = lineText.slice(0, position.character);
		const fullText = document.getText();
		const index = parseConfigSymbolIndex(fullText);

		// 1. provider: completions
		if (/provider\s*:\s*["']?[^"']*$/.test(lineUntilCursor)) {
			const items: vscode.CompletionItem[] = [];
			for (const [id, def] of index.providers) {
				const item = new vscode.CompletionItem(id, vscode.CompletionItemKind.Reference);
				item.detail = `${def.type || 'provider'} • ${def.baseUrl || 'no base url'}`;
				item.documentation = new vscode.MarkdownString(`**Nacho Flow Provider Definition**\n- Base URL: \`${def.baseUrl}\`\n- Type: \`${def.type}\``);
				items.push(item);
			}
			return items;
		}

		// 2. rescue_tier: completions
		if (/rescue_tier\s*:\s*["']?[^"']*$/.test(lineUntilCursor)) {
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
				items.push(item);
			}
			return items;
		}

		// 3. model: completions (with live discovery)
		if (/model\s*:\s*["']?[^"']*$/.test(lineUntilCursor)) {
			// Find tier at or before this line
			let targetProviderId: string | undefined;
			for (const [, tier] of index.tiers) {
				if (position.line >= tier.blockRange.startLine && position.line <= tier.blockRange.endLine) {
					targetProviderId = tier.provider;
					break;
				}
			}
			if (!targetProviderId && index.defaultTier) {
				if (position.line >= index.defaultTier.blockRange.startLine && position.line <= index.defaultTier.blockRange.endLine) {
					targetProviderId = index.defaultTier.provider;
				}
			}

			if (targetProviderId) {
				const provDef = index.providers.get(targetProviderId);
				if (provDef?.baseUrl) {
					try {
						const disc = await discoverModels(provDef.baseUrl);
						if (disc.online && disc.models.length > 0) {
							const quoteMatch = lineUntilCursor.match(/model\s*:\s*(["'])/);
							const alreadyHasQuote = !!quoteMatch;
							return disc.models.map((m) => {
								const label = alreadyHasQuote ? m : `"${m}"`;
								const item = new vscode.CompletionItem(label, vscode.CompletionItemKind.Value);
								item.detail = `Discovered on ${targetProviderId} (${disc.latencyMs}ms)`;
								item.insertText = label;
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
		if (/reasoning_effort\s*:\s*["']?[^"']*$/.test(lineUntilCursor)) {
			return ['low', 'medium', 'high'].map((effort) => {
				const item = new vscode.CompletionItem(effort, vscode.CompletionItemKind.EnumMember);
				item.detail = `Reasoning effort level: ${effort}`;
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
		if (index.defaultTier && isPositionInRange(position, index.defaultTier.nameValueRange)) {
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
					command: 'nacho-flow.pickProvider'
				};
				actions.push(pickAction);
			}

			if (issue.ruleId === 'UNKNOWN_RESCUE_TIER') {
				const pickTierAction = new vscode.CodeAction('💡 Pick from defined tiers...', vscode.CodeActionKind.QuickFix);
				pickTierAction.command = {
					title: 'Pick Rescue Tier',
					command: 'nacho-flow.pickRescueTier'
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
		const fullText = document.getText();
		const index = parseConfigSymbolIndex(fullText);
		const lenses: vscode.CodeLens[] = [];

		// Providers
		for (const [id, def] of index.providers) {
			const range = toVsCodeRange(def.idRange);
			const refCount = index.providerUsages.filter((u) => u.providerId === id).length;
			const refTitle = refCount === 0 ? '⚠️ 0 references (Unused)' : `${refCount} reference${refCount === 1 ? '' : 's'}`;

			lenses.push(
				new vscode.CodeLens(range, {
					title: refTitle,
					command: 'nacho-flow.findProviderReferences',
					arguments: [id]
				})
			);

			if (def.baseUrl) {
				lenses.push(
					new vscode.CodeLens(range, {
						title: '🔌 Ping Endpoint',
						command: 'nacho-flow.pingProviderEndpoint',
						arguments: [def.baseUrl, id]
					})
				);
			}
		}

		// Tiers
		for (const [name, tier] of index.tiers) {
			const range = toVsCodeRange(tier.nameValueRange);

			lenses.push(
				new vscode.CodeLens(range, {
					title: '🔌 Switch Provider',
					command: 'nacho-flow.pickProvider',
					arguments: [{ tierName: name }]
				})
			);

			lenses.push(
				new vscode.CodeLens(range, {
					title: '🧠 Pick Model',
					command: 'nacho-flow.pickModel',
					arguments: [{ tierName: name }]
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
		vscode.commands.registerCommand('nacho-flow.pickProvider', async (args?: { tierName?: string }) => {
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

			const selected = await vscode.window.showQuickPick(items, {
				placeHolder: args?.tierName ? `Select provider for ${args.tierName}` : 'Select a defined provider'
			});

			if (!selected) return;

			// If args.tierName provided, replace provider in that tier
			if (args?.tierName) {
				const tier = index.tiers.get(args.tierName);
				if (tier && tier.providerValueRange) {
					await editor.edit((editBuilder) => {
						editBuilder.replace(toVsCodeRange(tier.providerValueRange!), selected.providerId);
					});
					return;
				}
			}

			// Else insert/replace at cursor position
			await editor.edit((editBuilder) => {
				editBuilder.replace(editor.selection, selected.providerId);
			});
		}),

		vscode.commands.registerCommand('nacho-flow.pickRescueTier', async () => {
			const editor = vscode.window.activeTextEditor;
			if (!editor || !isNachoConfigDocument(editor.document)) return;

			const fullText = editor.document.getText();
			const index = parseConfigSymbolIndex(fullText);
			const tierNames = Array.from(index.tiers.keys());
			if (index.defaultTier?.name) tierNames.push(index.defaultTier.name);

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

			await editor.edit((editBuilder) => {
				editBuilder.replace(editor.selection, `"${selected.tierName}"`);
			});
		}),

		vscode.commands.registerCommand('nacho-flow.pickModel', async (args?: { tierName?: string }) => {
			const editor = vscode.window.activeTextEditor;
			if (!editor || !isNachoConfigDocument(editor.document)) return;

			const fullText = editor.document.getText();
			const index = parseConfigSymbolIndex(fullText);

			let targetTier = args?.tierName ? index.tiers.get(args.tierName) : undefined;
			if (!targetTier) {
				// Find tier under cursor
				const line = editor.selection.active.line;
				for (const [, tier] of index.tiers) {
					if (line >= tier.blockRange.startLine && line <= tier.blockRange.endLine) {
						targetTier = tier;
						break;
					}
				}
			}

			if (!targetTier || !targetTier.provider) {
				vscode.window.showWarningMessage('No tier with a configured provider found at cursor.');
				return;
			}

			const provDef = index.providers.get(targetTier.provider);
			if (!provDef || !provDef.baseUrl) {
				vscode.window.showWarningMessage(`Provider '${targetTier.provider}' has no base_url configured.`);
				return;
			}

			const disc = await discoverModels(provDef.baseUrl);
			if (!disc.online || disc.models.length === 0) {
				vscode.window.showErrorMessage(`Failed to discover models from ${provDef.baseUrl}: ${disc.error || 'No models returned'}`);
				return;
			}

			const items = disc.models.map((m) => ({
				label: `$(symbol-misc) ${m}`,
				description: `Discovered on ${targetTier!.provider} (${disc.latencyMs}ms)`,
				modelId: m
			}));

			const selected = await vscode.window.showQuickPick(items, {
				placeHolder: `Select model for ${targetTier.name}`
			});

			if (!selected) return;

			if (targetTier.modelValueRange) {
				await editor.edit((editBuilder) => {
					editBuilder.replace(toVsCodeRange(targetTier!.modelValueRange!), `"${selected.modelId}"`);
				});
			} else {
				await editor.edit((editBuilder) => {
					editBuilder.replace(editor.selection, `"${selected.modelId}"`);
				});
			}
		}),

		vscode.commands.registerCommand('nacho-flow.pingProviderEndpoint', async (baseUrl?: string, providerId?: string) => {
			if (!baseUrl) {
				const editor = vscode.window.activeTextEditor;
				if (editor && isNachoConfigDocument(editor.document)) {
					const index = parseConfigSymbolIndex(editor.document.getText());
					const firstProv = Array.from(index.providers.values())[0];
					baseUrl = firstProv?.baseUrl;
					providerId = firstProv?.id;
				}
			}

			if (!baseUrl) {
				vscode.window.showWarningMessage('No provider base_url available to ping.');
				return;
			}

			const disc = await discoverModels(baseUrl, { forceRefresh: true });
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
