<script>
  import { onMount, createEventDispatcher } from 'svelte';
  import { api } from '../api.js';
  import { User, Blocks, Clock, ClipboardList, AlertCircle, Check } from 'lucide-svelte';
  import Modal from '../dialogs/Modal.svelte';
  import Button from '../components/Button.svelte';
  import Label from '../components/Label.svelte';
  import { APP_NAME } from '../constants.js';
  import Toggle from '../components/Toggle.svelte';
  import { t } from '../stores/i18n.svelte.js';

  export let isOpen = true;

  const dispatch = createEventDispatcher();

  let currentStep = 1;
  let totalSteps = 2;
  let setupStatus = null;
  let loading = true;
  let submitting = false;
  let error = '';

  // Form data
  let adminUser = {
    email: '',
    username: 'admin',
    first_name: '',
    last_name: '',
    password: '',
    confirmPassword: ''
  };

  let moduleSettings = {
    time_tracking_enabled: true,
    test_management_enabled: true
  };

  let keyboardDiv;

  onMount(async () => {
    try {
      const response = await fetch('/api/setup/status');
      if (response.ok) {
        setupStatus = await response.json();
        // If setup is already completed, don't show the assistant
        if (setupStatus.setup_completed) {
          isOpen = false;
        }
      } else {
        console.error('Failed to get setup status');
      }
    } catch (err) {
      console.error('Error checking setup status:', err);
    }
    loading = false;

    // Focus the keyboard div to ensure key events work
    if (keyboardDiv) {
      keyboardDiv.focus();
    }
  });

  function handleKeyDown(event) {
    if (currentStep === 3) return; // Don't handle keys on completion step

    if (event.key === 'Enter') {
      event.preventDefault();
      if (currentStep < 2) {
        handleNext();
      } else if (currentStep === 2 && !submitting) {
        completeSetup();
      }
    } else if (event.key === 'Escape') {
      event.preventDefault();
      if (currentStep > 1 && currentStep < 3) {
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
    }
  }

  function validateCurrentStep() {
    error = '';

    if (currentStep === 1) {
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
          password_hash: adminUser.password // This will be hashed on the backend
        },
        module_settings: moduleSettings
      };

      const result = await api.setup.complete(setupData);

      currentStep = totalSteps; // Show success step

      try {
        dispatch('setup-completed', result);
      } catch (dispatchError) {
        console.error('Error dispatching setup-completed event:', dispatchError);
      }

      setTimeout(() => {
        isOpen = false;
        // Reload the page to reflect the new setup
        window.location.reload();
      }, 2000);
    } catch (err) {
      console.error('Setup error:', err);
      error = t('setup.setupError');
      submitting = false;
    }
  }

  $: progressPercentage = ((currentStep - 1) / (totalSteps - 1)) * 100;

  // Refocus keyboard div when step changes
  $: if (keyboardDiv && currentStep) {
    setTimeout(() => keyboardDiv.focus(), 100);
  }
</script>

