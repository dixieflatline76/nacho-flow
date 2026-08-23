# 🌮 Nacho Flow VS Code Extension - Implementation Plan

## Overview

This document outlines the implementation plan for the Nacho Flow VS Code Extension, a thin-client UI companion for the `nacho-flow` AI routing gateway. The extension provides real-time cost visibility, route inspection, circuit breaker management, and interactive configuration editing directly inside VS Code.

## 1. Project Structure & Build Tooling

### 1.1 Directory Structure
```
extension/
├── src/
│   ├── core/                 # Core extension logic
│   │   ├── api/              # REST API clients
│   │   ├── sse/              # SSE event handling
│   │   ├── config/           # Configuration management
│   │   └── types/            # TypeScript type definitions
│   ├── ui/                   # UI components
│   │   ├── status-bar/       # Status bar item
│   │   ├── webview/          # Webview panels
│   │   │   ├── dashboard/    # Main dashboard
│   │   │   ├── routes/       # Route history panel
│   │   │   ├── circuits/     # Circuit breaker panel
│   │   │   └── config/       # Configuration editor
│   │   └── components/       # Reusable UI components
│   └── utils/                # Utility functions
├── resources/                # Icons, webview assets
├── test/                     # Unit and integration tests
├── package.json              # Extension manifest
├── tsconfig.json             # TypeScript configuration
├── esbuild.config.js         # ESBuild configuration
└── README.md                 # Extension documentation
```

### 1.2 Build Tooling
- **TypeScript** as the primary language
- **ESBuild** for fast bundling and minification
- **VS Code Extension API** for UI components
- **Node.js** runtime (bundled with VS Code)

### 1.3 Dependencies
```json
{
  "devDependencies": {
    "@types/vscode": "^1.80.0",
    "@types/node": "16.x",
    "typescript": "^5.0.0",
    "esbuild": "^0.19.0",
    "@vscode/test-electron": "^2.3.0",
    "@vscode/vsce": "^2.19.0"
  },
  "dependencies": {
    "eventsource": "^2.0.2"
  }
}
```

### 1.4 Build Scripts
```json
{
  "scripts": {
    "compile": "tsc -p ./",
    "watch": "tsc -w -p ./",
    "package": "vsce package",
    "publish": "vsce publish",
    "test": "node ./out/test/runTest.js"
  }
}
```

## 2. SSE Client & Connection Health State Machine

### 2.1 SSE Client Implementation
The SSE client will connect to `/api/v1/events` endpoint to receive real-time updates:

```typescript
// src/core/sse/client.ts
export class SSEClient {
  private eventSource: EventSource | null = null;
  private baseUrl: string;
  private authToken: string;
  private listeners: Map<string, ((data: any) => void)[]> = new Map();
  private reconnectAttempts: number = 0;
  private maxReconnectAttempts: number = 5;
  private reconnectDelay: number = 1000;

  constructor(baseUrl: string, authToken: string) {
    this.baseUrl = baseUrl;
    this.authToken = authToken;
  }

  public connect(): void {
    const url = `${this.baseUrl}/api/v1/events`;
    const headers = {
      'Authorization': `Bearer ${this.authToken}`,
      'Accept': 'text/event-stream'
    };

    // Create EventSource with custom headers (requires polyfill or custom implementation)
    this.eventSource = new EventSource(url, {
      headers: headers
    });

    this.setupEventListeners();
  }

  private setupEventListeners(): void {
    if (!this.eventSource) return;

    this.eventSource.addEventListener('route_completed', (event) => {
      this.emit('routeCompleted', JSON.parse(event.data));
    });

    this.eventSource.addEventListener('circuit_state_changed', (event) => {
      this.emit('circuitStateChanged', JSON.parse(event.data));
    });

    this.eventSource.addEventListener('config_updated', (event) => {
      this.emit('configUpdated', JSON.parse(event.data));
    });

    this.eventSource.onerror = (error) => {
      this.handleConnectionError(error);
    };
  }

  public subscribe(event: string, callback: (data: any) => void): void {
    if (!this.listeners.has(event)) {
      this.listeners.set(event, []);
    }
    this.listeners.get(event)?.push(callback);
  }

  private emit(event: string, data: any): void {
    const callbacks = this.listeners.get(event);
    if (callbacks) {
      callbacks.forEach(callback => callback(data));
    }
  }

  private handleConnectionError(error: any): void {
    console.error('SSE connection error:', error);
    this.reconnect();
  }

  private reconnect(): void {
    if (this.reconnectAttempts >= this.maxReconnectAttempts) {
      console.error('Max reconnect attempts reached');
      return;
    }

    setTimeout(() => {
      this.reconnectAttempts++;
      this.connect();
    }, this.reconnectDelay * Math.pow(2, this.reconnectAttempts));
  }

  public disconnect(): void {
    if (this.eventSource) {
      this.eventSource.close();
      this.eventSource = null;
    }
  }
}
```

