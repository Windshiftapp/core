<script>
  import { Filter, X } from '@lucide/svelte';
  import { useDebounce } from 'runed';
  import SearchInput from '../../components/SearchInput.svelte';
  import Select from '../../components/Select.svelte';
  import Button from '../../components/Button.svelte';
  import FormField from '../../components/FormField.svelte';
  import UserPicker from '../../pickers/UserPicker.svelte';
  import PageLabelPicker from '../pages/PageLabelPicker.svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { requirementTypeOptions } from './requirementTypes.js';
  import { requirementStatusOptions } from './requirementStatuses.js';

  let {
    workspaceId,
    searchQuery = $bindable(''),
    filterType = $bindable(''),
    filterStatus = $bindable(''),
    filterOwnerId = $bindable(null),
    filterItemLinks = $bindable(''),
    filterTestLinks = $bindable(''),
    selectedLabelIds = $bindable(new Set()),
    onchange = () => {},
  } = $props();

  let showFilters = $state(false);
  let draftType = $state('');
  let draftStatus = $state('');
  let draftOwnerId = $state(null);
  let draftItemLinks = $state('');
  let draftTestLinks = $state('');
  let draftLabelIds = $state(new Set());

  const debouncedSearchApply = useDebounce(() => onchange(), 300);

  const typeOptions = $derived([
    { value: '', label: t('requirements.filters.allTypes') },
    ...requirementTypeOptions(t),
  ]);
  const statusOptions = $derived([
    { value: '', label: t('requirements.filters.allStatuses') },
    ...requirementStatusOptions(t),
  ]);
  const linkFilterOptions = $derived([
    { value: '', label: t('requirements.filters.anyLinks') },
    { value: 'true', label: t('requirements.filters.hasLinks') },
    { value: 'false', label: t('requirements.filters.noLinks') },
  ]);
  const testLinkFilterOptions = $derived([
    { value: '', label: t('requirements.filters.anyCoverage') },
    { value: 'true', label: t('requirements.filters.hasTestLinks') },
    { value: 'false', label: t('requirements.filters.uncovered') },
  ]);

  const activeFilterCount = $derived(
    [
      filterType,
      filterStatus,
      filterOwnerId,
      filterItemLinks,
      filterTestLinks,
      ...selectedLabelIds,
    ].filter(Boolean).length
  );

  const draftFilterCount = $derived(
    [
      draftType,
      draftStatus,
      draftOwnerId,
      draftItemLinks,
      draftTestLinks,
      ...draftLabelIds,
    ].filter(Boolean).length
  );

  function syncDraftFromApplied() {
    draftType = filterType;
    draftStatus = filterStatus;
    draftOwnerId = filterOwnerId;
    draftItemLinks = filterItemLinks;
    draftTestLinks = filterTestLinks;
    draftLabelIds = new Set(selectedLabelIds);
  }

  function toggleFilters() {
    if (!showFilters) syncDraftFromApplied();
    showFilters = !showFilters;
  }

  function closeFilters() {
    showFilters = false;
  }

  function applyDraft() {
    filterType = draftType;
    filterStatus = draftStatus;
    filterOwnerId = draftOwnerId;
    filterItemLinks = draftItemLinks;
    filterTestLinks = draftTestLinks;
    selectedLabelIds = new Set(draftLabelIds);
    showFilters = false;
    onchange();
  }

  function clearDraft() {
    draftType = '';
    draftStatus = '';
    draftOwnerId = null;
    draftItemLinks = '';
    draftTestLinks = '';
    draftLabelIds = new Set();
    filterType = '';
    filterStatus = '';
    filterOwnerId = null;
    filterItemLinks = '';
    filterTestLinks = '';
    selectedLabelIds = new Set();
    showFilters = false;
    onchange();
  }

  function onDraftLabelToggle(label) {
    const next = new Set(draftLabelIds);
    if (next.has(label.id)) next.delete(label.id);
    else next.add(label.id);
    draftLabelIds = next;
  }
</script>

