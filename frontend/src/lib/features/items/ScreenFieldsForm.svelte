<script>
  // Presentational renderer for a transition screen's field list. It takes the
  // resolved screen fields plus a values map keyed by the item update keys the
  // transition endpoint accepts, and emits changes. Shared by the transition
  // modal; the create form keeps its own store (WI-1835).
  import { onMount } from 'svelte';
  import Input from '../../components/Input.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import Label from '../../components/Label.svelte';
  import PriorityPicker from '../../pickers/PriorityPicker.svelte';
  import UserPicker from '../../pickers/UserPicker.svelte';
  import CustomFieldRenderer from './CustomFieldRenderer.svelte';
  import WorkspaceLabelCombobox from '../../pickers/WorkspaceLabelCombobox.svelte';
  import { customFields as customFieldsApi } from '../../api/configuration.js';
  import { t } from '../../stores/i18n.svelte.js';

  let {
    screenFields = [],
    values = $bindable({}),
    workspaceId = null,
    milestones = [],
    iterations = [],
    itemLabels = [],
    disabled = false,
  } = $props();

  let customFieldDefs = $state([]);
  // Names shown by the label combobox. Null means the item's current labels;
  // a selection (including a just-created label) replaces it.
  let labelSelection = $state(null);

  onMount(async () => {
    try {
      customFieldDefs = (await customFieldsApi.getAll()) || [];
    } catch {
      customFieldDefs = [];
    }
  });

  const customDefsById = $derived.by(() => {
    const map = new Map();
    for (const def of customFieldDefs) {
      map.set(String(def.id), def);
    }
    return map;
  });

  const systemFields = $derived((screenFields || []).filter((field) => field.field_type !== 'custom'));
  const customFields = $derived((screenFields || []).filter((field) => field.field_type === 'custom'));

  function setValue(key, value) {
    values = { ...values, [key]: value };
  }

  function customValue(defId) {
    const cfv = values.custom_field_values || {};
    return cfv[String(defId)] ?? null;
  }

  function setCustomValue(defId, value) {
    const cfv = { ...(values.custom_field_values || {}) };
    cfv[String(defId)] = value;
    setValue('custom_field_values', cfv);
  }

  function isRequired(field) {
    return field?.is_required === true;
  }

  // The combobox speaks label names. When the user has changed the set it is
  // carried in labelSelection; otherwise the item's current labels show.
  function labelNames() {
    if (labelSelection !== null) return labelSelection;
    return (itemLabels || []).map((label) => label.name);
  }

  function setLabels(result) {
    const selected = result?.labels || [];
    labelSelection = selected.map((label) => label?.name).filter(Boolean);
    const ids = selected.map((label) => label?.id).filter((id) => Number.isFinite(id));
    setValue('label_ids', ids);
  }
</script>

<div class="space-y-3" data-testid="screen-fields-form">
  {#each systemFields as field (field.field_identifier)}
    {@const identifier = field.field_identifier}
    {#if identifier === 'title'}
      <div class="space-y-1">
        <Label color="default">{t('common.title') || 'Title'}{#if isRequired(field)}<span style="color: var(--ds-text-danger, #ef4444);"> *</span>{/if}</Label>
        <Input value={values.title ?? ''} disabled={disabled} oninput={(e) => setValue('title', e.target.value)} dataTestid="transition-field-title" />
      </div>
    {:else if identifier === 'description'}
      <div class="space-y-1">
        <Label color="default">{t('common.description') || 'Description'}{#if isRequired(field)}<span style="color: var(--ds-text-danger, #ef4444);"> *</span>{/if}</Label>
        <Textarea value={values.description ?? ''} disabled={disabled} oninput={(e) => setValue('description', e.target.value)} data-testid="transition-field-description" />
      </div>
    {:else if identifier === 'priority'}
      <div class="space-y-1">
        <Label color="default">{t('common.priority')}{#if isRequired(field)}<span style="color: var(--ds-text-danger, #ef4444);"> *</span>{/if}</Label>
        <PriorityPicker
          {workspaceId}
          selectedPriorityId={values.priority_id ?? null}
          onChange={(id) => setValue('priority_id', id)}
          disabled={disabled}
        />
      </div>
    {:else if identifier === 'assignee'}
      <div class="space-y-1">
        <Label color="default">{t('common.assignee') || 'Assignee'}{#if isRequired(field)}<span style="color: var(--ds-text-danger, #ef4444);"> *</span>{/if}</Label>
        <UserPicker
          {workspaceId}
          value={values.assignee_id ?? null}
          disabled={disabled}
          onSelect={(user) => setValue('assignee_id', user?.id ?? null)}
        />
      </div>
    {:else if identifier === 'due_date' || identifier === 'start_date' || identifier === 'end_date'}
      <div class="space-y-1">
        <Label color="default">{identifier === 'due_date' ? t('items.dueDate') : identifier === 'start_date' ? t('items.startDate') : t('common.endDate')}{#if isRequired(field)}<span style="color: var(--ds-text-danger, #ef4444);"> *</span>{/if}</Label>
        <Input type="date" value={values[identifier] ?? ''} disabled={disabled} oninput={(e) => setValue(identifier, e.target.value)} />
      </div>
    {:else if identifier === 'story_points' || identifier === 'estimate_minutes' || identifier === 'estimate'}
      {@const key = identifier === 'estimate' ? 'estimate_minutes' : identifier}
      <div class="space-y-1">
        <Label color="default">{key === 'story_points' ? t('items.storyPoints') || 'Story points' : t('items.estimate') || 'Estimate'}{#if isRequired(field)}<span style="color: var(--ds-text-danger, #ef4444);"> *</span>{/if}</Label>
        <Input type="number" value={values[key] ?? ''} disabled={disabled} oninput={(e) => setValue(key, e.target.value === '' ? null : Number(e.target.value))} />
      </div>
    {:else if identifier === 'labels'}
      <div class="space-y-1">
        <Label color="default">{t('items.labels') || 'Labels'}{#if isRequired(field)}<span style="color: var(--ds-text-danger, #ef4444);"> *</span>{/if}</Label>
        <WorkspaceLabelCombobox
          {workspaceId}
          value={labelNames()}
          {disabled}
          onSelect={setLabels}
        />
      </div>
    {/if}
  {/each}

  {#each customFields as field (field.field_identifier)}
    {@const def = customDefsById.get(String(field.field_identifier))}
    {#if def}
      <div class="space-y-1">
        <Label color="default">{def.name}{#if isRequired(field)}<span style="color: var(--ds-text-danger, #ef4444);"> *</span>{/if}</Label>
        <CustomFieldRenderer
          field={def}
          value={customValue(def.id)}
          readonly={false}
          disabled={disabled}
          {workspaceId}
          {milestones}
          {iterations}
          onChange={(val) => setCustomValue(def.id, val)}
        />
      </div>
    {/if}
  {/each}
</div>
