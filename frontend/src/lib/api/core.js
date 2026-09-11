import { toLogical } from '../runtime/contextPath.js';
import {
  getClockOffset,
  getSampleCount,
  isClockDriftSignificant,
  updateOffset,
} from '../utils/serverClock.js';

// Use relative path for API calls - Vite proxy will handle dev, production uses same origin
export const API_BASE = '/api';
export const API_V2_BASE = '/api/v2';
export const ADMIN_UI_MUTATION_EVENT = 'windshift:admin-ui-mutation';

// Ensure the clock-drift warning toast fires at most once per session
let driftWarningShown = false;

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
  const optionKeys = Object.keys(options || {});
  if (optionKeys.some((key) => key !== 'method')) return null;

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

  return error;
}

/**
 * @param {string} base
 * @param {string} endpoint
 * @param {RequestInit & { timeout?: number }} [options]
 */
async function performFetchAPI(base, endpoint, options = {}) {
  const { timeout: requestedTimeout = 0, signal: callerSignal, ...fetchOptions } = options;
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
    // TypeError instead of AbortError. Once the document is hidden, no caller
    // can use the response, so preserve the cancellation semantics rather than
    // reporting a spurious connectivity failure during reload/unload.
    if (
      err instanceof TypeError &&
      (documentUnloading ||
        (typeof document !== 'undefined' && document.visibilityState === 'hidden'))
    ) {
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

    // After enough samples, warn admins once if drift is significant
    if (!driftWarningShown && getSampleCount() >= 3 && isClockDriftSignificant()) {
      driftWarningShown = true;
      // Dynamic import avoids circular deps (stores → api → stores)
      Promise.all([
        import('../stores'),
        import('../stores/toasts.svelte.js'),
        import('../stores/i18n.svelte.js'),
        import('../router.js'),
      ]).then(([{ authStore }, { addToast }, { t }, { navigate }]) => {
        let user;
        authStore.subscribe((s) => (user = s.currentUser))();
        if (/** @type {any} */ (user)?.is_system_admin) {
          const offsetSec = Math.round(getClockOffset() / 1000);
          const absMin = Math.floor(Math.abs(offsetSec) / 60);
          const absSec = Math.abs(offsetSec) % 60;
          const direction = offsetSec > 0 ? 'ahead' : 'behind';
          const amount =
            absMin > 0 ? `${absMin}m ${absSec}s ${direction}` : `${absSec}s ${direction}`;
          addToast({
            message: `Server clock appears to be ${amount}. Click for details.`,
            title: t('toast.warning'),
            variant: 'warning',
            clickable: true,
            onClick: () => navigate('/admin/diagnostics?subtab=clock'),
          });
        }
      });
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
        // Import auth store dynamically to avoid circular dependencies
        const { authStore } = await import('../stores');
        authStore.clearAuth();
      }
      throw apiError;
    }

    let result = null;
    if (response.status !== 204) {
      const contentType = response.headers.get('content-type');
      if (contentType?.includes('application/json')) {
        result = await response.json();
      }
    }

    const method = String(fetchOptions.method || 'GET').toUpperCase();
    if (!['GET', 'HEAD', 'OPTIONS'].includes(method)) {
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

export async function fetchV2Data(endpoint, options = {}) {
  const document = await fetchAPIV2(endpoint, options);
  return document?.data;
}

export async function fetchAllV2Pages(endpoint, options = {}) {
  const url = new URL(endpoint, 'https://windshift.invalid');
  url.searchParams.set('page_size', '100');
  const items = [];
  let page = 1;
  let totalPages = 1;
  do {
    url.searchParams.set('page', String(page));
    const document = await fetchAPIV2(`${url.pathname}${url.search}`, options);
    items.push(...(document?.data ?? []));
    totalPages = document?.pagination?.total_pages ?? 0;
    page += 1;
  } while (page <= totalPages);
  return items;
}

function fetchFromAPI(base, endpoint, options) {
  const key = inFlightGETKey(base, endpoint, options);
  if (!key) return performFetchAPI(base, endpoint, options);

  const existing = inFlightGetRequests.get(key);
  if (existing) return existing;

  let trackedRequest;
  trackedRequest = performFetchAPI(base, endpoint, options).finally(() => {
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
