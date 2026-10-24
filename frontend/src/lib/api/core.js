import { toLogical } from '../runtime/contextPath.js';
import {
  getClockOffset,
  getSampleCount,
  isClockDriftSignificant,
  updateOffset,
} from '../utils/serverClock.js';
import { READ_PRIORITY, scheduleRead } from './readScheduler.js';

// Bounded recovery for transient per-user concurrency rejections. Only safe,
// finite reads are eligible; mutations are never automatically replayed.
const RETRY_MAX_ATTEMPTS = 3;
const RETRY_MAX_ELAPSED_MS = 30000;
const RETRY_DEFAULT_DELAYS_MS = [1000, 2000];
const RETRY_JITTER_MS = 250;

// Use relative path for API calls - Vite proxy will handle dev, production uses same origin
export const API_BASE = '/api';
export const API_V2_BASE = '/api/v2';
export const ADMIN_UI_MUTATION_EVENT = 'windshift:admin-ui-mutation';

// Ensure the clock-drift warning toast fires at most once per session
let driftWarningShown = false;

// The API layer must not import stores or the router: those imports would put
// every API module back behind entry-level importers and turn store edits into
// full page reloads in dev. The app shell registers the side effects instead.
const apiEventHandlers = {
  /** @type {null | (() => void)} */
  onAuthExpired: null,
  /** @type {null | ((event: { offsetMs: number }) => void)} */
  onClockDrift: null,
};

/**
 * Register app-shell side effects for API responses. Handlers left out of
 * `handlers` keep their current value; pass `null` to unregister one.
 * @param {{ onAuthExpired?: (() => void) | null, onClockDrift?: ((event: { offsetMs: number }) => void) | null }} [handlers]
 */
export function configureAPIEventHandlers(handlers = {}) {
  if ('onAuthExpired' in handlers) apiEventHandlers.onAuthExpired = handlers.onAuthExpired ?? null;
  if ('onClockDrift' in handlers) apiEventHandlers.onClockDrift = handlers.onClockDrift ?? null;
}

// The per-user concurrency limiter (internal/middleware/user_concurrency.go)
// answers bursts with 429 + Retry-After. Safe, finite reads recover from that
// within the shared budget; writes are never automatically replayed.
const SAFE_METHODS = ['GET', 'HEAD', 'OPTIONS'];

// Chromium does not consistently report fetches cancelled by a document
// navigation as AbortError. Track the page lifecycle explicitly so callers do
// not mistake unload cancellation for a connectivity failure. `pageshow`
// restores the flag when a document returns from the back-forward cache.
let documentUnloading = false;
if (typeof window !== 'undefined') {
  window.addEventListener('pagehide', () => {
    documentUnloading = true;
  });
  window.addEventListener('pageshow', () => {
    documentUnloading = false;
  });
}

function requestLocale() {
  if (typeof document !== 'undefined' && document.documentElement.lang) {
    return document.documentElement.lang;
  }
  if (typeof localStorage !== 'undefined') {
    const saved = localStorage.getItem('windshift-locale');
    if (saved) return saved;
  }
  return typeof navigator !== 'undefined' ? navigator.language : 'en';
}

// In-flight GET ownership is scoped to the authenticated browser session.
// Settled responses are never retained here: this removes duplicate concurrent
// network work without turning live endpoints into a cache.
let apiRequestSessionKey = null;
const inFlightGetRequests = new Map();

function normalizeGETEndpoint(endpoint) {
  try {
    const url = new URL(endpoint, 'https://windshift.invalid');
    url.searchParams.sort();
    return `${url.pathname}${url.search}`;
  } catch {
    return endpoint;
  }
}

