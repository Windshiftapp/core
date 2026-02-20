// Use relative path for API calls - Vite proxy will handle dev, production uses same origin
export const API_BASE = '/api';

// CSRF token management
let csrfToken = null;

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

export async function getCSRFToken() {
  if (!csrfToken) {
    try {
      const response = await fetch(`${API_BASE}/csrf-token`, {
        method: 'GET',
        credentials: 'same-origin',
      });
      if (response.ok) {
        const data = await response.json();
        csrfToken = data.csrf_token;
      }
    } catch (error) {
      console.warn('Failed to get CSRF token:', error);
    }
  }
  return csrfToken;
}

export async function fetchAPI(endpoint, options = {}) {
  const headers = {
    'Content-Type': 'application/json',
    ...options.headers,
  };

  // Add CSRF token for state-changing operations
  if (options.method && ['POST', 'PUT', 'DELETE', 'PATCH'].includes(options.method.toUpperCase())) {
    const token = await getCSRFToken();
    if (token) {
      headers['X-CSRF-Token'] = token;
    }
    // Token is one-time use on the server; clear cache so the next request fetches a fresh one
    csrfToken = null;
  }

  const response = await fetch(`${API_BASE}${endpoint}`, {
    ...options,
    credentials: 'same-origin', // Include cookies for CSRF token
    headers,
  });

  // Handle CSRF token expiration (403 Forbidden)
  if (response.status === 403) {
    // Clear token and retry once
    csrfToken = null;
    const token = await getCSRFToken();
    if (
      token &&
      options.method &&
      ['POST', 'PUT', 'DELETE', 'PATCH'].includes(options.method.toUpperCase())
    ) {
      headers['X-CSRF-Token'] = token;
      const retryResponse = await fetch(`${API_BASE}${endpoint}`, {
        ...options,
        credentials: 'same-origin',
        headers,
      });

      if (!retryResponse.ok) {
        // Try to get a more descriptive error message from the response body
        let errorData = '';
        try {
          errorData = await retryResponse.text();
        } catch (_e) {
          // If we can't read the error body, use the status text
        }
        throw createApiError(retryResponse, errorData);
      }

      if (retryResponse.status === 204) {
        return null;
      }

      const contentType = retryResponse.headers.get('content-type');
      if (contentType?.includes('application/json')) {
        return retryResponse.json();
      }

      return null;
    }
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
