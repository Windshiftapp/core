<script>
  import { onMount, onDestroy } from 'svelte';
  import { Editor, rootCtx, defaultValueCtx, editorViewOptionsCtx, editorViewCtx } from '@milkdown/kit/core';
  import { commonmark, toggleStrongCommand, toggleEmphasisCommand, wrapInBulletListCommand, wrapInOrderedListCommand, toggleInlineCodeCommand } from '@milkdown/kit/preset/commonmark';
  import { gfm, toggleStrikethroughCommand } from '@milkdown/kit/preset/gfm';
  import { listener, listenerCtx } from '@milkdown/kit/plugin/listener';
  import { upload, uploadConfig } from '@milkdown/kit/plugin/upload';
  import { insert, callCommand } from '@milkdown/kit/utils';
  import { replaceAll } from '@milkdown/utils';
  import { nord } from '@milkdown/theme-nord';
  import '@milkdown/theme-nord/style.css';
  import { imageBlockComponent } from '@milkdown/kit/component/image-block';
  import { Bold, Italic, Code, List, ListOrdered, Strikethrough, Image as ImageIcon } from 'lucide-svelte';
  import { api } from '../api.js';
  import MentionPicker from '../pickers/MentionPicker.svelte';
  import { mentionDecorationPlugin } from './milkdown-mention-mark.js';
  import { t } from '../stores/i18n.svelte.js';
  import { attachmentStatus } from '../stores/attachmentStatus.svelte.js';

  export let content = '';
  export let placeholder = '';
  $: effectivePlaceholder = placeholder || t('editors.enterText');
  export let readonly = false;
  export let showToolbar = false; // Show formatting toolbar
  export let itemId = null; // Item ID for attachment uploads (backwards compatibility)
  export let entityType = null; // Entity type: 'item', 'test_case', etc.
  export let entityId = null; // Entity ID for attachment uploads
  export let onImageInsert = null; // Callback when image is inserted
  export let isPersonalWorkspace = false; // Flag to show warning in mention picker
  export let compact = false; // Use smaller height for compact layouts

  // Derive attachments enabled from store (falls back to true if not yet loaded to avoid flash)
  $: attachmentsEnabled = attachmentStatus.loaded ? attachmentStatus.enabled : true;

  // Compute effective entity info (supports both old itemId and new entityType/entityId)
  $: effectiveEntityType = entityType || (itemId ? 'item' : null);
  $: effectiveEntityId = entityId || itemId;

  let editorElement;
  let fileInput;
  let editor;
  let initialContent = content;

  // Mention picker state
  let mentionPickerOpen = false;
  let mentionQuery = '';
  let mentionPosition = { x: 0, y: 0 };
  let mentionRange = null; // { from, to } positions in the document

  // User hover card state
  let hoverCardVisible = false;
  let hoverCardPosition = { x: 0, y: 0 };
  let hoverCardUser = null;
  let hoverCardLoading = false;
  let hoverCardTimeout = null;
  let hideCardTimeout = null;
  let userCache = new Map();

  // Toolbar actions
  function toggleBold() {
    if (editor) editor.action(callCommand(toggleStrongCommand.key));
  }

  function toggleItalic() {
    if (editor) editor.action(callCommand(toggleEmphasisCommand.key));
  }

  function toggleCode() {
    if (editor) editor.action(callCommand(toggleInlineCodeCommand.key));
  }

  function toggleStrikethrough() {
    if (editor) editor.action(callCommand(toggleStrikethroughCommand.key));
  }

  function toggleBulletList() {
    if (editor) editor.action(callCommand(wrapInBulletListCommand.key));
  }

  function toggleOrderedList() {
    if (editor) editor.action(callCommand(wrapInOrderedListCommand.key));
  }

  // Mention handling functions
  function checkForMentionTrigger(view) {
    const { state } = view;
    const { selection } = state;
    const { $from } = selection;

    // Get text before cursor in current text block
    const textBefore = $from.parent.textContent.slice(0, $from.parentOffset);

    // Look for @ trigger pattern: @ followed by optional word characters
    const mentionMatch = textBefore.match(/@([a-zA-Z0-9_.-]*)$/);

    if (mentionMatch) {
      // Calculate positions
      const matchStart = $from.pos - mentionMatch[0].length;
      const matchEnd = $from.pos;

      // Get cursor coordinates for positioning the picker
      const coords = view.coordsAtPos(matchStart);

      mentionQuery = mentionMatch[1] || '';
      mentionPosition = {
        x: coords.left,
        y: coords.bottom + 4
      };
      mentionRange = { from: matchStart, to: matchEnd };
      mentionPickerOpen = true;
    } else {
      // Close picker if no mention pattern
      if (mentionPickerOpen) {
        closeMentionPicker();
      }
    }
  }

  function closeMentionPicker() {
    mentionPickerOpen = false;
    mentionQuery = '';
    mentionRange = null;
  }

  function handleMentionSelect(event) {
    const user = event.detail;
    if (!editor || !mentionRange) return;

    // Format the mention text based on whether display name has spaces
    const displayName = `${user.first_name || ''} ${user.last_name || ''}`.trim();
    const mentionText = displayName.includes(' ')
      ? `@"${displayName}"`
      : `@${user.username}`;

    // Get the editor view and replace the mention range with the formatted mention
    editor.action((ctx) => {
      const view = ctx.get(editorViewCtx);
      const { state, dispatch } = view;

      // Replace @query with @"Display Name" or @username
      const tr = state.tr.replaceWith(
        mentionRange.from,
        mentionRange.to,
        state.schema.text(mentionText + ' ')
      );
      dispatch(tr);
    });

    closeMentionPicker();
  }

  function handleMentionCancel() {
    closeMentionPicker();
  }

  // User hover card functions
  async function loadUserForHoverCard(identifier) {
    if (hoverCardLoading) return;

    // Check cache first
    if (userCache.has(identifier)) {
      hoverCardUser = userCache.get(identifier);
      return;
    }

    hoverCardLoading = true;
    try {
      const users = await api.getUsers();
      const user = users.find(u =>
        u.username === identifier ||
        `${u.first_name || ''} ${u.last_name || ''}`.trim() === identifier
      );
      if (user) {
        userCache.set(identifier, user);
        hoverCardUser = user;
      }
    } catch (e) {
      console.error('Failed to load user for hover card:', e);
    }
    hoverCardLoading = false;
  }

  function showHoverCard(element, identifier) {
    if (hideCardTimeout) {
      clearTimeout(hideCardTimeout);
      hideCardTimeout = null;
    }

    hoverCardTimeout = setTimeout(() => {
      const rect = element.getBoundingClientRect();
      hoverCardPosition = {
        x: rect.left + rect.width / 2,
        y: rect.top - 8
      };
      hoverCardVisible = true;
      hoverCardUser = null;
      loadUserForHoverCard(identifier);
    }, 300);
  }

  function hideHoverCard() {
    if (hoverCardTimeout) {
      clearTimeout(hoverCardTimeout);
      hoverCardTimeout = null;
    }

    hideCardTimeout = setTimeout(() => {
      hoverCardVisible = false;
      hoverCardUser = null;
    }, 100);
  }

  function hoverCardMouseEnter() {
    if (hideCardTimeout) {
      clearTimeout(hideCardTimeout);
      hideCardTimeout = null;
    }
  }

  function hoverCardMouseLeave() {
    hideHoverCard();
  }

  function getInitials(user) {
    const first = user.first_name?.[0] || '';
    const last = user.last_name?.[0] || '';
    return (first + last).toUpperCase() || '?';
  }

  // Event delegation handlers for mention hover (more efficient than MutationObserver)
  function handleMentionMouseOver(e) {
    const chip = e.target.closest('.mention-chip');
    if (chip) {
      const identifier = chip.dataset.mention;
      if (identifier) {
        showHoverCard(chip, identifier);
      }
    }
  }

  function handleMentionMouseOut(e) {
    const chip = e.target.closest('.mention-chip');
    if (chip) {
      // Only hide if we're actually leaving the chip (not entering a child)
      const relatedTarget = e.relatedTarget;
      if (!relatedTarget || !chip.contains(relatedTarget)) {
        hideHoverCard();
      }
    }
  }

  // Shared uploader logic for drag/drop/paste and the toolbar button
  async function uploadImages(files, schema, shouldInsertMarkdown = false) {
    if (!attachmentsEnabled) {
      console.log('[MilkdownEditor] Attachments disabled, skipping upload');
      return { nodes: [], attachments: [] };
    }
    console.log('[MilkdownEditor] uploader called with', files.length, 'files');

    const images = [];
    for (let i = 0; i < files.length; i++) {
      const file = files.item(i);
      if (!file || !file.type.includes('image')) continue;
      images.push(file);
    }

    console.log('[MilkdownEditor] Found', images.length, 'images');

    if (images.length === 0) {
      return { nodes: [], attachments: [] };
    }

    const results = await Promise.all(
      images.map(async (image) => {
        console.log('[MilkdownEditor] Uploading image:', image.name);
        try {
          const formData = new FormData();
          formData.append('file', image);

          if (effectiveEntityId && effectiveEntityType) {
            formData.append('entity_id', effectiveEntityId.toString());
            formData.append('entity_type', effectiveEntityType);
          }

          const result = await api.attachments.upload(formData);
          console.log('[MilkdownEditor] Upload result:', result);

          if (result.success && result.attachment) {
            const src = `/api/attachments/${result.attachment.id}/download`;
            const node = schema?.nodes?.image?.createAndFill({
              src,
              alt: image.name,
            }) || null;

            return {
              node,
              attachment: result.attachment,
              src,
              alt: image.name,
            };
          }
          return null;
        } catch (error) {
          console.error('[MilkdownEditor] Image upload error:', error);
          return null;
        }
      })
    );

    const nodes = [];
    const attachments = [];

    results.forEach((result) => {
      if (!result) return;
      if (result.node) nodes.push(result.node);
      if (result.attachment) attachments.push(result.attachment);
      if (shouldInsertMarkdown && result.src) {
        insertImage(result.src, result.alt || 'image');
      }
    });

    return { nodes, attachments };
  }

  // Adapter for Milkdown upload plugin (drag/drop/paste)
  async function uploader(files, schema) {
    const { nodes, attachments } = await uploadImages(files, schema, false);

    attachments.forEach((attachment) => {
      if (onImageInsert) {
        onImageInsert(attachment);
      }
    });

    return nodes;
  }

  onMount(async () => {
    try {
      console.log('[MilkdownEditor] Mount - effectiveEntityId:', effectiveEntityId, 'effectiveEntityType:', effectiveEntityType, 'readonly:', readonly);

      editor = await Editor.make()
        .config((ctx) => {
          ctx.set(rootCtx, editorElement);
          ctx.set(defaultValueCtx, initialContent || '');
          ctx.get(listenerCtx).markdownUpdated((ctx, markdown) => {
            content = markdown;
          });
          // Use set instead of update to handle case where context may not be initialized
          ctx.set(editorViewOptionsCtx, {
            editable: () => !readonly,
            attributes: {
              class: 'milkdown-editor-content',
              'data-placeholder': effectivePlaceholder
            },
            // Handle DOM events for mention detection
            handleDOMEvents: {
              keyup: (view, event) => {
                if (!readonly) {
                  checkForMentionTrigger(view);
                }
                return false; // Don't prevent default
              },
              click: (view, event) => {
                // Close mention picker when clicking elsewhere
                if (mentionPickerOpen) {
                  closeMentionPicker();
                }
                return false;
              }
            }
          });

          // Configure upload plugin following official docs pattern (only if attachments enabled)
          if (!readonly && attachmentsEnabled) {
            console.log('[MilkdownEditor] Configuring uploadConfig with uploader');
            ctx.update(uploadConfig.key, (prev) => ({
              ...prev,
              uploader,
            }));
          }
        })
        .config(nord)
        .use(commonmark)
        .use(gfm)
        .use(listener)
        .use(upload)  // Include upload in main chain per docs
        .use(imageBlockComponent)  // Enable image resizing
        .use(mentionDecorationPlugin)  // Add mention chip decorations
        .create();

      console.log('[MilkdownEditor] Editor created successfully');

      // Use event delegation for mention hover (much more efficient than MutationObserver)
      editorElement.addEventListener('mouseover', handleMentionMouseOver);
      editorElement.addEventListener('mouseout', handleMentionMouseOut);
    } catch (error) {
      console.error('Failed to initialize Milkdown editor:', error);
    }
  });

  onDestroy(async () => {
    // Clean up event delegation listeners
    if (editorElement) {
      editorElement.removeEventListener('mouseover', handleMentionMouseOver);
      editorElement.removeEventListener('mouseout', handleMentionMouseOut);
    }
    if (hoverCardTimeout) {
      clearTimeout(hoverCardTimeout);
    }
    if (hideCardTimeout) {
      clearTimeout(hideCardTimeout);
    }
    if (editor) {
      try {
        await editor.destroy();
      } catch (e) {
        console.error('Error destroying editor:', e);
      }
    }
  });

  export function focus() {
    if (editorElement && !readonly) {
      // Retry focusing until editor is ready (max 10 attempts over 500ms)
      let attempts = 0;
      const maxAttempts = 10;
      const attemptFocus = () => {
        const editorView = editorElement.querySelector('.ProseMirror');
        if (editorView) {
          editorView.focus();
        } else if (attempts < maxAttempts) {
          attempts++;
          setTimeout(attemptFocus, 50);
        }
      };
      attemptFocus();
    }
  }

  export function focusEnd() {
    if (editorElement && !readonly && editor) {
      // Retry focusing until editor is ready (max 10 attempts over 500ms)
      let attempts = 0;
      const maxAttempts = 10;
      const attemptFocus = () => {
        const proseMirror = editorElement.querySelector('.ProseMirror');
        if (proseMirror) {
          proseMirror.focus();
          // Move cursor to end of document
          editor.action((ctx) => {
            const view = ctx.get(editorViewCtx);
            const { state } = view;
            const endPos = state.doc.content.size;
            const tr = state.tr.setSelection(
              state.selection.constructor.near(state.doc.resolve(endPos))
            );
            view.dispatch(tr);
          });
        } else if (attempts < maxAttempts) {
          attempts++;
          setTimeout(attemptFocus, 50);
        }
      };
      attemptFocus();
    }
  }

  export function clear() {
    if (editor && !readonly) {
      editor.action(replaceAll(''));
      content = '';
    }
  }

  export function insertImage(src, alt = 'image', title = null) {
    if (!editor || readonly) {
      console.warn('Cannot insert image: editor not ready or readonly');
      return;
    }

    try {
      // Build markdown image syntax
      const imageMarkdown = title
        ? `![${alt}](${src} "${title}")`
        : `![${alt}](${src})`;

      // Use Milkdown's insert command to insert markdown at cursor position
      editor.action(insert(imageMarkdown));
    } catch (error) {
      console.error('Failed to insert image:', error);
    }
  }

  function handleClick() {
    if (!readonly) {
      focus();
    }
  }

  function openFilePicker() {
    if (readonly) return;
    fileInput?.click();
  }

  async function handleFileInputChange(event) {
    const files = event.target.files;
    if (!files || files.length === 0) return;

    const { attachments } = await uploadImages(files, null, true);

    attachments.forEach((attachment) => {
      if (onImageInsert) {
        onImageInsert(attachment);
      }
    });
    // Reset so the same file can be selected twice
    event.target.value = '';
  }

  // Keep readonly renders in sync when underlying markdown changes
  $: if (editor && readonly) {
    editor.action(replaceAll(content || ''));
  }