function inFlightGETKey(base, endpoint, options) {
  if (!apiRequestSessionKey) return null;
  const method = String(options?.method || 'GET').toUpperCase();
  if (method !== 'GET') return null;

  // Caller-specific request controls cannot safely share one underlying
  // fetch: aborting or timing out one consumer must not cancel another.
  // Scheduler metadata is not caller-specific and must not break sharing.
  const optionKeys = Object.keys(options || {});
  if (optionKeys.some((key) => key !== 'method' && key !== 'readPolicy')) return null;

  return `${apiRequestSessionKey}|${requestLocale()}|${base}|${normalizeGETEndpoint(endpoint)}`;
}

function isAdminUIPath() {
  if (typeof window === 'undefined') return false;
  const pathname = toLogical(window.location.pathname);
  return pathname === '/admin' || pathname.startsWith('/admin/');
}

function notifyAdminUIMutation(endpoint, method) {
  if (!isAdminUIPath()) return;
  window.dispatchEvent(
    new CustomEvent(ADMIN_UI_MUTATION_EVENT, {
      detail: { endpoint, method },
    })
  );
}

/** Replace the in-flight ownership scope after authentication changes. */
export function setAPIRequestSessionKey(sessionKey) {
  const nextKey = sessionKey == null ? null : String(sessionKey);
  if (nextKey === apiRequestSessionKey) return;
  apiRequestSessionKey = nextKey;
  inFlightGetRequests.clear();
}

export function clearAPIRequestSessionKey() {
  setAPIRequestSessionKey(null);
}

/**
 * Create an enhanced error object from an API response
 * @param {Response} response - Fetch Response object
 * @param {string} responseText - Response body text
 * @returns {Error} Enhanced error object with code, details, etc.
 */
function createApiError(response, responseText) {
  let fallbackMessage = `Request failed: ${response.statusText}`;
  if (!responseText && (response.status === 502 || response.status === 504)) {
    fallbackMessage = 'The server took too long to respond. Please try again shortly.';
  }
  const error = new Error(responseText || fallbackMessage);

  // Try to parse structured error from response
  try {
    const parsed = JSON.parse(responseText);
    const payload =
      typeof parsed.error === 'object' && parsed.error !== null ? parsed.error : parsed;
    /** @type {any} */ (error).code = payload.code;
    /** @type {any} */ (error).errorCode = payload.code; // Alias for compatibility
    /** @type {any} */ (error).details = payload.details || {};
    /** @type {any} */ (error).requestId = parsed.request_id;
    /** @type {any} */ (error).body = parsed;
    // Authentication policy responses carry flow-control fields alongside the
    // normal error envelope. Mirror the explicit fields callers need while
    // retaining the complete body for diagnostics.
    /** @type {any} */ (error).passkey_required = parsed.passkey_required === true;
    /** @type {any} */ (error).enrollment_required = parsed.enrollment_required === true;
    /** @type {any} */ (error).sso_required = parsed.sso_required === true;
    /** @type {any} */ (error).policy_message = parsed.policy_message;
    error.message =
      (typeof parsed.error === 'string' ? parsed.error : payload.message) || error.message;
  } catch {
    // Response is not JSON, keep original message
  }

  // Add HTTP status info
  /** @type {any} */ (error).status = response.status;
  /** @type {any} */ (error).statusText = response.statusText;
  // Retry-After recovery needs the raw header plus the server clock so a
  // relative HTTP-date delay is measured against the server's now, not ours.
  /** @type {any} */ (error).retryAfter = response.headers?.get?.('Retry-After') ?? null;
  /** @type {any} */ (error).serverDate = response.headers?.get?.('Date') ?? null;

  return error;
}

/**
 * @param {string} base
 * @param {string} endpoint
 * @param {RequestInit & { timeout?: number; text?: boolean }} [options]
 */
