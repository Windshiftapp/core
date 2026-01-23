<script>
  import { createEventDispatcher, onMount, tick } from 'svelte';
  import { navigate } from '../router.js';
  import { authStore, ssoStore } from '../stores';
  import { api } from '../api.js';
  import { authPolicy } from '../api/admin.js';
  import { Eye, EyeOff, Lock, User, AlertCircle, LogIn, Key } from 'lucide-svelte';
  import { APP_NAME } from '../constants.js';
  import Button from '../components/Button.svelte';
  import Label from '../components/Label.svelte';
  import {
    isWebAuthnSupported
  } from '../utils/webauthn-utils.js';
  import {
    deriveFidoError,
    evaluateFidoAvailability,
    getBaseLoginState,
    performFidoLogin
  } from '../utils/loginUtils.js';
  import { t } from '../stores/i18n.svelte.js';

  const dispatch = createEventDispatcher();

  export let isOpen = false;

  let emailOrUsername = '';
  let password = '';
  let rememberMe = false;
  let showPassword = false;
  let validationError = '';
  let fidoAvailable = false;
  let tryingFido = false;
  let showFidoOption = false;
  let ssoError = null;
  let ssoRequiredMessage = null;

  // Auth policy status (fetched on mount)
  let policyStatus = {
    hide_password_form: false,
    sso_enabled: false,
    passkey_required: false
  };

  // Focus the first input when dialog opens
  let emailInput;

  // Initialize SSO status and auth policy on mount
  onMount(async () => {
    await Promise.all([
      ssoStore.initStatus(),
      loadPolicyStatus()
    ]);
    // Check for SSO error in URL (after callback redirect)
    ssoError = ssoStore.checkForError();
  });

  // Load public policy status
  async function loadPolicyStatus() {
    try {
      policyStatus = await authPolicy.getPublicStatus();
    } catch (err) {
      console.warn('Failed to load auth policy status:', err);
      // Default to showing password form on error
      policyStatus = { hide_password_form: false, sso_enabled: false, passkey_required: false };
    }
  }

  $: if (isOpen && emailInput) {
    setTimeout(() => {
      try {
        emailInput?.focus();
      } catch (error) {
        console.warn('Failed to focus email input:', error);
      }
    }, 100);
  }
  
  // Clear form when dialog closes
  $: if (!isOpen) {
    clearForm();
  }
  
  function clearForm() {
    const baseState = getBaseLoginState();
    emailOrUsername = baseState.emailOrUsername;
    password = baseState.password;
    rememberMe = baseState.rememberMe;
    showPassword = baseState.showPassword;
    validationError = baseState.validationError;
    fidoAvailable = baseState.fidoAvailable;
    tryingFido = baseState.tryingFido;
    showFidoOption = baseState.showFidoOption;
    ssoError = null;
    ssoRequiredMessage = null;
    authStore.clearError();
  }

  // Handle SSO login
  function handleSSOLogin() {
    ssoStore.startLogin(rememberMe);
  }

  // Check if FIDO authentication is available when user enters email
  async function checkFidoAvailability() {
    const { available, showOption } = await evaluateFidoAvailability(api, emailOrUsername);
    fidoAvailable = available;
    showFidoOption = showOption;
  }

  // Attempt FIDO authentication
  async function handleFidoLogin() {
    if (!emailOrUsername.trim()) {
      validationError = 'Email or username is required';
      return;
    }

    // Check WebAuthn support
    if (!isWebAuthnSupported()) {
      validationError = 'WebAuthn is not supported by this browser';
      return;
    }

    try {
      tryingFido = true;
      validationError = '';
      authStore.clearError();

      const loginResult = await performFidoLogin(api, emailOrUsername);

      if (loginResult.success || loginResult.status === 'success') {
        // Update auth store with user info
        authStore.setAuthData(loginResult.user, loginResult.sessionToken || loginResult.session);
        isOpen = false;
        dispatch('success');
      } else {
        throw new Error(loginResult.message || 'FIDO authentication failed');
      }

    } catch (error) {
      console.error('FIDO authentication error:', error);
      validationError = deriveFidoError(error);
    } finally {
      tryingFido = false;
    }
  }
  
  async function handleSubmit() {
    // Clear previous errors
    validationError = '';
    ssoRequiredMessage = null;
    authStore.clearError();

    // Basic validation
    if (!emailOrUsername.trim()) {
      validationError = 'Email or username is required';
      return;
    }

    if (!password) {
      validationError = 'Password is required';
      return;
    }

    // Attempt login
    const result = await authStore.login({
      email_or_username: emailOrUsername.trim(),
      password: password,
      remember_me: rememberMe
    });

    if (result.success) {
      // Check if enrollment is required
      if (result.enrollment_required) {
        isOpen = false;
        dispatch('success');
        // Redirect to security settings for passkey enrollment
        navigate('/security?enroll=passkey');
        return;
      }

      isOpen = false;
      dispatch('success');
    } else if (result.sso_required) {
      // Show SSO required message instead of regular error
      ssoRequiredMessage = result.policy_message;
    }
  }
  
  function handleKeydown(event) {
    if (event.key === 'Enter' && !$authStore.loading) {
      handleSubmit();
    }
  }
