import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

// Stub serverClock — fetchAPI calls updateOffset(headers.Date) on every
// response. Without the stub the real clock-sample logic runs and pulls
// extra dependencies.
vi.mock('../utils/serverClock.js', () => ({
  updateOffset: vi.fn(),
  getClockOffset: vi.fn(() => 0),
  getSampleCount: vi.fn(() => 0),
  isClockDriftSignificant: vi.fn(() => false),
}));

// The 401 branch in fetchAPI dynamically imports '../stores' and calls
// authStore.clearAuth(). Stub it so we can assert the side effect without
// loading the entire stores barrel. The mock factory is hoisted, so the
// clearAuth spy lives inside it — referenced via the mocked module below.
vi.mock('../stores', () => ({
  authStore: { clearAuth: vi.fn(), subscribe: vi.fn() },
}));

import { authStore } from '../stores';
import { fetchAPI } from './core.js';

// Build a Response-like mock without depending on the actual Response
// constructor (jsdom's varies subtly across versions).
function makeResponse({ status = 200, statusText = 'OK', body = '', headers = {} }) {
  const h = new Map(Object.entries(headers));
  return {
    ok: status >= 200 && status < 300,
    status,
    statusText,
    headers: { get: (k) => h.get(k.toLowerCase()) ?? h.get(k) ?? null },
    text: vi.fn(() => Promise.resolve(body)),
    json: vi.fn(() => Promise.resolve(JSON.parse(body || '{}'))),
  };
}