async function performFetchAPI(base, endpoint, options = {}) {
  const {
    timeout: requestedTimeout = 0,
    signal: callerSignal,
    // Return the raw response body as text instead of parsing JSON —
    // used by endpoints that serve non-JSON documents (e.g. Gherkin).
    text: asText = false,
    ...fetchOptions
  } = options;
  const isFormData = typeof FormData !== 'undefined' && fetchOptions.body instanceof FormData;
  const headers = isFormData
    ? { 'Accept-Language': requestLocale(), ...fetchOptions.headers }
    : {
        'Content-Type': 'application/json',
        'Accept-Language': requestLocale(),
        ...fetchOptions.headers,
      };

  const timeoutMs = Number(requestedTimeout);
  const hasTimeout = Number.isFinite(timeoutMs) && timeoutMs > 0;
  const controller = hasTimeout ? new AbortController() : null;
  let timedOut = false;
  let timeoutId;
  let removeCallerAbortListener = () => {};

  if (controller && callerSignal) {
    const abortFromCaller = () => controller.abort();
    if (callerSignal.aborted) abortFromCaller();
    else {
      callerSignal.addEventListener('abort', abortFromCaller, { once: true });
      removeCallerAbortListener = () => callerSignal.removeEventListener('abort', abortFromCaller);
    }
  }
  if (controller) {
    timeoutId = setTimeout(() => {
      timedOut = true;
      controller.abort();
    }, timeoutMs);
  }

  const cleanup = () => {
    if (timeoutId !== undefined) clearTimeout(timeoutId);
    removeCallerAbortListener();
  };

  const timeoutError = () => {
    const error = new Error(
      'The server took too long to respond. Check your connection and try again.'
    );
    /** @type {any} */ (error).status = 0;
    /** @type {any} */ (error).code = 'REQUEST_TIMEOUT';
    return error;
  };

  let response;
  try {
    response = await fetch(`${base}${endpoint}`, {
      ...fetchOptions,
      credentials: 'same-origin', // Include cookies for session auth
      headers,
      signal: controller?.signal ?? callerSignal,
    });
  } catch (err) {
    cleanup();
    if (timedOut) throw timeoutError();
    // Caller-driven cancellation is control flow, not a connectivity failure.
    // Preserve AbortError so route/store loaders can silently discard superseded
    // work while the browser tears down the underlying HTTP request.
    if (err?.name === 'AbortError') throw err;
    // Chromium can surface fetches cancelled by a document navigation as a
    // TypeError instead of AbortError. Only an actual unload may claim that:
    // a hidden but live tab must still report offline/DNS/TLS failures as
    // connectivity errors so recovery handling is not bypassed.
    if (err instanceof TypeError && documentUnloading) {
      throw new DOMException('The document was unloaded', 'AbortError');
    }
    // Network errors (including offline, DNS, TLS, and CORS failures) surface
    // identically through fetch. Keep the user-facing copy actionable without
    // incorrectly diagnosing an ordinary mobile connectivity loss as CORS.
    const networkError = new Error(
      'Unable to connect to the server. Check your connection and try again.'
    );
    /** @type {any} */ (networkError).status = 0;
    /** @type {any} */ (networkError).code = 'NETWORK_ERROR';
    throw networkError;
  }

  try {
    // Track server-vs-client clock offset from the Date header
    updateOffset(response.headers.get('Date'));

    // After enough samples, report drift once so the shell can warn admins.
    if (!driftWarningShown && getSampleCount() >= 3 && isClockDriftSignificant()) {
      driftWarningShown = true;
      apiEventHandlers.onClockDrift?.({ offsetMs: getClockOffset() });
    }

    if (!response.ok) {
      // Read and parse the response before deciding whether a 401 represents an
      // expired login. Pending-auth sessions must survive while the user
      // completes enrollment or second-factor verification.
      let errorData = '';
      try {
        errorData = await response.text();
      } catch (_e) {
        // If we can't read the error body, use the status text
      }
      const apiError = createApiError(response, errorData);
      if (
        response.status === 401 &&
        /** @type {any} */ (apiError).code !== 'AUTHENTICATION_PENDING'
      ) {
        apiEventHandlers.onAuthExpired?.();
      }
      throw apiError;
    }

    let result = null;
    if (response.status !== 204) {
      const contentType = response.headers.get('content-type');
      if (asText) {
        result = await response.text();
      } else if (contentType?.includes('application/json')) {
        result = await response.json();
      }
    }

    const method = String(fetchOptions.method || 'GET').toUpperCase();
    if (!SAFE_METHODS.includes(method)) {
      notifyAdminUIMutation(endpoint, method);
    }

    return result;
  } catch (error) {
    if (timedOut) throw timeoutError();
    throw error;
  } finally {
    cleanup();
  }
}