</script>

{#if isOpen}
<!-- Full-screen login overlay that cannot be dismissed -->
<div class="fixed inset-0 bg-[var(--ds-surface)] flex items-center justify-center z-50">
  <div class="bg-[var(--ds-surface-raised)] rounded shadow-xl max-w-md w-full mx-4">
  <div class="p-6">
    <!-- Header -->
    <div class="text-center mb-6">
      <div class="flex justify-center mb-4">
        <img src="/cmicon-2.svg" alt={APP_NAME} class="w-12 h-12" />
      </div>
      <h2 class="text-xl font-semibold text-[var(--ds-text)]">{t('auth.signIn')}</h2>
      <p class="text-sm text-[var(--ds-text-subtle)] mt-1">{t('auth.loginSubtitle')}</p>
    </div>

    <!-- Error Messages -->
    {#if ssoError}
      <div class="mb-4 p-3 bg-[var(--ds-danger-subtle)] border border-[var(--ds-border-danger)] rounded-md">
        <div class="flex items-center">
          <AlertCircle class="w-4 h-4 text-[var(--ds-text-danger)] mr-2 flex-shrink-0" />
          <p class="text-sm text-[var(--ds-text-danger)]">{ssoError}</p>
        </div>
      </div>
    {/if}

    {#if ssoRequiredMessage}
      <div class="mb-4 p-3 bg-[var(--ds-warning-subtle)] border border-[var(--ds-border-warning)] rounded-md">
        <div class="flex items-start">
          <Key class="w-4 h-4 text-[var(--ds-text-warning)] mr-2 flex-shrink-0 mt-0.5" />
          <div>
            <p class="text-sm text-[var(--ds-text-warning)]">{ssoRequiredMessage}</p>
            {#if $ssoStore.enabled}
              <p class="text-xs text-[var(--ds-text-subtle)] mt-1">Use the SSO button above to sign in.</p>
            {/if}
          </div>
        </div>
      </div>
    {/if}

    {#if validationError}
      <div class="mb-4 p-3 bg-[var(--ds-danger-subtle)] border border-[var(--ds-border-danger)] rounded-md">
        <div class="flex items-center">
          <AlertCircle class="w-4 h-4 text-[var(--ds-text-danger)] mr-2 flex-shrink-0" />
          <p class="text-sm text-[var(--ds-text-danger)]">{validationError}</p>
        </div>
      </div>
    {/if}

    {#if $authStore.error && !ssoRequiredMessage}
      <div class="mb-4 p-3 bg-[var(--ds-danger-subtle)] border border-[var(--ds-border-danger)] rounded-md">
        <div class="flex items-center">
          <AlertCircle class="w-4 h-4 text-[var(--ds-text-danger)] mr-2 flex-shrink-0" />
          <p class="text-sm text-[var(--ds-text-danger)]">{$authStore.error}</p>
        </div>
      </div>
    {/if}

    <!-- SSO Login Button -->
    {#if $ssoStore.enabled && !$ssoStore.statusLoading}
      <Button
        variant="default"
        fullWidth={true}
        onclick={handleSSOLogin}
        disabled={$authStore.loading}
      >
        <LogIn class="w-4 h-4 mr-2" />
        {t('auth.continueWith', { provider: $ssoStore.providerName || 'SSO' })}
      </Button>

      {#if $ssoStore.allowPasswordLogin}
        <div class="relative my-4">
          <div class="absolute inset-0 flex items-center">
            <div class="w-full border-t border-[var(--ds-border)]"></div>
          </div>
          <div class="relative flex justify-center text-sm">
            <span class="px-2 bg-[var(--ds-surface-raised)] text-[var(--ds-text-subtle)]">{t('auth.orSignInWithPassword')}</span>
          </div>
        </div>
      {/if}
    {/if}

    <!-- Password Login Form (hidden if SSO-only mode or policy requires it) -->
    {#if policyStatus.hide_password_form}
      <!-- Password form hidden by auth policy -->
      <div class="p-4 bg-[var(--ds-background-neutral)] border border-[var(--ds-border)] rounded-md">
        <div class="flex items-start gap-3">
          <Key class="w-5 h-5 text-[var(--ds-icon)] flex-shrink-0 mt-0.5" />
          <div>
            <p class="text-sm text-[var(--ds-text)] font-medium">Password login is disabled</p>
            <p class="text-xs text-[var(--ds-text-subtle)] mt-1">
              {#if policyStatus.sso_enabled}
                Please use Single Sign-On (SSO) to sign in.
              {:else if policyStatus.passkey_required}
                Please use a passkey to sign in.
              {:else}
                Contact your administrator for access.
              {/if}
            </p>
          </div>
        </div>
      </div>

      <!-- Passkey login option when password is hidden -->
      {#if policyStatus.passkey_required}
        <div class="mt-4">
          <Label for="emailOrUsernamePasskey" color="default" class="mb-1">
            {t('auth.emailOrUsername')}
          </Label>
          <div class="relative">
            <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
              <User class="h-4 w-4 text-[var(--ds-icon)]" />
            </div>
            <input
              id="emailOrUsernamePasskey"
              type="text"
              bind:value={emailOrUsername}
              onblur={checkFidoAvailability}
              disabled={$authStore.loading || tryingFido}
              class="block w-full pl-10 pr-3 py-2 border border-[var(--ds-border)] rounded-md leading-5 bg-[var(--ds-background-input)] placeholder-[var(--ds-text-subtlest)] text-[var(--ds-text)] focus:outline-none focus:placeholder-[var(--ds-text-disabled)] focus:ring-1 focus:ring-[var(--ds-border-focused)] focus:border-[var(--ds-border-focused)] disabled:bg-[var(--ds-background-disabled)] disabled:text-[var(--ds-text-disabled)]"
              placeholder={t('placeholders.enterEmailOrUsername')}
              autocomplete="username"
            />
          </div>
        </div>

        <Button
          variant="primary"
          fullWidth={true}
          loading={tryingFido}
          disabled={$authStore.loading || tryingFido || !emailOrUsername.trim()}
          onclick={handleFidoLogin}
          class="mt-4"
        >
          {#if !tryingFido}
            <Key class="w-4 h-4 mr-2" />
          {/if}
          {tryingFido ? t('auth.touchSecurityKey') : t('auth.signInWithSecurityKey')}
        </Button>
      {/if}
    {:else if !$ssoStore.enabled || $ssoStore.allowPasswordLogin}

    <!-- Login Form -->
    <form onsubmit={(e) => { e.preventDefault(); handleSubmit(); }} class="space-y-4">
      <!-- Email/Username Field -->
      <div>
        <Label for="emailOrUsername" color="default" class="mb-1">
          {t('auth.emailOrUsername')}
        </Label>
        <div class="relative">
          <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
            <User class="h-4 w-4 text-[var(--ds-icon)]" />
          </div>
          <input
            bind:this={emailInput}
            id="emailOrUsername"
            type="text"
            bind:value={emailOrUsername}
            onkeydown={handleKeydown}
            onblur={checkFidoAvailability}
            disabled={$authStore.loading}
            class="block w-full pl-10 pr-3 py-2 border border-[var(--ds-border)] rounded-md leading-5 bg-[var(--ds-background-input)] placeholder-[var(--ds-text-subtlest)] text-[var(--ds-text)] focus:outline-none focus:placeholder-[var(--ds-text-disabled)] focus:ring-1 focus:ring-[var(--ds-border-focused)] focus:border-[var(--ds-border-focused)] disabled:bg-[var(--ds-background-disabled)] disabled:text-[var(--ds-text-disabled)]"
            placeholder={t('placeholders.enterEmailOrUsername')}
            autocomplete="username"
            required
          />
        </div>
      </div>

      <!-- Password Field -->
      <div>
        <Label for="password" color="default" class="mb-1">
          {t('common.password')}
        </Label>
        <div class="relative">
          <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
            <Lock class="h-4 w-4 text-[var(--ds-icon)]" />
          </div>
          <input
            id="password"
            type={showPassword ? 'text' : 'password'}
            bind:value={password}
            onkeydown={handleKeydown}
            disabled={$authStore.loading}
            class="block w-full pl-10 pr-10 py-2 border border-[var(--ds-border)] rounded-md leading-5 bg-[var(--ds-background-input)] placeholder-[var(--ds-text-subtlest)] text-[var(--ds-text)] focus:outline-none focus:placeholder-[var(--ds-text-disabled)] focus:ring-1 focus:ring-[var(--ds-border-focused)] focus:border-[var(--ds-border-focused)] disabled:bg-[var(--ds-background-disabled)] disabled:text-[var(--ds-text-disabled)]"
            placeholder={t('placeholders.enterPassword')}
            autocomplete="current-password"
            required
          />
          <button
            type="button"
            onclick={() => showPassword = !showPassword}
            disabled={$authStore.loading}
            class="absolute inset-y-0 right-0 pr-3 flex items-center text-[var(--ds-icon)] hover:text-[var(--ds-text)] disabled:hover:text-[var(--ds-icon)]"
          >
            {#if showPassword}
              <EyeOff class="h-4 w-4" />
            {:else}
              <Eye class="h-4 w-4" />
            {/if}
          </button>
        </div>
      </div>

      <!-- Remember Me -->
      <div class="flex items-center">
        <input
          id="rememberMe"
          type="checkbox"
          bind:checked={rememberMe}
          disabled={$authStore.loading}
          class="h-4 w-4 text-[var(--ds-interactive)] focus:ring-[var(--ds-border-focused)] border-[var(--ds-border)] rounded disabled:bg-[var(--ds-background-disabled)]"
        />
        <label for="rememberMe" class="ml-2 block text-sm text-[var(--ds-text)]">
          {t('auth.staySignedIn')}
        </label>
      </div>

      <!-- FIDO Authentication Option -->
      {#if showFidoOption && fidoAvailable}
        <div class="relative">
          <div class="absolute inset-0 flex items-center">
            <div class="w-full border-t border-[var(--ds-border)]"></div>
          </div>
          <div class="relative flex justify-center text-sm">
            <span class="px-2 bg-[var(--ds-surface-raised)] text-[var(--ds-text-subtle)]">{t('common.or')}</span>
          </div>
        </div>

        <Button
          variant="default"
          fullWidth={true}
          loading={tryingFido}
          disabled={$authStore.loading || tryingFido || !emailOrUsername.trim()}
          onclick={handleFidoLogin}
        >
          {#if !tryingFido}
            <svg class="w-4 h-4 mr-2" fill="currentColor" viewBox="0 0 20 20">
              <path fill-rule="evenodd" d="M18 8a6 6 0 01-7.743 5.743L10 14l-2 1-1 1H6v2H2v-4l4.257-4.257A6 6 0 1118 8zm-6-4a1 1 0 100 2 2 2 0 012 2 1 1 0 102 0 4 4 0 00-4-4z" clip-rule="evenodd" />
            </svg>
          {/if}
          {tryingFido ? t('auth.touchSecurityKey') : t('auth.signInWithSecurityKey')}
        </Button>
      {/if}

      <!-- Submit Button -->
      <Button
        variant="primary"
        type="submit"
        fullWidth={true}
        loading={$authStore.loading}
        disabled={$authStore.loading || !emailOrUsername.trim() || !password}
      >
        {$authStore.loading ? t('auth.loggingIn') : t('auth.signIn')}
      </Button>
    </form>
    {/if}
  </div>
  </div>
</div>
{/if}
