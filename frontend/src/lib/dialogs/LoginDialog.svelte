<script>
  import { onDestroy, onMount, tick } from 'svelte';
  import { navigate } from '../router.js';
  import { authStore } from '../stores/auth.svelte.js';
  import { ssoStore } from '../stores/sso.svelte.js';

  import { authPolicy } from '../api/admin.js';
  import Eye from '@lucide/svelte/icons/eye';
  import EyeOff from '@lucide/svelte/icons/eye-off';
  import Lock from '@lucide/svelte/icons/lock';
  import User from '@lucide/svelte/icons/user';
  import LogIn from '@lucide/svelte/icons/log-in';
  import Github from '@tabler/icons-svelte-runes/icons/brand-github';
  import Key from '@lucide/svelte/icons/key';
  import { brandStore } from '../stores/brand.svelte.js';
  import Button from '../components/Button.svelte';
  import Checkbox from '../components/Checkbox.svelte';
  import Input from '../components/Input.svelte';
  import Label from '../components/Label.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import Spinner from '../components/Spinner.svelte';
  import { isConditionalMediationAvailable, isWebAuthnSupported } from '../utils/webauthn-utils.js';
  import {
    deriveFidoError,
    performFidoLogin
  } from '../utils/loginUtils.js';
  import { t } from '../stores/i18n.svelte.js';

  let {
    isOpen = $bindable(false),
    onsuccess = undefined
  } = $props();

  let emailOrUsername = $state('');
  let password = $state('');
  let rememberMe = $state(false);
  let showPassword = $state(false);
  let validationError = $state('');
  let tryingFido = $state(false);
  let activeAttempt = $state(false);
  let committingPasskey = $state(false);
  let enrollment = $state(false);
  let invalidField = $state('');
  let passwordInput = $state(null);
  let attemptController = null;
  let conditionalController = null;
  let destroyed = false;
  let ssoError = $state(null);
  let ssoRequiredMessage = $state(null);
  let loginOptionsReady = $state(false);
  let retryingOptions = $state(false);

  // Auth policy status (fetched on mount)
  let policyStatus = $state({
    hide_password_form: false,
    sso_enabled: false,
    passkey_required: false,
    password_first_required: false
  });

  // Focus the first input when dialog opens
  let emailInput = $state(null);
  const busy = $derived(activeAttempt || $authStore.loading);
  const standalonePasskey = $derived(!policyStatus.password_first_required &&
    (!policyStatus.hide_password_form || policyStatus.passkey_required));
  const passkeyPrimary = $derived(standalonePasskey && policyStatus.passkey_required);

  // Initialize SSO status and auth policy on mount
  onMount(async () => {
    try {
      await Promise.all([
        ssoStore.initStatus(),
        loadPolicyStatus()
      ]);
      // Check for SSO error in URL (after callback redirect)
      ssoError = ssoStore.checkForError();
    } finally {
      loginOptionsReady = true;
    }
  });

  onDestroy(() => {
    destroyed = true;
    cancelCredentialRequests();
  });

  $effect(() => {
    if (isOpen && loginOptionsReady && standalonePasskey) {
      const controller = new AbortController();
      conditionalController = controller;
      void startConditionalLogin(controller);
      return () => controller.abort();
    }
  });

  async function startConditionalLogin(controller) {
    if (!await isConditionalMediationAvailable() || controller.signal.aborted) return;
    await runPasskey(controller, true);
  }

  function cancelCredentialRequests() {
    conditionalController?.abort();
    attemptController?.abort();
  }

  function cancelPasskey() {
    if (committingPasskey) return;
    attemptController?.abort();
    attemptController = null;
    activeAttempt = false;
    tryingFido = false;
  }

  async function retryLoginOptions() {
    if (busy || retryingOptions) return;
    retryingOptions = true;
    try {
      await ssoStore.initStatus();
    } finally {
      retryingOptions = false;
    }
  }

  // Load public policy status
  async function loadPolicyStatus() {
    try {
      policyStatus = await authPolicy.getPublicStatus();
    } catch (err) {
      console.warn('Failed to load auth policy status:', err);
      // Default to showing password form on error
      policyStatus = { hide_password_form: false, sso_enabled: false, passkey_required: false, password_first_required: false };
    }
  }

  $effect(() => {
    if (isOpen && emailInput) {
      try {
        emailInput.focus();
      } catch (error) {
        console.warn('Failed to focus email input:', error);
      }
    }
  });

  // Clear form when dialog closes
  $effect(() => {
    if (!isOpen) {
      clearForm();
    }
  });
  
  function clearForm() {
    cancelCredentialRequests();
    emailOrUsername = '';
    password = '';
    rememberMe = false;
    showPassword = false;
    validationError = '';
    invalidField = '';
    tryingFido = false;
    activeAttempt = false;
    committingPasskey = false;
    enrollment = false;
    ssoError = null;
    ssoRequiredMessage = null;
    authStore.clearError();
  }

  function handleSSOLogin() {
    handleSSOLoginForProvider(null);
  }

  function handleSSOLoginForProvider(provider) {
    if (busy) return;
    conditionalController?.abort();
    activeAttempt = true;
    clearErrors();
    try {
      if (provider) ssoStore.startLogin(provider.slug, provider.provider_type, rememberMe);
      else ssoStore.startLogin(rememberMe);
    } catch (error) {
      activeAttempt = false;
      validationError = error.message;
    }
  }

  async function handleFidoLogin() {
    if (busy || !standalonePasskey) return;
    conditionalController?.abort();
    const controller = new AbortController();
    attemptController = controller;
    await runPasskey(controller);
  }

  async function runPasskey(controller, conditional = false) {
    if (!isWebAuthnSupported()) {
      validationError = t('security.webAuthnNotSupported');
      return;
    }
    try {
      if (!conditional) {
        activeAttempt = true;
        tryingFido = true;
        clearErrors();
      }
      const loginResult = await performFidoLogin(conditional ? '' : emailOrUsername, rememberMe, {
        signal: controller.signal,
        conditional,
        getRememberMe: () => rememberMe,
        onCredential: () => {
          controller.signal.throwIfAborted();
          if (destroyed || !isOpen || (conditional && busy)) throw new DOMException('Cancelled', 'AbortError');
          activeAttempt = true;
          tryingFido = true;
          committingPasskey = true;
          clearErrors();
        }
      });
      if (controller.signal.aborted || destroyed || !isOpen) return;
      if (loginResult.success || loginResult.status === 'success') {
        await authStore.completePasskeyLogin(loginResult.user, { signal: controller.signal });
        if (controller.signal.aborted || destroyed || !isOpen) return;
        isOpen = false;
        onsuccess?.();
      } else {
        throw new Error(loginResult.message || t('security.authenticationFailed'));
      }
    } catch (error) {
      if (!controller.signal.aborted && !destroyed && isOpen && (!conditional || committingPasskey)) {
        validationError = deriveFidoError(error);
        authStore.clearError();
      }
    } finally {
      if (!controller.signal.aborted && !destroyed) {
        tryingFido = false;
        activeAttempt = false;
        committingPasskey = false;
      }
    }
  }

  function clearErrors() {
    validationError = '';
    invalidField = '';
    ssoRequiredMessage = null;
    authStore.clearError();
  }

  async function fieldError(field, message) {
    invalidField = field;
    validationError = message;
    await tick();
    (field === 'username' ? emailInput : passwordInput)?.focus();
  }

  async function handleSubmit() {
    if (busy || policyStatus.hide_password_form) return;
    clearErrors();
    if (!emailOrUsername.trim()) {
      await fieldError('username', t('auth.usernameRequired'));
      return;
    }
    if (!password) {
      await fieldError('password', t('auth.passwordRequired'));
      return;
    }
    conditionalController?.abort();
    const controller = new AbortController();
    attemptController = controller;
    activeAttempt = true;
    try {
      const result = await authStore.login({
        email_or_username: emailOrUsername.trim(),
        password,
        remember_me: rememberMe
      }, { signal: controller.signal });
      if (controller.signal.aborted || destroyed || !isOpen) return;
      if (result.success) {
        isOpen = false;
        onsuccess?.();
        if (result.enrollment_required) navigate('/security?enroll=passkey');
      } else if (result.passkey_required) {
        await runPasskey(controller);
      } else if (result.sso_required) {
        ssoRequiredMessage = result.policy_message;
      }
    } catch (error) {
      if (!controller.signal.aborted && !destroyed) validationError = error.message;
    } finally {
      if (!controller.signal.aborted && !destroyed) activeAttempt = false;
    }
  }
  
  function handleKeydown(event, fromPassword = false) {
    if (event.key !== 'Enter' || event.isComposing) return;
    event.preventDefault();
    if (busy) return;
    if (!fromPassword && standalonePasskey && !enrollment && (passkeyPrimary || !password)) handleFidoLogin();
    else handleSubmit();
  }
