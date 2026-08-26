import * as vscode from 'vscode';
import { TurnRecord } from '../../core/types/nacho-types';

export class RoutesPanel {
	private panel: vscode.WebviewPanel;
	private disposables: vscode.Disposable[] = [];

	constructor(extensionUri: vscode.Uri) {
		this.panel = vscode.window.createWebviewPanel(
			'nachoFlowRoutes',
			'Nacho Flow Routes',
			vscode.ViewColumn.One,
			{
				enableScripts: true,
				retainContextWhenHidden: true
			}
		);

		this.panel.webview.html = this.getHtmlForWebview(this.panel.webview, extensionUri);
		this.setupMessageHandling();
	}

	private getHtmlForWebview(webview: vscode.Webview, extensionUri: vscode.Uri): string {
		const scriptUri = webview.asWebviewUri(vscode.Uri.joinPath(extensionUri, 'resources', 'webview', 'routes.js'));
		const styleUri = webview.asWebviewUri(vscode.Uri.joinPath(extensionUri, 'resources', 'webview', 'routes.css'));

		return `
<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<link href="${styleUri}" rel="stylesheet">
	<title>Nacho Flow Routes</title>
</head>
<body>
	<div class="container">
		<h1>Route History</h1>
		<div id="routes-table"></div>
		<div id="loading">Loading routes...</div>
	</div>
	<script src="${scriptUri}"></script>
</body>
</html>`;
	}

	private setupMessageHandling(): void {
		this.panel.webview.onDidReceiveMessage(
			message => {
				switch (message.command) {
					case 'refresh':
						this.loadRoutes();
						break;
				}
			},
			null,
			this.disposables
		);
	}

	private async loadRoutes(): Promise<void> {
		// Send message to webview with route data
		// This would come from the REST client
	}

	public dispose(): void {
		this.panel.dispose();
		this.disposables.forEach(d => d.dispose());
	}
}