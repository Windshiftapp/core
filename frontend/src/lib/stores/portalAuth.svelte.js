import { derived, writable } from 'svelte/store';
import { portal, portalAuth, portalPasskey } from '../api/portal.js';
import { isWebAuthnSupported } from '../utils/webauthn-utils.js';
import { clearStores, getStoreValue } from './storeUtils.js';

/**
 * Portal Auth Store - Svelte Store Implementation
 * Manages portal customer authentication state using magic link authentication
 * Also handles internal user sessions when viewing the portal
 *
 * Converted from Svelte 5 runes to proper Svelte stores for reactivity
 */

function createPortalAuthStore() {
  // userBootstrap is a store so consumers can react to request_sharing changes
  // (WI-1139) as well as read it through the combined store value.
  const userBootstrap = writable(null);
  const customer = writable(null);
  const user = writable(null); // internal user
  let authCheckId = 0;
  const isAuthenticated = writable(false);
  const isInternal = writable(false); // true if authenticated via internal session
  // Whether the post-login passkey banner should be visible. Driven by the
  // /auth/me response (passkey_count + dismissed_passkey_prompt_at) and the
  // browser's WebAuthn capability check. Recomputed on each checkAuth.
  const showPasskeyBanner = writable(false);

  // Create a combined derived store for easy subscription
  const combined = derived(
    [customer, user, isAuthenticated, isInternal, showPasskeyBanner, userBootstrap],
    ([$customer, $user, $isAuthenticated, $isInternal, $showPasskeyBanner, $userBootstrap]) => ({
      customer: $customer,
      user: $user,
      isAuthenticated: $isAuthenticated,
      isInternal: $isInternal,
      showPasskeyBanner: $showPasskeyBanner,
      userBootstrap: $userBootstrap,
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

    get showPasskeyBanner() {
      return getStoreValue(showPasskeyBanner);
    },

    get userBootstrap() {
      return getStoreValue(userBootstrap);
    },

    /**
     * Check current authentication status for a portal
     * @param {string} slug - Portal slug
     */
    async checkAuth(slug) {
      const checkId = ++authCheckId;

      try {
        const response = await portal.getUserBootstrap(slug);
        if (checkId !== authCheckId) return null;
        userBootstrap.set(response);
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
        return response;
      } catch (_err) {
        if (checkId !== authCheckId) return null;
        // Not authenticated is not an error
        customer.set(null);
        user.set(null);
        isAuthenticated.set(false);
        isInternal.set(false);
        showPasskeyBanner.set(false);
        userBootstrap.set({ authenticated: false, my_requests: [], my_approvals: [] });
        return getStoreValue(userBootstrap);
      }
    },

    /**
     * Mark the post-login passkey banner as dismissed for this customer so
     * it doesn't reappear on subsequent sessions.
     */
    async dismissPasskeyPrompt(slug) {
      try {
        await portalPasskey.dismissPrompt(slug);
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
     * Merge fields into the cached customer without a /auth/me round-trip.
     * Used after the profile updates the customer's own language/timezone.
     */
    patchCustomer(patch) {
      const c = getStoreValue(customer);
      if (!c) return;
      customer.set({ ...c, ...patch });
    },

    /**
     * Verify a magic link token
     * @param {string} slug - Portal slug
     * @param {string} token - Magic link token
     */
    async verifyMagicLink(slug, token) {
      try {
        const response = await portalAuth.verifyMagicLink(slug, token);
        if (response.success) {
          // The caller dismisses the one-time-token UI before checkAuth updates
          // reactive auth state. Updating it here can remount the verifier and
          // redeem the same token a second time.
          return { success: true, customer: response.customer };
        }
        return {
          success: false,
          message: response.message,
          code: response.code,
          email: response.email,
        };
      } catch (err) {
        // fetchAPI throws on 4xx/5xx but attaches the parsed body when it can;
        // pull through code/email if available so the recovery flow still
        // fires for expired/used tokens.
        const body = err?.body || {};
        return {
          success: false,
          message: body.message || err.message,
          code: body.code,
          email: body.email,
        };
      }
    },

    /**
     * Logout the current portal customer
     * @param {string} slug - Portal slug
     */
    async logout(slug) {
      try {
        await portalAuth.logout(slug);
      } catch (err) {
        console.warn('Logout API call failed:', err);
      }

      // Clear auth state regardless of API call result
      clearStores(customer, user);
      isAuthenticated.set(false);
      isInternal.set(false);
      userBootstrap.set(null);
    },

    /**
     * Clear all state (used when navigating away from portal)
     */
    reset() {
      authCheckId++;
      clearStores(customer, user);
      isAuthenticated.set(false);
      isInternal.set(false);
      userBootstrap.set(null);
    },
  };
}

export const portalAuthStore = createPortalAuthStore();
