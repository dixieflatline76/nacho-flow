import * as vscode from 'vscode';
import { RoutesPanel } from './routes-panel';

// Mock VS Code API
jest.mock('vscode', () => ({
  window: {
    createWebviewPanel: jest.fn().mockReturnValue({
      webview: {
        asWebviewUri: jest.fn().mockImplementation((uri) => `mocked-uri-${uri.path}`),
        html: '',
        onDidReceiveMessage: jest.fn()
      },
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

describe('RoutesPanel', () => {
  let routesPanel: RoutesPanel;
  let mockWebviewPanel: any;
  let mockContext: any;

  beforeEach(() => {
    // Reset mocks
    jest.clearAllMocks();

    // Create mock webview panel
    mockWebviewPanel = {
      webview: {
        asWebviewUri: jest.fn().mockImplementation((uri) => `mocked-uri-${uri.path}`),
        html: '',
        onDidReceiveMessage: jest.fn()
      },
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

    // Create RoutesPanel instance
    routesPanel = new RoutesPanel(mockContext.extensionUri);
  });

  describe('constructor', () => {
    it('should create a webview panel with correct properties', () => {
      expect(vscode.window.createWebviewPanel).toHaveBeenCalledWith(
        'nachoFlowRoutes',
        'Nacho Flow Routes',
        vscode.ViewColumn.One,
        {
          enableScripts: true,
          retainContextWhenHidden: true
        }
      );
    });

    it('should set the HTML content for the webview', () => {
      expect(mockWebviewPanel.webview.html).toContain('<!DOCTYPE html>');
      expect(mockWebviewPanel.webview.html).toContain('<title>Nacho Flow Routes</title>');
      expect(mockWebviewPanel.webview.html).toContain('mocked-uri-[object Object]/resources/webview/routes.js');
      expect(mockWebviewPanel.webview.html).toContain('mocked-uri-[object Object]/resources/webview/routes.css');
    });

    it('should set up message handling and respond to refresh', async () => {
      expect(mockWebviewPanel.webview.onDidReceiveMessage).toHaveBeenCalled();
      const messageHandler = mockWebviewPanel.webview.onDidReceiveMessage.mock.calls[0][0];
      const loadRoutesSpy = jest.spyOn(routesPanel as any, 'loadRoutes');
      
      messageHandler({ command: 'refresh' });
      expect(loadRoutesSpy).toHaveBeenCalled();
      await (routesPanel as any).loadRoutes();
    });
  });

  describe('dispose', () => {
    it('should dispose the webview panel and disposables', () => {
      // Create a mock disposable
      const mockDisposable = { dispose: jest.fn() };
      (routesPanel as any).disposables = [mockDisposable];

      routesPanel.dispose();

      expect(mockWebviewPanel.dispose).toHaveBeenCalled();
      expect(mockDisposable.dispose).toHaveBeenCalled();
    });
  });
});