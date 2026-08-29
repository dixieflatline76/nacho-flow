import * as vscode from 'vscode';
import * as http from 'http';
import * as fs from 'fs';
import * as os from 'os';
import * as path from 'path';
import { AuthManager } from './config/auth-manager';
import { RestClient } from './api/client';
import { SSEClient } from './sse/client';
import { StatusBarManager } from '../ui/status-bar/item';
import { DashboardPanel } from '../ui/webview/dashboard';
import { SidebarViewProvider } from '../ui/sidebar/sidebar-view-provider';
import { ProcessManager, ParsedStartupError } from './process-manager';
import { TelemetryPoller, RefreshIntervalSeconds } from './telemetry-poller';

export class ExtensionController {
	private context: vscode.ExtensionContext;
	private authManager: AuthManager;
	private restClient: RestClient | null = null;
	private sseClient: SSEClient | null = null;
	private statusBar: StatusBarManager;
	private dashboardPanel: DashboardPanel | null = null;
	private sidebarProvider: SidebarViewProvider | null = null;
	private outputChannel: vscode.OutputChannel | null = null;
	private processManager!: ProcessManager;
	private telemetryPoller: TelemetryPoller | null = null;
	private activeTimeWindow: string = 'all_time';
	private routesRefreshInterval: RefreshIntervalSeconds = 60;

	constructor(context: vscode.ExtensionContext) {
		this.context = context;
		this.authManager = new AuthManager(context);
		this.statusBar = new StatusBarManager();
		this.processManager = new ProcessManager(context.extensionUri, (this.outputChannel as any) || { append: () => {}, appendLine: () => {} });
	}

	public async initialize(): Promise<void> {
		// Load persisted preferences
		this.activeTimeWindow = this.context.globalState?.get<string>('nachoFlow_timeWindow', 'all_time') || 'all_time';
		this.statusBar.setTimeWindow(this.activeTimeWindow);

		const savedInterval = this.context.globalState?.get<number>('nachoFlow_routesRefreshInterval');
		this.routesRefreshInterval = (typeof savedInterval !== 'undefined' ? savedInterval : 60) as RefreshIntervalSeconds;

		// Create Output Channel for live engine logs
		this.outputChannel = vscode.window.createOutputChannel('Nacho Flow Routing Engine');
		this.context.subscriptions.push(this.outputChannel);
		this.processManager = new ProcessManager(this.context.extensionUri, this.outputChannel);

		// Register commands
		this.registerCommands();

		// Register Activity Bar Sidebar View Provider
		this.registerSidebarProvider();

		// Auto-sync configuration to daemon when config is saved
		this.context.subscriptions.push(
			vscode.workspace.onDidSaveTextDocument(async (doc) => {
				const isConfigDoc = (this.activeConfigDocUri && doc.uri.toString() === this.activeConfigDocUri.toString()) ||
					doc.fileName.endsWith('config.yaml');
				if (isConfigDoc && this.restClient) {
					try {
						await this.restClient.updateConfigYaml(doc.getText());
						this.showTransientToast('🌮 Nacho Flow: Configuration updated and hot-reloaded!');
						await this.loadDashboardData();
						await this.syncSidebarState();
					} catch (err: any) {
						vscode.window.showErrorMessage(`Nacho Flow: Failed to update config: ${err.message || err}`);
					}
				}
			})
		);

		// Initialize clients
		await this.initializeClients();

		// Start periodic updates
		this.startPeriodicUpdates();

		// Initial sync and stats update
		await this.syncSidebarState();
		await this.updateStats();
	}

	private registerCommands(): void {
		this.context.subscriptions.push(
			vscode.commands.registerCommand('nacho-flow.showDashboard', () => {
				this.showDashboard();
			}),
			
			vscode.commands.registerCommand('nacho-flow.refreshStats', () => {
				this.updateStats();
			}),
			
			vscode.commands.registerCommand('nacho-flow.openConfig', () => {
				this.showConfigEditor();
			}),

			vscode.commands.registerCommand('nacho-flow.resetCircuit', () => {
				this.resetCircuit();
			}),

			vscode.commands.registerCommand('nacho-flow.setAuthToken', async () => {
				const token = await vscode.window.showInputBox({
					prompt: 'Enter Nacho Flow Bearer Auth Token (leave blank to clear)',
					password: true,
					ignoreFocusOut: true
				});
				if (token !== undefined) {
					if (token.trim() === '') {
						await this.authManager.deleteAuthToken();
						vscode.window.showInformationMessage('Nacho Flow: Auth token cleared');
					} else {
						await this.authManager.setAuthToken(token.trim());
						vscode.window.showInformationMessage('Nacho Flow: Auth token saved securely');
					}
					await this.initializeClients();
					await this.updateStats();
				}
			}),

			vscode.commands.registerCommand('nacho-flow.runOptimizer', () => {
				this.runOptimizer();
			}),

			vscode.commands.registerCommand('nacho-flow.refreshDeals', () => {
				this.refreshDeals(true);
			}),

			vscode.commands.registerCommand('nacho-flow.openSettings', () => {
				this.openSettings();
			}),

			vscode.commands.registerCommand('nacho-flow.setTimeWindowToday', () => {
				this.setTimeWindow('today');
			}),

			vscode.commands.registerCommand('nacho-flow.setTimeWindowWeek', () => {
				this.setTimeWindow('this_week');
			}),

			vscode.commands.registerCommand('nacho-flow.setTimeWindowMonth', () => {
				this.setTimeWindow('this_month');
			}),

			vscode.commands.registerCommand('nacho-flow.setTimeWindowAllTime', () => {
				this.setTimeWindow('all_time');
			}),

			vscode.commands.registerCommand('nacho-flow.openDocs', () => {
				vscode.env.openExternal(vscode.Uri.parse('https://spicebox.dev/nacho-flow/docs.html?doc=user_guide'));
			}),

			vscode.commands.registerCommand('nacho-flow.openSupport', () => {
				vscode.env.openExternal(vscode.Uri.parse('https://spicebox.dev/nacho-flow/support.html'));
			})
		);
	}

