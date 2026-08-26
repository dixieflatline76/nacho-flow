import * as vscode from 'vscode';
import { CircuitInfo } from '../../core/types/nacho-types';

export class CircuitsPanel {
	private panel: vscode.WebviewPanel;

	constructor(extensionUri: vscode.Uri) {
		this.panel = vscode.window.createWebviewPanel(
			'nachoFlowCircuits',
			'Nacho Flow Circuits',
			vscode.ViewColumn.One,
			{
				enableScripts: true,
				retainContextWhenHidden: true
			}
		);

		this.panel.webview.html = this.getHtmlForWebview(this.panel.webview, extensionUri);
	}

	private getHtmlForWebview(webview: vscode.Webview, extensionUri: vscode.Uri): string {
		const scriptUri = webview.asWebviewUri(vscode.Uri.joinPath(extensionUri, 'resources', 'webview', 'circuits.js'));
		const styleUri = webview.asWebviewUri(vscode.Uri.joinPath(extensionUri, 'resources', 'webview', 'circuits.css'));

		return `
<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<link href="${styleUri}" rel="stylesheet">
	<title>Nacho Flow Circuits</title>
</head>
<body>
	<div class="container">
		<h1>Circuit Breakers</h1>
		<div id="circuits-container"></div>
		<button id="reset-all-btn">Reset All Circuits</button>
	</div>
	<script src="${scriptUri}"></script>
</body>
</html>`;
	}

	public dispose(): void {
		this.panel.dispose();
	}
}