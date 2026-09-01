import * as vscode from 'vscode';

export class DashboardPanel {
	private panel: vscode.WebviewPanel;
	private disposables: vscode.Disposable[] = [];
	private isDisposed: boolean = false;

	constructor(
		extensionUri: vscode.Uri,
		onMessage?: (message: any) => void,
		onDispose?: () => void
	) {
		this.panel = vscode.window.createWebviewPanel(
			'nachoFlowDashboard',
			'Nacho Flow Dashboard',
			vscode.ViewColumn.One,
			{
				enableScripts: true,
				retainContextWhenHidden: true
			}
		);

		this.panel.webview.html = this.getHtmlForWebview(this.panel.webview, extensionUri);

		if (onMessage) {
			this.panel.webview.onDidReceiveMessage(onMessage, null, this.disposables);
		}

		this.panel.onDidDispose(() => {
			this.isDisposed = true;
			this.dispose();
			if (onDispose) {
				onDispose();
			}
		}, null, this.disposables);
	}

	private getHtmlForWebview(webview: vscode.Webview, extensionUri: vscode.Uri): string {
		const scriptUri = webview.asWebviewUri(vscode.Uri.joinPath(extensionUri, 'resources', 'webview', 'dashboard.js'));
		const styleUri = webview.asWebviewUri(vscode.Uri.joinPath(extensionUri, 'resources', 'webview', 'dashboard.css'));

		return `
<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<link href="${styleUri}" rel="stylesheet">
	<title>Nacho Flow Dashboard</title>
</head>
<body>
	<div class="container">
		<div class="dashboard-header">
			<div class="header-brand-row">
				<div class="header-title">
					<h1>🌮 Nacho Flow Dashboard</h1>
					<span class="version-tag">Smart Hybrid AI Gateway for Autonomous Coding Agents</span>
				</div>
			</div>
			<div class="telemetry-bar-row">
				<div class="telemetry-pulse-group">
					<div class="refresh-interval-tabs">
						<span class="refresh-interval-label">Auto-Refresh:</span>
						<button id="refresh-60s" class="tab-btn active" onclick="setRoutesRefreshInterval(60)">60s</button>
						<button id="refresh-30s" class="tab-btn" onclick="setRoutesRefreshInterval(30)">30s</button>
						<button id="refresh-15s" class="tab-btn" onclick="setRoutesRefreshInterval(15)">15s</button>
						<button id="refresh-off" class="tab-btn" onclick="setRoutesRefreshInterval(0)">Off</button>
					</div>
					<button id="btn-refresh-now" class="btn btn-toolbar" onclick="refreshAll()">
						<svg class="toolbar-svg" viewBox="0 0 24 24" fill="currentColor"><path d="M17.65 6.35A7.958 7.958 0 0012 4c-4.42 0-7.99 3.58-7.99 8s3.57 8 7.99 8c3.73 0 6.84-2.55 7.73-6h-2.08A5.99 5.99 0 0112 18c-3.31 0-6-2.69-6-6s2.69-6 6-6c1.66 0 3.14.69 4.22 1.78L13 11h7V4l-2.35 2.35z"/></svg>
						Refresh Now
					</button>
				</div>
				<button id="btn-open-settings" class="btn btn-toolbar btn-settings" onclick="openSettings()" title="Nacho Flow Extension Settings">
					<svg class="toolbar-svg" viewBox="0 0 24 24" fill="currentColor"><path d="M19.14 12.94c.04-.3.06-.61.06-.94 0-.32-.02-.64-.07-.94l2.03-1.58a.49.49 0 00.12-.61l-1.92-3.32a.488.488 0 00-.59-.22l-2.39.96c-.5-.38-1.03-.7-1.62-.94l-.36-2.54a.484.484 0 00-.48-.41h-3.84c-.24 0-.43.17-.47.41l-.36 2.54c-.59.24-1.13.57-1.62.94l-2.39-.96c-.22-.08-.47 0-.59.22L2.74 8.87c-.12.21-.08.47.12.61l2.03 1.58c-.05.3-.09.63-.09.94s.02.64.07.94l-2.03 1.58a.49.49 0 00-.12.61l1.92 3.32c.12.22.37.29.59.22l2.39-.96c.5.38 1.03.7 1.62.94l.36 2.54c.05.24.24.41.48.41h3.84c.24 0 .44-.17.47-.41l.36-2.54c.59-.24 1.13-.56 1.62-.94l2.39.96c.22.08.47 0 .59-.22l1.92-3.32c.12-.22.07-.47-.12-.61l-2.01-1.58zM12 15.6c-1.98 0-3.6-1.62-3.6-3.6s1.62-3.6 3.6-3.6 3.6 1.62 3.6 3.6-1.62 3.6-3.6 3.6z"/></svg>
					Settings
				</button>
			</div>
		</div>

		<!-- SECTION 1: Telemetry & Flight Instruments -->
		<div class="dashboard-grid telemetry-grid">
			<div class="panel stats-panel">
				<div class="panel-header-with-tabs">
					<h2>📊 Statistics & Cost Savings</h2>
					<div class="time-window-tabs">
						<button id="tab-all_time" class="tab-btn active" onclick="setTimeWindow('all_time')">All Time</button>
						<button id="tab-today" class="tab-btn" onclick="setTimeWindow('today')">Today</button>
						<button id="tab-yesterday" class="tab-btn" onclick="setTimeWindow('yesterday')">Yesterday</button>
						<button id="tab-this_week" class="tab-btn" onclick="setTimeWindow('this_week')">This Week</button>
						<button id="tab-this_month" class="tab-btn" onclick="setTimeWindow('this_month')">This Month</button>
					</div>
				</div>
				<div id="stats-timeframe-info" class="timeframe-info"></div>
				<div id="stats-content">Loading...</div>
			</div>

			<div class="panel cycle-killer-panel">
				<h2>🛡️ Cycle Killer (Qu'est-ce que c'est?): Loop Defense & Proactive Rescue</h2>
				<div id="cycle-killer-content">Loading defense telemetry...</div>
			</div>

			<div class="panel routes-panel">
				<h2>🛣️ Recent Routes</h2>
				<div id="routes-content">Loading...</div>
			</div>
		</div>

		<!-- SECTION 2: Control Center & Optimization -->
		<div class="control-center-section">
			<div class="control-center-header">
				<div class="control-center-title-group">
					<h2>🎛️ Nacho Flow Control Center</h2>
					<p class="section-subtitle">Routing Policy Tuning, Market Intelligence & Fault Tolerance</p>
				</div>
				<div class="header-toolbar">
					<button id="btn-run-tuner" class="btn btn-toolbar btn-tuner" onclick="runOptimizer()">
						<svg class="toolbar-svg" viewBox="0 0 24 24" fill="currentColor"><path d="M7 2v11h3v9l7-12h-4l4-8z"/></svg>
						Run Auto-Tuner
					</button>
					<button id="btn-refresh-deals" class="btn btn-toolbar btn-deals" onclick="refreshDeals()">
						<svg class="toolbar-svg" viewBox="0 0 24 24" fill="currentColor"><path d="M13.5.67s.74 2.65.74 4.8c0 2.06-1.35 3.73-3.41 3.73-2.07 0-3.63-1.67-3.63-3.73l.03-.36C5.21 7.51 4 10.62 4 14c0 4.42 3.58 8 8 8s8-3.58 8-8C20 8.61 17.41 3.8 13.5.67zM11.71 19c-1.78 0-3.22-1.4-3.22-3.14 0-1.62 1.05-2.76 2.81-3.12 1.77-.36 3.6-1.21 4.62-2.58.39 1.29.59 2.65.59 4.04 0 2.65-2.15 4.8-4.8 4.8z"/></svg>
						Refresh Deals
					</button>
					<button id="btn-edit-config" class="btn btn-toolbar" onclick="editConfig()">
						<svg class="toolbar-svg" viewBox="0 0 24 24" fill="currentColor"><path d="M14 2H6c-1.1 0-1.99.9-1.99 2L4 20c0 1.1.89 2 1.99 2H18c1.1 0 2-.9 2-2V8l-6-6zm2 16H8v-2h8v2zm0-4H8v-2h8v2zm-3-5V3.5L18.5 9H13z"/></svg>
						config.yaml
					</button>
				</div>
			</div>

			<div id="tuner-banner" class="tuner-banner" style="display:none;"></div>

			<div class="dashboard-grid control-center-grid">
				<div class="panel deals-panel">
					<h2>🔥 Heat Seeker: Live Model Deals</h2>
					<div id="deals-content">Loading live deals from OpenRouter...</div>
				</div>
				<div class="panel circuits-panel">
					<h2>🔌 Provider Circuit Breakers & 0ms Failover</h2>
					<div id="circuits-content">Loading...</div>
				</div>
				<div class="panel config-panel">
					<h2>📝 Active Tier Policies</h2>
					<div id="config-content">Loading...</div>
				</div>
			</div>
		</div>
	</div>
	<script src="${scriptUri}"></script>
</body>
</html>`;
	}

