import { derived, writable } from 'svelte/store';
import { api } from '../api.js';

/**
 * SSO Store - manages Single Sign-On status and configuration
 *
 * This store handles:
 * - Public SSO status (for login page)
 * - Admin SSO provider management
 * - User external account management
 */
function createSSOStore() {
  // Public SSO status (fetched without auth)
  const status = writable({
    enabled: false,
    providerName: null,
    providerSlug: null,
    allowPasswordLogin: true,
  });
  const statusLoading = writable(true);
  const statusError = writable(null);

  // Admin: SSO providers list
  const providers = writable([]);
  const providersLoading = writable(false);
  const providersError = writable(null);

  // User: External accounts
  const externalAccounts = writable([]);
  const externalAccountsLoading = writable(false);

  // Cache for email verification status (to avoid repeated API calls)
  let verificationStatusCache = null;

  // Combined derived store for easy subscription
  const combined = derived(
    [
      status,
      statusLoading,
      statusError,
      providers,
      providersLoading,
      providersError,
      externalAccounts,
      externalAccountsLoading,
    ],
    ([
      $status,
      $statusLoading,
      $statusError,
      $providers,
      $providersLoading,
      $providersError,
      $externalAccounts,
      $externalAccountsLoading,
    ]) => ({
      // Public status
      enabled: $status.enabled,
      providerName: $status.providerName,
      providerSlug: $status.providerSlug,
      allowPasswordLogin: $status.allowPasswordLogin,
      statusLoading: $statusLoading,
      statusError: $statusError,
      // Admin providers
      providers: $providers,
      providersLoading: $providersLoading,
      providersError: $providersError,
      // User external accounts
      externalAccounts: $externalAccounts,
      externalAccountsLoading: $externalAccountsLoading,
    })
  );

  return {
    subscribe: combined.subscribe,

    // Initialize SSO status (called on app load, no auth required)
    async initStatus() {
      statusLoading.set(true);
      statusError.set(null);

      try {
        const data = await api.sso.getStatus();
        status.set({
          enabled: data.enabled,
          providerName: data.provider_name || null,
          providerSlug: data.provider_slug || null,
          allowPasswordLogin: data.allow_password_login !== false,
        });
        statusLoading.set(false);
      } catch (err) {
        console.warn('Failed to get SSO status:', err);
        status.set({
          enabled: false,
          providerName: null,
          providerSlug: null,
          allowPasswordLogin: true,
        });
        statusLoading.set(false);
        statusError.set(err.message);
      }
    },

    // Start SSO login (redirects to IdP)
    startLogin(rememberMe = false) {
      let currentStatus;
      status.subscribe((s) => (currentStatus = s))();

      if (!currentStatus.enabled || !currentStatus.providerSlug) {
        console.error('SSO not enabled or provider not configured');
        return;
      }

      // Redirect to SSO login endpoint
      const url = api.sso.startLogin(currentStatus.providerSlug, rememberMe);
      window.location.href = url;
    },

    // Admin: Load providers list
    async loadProviders() {
      providersLoading.set(true);
      providersError.set(null);

      try {
        const data = await api.sso.listProviders();
        providers.set(data || []);
        providersLoading.set(false);
      } catch (err) {
        console.error('Failed to load SSO providers:', err);
        providersError.set(err.message);
        providersLoading.set(false);
      }
    },

    // Admin: Get a single provider
    async getProvider(id) {
      try {
        return await api.sso.getProvider(id);
      } catch (err) {
        console.error('Failed to get SSO provider:', err);
        throw err;
      }
    },

    // Admin: Create a new provider
    async createProvider(data) {
      try {
        const provider = await api.sso.createProvider(data);
        // Refresh providers list
        await this.loadProviders();
        // Refresh status
        await this.initStatus();
        return provider;
      } catch (err) {
        console.error('Failed to create SSO provider:', err);
        throw err;
      }
    },

    // Admin: Update a provider
    async updateProvider(id, data) {
      try {
        const provider = await api.sso.updateProvider(id, data);
        // Refresh providers list
        await this.loadProviders();
        // Refresh status
        await this.initStatus();
        return provider;
      } catch (err) {
        console.error('Failed to update SSO provider:', err);
        throw err;
      }
    },

    // Admin: Delete a provider
    async deleteProvider(id) {
      try {
        await api.sso.deleteProvider(id);
        // Refresh providers list
        await this.loadProviders();
        // Refresh status
        await this.initStatus();
      } catch (err) {
        console.error('Failed to delete SSO provider:', err);
        throw err;
      }
    },

    // Admin: Test provider connection
    async testProvider(id) {
      try {
        return await api.sso.testProvider(id);
      } catch (err) {
        console.error('Failed to test SSO provider:', err);
        throw err;
      }
    },

    // User: Load external accounts
    async loadExternalAccounts() {
      externalAccountsLoading.set(true);

      try {
        const data = await api.sso.getExternalAccounts();
        externalAccounts.set(data || []);
        externalAccountsLoading.set(false);
      } catch (err) {
        console.error('Failed to load external accounts:', err);
        externalAccountsLoading.set(false);
      }
    },

    // User: Unlink an external account
    async unlinkExternalAccount(id) {
      try {
        await api.sso.unlinkExternalAccount(id);
        // Refresh external accounts
        await this.loadExternalAccounts();
      } catch (err) {
        console.error('Failed to unlink external account:', err);
        throw err;
      }
    },

    // Check for SSO error in URL (after callback redirect)
    checkForError() {
      const urlParams = new URLSearchParams(window.location.search);
      const ssoError = urlParams.get('sso_error');
      if (ssoError) {
        // Clear the error from URL
        const url = new URL(window.location.href);
        url.searchParams.delete('sso_error');
        window.history.replaceState({}, '', url.toString());
        return ssoError;
      }
      return null;
    },

    // Check for email verification pending state in URL (after SSO callback)
    checkForEmailVerificationPending() {
      const urlParams = new URLSearchParams(window.location.search);
      const verifyEmail = urlParams.get('verify_email');
      if (verifyEmail === 'pending') {
        // Clear the param from URL
        const url = new URL(window.location.href);
        url.searchParams.delete('verify_email');
        window.history.replaceState({}, '', url.toString());
        return true;
      }
      return false;
    },

    // Get email verification status for current user (with caching)
    async getVerificationStatus() {
      // Return cached result if available
      if (verificationStatusCache !== null) {
        return verificationStatusCache;
      }

      // Check if SSO is enabled first - no need to check verification if SSO isn't configured
      let currentStatus;
      status.subscribe((s) => (currentStatus = s))();

      if (!currentStatus.enabled) {
        // No SSO configured - skip the API call entirely
        verificationStatusCache = { email_verified: true, configured: false };
        return verificationStatusCache;
      }

      try {
        const result = await api.auth.getVerificationStatus();
        verificationStatusCache = result;
        return result;
      } catch (err) {
        console.error('Failed to get verification status:', err);
        // Return verified=true on error for backwards compatibility
        verificationStatusCache = { email_verified: true, configured: false };
        return verificationStatusCache;
      }
    },

    // Clear verification status cache (call when user verifies email)
    clearVerificationCache() {
      verificationStatusCache = null;
    },

    // Resend verification email
    async resendVerificationEmail() {
      try {
        return await api.auth.resendVerification();
      } catch (err) {
        console.error('Failed to resend verification email:', err);
        throw err;
      }
    },
  };
}

export const ssoStore = createSSOStore();
