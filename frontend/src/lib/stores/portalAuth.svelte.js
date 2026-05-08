import { derived, writable } from 'svelte/store';
import { api } from '../api.js';
import {
  isWebAuthnSupported,
  prepareCredentialRequestOptions,
  processCredentialRequestResponse,
} from '../utils/webauthn-utils.js';
import { clearStores, getStoreValue } from './storeUtils.js';

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
  // Whether the post-login passkey banner should be visible. Driven by the
  // /auth/me response (passkey_count + dismissed_passkey_prompt_at) and the
  // browser's WebAuthn capability check. Recomputed on each checkAuth.
  const showPasskeyBanner = writable(false);

  // Create a combined derived store for easy subscription
  const combined = derived(
    [customer, user, isAuthenticated, isInternal, loading, error, emailSent, showPasskeyBanner],
    ([
      $customer,
      $user,
      $isAuthenticated,
      $isInternal,
      $loading,
      $error,
      $emailSent,
      $showPasskeyBanner,
    ]) => ({
      customer: $customer,
      user: $user,
      isAuthenticated: $isAuthenticated,
      isInternal: $isInternal,
      loading: $loading,
      error: $error,
      emailSent: $emailSent,
      showPasskeyBanner: $showPasskeyBanner,
    })
  );

  function recomputePasskeyBanner(c) {
    if (!c || c.passkey_count > 0 || c.dismissed_passkey_prompt_at) {
      showPasskeyBanner.set(false);
      return;
    }
    showPasskeyBanner.set(isWebAuthnSupported());
  }

  return {
    // Subscribe to combined state
    subscribe: combined.subscribe,

    // Convenience getters for backwards compatibility with direct property access
    get customer() {
      return getStoreValue(customer);
    },

    get user() {
      return getStoreValue(user);
    },

    get isAuthenticated() {
      return getStoreValue(isAuthenticated);
    },

    get isInternal() {
      return getStoreValue(isInternal);
    },

    get loading() {
      return getStoreValue(loading);
    },

    get error() {
      return getStoreValue(error);
    },

    get emailSent() {
      return getStoreValue(emailSent);
    },

    get showPasskeyBanner() {
      return getStoreValue(showPasskeyBanner);
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
            showPasskeyBanner.set(false);
          } else {
            // Portal customer authenticated
            customer.set(response.customer);
            user.set(null);
            isInternal.set(false);
            recomputePasskeyBanner(response.customer);
          }
          isAuthenticated.set(true);
        } else {
          customer.set(null);
          user.set(null);
          isAuthenticated.set(false);
          isInternal.set(false);
          showPasskeyBanner.set(false);
        }
      } catch (_err) {
        // Not authenticated is not an error
        customer.set(null);
        user.set(null);
        isAuthenticated.set(false);
        isInternal.set(false);
        showPasskeyBanner.set(false);
      } finally {
        loading.set(false);
      }
    },

    /**
     * Sign in via a discoverable passkey (no email required). Resolves to
     * { success, message? } and updates the store on success.
     */
    async loginWithPasskey(slug) {
      if (!isWebAuthnSupported()) {
        const msg = 'Passkeys are not supported in this browser.';
        error.set(msg);
        return { success: false, message: msg };
      }
      loading.set(true);
      error.set(null);
      try {
        const startResponse = await api.portalPasskey.startLogin(slug);
        const requestOptions = prepareCredentialRequestOptions(startResponse);
        const credential = await navigator.credentials.get(requestOptions);
        if (!credential) {
          throw new Error('No credential returned from authenticator');
        }
        const processed = processCredentialRequestResponse(/** @type {any} */ (credential));
        const completeResponse = await api.portalPasskey.completeLogin(slug, {
          sessionId: startResponse.sessionId,
          response: processed,
        });
        if (completeResponse.success) {
          // Server set the session cookie; refresh state so UI reflects it.
          await this.checkAuth(slug);
          return { success: true };
        }
        const msg = completeResponse.message || 'Passkey sign-in failed';
        error.set(msg);
        return { success: false, message: msg };
      } catch (err) {
        // NotAllowedError = user cancelled the prompt; surface a friendlier copy.
        const msg =
          err?.name === 'NotAllowedError'
            ? 'Passkey sign-in was cancelled.'
            : err?.message || 'Passkey sign-in failed';
        error.set(msg);
        return { success: false, message: msg };
      } finally {
        loading.set(false);
      }
    },

    /**
     * Mark the post-login passkey banner as dismissed for this customer so
     * it doesn't reappear on subsequent sessions.
     */
    async dismissPasskeyPrompt(slug) {
      try {
        await api.portalPasskey.dismissPrompt(slug);
      } catch (err) {
        console.warn('Failed to persist passkey prompt dismissal:', err);
      }
      showPasskeyBanner.set(false);
      const c = getStoreValue(customer);
      if (c) {
        customer.set({ ...c, dismissed_passkey_prompt_at: new Date().toISOString() });
      }
    },

    /**
     * Refresh the local passkey count / banner state after a registration or
     * removal so the UI updates without a full /auth/me round-trip.
     */
    setPasskeyCount(count) {
      const c = getStoreValue(customer);
      if (!c) return;
      const updated = { ...c, passkey_count: count };
      customer.set(updated);
      recomputePasskeyBanner(updated);
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
          return {
            success: false,
            message: response.message,
            code: response.code,
            email: response.email,
          };
        }
      } catch (err) {
        // fetchAPI throws on 4xx/5xx but attaches the parsed body when it can;
        // pull through code/email if available so the recovery flow still
        // fires for expired/used tokens.
        error.set(err.message || 'Invalid or expired link');
        const body = err?.body || {};
        return {
          success: false,
          message: body.message || err.message,
          code: body.code,
          email: body.email,
        };
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
      clearStores(customer, user, error);
      isAuthenticated.set(false);
      isInternal.set(false);
      loading.set(false);
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
      clearStores(customer, user, error);
      isAuthenticated.set(false);
      isInternal.set(false);
      loading.set(false);
      emailSent.set(false);
    },
  };
}

export const portalAuthStore = createPortalAuthStore();
