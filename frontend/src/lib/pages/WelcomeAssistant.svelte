<script>
  import { onMount, tick } from 'svelte';
  import { api } from '../api.js';
  import { User, Blocks, ClipboardList, AlertCircle, Languages } from '@lucide/svelte';
  import Modal from '../dialogs/Modal.svelte';
  import Button from '../components/Button.svelte';
  import Seagulls from '../components/Seagulls.svelte';
  import WaveBackground from '../components/WaveBackground.svelte';
  import { APP_NAME } from '../constants.js';
  import Toggle from '../components/Toggle.svelte';
  import Progress from '../components/Progress.svelte';
  import { i18n, SUPPORTED_LOCALES, t } from '../stores/i18n.svelte.js';
	import TextField from '../components/TextField.svelte';
	import SelectField from '../components/SelectField.svelte';

  let {
    isOpen = $bindable(true),
    'onsetup-completed': onsetupCompleted = null
  } = $props();

  let currentStep = $state(1);
  const totalSteps = 3;
  let submitting = $state(false);
  let error = $state('');
  let selectedLanguage = $state(i18n.locale);
  const languageOptions = SUPPORTED_LOCALES.map(locale => ({
    value: locale.code,
    label: locale.name
  }));

  // Form data
  let adminUser = $state({
    email: '',
    username: 'admin',
    first_name: '',
    last_name: '',
    password: '',
    confirmPassword: ''
  });

  let moduleSettings = $state({
    test_management_enabled: true
  });

  let keyboardDiv = $state(null);

  onMount(() => {
    keyboardDiv?.focus();
  });

  function handleKeyDown(event) {
    if (event.key === 'Enter') {
      event.preventDefault();
      if (currentStep < totalSteps) {
        handleNext();
      } else if (currentStep === totalSteps && !submitting) {
        completeSetup();
      }
    } else if (event.key === 'Escape') {
      event.preventDefault();
      if (currentStep > 1) {
        previousStep();
      }
    }
  }

  function nextStep() {
    if (currentStep < totalSteps) {
      currentStep++;
    }
  }

  function previousStep() {
    if (currentStep > 1) {
      currentStep--;
      error = '';
    }
  }

  function validateCurrentStep() {
    error = '';

    if (currentStep === 2) {
      // Validate admin user form
      if (!adminUser.email || !adminUser.first_name || !adminUser.last_name || !adminUser.password) {
        error = t('setup.fillAllRequired');
        return false;
      }
      if (adminUser.password !== adminUser.confirmPassword) {
        error = t('setup.passwordsMustMatch');
        return false;
      }
      if (!adminUser.email.includes('@')) {
        error = t('setup.invalidEmail');
        return false;
      }
    }

    return true;
  }

  function handleNext() {
    if (validateCurrentStep()) {
      nextStep();
    }
  }

  async function handleLanguageChange(localeCode) {
    selectedLanguage = localeCode;
    await i18n.setLocale(localeCode);
  }

  async function completeSetup() {
    if (!validateCurrentStep()) {
      return;
    }

    submitting = true;
    error = '';

    try {
      const setupData = {
        admin_user: {
          email: adminUser.email,
          username: adminUser.username,
          first_name: adminUser.first_name,
          last_name: adminUser.last_name,
          language: selectedLanguage,
          password: adminUser.password
        },
        module_settings: moduleSettings
      };

      const result = await api.setup.complete(setupData);

      try {
        onsetupCompleted?.(result);
      } catch (callbackError) {
        console.error('Error calling setup-completed callback:', callbackError);
      }

      isOpen = false;
      // Reload the page to reflect the new setup
      window.location.reload();
    } catch (err) {
      console.error('Setup error:', err);
      // Show the server's message when it sends one, for example a password over
      // bcrypt's 72 byte input limit. The generic string stays as the fallback.
      error = err?.message || t('setup.setupError');
      submitting = false;
    }
  }

  let progressPercentage = $derived(((currentStep - 1) / (totalSteps - 1)) * 100);

  // Refocus keyboard div when step changes
  $effect(() => {
    currentStep;
    if (keyboardDiv) {
      tick().then(() => keyboardDiv?.focus());
    }
  });
</script>

