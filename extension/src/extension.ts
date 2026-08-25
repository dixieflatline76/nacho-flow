import * as vscode from 'vscode';
import { ExtensionController } from './core/controller';

let controller: ExtensionController | undefined;

export async function activate(context: vscode.ExtensionContext) {
	console.log('Nacho Flow extension is activating...');
	
	controller = new ExtensionController(context);
	await controller.initialize();
	
	console.log('Nacho Flow extension is now active!');
}

export function deactivate() {
	console.log('Nacho Flow extension is deactivating...');
	if (controller) {
		controller.dispose();
	}
}