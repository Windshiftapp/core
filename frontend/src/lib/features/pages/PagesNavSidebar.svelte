<script>
  import { onDestroy, untrack } from 'svelte';
  import { draggable, dropTargetForElements } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import { attachInstruction, extractInstruction } from '@atlaskit/pragmatic-drag-and-drop-hitbox/tree-item';
  import { isSelfOrDescendant, orderPagesDepthFirst } from './pageHierarchy.js';
  import { api } from '../../api.js';
  import { navigate, currentRoute } from '../../router.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { confirm } from '../../composables/useConfirm.js';
  import { errorToast } from '../../stores/toasts.svelte.js';
  import {
    IconPlus as Plus,
    IconDots as Dots,
    IconBook as Book,
    IconX as X,
    IconSearch as Search,
    IconChevronRight as ChevronRight,
    IconChevronDown as ChevronDown,
    IconChevronsDown as ChevronsDown,
    IconChevronsUp as ChevronsUp,
    IconArchive as Archive
  } from '@tabler/icons-svelte-runes';
  import DropdownMenu from '../../layout/DropdownMenu.svelte';
  import ScrollableSidebar from '../../layout/ScrollableSidebar.svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import Tooltip from '../../components/Tooltip.svelte';
  import Input from '../../components/Input.svelte';
  import PageMoveDialog from './PageMoveDialog.svelte';
  import PagePermissionsDialog from './PagePermissionsDialog.svelte';
  import PageLabelPicker from './PageLabelPicker.svelte';
  import { workspacePermissions } from '../../stores/workspacePermissions.svelte.js';
  import { workspaceIconMap } from '../../utils/icons.js';
  import { pagesTreeRefresh } from './pagesTreeRefresh.svelte.js';
  import { pagesFocusTitle } from './pagesFocusTitle.svelte.js';
  import { pagesFilter } from './pagesFilter.svelte.js';

  let { workspaceId, embedded = false } = $props();

  let pages = $state([]);
  let loading = $state(true);
  let creating = $state(false);
  let moveDialogOpen = $state(false);
  let moveDialogPage = $state(null);
  let permsDialogOpen = $state(false);
  let permsDialogPage = $state(null);
  let dndState = $state(new Map());
  let dwellExpandTimer;
  let dwellExpandPageId = null;

  // Cache of workspace labels keyed by id. Populated lazily on first
  // page-tree load by walking the preloaded `labels` arrays on each page.
  // Lets the filter row render colored chips for active filters even when
  // the picker popover is closed.
  let labelLookup = $state(/** @type {Map<number, any>} */ (new Map()));

  // Set of page ids whose subtree is currently shown. Persisted to
  // localStorage per workspace; first visit defaults to "every root
  // expanded, every nested subtree collapsed". The set stores expanded
  // ids (not collapsed) because the default seed is small (one entry per
  // root) and only grows as the user opens subtrees.
  let expandedIds = $state(/** @type {Set<number>} */ (new Set()));

  function expandedStorageKey(wsId) {
    return `pagesTree_expanded_${wsId}`;
  }

  function loadExpanded(wsId, rootIds) {
    if (typeof localStorage === 'undefined') return new Set(rootIds);
    try {
      const raw = localStorage.getItem(expandedStorageKey(wsId));
      if (raw == null) return new Set(rootIds);
      const parsed = JSON.parse(raw);
      return new Set(Array.isArray(parsed) ? parsed.map(Number) : []);
    } catch {
      return new Set(rootIds);
    }
  }

  function persistExpanded() {
    if (typeof localStorage === 'undefined') return;
    try {
      localStorage.setItem(
        expandedStorageKey(workspaceId),
        JSON.stringify(Array.from(expandedIds)),
      );
    } catch {
      // Storage quota / private-window fallback: ignore — collapse state
      // gracefully degrades to in-memory only for this session.
    }
  }

  function toggleNode(pageId) {
    const next = new Set(expandedIds);
    if (next.has(pageId)) next.delete(pageId);
    else next.add(pageId);
    expandedIds = next;
    persistExpanded();
  }

  function expandAll() {
    expandedIds = new Set(pages.map((p) => p.id));
    persistExpanded();
  }

  function collapseAll() {
    expandedIds = new Set();
    persistExpanded();
  }

  // Reset the per-workspace filter when the workspace id changes — filters
  // are session-only and a workspace switch shouldn't carry a stale label
  // set into a workspace where those ids don't exist.
  $effect(() => {
    pagesFilter.reset(workspaceId);
  });

  let filterLabelIds = $derived(pagesFilter.labelIds);
  let activeFilterLabels = $derived(
    Array.from(filterLabelIds)
      .map((id) => labelLookup.get(id))
      .filter(Boolean)
  );

  // Visibility filter: when no filters are active, every page is visible.
  // Otherwise, a page is visible iff its own labels intersect the filter set
  // OR it is an ancestor of a matching page (kept for tree context). Ancestor
  // detection uses the materialized `path` field that the backend stamps on
  // every page row.
  let labelVisibleIds = $derived.by(() => {
    if (filterLabelIds.size === 0) return null; // null means "show everything"
    const visible = new Set();
    for (const page of pages) {
      const hit = (page.labels || []).some((l) => filterLabelIds.has(l.id));
      if (!hit) continue;
      visible.add(page.id);
      // Walk the path "1/4/9/" → ancestor ids 1, 4, 9.
      const segments = (page.path || '').split('/').filter(Boolean);
      for (const seg of segments) {
        const ancestorId = Number(seg);
        if (Number.isFinite(ancestorId)) visible.add(ancestorId);
      }
    }
    return visible;
  });

  // Combined visibility: a page is visible iff every active filter says so.
  // Label + title search compose with AND semantics — both filters must
  // include the page (or its descendant chain) for it to survive.
  let visibleIds = $derived.by(() => {
    if (labelVisibleIds === null && titleMatchIds === null) return null;
    if (labelVisibleIds === null) return titleMatchIds;
    if (titleMatchIds === null) return labelVisibleIds;
    const intersection = new Set();
    for (const id of labelVisibleIds) {
      if (titleMatchIds.has(id)) intersection.add(id);
    }
    return intersection;
  });

  function isLabelHit(page) {
    return (page.labels || []).some((l) => filterLabelIds.has(l.id));
  }

  function isPageHit(page) {
    // "Hit" = matches every active filter, not just an ancestor of a match.
    const titleOK = titleMatchIds === null || isSearchHit(page);
    const labelOK = filterLabelIds.size === 0 || isLabelHit(page);
    return titleOK && labelOK;
  }

  function isAncestorOnly(page) {
    // Visible only because of the ancestor-context rule; rendered dimmer
    // so the user can tell which rows are the actual matches.
    if (visibleIds === null) return false;
    return visibleIds.has(page.id) && !isPageHit(page);
  }

  // Number of direct children per page id. Drives the per-row chevron
  // (only render it on rows that actually have children).
  let childCountById = $derived.by(() => {
    const m = new Map();
    for (const p of pages) {
      const parent = p.parent_id;
      if (parent == null) continue;
      m.set(parent, (m.get(parent) || 0) + 1);
    }
    return m;
  });

  // id -> page lookup, rebuilt once per render. The DnD wiring resolves a
  // page by id for every rendered row (and again inside canDrop /
  // handleDrop); a Map makes each lookup O(1) instead of pages.find O(n)
  // per call — pages.find per row is what made setupDnd O(n^2) per tree
  // mutation (~1M ops at 1000 nodes).
  let pageById = $derived.by(() => {
    const m = new Map();
    for (const p of pages) m.set(p.id, p);
    return m;
  });

  // Filters virtually expand matching ancestors without changing saved collapse
  // state, which returns when the filter clears.
  let effectiveExpanded = $derived.by(() => {
    if (visibleIds === null) return expandedIds;
    const out = new Set(expandedIds);
    for (const id of visibleIds) out.add(id);
    return out;
  });

  function isCollapseHidden(page) {
    if (!page.path) return false; // root pages are never hidden by collapse
    const segments = page.path.split('/').filter(Boolean);
    for (const seg of segments) {
      if (!effectiveExpanded.has(Number(seg))) return true;
    }
    return false;
  }

  // Render only visible rows, avoiding hidden subtree DOM nodes and ensuring
  // drag/drop sees only draggable rows.
  let visibleRows = $derived.by(() => {
    const out = [];
    for (const page of pages) {
      if (visibleIds !== null && !visibleIds.has(page.id)) continue;
      if (isCollapseHidden(page)) continue;
      out.push(page);
    }
    return out;
  });

  function onFilterToggle(label) {
    labelLookup.set(label.id, label);
    labelLookup = labelLookup; // trigger reactivity
    pagesFilter.toggle(workspaceId, label.id);
  }

  function removeFilter(labelId) {
    pagesFilter.remove(workspaceId, labelId);
  }

  function clearFilters() {
    pagesFilter.clear(workspaceId);
  }

  // --- title search ---
  // Local search filter on page titles. Client-side substring match
  // (case-insensitive) over the already-loaded tree — fast, no extra
  // round-trip. The input lives in a toggleable row revealed by the
  // search icon in the header so the default sidebar stays compact.
  let searchOpen = $state(false);
  let searchQuery = $state('');
  let searchInputEl = $state(null);

  function toggleSearch() {
    searchOpen = !searchOpen;
    if (searchOpen) {
      // Focus on next tick so the input mounts first.
      setTimeout(() => searchInputEl?.focus(), 0);
    } else {
      searchQuery = '';
    }
  }

  function clearSearch() {
    searchQuery = '';
    searchInputEl?.focus();
  }

  function onSearchKeydown(e) {
    if (e.key === 'Escape') {
      e.preventDefault();
      searchOpen = false;
      searchQuery = '';
    }
  }

  // Title-match: case-insensitive substring on each page's title. Pages
  // that don't match are hidden; ancestors of matches stay visible for
  // tree context (same rule the label filter uses).
  let titleMatchIds = $derived.by(() => {
    const q = searchQuery.trim().toLowerCase();
    if (!q) return null;
    const visible = new Set();
    for (const page of pages) {
      if (!page.title?.toLowerCase().includes(q)) continue;
      visible.add(page.id);
      const segments = (page.path || '').split('/').filter(Boolean);
      for (const seg of segments) {
        const ancestorId = Number(seg);
        if (Number.isFinite(ancestorId)) visible.add(ancestorId);
      }
    }
    return visible;
  });

  function isSearchHit(page) {
    if (titleMatchIds === null) return false;
    const q = searchQuery.trim().toLowerCase();
    return Boolean(q) && page.title?.toLowerCase().includes(q);
  }

  // The currently active page id comes from the route param, not local state —
  // navigating back/forward (or PagesView selecting via a different path) must
  // keep the sidebar's highlight in sync.
  let activePageId = $derived(
    $currentRoute?.params?.pageId ? Number($currentRoute.params.pageId) : null
  );

  // Reveal the active page in the tree: navigating to a page (sidebar
  // click, deep link, in-page link, search result) expands every ancestor
  // so the highlighted row is actually visible. Runs once per navigation —
  // the lastRevealedId guard keeps it from fighting the user if they
  // manually collapse an ancestor while staying on the page.
  let lastRevealedId = null;
  $effect(() => {
    const id = activePageId;
    const lookup = pageById;
    if (id == null || lastRevealedId === id) return;
    const page = lookup.get(id);
    if (!page) return; // tree not loaded yet, or page not in this workspace
    lastRevealedId = id;
    untrack(() => {
      const ancestorIds = (page.path || '')
        .split('/')
        .filter(Boolean)
        .map(Number)
        .filter((a) => Number.isFinite(a) && !expandedIds.has(a));
      if (ancestorIds.length === 0) return;
      const next = new Set(expandedIds);
      for (const a of ancestorIds) next.add(a);
      expandedIds = next;
      persistExpanded();
    });
  });

  onDestroy(() => {
    clearDwellExpand();
  });

  // Initial load + external refresh in one effect. Reading
  // `pagesTreeRefresh.tick` makes the effect re-run when it bumps; the
  // effect's first invocation handles the mount-time load. Critically we
  // do NOT read `loading` here — loadTree() mutates it, which would
  // self-retrigger the effect into an infinite loop.
  $effect(() => {
    pagesTreeRefresh.tick;
    loadTree();
  });

  // A title-only save patches the one renamed node in place rather than
  // refetching the whole tree — no network call, no expand/collapse reseed,
  // no DnD re-wire, no flash. A no-op if the page isn't in the loaded tree.
  $effect(() => {
    const r = pagesTreeRefresh.renamed;
    if (!r) return;
    const node = pages.find((p) => p.id === r.id);
    if (node && node.title !== r.title) {
      node.title = r.title;
      pages = pages;
    }
  });

  async function loadTree() {
    loading = true;
    try {
      pages = orderPagesDepthFirst((await api.pages.getAll(workspaceId)) || []);
      // Cache every label we encounter so the filter row can render names
      // + colors for active filters without an extra round-trip.
      for (const page of pages) {
        for (const label of page.labels || []) {
          labelLookup.set(label.id, label);
        }
      }
      labelLookup = labelLookup;

      // Seed expanded state from localStorage; on a workspace's first
      // visit the seed is just the root ids so the tree opens with the
      // top level visible and every nested subtree collapsed.
      const rootIds = pages
        .filter((page) => page.parent_id == null)
        .map((page) => page.id)
        .filter((id) => Number.isFinite(id));
      expandedIds = loadExpanded(workspaceId, rootIds);
    } catch (err) {
      errorToast(err?.message || t('pages.errorLoadTree'));
    } finally {
      loading = false;
    }
  }

  function selectPage(id) {
    // Selecting a page opens its own subtree so its direct children are
    // immediately available. Descendant subtrees keep their existing state.
    if ((childCountById.get(id) || 0) > 0 && !expandedIds.has(id)) {
      expandedIds = new Set(expandedIds).add(id);
      persistExpanded();
    }
    navigate(`/workspaces/${workspaceId}/pages/${id}`);
  }

  async function createPage(parentId) {
    if (creating) return;
    creating = true;
    try {
      const page = await api.pages.createPage(workspaceId, {
        title: t('pages.untitled'),
        content: '',
        parentId: parentId ?? null,
      });
      // Auto-expand the parent so the new child row is visible after
      // loadTree() reseeds expandedIds from localStorage. Without this,
      // adding a child under a collapsed (or never-expanded) parent
      // would create a row the user can't see.
      if (parentId != null && !expandedIds.has(parentId)) {
        expandedIds = new Set(expandedIds).add(parentId);
        persistExpanded();
      }
      pagesFocusTitle.request(page.id);
      navigate(`/workspaces/${workspaceId}/pages/${page.id}`);
      await loadTree();
    } catch (err) {
      errorToast(err?.message || t('pages.errorCreate'));
    } finally {
      creating = false;
    }
  }

  async function archivePage(page) {
    const ok = await confirm({
      title: t('pages.archiveTitle', { title: page.title }),
      message: t('pages.archiveMessage'),
      confirmText: t('pages.archiveConfirm'),
      cancelText: t('common.cancel'),
      variant: 'danger',
    });
    if (!ok) return;
    try {
      await api.pages.archivePage(workspaceId, page.id);
      if (activePageId === page.id) {
        navigate(`/workspaces/${workspaceId}/pages`);
      }
      await loadTree();
    } catch (err) {
      errorToast(err?.message || t('pages.errorArchive'));
    }
  }

  function requestRename(page) {
    if (activePageId !== page.id) {
      navigate(`/workspaces/${workspaceId}/pages/${page.id}`);
    }
    pagesFocusTitle.request(page.id);
  }

  function kebabItems(page) {
    return [
      { id: 'add-child', type: 'regular', icon: Plus, title: t('pages.menuAddChild'), onClick: () => createPage(page.id) },
      { id: 'rename', type: 'regular', title: t('pages.menuRename'), onClick: () => requestRename(page) },
      { id: 'move', type: 'regular', title: t('pages.menuMove'), onClick: () => { moveDialogPage = page; moveDialogOpen = true; } },
      { id: 'permissions', type: 'regular', title: t('pages.menuPermissions'), onClick: () => { permsDialogPage = page; permsDialogOpen = true; } },
      { id: 'divider', type: 'divider' },
      { id: 'archive', type: 'regular', title: t('pages.menuArchive'), color: 'var(--ds-text-danger)', onClick: () => archivePage(page) },
    ];
  }

  // --- DnD ---

  function clearDwellExpand(pageId = null) {
    if (pageId != null && dwellExpandPageId !== pageId) return;
    if (dwellExpandTimer) clearTimeout(dwellExpandTimer);
    dwellExpandTimer = undefined;
    dwellExpandPageId = null;
  }

  function instructionDropMode(instruction) {
    if (instruction?.type === 'reorder-above') return 'top';
    if (instruction?.type === 'reorder-below') return 'bottom';
    if (instruction?.type === 'make-child') return 'child';
    return null;
  }

  function scheduleDwellExpand(pageId, dropMode) {
    const canExpand =
      dropMode === 'child' &&
      (childCountById.get(pageId) || 0) > 0 &&
      !expandedIds.has(pageId);
    if (!canExpand) {
      clearDwellExpand(pageId);
      return;
    }
    if (dwellExpandPageId === pageId && dwellExpandTimer) return;

    clearDwellExpand();
    dwellExpandPageId = pageId;
    dwellExpandTimer = setTimeout(() => {
      const next = new Set(expandedIds);
      next.add(pageId);
      expandedIds = next;
      persistExpanded();
      dwellExpandTimer = undefined;
      dwellExpandPageId = null;
    }, 500);
  }

  function updateDropState(pageId, instruction) {
    const dropMode = instructionDropMode(instruction);
    dndState.set(pageId, { dropMode, over: dropMode !== null });
    dndState = new Map(dndState);
    scheduleDwellExpand(pageId, dropMode);
  }

  function wirePageDnd(element, pageId) {
    const dragCleanup = draggable({
      element,
      getInitialData: () => {
        const page = pageById.get(pageId);
        return { type: 'page', pageId, parentId: page?.parent_id ?? null };
      },
      onDragStart: () => {
        element.style.opacity = '0.5';
      },
      onDrop: () => {
        element.style.opacity = '';
        clearDwellExpand();
        dndState = new Map();
      },
    });

    const dropCleanup = dropTargetForElements({
      element,
      canDrop: ({ source }) => {
        if (source.data.type !== 'page') return false;
        // Forbid dropping a page onto itself or any of its own descendants.
        if (source.data.pageId === pageId) return false;
        const page = pageById.get(pageId);
        const dragged = pageById.get(source.data.pageId);
        if (!page) return false;
        // Source row not in the current tree slice: the self-check above
        // is all we can enforce here, and the backend answers 409 on a
        // cycle regardless.
        return !dragged || !isSelfOrDescendant(page, dragged);
      },
      getData: ({ input, element: target }) => {
        const page = pageById.get(pageId);
        return attachInstruction({}, {
          input,
          element: target,
          currentLevel: page?.depth ?? 0,
          indentPerLevel: 12,
          mode: 'standard',
        });
      },
      onDragEnter: ({ self }) => {
        updateDropState(pageId, extractInstruction(self.data));
      },
      onDrag: ({ self }) => {
        updateDropState(pageId, extractInstruction(self.data));
      },
      onDragLeave: () => {
        clearDwellExpand(pageId);
        dndState.set(pageId, { dropMode: null, over: false });
        dndState = new Map(dndState);
      },
      onDrop: ({ self, source }) => {
        const dropMode = instructionDropMode(extractInstruction(self.data));
        clearDwellExpand(pageId);
        dndState = new Map();
        handleDrop(source.data.pageId, pageId, dropMode);
      },
    });

    return {
      destroy() {
        clearDwellExpand(pageId);
        element.style.opacity = '';
        dragCleanup();
        dropCleanup();
      },
    };
  }

  async function handleDrop(draggedId, targetId, dropMode) {
    if (draggedId === targetId || !dropMode) return;
    const target = pageById.get(targetId);
    if (!target) return;
    let newParentId;
    let prevSiblingId = null;
    let nextSiblingId = null;

    if (dropMode === 'top' || dropMode === 'bottom') {
      // Sibling drop: parent of dropped page becomes parent of target.
      newParentId = target.parent_id ?? null;
      // Identify the target's siblings (children of newParentId, in
      // display order) so we can pick the prev/next that bracket the
      // drop. `pages` is depth-first so siblings are contiguous when
      // filtered by parent_id; index by position to find neighbors.
      const siblings = pages.filter((p) => (p.parent_id ?? null) === newParentId);
      const targetIdx = siblings.findIndex((p) => p.id === targetId);
      if (targetIdx === -1) return;
      if (dropMode === 'top') {
        nextSiblingId = target.id;
        prevSiblingId = targetIdx > 0 ? siblings[targetIdx - 1].id : null;
      } else {
        prevSiblingId = target.id;
        nextSiblingId = targetIdx < siblings.length - 1 ? siblings[targetIdx + 1].id : null;
      }
      // A sibling pointer that names the dragged page itself is
      // meaningless (it's about to move). Drop it so the server picks an
      // open-ended neighbor and the move still places correctly.
      if (prevSiblingId === draggedId) prevSiblingId = null;
      if (nextSiblingId === draggedId) nextSiblingId = null;
    } else {
      // Drop on the row body's middle band: make the dragged page a child
      // of the target. Position is "end of list" — server
      // computes a frac_index after the last existing child.
      newParentId = targetId;
    }

    try {
      await api.pages.movePage(workspaceId, draggedId, newParentId, {
        prevSiblingId,
        nextSiblingId,
      });
      await loadTree();
    } catch (err) {
      errorToast(err?.message || t('pages.errorMove'));
    }
  }
