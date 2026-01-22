<script>
  import { Link2, Plus, Paperclip, PenTool, Zap, ChevronDown } from 'lucide-svelte';
  import { tick, onMount, onDestroy } from 'svelte';
  import Button from '../../components/Button.svelte';
  import MilkdownEditor from '../../editors/LazyMilkdownEditor.svelte';
  import AttachmentDiagramList from '../assets/AttachmentDiagramList.svelte';
  import { createEventDispatcher } from 'svelte';
  import { getShortcut, matchesShortcut, getDisplayString } from '../../utils/keyboardShortcuts.js';
  import { t } from '../../stores/i18n.svelte.js';

  const dispatch = createEventDispatcher();

  // Get shortcut configurations
  const saveShortcut = getShortcut('description', 'save');
  const cancelShortcut = getShortcut('description', 'cancel');

  export let item;
  export let editingDescription = false;
  export let editDescription = '';
  export let saving = false;
  export let availableSubIssueTypes = [];

  // Attachments and diagrams
  export let attachments = [];
  export let diagrams = [];
  export let attachmentSettings = null;
  export let showLinkButton = true;

  // Manual actions
  export let manualActions = [];

  let milkdownEditor;
  let showActionsMenu = false;
  let actionsMenuRef;

  // Handle image insertions from attachments or uploads
  export function insertImage(imageData) {
    // If not currently editing, start editing first
    if (!editingDescription) {
      startEditingDescription();
      // Wait for editor to be ready, then insert
      setTimeout(() => {
        if (milkdownEditor) {
          milkdownEditor.insertImage(imageData.src, imageData.alt || 'image', imageData.title);
        }
      }, 150);
    } else {
      // Editor is already active, insert at cursor position
      if (milkdownEditor) {
        milkdownEditor.insertImage(imageData.src, imageData.alt || 'image', imageData.title);
      }
    }
  }

  // Handle image uploaded via editor
  function handleImageInsert(attachment) {
    // Dispatch to parent to refresh attachment list
    dispatch('image-uploaded', { attachment });
  }

  async function startEditingDescription() {
    editDescription = item.description || '';
    editingDescription = true;
    // Wait for the editor to be rendered, then focus it
    await tick();
    if (milkdownEditor) {
      milkdownEditor.focus();
    }
  }

  function saveDescription() {
    dispatch('save-field', { field: 'description', value: editDescription });
  }

  function cancelEdit() {
    dispatch('cancel-edit', { field: 'description' });
  }

  function handleKeydown(event) {
    // Check for save shortcut (Ctrl/Cmd+Enter)
    if (matchesShortcut(event, saveShortcut)) {
      event.preventDefault();
      saveDescription();
    } else if (matchesShortcut(event, cancelShortcut)) {
      event.preventDefault();
      event.stopPropagation(); // Stop propagation to prevent the modal from closing
      cancelEdit();
    }
  }

  function handleDeleteAttachment(attachment) {
    dispatch('attachment-delete', attachment);
  }

  function handleNewDiagram() {
    dispatch('new-diagram');
  }

  function handleEditDiagram(diagram) {
    dispatch('edit-diagram', diagram);
  }

  function handleDeleteDiagram(diagram) {
    dispatch('delete-diagram', diagram);
  }

  function handleClickOutside(event) {
    if (actionsMenuRef && !actionsMenuRef.contains(event.target)) {
      showActionsMenu = false;
    }
  }

  function handleExecuteAction(action) {
    dispatch('execute-action', action);
    showActionsMenu = false;
  }

  onMount(() => {
    document.addEventListener('click', handleClickOutside);
  });

  onDestroy(() => {
    document.removeEventListener('click', handleClickOutside);
  });
</script>

