import { RestClient } from './client';
import * as http from 'http';
import * as https from 'https';

// Mock the http and https modules
jest.mock('http');
jest.mock('https');

describe('RestClient', () => {
  let restClient: RestClient;
  const baseUrl = 'http://localhost:8000';
  const authToken = 'test-token';

  beforeEach(() => {
    restClient = new RestClient(baseUrl, authToken);
  });

  afterEach(() => {
    jest.clearAllMocks();
  });

  describe('getInfo', () => {
    it('should make a GET request to /api/v1/info', async () => {
      const mockResponse = { uptime: 1000, version: '1.0.0' };
      mockHttpRequest(mockResponse, 200);

      const result = await restClient.getInfo();

      expect(result).toEqual(mockResponse);
      expect(http.request).toHaveBeenCalledWith(
        expect.objectContaining({
          hostname: 'localhost',
          port: '8000',
          path: '/api/v1/info',
          method: 'GET',
          headers: expect.objectContaining({
            'Authorization': `Bearer ${authToken}`,
            'Content-Type': 'application/json'
          })
        }),
        expect.any(Function)
      );
    });

    it('should handle 401 Unauthorized', async () => {
      mockHttpRequest({ error: 'Unauthorized' }, 401);

      await expect(restClient.getInfo()).rejects.toThrow('HTTP 401: Unauthorized');
    });

    it('should handle 500 Internal Server Error', async () => {
      mockHttpRequest({ error: 'Internal Server Error' }, 500);

      await expect(restClient.getInfo()).rejects.toThrow('HTTP 500: Internal Server Error');
    });

    it('should handle network timeout', async () => {
      mockHttpRequestTimeout();

      await expect(restClient.getInfo()).rejects.toThrow('Request timeout');
    });

    it('should handle malformed JSON response', async () => {
      mockHttpRequest('invalid json', 200);

      await expect(restClient.getInfo()).rejects.toThrow('Failed to parse JSON response');
    });
  });

  describe('getRoutes', () => {
    it('should make a GET request to /api/v1/routes', async () => {
      const mockResponse = { routes: [] };
      mockHttpRequest(mockResponse, 200);

      const result = await restClient.getRoutes();

      expect(result).toEqual(mockResponse);
      expect(http.request).toHaveBeenCalledWith(
        expect.objectContaining({
          path: '/api/v1/routes'
        }),
        expect.any(Function)
      );
    });

    it('should make a GET request to /api/v1/routes with limit parameter', async () => {
      const mockResponse = { routes: [] };
      mockHttpRequest(mockResponse, 200);

      const result = await restClient.getRoutes(10);

      expect(result).toEqual(mockResponse);
      expect(http.request).toHaveBeenCalledWith(
        expect.objectContaining({
          path: '/api/v1/routes?limit=10'
        }),
        expect.any(Function)
      );
    });
  });

  describe('getCircuits', () => {
    it('should make a GET request to /api/v1/circuits', async () => {
      const mockResponse = { circuits: [] };
      mockHttpRequest(mockResponse, 200);

      const result = await restClient.getCircuits();

      expect(result).toEqual(mockResponse);
      expect(http.request).toHaveBeenCalledWith(
        expect.objectContaining({
          path: '/api/v1/circuits'
        }),
        expect.any(Function)
      );
    });
  });

  describe('resetCircuit', () => {
    it('should make a POST request to /api/v1/circuits/reset', async () => {
      const mockResponse = { success: true };
      mockHttpRequest(mockResponse, 200);

      const result = await restClient.resetCircuit();

      expect(result).toEqual(mockResponse);
      expect(http.request).toHaveBeenCalledWith(
        expect.objectContaining({
          path: '/api/v1/circuits/reset',
          method: 'POST'
        }),
        expect.any(Function)
      );
    });

    it('should make a POST request to /api/v1/circuits/reset with provider', async () => {
      const mockResponse = { success: true };
      const { mockRequest } = mockHttpRequest(mockResponse, 200);
      const provider = 'openai';

      const result = await restClient.resetCircuit(provider);

      expect(result).toEqual(mockResponse);
      expect(http.request).toHaveBeenCalledWith(
        expect.objectContaining({
          path: '/api/v1/circuits/reset',
          method: 'POST'
        }),
        expect.any(Function)
      );
      expect(mockRequest.write).toHaveBeenCalledWith(JSON.stringify({ provider }));
    });
  });

  describe('getPricing', () => {
    it('should make a GET request to /api/v1/pricing', async () => {
      const mockResponse = { pricing: {} };
      mockHttpRequest(mockResponse, 200);

      const result = await restClient.getPricing();

      expect(result).toEqual(mockResponse);
      expect(http.request).toHaveBeenCalledWith(
        expect.objectContaining({
          path: '/api/v1/pricing'
        }),
        expect.any(Function)
      );
    });
  });

  describe('getConfig', () => {
    it('should make a GET request to /api/v1/config', async () => {
      const mockResponse = { config: {} };
      mockHttpRequest(mockResponse, 200);

      const result = await restClient.getConfig();

      expect(result).toEqual(mockResponse);
      expect(http.request).toHaveBeenCalledWith(
        expect.objectContaining({
          path: '/api/v1/config'
        }),
        expect.any(Function)
      );
    });
  });

  describe('updateConfig', () => {
    it('should make a PUT request to /api/v1/config', async () => {
      const mockResponse = { success: true };
      const { mockRequest } = mockHttpRequest(mockResponse, 200);
      const config = { test: 'config' };

      const result = await restClient.updateConfig(config);

      expect(result).toEqual(mockResponse);
      expect(http.request).toHaveBeenCalledWith(
        expect.objectContaining({
          path: '/api/v1/config',
          method: 'PUT'
        }),
        expect.any(Function)
      );
      expect(mockRequest.write).toHaveBeenCalledWith(JSON.stringify(config));
    });

    it('should make a PUT request to /api/v1/config with dry_run parameter', async () => {
      const mockResponse = { success: true };
      mockHttpRequest(mockResponse, 200);
      const config = { test: 'config' };

      const result = await restClient.updateConfig(config, true);

      expect(result).toEqual(mockResponse);
      expect(http.request).toHaveBeenCalledWith(
        expect.objectContaining({
          path: '/api/v1/config?dry_run=true',
          method: 'PUT'
        }),
        expect.any(Function)
      );
    });
  });

  describe('getConfigYaml and updateConfigYaml', () => {
    it('should fetch raw YAML from /api/v1/config?format=yaml', async () => {
      const rawYaml = 'port: 8000\nauth_token: test';
      mockHttpRequest(rawYaml, 200, { 'content-type': 'application/x-yaml' });

      const result = await restClient.getConfigYaml();

      expect(result).toBe(rawYaml);
      expect(http.request).toHaveBeenCalledWith(
        expect.objectContaining({
          path: '/api/v1/config?format=yaml'
        }),
        expect.any(Function)
      );
    });

    it('should update config via raw YAML with PUT /api/v1/config', async () => {
      const mockResponse = { status: 'ok' };
      const { mockRequest } = mockHttpRequest(mockResponse, 200);
      const yamlPayload = 'port: 8000\nauth_token: test';

      const result = await restClient.updateConfigYaml(yamlPayload, true);

      expect(result).toEqual(mockResponse);
      expect(http.request).toHaveBeenCalledWith(
        expect.objectContaining({
          path: '/api/v1/config?dry_run=true',
          method: 'PUT',
          headers: expect.objectContaining({
            'Content-Type': 'application/x-yaml'
          })
        }),
        expect.any(Function)
      );
      expect(mockRequest.write).toHaveBeenCalledWith(yamlPayload);
    });
  });

  describe('tune', () => {
    it('should make a POST request to /api/v1/tune', async () => {
      const mockResponse = { tuning: {} };
      mockHttpRequest(mockResponse, 200);

      const result = await restClient.tune();

      expect(result).toEqual(mockResponse);
      expect(http.request).toHaveBeenCalledWith(
        expect.objectContaining({
          path: '/api/v1/tune',
          method: 'POST'
        }),
        expect.any(Function)
      );
    });
  });

  describe('getStats', () => {
    it('should make a GET request to /v1/stats', async () => {
      const mockResponse = { stats: {} };
      mockHttpRequest(mockResponse, 200);

      const result = await restClient.getStats();

      expect(result).toEqual(mockResponse);
      expect(http.request).toHaveBeenCalledWith(
        expect.objectContaining({
          path: '/v1/stats'
        }),
        expect.any(Function)
      );
    });
  });

  describe('getDeals', () => {
    it('should make a GET request to /api/v1/deals', async () => {
      const mockResponse = { deals: [] };
      mockHttpRequest(mockResponse, 200);

      const result = await restClient.getDeals();

      expect(result).toEqual(mockResponse);
      expect(http.request).toHaveBeenCalledWith(
        expect.objectContaining({
          path: '/api/v1/deals'
        }),
        expect.any(Function)
      );
    });
  });

  describe('resetStats', () => {
    it('should make a POST request to /api/v1/stats/reset', async () => {
      const mockResponse = { status: 'ok', message: 'Stats reset' };
      mockHttpRequest(mockResponse, 200);

      const result = await restClient.resetStats();

      expect(result).toEqual(mockResponse);
      expect(http.request).toHaveBeenCalledWith(
        expect.objectContaining({
          path: '/api/v1/stats/reset',
          method: 'POST'
        }),
        expect.any(Function)
      );
    });
  });

  describe('recalculateStats', () => {
    it('should make a POST request to /api/v1/stats/recalculate', async () => {
      const mockResponse = { status: 'ok', records_processed: 5 };
      mockHttpRequest(mockResponse, 200);

      const result = await restClient.recalculateStats();

      expect(result).toEqual(mockResponse);
      expect(http.request).toHaveBeenCalledWith(
        expect.objectContaining({
          path: '/api/v1/stats/recalculate',
          method: 'POST'
        }),
        expect.any(Function)
      );
    });
  });

  describe('getHealth', () => {
    it('should make a GET request to /health', async () => {
      const mockResponse = { status: 'healthy', version: '0.6.0' };
      mockHttpRequest(mockResponse, 200);

      const result = await restClient.getHealth();

      expect(result).toEqual(mockResponse);
      expect(http.request).toHaveBeenCalledWith(
        expect.objectContaining({
          path: '/health'
        }),
        expect.any(Function)
      );
    });
  });
});

