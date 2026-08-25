/**
 * Test to verify SSE streaming connectivity to Nacho Flow daemon
 * Uses Node.js built-in modules only
 */

const http = require('http');
const https = require('https');
const { URL } = require('url');

class SSETest {
    constructor() {
        this.baseUrl = 'http://192.168.0.205:8000';
        this.authToken = 'sk-nacho-gateway-token'; // Correct auth token
        this.request = null;
        this.buffer = '';
        this.eventCount = 0;
        this.maxEvents = 10; // Stop after 10 events
    }
    
    connect() {
        console.log('Connecting to SSE stream...');
        console.log(`URL: ${this.baseUrl}/api/v1/events`);
        console.log(`Auth Token: ${this.authToken}\n`);
        
        try {
            const url = new URL('/api/v1/events', this.baseUrl);
            
            const requestOptions = {
                hostname: url.hostname,
                port: url.port,
                path: url.pathname + url.search,
                method: 'GET',
                headers: {
                    'Authorization': `Bearer ${this.authToken}`,
                    'Accept': 'text/event-stream',
                    'Cache-Control': 'no-cache',
                    'Connection': 'keep-alive'
                }
            };
            
            this.request = http.request(requestOptions, (res) => {
                console.log(`✅ SSE connection established (Status: ${res.statusCode})`);
                console.log(`Content-Type: ${res.headers['content-type']}\n`);
                
                res.on('data', (chunk) => {
                    this.handleSSEData(chunk.toString());
                });
                
                res.on('end', () => {
                    console.log('SSE connection closed by server');
                });
                
                res.on('error', (error) => {
                    console.error('SSE response error:', error.message);
                });
            });
            
            this.request.on('error', (error) => {
                console.error('SSE request error:', error.message);
            });
            
            this.request.setTimeout(0); // No timeout for SSE
            this.request.end();
            
        } catch (error) {
            console.error('SSE connection failed:', error.message);
        }
    }

    handleSSEData(data) {
        // Buffer incoming data
        this.buffer += data;
        
        // Process complete events
        let newlineIndex;
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
            
            // Handle the event
            if (eventType && eventData) {
                this.handleEvent(eventType, eventData);
            } else if (eventType === 'ping') {
                console.log(`📡 Keep-alive ping received`);
            }
        }
    }

    handleEvent(eventType, eventData) {
        this.eventCount++;
        
        try {
            const parsedData = JSON.parse(eventData);
            const timestamp = new Date().toISOString();
            
            console.log(`🔔 [${timestamp}] SSE Event ${this.eventCount}: ${eventType}`);
            
            switch (eventType) {
                case 'route_completed':
                    console.log(`   Request ID: ${parsedData.request_id}`);
                    console.log(`   Tier: ${parsedData.selected_tier}`);
                    console.log(`   Tokens: ${parsedData.tokens}`);
                    console.log(`   Latency: ${parsedData.latency_ms}ms`);
                    console.log(`   Cost Saved: $${parsedData.cost_saved_usd}`);
                    break;
                    
                case 'circuit_state_changed':
                    console.log(`   Provider: ${parsedData.provider}`);
                    console.log(`   State: ${parsedData.state}`);
                    console.log(`   Failures: ${parsedData.failures}`);
                    break;
                    
                case 'config_updated':
                    console.log(`   Timestamp: ${parsedData.timestamp}`);
                    console.log(`   Version: ${parsedData.version}`);
                    break;
                    
                default:
                    console.log(`   Data: ${JSON.stringify(parsedData, null, 2)}`);
            }
            
            console.log('');
            
            // Stop after max events
            if (this.eventCount >= this.maxEvents) {
                console.log(`Reached ${this.maxEvents} events, disconnecting...`);
                this.disconnect();
                process.exit(0);
            }
            
        } catch (error) {
            console.error('Failed to parse SSE event data:', error.message);
            console.log(`   Raw data: ${eventData}`);
            console.log('');
        }
    }

    disconnect() {
        if (this.request) {
            this.request.destroy();
            this.request = null;
        }
        console.log('SSE connection disconnected');
    }
}

// Run the SSE test
console.log('=== Nacho Flow SSE Streaming Test ===\n');

const sseTest = new SSETest();
sseTest.connect();

// Keep the process alive for 60 seconds to receive events
const timeout = setTimeout(() => {
    console.log('60 seconds elapsed, disconnecting...');
    sseTest.disconnect();
    process.exit(0);
}, 60000);

// Handle graceful shutdown
process.on('SIGINT', () => {
    console.log('\nReceived SIGINT, disconnecting...');
    clearTimeout(timeout);
    sseTest.disconnect();
    process.exit(0);
});

console.log('Listening for SSE events for up to 60 seconds or 10 events...\n');