<script>
  import { brandStyle } from '../lib/utils/brandStyle.js';
  import { onMount } from 'svelte';
  import AlertBox from '../lib/components/AlertBox.svelte';
  import Button from '../lib/components/Button.svelte';
  import Checkbox from '../lib/components/Checkbox.svelte';
  import Input from '../lib/components/Input.svelte';
  import Label from '../lib/components/Label.svelte';
  import NativeSelect from '../lib/components/NativeSelect.svelte';
  import Progress from '../lib/components/Progress.svelte';
  import Spinner from '../lib/components/Spinner.svelte';
  import Textarea from '../lib/components/Textarea.svelte';

  let {
    baseUrl,
    slug,
    formId = null,
    prefill = {},
    theme = 'auto',
    onSuccess = () => {},
    onError = () => {},
  } = $props();

  let channel = $state(null);
  let forms = $state([]);
  let fields = $state([]);
  let customFieldDefinitions = $state([]);
  let selectedFormId = $state(null);
  let loadedDetailFormId = $state(null);
  let loading = $state(true);
  let fieldsLoading = $state(false);
  let submitting = $state(false);
  let error = $state('');
  let authenticationRequired = $state(false);
  let success = $state('');
  let values = $state({ title: '', description: '', custom_fields: {} });
  let steps = $state([1]);
  let currentStep = $state(1);

  let brandColor = $derived(channel?.brand_color || null);
  let effectiveTheme = $derived(
    theme === 'dark' ||
      channel?.theme === 'dark' ||
      ((theme === 'auto' || !theme) &&
        channel?.theme === 'auto' &&
        globalThis.matchMedia?.('(prefers-color-scheme: dark)').matches)
      ? 'dark'
      : 'light'
  );
  let selectedForm = $derived(forms.find((form) => form.id === selectedFormId));
  let currentFields = $derived(
    fields.filter((field) => (field.step_number || 1) === currentStep)
  );
  let currentStepIndex = $derived(steps.indexOf(currentStep));
  let isFirstStep = $derived(currentStepIndex === 0);
  let isLastStep = $derived(currentStepIndex === steps.length - 1);

  onMount(() => {
    selectedFormId = formId ? Number(formId) : null;
    load();
  });

  $effect(() => {
    if (selectedFormId && loadedDetailFormId !== selectedFormId) loadFields();
  });

  async function request(path, options = {}) {
    const response = await fetch(`${baseUrl}/api${path}`, {
      ...options,
      // Same-origin widgets may use the browser's Windshift session. Browsers
      // still omit credentials for cross-origin widgets, whose authenticated
      // mode is intentionally unsupported by the integration UI.
      credentials: 'same-origin',
      headers: {
        Accept: 'application/json',
        ...(options.body ? { 'Content-Type': 'application/json' } : {}),
        ...options.headers,
      },
    });

    if (!response.ok) {
      let message = response.statusText || 'Request failed';
      try {
        const body = await response.json();
        message = body.error || body.message || message;
      } catch {
        // Keep fallback.
      }
      throw Object.assign(new Error(message), { status: response.status });
    }

    if (response.status === 204) return null;
    return response.json();
  }

  async function load() {
    try {
      loading = true;
      error = '';
      authenticationRequired = false;
      const bootstrap = await request(`/forms/${encodeURIComponent(slug)}/bootstrap`);
      channel = bootstrap.channel;
      forms = bootstrap.forms || [];
      const nextFormId = selectedFormId || (forms.length === 1 ? forms[0].id : null);
      if (bootstrap.form_detail?.form_id === nextFormId) applyDetail(bootstrap.form_detail);
      selectedFormId = nextFormId;
    } catch (err) {
      error = err.message || 'Unable to load form';
      onError(err);
    } finally {
      loading = false;
    }
  }

  async function loadFields() {
    try {
      fieldsLoading = true;
      error = '';
      authenticationRequired = false;
      const detail = await request(
        `/forms/${encodeURIComponent(slug)}/forms/${encodeURIComponent(selectedFormId)}/detail`
      );
      applyDetail(detail);
    } catch (err) {
      error = err.message || 'Unable to load form fields';
      onError(err);
    } finally {
      fieldsLoading = false;
    }
  }

  function applyDetail(detail) {
    fields = detail.fields || [];
    customFieldDefinitions = detail.custom_field_definitions || [];
    const stepNumbers = [...new Set(fields.map((field) => field.step_number || 1))].sort(
      (a, b) => a - b
    );
    steps = stepNumbers.length > 0 ? stepNumbers : [1];
    currentStep = steps[0];
    values = initialValues(fields);
    loadedDetailFormId = detail.form_id;
  }

  function initialValues(nextFields) {
    const next = {
      title: prefill.title || '',
      description: prefill.description || '',
      custom_fields: { ...(prefill.customFields || {}) },
    };

    for (const field of nextFields) {
      if (field.field_type === 'default') continue;
      const key = field.field_identifier;
      if (next.custom_fields[key] !== undefined) continue;
      if (key === 'email' && prefill.email) next.custom_fields[key] = prefill.email;
      else if (key === 'name' && prefill.name) next.custom_fields[key] = prefill.name;
      else if (fieldKind(field) === 'checkbox') next.custom_fields[key] = false;
      else if (fieldKind(field) === 'multiselect') next.custom_fields[key] = [];
      else next.custom_fields[key] = '';
    }
    return next;
  }

  function labelFor(field) {
    return field.display_name || field.field_label || field.field_name || field.field_identifier;
  }

  function customFieldDefinition(field) {
    return customFieldDefinitions.find(
      (definition) => String(definition.id) === String(field.field_identifier)
    );
  }

  function fieldKind(field) {
    if (field.field_type === 'custom') return customFieldDefinition(field)?.field_type || 'text';
    if (field.field_type === 'virtual') return field.virtual_field_type || 'text';
    return field.field_identifier === 'description' ? 'textarea' : 'text';
  }

  function optionsFor(field) {
    const raw =
      field.field_type === 'custom'
        ? customFieldDefinition(field)?.options
        : field.virtual_field_options;
    if (!raw) return [];
    if (Array.isArray(raw)) return normalizeOptions(raw);
    try {
      const parsed = JSON.parse(raw);
      return normalizeOptions(Array.isArray(parsed) ? parsed : parsed?.items || []);
    } catch {
      return [];
    }
  }

  function normalizeOptions(options) {
    return options.map((option) => ({
      value: option?.id ?? option?.value ?? option,
      label: option?.label ?? String(option),
    }));
  }

  function selectValue(field, rawValue) {
    if (field.field_type !== 'custom' || rawValue === '') return rawValue;
    const numeric = Number(rawValue);
    return Number.isFinite(numeric) ? numeric : rawValue;
  }

  function valueFor(field) {
    if (field.field_type === 'default') return values[field.field_identifier] || '';
    return values.custom_fields[field.field_identifier] ?? '';
  }

  function setValue(field, value) {
    if (field.field_type === 'default') {
      values = { ...values, [field.field_identifier]: value };
    } else {
      values = {
        ...values,
        custom_fields: { ...values.custom_fields, [field.field_identifier]: value },
      };
    }
  }

  function setMultiValue(field, optionValue, checked) {
    const resolvedValue = selectValue(field, optionValue);
    const currentValue = Array.isArray(valueFor(field)) ? valueFor(field) : [];
    const withoutValue = currentValue.filter(
      (entry) => String(entry) !== String(resolvedValue)
    );
    setValue(field, checked ? [...withoutValue, resolvedValue] : withoutValue);
  }

  function validate(fieldsToValidate = fields) {
    for (const field of fieldsToValidate) {
      if (!field.is_required) continue;
      const value = valueFor(field);
      if (
        value === undefined ||
        value === null ||
        value === '' ||
        (Array.isArray(value) && value.length === 0) ||
        (fieldKind(field) === 'checkbox' && value !== true)
      ) {
        error = `${labelFor(field)} is required`;
        return false;
      }
    }
    return true;
  }

  function goToNextStep() {
    error = '';
    if (!validate(currentFields)) return;
    if (!isLastStep) currentStep = steps[currentStepIndex + 1];
  }

  function goToPreviousStep() {
    error = '';
    if (!isFirstStep) currentStep = steps[currentStepIndex - 1];
  }

  function handleFormSubmit() {
    if (isLastStep) submit();
    else goToNextStep();
  }

  async function submit() {
    try {
      error = '';
      success = '';
      if (!validate()) return;
      submitting = true;
      const result = await request(`/forms/${encodeURIComponent(slug)}/submit`, {
        method: 'POST',
        body: JSON.stringify({
          request_type_id: selectedFormId,
          title: values.title,
          description: values.description,
          custom_fields: values.custom_fields,
        }),
      });
      success = result?.success_message || 'Thank you for your submission!';
      onSuccess(result);
    } catch (err) {
      authenticationRequired = err.status === 403;
      error = err.message || 'Unable to submit form';
      onError(err);
    } finally {
      submitting = false;
    }
  }