	public async openSettings(): Promise<void> {
		await vscode.commands.executeCommand('nacho-flow.sidebarView.focus');
	}

	private registerSidebarProvider(): void {
		this.sidebarProvider = new SidebarViewProvider(
			this.context.extensionUri,
			async (message) => {
				switch (message.command) {
					case 'initialize':
					case 'refreshAll':
						await this.syncSidebarState();
						break;
					case 'setEngineMode': {
						const mode = message.mode === 'remote' ? 'remote' : 'local';
						await this.authManager.setEngineMode(mode);
						await this.initializeClients();
						await this.syncSidebarState();
						await this.updateStats();
						await this.loadDashboardData();
						this.showTransientToast(`🌮 Nacho Flow: Switched to ${mode === 'local' ? 'Local Engine' : 'Remote Server'}`);
						break;
					}
					case 'startEngine': {
						const daemonUrl = await this.authManager.getBaseUrl();
						if (!this.processManager.isLocalUrl(daemonUrl)) {
							vscode.window.showWarningMessage('Nacho Flow: Cannot start remote engine. Please ensure the daemon is running on its host server.');
							break;
						}
						if (this.sidebarProvider) {
							this.sidebarProvider.updateEngineStatus({ starting: true });
						}
						this.showTransientToast('▶️ Nacho Flow: Starting Routing Engine...');
						const result = await this.processManager.start(daemonUrl);
						if (!result.success && result.error) {
							await this.handleEngineStartError(result);
						}
						await this.initializeClients();
						await this.syncSidebarState();
						await this.updateStats();
						await this.loadDashboardData();
						break;
					}
					case 'restartEngine': {
						const daemonUrl = await this.authManager.getBaseUrl();
						if (!this.processManager.isLocalUrl(daemonUrl)) {
							vscode.window.showWarningMessage('Nacho Flow: Cannot restart remote engine.');
							break;
						}
						if (this.sidebarProvider) {
							this.sidebarProvider.updateEngineStatus({ starting: true });
						}
						this.showTransientToast('🔄 Nacho Flow: Restarting Routing Engine...');
						const result = await this.processManager.restart(daemonUrl);
						if (!result.success && result.error) {
							await this.handleEngineStartError(result);
						}
						await this.initializeClients();
						await this.syncSidebarState();
						await this.updateStats();
						await this.loadDashboardData();
						break;
					}
					case 'stopEngine': {
						const daemonUrl = await this.authManager.getBaseUrl();
						if (!this.processManager.isLocalUrl(daemonUrl)) {
							vscode.window.showWarningMessage('Nacho Flow: Cannot stop remote engine.');
							break;
						}
						await this.processManager.stop();
						this.showTransientToast('⏹️ Nacho Flow: Routing Engine stopped');
						if (this.sidebarProvider) {
							this.sidebarProvider.updateEngineStatus({ connected: false, error: 'Stopped by user' });
						}
						this.statusBar.updateStats(null);
						break;
					}
					case 'openLogs':
						if (this.outputChannel) {
							this.outputChannel.show();
						}
						break;
					case 'testConnection':
						await this.handleTestConnection(message.url, message.token);
						break;
					case 'saveEngineSettings':
						await this.handleSaveSettings(message.url, message.token);
						await this.syncSidebarState();
						break;
					case 'editConfig':
						await this.showConfigEditor();
						break;
					case 'saveOpenRouterKey':
						if (message.key) {
							await this.authManager.setAuthToken(message.key.trim());
							this.showTransientToast('🔑 OpenRouter API Key saved securely!');
							await this.syncSidebarState();
						}
						break;
					case 'copyToClipboard':
						if (message.text) {
							await vscode.env.clipboard.writeText(message.text);
							this.showTransientToast(`📋 Copied ${message.label || 'text'} to clipboard!`);
						}
						break;
					case 'copyActiveToken': {
						const token = await this.authManager.getAuthToken();
						if (token) {
							await vscode.env.clipboard.writeText(token);
							this.showTransientToast('📋 Nacho Flow: Auth token copied to clipboard!');
						} else {
							this.showTransientToast('ℹ️ Nacho Flow: No auth token configured (auth disabled)');
						}
						break;
					}
					case 'openExternalUrl':
						if (message.url) {
							await vscode.env.openExternal(vscode.Uri.parse(message.url));
						}
						break;
					case 'openDocs':
						await vscode.env.openExternal(vscode.Uri.parse('https://spicebox.dev/nacho-flow/docs.html?doc=user_guide'));
						break;
					case 'openSupport':
						await vscode.env.openExternal(vscode.Uri.parse('https://spicebox.dev/nacho-flow/support.html'));
						break;
					case 'openMarketplace':
						if (message.extensionId) {
							await vscode.commands.executeCommand('workbench.extensions.search', message.extensionId);
						}
						break;
					case 'recalculateStats':
						await this.handleRecalculateStats();
						break;
					case 'resetStats':
						await this.handleResetStats();
						break;
					case 'resetCircuits':
						if (this.restClient) {
							await this.restClient.resetCircuit();
							this.showTransientToast('🔌 All circuit breakers reset!');
						}
						break;
					case 'openDashboard':
						this.showDashboard();
						break;
				}
			}
		);

		this.context.subscriptions.push(
			vscode.window.registerWebviewViewProvider(
				SidebarViewProvider.viewType,
				this.sidebarProvider
			)
		);
	}

