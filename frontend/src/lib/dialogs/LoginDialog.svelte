<script>
  import { createEventDispatcher, onMount, tick } from 'svelte';
  import { authStore, ssoStore } from '../stores';
  import { api } from '../api.js';
  import { Eye, EyeOff, Lock, User, AlertCircle, LogIn } from 'lucide-svelte';
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

  // Focus the first input when dialog opens
  let emailInput;

  // Initialize SSO status on mount
  onMount(async () => {
    await ssoStore.initStatus();
    // Check for SSO error in URL (after callback redirect)
    ssoError = ssoStore.checkForError();
  });

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
      isOpen = false;
      dispatch('success');
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
      <h2 class="text-xl font-semibold text-[var(--ds-text)]">Sign In</h2>
      <p class="text-sm text-[var(--ds-text-subtle)] mt-1">Please sign in to continue</p>
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

    {#if validationError}
      <div class="mb-4 p-3 bg-[var(--ds-danger-subtle)] border border-[var(--ds-border-danger)] rounded-md">
        <div class="flex items-center">
          <AlertCircle class="w-4 h-4 text-[var(--ds-text-danger)] mr-2 flex-shrink-0" />
          <p class="text-sm text-[var(--ds-text-danger)]">{validationError}</p>
        </div>
      </div>
    {/if}

    {#if $authStore.error}
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
        Sign in with {$ssoStore.providerName || 'SSO'}
      </Button>

      {#if $ssoStore.allowPasswordLogin}
        <div class="relative my-4">
          <div class="absolute inset-0 flex items-center">
            <div class="w-full border-t border-[var(--ds-border)]"></div>
          </div>
          <div class="relative flex justify-center text-sm">
            <span class="px-2 bg-[var(--ds-surface-raised)] text-[var(--ds-text-subtle)]">or sign in with password</span>
          </div>
        </div>
      {/if}
    {/if}

    <!-- Password Login Form (hidden if SSO-only mode) -->
    {#if !$ssoStore.enabled || $ssoStore.allowPasswordLogin}

    <!-- Login Form -->
    <form onsubmit={(e) => { e.preventDefault(); handleSubmit(); }} class="space-y-4">
      <!-- Email/Username Field -->
      <div>
        <Label for="emailOrUsername" color="default" class="mb-1">
          Email or Username
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
            placeholder="Enter email or username"
            autocomplete="username"
            required
          />
        </div>
      </div>

      <!-- Password Field -->
      <div>
        <Label for="password" color="default" class="mb-1">
          Password
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
            placeholder="Enter password"
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
          Keep me signed in for 30 days
        </label>
      </div>

      <!-- FIDO Authentication Option -->
      {#if showFidoOption && fidoAvailable}
        <div class="relative">
          <div class="absolute inset-0 flex items-center">
            <div class="w-full border-t border-[var(--ds-border)]"></div>
          </div>
          <div class="relative flex justify-center text-sm">
            <span class="px-2 bg-[var(--ds-surface-raised)] text-[var(--ds-text-subtle)]">or</span>
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
          {tryingFido ? 'Touch your security key...' : 'Sign in with Security Key'}
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
        {$authStore.loading ? 'Signing in...' : 'Sign In'}
      </Button>
    </form>
    {/if}
  </div>
  </div>
</div>
{/if}
