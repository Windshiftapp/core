/**
 * Store for managing Security page state.
 * Uses Svelte 5 class-based reactive state pattern.
 * Centralizes credentials, tokens, SSH keys, and password management.
 */
import { api } from '../api.js';

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

  // === Enrollment Banner ===
  showEnrollmentBanner = $state(false);
  enrollmentType = $state('');

  // === Modals ===
  showAddCredential = $state(false);
  showAddToken = $state(false);
  showNewToken = $state(false);
  showChangePassword = $state(false);
  showConfirmDialog = $state(false);

  // === Credential Form ===
  credentialType = $state('fido'); // 'fido' or 'ssh'
  newCredentialName = $state('');
  newSSHPublicKey = $state('');
  enrollingFIDO = $state(false);
  testingLogin = $state(false);
  loginTestResult = $state('');

  // === Token Form ===
  newTokenName = $state('');
  newTokenScopes = $state([]);
  newTokenExpiry = $state('');
  newTokenValue = $state('');
  creatingToken = $state(false);

  // === Confirm Dialog ===
  confirmDialogConfig = $state({
    title: '',
    message: '',
    action: null
  });

  // === Change Password ===
  changePasswordData = $state({
    current_password: '',
    new_password: '',
    confirm_password: '',
    logout_all: false
  });
  changePasswordLoading = $state(false);
  changePasswordError = $state('');
  changePasswordSuccess = $state(false);

  // === Initialization ===

  /**
   * Set the current user ID and trigger initialization.
   */
  setCurrentUserId(userId) {
    if (userId && !this.initialized) {
      this.currentUserId = userId;
      this.initialized = true;
      this.loadUserProfile();
      this.loadCredentials();
      this.loadApiTokens();
    }
  }

  /**
   * Check for enrollment query parameter and show banner.
   */
  checkEnrollmentRequired(enrollType) {
    if (enrollType === 'passkey') {
      this.showEnrollmentBanner = true;
      this.enrollmentType = 'passkey';
      // Auto-open the add credential modal
      setTimeout(() => {
        this.credentialType = 'fido';
        this.showAddCredential = true;
      }, 500);
    }
  }

  /**
   * Dismiss enrollment banner.
   */
  dismissEnrollmentBanner() {
    this.showEnrollmentBanner = false;
    this.enrollmentType = '';
  }

  // === Data Loading ===

  async loadUserProfile() {
    if (!this.currentUserId) return;
    try {
      this.loading = true;
      this.user = await api.getUser(this.currentUserId);
    } catch (err) {
      console.error('Failed to load user profile:', err);
      throw err;
    } finally {
      this.loading = false;
    }
  }

  async loadCredentials() {
    if (!this.currentUserId) return;
    try {
      this.credentialsLoading = true;
      this.credentials = await api.getUserCredentials(this.currentUserId) || [];
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
      this.apiTokens = await api.getApiTokens() || [];
    } catch (err) {
      console.warn('Failed to load API tokens:', err);
      this.apiTokens = [];
    } finally {
      this.tokensLoading = false;
    }
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
      const registrationData = await api.startFIDORegistration(this.currentUserId, this.newCredentialName.trim());

      // Extract session ID and options
      const sessionId = registrationData.sessionId;
      const publicKeyOptions = registrationData.publicKey || registrationData.options || registrationData;

      if (!publicKeyOptions || !publicKeyOptions.challenge) {
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
        response: credentialResponse
      };

      await api.completeFIDORegistration(this.currentUserId, completionData);
      await this.loadCredentials();

      const wasEnrollmentRequired = this.showEnrollmentBanner;
      if (wasEnrollmentRequired) {
        this.dismissEnrollmentBanner();
      }

      this.resetCredentialForm();
      return { success: true, wasEnrollmentRequired };
    } catch (err) {
      console.error('FIDO registration error:', err);
      throw err;
    } finally {
      this.enrollingFIDO = false;
    }
  }

  async createSSHKey() {
    if (!this.currentUserId || !this.newCredentialName.trim() || !this.newSSHPublicKey.trim()) return;

    try {
      this.loading = true;
      await api.createSSHKey(this.currentUserId, this.newCredentialName.trim(), this.newSSHPublicKey.trim());
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

  /**
   * Set up confirm dialog for credential removal.
   */
  confirmRemoveCredential(credentialId, credentialName) {
    this.confirmDialogConfig = {
      title: 'Remove Security Credential',
      message: `Are you sure you want to remove the security credential "${credentialName}"? This action cannot be undone.`,
      action: () => this.removeCredential(credentialId)
    };
    this.showConfirmDialog = true;
  }

  // === Token Actions ===

  async createApiToken() {
    if (!this.newTokenName.trim()) return;

    try {
      this.creatingToken = true;
      const tokenData = {
        name: this.newTokenName.trim(),
        permissions: this.newTokenScopes.length > 0 ? this.newTokenScopes : ['read'],
        expires_at: this.newTokenExpiry || null
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

  /**
   * Set up confirm dialog for token revocation.
   */
  confirmRevokeApiToken(tokenId, tokenName) {
    this.confirmDialogConfig = {
      title: 'Revoke API Token',
      message: `Are you sure you want to revoke the token "${tokenName}"? This action cannot be undone and will immediately invalidate the token.`,
      action: () => this.revokeApiToken(tokenId)
    };
    this.showConfirmDialog = true;
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
        logout_all: this.changePasswordData.logout_all
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
      logout_all: false
    };
  }

  closeNewTokenDisplay() {
    this.showNewToken = false;
    this.newTokenValue = '';
  }

  handleConfirmDialogConfirm() {
    if (this.confirmDialogConfig.action) {
      this.confirmDialogConfig.action();
    }
    this.showConfirmDialog = false;
  }

  handleConfirmDialogCancel() {
    this.showConfirmDialog = false;
  }

  // === Form Resets ===

  resetCredentialForm() {
    this.newCredentialName = '';
    this.newSSHPublicKey = '';
    this.credentialType = 'fido';
    this.showAddCredential = false;
  }

  resetTokenForm() {
    this.newTokenName = '';
    this.newTokenScopes = [];
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
    this.showEnrollmentBanner = false;
    this.enrollmentType = '';
    this.showAddCredential = false;
    this.showAddToken = false;
    this.showNewToken = false;
    this.showChangePassword = false;
    this.showConfirmDialog = false;
    this.credentialType = 'fido';
    this.newCredentialName = '';
    this.newSSHPublicKey = '';
    this.enrollingFIDO = false;
    this.testingLogin = false;
    this.loginTestResult = '';
    this.newTokenName = '';
    this.newTokenScopes = [];
    this.newTokenExpiry = '';
    this.newTokenValue = '';
    this.creatingToken = false;
    this.confirmDialogConfig = { title: '', message: '', action: null };
    this.changePasswordData = {
      current_password: '',
      new_password: '',
      confirm_password: '',
      logout_all: false
    };
    this.changePasswordLoading = false;
    this.changePasswordError = '';
    this.changePasswordSuccess = false;
  }
}

export const securityStore = new SecurityStore();
