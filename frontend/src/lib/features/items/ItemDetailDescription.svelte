<script>
  import Link2 from '@lucide/svelte/icons/link-2';
  import Plus from '@lucide/svelte/icons/plus';
  import Paperclip from '@lucide/svelte/icons/paperclip';
  import IconGraph from '@tabler/icons-svelte-runes/icons/graph';
  import Zap from '@lucide/svelte/icons/zap';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import { tick } from 'svelte';
  import Button from '../../components/Button.svelte';
  import FileInput from '../../components/FileInput.svelte';
  import MilkdownEditor from '../../editors/LazyMilkdownEditor.svelte';
  import AttachmentDiagramList from '../assets/AttachmentDiagramList.svelte';
  import AIActionsDropdown from './AIActionsDropdown.svelte';
  import { getShortcut, matchesShortcut, getDisplayString } from '../../utils/keyboardShortcuts.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { attachmentStatus } from '../../stores/attachmentStatus.svelte.js';
  import { aiStore } from '../../stores/aiStore.svelte.js';
  import { onClickOutside } from 'runed';

  // Get shortcut configurations
  const saveShortcut = getShortcut('description', 'save');
  const cancelShortcut = getShortcut('description', 'cancel');

  let {
    item,
    editingDescription = $bindable(false),
    editDescription = $bindable(''),
    saving = false,
    availableSubIssueTypes = [],
    attachments = [],
    diagrams = [],
    showLinkButton = true,
    showDiagramButton = true,
    showAIActions = true,
    manualActions = [],
    canCreate = false,
    // Write affordances (edit description, add link, attach, diagram) are
    // offered only when the user may edit the item; personal tasks stay
    // editable via the default.
    canEdit = true,
    onsavefield = undefined,
    oncanceledit = undefined,
    onstartEditingDescription = undefined,
    onshowAddLink = undefined,
    oncreateSubIssue = undefined,
    onimageuploaded = undefined,
    onattachmentUpload = undefined,
    onattachmentUploadFiles = undefined,
    onattachmentDelete = undefined,
    onnewDiagram = undefined,
    oneditDiagram = undefined,
    ondeleteDiagram = undefined,
    onexecuteAction = undefined,
    onaiaction = undefined,
  } = $props();

  let milkdownEditor = $state(null);
  let showActionsMenu = $state(false);
  let actionsMenuRef = $state(null);

  // Local state for editor content (initialized from prop when editing starts)
  let editorContent = $state('');

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
    onimageuploaded?.({ attachment });
  }

  function startEditingDescription() {
    onstartEditingDescription?.();
  }

  // The description display is one big click-to-edit target. Links rendered
  // inside it must keep their native behaviour: entering edit mode re-creates
  // the editor and removes the anchor, which cancels the pending navigation in
  // some browsers. Ignore clicks that land on a link so the browser follows it.
  function isLinkTarget(event) {
    return !!event.target?.closest?.('a[href]');
  }

  function handleDescriptionClick(event) {
    if (isLinkTarget(event)) return;
    startEditingDescription();
  }

  function handleDescriptionKeydown(event) {
    if (event.key !== 'Enter' || isLinkTarget(event)) return;
    startEditingDescription();
  }

  // Initialize editor content when editing starts
  $effect(() => {
    if (editingDescription) {
      editorContent = editDescription;
    }
  });

  // Focus editor when it becomes available during editing
  // Both dependencies must be at the top level to be tracked properly
  $effect(() => {
    const isEditing = editingDescription;
    const editor = milkdownEditor;
    if (isEditing && editor) {
      tick().then(() => editor.focusEnd());
    }
  });

  function saveDescription() {
    onsavefield?.({ field: 'description', value: editorContent });
  }

  function cancelEdit() {
    oncanceledit?.({ field: 'description' });
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
    onattachmentDelete?.(attachment);
  }

  function handleNewDiagram() {
    onnewDiagram?.();
  }

  function handleEditDiagram(diagram) {
    oneditDiagram?.(diagram);
  }

  function handleDeleteDiagram(diagram) {
    ondeleteDiagram?.(diagram);
  }

  // Handle click outside using runed
  onClickOutside(
    () => actionsMenuRef,
    () => { showActionsMenu = false; }
  );

  function handleExecuteAction(action) {
    onexecuteAction?.(action);
    showActionsMenu = false;
  }
