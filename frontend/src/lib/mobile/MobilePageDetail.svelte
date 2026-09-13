<script>
  import { ChevronRight, House, Loader, Pencil, X } from '@lucide/svelte';
  import { api } from '../api.js';
  import { navigate } from '../router.js';
  import { errorToast, infoToast } from '../stores/toasts.svelte.js';
  import { workspacesStore } from '../stores';
  import { formatRelativeCompact } from '../utils/dateFormatter.js';
  import { renderMarkdown } from '../utils/render-markdown.js';
  import SafeMarkdown from '../components/SafeMarkdown.svelte';
  import MobileHeader from './MobileHeader.svelte';
  import { pageAncestors, pageChildren } from './mobilePagesData.js';

  // Phone page reader: rendered markdown, breadcrumb, and sub-page rows.
  // Editing is a plain markdown textarea (the Milkdown rich editor stays
  // desktop-only) guarded by the page's content hash against lost updates.
  let { workspaceId, pageId } = $props();

  let page = $state(null);
  let flatPages = $state([]);
  let canEdit = $state(false);
  let loading = $state(true);
  let errored = $state(false);
  let editing = $state(false);
  let draftTitle = $state('');
  let draftContent = $state('');
  let saving = $state(false);
  // Guard in-place navigation (page → sub-page) against out-of-order loads.
  let loadToken = 0;

  const workspaceName = $derived.by(() => {
    const store = $workspacesStore;
    const regular = store?.regularWorkspaces?.find((ws) => ws.id === workspaceId);
    if (regular) return regular.name;
    if (store?.personalWorkspace?.id === workspaceId) return store.personalWorkspace.name ?? '';
    return '';
  });
  const ancestors = $derived(pageAncestors(flatPages, page));
  const children = $derived(pageChildren(flatPages, pageId));
  const contentHtml = $derived(page ? renderMarkdown(page.content) : '');

  function back() {
    if (window.history.length > 1) window.history.back();
    else navigate('/m/pages');
  }

  function openSubPage(id) {
    navigate(`/m/pages/${workspaceId}/${id}`);
  }

  function startEditing() {
    draftTitle = page.title;
    draftContent = page.content;
    editing = true;
  }

  function cancelEditing() {
    editing = false;
  }

  async function save() {
    if (saving) return;
    saving = true;
    try {
      const updated = await api.pages.updatePage(workspaceId, pageId, {
        title: draftTitle.trim() || page.title,
        content: draftContent,
        expectedContentHash: page.content_hash,
      });
      page = { ...page, ...updated };
      editing = false;
      infoToast('Page saved.');
    } catch (err) {
      console.error('Failed to save page:', err);
      errorToast(
        err?.status === 409
          ? 'This page changed elsewhere. Reload and try again.'
          : 'Saving failed. Try again.',
      );
    } finally {
      saving = false;
    }
  }

  async function load(token) {
    loading = true;
    errored = false;
    editing = false;
    try {
      // Fetch the page and the workspace tree in parallel; the tree powers
      // breadcrumbs + sub-page rows and may be permission-denied on its own.
      const [pageRes, listRes, permsRes] = await Promise.allSettled([
        api.pages.getPage(workspaceId, pageId),
        api.pages.getAll(workspaceId),
        api.pages.getPermissions(workspaceId, pageId),
      ]);
      if (token !== loadToken) return;
      if (pageRes.status === 'rejected') throw pageRes.reason;
      page = pageRes.value;
      flatPages = listRes.status === 'fulfilled' ? (listRes.value ?? []) : [];
      const level = permsRes.status === 'fulfilled' ? (permsRes.value?.effective_level ?? '') : '';
      canEdit = level === 'edit' || level === 'admin';
    } catch (err) {
      console.error('Failed to load page:', err);
      if (token === loadToken) errored = true;
    } finally {
      if (token === loadToken) loading = false;
    }
  }

  // Reload on id change — the component is reused when navigating into a
  // sub-page, so onMount alone would show stale content.
  $effect(() => {
    const ws = workspaceId;
    const id = pageId;
    if (ws == null || id == null) return;
    const token = ++loadToken;
    page = null;
    flatPages = [];
    canEdit = false;
    load(token);
  });
</script>