</script>

<div class="milkdown-wrapper" class:has-toolbar={showToolbar && !readonly}>
  {#if showToolbar && !readonly}
    <div class="milkdown-toolbar" tabindex="-1" aria-hidden="true">
      <button type="button" class="toolbar-btn" tabindex="-1" onclick={toggleBold} title={t('editors.bold')}>
        <Bold size={14} />
      </button>
      <button type="button" class="toolbar-btn" tabindex="-1" onclick={toggleItalic} title={t('editors.italic')}>
        <Italic size={14} />
      </button>
      <button type="button" class="toolbar-btn" tabindex="-1" onclick={toggleStrikethrough} title={t('editors.strikethrough')}>
        <Strikethrough size={14} />
      </button>
      <button type="button" class="toolbar-btn" tabindex="-1" onclick={toggleCode} title={t('editors.inlineCode')}>
        <Code size={14} />
      </button>
      <div class="toolbar-divider"></div>
      <button type="button" class="toolbar-btn" tabindex="-1" onclick={toggleBulletList} title={t('editors.bulletList')}>
        <List size={14} />
      </button>
      <button type="button" class="toolbar-btn" tabindex="-1" onclick={toggleOrderedList} title={t('editors.numberedList')}>
        <ListOrdered size={14} />
      </button>
      {#if attachmentsEnabled}
        <button type="button" class="toolbar-btn" tabindex="-1" onclick={openFilePicker} title={t('editors.insertImage')}>
          <ImageIcon size={14} />
        </button>
      {/if}
    </div>
  {/if}
  <div bind:this={editorElement} class="milkdown-editor" class:readonly class:compact class:has-toolbar={showToolbar && !readonly} onclick={handleClick}></div>
</div>
<input
  bind:this={fileInput}
  type="file"
  accept="image/*"
  multiple
  tabindex="-1"
  aria-hidden="true"
  class="hidden-file-input"
  onchange={handleFileInputChange}
/>
<!-- Mention Picker for @ mentions -->
<MentionPicker
  bind:open={mentionPickerOpen}
  query={mentionQuery}
  position={mentionPosition}
  {isPersonalWorkspace}
  onselect={handleMentionSelect}
  oncancel={handleMentionCancel}
/>
<!-- User Hover Card for mentions -->
{#if hoverCardVisible}
  <div
    class="user-hover-card"
    style="left: {hoverCardPosition.x}px; top: {hoverCardPosition.y}px;"
    onmouseenter={hoverCardMouseEnter}
    onmouseleave={hoverCardMouseLeave}
  >
    {#if hoverCardLoading}
      <div class="hc-loading">{t('common.loading')}</div>
    {:else if hoverCardUser}
      <div class="hc-user-card">
        <div class="hc-avatar">
          {#if hoverCardUser.avatar_url}
            <img src={hoverCardUser.avatar_url} alt="" />
          {:else}
            <span class="hc-initials">{getInitials(hoverCardUser)}</span>
          {/if}
        </div>
        <div class="hc-info">
          <div class="hc-name">{hoverCardUser.first_name || ''} {hoverCardUser.last_name || ''}</div>
          {#if hoverCardUser.email}
            <div class="hc-email">{hoverCardUser.email}</div>
          {/if}
          {#if hoverCardUser.username}
            <div class="hc-username">@{hoverCardUser.username}</div>
          {/if}
        </div>
      </div>
    {:else}
      <div class="hc-not-found">{t('editors.userNotFound')}</div>
    {/if}
  </div>
{/if}

<style>
  .hidden-file-input {
    position: absolute;
    opacity: 0;
    pointer-events: none;
    width: 0;
    height: 0;
  }

  .milkdown-wrapper {
    border-radius: 0.375rem;
    overflow: hidden;
  }

  .milkdown-image-block.selected {
    outline: none;
  }
  .milkdown-wrapper:focus-within {
    outline: none;
    box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.5);
  }

  .milkdown-toolbar {
    display: flex;
    align-items: center;
    gap: 2px;
    padding: 4px 8px;
    background: var(--ds-surface);
    border: 1px solid var(--ds-border);
    border-bottom: none;
    border-top-left-radius: 0.375rem;
    border-top-right-radius: 0.375rem;
  }

  .toolbar-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 26px;
    height: 26px;
    border: none;
    background: transparent;
    border-radius: 4px;
    color: var(--ds-text-subtle, #6b7280);
    cursor: pointer;
    transition: all 0.15s;
  }

  .toolbar-btn:hover {
    background: var(--ds-surface);
    color: var(--ds-text);
  }

  .toolbar-divider {
    width: 1px;
    height: 16px;
    background: var(--ds-border, rgba(0, 0, 0, 0.12));
    margin: 0 4px;
  }

  .milkdown-editor {
    border: 1px solid var(--ds-border);
    border-radius: 0.375rem;
    overflow: hidden;
    background: var(--ds-background-input);
  }

  .milkdown-editor.has-toolbar {
    border-top: none;
    border-top-left-radius: 0;
    border-top-right-radius: 0;
  }

  .milkdown-editor.readonly {
    border: none;
    background: transparent;
  }

  :global(.milkdown-editor .milkdown) {
    padding: 0.75rem;
    min-height: 150px;
    /* Override Nord theme CSS variables for typography */
    --text-base: 0.875rem;
    --text-base--line-height: calc(1.5 / 0.875);
    font-size: 14px;
  }

  :global(.milkdown-editor.readonly .milkdown) {
    padding: 0.5rem 0;
    min-height: auto;
  }

  :global(.milkdown-editor.compact .milkdown) {
    min-height: 80px;
  }

  :global(.milkdown-editor .ProseMirror) {
    outline: none;
  }

  :global(.milkdown-editor .ProseMirror p.is-empty:first-child::before) {
    content: attr(data-placeholder);
    float: left;
    color: var(--ds-text-subtlest, #9ca3af);
    pointer-events: none;
    height: 0;
  }

  /* Basic typography styles */
  :global(.milkdown-editor .ProseMirror h1) {
    font-size: 1.35rem;
    font-weight: 700;
    margin: 0.75rem 0 0.5rem;
  }

  :global(.milkdown-editor .ProseMirror h2) {
    font-size: 1.2rem;
    font-weight: 600;
    margin: 0.75rem 0 0.5rem;
  }

  :global(.milkdown-editor .ProseMirror h3) {
    font-size: 1.05rem;
    font-weight: 600;
    margin: 0.5rem 0 0.25rem;
  }

  :global(.milkdown-editor .ProseMirror p) {
    margin: 0.25rem 0;
  }

  /* Preserve blank lines from remarkPreserveEmptyLinePlugin */
  :global(.milkdown-editor .ProseMirror br) {
    display: block;
    content: "";
    margin-top: 0.5rem;
  }

  :global(.milkdown-editor .ProseMirror ul),
  :global(.milkdown-editor .ProseMirror ol) {
    padding-left: 1.5rem;
    margin: 0.25rem 0;
  }

  :global(.milkdown-editor .ProseMirror ul) {
    list-style-type: disc;
  }

  :global(.milkdown-editor .ProseMirror ol) {
    list-style-type: decimal;
  }

  :global(.milkdown-editor .ProseMirror li) {
    margin: 0.25rem 0;
  }

  :global(.milkdown-editor .ProseMirror blockquote) {
    border-left: 4px solid var(--ds-border);
    padding-left: 1rem;
    margin: 0.25rem 0;
    font-style: italic;
    color: var(--ds-text-subtle);
  }

  :global(.milkdown-editor .ProseMirror code) {
    background-color: var(--ds-surface);
    padding: 0.125rem 0.25rem;
    border-radius: 0.25rem;
    font-family: ui-monospace, monospace;
    font-size: 0.875rem;
  }

  :global(.milkdown-editor .ProseMirror pre) {
    background-color: var(--ds-surface-card);
    color: var(--ds-text);
    padding: 1rem;
    border-radius: 0.375rem;
    overflow-x: auto;
    margin: 0.25rem 0;
    border: 1px solid var(--ds-border);
  }

  :global(.milkdown-editor .ProseMirror pre code) {
    background: none;
    padding: 0;
    color: inherit;
  }

  :global(.milkdown-editor .ProseMirror strong) {
    font-weight: 700;
  }

  :global(.milkdown-editor .ProseMirror em) {
    font-style: italic;
  }

  /* GFM: Strikethrough */
  :global(.milkdown-editor .ProseMirror del) {
    text-decoration: line-through;
  }

  /* GFM: Task lists */
  :global(.milkdown-editor .ProseMirror .task-list-item) {
    display: flex;
    align-items: flex-start;
    list-style: none;
  }

  :global(.milkdown-editor .ProseMirror .task-list-item input[type="checkbox"]) {
    margin-right: 0.5rem;
    margin-top: 0.25rem;
    cursor: pointer;
  }

  /* GFM: Tables */
  :global(.milkdown-editor .ProseMirror table) {
    border-collapse: collapse;
    width: 100%;
    margin: 0.25rem 0;
    overflow: hidden;
  }

  :global(.milkdown-editor .ProseMirror table th),
  :global(.milkdown-editor .ProseMirror table td) {
    border: 1px solid var(--ds-border);
    padding: 0.5rem 0.75rem;
    text-align: left;
  }

  :global(.milkdown-editor .ProseMirror table th) {
    background-color: var(--ds-surface);
    font-weight: 600;
  }

  :global(.milkdown-editor .ProseMirror table tr:hover) {
    background-color: var(--ds-surface);
  }

  /* GFM: Links (including autolinks) */
  :global(.milkdown-editor .ProseMirror a) {
    color: var(--ds-text-link);
    text-decoration: underline;
    cursor: pointer;
  }

  :global(.milkdown-editor .ProseMirror a:hover) {
    color: var(--ds-text-link-hovered);
  }

  /* ===== Image Block Styles ===== */

  /* Main container */
  :global(.milkdown-image-block) {
    margin: 0.5rem 0;
    border-radius: 6px;
    overflow: hidden;
  }

  /* Upload/Input mode */
  :global(.milkdown-image-block .image-edit) {
    display: flex;
    flex-direction: column;
    align-items: center;
    padding: 1.5rem;
    border: 2px dashed var(--ds-border);
    border-radius: 6px;
    background: var(--ds-surface);
  }

  :global(.milkdown-image-block .image-icon) {
    font-size: 2rem;
    margin-bottom: 0.75rem;
    color: var(--ds-text-subtlest);
  }

  :global(.milkdown-image-block .link-importer) {
    position: relative;
    display: flex;
    align-items: center;
    width: 100%;
    max-width: 400px;
    border: 1px solid var(--ds-border);
    border-radius: 6px;
    background: var(--ds-surface-raised);
    overflow: hidden;
  }

  :global(.milkdown-image-block .link-importer.focus) {
    border-color: var(--ds-interactive);
    box-shadow: 0 0 0 2px var(--ds-interactive-subtle);
  }

  :global(.milkdown-image-block .link-input-area) {
    flex: 1;
    padding: 0.5rem 0.75rem;
    border: none;
    font-size: 0.875rem;
    outline: none;
    background: transparent;
    color: var(--ds-text);
  }

  :global(.milkdown-image-block .placeholder) {
    position: absolute;
    inset: 0;
    display: flex;
    align-items: center;
    padding: 0 0.75rem;
    pointer-events: none;
  }

  :global(.milkdown-image-block .placeholder .uploader) {
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 4px 8px;
    background: var(--ds-surface);
    border-radius: 4px;
    font-size: 0.75rem;
    color: var(--ds-text);
    cursor: pointer;
    pointer-events: auto;
    transition: background 0.15s;
  }

  :global(.milkdown-image-block .placeholder .uploader:hover) {
    background: var(--ds-surface);
  }

  :global(.milkdown-image-block .placeholder .text) {
    margin-left: 0.5rem;
    font-size: 0.875rem;
    color: var(--ds-text-subtlest);
    cursor: text;
    pointer-events: auto;
  }

  :global(.milkdown-image-block .placeholder .hidden) {
    display: none;
  }

  :global(.milkdown-image-block .confirm) {
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 0.5rem;
    background: var(--ds-interactive);
    color: var(--ds-text-inverse);
    cursor: pointer;
    transition: background 0.15s;
  }

  :global(.milkdown-image-block .confirm:hover) {
    background: var(--ds-interactive-hovered);
  }

  /* Viewer mode (image loaded) */
  :global(.milkdown-image-block .image-wrapper) {
    position: relative;
    display: inline-block;
  }

  :global(.milkdown-image-block .image-wrapper img) {
    display: block;
    max-width: 100%;
    border-radius: 4px;
  }

  :global(.milkdown-theme-nord img) {
    box-shadow: none;
  }

  :global(.milkdown-image-block .operation) {
    position: absolute;
    top: 8px;
    right: 8px;
    display: flex;
    gap: 4px;
    opacity: 0;
    transition: opacity 0.15s;
  }

  :global(.milkdown-image-block:hover .operation) {
    opacity: 1;
  }

  :global(.milkdown-image-block .operation-item) {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    background: var(--ds-surface-raised);
    border: 1px solid var(--ds-border);
    border-radius: 4px;
    cursor: pointer;
    box-shadow: 0 1px 2px rgba(0, 0, 0, 0.05);
    transition: all 0.15s;
  }

  :global(.milkdown-image-block .operation-item:hover) {
    background: var(--ds-surface);
  }

  :global(.milkdown-image-block .image-resize-handle) {
    position: absolute;
    bottom: -6px;
    left: 0;
    right: 0;
    height: 4px;
    background: linear-gradient(90deg, var(--ds-interactive-subtle, #60a5fa), var(--ds-interactive, #2874bb));
    border-radius: 9999px;
    box-shadow:
      0 0 0 1px var(--ds-surface-raised, #ffffff),
      0 4px 10px rgba(59, 130, 246, 0.25);
    cursor: ns-resize;
    opacity: 0;
    transition: opacity 0.15s;
    z-index: 40;
  }

  :global(.milkdown-image-block:hover .image-resize-handle) {
    opacity: 0.98;
  }

  /* Hide caption functionality - not needed */
  :global(.milkdown-image-block .caption-input),
  :global(.milkdown-image-block .operation),
  :global(.milkdown-image-block .caption) {
    display: none !important;
  }

  /* ===== Mention Chip Styles ===== */
  :global(.mention-chip) {
    background: var(--ds-accent-blue-subtler, #e9f2ff);
    color: var(--ds-accent-blue, #0052cc);
    padding: 2px 6px;
    border-radius: 12px;
    font-weight: 500;
    font-size: 0.875em;
    cursor: pointer;
    display: inline;
    white-space: nowrap;
  }

  :global(.mention-chip:hover) {
    background: var(--ds-background-accent-blue-subtle, #cce0ff);
  }

  /* Hidden parts of quoted mentions (@" and ") */
  :global(.mention-chip-hidden) {
    font-size: 0;
    width: 0;
    display: inline;
  }

  /* Name part of quoted mentions - needs @ prefix via CSS */
  :global(.mention-chip-name::before) {
    content: '@';
  }

  /* ===== User Hover Card Styles ===== */
  .user-hover-card {
    position: fixed;
    transform: translate(-50%, -100%);
    background: var(--ds-surface-raised, white);
    border: 1px solid var(--ds-border, #dfe1e6);
    border-radius: 8px;
    padding: 12px;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
    min-width: 200px;
    z-index: 1000;
  }

  .hc-loading, .hc-not-found {
    font-size: 13px;
    color: var(--ds-text-subtle, #6b778c);
    padding: 4px;
  }

  .hc-user-card {
    display: flex;
    gap: 12px;
    align-items: center;
  }

  .hc-avatar {
    width: 40px;
    height: 40px;
    border-radius: 50%;
    background: #3b82f6;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    overflow: hidden;
  }

  .hc-avatar img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }

  .hc-initials {
    color: white;
    font-weight: 600;
    font-size: 14px;
  }

  .hc-info {
    min-width: 0;
  }

  .hc-name {
    font-weight: 600;
    color: var(--ds-text, #172b4d);
    font-size: 14px;
  }

  .hc-email {
    font-size: 12px;
    color: var(--ds-text-subtle, #6b778c);
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .hc-username {
    font-size: 12px;
    color: var(--ds-accent-blue, #0052cc);
  }
</style>
