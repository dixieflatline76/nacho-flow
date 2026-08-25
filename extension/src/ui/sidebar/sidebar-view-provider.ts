import * as vscode from 'vscode';

export interface SidebarMessageHandler {
	(message: { command: string; [key: string]: any }): Promise<void> | void;
}

export class SidebarViewProvider implements vscode.WebviewViewProvider {
	public static readonly viewType = 'nacho-flow.sidebarView';
	private view?: vscode.WebviewView;

	constructor(
		private readonly extensionUri: vscode.Uri,
		private readonly onMessage?: SidebarMessageHandler
	) {}

	public resolveWebviewView(
		webviewView: vscode.WebviewView,
		_context: vscode.WebviewViewResolveContext,
		_token: vscode.CancellationToken
	): void {
		this.view = webviewView;

		webviewView.webview.options = {
			enableScripts: true,
			localResourceRoots: [this.extensionUri]
		};

		webviewView.webview.html = this.getHtmlForWebview(webviewView.webview);

		if (this.onMessage) {
			webviewView.webview.onDidReceiveMessage(this.onMessage);
		}
	}

	public updateState(data: any): void {
		this.safePostMessage({ command: 'updateState', data });
	}

	public updateEngineStatus(data: any): void {
		this.safePostMessage({ command: 'updateEngineStatus', data });
	}

	public updateOllamaStatus(data: any): void {
		this.safePostMessage({ command: 'updateOllamaStatus', data });
	}

	private safePostMessage(message: any): void {
		if (!this.view) return;
		try {
			this.view.webview.postMessage(message);
		} catch (_) {}
	}

