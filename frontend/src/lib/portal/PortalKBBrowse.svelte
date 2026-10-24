<script>
  import { onMount } from 'svelte';
  import BookOpen from '@lucide/svelte/icons/book-open';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import FileText from '@lucide/svelte/icons/file-text';
  import { portal } from '../api/portal.js';
  import { navigate } from '../router.js';
  import { portalCustomizationStore as portalStore } from '../stores/portal.svelte.js';
  import StateDisplay from '../components/StateDisplay.svelte';
  import EmptyState from '../components/EmptyState.svelte';

  /**
   * Knowledge-base browse listing: every workspace page this portal
   * publishes, as a nested tree. Entry point is /portal/{slug}/kb; each
   * row deep-links to /portal/{slug}/kb/{pageId}.
   */

  let pages = $state([]);
  let loading = $state(true);
  let error = $state('');

  const slug = $derived(portalStore.currentSlug);

  // Children lookup keyed by parent id; roots have parent_id null. The
  // server returns each wired workspace's pages in stable sibling order.
  let childrenByParent = $derived.by(() => {
    const map = new Map();
    for (const page of pages) {
      const key = page.parent_id ?? null;
      if (!map.has(key)) map.set(key, []);
      map.get(key).push(page);
    }
    return map;
  });

  let roots = $derived(childrenByParent.get(null) ?? []);
  let totalArticles = $derived(pages.length);

  async function load() {
    loading = true;
    error = '';
    try {
      const result = await portal.listKnowledgeBasePages(slug);
      pages = Array.isArray(result) ? result : (result?.data ?? []);
    } catch (err) {
      error = err?.message || 'Failed to load the knowledge base.';
      pages = [];
    } finally {
      loading = false;
    }
  }

  onMount(load);
</script>

<div class="kb-browse" data-testid="portal-kb-browse">
  <div class="kb-browse__header">
    <div>
      <h1 class="kb-browse__title">
        <BookOpen class="kb-browse__title-icon" />
        Knowledge base
      </h1>
      {#if !loading && !error}
        <p class="kb-browse__count" data-testid="portal-kb-browse-count">
          {totalArticles} article{totalArticles === 1 ? '' : 's'}
        </p>
      {/if}
    </div>
  </div>

  {#if loading}
    <StateDisplay type="loading" size="lg" message="Loading articles..." />
  {:else if error}
    <StateDisplay type="error" title="Load Failed" message={error} />
  {:else if roots.length === 0}
    <EmptyState
      icon={FileText}
      title="Nothing published yet"
      message="No articles are available in this knowledge base right now."
    />
  {:else}
    <nav class="kb-tree" aria-label="Knowledge base articles">
      {@render rows(roots, 0)}
    </nav>
  {/if}
</div>

{#snippet rows(children, depth)}
  <ul class="kb-tree__level" class:kb-tree__level--nested={depth > 0}>
    {#each children as page (page.page_id)}
      <li>
        <button
          type="button"
          class="kb-tree__row"
          class:kb-tree__row--nested={depth > 0}
          style="--kb-depth: {depth};"
          data-testid="portal-kb-row-{page.page_id}"
          onclick={() => navigate(`/portal/${slug}/kb/${page.page_id}`)}
        >
          <FileText class="kb-tree__row-icon" />
          <span class="kb-tree__row-title">{page.title}</span>
          <ChevronRight class="kb-tree__row-chevron" />
        </button>
        {#if (childrenByParent.get(page.page_id) ?? []).length > 0}
          {@render rows(childrenByParent.get(page.page_id) ?? [], depth + 1)}
        {/if}
      </li>
    {/each}
  </ul>
{/snippet}

<style>
  .kb-browse {
    max-width: 56rem;
    margin: 0 auto;
  }

  .kb-browse__header {
    margin-bottom: 1.5rem;
  }

  .kb-browse__title {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 1.5rem;
    font-weight: 600;
    color: var(--ds-text);
  }

  .kb-browse__count {
    margin-top: 0.25rem;
    font-size: 0.875rem;
    color: var(--ds-text-subtle);
  }

  .kb-tree,
  .kb-tree__level {
    list-style: none;
    margin: 0;
    padding: 0;
  }

  .kb-tree__level--nested {
    margin-left: 1.25rem;
    border-left: 1px solid var(--ds-border);
    padding-left: 0.5rem;
  }

  .kb-tree__row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    width: 100%;
    padding: 0.625rem 0.75rem;
    border: 1px solid transparent;
    border-radius: 0.5rem;
    background: transparent;
    cursor: pointer;
    text-align: left;
    transition: background-color 150ms ease, border-color 150ms ease;
  }

  .kb-tree__row:hover {
    background-color: var(--ds-background-neutral);
    border-color: var(--ds-border);
  }

  .kb-tree__row-title {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--ds-text);
    font-size: 0.9375rem;
  }

</style>
