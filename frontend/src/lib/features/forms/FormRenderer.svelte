<script>
  import { untrack } from 'svelte';
  import StateDisplay from '../../components/StateDisplay.svelte';
  import ExternalLink from '@lucide/svelte/icons/external-link';
  import FileText from '@lucide/svelte/icons/file-text';
  import LogIn from '@lucide/svelte/icons/log-in';
  import Paperclip from '@lucide/svelte/icons/paperclip';
  import RotateCcw from '@lucide/svelte/icons/rotate-ccw';
  import { forms } from '../../api/forms.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { authStore } from '../../stores/auth.svelte.js';
  import Label from '../../components/Label.svelte';
  import FileInput from '../../components/FileInput.svelte';
  import AlertBox from '../../components/AlertBox.svelte';
  import Button from '../../components/Button.svelte';
  import Progress from '../../components/Progress.svelte';
  import { toExternal } from '../../runtime/contextPath.js';
  import { loadPublicFormDetail } from './publicFormData.js';
  import FormFields from './FormFields.svelte';
  import {
    buildFormSteps,
    clampFormStep,
    initializeFormValues,
    validateFormStep,
  } from './formModel.js';
  import {
    clearPublicFormDraft,
    loadPublicFormDraftForIdentity,
    savePublicFormDraft,
  } from './publicFormDrafts.js';

  let {
    formSlug = '',
    formId = null,
    formConfig = null,
    attachmentConfig = null,
    initialDetail = null,
    brandColor = null,
    isDarkMode = false,
    initialValues = null,
    submitForm = null,
    authenticationRequired = false,
    embed = false,
    preview = false,
    returnPath = '',
    onSubmitted = () => {},
  } = $props();

  let submitButtonText = $derived(formConfig?.submit_button_text || 'Submit');
  let fields = $state([]);
  let customFieldDefinitions = $state([]);
  let activeDetail = $state(null);
  let loading = $state(true);
  let submitting = $state(false);
  let error = $state(null);
  let submissionRequiresAuth = $state(false);
  let success = $state(false);
  let successMessage = $state('');
  let redirectUrl = $state('');
  let loadSequence = 0;

  let steps = $state([1]);
  let currentStep = $state(1);
  let totalSteps = $derived(steps.length);
  let isLastStep = $derived(currentStep === Math.max(...steps));
  let isFirstStep = $derived(currentStep === Math.min(...steps));

  let formData = $state({ title: '', description: '' });
  let customFieldValues = $state({});
  let attachments = $state([]);
  let attachmentInput = $state(null);

  let draftReady = $state(false);
  let resumedDraft = $state(null);
  let draftStatus = $state('');
  let draftBaseline = $state('');
  let draftStatusTimer = null;
  let needsAuthentication = $derived(
    !preview && (authenticationRequired || submissionRequiresAuth)
  );
  let hostedFormPath = $derived(`/forms/${encodeURIComponent(formSlug)}/${formId}`);
  let signInReturnPath = $derived(
    returnPath || (typeof window !== 'undefined' ? `${window.location.pathname}${window.location.search}` : hostedFormPath)
  );
  let signInUrl = $derived(
    toExternal(`/login?return_to=${encodeURIComponent(signInReturnPath)}`)
  );

  $effect(() => {
    const activeSlug = formSlug;
    const activeFormId = formId;
    const seededDetail = initialDetail;
    if (!activeSlug || !activeFormId) return;

    const sequence = ++loadSequence;
    untrack(() => {
      if (seededDetail?.form_id === activeFormId) {
        applyDetail(seededDetail);
      } else {
        void loadFields(activeSlug, activeFormId, sequence);
      }
    });
  });

  $effect(() => {
    const snapshot = {
      title: formData.title,
      description: formData.description,
      custom_fields: JSON.parse(JSON.stringify(customFieldValues)),
      current_step: currentStep,
    };
    if (!draftReady || preview || success) return;
    const { storage: draftStorage, userId: draftUserId } = resolveDraftStore();
    if (!draftStorage) return;
    if (JSON.stringify(snapshot) === draftBaseline) return;

    draftStatus = 'saving';
    const timer = setTimeout(() => {
      const saved = savePublicFormDraft(draftStorage, formSlug, formId, snapshot, draftUserId);
      draftStatus = saved ? 'saved' : '';
      if (saved) {
        draftBaseline = JSON.stringify(snapshot);
        if (draftStatusTimer) clearTimeout(draftStatusTimer);
        draftStatusTimer = setTimeout(() => {
          draftStatus = '';
          draftStatusTimer = null;
        }, 1800);
      }
    }, 300);

    return () => clearTimeout(timer);
  });

  $effect(() => {
    return () => {
      if (draftStatusTimer) clearTimeout(draftStatusTimer);
    };
  });

  function resolveDraftStore() {
    // Authenticated drafts bind to the user identity and persist in
    // localStorage so progress is restored after sign-in. Anonymous drafts
    // use sessionStorage so they are scoped to the current tab and never
    // survive into the next browser session on a shared device.
    if (authStore.isAuthenticated) {
      const userId = authStore.currentUser?.id;
      if (userId != null && typeof localStorage !== 'undefined') {
        return { storage: localStorage, userId };
      }
    }
    if (typeof sessionStorage !== 'undefined') {
      return { storage: sessionStorage, userId: null };
    }
    return { storage: null, userId: null };
  }

  function applyDetail(detail) {
    draftReady = false;
    draftStatus = '';
    if (draftStatusTimer) {
      clearTimeout(draftStatusTimer);
      draftStatusTimer = null;
    }
    activeDetail = detail;
    error = null;
    submissionRequiresAuth = false;
    success = false;
    fields = detail.fields || [];
    customFieldDefinitions = detail.custom_field_definitions || [];
    steps = buildFormSteps(fields);

    let values = initializeFormValues(fields, initialValues, customFieldDefinitions);
    let nextStep = steps[0];
    resumedDraft = null;

    // Purge any pre-0.8.4 anonymous draft left in localStorage under the
    // legacy user-less key so locally persisted form values do not linger.
    if (typeof localStorage !== 'undefined') {
      clearPublicFormDraft(localStorage, formSlug, formId);
    }

    if (!preview && !initialValues) {
      const draftUserId = authStore.isAuthenticated ? authStore.currentUser?.id : null;
      const draft = loadPublicFormDraftForIdentity({
        anonymousStorage: typeof sessionStorage !== 'undefined' ? sessionStorage : null,
        authenticatedStorage: typeof localStorage !== 'undefined' ? localStorage : null,
        slug: formSlug,
        formId,
        userId: draftUserId,
      });
      if (draft) {
        values = initializeFormValues(fields, draft, customFieldDefinitions);
        nextStep = clampFormStep(steps, draft.current_step);
        resumedDraft = draft;
      }
    }

    formData = values.formData;
    customFieldValues = values.customFieldValues;
    currentStep = nextStep;
    attachments = [];
    draftBaseline = JSON.stringify({
      title: formData.title,
      description: formData.description,
      custom_fields: JSON.parse(JSON.stringify(customFieldValues)),
      current_step: currentStep,
    });
    loading = false;
    queueMicrotask(() => {
      draftReady = true;
    });
  }

  async function loadFields(activeSlug, activeFormId, sequence) {
    try {
      loading = true;
      error = null;
      submissionRequiresAuth = false;
      success = false;

      const detail = await loadPublicFormDetail(activeSlug, activeFormId);
      if (sequence !== loadSequence) return;
      applyDetail(detail);
    } catch (err) {
      if (sequence !== loadSequence) return;
      console.error('Failed to load form fields:', err);
      error = err.message || 'Failed to load form fields';
    } finally {
      if (sequence === loadSequence) loading = false;
    }
  }

  function validateCurrentStep() {
    const message = validateFormStep({
      fields,
      step: currentStep,
      formData,
      customFieldValues,
      customFieldDefinitions,
    });
    error = message || null;
    return !message;
  }

  function goToNextStep() {
    error = null;
    if (!validateCurrentStep()) return;
    const currentIndex = steps.indexOf(currentStep);
    if (currentIndex < steps.length - 1) currentStep = steps[currentIndex + 1];
  }

  function goToPrevStep() {
    error = null;
    const currentIndex = steps.indexOf(currentStep);
    if (currentIndex > 0) currentStep = steps[currentIndex - 1];
  }

  function handleAttachmentChange(event) {
    attachments = Array.from(event.currentTarget.files || []);
    error = null;
  }

  function startFresh() {
    const { storage: draftStorage, userId: draftUserId } = resolveDraftStore();
    if (draftStorage) {
      clearPublicFormDraft(draftStorage, formSlug, formId, draftUserId);
    }
    resumedDraft = null;
    if (activeDetail) applyDetail(activeDetail);
  }

  function submitAnother() {
    success = false;
    successMessage = '';
    redirectUrl = '';
    if (activeDetail) applyDetail(activeDetail);
  }

  async function handleSubmit() {
    if (preview || needsAuthentication) return;

    try {
      for (const step of steps) {
        currentStep = step;
        if (!validateCurrentStep()) return;
      }
      currentStep = Math.max(...steps);

      submitting = true;
      error = null;
      submissionRequiresAuth = false;

      if (attachmentConfig?.enabled) {
        const maxFiles = attachmentConfig.max_files || 5;
        if (attachments.length > maxFiles) {
          error = `You can attach at most ${maxFiles} files.`;
          return;
        }
        const oversized = attachments.find((file) => file.size > attachmentConfig.max_file_size);
        if (oversized) {
          error = `${oversized.name} exceeds the attachment size limit.`;
          return;
        }
      }

      const submissionData = {
        request_type_id: formId,
        title: formData.title,
        description: formData.description,
        custom_fields: customFieldValues,
      };

      const result = submitForm
        ? await submitForm(submissionData, attachments)
        : await forms.submit(formSlug, submissionData, attachments);

      draftReady = false;
      const { storage: draftStorage, userId: draftUserId } = resolveDraftStore();
      if (draftStorage) {
        clearPublicFormDraft(draftStorage, formSlug, formId, draftUserId);
      }
      success = true;
      successMessage = result.success_message || 'Thank you for your submission!';
      redirectUrl = result.redirect_url || '';
      onSubmitted(result);

      if (redirectUrl) {
        setTimeout(() => {
          window.location.href = toExternal(redirectUrl);
        }, 2000);
      }
    } catch (err) {
      console.error('Failed to submit form:', err);
      submissionRequiresAuth = err.status === 403;
      error = err.message || 'Failed to submit form';
    } finally {
      submitting = false;
    }
  }