	public async syncSidebarState(): Promise<void> {
		if (!this.sidebarProvider) return;

		const baseUrl = await this.authManager.getBaseUrl();
		const isRemote = baseUrl !== 'http://127.0.0.1:8000' && baseUrl !== 'http://localhost:8000';
		const token = await this.authManager.getAuthToken();

		let engineStatus: any = { connected: false, version: '', error: 'Offline' };
		const providers: any[] = [];

		if (this.restClient) {
			try {
				const health = await this.restClient.getHealth();
				engineStatus = { connected: true, version: health?.version || 'Online', error: '' };
			} catch (err: any) {
				engineStatus = { connected: false, version: '', error: err.message || 'Connection refused' };
			}

			try {
				const config = await this.restClient.getConfig();
				const circuits = await this.restClient.getCircuits();
				const circuitList = circuits?.circuits || [];

				if (config?.providers) {
					for (const [key, pConfig] of Object.entries(config.providers as Record<string, any>)) {
						const circuit = circuitList.find((c: any) => c.provider === key);
						const isActive = !circuit || circuit.state === 'closed';
						const circuitState = circuit ? circuit.state : 'closed';

						// Find tier models matching this provider
						const tierModels = (config.tiers || [])
							.filter((t: any) => t.provider === key)
							.map((t: any) => t.model);
						if (config.default_tier?.provider === key && config.default_tier.model && !tierModels.includes(config.default_tier.model)) {
							tierModels.push(config.default_tier.model);
						}

						let icon = '🔌';
						let displayName = key.toUpperCase();
						let pType: 'local' | 'cloud' = pConfig?.type === 'local' ? 'local' : 'cloud';

						if (key === 'ollama') {
							icon = '🦙';
							displayName = 'Ollama';
							pType = 'local';
						} else if (key === 'openrouter') {
							icon = '⚡';
							displayName = 'OpenRouter';
							pType = 'cloud';
						} else if (key === 'vllm') {
							icon = '🚀';
							displayName = 'vLLM';
							pType = 'local';
						} else if (key === 'sglang') {
							icon = '⚡';
							displayName = 'SGLang';
							pType = 'local';
						} else if (key.includes('llama')) {
							icon = '🦙';
							displayName = 'llama.cpp';
							pType = 'local';
						} else if (key === 'openai') {
							icon = '🟢';
							displayName = 'OpenAI';
							pType = 'cloud';
						} else if (key === 'anthropic') {
							icon = '🧠';
							displayName = 'Anthropic';
							pType = 'cloud';
						} else if (key === 'deepseek') {
							icon = '🐋';
							displayName = 'DeepSeek';
							pType = 'cloud';
						} else {
							displayName = key.charAt(0).toUpperCase() + key.slice(1);
						}

						providers.push({
							id: key,
							name: displayName,
							icon,
							type: pType,
							baseUrl: pConfig?.base_url || '',
							models: tierModels,
							active: isActive,
							circuitState
						});
					}
				}
			} catch (_) {}
		}

		const engineMode = this.authManager.getEngineMode();
		const remoteUrl = this.authManager.getRemoteUrl();
		const remoteToken = await this.authManager.getRemoteToken();

		this.sidebarProvider.updateState({
			engineMode,
			remoteUrl,
			token: remoteToken || '',
			hasToken: !!token,
			engineStatus,
			providers
		});
	}

	private async initializeClients(): Promise<void> {
		const baseUrl = await this.authManager.getBaseUrl();
		const authToken = await this.authManager.getAuthToken();
		
		this.restClient = new RestClient(baseUrl, authToken);
		this.sseClient = new SSEClient(baseUrl, authToken);
		this.sseClient.connect();
		
		// Setup SSE event handlers
		this.setupSSEHandlers();
	}

