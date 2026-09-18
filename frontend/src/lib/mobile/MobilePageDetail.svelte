<script>
  import { ChevronRight, House, Loader, Pencil, X } from '@lucide/svelte';
  import { api } from '../api.js';
  import { navigate } from '../router.js';
  import { errorToast, infoToast } from '../stores/toasts.svelte.js';
  import { workspacesStore } from '../stores';
  import { t } from '../stores/i18n.svelte.js';
  import { formatRelativeCompact } from '../utils/dateFormatter.js';
  import { renderMarkdown } from '../utils/render-markdown.js';
  import SafeMarkdown from '../components/SafeMarkdown.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import MobileHeader from './MobileHeader.svelte';
  import MobilePromoteRequirementSheet from './MobilePromoteRequirementSheet.svelte';
  import { autoGrow, enterMovesFocus } from './autoGrowTextarea.js';
  import { pageAncestors, pageChildren } from './mobilePagesData.js';

  // Phone page reader: rendered markdown, breadcrumb, and sub-page rows.
  // Editing is a borderless full-page form (same Linear-style hero fields as
  // the item editors) guarded by the page's content hash against lost updates.
  let { workspaceId, pageId } = $props();

  let page = $state(null);
  let flatPages = $state([]);
  let canEdit = $state(false);
  let loading = $state(true);
  let errored = $state(false);
  let editing = $state(false);
  let draftTitle = $state('');
  let draftContent = $state('');
  let draftContentField = $state(null);
  let saving = $state(false);
  let pageRequirement = $state(null);
  let promoteSheetOpen = $state(false);
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

  function openRequirementRegistry() {
    if (!pageRequirement) return;
    navigate(`/m/requirements/${workspaceId}/${pageRequirement.requirement_number}`);
  }

  function handleRequirementPromoted(promoted) {
    pageRequirement = promoted;
    navigate(`/m/requirements/${workspaceId}/${promoted.requirement_number}`);
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
      const [pageRes, listRes, permsRes, reqRes] = await Promise.allSettled([
        api.pages.getPage(workspaceId, pageId),
        api.pages.getAll(workspaceId),
        api.pages.getPermissions(workspaceId, pageId),
        api.requirements.getByPage(workspaceId, pageId),
      ]);
      if (token !== loadToken) return;
      if (pageRes.status === 'rejected') throw pageRes.reason;
      page = pageRes.value;
      flatPages = listRes.status === 'fulfilled' ? (listRes.value ?? []) : [];
      const level = permsRes.status === 'fulfilled' ? (permsRes.value?.effective_level ?? '') : '';
      canEdit = level === 'edit' || level === 'admin';
      pageRequirement = reqRes.status === 'fulfilled' ? reqRes.value : null;
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
    pageRequirement = null;
    promoteSheetOpen = false;
    load(token);
  });
</script>

<MobileHeader title={page?.title ?? ''} onback={back}>
  {#snippet right()}
    {#if editing}
      <button class="hdr-btn" onclick={cancelEditing} data-testid="mobile-page-cancel" aria-label="Cancel editing" type="button">
        <X size={20} />
      </button>
    {:else if page}
      {#if pageRequirement?.key}
        <button
          class="hdr-btn requirement-link"
          onclick={openRequirementRegistry}
          data-testid="mobile-page-open-requirement"
          aria-label={t('requirements.mobile.openRequirement')}
          type="button"
        >
          <Lozenge color="blue">{pageRequirement.key}</Lozenge>
        </button>
      {:else if canEdit}
        <button
          class="hdr-btn promote-btn"
          onclick={() => (promoteSheetOpen = true)}
          data-testid="mobile-page-promote-requirement"
          type="button"
        >
          {t('requirements.mobile.promoteAction')}
        </button>
        <button class="hdr-btn" onclick={startEditing} data-testid="mobile-page-edit" aria-label="Edit page" type="button">
          <Pencil size={18} />
        </button>
      {/if}
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
  <!-- Borderless Linear-style editor: hero title + content, actions pinned
       to the bottom edge. Same modality as the item create/edit pages. -->
  <div class="editor" data-testid="mobile-page-editor">
    <textarea
      class="hero-title"
      bind:value={draftTitle}
      rows={1}
      enterkeyhint="next"
      placeholder="Page title"
      use:autoGrow={draftTitle}
      use:enterMovesFocus={{ next: draftContentField }}
      data-testid="mobile-page-title-input"
      aria-label="Page title"
    ></textarea>
    <textarea
      class="hero-content"
      bind:value={draftContent}
      bind:this={draftContentField}
      data-testid="mobile-page-content-input"
      aria-label="Page content (Markdown)"
      spellcheck="false"
      placeholder="Write something…"
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

<MobilePromoteRequirementSheet
  {workspaceId}
  {pageId}
  bind:isOpen={promoteSheetOpen}
  onPromoted={handleRequirementPromoted}
/>

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
  .promote-btn {
    width: auto;
    padding: 0 0.5rem;
    font-size: 0.8125rem;
    font-weight: var(--font-semibold, 600);
    color: var(--ds-text-link, var(--ds-interactive));
  }
  .requirement-link {
    width: auto;
    padding: 0 0.25rem;
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

  /* Borderless editor fields — same hero treatment as the item editors. */
  .editor {
    display: flex;
    flex-direction: column;
    min-height: 100%;
    box-sizing: border-box;
    padding: 0.75rem 1rem 0;
    gap: 0.5rem;
  }
  .hero-title {
    width: 100%;
    margin: 0.5rem 0 0;
    padding: 0;
    border: none;
    background: transparent;
    color: var(--ds-text);
    font-family: inherit;
    font-size: 1.35rem;
    font-weight: var(--font-semibold, 600);
    line-height: 1.25;
    overflow: hidden;
    resize: none;
  }
  .hero-title::placeholder { color: var(--ds-text-subtlest, var(--ds-text-subtle)); font-weight: var(--font-semibold, 600); }
  .hero-content {
    width: 100%;
    min-height: 50dvh;
    padding: 0;
    border: none;
    background: transparent;
    color: var(--ds-text);
    font-family: inherit;
    font-size: max(1rem, 16px);
    line-height: 1.55;
    resize: none;
  }
  .hero-content::placeholder { color: var(--ds-text-subtlest, var(--ds-text-subtle)); }
  .hero-title:focus,
  .hero-content:focus { outline: none; }

  .editor-actions {
    position: sticky;
    bottom: 0;
    z-index: 20;
    display: flex;
    gap: 0.5rem;
    justify-content: flex-end;
    margin-top: auto;
    padding: 0.6rem 0 calc(env(safe-area-inset-bottom, 0px) + 0.75rem);
    background: linear-gradient(to top, var(--ds-surface) 65%, transparent);
  }
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
    border: none;
    background: var(--ds-background-neutral);
    color: var(--ds-text);
  }
  .btn.primary {
    border: none;
    background: var(--ds-interactive);
    color: var(--ds-text-inverse, #fff);
  }
  .btn:disabled { opacity: 0.6; }
</style>
