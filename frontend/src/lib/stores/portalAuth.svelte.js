import { api } from '../api.js';

/**
 * Portal Auth Store - Svelte 5 Runes Implementation
 * Manages portal customer authentication state using magic link authentication
 * Also handles internal user sessions when viewing the portal
 */

// Svelte 5 reactive state
let customer = $state(null);
let user = $state(null); // internal user
let isAuthenticated = $state(false);
let isInternal = $state(false); // true if authenticated via internal session
let loading = $state(false);
let error = $state(null);
let emailSent = $state(false);

export const portalAuthStore = {
  // Direct property access (reactive in Svelte 5)
  get customer() { return customer; },
  get user() { return user; },
  get isAuthenticated() { return isAuthenticated; },
  get isInternal() { return isInternal; },
  get loading() { return loading; },
  get error() { return error; },
  get emailSent() { return emailSent; },

  /**
   * Check current authentication status for a portal
   * @param {string} slug - Portal slug
   */
  async checkAuth(slug) {
    loading = true;
    error = null;

    try {
      const response = await api.portalAuth.getCurrentCustomer(slug);
      if (response.authenticated) {
        if (response.is_internal) {
          // Internal user authenticated
          user = response.user;
          customer = null;
          isInternal = true;
        } else {
          // Portal customer authenticated
          customer = response.customer;
          user = null;
          isInternal = false;
        }
        isAuthenticated = true;
      } else {
        customer = null;
        user = null;
        isAuthenticated = false;
        isInternal = false;
      }
    } catch (err) {
      // Not authenticated is not an error
      customer = null;
      user = null;
      isAuthenticated = false;
      isInternal = false;
    } finally {
      loading = false;
    }
  },

  /**
   * Request a magic link email
   * @param {string} slug - Portal slug
   * @param {string} email - Customer email
   */
  async requestMagicLink(slug, email) {
    loading = true;
    error = null;
    emailSent = false;

    try {
      await api.portalAuth.requestMagicLink(slug, email);
      // Always show success (prevents email enumeration)
      emailSent = true;
      return { success: true };
    } catch (err) {
      error = err.message || 'Failed to send magic link';
      return { success: false, message: err.message };
    } finally {
      loading = false;
    }
  },

  /**
   * Verify a magic link token
   * @param {string} slug - Portal slug
   * @param {string} token - Magic link token
   */
  async verifyMagicLink(slug, token) {
    loading = true;
    error = null;

    try {
      const response = await api.portalAuth.verifyMagicLink(slug, token);
      if (response.success) {
        customer = response.customer;
        isAuthenticated = true;
        return { success: true, customer: response.customer };
      } else {
        error = response.message || 'Invalid or expired link';
        return { success: false, message: response.message };
      }
    } catch (err) {
      error = err.message || 'Invalid or expired link';
      return { success: false, message: err.message };
    } finally {
      loading = false;
    }
  },

  /**
   * Logout the current portal customer
   * @param {string} slug - Portal slug
   */
  async logout(slug) {
    loading = true;

    try {
      await api.portalAuth.logout(slug);
    } catch (err) {
      console.warn('Logout API call failed:', err);
    }

    // Clear auth state regardless of API call result
    customer = null;
    user = null;
    isAuthenticated = false;
    isInternal = false;
    loading = false;
    error = null;
    emailSent = false;
  },

  /**
   * Clear the error state
   */
  clearError() {
    error = null;
  },

  /**
   * Reset the email sent state
   */
  resetEmailSent() {
    emailSent = false;
  },

  /**
   * Clear all state (used when navigating away from portal)
   */
  reset() {
    customer = null;
    user = null;
    isAuthenticated = false;
    isInternal = false;
    loading = false;
    error = null;
    emailSent = false;
  }
};
