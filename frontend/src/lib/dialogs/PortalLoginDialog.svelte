<script>
  import { createEventDispatcher, tick } from 'svelte';
  import { authStore } from '../stores';
  import { api } from '../api.js';
  import { Eye, EyeOff, Lock, User, AlertCircle, X } from 'lucide-svelte';
  import Button from '../components/Button.svelte';
  import { isWebAuthnSupported } from '../utils/webauthn-utils.js';
  import {
    deriveFidoError,
    evaluateFidoAvailability,
    getBaseLoginState,
    performFidoLogin
  } from '../utils/loginUtils.js';
  import { t } from '../stores/i18n.svelte.js';

  const dispatch = createEventDispatcher();

  export let isOpen = false;
  export let gradientValue = 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)';
  export let isDarkMode = false;

  let emailOrUsername = '';
  let password = '';
  let rememberMe = false;
  let showPassword = false;
  let validationError = '';
  let fidoAvailable = false;
  let tryingFido = false;
  let showFidoOption = false;

  // Focus the first input when dialog opens
  let emailInput;

  $: if (isOpen && emailInput) {
    tick().then(() => {
      try {
        emailInput?.focus();
      } catch (error) {
        console.warn('Failed to focus email input:', error);
      }
    });
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
    authStore.clearError();
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
      validationError = t('portalLogin.emailRequired');
      return;
    }

    if (!isWebAuthnSupported()) {
      validationError = t('portalLogin.webAuthnNotSupported');
      return;
    }

    try {
      tryingFido = true;
      validationError = '';
      authStore.clearError();

      const loginResult = await performFidoLogin(api, emailOrUsername);

      if (loginResult.success) {
        // Update auth store with user info
        authStore.setAuthData(loginResult.user, loginResult.session);
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
      validationError = t('portalLogin.emailRequired');
      return;
    }

    if (!password) {
      validationError = t('portalLogin.passwordRequired');
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
    if (event.key === 'Enter' && !authStore.loading) {
      handleSubmit();
    }
  }

  function handleGlobalKeydown(event) {
    if (event.key === 'Escape' && isOpen) {
      isOpen = false;
    }
  }

  function handleClose() {
    isOpen = false;
  }
</script>

<!-- Global keydown listener for ESC key (must be at top level) -->
<svelte:window onkeydown={handleGlobalKeydown} />

{#if isOpen}
<!-- Portal-styled login overlay -->
<div
  class="fixed inset-0 flex items-center justify-center z-50 p-4"
  style="background-color: rgba(0, 0, 0, 0.4); backdrop-filter: blur(8px);"
  onclick={handleClose}
>
  <!-- Login Card with portal gradient accent -->
  <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
  <div
    class="w-full max-w-md rounded-2xl shadow-2xl overflow-hidden"
    style="background-color: {isDarkMode ? '#1e293b' : '#ffffff'};"
    onclick={(e) => e.stopPropagation()}
  >
    <!-- Gradient Header -->
    <div
      class="px-8 py-12 text-white text-center relative"
      style="background: {gradientValue};"
    >
      <div class="absolute top-4 right-4">
        <button
          type="button"
          onclick={handleClose}
          class="p-2 rounded hover:bg-white/10 transition-all"
        >
          <X class="w-5 h-5" />
        </button>
      </div>

      <div class="flex justify-center mb-4">
        <div class="w-16 h-16 rounded-full bg-white/20 backdrop-blur-sm flex items-center justify-center">
          <User class="w-8 h-8 text-white" />
        </div>
      </div>
      <h2 class="text-2xl font-bold">{t('portalLogin.welcomeBack')}</h2>
      <p class="text-white/80 mt-2">{t('portalLogin.signInToCustomize')}</p>
    </div>

    <!-- Form Content -->
    <div class="px-8 py-6">
      <!-- Error Messages -->
      {#if validationError}
        <div class="mb-4 p-3 rounded" style="background-color: {isDarkMode ? 'rgba(239, 68, 68, 0.1)' : '#fef2f2'}; border: 1px solid {isDarkMode ? 'rgba(239, 68, 68, 0.3)' : '#fecaca'};">
          <div class="flex items-center">
            <AlertCircle class="w-4 h-4 mr-2" style="color: {isDarkMode ? '#fca5a5' : '#dc2626'};" />
            <p class="text-sm" style="color: {isDarkMode ? '#fca5a5' : '#dc2626'};">{validationError}</p>
          </div>
        </div>
      {/if}

      {#if authStore.error}
        <div class="mb-4 p-3 rounded" style="background-color: {isDarkMode ? 'rgba(239, 68, 68, 0.1)' : '#fef2f2'}; border: 1px solid {isDarkMode ? 'rgba(239, 68, 68, 0.3)' : '#fecaca'};">
          <div class="flex items-center">
            <AlertCircle class="w-4 h-4 mr-2" style="color: {isDarkMode ? '#fca5a5' : '#dc2626'};" />
            <p class="text-sm" style="color: {isDarkMode ? '#fca5a5' : '#dc2626'};">{authStore.error}</p>
          </div>
        </div>
      {/if}

      <!-- Login Form -->
      <form onsubmit={(e) => { e.preventDefault(); handleSubmit(); }} class="space-y-4">
        <!-- Email/Username Field -->
        <div>
          <label for="emailOrUsername" class="block text-sm font-medium mb-2" style="color: {isDarkMode ? '#e2e8f0' : '#374151'};">
            {t('portalLogin.emailOrUsername')}
          </label>
          <div class="relative">
            <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
              <User class="h-4 w-4" style="color: {isDarkMode ? '#64748b' : '#9ca3af'};" />
            </div>
            <input
              bind:this={emailInput}
              id="emailOrUsername"
              type="text"
              bind:value={emailOrUsername}
              onkeydown={handleKeydown}
              onblur={checkFidoAvailability}
              disabled={authStore.loading}
              class="block w-full pl-10 pr-3 py-2.5 rounded leading-5 focus:outline-none focus:ring-2 focus:ring-offset-0 transition-all"
              style="background-color: {isDarkMode ? '#334155' : '#f9fafb'}; color: {isDarkMode ? '#e2e8f0' : '#111827'}; border: 1px solid {isDarkMode ? '#475569' : '#e5e7eb'};"
              placeholder={t('portalLogin.enterEmailOrUsername')}
              autocomplete="username"
              required
            />
          </div>
        </div>

        <!-- Password Field -->
        <div>
          <label for="password" class="block text-sm font-medium mb-2" style="color: {isDarkMode ? '#e2e8f0' : '#374151'};">
            {t('portalLogin.password')}
          </label>
          <div class="relative">
            <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
              <Lock class="h-4 w-4" style="color: {isDarkMode ? '#64748b' : '#9ca3af'};" />
            </div>
            <input
              id="password"
              type={showPassword ? 'text' : 'password'}
              bind:value={password}
              onkeydown={handleKeydown}
              disabled={authStore.loading}
              class="block w-full pl-10 pr-10 py-2.5 rounded leading-5 focus:outline-none focus:ring-2 focus:ring-offset-0 transition-all"
              style="background-color: {isDarkMode ? '#334155' : '#f9fafb'}; color: {isDarkMode ? '#e2e8f0' : '#111827'}; border: 1px solid {isDarkMode ? '#475569' : '#e5e7eb'};"
              placeholder={t('portalLogin.enterPassword')}
              autocomplete="current-password"
              required
            />
            <button
              type="button"
              onclick={() => showPassword = !showPassword}
              disabled={authStore.loading}
              class="absolute inset-y-0 right-0 pr-3 flex items-center transition-opacity hover:opacity-70"
              style="color: {isDarkMode ? '#64748b' : '#9ca3af'};"
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
            disabled={authStore.loading}
            class="h-4 w-4 rounded border-gray-300 focus:ring-2 focus:ring-offset-0"
          />
          <label for="rememberMe" class="ml-2 block text-sm" style="color: {isDarkMode ? '#cbd5e1' : '#4b5563'};">
            {t('portalLogin.keepMeSignedIn')}
          </label>
        </div>

        <!-- FIDO Authentication Option -->
        {#if showFidoOption && fidoAvailable}
          <div class="relative">
            <div class="absolute inset-0 flex items-center">
              <div class="w-full border-t" style="border-color: {isDarkMode ? '#475569' : '#e5e7eb'};"></div>
            </div>
            <div class="relative flex justify-center text-sm">
              <span class="px-2" style="background-color: {isDarkMode ? '#1e293b' : '#ffffff'}; color: {isDarkMode ? '#94a3b8' : '#6b7280'};">{t('portalLogin.or')}</span>
            </div>
          </div>

          <Button
            variant="default"
            fullWidth={true}
            loading={tryingFido}
            disabled={authStore.loading || tryingFido || !emailOrUsername.trim()}
            onclick={handleFidoLogin}
          >
            {#if !tryingFido}
              <svg class="w-4 h-4 mr-2" fill="currentColor" viewBox="0 0 20 20">
                <path fill-rule="evenodd" d="M18 8a6 6 0 01-7.743 5.743L10 14l-2 1-1 1H6v2H2v-4l4.257-4.257A6 6 0 1118 8zm-6-4a1 1 0 100 2 2 2 0 012 2 1 1 0 102 0 4 4 0 00-4-4z" clip-rule="evenodd" />
              </svg>
            {/if}
            {tryingFido ? t('portalLogin.touchSecurityKey') : t('portalLogin.signInWithSecurityKey')}
          </Button>
        {/if}

        <!-- Submit Button -->
        <Button
          variant="primary"
          type="submit"
          fullWidth={true}
          loading={authStore.loading}
          disabled={authStore.loading || !emailOrUsername.trim() || !password}
        >
          {authStore.loading ? t('portalLogin.signingIn') : t('portalLogin.signIn')}
        </Button>
      </form>
    </div>
  </div>
</div>
{/if}
