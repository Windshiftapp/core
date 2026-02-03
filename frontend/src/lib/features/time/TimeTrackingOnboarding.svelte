<script>
  import { api } from '../../api.js';
  import { Clock, Briefcase, Check, AlertCircle, User, X } from 'lucide-svelte';
  import Button from '../../components/Button.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import AlertBox from '../../components/AlertBox.svelte';
  import Label from '../../components/Label.svelte';
  import { t } from '../../stores/i18n.svelte.js';

  let { oncancel, oncompleted } = $props();

  let currentStep = $state(1);
  let totalSteps = 2;
  let loading = $state(false);
  let error = $state('');

  let customerData = $state({
    name: '',
    email: '',
    contact_person: '',
    active: true
  });

  let projectData = $state({
    customer_id: null,
    name: '',
    description: '',
    hourly_rate: 0,
    active: true
  });

  let createdCustomer = $state(null);
  let createdProject = $state(null);

  async function createCustomer() {
    if (!customerData.name.trim()) {
      error = t('time.onboarding.organizationNameRequired');
      return false;
    }

    try {
      loading = true;
      error = '';

      // Create the customer organisation
      createdCustomer = await api.customerOrganisations.create(customerData);
      projectData.customer_id = createdCustomer.id;

      // Create a portal customer (contact) for this organisation
      // Get role IDs for "Primary Contact" and "Portal Customer"
      const roles = await api.contactRoles.getAll();
      const primaryContactRole = roles.find(r => r.name === 'Primary Contact');
      const portalCustomerRole = roles.find(r => r.name === 'Portal Customer');
      const roleIds = [primaryContactRole?.id, portalCustomerRole?.id].filter(id => id != null);

      // Create the portal customer with the contact person info
      if (customerData.contact_person && customerData.email) {
        await api.portalCustomers.create({
          name: customerData.contact_person,
          email: customerData.email,
          customer_organisation_id: createdCustomer.id,
          is_primary: true,
          role_ids: roleIds
        });
      }

      return true;
    } catch (err) {
      console.error('Failed to create customer:', err);
      error = t('time.onboarding.failedToCreateCustomer');
      return false;
    } finally {
      loading = false;
    }
  }

  async function createProject() {
    if (!projectData.name.trim()) {
      error = t('time.onboarding.projectNameRequired');
      return false;
    }

    try {
      loading = true;
      error = '';
      createdProject = await api.time.projects.create(projectData);
      return true;
    } catch (err) {
      console.error('Failed to create project:', err);
      error = t('time.onboarding.failedToCreateProject');
      return false;
    } finally {
      loading = false;
    }
  }

  async function handleNext() {
    if (currentStep === 1) {
      const success = await createCustomer();
      if (success) {
        currentStep = 2;
        error = '';
      }
    } else if (currentStep === 2) {
      const success = await createProject();
      if (success) {
        // Complete onboarding
        oncompleted?.({
          detail: {
            customer: createdCustomer,
            project: createdProject
          }
        });
      }
    }
  }

  function handleBack() {
    if (currentStep > 1) {
      currentStep--;
      error = '';
    }
  }

  function handleCancel() {
    oncancel?.();
  }

  let progressPercentage = $derived(((currentStep - 1) / (totalSteps - 1)) * 100);

  function handleKeyDown(event) {
    if (event.key === 'Enter') {
      event.preventDefault();
      if (!loading && ((currentStep === 1 && customerData.name.trim()) || (currentStep === 2 && projectData.name.trim()))) {
        handleNext();
      }
    }
  }
</script>

<Modal
  isOpen={true}
  maxWidth="max-w-2xl"
  onclose={handleCancel}
