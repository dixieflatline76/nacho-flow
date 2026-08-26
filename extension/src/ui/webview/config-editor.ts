import * as vscode from 'vscode';
import { Config } from '../../core/types/nacho-types';

export class ConfigEditorPanel {
	private panel: vscode.WebviewPanel;

	constructor(extensionUri: vscode.Uri) {
		this.panel = vscode.window.createWebviewPanel(
			'nachoFlowConfig',
			'Nacho Flow Config',
			vscode.ViewColumn.One,
			{
				enableScripts: true,
				retainContextWhenHidden: true
			}
		);

		this.panel.webview.html = this.getHtmlForWebview(this.panel.webview, extensionUri);
	}

	private getHtmlForWebview(webview: vscode.Webview, extensionUri: vscode.Uri): string {
		const scriptUri = webview.asWebviewUri(vscode.Uri.joinPath(extensionUri, 'resources', 'webview', 'config.js'));
		const styleUri = webview.asWebviewUri(vscode.Uri.joinPath(extensionUri, 'resources', 'webview', 'config.css'));

		return `
<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<link href="${styleUri}" rel="stylesheet">
	<title>Nacho Flow Config</title>
</head>
<body>
	<div class="container">
		<h1>Configuration Editor</h1>
		<form id="config-form">
			<div id="config-editor"></div>
			<div class="actions">
				<button type="button" id="validate-btn">Validate</button>
				<button type="button" id="apply-btn">Apply Changes</button>
			</div>
		</form>
	</div>
	<script src="${scriptUri}"></script>
</body>
</html>`;
	}

	public dispose(): void {
		this.panel.dispose();
	}
}