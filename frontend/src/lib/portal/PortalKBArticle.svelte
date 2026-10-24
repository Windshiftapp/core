<script>
  import { onMount } from 'svelte';
  import ArrowLeft from '@lucide/svelte/icons/arrow-left';
  import BookOpen from '@lucide/svelte/icons/book-open';
  import { portal } from '../api/portal.js';
  import { navigate } from '../router.js';
  import { portalCustomizationStore as portalStore } from '../stores/portal.svelte.js';
  import StateDisplay from '../components/StateDisplay.svelte';
  import LazyMilkdownEditor from '../editors/LazyMilkdownEditor.svelte';

  /**
   * Full-page knowledge-base article. Deep-linkable at
   * /portal/{slug}/kb/{pageId}; the server 404s anything this portal's
   * wiring does not publish.
   */

  let { pageId } = $props();

  let page = $state(null);
  let loading = $state(true);
  let error = $state('');
  let requestSeq = 0;

  const slug = $derived(portalStore.currentSlug);

  async function load() {
    const seq = ++requestSeq;
    loading = true;
    error = '';
    try {
      // The navigation origin rides on the query string (search results append
      // ?source=search, ticket-conversation links ?source=ticket); anything
      // else is a plain browse view.
      const source = new URLSearchParams(window.location.search).get('source') || '';
      const result = await portal.getKnowledgeBasePage(slug, pageId, source);
      if (seq !== requestSeq) return;
      page = result;
    } catch (err) {
      if (seq !== requestSeq) return;
      page = null;
      error = err?.message || 'Failed to load this article.';
    } finally {
      if (seq === requestSeq) loading = false;
    }
  }

  // Reload when the route's page id changes (in-portal navigation between
  // articles) — not on every reactive touch of pageId.
  let lastLoadedId = null;
  $effect(() => {
    const id = pageId;
    if (id === lastLoadedId) return;
    lastLoadedId = id;
    load();
  });

  function goBack() {
    if (window.history.length > 1) {
      window.history.back();
    } else {
      navigate(`/portal/${slug}/kb`);
    }
  }
</script>

<div class="kb-article" data-testid="portal-kb-article">
  <button
    type="button"
    class="kb-article__back"
    data-testid="portal-kb-back"
    onclick={goBack}
  >
    <ArrowLeft class="kb-article__back-icon" />
    Knowledge base
  </button>

  {#if loading}
    <StateDisplay type="loading" size="lg" message="Loading article..." />
  {:else if error}
    <StateDisplay type="error" title="Article unavailable" message={error} />
  {:else if page}
    <article class="kb-article__card">
      <header class="kb-article__header">
        <BookOpen class="kb-article__icon" />
        <h1 class="kb-article__title" data-testid="portal-kb-article-title">{page.title}</h1>
      </header>
      {#if page.updated_at}
        <p class="kb-article__updated">
          Last updated
          {new Date(page.updated_at).toLocaleDateString(undefined, {
            year: 'numeric',
            month: 'long',
            day: 'numeric',
          })}
        </p>
      {/if}
      <div class="kb-article__body" data-testid="portal-kb-article-content">
        <LazyMilkdownEditor
          content={page.content}
          readonly={true}
          enableDiagrams={true}
        />
      </div>
    </article>
  {/if}
</div>

<style>
  .kb-article {
    max-width: 56rem;
    margin: 0 auto;
  }

  .kb-article__back {
    display: inline-flex;
    align-items: center;
    gap: 0.375rem;
    margin-bottom: 1rem;
    padding: 0.375rem 0.75rem;
    border: none;
    border-radius: 0.5rem;
    background: transparent;
    color: var(--ds-text-subtle);
    font-size: 0.875rem;
    cursor: pointer;
    transition: background-color 150ms ease, color 150ms ease;
  }

  .kb-article__back:hover {
    background-color: var(--ds-background-neutral);
    color: var(--ds-text);
  }

  .kb-article__card {
    background-color: var(--ds-surface-card);
    border: 1px solid var(--ds-border);
    border-radius: 0.75rem;
    padding: 2rem;
  }

  .kb-article__header {
    display: flex;
    align-items: center;
    gap: 0.625rem;
  }


  .kb-article__title {
    font-size: 1.5rem;
    font-weight: 600;
    color: var(--ds-text);
    min-width: 0;
  }

  .kb-article__updated {
    margin-top: 0.375rem;
    font-size: 0.8125rem;
    color: var(--ds-text-subtle);
  }

  .kb-article__body {
    margin-top: 1.25rem;
  }
</style>