</script>

<div
  class="wsf-root"
  class:ds-brand-scope={Boolean(brandStyle(brandColor))}
  data-ds-color-mode={effectiveTheme}
  style={brandStyle(brandColor)}
>
  <div class="wsf-card">
    {#if loading}
      <div class="wsf-loading"><Spinner size="md" /></div>
    {:else if error && !selectedFormId}
      <AlertBox variant="error" message={error} />
    {:else if !selectedFormId}
      {#if channel?.name}<h2 class="wsf-title">{channel.name}</h2>{/if}
      <div class="wsf-description">Choose a form to continue.</div>
      {#each forms as form}
        <div class="wsf-field">
          <Button variant="default" fullWidth onclick={() => (selectedFormId = form.id)}>
            {form.name}
          </Button>
        </div>
      {/each}
    {:else if success}
      <div id="wsf-success">
        <AlertBox variant="success" message={success} />
      </div>
    {:else}
      {#if selectedForm}
        <h2 class="wsf-title">{selectedForm.name}</h2>
        <p class="wsf-description">
          {selectedForm.description || 'Complete the fields below to send your request.'}
        </p>
      {/if}

      {#if fieldsLoading}
        <div class="wsf-loading"><Spinner size="md" /></div>
      {:else}
        {#if authenticationRequired}
          <div class="wsf-error" data-testid="form-auth-required">
            <AlertBox
              variant="warning"
              message="This form requires a Windshift sign-in. Sign in in this browser, then retry from the hosted form."
            />
            <a href={baseUrl || '/'} target="_blank" rel="noreferrer">Open Windshift to sign in</a>
          </div>
        {:else if error}
          <AlertBox variant="error" message={error} class="wsf-error" />
        {/if}
        <form onsubmit={(event) => { event.preventDefault(); handleFormSubmit(); }}>
          {#if steps.length > 1}
            <div class="wsf-progress">
              <div class="wsf-progress-label">
                <span>Step {currentStepIndex + 1} of {steps.length}</span>
                <span>{Math.round(((currentStepIndex + 1) / steps.length) * 100)}%</span>
              </div>
              <Progress value={currentStepIndex + 1} max={steps.length} size="sm" />
            </div>
          {/if}
          {#each currentFields as field}
            {@const kind = fieldKind(field)}
            <div class="wsf-field">
              {#if kind === 'checkbox'}
                <Checkbox
                  id={`wsf-${field.field_identifier}`}
                  checked={Boolean(valueFor(field))}
                  label={`${labelFor(field)}${field.is_required ? ' *' : ''}`}
                  onchange={(checked) => setValue(field, checked)}
                />
              {:else}
                <Label
                  for={`wsf-${field.field_identifier}`}
                  required={field.is_required}
                  color="default"
                  class="wsf-label"
                >
                  {labelFor(field)}
                </Label>
                {#if kind === 'textarea'}
                  <Textarea
                    id={`wsf-${field.field_identifier}`}
                    value={valueFor(field)}
                    rows={4}
                    required={field.is_required}
                    oninput={(event) => setValue(field, event.currentTarget.value)}
                  />
                {:else if kind === 'select'}
                  <NativeSelect
                    id={`wsf-${field.field_identifier}`}
                    value={valueFor(field)}
                    options={optionsFor(field)}
                    placeholder="Select…"
                    required={field.is_required}
                    onchange={(value) => setValue(field, selectValue(field, value))}
                  />
                {:else if kind === 'multiselect'}
                  <div
                    id={`wsf-${field.field_identifier}`}
                    class="wsf-multiselect"
                    role="group"
                    aria-label={labelFor(field)}
                  >
                    {#each optionsFor(field) as option, optionIndex}
                      <Checkbox
                        id={`wsf-${field.field_identifier}-option-${optionIndex}`}
                        checked={(Array.isArray(valueFor(field)) ? valueFor(field) : []).some(
                          (entry) => String(entry) === String(selectValue(field, option.value))
                        )}
                        label={option.label}
                        size="small"
                        onchange={(checked) => setMultiValue(field, option.value, checked)}
                      />
                    {/each}
                  </div>
                {:else}
                  <Input
                    id={`wsf-${field.field_identifier}`}
                    dataTestid={`wsf-${field.field_identifier}`}
                    type={kind === 'email' ? 'email' : kind === 'date' ? 'date' : kind === 'number' ? 'number' : 'text'}
                    step={kind === 'number' ? 'any' : undefined}
                    value={valueFor(field)}
                    required={field.is_required}
                    oninput={(event) =>
                      setValue(
                        field,
                        kind === 'number' && event.currentTarget.value !== ''
                          ? Number(event.currentTarget.value)
                          : event.currentTarget.value
                      )}
                  />
                {/if}
              {/if}
              {#if field.help_text}<div class="wsf-help">{field.help_text}</div>{/if}
            </div>
          {/each}
          <div class="wsf-actions">
            {#if !isFirstStep}
              <Button variant="default" onclick={goToPreviousStep}>Back</Button>
            {/if}
            <Button id="wsf-submit" variant="primary" type="submit" disabled={submitting} loading={submitting}>
              {#if submitting}
                Submitting…
              {:else if isLastStep}
                {selectedForm?.config?.submit_button_text || 'Submit'}
              {:else}
                Next
              {/if}
            </Button>
          </div>
        </form>
      {/if}
    {/if}
  </div>
</div>