	private setupSSEHandlers(): void {
		if (!this.sseClient) return;
		
		this.sseClient.subscribe('routeCompleted', (_data) => {
			// Handle route completion
			this.updateStats();
		});
		
		this.sseClient.subscribe('circuitStateChanged', (_data) => {
			// Handle circuit state change
			this.updateStats();
		});
		
		this.sseClient.subscribe('configUpdated', (_data) => {
			// Handle config update
			this.updateStats();
		});
	}

	private showDashboard(): void {
		if (this.dashboardPanel) {
			this.dashboardPanel.dispose();
		}
		
		this.dashboardPanel = new DashboardPanel(
			this.context.extensionUri,
			async (message) => {
				switch (message.command) {
					case 'initialize':
						await this.loadDashboardData(false);
						break;
					case 'refreshAll':
						await this.loadDashboardData(true);
						break;
					case 'runOptimizer':
						await this.runOptimizer();
						break;
					case 'refreshDeals':
						await this.refreshDeals(true);
						break;
					case 'applyOptimization':
						await this.applyOptimization(message.data);
						break;
					case 'copyToClipboard':
						if (message.data?.text) {
							await vscode.env.clipboard.writeText(message.data.text);
							this.showTransientToast(`📋 Copied "${message.data.text}" to clipboard`);
						}
						break;
					case 'adoptDeal':
						if (message.data?.modelId) {
							await this.adoptDeal(message.data.modelId, message.data.provider, message.data.recommendedTiers);
						}
						break;
					case 'resetCircuit':
						if (this.restClient) {
							await this.restClient.resetCircuit(message.provider).catch(() => null);
							const circuits = await this.restClient.getCircuits().catch(() => null);
							if (circuits && this.dashboardPanel) {
								this.dashboardPanel.updateCircuits(circuits);
							}
							this.showTransientToast(`Nacho Flow: Circuit breaker reset (${message.provider || 'All Providers'})`);
						}
						break;
					case 'editConfig':
						this.showConfigEditor();
						break;
					case 'saveSettings':
						await this.handleSaveSettings(message.url, message.token);
						break;
					case 'testConnection':
						await this.handleTestConnection(message.url, message.token);
						break;
					case 'recalculateStats':
						await this.handleRecalculateStats();
						break;
					case 'resetStats':
						await this.handleResetStats();
						break;
					case 'setTimeWindow':
						if (message.timeWindow) {
							await this.setTimeWindow(message.timeWindow, false);
						}
						break;
					case 'setRoutesRefreshInterval':
						if (typeof message.interval !== 'undefined') {
							await this.setRoutesRefreshInterval(message.interval, false);
						}
						break;
					case 'openSettings':
						await this.openSettings();
						break;
				}
			},
			() => {
				this.dashboardPanel = null;
			}
		);
		this.context.subscriptions.push(this.dashboardPanel);
		
		// Wire webview visibility observer to pause/resume poller
		if (this.dashboardPanel && typeof this.dashboardPanel.onDidChangeViewState === 'function') {
			this.dashboardPanel.onDidChangeViewState((e) => {
				if (e.webviewPanel.visible) {
					this.telemetryPoller?.resume(true);
				} else {
					this.telemetryPoller?.pause();
				}
			});
		}

		// Send initial active timeframe and refresh interval to dashboard
		this.dashboardPanel.setTimeWindow(this.activeTimeWindow);
		this.dashboardPanel.setRoutesRefreshInterval(this.routesRefreshInterval);

		// Load initial data
		this.loadDashboardData();
	}

	public async setTimeWindow(timeWindow: string, notifyDashboard: boolean = true): Promise<void> {
		this.activeTimeWindow = timeWindow || 'all_time';
		if (this.context.globalState) {
			await this.context.globalState.update('nachoFlow_timeWindow', this.activeTimeWindow);
		}
		this.statusBar.setTimeWindow(this.activeTimeWindow);
		if (notifyDashboard && this.dashboardPanel) {
			this.dashboardPanel.setTimeWindow(this.activeTimeWindow);
		}
	}

	public async setRoutesRefreshInterval(interval: number, notifyDashboard: boolean = true): Promise<void> {
		this.routesRefreshInterval = (Number(interval) as RefreshIntervalSeconds) || 0;
		if (this.context.globalState) {
			await this.context.globalState.update('nachoFlow_routesRefreshInterval', this.routesRefreshInterval);
		}
		if (this.telemetryPoller) {
			this.telemetryPoller.setIntervalSeconds(this.routesRefreshInterval);
		}
		if (notifyDashboard && this.dashboardPanel) {
			this.dashboardPanel.setRoutesRefreshInterval(this.routesRefreshInterval);
		}
	}

