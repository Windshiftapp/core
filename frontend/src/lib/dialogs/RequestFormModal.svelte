<script>
  import { api } from '../api.js';
  import { authStore } from '../stores';
  import { portalAuthStore } from '../stores/portalAuth.svelte.js';
  import { portalStore, gradients, iconMap } from '../stores/portal.svelte.js';
  import Button from '../components/Button.svelte';
  import CustomFieldRenderer from '../features/items/CustomFieldRenderer.svelte';
  import Spinner from '../components/Spinner.svelte';
  import Textarea from '../components/Textarea.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import Label from '../components/Label.svelte';
  import PortalModal from './PortalModal.svelte';
  import { ChevronLeft, ChevronRight, Package, X, Check } from 'lucide-svelte';
  import BasePicker from '../pickers/BasePicker.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let {
    isOpen = $bindable(false),
    requestType = null,
    portalSlug = '',
    isDarkMode = false,
    onsubmitted = () => {},
    onclose = () => {}
  } = $props();

  // Direct store access (Svelte 5 reactive)

  let fields = $state([]);
  let customFieldDefinitions = $state([]);
  let loading = $state(false);
  let submitting = $state(false);
  let error = $state(null);
  let success = $state(false);

  // Multi-step support
  let steps = $state([1]);
  let currentStep = $state(1);

  // Form data
  let formData = $state({
    title: '',
    description: ''
  });
  let customFieldValues = $state({});

  // Computed: fields for current step
  let currentStepFields = $derived(fields.filter(f => (f.step_number || 1) === currentStep));
  let totalSteps = $derived(steps.length);
  let isLastStep = $derived(currentStep === Math.max(...steps));
  let isFirstStep = $derived(currentStep === Math.min(...steps));

  // Get gradient - use portal gradient or fallback
  let gradientValue = $derived(
    gradients[portalStore.selectedGradient]?.value ||
    gradients[0].value
  );

  // Load fields when modal opens
  $effect(() => {
    if (isOpen && requestType) {
      loadFields();
    }
  });

  // Clear form when modal closes
  $effect(() => {
    if (!isOpen) {
      clearForm();
    }
  });

  async function loadFields() {
    try {
      loading = true;
      error = null;
      success = false;

      // Load request type fields configuration
      fields = await api.requestTypes.getFields(requestType.id);

      // Calculate steps from field data
      const stepNumbers = [...new Set(fields.map(f => f.step_number || 1))].sort((a, b) => a - b);
      steps = stepNumbers.length > 0 ? stepNumbers : [1];
      currentStep = Math.min(...steps);

      // Load all custom field definitions for rendering
      const allCustomFields = await api.customFields.getAll();
      customFieldDefinitions = allCustomFields || [];

      // Initialize custom field values (for both custom and virtual fields)
      customFieldValues = {};
      fields.forEach(field => {
        if (field.field_type === 'custom' || field.field_type === 'virtual') {
          // For checkbox virtual fields, initialize to false
          if (field.field_type === 'virtual' && field.virtual_field_type === 'checkbox') {
            customFieldValues[field.field_identifier] = false;
          } else {
            customFieldValues[field.field_identifier] = '';
          }
        }
      });
    } catch (err) {
      console.error('Failed to load request type fields:', err);
      error = err.message || t('requestForm.failedToLoadFields');
    } finally {
      loading = false;
    }
  }

  function clearForm() {
    formData = {
      title: '',
      description: ''
    };
    customFieldValues = {};
    error = null;
    success = false;
    currentStep = 1;
  }

  function isFieldRequired(fieldIdentifier) {
    const field = fields.find(f => f.field_identifier === fieldIdentifier);
    return field ? field.is_required : false;
  }

  function hasField(fieldIdentifier) {
    return fields.some(f => f.field_identifier === fieldIdentifier);
  }

  function hasFieldInCurrentStep(fieldIdentifier) {
    return currentStepFields.some(f => f.field_identifier === fieldIdentifier);
  }

  function getCustomFieldDefinition(fieldId) {
    return customFieldDefinitions.find(f => f.id.toString() === fieldId);
  }

  function getFieldLabel(field) {
    return field.display_name || field.field_label || field.field_name || field.field_identifier;
  }

  function validateCurrentStep() {
    // Validate fields in current step
    for (const field of currentStepFields) {
      if (!field.is_required) continue;

      if (field.field_type === 'default') {
        if (field.field_identifier === 'title' && !formData.title.trim()) {
          error = `${getFieldLabel(field)} is required`;
          return false;
        }
        if (field.field_identifier === 'description' && !formData.description.trim()) {
          error = `${getFieldLabel(field)} is required`;
          return false;
        }
      } else if (field.field_type === 'custom') {
        const value = customFieldValues[field.field_identifier];
        if (value === undefined || value === null || value === '') {
          const fieldDef = getCustomFieldDefinition(field.field_identifier);
          error = `${field.display_name || fieldDef?.name || 'Field'} is required`;
          return false;
        }
      } else if (field.field_type === 'virtual') {
        const value = customFieldValues[field.field_identifier];
        // Checkbox fields with false are valid (only truly empty is invalid)
        if (field.virtual_field_type === 'checkbox') {
          // Checkbox is always valid (false is a valid value)
        } else if (value === undefined || value === null || value === '') {
          error = `${getFieldLabel(field)} is required`;
          return false;
        }
      }
    }

    return true;
  }

  function goToNextStep() {
    error = null;
    if (!validateCurrentStep()) return;

    const currentIndex = steps.indexOf(currentStep);
    if (currentIndex < steps.length - 1) {
      currentStep = steps[currentIndex + 1];
    }
  }

  function goToPrevStep() {
    error = null;
    const currentIndex = steps.indexOf(currentStep);
    if (currentIndex > 0) {
      currentStep = steps[currentIndex - 1];
    }
  }

  async function handleSubmit() {
    try {
      // Validate all steps
      for (const step of steps) {
        currentStep = step;
        if (!validateCurrentStep()) {
          return;
        }
      }

      // Reset to last step for UI consistency during submission
      currentStep = Math.max(...steps);

      submitting = true;
      error = null;

      // Submit to portal (user info comes from authenticated session)
      const submissionData = {
        request_type_id: requestType.id,
        title: formData.title,
        description: formData.description,
        custom_fields: customFieldValues
      };

      await api.portal.submit(portalSlug, submissionData);

      success = true;

      // Close modal after short delay
      setTimeout(() => {
        handleClose();
        onsubmitted();
      }, 1500);
    } catch (err) {
      console.error('Failed to submit request:', err);
      error = err.message || t('requestForm.failedToSubmit');
    } finally {
      submitting = false;
    }
  }

  function handleClose() {
    isOpen = false;
    onclose();
  }

  function parseSelectOptions(optionsJson) {
    try {
      return JSON.parse(optionsJson) || [];
    } catch {
      return [];
    }
  }
