import { derived, writable } from 'svelte/store';
import { api } from '../api.js';

function createAuthStore() {
  const user = writable(null);
  const session = writable(null);
  const isAuthenticated = writable(false);
  const loading = writable(false);
  const error = writable(null);

  // Create a combined derived store for easy subscription
  const combined = derived(
    [user, session, isAuthenticated, loading, error],
    ([$user, $session, $isAuthenticated, $loading, $error]) => ({
      user: $user,
      currentUser: $user,
      session: $session,
      isAuthenticated: $isAuthenticated,
      loading: $loading,
      error: $error,
    })
  );

  return {
    // Subscribe to combined state
    subscribe: combined.subscribe,

    // Convenience getters for backwards compatibility with direct property access
    get currentUser() {
      let value;
      user.subscribe((v) => (value = v))();
      return value;
    },

    get isAuthenticated() {
      let value;
      isAuthenticated.subscribe((v) => (value = v))();
      return value;
    },

    get loading() {
      let value;
      loading.subscribe((v) => (value = v))();
      return value;
    },

    get error() {
      let value;
      error.subscribe((v) => (value = v))();
      return value;
    },

    // Initialize auth state by checking current session
    async init() {
      loading.set(true);

      try {
        const response = await api.auth.getCurrentUser();
        user.set(response.user);
        session.set(response.session);
        isAuthenticated.set(true);
        loading.set(false);
        error.set(null);
      } catch (_err) {
        // If we can't get current user, user is not authenticated
        user.set(null);
        session.set(null);
        isAuthenticated.set(false);
        loading.set(false);
        error.set(null);
      }
    },

    // Login with credentials
    async login(credentials) {
      loading.set(true);
      error.set(null);

      try {
        const response = await api.auth.login(credentials);

        // Handle policy-related responses
        if (response.sso_required) {
          isAuthenticated.set(false);
          loading.set(false);
          error.set(response.policy_message || 'SSO login required');
          return {
            success: false,
            sso_required: true,
            policy_message:
              response.policy_message || 'Password login is disabled. Please use SSO.',
          };
        }

        if (response.success) {
          user.set(response.user);
          isAuthenticated.set(true);
          loading.set(false);
          error.set(null);

          // Get session details
          const sessionResponse = await api.auth.getCurrentUser();
          session.set(sessionResponse.session);

          // Return enrollment status if required
          return {
            success: true,
            enrollment_required: response.enrollment_required || false,
            policy_message: response.policy_message,
          };
        } else {
          isAuthenticated.set(false);
          loading.set(false);
          error.set(response.message || 'Login failed');
          return { success: false, message: response.message || 'Login failed' };
        }
      } catch (err) {
        // Check if error response contains policy info
        if (err.sso_required) {
          isAuthenticated.set(false);
          loading.set(false);
          error.set(err.policy_message || 'SSO login required');
          return {
            success: false,
            sso_required: true,
            policy_message: err.policy_message || 'Password login is disabled. Please use SSO.',
          };
        }

        isAuthenticated.set(false);
        loading.set(false);
        error.set(err.message || 'Login failed');
        return { success: false, message: err.message || 'Login failed' };
      }
    },

    // Logout
    async logout() {
      loading.set(true);

      try {
        await api.auth.logout();
      } catch (err) {
        console.warn('Logout API call failed:', err);
      }

      // Clear auth state regardless of API call result
      user.set(null);
      session.set(null);
      isAuthenticated.set(false);
      loading.set(false);
      error.set(null);
    },

    // Logout from all sessions
    async logoutAll() {
      loading.set(true);

      try {
        await api.auth.logoutAll();
      } catch (err) {
        console.warn('Logout all API call failed:', err);
      }

      // Clear auth state regardless of API call result
      user.set(null);
      session.set(null);
      isAuthenticated.set(false);
      loading.set(false);
      error.set(null);
    },

    // Refresh session
    async refreshSession(rememberMe = false) {
      try {
        await api.auth.refreshSession({ remember_me: rememberMe });

        // Update session info
        const response = await api.auth.getCurrentUser();
        session.set(response.session);

        return true;
      } catch (err) {
        console.warn('Session refresh failed:', err);
        return false;
      }
    },

    // Change password
    async changePassword(passwordData) {
      loading.set(true);
      error.set(null);

      try {
        const response = await api.auth.changePassword(passwordData);
        loading.set(false);
        error.set(null);

        return { success: true, message: response.message || 'Password changed successfully' };
      } catch (err) {
        loading.set(false);
        error.set(err.message || 'Failed to change password');
        return { success: false, message: err.message || 'Failed to change password' };
      }
    },

    // Clear authentication (called on 401 errors)
    clearAuth() {
      user.set(null);
      session.set(null);
      isAuthenticated.set(false);
      loading.set(false);
      error.set('Session expired. Please log in again.');
    },

    // Set authentication data (used by FIDO login)
    setAuthData(userData, sessionData) {
      user.set(userData);
      session.set(sessionData);
      isAuthenticated.set(true);
      loading.set(false);
      error.set(null);
    },

    // Clear error
    clearError() {
      error.set(null);
    },
  };
}

export const authStore = createAuthStore();
