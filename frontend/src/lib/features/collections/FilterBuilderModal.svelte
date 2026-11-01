<script>
  import Plus from '@lucide/svelte/icons/plus';
  import Button from '../../components/Button.svelte';
  import Checkbox from '../../components/Checkbox.svelte';
  import Input from '../../components/Input.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import DynamicFieldFilter from '../items/DynamicFieldFilter.svelte';
  import { t } from '../../stores/i18n.svelte.js';

  // The one place the dynamic filter builder lives. It is opened from the
  // filter menu either to build/apply a filter or to name an already-active
  // filter and decide whether it is personal or shared.
  let {
    isOpen = $bindable(false),
    initialRows = [],
    startInSaveMode = false,
    canShare = false,
    onApply = () => {},
    onSave = async () => ({ ok: true }),
    onClear = () => {},
  } = $props();

  let draftRows = $state([]);
  let saveName = $state('');
  let saveShared = $state(false);
  let showSave = $state(false);
  let error = $state('');
  let busy = $state(false);

  const hasRows = $derived(
    draftRows.some((row) => row.field && (row.value || row.values?.length))
  );

  function blankRow() {
    return { field: null, operator: '=', value: '', values: [] };
  }

  function cloneRows(rows) {
    return JSON.parse(JSON.stringify(rows ?? []));
  }

  // Reset the draft every time the modal opens so it reflects the active
  // filter (or starts blank) instead of the previous session.
  let wasOpen = false;
  $effect(() => {
    if (isOpen && !wasOpen) {
      const rows = cloneRows(initialRows);
      draftRows = rows.length > 0 ? rows : [blankRow()];
      showSave = startInSaveMode;
      saveName = '';
      saveShared = false;
      error = '';
      busy = false;
    }
    wasOpen = isOpen;
  });

  function addRow() {
    draftRows = [...draftRows, blankRow()];
  }

  function updateRow(index, row) {
    draftRows = draftRows.map((current, currentIndex) => (currentIndex === index ? row : current));
  }

  function removeRow(index) {
    draftRows = draftRows.filter((_, currentIndex) => currentIndex !== index);
  }

  function apply() {
    onApply(cloneRows(draftRows));
    isOpen = false;
  }

  function clearAll() {
    draftRows = [blankRow()];
    error = '';
    onClear();
  }

  function startSave() {
    showSave = true;
    error = '';
  }

  async function save() {
    if (!saveName.trim()) {
      error = t('collections.filterNameRequired');
      return;
    }
    if (!hasRows) {
      error = t('collections.noActiveFilters');
      return;
    }
    busy = true;
    const result = await onSave({
      name: saveName.trim(),
      shared: saveShared,
      rows: cloneRows(draftRows),
    });
    busy = false;
    if (result?.ok === false) {
      error = result.error || t('common.error');
      return;
    }
    isOpen = false;
  }

  function submit() {
    if (showSave) void save();
    else apply();
  }
</script>

<Modal
  bind:isOpen
  maxWidth="max-w-3xl"
  dataTestid="filter-builder-modal"
  closeOnBackdropClick={false}
  onSubmit={submit}
  submitDisabled={!hasRows && !showSave}
>
  <div class="p-4">
    <h3 class="mb-3 text-lg font-semibold" style="color: var(--ds-text);">
      {showSave ? t('collections.saveFilter') : t('collections.filters')}
    </h3>

    <div class="flex flex-col gap-2" data-testid="filter-builder-rows">
      {#each draftRows as row, index (index)}
        <DynamicFieldFilter
          filter={row}
          testIdPrefix={`collection-dynamic-filter-${index}`}
          onchange={(updated) => updateRow(index, updated)}
          onremove={() => removeRow(index)}
          onexecute={apply}
        />
      {/each}
    </div>

    <div class="mt-3">
      <Button variant="ghost" size="sm" icon={Plus} onclick={addRow} dataTestid="filter-builder-add">
        {t('common.addFilter')}
      </Button>
    </div>

    {#if showSave}
      <div class="mt-4 border-t pt-3" style="border-color: var(--ds-border);">
        <label
          for="filter-builder-name"
          class="mb-1.5 block text-xs font-medium"
          style="color: var(--ds-text-subtle);"
        >
          {t('collections.filterNamePlaceholder')}
        </label>
        <Input
          id="filter-builder-name"
          type="text"
          bind:value={saveName}
          maxlength={120}
          size="small"
          dataTestid="filter-builder-name"
          placeholder={t('collections.filterNamePlaceholder')}
          class="max-w-sm"
        />
        <div class="mt-3">
          <Checkbox
            bind:checked={saveShared}
            disabled={!canShare}
            dataTestid="filter-builder-shared"
            label={t('collections.shareFilter')}
            hint={canShare ? t('collections.shareFilterHint') : t('collections.viewsReadOnly')}
            size="small"
          />
        </div>
      </div>
    {/if}

    {#if error}
      <p class="mt-3 text-xs" style="color: var(--ds-text-danger);" data-testid="filter-builder-error">
        {error}
      </p>
    {/if}
  </div>

  <div
    class="flex items-center justify-between gap-2 border-t px-4 py-3"
    style="border-color: var(--ds-border);"
  >
    <Button
      variant="ghost"
      size="sm"
      onclick={clearAll}
      disabled={!hasRows}
      dataTestid="filter-builder-clear"
    >
      {t('common.clear')}
    </Button>
    <div class="flex items-center gap-2">
      <Button variant="ghost" size="sm" onclick={() => (isOpen = false)}>
        {t('common.cancel')}
      </Button>
      {#if showSave}
        <Button
          variant="primary"
          size="sm"
          onclick={save}
          disabled={busy || !hasRows}
          dataTestid="filter-builder-save"
        >
          {t('common.save')}
        </Button>
      {:else}
        <Button
          variant="secondary"
          size="sm"
          onclick={startSave}
          disabled={!hasRows}
          dataTestid="filter-builder-save-toggle"
        >
          {t('collections.saveFilter')}
        </Button>
        <Button
          variant="primary"
          size="sm"
          onclick={apply}
          disabled={!hasRows}
          dataTestid="dynamic-filter-apply"
        >
          {t('common.apply')}
        </Button>
      {/if}
    </div>
  </div>
</Modal>
