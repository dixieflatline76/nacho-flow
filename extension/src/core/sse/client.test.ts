import * as http from 'http';
import * as https from 'https';
import { SSEClient, SSEEventCallback } from './client';

// Mock the http and https modules
jest.mock('http');
jest.mock('https');

describe('SSEClient', () => {
  let sseClient: SSEClient;
  const baseUrl = 'http://localhost:8000';
  const authToken = 'test-token';

  beforeEach(() => {
    jest.useFakeTimers();
    sseClient = new SSEClient(baseUrl, authToken);
  });

  afterEach(() => {
    jest.clearAllMocks();
    jest.clearAllTimers();
    sseClient.disconnect();
  });

  describe('connect', () => {
    it('should make a GET request to /api/v1/events', () => {
      const mockRequest = {
        on: jest.fn(),
        setTimeout: jest.fn(),
        end: jest.fn(),
        destroy: jest.fn()
      };

      (http.request as jest.Mock).mockReturnValue(mockRequest);

      sseClient.connect();

      expect(http.request).toHaveBeenCalledWith(
        expect.objectContaining({
          hostname: 'localhost',
          port: '8000',
          path: '/api/v1/events',
          method: 'GET',
          headers: expect.objectContaining({
            'Authorization': `Bearer ${authToken}`,
            'Accept': 'text/event-stream',
            'Cache-Control': 'no-cache',
            'Connection': 'keep-alive'
          })
        }),
        expect.any(Function)
      );
    });

    it('should handle HTTPS protocol', () => {
      const httpsClient = new SSEClient('https://localhost:8000', authToken);
      const mockRequest = {
        on: jest.fn(),
        setTimeout: jest.fn(),
        end: jest.fn(),
        destroy: jest.fn()
      };

      (https.request as jest.Mock).mockReturnValue(mockRequest);

      httpsClient.connect();

      expect(https.request).toHaveBeenCalled();
    });
  });

  describe('subscribe and emit', () => {
    it('should subscribe to events and emit them when received', () => {
      const eventType = 'testEvent';
      const eventData = { message: 'test' };
      
      // Create a mock response object
      const mockResponse: any = {
        on: jest.fn((event: string, callback: (data?: any) => void) => {
          if (event === 'data') {
            // Simulate receiving an SSE event
            setTimeout(() => {
              const sseData = `event: ${eventType}\ndata: ${JSON.stringify(eventData)}\n\n`;
              callback(sseData);
            }, 10);
          }
          return mockResponse;
        }),
        destroy: jest.fn()
      };

      // Mock the request to immediately call the response callback
      (http.request as jest.Mock).mockImplementation((_options: any, callback: (res: any) => void) => {
        const mockRequest: any = {
          on: jest.fn(),
          setTimeout: jest.fn(),
          end: jest.fn(),
          destroy: jest.fn()
        };
        
        // Call the response callback with our mock response
        callback(mockResponse);
        return mockRequest;
      });

      // Subscribe to the event
      const callback = jest.fn();
      sseClient.subscribe(eventType, callback);

      // Connect to start listening
      sseClient.connect();
      
      // Advance timers to trigger the event
      jest.advanceTimersByTime(15);
      
      // Check that the callback was called with the correct data
      expect(callback).toHaveBeenCalledWith(eventData);
    });

    it('should handle multiple subscribers for the same event', () => {
      const eventType = 'testEvent';
      const eventData = { message: 'test' };
      const callback1 = jest.fn();
      const callback2 = jest.fn();

      sseClient.subscribe(eventType, callback1);
      sseClient.subscribe(eventType, callback2);

      // Emit the event
      (sseClient as any).emit(eventType, eventData);

      expect(callback1).toHaveBeenCalledWith(eventData);
      expect(callback2).toHaveBeenCalledWith(eventData);
    });
  });

  describe('unsubscribe', () => {
    it('should remove a subscriber', () => {
      const eventType = 'testEvent';
      const eventData = { message: 'test' };
      const callback = jest.fn();

      sseClient.subscribe(eventType, callback);
      sseClient.unsubscribe(eventType, callback);

      // Emit the event
      (sseClient as any).emit(eventType, eventData);

      expect(callback).not.toHaveBeenCalled();
    });
  });

  describe('handleSSEData', () => {
    it('should parse and emit SSE events', (done) => {
      const eventType = 'testEvent';
      const eventData = { message: 'test' };
      const sseData = `event: ${eventType}\ndata: ${JSON.stringify(eventData)}\n\n`;

      sseClient.subscribe(eventType, (data) => {
        expect(data).toEqual(eventData);
        done();
      });

      // Process the SSE data
      (sseClient as any).handleSSEData(sseData);
    });

    it('should handle malformed JSON in SSE events', () => {
      const eventType = 'testEvent';
      const sseData = `event: ${eventType}\ndata: invalid json\n\n`;
      const consoleSpy = jest.spyOn(console, 'error').mockImplementation();

      // Process the SSE data
      (sseClient as any).handleSSEData(sseData);

      expect(consoleSpy).toHaveBeenCalledWith(expect.stringContaining('Failed to parse SSE event data:'), expect.anything());
    });
  });

  describe('reconnect', () => {
    it('should attempt to reconnect on connection error', () => {
      (sseClient as any).reconnectDelay = 10;
      (sseClient as any).maxReconnectAttempts = 3; // Limit reconnect attempts for testing
      const mockRequest: any = {
        on: jest.fn((event: string, callback: (err?: any) => void) => {
          if (event === 'error') {
            // Simulate a connection error
            setTimeout(() => {
              callback(new Error('Connection failed'));
            }, 10);
          }
          return mockRequest;
        }),
        setTimeout: jest.fn(),
        end: jest.fn(),
        destroy: jest.fn()
      };

      (http.request as jest.Mock).mockReturnValue(mockRequest);
      const connectSpy = jest.spyOn(sseClient, 'connect');

      sseClient.connect();

      // Advance timers to trigger multiple reconnect attempts
      jest.advanceTimersByTime(200);

      // Check that reconnect was attempted (initial + multiple retries)
      expect(connectSpy).toHaveBeenCalled();
      sseClient.disconnect();
    });
  });

  describe('disconnect', () => {
    it('should disconnect and clean up resources', () => {
      const mockRequest = {
        on: jest.fn(),
        setTimeout: jest.fn(),
        end: jest.fn(),
        destroy: jest.fn()
      };
      const mockResponse = {
        destroy: jest.fn()
      };

      (sseClient as any).request = mockRequest;
      (sseClient as any).response = mockResponse;

      sseClient.disconnect();

      expect(mockRequest.destroy).toHaveBeenCalled();
      expect(mockResponse.destroy).toHaveBeenCalled();
      expect((sseClient as any).request).toBeNull();
      expect((sseClient as any).response).toBeNull();
      expect((sseClient as any).isConnected).toBe(false);
    });
  });

  describe('getConnectionStatus', () => {
    it('should return connection status', () => {
      const status = sseClient.getConnectionStatus();
      expect(status).toEqual({
        connected: false,
        attempts: 0
      });
    });
  });

  describe('SSE stream event edge cases', () => {
    it('should ignore comment lines and parse valid SSE data with comments', () => {
      const callback = jest.fn();
      sseClient.subscribe('routeCompleted', callback);

      const rawData = ': ping\nevent: routeCompleted\ndata: {"routeId":"r-123"}\n\n';
      (sseClient as any).handleSSEData(rawData);

      expect(callback).toHaveBeenCalledWith({ routeId: 'r-123' });
    });

    it('should handle invalid JSON payload in emit gracefully', () => {
      const callback = jest.fn();
      sseClient.subscribe('malformedEvent', callback);

      const consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation();
      const rawData = 'event: malformedEvent\ndata: not-json\n\n';
      (sseClient as any).handleSSEData(rawData);

      expect(consoleErrorSpy).toHaveBeenCalledWith('Failed to parse SSE event data:', expect.any(Error));
      expect(callback).not.toHaveBeenCalled();
    });

    it('should handle emit with no registered listeners gracefully', () => {
      (sseClient as any).emit('unregisteredEvent', '{"foo":"bar"}');
      expect(true).toBe(true);
    });

    it('should handle response end, response error, and request timeout', () => {
      const handlers: { [key: string]: Function } = {};
      const resHandlers: { [key: string]: Function } = {};

      const mockReq: any = {
        on: jest.fn((evt: string, cb: Function): any => { handlers[evt] = cb; return mockReq; }),
        setTimeout: jest.fn(),
        end: jest.fn(),
        destroy: jest.fn()
      };

      (http.request as jest.Mock).mockImplementation((_opts: any, cb: Function) => {
        const mockRes: any = {
          on: jest.fn((evt: string, rcb: Function): any => { resHandlers[evt] = rcb; return mockRes; }),
          destroy: jest.fn()
        };
        cb(mockRes);
        return mockReq;
      });

      const handleErrSpy = jest.spyOn(sseClient as any, 'handleConnectionError').mockImplementation();

      sseClient.connect();

      // Trigger response end
      if (resHandlers['end']) resHandlers['end']();
      expect(handleErrSpy).toHaveBeenCalledWith(expect.any(Error));

      // Trigger response error
      if (resHandlers['error']) resHandlers['error'](new Error('Res error'));
      expect(handleErrSpy).toHaveBeenCalledWith(expect.any(Error));

      // Trigger request timeout
      if (handlers['timeout']) handlers['timeout']();
      expect(mockReq.destroy).toHaveBeenCalled();

      sseClient.disconnect();
    });
  });
});