// Helper functions for mocking HTTP requests
function mockHttpRequest(responseData: any, statusCode: number, headers: any = {}) {
  const mockRequest: any = {
    on: jest.fn(),
    end: jest.fn(),
    write: jest.fn()
  };

  const mockResponse: any = {
    statusCode: statusCode,
    headers: headers,
    statusMessage: statusCode === 200 ? 'OK' : (statusCode === 401 ? 'Unauthorized' : 'Internal Server Error'),
    on: jest.fn((event: string, callback: (data?: any) => void) => {
      if (event === 'data') {
        callback(typeof responseData === 'string' ? responseData : JSON.stringify(responseData));
      } else if (event === 'end') {
        callback();
      }
      return mockResponse;
    })
  };

  (http.request as jest.Mock).mockImplementation((_options: any, callback: (res: any) => void) => {
    if (typeof callback === 'function') {
      callback(mockResponse);
    }
    return mockRequest;
  });

  return { mockRequest, mockResponse };
}

function mockHttpRequestTimeout() {
  const mockRequest: any = {
    on: jest.fn((event: string, callback: () => void) => {
      if (event === 'timeout') {
        callback();
      }
      return mockRequest;
    }),
    end: jest.fn(),
    write: jest.fn(),
    destroy: jest.fn()
  };

  (http.request as jest.Mock).mockReturnValue(mockRequest);
}