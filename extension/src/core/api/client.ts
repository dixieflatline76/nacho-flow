import * as http from 'http';
import * as https from 'https';
import { URL } from 'url';

/**
 * Real HTTP Client for Nacho Flow VS Code Extension
 * Uses Node.js built-in http/https modules for actual communication
 */

export class RestClient {
    private baseUrl: string;
    private authToken?: string;

    constructor(baseUrl: string, authToken?: string) {
        this.baseUrl = baseUrl;
        this.authToken = authToken;
    }

    private async request<T>(endpoint: string, options: any = {}): Promise<T> {
        const url = new URL(endpoint, this.baseUrl);
        const isHttps = url.protocol === 'https:';
        
        const headers: any = {
            'Content-Type': options.contentType || 'application/json',
            ...options.headers
        };
        if (this.authToken) {
            headers['Authorization'] = `Bearer ${this.authToken}`;
        }

        return new Promise((resolve, reject) => {
            const requestOptions: any = {
                hostname: url.hostname,
                port: url.port || (isHttps ? 443 : 80),
                path: url.pathname + url.search,
                method: options.method || 'GET',
                headers
            };

            const client = isHttps ? https : http;
            
            const req = client.request(requestOptions, (res) => {
                let data = '';
                
                res.on('data', (chunk) => {
                    data += chunk;
                });
                
                res.on('end', () => {
                    if (res.statusCode && res.statusCode >= 200 && res.statusCode < 300) {
                        const contentType = res.headers['content-type'] || '';
                        if (options.rawText || contentType.includes('yaml') || contentType.includes('text/plain')) {
                            resolve(data as unknown as T);
                            return;
                        }
                        try {
                            const jsonData = JSON.parse(data);
                            resolve(jsonData as T);
                        } catch (parseError) {
                            reject(new Error(`Failed to parse JSON response: ${parseError}`));
                        }
                    } else {
                        reject(new Error(`HTTP ${res.statusCode}: ${res.statusMessage || data}`));
                    }
                });
            });
            
            req.on('error', (error) => {
                reject(new Error(`Request failed: ${error.message}`));
            });
            
            req.on('timeout', () => {
                req.destroy();
                reject(new Error('Request timeout'));
            });
            
            if (options.body) {
                req.write(options.body);
            }
            
            req.end();
        });
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
        const body = provider ? JSON.stringify({ provider }) : undefined;
        return this.request('/api/v1/circuits/reset', {
            method: 'POST',
            body: body
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

    // Config YAML endpoint
    public async getConfigYaml(): Promise<string> {
        return this.request<string>('/api/v1/config?format=yaml', {
            headers: { Accept: 'application/x-yaml' },
            rawText: true
        });
    }

    // Update config endpoint (JSON or object)
    public async updateConfig(config: any, dryRun: boolean = false): Promise<any> {
        const query = dryRun ? '?dry_run=true' : '';
        return this.request(`/api/v1/config${query}`, {
            method: 'PUT',
            body: JSON.stringify(config)
        });
    }

    // Update config endpoint with raw YAML string
    public async updateConfigYaml(yamlContent: string, dryRun: boolean = false): Promise<any> {
        const query = dryRun ? '?dry_run=true' : '';
        return this.request(`/api/v1/config${query}`, {
            method: 'PUT',
            contentType: 'application/x-yaml',
            body: yamlContent
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

    // Heatseeker Deals endpoint
    public async getDeals(): Promise<any> {
        return this.request('/api/v1/deals');
    }

    // Send administrative command directive
    public async sendDirective(action: string, payload?: any): Promise<{
        status: string;
        action: string;
        requires_restart: boolean;
        message?: string;
        details?: any;
    }> {
        return this.request('/api/v1/directive', {
            method: 'POST',
            body: JSON.stringify({ action, payload })
        });
    }

    // Reset stats endpoint (dispatches PURGE_ALL_LOGS directive)
    public async resetStats(): Promise<any> {
        return this.sendDirective('PURGE_ALL_LOGS');
    }

    // Recalculate stats endpoint
    public async recalculateStats(): Promise<any> {
        return this.request('/api/v1/stats/recalculate', {
            method: 'POST'
        });
    }

    // Health check endpoint
    public async getHealth(): Promise<any> {
        return this.request('/health');
    }
}