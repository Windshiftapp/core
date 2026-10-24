<script>
  import { portal } from '../api/portal.js';
  import { requestTypes } from '../api/channels.js';
  import { customFields } from '../api/configuration.js';
  import StateDisplay from '../components/StateDisplay.svelte';
  import { authStore } from '../stores/auth.svelte.js';
  import { portalAuthStore } from '../stores/portalAuth.svelte.js';
  import { portalCustomizationStore as portalStore } from '../stores/portal.svelte.js';
  import { iconMap } from '../stores/portalPresentation.js';
  import Button from '../components/Button.svelte';
  import Checkbox from '../components/Checkbox.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import PortalModal from './PortalModal.svelte';
  import ChevronLeft from '@lucide/svelte/icons/chevron-left';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import Package from '@lucide/svelte/icons/package';
  import Paperclip from '@lucide/svelte/icons/paperclip';
  import X from '@lucide/svelte/icons/x';
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
    // Field values to seed on open, keyed by field_identifier (e.g. the
    // clicked asset id for an asset field). Applied after any draft resume so
    // the prefill wins for its target field while other draft values remain.
    prefill = {},
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

  // Organisation sharing (WI-1139): the org decides whether contacts may opt
  // in, and the creator's choice is fixed at submission. automatic shares
  // every request, disabled never shares.
  let shareWithOrganisation = $state(false);
  const orgSharingMode = $derived($portalAuthStore.userBootstrap?.request_sharing || 'disabled');
  const canShareWithOrg = $derived(orgSharingMode === 'requester_choice');

  // Multi-step support
  let steps = $state([1]);
  let currentStep = $state(1);

  // Form data
  let formData = $state({
    title: '',
    description: ''
  });
  let customFieldValues = $state({});

  // Show the resume banner only for a draft loaded when the portal form opens.
  let resumedDraft = $state(null);
  let savingDraft = $state(false);
  let draftJustSaved = $state(false);
  let draftSavedTimer = null;

  let totalSteps = $derived(steps.length);
  let isLastStep = $derived(currentStep === Math.max(...steps));
  let isFirstStep = $derived(currentStep === Math.min(...steps));
  let hasPortalVisual = $derived(portalStore.hasBackgroundImage || portalStore.hasGradient);

  // Files staged for upload once the request exists. Drafts store form data
  // only, so staged files intentionally do not survive close-and-reopen.
  let stagedFiles = $state([]);
  let uploadingFiles = $state(false);
  let uploadFailures = $state([]);


  // Load fields when modal opens
  $effect(() => {
    // Read prefill here so a URL-driven change re-seeds the form.
    const prefillValues = prefill;
    if (isOpen && requestType) {
      loadFields(prefillValues);
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

  async function loadFields(prefillValues = {}) {
    try {
      loading = true;
      error = null;
      success = false;

      // Load request type fields configuration
      // Use portal API if on portal, otherwise use internal API
      if (portalSlug) {
        fields = await portal.getRequestTypeFields(portalSlug, requestType.id);
      } else {
        fields = await requestTypes.getFields(requestType.id);
      }

      // Calculate steps from field data
      steps = buildFormSteps(fields);
      currentStep = steps[0];

      // Load custom field definitions for rendering
      // Use portal API if on portal (only returns fields used by this portal)
      // Otherwise use internal API (returns all fields)
      if (portalSlug) {
        customFieldDefinitions = await portal.getCustomFields(portalSlug) || [];
      } else {
        customFieldDefinitions = await customFields.getAll();
      }

      const initialValues = initializeFormValues(fields, null, customFieldDefinitions);
      formData = initialValues.formData;
      customFieldValues = initialValues.customFieldValues;

      // Auto-resume any saved draft for this request type. Only the portal
      // path has drafts — internal usage of this modal opens fresh.
      if (portalSlug) {
        await applyDraftIfPresent();
      }

      // Apply prefill last so it wins over a resumed draft for its target
      // field. Fields the form does not configure are ignored here and would
      // be dropped by the submit API anyway.
      applyPrefill(prefillValues);
    } catch (err) {
      console.error('Failed to load request type fields:', err);
      error = err.message || t('requestForm.failedToLoadFields');
    } finally {
      loading = false;
    }
  }

  async function applyDraftIfPresent() {
    try {
      const draft = await portal.drafts.getForRequestType(portalSlug, requestType.id);
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

  // Seed fields configured in the request type whose field_identifier is
  // present in the prefill map. Unknown keys are ignored.
  function applyPrefill(prefillValues) {
    if (!prefillValues || typeof prefillValues !== 'object') return;
    for (const field of fields) {
      const identifier = field.field_identifier;
      if (!identifier || prefillValues[identifier] === undefined) continue;
      const value = prefillValues[identifier];
      if (field.field_type === 'default') {
        formData = { ...formData, [identifier]: value };
      } else if (field.field_type === 'custom' || field.field_type === 'virtual') {
        customFieldValues = { ...customFieldValues, [identifier]: value };
      }
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
    stagedFiles = [];
    uploadingFiles = false;
    uploadFailures = [];
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
      const saved = await portal.drafts.save(portalSlug, buildDraftPayload());
      if (saved) {
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
      await portal.drafts.delete(portalSlug, requestType.id);
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
    applyPrefill(prefill);
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
        custom_fields: customFieldValues,
        share_with_organisation: canShareWithOrg && shareWithOrganisation
      };

      const result = await portal.submit(portalSlug, submissionData);

      // Upload staged files to the created request. The request already
      // exists at this point, so per-file failures are reported but never
      // fail the submission — the customer can retry from the timeline.
      if (stagedFiles.length > 0) {
        uploadingFiles = true;
        const failures = [];
        for (const file of stagedFiles) {
          try {
            await portal.addRequestAttachment(portalSlug, result.item_id, file);
          } catch (err) {
            console.error('Failed to upload attachment after submit:', err);
            failures.push(file.name);
          }
        }
        uploadFailures = failures;
        uploadingFiles = false;
      }

      success = true;

      // Server-side SubmitToPortal already drops the draft on success, but
      // call delete here too so callers polling drafts.list right after
      // submission see a consistent view without depending on backend
      // ordering. Best-effort.
      if (portalSlug) {
        portal.drafts.delete(portalSlug, requestType.id).catch((err) => {
          if (err?.status !== 404) {
            console.warn('Failed to delete draft after submit:', err);
          }
        });
      }

      // Close modal after short delay. Upload failures keep it open so the
      // customer can read the retry hint before the timeline takes over.
      setTimeout(
        () => {
          handleClose();
          onsubmitted(result.item_id);
        },
        uploadFailures.length > 0 ? 6000 : 1500
      );
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
      <StateDisplay type="loading" />
    {:else if success}
      <div class="px-6 py-4">
        <AlertBox variant="success" message={t('requestForm.requestSubmittedSuccess')} />
        {#if uploadFailures.length > 0}
          <AlertBox
            variant="warning"
            message="Some attachments failed to upload. You can attach them from the request timeline."
            class="mt-3"
          />
        {/if}
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

          <!-- Staged attachments (last step only). Uploads happen after the
               request is created; per-file failures surface on the success
               screen and can be retried from the request timeline. -->
          {#if isLastStep}
            <div class="pt-4" data-testid="request-form-attachments">
              <label
                class="inline-flex items-center gap-1.5 text-sm cursor-pointer hover:underline"
                style="color: var(--ds-text-link);"
                for="request-form-attachment-input"
              >
                <Paperclip class="w-4 h-4" aria-hidden="true" />
                Attach files
              </label>
              <input
                id="request-form-attachment-input"
                type="file"
                class="hidden"
                multiple
                onchange={(event) => {
                  const input = event.currentTarget;
                  stagedFiles = [...stagedFiles, ...Array.from(input.files ?? [])];
                  input.value = '';
                }}
              />
              {#if stagedFiles.length > 0}
                <ul class="mt-2 space-y-1">
                  {#each stagedFiles as file, index}
                    <li class="flex items-center gap-2 text-xs" style="color: var(--ds-text-subtle);">
                      <span class="truncate max-w-60">{file.name}</span>
                      <button
                        type="button"
                        class="hover:underline"
                        style="color: var(--ds-text-link);"
                        onclick={() => (stagedFiles = stagedFiles.filter((_, i) => i !== index))}
                      >
                        Remove
                      </button>
                    </li>
                  {/each}
                </ul>
              {/if}
            </div>
          {/if}

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

          {#if isLastStep && canShareWithOrg}
            <div class="pt-4 border-t" style="border-color: var(--ds-border);">
              <Checkbox
                bind:checked={shareWithOrganisation}
                dataTestid="request-form-share-with-organisation"
                label={t('requestForm.shareWithOrganisation')}
                hint={t('requestForm.shareWithOrganisationHint')}
                size="small"
              />
            </div>
          {:else if isLastStep && orgSharingMode === 'automatic'}
            <div class="pt-4 border-t" style="border-color: var(--ds-border);">
              <p class="text-xs" style="color: var(--ds-text-subtle);">
                {t('requestForm.sharedWithOrganisationNote')}
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
              {submitting || uploadingFiles ? t('requestForm.submitting') : t('requestForm.submitRequest')}
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