	private safePostMessage(message: any): void {
		if (this.isDisposed) return;
		try {
			this.panel.webview.postMessage(message);
		} catch (_) {
			this.isDisposed = true;
		}
	}

	public updateStats(data: any): void {
		this.safePostMessage({ command: 'updateStats', data });
	}

	public updateDeals(data: any): void {
		this.safePostMessage({ command: 'updateDeals', data });
	}

	public updateRoutes(data: any): void {
		this.safePostMessage({ command: 'updateRoutes', data });
	}

	public updateCircuits(data: any): void {
		this.safePostMessage({ command: 'updateCircuits', data });
	}

	public updateConfig(data: any): void {
		this.safePostMessage({ command: 'updateConfig', data });
	}

	public updateOptimization(data: any): void {
		this.safePostMessage({ command: 'updateOptimization', data });
	}

	public setTimeWindow(timeWindow: string): void {
		this.safePostMessage({ command: 'setTimeWindow', data: { timeWindow } });
	}

	public setRoutesRefreshInterval(interval: number): void {
		this.safePostMessage({ command: 'setRoutesRefreshInterval', data: { interval } });
	}

	public get isVisible(): boolean {
		return !this.isDisposed && this.panel.visible;
	}

	public get onDidChangeViewState(): vscode.Event<vscode.WebviewPanelOnDidChangeViewStateEvent> {
		return this.panel.onDidChangeViewState;
	}

	public dispose(): void {
		this.isDisposed = true;
		while (this.disposables.length) {
			const d = this.disposables.pop();
			if (d) {
				d.dispose();
			}
		}
		try {
			this.panel.dispose();
		} catch (_) {}
	}
}