### 2.2 Connection Health State Machine
```typescript
// src/core/sse/connection-state.ts
export enum ConnectionState {
  Disconnected = 'disconnected',
  Connecting = 'connecting',
  Connected = 'connected',
  Error = 'error'
}

export class ConnectionHealthStateMachine {
  private state: ConnectionState = ConnectionState.Disconnected;
  private errorCount: number = 0;
  private maxErrors: number = 3;

  public transitionToConnecting(): void {
    this.state = ConnectionState.Connecting;
    this.notifyStateChange();
  }

  public transitionToConnected(): void {
    this.state = ConnectionState.Connected;
    this.errorCount = 0;
    this.notifyStateChange();
  }

  public transitionToError(): void {
    this.state = ConnectionState.Error;
    this.errorCount++;
    this.notifyStateChange();
    
    if (this.errorCount >= this.maxErrors) {
      this.transitionToDisconnected();
    }
  }

  public transitionToDisconnected(): void {
    this.state = ConnectionState.Disconnected;
    this.notifyStateChange();
  }

  private notifyStateChange(): void {
    // Notify UI components about state change
    // This could trigger status bar updates, etc.
  }

  public getCurrentState(): ConnectionState {
    return this.state;
  }
}
```

## 3. REST Client & Token Security (SecretStorage)

### 3.1 REST Client Implementation
```typescript
// src/core/api/client.ts
import * as vscode from 'vscode';

export class RestClient {
  private baseUrl: string;
  private authToken: string;

  constructor(baseUrl: string, authToken: string) {
    this.baseUrl = baseUrl;
    this.authToken = authToken;
  }

  private async request<T>(endpoint: string, options: RequestInit = {}): Promise<T> {
    const url = `${this.baseUrl}${endpoint}`;
    const headers = {
      'Authorization': `Bearer ${this.authToken}`,
      'Content-Type': 'application/json',
      ...options.headers
    };

    const response = await fetch(url, {
      ...options,
      headers
    });

    if (!response.ok) {
      throw new Error(`HTTP ${response.status}: ${response.statusText}`);
    }

    return response.json();
  }

  // API Info endpoint
  public async getInfo(): Promise<any> {
    return this.request('/api/v1/info');
  }

  // Routes endpoint
  public async getRoutes(limit?: number): Promise<any> {
    const query = limit ? `?limit=${limit}` : '';
    return this.request(`/api/v1/routes${query}`);
  }

  // Circuits endpoint
  public async getCircuits(): Promise<any> {
    return this.request('/api/v1/circuits');
  }

  // Reset circuit endpoint
  public async resetCircuit(provider?: string): Promise<any> {
    const body = provider ? { provider } : undefined;
    return this.request('/api/v1/circuits/reset', {
      method: 'POST',
      body: body ? JSON.stringify(body) : undefined
    });
  }

  // Pricing endpoint
  public async getPricing(): Promise<any> {
    return this.request('/api/v1/pricing');
  }

  // Config endpoint
  public async getConfig(): Promise<any> {
    return this.request('/api/v1/config');
  }

  // Update config endpoint
  public async updateConfig(config: any, dryRun: boolean = false): Promise<any> {
    const query = dryRun ? '?dry_run=true' : '';
    return this.request(`/api/v1/config${query}`, {
      method: 'PUT',
      body: JSON.stringify(config)
    });
  }

  // Tune endpoint
  public async tune(): Promise<any> {
    return this.request('/api/v1/tune', {
      method: 'POST'
    });
  }

  // Stats endpoint
  public async getStats(): Promise<any> {
    return this.request('/v1/stats');
  }
}
```

