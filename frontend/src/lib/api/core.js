import {
  getClockOffset,
  getSampleCount,
  isClockDriftSignificant,
  updateOffset,
} from '../utils/serverClock.js';

// Use relative path for API calls - Vite proxy will handle dev, production uses same origin
export const API_BASE = '/api';

// Ensure the clock-drift warning toast fires at most once per session
let driftWarningShown = false;

/**
 * Create an enhanced error object from an API response
 * @param {Response} response - Fetch Response object
 * @param {string} responseText - Response body text
 * @returns {Error} Enhanced error object with code, details, etc.
 */
function createApiError(response, responseText) {
  const error = new Error(responseText || `Request failed: ${response.statusText}`);

  // Try to parse structured error from response
  try {
    const parsed = JSON.parse(responseText);
    error.code = parsed.code;
    error.errorCode = parsed.code; // Alias for compatibility
    error.details = parsed.details || {};
    error.requestId = parsed.request_id;
    error.message = parsed.error || parsed.message || error.message;
  } catch {
    // Response is not JSON, keep original message
  }

  // Add HTTP status info
  error.status = response.status;
  error.statusText = response.statusText;

  return error;
}

export async function fetchAPI(endpoint, options = {}) {
  const headers = {
    'Content-Type': 'application/json',
    ...options.headers,
  };

  const response = await fetch(`${API_BASE}${endpoint}`, {
    ...options,
    credentials: 'same-origin', // Include cookies for session auth
    headers,
  });

  // Track server-vs-client clock offset from the Date header
  updateOffset(response.headers.get('Date'));

  // After enough samples, warn admins once if drift is significant
  if (!driftWarningShown && getSampleCount() >= 3 && isClockDriftSignificant()) {
    driftWarningShown = true;
    // Dynamic import avoids circular deps (stores → api → stores)
    Promise.all([import('../stores'), import('../stores/toasts.svelte.js')]).then(
      ([{ authStore }, { warningToast }]) => {
        let user;
        authStore.subscribe((s) => (user = s.currentUser))();
        if (user?.is_system_admin) {
          const offsetSec = Math.round(getClockOffset() / 1000);
          const absMin = Math.floor(Math.abs(offsetSec) / 60);
          const absSec = Math.abs(offsetSec) % 60;
          const direction = offsetSec > 0 ? 'ahead' : 'behind';
          const amount =
            absMin > 0 ? `${absMin}m ${absSec}s ${direction}` : `${absSec}s ${direction}`;
          warningToast(`Server clock appears to be ${amount}. Timestamps may be inaccurate.`);
        }
      }
    );
  }

  if (!response.ok) {
    // Handle authentication errors
    if (response.status === 401) {
      // Import auth store dynamically to avoid circular dependencies
      const { authStore } = await import('../stores');
      authStore.clearAuth();
    }
    // Try to get a more descriptive error message from the response body
    let errorData = '';
    try {
      errorData = await response.text();
    } catch (_e) {
      // If we can't read the error body, use the status text
    }
    throw createApiError(response, errorData);
  }

  if (response.status === 204) {
    return null;
  }

  const contentType = response.headers.get('content-type');
  if (contentType?.includes('application/json')) {
    return response.json();
  }

  return null;
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
