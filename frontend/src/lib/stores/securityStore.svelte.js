/**
 * Store for managing Security page state.
 * Uses Svelte 5 class-based reactive state pattern.
 * Centralizes credentials, tokens, SSH keys, and password management.
 */
import { api } from '../api.js';
import { capabilitiesStore } from './capabilities.svelte.js';
import { scopeCatalogStore } from './scopeCatalog.svelte.js';

class SecurityStore {
  // === User ===
  user = $state(null);
  loading = $state(false);
  initialized = $state(false);
  currentUserId = $state(null);

  // === Credentials ===
  credentials = $state([]);
  credentialsLoading = $state(false);

  // === API Tokens ===
  apiTokens = $state([]);
  tokensLoading = $state(false);

  // === Features ===
  sshAvailable = $state(false);

  // === Enrollment Banner ===
  showEnrollmentBanner = $state(false);
  enrollmentType = $state('');
  enrollmentOnly = $state(false);

  // === Modals ===
  showAddCredential = $state(false);
  showAddToken = $state(false);
  showNewToken = $state(false);
  showChangePassword = $state(false);
  // === Credential Form ===
  credentialType = $state('fido'); // 'fido' or 'ssh'
  newCredentialName = $state('');
  newSSHPublicKey = $state('');
  enrollingFIDO = $state(false);
  testingLogin = $state(false);
  loginTestResult = $state('');

  // === Token Form ===
  // The grantable scopes, loaded from the server catalog (auth.ScopeCatalog).
  get scopeCatalog() {
    return scopeCatalogStore.catalog;
  }
  newTokenName = $state('');
  // A conservative starting selection; the "Agent default" preset in the picker
  // applies the same set an MCP or `ws` CLI token gets when minted without one.
  newTokenScopes = $state(['items:read', 'workspaces:read', 'pages:read', 'users:read']);
  newTokenExpiry = $state('');
  newTokenValue = $state('');
  creatingToken = $state(false);

  // === Change Password ===
  changePasswordData = $state({
    current_password: '',
    new_password: '',
    confirm_password: '',
    logout_all: false,
  });
  changePasswordLoading = $state(false);
  changePasswordError = $state('');
  changePasswordSuccess = $state(false);

  // === Initialization ===

  /**
   * Set the current user ID and trigger initialization.
   */
  setCurrentUserId(userId, { enrollmentOnly = false } = {}) {
    if (userId && this.currentUserId != null && String(this.currentUserId) !== String(userId)) {
      this.reset();
    }
    this.enrollmentOnly = enrollmentOnly;
    if (enrollmentOnly) {
      this.showEnrollmentBanner = true;
      this.enrollmentType = 'passkey';
      this.credentialType = 'fido';
      this.showAddCredential = true;
    }
    if (userId && !this.initialized) {
      this.currentUserId = userId;
      this.initialized = true;
      this.loadFeatures();
      // A restricted enrollment session may call only /auth/me and WebAuthn
      // registration endpoints. Avoid unrelated profile/token requests until
      // registration elevates the session.
      if (!enrollmentOnly) {
        this.loadUserProfile();
        this.loadCredentials();
        this.loadApiTokens();
        this.loadScopeCatalog();
      }
    }
  }

  /**
   * Check for enrollment query parameter and show banner.
   */
  checkEnrollmentRequired(enrollType) {
    if (enrollType === 'passkey') {
      this.showEnrollmentBanner = true;
      this.enrollmentType = 'passkey';
      this.enrollmentOnly = true;
      this.credentialType = 'fido';
      this.showAddCredential = true;
    }
  }

  /**
   * Dismiss enrollment banner.
   */
  dismissEnrollmentBanner() {
    if (this.enrollmentOnly) return;
    this.showEnrollmentBanner = false;
    this.enrollmentType = '';
  }

  // === Data Loading ===

  async loadFeatures() {
    await capabilitiesStore.load();
    this.sshAvailable = capabilitiesStore.sshAvailable;
  }

  async loadUserProfile() {
    if (!this.currentUserId) return;
    try {
      this.loading = true;
      this.user = await api.getUser(this.currentUserId);
    } catch (err) {
      console.warn('Failed to load user profile:', err);
      this.user = null;
    } finally {
      this.loading = false;
    }
  }

