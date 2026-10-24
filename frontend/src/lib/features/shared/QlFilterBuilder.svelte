<script>
  import { onMount } from 'svelte';
  import Plus from '@lucide/svelte/icons/plus';
  import X from '@lucide/svelte/icons/x';
  import { t } from '../../stores/i18n.svelte.js';
  import { queryLanguage } from '../../api/queryLanguage.js';
  import { QLBuilder } from '../../utils/ql.js';
  import { completionFieldToFilterField } from '../../utils/qlCompletion.js';
  import Button from '../../components/Button.svelte';
  import DynamicFieldFilter from '../items/DynamicFieldFilter.svelte';
  import QlQueryBar from './QlQueryBar.svelte';

  let {
    qlQuery = '',
    filterState = null,
    completionCatalog = null,
    fieldGroups = null,
    customFieldItems = null,
    compact = true,
    testIdPrefix = 'ql-filter-builder',
    onchange = null,
  } = $props();

  let mode = $state('builder');
  let filters = $state([]);
  let rawQl = $state('');
  let hydrationFailed = $state(false);
  let catalog = $state(null);

  const builtQl = $derived(QLBuilder.buildQuery({ dynamicFields: filters }));

  onMount(() => {
    if (completionCatalog) catalog = completionCatalog;
    void initialize();
  });

  // Prefer the persisted builder state; fall back to recovering it from the
  // stored QL, and keep raw mode when the QL uses clauses the builder cannot
  // represent so nothing is silently lost.
  async function initialize() {
    rawQl = qlQuery ?? '';
    const state = parseFilterState(filterState);
    if (state?.dynamicFields?.length) {
      filters = state.dynamicFields;
      mode = 'builder';
      return;
    }
    if (rawQl.trim()) {
      mode = (await hydrateFromQl(rawQl)) ? 'builder' : 'raw';
      return;
    }
    mode = 'builder';
  }

  function parseFilterState(value) {
    if (!value) return null;
    try {
      const parsed = typeof value === 'string' ? JSON.parse(value) : value;
      return parsed && Array.isArray(parsed.dynamicFields) ? parsed : null;
    } catch {
      return null;
    }
  }

  async function ensureCatalog() {
    if (catalog) return catalog;
    try {
      catalog = await queryLanguage.getCatalog();
    } catch (error) {
      console.warn('Failed to load QL catalog for builder hydration:', error);
      catalog = null;
    }
    return catalog;
  }

  async function hydrateFromQl(ql) {
    if (!ql?.trim()) {
      filters = [];
      hydrationFailed = false;
      return true;
    }
    const resolved = await ensureCatalog();
    const customFields = (resolved?.fields || [])
      .filter((field) => /^cfid_\d+$/i.test(field.name))
      .map(completionFieldToFilterField);
    const parsed = QLBuilder.tryParseToBuilder(ql, { customFields });
    // Only accept a hydration that lands entirely in dynamic filters. Status,
    // priority, workspace, and search clauses live outside this component's
    // state, so accepting them would drop them on the next builder edit.
    const onlyDynamic =
      parsed &&
      !parsed.dropped &&
      parsed.workspaces.length === 0 &&
      parsed.statuses.length === 0 &&
      parsed.priorities.length === 0 &&
      parsed.search === '';
    if (!onlyDynamic) {
      hydrationFailed = true;
      return false;
    }
    filters = parsed.dynamicFields;
    hydrationFailed = false;
    return true;
  }

  function selectBuilderMode() {
    if (mode === 'builder') return;
    void (async () => {
      if (await hydrateFromQl(rawQl)) {
        mode = 'builder';
        emit();
      }
    })();
  }

  function selectRawMode() {
    if (mode === 'raw') return;
    rawQl = builtQl;
    hydrationFailed = false;
    mode = 'raw';
    emit();
  }

  function addFilter() {
    filters = [...filters, { field: null, operator: '=', value: '', values: [] }];
    emit();
  }

  function updateFilter(index, filter) {
    filters = filters.map((current, i) => (i === index ? filter : current));
    emit();
  }

  function removeFilter(index) {
    filters = filters.filter((_, i) => i !== index);
    emit();
  }

  function handleRawChange(value) {
    rawQl = value;
    emit();
  }

  function emit() {
    const ql = mode === 'raw' ? rawQl : builtQl;
    const state = mode === 'builder' ? JSON.stringify({ dynamicFields: filters }) : null;
    onchange?.({ ql_query: ql, filter_state: state, mode });
  }

  function testId(suffix) {
    return `${testIdPrefix}-${suffix}`;
  }
</script>

<div class="flex flex-col gap-3" data-testid={testIdPrefix}>
  <div class="flex items-center gap-1" role="tablist" data-testid={testId('modes')}>
    <button
      type="button"
      role="tab"
      aria-selected={mode === 'builder'}
      class="px-3 py-1.5 rounded-md text-sm border transition-colors whitespace-nowrap"
      style={mode === 'builder'
        ? 'color: var(--ds-text); background-color: var(--ds-surface-hovered); border-color: var(--ds-border-bold);'
        : 'color: var(--ds-text-subtle); background-color: transparent; border-color: var(--ds-border);'}
      data-testid={testId('mode-builder')}
      onclick={selectBuilderMode}
    >
      {t('collections.builderMode')}
    </button>
    <button
      type="button"
      role="tab"
      aria-selected={mode === 'raw'}
      class="px-3 py-1.5 rounded-md text-sm border transition-colors whitespace-nowrap"
      style={mode === 'raw'
        ? 'color: var(--ds-text); background-color: var(--ds-surface-hovered); border-color: var(--ds-border-bold);'
        : 'color: var(--ds-text-subtle); background-color: transparent; border-color: var(--ds-border);'}
      data-testid={testId('mode-raw')}
      onclick={selectRawMode}
    >
      {t('collections.rawMode')}
    </button>
  </div>

  {#if mode === 'builder'}
    <div class="flex flex-col gap-2" data-testid={testId('builder')}>
      {#each filters as filter, index (index)}
        <div class="flex items-start gap-2" data-testid={testId(`row-${index}`)}>
          <div class="flex-1 min-w-0">
            <DynamicFieldFilter
              {filter}
              compact={true}
              testIdPrefix={testId(`filter-${index}`)}
              {fieldGroups}
              {customFieldItems}
              onchange={(updated) => updateFilter(index, updated)}
              onremove={() => removeFilter(index)}
            />
          </div>
          <button
            type="button"
            class="p-2 rounded transition-colors"
            style="color: var(--ds-text-subtle);"
            title={t('common.remove')}
            data-testid={testId(`remove-${index}`)}
            onclick={() => removeFilter(index)}
          >
            <X class="w-4 h-4" />
          </button>
        </div>
      {/each}
      <!-- shortcut-guard-exempt: contextual filter-row action inside the builder. -->
      <Button
        dataTestid={testId('add')}
        variant="ghost"
        size="sm"
        icon={Plus}
        onclick={addFilter}
        class="self-start"
      >
        {t('collections.addFieldFilter')}
      </Button>
    </div>
  {:else}
    <div data-testid={testId('raw')}>
      <QlQueryBar
        query={rawQl}
        mode="raw"
        {compact}
        completionCatalog={catalog}
        editorTestId={testId('ql-editor')}
        onquerychange={handleRawChange}
      />
    </div>
  {/if}

  {#if hydrationFailed}
    <p class="text-xs" style="color: var(--ds-text-subtle);" data-testid={testId('hydration-failed')}>
      {t('collections.builderRecoveryDropped')}
    </p>
  {/if}
</div>