### 3.2 Token Security with SecretStorage
```typescript
// src/core/config/auth-manager.ts
import * as vscode from 'vscode';

export class AuthManager {
  private static readonly SECRET_STORAGE_KEY = 'nacho-flow.auth-token';
  private context: vscode.ExtensionContext;

  constructor(context: vscode.ExtensionContext) {
    this.context = context;
  }

  public async getAuthToken(): Promise<string | undefined> {
    return this.context.secrets.get(AuthManager.SECRET_STORAGE_KEY);
  }

  public async setAuthToken(token: string): Promise<void> {
    await this.context.secrets.store(AuthManager.SECRET_STORAGE_KEY, token);
  }

  public async deleteAuthToken(): Promise<void> {
    await this.context.secrets.delete(AuthManager.SECRET_STORAGE_KEY);
  }

  public async getBaseUrl(): Promise<string> {
    return vscode.workspace.getConfiguration('nachoFlow').get('daemonUrl', 'http://127.0.0.1:8000');
  }
}
```

## 4. UI Components

### 4.1 Status Bar Item
```typescript
// src/ui/status-bar/item.ts
import * as vscode from 'vscode';

export class StatusBarManager {
  private item: vscode.StatusBarItem;
  private stats: any = null;

  constructor() {
    this.item = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Right, 100);
    this.item.command = 'nacho-flow.showDashboard';
    this.updateStatusBar();
  }

  public updateStats(stats: any): void {
    this.stats = stats;
    this.updateStatusBar();
  }

  private updateStatusBar(): void {
    if (!this.stats) {
      this.item.text = '$(pulse) Nacho: Initializing...';
      this.item.tooltip = 'Connecting to Nacho Flow daemon';
    } else {
      const saved = this.stats.estimated_cost_saved_usd || 0;
      const localPercentage = this.calculateLocalPercentage();
      this.item.text = `$(pulse) Nacho: $${saved.toFixed(2)} saved (${localPercentage}% local)`;
      this.item.tooltip = this.buildTooltip();
    }
    this.item.show();
  }

  private calculateLocalPercentage(): number {
    if (!this.stats) return 0;
    const total = this.stats.total_requests || 0;
    const local = this.stats.tier_breakdown?.tier1_local_free || 0;
    return total > 0 ? Math.round((local / total) * 100) : 0;
  }

  private buildTooltip(): string {
    if (!this.stats) return 'Nacho Flow extension';
    
    return `Nacho Flow Extension
Total Requests: ${this.stats.total_requests || 0}
Local Requests: ${this.stats.tier_breakdown?.tier1_local_free || 0}
Cost Saved: $${(this.stats.estimated_cost_saved_usd || 0).toFixed(2)}
Connected to: ${this.getBaseUrl()}`;
  }

  public dispose(): void {
    this.item.dispose();
  }

  private getBaseUrl(): string {
    return vscode.workspace.getConfiguration('nachoFlow').get('daemonUrl', 'http://127.0.0.1:8000');
  }
}
```

### 4.2 Webview Dashboard

#### 4.2.1 Dashboard Structure
The dashboard will consist of multiple panels:
1. **Overview Panel** - Summary statistics and quick actions
2. **Route History Panel** - Table of recent route completions
3. **Circuit Breaker Panel** - Status cards for each provider
4. **Configuration Editor** - Interactive config editor with validation

#### 4.2.2 Route History Webview
```typescript
// src/ui/webview/routes-panel.ts
import * as vscode from 'vscode';

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
```

#### 4.2.3 Circuit Breaker Webview
```typescript
// src/ui/webview/circuits-panel.ts
import * as vscode from 'vscode';

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
}
```

### 4.3 Config Editor Webview
```typescript
// src/ui/webview/config-editor.ts
import * as vscode from 'vscode';

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
}
```

## 5. Data Models and Interfaces

