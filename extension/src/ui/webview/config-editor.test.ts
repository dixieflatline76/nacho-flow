import * as vscode from 'vscode';
import { ConfigEditorPanel } from './config-editor';

// Mock VS Code API
jest.mock('vscode', () => ({
  window: {
    createWebviewPanel: jest.fn().mockReturnValue({
      webview: {
        asWebviewUri: jest.fn().mockImplementation((uri) => `mocked-uri-${uri.path}`),
        html: ''
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

describe('ConfigEditorPanel', () => {
  let configEditorPanel: ConfigEditorPanel;
  let mockWebviewPanel: any;
  let mockContext: any;

  beforeEach(() => {
    // Reset mocks
    jest.clearAllMocks();

    // Create mock webview panel
    mockWebviewPanel = {
      webview: {
        asWebviewUri: jest.fn().mockImplementation((uri) => `mocked-uri-${uri.path}`),
        html: ''
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

    // Create ConfigEditorPanel instance
    configEditorPanel = new ConfigEditorPanel(mockContext.extensionUri);
  });

  describe('constructor', () => {
    it('should create a webview panel with correct properties', () => {
      expect(vscode.window.createWebviewPanel).toHaveBeenCalledWith(
        'nachoFlowConfig',
        'Nacho Flow Config',
        vscode.ViewColumn.One,
        {
          enableScripts: true,
          retainContextWhenHidden: true
        }
      );
    });

    it('should set the HTML content for the webview', () => {
      expect(mockWebviewPanel.webview.html).toContain('<!DOCTYPE html>');
      expect(mockWebviewPanel.webview.html).toContain('<title>Nacho Flow Config</title>');
      expect(mockWebviewPanel.webview.html).toContain('mocked-uri-[object Object]/resources/webview/config.js');
      expect(mockWebviewPanel.webview.html).toContain('mocked-uri-[object Object]/resources/webview/config.css');
    });
  });

  describe('dispose', () => {
    it('should dispose the webview panel', () => {
      configEditorPanel.dispose();
      expect(mockWebviewPanel.dispose).toHaveBeenCalled();
    });
  });
});