<script>
  import { contactRoles, customerOrganisations, portalCustomers } from '../../api/portal.js';
  import { time } from '../../api/time.js';
  import Progress from '../../components/Progress.svelte';
  import Clock from '@lucide/svelte/icons/clock';
  import Briefcase from '@lucide/svelte/icons/briefcase';
  import User from '@lucide/svelte/icons/user';
  import X from '@lucide/svelte/icons/x';
  import Button from '../../components/Button.svelte';
  import Input from '../../components/Input.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import AlertBox from '../../components/AlertBox.svelte';
  import Label from '../../components/Label.svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import DescriptionText from '../../components/DescriptionText.svelte';
	import TextareaField from '../../components/TextareaField.svelte';
	import TextField from '../../components/TextField.svelte';

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
      createdCustomer = await customerOrganisations.create(customerData);
      projectData.customer_id = createdCustomer.id;

      // Create a portal customer (contact) for this organisation
      // Get role IDs for "Primary Contact" and "Portal Customer"
      const roles = await contactRoles.getAll();
      const primaryContactRole = roles.find(r => r.name === 'Primary Contact');
      const portalCustomerRole = roles.find(r => r.name === 'Portal Customer');
      const roleIds = [primaryContactRole?.id, portalCustomerRole?.id].filter(id => id != null);

      // Create the portal customer with the contact person info
      if (customerData.contact_person && customerData.email) {
        await portalCustomers.create({
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
      createdProject = await time.projects.create(projectData);
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
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <div tabindex="-1" role="form" onkeydown={handleKeyDown} class="max-h-[90vh] overflow-y-auto outline-none">
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
        <Progress value={progressPercentage} />
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
              <TextField
                label={t('time.organizations.name')}
                id="customer_name"
                required
                type="text"
                size="small"
                placeholder={t('time.onboarding.organizationNamePlaceholder')}
                bind:value={customerData.name}
              />
            </div>

            <div>
              <TextField
                label={t('time.organizations.emailOptional')}
                id="customer_email"
                type="email"
                size="small"
                placeholder={t('time.onboarding.emailPlaceholder')}
                bind:value={customerData.email}
              />
            </div>

            <div>
              <TextField
                label={t('time.organizations.contactPersonOptional')}
                id="contact_person"
                type="text"
                size="small"
                placeholder={t('time.onboarding.contactPersonPlaceholder')}
                bind:value={customerData.contact_person}
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
              <TextField
                label={t('time.projects.projectName')}
                id="project_name"
                required
                type="text"
                size="small"
                placeholder={t('time.onboarding.projectNamePlaceholder')}
                bind:value={projectData.name}
              />
            </div>

            <div>
              <TextareaField
                label={t('time.projects.descriptionOptional')}
                id="project_description"
                rows={3}
                placeholder={t('time.onboarding.projectDescriptionPlaceholder')}
                bind:value={projectData.description}
              />
            </div>

            <div>
              <Label for="hourly_rate" class="mb-2">{t('time.projects.hourlyRateOptional')}</Label>
              <div class="relative">
                <span class="absolute left-3 top-1/2 transform -translate-y-1/2" style="color: var(--ds-text-subtle);">$</span>
                <Input
                  id="hourly_rate"
                  type="number"
                  bind:value={projectData.hourly_rate}
                  min="0"
                  step="0.01"
                  class="pl-8"
                  size="small"
                  placeholder="0.00"
                />
              </div>
              <DescriptionText variant="subtlest">{t('time.onboarding.hourlyRateHint')}</DescriptionText>
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
