<script>
  import { onMount, onDestroy } from 'svelte';
  import { pageLabels, pages } from '../../api/pages.js';
  import { navigate } from '../../router.js';
  import PageEditor from './PageEditor.svelte';
  import PageToolbar from './PageToolbar.svelte';
  import PageLabelsRow from './PageLabelsRow.svelte';
  import PageToc from './PageToc.svelte';
  import PagePermissionsDialog from './PagePermissionsDialog.svelte';
  import PageMoveDialog from './PageMoveDialog.svelte';
  import PagesHistoryDrawer from './PagesHistoryDrawer.svelte';
  import { workspaceIconMap } from '../../utils/icons.js';
  import Book from '@tabler/icons-svelte-runes/icons/book';
  import { parseMarkdownHeadings, slugify } from './markdownToc.js';
  import { confirm } from '../../composables/useConfirm.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { pagesTreeRefresh } from './pagesTreeRefresh.svelte.js';
  import { pagesFocusTitle } from './pagesFocusTitle.svelte.js';
  import { createPageAutosaveQueue } from './pageAutosaveQueue.js';
  import { createPageLinksState } from './pageLinksState.svelte.js';
  import { mergePageUpdate } from './pageState.js';
  import { agentRuns } from '../../stores/agentRuns.svelte.js';
  import { workspacePermissions } from '../../stores/workspacePermissions.svelte.js';

  /** Right-pane knowledge-page editor with sidebar-owned tree/actions and
   * debounced autosave instead of an explicit Save button.
   * Sub-surfaces: PageToolbar (header row + title), PageLabelsRow
   * (icon + label chips), PageEditor (body), PageToc (read-mode rail). */
  let { workspaceId, pageId = null } = $props();

  // Coalesce typing without noticeably delaying autosave.
  const AUTOSAVE_DEBOUNCE_MS = 1200;

  let selectedId = $state(null);
  let selectedPage = $state(null);
  let draftTitle = $state('');
  let draftContent = $state('');
  let dirty = $state(false);
  let loadingPage = $state(false);
  let error = $state('');
  // Monotonic token for loadPage. A user who clicks page A then quickly
  // page B can get the slow A response back after B's fast response —
  // without this guard, the late A response would clobber selectedPage and
  // the editor would silently swap to A's content. The token also lets the
  // route-clearing branch invalidate any in-flight load.
  let loadPageRequestSeq = 0;
  let permsDialogOpen = $state(false);
  let moveDialogOpen = $state(false);
  let historyDrawerOpen = $state(false);
  let titleInputEl = $state(null);
  let pageEffectiveLevel = $state('');
  let pagePermissionsLoaded = $state(false);
  let canEditPage = $derived(
    pagePermissionsLoaded &&
      (pageEffectiveLevel === 'edit' || pageEffectiveLevel === 'admin')
  );
  let appearanceSaving = $state(false);
  let pickerIcon = $state('Plus');
  let pickerColor = $state('#3b82f6');
  let PageTitleIcon = $derived(selectedPage?.metadata?.icon ? workspaceIconMap[selectedPage.metadata.icon] : null);
  let pageTitleIconColor = $derived(selectedPage?.metadata?.color || 'var(--ds-text-subtle)');

  // 'idle' = nothing to save; 'pending' = waiting for the debounce
  // timer; 'saving' = request in flight; 'saved' = last write
  // succeeded; 'error' = last write failed.
  let saveStatus = $state('idle');
  let saveTimer = null;
  // True while the serialized queue is actively writing a snapshot.
  let saveInFlight = false;
  // The latest server-confirmed hash for each page saved during this view's
  // lifetime. A second snapshot may be queued while the first is in flight;
  // resolving the hash at write time keeps that serialized follow-up from
  // conflicting with our own immediately preceding save.
  const contentHashByPageId = new Map();

  const pageAutosaveQueue = createPageAutosaveQueue(
    persistSaveSnapshot,
    (left, right) =>
      left.workspaceId === right.workspaceId &&
      left.title === right.title &&
      left.content === right.content
  );

  const pageLinksState = createPageLinksState();

  // 'edit' (default) shows the formatting toolbar + lets the user type.
  // 'read' renders the body read-only, hides the toolbar (via
  // MilkdownEditor's own derivation on `readonly`), and surfaces the
  // table of contents on the right. Resets to 'edit' when the route
  // changes — writing-first default matches Confluence/Notion.
  let mode = $state('edit');
  let canvasExpanded = $state(false);

  let headings = $derived(parseMarkdownHeadings(draftContent));

  onMount(async () => {
    if (pageId) {
      selectedId = pageId;
      await loadPage(pageId);
    }
  });

  // Live-reload after AI chat agent runs so page edits made through update_page
  // become visible without requiring a manual refresh. If the user currently
  // has local unsaved edits, skip the reload rather than clobber their draft.
  onMount(() => agentRuns.subscribe(() => {
    pagesTreeRefresh.bump();
    if (!selectedPage?.id || dirty || saveInFlight) return;
    loadPage(selectedPage.id);
  }));

  onDestroy(() => {
    // Capture the pending draft before this view is torn down so in-app
    // navigation cannot discard the debounce window.
    if (dirty) {
      void flushSave();
    } else if (saveTimer) {
      clearTimeout(saveTimer);
    }
  });

  // Sync to route changes — navigating to a different page id loads
  // it; navigating back to the bare /pages clears the selection.
  // Flush any pending edits to the old page first so the user doesn't
  // lose mid-debounce work when jumping between pages.
  $effect(() => {
    if (pageId === selectedId) return;
    if (dirty) flushSave();
    selectedId = pageId;
    mode = 'edit';
    if (pageId) {
      loadPage(pageId);
    } else {
      // Bump the token so any in-flight loadPage can't write back into
      // the cleared state when its response eventually lands.
      loadPageRequestSeq++;
      selectedPage = null;
      draftTitle = '';
      draftContent = '';
      dirty = false;
      saveStatus = 'idle';
      pageLinksState.clear();
      pageEffectiveLevel = '';
      pagePermissionsLoaded = false;
      pagePublication = null;
    }
  });

  // Honor a focus-title request from the sidebar (after + creates a
  // new page or "Rename" is picked from the kebab). Watch the tick so
  // repeated requests for the same page still re-focus.
  $effect(() => {
    pagesFocusTitle.tick;
    if (!pagesFocusTitle.pageId) return;
    if (!selectedPage || pagesFocusTitle.pageId !== selectedPage.id) return;
    if (!canEditPage) return;
    if (!titleInputEl) return;
    titleInputEl.focus();
    titleInputEl.select();
    pagesFocusTitle.clear();
  });

  async function loadPage(id) {
    const requestSeq = ++loadPageRequestSeq;
    loadingPage = true;
    error = '';
    try {
      const page = await pages.getPage(workspaceId, id);
      if (requestSeq !== loadPageRequestSeq) return;
      selectedPage = page;
      contentHashByPageId.set(page.id, page.content_hash);
      draftTitle = page.title;
      draftContent = page.content;
      pickerIcon = page.metadata?.icon || 'Plus';
      pickerColor = page.metadata?.color || '#3b82f6';
      dirty = false;
      saveStatus = 'idle';
      pageEffectiveLevel = '';
      pagePermissionsLoaded = false;
      // Run in parallel: linked work items / permissions are independent
      // of the page payload, and the link-types list is cached for the session.
      void pageLinksState.loadFor(id);
      void ensurePageEffectiveLevel(id);
      void pageLinksState.ensureLinkTypesLoaded();
      void loadPagePublication(id);
    } catch (err) {
      if (requestSeq !== loadPageRequestSeq) return;
      error = err?.message || t('pages.errorLoadPage');
      selectedPage = null;
      pageEffectiveLevel = '';
      pagePermissionsLoaded = false;
      pagePublication = null;
    } finally {
      if (requestSeq === loadPageRequestSeq) loadingPage = false;
    }
  }

  // Portal knowledge-base publication state. Non-fatal: a failed lookup
  // just means no public-viewable banner.
  let pagePublication = $state(null);
  let pagePublicationRequestSeq = 0;

  async function loadPagePublication(id) {
    const requestSeq = ++pagePublicationRequestSeq;
    try {
      const publication = await pages.getPublication(workspaceId, id);
      if (requestSeq !== pagePublicationRequestSeq || selectedPage?.id !== id) return;
      pagePublication = publication;
    } catch (err) {
      if (requestSeq !== pagePublicationRequestSeq) return;
      pagePublication = null;
    }
  }

  async function ensurePageEffectiveLevel(id) {
    try {
      const perms = await pages.getPermissions(workspaceId, id);
      if (selectedPage?.id === id) {
        pageEffectiveLevel = perms?.effective_level || '';
        if (pageEffectiveLevel !== 'edit' && pageEffectiveLevel !== 'admin') {
          mode = 'read';
        }
      }
    } catch (err) {
      console.error('failed to load page permissions', err);
      if (selectedPage?.id === id) pageEffectiveLevel = '';
    } finally {
      if (selectedPage?.id === id) pagePermissionsLoaded = true;
    }
  }

  // Label chip helpers — attach/detach mutate the local selectedPage.labels
  // optimistically so the chip row reflects state instantly; on error we
  // roll back and surface the failure in the error banner.
  let selectedLabelIds = $derived(
    new Set((selectedPage?.labels || []).map((l) => l.id))
  );

  async function onLabelToggle(label) {
    if (!selectedPage) return;
    const isAttached = selectedLabelIds.has(label.id);
    const previous = selectedPage.labels || [];
    try {
      if (isAttached) {
        selectedPage.labels = previous.filter((l) => l.id !== label.id);
        await pageLabels.removeFromPage(workspaceId, selectedPage.id, label.id);
      } else {
        selectedPage.labels = [...previous, label].sort((a, b) =>
          a.name.localeCompare(b.name)
        );
        await pageLabels.addToPage(workspaceId, selectedPage.id, label.id);
      }
      pagesTreeRefresh.bump();
    } catch (err) {
      selectedPage.labels = previous;
      error = err?.message || t(isAttached ? 'pages.labelsErrorDetach' : 'pages.labelsErrorAttach');
    }
  }

  function scheduleAutoSave() {
    if (!selectedPage) return;
    saveStatus = 'pending';
    if (saveTimer) clearTimeout(saveTimer);
    saveTimer = setTimeout(() => {
      saveTimer = null;
      flushSave();
    }, AUTOSAVE_DEBOUNCE_MS);
  }

  /**
   * Push the current draft to the server immediately. Captures the
   * page id + draft contents up front so a concurrent navigation
   * (which would reassign selectedPage to a different row) cannot let
   * the response overwrite the wrong page's local state, and so a
   * slow response cannot clobber keystrokes typed during the in-flight
   * window — we only fold the response back into draftTitle /
   * draftContent when the draft is still identical to what we sent.
   */
  function flushSave() {
    if (!selectedPage || !dirty) return Promise.resolve();
    if (saveTimer) {
      clearTimeout(saveTimer);
      saveTimer = null;
    }

    const snapshot = {
      workspaceId,
      pageId: selectedPage.id,
      previousTitle: selectedPage.title,
      title: draftTitle,
      content: draftContent,
      expectedContentHash: selectedPage.content_hash,
    };
    return pageAutosaveQueue.enqueue(snapshot.pageId, snapshot);
  }

  async function persistSaveSnapshot(snapshot) {
    const { workspaceId: targetWorkspaceId, pageId: targetId } = snapshot;
    if (selectedPage?.id === targetId) {
      saveStatus = 'saving';
      error = '';
    }
    saveInFlight = true;
    try {
      const updated = await pages.updatePage(targetWorkspaceId, targetId, {
        title: snapshot.title,
        content: snapshot.content,
        expectedContentHash:
          contentHashByPageId.get(targetId) ?? snapshot.expectedContentHash,
      });
      contentHashByPageId.set(targetId, updated.content_hash);
      // Only fold the response back into local state if we're still on
      // the same page; a fast-switching user has already moved on.
      if (selectedPage?.id === targetId) {
        selectedPage = mergePageUpdate(selectedPage, updated);
        // If the user kept typing while the request was in flight,
        // the local draft is newer than what the server just echoed
        // back. Preserve the newer draft and let the next debounce
        // ship it; only clear `dirty` when the snapshot we sent is
        // still the current draft.
        const stillClean =
          draftTitle === snapshot.title && draftContent === snapshot.content;
        if (stillClean) {
          draftTitle = updated.title;
          draftContent = updated.content;
          dirty = false;
          saveStatus = 'saved';
        } else {
          saveStatus = 'pending';
          scheduleAutoSave();
        }
      }
      // The sidebar shows only titles, so a content-only save needs no
      // sidebar update at all. Signal a targeted rename only when the title
      // actually changed — that patches one row in place instead of
      // refetching (and flashing) the whole tree.
      if (updated.title !== snapshot.previousTitle) {
        pagesTreeRefresh.rename(updated.id, updated.title);
      }
    } catch (err) {
      if (selectedPage?.id === targetId) {
        saveStatus = 'error';
        error = err?.message || t('pages.errorSave');
      } else {
        console.error(`failed to save page ${targetId}`, err);
      }
    } finally {
      saveInFlight = false;
    }
  }

  async function archivePage() {
    if (!selectedPage) return;
    const ok = await confirm({
      title: t('pages.archiveTitle', { title: selectedPage.title }),
      message: t('pages.archiveMessage'),
      confirmText: t('pages.archiveConfirm'),
      cancelText: t('common.cancel'),
      variant: 'danger',
    });
    if (!ok) return;
    try {
      await pages.archivePage(workspaceId, selectedPage.id);
      // Drop any pending autosave for a page that no longer exists.
      if (saveTimer) {
        clearTimeout(saveTimer);
        saveTimer = null;
      }
      selectedPage = null;
      dirty = false;
      saveStatus = 'idle';
      pagesTreeRefresh.bump();
      navigate(`/workspaces/${workspaceId}/pages`);
    } catch (err) {
      error = err?.message || t('pages.errorArchive');
    }
  }

  function onContentInput(value) {
    if (!selectedPage) return;
    draftContent = value;
    if (value !== selectedPage.content || draftTitle !== selectedPage.title) {
      dirty = true;
      scheduleAutoSave();
    }
  }

  function onDiagramPersisted({ contentHash, pageContent }) {
    if (!selectedPage) return;
    if (saveTimer) {
      clearTimeout(saveTimer);
      saveTimer = null;
    }
    draftContent = pageContent;
    selectedPage = {
      ...selectedPage,
      content: pageContent,
      content_hash: contentHash,
    };
    contentHashByPageId.set(selectedPage.id, contentHash);
    dirty = false;
    saveStatus = 'saved';
  }

  function onTitleInput(event) {
    if (!selectedPage) return;
    draftTitle = event.target.value;
    if (draftTitle !== selectedPage.title) {
      dirty = true;
      scheduleAutoSave();
    }
  }

  async function updatePageAppearance({ icon = pickerIcon, color = pickerColor, clear = false } = {}) {
    if (!selectedPage || appearanceSaving) return;
    const targetWorkspaceId = workspaceId;
    const targetPageId = selectedPage.id;
    appearanceSaving = true;
    let previous = null;
    try {
      if (dirty) await flushSave();
      if (selectedPage?.id !== targetPageId) return;

      previous = selectedPage.metadata || {};
      const metadata = { ...previous };
      if (clear) {
        delete metadata.icon;
        delete metadata.color;
      } else {
        metadata.icon = icon;
        metadata.color = color;
      }
      selectedPage.metadata = metadata;
      selectedPage = selectedPage;

      const updated = await pages.updatePage(targetWorkspaceId, targetPageId, {
        title: draftTitle,
        content: draftContent,
        metadata,
        expectedContentHash:
          contentHashByPageId.get(targetPageId) ?? selectedPage.content_hash,
      });
      contentHashByPageId.set(targetPageId, updated.content_hash);
      if (selectedPage?.id === updated.id) {
        selectedPage = mergePageUpdate(selectedPage, updated, draftContent);
        pickerIcon = updated.metadata?.icon || 'Plus';
        pickerColor = updated.metadata?.color || '#3b82f6';
      }
      pagesTreeRefresh.bump();
    } catch (err) {
      if (selectedPage?.id === targetPageId) {
        if (previous) {
          selectedPage.metadata = previous;
          selectedPage = selectedPage;
        }
        error = err?.message || t('pages.errorSave');
      } else {
        console.error(`failed to update appearance for page ${targetPageId}`, err);
      }
    } finally {
      appearanceSaving = false;
    }
  }

  function onTitleKeydown(event) {
    if (event.key === 'Enter') {
      // Move focus into the body so the user can start typing right
      // away after committing the title.
      event.preventDefault();
      const editor = document.querySelector('.editor-frame .ProseMirror');
      if (editor instanceof HTMLElement) editor.focus();
    }
  }

  // Toolbar actions mirror the server contract: move needs edit on the
  // page, the ACL dialog needs admin. History and print are view-level.
  let canAdminPage = $derived(pagePermissionsLoaded && pageEffectiveLevel === 'admin');
  let toolbarMenuItems = $derived([
    ...(canEditPage
      ? [
          {
            id: 'move',
            type: 'regular',
            title: t('pages.menuMove'),
            testid: 'page-menu-move',
            onClick: () => (moveDialogOpen = true),
          },
        ]
      : []),
    ...(canAdminPage
      ? [
          {
            id: 'permissions',
            type: 'regular',
            title: t('pages.menuPermissions'),
            testid: 'page-menu-permissions',
            onClick: () => (permsDialogOpen = true),
          },
        ]
      : []),
    {
      id: 'history',
      type: 'regular',
      title: t('pages.menuHistory'),
      testid: 'page-menu-history',
      onClick: () => (historyDrawerOpen = true),
    },
    {
      id: 'print',
      type: 'regular',
      title: t('pages.menuPrint'),
      testid: 'page-menu-print',
      // Open the chrome-free print view in a new tab so the editor tab
      // (and any in-flight autosave) is left untouched; the print tab
      // auto-opens the browser print dialog once content has rendered.
      onClick: () =>
        window.open(
          `/workspaces/${workspaceId}/pages/${selectedPage.id}/print`,
          '_blank',
          'noopener'
        ),
    },
    // Archive needs per-page admin plus the workspace page.delete key,
    // matching the application service's Archive contract.
    ...(canAdminPage && workspacePermissions.canDeletePages(workspaceId)
      ? [
          { id: 'divider', type: 'divider' },
          {
            id: 'archive',
            type: 'regular',
            title: t('pages.menuArchive'),
            color: 'var(--ds-text-danger)',
            testid: 'page-menu-archive',
            onClick: archivePage,
          },
        ]
      : []),
  ]);

  let statusLabel = $derived.by(() => {
    switch (saveStatus) {
      case 'saving':
        return t('pages.statusSaving');
      case 'pending':
        return t('pages.statusUnsaved');
      case 'saved':
        return t('pages.statusSaved');
      case 'error':
        return t('pages.statusError');
      default:
        return '';
    }
  });

  // TOC click → find the matching heading in the rendered ProseMirror
  // DOM and scrollIntoView. Match by text slug so the lookup works
  // even though Milkdown doesn't stamp heading IDs onto the DOM.
  function scrollToHeading(heading) {
    const root = document.querySelector('.editor-frame .ProseMirror');
    if (!root) return;
    const nodes = root.querySelectorAll('h1, h2, h3, h4, h5, h6');
    for (const node of nodes) {
      if (slugify(node.textContent || '') === heading.slug) {
        node.scrollIntoView({ behavior: 'smooth', block: 'start' });
        try {
          history.replaceState(null, '', `#${heading.slug}`);
        } catch (_) {
          /* ignore SecurityError in sandboxed previews */
        }
        return;
      }
    }
  }

  $effect(() => {
    if (!selectedPage || loadingPage) return;
    const hash = window.location.hash?.slice(1);
    if (!hash) return;
    const target = headings.find((h) => h.slug === hash);
    if (!target) return;
    const handle = setTimeout(() => scrollToHeading(target), 100);
    return () => clearTimeout(handle);
  });