<div class="pt-2">
  <div class="mb-4">
    <h3 class="text-sm font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle); font-size: 11px;">{t('common.description')}</h3>
  </div>
  
  {#if editingDescription}
    <div class="space-y-3" onkeydown={handleKeydown}>
      <MilkdownEditor
        bind:this={milkdownEditor}
        bind:content={editDescription}
        placeholder={t('items.enterDescription')}
        showToolbar={true}
        itemId={item.id}
        onImageInsert={handleImageInsert}
      />
      <div class="flex items-center gap-2">
        <Button variant="primary" onclick={saveDescription} disabled={saving} keyboardHint={getDisplayString(saveShortcut)}>
          {t('common.save')}
        </Button>
        <Button variant="default" onclick={cancelEdit}>
          {t('common.cancel')}
        </Button>
      </div>
    </div>
  {:else if item.description}
    <div
      onclick={startEditingDescription}
      onkeydown={(e) => e.key === 'Enter' && startEditingDescription()}
      role="button"
      tabindex="0"
      class="description-hover text-left w-full rounded cursor-pointer transition-all duration-150"
      style="color: var(--ds-text);"
      title={t('items.clickToEditDescription')}
    >
      <MilkdownEditor
        content={item.description}
        readonly={true}
        showToolbar={false}
      />
    </div>
  {:else}
    <button
      onclick={startEditingDescription}
      class="text-left w-full py-2 text-sm transition-colors cursor-pointer"
      style="color: var(--ds-text-subtle);"
      onmouseenter={(e) => e.currentTarget.style.color = 'var(--ds-text)'}
      onmouseleave={(e) => e.currentTarget.style.color = 'var(--ds-text-subtle)'}
      title={t('items.clickToAddDescription')}
    >
      {t('items.noDescriptionProvided')}
    </button>
  {/if}
  
  <!-- Action buttons - icon only, label slides in on hover -->
  <div class="mt-5 flex gap-1">
    {#if showLinkButton}
      <button
        class="action-btn inline-flex items-center gap-1.5 px-2 py-1.5 rounded text-xs transition-all"
        style="color: var(--ds-text-subtle);"
        onclick={() => dispatch('show-add-link')}
        title={t('items.addLink')}
      >
        <Link2 class="w-4 h-4 flex-shrink-0" />
        <span class="action-label">{t('common.link')}</span>
      </button>
    {/if}
    {#if availableSubIssueTypes.length > 0}
      <button
        class="action-btn inline-flex items-center gap-1.5 px-2 py-1.5 rounded text-xs transition-all"
        style="color: var(--ds-text-subtle);"
        onclick={() => dispatch('create-sub-issue')}
        title={t('items.createChild')}
      >
        <Plus class="w-4 h-4 flex-shrink-0" />
        <span class="action-label">{t('items.child')}</span>
      </button>
    {/if}
    {#if attachmentSettings?.enabled}
      <label
        class="action-btn inline-flex items-center gap-1.5 px-2 py-1.5 rounded text-xs transition-all cursor-pointer"
        style="color: var(--ds-text-subtle);"
        title={t('items.attachFile')}
      >
        <Paperclip class="w-4 h-4 flex-shrink-0" />
        <span class="action-label">{t('items.attach')}</span>
        <input
          type="file"
          class="hidden"
          multiple
          onchange={(e) => {
            const files = e.target.files;
            if (files?.length) {
              dispatch('attachment-upload-files', { files: Array.from(files) });
            }
            e.target.value = '';
          }}
        />
      </label>
      <button
        class="action-btn inline-flex items-center gap-1.5 px-2 py-1.5 rounded text-xs transition-all"
        style="color: var(--ds-text-subtle);"
        onclick={handleNewDiagram}
        title={t('items.newDiagram')}
      >
        <PenTool class="w-4 h-4 flex-shrink-0" />
        <span class="action-label">{t('items.diagram')}</span>
      </button>
    {/if}
    {#if manualActions.length > 0}
      <div class="relative" bind:this={actionsMenuRef}>
        <button
          class="action-btn inline-flex items-center gap-1.5 px-2 py-1.5 rounded text-xs transition-all"
          style="color: var(--ds-text-subtle);"
          onclick={(e) => { e.stopPropagation(); showActionsMenu = !showActionsMenu; }}
          title={t('actions.title')}
        >
          <Zap class="w-4 h-4 flex-shrink-0" />
          <span class="action-label">{t('actions.title')}</span>
          <ChevronDown class="w-3 h-3 ml-0.5" />
        </button>

        {#if showActionsMenu}
          <div class="absolute left-0 top-full mt-1 z-50 min-w-[200px] rounded-md shadow-lg py-1" style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);">
            {#each manualActions as action}
              <button
                class="w-full px-3 py-2 text-left text-sm flex items-center gap-2 transition-colors"
                style="color: var(--ds-text);"
                onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
                onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'transparent'}
                onclick={() => handleExecuteAction(action)}
              >
                <Zap class="w-4 h-4 text-amber-500 flex-shrink-0" />
                {action.name}
              </button>
            {/each}
          </div>
        {/if}
      </div>
    {/if}
  </div>

  <!-- Attachments & Diagrams list -->
  <div class="mt-4">
    <AttachmentDiagramList
      {attachments}
      {diagrams}
      on:delete={e => handleDeleteAttachment(e.detail)}
      on:edit-diagram={e => handleEditDiagram(e.detail)}
      on:delete-diagram={e => handleDeleteDiagram(e.detail)}
    />
  </div>
</div>

<style>
  .description-hover {
    padding: 0;
  }
  .description-hover:hover {
    padding: 0.5rem;
    background-color: var(--ds-background-neutral-hovered);
  }
  .action-btn {
    overflow: hidden;
    color: var(--ds-text-subtle);
  }
  .action-btn:hover {
    color: var(--ds-text-subtle);
  }
  .action-label {
    max-width: 0;
    opacity: 0;
    overflow: hidden;
    white-space: nowrap;
    transition: max-width 0.2s ease, opacity 0.2s ease;
  }
  .action-btn:hover .action-label {
    max-width: 80px;
    opacity: 1;
  }
</style>