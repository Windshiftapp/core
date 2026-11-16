<script>
  import { portalCustomizationStore as portalStore } from '../stores/portal.svelte.js';
  import { portalSearchStore } from '../stores/portalSearch.svelte.js';
  import SearchInput from '../components/SearchInput.svelte';
  import Link from '../components/Link.svelte';
  import Spinner from '../components/Spinner.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import PortalKBBrowse from '../portal/PortalKBBrowse.svelte';

  const external = $derived(portalStore.parseDocmostShareLink(portalStore.knowledgeBaseShareLink));
  function href(result) {
    return result.source === 'workspace_page'
      ? `/portal/${portalStore.currentSlug}/kb/${result.page_id}?source=search`
      : `${external.baseURL}/share/${external.shareID}/p/${result.slugId}`;
  }
</script>

<div class="space-y-5" data-testid="compact-knowledge">
  <SearchInput
    bind:value={portalSearchStore.query}
    dataTestid="compact-kb-search"
    placeholder="Search the knowledge base"
    on_input={() => portalSearchStore.searchDebounced()}
    on_keydown={event => { if (event.key === 'Enter') portalSearchStore.search(); }}
  />
  {#if portalSearchStore.visible}
    {#if portalSearchStore.loading}<Spinner />
    {:else if portalSearchStore.error}<AlertBox variant="error" message={portalSearchStore.error} />
    {:else}
      <div class="space-y-3">
        {#each portalSearchStore.results?.data || [] as result}
          <Link href={href(result)} target={result.source === 'workspace_page' ? undefined : '_blank'} rel="noopener" class="block rounded border border-ds-border p-4" data-testid="compact-kb-result">
            <h3 class="font-medium">{result.title}</h3>
            <p class="text-sm text-ds-text-subtle">{(result.highlight || result.excerpt || '').replace(/<[^>]*>/g, '')}</p>
          </Link>
        {:else}<EmptyState title="No matching articles" />{/each}
      </div>
    {/if}
  {:else if portalStore.knowledgeBasePageSources.length}
    <PortalKBBrowse />
  {:else if external.shareID}
    <Link href={portalStore.knowledgeBaseShareLink} target="_blank" rel="noopener">Open knowledge base</Link>
  {:else}<EmptyState title="No articles available" />{/if}
</div>