</script>

{#if isOpen}
<!-- Full-screen login overlay that cannot be dismissed -->
<div
  class="fixed inset-0 bg-[var(--ds-surface)] flex items-center justify-center z-50"
  data-testid="login-dialog"
>
  <div class="bg-[var(--ds-surface-raised)] rounded shadow-xl max-w-md w-full mx-4">
  <div class="p-6">
    <!-- Header -->
    <div class="text-center mb-6">
      <div class="flex justify-center mb-4">
        <img src="windshift-3.svg" alt={brandStore.siteTitle} class="w-12 h-12" />
      </div>
      <h2 class="text-xl font-semibold text-[var(--ds-text)]">{t('auth.signIn')}</h2>
      <p class="text-sm text-[var(--ds-text-subtle)] mt-1">{t('auth.loginSubtitle')}</p>
    </div>

    <!-- Error Messages -->
    {#if ssoError}
      <div data-testid="login-sso-error">
        <AlertBox variant="error" role="alert" message={ssoError} class="mb-4" />
      </div>
    {/if}

    {#if ssoRequiredMessage}
      <AlertBox variant="warning" role="alert" class="mb-4">
        {ssoRequiredMessage}
        {#if $ssoStore.enabled}
          <p class="text-xs text-[var(--ds-text-subtle)] mt-1">{t('auth.useSSO')}</p>
        {/if}
      </AlertBox>
    {/if}

    {#if validationError}
      <AlertBox id="login-validation-error" role="alert" variant="error" message={validationError} class="mb-4" />
    {/if}

    {#if $authStore.error && !ssoRequiredMessage && !validationError}
      <AlertBox role="alert" variant="error" message={$authStore.error} class="mb-4" />
    {/if}

    {#if !loginOptionsReady}
      <div
        class="flex min-h-40 items-center justify-center gap-3 text-sm text-[var(--ds-text-subtle)]"
        data-testid="login-options-loading"
      >
        <Spinner size="sm" />
        <span>{t('common.loading')}</span>
      </div>
    {:else}
    {#if $ssoStore.statusError}
      <AlertBox variant="error" role="alert" class="mb-4">
        <p>{t('auth.loginOptionsError')}</p>
        <Button onclick={retryLoginOptions} disabled={busy} loading={retryingOptions} dataTestid="login-options-retry" class="mt-3">
          {t('common.retry')}
        </Button>
      </AlertBox>
    {/if}
    <!-- Keep the session choice in front of SSO so it also remains available
         when the authentication policy hides the password form. -->
    {#if $ssoStore.enabled && !$ssoStore.statusLoading}
      <Checkbox
        bind:checked={rememberMe}
        disabled={busy}
        label={t('auth.staySignedIn')}
        size="small"
        class="mb-4"
        dataTestid="login-remember-me"
      />

      <!-- SSO Login Button(s) -->
      {#if $ssoStore.providers.length <= 1}
        <Button
          variant="default"
          fullWidth={true}
          onclick={handleSSOLogin}
          disabled={busy}
          dataTestid="login-sso"
          class={$ssoStore.providerType === 'github' ? 'github-login' : ''}
        >
          {#if $ssoStore.providerType === 'github'}
            <Github class="w-5 h-5 mr-2" data-testid="login-github-icon" />
          {:else}
            <LogIn class="w-4 h-4 mr-2" />
          {/if}
          {t('auth.continueWith', { provider: $ssoStore.providerName || 'SSO' })}
        </Button>
      {:else}
        <div class="space-y-2">
          {#each $ssoStore.providers as provider}
            <Button
              variant="default"
              fullWidth={true}
              onclick={() => handleSSOLoginForProvider(provider)}
              disabled={busy}
              dataTestid={`login-sso-${provider.slug}`}
              class={provider.provider_type === 'github' ? 'github-login' : ''}
            >
              {#if provider.provider_type === 'github'}
                <Github class="w-5 h-5 mr-2" data-testid={`login-github-icon-${provider.slug}`} />
              {:else}
                <LogIn class="w-4 h-4 mr-2" />
              {/if}
              {t('auth.continueWith', { provider: provider.name || 'SSO' })}
            </Button>
          {/each}
        </div>
      {/if}

      {#if !policyStatus.hide_password_form}
        <div class="relative my-4">
          <div class="absolute inset-0 flex items-center">
            <div class="w-full border-t border-[var(--ds-border)]"></div>
          </div>
          <div class="relative flex justify-center text-sm">
            <span class="px-2 bg-[var(--ds-surface-raised)] text-[var(--ds-text-subtle)]">{t('common.or')}</span>
          </div>
        </div>
      {/if}
    {/if}

    <!-- Password Login Form (hidden if SSO-only mode or policy requires it) -->
    {#if policyStatus.hide_password_form && !standalonePasskey}
      <!-- Password form hidden by auth policy -->
      <div class="p-4 bg-[var(--ds-background-neutral)] border border-[var(--ds-border)] rounded-md">
        <div class="flex items-start gap-3">
          <Key class="w-5 h-5 text-[var(--ds-icon)] flex-shrink-0 mt-0.5" />
          <div>
            <p class="text-sm text-[var(--ds-text)] font-medium">{t('auth.passwordDisabled')}</p>
            <p class="text-xs text-[var(--ds-text-subtle)] mt-1">
              {#if policyStatus.sso_enabled}
                {t('auth.useSSO')}
              {:else if policyStatus.passkey_required}
                {t('auth.usePasskey')}
              {:else}
                {t('auth.contactAdmin')}
              {/if}
            </p>
          </div>
        </div>
      </div>

    {:else}

    <!-- Login Form -->
    <form
      onsubmit={(e) => { e.preventDefault(); handleSubmit(); }}
      novalidate
      class="space-y-4"
      data-testid="login-password-form"
    >
      <!-- Email/Username Field -->
      <div>
        <Label for="emailOrUsername" color="default" class="mb-1">
          {t('auth.emailOrUsername')}
        </Label>
        <div class="relative">
          <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
            <User class="h-4 w-4 text-[var(--ds-icon)]" />
          </div>
          <Input
            bind:inputRef={emailInput}
            id="emailOrUsername"
            type="text"
            bind:value={emailOrUsername}
            onchange={(event) => emailOrUsername = event.currentTarget.value}
            onkeydown={(event) => handleKeydown(event)}
            disabled={busy}
            class="pl-10 pr-3 py-2 leading-5"
            placeholder={t('placeholders.enterEmailOrUsername')}
            autocomplete={standalonePasskey ? 'username webauthn' : 'username'}
            required={!standalonePasskey}
            ariaInvalid={invalidField === 'username'}
            ariaDescribedby={invalidField === 'username' ? 'login-validation-error' : standalonePasskey ? 'login-passkey-hint' : undefined}
          />
        </div>
      </div>

      {#if standalonePasskey}
        <p id="login-passkey-hint" class="text-xs text-[var(--ds-text-subtle)]">{t('auth.passkeyHint')}</p>
        <Button
          variant={passkeyPrimary ? 'primary' : 'default'}
          fullWidth={true}
          loading={tryingFido}
          disabled={busy}
          onclick={handleFidoLogin}
          dataTestid="login-passkey"
          icon={Key}
        >
          {tryingFido ? t('auth.passkeyProgress') : t('auth.signInWithPasskey')}
        </Button>
      {/if}

      {#if tryingFido && !committingPasskey}
        <Button fullWidth onclick={cancelPasskey} dataTestid="login-cancel-passkey">{t('common.cancel')}</Button>
      {/if}

      {#if passkeyPrimary && !policyStatus.hide_password_form}
        <Button variant="link" disabled={busy} onclick={() => { conditionalController?.abort(); enrollment = !enrollment; password = ''; clearErrors(); }} dataTestid="login-enrollment">
          {t('auth.enrollFirstPasskey')}
        </Button>
        {#if enrollment}
          <p class="text-sm text-[var(--ds-text-subtle)]">{t('auth.enrollmentPasswordHint')}</p>
        {/if}
      {/if}

      {#if !policyStatus.hide_password_form && (!passkeyPrimary || enrollment)}
      <!-- Password Field -->
      <div>
        <Label for="password" color="default" class="mb-1">
          {t('common.password')}
        </Label>
        <div class="relative">
          <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
            <Lock class="h-4 w-4 text-[var(--ds-icon)]" />
          </div>
          <Input
            id="password"
            bind:inputRef={passwordInput}
            type={showPassword ? 'text' : 'password'}
            bind:value={password}
            onkeydown={(event) => handleKeydown(event, true)}
            onfocus={() => conditionalController?.abort()}
            disabled={busy}
            class="pl-10 pr-10 py-2 leading-5"
            placeholder={t('placeholders.enterPassword')}
            autocomplete="current-password"
            required
            ariaInvalid={invalidField === 'password'}
            ariaDescribedby={invalidField === 'password' ? 'login-validation-error' : undefined}
          />
          <Button
            variant="ghost"
            size="small"
            ariaLabel={showPassword ? t('auth.hidePassword') : t('auth.showPassword')}
            pressed={showPassword}
            dataTestid="login-password-visibility"
            onclick={() => showPassword = !showPassword}
            disabled={busy}
            class="absolute inset-y-0 right-0"
          >
            {#if showPassword}
              <EyeOff class="h-4 w-4" />
            {:else}
              <Eye class="h-4 w-4" />
            {/if}
          </Button>
        </div>
      </div>
      {/if}

      {#if !$ssoStore.enabled || $ssoStore.statusLoading}
        <Checkbox
          bind:checked={rememberMe}
          disabled={busy}
          label={t('auth.staySignedIn')}
          size="small"
          dataTestid="login-remember-me"
        />
      {/if}

      {#if !policyStatus.hide_password_form && (!passkeyPrimary || enrollment)}
      <!-- Submit Button -->
      <Button
        variant="primary"
        type="submit"
        dataTestid="login-submit"
        fullWidth={true}
        loading={$authStore.loading}
        disabled={busy}
      >
        {$authStore.loading ? t('auth.loggingIn') : passkeyPrimary ? t('auth.enrollFirstPasskey') : t('auth.signIn')}
      </Button>
      {/if}
    </form>
    {/if}
    {/if}
  </div>
  </div>
</div>
{/if}

<style>
  :global(button.github-login) {
    background-color: var(--ds-background-neutral-bold);
    color: var(--ds-text-inverse);
    border-color: var(--ds-background-neutral-bold);
  }

  :global(button.github-login:hover:not(:disabled)) {
    background-color: var(--ds-background-neutral-bold-hovered);
  }
</style>
