/**
 * Simple test to verify authenticated connectivity to Nacho Flow daemon
 * Uses Node.js built-in modules only
 */

const http = require('http');
const https = require('https');
const { URL } = require('url');

class ConnectivityTest {
    constructor() {
        this.baseUrl = 'http://192.168.0.205:8000';
        this.authToken = 'sk-nacho-gateway-token'; // Correct auth token
    }
    
    async testEndpoint(endpoint, description) {
        console.log(`Testing ${description} (${endpoint})...`);
        
        return new Promise((resolve) => {
            const url = new URL(endpoint, this.baseUrl);
            
            const requestOptions = {
                hostname: url.hostname,
                port: url.port,
                path: url.pathname + url.search,
                method: 'GET',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': `Bearer ${this.authToken}`
                }
            };
            
            const req = http.request(requestOptions, (res) => {
                let data = '';
                
                res.on('data', (chunk) => {
                    data += chunk;
                });
                
                res.on('end', () => {
                    if (res.statusCode && res.statusCode >= 200 && res.statusCode < 300) {
                        try {
                            const jsonData = JSON.parse(data);
                            console.log(`✅ ${description} successful`);
                            console.log(`   Status: ${res.statusCode}`);
                            if (endpoint === '/api/v1/info') {
                                console.log(`   Service: ${jsonData.service}`);
                                console.log(`   Version: ${jsonData.version}`);
                                console.log(`   Uptime: ${jsonData.uptime_seconds} seconds`);
                            } else if (endpoint === '/v1/stats') {
                                console.log(`   Total Requests: ${jsonData.total_requests}`);
                                console.log(`   Cost Saved: $${jsonData.estimated_cost_saved_usd}`);
                                console.log(`   Local Requests: ${jsonData.tier_breakdown?.tier1_local_free || 0}`);
                            } else if (endpoint === '/api/v1/circuits') {
                                console.log(`   Circuits: ${jsonData.circuits?.length || 0}`);
                                if (jsonData.circuits && jsonData.circuits.length > 0) {
                                    jsonData.circuits.forEach(circuit => {
                                        console.log(`     - ${circuit.name}: ${circuit.state} (${circuit.failures}/${circuit.failure_threshold})`);
                                    });
                                }
                            } else if (endpoint === '/api/v1/routes') {
                                console.log(`   Routes: ${jsonData.routes?.length || 0}`);
                                console.log(`   Total Tracked: ${jsonData.total_tracked || 0}`);
                                if (jsonData.routes && jsonData.routes.length > 0) {
                                    const sampleRoute = jsonData.routes[0];
                                    console.log(`   Sample Route:`);
                                    console.log(`     Tier: ${sampleRoute.selected_tier}`);
                                    console.log(`     Tokens: ${sampleRoute.tokens}`);
                                    console.log(`     Latency: ${sampleRoute.latency_ms}ms`);
                                    console.log(`     Cost Saved: $${sampleRoute.cost_saved_usd}`);
                                }
                            } else if (endpoint === '/api/v1/pricing') {
                                console.log(`   Benchmark Model: ${jsonData.benchmark_model}`);
                                console.log(`   Pricing Entries: ${Object.keys(jsonData.pricing || {}).length}`);
                            }
                            console.log('');
                            resolve(jsonData);
                        } catch (parseError) {
                            console.error(`❌ Failed to parse JSON response from ${description}:`, parseError.message);
                            console.log(`   Raw data: ${data.substring(0, 200)}...`);
                            console.log('');
                            resolve(null);
                        }
                    } else {
                        console.error(`❌ ${description} failed with HTTP ${res.statusCode}: ${res.statusMessage || data}`);
                        console.log('');
                        resolve(null);
                    }
                });
            });
            
            req.on('error', (error) => {
                console.error(`❌ ${description} request failed:`, error.message);
                console.log('');
                resolve(null);
            });
            
            req.setTimeout(10000, () => {
                req.destroy();
                console.error(`❌ ${description} request timeout`);
                console.log('');
                resolve(null);
            });
            
            req.end();
        });
    }
    
    async runTests() {
        console.log('=== Nacho Flow Live Authenticated Test ===\n');
        console.log(`Connecting to: ${this.baseUrl}`);
        console.log(`Using auth token: ${this.authToken}\n`);
        
        // Test all endpoints
        const info = await this.testEndpoint('/api/v1/info', 'Daemon Info');
        const stats = await this.testEndpoint('/v1/stats', 'Statistics');
        const circuits = await this.testEndpoint('/api/v1/circuits', 'Circuit Breakers');
        const routes = await this.testEndpoint('/api/v1/routes?limit=3', 'Recent Routes');
        const pricing = await this.testEndpoint('/api/v1/pricing', 'Pricing Oracle');
        
        console.log('=== Test Results Summary ===');
        console.log(`Daemon Info: ${info ? '✅ Success' : '❌ Failed'}`);
        console.log(`Statistics: ${stats ? '✅ Success' : '❌ Failed'}`);
        console.log(`Circuits: ${circuits ? '✅ Success' : '❌ Failed'}`);
        console.log(`Routes: ${routes ? '✅ Success' : '❌ Failed'}`);
        console.log(`Pricing: ${pricing ? '✅ Success' : '❌ Failed'}`);
        
        if (info && stats && circuits && routes && pricing) {
            console.log('\n🎉 All authenticated tests completed successfully!');
            console.log('\n=== Sample Data ===');
            if (stats) {
                console.log('Statistics:');
                console.log(`  Total Requests: ${stats.total_requests}`);
                console.log(`  Cost Saved: $${stats.estimated_cost_saved_usd}`);
                console.log(`  Local Requests: ${stats.tier_breakdown?.tier1_local_free || 0}`);
            }
            if (routes && routes.routes && routes.routes.length > 0) {
                console.log('\nRecent Route Sample:');
                const sampleRoute = routes.routes[0];
                console.log(`  Time: ${sampleRoute.timestamp}`);
                console.log(`  Tier: ${sampleRoute.selected_tier}`);
                console.log(`  Tokens: ${sampleRoute.tokens}`);
                console.log(`  Latency: ${sampleRoute.latency_ms}ms`);
                console.log(`  Cost Saved: $${sampleRoute.cost_saved_usd}`);
            }
            if (circuits && circuits.circuits && circuits.circuits.length > 0) {
                console.log('\nCircuit Breakers:');
                circuits.circuits.forEach(circuit => {
                    console.log(`  ${circuit.name}: ${circuit.state} (${circuit.failures}/${circuit.failure_threshold})`);
                });
            }
            return true;
        } else {
            console.log('\n❌ Some authenticated tests failed!');
            return false;
        }
    }
}

// Run the test
const test = new ConnectivityTest();
test.runTests().catch(console.error);