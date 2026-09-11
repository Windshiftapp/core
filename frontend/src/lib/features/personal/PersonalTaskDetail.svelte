<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { attachmentStatus } from '../../stores';
  import { workspaceDataStore } from '../../stores/workspaceDataStore.svelte.js';
  import Modal from '../../dialogs/Modal.svelte';
  import Comments from '../items/Comments.svelte';
  import ItemDetailDescription from '../items/ItemDetailDescription.svelte';
  import { X, Calendar, MessageSquare, ExternalLink } from '@lucide/svelte';
  import Button from '../../components/Button.svelte';
  import Checkbox from '../../components/Checkbox.svelte';
  import Input from '../../components/Input.svelte';
  import { copyToClipboard } from '../../utils/clipboard.js';
  import { formatDateOnly } from '../../utils/dateFormatter.js';
  import { navigate } from '../../router.js';
  import ItemDetailBreadcrumbs from '../items/ItemDetailBreadcrumbs.svelte';
  import { t } from '../../stores/i18n.svelte.js';

  let {
    itemId,
    workspaceId,
    isModal = true,
    onclose = null,
    onupdate = null
  } = $props();

  const STATUS_ID_OPEN = 1;
  const STATUS_ID_DONE = 3;

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

  onMount(async () => {
    const initialLoads = [loadItem()];
    if (!isModal) initialLoads.push(loadWorkspace());
    if (attachmentStatus.enabled) initialLoads.push(loadAttachments());
    await Promise.all(initialLoads);

    if (!isModal) await loadHierarchyData();
    loading = false;
  });

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
      attachments = response?.data || [];
    } catch (err) {
      console.error('Failed to load attachments:', err);
      attachments = [];
    }
  }

  // Breadcrumbs data loading functions
  async function loadWorkspace() {
    try {
      await workspaceDataStore.initialize(workspaceId);
      workspace = workspaceDataStore.workspace;
    } catch (err) {
      console.error('Failed to load workspace:', err);
    }
  }

  async function loadHierarchyData() {
    try {
      const [itemTypesData, hierarchyLevels, ancestors] = await Promise.all([
        Promise.resolve(workspaceDataStore.itemTypes),
        api.hierarchyLevels.getAll(),
        item?.parent_id ? api.items.getAncestors(item.id) : Promise.resolve([]),
      ]);
      itemTypes = itemTypesData || [];
      parentHierarchy = (ancestors || []).map((ancestor) => {
        if (!ancestor.item_type_id) return ancestor;
        const itemType = itemTypes.find((type) => type.id === ancestor.item_type_id);
        return { ...ancestor, itemType };
      });
      if (item?.item_type_id) {
        currentItemType = itemTypes.find(type => type.id === item.item_type_id);
        if (currentItemType) {
          currentHierarchyLevel = hierarchyLevels.find(level => level.level === currentItemType.hierarchy_level);
        }
      }
    } catch (err) {
      console.error('Failed to load hierarchy data:', err);
      parentHierarchy = [];
      currentItemType = null;
      currentHierarchyLevel = null;
    }
  }

  // Done toggle logic (from TodoList)
  function isTaskCompleted() {
    return item?.status_id === STATUS_ID_DONE;
  }

  async function toggleDone() {
    try {
      saving = true;
      const targetStatusId = isTaskCompleted() ? STATUS_ID_OPEN : STATUS_ID_DONE;
      await api.items.transition(item.id, targetStatusId);
      item = { ...item, status_id: targetStatusId };
      onupdate?.();
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
      onupdate?.();
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
      onupdate?.();
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
      onupdate?.();
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

  function handleSaveField(data) {
    const { field, value } = data;
    if (field === 'description') {
      editDescription = value;
      saveDescription();
    }
  }

  function handleCancelEdit(data) {
    const { field } = data;
    if (field === 'description') {
      cancelDescription();
    }
  }

  async function handleAttachmentUploadFiles(data) {
    const { files } = data;
    for (const file of files) {
      try {
        await api.attachments.uploadToItem(item.id, file);
      } catch (err) {
        console.error('Failed to upload:', err);
      }
    }
    await loadAttachments();
  }

  async function handleAttachmentDelete(attachment) {
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
      onclose?.();
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
          {workspaceId}
          onnavigate={(path) => navigate(path)}
          onparentChanged={loadHierarchyData}
          oncopyKey={() => {
            copyToClipboard(`${workspace?.key || 'WORK'}-${item.workspace_item_number}`);
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
          dataTestid="personal-task-status-toggle"
        />

        <!-- Editable Title -->
        {#if editingTitle}
          <Input
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
        <div class="flex items-center gap-1">
          <Button variant="ghost" icon={ExternalLink} href={`/personal/items/${itemId}`} title={t('items.fullDetails')} />
          <Button variant="ghost" icon={X} onclick={closeModal} title={t('common.close')} />
        </div>
      {/if}
    </div>

    <!-- Body -->
    <div class="{isModal ? 'p-4 max-h-[70vh] overflow-y-auto' : 'px-6 py-6'}">
      <!-- Due Date -->
      <div class="mb-4" data-testid="personal-task-due-date">
        {#if editingDueDate}
          <div class="flex items-center gap-2">
            <Calendar class="w-4 h-4" style="color: var(--ds-text-subtle);" />
            <!-- svelte-ignore a11y_autofocus -->
            <Input
              type="date"
              value={item.due_date?.split('T')[0] || ''}
              onchange={(e) => saveDueDate(e.currentTarget.value || null)}
              onblur={() => editingDueDate = false}
              size="small"
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
              {item.due_date ? formatDateOnly(item.due_date) : t('personal.setDueDate')}
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
          showDiagramButton={false}
          showAIActions={false}
          {attachments}
          diagrams={[]}
          onsavefield={handleSaveField}
          oncanceledit={handleCancelEdit}
          onstartEditingDescription={() => { editingDescription = true; }}
          onattachmentUploadFiles={handleAttachmentUploadFiles}
          onattachmentDelete={handleAttachmentDelete}
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
      <Comments itemId={item.id} workspaceId={item.workspace_id} isPersonalWorkspace={true} onCommentsLoaded={handleCommentsLoaded} />
    </div>
  {/if}
{/snippet}

{#if isModal}
  <Modal isOpen={true} onclose={closeModal} maxWidth="max-w-2xl" dataTestid="personal-task-detail">
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