	public async adoptDeal(modelId: string, _provider?: string, recommendedTiers?: string[]): Promise<void> {
		if (!this.restClient) {
			vscode.window.showErrorMessage('Nacho Flow: Daemon client not initialized');
			return;
		}

		try {
			const yamlContent = await this.restClient.getConfigYaml();
			if (!yamlContent) {
				vscode.window.showErrorMessage('Nacho Flow: Unable to fetch configuration from daemon');
				return;
			}

			const config = await this.restClient.getConfig().catch(() => null);
			const tiers: Array<{ name: string; model: string; isDefault?: boolean }> = [];
			if (config?.tiers) {
				tiers.push(...config.tiers);
			}
			if (config?.default_tier) {
				tiers.push({ ...config.default_tier, isDefault: true });
			}

			// If config JSON wasn't parsed or tiers empty, discover tier names from YAML
			if (tiers.length === 0) {
				const tierMatches = Array.from(yamlContent.matchAll(/name:\s*["']?([^"'\n]+)["']?/g));
				for (const match of tierMatches) {
					tiers.push({ name: match[1].trim(), model: '' });
				}
			}

			if (tiers.length === 0) {
				vscode.window.showWarningMessage('Nacho Flow: No routing tiers found in config');
				return;
			}

			const quickPickItems = tiers.map(t => {
				const isRec = recommendedTiers?.some(r =>
					r.toLowerCase() === t.name.toLowerCase() ||
					t.name.toLowerCase().includes(r.toLowerCase())
				);
				const label = `${isRec ? '⭐ ' : ''}${t.name}`;
				const description = t.model ? `Current: ${t.model}` : (t.isDefault ? 'Default Tier' : '');
				const detail = isRec ? 'Recommended tier for this model index' : undefined;
				return {
					label,
					description,
					detail,
					tier: t
				};
			});

			const selected = await vscode.window.showQuickPick(quickPickItems, {
				placeHolder: `Select tier to adopt ${modelId} into:`,
				matchOnDescription: true,
				matchOnDetail: true
			});

			if (!selected) {
				return;
			}

			const targetTierName = selected.tier.name;
			const updatedYaml = this.replaceTierModelInYaml(yamlContent, targetTierName, selected.tier.isDefault, modelId);

			if (updatedYaml === yamlContent) {
				vscode.window.showWarningMessage(`Nacho Flow: Could not find model field for tier "${targetTierName}" in config YAML`);
				return;
			}

			await this.restClient.updateConfigYaml(updatedYaml);
			this.showTransientToast(`🎉 Adopted ${modelId} into ${targetTierName}!`);

			await this.loadDashboardData();
			await this.updateStats();
		} catch (error: any) {
			vscode.window.showErrorMessage(`Nacho Flow: Failed to adopt deal: ${error.message || error}`);
		}
	}

