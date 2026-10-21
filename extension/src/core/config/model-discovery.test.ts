import * as http from 'http';
import * as https from 'https';
import { EventEmitter } from 'events';
import { discoverModels, clearModelDiscoveryCache, ModelDiscoveryResult, ANTHROPIC_STANDARD_MODELS } from './model-discovery';

jest.mock('http');
jest.mock('https');

describe('ModelDiscovery', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearModelDiscoveryCache();
  });

  function setupMockRequest(
    protocolModule: typeof http | typeof https,
    options: {
      statusCode?: number;
      body?: string;
      error?: Error;
      timeout?: boolean;
      delayMs?: number;
    }
  ) {
    const mockReq = new EventEmitter() as any;
    mockReq.setTimeout = jest.fn().mockImplementation((_timeoutMs: number, cb?: () => void) => {
      if (options.timeout && cb) {
        setTimeout(cb, 10);
      }
      return mockReq;
    });
    mockReq.destroy = jest.fn();
    mockReq.end = jest.fn();

    (protocolModule.request as jest.Mock).mockImplementation((_url: any, _reqOpts: any, cb?: (res: any) => void) => {
      if (options.error) {
        setTimeout(() => mockReq.emit('error', options.error), options.delayMs || 5);
        return mockReq;
      }

      if (options.timeout) {
        setTimeout(() => {
          mockReq.emit('timeout');
          mockReq.emit('error', new Error('Request timed out'));
        }, 15);
        return mockReq;
      }

      const mockRes = new EventEmitter() as any;
      mockRes.statusCode = options.statusCode ?? 200;
      mockRes.statusMessage = options.statusCode === 404 ? 'Not Found' : 'OK';

      setTimeout(() => {
        if (cb) cb(mockRes);
        if (options.body !== undefined) {
          mockRes.emit('data', Buffer.from(options.body));
        }
        mockRes.emit('end');
      }, options.delayMs || 5);

      return mockReq;
    });

    return mockReq;
  }

  it('should discover models from OpenAI-compatible /models endpoint', async () => {
    setupMockRequest(http, {
      statusCode: 200,
      body: JSON.stringify({
        data: [
          { id: 'qwen2.5-coder-32b' },
          { id: 'deepseek-r1-distill-qwen-32b' }
        ]
      })
    });

    const result = await discoverModels('http://127.0.0.1:8080/v1');

    expect(result.online).toBe(true);
    expect(result.models).toEqual(['qwen2.5-coder-32b', 'deepseek-r1-distill-qwen-32b']);
    expect(result.latencyMs).toBeGreaterThanOrEqual(0);
    expect(result.error).toBeUndefined();
  });

  it('should support https endpoints', async () => {
    setupMockRequest(https, {
      statusCode: 200,
      body: JSON.stringify({
        data: [{ id: 'anthropic/claude-3.5-sonnet' }]
      })
    });

    const result = await discoverModels('https://openrouter.ai/api/v1');

    expect(result.online).toBe(true);
    expect(result.models).toEqual(['anthropic/claude-3.5-sonnet']);
  });

  it('should fall back to Ollama /api/tags if /models returns 404', async () => {
    // First call to /models returns 404
    // Second call to /api/tags returns Ollama tags
    let callCount = 0;
    (http.request as jest.Mock).mockImplementation((_url: any, _reqOpts: any, cb?: (res: any) => void) => {
      callCount++;
      const mockReq = new EventEmitter() as any;
      mockReq.setTimeout = jest.fn().mockReturnValue(mockReq);
      mockReq.destroy = jest.fn();
      mockReq.end = jest.fn();

      const mockRes = new EventEmitter() as any;
      if (callCount === 1) {
        mockRes.statusCode = 404;
        mockRes.statusMessage = 'Not Found';
        setTimeout(() => {
          if (cb) cb(mockRes);
          mockRes.emit('data', Buffer.from('Not found'));
          mockRes.emit('end');
        }, 5);
      } else {
        mockRes.statusCode = 200;
        mockRes.statusMessage = 'OK';
        setTimeout(() => {
          if (cb) cb(mockRes);
          mockRes.emit('data', Buffer.from(JSON.stringify({
            models: [
              { name: 'llama3.1:8b' },
              { name: 'qwen2.5-coder:14b' }
            ]
          })));
          mockRes.emit('end');
        }, 5);
      }

      return mockReq;
    });

    const result = await discoverModels('http://127.0.0.1:11434/v1');

    expect(result.online).toBe(true);
    expect(result.models).toEqual(['llama3.1:8b', 'qwen2.5-coder:14b']);
  });

  it('should use 30-second in-memory cache on subsequent calls', async () => {
    setupMockRequest(http, {
      statusCode: 200,
      body: JSON.stringify({ data: [{ id: 'cached-model' }] })
    });

    const first = await discoverModels('http://127.0.0.1:8080/v1');
    expect(first.models).toEqual(['cached-model']);
    expect(http.request).toHaveBeenCalledTimes(1);

    // Second call should return from cache without network request
    const second = await discoverModels('http://127.0.0.1:8080/v1');
    expect(second.models).toEqual(['cached-model']);
    expect(http.request).toHaveBeenCalledTimes(1);

    // forceRefresh: true should bypass cache
    const refreshed = await discoverModels('http://127.0.0.1:8080/v1', { forceRefresh: true });
    expect(refreshed.models).toEqual(['cached-model']);
    expect(http.request).toHaveBeenCalledTimes(2);
  });

  it('should clear cache via clearModelDiscoveryCache', async () => {
    setupMockRequest(http, {
      statusCode: 200,
      body: JSON.stringify({ data: [{ id: 'model-a' }] })
    });

    await discoverModels('http://127.0.0.1:8080/v1');
    expect(http.request).toHaveBeenCalledTimes(1);

    clearModelDiscoveryCache();

    await discoverModels('http://127.0.0.1:8080/v1');
    expect(http.request).toHaveBeenCalledTimes(2);
  });

  it('should handle connection timeout and mark online: false', async () => {
    setupMockRequest(http, { timeout: true });

    const result = await discoverModels('http://127.0.0.1:9999/v1', { timeoutMs: 100 });

    expect(result.online).toBe(false);
    expect(result.models).toEqual([]);
    expect(result.error).toContain('timed out');
  });

  it('should handle network error (e.g. ECONNREFUSED) and mark online: false', async () => {
    const connErr = new Error('connect ECONNREFUSED 127.0.0.1:9999');
    (connErr as any).code = 'ECONNREFUSED';
    setupMockRequest(http, { error: connErr });

    const result = await discoverModels('http://127.0.0.1:9999/v1');

    expect(result.online).toBe(false);
    expect(result.models).toEqual([]);
    expect(result.error).toContain('ECONNREFUSED');
  });

  it('should report online with error on HTTP error codes (e.g. 401)', async () => {
    setupMockRequest(http, {
      statusCode: 401,
      body: JSON.stringify({ error: { message: 'Invalid API key' } })
    });

    const result = await discoverModels('http://127.0.0.1:8080/v1');

    expect(result.online).toBe(true);
    expect(result.models).toEqual([]);
    expect(result.error).toContain('401');
  });

  it('should handle malformed JSON response gracefully', async () => {
    setupMockRequest(http, {
      statusCode: 200,
      body: '<html>Not JSON</html>'
    });

    const result = await discoverModels('http://127.0.0.1:8080/v1');

    expect(result.online).toBe(true);
    expect(result.models).toEqual([]);
    expect(result.error).toBeDefined();
  });

  it('should return empty models when payload has no data or models array', async () => {
    setupMockRequest(http, {
      statusCode: 200,
      body: JSON.stringify({ other: 123 })
    });

    const result = await discoverModels('http://127.0.0.1:8080/v1');

    expect(result.online).toBe(true);
    expect(result.models).toEqual([]);
  });

  it('should handle invalid URLs gracefully', async () => {
    const result = await discoverModels('not-a-valid-url');

    expect(result.online).toBe(false);
    expect(result.models).toEqual([]);
    expect(result.error).toContain('Invalid URL');
  });

  it('should fallback to ANTHROPIC_STANDARD_MODELS when Anthropic returns 401/404 or empty models', async () => {
    setupMockRequest(https, {
      statusCode: 401,
      body: JSON.stringify({ error: { message: 'x-api-key header missing' } })
    });

    const result = await discoverModels('https://api.anthropic.com');

    expect(result.online).toBe(true);
    expect(result.models).toContain('claude-haiku-5-5');
    expect(result.models).toContain('claude-sonnet-5-5');
    expect(result.models).toContain('claude-opus-5-5');
    expect(result.models).toContain('claude-fable-5-1');
    expect(result.models).toContain('claude-sonnet-4-6');
    expect(result.models).toContain('claude-opus-4-6');
    expect(result.models).toContain('claude-sonnet-4-5-20250929');
    expect(result.models).toContain('claude-haiku-4-5-20251001');
    expect(result.models).toContain('claude-3-7-sonnet-20250219');
    expect(result.models).toContain('claude-3-5-sonnet-20241022');
    expect(result.models).toContain('claude-3-5-haiku-20241022');
    expect(result.models).toContain('claude-3-opus-20240229');
    expect(result.models.length).toBe(ANTHROPIC_STANDARD_MODELS.length);
  });

  it('should resolve environment variables in apiKey like ${ANTHROPIC_API_KEY} and $ANTHROPIC_API_KEY', async () => {
    process.env.ANTHROPIC_API_KEY = 'secret-env-ant-token';
    process.env.OPENROUTER_API_KEY = 'secret-env-or-token';

    setupMockRequest(https, {
      statusCode: 200,
      body: JSON.stringify({ data: [{ id: 'claude-test' }] })
    });

    await discoverModels('https://api.anthropic.com/v1', {
      apiKey: '${ANTHROPIC_API_KEY}'
    });

    expect(https.request).toHaveBeenCalledWith(
      expect.anything(),
      expect.objectContaining({
        headers: expect.objectContaining({
          'x-api-key': 'secret-env-ant-token'
        })
      }),
      expect.any(Function)
    );

    setupMockRequest(http, {
      statusCode: 200,
      body: JSON.stringify({ data: [{ id: 'or-test' }] })
    });

    await discoverModels('http://openrouter.ai/api/v1', {
      apiKey: '$OPENROUTER_API_KEY'
    });

    expect(http.request).toHaveBeenCalledWith(
      expect.anything(),
      expect.objectContaining({
        headers: expect.objectContaining({
          Authorization: 'Bearer secret-env-or-token'
        })
      }),
      expect.any(Function)
    );

    delete process.env.ANTHROPIC_API_KEY;
    delete process.env.OPENROUTER_API_KEY;
  });

  it('should resolve environment variable references using ENV_ prefix', async () => {
    process.env.OPENROUTER_API_KEY = 'sk-or-env-prefixed-secret';
    process.env.ANTHROPIC_API_KEY = 'sk-ant-env-prefixed-secret';

    setupMockRequest(http, {
      statusCode: 200,
      body: JSON.stringify({ data: [{ id: 'openrouter/model-1' }] })
    });

    await discoverModels('http://openrouter.ai/api/v1', {
      apiKey: 'ENV_OPENROUTER_API_KEY'
    });

    expect(http.request).toHaveBeenCalledWith(
      expect.anything(),
      expect.objectContaining({
        headers: expect.objectContaining({
          Authorization: 'Bearer sk-or-env-prefixed-secret'
        })
      }),
      expect.any(Function)
    );

    setupMockRequest(https, {
      statusCode: 200,
      body: JSON.stringify({ data: [{ id: 'claude-3-7-sonnet' }] })
    });

    await discoverModels('https://api.anthropic.com/v1', {
      apiKey: 'ENV_ANTHROPIC_API_KEY'
    });

    expect(https.request).toHaveBeenCalledWith(
      expect.anything(),
      expect.objectContaining({
        headers: expect.objectContaining({
          'x-api-key': 'sk-ant-env-prefixed-secret',
          'anthropic-version': '2023-06-01'
        })
      }),
      expect.any(Function)
    );

    delete process.env.OPENROUTER_API_KEY;
    delete process.env.ANTHROPIC_API_KEY;
  });

  it('should omit Authorization header when ENV_ or ${ENV} api_key placeholder is unset in process.env', async () => {
    delete process.env.UNSET_SECRET_KEY;

    setupMockRequest(http, {
      statusCode: 200,
      body: JSON.stringify({ data: [{ id: 'local-model-1' }] })
    });

    await discoverModels('http://localhost:8080/v1', {
      apiKey: 'ENV_UNSET_SECRET_KEY'
    });

    const callHeaders = (http.request as jest.Mock).mock.calls[0][1].headers;
    expect(callHeaders.Authorization).toBeUndefined();
  });

  it('should pass Anthropic headers when apiKey is provided', async () => {
    setupMockRequest(https, {
      statusCode: 200,
      body: JSON.stringify({
        data: [{ id: 'claude-custom-tier' }]
      })
    });

    const result = await discoverModels('https://api.anthropic.com/v1', {
      apiKey: 'sk-ant-test-key-123'
    });

    expect(result.online).toBe(true);
    expect(result.models).toContain('claude-custom-tier');
    expect(https.request).toHaveBeenCalledWith(
      expect.objectContaining({ pathname: '/v1/models' }),
      expect.objectContaining({
        headers: expect.objectContaining({
          'x-api-key': 'sk-ant-test-key-123',
          'anthropic-version': '2023-06-01'
        })
      }),
      expect.any(Function)
    );
  });

  it('should pass Authorization header for non-Anthropic providers with apiKey', async () => {
    setupMockRequest(http, {
      statusCode: 200,
      body: JSON.stringify({
        data: [{ id: 'custom-openrouter-model' }]
      })
    });

    const result = await discoverModels('http://openrouter.ai/api/v1', {
      apiKey: 'sk-or-test-key'
    });

    expect(result.online).toBe(true);
    expect(result.models).toContain('custom-openrouter-model');
    expect(http.request).toHaveBeenCalledWith(
      expect.objectContaining({ pathname: '/api/v1/models' }),
      expect.objectContaining({
        headers: expect.objectContaining({
          Authorization: 'Bearer sk-or-test-key'
        })
      }),
      expect.any(Function)
    );
  });
});