>
  <!-- svelte-ignore a11y-no-noninteractive-tabindex -->
  <div tabindex="0" onkeydown={handleKeyDown} class="max-h-[90vh] overflow-y-auto outline-none">
    <div class="px-8 py-6">
      <!-- Header -->
      <div class="text-center mb-8 relative">
        <!-- Close Button -->
        <button
          onclick={handleCancel}
          class="absolute right-0 top-0 p-2 transition-colors rounded"
          style="color: var(--ds-text-subtlest);"
          title={t('common.cancel')}
        >
          <X class="w-5 h-5" />
        </button>

        <div class="flex justify-center mb-4">
          <div class="w-16 h-16 rounded-full flex items-center justify-center" style="background-color: var(--ds-accent-blue-subtle);">
            <Clock class="w-8 h-8" style="color: var(--ds-icon-accent-blue);" />
          </div>
        </div>
        <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">{t('time.onboarding.title')}</h1>
        <p style="color: var(--ds-text-subtle);">{t('time.onboarding.subtitle')}</p>
      </div>

      <!-- Progress Bar -->
      <div class="mb-8">
        <div class="flex items-center justify-between mb-2">
          <span class="text-sm font-medium" style="color: var(--ds-text-subtle);">{t('time.onboarding.setupProgress')}</span>
          <span class="text-sm" style="color: var(--ds-text-subtlest);">{t('time.onboarding.stepOf', { current: currentStep, total: totalSteps })}</span>
        </div>
        <div class="w-full rounded-full h-2" style="background-color: var(--ds-background-neutral);">
          <div
            class="bg-blue-600 h-2 rounded-full transition-all duration-300"
            style="width: {progressPercentage}%"
          ></div>
        </div>
      </div>

      <!-- Error Message -->
      {#if error}
        <div class="mb-6">
          <AlertBox variant="error" message={error} />
        </div>
      {/if}

      <!-- Step 1: Create Customer -->
      {#if currentStep === 1}
        <div class="space-y-6">
          <div class="text-center">
            <div class="w-12 h-12 rounded-full flex items-center justify-center mx-auto mb-4" style="background-color: var(--ds-accent-blue-subtle);">
              <User class="w-6 h-6" style="color: var(--ds-icon-accent-blue);" />
            </div>
            <h2 class="text-xl font-semibold mb-2" style="color: var(--ds-text);">{t('time.onboarding.createCustomerTitle')}</h2>
            <p style="color: var(--ds-text-subtle);">{t('time.onboarding.createCustomerDescription')}</p>
          </div>

          <div class="space-y-4">
            <div>
              <Label for="customer_name" required class="mb-2">{t('time.organizations.name')}</Label>
              <input
                id="customer_name"
                type="text"
                bind:value={customerData.name}
                class="w-full px-3 py-2 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
                style="background-color: var(--ds-surface); border: 1px solid var(--ds-border); color: var(--ds-text);"
                placeholder={t('time.onboarding.organizationNamePlaceholder')}
                required
              />
            </div>

            <div>
              <Label for="customer_email" class="mb-2">{t('time.organizations.emailOptional')}</Label>
              <input
                id="customer_email"
                type="email"
                bind:value={customerData.email}
                class="w-full px-3 py-2 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
                style="background-color: var(--ds-surface); border: 1px solid var(--ds-border); color: var(--ds-text);"
                placeholder={t('time.onboarding.emailPlaceholder')}
              />
            </div>

            <div>
              <Label for="contact_person" class="mb-2">{t('time.organizations.contactPersonOptional')}</Label>
              <input
                id="contact_person"
                type="text"
                bind:value={customerData.contact_person}
                class="w-full px-3 py-2 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
                style="background-color: var(--ds-surface); border: 1px solid var(--ds-border); color: var(--ds-text);"
                placeholder={t('time.onboarding.contactPersonPlaceholder')}
              />
            </div>
          </div>
        </div>
      {/if}

      <!-- Step 2: Create Project -->
      {#if currentStep === 2}
        <div class="space-y-6">
          <div class="text-center">
            <div class="w-12 h-12 rounded-full flex items-center justify-center mx-auto mb-4" style="background-color: var(--ds-accent-green-subtle);">
              <Briefcase class="w-6 h-6" style="color: var(--ds-icon-accent-green);" />
            </div>
            <h2 class="text-xl font-semibold mb-2" style="color: var(--ds-text);">{t('time.onboarding.createProjectTitle')}</h2>
            <p style="color: var(--ds-text-subtle);">{t('time.onboarding.createProjectDescription')}</p>
          </div>

          <!-- Show created customer -->
          {#if createdCustomer}
            <AlertBox variant="success" message={t('time.onboarding.customerCreatedSuccess', { name: createdCustomer.name })} />
          {/if}

          <div class="space-y-4">
            <div>
              <Label for="project_name" required class="mb-2">{t('time.projects.projectName')}</Label>
              <input
                id="project_name"
                type="text"
                bind:value={projectData.name}
                class="w-full px-3 py-2 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
                style="background-color: var(--ds-surface); border: 1px solid var(--ds-border); color: var(--ds-text);"
                placeholder={t('time.onboarding.projectNamePlaceholder')}
                required
              />
            </div>

            <div>
              <Label for="project_description" class="mb-2">{t('time.projects.descriptionOptional')}</Label>
              <Textarea
                id="project_description"
                bind:value={projectData.description}
                rows={3}
                placeholder={t('time.onboarding.projectDescriptionPlaceholder')}
              />
            </div>

            <div>
              <Label for="hourly_rate" class="mb-2">{t('time.projects.hourlyRateOptional')}</Label>
              <div class="relative">
                <span class="absolute left-3 top-1/2 transform -translate-y-1/2" style="color: var(--ds-text-subtle);">$</span>
                <input
                  id="hourly_rate"
                  type="number"
                  bind:value={projectData.hourly_rate}
                  min="0"
                  step="0.01"
                  class="w-full pl-8 pr-3 py-2 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
                  style="background-color: var(--ds-surface); border: 1px solid var(--ds-border); color: var(--ds-text);"
                  placeholder="0.00"
                />
              </div>
              <p class="text-xs mt-1" style="color: var(--ds-text-subtlest);">{t('time.onboarding.hourlyRateHint')}</p>
            </div>
          </div>
        </div>
      {/if}

      <!-- Actions -->
      <div class="flex justify-between items-center mt-8 pt-6 border-t" style="border-color: var(--ds-border);">
        <div>
          {#if currentStep > 1}
            <Button
              variant="ghost"
              onclick={handleBack}
              disabled={loading}
            >
              {t('common.back')}
            </Button>
          {:else}
            <Button
              variant="ghost"
              onclick={handleCancel}
              disabled={loading}
              keyboardHint="Esc"
            >
              {t('time.onboarding.skipForNow')}
            </Button>
          {/if}
        </div>

        <div class="flex gap-3">
          <Button
            variant="primary"
            onclick={handleNext}
            disabled={loading || (currentStep === 1 && !customerData.name.trim()) || (currentStep === 2 && !projectData.name.trim())}
            loading={loading}
            keyboardHint="↵"
          >
            {currentStep === totalSteps ? t('time.onboarding.completeSetup') : t('common.next')}
          </Button>
        </div>
      </div>
    </div>
  </div>
</Modal>