beforeEach(() => {
  vi.clearAllMocks();
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe('fetchAPI — happy path', () => {
  test('returns parsed JSON body for 200 with JSON content-type', async () => {
    global.fetch = vi.fn(() =>
      Promise.resolve(
        makeResponse({
          status: 200,
          body: '{"id":42,"name":"alpha"}',
          headers: { 'content-type': 'application/json' },
        })
      )
    );

    const result = await fetchAPI('/items/42');
    expect(result).toEqual({ id: 42, name: 'alpha' });
    expect(global.fetch).toHaveBeenCalledWith(
      '/api/items/42',
      expect.objectContaining({
        credentials: 'same-origin',
        headers: expect.objectContaining({ 'Content-Type': 'application/json' }),
      })
    );
  });

  test('returns null for 204 No Content', async () => {
    global.fetch = vi.fn(() =>
      Promise.resolve(makeResponse({ status: 204, statusText: 'No Content' }))
    );
    await expect(fetchAPI('/items/42', { method: 'DELETE' })).resolves.toBeNull();
  });

  test('returns null when content-type is not JSON', async () => {
    global.fetch = vi.fn(() =>
      Promise.resolve(
        makeResponse({
          status: 200,
          body: '<html>hi</html>',
          headers: { 'content-type': 'text/html' },
        })
      )
    );
    await expect(fetchAPI('/static/banner')).resolves.toBeNull();
  });
});

describe('fetchAPI — error mapping', () => {
  test('parses structured JSON error envelopes', async () => {
    global.fetch = vi.fn(() =>
      Promise.resolve(
        makeResponse({
          status: 400,
          statusText: 'Bad Request',
          body: JSON.stringify({
            error: 'Title is required',
            code: 'VALIDATION_FAILED',
            details: { field: 'title' },
            request_id: 'req-abc',
          }),
        })
      )
    );

    let caught;
    try {
      await fetchAPI('/items', { method: 'POST', body: '{}' });
    } catch (e) {
      caught = e;
    }

    expect(caught).toBeInstanceOf(Error);
    expect(caught.message).toBe('Title is required');
    expect(caught.code).toBe('VALIDATION_FAILED');
    expect(caught.errorCode).toBe('VALIDATION_FAILED'); // alias
    expect(caught.details).toEqual({ field: 'title' });
    expect(caught.requestId).toBe('req-abc');
    expect(caught.status).toBe(400);
    expect(caught.statusText).toBe('Bad Request');
  });

  test("falls back to 'message' field when 'error' is missing", async () => {
    global.fetch = vi.fn(() =>
      Promise.resolve(
        makeResponse({
          status: 422,
          statusText: 'Unprocessable',
          body: JSON.stringify({ message: 'Validation failed', code: 'VAL' }),
        })
      )
    );

    let caught;
    try {
      await fetchAPI('/x');
    } catch (e) {
      caught = e;
    }
    expect(caught.message).toBe('Validation failed');
    expect(caught.code).toBe('VAL');
  });

  test('non-JSON body becomes the error message verbatim', async () => {
    global.fetch = vi.fn(() =>
      Promise.resolve(
        makeResponse({
          status: 500,
          statusText: 'Internal Server Error',
          body: 'something broke',
        })
      )
    );
    let caught;
    try {
      await fetchAPI('/x');
    } catch (e) {
      caught = e;
    }
    expect(caught.message).toBe('something broke');
    expect(caught.status).toBe(500);
    expect(caught.code).toBeUndefined();
  });

  test('empty 502 produces the dedicated gateway-timeout message', async () => {
    global.fetch = vi.fn(() =>
      Promise.resolve(makeResponse({ status: 502, statusText: 'Bad Gateway', body: '' }))
    );
    let caught;
    try {
      await fetchAPI('/x');
    } catch (e) {
      caught = e;
    }
    expect(caught.message).toBe('The server took too long to respond. Please try again shortly.');
  });

  test('empty 504 produces the dedicated gateway-timeout message', async () => {
    global.fetch = vi.fn(() =>
      Promise.resolve(makeResponse({ status: 504, statusText: 'Gateway Timeout', body: '' }))
    );
    let caught;
    try {
      await fetchAPI('/x');
    } catch (e) {
      caught = e;
    }
    expect(caught.message).toBe('The server took too long to respond. Please try again shortly.');
  });

  test('501 with empty body uses statusText fallback (not the 502/504 message)', async () => {
    global.fetch = vi.fn(() =>
      Promise.resolve(makeResponse({ status: 501, statusText: 'Not Implemented', body: '' }))
    );
    let caught;
    try {
      await fetchAPI('/x');
    } catch (e) {
      caught = e;
    }
    expect(caught.message).toBe('Request failed: Not Implemented');
  });
});

describe('fetchAPI — 401 logout side effect', () => {
  test('calls authStore.clearAuth() when the server returns 401', async () => {
    global.fetch = vi.fn(() =>
      Promise.resolve(
        makeResponse({
          status: 401,
          statusText: 'Unauthorized',
          body: JSON.stringify({ error: 'no session' }),
        })
      )
    );

    await expect(fetchAPI('/items')).rejects.toThrow('no session');
    expect(authStore.clearAuth).toHaveBeenCalledTimes(1);
  });

  test('does not call clearAuth on non-401 errors', async () => {
    global.fetch = vi.fn(() =>
      Promise.resolve(
        makeResponse({
          status: 403,
          body: JSON.stringify({ error: 'forbidden' }),
        })
      )
    );
    await expect(fetchAPI('/items')).rejects.toThrow('forbidden');
    expect(authStore.clearAuth).not.toHaveBeenCalled();
  });
});

describe('fetchAPI — network/CORS errors', () => {
  test('TypeError from fetch surfaces as a NETWORK_ERROR with helpful copy', async () => {
    global.fetch = vi.fn(() => Promise.reject(new TypeError('Failed to fetch')));

    let caught;
    try {
      await fetchAPI('/items');
    } catch (e) {
      caught = e;
    }
    expect(caught).toBeInstanceOf(Error);
    expect(caught.status).toBe(0);
    expect(caught.code).toBe('NETWORK_ERROR');
    // The user-facing message hints at the most common cause (CORS).
    expect(caught.message).toMatch(/CORS configuration/);
  });
});

describe('fetchAPI — request shape', () => {
  test('threads options.headers and body through to fetch', async () => {
    global.fetch = vi.fn(() =>
      Promise.resolve(
        makeResponse({
          status: 200,
          body: '{}',
          headers: { 'content-type': 'application/json' },
        })
      )
    );

    await fetchAPI('/items', {
      method: 'POST',
      headers: { 'X-Custom': 'yes' },
      body: '{"x":1}',
    });

    expect(global.fetch).toHaveBeenCalledWith(
      '/api/items',
      expect.objectContaining({
        method: 'POST',
        body: '{"x":1}',
        headers: expect.objectContaining({
          'Content-Type': 'application/json',
          'X-Custom': 'yes',
        }),
      })
    );
  });
});
