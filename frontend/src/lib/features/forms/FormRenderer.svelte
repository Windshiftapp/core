<script>
  import { api } from '../../api.js';
  import { t } from '../../stores/i18n.svelte.js';
  import CustomFieldRenderer from '../items/CustomFieldRenderer.svelte';
  import Spinner from '../../components/Spinner.svelte';
  import Label from '../../components/Label.svelte';

  let {
    formSlug = '',
    formId = null,
    formConfig = null,
    brandColor = '#14b8a6',
    isDarkMode = false,
    onSubmitted = () => {},
  } = $props();

  let submitButtonText = $derived(formConfig?.submit_button_text || 'Submit');

  let fields = $state([]);
  let customFieldDefinitions = $state([]);
  let loading = $state(true);
  let submitting = $state(false);
  let error = $state(null);
  let success = $state(false);
  let successMessage = $state('');
  let redirectUrl = $state('');

  // Multi-step
  let steps = $state([1]);
  let currentStep = $state(1);
  let currentStepFields = $derived(fields.filter(f => (f.step_number || 1) === currentStep));
  let totalSteps = $derived(steps.length);
  let isLastStep = $derived(currentStep === Math.max(...steps));
  let isFirstStep = $derived(currentStep === Math.min(...steps));

  // Form data
  let formData = $state({ title: '', description: '' });
  let customFieldValues = $state({});

  // Load fields when formId changes
  $effect(() => {
    if (formSlug && formId) {
      loadFields();
    }
  });

  async function loadFields() {
    try {
      loading = true;
      error = null;
      success = false;

      const [fieldsResult, cfResult] = await Promise.all([
        api.forms.getFormFields(formSlug, formId),
        api.forms.getCustomFields(formSlug),
      ]);

      fields = fieldsResult || [];
      customFieldDefinitions = cfResult || [];

      const stepNumbers = [...new Set(fields.map(f => f.step_number || 1))].sort((a, b) => a - b);
      steps = stepNumbers.length > 0 ? stepNumbers : [1];
      currentStep = Math.min(...steps);

      customFieldValues = {};
      fields.forEach(field => {
        if (field.field_type === 'custom' || field.field_type === 'virtual') {
          if (field.field_type === 'virtual' && field.virtual_field_type === 'checkbox') {
            customFieldValues[field.field_identifier] = false;
          } else {
            customFieldValues[field.field_identifier] = '';
          }
        }
      });
    } catch (err) {
      console.error('Failed to load form fields:', err);
      error = err.message || 'Failed to load form fields';
    } finally {
      loading = false;
    }
  }

  function getFieldLabel(field) {
    return field.display_name || field.field_label || field.field_name || field.field_identifier;
  }

  function getCustomFieldDefinition(fieldId) {
    return customFieldDefinitions.find(f => f.id.toString() === fieldId);
  }

  function hasFieldInCurrentStep(fieldIdentifier) {
    return currentStepFields.some(f => f.field_identifier === fieldIdentifier);
  }

  function validateCurrentStep() {
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
          error = `${getFieldLabel(field)} is required`;
          return false;
        }
      } else if (field.field_type === 'virtual') {
        const value = customFieldValues[field.field_identifier];
        if (field.virtual_field_type !== 'checkbox' && (value === undefined || value === null || value === '')) {
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

  function parseSelectOptions(optionsJson) {
    try {
      return JSON.parse(optionsJson) || [];
    } catch {
      return [];
    }
  }

  async function handleSubmit() {
    try {
      for (const step of steps) {
        currentStep = step;
        if (!validateCurrentStep()) return;
      }
      currentStep = Math.max(...steps);

      submitting = true;
      error = null;

      const submissionData = {
        request_type_id: formId,
        title: formData.title,
        description: formData.description,
        custom_fields: customFieldValues,
      };

      const result = await api.forms.submit(formSlug, submissionData);

      success = true;
      successMessage = result.success_message || 'Thank you for your submission!';
      redirectUrl = result.redirect_url || '';

      onSubmitted(result);

      if (redirectUrl) {
        setTimeout(() => {
          window.location.href = redirectUrl;
        }, 2000);
      }
    } catch (err) {
      console.error('Failed to submit form:', err);
      error = err.message || 'Failed to submit form';
    } finally {
      submitting = false;
    }
  }
</script>

{#if loading}
  <div class="flex items-center justify-center py-12">
    <Spinner />
  </div>
{:else if success}
  <div class="text-center py-12">
    <div class="w-16 h-16 mx-auto mb-4 rounded-full flex items-center justify-center" style="background-color: {brandColor}20;">
      <svg class="w-8 h-8" style="color: {brandColor};" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M20 6L9 17l-5-5" stroke-linecap="round" stroke-linejoin="round"/>
      </svg>
    </div>
    <h3 class="text-lg font-semibold mb-2" style="color: {isDarkMode ? '#e2e8f0' : '#111827'};">
      {successMessage}
    </h3>
    {#if redirectUrl}
      <p class="text-sm" style="color: {isDarkMode ? '#94a3b8' : '#6b7280'};">
        Redirecting...
      </p>
    {/if}
  </div>
{:else}
  <form onsubmit={(e) => { e.preventDefault(); isLastStep ? handleSubmit() : goToNextStep(); }}>
    {#if error}
      <div class="mb-4 p-3 rounded border" style="background-color: {isDarkMode ? '#1e1e2e' : '#fef2f2'}; border-color: {isDarkMode ? '#7f1d1d' : '#fecaca'};">
        <p class="text-sm" style="color: {isDarkMode ? '#fca5a5' : '#dc2626'};">{error}</p>
      </div>
    {/if}

    <!-- Step indicator -->
    {#if totalSteps > 1}
      <div class="flex items-center justify-center gap-2 mb-6">
        {#each steps as step, index}
          {@const isCompleted = step < currentStep}
          {@const isCurrent = step === currentStep}
          <div class="flex items-center">
            <div
              class="w-8 h-8 rounded-full flex items-center justify-center text-sm font-medium transition-all"
              style="background-color: {isCurrent || isCompleted ? brandColor : isDarkMode ? '#374151' : '#e5e7eb'}; color: {isCurrent || isCompleted ? 'white' : isDarkMode ? '#9ca3af' : '#6b7280'};"
            >
              {#if isCompleted}
                <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M20 6L9 17l-5-5" stroke-linecap="round" stroke-linejoin="round"/>
                </svg>
              {:else}
                {index + 1}
              {/if}
            </div>
            {#if index < steps.length - 1}
              <div class="w-8 h-0.5 mx-1" style="background-color: {isCompleted ? brandColor : isDarkMode ? '#374151' : '#e5e7eb'};"></div>
            {/if}
          </div>
        {/each}
      </div>
    {/if}

    <div class="space-y-4">
      <!-- Default Fields -->
      {#if hasFieldInCurrentStep('title')}
        {@const titleField = currentStepFields.find(f => f.field_identifier === 'title')}
        <div>
          <Label for="form-title" required={titleField.is_required} class="mb-2" color="default">
            {titleField.display_name || t('requestForm.title')}
          </Label>
          <input
            id="form-title"
            bind:value={formData.title}
            type="text"
            class="w-full px-4 py-3 rounded-lg border focus:outline-none focus:ring-2"
            style="background-color: {isDarkMode ? '#1e293b' : '#ffffff'}; color: {isDarkMode ? '#e2e8f0' : '#111827'}; border-color: {isDarkMode ? '#475569' : '#d1d5db'}; --tw-ring-color: {brandColor};"
            placeholder={t('requestForm.enterTitle')}
            required={titleField.is_required}
          />
          {#if titleField.description}
            <p class="text-xs mt-1" style="color: {isDarkMode ? '#94a3b8' : '#6b7280'};">{titleField.description}</p>
          {/if}
        </div>
      {/if}

      {#if hasFieldInCurrentStep('description')}
        {@const descField = currentStepFields.find(f => f.field_identifier === 'description')}
        <div>
          <Label for="form-description" required={descField.is_required} class="mb-2" color="default">
            {descField.display_name || t('requestForm.description')}
          </Label>
          <textarea
            id="form-description"
            bind:value={formData.description}
            rows="4"
            class="w-full px-4 py-3 rounded-lg border focus:outline-none focus:ring-2"
            style="background-color: {isDarkMode ? '#1e293b' : '#ffffff'}; color: {isDarkMode ? '#e2e8f0' : '#111827'}; border-color: {isDarkMode ? '#475569' : '#d1d5db'}; --tw-ring-color: {brandColor};"
            placeholder={t('requestForm.describeRequest')}
            required={descField.is_required}
          ></textarea>
          {#if descField.description}
            <p class="text-xs mt-1" style="color: {isDarkMode ? '#94a3b8' : '#6b7280'};">{descField.description}</p>
          {/if}
        </div>
      {/if}

      <!-- Custom Fields -->
      {#each currentStepFields.filter(f => f.field_type === 'custom') as field}
        {@const fieldDef = getCustomFieldDefinition(field.field_identifier)}
        {#if fieldDef}
          <div>
            <Label required={field.is_required} class="mb-2" color="default">
              {field.display_name || fieldDef.name}
            </Label>
            <CustomFieldRenderer
              field={fieldDef}
              value={customFieldValues[field.field_identifier]}
              onChange={(val) => { customFieldValues[field.field_identifier] = val; }}
              readonly={false}
              required={field.is_required}
              {isDarkMode}
            />
            {#if field.description}
              <p class="text-xs mt-1" style="color: {isDarkMode ? '#94a3b8' : '#6b7280'};">{field.description}</p>
            {/if}
          </div>
        {/if}
      {/each}

      <!-- Virtual Fields -->
      {#each currentStepFields.filter(f => f.field_type === 'virtual') as field}
        <div>
          <Label required={field.is_required} class="mb-2" color="default">
            {getFieldLabel(field)}
          </Label>
          {#if field.virtual_field_type === 'textarea'}
            <textarea
              bind:value={customFieldValues[field.field_identifier]}
              rows="3"
              class="w-full px-4 py-3 rounded-lg border focus:outline-none focus:ring-2"
              style="background-color: {isDarkMode ? '#1e293b' : '#ffffff'}; color: {isDarkMode ? '#e2e8f0' : '#111827'}; border-color: {isDarkMode ? '#475569' : '#d1d5db'}; --tw-ring-color: {brandColor};"
              required={field.is_required}
            ></textarea>
          {:else if field.virtual_field_type === 'select'}
            {@const options = parseSelectOptions(field.virtual_field_options)}
            <select
              bind:value={customFieldValues[field.field_identifier]}
              class="w-full px-4 py-3 rounded-lg border focus:outline-none focus:ring-2"
              style="background-color: {isDarkMode ? '#1e293b' : '#ffffff'}; color: {isDarkMode ? '#e2e8f0' : '#111827'}; border-color: {isDarkMode ? '#475569' : '#d1d5db'}; --tw-ring-color: {brandColor};"
              required={field.is_required}
            >
              <option value="">{t('requestForm.selectOption')}</option>
              {#each options as opt}
                <option value={opt.value}>{opt.label}</option>
              {/each}
            </select>
          {:else if field.virtual_field_type === 'checkbox'}
            <label class="flex items-center gap-3 cursor-pointer">
              <input
                type="checkbox"
                bind:checked={customFieldValues[field.field_identifier]}
                class="w-5 h-5 rounded"
                style="accent-color: {brandColor};"
              />
              <span class="text-sm" style="color: {isDarkMode ? '#e2e8f0' : '#374151'};">
                {getFieldLabel(field)}
              </span>
            </label>
          {:else}
            <input
              bind:value={customFieldValues[field.field_identifier]}
              type="text"
              class="w-full px-4 py-3 rounded-lg border focus:outline-none focus:ring-2"
              style="background-color: {isDarkMode ? '#1e293b' : '#ffffff'}; color: {isDarkMode ? '#e2e8f0' : '#111827'}; border-color: {isDarkMode ? '#475569' : '#d1d5db'}; --tw-ring-color: {brandColor};"
              required={field.is_required}
            />
          {/if}
          {#if field.description}
            <p class="text-xs mt-1" style="color: {isDarkMode ? '#94a3b8' : '#6b7280'};">{field.description}</p>
          {/if}
        </div>
      {/each}
    </div>

    <!-- Navigation / Submit -->
    <div class="flex items-center justify-between mt-6 pt-4 border-t" style="border-color: {isDarkMode ? '#374151' : '#e5e7eb'};">
      {#if !isFirstStep}
        <button
          type="button"
          onclick={goToPrevStep}
          class="px-4 py-2 text-sm font-medium rounded-lg border transition-colors"
          style="border-color: {isDarkMode ? '#475569' : '#d1d5db'}; color: {isDarkMode ? '#e2e8f0' : '#374151'};"
        >
          Back
        </button>
      {:else}
        <div></div>
      {/if}

      <button
        type="submit"
        disabled={submitting}
        class="px-6 py-2.5 text-sm font-medium text-white rounded-lg transition-colors disabled:opacity-50"
        style="background-color: {brandColor};"
      >
        {#if submitting}
          Submitting...
        {:else if isLastStep}
          {submitButtonText}
        {:else}
          Next
        {/if}
      </button>
    </div>
  </form>
{/if}