{#if isOpen}
  <div class="fixed inset-0 z-40 setup-gradient overflow-hidden">
    <WaveBackground />
    <Seagulls />
  </div>
  <Modal bind:isOpen={isOpen} maxWidth="max-w-2xl" preventClose={true} noBackdrop={true} zIndexClass="z-50 !items-center !pt-0 setup-modal" dataTestid="setup-modal">
    <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
    <div bind:this={keyboardDiv} role="dialog" tabindex="0" onkeydown={handleKeyDown} class="outline-none">
    <div class="px-6 py-8">
      <!-- Header -->
      <div class="text-center mb-8">
        <div class="flex justify-center mb-4">
          <img src="windshift-3.svg" alt={APP_NAME} class="w-16 h-16" />
        </div>
        <h1 class="text-3xl font-bold mb-2" style="color: var(--ds-text);">{t('setup.welcomeTo', { appName: APP_NAME })}</h1>
        <p class="text-lg" style="color: var(--ds-text-subtle);">{t('setup.setupMessage')}</p>
      </div>

      <!-- Progress Bar -->
      <div class="mb-8">
        <div class="flex items-center justify-between mb-2">
          <span class="text-sm font-medium" style="color: var(--ds-text);">{t('setup.setupProgress')}</span>
          <span class="text-sm" style="color: var(--ds-text-subtle);">{t('setup.step')} {currentStep} {t('setup.of')} {totalSteps}</span>
        </div>
        <Progress value={progressPercentage} />
      </div>

      <!-- Error Message -->
      {#if error}
        <div data-testid="setup-validation-error" class="mb-6 p-4 rounded flex items-center gap-2" style="background-color: var(--ds-danger-subtle); border: 1px solid var(--ds-border-danger); color: var(--ds-text-danger);">
          <AlertCircle class="w-5 h-5 flex-shrink-0" />
          <span>{error}</span>
        </div>
      {/if}

      <!-- Step 1: Choose interface language -->
      {#if currentStep === 1}
        <div class="space-y-6">
          <div class="text-center">
            <div class="w-12 h-12 rounded-full flex items-center justify-center mx-auto mb-4" style="background-color: var(--ds-surface-information);">
              <Languages class="w-6 h-6" style="color: var(--ds-icon-info);" />
            </div>
            <h2 class="text-xl font-semibold mb-2" style="color: var(--ds-text);">{t('setup.chooseLanguage')}</h2>
            <p style="color: var(--ds-text-subtle);">{t('setup.chooseLanguageDesc')}</p>
          </div>

          <div class="max-w-sm mx-auto">
            <SelectField
              label={t('setup.language')}
              id="setup-language"
              labelColor="default"
              options={languageOptions}
              onchange={handleLanguageChange}
              ariaLabel={t('setup.language')}
              bind:value={selectedLanguage}
            />
          </div>
        </div>
      {/if}

      <!-- Step 2: Create Admin Account -->
      {#if currentStep === 2}
        <div class="space-y-6">
          <div class="text-center">
            <div class="w-12 h-12 rounded-full flex items-center justify-center mx-auto mb-4" style="background-color: var(--ds-surface-information);">
              <User class="w-6 h-6" style="color: var(--ds-icon-info);" />
            </div>
            <h2 data-testid="setup-admin-heading" class="text-xl font-semibold mb-2" style="color: var(--ds-text);">{t('setup.createAdminAccount')}</h2>
            <p style="color: var(--ds-text-subtle);">{t('setup.adminAccountDesc', { appName: APP_NAME })}</p>
          </div>

          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <TextField
                label={t('setup.firstName')}
                id="first_name"
                required
                labelColor="default"
                type="text"
                placeholder={t('setup.firstName')}
                size="small"
                bind:value={adminUser.first_name}
              />
            </div>

            <div>
              <TextField
                label={t('setup.lastName')}
                id="last_name"
                required
                labelColor="default"
                type="text"
                placeholder={t('setup.lastName')}
                size="small"
                bind:value={adminUser.last_name}
              />
            </div>
          </div>

          <div>
            <TextField
              label={t('setup.emailAddress')}
              id="email"
              required
              labelColor="default"
              type="email"
              placeholder={t('setup.emailAddress')}
              size="small"
              bind:value={adminUser.email}
            />
          </div>

          <div>
            <TextField
              label={t('setup.username')}
              id="username"
              labelColor="default"
              type="text"
              placeholder={t('setup.username')}
              size="small"
              bind:value={adminUser.username}
            />
          </div>

          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <TextField
                label={t('setup.password')}
                id="password"
                required
                labelColor="default"
                type="password"
                placeholder="••••••••"
                size="small"
                bind:value={adminUser.password}
              />
            </div>

            <div>
              <TextField
                label={t('setup.confirmPassword')}
                id="confirm_password"
                required
                labelColor="default"
                type="password"
                placeholder="••••••••"
                size="small"
                bind:value={adminUser.confirmPassword}
              />
            </div>
          </div>
        </div>
      {/if}

      <!-- Step 3: Configure Modules -->
      {#if currentStep === 3}
        <div class="space-y-6">
          <div class="text-center">
            <div class="w-12 h-12 rounded-full flex items-center justify-center mx-auto mb-4" style="background-color: var(--ds-surface-information);">
              <Blocks class="w-6 h-6" style="color: var(--ds-icon-info);" />
            </div>
            <h2 data-testid="setup-modules-heading" class="text-xl font-semibold mb-2" style="color: var(--ds-text);">{t('setup.configureModules')}</h2>
            <p style="color: var(--ds-text-subtle);">{t('setup.configureModulesDesc')}</p>
          </div>

          <div class="space-y-4">
            <div class="border rounded p-4" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
              <div class="flex items-center justify-between">
                <div class="flex items-center gap-3">
                  <div class="w-10 h-10 rounded flex items-center justify-center" style="background-color: var(--ds-surface-information);">
                    <ClipboardList class="w-5 h-5" style="color: var(--ds-icon-info);" />
                  </div>
                  <div>
                    <h3 class="font-medium" style="color: var(--ds-text);">{t('setup.testManagement')}</h3>
                    <p class="text-sm" style="color: var(--ds-text-subtle);">{t('setup.testManagementDesc')}</p>
                  </div>
                </div>
<Toggle bind:checked={moduleSettings.test_management_enabled} dataTestid="setup-test-management" />
              </div>
            </div>
          </div>

          <div class="h-8"></div>
        </div>
      {/if}

      <!-- Actions -->
      <div class="flex justify-between items-center mt-8 pt-6 border-t" style="border-color: var(--ds-border);">
        <div>
          {#if currentStep > 1}
            <Button
              variant="ghost"
              onclick={previousStep}
              title={t('setup.goBackEsc')}
              keyboardHint="Esc"
            >
              {t('setup.back')}
            </Button>
          {/if}
        </div>

        <div class="flex gap-3">
          {#if currentStep < totalSteps}
            <Button
              variant="primary"
              onclick={handleNext}
              title={t('setup.continueNextStepEnter')}
              keyboardHint="↵"
              dataTestid="setup-next"
            >
              {t('setup.next')}
            </Button>
          {:else if currentStep === totalSteps}
            <Button
              variant="primary"
              onclick={completeSetup}
              disabled={submitting}
              loading={submitting}
              title={t('setup.completeSetupEnter')}
              keyboardHint={submitting ? null : '↵'}
              dataTestid="setup-complete"
            >
              {#if submitting}
                {t('setup.settingUp')}
              {:else}
                {t('setup.completeSetup')}
              {/if}
            </Button>
          {/if}
        </div>
      </div>
    </div>
    </div>
  </Modal>
{/if}

<style>
  :global(.setup-modal > div) {
    box-shadow:
      0 0 60px 5px rgba(26, 177, 188, 0.4),
      0 0 100px 20px rgba(40, 116, 187, 0.3),
      0 25px 50px -12px rgba(0, 0, 0, 0.5) !important;
  }

  .setup-gradient {
    background: linear-gradient(135deg, #1d5a94 0%, #2563a8 20%, #2874BB 40%, #1f93a5 60%, #1AB1BC 80%, #2874BB 100%);
    background-size: 200% 200%;
    animation: setup-gradient-shift 90s ease infinite;
  }

  @keyframes setup-gradient-shift {
    0% { background-position: 0% 50%; }
    50% { background-position: 100% 50%; }
    100% { background-position: 0% 50%; }
  }

  @media (prefers-reduced-motion: reduce) {
    .setup-gradient {
      animation: none;
      background: linear-gradient(135deg, #1d5a94 0%, #2874BB 50%, #1AB1BC 100%);
      background-size: 100% 100%;
    }
  }
</style>