	private getHtmlForWebview(webview: vscode.Webview): string {
		const styleUri = webview.asWebviewUri(vscode.Uri.joinPath(this.extensionUri, 'resources', 'sidebar', 'sidebar.css'));
		const scriptUri = webview.asWebviewUri(vscode.Uri.joinPath(this.extensionUri, 'resources', 'sidebar', 'sidebar.js'));

		return `<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<link href="${styleUri}" rel="stylesheet">
	<title>Nacho Flow</title>
</head>
<body>
	<div class="sidebar-header">
		<div class="brand-row">
			<div class="brand-title">🌮 Nacho Flow</div>
		</div>
		<div id="engine-status-chip" class="status-chip chip-gray">⚪ Engine Offline (Click ▶️ Start)</div>
	</div>

	<!-- 1. Routing Engine Host -->
	<div class="section-card">
		<div class="section-header">🌐 1. Routing Engine</div>
		<div class="section-body">
			<div class="radio-group">
				<label class="radio-label">
					<input type="radio" name="engine-mode" id="radio-mode-local" value="local" checked onchange="setEngineMode('local')" />
					<span>This Machine</span>
				</label>
				<label class="radio-label">
					<input type="radio" name="engine-mode" id="radio-mode-remote" value="remote" onchange="setEngineMode('remote')" />
					<span>Remote Server</span>
				</label>
			</div>

			<!-- Local Engine Controls -->
			<div id="local-engine-controls" class="form-group">
				<div class="btn-row">
					<button id="btn-engine-start" class="btn btn-secondary btn-compact" onclick="startEngine()">▶️ Start</button>
					<button id="btn-engine-stop" class="btn btn-secondary btn-compact" onclick="stopEngine()" style="display: none;">⏹️ Stop</button>
					<button id="btn-engine-restart" class="btn btn-secondary btn-compact" onclick="restartEngine()">🔄 Restart</button>
					<button class="btn btn-secondary btn-compact" onclick="openEngineLogs()">📜 Logs</button>
				</div>
			</div>

			<!-- Remote Engine Controls -->
			<div id="remote-engine-controls" class="form-group" style="display: none; gap: 8px;">
				<div class="form-group">
					<label for="remote-engine-url">Server Endpoint URL</label>
					<div class="input-row">
						<input type="text" id="remote-engine-url" placeholder="http://192.168.0.205:8000" />
						<button class="btn btn-secondary btn-compact" onclick="testRemoteConnection()">⚡ Test</button>
					</div>
				</div>
				<div class="form-group">
					<label for="remote-engine-token">Bearer Auth Token</label>
					<div class="input-row">
						<input type="password" id="remote-engine-token" placeholder="Optional bearer auth token" />
						<button class="btn btn-secondary btn-compact" id="btn-token-eye" onclick="togglePasswordVisibility('remote-engine-token', 'btn-token-eye')">👁️</button>
					</div>
				</div>
				<button class="btn btn-primary btn-compact" onclick="saveRemoteSettings()">💾 Save Remote Server</button>
			</div>
		</div>
	</div>

	<!-- 2. Upstream Inference Engines -->
	<div class="section-card">
		<div class="section-header">
			<span>⚡ 2. Upstream Inference Engines</span>
			<button class="icon-link-btn" onclick="openConfigFile()" title="Open config.yaml in editor">📝 config.yaml</button>
		</div>
		<div id="providers-container" class="section-body">
			<div class="partner-desc">Connecting to engine providers...</div>
		</div>
	</div>

	<!-- 3. Coding Agents -->
	<div class="section-card">
		<div class="section-header">🤖 3. Coding Agents</div>
		<div class="section-body">
			<div class="partner-desc">Configure OpenAI-compatible provider in Roo Code, Cline, or Cursor:</div>
			<div class="agent-config-card">
				<div class="agent-config-row">
					<span class="agent-config-label">Base URL</span>
					<div class="agent-config-val">
						<span id="proxy-endpoint-text" class="agent-config-code">http://127.0.0.1:8000/v1</span>
						<button class="btn-icon-copy" onclick="copyRooEndpoint()" title="Copy Base URL">📋 Copy</button>
					</div>
				</div>
				<div class="agent-config-row">
					<span class="agent-config-label">API Key</span>
					<div class="agent-config-val">
						<span id="agent-token-text" class="agent-config-code">•••••••• (Auth Token)</span>
						<button class="btn-icon-copy" onclick="copyActiveToken()" title="Copy Active Token">📋 Copy</button>
					</div>
				</div>
				<div class="agent-config-row">
					<span class="agent-config-label">Model ID</span>
					<div class="agent-config-val">
						<span class="agent-config-code">nacho-hybrid</span>
						<button class="btn-icon-copy" onclick="copyModelId()" title="Copy Model ID (nacho-hybrid)">📋 Copy</button>
					</div>
				</div>
			</div>
			<div class="btn-row" style="margin-top: 4px;">
				<button class="btn btn-secondary btn-compact" onclick="openMarketplace('RooVeterinaryInc.roo-cline')">
					<span class="brand-logo-svg roo"><svg viewBox="0 0 24 24" fill="currentColor"><path d="M1 11.23L5.18 10.45L12 5.03L18.97 7.04L19.59 5.34L23 10.45L20.83 10.61L15.56 12L14.01 14.17L16.8 18.97L15.41 18.97L12 13.86L12 11.54L9.37 9.83L5.8 11.69Z"/></svg></span>
					Install Roo Code
				</button>
				<button class="btn btn-secondary btn-compact" onclick="openMarketplace('saoudrizwan.claude-dev')">
					<span class="brand-logo-svg cline"><svg viewBox="0 0 466.73 487.04" fill="currentColor"><path d="M463.6,275.08l-29.26-58.75v-33.83c0-56.08-45.01-101.5-100.53-101.5h-50.01c3.62-7.43,5.61-15.79,5.61-24.61,0-31.17-25.08-56.39-56.07-56.39s-56.07,25.22-56.07,56.39c0,8.82,1.99,17.17,5.61,24.61h-50.01c-55.51,0-100.52,45.42-100.52,101.5v33.83l-29.87,58.59c-3.01,5.9-3.01,12.92,0,18.81l29.87,57.93v33.83c0,56.08,45.01,101.5,100.52,101.5h200.95c55.51,0,100.53-45.42,100.53-101.5v-33.83l29.21-58.13c2.9-5.79,2.9-12.61.05-18.46ZM202.75,322.96c0,25.48-20.54,46.14-45.88,46.14s-45.88-20.66-45.88-46.14v-82.02c0-25.48,20.54-46.14,45.88-46.14s45.88,20.66,45.88,46.14v82.02ZM350.58,322.96c0,25.48-20.54,46.14-45.88,46.14s-45.88-20.66-45.88-46.14v-82.02c0-25.48,20.54-46.14,45.88-46.14s45.88,20.66,45.88,46.14v82.02Z"/></svg></span>
					Install Cline
				</button>
			</div>
		</div>
	</div>

	<!-- 4. Maintenance & Operations -->
	<div class="section-card">
		<div class="section-header">🛠️ 4. Maintenance & Operations</div>
		<div class="section-body">
			<button class="btn btn-secondary" onclick="recalculateStats()">
				<span class="brand-logo-svg" style="width: 14px; height: 14px;"><svg viewBox="0 0 24 24" fill="currentColor"><path d="M17.65 6.35A7.958 7.958 0 0012 4c-4.42 0-7.99 3.58-7.99 8s3.57 8 7.99 8c3.73 0 6.84-2.55 7.73-6h-2.08A5.99 5.99 0 0112 18c-3.31 0-6-2.69-6-6s2.69-6 6-6c1.66 0 3.14.69 4.22 1.78L13 11h7V4l-2.35 2.35z"/></svg></span>
				Recalculate Stats from Logs
			</button>
			<button class="btn btn-secondary" onclick="resetCircuits()">
				<span class="brand-logo-svg" style="width: 14px; height: 14px;"><svg viewBox="0 0 24 24" fill="currentColor"><path d="M7 2v11h3v9l7-12h-4l4-8z"/></svg></span>
				Reset Circuit Breakers
			</button>
			<button class="btn btn-danger" onclick="confirmResetStats()">
				<span class="brand-logo-svg" style="width: 14px; height: 14px;"><svg viewBox="0 0 24 24" fill="currentColor"><path d="M6 19c0 1.1.9 2 2 2h8c1.1 0 2-.9 2-2V7H6v12zM19 4h-3.5l-1-1h-5l-1 1H5v2h14V4z"/></svg></span>
				Reset All Stats to $0.00
			</button>
		</div>
	</div>

	<!-- Dashboard Launcher -->
	<button class="btn btn-primary" onclick="openDashboard()" style="margin-top: 4px; padding: 10px; font-weight: 600;">
		<span class="brand-logo-svg" style="width: 15px; height: 15px;"><svg viewBox="0 0 24 24" fill="currentColor"><path d="M19 3H5c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2V5c0-1.1-.9-2-2-2zM9 17H7v-7h2v7zm4 0h-2V7h2v10zm4 0h-2v-4h2v4z"/></svg></span>
		Open Full Analytics Dashboard
	</button>

	<!-- Hardware Specs Guide Modal -->
	<div id="specs-modal" class="modal-overlay" style="display: none;">
		<div class="modal-content">
			<h3>ℹ️ Hardware & GPU VRAM Guide</h3>
			<p style="font-size: 11px; color: var(--vscode-descriptionForeground); margin-top: 0;">Recommended local model: <strong>qwen2.5-coder:14b</strong></p>
			<ul>
				<li><strong>🎮 NVIDIA / AMD GPUs (Dedicated VRAM)</strong>:
					<br/>• <strong>7B Models</strong>: Needs 6GB–8GB VRAM (e.g. RTX 3060/4060, RX 6600).
					<br/>• <strong>14B Models</strong>: Needs 10GB–12GB VRAM (e.g. RTX 3080/4070, RX 6800).
				</li>
				<li><strong>🍎 Apple Silicon (M1 / M2 / M3 / M4)</strong>:
					<br/>• Uses Unified Memory (system RAM is shared between CPU & GPU).
					<br/>• <strong>16GB+ RAM</strong>: Runs 14B models at full GPU acceleration.
					<br/>• <strong>8GB RAM</strong>: Runs 7B or 3B models.
				</li>
				<li><strong>💻 CPU Mode (RAM Fallback)</strong>:
					<br/>• If no dedicated GPU is available, Ollama will run in standard system RAM (functional, but slower token generation).
				</li>
			</ul>
			<button class="btn btn-secondary btn-compact" onclick="closeSpecsModal()">Close</button>
		</div>
	</div>

	<!-- Danger Reset Confirmation Modal -->
	<div id="danger-modal" class="modal-overlay" style="display: none;">
		<div class="modal-content">
			<h3 style="color: var(--error-color);">⚠️ Reset Telemetry</h3>
			<p style="font-size: 11px; margin-bottom: 12px;">Reset all cost counters and token telemetry to $0.00? This cannot be undone.</p>
			<div class="btn-row">
				<button class="btn btn-secondary btn-compact" onclick="closeDangerModal()">Cancel</button>
				<button class="btn btn-danger btn-compact" onclick="executeResetStats()">Reset All Stats</button>
			</div>
		</div>
	</div>

	<script src="${scriptUri}"></script>
</body>
</html>`;
	}
}