export function fetchAPI(endpoint, options = {}) {
  return fetchFromAPI(API_BASE, endpoint, options);
}

export function fetchAPIV2(endpoint, options = {}) {
  return fetchFromAPI(API_V2_BASE, endpoint, options);
}

function isSafeReadMethod(options) {
  return SAFE_METHODS.includes(String(options?.method || 'GET').toUpperCase());
}

/**
 * Delay before retrying a 429: Retry-After (seconds or HTTP-date) when present,
 * otherwise 1s then 2s. Jitter is added on top so parallel retries spread out;
 * it never shortens the server-requested delay.
 * @param {any} error
 * @param {number} attempt
 * @param {() => number} [random]
 */
export function concurrencyRetryDelayMs(error, attempt, random = Math.random) {
  return parseRetryAfterMs(error, attempt) + random() * RETRY_JITTER_MS;
}

function parseRetryAfterMs(error, attemptIndex) {
  const fallback = RETRY_DEFAULT_DELAYS_MS[attemptIndex] ?? RETRY_DEFAULT_DELAYS_MS.at(-1);
  const header = error?.retryAfter;
  if (header == null || String(header).trim() === '') return fallback;

  const seconds = Number(header);
  if (Number.isFinite(seconds) && seconds >= 0) return seconds * 1000;

  const dateMs = Date.parse(header);
  if (Number.isNaN(dateMs)) return fallback;

  // A relative HTTP-date is resolved against the server's Date header when
  // present so client clock drift does not lengthen or shorten the wait.
  const serverNow = Date.parse(error?.serverDate);
  const now = Number.isNaN(serverNow) ? Date.now() : serverNow;
  return Math.max(0, dateMs - now);
}

function abortError() {
  return new DOMException('The read was aborted', 'AbortError');
}

function sleepWithAbort(ms, signal) {
  return new Promise((resolve, reject) => {
    if (signal?.aborted) {
      reject(abortError());
      return;
    }
    let timer;
    const onAbort = () => {
      clearTimeout(timer);
      signal?.removeEventListener('abort', onAbort);
      reject(abortError());
    };
    timer = setTimeout(() => {
      signal?.removeEventListener('abort', onAbort);
      resolve();
    }, ms);
    signal?.addEventListener('abort', onAbort, { once: true });
  });
}

/**
 * Perform one physical attempt, entering the shared read budget when the
 * caller opted in with `readPolicy`.
 */
function performFetchAttempt(base, endpoint, options) {
  const { readPolicy, ...rest } = options;
  const priority = readPolicy?.priority;
  if (priority !== READ_PRIORITY.FOREGROUND && priority !== READ_PRIORITY.BACKGROUND) {
    return performFetchAPI(base, endpoint, rest);
  }
  // The scheduler owns the attempt signal so queued/active cancellation can
  // abort the underlying fetch while caller signals keep transport sharing off.
  return scheduleRead(priority, rest.signal, (signal) =>
    performFetchAPI(base, endpoint, { ...rest, signal })
  );
}