  async loadCredentials() {
    if (!this.currentUserId) return;
    try {
      this.credentialsLoading = true;
      this.credentials = (await api.getUserCredentials(this.currentUserId)) || [];
    } catch (err) {
      console.warn('Failed to load credentials:', err);
      this.credentials = [];
    } finally {
      this.credentialsLoading = false;
    }
  }

  async loadApiTokens() {
    try {
      this.tokensLoading = true;
      this.apiTokens = (await api.getApiTokens()) || [];
    } catch (err) {
      console.warn('Failed to load API tokens:', err);
      this.apiTokens = [];
    } finally {
      this.tokensLoading = false;
    }
  }

  /**
   * Load the grantable scope catalog the token picker renders. Served from
   * auth.ScopeCatalog so the picker always offers exactly what the server
   * accepts, instead of a copy in the frontend that drifts as scopes are added.
   */
  async loadScopeCatalog() {
    return scopeCatalogStore.load();
  }

  // === Credential Actions ===

  /**
   * Start FIDO2 registration process.
   * Returns WebAuthn options for browser to create credential.
   */
  async startFIDORegistration(prepareOptions, processResponse) {
    if (!this.currentUserId || !this.newCredentialName.trim()) return;

    try {
      this.enrollingFIDO = true;

      // Start registration with server
      const registrationData = await api.startFIDORegistration(
        this.currentUserId,
        this.newCredentialName.trim()
      );

      // Extract session ID and options
      const sessionId = registrationData.sessionId;
      const publicKeyOptions =
        registrationData.publicKey || registrationData.options || registrationData;

      if (!publicKeyOptions?.challenge) {
        throw new Error('Invalid registration response from server');
      }

      // Prepare options for browser API (callback from component)
      const credentialCreationOptions = prepareOptions(publicKeyOptions);

      // Create credential using browser API
      const credential = await navigator.credentials.create(credentialCreationOptions);

      // Process credential for server (callback from component)
      const credentialResponse = processResponse(credential);

      // Complete registration with server
      const completionData = {
        sessionId: sessionId,
        credentialName: this.newCredentialName.trim(),
        response: credentialResponse,
      };

      await api.completeFIDORegistration(this.currentUserId, completionData);

      const wasEnrollmentRequired = this.enrollmentOnly;
      if (wasEnrollmentRequired) {
        this.enrollmentOnly = false;
        this.showEnrollmentBanner = false;
        this.enrollmentType = '';
      }
      this.resetCredentialForm();

      if (wasEnrollmentRequired) {
        await Promise.allSettled([
          this.loadUserProfile(),
          this.loadCredentials(),
          this.loadApiTokens(),
        ]);
      } else {
        await this.loadCredentials();
      }
      return { success: true, wasEnrollmentRequired };
    } catch (err) {
      console.error('FIDO registration error:', err);
      throw err;
    } finally {
      this.enrollingFIDO = false;
    }
  }

  async createSSHKey() {
    if (!this.currentUserId || !this.newCredentialName.trim() || !this.newSSHPublicKey.trim())
      return;

    try {
      this.loading = true;
      await api.createSSHKey(
        this.currentUserId,
        this.newCredentialName.trim(),
        this.newSSHPublicKey.trim()
      );
      await this.loadCredentials();
      this.resetCredentialForm();
    } catch (err) {
      console.error('Failed to add SSH key:', err);
      throw err;
    } finally {
      this.loading = false;
    }
  }

  async removeCredential(credentialId) {
    if (!this.currentUserId) return;
    try {
      await api.removeUserCredential(this.currentUserId, credentialId);
      await this.loadCredentials();
    } catch (err) {
      console.error('Failed to remove credential:', err);
      throw err;
    }
  }

  // === Token Actions ===

