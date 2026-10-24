<script>
  import { onMount, onDestroy } from 'svelte';
  import { useEventListener } from 'runed';
  import { Editor, rootCtx, defaultValueCtx, editorViewOptionsCtx, editorViewCtx, serializerCtx, remarkPluginsCtx } from '@milkdown/kit/core';
  import { commonmark, toggleStrongCommand, toggleEmphasisCommand, wrapInBulletListCommand, wrapInOrderedListCommand, toggleInlineCodeCommand } from '@milkdown/kit/preset/commonmark';
  import { gfm, toggleStrikethroughCommand } from '@milkdown/kit/preset/gfm';
  import { listener, listenerCtx } from '@milkdown/kit/plugin/listener';
  import { clipboard } from '@milkdown/kit/plugin/clipboard';
  import { upload, uploadConfig } from '@milkdown/kit/plugin/upload';
  import { insert, callCommand } from '@milkdown/kit/utils';
  import { replaceAll } from '@milkdown/utils';
  import { nord } from '@milkdown/theme-nord';
  import '@milkdown/theme-nord/style.css';
  import { imageBlockComponent } from '@milkdown/kit/component/image-block';
  import Bold from '@lucide/svelte/icons/bold';
  import Italic from '@lucide/svelte/icons/italic';
  import Code from '@lucide/svelte/icons/code';
  import List from '@lucide/svelte/icons/list';
  import ListOrdered from '@lucide/svelte/icons/list-ordered';
  import Strikethrough from '@lucide/svelte/icons/strikethrough';
  import ImageIcon from '@lucide/svelte/icons/image';
  import Workflow from '@lucide/svelte/icons/workflow';
  import BookOpen from '@lucide/svelte/icons/book-open';
  import { getAssignableUsers, getUsers } from '../api/users.js';
  import { attachments as attachmentsApi } from '../api/misc.js';
  import { pages } from '../api/pages.js';
  import { confirm } from '../composables/useConfirm.js';
  import Tooltip from '../components/Tooltip.svelte';
  import FileInput from '../components/FileInput.svelte';
  import MentionPicker from '../pickers/MentionPicker.svelte';
  import { mentionDecorationPlugin } from './milkdown-mention-mark.js';
  import { mentionTextFor, resolveMentionUser } from './mentionResolver.js';
  import { linkSanitizerPlugin } from './milkdown-link-sanitizer.js';
  import { imageBlockerPlugin } from './milkdown-image-blocker.js';
  import { rewriteBreakHTML } from './milkdown-hardbreak.js';
  import { excalidrawBlock } from './milkdown-excalidraw-block.svelte.js';
  import PageDiagramModal from '../features/pages/PageDiagramModal.svelte';
  import { highlightCodeBlocks } from './code-highlight.js';
  import { t } from '../stores/i18n.svelte.js';
  import { attachmentStatus } from '../stores/attachmentStatus.svelte.js';
  import { navigate } from '../router.js';

  let {
    content = $bindable(''), placeholder = '', readonly = false,
    showToolbar = false, hideToolbarUntilFocus = false, itemId = null, entityType = null,
    entityId = null, onImageInsert = null, onContentChange = null, isPersonalWorkspace = false, compact = false,
    customUploadFn = null, downloadUrlBase = '/api/attachments', deferImageUploads = false,
    onDeferredImageUpload = null,
    allowImageUpload = true,
    enableDiagrams = false,
    workspaceId = null,
    enablePageLinks = false,
    expectedContentHash = '',
    onBeforeDiagramOpen = async () => {},
    onDiagramPersisted = (_payload) => {},
    testId = null
  } = $props();

  // Diagram modal state — only meaningful when enableDiagrams=true.
  let diagramModal = $state({ open: false, mode: 'create', attachmentId: null, name: '', getPos: null });

  // Page-link picker state — only meaningful when enablePageLinks=true.
  let pageLinkPickerOpen = $state(false);
  let pageLinkQuery = $state('');
  let pageLinkResults = $state([]);
  let pageLinkSearching = $state(false);
  let pageLinkTimer = null;
  let pageLinkSearchVersion = 0;

  const effectivePlaceholder = $derived(placeholder || t('editors.enterText'));

  // Derive attachments enabled from store (falls back to true if not yet loaded to avoid flash)
  const attachmentsEnabled = $derived(attachmentStatus.loaded ? attachmentStatus.enabled : true);

  // Allow uploads when either the main attachment system is enabled OR a custom upload function is provided.
  // `allowImageUpload={false}` turns uploads off for content that has no attachment support (e.g. milestones).
  const canUploadImages = $derived(allowImageUpload && (attachmentsEnabled || !!customUploadFn));

  // Compute effective entity info (supports both old itemId and new entityType/entityId)
  const effectiveEntityType = $derived(entityType || (itemId ? 'item' : null));
  const effectiveEntityId = $derived(entityId || itemId);

  let isFocused = $state(false);
  const toolbarVisible = $derived(showToolbar && !readonly && (!hideToolbarUntilFocus || isFocused));

  // Blur collapses the focus-revealed toolbar, which reflows everything below
  // the editor. Hide it on a short delay instead so a click that started on a
  // control under the editor (e.g. the comment submit button) lands before the
  // shift; refocusing cancels the pending hide (WI-1349).
  let toolbarHideTimer = null;

  function handleEditorFocusIn() {
    clearTimeout(toolbarHideTimer);
    toolbarHideTimer = null;
    isFocused = true;
  }

  function handleEditorFocusOut(e) {
    if (e.currentTarget.contains(/** @type {Node | null} */ (e.relatedTarget))) return;
    clearTimeout(toolbarHideTimer);
    toolbarHideTimer = setTimeout(() => {
      toolbarHideTimer = null;
      isFocused = false;
    }, 250);
  }

  let editorElement = $state(null);
  let fileInput = $state(null);
  let editor = $state(null);
  let disposed = false;
  let initialContent = content;

  async function destroyEditor(target) {
    if (!target) return;
    try {
      await target.destroy();
    } catch (e) {
      console.error('Error destroying editor:', e);
    }
  }

  // Mention picker state
  let mentionPickerOpen = $state(false);
  let mentionQuery = $state('');
  let mentionPosition = $state({ x: 0, y: 0 });
  let mentionRange = $state(null); // { from, to } positions in the document

  // User hover card state
  let hoverCardVisible = $state(false);
  let hoverCardPosition = $state({ x: 0, y: 0 });
  let hoverCardUser = $state(null);
  let hoverCardMatches = $state([]);
  let hoverCardLoading = $state(false);
  let hoverCardTimeout = $state(null);
  let hideCardTimeout = $state(null);
  let hoverCardRequest = 0;
  // Workspace roster used to resolve mention identifiers, loaded once per editor.
  let mentionRoster = null;
  let mentionRosterPromise = null;

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
    const fromPos = selection.$from;

    // Get text before cursor in current text block
    const textBefore = fromPos.parent.textContent.slice(0, fromPos.parentOffset);

    // Look for @ trigger pattern: @ followed by optional word characters
    const mentionMatch = textBefore.match(/(?<![a-zA-Z0-9.])@([a-zA-Z0-9_.-]*)$/);

    if (mentionMatch) {
      // Calculate positions
      const matchStart = fromPos.pos - mentionMatch[0].length;
      const matchEnd = fromPos.pos;

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

  function handleMentionSelect(user) {
    if (!editor || !mentionRange) return;
    const range = mentionRange;

    // The picker supplies the identifier from the full roster so a duplicated
    // display name becomes @username instead of an ambiguous @"Name".
    const mentionText = user.mentionText || mentionTextFor(user, [user]);

    // Get the editor view and replace the mention range with the formatted mention
    editor.action((ctx) => {
      const view = ctx.get(editorViewCtx);
      const { state, dispatch } = view;

      // Replace @query with @"Display Name" or @username
      const tr = state.tr.replaceWith(
        range.from,
        range.to,
        state.schema.text(mentionText + ' ')
      );
      dispatch(tr);
      view.focus();

      // The listener plugin normally mirrors document changes back into the
      // bindable Markdown value. Under a busy event loop, however, a submit
      // can observe the old @query before that listener callback runs. Keep
      // programmatic mention insertion atomic from the form's perspective.
      const markdown = ctx.get(serializerCtx)(tr.doc);
      content = markdown;
      onContentChange?.(markdown);
    });

    closeMentionPicker();
  }

  function handleMentionCancel() {
    closeMentionPicker();
  }

  // User hover card functions
  function ensureMentionRoster() {
    if (mentionRoster) return Promise.resolve(mentionRoster);
    if (!mentionRosterPromise) {
      mentionRosterPromise = (workspaceId
        ? getAssignableUsers(workspaceId)
        : getUsers())
        .then((roster) => {
          mentionRoster = roster || [];
          return mentionRoster;
        })
        .catch((error) => {
          mentionRosterPromise = null;
          throw error;
        });
    }
    return mentionRosterPromise;
  }

  // Resolve the hovered mention against the same roster the picker offers, so
  // the card shows the person the mention refers to. A display-name mention
  // that matches several people shows every candidate instead of guessing.
  async function loadUserForHoverCard(identifier) {
    const request = ++hoverCardRequest;
    hoverCardLoading = true;
    hoverCardUser = null;
    hoverCardMatches = [];
    try {
      const roster = await ensureMentionRoster();
      if (request !== hoverCardRequest) return;
      const resolved = resolveMentionUser(roster, identifier);
      hoverCardUser = resolved.user;
      hoverCardMatches = resolved.user ? [] : resolved.matches;
    } catch (e) {
      if (request !== hoverCardRequest) return;
      console.error('Failed to load users for hover card:', e);
    }
    if (request === hoverCardRequest) hoverCardLoading = false;
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
      hoverCardMatches = [];
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

  // Runed event listeners for mention hover (more efficient than MutationObserver)
  useEventListener(() => editorElement, 'mouseover', handleMentionMouseOver);
  useEventListener(() => editorElement, 'mouseout', handleMentionMouseOut);

  // Shared uploader logic for drag/drop/paste and the toolbar button
  async function uploadImages(files, schema, shouldInsertMarkdown = false) {
    if (!canUploadImages) {
      return { nodes: [], attachments: [] };
    }

    const images = [];
    for (let i = 0; i < files.length; i++) {
      const file = files.item(i);
      if (!file || !file.type.includes('image')) continue;
      images.push(file);
    }

    if (images.length === 0) {
      return { nodes: [], attachments: [] };
    }

    const results = await Promise.all(
      images.map(async (image) => {
        try {
          if (deferImageUploads && !effectiveEntityId && !customUploadFn) {
            const src = URL.createObjectURL(image);
            onDeferredImageUpload?.({ file: image, url: src });
            const node = schema?.nodes?.image?.createAndFill({
              src,
              alt: image.name,
            }) || null;

            return {
              node,
              attachment: null,
              src,
              alt: image.name,
            };
          }

          let result;
          if (!customUploadFn && effectiveEntityType === 'item' && effectiveEntityId) {
            const attachment = await attachmentsApi.uploadToItem(effectiveEntityId, image);
            result = { success: true, attachment };
          } else {
            const formData = new FormData();
            formData.append('file', image);
            if (effectiveEntityId && effectiveEntityType) {
              formData.append('entity_id', effectiveEntityId.toString());
              formData.append('entity_type', effectiveEntityType);
            }
            result = customUploadFn
              ? await customUploadFn(formData)
              : await attachmentsApi.upload(formData);
          }
          if (result.success && result.attachment) {
            const src = downloadUrlBase === '/api/attachments'
              ? attachmentsApi.getContentUrl(result.attachment.id)
              : `${downloadUrlBase}/${result.attachment.id}/download`;
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

  async function openDiagramCreate() {
    if (readonly || !enableDiagrams) return;
    await onBeforeDiagramOpen();
    diagramModal = { open: true, mode: 'create', attachmentId: null, name: '', getPos: null };
  }

  function pageIdFromHref(href) {
    const match = String(href || '').trim().match(/^page:(\d+)$/i);
    if (!match) return null;
    const id = Number(match[1]);
    return Number.isSafeInteger(id) && id > 0 ? id : null;
  }

  function handleInternalPageLinkClick(event) {
    if (!workspaceId) return false;
    const link = event.target?.closest?.('a[href]');
    if (!link) return false;
    const targetPageId = pageIdFromHref(link.getAttribute('href'));
    if (!targetPageId) return false;
    event.preventDefault();
    event.stopPropagation();
    navigate(`/workspaces/${workspaceId}/pages/${targetPageId}`);
    return true;
  }

  function handleDiagramSaved({ attachmentId, name, contentHash, pageContent }) {
    if (!editor) return;
    editor.action(replaceAll(pageContent || ''));
    onDiagramPersisted({ attachmentId, name, contentHash, pageContent });
  }

  async function deleteDiagramNode(attachmentId, getPos) {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('components.diagram.confirmDelete'),
      confirmText: t('common.delete'),
      variant: 'danger',
    });
    if (!confirmed || !editor || typeof getPos !== 'function') return;

    editor.action((ctx) => {
      const view = ctx.get(editorViewCtx);
      let position;
      try {
        position = getPos();
      } catch {
        return;
      }
      if (!Number.isInteger(position)) return;
      const node = view.state.doc.nodeAt(position);
      if (
        !node ||
        node.type.name !== 'excalidraw' ||
        node.attrs.attachmentId !== attachmentId
      ) {
        return;
      }
      view.dispatch(
        view.state.tr.delete(position, position + node.nodeSize).scrollIntoView()
      );
      view.focus();
    });
  }

  onMount(async () => {
    try {
      const builder = Editor.make()
        .config((ctx) => {
          ctx.set(rootCtx, editorElement);
          ctx.set(defaultValueCtx, initialContent || '');
          // Parse `<br>`-style html as hard breaks (see milkdown-hardbreak.js).
          // Registered through remarkPluginsCtx here — config runs before the
          // schema step snapshots the remark processor; a `.use($remark)`
          // plugin appends after that snapshot and never takes effect.
          ctx.update(remarkPluginsCtx, (plugins) => [
            ...plugins,
            { plugin: rewriteBreakHTML, options: {} },
          ]);
          ctx.get(listenerCtx).markdownUpdated((ctx, markdown) => {
            if (disposed) return;
            // Listener notifications can be delivered after a newer editor
            // transaction (for example, selecting an @ mention). Always
            // serialize the current document so a delayed callback cannot
            // regress the bindable value to an older Markdown snapshot.
            let currentMarkdown = markdown;
            try {
              const view = ctx.get(editorViewCtx);
              if (view) currentMarkdown = ctx.get(serializerCtx)(view.state.doc);
            } catch {
              // The listener can outlive the editor view during teardown.
            }
            content = currentMarkdown;
            if (onContentChange) onContentChange(currentMarkdown);
          });
          // Detect mentions on all changes: mobile keyboards may omit keyup.
          // Overlap with desktop keyup is safe because the check only reads state.
          ctx.get(listenerCtx).updated((updateCtx) => {
            if (readonly || disposed) return;
            try {
              const view = updateCtx.get(editorViewCtx);
              if (view) checkForMentionTrigger(view);
            } catch {
              // Ignore an editor view not yet ready.
            }
          });
          // Context may not be initialized for update.
          ctx.set(editorViewOptionsCtx, {
            editable: () => !readonly,
            attributes: {
              class: 'milkdown-editor-content',
              'data-placeholder': effectivePlaceholder
            },
            // Handle DOM events for mention detection
            handleDOMEvents: {
              keydown: (_view, event) => {
                // Bypass ProseMirror's bare-Escape prevention so enclosing
                // modals close; let the open mention picker handle Escape.
                if (event.key === 'Escape' && !mentionPickerOpen) {
                  return true;
                }
                return false;
              },
              keyup: (view, event) => {
                if (!readonly) {
                  checkForMentionTrigger(view);
                }
                return false; // Don't prevent default
              },
              click: (view, event) => {
                if (handleInternalPageLinkClick(event)) {
                  return true;
                }
                // Close mention picker when clicking elsewhere
                if (mentionPickerOpen) {
                  closeMentionPicker();
                }
                return false;
              }
            }
          });

          // Configure upload plugin following official docs pattern (only if attachments enabled)
          if (!readonly && canUploadImages) {
            ctx.update(uploadConfig.key, (prev) => ({
              ...prev,
              uploader,
            }));
          }
          // With uploads explicitly off, swallow pasted/dropped files instead of
          // letting the plugin's default uploader embed them as base64 images.
          // (imageBlockerPlugin would reject the insert anyway, but it would also
          // drop the upload plugin's placeholder cleanup in the same transaction.)
          if (!readonly && !allowImageUpload) {
            ctx.update(uploadConfig.key, (prev) => ({
              ...prev,
              uploader: async () => [],
            }));
          }
        })
        .config(nord)
        .use(commonmark)
        .use(gfm)
        .use(listener)
        .use(clipboard)
        .use(upload)  // Include upload in main chain per docs
        .use(imageBlockComponent)  // Enable image resizing
        .use(mentionDecorationPlugin)  // Add mention chip decorations
        .use(linkSanitizerPlugin);     // Sanitize dangerous URL schemes in links/images

      if (enableDiagrams) {
        builder.use(excalidrawBlock);
      }
      if (!readonly && !allowImageUpload) {
        builder.use(imageBlockerPlugin);
      }

      const created = await builder.create();
      if (disposed) {
        // The component unmounted while the async builder was still running;
        // destroy the late editor here so teardown owns it exactly once.
        await destroyEditor(created);
        return;
      }
      editor = created;
    } catch (error) {
      if (!disposed) {
        console.error('Failed to initialize Milkdown editor:', error);
      }
    }
  });

  // Listen for "edit me" requests from node views. The custom event bubbles
  // from inside the diagram block view; the wrapper owns the modal so the
  // node view stays presentational.
  $effect(() => {
    if (!editorElement || !enableDiagrams) return;
    const handler = async (e) => {
      const { attachmentId, name, getPos } = e.detail || {};
      await onBeforeDiagramOpen();
      diagramModal = { open: true, mode: 'edit', attachmentId, name: name || '', getPos };
    };
    const deleteHandler = (e) => {
      const { attachmentId, getPos } = e.detail || {};
      void deleteDiagramNode(attachmentId, getPos);
    };
    editorElement.addEventListener('excalidraw:edit', handler);
    editorElement.addEventListener('excalidraw:delete', deleteHandler);
    return () => {
      editorElement.removeEventListener('excalidraw:edit', handler);
      editorElement.removeEventListener('excalidraw:delete', deleteHandler);
    };
  });

  onDestroy(async () => {
    disposed = true;
    if (hoverCardTimeout) {
      clearTimeout(hoverCardTimeout);
    }
    if (hideCardTimeout) {
      clearTimeout(hideCardTimeout);
    }
    if (toolbarHideTimer) {
      clearTimeout(toolbarHideTimer);
      toolbarHideTimer = null;
    }
    clearTimeout(pageLinkTimer);
    if (editor) {
      const target = editor;
      editor = null;
      await destroyEditor(target);
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

  // Insert markdown at the cursor (canned responses, WI-1138). The listener
  // plugin mirrors the document change back into the bindable content value.
  export function insertMarkdown(markdown) {
    if (!editor || readonly) {
      console.warn('Cannot insert markdown: editor not ready or readonly');
      return;
    }
    try {
      editor.action(insert(markdown));
    } catch (error) {
      console.error('Failed to insert markdown:', error);
    }
  }

  function togglePageLinkPicker() {
    pageLinkPickerOpen = !pageLinkPickerOpen;
    if (pageLinkPickerOpen) {
      pageLinkQuery = '';
      pageLinkResults = [];
      pageLinkSearchVersion += 1;
    }
  }

  function closePageLinkPicker() {
    clearTimeout(pageLinkTimer);
    pageLinkSearchVersion += 1;
    pageLinkPickerOpen = false;
  }

  function handlePageLinkQuery(event) {
    pageLinkQuery = event.currentTarget.value;
    clearTimeout(pageLinkTimer);
    const trimmed = pageLinkQuery.trim();
    if (!trimmed || !workspaceId) {
      pageLinkSearchVersion += 1;
      pageLinkResults = [];
      pageLinkSearching = false;
      return;
    }
    pageLinkSearching = true;
    const version = ++pageLinkSearchVersion;
    pageLinkTimer = setTimeout(async () => {
      try {
        const result = await pages.searchPages(workspaceId, trimmed, { limit: 8 });
        if (version !== pageLinkSearchVersion) return;
        pageLinkResults = result?.results ?? (Array.isArray(result) ? result : []);
      } catch (error) {
        if (version !== pageLinkSearchVersion) return;
        console.error('Page search failed:', error);
        pageLinkResults = [];
      } finally {
        if (version === pageLinkSearchVersion) pageLinkSearching = false;
      }
    }, 250);
  }

  function insertPageLink(page) {
    if (!editor || readonly || !page?.id) return;
    const title = String(page.title || `Page ${page.id}`).replace(/[/\\]/g, ' ');
    try {
      editor.action(insert(`[${title}](page:${page.id})`));
      closePageLinkPicker();
      focus();
    } catch (error) {
      console.error('Failed to insert page link:', error);
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
  $effect(() => {
    if (editor && readonly) {
      editor.action(replaceAll(content || ''));
      requestAnimationFrame(() => {
        highlightCodeBlocks(editorElement).catch((e) => console.warn('code highlight failed', e));
      });
    }
  });

  // Page permissions load after the document, so a diagram node view can be
  // created while the editor is provisionally readonly. ProseMirror does not
  // update custom node views when only `view.editable` changes, so replace the
  // node-view map with an equivalent copy to make it rebuild those views using
  // the current permission state.
  $effect(() => {
    readonly;
    if (!editor) return;
    editor.action((ctx) => {
      const view = ctx.get(editorViewCtx);
      view.setProps({ nodeViews: { ...(view.props.nodeViews || {}) } });
    });
  });
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="milkdown-wrapper" class:has-toolbar={toolbarVisible}
  onfocusin={handleEditorFocusIn}
  onfocusout={handleEditorFocusOut}
>
  {#if toolbarVisible}
    <div class="milkdown-toolbar" tabindex="-1" data-testid="milkdown-toolbar">
      <Tooltip content={t('editors.bold')} placement="bottom">
        {#snippet children()}
          <button type="button" class="toolbar-btn" tabindex="-1" onclick={toggleBold} aria-label={t('editors.bold')}>
            <Bold size={14} />
          </button>
        {/snippet}
      </Tooltip>
      <Tooltip content={t('editors.italic')} placement="bottom">
        {#snippet children()}
          <button type="button" class="toolbar-btn" tabindex="-1" onclick={toggleItalic} aria-label={t('editors.italic')}>
            <Italic size={14} />
          </button>
        {/snippet}
      </Tooltip>
      <Tooltip content={t('editors.strikethrough')} placement="bottom">
        {#snippet children()}
          <button type="button" class="toolbar-btn" tabindex="-1" onclick={toggleStrikethrough} aria-label={t('editors.strikethrough')}>
            <Strikethrough size={14} />
          </button>
        {/snippet}
      </Tooltip>
      <Tooltip content={t('editors.inlineCode')} placement="bottom">
        {#snippet children()}
          <button type="button" class="toolbar-btn" tabindex="-1" onclick={toggleCode} aria-label={t('editors.inlineCode')}>
            <Code size={14} />
          </button>
        {/snippet}
      </Tooltip>
      <div class="toolbar-divider"></div>
      <Tooltip content={t('editors.bulletList')} placement="bottom">
        {#snippet children()}
          <button type="button" class="toolbar-btn" tabindex="-1" onclick={toggleBulletList} aria-label={t('editors.bulletList')}>
            <List size={14} />
          </button>
        {/snippet}
      </Tooltip>
      <Tooltip content={t('editors.numberedList')} placement="bottom">
        {#snippet children()}
          <button type="button" class="toolbar-btn" tabindex="-1" onclick={toggleOrderedList} aria-label={t('editors.numberedList')}>
            <ListOrdered size={14} />
          </button>
        {/snippet}
      </Tooltip>
      {#if canUploadImages}
        <Tooltip content={t('editors.insertImage')} placement="bottom">
          {#snippet children()}
            <button type="button" class="toolbar-btn" tabindex="-1" onclick={openFilePicker} aria-label={t('editors.insertImage')}>
              <ImageIcon size={14} />
            </button>
          {/snippet}
        </Tooltip>
      {/if}
      {#if enableDiagrams}
        <Tooltip content={t('editors.insertDiagram')} placement="bottom">
          {#snippet children()}
            <button type="button" class="toolbar-btn" tabindex="-1" onclick={openDiagramCreate} aria-label={t('editors.insertDiagram')} data-testid="milkdown-insert-diagram">
              <Workflow size={14} />
            </button>
          {/snippet}
        </Tooltip>
      {/if}
      {#if enablePageLinks && workspaceId}
        <Tooltip content={t('editors.insertPageLink')} placement="bottom">
          {#snippet children()}
            <button type="button" class="toolbar-btn" tabindex="-1" onclick={togglePageLinkPicker} aria-label={t('editors.insertPageLink')} data-testid="milkdown-insert-page-link">
              <BookOpen size={14} />
            </button>
          {/snippet}
        </Tooltip>
      {/if}
    </div>
  {/if}
  <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
  <div
    bind:this={editorElement}
    data-testid={testId}
    data-ready={editor ? 'true' : 'false'}
    class="milkdown-editor"
    class:readonly
    class:compact
    class:has-toolbar={toolbarVisible}
    onclick={handleClick}
  ></div>
</div>
<FileInput
  bind:inputRef={fileInput}
  accept="image/*"
  multiple
  tabindex="-1"
  ariaHidden="true"
  class="hidden-file-input"
  onchange={handleFileInputChange}
/>
{#if enableDiagrams}
  <PageDiagramModal
    bind:open={diagramModal.open}
    mode={diagramModal.mode}
    initialAttachmentId={diagramModal.attachmentId}
    initialName={diagramModal.name}
    {workspaceId}
    pageId={entityType === 'page' ? entityId : null}
    {expectedContentHash}
    onSaved={handleDiagramSaved}
  />
{/if}
<!-- Page-link picker for inserting knowledge-page links -->
{#if pageLinkPickerOpen}
  <!-- Container only: the click handler stops propagation to the editor.
       role="presentation" keeps it out of the accessibility tree. -->
  <div
    class="page-link-picker"
    role="presentation"
    data-testid="page-link-picker"
    onclick={(e) => e.stopPropagation()}
  >
    <input
      type="text"
      class="page-link-input"
      placeholder={t('editors.searchPagesToLink')}
      value={pageLinkQuery}
      oninput={handlePageLinkQuery}
      data-testid="page-link-search"
    />
    {#if pageLinkSearching}
      <div class="page-link-empty">{t('common.searching')}</div>
    {:else if pageLinkResults.length === 0}
      <div class="page-link-empty">{t('common.noResults')}</div>
    {:else}
      <ul class="page-link-results">
        {#each pageLinkResults as page (page.id)}
          <li>
            <button
              type="button"
              class="page-link-option"
              data-testid="page-link-option"
              onclick={() => insertPageLink(page)}
            >
              <BookOpen size={13} />
              <span class="page-link-title">{page.title}</span>
            </button>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
{/if}
<!-- Mention Picker for @ mentions -->
<MentionPicker
  bind:open={mentionPickerOpen}
  query={mentionQuery}
  position={mentionPosition}
  {workspaceId}
  {isPersonalWorkspace}
  onSelect={handleMentionSelect}
  onCancel={handleMentionCancel}
/>
<!-- User Hover Card for mentions -->
{#snippet hoverUserCard(user, candidate = false)}
  <div
    class="hc-user-card"
    data-testid={candidate ? `mention-hover-card-candidate-${user.id}` : 'mention-hover-card-user'}
  >
    <div class="hc-avatar">
      {#if user.avatar_url}
        <img src={user.avatar_url} alt="" />
      {:else}
        <span class="hc-initials">{getInitials(user)}</span>
      {/if}
    </div>
    <div class="hc-info">
      <div class="hc-name">{user.first_name || ''} {user.last_name || ''}</div>
      {#if user.email}
        <div class="hc-email">{user.email}</div>
      {/if}
      {#if user.username}
        <div class="hc-username">@{user.username}</div>
      {/if}
    </div>
  </div>
{/snippet}

{#if hoverCardVisible}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="user-hover-card"
    data-testid="mention-hover-card"
    style="left: {hoverCardPosition.x}px; top: {hoverCardPosition.y}px;"
    onmouseenter={hoverCardMouseEnter}
    onmouseleave={hoverCardMouseLeave}
    role="tooltip"
  >
    {#if hoverCardLoading}
      <div class="hc-loading">{t('common.loading')}</div>
    {:else if hoverCardUser}
      {@render hoverUserCard(hoverCardUser)}
    {:else if hoverCardMatches.length > 1}
      <div class="hc-candidates" data-testid="mention-hover-card-candidates">
        <div class="hc-candidates-title">{t('editors.multipleUsersMatch')}</div>
        {#each hoverCardMatches as candidate (candidate.id)}
          {@render hoverUserCard(candidate, true)}
        {/each}
      </div>
    {:else}
      <div class="hc-not-found">{t('editors.userNotFound')}</div>
    {/if}
  </div>
{/if}

<style>
  :global(.hidden-file-input) {
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

  /* Show the placeholder only for a truly empty document. `:only-child` ignores
     text nodes, so match the empty-paragraph trailing break by class — a hard
     break (`<br data-type="hardbreak">`) must not re-trigger the placeholder. */
  :global(.milkdown-editor:not(.readonly) .ProseMirror[data-placeholder]:has(> p:first-child:last-child:empty)::before),
  :global(.milkdown-editor:not(.readonly) .ProseMirror[data-placeholder]:has(> p:first-child:last-child > br.ProseMirror-trailingBreak:only-child)::before) {
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
    margin: 1.5rem 0 0.75rem;
  }

  :global(.milkdown-editor .ProseMirror h2) {
    font-size: 1.2rem;
    font-weight: 600;
    margin: 1.25rem 0 0.625rem;
  }

  :global(.milkdown-editor .ProseMirror h3) {
    font-size: 1.05rem;
    font-weight: 600;
    margin: 1rem 0 0.5rem;
  }

  :global(.milkdown-editor .ProseMirror p) {
    margin: 0.5rem 0;
  }

  :global(.milkdown-editor .ProseMirror li p) {
    margin: 0.25rem 0;
  }

  :global(.milkdown-editor .ProseMirror > h1:first-child),
  :global(.milkdown-editor .ProseMirror > h2:first-child),
  :global(.milkdown-editor .ProseMirror > h3:first-child) {
    margin-top: 0;
  }

  /* Preserve blank lines from remarkPreserveEmptyLinePlugin */
  :global(.milkdown-editor .ProseMirror br:not(.ProseMirror-trailingBreak)) {
    display: block;
    content: "";
    margin-top: 0.5rem;
  }

  :global(.milkdown-editor .ProseMirror ul),
  :global(.milkdown-editor .ProseMirror ol) {
    padding-left: 1.5rem;
    margin: 0.5rem 0;
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
    margin: 0.75rem 0;
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

  :global(.milkdown-editor .ProseMirror pre:not(.shiki)) {
    background-color: var(--ds-surface-card);
    color: var(--ds-text);
    padding: 1rem;
    border-radius: 0.375rem;
    overflow-x: auto;
    margin: 0.75rem 0;
    border: 1px solid var(--ds-border);
  }

  :global(.milkdown-editor .ProseMirror pre:not(.shiki) code) {
    background: none;
    padding: 0;
    color: inherit;
  }

  :global(.milkdown-editor .ProseMirror pre.shiki) {
    padding: 1rem;
    border-radius: 0.375rem;
    overflow-x: auto;
    margin: 0.75rem 0;
    border: 1px solid var(--ds-border);
    font-size: 0.875rem;
    color: var(--shiki-light);
    background-color: var(--shiki-light-bg);
  }

  :global(.milkdown-editor .ProseMirror pre.shiki code) {
    background: none;
    padding: 0;
    font-family: ui-monospace, monospace;
  }

  :global(.milkdown-editor .ProseMirror pre.shiki span) {
    color: var(--shiki-light);
    background-color: var(--shiki-light-bg);
    font-style: var(--shiki-light-font-style);
    font-weight: var(--shiki-light-font-weight);
    text-decoration: var(--shiki-light-text-decoration);
  }

  :global(html[data-color-mode='dark'] .milkdown-editor .ProseMirror pre.shiki),
  :global(html[data-color-mode='dark'] .milkdown-editor .ProseMirror pre.shiki span) {
    color: var(--shiki-dark) !important;
    background-color: var(--shiki-dark-bg) !important;
    font-style: var(--shiki-dark-font-style) !important;
    font-weight: var(--shiki-dark-font-weight) !important;
    text-decoration: var(--shiki-dark-text-decoration) !important;
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
    table-layout: auto;
    width: 100%;
    margin: 0.75rem 0;
    overflow: hidden;
  }

  /* Override nord theme's alternating column backgrounds */
  :global(.milkdown-editor .ProseMirror :where(td, th):nth-child(odd)) {
    background-color: transparent;
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
    max-width: 100%;
  }

  :global(.milkdown-image-block .image-wrapper img),
  :global(.milkdown-editor .ProseMirror img) {
    display: block;
    max-width: 100%;
    height: auto;
    object-fit: contain;
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

  /* ===== Page-link picker styles ===== */
  .page-link-picker {
    background: var(--ds-surface-raised, white);
    border: 1px solid var(--ds-border, #dfe1e6);
    border-radius: 8px;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
    width: 280px;
    max-width: 100%;
    padding: 6px;
    margin-top: 4px;
    z-index: 1000;
  }

  .page-link-input {
    width: 100%;
    box-sizing: border-box;
    font-size: 13px;
    padding: 6px 8px;
    border: 1px solid var(--ds-border, #dfe1e6);
    border-radius: 6px;
    background: var(--ds-bg, white);
    color: var(--ds-text, #172b4d);
    margin-bottom: 4px;
  }

  .page-link-input:focus {
    outline: none;
    border-color: var(--ds-interactive, #2563eb);
  }

  .page-link-empty {
    font-size: 12px;
    color: var(--ds-text-subtle, #6b778c);
    padding: 6px 4px;
  }

  .page-link-results {
    list-style: none;
    margin: 0;
    padding: 0;
    max-height: 200px;
    overflow-y: auto;
  }

  .page-link-option {
    display: flex;
    align-items: center;
    gap: 6px;
    width: 100%;
    text-align: left;
    background: none;
    border: none;
    border-radius: 4px;
    padding: 6px;
    font-size: 13px;
    color: var(--ds-text, #172b4d);
    cursor: pointer;
  }

  .page-link-option:hover {
    background: var(--ds-bg-subtle, #f4f5f7);
  }

  .page-link-title {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
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

  .hc-candidates {
    display: flex;
    flex-direction: column;
    gap: 10px;
    min-width: 220px;
  }

  .hc-candidates-title {
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    color: var(--ds-text-subtle, #6b778c);
  }
</style>