async function performFetchWithRetry(base, endpoint, options) {
  if (!isSafeReadMethod(options)) {
    return performFetchAttempt(base, endpoint, options);
  }

  const startedAt = Date.now();
  const sessionKey = apiRequestSessionKey;
  let lastError;

  for (let attempt = 0; attempt < RETRY_MAX_ATTEMPTS; attempt += 1) {
    try {
      return await performFetchAttempt(base, endpoint, options);
    } catch (error) {
      lastError = error;
      if (/** @type {any} */ (error)?.status !== 429) throw error;
      const isLastAttempt = attempt >= RETRY_MAX_ATTEMPTS - 1;
      // A session replacement must not continue an old session's retry
      // sequence. A read that started before auth settled (null key) may still
      // retry once the authenticated session is established.
      const sessionReplaced = sessionKey !== null && apiRequestSessionKey !== sessionKey;
      if (isLastAttempt || sessionReplaced) throw error;

      const delayMs = parseRetryAfterMs(error, attempt);
      const remainingMs = RETRY_MAX_ELAPSED_MS - (Date.now() - startedAt);
      if (delayMs > remainingMs) throw error;

      // The slot is free during this sleep because the failed attempt already
      // released it.
      await sleepWithAbort(concurrencyRetryDelayMs(error, attempt), options?.signal);

      if (sessionKey !== null && apiRequestSessionKey !== sessionKey) throw lastError;
    }
  }

  throw lastError;
}

export async function fetchV2Data(endpoint, options = {}) {
  const document = await fetchAPIV2(endpoint, options);
  return document?.data;
}

/** Fetch a v2 endpoint that responds with a plain text document. */
export async function fetchV2Text(endpoint, options = {}) {
  return fetchAPIV2(endpoint, { ...options, text: true });
}

export async function fetchAllV2Pages(endpoint, options = {}) {
  // Fat payloads can raise pageSize per call to cut round trips; the v2 API
  // caps page_size at 1000.
  const { pageSize = 100, ...fetchOptions } = options;
  const url = new URL(endpoint, 'https://windshift.invalid');
  url.searchParams.set('page_size', String(pageSize));
  const items = [];
  const seenIds = new Set();
  let page = 1;
  let totalPages = 1;
  do {
    url.searchParams.set('page', String(page));
    const document = await fetchAPIV2(`${url.pathname}${url.search}`, fetchOptions);
    for (const item of document?.data ?? []) {
      // Offset pagination drifts when rows shift between page fetches, so the
      // same row can appear on two consecutive pages. Keep the first copy.
      const key = item?.id ?? item?.key;
      if (key !== undefined && key !== null) {
        const dedupeKey = String(key);
        if (seenIds.has(dedupeKey)) continue;
        seenIds.add(dedupeKey);
      }
      items.push(item);
    }
    totalPages = document?.pagination?.total_pages ?? 0;
    page += 1;
  } while (page <= totalPages);
  return items;
}

function fetchFromAPI(base, endpoint, options) {
  const key = inFlightGETKey(base, endpoint, options);
  // Retry lives inside the shared promise so duplicate callers observe one
  // sequence and never multiply the physical attempts.
  if (!key) return performFetchWithRetry(base, endpoint, options);

  const existing = inFlightGetRequests.get(key);
  if (existing) return existing;

  let trackedRequest;
  trackedRequest = performFetchWithRetry(base, endpoint, options).finally(() => {
    if (inFlightGetRequests.get(key) === trackedRequest) {
      inFlightGetRequests.delete(key);
    }
  });
  inFlightGetRequests.set(key, trackedRequest);
  return trackedRequest;
}

// Generic HTTP methods
export const get = (endpoint) => fetchAPI(endpoint);
export const post = (endpoint, data) =>
  fetchAPI(endpoint, {
    method: 'POST',
    body: JSON.stringify(data),
  });
export const put = (endpoint, data) =>
  fetchAPI(endpoint, {
    method: 'PUT',
    body: JSON.stringify(data),
  });
export const del = (endpoint) =>
  fetchAPI(endpoint, {
    method: 'DELETE',
  });
