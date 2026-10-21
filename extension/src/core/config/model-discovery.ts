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
	apiKey?: string;
	providerType?: string;
}

export const ANTHROPIC_STANDARD_MODELS = [
	'claude-haiku-5-5',
	'claude-sonnet-5-5',
	'claude-opus-5-5',
	'claude-fable-5-1',
	'claude-sonnet-4-6',
	'claude-opus-4-6',
	'claude-sonnet-4-5-20250929',
	'claude-haiku-4-5-20251001',
	'claude-3-7-sonnet-20250219',
	'claude-3-5-sonnet-20241022',
	'claude-3-5-haiku-20241022',
	'claude-3-opus-20240229'
];

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

function requestHttp(
	targetUrl: URL,
	timeoutMs: number,
	customHeaders?: Record<string, string>
): Promise<RawHttpResponse> {
	return new Promise((resolve, reject) => {
		const isHttps = targetUrl.protocol === 'https:';
		const client = isHttps ? https : http;
		const startTime = Date.now();

		const headers: Record<string, string> = {
			Accept: 'application/json',
			...(customHeaders || {})
		};

		const req = client.request(
			targetUrl,
			{
				method: 'GET',
				headers
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

	// OpenAI-compatible & Anthropic: data: [{ id: "model-name" }]
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
 * Probes an OpenAI-compatible, Anthropic, or Ollama endpoint to discover available models.
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

	const isAnthropic = parsedBase.hostname.includes('anthropic.com') || options?.providerType === 'anthropic';
	const cacheKey = parsedBase.origin + parsedBase.pathname.replace(/\/+$/, '') + (options?.apiKey ? ':auth' : '');

	if (!forceRefresh) {
		const cached = discoveryCache.get(cacheKey);
		if (cached && Date.now() - cached.timestamp < DEFAULT_TTL_MS) {
			return cached.result;
		}
	}

	// Resolve environment variable references in apiKey (e.g. ENV_VAR, ${ENV_VAR}, or $ENV_VAR)
	let resolvedApiKey = options?.apiKey;
	if (resolvedApiKey) {
		const envMatch =
			resolvedApiKey.match(/^ENV_([A-Za-z0-9_]+)$/) ||
			resolvedApiKey.match(/^\$\{([A-Za-z0-9_]+)\}$/) ||
			resolvedApiKey.match(/^\$([A-Za-z0-9_]+)$/);
		if (envMatch) {
			resolvedApiKey = process.env[envMatch[1]] || undefined;
		}
	}

	// Determine candidate URLs & headers
	const customHeaders: Record<string, string> = {};
	if (resolvedApiKey) {
		if (isAnthropic) {
			customHeaders['x-api-key'] = resolvedApiKey;
			customHeaders['anthropic-version'] = '2023-06-01';
		} else {
			customHeaders['Authorization'] = `Bearer ${resolvedApiKey}`;
		}
	}

	const basePath = parsedBase.pathname.replace(/\/+$/, '');
	let primaryPath: string;
	if (isAnthropic) {
		primaryPath = basePath.length > 0
			? (basePath.endsWith('/v1') ? `${basePath}/models` : `${basePath}/v1/models`)
			: '/v1/models';
	} else {
		primaryPath = basePath.length > 0 ? `${basePath}/models` : '/models';
	}
	const primaryUrl = new URL(primaryPath, parsedBase.origin);

	try {
		const res = await requestHttp(primaryUrl, timeoutMs, customHeaders);

		if (res.statusCode >= 200 && res.statusCode < 300) {
			try {
				const json = JSON.parse(res.body);
				let models = extractModelsFromPayload(json);
				if (models.length === 0 && isAnthropic) {
					models = [...ANTHROPIC_STANDARD_MODELS];
				}
				const result: ModelDiscoveryResult = {
					online: true,
					latencyMs: res.latencyMs,
					models
				};
				discoveryCache.set(cacheKey, { timestamp: Date.now(), result });
				return result;
			} catch (jsonErr: any) {
				const models = isAnthropic ? [...ANTHROPIC_STANDARD_MODELS] : [];
				const result: ModelDiscoveryResult = {
					online: true,
					latencyMs: res.latencyMs,
					models,
					error: models.length > 0 ? undefined : `Failed to parse response JSON: ${jsonErr.message}`
				};
				discoveryCache.set(cacheKey, { timestamp: Date.now(), result });
				return result;
			}
		}

		// Anthropic fallback on 401 / 403 / 404 (Anthropic cloud is online, provide standard models)
		if (isAnthropic) {
			const result: ModelDiscoveryResult = {
				online: true,
				latencyMs: res.latencyMs,
				models: [...ANTHROPIC_STANDARD_MODELS]
			};
			discoveryCache.set(cacheKey, { timestamp: Date.now(), result });
			return result;
		}

		// If 404, probe Ollama /api/tags fallback
		if (res.statusCode === 404) {
			const ollamaUrl = new URL('/api/tags', parsedBase.origin);
			try {
				const ollamaRes = await requestHttp(ollamaUrl, timeoutMs, customHeaders);
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
