<script>
  import { onMount } from 'svelte';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import X from '@lucide/svelte/icons/x';
  import { queryLanguage } from '../api/queryLanguage.js';
  import { t } from '../stores/i18n.svelte.js';
  import BasePicker from './BasePicker.svelte';
  import { controlSizeClasses } from '../utils/controlSizes.js';
  import {
    completionFieldToFilterField,
    findQlCompletionField,
  } from '../utils/qlCompletion.js';

  let {
    placeholder = '',
    selectedField = $bindable(null),
    disabled = false,
    fieldGroups = null,
    customFieldItems = null,
    excludedFieldIds = [],
    includeAggregates = false,
    // 'catalog' surfaces every item field the QL backend accepts (used by the
    // filter builders) instead of the curated subset, so new backend fields
    // cannot drift out of reach. 'standard' keeps the curated list for the
    // action editors, which only support a fixed set of writable fields.
    fieldSource = 'standard',
    onSelect = () => {},
    onClear = () => {}
  } = $props();

  const resolvedPlaceholder = $derived(placeholder || t('pickers.selectField'));

  let customFields = $state([]);
  let completionCatalog = $state(null);

  // Helper to get field translation (handles both object and string formats)
  function getFieldTranslation(fieldKey) {
    const field = t(`pickers.fields.${fieldKey}`);
    if (typeof field === 'object' && field !== null) {
      const f = /** @type {{ name?: string, description?: string }} */ (field);
      return { name: f.name || fieldKey, description: f.description || '' };
    }
    return { name: field || fieldKey, description: '' };
  }

  // Catalog fields have no `pickers.fields` entry, so map the few identity
  // fields users look for by name and fall back to the raw catalog label.
  const CATALOG_FIELD_LABELS = {
    key: () => t('common.key'),
    id: () => t('common.id'),
    parent: () => getFieldTranslation('parent').name,
  };

  function catalogFieldName(field) {
    return CATALOG_FIELD_LABELS[field.name]?.() || field.label || field.name;
  }

  // Normalize the QL value type to the vocabulary the filter rows render.
  function catalogFilterType(field) {
    const source = field.value_help?.source;
    if (source === 'users' || source === 'teams') return 'user';
    if (source) return 'enum';
    if (field.value_type === 'string') return 'text';
    return field.value_type || 'text';
  }

  // Standard fields grouped by category
  const curatedFieldGroups = $derived((fieldGroups || [
    {
      category: t('pickers.fieldCategories.basic'),
      fields: [
        { id: 'title', name: getFieldTranslation('title').name, type: 'text', description: getFieldTranslation('title').description },
        { id: 'description', name: getFieldTranslation('description').name, type: 'text', description: getFieldTranslation('description').description },
        { id: 'status', name: getFieldTranslation('status').name, type: 'enum', description: getFieldTranslation('status').description },
        { id: 'priority', name: getFieldTranslation('priority').name, type: 'enum', description: getFieldTranslation('priority').description },
        { id: 'itemType', name: getFieldTranslation('type').name, type: 'enum', description: getFieldTranslation('type').description }
      ]
    },
    {
      category: t('pickers.fieldCategories.people'),
      fields: [
        { id: 'assignee', name: getFieldTranslation('assignee').name, type: 'user', description: getFieldTranslation('assignee').description },
        { id: 'reporter', name: getFieldTranslation('reporter').name, type: 'user', description: getFieldTranslation('reporter').description }
      ]
    },
    {
      category: t('pickers.fieldCategories.dates'),
      fields: [
        { id: 'createdAt', name: getFieldTranslation('createdAt').name, type: 'date', description: getFieldTranslation('createdAt').description },
        { id: 'updatedAt', name: getFieldTranslation('updatedAt').name, type: 'date', description: getFieldTranslation('updatedAt').description },
        { id: 'dueDate', name: getFieldTranslation('dueDate').name, type: 'date', description: getFieldTranslation('dueDate').description }
      ]
    },
    {
      category: t('pickers.fieldCategories.workflow'),
      fields: [
        { id: 'milestone', name: getFieldTranslation('milestone').name, type: 'enum', description: getFieldTranslation('milestone').description },
        { id: 'iteration', name: getFieldTranslation('iteration').name, type: 'enum', description: getFieldTranslation('iteration').description },
        { id: 'labels', name: getFieldTranslation('labels').name, type: 'enum', description: getFieldTranslation('labels').description }
      ]
    },
    ...(includeAggregates
      ? [
          {
            category: t('pickers.fieldCategories.automation'),
            fields: [
              { id: 'open_child_count', name: getFieldTranslation('openChildCount').name, type: 'number', description: getFieldTranslation('openChildCount').description },
              { id: 'open_descendant_count', name: getFieldTranslation('openDescendantCount').name, type: 'number', description: getFieldTranslation('openDescendantCount').description }
            ]
          }
        ]
      : []),
  ]).map((group) => ({
    ...group,
    fields: group.fields.map((field) => ({
      ...field,
      completion: findQlCompletionField(completionCatalog, field),
    })),
  })));

  // Catalog fields not covered by the curated groups (key, id, parent, …) are
  // appended ungrouped, so the filter picker always matches the backend QL
  // catalog (GH #328).
  const standardFields = $derived.by(() => {
    if (fieldSource !== 'catalog' || !completionCatalog) return curatedFieldGroups;

    const covered = new Set();
    for (const group of curatedFieldGroups) {
      for (const field of group.fields) {
        covered.add(field.id);
        if (field.completion?.name) covered.add(field.completion.name);
      }
    }
    const extra = (completionCatalog.fields || [])
      .filter((field) => !/^cfid_\d+$/i.test(field.name) && !covered.has(field.name))
      .map((field) => ({
        ...completionFieldToFilterField(field),
        name: catalogFieldName(field),
        type: catalogFilterType(field),
      }));

    return extra.length
      ? [...curatedFieldGroups, { category: null, fields: extra }]
      : curatedFieldGroups;
  });

  // Flatten the grouped catalog for BasePicker, tagging each field with the
  // category header it renders under.
  const fieldItems = $derived.by(() => {
    const items = [];
    for (const group of standardFields) {
      for (const field of group.fields) {
        if (!excludedFieldIds.includes(field.id)) items.push({ ...field, group: group.category });
      }
    }
    const customGroup = t('pickers.customFields');
    for (const field of customFields) {
      if (!excludedFieldIds.includes(field.id)) items.push({ ...field, group: customGroup });
    }
    return items;
  });

  onMount(async () => {
    await loadCustomFields();
  });

  async function loadCustomFields() {
    if (customFieldItems) {
      customFields = customFieldItems;
      return;
    }
    try {
      completionCatalog = await queryLanguage.getCatalog();
      customFields = (completionCatalog?.fields || [])
        .filter((field) => /^cfid_\d+$/i.test(field.name))
        .map(completionFieldToFilterField);
    } catch (error) {
      console.error('Failed to load query-language fields:', error);
      customFields = [];
    }
  }

  function handleSelect(field) {
    selectedField = field;
    onSelect(field);
  }

  function clearSelection() {
    selectedField = null;
    onClear();
  }

  function getFieldTypeLabel(type) {
    const labels = {
      text: t('pickers.fieldTypes.text'),
      number: t('pickers.fieldTypes.number'),
      date: t('pickers.fieldTypes.date'),
      enum: t('pickers.fieldTypes.select'),
      boolean: t('pickers.fieldTypes.boolean'),
      user: t('pickers.fieldTypes.user'),
      reference: t('pickers.fieldTypes.reference'),
      select: t('pickers.fieldTypes.select'),
      multiselect: t('pickers.fieldTypes.select'),
      textarea: t('pickers.fieldTypes.textArea'),
      identifier: t('pickers.fieldTypes.identifier')
    };
    return labels[type] || type;
  }

  function getFieldTypeColor(type) {
    const colors = {
      text: 'bg-ds-accent-blue-subtle text-ds-text-accent-blue',
      number: 'bg-ds-accent-green-subtle text-ds-text-accent-green',
      date: 'bg-ds-accent-purple-subtle text-ds-text-accent-purple',
      enum: 'bg-ds-accent-orange-subtle text-ds-text-accent-orange',
      boolean: 'bg-ds-background-neutral text-ds-text-subtle',
      user: 'bg-ds-accent-teal-subtle text-ds-text-accent-teal',
      reference: 'bg-ds-accent-red-subtle text-ds-text-accent-red',
      select: 'bg-ds-accent-orange-subtle text-ds-text-accent-orange',
      multiselect: 'bg-ds-accent-orange-subtle text-ds-text-accent-orange',
      textarea: 'bg-ds-accent-blue-subtle text-ds-text-accent-blue'
    };
    return colors[type] || 'bg-ds-background-neutral text-ds-text-subtle';
  }