</script>

<div
  class:ds-brand-scope={Boolean(brandColor)}
  style={brandColor ? `--ds-brand-color: ${brandColor}` : undefined}
>
  {#if loading}
    <StateDisplay type="loading" />
  {:else if success}
    <div data-testid="public-form-success">
      <AlertBox variant="success">
        <div>
          <p class="font-medium" style="color: var(--ds-text);">Response received</p>
          <p class="mt-0.5" style="color: var(--ds-text-subtle);">{successMessage}</p>
        </div>
      </AlertBox>
      <div class="mt-4 flex flex-wrap gap-2">
        <Button
          type="button"
          variant="default"
          icon={RotateCcw}
          dataTestid="public-form-submit-another"
          onclick={submitAnother}
        >
          Submit another response
        </Button>
        {#if redirectUrl}
          <Button
            type="button"
            variant="primary"
            icon={ExternalLink}
            dataTestid="public-form-success-continue"
            onclick={() => {
              window.location.href = toExternal(redirectUrl);
            }}
          >
            Continue
          </Button>
        {/if}
      </div>
      {#if redirectUrl}
        <p class="mt-2 text-xs" style="color: var(--ds-text-subtle);">Redirecting shortly…</p>
      {/if}
    </div>
  {:else if needsAuthentication}
    <div data-testid="form-auth-required">
      <div>
        <AlertBox
          variant="warning"
          message={embed
            ? 'This form requires a Windshift sign-in and cannot be completed inside an embed.'
            : 'Sign in to continue. Your saved progress will be restored when you return.'}
        />
        <Button
          href={embed ? toExternal(hostedFormPath) : signInUrl}
          target={embed ? '_blank' : undefined}
          variant="primary"
          icon={embed ? ExternalLink : LogIn}
          dataTestid="public-form-auth-action"
          class="mt-4"
        >
          {embed ? 'Open hosted form' : 'Sign in and return'}
        </Button>
      </div>
    </div>
  {:else}
    {#if preview}
      <div class="mb-4">
        <AlertBox variant="info" message="Preview mode — submissions are disabled." />
      </div>
    {:else if resumedDraft}
      <div
        data-testid="public-form-draft-resume"
        class="mb-4 flex items-center justify-between gap-3 rounded-lg border p-3"
        style="border-color: var(--ds-border); background: var(--ds-background-neutral);"
      >
        <p class="text-sm" style="color: var(--ds-text-subtle);">Your saved progress was restored.</p>
        <button
          type="button"
          data-testid="public-form-draft-start-fresh"
          class="text-sm font-medium hover:underline"
          style="color: var(--ds-text-link);"
          onclick={startFresh}
        >
          Start fresh
        </button>
      </div>
    {/if}

    <form onsubmit={(event) => { event.preventDefault(); isLastStep ? handleSubmit() : goToNextStep(); }}>
      {#if error}
        <AlertBox variant="error" message={error} class="mb-4" />
      {/if}

      {#if totalSteps > 1}
        <div class="mb-6">
          <div class="mb-2 flex items-center justify-between text-xs font-medium" style="color: var(--ds-text-subtle);">
            <span>Step {steps.indexOf(currentStep) + 1} of {totalSteps}</span>
            <span>{Math.round(((steps.indexOf(currentStep) + 1) / totalSteps) * 100)}%</span>
          </div>
          <Progress value={steps.indexOf(currentStep) + 1} max={totalSteps} size="sm" />
        </div>
      {/if}

      <FormFields
        {fields}
        {customFieldDefinitions}
        {currentStep}
        bind:formData
        bind:customFieldValues
        {isDarkMode}
        idPrefix="form"
      />

      {#if isLastStep && attachmentConfig?.enabled}
        <div class="mt-4">
          <Label for="form-attachments" class="mb-1.5" color="default">Attachments</Label>
          <FileInput
            id="form-attachments"
            bind:inputRef={attachmentInput}
            dataTestid="public-form-attachments"
            multiple
            accept={attachmentConfig.allowed_mime_types?.join(',') || undefined}
            onchange={handleAttachmentChange}
            class="sr-only"
          />
          <div class="flex flex-wrap items-center gap-2">
            <Button
              type="button"
              variant="default"
              size="small"
              icon={Paperclip}
              onclick={() => attachmentInput?.click()}
            >
              Choose files
            </Button>
            <span class="text-xs" style="color: var(--ds-text-subtle);">
              {attachments.length === 0
                ? 'No files selected'
                : `${attachments.length} ${attachments.length === 1 ? 'file' : 'files'} selected`}
            </span>
          </div>
          <p class="mt-1 text-xs" style="color: var(--ds-text-subtle);">
            Up to {attachmentConfig.max_files || 5} files, {Math.floor(attachmentConfig.max_file_size / 1048576)} MB each.
          </p>
          {#if attachments.length > 0}
            <ul
              data-testid="public-form-attachment-list"
              class="mt-2 divide-y rounded border text-xs"
              style="border-color: var(--ds-border); --tw-divide-color: var(--ds-border);"
            >
              {#each attachments as attachment}
                <li class="flex items-center gap-2 px-3 py-2" style="color: var(--ds-text);">
                  <FileText class="h-4 w-4 shrink-0" style="color: var(--ds-icon);" />
                  <span class="min-w-0 truncate">{attachment.name}</span>
                </li>
              {/each}
            </ul>
          {/if}
        </div>
      {/if}

      <div class="mt-6 flex items-center justify-between border-t pt-4" style="border-color: var(--ds-border);">
        <div class="flex items-center gap-3">
          {#if !isFirstStep}
            <Button
              type="button"
              variant="default"
              dataTestid="public-form-back-step"
              onclick={goToPrevStep}
            >
              Back
            </Button>
          {/if}
          {#if !preview && draftStatus}
            <span class="text-xs" data-testid="public-form-draft-status" style="color: var(--ds-text-subtle);">
              {draftStatus === 'saving' ? 'Saving draft…' : 'Draft saved'}
            </span>
          {/if}
        </div>

        <Button
          type="submit"
          variant="primary"
          dataTestid="public-form-submit"
          disabled={submitting || (preview && isLastStep)}
          loading={submitting}
        >
          {isLastStep ? (preview ? 'Preview only' : submitButtonText) : 'Next'}
        </Button>
      </div>
    </form>
  {/if}
</div>
