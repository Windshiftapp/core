<script>
  import { onMount } from 'svelte';
  import Plus from '@lucide/svelte/icons/plus';
  import Edit from '@lucide/svelte/icons/edit';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import Button from '../components/Button.svelte';
  import DataTable from '../components/DataTable.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import EntityFormModal from './EntityFormModal.svelte';
  import EntityRowActions from './EntityRowActions.svelte';
  import { confirm } from '../composables/useConfirm.js';
  import { t } from '../stores/i18n.svelte.js';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';
  import './settings-form.css';

  /**
   * Shared list / search / modal / confirm scaffold for the settings CRUD pages.
   *
   * Owns the load, save, delete and form-open state machine and the surrounding
   * chrome. The page keeps its own form state and supplies its columns, row
   * cells, form fields and API calls; the form body stays a snippet because
   * every entity has bespoke fields.
   *
   * @type {{
   *   icon?: any,
   *   title?: string,
   *   subtitle?: string,
   *   count?: string | null,
   *   addLabel?: string,
   *   addId?: string,
   *   addTestid?: string,
   *   hotkeyScope?: string,
   *   emptyMessage?: string,
   *   emptyDescription?: string,
   *   emptyIcon?: any,
   *   columns?: any[],
   *   load: () => Promise<any[]>,
   *   create?: () => Promise<any>,
   *   update?: (id: any) => Promise<any>,
   *   remove?: (item: any) => Promise<any>,
   *   onOpenCreate?: (items: any[]) => void,
   *   onOpenEdit?: (item: any) => void,
   *   confirmDeleteMessage: (item: any) => string,
   *   onValidate?: () => string | null,
   *   actionItems?: ((item: any, helpers: { edit: (item: any) => void, remove: (item: any) => void, items: any[], reload: () => void }) => any[]) | null,
   *   actionTriggerTestid?: ((item: any) => string) | null,
   *   confirmTestid?: string,
   *   errorTestid?: string,
   *   canActOn?: (item: any) => boolean,
   *   rowTestid?: (item: any) => string,
   *   rowAttrs?: (item: any) => Record<string, any>,
   *   actionTestidPrefix?: string,
   *   createTitle?: string,
   *   editTitle?: string,
   *   createSubmitId?: string,
   *   editSubmitId?: string,
   *   submitDisabled?: () => boolean,
   *   onAfterSave?: () => void,
   *   onAfterDelete?: () => void,
   *   onError?: (err: any, phase: 'load' | 'save' | 'delete', item: any) => boolean | void,
   *   errorLabels?: { load?: string, save?: string, delete?: string },
   *   formFields: import('svelte').Snippet,
   *   actionsCell?: import('svelte').Snippet<[any, any]> | null,
   *   cellSnippets?: Record<string, import('svelte').Snippet<[any]>>,
   *   extraData?: any,
   * }} props
   */
  let {
    icon = undefined,
    title = '',
    subtitle = '',
    count = null,
    addLabel = '',
    addId = undefined,
    addTestid = undefined,
    hotkeyScope = undefined,
    emptyMessage = '',
    emptyDescription = '',
    emptyIcon = icon,
    columns = [],
    load,
    create = undefined,
    update = undefined,
    remove = undefined,
    onOpenCreate = undefined,
    onOpenEdit = undefined,
    confirmDeleteMessage,
    onValidate = undefined,
    actionItems = null,
    actionTriggerTestid = null,
    confirmTestid = 'entity-form-confirm',
    errorTestid = undefined,
    canActOn = () => true,
    rowTestid = undefined,
    rowAttrs = undefined,
    actionTestidPrefix = 'entity',
    createTitle = t('common.create'),
    editTitle = t('common.edit'),
    createSubmitId = undefined,
    editSubmitId = undefined,
    submitDisabled = () => false,
    onAfterSave = undefined,
    onAfterDelete = undefined,
    onError = undefined,
    errorLabels = {},
    formFields,
    actionsCell = null,
    cellSnippets = {},
    extraData = undefined,
  } = $props();

  let items = $state([]);
  let loading = $state(true);
  let saving = $state(false);
  let error = $state(null);
  let showForm = $state(false);
  let editingItem = $state(null);

  function reportError(err, phase) {
    // Returning false lets the scaffold fall back to its own error banner.
    if (onError && onError(err, phase, editingItem) !== false) {
      return;
    }
    const fallback = {
      load: t('dialogs.alerts.failedToLoad', { error: title }),
      save: t('dialogs.alerts.failedToSave', { error: title }),
      delete: t('dialogs.alerts.failedToDelete', { error: title }),
    }[phase];
    error = err?.message || errorLabels[phase] || fallback;
  }

  async function reload() {
    try {
      loading = true;
      error = null;
      items = (await load()) ?? [];
    } catch (err) {
      reportError(err, 'load');
    } finally {
      loading = false;
    }
  }

  onMount(reload);

  function startCreate() {
    editingItem = null;
    onOpenCreate?.(items);
    showForm = true;
  }

  function startEdit(item) {
    editingItem = item;
    onOpenEdit?.(item);
    showForm = true;
  }

  function closeForm() {
    showForm = false;
    editingItem = null;
  }

  async function save() {
    if (saving || submitDisabled()) return;
    const validationError = onValidate?.();
    if (validationError) {
      error = validationError;
      return;
    }
    saving = true;
    try {
      if (editingItem) {
        await update?.(editingItem.id);
      } else {
        await create?.();
      }
      await reload();
      closeForm();
      onAfterSave?.();
    } catch (err) {
      reportError(err, 'save');
    } finally {
      saving = false;
    }
  }

  async function deleteItem(item) {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: confirmDeleteMessage(item),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger',
    });
    if (!confirmed) return;
    try {
      await remove?.(item);
      await reload();
      onAfterDelete?.();
    } catch (err) {
      reportError(err, 'delete');
    }
  }

  // Pages that keep their own row dropdown still need the scaffold's open/delete
  // flow, so the builder receives the helpers.
  const resolvedRowAttrs = $derived((item) => ({
    ...(rowTestid ? { 'data-testid': rowTestid(item) } : {}),
    ...(rowAttrs ? rowAttrs(item) : {}),
  }));

  const resolvedActionItems = $derived(
    actionItems
      ? (item) => actionItems(item, { edit: startEdit, remove: deleteItem, items, reload })
      : null
  );

  function defaultActions(item) {
    return [
      {
        id: 'edit',
        icon: Edit,
        title: t('common.edit'),
        testId: `${actionTestidPrefix}-edit-${item.id}`,
        onclick: () => startEdit(item),
      },
      {
        id: 'delete',
        icon: Trash2,
        title: t('common.delete'),
        testId: `${actionTestidPrefix}-delete-${item.id}`,
        danger: true,
        onclick: () => deleteItem(item),
      },
    ];
  }