  async createApiToken() {
    if (!this.newTokenName.trim()) return;

    try {
      this.creatingToken = true;
      const tokenData = {
        name: this.newTokenName.trim(),
        permissions: this.newTokenScopes,
        expires_on: this.newTokenExpiry || null,
      };

      const result = await api.createApiToken(tokenData);
      this.newTokenValue = result.token;
      this.showNewToken = true;

      await this.loadApiTokens();
      this.resetTokenForm();
    } catch (err) {
      console.error('Failed to create token:', err);
      throw err;
    } finally {
      this.creatingToken = false;
    }
  }

  async revokeApiToken(tokenId) {
    try {
      await api.revokeApiToken(tokenId);
      await this.loadApiTokens();
    } catch (err) {
      console.error('Failed to revoke token:', err);
      throw err;
    }
  }

  // === Password Change ===

  async changePassword() {
    this.changePasswordError = '';

    // Validate passwords match
    if (this.changePasswordData.new_password !== this.changePasswordData.confirm_password) {
      this.changePasswordError = 'New passwords do not match';
      return { success: false, error: this.changePasswordError };
    }

    // Validate minimum length
    if (this.changePasswordData.new_password.length < 8) {
      this.changePasswordError = 'Password must be at least 8 characters';
      return { success: false, error: this.changePasswordError };
    }

    this.changePasswordLoading = true;
    try {
      await api.auth.changePassword({
        current_password: this.changePasswordData.current_password,
        new_password: this.changePasswordData.new_password,
        logout_all: this.changePasswordData.logout_all,
      });
      this.changePasswordSuccess = true;

      // Reset form after brief delay
      setTimeout(() => {
        this.closeChangePasswordModal();
      }, 2000);

      return { success: true };
    } catch (err) {
      this.changePasswordError = err.message || 'Failed to change password';
      return { success: false, error: this.changePasswordError };
    } finally {
      this.changePasswordLoading = false;
    }
  }

  // === Modal Controls ===

  openAddCredentialModal() {
    this.showAddCredential = true;
  }

  openAddTokenModal() {
    this.showAddToken = true;
  }

  openChangePasswordModal() {
    this.showChangePassword = true;
  }

  closeChangePasswordModal() {
    this.showChangePassword = false;
    this.changePasswordError = '';
    this.changePasswordSuccess = false;
    this.changePasswordData = {
      current_password: '',
      new_password: '',
      confirm_password: '',
      logout_all: false,
    };
  }

  closeNewTokenDisplay() {
    this.showNewToken = false;
    this.newTokenValue = '';
  }

  // === Form Resets ===

  resetCredentialForm() {
    if (this.enrollmentOnly) {
      this.credentialType = 'fido';
      this.showAddCredential = true;
      return;
    }
    this.newCredentialName = '';
    this.newSSHPublicKey = '';
    this.credentialType = 'fido';
    this.showAddCredential = false;
  }

  resetTokenForm() {
    this.newTokenName = '';
    this.newTokenScopes = ['items:read', 'workspaces:read', 'pages:read', 'users:read'];
    this.newTokenExpiry = '';
    this.showAddToken = false;
  }

  // === Full Reset ===

  reset() {
    this.user = null;
    this.loading = false;
    this.initialized = false;
    this.currentUserId = null;
    this.credentials = [];
    this.credentialsLoading = false;
    this.apiTokens = [];
    this.tokensLoading = false;
    this.sshAvailable = false;
    this.showEnrollmentBanner = false;
    this.enrollmentType = '';
    this.enrollmentOnly = false;
    this.showAddCredential = false;
    this.showAddToken = false;
    this.showNewToken = false;
    this.showChangePassword = false;
    this.credentialType = 'fido';
    this.newCredentialName = '';
    this.newSSHPublicKey = '';
    this.enrollingFIDO = false;
    this.testingLogin = false;
    this.loginTestResult = '';
    this.newTokenName = '';
    this.newTokenScopes = ['items:read', 'workspaces:read', 'pages:read', 'users:read'];
    this.newTokenExpiry = '';
    this.newTokenValue = '';
    this.creatingToken = false;
    this.changePasswordData = {
      current_password: '',
      new_password: '',
      confirm_password: '',
      logout_all: false,
    };
    this.changePasswordLoading = false;
    this.changePasswordError = '';
    this.changePasswordSuccess = false;
  }
}

export const securityStore = new SecurityStore();