</script>

<BasePicker
  items={fieldItems}
  value={selectedField?.id ?? null}
  placeholder={resolvedPlaceholder}
  {disabled}
  searchFields={['name', 'description']}
  groupBy={(field) => field.group}
  getValue={(field) => field.id}
  getLabel={(field) => field.name}
  optionTestid={(opt) => `field-option-${opt.value}`}
  searchTestid="field-selector-search"
  searchPlaceholder={t('pickers.searchFields')}
  menuTestid="field-selector-menu"
  scrollTestid="field-selector-scroll"
  onSelect={handleSelect}
>
  {#snippet children()}
    <div
      data-testid="field-selector-trigger"
      aria-disabled={disabled}
      class="w-full flex items-center justify-between {controlSizeClasses('medium')} border rounded transition-colors"
      style="border-color: var(--ds-border); background-color: {disabled ? 'var(--ds-background-neutral)' : 'var(--ds-surface)'}; {disabled ? 'opacity: 0.5; cursor: not-allowed;' : ''}"
    >
      {#if selectedField}
        <div class="flex items-center gap-2 flex-1 min-w-0">
          <span class="font-medium truncate" data-testid="field-selector-value" style="color: var(--ds-text);">{selectedField.name}</span>
          <span class="text-xs px-1.5 py-0.5 rounded {getFieldTypeColor(selectedField.type)}">
            {getFieldTypeLabel(selectedField.type)}
          </span>
          {#if selectedField.isCustom}
            <span class="text-xs px-1.5 py-0.5 rounded bg-ds-accent-purple-subtle text-ds-text-accent-purple">{t('pickers.custom')}</span>
          {/if}
        </div>
        <div class="flex items-center gap-1">
          <button
            type="button"
            onclick={(e) => { e.stopPropagation(); clearSelection(); }}
            class="p-1 rounded transition-colors hover-bg"
            title={t('pickers.clearSelection')}
          >
            <X class="w-4 h-4" style="color: var(--ds-text-subtle);" />
          </button>
          <ChevronDown class="w-4 h-4" style="color: var(--ds-text-subtle);" />
        </div>
      {:else}
        <span style="color: var(--ds-text-subtle);">{resolvedPlaceholder}</span>
        <ChevronDown class="w-4 h-4" style="color: var(--ds-text-subtle);" />
      {/if}
    </div>
  {/snippet}

  {#snippet itemSnippet({ item: field })}
    <div class="flex-1 min-w-0">
      <div class="flex items-center gap-2">
        <span class="font-medium" style="color: var(--ds-text);">{field.name}</span>
        <span class="text-xs px-1.5 py-0.5 rounded {getFieldTypeColor(field.type)}">
          {getFieldTypeLabel(field.type)}
        </span>
        {#if field.isCustom}
          <span class="text-xs px-1.5 py-0.5 rounded bg-ds-accent-purple-subtle text-ds-text-accent-purple">{t('pickers.custom')}</span>
        {/if}
      </div>
      {#if field.description}
        <p class="text-xs mt-0.5" style="color: var(--ds-text-subtle);">{field.description}</p>
      {/if}
    </div>
  {/snippet}

  {#snippet noResultsSnippet({ searchQuery })}
    <div class="p-4 text-center text-sm" style="color: var(--ds-text-subtle);">
      {t('pickers.noFieldsFound', { query: searchQuery })}
    </div>
  {/snippet}
</BasePicker>

<style>
  .hover-bg:hover {
    background-color: var(--ds-background-neutral-hovered);
  }
</style>