</script>

<PageHeader {icon} {title} {subtitle} {count}>
  {#snippet actions()}
    <Button
      id={addId}
      variant="primary"
      icon={Plus}
      onclick={startCreate}
      disabled={loading}
      dataTestid={addTestid}
      keyboardHint="A"
      hotkeyConfig={hotkeyScope
        ? { key: toHotkeyString(hotkeyScope, 'add'), guard: () => !showForm }
        : undefined}
    >
      {addLabel}
    </Button>
  {/snippet}
</PageHeader>

{#if error}
  <div class="error" data-testid={errorTestid}>{error}</div>
{/if}

<DataTable
  {columns}
  data={items}
  keyField="id"
  {emptyMessage}
  {emptyDescription}
  {emptyIcon}
  rowAttrs={resolvedRowAttrs}
  actionItems={resolvedActionItems}
  {actionTriggerTestid}
  {...cellSnippets}
>
  {#snippet actions(item)}
    {#if actionsCell}
      {@render actionsCell(item, {
        edit: startEdit,
        remove: deleteItem,
        saving,
        extraData,
        items,
        reload,
      })}
    {:else if canActOn(item)}
      <EntityRowActions actions={defaultActions(item)} />
    {/if}
  {/snippet}
</DataTable>

{#if showForm}
  <EntityFormModal
    title={editingItem ? editTitle : createTitle}
    onclose={closeForm}
    onsubmit={save}
    disabled={submitDisabled()}
    {saving}
    confirmLabel={editingItem ? t('common.update') : t('common.create')}
    submitId={editingItem ? editSubmitId : createSubmitId}
    {confirmTestid}
  >
    {#snippet fields()}
      {@render formFields()}
    {/snippet}
  </EntityFormModal>
{/if}
