import * as vscode from 'vscode';
import { SidebarViewProvider } from './sidebar-view-provider';

// Mock VS Code API
jest.mock('vscode', () => ({
  Uri: {
    joinPath: jest.fn().mockImplementation((...paths) => ({
      path: paths.join('/')
    }))
  }
}), { virtual: true });

describe('SidebarViewProvider', () => {
  let provider: SidebarViewProvider;
  let mockWebviewView: any;
  let mockExtensionUri: any;

  beforeEach(() => {
    jest.clearAllMocks();

    mockWebviewView = {
      webview: {
        options: {},
        html: '',
        asWebviewUri: jest.fn().mockImplementation((uri) => `mocked-uri-${uri.path}`),
        postMessage: jest.fn(),
        onDidReceiveMessage: jest.fn()
      }
    };

    mockExtensionUri = { path: '/mock/extension/path' };
    provider = new SidebarViewProvider(mockExtensionUri);
  });

  describe('resolveWebviewView', () => {
    it('should set webview options, HTML, and register message listener', () => {
      const messageHandler = jest.fn();
      const testProvider = new SidebarViewProvider(mockExtensionUri, messageHandler);

      testProvider.resolveWebviewView(mockWebviewView, {} as any, {} as any);

      expect(mockWebviewView.webview.options.enableScripts).toBe(true);
      expect(mockWebviewView.webview.html).toContain('<!DOCTYPE html>');
      expect(mockWebviewView.webview.html).toContain('🌮 Nacho Flow');
      expect(mockWebviewView.webview.html).toContain('qwen2.5-coder:14b');
      expect(mockWebviewView.webview.onDidReceiveMessage).toHaveBeenCalledWith(messageHandler);
    });
  });

  describe('update methods', () => {
    beforeEach(() => {
      provider.resolveWebviewView(mockWebviewView, {} as any, {} as any);
    });

    it('should post updateState message', () => {
      provider.updateState({ engineMode: 'local', remoteUrl: 'http://localhost:8000' });
      expect(mockWebviewView.webview.postMessage).toHaveBeenCalledWith({
        command: 'updateState',
        data: { engineMode: 'local', remoteUrl: 'http://localhost:8000' }
      });
    });

    it('should post updateEngineStatus message', () => {
      provider.updateEngineStatus({ connected: true, version: '0.6.0' });
      expect(mockWebviewView.webview.postMessage).toHaveBeenCalledWith({
        command: 'updateEngineStatus',
        data: { connected: true, version: '0.6.0' }
      });
    });

    it('should post updateOllamaStatus message', () => {
      provider.updateOllamaStatus({ connected: true, models: ['qwen2.5-coder:14b'] });
      expect(mockWebviewView.webview.postMessage).toHaveBeenCalledWith({
        command: 'updateOllamaStatus',
        data: { connected: true, models: ['qwen2.5-coder:14b'] }
      });
    });

    it('should safely swallow error if postMessage throws', () => {
      mockWebviewView.webview.postMessage.mockImplementationOnce(() => {
        throw new Error('Disposed');
      });
      expect(() => provider.updateState({ test: true })).not.toThrow();
    });

    it('should safely handle update when view is not resolved', () => {
      const freshProvider = new SidebarViewProvider(mockExtensionUri);
      expect(() => freshProvider.updateState({ test: true })).not.toThrow();
      expect(() => freshProvider.updateEngineStatus({ test: true })).not.toThrow();
      expect(() => freshProvider.updateOllamaStatus({ test: true })).not.toThrow();
    });
  });
});