### 5.1 TypeScript Interfaces
```typescript
// src/core/types/nacho-types.ts

// Route Record
export interface TurnRecord {
  timestamp: string;
  request_id: string;
  tokens: number;
  has_images: boolean;
  has_tools: boolean;
  keywords: string[];
  selected_tier: string;
  target_model: string;
  provider: string;
  is_local: boolean;
  is_fallback: boolean;
  latency_ms: number;
  status_code: number;
  is_retry: boolean;
  cost_saved_usd: number;
}

// Circuit Information
export interface CircuitInfo {
  provider: string;
  name: string;
  state: 'closed' | 'open' | 'half-open';
  failures: number;
  failure_threshold: number;
  cooldown_seconds: number;
  is_available: boolean;
}

// Configuration
export interface ProviderConfig {
  base_url: string;
  api_key?: string;
  type?: 'local' | 'cloud';
  headers?: Record<string, string>;
}

export interface Tier {
  name: string;
  model: string;
  provider: string;
  when: string;
  strip_images?: boolean;
  reasoning_effort?: string;
  max_context?: number;
}

export interface Config {
  port: number;
  auth_token?: string;
  providers: Record<string, ProviderConfig>;
  tiers: Tier[];
  default_tier: Tier;
}

// Stats
export interface TierMetrics {
  tier1_local_free: number;
  tier2_cloud_coder: number;
  tier3_cloud_reasoning: number;
  tier4_cloud_vision: number;
  explicit_override: number;
  fallbacks: number;
}

export interface StatsSnapshot {
  started_at: string;
  total_requests: number;
  tier_breakdown: TierMetrics;
  total_tokens_routed_locally: number;
  estimated_cost_saved_usd: number;
}

// Pricing
export interface ModelPricing {
  prompt_cost_per_million: number;
  completion_cost_per_million: number;
}

// Tuning Result
export interface TuningResult {
  optimal_threshold: number;
  friction_keywords: string[];
  synthesized_rule: string;
  current_cost_usd: number;
  projected_cost_usd: number;
  projected_savings_usd: number;
  retries_eliminated: number;
  total_sample_turns: number;
}
```

## 6. Extension Activation and Main Controller

### 6.1 Extension Entry Point
```typescript
// src/extension.ts
import * as vscode from 'vscode';
import { ExtensionController } from './core/controller';

let controller: ExtensionController;

export async function activate(context: vscode.ExtensionContext) {
  controller = new ExtensionController(context);
  await controller.initialize();
}

export function deactivate() {
  if (controller) {
    controller.dispose();
  }
}
```

### 6.2 Main Controller
```typescript
// src/core/controller.ts
import * as vscode from 'vscode';
import { AuthManager } from './config/auth-manager';
import { RestClient } from './api/client';
import { SSEClient } from './sse/client';
import { StatusBarManager } from '../ui/status-bar/item';
import { RoutesPanel } from '../ui/webview/routes-panel';
import { CircuitsPanel } from '../ui/webview/circuits-panel';
import { ConfigEditorPanel } from '../ui/webview/config-editor';

export class ExtensionController {
  private context: vscode.ExtensionContext;
  private authManager: AuthManager;
  private restClient: RestClient | null = null;
  private sseClient: SSEClient | null = null;
  private statusBar: StatusBarManager;
  private updateInterval: NodeJS.Timeout | null = null;

  constructor(context: vscode.ExtensionContext) {
    this.context = context;
    this.authManager = new AuthManager(context);
    this.statusBar = new StatusBarManager();
  }

  public async initialize(): Promise<void> {
    // Register commands
    this.registerCommands();

    // Initialize clients
    await this.initializeClients();

    // Start periodic updates
    this.startPeriodicUpdates();
  }

  private registerCommands(): void {
    this.context.subscriptions.push(
      vscode.commands.registerCommand('nacho-flow.showDashboard', () => {
        // Show main dashboard
      }),
      
      vscode.commands.registerCommand('nacho-flow.refreshStats', () => {
        this.updateStats();
      }),
      
      vscode.commands.registerCommand('nacho-flow.openConfig', () => {
        // Open config editor
      })
    );
  }

  private async initializeClients(): Promise<void> {
    const baseUrl = await this.authManager.getBaseUrl();
    const authToken = await this.authManager.getAuthToken();
    
    if (authToken) {
      this.restClient = new RestClient(baseUrl, authToken);
      this.sseClient = new SSEClient(baseUrl, authToken);
      this.sseClient.connect();
      
      // Setup SSE event handlers
      this.setupSSEHandlers();
    }
  }

  private setupSSEHandlers(): void {
    if (!this.sseClient) return;
    
    this.sseClient.subscribe('routeCompleted', (data) => {
      // Handle route completion
      this.updateStats();
    });
    
    this.sseClient.subscribe('circuitStateChanged', (data) => {
      // Handle circuit state change
    });
    
    this.sseClient.subscribe('configUpdated', (data) => {
      // Handle config update
      this.updateStats();
    });
  }

  private async updateStats(): Promise<void> {
    if (!this.restClient) return;
    
    try {
      const stats = await this.restClient.getStats();
      this.statusBar.updateStats(stats);
    } catch (error) {
      console.error('Failed to update stats:', error);
    }
  }

  private startPeriodicUpdates(): void {
    this.updateInterval = setInterval(() => {
      this.updateStats();
    }, 30000); // Update every 30 seconds
  }

  public dispose(): void {
    if (this.updateInterval) {
      clearInterval(this.updateInterval);
    }
    this.statusBar.dispose();
    if (this.sseClient) {
      this.sseClient.disconnect();
    }
  }
}
```

