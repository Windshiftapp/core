import { derived, writable } from 'svelte/store';
import { auth } from '../api/auth.js';
import { setAPIRequestSessionKey } from '../api/core.js';
import { resetSLAState } from '../features/sla/slaState.js';
import { clearStores, getStoreValue } from './storeUtils.js';
import { workspacesStore } from './workspaces.svelte.js';

function policyValue(error, field) {
  return error?.[field] ?? error?.body?.[field];
}

function hasPolicyFlag(error, field) {
  return policyValue(error, field) === true;
}

function createAuthStore() {
  /** @type {import('svelte/store').Writable<{id: string, email: string, name: string, language: string, avatar_url: string, role: string, is_system_admin?: boolean, [key: string]: any} | null>} */
  const user = writable(null);
  const session = writable(null);
  const isAuthenticated = writable(false);
  const loading = writable(false);
  const error = writable(null);

  // Identity-scoped stores (workspace list, personal workspace id) must never
  // survive an un-authentication event — the next login belongs to a
  // potentially different user.
  function resetIdentity() {
    clearStores(user, session, error);
    workspacesStore.clear();
  }

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

  // Keep API request coalescing isolated to one authenticated session. The
  // server does not expose the cookie token, so user id + session creation
  // time is the stable public session marker; tests/legacy responses may
  // provide an explicit id instead.
  let lastSessionKey = null;
  combined.subscribe((state) => {
    if (!state.isAuthenticated || state.user?.id == null) {
      if (lastSessionKey !== null) {
        // Protected SLA data must not survive a session change.
        resetSLAState();
        lastSessionKey = null;
      }
      setAPIRequestSessionKey(null);
      return;
    }
    const marker =
      state.session?.id || state.session?.created_at || state.session?.expires_at || 'active';
    const nextKey = `auth:${state.user.id}:${marker}`;
    if (nextKey !== lastSessionKey) {
      resetSLAState();
      lastSessionKey = nextKey;
    }
    setAPIRequestSessionKey(nextKey);
  });

  return {
    // Subscribe to combined state
    subscribe: combined.subscribe,

    // Convenience getters for backwards compatibility with direct property access
    get currentUser() {
      return getStoreValue(user);
    },

    get isAuthenticated() {
      return getStoreValue(isAuthenticated);
    },

    get loading() {
      return getStoreValue(loading);
    },

    get error() {
      return getStoreValue(error);
    },

    // Initialize auth state by checking current session
    async init(options) {
      loading.set(true);

      try {
        const response = await auth.getCurrentUser(options);
        if (response.session?.auth_pending_type === 'passkey_verification') {
          // A refreshed password+passkey browser session is still only the
          // first factor. Return to login rather than rendering the app as if
          // authentication had completed; the user can restart the short flow.
          clearStores(user, session);
          isAuthenticated.set(false);
          loading.set(false);
          error.set(null);
          return { status: 'unauthenticated' };
        }
        user.set(response.user);
        session.set(response.session);
        isAuthenticated.set(true);
        loading.set(false);
        error.set(null);
        return { status: 'authenticated' };
      } catch (err) {
        loading.set(false);
        if (err?.status === 401) {
          // Only an explicit unauthenticated response proves that the browser
          // session is invalid. A transport failure leaves the prior state
          // intact and lets the bootstrap UI offer a meaningful retry.
          user.set(null);
          session.set(null);
          isAuthenticated.set(false);
          error.set(null);
          return { status: 'unauthenticated' };
        }
        error.set(err?.message || 'Unable to verify your session.');
        return { status: 'error', error: err };
      }
    },

    // Login with credentials
    async login(credentials) {
      loading.set(true);
      error.set(null);

      try {
        const response = await auth.login(credentials);

        // Handle policy-related responses
        if (response.sso_required) {
          clearStores(user, session);
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

        if (response.passkey_required) {
          clearStores(user, session);
          isAuthenticated.set(false);
          loading.set(false);
          error.set(null);
          return {
            success: false,
            passkey_required: true,
            policy_message: response.policy_message,
          };
        }

        if (response.success) {
          // Get session details before marking the store authenticated so a
          // follow-up failure cannot leave user/isAuthenticated inconsistent.
          const sessionResponse = await auth.getCurrentUser();
          user.set(sessionResponse.user || response.user);
          session.set(sessionResponse.session);
          isAuthenticated.set(true);
          loading.set(false);
          error.set(null);

          // Return enrollment status if required
          return {
            success: true,
            enrollment_required: response.enrollment_required || false,
            policy_message: response.policy_message,
          };
        } else {
          clearStores(user, session);
          isAuthenticated.set(false);
          loading.set(false);
          error.set(response.message || 'Login failed');
          return { success: false, message: response.message || 'Login failed' };
        }
      } catch (err) {
        // Check if error response contains policy info
        if (hasPolicyFlag(err, 'passkey_required')) {
          clearStores(user, session);
          isAuthenticated.set(false);
          loading.set(false);
          error.set(null);
          return {
            success: false,
            passkey_required: true,
            policy_message: policyValue(err, 'policy_message'),
          };
        }

        if (hasPolicyFlag(err, 'sso_required')) {
          clearStores(user, session);
          isAuthenticated.set(false);
          loading.set(false);
          const policyMessage = policyValue(err, 'policy_message');
          error.set(policyMessage || 'SSO login required');
          return {
            success: false,
            sso_required: true,
            policy_message: policyMessage || 'Password login is disabled. Please use SSO.',
          };
        }

        clearStores(user, session);
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
        await auth.logout();
      } catch (err) {
        console.warn('Logout API call failed:', err);
      }

      resetIdentity();
      isAuthenticated.set(false);
      loading.set(false);
    },

    // Logout from all sessions
    async logoutAll() {
      loading.set(true);

      try {
        await auth.logoutAll();
      } catch (err) {
        console.warn('Logout all API call failed:', err);
      }

      resetIdentity();
      isAuthenticated.set(false);
      loading.set(false);
    },

    // Refresh session
    async refreshSession(rememberMe = false) {
      try {
        await auth.refreshSession({ remember_me: rememberMe });

        // Update session info
        const response = await auth.getCurrentUser();
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
        const response = await auth.changePassword(passwordData);
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
      resetIdentity();
      isAuthenticated.set(false);
      loading.set(false);
      error.set('Session expired. Please log in again.');
    },

    // Confirm that a WebAuthn completion created/elevated the browser session,
    // then populate canonical user and session metadata from /auth/me.
    async completePasskeyLogin(fallbackUser = null) {
      loading.set(true);
      error.set(null);
      try {
        const response = await auth.getCurrentUser();
        user.set(response.user || fallbackUser);
        session.set(response.session);
        isAuthenticated.set(true);
        loading.set(false);
        return true;
      } catch (err) {
        clearStores(user, session);
        isAuthenticated.set(false);
        loading.set(false);
        error.set(err.message || 'Failed to complete passkey authentication');
        throw err;
      }
    },

    // Set authentication data directly for callers that already hold canonical
    // session metadata.
    setAuthData(userData, sessionData) {
      user.set(userData);
      session.set(sessionData);
      isAuthenticated.set(true);
      loading.set(false);
      error.set(null);
    },

    patchCurrentUser(updates) {
      user.update((current) => (current ? { ...current, ...updates } : current));
    },

    // Clear error
    clearError() {
      error.set(null);
    },
  };
}

export const authStore = createAuthStore();