</script>

<main
  class="flex h-full min-h-0 flex-col overflow-y-auto @container/pages [--page-gutter:clamp(1rem,4cqi,3rem)]"
  data-testid="pages-view"
>
  {#if error}
    <div
      class="mx-auto mb-4 max-w-[880px] rounded-sm bg-ds-status-danger-bg px-4 py-3 text-[0.875rem] text-ds-text-danger"
      role="alert"
      data-testid="page-error"
    >
      {error}
    </div>
  {/if}

  {#if !selectedPage && !loadingPage}
    <div class="mx-auto flex max-w-[880px] flex-col items-start gap-2 px-12 py-8 text-ds-text-subtle">
      <Book size={48} color="var(--ds-text-subtle)" />
      <h1 class="mt-2 mb-1 text-xl text-ds-text">{t('pages.emptyPaneTitle')}</h1>
      <p class="m-0">{t('pages.emptyPaneDescription')}</p>
    </div>
  {:else if loadingPage}
    <p class="mx-auto max-w-[880px] p-4 px-12 text-[0.875rem] text-ds-text-subtle">
      {t('pages.pageLoading')}
    </p>
  {:else if selectedPage}
    {#if pagePublication?.publicly_viewable}
      <!-- Green status bar above the page when a portal knowledge base
           publishes it. Sits outside the centered page frame so the bar
           runs edge to edge across the whole pane. -->
      <div
        class="mb-4 flex items-center gap-2 border-b border-ds-status-success-border bg-ds-status-success-bg px-(--page-gutter) py-2 text-[0.8125rem] text-ds-text-success"
        data-testid="page-publication-banner"
      >
        {pagePublication.portals?.length
          ? t('pages.publiclyViewableBannerWithPortals', {
              portals: pagePublication.portals.join(', '),
            })
          : t('pages.publiclyViewableBanner')}
      </div>
    {/if}
    <!-- Keep the complete document inside the pane's scrollable canvas.
         Above the 1281px viewport the frame centers at 75% pane width
         unless the user widens the canvas. -->
    <div
      class="mx-auto flex w-full flex-col gap-4 [flex:1_0_auto] {canvasExpanded
        ? 'max-w-none'
        : 'min-[1281px]:max-w-[75%]'}"
      class:canvas-expanded={canvasExpanded}
      data-testid="page-canvas"
      data-width={canvasExpanded ? 'wide' : 'comfortable'}
    >
      <PageToolbar
        page={selectedPage}
        {workspaceId}
        pageLinks={pageLinksState.pageLinks}
        loadingPageLinks={pageLinksState.loading}
        pageLinkTypeId={pageLinksState.pageLinkTypeId}
        {statusLabel}
        {saveStatus}
        {canEditPage}
        menuItems={toolbarMenuItems}
        bind:mode
        bind:canvasExpanded
        title={draftTitle}
        bind:titleInputEl
        {PageTitleIcon}
        {pageTitleIconColor}
        onTitleInput={onTitleInput}
        onTitleKeydown={onTitleKeydown}
        onLinkCreated={pageLinksState.handleCreated}
        onLinkRemoved={pageLinksState.handleRemoved}
      />
      <PageLabelsRow
        page={selectedPage}
        {workspaceId}
        editable={mode === 'edit' && canEditPage}
        {appearanceSaving}
        bind:pickerIcon
        bind:pickerColor
        {selectedLabelIds}
        onToggleLabel={onLabelToggle}
        onChangeAppearance={updatePageAppearance}
        onClearAppearance={() => updatePageAppearance({ clear: true })}
      />
      <div class="flex min-h-0 flex-1 flex-row gap-8">
        <PageEditor
          entityId={selectedPage.id}
          {workspaceId}
          bind:content={draftContent}
          readonly={mode === 'read' || !canEditPage}
          expectedContentHash={selectedPage.content_hash}
          onBeforeDiagramOpen={flushSave}
          onDiagramPersisted={onDiagramPersisted}
          onContentChange={onContentInput}
        />
        {#if mode === 'read' && headings.length > 0}
          <PageToc {headings} onselect={scrollToHeading} />
        {/if}
      </div>
    </div>
  {/if}
</main>

{#if selectedPage}
  <PagePermissionsDialog
    bind:isOpen={permsDialogOpen}
    {workspaceId}
    pageId={selectedPage.id}
    onUpdated={() => pagesTreeRefresh.bump()}
  />
  <PageMoveDialog
    bind:isOpen={moveDialogOpen}
    {workspaceId}
    page={selectedPage}
    onMoved={async (moved) => {
      pagesTreeRefresh.bump();
      if (moved?.workspace_id !== workspaceId) {
        selectedPage = null;
        navigate(`/workspaces/${workspaceId}/pages`);
        return;
      }
      if (selectedPage) await loadPage(selectedPage.id);
    }}
  />
  <PagesHistoryDrawer
    bind:open={historyDrawerOpen}
    {workspaceId}
    pageId={selectedPage.id}
    canRestore={pageEffectiveLevel === 'edit' || pageEffectiveLevel === 'admin'}
    onRestored={async () => {
      if (selectedPage) await loadPage(selectedPage.id);
    }}
  />
{/if}