## 7. Local Testing and Verification Plan

### 7.1 Test Environment Setup
1. **Local Development Environment**
   - VS Code with extension development host
   - Nacho Flow daemon running locally
   - Test configuration file with multiple providers

2. **Mock Server for API Testing**
   - Create a mock server that simulates the Nacho Flow API endpoints
   - Use libraries like `express` or `json-server` for quick mocking

### 7.2 Unit Testing Strategy
```typescript
// test/unit/api-client.test.ts
import * as assert from 'assert';
import { RestClient } from '../../src/core/api/client';

suite('RestClient Tests', () => {
  test('should create valid request URL', () => {
    const client = new RestClient('http://localhost:8000', 'test-token');
    // Add assertions for URL construction
  });

  test('should handle authentication correctly', () => {
    // Test auth header inclusion
  });
});
```

### 7.3 Integration Testing Plan
1. **SSE Connection Testing**
   - Verify successful connection to `/api/v1/events`
   - Test event reception and parsing
   - Validate reconnection logic

2. **REST API Testing**
   - Test all endpoint interactions
   - Validate response parsing
   - Check error handling

3. **UI Component Testing**
   - Status bar updates
   - Webview rendering
   - User interaction flows

### 7.4 Manual Testing Scenarios
1. **Daemon Connectivity**
   - Extension starts when daemon is running
   - Extension handles daemon downtime gracefully
   - Extension reconnects when daemon comes back online

2. **Authentication Flow**
   - Token storage and retrieval
   - Secure handling of sensitive data
   - Configuration persistence

3. **Real-time Updates**
   - Route completion events update UI
   - Circuit breaker state changes reflected immediately
   - Configuration updates propagate correctly

4. **Edge Cases**
   - Network interruptions
   - Invalid configuration states
   - High-frequency event streams

## 8. Deployment and Distribution

### 8.1 Packaging
- Use `vsce` to package the extension
- Ensure all assets are properly included
- Validate package size and content

### 8.2 Publishing
- Publish to VS Code Marketplace
- Set up automated builds with GitHub Actions
- Version management aligned with Nacho Flow releases

## 9. Future Enhancements

### 9.1 Planned Features
1. **Enhanced Visualization**
   - Charts for cost savings over time
   - Provider performance metrics
   - Tier utilization graphs

2. **Advanced Configuration Editor**
   - Syntax highlighting for expr rules
   - Real-time validation and error checking
   - Configuration templates and examples

3. **Notification System**
   - Alerts for circuit breaker trips
   - Cost saving milestones
   - Configuration change notifications

### 9.2 Performance Optimizations
1. **Memory Management**
   - Efficient event handling
   - Proper disposal of resources
   - Caching strategies for frequently accessed data

2. **Network Efficiency**
   - Connection pooling
   - Request batching where appropriate
   - Intelligent polling intervals

This implementation plan provides a comprehensive roadmap for developing the Nacho Flow VS Code Extension as a pure presentation and control layer that communicates with the daemon exclusively via HTTP REST and SSE streams, maintaining strict separation from the backend logic.