{#if !loading && isOpen}
  <div class="fixed inset-0 z-40 setup-gradient"></div>
  <Modal bind:isOpen={isOpen} maxWidth="max-w-2xl" preventClose={true} noBackdrop={true} zIndexClass="z-50 !items-center !pt-0 setup-modal">
    <!-- svelte-ignore a11y-no-noninteractive-tabindex -->
    <div bind:this={keyboardDiv} role="dialog" tabindex="0" onkeydown={handleKeyDown} class="outline-none">
    <div class="px-6 py-8">
      <!-- Header -->
      <div class="text-center mb-8">
        <div class="flex justify-center mb-4">
          <img src="/windshift-3.svg" alt={APP_NAME} class="w-16 h-16" />
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
        <div class="w-full rounded-full h-2" style="background-color: var(--ds-surface);">
          <div
            class="h-2 rounded-full transition-all duration-300"
            style="width: {progressPercentage}%; background: linear-gradient(90deg, #1388E7 0%, #1AB1BC 100%);"
          ></div>
        </div>
      </div>

      <!-- Error Message -->
      {#if error}
        <div class="mb-6 p-4 rounded flex items-center gap-2" style="background-color: var(--ds-danger-subtle); border: 1px solid var(--ds-border-danger); color: var(--ds-text-danger);">
          <AlertCircle class="w-5 h-5 flex-shrink-0" />
          <span>{error}</span>
        </div>
      {/if}

      <!-- Step 1: Create Admin Account -->
      {#if currentStep === 1}
        <div class="space-y-6">
          <div class="text-center">
            <div class="w-12 h-12 rounded-full flex items-center justify-center mx-auto mb-4" style="background-color: var(--ds-surface-information);">
              <User class="w-6 h-6" style="color: var(--ds-icon-info);" />
            </div>
            <h2 class="text-xl font-semibold mb-2" style="color: var(--ds-text);">{t('setup.createAdminAccount')}</h2>
            <p style="color: var(--ds-text-subtle);">{t('setup.adminAccountDesc', { appName: APP_NAME })}</p>
          </div>

          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <Label for="first_name" color="default" required class="mb-2">{t('setup.firstName')}</Label>
              <input
                id="first_name"
                type="text"
                bind:value={adminUser.first_name}
                class="w-full px-3 py-2 border rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
                style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text);"
                placeholder="John"
                required
              />
            </div>

            <div>
              <Label for="last_name" color="default" required class="mb-2">{t('setup.lastName')}</Label>
              <input
                id="last_name"
                type="text"
                bind:value={adminUser.last_name}
                class="w-full px-3 py-2 border rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
                style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text);"
                placeholder="Doe"
                required
              />
            </div>
          </div>

          <div>
            <Label for="email" color="default" required class="mb-2">{t('setup.emailAddress')}</Label>
            <input
              id="email"
              type="email"
              bind:value={adminUser.email}
              class="w-full px-3 py-2 border rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
              style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text);"
              placeholder="admin@example.com"
              required
            />
          </div>

          <div>
            <Label for="username" color="default" class="mb-2">{t('setup.username')}</Label>
            <input
              id="username"
              type="text"
              bind:value={adminUser.username}
              class="w-full px-3 py-2 border rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
              style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text);"
              placeholder="admin"
            />
          </div>

          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <Label for="password" color="default" required class="mb-2">{t('setup.password')}</Label>
              <input
                id="password"
                type="password"
                bind:value={adminUser.password}
                class="w-full px-3 py-2 border rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
                style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text);"
                placeholder="••••••••"
                required
              />
            </div>

            <div>
              <Label for="confirm_password" color="default" required class="mb-2">{t('setup.confirmPassword')}</Label>
              <input
                id="confirm_password"
                type="password"
                bind:value={adminUser.confirmPassword}
                class="w-full px-3 py-2 border rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
                style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text);"
                placeholder="••••••••"
                required
              />
            </div>
          </div>
        </div>
      {/if}

      <!-- Step 2: Configure Modules -->
      {#if currentStep === 2}
        <div class="space-y-6">
          <div class="text-center">
            <div class="w-12 h-12 rounded-full flex items-center justify-center mx-auto mb-4" style="background-color: var(--ds-surface-information);">
              <Blocks class="w-6 h-6" style="color: var(--ds-icon-info);" />
            </div>
            <h2 class="text-xl font-semibold mb-2" style="color: var(--ds-text);">{t('setup.configureModules')}</h2>
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
<Toggle bind:checked={moduleSettings.test_management_enabled} />
              </div>
            </div>
          </div>

          <div class="h-8"></div>
        </div>
      {/if}

      <!-- Step 3: Setup Complete -->
      {#if currentStep === 3}
        <div class="text-center space-y-6">
          <div class="w-16 h-16 rounded-full flex items-center justify-center mx-auto" style="background-color: var(--ds-surface-success);">
            <Check class="w-8 h-8" style="color: var(--ds-icon-success);" />
          </div>
          <h2 class="text-2xl font-semibold" style="color: var(--ds-text);">{t('setup.setupComplete')}</h2>
          <p class="text-lg" style="color: var(--ds-text-subtle);">{t('setup.setupCompleteMessage', { appName: APP_NAME })}</p>

          <div class="border rounded p-4" style="background-color: var(--ds-surface-success); border-color: var(--ds-border-success);">
            <h3 class="font-medium mb-2" style="color: var(--ds-text-success);">{t('setup.whatsNext')}</h3>
            <ul class="text-sm space-y-1 text-left" style="color: var(--ds-text-success);">
              <li>• {t('setup.whatsNextCreateWorkspace')}</li>
              <li>• {t('setup.whatsNextSetupWorkflows')}</li>
              <li>• {t('setup.whatsNextInviteTeam')}</li>
              <li>• {t('setup.whatsNextStartCreating')}</li>
            </ul>
          </div>
        </div>
      {/if}

      <!-- Actions -->
      <div class="flex justify-between items-center mt-8 pt-6 border-t" style="border-color: var(--ds-border);">
        <div>
          {#if currentStep > 1 && currentStep < 3}
            <Button
              variant="ghost"
              onclick={previousStep}
              title={t('setup.goBackEsc')}
            >
              <span class="flex items-center gap-2">
                {t('setup.back')}
                <kbd class="px-1 py-0.5 rounded text-xs" style="background-color: var(--ds-surface); color: var(--ds-text-subtle);">Esc</kbd>
              </span>
            </Button>
          {/if}
        </div>

        <div class="flex gap-3">
          {#if currentStep < 2}
            <Button
              variant="primary"
              onclick={handleNext}
              title={t('setup.continueNextStepEnter')}
            >
              <span class="flex items-center gap-2">
                {t('setup.next')}
                <kbd class="px-1 py-0.5 bg-blue-700 rounded text-xs">↵</kbd>
              </span>
            </Button>
          {:else if currentStep === 2}
            <Button
              variant="primary"
              onclick={completeSetup}
              disabled={submitting}
              loading={submitting}
              title={t('setup.completeSetupEnter')}
            >
              {#if submitting}
                {t('setup.settingUp')}
              {:else}
                <span class="flex items-center gap-2">
                  {t('setup.completeSetup')}
                  <kbd class="px-1 py-0.5 bg-blue-700 rounded text-xs">↵</kbd>
                </span>
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
