<script>
  import { api } from '../api.js';
  import { authStore } from '../stores';
  import { portalAuthStore } from '../stores/portalAuth.svelte.js';
  import { portalCustomizationStore as portalStore } from '../stores/portal.svelte.js';
  import { iconMap } from '../stores/portalPresentation.js';
  import Button from '../components/Button.svelte';
  import Spinner from '../components/Spinner.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import PortalModal from './PortalModal.svelte';
  import { ChevronLeft, ChevronRight, Package, X } from '@lucide/svelte';
  import { t } from '../stores/i18n.svelte.js';
  import FormFields from '../features/forms/FormFields.svelte';
  import {
    buildFormSteps,
    clampFormStep,
    initializeFormValues,
    validateFormStep,
  } from '../features/forms/formModel.js';

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

  // Draft state — only used on the portal path (portalSlug truthy).
  // resumedDraft tracks the most recently loaded draft so we can show the
  // "Resuming draft" banner; cleared once the user opts to start fresh.
  // savingDraft is shown briefly while the auto-save fetch is in flight so
  // users get feedback that progress is persisted.
  let resumedDraft = $state(null);
  let savingDraft = $state(false);
  let draftJustSaved = $state(false);
  let draftSavedTimer = null;

  let totalSteps = $derived(steps.length);
  let isLastStep = $derived(currentStep === Math.max(...steps));
  let isFirstStep = $derived(currentStep === Math.min(...steps));
  let hasPortalVisual = $derived(portalStore.hasBackgroundImage || portalStore.hasGradient);


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

  // Cancel any pending "Draft saved" indicator timer on component teardown
  // so we don't write to state after unmount.
  $effect(() => {
    return () => {
      if (draftSavedTimer) {
        clearTimeout(draftSavedTimer);
        draftSavedTimer = null;
      }
    };
  });

  async function loadFields() {
    try {
      loading = true;
      error = null;
      success = false;

      // Load request type fields configuration
      // Use portal API if on portal, otherwise use internal API
      if (portalSlug) {
        fields = await api.portal.getRequestTypeFields(portalSlug, requestType.id);
      } else {
        fields = await api.requestTypes.getFields(requestType.id);
      }

      // Calculate steps from field data
      steps = buildFormSteps(fields);
      currentStep = steps[0];

      // Load custom field definitions for rendering
      // Use portal API if on portal (only returns fields used by this portal)
      // Otherwise use internal API (returns all fields)
      if (portalSlug) {
        customFieldDefinitions = await api.portal.getCustomFields(portalSlug) || [];
      } else {
        customFieldDefinitions = await api.customFields.getAll();
      }

      const initialValues = initializeFormValues(fields, null, customFieldDefinitions);
      formData = initialValues.formData;
      customFieldValues = initialValues.customFieldValues;

      // Auto-resume any saved draft for this request type. Only the portal
      // path has drafts — internal usage of this modal opens fresh.
      if (portalSlug) {
        await applyDraftIfPresent();
      }
    } catch (err) {
      console.error('Failed to load request type fields:', err);
      error = err.message || t('requestForm.failedToLoadFields');
    } finally {
      loading = false;
    }
  }

  async function applyDraftIfPresent() {
    try {
      const draft = await api.portal.drafts.getForRequestType(portalSlug, requestType.id);
      if (!draft) {
        resumedDraft = null;
        return;
      }
      const restored = initializeFormValues(fields, {
        title: draft.title,
        description: draft.description,
        custom_fields: draft.custom_field_values,
      }, customFieldDefinitions);
      formData = restored.formData;
      customFieldValues = restored.customFieldValues;
      currentStep = clampFormStep(steps, draft.current_step);
      resumedDraft = draft;
    } catch (err) {
      // A missing draft already returns null; anything that reaches here is a
      // real failure. Log and continue with the empty form — losing the
      // resume is far better than blocking submission.
      console.warn('Failed to load draft for resume:', err);
      resumedDraft = null;
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
    resumedDraft = null;
    savingDraft = false;
    draftJustSaved = false;
  }

  function buildDraftPayload() {
    return {
      request_type_id: requestType.id,
      title: formData.title || '',
      description: formData.description || '',
      custom_fields: customFieldValues,
      current_step: currentStep
    };
  }

  // Persist the current form state. Called when the user advances a step and
  // immediately before submission attempts. Fire-and-forget: the form does
  // not block on it. Returns a promise so callers may await if they want to
  // (e.g. before navigating away).
  async function saveDraft() {
    if (!portalSlug || !requestType) return;
    savingDraft = true;
    draftJustSaved = false;
    try {
      const saved = await api.portal.drafts.save(portalSlug, buildDraftPayload());
      if (saved) {
        resumedDraft = saved;
        draftJustSaved = true;
        if (draftSavedTimer) clearTimeout(draftSavedTimer);
        draftSavedTimer = setTimeout(() => {
          draftJustSaved = false;
          draftSavedTimer = null;
        }, 1500);
      }
    } catch (err) {
      console.warn('Failed to save draft:', err);
    } finally {
      savingDraft = false;
    }
  }

  async function startFreshFromDraft() {
    if (!portalSlug || !requestType) return;
    try {
      await api.portal.drafts.delete(portalSlug, requestType.id);
    } catch (err) {
      // 404 (no draft to delete) is fine — the user is starting fresh anyway.
      if (err?.status !== 404) {
        console.warn('Failed to delete draft:', err);
      }
    }
    // Reset form state to a pristine first-step view, but keep the loaded
    // fields metadata (no need to re-fetch).
    const reset = initializeFormValues(fields, null, customFieldDefinitions);
    formData = reset.formData;
    customFieldValues = reset.customFieldValues;
    currentStep = steps[0] || 1;
    resumedDraft = null;
    error = null;
  }

  function validateCurrentStep() {
    const message = validateFormStep({
      fields,
      step: currentStep,
      formData,
      customFieldValues,
      customFieldDefinitions,
      requiredMessage: (label) => t('requestForm.fieldRequired', { field: label }),
    });
    error = message || null;
    return !message;
  }

  function goToNextStep() {
    error = null;
    if (!validateCurrentStep()) return;

    const currentIndex = steps.indexOf(currentStep);
    if (currentIndex < steps.length - 1) {
      currentStep = steps[currentIndex + 1];
    }
    // Persist after advancing so the new currentStep is what we resume to.
    // Fire-and-forget — navigation isn't blocked on this.
    if (portalSlug) {
      saveDraft();
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

      // Persist the latest state once more before submitting. If the submit
      // itself fails (network, validation, rate-limit), the draft still
      // reflects what the user just typed on the final step.
      if (portalSlug) {
        await saveDraft();
      }

      // Submit to portal (user info comes from authenticated session)
      const submissionData = {
        request_type_id: requestType.id,
        title: formData.title,
        description: formData.description,
        custom_fields: customFieldValues
      };

      const result = await api.portal.submit(portalSlug, submissionData);

      success = true;

      // Server-side SubmitToPortal already drops the draft on success, but
      // call delete here too so callers polling drafts.list right after
      // submission see a consistent view without depending on backend
      // ordering. Best-effort.
      if (portalSlug) {
        api.portal.drafts.delete(portalSlug, requestType.id).catch((err) => {
          if (err?.status !== 404) {
            console.warn('Failed to delete draft after submit:', err);
          }
        });
      }

      // Close modal after short delay
      setTimeout(() => {
        handleClose();
        onsubmitted(result.item_id);
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
    <!-- Compact task header -->
    {@const RequestTypeIcon = iconMap[requestType?.icon] || Package}
    <div
      class="px-5 sm:px-6 py-5 border-b flex items-start gap-3 relative"
      style="{hasPortalVisual
        ? portalStore.headerBackgroundStyle
        : 'background-color: var(--ds-surface-card);'} border-color: {hasPortalVisual
        ? 'rgba(255,255,255,0.18)'
        : 'var(--ds-border)'};"
    >
      <div
        class="w-9 h-9 rounded-md flex items-center justify-center flex-none"
        style="background-color: {hasPortalVisual ? 'rgba(255,255,255,0.14)' : 'var(--ds-background-neutral)'}; color: {hasPortalVisual ? '#ffffff' : 'var(--ds-text-subtle)'};"
      >
        <RequestTypeIcon class="w-[18px] h-[18px]" />
      </div>
      <div class="min-w-0 flex-1 pr-8">
        <h2 class="text-lg font-semibold leading-6" style="color: {hasPortalVisual ? '#ffffff' : 'var(--ds-text)'};">{requestType?.name}</h2>
        {#if requestType?.description}
          <p class="mt-1 text-sm leading-5" style="color: {hasPortalVisual ? 'rgba(255,255,255,0.82)' : 'var(--ds-text-subtle)'};">{requestType.description}</p>
        {/if}
        {#if totalSteps > 1}
          <div class="mt-3 flex items-center gap-3">
            <span class="text-xs font-medium" style="color: {hasPortalVisual ? 'rgba(255,255,255,0.82)' : 'var(--ds-text-subtle)'};">
              Step {steps.indexOf(currentStep) + 1} of {totalSteps}
            </span>
            <div class="h-1 flex-1 max-w-32 rounded-full overflow-hidden" style="background-color: {hasPortalVisual ? 'rgba(255,255,255,0.28)' : 'var(--ds-background-neutral)'};">
              <div
                class="h-full rounded-full transition-all"
                style="width: {((steps.indexOf(currentStep) + 1) / totalSteps) * 100}%; background-color: {hasPortalVisual ? '#ffffff' : 'var(--ds-interactive, #2563eb)'};"
              ></div>
            </div>
          </div>
        {/if}
      </div>
      <button
        onclick={handleClose}
        class="absolute top-4 right-4 p-1.5 rounded-md transition-colors"
        style="color: {hasPortalVisual ? 'rgba(255,255,255,0.88)' : 'var(--ds-text-subtle)'};"
        aria-label="Close"
      >
        <X class="w-5 h-5" />
      </button>
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
      <div class="px-5 sm:px-6 py-5 sm:py-6 max-h-[60vh] overflow-y-auto">
        {#if error}
          <AlertBox variant="error" message={error} class="mb-4" />
        {/if}

        {#if resumedDraft && portalSlug}
          <div
            data-testid="request-form-draft-resume-banner"
            class="mb-5 pb-4 border-b flex items-center justify-between gap-3"
            style="border-color: var(--ds-border);"
          >
            <p class="text-sm" style="color: var(--ds-text-subtle);">
              {t('portal.draftResumeBanner')}
            </p>
            <button
              type="button"
              onclick={startFreshFromDraft}
              class="text-sm font-medium hover:underline whitespace-nowrap"
              style="color: var(--ds-text-link);"
            >
              {t('portal.draftStartFresh')}
            </button>
          </div>
        {/if}

        <div class="space-y-4">
          <FormFields
            {fields}
            {customFieldDefinitions}
            {currentStep}
            bind:formData
            bind:customFieldValues
            {isDarkMode}
            idPrefix="request"
          />

          <!-- Submitting as info (only on last step, only when we know who).
               portalAuthStore has two authenticated shapes: an internal user
               (signed into the main app and using the portal) populates `user`
               with customer null; a portal customer populates `customer` with
               user null. Handle both, plus the standalone authStore. -->
          {#if isLastStep && ((authStore.isAuthenticated && authStore.currentUser) || ($portalAuthStore.isAuthenticated && ($portalAuthStore.user || $portalAuthStore.customer)))}
            <div class="pt-4 border-t" style="border-color: var(--ds-border);">
              <p class="text-xs" style="color: var(--ds-text-subtle);">
                {#if authStore.isAuthenticated && authStore.currentUser}
                  {t('requestForm.submittingAs', { name: `${authStore.currentUser?.first_name} ${authStore.currentUser?.last_name}`, email: authStore.currentUser?.email })}
                {:else if $portalAuthStore.isAuthenticated && $portalAuthStore.user}
                  {t('requestForm.submittingAs', { name: $portalAuthStore.user.name || `${$portalAuthStore.user.first_name ?? ''} ${$portalAuthStore.user.last_name ?? ''}`.trim(), email: $portalAuthStore.user.email })}
                {:else if $portalAuthStore.isAuthenticated && $portalAuthStore.customer}
                  {t('requestForm.submittingAs', { name: $portalAuthStore.customer.name || t('portal.portalCustomer'), email: $portalAuthStore.customer.email })}
                {/if}
              </p>
            </div>
          {/if}
        </div>
      </div>

      <!-- Footer with Navigation Buttons (fixed at bottom) -->
      <div
        class="px-5 sm:px-6 py-4 border-t flex items-center justify-between gap-3"
        style="border-color: {isDarkMode ? '#475569' : '#e5e7eb'};"
      >
        <div>
          {#if !isFirstStep}
            <Button
              dataTestid="request-form-back-step"
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
          {#if portalSlug && (savingDraft || draftJustSaved)}
            <span class="text-xs whitespace-nowrap" style="color: {isDarkMode ? '#94a3b8' : '#6b7280'};">
              {savingDraft ? t('portal.draftSaving') : t('portal.draftSaved')}
            </span>
          {/if}
          <div class="hidden sm:block">
            <Button
              onclick={handleClose}
              variant="default"
              size="medium"
              disabled={submitting}
            >
              {t('common.cancel')}
            </Button>
          </div>
          {#if isLastStep}
            <Button
              dataTestid="request-form-submit"
              onclick={handleSubmit}
              variant="primary"
              size="medium"
              disabled={submitting || loading}
            >
              {submitting ? t('requestForm.submitting') : t('requestForm.submitRequest')}
            </Button>
          {:else}
            <Button
              dataTestid="request-form-next-step"
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
