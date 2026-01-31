<script>
  import { onMount, createEventDispatcher } from 'svelte';
  import { api } from '../../api.js';
  import { attachmentStatus } from '../../stores';
  import Modal from '../../dialogs/Modal.svelte';
  import Comments from '../items/Comments.svelte';
  import ItemDetailDescription from '../items/ItemDetailDescription.svelte';
  import { X, Calendar, MessageSquare } from 'lucide-svelte';
  import Button from '../../components/Button.svelte';
  import Checkbox from '../../components/Checkbox.svelte';
  import { itemTypeIconMap } from '../../utils/icons.js';
  import { formatDate } from '../../utils/dateFormatter.js';
  import { navigate } from '../../router.js';
  import ItemDetailBreadcrumbs from '../items/ItemDetailBreadcrumbs.svelte';
  import { t } from '../../stores/i18n.svelte.js';

  // Use centralized icon map for work item types
  const iconMap = itemTypeIconMap;

  const dispatch = createEventDispatcher();

  let {
    itemId,
    workspaceId,
    statuses = [],
    isModal = true
  } = $props();

  // Load statuses if not provided (for full-page mode)
  let localStatuses = $state([]);

  // Core state
  let item = $state(null);
  let loading = $state(true);
  let saving = $state(false);
  let error = $state(null);

  // Editing states
  let editingTitle = $state(false);
  let editTitle = $state('');
  let editingDescription = $state(false);
  let editDescription = $state('');
  let editingDueDate = $state(false);


  // Attachment state
  let attachments = $state([]);

  // Comment count for badge
  let commentCount = $state(0);

  // Breadcrumbs state (for ItemDetailBreadcrumbs component)
  let workspace = $state(null);
  let parentHierarchy = $state([]);
  let currentItemType = $state(null);
  let currentHierarchyLevel = $state(null);
  let itemTypes = $state([]);

  // Use provided statuses or fall back to locally loaded ones
  let effectiveStatuses = $derived(statuses.length > 0 ? statuses : localStatuses);

  onMount(async () => {
    // Load statuses if not provided (full-page mode)
    if (statuses.length === 0) {
      await loadStatuses();
    }
    await loadItem();

    // Load breadcrumbs data for full-page mode
    await loadWorkspace();
    if (item?.parent_id) {
      await loadParentHierarchy();
    }
    await loadItemTypeData();

    if (attachmentStatus.enabled) {
      await loadAttachments();
    }
    loading = false;
  });

  async function loadStatuses() {
    try {
      localStatuses = await api.statuses.getAll();
    } catch (err) {
      console.error('Failed to load statuses:', err);
      localStatuses = [];
    }
  }

  async function loadItem() {
    try {
      item = await api.items.get(itemId);
      editTitle = item.title || '';
      editDescription = item.description || '';
    } catch (err) {
      console.error('Failed to load item:', err);
      error = 'Failed to load task';
    }
  }

  async function loadAttachments() {
    try {
      const response = await api.attachments.getByItem(itemId);
      attachments = response?.attachments || response || [];
    } catch (err) {
      console.error('Failed to load attachments:', err);
      attachments = [];
    }
  }

  // Breadcrumbs data loading functions
  async function loadWorkspace() {
    try {
      workspace = await api.workspaces.get(workspaceId);
    } catch (err) {
      console.error('Failed to load workspace:', err);
    }
  }

  async function loadParentHierarchy() {
    if (!item?.parent_id) {
      parentHierarchy = [];
      return;
    }
    try {
      const ancestors = await api.items.getAncestors(item.id);
      const itemTypesData = await api.itemTypes.getAll();
      parentHierarchy = ancestors.map(ancestor => {
        if (ancestor.item_type_id) {
          const itemType = itemTypesData.find(type => type.id === ancestor.item_type_id);
          return { ...ancestor, itemType };
        }
        return ancestor;
      });
    } catch (err) {
      console.error('Failed to load parent hierarchy:', err);
      parentHierarchy = [];
    }
  }

  async function loadItemTypeData() {
    try {
      const [itemTypesData, hierarchyLevels] = await Promise.all([
        api.itemTypes.getAll(),
        api.hierarchyLevels.getAll()
      ]);
      itemTypes = itemTypesData || [];
      if (item?.item_type_id) {
        currentItemType = itemTypes.find(type => type.id === item.item_type_id);
        if (currentItemType) {
          currentHierarchyLevel = hierarchyLevels.find(level => level.level === currentItemType.hierarchy_level);
        }
      }
    } catch (err) {
      console.error('Failed to load item type data:', err);
      currentItemType = null;
      currentHierarchyLevel = null;
    }
  }

  // Done toggle logic (from TodoList)
  function isTaskCompleted() {
    const status = effectiveStatuses.find(s => s.id === item?.status_id);
    return status?.category_name === 'Done' ||
           status?.name?.toLowerCase().includes('complete') ||
           status?.name?.toLowerCase().includes('done');
  }

  async function toggleDone() {
    try {
      saving = true;
      let targetStatusId;

      if (isTaskCompleted()) {
        // Move to "Open" or first non-done status
        const openStatus = effectiveStatuses.find(s => s.name?.toLowerCase() === 'open') ||
                          effectiveStatuses.find(s => s.category_name !== 'Done') ||
                          effectiveStatuses[0];
        targetStatusId = openStatus?.id;
      } else {
        // Move to "Done" status
        const doneStatus = effectiveStatuses.find(s => s.category_name === 'Done') ||
                          effectiveStatuses.find(s => s.name?.toLowerCase().includes('done')) ||
                          effectiveStatuses.find(s => s.name?.toLowerCase().includes('complete'));
        targetStatusId = doneStatus?.id;
      }

      if (targetStatusId) {
        await api.items.update(item.id, { status_id: targetStatusId });
        item = { ...item, status_id: targetStatusId };
        dispatch('update');
      }
    } catch (err) {
      console.error('Failed to toggle done status:', err);
      error = err.message;
    } finally {
      saving = false;
    }
  }

  async function saveTitle() {
    if (!editTitle.trim() || editTitle === item.title) {
      editingTitle = false;
      editTitle = item.title || '';
      return;
    }

    try {
      saving = true;
      await api.items.update(item.id, { title: editTitle.trim() });
      item = { ...item, title: editTitle.trim() };
      editingTitle = false;
      dispatch('update');
    } catch (err) {
      console.error('Failed to save title:', err);
      error = err.message;
    } finally {
      saving = false;
    }
  }

  async function saveDescription() {
    try {
      saving = true;
      await api.items.update(item.id, { description: editDescription });
      item = { ...item, description: editDescription };
      editingDescription = false;
      dispatch('update');
    } catch (err) {
      console.error('Failed to save description:', err);
      error = err.message;
    } finally {
      saving = false;
    }
  }

  async function saveDueDate(newValue) {
    try {
      saving = true;
      const dueDate = newValue || null;
      await api.items.update(item.id, { due_date: dueDate });
      item = { ...item, due_date: dueDate };
      editingDueDate = false;
      dispatch('update');
    } catch (err) {
      console.error('Failed to save due date:', err);
      error = err.message;
    } finally {
      saving = false;
    }
  }

  function clearDueDate(e) {
    e.stopPropagation();
    saveDueDate(null);
  }

  function handleTitleKeydown(e) {
    if (e.key === 'Enter') {
      e.preventDefault();
      saveTitle();
    } else if (e.key === 'Escape') {
      editingTitle = false;
      editTitle = item.title || '';
    }
  }

  function cancelDescription() {
    editingDescription = false;
    editDescription = item.description || '';
  }

  function handleSaveField(event) {
    const { field, value } = event.detail;
    if (field === 'description') {
      editDescription = value;
      saveDescription();
    }
  }

  function handleCancelEdit(event) {
    const { field } = event.detail;
    if (field === 'description') {
      cancelDescription();
    }
  }

  async function handleAttachmentUploadFiles(event) {
    const { files } = event.detail;
    for (const file of files) {
      try {
        const formData = new FormData();
        formData.append('file', file);
        formData.append('item_id', item.id);
        await api.attachments.upload(formData);
      } catch (err) {
        console.error('Failed to upload:', err);
      }
    }
    await loadAttachments();
  }

  async function handleAttachmentDelete(event) {
    const attachment = event.detail;
    try {
      await api.attachments.delete(attachment.id);
      attachments = attachments.filter(a => a.id !== attachment.id);
    } catch (err) {
      console.error('Failed to delete attachment:', err);
      error = 'Failed to delete attachment';
    }
  }

  function handleCommentsLoaded(data) {
    commentCount = data.count;
  }

  function closeModal() {
    if (isModal) {
      dispatch('close');
    } else {
      // Full-page mode: navigate back
      window.history.back();
    }
  }