<MobileHeader title={page?.title ?? ''} onback={back}>
  {#snippet right()}
    {#if editing}
      <button class="hdr-btn" onclick={cancelEditing} data-testid="mobile-page-cancel" aria-label="Cancel editing" type="button">
        <X size={20} />
      </button>
    {:else if canEdit && page}
      <button class="hdr-btn" onclick={startEditing} data-testid="mobile-page-edit" aria-label="Edit page" type="button">
        <Pencil size={18} />
      </button>
    {/if}
  {/snippet}
</MobileHeader>

{#if loading}
  <div class="center" data-testid="page-loading"><Loader class="spin" size={22} /></div>
{:else if errored || !page}
  <div class="msg" data-testid="page-error">
    <p>Couldn't load this page.</p>
    <button class="retry" onclick={() => load(++loadToken)} disabled={loading} type="button">Retry</button>
  </div>
{:else if editing}
  <div class="editor" data-testid="mobile-page-editor">
    <input
      class="title-input"
      bind:value={draftTitle}
      data-testid="mobile-page-title-input"
      aria-label="Page title"
      type="text"
    />
    <textarea
      class="content-input"
      bind:value={draftContent}
      data-testid="mobile-page-content-input"
      aria-label="Page content (Markdown)"
      spellcheck="false"
    ></textarea>
    <div class="editor-actions">
      <button class="btn secondary" onclick={cancelEditing} data-testid="mobile-page-editor-cancel" type="button">Cancel</button>
      <button class="btn primary" onclick={save} disabled={saving} data-testid="mobile-page-save" type="button">
        {#if saving}<Loader class="spin" size={16} />{/if}
        Save
      </button>
    </div>
  </div>
{:else}
  <div class="detail" data-testid="mobile-page-detail">
    {#if ancestors.length > 0}
      <nav class="breadcrumb" data-testid="page-breadcrumb" aria-label="Ancestor pages">
        {#each ancestors as anc (anc.id)}
          <button class="crumb" onclick={() => openSubPage(anc.id)} data-testid="page-breadcrumb-crumb" type="button">
            {#if anc.is_home}<House size={12} />{/if}
            {anc.title}
          </button>
          <ChevronRight size={13} class="crumb-sep" />
        {/each}
        <span class="crumb current">{page.title}</span>
      </nav>
    {/if}

    <h1 class="title" data-testid="mobile-page-title">
      {#if page.is_home}<House size={16} class="title-home" />{/if}
      {page.title}
    </h1>

    <p class="meta" data-testid="mobile-page-meta">
      {#if workspaceName}{workspaceName} · {/if}
      Updated {page.updated_at ? formatRelativeCompact(new Date(page.updated_at)) : '—'}
    </p>

    {#if page.content}
      <SafeMarkdown html={contentHtml} testid="mobile-page-content" />
    {:else}
      <p class="empty" data-testid="mobile-page-empty">This page is empty.</p>
    {/if}

    {#if children.length > 0}
      <section class="subpages" data-testid="mobile-page-subpages">
        <h2 class="section-title">Sub-pages</h2>
        <div class="sub-rows">
          {#each children as child (child.id)}
            <button
              class="sub-row"
              onclick={() => openSubPage(child.id)}
              data-testid="mobile-page-child"
              data-page-id={child.id}
              type="button"
            >
              <span class="sub-title">{child.title}</span>
              <ChevronRight size={16} class="chev" />
            </button>
          {/each}
        </div>
      </section>
    {/if}
  </div>
{/if}

<style>
  .hdr-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 36px;
    height: 36px;
    border: none;
    background: transparent;
    color: var(--ds-text);
    cursor: pointer;
  }

  .center { display: flex; justify-content: center; padding: 3rem; color: var(--ds-text-subtle); }
  :global(.spin) { animation: spin 1s linear infinite; }
  @keyframes spin { to { transform: rotate(360deg); } }

  .msg { padding: 3rem 1.25rem; text-align: center; color: var(--ds-text-subtle); }
  .msg p { margin: 0; }
  .retry {
    min-height: 40px;
    margin-top: 0.75rem;
    padding: 0.45rem 1rem;
    border: 1px solid var(--ds-interactive);
    border-radius: var(--radius-md, 6px);
    background: var(--ds-interactive);
    color: var(--ds-text-inverse, #fff);
    font: inherit;
    font-weight: var(--font-semibold, 600);
    cursor: pointer;
  }
  .retry:disabled { opacity: 0.6; }

  .detail { padding: 0.875rem 0.875rem 2rem; }

  .breadcrumb {
    display: flex;
    align-items: center;
    flex-wrap: nowrap;
    gap: 0.2rem;
    margin-bottom: 0.75rem;
    overflow-x: auto;
    -webkit-overflow-scrolling: touch;
  }
  .crumb {
    display: inline-flex;
    align-items: center;
    gap: 0.25rem;
    flex-shrink: 0;
    border: none;
    background: transparent;
    cursor: pointer;
    padding: 2px 4px;
    font-size: 0.8125rem;
    color: var(--ds-text-link, var(--ds-interactive));
    white-space: nowrap;
  }
  .crumb-sep { flex-shrink: 0; color: var(--ds-text-subtlest, var(--ds-text-subtle)); }
  .crumb.current {
    flex-shrink: 0;
    cursor: default;
    color: var(--ds-text-subtle);
  }

  .title {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    font-size: 1.35rem;
    font-weight: var(--font-semibold, 600);
    color: var(--ds-text);
    margin: 0 0 0.35rem;
    line-height: 1.25;
  }
  .title :global(.title-home) { color: var(--ds-icon-subtle, var(--ds-text-subtle)); flex-shrink: 0; }

  .meta {
    font-size: 0.78125rem;
    color: var(--ds-text-subtle);
    margin: 0 0 1.1rem;
  }

  .empty { color: var(--ds-text-subtle); }

  .subpages { margin-top: 1.75rem; border-top: 1px solid var(--ds-border); padding-top: 1rem; }
  .section-title {
    font-size: 0.9375rem;
    font-weight: var(--font-semibold, 600);
    color: var(--ds-text);
    margin: 0 0 0.5rem;
  }
  .sub-rows {
    display: flex;
    flex-direction: column;
    border: 1px solid var(--ds-border);
    border-radius: var(--radius-lg, 8px);
    overflow: hidden;
  }
  .sub-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    min-height: 46px;
    padding: 0.35rem 0.75rem;
    border: none;
    border-bottom: 1px solid var(--ds-border);
    background: transparent;
    text-align: left;
    cursor: pointer;
  }
  .sub-row:last-child { border-bottom: none; }
  .sub-row:active { background-color: var(--ds-background-neutral-hovered); }
  .sub-title {
    font-size: 0.9375rem;
    color: var(--ds-text);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .sub-row :global(.chev) { color: var(--ds-icon-subtle, var(--ds-text-subtle)); flex-shrink: 0; }

  .editor { display: flex; flex-direction: column; gap: 0.6rem; padding: 0.75rem 0.875rem 2rem; }
  .title-input {
    min-height: 44px;
    padding: 0.4rem 0.6rem;
    border: 1px solid var(--ds-border);
    border-radius: var(--radius-lg, 8px);
    background-color: var(--ds-surface-raised);
    font-size: 1.125rem;
    font-weight: var(--font-semibold, 600);
    color: var(--ds-text);
  }
  .content-input {
    min-height: 55dvh;
    padding: 0.6rem;
    border: 1px solid var(--ds-border);
    border-radius: var(--radius-lg, 8px);
    background-color: var(--ds-surface-raised);
    font-family: var(--font-mono, monospace);
    font-size: 0.875rem;
    line-height: 1.5;
    color: var(--ds-text);
    resize: vertical;
  }
  .title-input:focus,
  .content-input:focus { outline: 2px solid var(--ds-interactive); outline-offset: -1px; }
  .editor-actions { display: flex; gap: 0.5rem; justify-content: flex-end; }
  .btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 0.35rem;
    min-height: 42px;
    padding: 0 1.1rem;
    border-radius: var(--radius-lg, 8px);
    font: inherit;
    font-weight: var(--font-semibold, 600);
    cursor: pointer;
  }
  .btn.secondary {
    border: 1px solid var(--ds-border);
    background: var(--ds-surface);
    color: var(--ds-text);
  }
  .btn.primary {
    border: 1px solid var(--ds-interactive);
    background: var(--ds-interactive);
    color: var(--ds-text-inverse, #fff);
  }
  .btn:disabled { opacity: 0.6; }
</style>
