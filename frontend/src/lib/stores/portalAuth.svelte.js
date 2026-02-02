import { writable, derived } from 'svelte/store';
import { api } from '../api.js';

/**
 * Portal Auth Store - Svelte Store Implementation
 * Manages portal customer authentication state using magic link authentication
 * Also handles internal user sessions when viewing the portal
 *
 * Converted from Svelte 5 runes to proper Svelte stores for reactivity
 */

function createPortalAuthStore() {
  const customer = writable(null);
  const user = writable(null); // internal user
  const isAuthenticated = writable(false);
  const isInternal = writable(false); // true if authenticated via internal session
  const loading = writable(false);
  const error = writable(null);
  const emailSent = writable(false);

  // Create a combined derived store for easy subscription
  const combined = derived(
    [customer, user, isAuthenticated, isInternal, loading, error, emailSent],
    ([$customer, $user, $isAuthenticated, $isInternal, $loading, $error, $emailSent]) => ({
      customer: $customer,
      user: $user,
      isAuthenticated: $isAuthenticated,
      isInternal: $isInternal,
      loading: $loading,
      error: $error,
      emailSent: $emailSent
    })
  );

  return {
    // Subscribe to combined state
    subscribe: combined.subscribe,

    // Convenience getters for backwards compatibility with direct property access
    get customer() {
      let value;
      customer.subscribe(v => value = v)();
      return value;
    },

    get user() {
      let value;
      user.subscribe(v => value = v)();
      return value;
    },

    get isAuthenticated() {
      let value;
      isAuthenticated.subscribe(v => value = v)();
      return value;
    },

    get isInternal() {
      let value;
      isInternal.subscribe(v => value = v)();
      return value;
    },

    get loading() {
      let value;
      loading.subscribe(v => value = v)();
      return value;
    },

    get error() {
      let value;
      error.subscribe(v => value = v)();
      return value;
    },

    get emailSent() {
      let value;
      emailSent.subscribe(v => value = v)();
      return value;
    },

    /**
     * Check current authentication status for a portal
     * @param {string} slug - Portal slug
     */
    async checkAuth(slug) {
      loading.set(true);
      error.set(null);

      try {
        const response = await api.portalAuth.getCurrentCustomer(slug);
        if (response.authenticated) {
          if (response.is_internal) {
            // Internal user authenticated
            user.set(response.user);
            customer.set(null);
            isInternal.set(true);
          } else {
            // Portal customer authenticated
            customer.set(response.customer);
            user.set(null);
            isInternal.set(false);
          }
          isAuthenticated.set(true);
        } else {
          customer.set(null);
          user.set(null);
          isAuthenticated.set(false);
          isInternal.set(false);
        }
      } catch (err) {
        // Not authenticated is not an error
        customer.set(null);
        user.set(null);
        isAuthenticated.set(false);
        isInternal.set(false);
      } finally {
        loading.set(false);
      }
    },

    /**
     * Request a magic link email
     * @param {string} slug - Portal slug
     * @param {string} email - Customer email
     */
    async requestMagicLink(slug, email) {
      loading.set(true);
      error.set(null);
      emailSent.set(false);

      try {
        await api.portalAuth.requestMagicLink(slug, email);
        // Always show success (prevents email enumeration)
        emailSent.set(true);
        return { success: true };
      } catch (err) {
        error.set(err.message || 'Failed to send magic link');
        return { success: false, message: err.message };
      } finally {
        loading.set(false);
      }
    },

    /**
     * Verify a magic link token
     * @param {string} slug - Portal slug
     * @param {string} token - Magic link token
     */
    async verifyMagicLink(slug, token) {
      loading.set(true);
      error.set(null);

      try {
        const response = await api.portalAuth.verifyMagicLink(slug, token);
        if (response.success) {
          customer.set(response.customer);
          isAuthenticated.set(true);
          return { success: true, customer: response.customer };
        } else {
          error.set(response.message || 'Invalid or expired link');
          return { success: false, message: response.message };
        }
      } catch (err) {
        error.set(err.message || 'Invalid or expired link');
        return { success: false, message: err.message };
      } finally {
        loading.set(false);
      }
    },

    /**
     * Logout the current portal customer
     * @param {string} slug - Portal slug
     */
    async logout(slug) {
      loading.set(true);

      try {
        await api.portalAuth.logout(slug);
      } catch (err) {
        console.warn('Logout API call failed:', err);
      }

      // Clear auth state regardless of API call result
      customer.set(null);
      user.set(null);
      isAuthenticated.set(false);
      isInternal.set(false);
      loading.set(false);
      error.set(null);
      emailSent.set(false);
    },

    /**
     * Clear the error state
     */
    clearError() {
      error.set(null);
    },

    /**
     * Reset the email sent state
     */
    resetEmailSent() {
      emailSent.set(false);
    },

    /**
     * Clear all state (used when navigating away from portal)
     */
    reset() {
      customer.set(null);
      user.set(null);
      isAuthenticated.set(false);
      isInternal.set(false);
      loading.set(false);
      error.set(null);
      emailSent.set(false);
    }
  };
}

export const portalAuthStore = createPortalAuthStore();
