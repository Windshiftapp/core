<script>
  import { collectionEditorOptions } from '../stores/collectionEditorOptions.svelte.js';
  import { objectDisplayName } from '../utils/systemLabels.js';
  import ItemPicker from './ItemPicker.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import { createStatusPickerConfig } from './pickerConfigs.js';
  import { items as itemsApi } from '../api/items.js';
  import TransitionScreenModal from '../dialogs/TransitionScreenModal.svelte';
  import { surfaceError } from '../stores/toasts.svelte.js';

  // Inline status editor shared by the list/tree cells and the item detail
  // link and child rows. Workspace statuses are loaded lazily per owning
  // workspace and used to resolve the current label. When itemId is given,
  // the selectable options are that item's workflow transitions, so a row
  // never offers statuses its item type's workflow does not allow. A move
  // whose transition carries a screen opens the shared modal first and
  // submits its fields with the move (WI-1835).
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

  // A chosen move whose transition carries a screen pauses on the modal until
  // its fields are collected, then continues through onselect.
  let screenMove = $state(null);
  let showScreen = $state(false);
  let preparingScreen = $state(false);
  let screenRequest = 0;

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
          // Present when the governing transition opens a screen before the
          // move; the picker collects its fields first.
          screen: transition.screen || null,
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
    screenRequest++;
    transitionOptions = null;
    loadingTransitions = false;
    showScreen = false;
    preparingScreen = false;
    screenMove = null;
  });

  let pickerItems = $derived(transitionOptions ?? editorOptions.statuses);
  let pickerLoading = $derived(
    itemId ? loadingTransitions || (transitionOptions === null && editorOptions.loading.statuses) : editorOptions.loading.statuses
  );

  function handleOpen() {
    collectionEditorOptions.load(workspaceId, 'statuses');
    if (itemId) loadTransitions();
  }

  // A screen-bearing move needs the item's stored values to seed the form, so
  // load the full row first (list and tree rows carry only a summary). A failed
  // load still opens the screen; only the seed values are lost.
  async function openTransitionScreen(option) {
    const request = ++screenRequest;
    preparingScreen = true;
    let sourceItem = null;
    try {
      if (itemId) {
        try {
          sourceItem = await itemsApi.get(itemId);
        } catch (err) {
          surfaceError(err, { context: 'loadInlineTransitionItem', silent: true });
        }
      }
      if (request !== screenRequest) return;
      // Milestone and iteration custom fields resolve against the owning
      // workspace's directories.
      collectionEditorOptions.load(workspaceId, 'milestones');
      collectionEditorOptions.load(workspaceId, 'iterations');
      screenMove = {
        statusId: option.id,
        screen: option.screen,
        statusName: objectDisplayName(option, 'status'),
        item: sourceItem,
      };
      showScreen = true;
    } finally {
      if (request === screenRequest) preparingScreen = false;
    }
  }

  async function handleSelect(selected) {
    const nextStatusId = selected?.id;
    if (!nextStatusId || nextStatusId === statusId) return;
    if (selected.screen) {
      await openTransitionScreen(selected);
      return;
    }
    await onselect?.(nextStatusId);
  }

  async function handleScreenConfirm(fields) {
    if (!screenMove) return;
    // Errors propagate back to the modal so it can render screen_required
    // missing fields inline instead of a toast.
    await onselect?.(screenMove.statusId, fields);
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
    disabled={preparingScreen}
    onOpen={handleOpen}
    onSelect={handleSelect}
  >
    {#snippet children()}
      <span class="cursor-pointer" data-testid="item-status-picker-trigger">
        <Lozenge text={label ?? 'Set status'} customBg={color} />
      </span>
    {/snippet}
  </ItemPicker>
{:else}
  <Lozenge text={label ?? emptyText} customBg={color} />
{/if}

{#if showScreen && screenMove}
  <TransitionScreenModal
    bind:show={showScreen}
    screen={screenMove.screen}
    targetStatusName={screenMove.statusName}
    workspaceId={workspaceId}
    milestones={editorOptions.milestones}
    iterations={editorOptions.iterations}
    itemLabels={screenMove.item?.labels || []}
    item={screenMove.item}
    onconfirm={handleScreenConfirm}
    oncancel={() => (screenMove = null)}
  />
{/if}