</script>

{#snippet taskContent()}
  {#if loading}
    <div class="p-8 text-center" style="color: var(--ds-text-subtle);">{t('nav.loading')}</div>
  {:else if error && !item}
    <div class="p-8 text-center text-red-600">{error}</div>
  {:else if item}
    {#if !isModal && workspace}
      <!-- Breadcrumbs for full-page mode using ItemDetailBreadcrumbs -->
      <div class="px-6 pt-6">
        <ItemDetailBreadcrumbs
          {workspace}
          {parentHierarchy}
          {currentItemType}
          {currentHierarchyLevel}
          {item}
          {iconMap}
          {workspaceId}
          onnavigate={(e) => navigate(e.detail.path)}
          onparent-changed={loadParentHierarchy}
          oncopy-key={() => {
            navigator.clipboard.writeText(`${workspace?.key || 'WORK'}-${item.workspace_item_number}`);
          }}
        />
      </div>
    {/if}

    <!-- Header with Done checkbox and Title -->
    <div class="flex items-center justify-between {isModal ? 'p-4' : 'px-6 py-4'} border-b" style="border-color: var(--ds-border);">
      <div class="flex items-center gap-3 flex-1 min-w-0">
        <!-- Done Checkbox -->
        <Checkbox
          checked={isTaskCompleted()}
          onchange={toggleDone}
          disabled={saving}
          size="medium"
          class="task-complete-checkbox"
        />

        <!-- Editable Title -->
        {#if editingTitle}
          <!-- svelte-ignore a11y_autofocus -->
          <input
            type="text"
            bind:value={editTitle}
            onblur={saveTitle}
            onkeydown={handleTitleKeydown}
            class="flex-1 text-lg font-semibold border-b-2 focus:outline-none min-w-0"
            style="background: transparent; color: var(--ds-text); border-color: #3b82f6;"
            autofocus
          />
        {:else}
          <button
            type="button"
            onclick={() => { editingTitle = true; }}
            class="flex-1 text-lg font-semibold cursor-pointer hover:opacity-70 truncate text-left {isTaskCompleted() ? 'line-through opacity-60' : ''}"
            style="color: var(--ds-text); background: none; border: none; padding: 0;"
          >
            {item.title}
          </button>
        {/if}
      </div>

      {#if isModal}
        <Button variant="ghost" icon={X} onclick={closeModal} title={t('common.close')} />
      {/if}
    </div>

    <!-- Body -->
    <div class="{isModal ? 'p-4 max-h-[70vh] overflow-y-auto' : 'px-6 py-6'}">
      <!-- Due Date -->
      <div class="mb-4">
        {#if editingDueDate}
          <div class="flex items-center gap-2">
            <Calendar class="w-4 h-4" style="color: var(--ds-text-subtle);" />
            <!-- svelte-ignore a11y_autofocus -->
            <input
              type="date"
              value={item.due_date?.split('T')[0] || ''}
              onchange={(e) => saveDueDate(e.target.value || null)}
              onblur={() => editingDueDate = false}
              class="px-2 py-1 border rounded text-sm"
              style="background-color: var(--ds-surface); color: var(--ds-text); border-color: var(--ds-border);"
              autofocus
            />
          </div>
        {:else}
          <div class="flex items-center gap-2">
            <button
              type="button"
              onclick={() => editingDueDate = true}
              class="flex items-center gap-2 text-sm hover:opacity-70"
              style="color: {item.due_date ? 'var(--ds-text)' : 'var(--ds-text-subtle)'}; background: none; border: none; padding: 0;"
            >
              <Calendar class="w-4 h-4" />
              {item.due_date ? formatDate(item.due_date) : t('personal.setDueDate')}
            </button>
            {#if item.due_date}
              <button
                type="button"
                onclick={clearDueDate}
                class="p-0.5 hover:opacity-70 rounded"
                style="color: var(--ds-text-subtle);"
              >
                <X class="w-3 h-3" />
              </button>
            {/if}
          </div>
        {/if}
      </div>

      <!-- Description -->
      <div class="mb-6">
        <ItemDetailDescription
          {item}
          bind:editingDescription
          bind:editDescription
          {saving}
          availableSubIssueTypes={[]}
          showLinkButton={false}
          {attachments}
          diagrams={[]}
          on:save-field={handleSaveField}
          on:cancel-edit={handleCancelEdit}
          on:attachment-upload-files={handleAttachmentUploadFiles}
          on:attachment-delete={handleAttachmentDelete}
        />
      </div>

      <!-- Comments Section -->
      <div class="border-b mb-4 pb-2" style="border-color: var(--ds-border);">
        <div class="flex items-center gap-1.5 text-sm font-medium" style="color: var(--ds-text-subtle);">
          <MessageSquare class="w-4 h-4" />
          {t('personal.comments')}
          {#if commentCount > 0}
            <span class="text-xs px-1.5 py-0.5 rounded-full" style="background-color: var(--ds-surface-raised);">{commentCount}</span>
          {/if}
        </div>
      </div>
      <Comments itemId={item.id} isPersonalWorkspace={true} onCommentsLoaded={handleCommentsLoaded} />
    </div>
  {/if}
{/snippet}

{#if isModal}
  <Modal isOpen={true} onclose={closeModal} maxWidth="max-w-2xl">
    {@render taskContent()}
  </Modal>
{:else}
  <!-- Full-page mode -->
  <div class="min-h-[calc(100vh-64px)]" style="background-color: var(--ds-surface);">
    <div class="max-w-4xl mx-auto" style="background-color: var(--ds-surface);">
      {@render taskContent()}
    </div>
  </div>
{/if}
