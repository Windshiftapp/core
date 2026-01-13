// Use relative path for API calls - Vite proxy will handle dev, production uses same origin
export const API_BASE = '/api';

// CSRF token management
let csrfToken = null;

export async function getCSRFToken() {
  if (!csrfToken) {
    try {
      const response = await fetch(`${API_BASE}/csrf-token`, {
        method: 'GET',
        credentials: 'same-origin'
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
    if (token && options.method && ['POST', 'PUT', 'DELETE', 'PATCH'].includes(options.method.toUpperCase())) {
      headers['X-CSRF-Token'] = token;
      const retryResponse = await fetch(`${API_BASE}${endpoint}`, {
        ...options,
        credentials: 'same-origin',
        headers,
      });

      if (!retryResponse.ok) {
        // Try to get a more descriptive error message from the response body
        let errorMessage = `Request failed: ${retryResponse.statusText}`;
        try {
          const errorData = await retryResponse.text();
          if (errorData) {
            errorMessage = errorData;
          }
        } catch (e) {
          // If we can't read the error body, use the status text
        }
        throw new Error(errorMessage);
      }

      if (retryResponse.status === 204) {
        return null;
      }

      const contentType = retryResponse.headers.get('content-type');
      if (contentType && contentType.includes('application/json')) {
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
    let errorMessage = `Request failed: ${response.statusText}`;
    try {
      const errorData = await response.text();
      if (errorData) {
        errorMessage = errorData;
      }
    } catch (e) {
      // If we can't read the error body, use the status text
    }
    throw new Error(errorMessage);
  }

  if (response.status === 204) {
    return null;
  }

  const contentType = response.headers.get('content-type');
  if (contentType && contentType.includes('application/json')) {
    return response.json();
  }

  return null;
}

// Generic HTTP methods
export const get = (endpoint) => fetchAPI(endpoint);
export const post = (endpoint, data) => fetchAPI(endpoint, {
  method: 'POST',
  body: JSON.stringify(data),
});
export const put = (endpoint, data) => fetchAPI(endpoint, {
  method: 'PUT',
  body: JSON.stringify(data),
});
export const del = (endpoint) => fetchAPI(endpoint, {
  method: 'DELETE',
});