</script>

<div class="pt-2">
  {#if editingDescription}
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div class="space-y-3" onkeydown={handleKeydown} data-testid="item-description-editor">
      <MilkdownEditor
        bind:this={milkdownEditor}
        bind:content={editorContent}
        placeholder={t('items.enterDescription')}
        showToolbar={true}
        itemId={item.id}
        workspaceId={item.workspace_id}
        onImageInsert={handleImageInsert}
      />
      <div class="flex items-center gap-2">
        <Button variant="primary" onclick={saveDescription} disabled={saving} keyboardHint={getDisplayString(saveShortcut)} dataTestid="item-description-save">
          {t('common.save')}
        </Button>
        <Button variant="default" onclick={cancelEdit}>
          {t('common.cancel')}
        </Button>
      </div>
    </div>
  {:else if item.description}
    {#if canEdit}
      <div
        onclick={handleDescriptionClick}
        onkeydown={handleDescriptionKeydown}
        role="button"
        tabindex="0"
        class="description-hover text-left rounded cursor-pointer transition-colors duration-150"
        style="color: var(--ds-text);"
        title={t('items.clickToEditDescription')}
        data-testid="item-description-display"
      >
        <MilkdownEditor content={item.description} readonly={true} showToolbar={false} workspaceId={item.workspace_id} />
      </div>
    {:else}
      <div
        class="text-left rounded"
        style="color: var(--ds-text);"
        data-testid="item-description-display"
      >
        <MilkdownEditor content={item.description} readonly={true} showToolbar={false} workspaceId={item.workspace_id} />
      </div>
    {/if}
  {:else if canEdit}
    <button
      data-testid="item-description-empty"
      onclick={startEditingDescription}
      class="description-empty text-left text-[0.875rem] transition-colors cursor-pointer"
      title={t('items.clickToAddDescription')}
    >
      {t('items.noDescriptionProvided')}
    </button>
  {:else}
    <p
      data-testid="item-description-empty"
      class="text-left w-full py-2 text-[0.875rem]"
      style="color: var(--ds-text-subtle);"
    >
      {t('items.noDescriptionProvided')}
    </p>
  {/if}
  
  <!-- Action buttons - icon only, label slides in on hover. The row's -mx-2
       compensates the buttons' px-2: icons sit on the text column edge and
       the leftmost button's hover chip lines up with the title/description
       hover pills (8px outset). Each button keeps its own hover chip. -->
  <div class="mt-5 flex gap-1 -mx-2">
    {#if showLinkButton && canEdit}
      <button
        data-testid="add-link-button"
        class="action-btn inline-flex items-center gap-1.5 px-2 py-1.5 rounded text-xs transition-all"
        style="color: var(--ds-text-subtle);"
        onclick={() => onshowAddLink?.()}
        title={t('items.addLink')}
      >
        <Link2 class="w-4 h-4 flex-shrink-0" />
        <span class="action-label">{t('common.link')}</span>
      </button>
    {/if}
    {#if availableSubIssueTypes.length > 0}
      <button
        data-testid="item-create-child"
        class="action-btn inline-flex items-center gap-1.5 px-2 py-1.5 rounded text-xs transition-all"
        style="color: var(--ds-text-subtle);"
        onclick={() => oncreateSubIssue?.()}
        title={t('items.createChild')}
      >
        <Plus class="w-4 h-4 flex-shrink-0" />
        <span class="action-label">{t('items.child')}</span>
      </button>
    {/if}
    {#if attachmentStatus.enabled && canEdit}
      <label
        data-testid="item-attachment-upload"
        class="action-btn inline-flex items-center gap-1.5 px-2 py-1.5 rounded text-xs transition-all cursor-pointer"
        style="color: var(--ds-text-subtle);"
        title={t('items.attachFile')}
      >
        <Paperclip class="w-4 h-4 flex-shrink-0" />
        <span class="action-label">{t('items.attach')}</span>
        <FileInput
          class="hidden"
          multiple
          onchange={(e) => {
            const files = e.currentTarget.files;
            if (files?.length) {
              onattachmentUploadFiles?.({ files: Array.from(files) });
            }
            e.currentTarget.value = '';
          }}
        />
      </label>
      {#if showDiagramButton && canEdit}
        <button
          class="action-btn inline-flex items-center gap-1.5 px-2 py-1.5 rounded text-xs transition-all"
          style="color: var(--ds-text-subtle);"
          onclick={handleNewDiagram}
          title={t('items.newDiagram')}
        >
          <IconGraph class="w-4 h-4 flex-shrink-0" />
          <span class="action-label">{t('items.diagram')}</span>
        </button>
      {/if}
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
                class="manual-action w-full px-3 py-2 text-left text-sm flex items-center gap-2 transition-colors"
                onclick={() => handleExecuteAction(action)}
              >
                <Zap class="w-4 h-4 text-ds-accent-yellow flex-shrink-0" />
                {action.name}
              </button>
            {/each}
          </div>
        {/if}
      </div>
    {/if}
    {#if showAIActions && aiStore.available}
      <AIActionsDropdown
        {item}
        {availableSubIssueTypes}
        {canCreate}
        onaiaction={(data) => onaiaction?.(data)}
      />
    {/if}
  </div>

  <!-- Attachments & Diagrams list -->
  <div class="mt-4">
    <AttachmentDiagramList
      {attachments}
      {diagrams}
      ondelete={handleDeleteAttachment}
      oneditdiagram={handleEditDiagram}
      ondeletediagram={handleDeleteDiagram}
    />
  </div>
</div>

<style>
  .description-hover {
    /* constant padding with compensating negative margin: hover must only
       repaint — a padding change on hover resizes the box and re-wraps the
       description text, which reads as a wiggle */
    width: calc(100% + 1rem);
    padding: 0.5rem;
    margin: -0.5rem;
    border-radius: 0.25rem;
  }
  /* Same pill as the title hover: 8px outset in the neutral hover token. */
  .description-hover:hover,
  .description-empty:hover {
    background-color: var(--ds-background-neutral-hovered);
  }
  .action-btn {
    overflow: hidden;
    color: var(--ds-text-subtle);
  }
  /* Lucide glyphs draw ~0.7px inside their box at 16px (24-unit viewBox with
     round caps), which reads as a right-shift against the text column at
     high zoom. Compensate so the icon ink sits on the column edge. */
  .action-btn > :global(svg) {
    margin-left: -0.5px;
  }
  .action-btn:hover {
    color: var(--ds-text-subtle);
    background-color: var(--ds-background-neutral-hovered);
  }
  .action-label {
    max-width: 0;
    opacity: 0;
    overflow: hidden;
    transform: translateX(-0.25rem);
    white-space: nowrap;
    transition: opacity 0.2s ease, transform 0.2s ease;
  }
  .action-btn:hover .action-label {
    max-width: 80px;
    opacity: 1;
    transform: translateX(0);
  }
  .description-empty {
    /* Same 8px-outset pill geometry as .description-hover above. */
    width: calc(100% + 1rem);
    margin: 0 -0.5rem;
    padding: 0.5rem;
    border-radius: 0.25rem;
    color: var(--ds-text-subtle);
  }

  .description-empty:hover {
    color: var(--ds-text);
  }

  .manual-action {
    color: var(--ds-text);
  }

  .manual-action:hover {
    background-color: var(--ds-background-neutral-hovered);
  }
</style>
