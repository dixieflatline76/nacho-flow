import * as http from 'http';
import * as https from 'https';
import { URL } from 'url';

/**
 * Real SSE Client for Nacho Flow VS Code Extension
 * Uses Node.js built-in http/https modules for actual SSE streaming
 */

export type SSEEventCallback = (data: any) => void;

export class SSEClient {
    private baseUrl: string;
    private authToken?: string;
    private listeners: Map<string, SSEEventCallback[]> = new Map();
    private isConnected: boolean = false;
    private reconnectAttempts: number = 0;
    private maxReconnectAttempts: number = 5;
    private reconnectDelay: number = 1000;
    private request: http.ClientRequest | null = null;
    private response: http.IncomingMessage | null = null;
    private reconnectTimer: NodeJS.Timeout | null = null;
    private buffer: string = '';

    constructor(baseUrl: string, authToken?: string) {
        this.baseUrl = baseUrl;
        this.authToken = authToken;
    }

    public connect(): void {
        try {
            const url = new URL('/api/v1/events', this.baseUrl);
            const isHttps = url.protocol === 'https:';
            
            const headers: any = {
                'Accept': 'text/event-stream',
                'Cache-Control': 'no-cache',
                'Connection': 'keep-alive'
            };
            if (this.authToken) {
                headers['Authorization'] = `Bearer ${this.authToken}`;
            }

            const requestOptions: any = {
                hostname: url.hostname,
                port: url.port || (isHttps ? 443 : 80),
                path: url.pathname + url.search,
                method: 'GET',
                headers
            };
            
            const client = isHttps ? https : http;
            
            this.request = client.request(requestOptions, (res) => {
                this.response = res;
                this.isConnected = true;
                this.reconnectAttempts = 0;
                this.buffer = '';
                
                res.on('data', (chunk) => {
                    this.handleSSEData(chunk.toString());
                });
                
                res.on('end', () => {
                    this.isConnected = false;
                    this.handleConnectionError(new Error('SSE connection closed'));
                });
                
                res.on('error', (error) => {
                    this.handleConnectionError(error);
                });
            });
            
            this.request.on('error', (error) => {
                this.handleConnectionError(error);
            });
            
            this.request.on('timeout', () => {
                if (this.request) {
                    this.request.destroy();
                }
                this.handleConnectionError(new Error('SSE connection timeout'));
            });
            
            this.request.setTimeout(0); // No timeout for SSE
            this.request.end();
            
        } catch (error) {
            this.handleConnectionError(error);
        }
    }

    private handleSSEData(data: string): void {
        // Buffer incoming data
        this.buffer += data;
        
        // Process complete events
        let newlineIndex: number;
        while ((newlineIndex = this.buffer.indexOf('\n\n')) !== -1) {
            const eventBlock = this.buffer.substring(0, newlineIndex);
            this.buffer = this.buffer.substring(newlineIndex + 2);
            
            // Parse the event block
            const lines = eventBlock.split('\n');
            let eventType = '';
            let eventData = '';
            
            for (const line of lines) {
                if (line.startsWith('event:')) {
                    eventType = line.substring(6).trim();
                } else if (line.startsWith('data:')) {
                    eventData = line.substring(5).trim();
                } else if (line.startsWith(':')) {
                    // Comment line, ignore
                    continue;
                }
            }
            
            // Emit the event if we have both type and data
            if (eventType && eventData) {
                try {
                    const parsedData = JSON.parse(eventData);
                    this.emit(eventType, parsedData);
                } catch (error) {
                    console.error('Failed to parse SSE event data:', error);
                }
            }
        }
    }

    public subscribe(event: string, callback: SSEEventCallback): void {
        if (!this.listeners.has(event)) {
            this.listeners.set(event, []);
        }
        this.listeners.get(event)?.push(callback);
    }

    public unsubscribe(event: string, callback: SSEEventCallback): void {
        const callbacks = this.listeners.get(event);
        if (callbacks) {
            const index = callbacks.indexOf(callback);
            if (index > -1) {
                callbacks.splice(index, 1);
            }
        }
    }

    private emit(event: string, data: any): void {
        const callbacks = this.listeners.get(event);
        if (callbacks) {
            callbacks.forEach(callback => {
                try {
                    callback(data);
                } catch (error) {
                    console.error(`Error in SSE callback for event ${event}:`, error);
                }
            });
        }
    }

    private handleConnectionError(error: any): void {
        console.error('SSE connection error:', error.message);
        this.isConnected = false;
        this.reconnect();
    }

    private reconnect(): void {
        if (this.reconnectAttempts >= this.maxReconnectAttempts) {
            console.error('Max reconnect attempts reached, giving up');
            return;
        }

        this.reconnectTimer = setTimeout(() => {
            this.reconnectAttempts++;
            console.log(`Attempting to reconnect (${this.reconnectAttempts}/${this.maxReconnectAttempts})`);
            this.connect();
        }, this.reconnectDelay * Math.pow(2, this.reconnectAttempts));
    }

    public disconnect(): void {
        if (this.reconnectTimer) {
            clearTimeout(this.reconnectTimer);
            this.reconnectTimer = null;
        }
        if (this.request) {
            this.request.destroy();
            this.request = null;
        }
        if (this.response) {
            this.response.destroy();
            this.response = null;
        }
        this.isConnected = false;
        this.buffer = '';
    }

    public getConnectionStatus(): { connected: boolean; attempts: number } {
        return {
            connected: this.isConnected,
            attempts: this.reconnectAttempts
        };
    }
}