</script>

{#snippet sidebarHeader()}
  <header class="header">
    <div class="title-row">
      <h2>{t('pages.treeHeading')}</h2>
      <div class="title-actions">
        <Tooltip content={t('pages.searchAria')} placement="bottom" class="inline-flex">
          <button
            id="pages-search-button"
            class="header-button"
            class:header-button--active={searchOpen}
            type="button"
            onclick={toggleSearch}
            aria-label={t('pages.searchAria')}
            aria-pressed={searchOpen}
            data-testid="pages-search-toggle"
          >
            <Search size={16} />
          </button>
        </Tooltip>
        <Tooltip content={t('pages.expandAllAria')} placement="bottom" class="inline-flex">
          <button
            class="header-button"
            type="button"
            onclick={expandAll}
            aria-label={t('pages.expandAllAria')}
            data-testid="pages-expand-all"
          >
            <ChevronsDown size={16} />
          </button>
        </Tooltip>
        <Tooltip content={t('pages.collapseAllAria')} placement="bottom" class="inline-flex">
          <button
            class="header-button"
            type="button"
            onclick={collapseAll}
            aria-label={t('pages.collapseAllAria')}
            data-testid="pages-collapse-all"
          >
            <ChevronsUp size={16} />
          </button>
        </Tooltip>
        {#if workspacePermissions.canAdminWorkspace(workspaceId)}
          <Tooltip content={t('pages.archivedOpenAria')} placement="bottom" class="inline-flex">
            <button
              class="header-button"
              class:header-button--active={$currentRoute.view === 'workspace-pages-archived'}
              type="button"
              onclick={() => navigate(`/workspaces/${workspaceId}/pages/archived`)}
              aria-label={t('pages.archivedOpenAria')}
              data-testid="pages-archived-open"
            >
              <Archive size={16} />
            </button>
          </Tooltip>
        {/if}
        <Tooltip content={t('pages.addPageAria')} placement="bottom" class="inline-flex">
          <button
            id="pages-add-button"
            class="header-button"
            type="button"
            onclick={() => createPage(null)}
            disabled={creating}
            aria-label={t('pages.addPageAria')}
          >
            <Plus size={16} />
          </button>
        </Tooltip>
      </div>
    </div>
  </header>

  {#if searchOpen}
    <div class="search-row" data-testid="pages-search-row">
      <Search size={14} class="search-row__icon" aria-hidden="true" />
      <Input
        bind:inputRef={searchInputEl}
        bind:value={searchQuery}
        onkeydown={onSearchKeydown}
        type="text"
        class="search-input"
        placeholder={t('pages.searchPlaceholder')}
        dataTestid="pages-search-input"
        size="small"
      />
      {#if searchQuery}
        <button
          type="button"
          class="search-row__clear"
          onclick={clearSearch}
          aria-label={t('pages.searchClear')}
        >
          <X size={12} />
        </button>
      {/if}
    </div>
  {/if}

  <div class="filter-row" data-testid="pages-filter-row">
    {#each activeFilterLabels as label (label.id)}
      <span
        class="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs"
        style="background-color: {label.color || '#3B82F6'}1A; color: var(--ds-text); border: 1px solid {label.color || '#3B82F6'};"
        data-testid="pages-filter-chip"
        data-label-id={label.id}
      >
        <span
          class="inline-block w-2 h-2 rounded-full"
          style="background-color: {label.color || '#3B82F6'};"
          aria-hidden="true"
        ></span>
        {label.name}
        <button
          type="button"
          class="filter-chip__remove"
          onclick={() => removeFilter(label.id)}
          aria-label={t('pages.labelsRemoveAria', { name: label.name })}
        >
          <X size={12} />
        </button>
      </span>
    {/each}
    <PageLabelPicker
      {workspaceId}
      selectedIds={filterLabelIds}
      allowCreate={false}
      onToggle={onFilterToggle}
      triggerLabel={t('pages.labelsFilterTitle')}
      triggerTestid="pages-filter-trigger"
    />
    {#if activeFilterLabels.length > 0}
      <button
        type="button"
        class="filter-clear"
        onclick={clearFilters}
        data-testid="pages-filter-clear"
      >
        {t('pages.labelsFilterClear')}
      </button>
    {/if}
  </div>
{/snippet}

<ScrollableSidebar
  as="aside"
  class="pages-sidebar {embedded ? 'pages-sidebar--embedded' : ''}"
  data-testid="pages-nav-sidebar"
  aria-label={t('pages.treeHeading')}
  header={sidebarHeader}
  scrollTestid="pages-navigation-scroll"
>
  {#if loading}
    <p class="status">{t('pages.treeLoading')}</p>
  {:else if pages.length === 0}
    <div class="tree-empty">
      <EmptyState
        icon={Book}
        title={t('pages.treeEmptyTitle')}
        description={t('pages.treeEmptyDescription')}
      />
    </div>
  {:else}
    <ul class="tree" data-testid="page-tree">
      {#each visibleRows as page (page.id)}
        {@const dropMode = dndState.get(page.id)?.dropMode}
        {@const isOver = dndState.get(page.id)?.over}
        {@const hasChildren = (childCountById.get(page.id) || 0) > 0}
        {@const isExpanded = expandedIds.has(page.id)}
        {@const dimmed = isAncestorOnly(page)}
        {@const PageIcon = page.metadata?.icon ? workspaceIconMap[page.metadata.icon] : null}
        {@const pageColor = page.metadata?.color || 'var(--ds-text-subtle)'}
        <li
          class="tree-item"
          class:active={activePageId === page.id}
          class:drop-top={dropMode === 'top'}
          class:drop-bottom={dropMode === 'bottom'}
          class:drop-on={dropMode === 'child'}
          class:dimmed
          use:wirePageDnd={page.id}
          data-page-row={page.id}
          data-testid={`page-tree-item-${page.id}`}
          data-page-id={page.id}
          data-expanded={hasChildren ? String(isExpanded) : undefined}
          data-drop-mode={isOver ? dropMode : undefined}
          style="padding-left: {1 + page.depth * 0.75}rem"
        >
          {#if hasChildren}
            <Tooltip
              content={t('pages.toggleSubtreeAria', { title: page.title })}
              placement="bottom"
              class="inline-flex"
            >
              <button
                type="button"
                class="chevron"
                onclick={(e) => {
                  e.stopPropagation();
                  toggleNode(page.id);
                }}
                aria-expanded={isExpanded}
                aria-label={t('pages.toggleSubtreeAria', { title: page.title })}
                data-testid="page-tree-chevron"
              >
                {#if isExpanded}
                  <ChevronDown size={14} />
                {:else}
                  <ChevronRight size={14} />
                {/if}
              </button>
            </Tooltip>
          {:else}
            <span class="chevron chevron--placeholder" aria-hidden="true"></span>
          {/if}
          <button
            class="page-button"
            type="button"
            onclick={() => selectPage(page.id)}
            data-testid="page-tree-page"
          >
            {#if PageIcon}
              <PageIcon size={14} class="page-button__icon" style="color: {pageColor};" aria-hidden="true" />
            {/if}
            <span class="page-button__title">{page.title}</span>
          </button>
          <span class="kebab-slot">
            <Tooltip content={t('common.actions')} placement="bottom" class="inline-flex">
              <DropdownMenu
                triggerIcon={Dots}
                triggerIconClass="w-4 h-4"
                items={kebabItems(page)}
                showChevron={false}
                iconOnly={true}
                placement="bottom-end"
                triggerClass="kebab-trigger"
                triggerTestid="page-kebab"
              />
            </Tooltip>
          </span>
        </li>
      {/each}
    </ul>
  {/if}
</ScrollableSidebar>

{#if moveDialogPage}
  <PageMoveDialog
    bind:isOpen={moveDialogOpen}
    {workspaceId}
    page={moveDialogPage}
    onMoved={loadTree}
  />
{/if}

{#if permsDialogPage}
  <PagePermissionsDialog
    bind:isOpen={permsDialogOpen}
    {workspaceId}
    pageId={permsDialogPage.id}
    onUpdated={loadTree}
  />
{/if}

<style>
  :global(.pages-sidebar) {
    height: 100%;
    background: var(--ds-surface);
    border-right: 1px solid var(--ds-border);
  }

  :global(.pages-sidebar--embedded) {
    height: 100%;
    min-height: 0;
    flex: 1;
    border-right: none;
  }

  .header {
    padding: 0.75rem 0.75rem 0.5rem;
    border-bottom: 1px solid var(--ds-border);
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .title-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0 0.25rem;
  }

  .title-row h2 {
    margin: 0;
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--ds-text-subtle);
  }

  .title-actions {
    display: inline-flex;
    align-items: center;
    gap: 0.125rem;
  }

  .header-button {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 1.5rem;
    height: 1.5rem;
    border-radius: 0.25rem;
    border: none;
    background: transparent;
    color: var(--ds-text-subtle);
    cursor: pointer;
  }

  .header-button:hover:not(:disabled) {
    background: var(--ds-background-neutral-hovered);
    color: var(--ds-text);
  }

  .header-button:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .header-button--active {
    background: var(--ds-surface-selected);
    color: var(--ds-text);
  }

  .search-row {
    display: flex;
    align-items: center;
    gap: 0.375rem;
    padding: 0.5rem 1rem;
    border-bottom: 1px solid var(--ds-border);
  }

  :global(.search-row__icon) {
    color: var(--ds-text-subtle);
    flex-shrink: 0;
  }

  .search-row__clear {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    background: transparent;
    border: none;
    color: var(--ds-text-subtle);
    cursor: pointer;
    padding: 0.125rem;
    border-radius: 0.25rem;
    flex-shrink: 0;
  }

  .search-row__clear:hover {
    color: var(--ds-text);
    background: var(--ds-background-neutral-hovered);
  }

  .filter-row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.375rem;
    padding: 0.5rem 1rem;
    border-bottom: 1px solid var(--ds-border);
  }

  .filter-chip__remove {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    background: transparent;
    border: none;
    color: inherit;
    cursor: pointer;
    padding: 0;
    opacity: 0.7;
    transition: opacity 120ms;
  }

  .filter-chip__remove:hover {
    opacity: 1;
  }

  .filter-clear {
    background: transparent;
    border: none;
    color: var(--ds-text-subtle);
    font-size: 0.75rem;
    cursor: pointer;
    padding: 0.125rem 0.25rem;
  }

  .filter-clear:hover {
    color: var(--ds-text);
    text-decoration: underline;
  }

  .tree {
    list-style: none;
    padding: 0.5rem 0;
    margin: 0;
  }

  .tree-item {
    position: relative;
    display: flex;
    align-items: center;
    gap: 0.25rem;
    padding-right: 0.5rem;
    transition: background-color var(--duration-fast, 100ms) ease;
  }

  .tree-item.dimmed .page-button {
    color: var(--ds-text-subtle);
    opacity: 0.7;
  }

  .chevron {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 1.125rem;
    height: 1.125rem;
    flex-shrink: 0;
    border: none;
    background: transparent;
    color: var(--ds-text-subtle);
    border-radius: 0.25rem;
    cursor: pointer;
    padding: 0;
  }

  .chevron:hover {
    background: var(--ds-background-neutral-hovered);
    color: var(--ds-text);
  }

  /* Empty slot keeps title text aligned with rows that DO have a chevron,
     so leaf and branch rows share a left edge. */
  .chevron--placeholder {
    cursor: default;
  }

  .chevron--placeholder::before {
    content: '';
    width: 0.25rem;
    height: 0.25rem;
    border: 1px solid currentColor;
    border-radius: 9999px;
    opacity: 0.7;
  }

  .chevron--placeholder:hover {
    background: transparent;
    color: var(--ds-text-subtle);
  }

  .tree-item.active .page-button {
    background: var(--ds-surface-selected);
    font-weight: 500;
  }

  /* Drop affordances: a thin line for sibling drops, full-row tint for child drops */
  .tree-item.drop-top::before,
  .tree-item.drop-bottom::after {
    content: '';
    position: absolute;
    left: 0.25rem;
    right: 0.25rem;
    height: 2px;
    background: var(--ds-border-focused, #3b82f6);
    pointer-events: none;
  }

  .tree-item.drop-top::before {
    top: -1px;
  }

  .tree-item.drop-bottom::after {
    bottom: -1px;
  }

  .tree-item.drop-on {
    background: color-mix(in srgb, var(--ds-border-focused, #3b82f6) 12%, transparent);
  }

  .page-button {
    flex: 1;
    min-width: 0;
    display: inline-flex;
    align-items: center;
    gap: 0.375rem;
    text-align: left;
    background: transparent;
    border: none;
    padding: 0.375rem 0.375rem;
    font-size: 0.875rem;
    color: var(--ds-text);
    cursor: pointer;
    border-radius: 0.25rem;
    white-space: nowrap;
    overflow: hidden;
  }

  :global(.page-button__icon) {
    flex-shrink: 0;
  }

  .page-button__title {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .page-button:hover {
    background: var(--ds-background-neutral-hovered);
  }

  .kebab-slot {
    opacity: 0;
    transition: opacity var(--duration-fast, 100ms) ease;
  }

  .tree-item:hover .kebab-slot,
  .tree-item:focus-within .kebab-slot {
    opacity: 1;
  }

  :global(.kebab-trigger) {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 1.5rem;
    height: 1.5rem;
    border-radius: 0.25rem;
    border: none;
    background: transparent;
    color: var(--ds-text-subtle);
    cursor: pointer;
  }

  :global(.kebab-trigger:hover) {
    background: var(--ds-background-neutral-hovered);
    color: var(--ds-text);
  }

  .status {
    color: var(--ds-text-subtle);
    font-size: 0.875rem;
    padding: 1rem;
  }

  .tree-empty {
    padding: 0.5rem;
  }
</style>