	public replaceTierModelInYaml(yaml: string, tierName: string, isDefault: boolean | undefined, newModel: string): string {
		const lines = yaml.split('\n');
		let inTargetTier = false;
		let inDefaultTier = false;
		let inTiersList = false;

		for (let i = 0; i < lines.length; i++) {
			const line = lines[i];

			if (/^tiers\s*:/.test(line)) {
				inTiersList = true;
				inDefaultTier = false;
				inTargetTier = false;
				continue;
			}

			if (/^default_tier\s*:/.test(line)) {
				inDefaultTier = true;
				inTiersList = false;
				inTargetTier = !!isDefault;
				continue;
			}

			// Root-level non-tier key ends current section
			if (/^[a-zA-Z0-9_-]+\s*:/.test(line) && !line.startsWith(' ') && !line.startsWith('\t')) {
				inTiersList = false;
				inDefaultTier = false;
				inTargetTier = false;
			}

			if (inTiersList) {
				const nameMatch = line.match(/^\s*(?:-\s+)?name\s*:\s*["']?([^"'\n#]+)["']?/);
				if (nameMatch) {
					inTargetTier = nameMatch[1].trim() === tierName;
				}
			}

			if (inTargetTier) {
				const modelMatch = line.match(/^(\s*(?:-\s+)?model\s*:\s*)(["']?[^#\n]*?["']?)(\s*(?:#.*)?)$/);
				if (modelMatch) {
					const prefix = modelMatch[1];
					const trailingComment = modelMatch[3];
					lines[i] = `${prefix}${newModel}${trailingComment}`;
					return lines.join('\n');
				}
			}
		}

		return yaml;
	}

	private activeConfigDocUri: vscode.Uri | null = null;

	public getStandardConfigPaths(): string[] {
		const home = os.homedir();
		const paths: string[] = [];
		if (process.platform === 'win32') {
			const appData = process.env.APPDATA || path.join(home, 'AppData', 'Roaming');
			paths.push(path.join(appData, 'nacho-flow', 'config.yaml'));
		} else if (process.platform === 'darwin') {
			paths.push(path.join(home, 'Library', 'Application Support', 'nacho-flow', 'config.yaml'));
			paths.push(path.join(home, '.config', 'nacho-flow', 'config.yaml'));
		} else {
			paths.push(path.join(home, '.config', 'nacho-flow', 'config.yaml'));
		}
		paths.push(path.join(home, '.nacho-flow', 'config.yaml'));
		return paths;
	}

	public fileExists(p: string): boolean {
		return fs.existsSync(p);
	}

	private async showConfigEditor(): Promise<void> {
		// 1. Try to fetch live configuration from running daemon REST API
		if (this.restClient) {
			try {
				const yamlContent = await this.restClient.getConfigYaml();
				if (yamlContent) {
					const storageDir = this.context.globalStorageUri || this.context.extensionUri;
					const configFileUri = vscode.Uri.joinPath(storageDir, 'nacho-flow-config.yaml');
					
					try {
						await vscode.workspace.fs.createDirectory(storageDir);
					} catch (_) {}
					await vscode.workspace.fs.writeFile(configFileUri, Buffer.from(yamlContent, 'utf8'));

					const doc = await vscode.workspace.openTextDocument(configFileUri);
					this.activeConfigDocUri = doc.uri;
					await vscode.window.showTextDocument(doc, vscode.ViewColumn.Beside);
					return;
				}
			} catch (_) {}
		}

		// 2. Check standard OS user config directory locations (macOS, Linux, Windows)
		const standardPaths = this.getStandardConfigPaths();
		for (const p of standardPaths) {
			if (this.fileExists(p)) {
				try {
					const doc = await vscode.workspace.openTextDocument(vscode.Uri.file(p));
					this.activeConfigDocUri = doc.uri;
					await vscode.window.showTextDocument(doc, vscode.ViewColumn.Beside);
					return;
				} catch (_) {}
			}
		}

		// 3. Search open workspace folders for local config.yaml
		const files = await vscode.workspace.findFiles('**/config.yaml', '**/node_modules/**', 1);
		if (files.length > 0) {
			const doc = await vscode.workspace.openTextDocument(files[0]);
			this.activeConfigDocUri = doc.uri;
			await vscode.window.showTextDocument(doc, vscode.ViewColumn.Beside);
			return;
		}

		// 4. If config does not exist anywhere, auto-bootstrap starter config.yaml
		if (standardPaths.length > 0) {
			const targetPath = standardPaths[0];
			try {
				const targetDir = path.dirname(targetPath);
				if (!fs.existsSync(targetDir)) {
					fs.mkdirSync(targetDir, { recursive: true });
				}
				const starter = `# =============================================================================\n# 🌮 NACHO FLOW CONFIGURATION\n# Intelligent Semantic AI Gateway & Multi-Tier Cost Optimizer\n# =============================================================================\n\nport: 8000\n\nproviders:\n  ollama:\n    base_url: "http://127.0.0.1:11434"\n    type: "local"\n\n  openrouter:\n    base_url: "https://openrouter.ai/api/v1"\n    type: "cloud"\n    api_key: "ENV_OPENROUTER_API_KEY"\n`;
				fs.writeFileSync(targetPath, starter, 'utf8');
				const doc = await vscode.workspace.openTextDocument(vscode.Uri.file(targetPath));
				this.activeConfigDocUri = doc.uri;
				await vscode.window.showTextDocument(doc, vscode.ViewColumn.Beside);
				return;
			} catch (_) {}
		}

		vscode.window.showWarningMessage('Nacho Flow: Unable to fetch config from daemon or workspace');
	}

	private async handleEngineStartError(result: { success: boolean; error?: string; parsedError?: ParsedStartupError }): Promise<void> {
		if (result.success || !result.error) {
			return;
		}
		const parsed = result.parsedError;
		if (parsed) {
			if (parsed.type === 'PORT_IN_USE' || parsed.type === 'CONFIG_ERROR' || parsed.type === 'RULE_ERROR') {
				const choice = await vscode.window.showErrorMessage(parsed.message, '📝 Open config.yaml');
				if (choice === '📝 Open config.yaml') {
					await this.showConfigEditor();
				}
				return;
			}
		}
		vscode.window.showErrorMessage(result.error);
	}

	public async runOptimizer(): Promise<void> {
		if (!this.restClient) {
			vscode.window.showErrorMessage('Nacho Flow: Daemon client not initialized');
			return;
		}

		if (!this.dashboardPanel) {
			this.showDashboard();
		}

		try {
			const result = await this.restClient.tune();
			if (this.dashboardPanel) {
				this.dashboardPanel.updateOptimization(result);
			}
		} catch (error: any) {
			vscode.window.showErrorMessage(`Nacho Flow: Optimization failed: ${error.message}`);
		}
	}

	public async refreshDeals(manual: boolean = false): Promise<void> {
		if (!this.restClient) return;

		if (manual && !this.dashboardPanel) {
			this.showDashboard();
		}

		try {
			const deals = await this.restClient.getDeals();
			if (deals && this.dashboardPanel) {
				this.dashboardPanel.updateDeals(deals);
			}
			if (manual) {
				const count = deals?.deals?.length || 0;
				this.showTransientToast(`🔥 Nacho Flow: Refreshed ${count} Heatseeker deals`);
			}
		} catch (error: any) {
			console.error('Failed to refresh deals:', error);
			if (manual) {
				vscode.window.showErrorMessage(`Nacho Flow: Failed to refresh deals: ${error.message}`);
			}
		}
	}

	public async applyOptimization(optData?: any): Promise<void> {
		if (!this.restClient) {
			vscode.window.showErrorMessage('Nacho Flow: Daemon client not initialized');
			return;
		}

		try {
			const data = optData || await this.restClient.tune();
			const targetTier = data?.target_tier_name;
			const synthesizedRule = data?.synthesized_rule;

			if (!targetTier || !synthesizedRule) {
				vscode.window.showWarningMessage('Nacho Flow: No optimization policy available to apply');
				return;
			}

			const yamlContent = await this.restClient.getConfigYaml();
			if (!yamlContent) {
				vscode.window.showErrorMessage('Nacho Flow: Unable to fetch configuration from daemon');
				return;
			}

			const updatedYaml = this.replaceTierRuleInYaml(yamlContent, targetTier, synthesizedRule);
			if (updatedYaml === yamlContent) {
				vscode.window.showWarningMessage(`Nacho Flow: Could not locate rule for tier "${targetTier}" in config YAML`);
				return;
			}

			await this.restClient.updateConfigYaml(updatedYaml);
			this.showTransientToast(`🎉 Applied Auto-Tuner policy to ${targetTier}!`);

			if (this.dashboardPanel) {
				this.dashboardPanel.updateOptimization(null);
			}
			await this.loadDashboardData();
			await this.updateStats();
		} catch (error: any) {
			vscode.window.showErrorMessage(`Nacho Flow: Failed to apply optimization: ${error.message || error}`);
		}
	}

	public replaceTierRuleInYaml(yaml: string, tierName: string, newRule: string): string {
		const lines = yaml.split('\n');
		let inTargetTier = false;
		let inTiersList = false;

		for (let i = 0; i < lines.length; i++) {
			const line = lines[i];

			if (/^tiers\s*:/.test(line)) {
				inTiersList = true;
				inTargetTier = false;
				continue;
			}

			// Root-level non-tier key ends current section
			if (/^[a-zA-Z0-9_-]+\s*:/.test(line) && !line.startsWith(' ') && !line.startsWith('\t')) {
				inTiersList = false;
				inTargetTier = false;
			}

			if (inTiersList) {
				const nameMatch = line.match(/^\s*(?:-\s+)?name\s*:\s*["']?([^"'\n#]+)["']?/);
				if (nameMatch) {
					inTargetTier = nameMatch[1].trim() === tierName;
				}
			}

			if (inTargetTier) {
				const whenMatch = line.match(/^(\s*(?:-\s+)?when\s*:\s*)(["']?[^#\n]*?["']?)(\s*(?:#.*)?)$/);
				if (whenMatch) {
					const prefix = whenMatch[1];
					const trailingComment = whenMatch[3];
					lines[i] = `${prefix}"${newRule}"${trailingComment}`;
					return lines.join('\n');
				}
			}
		}

		return yaml;
	}

	private async resetCircuit(): Promise<void> {
		if (!this.restClient) {
			vscode.window.showErrorMessage('Nacho Flow: Daemon client not initialized');
			return;
		}
		const providers = ['All Circuits', 'openrouter', 'ollama'];
		const selected = await vscode.window.showQuickPick(providers, {
			placeHolder: 'Select circuit breaker to reset'
		});
		if (selected) {
			const provider = selected === 'All Circuits' ? undefined : selected;
			try {
				await this.restClient.resetCircuit(provider);
				this.showTransientToast(`Nacho Flow: Circuit breaker reset (${selected})`);
				if (this.dashboardPanel) {
					const circuits = await this.restClient.getCircuits().catch(() => null);
					if (circuits) {
						this.dashboardPanel.updateCircuits(circuits);
					}
				}
			} catch (error: any) {
				vscode.window.showErrorMessage(`Nacho Flow: Failed to reset circuit: ${error.message}`);
			}
		}
	}

	private async loadDashboardData(manual: boolean = false): Promise<void> {
		if (!this.restClient || !this.dashboardPanel) return;
		
		try {
			const stats = await this.restClient.getStats();
			if (stats && this.dashboardPanel) {
				this.dashboardPanel.updateStats(stats);
			}
		} catch (_) {}

		try {
			const deals = await this.restClient.getDeals();
			if (deals && this.dashboardPanel) {
				this.dashboardPanel.updateDeals(deals);
			}
		} catch (_) {}

		try {
			const routes = await this.restClient.getRoutes(10);
			if (routes && this.dashboardPanel) {
				this.dashboardPanel.updateRoutes(routes);
			}
		} catch (_) {}

		try {
			const circuits = await this.restClient.getCircuits();
			if (circuits && this.dashboardPanel) {
				this.dashboardPanel.updateCircuits(circuits);
			}
		} catch (_) {}

		try {
			const config = await this.restClient.getConfig();
			if (config && this.dashboardPanel) {
				this.dashboardPanel.updateConfig(config);
			}
		} catch (_) {}

		try {
			await this.syncSidebarState();
		} catch (_) {}

		if (manual) {
			this.showTransientToast('🔄 Nacho Flow: Telemetry & dashboard refreshed');
		}
	}

	private async handleSaveSettings(url?: string, token?: string): Promise<void> {
		try {
			if (url && url.trim() !== '') {
				await this.authManager.setRemoteUrl(url.trim());
			}
			if (token !== undefined) {
				await this.authManager.setRemoteToken(token.trim());
			}
			await this.authManager.setEngineMode('remote');
			await this.initializeClients();
			this.showTransientToast('🌮 Nacho Flow: Remote server settings saved!');
			await this.handleTestConnection(url, token);
			await this.loadDashboardData();
			await this.updateStats();
		} catch (error: any) {
			vscode.window.showErrorMessage(`Nacho Flow: Failed to save settings: ${error.message || error}`);
		}
	}

	private async handleTestConnection(url?: string, token?: string): Promise<void> {
		const targetUrl = url && url.trim() !== '' ? url.trim() : await this.authManager.getBaseUrl();
		let targetToken = token !== undefined && token.trim() !== '' ? token.trim() : undefined;
		if (targetToken === undefined) {
			targetToken = this.processManager.isLocalUrl(targetUrl)
				? await this.authManager.getAuthToken()
				: await this.authManager.getRemoteToken();
		}
		
		if (this.sidebarProvider) {
			this.sidebarProvider.updateEngineStatus({ testing: true });
		}

		const testClient = new RestClient(targetUrl, targetToken);
		try {
			const health = await testClient.getHealth();
			if (this.sidebarProvider) {
				this.sidebarProvider.updateEngineStatus({
					connected: true,
					version: health?.version || 'Online'
				});
			}
			this.showTransientToast(`🟢 Routing Engine (${targetUrl}) verified!`);
		} catch (err: any) {
			if (this.sidebarProvider) {
				this.sidebarProvider.updateEngineStatus({
					connected: false,
					error: err.message || 'Connection refused'
				});
			}
			this.showTransientToast(`🔴 Connection failed: ${err.message || 'Connection refused'}`);
		}
	}

	private async handleRecalculateStats(): Promise<void> {
		if (!this.restClient) {
			vscode.window.showErrorMessage('Nacho Flow: Daemon client not initialized');
			return;
		}
		try {
			const updatedStats = await this.restClient.recalculateStats();
			if (this.dashboardPanel && updatedStats) {
				this.dashboardPanel.updateStats(updatedStats);
			}
			this.showTransientToast('🔄 Nacho Flow: Historical stats recalculated from logs!');
			await this.loadDashboardData();
			await this.updateStats();
		} catch (error: any) {
			vscode.window.showErrorMessage(`Nacho Flow: Failed to recalculate stats: ${error.message || error}`);
		}
	}

	private async handleResetStats(): Promise<void> {
		if (!this.restClient) {
			vscode.window.showErrorMessage('Nacho Flow: Daemon client not initialized');
			return;
		}
		try {
			const resetStats = await this.restClient.resetStats();
			if (this.dashboardPanel && resetStats) {
				this.dashboardPanel.updateStats(resetStats);
			}
			this.showTransientToast('🗑️ Nacho Flow: All metrics & stats reset to $0.00!');
			await this.loadDashboardData();
			await this.updateStats();
		} catch (error: any) {
			vscode.window.showErrorMessage(`Nacho Flow: Failed to reset stats: ${error.message || error}`);
		}
	}

	private showTransientToast(message: string, durationMs: number = 3000): void {
		vscode.window.withProgress(
			{
				location: vscode.ProgressLocation.Notification,
				title: message,
				cancellable: false
			},
			async () => {
				await new Promise((resolve) => setTimeout(resolve, durationMs));
			}
		);
	}

	private async pollTelemetry(): Promise<void> {
		if (!this.restClient) return;

		try {
			const statsPromise = typeof this.restClient.getStats === 'function'
				? Promise.resolve().then(() => this.restClient!.getStats()).catch(() => null)
				: Promise.resolve(null);
			const routesPromise = (this.dashboardPanel && typeof this.restClient.getRoutes === 'function')
				? Promise.resolve().then(() => this.restClient!.getRoutes(10)).catch(() => null)
				: Promise.resolve(null);

			const [stats, routes] = await Promise.all([statsPromise, routesPromise]);

			if (stats) {
				this.statusBar.updateStats(stats);
				if (this.dashboardPanel) {
					this.dashboardPanel.updateStats(stats);
				}
			} else {
				this.statusBar.updateStats(null);
			}

			if (routes && this.dashboardPanel) {
				this.dashboardPanel.updateRoutes(routes);
			}
		} catch (error) {
			console.error('Failed to poll telemetry:', error);
			this.statusBar.updateStats(null);
		}
	}

	private async updateStats(): Promise<void> {
		await this.pollTelemetry();
	}

	private startPeriodicUpdates(): void {
		if (!this.telemetryPoller) {
			this.telemetryPoller = new TelemetryPoller({
				intervalSeconds: this.routesRefreshInterval,
				onTick: async () => {
					await this.pollTelemetry();
				}
			});
			this.context.subscriptions.push(this.telemetryPoller);
		}
	}

	public dispose(): void {
		if (this.telemetryPoller) {
			this.telemetryPoller.dispose();
			this.telemetryPoller = null;
		}
		this.statusBar.dispose();
		if (this.sseClient) {
			this.sseClient.disconnect();
		}
		if (this.processManager) {
			this.processManager.dispose();
		}
	}
}