<div class="requirements-toolbar" data-testid="requirements-filters">
  <SearchInput
    bind:value={searchQuery}
    placeholder={t('requirements.filters.search')}
    class="requirements-toolbar__search"
    dataTestid="requirements-filter-search"
    on_input={() => debouncedSearchApply()}
  />

  <div class="requirements-toolbar__filter">
    <Button
      variant={activeFilterCount > 0 ? 'selected' : 'ghost'}
      size="medium"
      icon={Filter}
      dataTestid="requirements-filter-toggle"
      onclick={toggleFilters}
    >
      <span>{t('common.filter')}</span>
      {#if activeFilterCount > 0}
        <span class="requirements-toolbar__badge">{activeFilterCount}</span>
      {/if}
    </Button>

    {#if showFilters}
      <div
        class="requirements-filter-popover"
        data-testid="requirements-filter-panel"
        role="dialog"
        aria-label={t('common.filter')}
      >
        <div class="requirements-filter-popover__fields">
          <FormField label={t('requirements.fieldType')}>
            <Select bind:value={draftType} options={typeOptions} size="small" />
          </FormField>
          <FormField label={t('requirements.fieldStatus')}>
            <Select bind:value={draftStatus} options={statusOptions} size="small" />
          </FormField>
          <FormField label={t('requirements.fieldOwner')}>
            <UserPicker bind:value={draftOwnerId} {workspaceId} />
          </FormField>
          <FormField label={t('requirements.columnLinkedItems')}>
            <Select bind:value={draftItemLinks} options={linkFilterOptions} size="small" />
          </FormField>
          <FormField label={t('requirements.columnTests')}>
            <Select bind:value={draftTestLinks} options={testLinkFilterOptions} size="small" />
          </FormField>
          <FormField label={t('requirements.filters.labels')}>
            <PageLabelPicker
              {workspaceId}
              selectedIds={draftLabelIds}
              allowCreate={false}
              triggerLabel={t('requirements.filters.labels')}
              triggerTestid="requirements-filter-labels"
              onToggle={onDraftLabelToggle}
            />
          </FormField>
        </div>

        <div class="requirements-filter-popover__actions">
          {#if draftFilterCount > 0 || activeFilterCount > 0}
            <Button variant="ghost" size="sm" icon={X} onclick={clearDraft}>
              {t('common.clear')}
            </Button>
          {/if}
          <Button variant="primary" size="sm" dataTestid="requirements-filter-apply" onclick={applyDraft}>
            {t('common.apply')}
          </Button>
        </div>
      </div>
    {/if}
  </div>
</div>

{#if showFilters}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="requirements-filter-backdrop" onmousedown={closeFilters}></div>
{/if}

<style>
  .requirements-toolbar {
    display: flex;
    align-items: center;
    gap: 1rem;
    margin-bottom: 1rem;
    min-width: 0;
  }

  .requirements-toolbar__search {
    min-width: 0;
    flex: 1;
    max-width: 28rem;
  }

  .requirements-toolbar__filter {
    position: relative;
    flex-shrink: 0;
  }

  .requirements-toolbar__badge {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 1.25rem;
    height: 1.25rem;
    padding: 0 0.35rem;
    border-radius: 999px;
    font-size: 0.75rem;
    font-weight: 600;
    background: color-mix(in srgb, white 25%, transparent);
  }

  .requirements-filter-popover {
    position: absolute;
    left: 0;
    top: calc(100% + 0.5rem);
    z-index: 20;
    width: min(22rem, calc(100vw - 2rem));
    border: 1px solid var(--ds-border);
    border-radius: 0.5rem;
    background: var(--ds-surface-overlay);
    box-shadow: 0 10px 25px -5px rgba(0, 0, 0, 0.25);
    padding: 0.75rem;
  }

  .requirements-filter-popover__fields {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    max-height: min(60vh, 28rem);
    overflow-y: auto;
    padding-inline: 2px;
  }

  .requirements-filter-popover__fields :global(.mb-4:last-child) {
    margin-bottom: 0;
  }

  .requirements-filter-popover__actions {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 0.5rem;
    margin-top: 0.75rem;
    padding-top: 0.75rem;
    border-top: 1px solid var(--ds-border);
  }

  .requirements-filter-backdrop {
    position: fixed;
    inset: 0;
    z-index: 10;
  }
</style>
