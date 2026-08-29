import * as vscode from 'vscode';
import { DashboardPanel } from './dashboard';

// Mock VS Code API
jest.mock('vscode', () => ({
  window: {
    createWebviewPanel: jest.fn().mockReturnValue({
      webview: {
        asWebviewUri: jest.fn().mockImplementation((uri) => `mocked-uri-${uri.path}`),
        html: '',
        postMessage: jest.fn(),
        onDidReceiveMessage: jest.fn()
      },
      onDidDispose: jest.fn(),
      dispose: jest.fn()
    })
  },
  ViewColumn: {
    One: 1
  },
  Uri: {
    joinPath: jest.fn().mockImplementation((...paths) => ({
      path: paths.join('/')
    }))
  }
}), { virtual: true });

describe('DashboardPanel', () => {
  let dashboardPanel: DashboardPanel;
  let mockWebviewPanel: any;
  let mockContext: any;

  beforeEach(() => {
    // Reset mocks
    jest.clearAllMocks();

    // Create mock webview panel
    mockWebviewPanel = {
      visible: true,
      webview: {
        asWebviewUri: jest.fn().mockImplementation((uri) => `mocked-uri-${uri.path}`),
        html: '',
        postMessage: jest.fn(),
        onDidReceiveMessage: jest.fn()
      },
      onDidChangeViewState: jest.fn(),
      onDidDispose: jest.fn(),
      dispose: jest.fn()
    };

    // Mock createWebviewPanel to return our mock
    (vscode.window.createWebviewPanel as jest.Mock).mockReturnValue(mockWebviewPanel);

    // Create mock context
    mockContext = {
      extensionUri: {
        path: '/test/extension'
      }
    };

    // Create DashboardPanel instance
    dashboardPanel = new DashboardPanel(mockContext.extensionUri);
  });

  describe('constructor', () => {
    it('should create a webview panel with correct properties', () => {
      expect(vscode.window.createWebviewPanel).toHaveBeenCalledWith(
        'nachoFlowDashboard',
        'Nacho Flow Dashboard',
        vscode.ViewColumn.One,
        {
          enableScripts: true,
          retainContextWhenHidden: true
        }
      );
    });

    it('should set the HTML content for the webview with auto-refresh controls', () => {
      expect(mockWebviewPanel.webview.html).toContain('<!DOCTYPE html>');
      expect(mockWebviewPanel.webview.html).toContain('<title>Nacho Flow Dashboard</title>');
      expect(mockWebviewPanel.webview.html).toContain('mocked-uri-[object Object]/resources/webview/dashboard.js');
      expect(mockWebviewPanel.webview.html).toContain('mocked-uri-[object Object]/resources/webview/dashboard.css');
      expect(mockWebviewPanel.webview.html).toContain('id="refresh-60s"');
      expect(mockWebviewPanel.webview.html).toContain('id="refresh-30s"');
      expect(mockWebviewPanel.webview.html).toContain('id="refresh-15s"');
      expect(mockWebviewPanel.webview.html).toContain('id="refresh-off"');
      expect(mockWebviewPanel.webview.html).toContain('id="btn-open-settings"');
      expect(mockWebviewPanel.webview.html).toContain('control-center-section');
      expect(mockWebviewPanel.webview.html).toContain('cycle-killer-panel');
      expect(mockWebviewPanel.webview.html).toContain('id="cycle-killer-content"');
    });

    it('should set up message listener when onMessage is provided', () => {
      mockWebviewPanel.webview.onDidReceiveMessage = jest.fn();
      const onMsg = jest.fn();
      const panel = new DashboardPanel(mockContext.extensionUri, onMsg);

      expect(mockWebviewPanel.webview.onDidReceiveMessage).toHaveBeenCalledWith(onMsg, null, expect.any(Array));
    });
  });

  describe('update methods', () => {
    beforeEach(() => {
      mockWebviewPanel.webview.postMessage = jest.fn();
    });

    it('should post updateStats message', () => {
      dashboardPanel.updateStats({ total_requests: 42 });
      expect(mockWebviewPanel.webview.postMessage).toHaveBeenCalledWith({
        command: 'updateStats',
        data: { total_requests: 42 }
      });
    });

    it('should post updateRoutes message', () => {
      dashboardPanel.updateRoutes({ routes: [] });
      expect(mockWebviewPanel.webview.postMessage).toHaveBeenCalledWith({
        command: 'updateRoutes',
        data: { routes: [] }
      });
    });

    it('should post setRoutesRefreshInterval message', () => {
      dashboardPanel.setRoutesRefreshInterval(30);
      expect(mockWebviewPanel.webview.postMessage).toHaveBeenCalledWith({
        command: 'setRoutesRefreshInterval',
        data: { interval: 30 }
      });
    });

    it('should expose isVisible and onDidChangeViewState getters', () => {
      expect(dashboardPanel.isVisible).toBe(true);
      expect(dashboardPanel.onDidChangeViewState).toBe(mockWebviewPanel.onDidChangeViewState);
    });

    it('should post updateCircuits message', () => {
      dashboardPanel.updateCircuits({ circuits: [] });
      expect(mockWebviewPanel.webview.postMessage).toHaveBeenCalledWith({
        command: 'updateCircuits',
        data: { circuits: [] }
      });
    });

    it('should post updateConfig message', () => {
      dashboardPanel.updateConfig({ port: 8000 });
      expect(mockWebviewPanel.webview.postMessage).toHaveBeenCalledWith({
        command: 'updateConfig',
        data: { port: 8000 }
      });
    });

    it('should post updateDeals message', () => {
      dashboardPanel.updateDeals({ deals: [] });
      expect(mockWebviewPanel.webview.postMessage).toHaveBeenCalledWith({
        command: 'updateDeals',
        data: { deals: [] }
      });
    });

    it('should post updateOptimization message', () => {
      dashboardPanel.updateOptimization({ message: 'Optimized' });
      expect(mockWebviewPanel.webview.postMessage).toHaveBeenCalledWith({
        command: 'updateOptimization',
        data: { message: 'Optimized' }
      });
    });

    it('should safely swallow error if postMessage throws when disposed', () => {
      mockWebviewPanel.webview.postMessage.mockImplementationOnce(() => {
        throw new Error('Webview is disposed');
      });
      expect(() => dashboardPanel.updateStats({ total_requests: 1 })).not.toThrow();
      // Subsequent calls do not call postMessage
      dashboardPanel.updateDeals({ deals: [] });
    });
  });

  describe('dispose', () => {
    it('should dispose the webview panel and invoke onDispose callback', () => {
      const onDisposeMock = jest.fn();
      let disposeHandler: any;
      mockWebviewPanel.onDidDispose.mockImplementationOnce((cb: any) => {
        disposeHandler = cb;
      });

      const panel = new DashboardPanel(mockContext.extensionUri, undefined, onDisposeMock);
      expect(mockWebviewPanel.onDidDispose).toHaveBeenCalled();
      
      if (disposeHandler) {
        disposeHandler();
      }
      expect(onDisposeMock).toHaveBeenCalled();
      expect(mockWebviewPanel.dispose).toHaveBeenCalled();
    });
  });
});