<script>
  import Button from '../../components/Button.svelte';
  import FormField from '../../components/FormField.svelte';
  import Input from '../../components/Input.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import Select from '../../components/Select.svelte';
  import UserPicker from '../../pickers/UserPicker.svelte';
  import PagePicker from '../../pickers/PagePicker.svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { confirm } from '../../composables/useConfirm.js';
  import { requirementTypeOptions } from './requirementTypes.js';
  import { requirementStatusOptions } from './requirementStatuses.js';
  import {
    DEFAULT_REQUIREMENT_STATUS,
    DEFAULT_REQUIREMENT_TYPE,
    applyStarterTemplate,
    shouldConfirmTemplateReplace,
  } from './requirementFormHelpers.js';

  let {
    workspaceId,
    title = $bindable(''),
    content = $bindable(''),
    requirementType = $bindable(DEFAULT_REQUIREMENT_TYPE),
    status = $bindable(DEFAULT_REQUIREMENT_STATUS),
    ownerId = $bindable(null),
    parentId = $bindable(null),
    contentTouched = $bindable(false),
    saving = false,
    disabled = false,
  } = $props();

  let previousRequirementType = $state(DEFAULT_REQUIREMENT_TYPE);

  const typeOptions = $derived(requirementTypeOptions(t));
  const statusOptions = $derived(requirementStatusOptions(t));

  export function resetForm(defaultOwnerId = null) {
    title = '';
    requirementType = DEFAULT_REQUIREMENT_TYPE;
    status = DEFAULT_REQUIREMENT_STATUS;
    ownerId = defaultOwnerId;
    parentId = null;
    contentTouched = false;
    previousRequirementType = DEFAULT_REQUIREMENT_TYPE;
    applyTemplate(DEFAULT_REQUIREMENT_TYPE);
  }

  function applyTemplate(type) {
    content = applyStarterTemplate(type, t);
    contentTouched = false;
  }

  async function confirmReplace() {
    return confirm({
      title: t('requirements.fieldType'),
      message: t('requirements.templates.replaceConfirm'),
      confirmText: t('common.confirm'),
      cancelText: t('common.cancel'),
      variant: 'info',
    });
  }

  async function handleTypeChange() {
    const newType = requirementType;
    const oldType = previousRequirementType;
    if (newType === oldType) return;

    if (shouldConfirmTemplateReplace(content, contentTouched)) {
      const ok = await confirmReplace();
      if (!ok) {
        requirementType = oldType;
        return;
      }
    }

    applyTemplate(newType);
    previousRequirementType = newType;
  }

  async function resetToTemplate() {
    if (shouldConfirmTemplateReplace(content, contentTouched)) {
      const ok = await confirmReplace();
      if (!ok) return;
    }
    applyTemplate(requirementType);
  }

  function onContentInput() {
    contentTouched = true;
  }
</script>

<div class="create-form" data-testid="requirement-create-form">
  <FormField label={t('requirements.fieldTitle')} required>
    <Input bind:value={title} disabled={disabled || saving} />
  </FormField>

  <div class="create-form__grid">
    <div class="create-form__meta">
      <FormField label={t('requirements.fieldType')} required>
        <Select
          bind:value={requirementType}
          options={typeOptions}
          onchange={handleTypeChange}
          disabled={disabled || saving}
        />
      </FormField>
      <FormField label={t('requirements.fieldStatus')}>
        <Select bind:value={status} options={statusOptions} disabled={disabled || saving} />
      </FormField>
      <FormField label={t('requirements.fieldOwner')}>
        <UserPicker bind:value={ownerId} {workspaceId} disabled={disabled || saving} />
      </FormField>
      <FormField label={t('requirements.fieldParent')}>
        <PagePicker bind:value={parentId} {workspaceId} disabled={disabled || saving} />
      </FormField>
    </div>

    <FormField label={t('requirements.fieldContent')} class="create-form__content">
      <div class="content-field">
        <Textarea
          bind:value={content}
          rows={16}
          oninput={onContentInput}
          disabled={disabled || saving}
        />
        <Button variant="ghost" size="sm" onclick={resetToTemplate} disabled={disabled || saving}>
          {t('requirements.templates.reset')}
        </Button>
      </div>
    </FormField>
  </div>
</div>

<style>
  .create-form {
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  .create-form__grid {
    display: grid;
    grid-template-columns: minmax(240px, 320px) minmax(0, 1fr);
    gap: 1.5rem;
    align-items: start;
  }

  .create-form__meta {
    display: flex;
    flex-direction: column;
    gap: 1rem;
    /* Keep focus rings from being clipped by the scroll container. */
    padding-inline: 3px;
  }

  .content-field {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.35rem;
    width: 100%;
  }

  @media (max-width: 768px) {
    .create-form__grid {
      grid-template-columns: 1fr;
    }
  }
</style>
