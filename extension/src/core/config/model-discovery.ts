import * as http from 'http';
import * as https from 'https';
import { URL } from 'url';

export interface ModelDiscoveryResult {
	online: boolean;
	latencyMs: number;
	models: string[];
	error?: string;
}

export interface ModelDiscoveryOptions {
	timeoutMs?: number;
	forceRefresh?: boolean;
}

const DEFAULT_TIMEOUT_MS = 1500;
const DEFAULT_TTL_MS = 30000; // 30 seconds

interface CacheEntry {
	timestamp: number;
	result: ModelDiscoveryResult;
}

const discoveryCache = new Map<string, CacheEntry>();

/**
 * Clears the in-memory model discovery cache.
 */
export function clearModelDiscoveryCache(): void {
	discoveryCache.clear();
}

interface RawHttpResponse {
	statusCode: number;
	statusMessage: string;
	body: string;
	latencyMs: number;
}

function requestHttp(targetUrl: URL, timeoutMs: number): Promise<RawHttpResponse> {
	return new Promise((resolve, reject) => {
		const isHttps = targetUrl.protocol === 'https:';
		const client = isHttps ? https : http;
		const startTime = Date.now();

		const req = client.request(
			targetUrl,
			{
				method: 'GET',
				headers: {
					Accept: 'application/json'
				}
			},
			(res) => {
				const chunks: Buffer[] = [];
				res.on('data', (chunk) => chunks.push(Buffer.from(chunk)));
				res.on('end', () => {
					const latencyMs = Date.now() - startTime;
					const body = Buffer.concat(chunks).toString('utf8');
					resolve({
						statusCode: res.statusCode || 200,
						statusMessage: res.statusMessage || '',
						body,
						latencyMs
					});
				});
			}
		);

		req.setTimeout(timeoutMs, () => {
			req.destroy(new Error(`Request timed out after ${timeoutMs}ms`));
		});

		req.on('error', (err) => {
			reject(err);
		});

		req.end();
	});
}

function extractModelsFromPayload(parsed: any): string[] {
	if (!parsed || typeof parsed !== 'object') {
		return [];
	}

	// OpenAI-compatible: data: [{ id: "model-name" }]
	if (Array.isArray(parsed.data)) {
		return parsed.data
			.map((item: any) => (typeof item === 'string' ? item : item?.id || item?.name))
			.filter((id: any): id is string => typeof id === 'string' && id.trim().length > 0);
	}

	// Ollama /api/tags or /v1/models: models: [{ name: "model:tag" }]
	if (Array.isArray(parsed.models)) {
		return parsed.models
			.map((item: any) => (typeof item === 'string' ? item : item?.name || item?.id))
			.filter((name: any): name is string => typeof name === 'string' && name.trim().length > 0);
	}

	return [];
}

/**
 * Probes an OpenAI-compatible or Ollama endpoint to discover available models.
 */
export async function discoverModels(
	baseUrl: string,
	options?: ModelDiscoveryOptions
): Promise<ModelDiscoveryResult> {
	const timeoutMs = options?.timeoutMs ?? DEFAULT_TIMEOUT_MS;
	const forceRefresh = options?.forceRefresh ?? false;

	// Validate URL
	let parsedBase: URL;
	try {
		parsedBase = new URL(baseUrl);
	} catch (e: any) {
		return {
			online: false,
			latencyMs: 0,
			models: [],
			error: `Invalid URL '${baseUrl}': ${e.message}`
		};
	}

	const cacheKey = parsedBase.origin + parsedBase.pathname.replace(/\/+$/, '');

	if (!forceRefresh) {
		const cached = discoveryCache.get(cacheKey);
		if (cached && Date.now() - cached.timestamp < DEFAULT_TTL_MS) {
			return cached.result;
		}
	}

	// Determine candidate URLs
	// Primary: <base_url>/models
	const basePath = parsedBase.pathname.replace(/\/+$/, '');
	const primaryPath = basePath.length > 0 ? `${basePath}/models` : '/models';
	const primaryUrl = new URL(primaryPath, parsedBase.origin);

	try {
		const res = await requestHttp(primaryUrl, timeoutMs);

		if (res.statusCode >= 200 && res.statusCode < 300) {
			try {
				const json = JSON.parse(res.body);
				const models = extractModelsFromPayload(json);
				const result: ModelDiscoveryResult = {
					online: true,
					latencyMs: res.latencyMs,
					models
				};
				discoveryCache.set(cacheKey, { timestamp: Date.now(), result });
				return result;
			} catch (jsonErr: any) {
				const result: ModelDiscoveryResult = {
					online: true,
					latencyMs: res.latencyMs,
					models: [],
					error: `Failed to parse response JSON: ${jsonErr.message}`
				};
				return result;
			}
		}

		// If 404, probe Ollama /api/tags fallback
		if (res.statusCode === 404) {
			const ollamaUrl = new URL('/api/tags', parsedBase.origin);
			try {
				const ollamaRes = await requestHttp(ollamaUrl, timeoutMs);
				if (ollamaRes.statusCode >= 200 && ollamaRes.statusCode < 300) {
					const json = JSON.parse(ollamaRes.body);
					const models = extractModelsFromPayload(json);
					const result: ModelDiscoveryResult = {
						online: true,
						latencyMs: ollamaRes.latencyMs,
						models
					};
					discoveryCache.set(cacheKey, { timestamp: Date.now(), result });
					return result;
				}
			} catch {
				// Fall through to report primary 404
			}
		}

		// Non-200 HTTP response
		return {
			online: true,
			latencyMs: res.latencyMs,
			models: [],
			error: `HTTP ${res.statusCode} ${res.statusMessage || ''}`.trim()
		};
	} catch (err: any) {
		const isTimeout = err.message?.includes('timed out');
		return {
			online: false,
			latencyMs: timeoutMs,
			models: [],
			error: isTimeout ? `Request timed out after ${timeoutMs}ms` : err.message
		};
	}
}
