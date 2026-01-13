<script>
  import { onMount, createEventDispatcher } from 'svelte';
  import { api } from '../api.js';
  import { authStore } from '../stores';
  import Button from '../components/Button.svelte';
  import CustomFieldRenderer from '../features/items/CustomFieldRenderer.svelte';
  import Spinner from '../components/Spinner.svelte';
  import Textarea from '../components/Textarea.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import PortalModal from './PortalModal.svelte';
  import { ChevronLeft, ChevronRight } from 'lucide-svelte';
  import BasePicker from '../pickers/BasePicker.svelte';

  const dispatch = createEventDispatcher();

  export let isOpen = false;
  export let requestType = null;
  export let portalSlug = '';
  export let isDarkMode = false;

  let fields = [];
  let customFieldDefinitions = [];
  let loading = false;
  let submitting = false;
  let error = null;
  let success = false;

  // Multi-step support
  let steps = [1];
  let currentStep = 1;

  // Form data
  let formData = {
    title: '',
    description: '',
    name: '',
    email: ''
  };
  let customFieldValues = {};

  // Computed: fields for current step
  $: currentStepFields = fields.filter(f => (f.step_number || 1) === currentStep);
  $: totalSteps = steps.length;
  $: isLastStep = currentStep === Math.max(...steps);
  $: isFirstStep = currentStep === Math.min(...steps);

  // Load fields when modal opens
  $: if (isOpen && requestType) {
    loadFields();
  }

  // Clear form when modal closes
  $: if (!isOpen) {
    clearForm();
  }

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
      error = err.message || 'Failed to load form fields';
    } finally {
      loading = false;
    }
  }

  function clearForm() {
    formData = {
      title: '',
      description: '',
      name: '',
      email: ''
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

      // Validate name and email only for anonymous users
      if (!authStore.isAuthenticated) {
        if (!formData.name.trim()) {
          error = 'Name is required';
          return;
        }
        if (!formData.email.trim()) {
          error = 'Email is required';
          return;
        }
      }

      submitting = true;
      error = null;

      // Submit to portal
      const submissionData = {
        request_type_id: requestType.id,
        title: formData.title,
        description: formData.description,
        custom_fields: customFieldValues
      };

      // Only include name and email for anonymous users
      if (!authStore.isAuthenticated) {
        submissionData.name = formData.name;
        submissionData.email = formData.email;
      }

      await api.portal.submit(portalSlug, submissionData);

      success = true;

      // Close modal after short delay
      setTimeout(() => {
        handleClose();
        dispatch('submitted');
      }, 1500);
    } catch (err) {
      console.error('Failed to submit request:', err);
      error = err.message || 'Failed to submit request';
    } finally {
      submitting = false;
    }
  }

  function handleClose() {
    isOpen = false;
    dispatch('close');
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
    title={requestType?.name}
    subtitle={requestType?.description}
    onClose={handleClose}
    bodyClass="px-6 py-4 max-h-[60vh] overflow-y-auto"
  >
    {#if loading}
      <div class="flex items-center justify-center py-12">
        <Spinner />
      </div>
    {:else if success}
      <div class="mb-4">
        <AlertBox variant="success" message="Request submitted successfully! We'll get back to you soon." />
      </div>
    {:else}
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

      <!-- Step Indicator (only show if multi-step) -->
      {#if totalSteps > 1}
        <div class="flex items-center justify-center gap-2 mb-6">
          {#each steps as step, index}
            <div class="flex items-center">
              <div
                class="w-8 h-8 rounded-full flex items-center justify-center text-sm font-medium transition-all"
                style="background-color: {currentStep === step ? '#3b82f6' : currentStep > step ? '#22c55e' : (isDarkMode ? '#475569' : '#e5e7eb')}; color: {currentStep >= step ? '#ffffff' : (isDarkMode ? '#94a3b8' : '#6b7280')};"
              >
                {step}
              </div>
              {#if index < steps.length - 1}
                <div
                  class="w-8 h-0.5 mx-1"
                  style="background-color: {currentStep > step ? '#22c55e' : (isDarkMode ? '#475569' : '#e5e7eb')};"
                ></div>
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
            <label for="request-title" class="block text-sm font-medium mb-2" style="color: {isDarkMode ? '#9ca3af' : '#374151'};">
              {getFieldLabel(titleField)}
              {#if titleField.is_required}<span class="text-red-500">*</span>{/if}
            </label>
            <input
              id="request-title"
              bind:value={formData.title}
              type="text"
              class="w-full px-4 py-3 rounded border focus:outline-none focus:ring-2 focus:ring-blue-500"
              style="background-color: {isDarkMode ? '#1e293b' : '#ffffff'}; color: {isDarkMode ? '#e2e8f0' : '#111827'}; border-color: {isDarkMode ? '#475569' : '#d1d5db'};"
              placeholder="Enter a title for your request"
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
            <label for="request-description" class="block text-sm font-medium mb-2" style="color: {isDarkMode ? '#9ca3af' : '#374151'};">
              {getFieldLabel(descField)}
              {#if descField.is_required}<span class="text-red-500">*</span>{/if}
            </label>
            <Textarea
              id="request-description"
              bind:value={formData.description}
              rows={4}
              class="w-full"
              style="background-color: {isDarkMode ? '#1e293b' : '#ffffff'}; color: {isDarkMode ? '#e2e8f0' : '#111827'}; border-color: {isDarkMode ? '#475569' : '#d1d5db'};"
              placeholder="Please describe your request"
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
                <label class="block text-sm font-medium mb-2" style="color: {isDarkMode ? '#9ca3af' : '#374151'};">
                  {field.display_name || fieldDef.name}
                  {#if field.is_required}<span class="text-red-500">*</span>{/if}
                </label>
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
            <label class="block text-sm font-medium mb-2" style="color: {isDarkMode ? '#9ca3af' : '#374151'};">
              {getFieldLabel(field)}
              {#if field.is_required}<span class="text-red-500">*</span>{/if}
            </label>
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
                placeholder="Select an option..."
                showUnassigned={true}
                unassignedLabel="Select an option..."
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

        <!-- Name/Email for anonymous users (only on last step) -->
        {#if isLastStep && !authStore.isAuthenticated}
          <div>
            <label for="request-name" class="block text-sm font-medium mb-2" style="color: {isDarkMode ? '#9ca3af' : '#374151'};">
              Name
              <span class="text-red-500">*</span>
            </label>
            <input
              id="request-name"
              bind:value={formData.name}
              type="text"
              class="w-full px-4 py-3 rounded border focus:outline-none focus:ring-2 focus:ring-blue-500"
              style="background-color: {isDarkMode ? '#1e293b' : '#ffffff'}; color: {isDarkMode ? '#e2e8f0' : '#111827'}; border-color: {isDarkMode ? '#475569' : '#d1d5db'};"
              placeholder="Your name"
              required
            />
          </div>

          <div>
            <label for="request-email" class="block text-sm font-medium mb-2" style="color: {isDarkMode ? '#9ca3af' : '#374151'};">
              Email
              <span class="text-red-500">*</span>
            </label>
            <input
              id="request-email"
              bind:value={formData.email}
              type="email"
              class="w-full px-4 py-3 rounded border focus:outline-none focus:ring-2 focus:ring-blue-500"
              style="background-color: {isDarkMode ? '#1e293b' : '#ffffff'}; color: {isDarkMode ? '#e2e8f0' : '#111827'}; border-color: {isDarkMode ? '#475569' : '#d1d5db'};"
              placeholder="your@email.com"
              required
            />
            <p class="text-xs mt-1" style="color: {isDarkMode ? '#94a3b8' : '#6b7280'};">
              We'll use this to follow up on your request
            </p>
          </div>
        {:else if isLastStep && authStore.isAuthenticated}
          <div class="p-3 rounded border" style="background-color: {isDarkMode ? 'rgba(59, 130, 246, 0.1)' : '#eff6ff'}; border-color: {isDarkMode ? 'rgba(59, 130, 246, 0.3)' : '#bfdbfe'};">
            <p class="text-sm" style="color: {isDarkMode ? '#93c5fd' : '#1e40af'};">
              Submitting as {authStore.currentUser?.first_name} {authStore.currentUser?.last_name} ({authStore.currentUser?.email})
            </p>
          </div>
        {/if}
      </div>

      <!-- Navigation / Submit Buttons -->
      <div class="flex items-center justify-between gap-3 pt-4">
        <div>
          {#if !isFirstStep}
            <Button
              onclick={goToPrevStep}
              variant="default"
              size="medium"
              disabled={submitting}
            >
              <ChevronLeft class="w-4 h-4 mr-1" />
              Back
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
            Cancel
          </Button>
          {#if isLastStep}
            <Button
              onclick={handleSubmit}
              variant="primary"
              size="medium"
              disabled={submitting || loading}
            >
              {submitting ? 'Submitting...' : 'Submit Request'}
            </Button>
          {:else}
            <Button
              onclick={goToNextStep}
              variant="primary"
              size="medium"
            >
              Next
              <ChevronRight class="w-4 h-4 ml-1" />
            </Button>
          {/if}
        </div>
      </div>
    {/if}
  </PortalModal>
{/if}