</script>

{#if isOpen && requestType}
  <PortalModal
    isOpen={isOpen}
    isDarkMode={isDarkMode}
    maxWidth="max-w-2xl"
    showHeader={false}
    bodyClass=""
    onClose={handleClose}
  >
    <!-- Gradient Header with Integrated Stepper -->
    <div
      class="px-8 pt-8 pb-6 text-white text-center relative"
      style="background: {gradientValue};"
    >
      <!-- Close button -->
      <button
        onclick={handleClose}
        class="absolute top-4 right-4 p-2 rounded-full hover:bg-white/20 transition-all"
        aria-label="Close"
      >
        <X class="w-5 h-5" />
      </button>

      <!-- Icon -->
      <div class="flex justify-center mb-3">
        <div class="w-14 h-14 rounded-full bg-white/20 backdrop-blur-sm flex items-center justify-center">
          <svelte:component this={iconMap[requestType?.icon] || Package} class="w-7 h-7 text-white" />
        </div>
      </div>

      <!-- Title -->
      <h2 class="text-xl font-bold">{requestType?.name}</h2>
      {#if requestType?.description}
        <p class="text-white/80 mt-1 text-sm">{requestType.description}</p>
      {/if}

      <!-- Integrated Stepper (white theme for gradient background) -->
      {#if totalSteps > 1}
        <div class="mt-6 flex items-center justify-center gap-2">
          {#each steps as step, index}
            {@const isCompleted = index + 1 < currentStep}
            {@const isCurrent = index + 1 === currentStep}
            <div class="flex items-center">
              <div
                class="w-8 h-8 rounded-full flex items-center justify-center text-sm font-medium transition-all"
                style="background: {isCurrent || isCompleted ? 'white' : 'rgba(255,255,255,0.3)'}; color: {isCurrent || isCompleted ? '#667eea' : 'white'};"
              >
                {#if isCompleted}
                  <Check class="w-4 h-4" />
                {:else}
                  {index + 1}
                {/if}
              </div>
              {#if index < steps.length - 1}
                <div
                  class="w-8 h-0.5 mx-1"
                  style="background: {isCompleted ? 'white' : 'rgba(255,255,255,0.3)'};"
                ></div>
              {/if}
            </div>
          {/each}
        </div>
      {/if}
    </div>

    <!-- Form Body -->
    {#if loading}
      <div class="flex items-center justify-center py-12">
        <Spinner />
      </div>
    {:else if success}
      <div class="px-6 py-4">
        <AlertBox variant="success" message={t('requestForm.requestSubmittedSuccess')} />
      </div>
    {:else}
      <div class="px-6 py-4 max-h-[50vh] overflow-y-auto">
        {#if error}
          <div
            class="mb-4 p-3 rounded border"
            style="background-color: {isDarkMode ? 'rgba(239, 68, 68, 0.1)' : '#fef2f2'}; border-color: {isDarkMode ? 'rgba(239, 68, 68, 0.3)' : '#fecaca'};"
          >
            <p class="text-sm" style="color: {isDarkMode ? '#fca5a5' : '#dc2626'};">
              {error}
            </p>
          </div>
        {/if}

        <div class="space-y-4">
        <!-- Default Fields -->
        {#if hasFieldInCurrentStep('title')}
          {@const titleField = currentStepFields.find(f => f.field_identifier === 'title')}
          <div>
            <Label for="request-title" required={titleField.is_required} class="mb-2">
              {titleField.display_name || t('requestForm.title')}
            </Label>
            <input
              id="request-title"
              bind:value={formData.title}
              type="text"
              class="w-full px-4 py-3 rounded border focus:outline-none focus:ring-2 focus:ring-blue-500"
              style="background-color: {isDarkMode ? '#1e293b' : '#ffffff'}; color: {isDarkMode ? '#e2e8f0' : '#111827'}; border-color: {isDarkMode ? '#475569' : '#d1d5db'};"
              placeholder={t('requestForm.enterTitle')}
              required={titleField.is_required}
            />
            {#if titleField.description}
              <p class="text-xs mt-1" style="color: {isDarkMode ? '#94a3b8' : '#6b7280'};">
                {titleField.description}
              </p>
            {/if}
          </div>
        {/if}

        {#if hasFieldInCurrentStep('description')}
          {@const descField = currentStepFields.find(f => f.field_identifier === 'description')}
          <div>
            <Label for="request-description" required={descField.is_required} class="mb-2">
              {descField.display_name || t('requestForm.description')}
            </Label>
            <Textarea
              id="request-description"
              bind:value={formData.description}
              rows={4}
              class="w-full"
              style="background-color: {isDarkMode ? '#1e293b' : '#ffffff'}; color: {isDarkMode ? '#e2e8f0' : '#111827'}; border-color: {isDarkMode ? '#475569' : '#d1d5db'};"
              placeholder={t('requestForm.describeRequest')}
              required={descField.is_required}
            />
            {#if descField.description}
              <p class="text-xs mt-1" style="color: {isDarkMode ? '#94a3b8' : '#6b7280'};">
                {descField.description}
              </p>
            {/if}
          </div>
        {/if}

        <!-- Custom Fields -->
        {#each currentStepFields.filter(f => f.field_type === 'custom') as field}
          {@const fieldDef = getCustomFieldDefinition(field.field_identifier)}
          {#if fieldDef}
            <div>
              {#if field.display_name || field.description}
                <Label required={field.is_required} class="mb-2">
                  {field.display_name || fieldDef.name}
                </Label>
                {#if field.description}
                  <p class="text-xs mb-2" style="color: {isDarkMode ? '#94a3b8' : '#6b7280'};">
                    {field.description}
                  </p>
                {/if}
                <CustomFieldRenderer
                  field={{ ...fieldDef, is_required: field.is_required, name: '' }}
                  bind:value={customFieldValues[field.field_identifier]}
                  readonly={false}
                  onChange={(val) => customFieldValues[field.field_identifier] = val}
                  milestones={[]}
                  {isDarkMode}
                />
              {:else}
                <CustomFieldRenderer
                  field={{ ...fieldDef, is_required: field.is_required }}
                  bind:value={customFieldValues[field.field_identifier]}
                  readonly={false}
                  onChange={(val) => customFieldValues[field.field_identifier] = val}
                  milestones={[]}
                  {isDarkMode}
                />
              {/if}
            </div>
          {/if}
        {/each}

        <!-- Virtual Fields -->
        {#each currentStepFields.filter(f => f.field_type === 'virtual') as field}
          <div>
            <Label required={field.is_required} class="mb-2">
              {getFieldLabel(field)}
            </Label>
            {#if field.description}
              <p class="text-xs mb-2" style="color: {isDarkMode ? '#94a3b8' : '#6b7280'};">
                {field.description}
              </p>
            {/if}

            {#if field.virtual_field_type === 'text'}
              <input
                type="text"
                bind:value={customFieldValues[field.field_identifier]}
                class="w-full px-4 py-3 rounded border focus:outline-none focus:ring-2 focus:ring-blue-500"
                style="background-color: {isDarkMode ? '#1e293b' : '#ffffff'}; color: {isDarkMode ? '#e2e8f0' : '#111827'}; border-color: {isDarkMode ? '#475569' : '#d1d5db'};"
                placeholder={field.display_name || field.field_name}
              />
            {:else if field.virtual_field_type === 'textarea'}
              <Textarea
                bind:value={customFieldValues[field.field_identifier]}
                rows={4}
                class="w-full"
                style="background-color: {isDarkMode ? '#1e293b' : '#ffffff'}; color: {isDarkMode ? '#e2e8f0' : '#111827'}; border-color: {isDarkMode ? '#475569' : '#d1d5db'};"
                placeholder={field.display_name || field.field_name}
              />
            {:else if field.virtual_field_type === 'select'}
              <BasePicker
                bind:value={customFieldValues[field.field_identifier]}
                items={parseSelectOptions(field.virtual_field_options)}
                placeholder={t('requestForm.selectOption')}
                showUnassigned={true}
                unassignedLabel={t('requestForm.selectOption')}
                getValue={(option) => option.value}
                getLabel={(option) => option.label}
              />
            {:else if field.virtual_field_type === 'checkbox'}
              <label class="flex items-center gap-3 cursor-pointer">
                <input
                  type="checkbox"
                  bind:checked={customFieldValues[field.field_identifier]}
                  class="h-5 w-5 rounded border-gray-300 focus:ring-2 focus:ring-blue-500"
                />
                <span class="text-sm" style="color: {isDarkMode ? '#e2e8f0' : '#374151'};">
                  {field.display_name || field.field_name}
                </span>
              </label>
            {/if}
          </div>
        {/each}

          <!-- Submitting as info (only on last step) -->
          {#if isLastStep}
            <div class="p-3 rounded border" style="background-color: {isDarkMode ? 'rgba(59, 130, 246, 0.1)' : '#eff6ff'}; border-color: {isDarkMode ? 'rgba(59, 130, 246, 0.3)' : '#bfdbfe'};">
              <p class="text-sm" style="color: {isDarkMode ? '#93c5fd' : '#1e40af'};">
                {#if authStore.isAuthenticated && authStore.currentUser}
                  {t('requestForm.submittingAs', { name: `${authStore.currentUser?.first_name} ${authStore.currentUser?.last_name}`, email: authStore.currentUser?.email })}
                {:else if portalAuthStore.isAuthenticated && portalAuthStore.customer}
                  {t('requestForm.submittingAs', { name: portalAuthStore.customer.name || t('portal.portalCustomer'), email: portalAuthStore.customer.email })}
                {/if}
              </p>
            </div>
          {/if}
        </div>
      </div>

      <!-- Footer with Navigation Buttons (fixed at bottom) -->
      <div
        class="px-6 py-4 border-t flex items-center justify-between"
        style="border-color: {isDarkMode ? '#475569' : '#e5e7eb'};"
      >
        <div>
          {#if !isFirstStep}
            <Button
              onclick={goToPrevStep}
              variant="default"
              size="medium"
              disabled={submitting}
            >
              <ChevronLeft class="w-4 h-4 mr-1" />
              {t('common.back')}
            </Button>
          {/if}
        </div>

        <div class="flex items-center gap-3">
          <Button
            onclick={handleClose}
            variant="default"
            size="medium"
            disabled={submitting}
          >
            {t('common.cancel')}
          </Button>
          {#if isLastStep}
            <Button
              onclick={handleSubmit}
              variant="primary"
              size="medium"
              disabled={submitting || loading}
            >
              {submitting ? t('requestForm.submitting') : t('requestForm.submitRequest')}
            </Button>
          {:else}
            <Button
              onclick={goToNextStep}
              variant="primary"
              size="medium"
            >
              {t('common.next')}
              <ChevronRight class="w-4 h-4 ml-1" />
            </Button>
          {/if}
        </div>
      </div>
    {/if}
  </PortalModal>
{/if}
