<script>
  import { collectionEditorOptions } from '../stores/collectionEditorOptions.svelte.js';
  import { objectDisplayName } from '../utils/systemLabels.js';
  import ItemPicker from './ItemPicker.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import { createStatusPickerConfig } from './pickerConfigs.js';
  import { items as itemsApi } from '../api/items.js';

  // Inline status editor shared by the list/tree cells and the item detail
  // link and child rows. Workspace statuses are loaded lazily per owning
  // workspace and used to resolve the current label. When itemId is given,
  // the selectable options are that item's workflow transitions, so a row
  // never offers statuses its item type's workflow does not allow.
  let {
    itemId = null,
    workspaceId,
    statusId,
    statusName = '',
    statusBuiltinKey = '',
    // Category color the row already carries, used until the options load.
    statusColor = null,
    statuses = [],
    statusCategories = [],
    canEdit = true,
    emptyText = '-',
    onselect = undefined,
  } = $props();

  let editorOptions = $derived(collectionEditorOptions.get(workspaceId));
  let selectedStatus = $derived(
    [...editorOptions.statuses, ...statuses].find((s) => s.id === statusId) ||
      (statusName ? { name: statusName, builtin_key: statusBuiltinKey, category_id: null } : null)
  );
  let color = $derived(
    statusCategories.find((sc) => sc.id === selectedStatus?.category_id)?.color ||
      selectedStatus?.category_color ||
      statusColor ||
      '#6b7280'
  );
  let label = $derived(selectedStatus ? objectDisplayName(selectedStatus, 'status') : null);
  const statusConfig = $derived(createStatusPickerConfig(statusCategories));

  // null = use workspace statuses (not loaded, failed, or no workflow).
  let transitionOptions = $state(null);
  let loadingTransitions = $state(false);
  let transitionRequest = 0;

  // Refetched on every open: the allowed moves depend on the item's current
  // status, which can change between opens.
  async function loadTransitions() {
    const request = ++transitionRequest;
    loadingTransitions = true;
    try {
      const result = await itemsApi.getAvailableStatusTransitions(itemId);
      if (request !== transitionRequest) return;
      const available = result?.available_transitions || [];
      // A workflow always returns at least the current status, so an empty
      // list means the item type has no workflow and any status is allowed.
      if (available.length === 0) {
        transitionOptions = null;
        return;
      }
      transitionOptions = available.map((transition) => {
        const known = editorOptions.statuses.find((s) => s.id === transition.id);
        return {
          id: transition.id,
          name: transition.name,
          builtin_key: transition.builtin_key,
          category_color: transition.category_color || null,
          category_id: known?.category_id ?? null,
          display_name: known?.display_name,
        };
      });
    } catch {
      // Keep the workspace list as a fallback; the transition endpoint still
      // rejects moves the workflow does not allow.
      if (request === transitionRequest) transitionOptions = null;
    } finally {
      if (request === transitionRequest) loadingTransitions = false;
    }
  }

  // Item IDs are stable for a row, but a recycled component can be handed a
  // different item; drop options that belong to the previous one.
  $effect(() => {
    void itemId;
    transitionRequest++;
    transitionOptions = null;
    loadingTransitions = false;
  });

  let pickerItems = $derived(transitionOptions ?? editorOptions.statuses);
  let pickerLoading = $derived(
    itemId ? loadingTransitions || (transitionOptions === null && editorOptions.loading.statuses) : editorOptions.loading.statuses
  );

  function handleOpen() {
    collectionEditorOptions.load(workspaceId, 'statuses');
    if (itemId) loadTransitions();
  }
</script>

{#if canEdit}
  <ItemPicker
    value={statusId}
    items={pickerItems}
    loading={pickerLoading}
    config={statusConfig}
    placeholder="Set status"
    showUnassigned={false}
    allowClear={false}
    onOpen={handleOpen}
    onSelect={async (selected) => {
      const nextStatusId = selected?.id;
      if (nextStatusId && nextStatusId !== statusId) {
        await onselect?.(nextStatusId);
      }
    }}
  >
    {#snippet children()}
      <span class="cursor-pointer">
        <Lozenge text={label ?? 'Set status'} customBg={color} />
      </span>
    {/snippet}
  </ItemPicker>
{:else}
  <Lozenge text={label ?? emptyText} customBg={color} />
{/if}
