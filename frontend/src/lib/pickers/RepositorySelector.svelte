<script>
  import { createEventDispatcher, onMount } from 'svelte';
  import { api } from '../api.js';
  import Button from '../components/Button.svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';
  import { X, Search, Loader2, Check, GitBranch, Lock, Globe, ChevronLeft, ChevronRight } from 'lucide-svelte';

  export let workspaceId;
  export let connection;

  const dispatch = createEventDispatcher();

  let loading = true;
  let linking = false;
  let searchQuery = '';
  let repositories = [];
  let selectedRepos = new Set();
  let error = null;

  // Pagination
  let page = 1;
  let perPage = 30;
  let hasMore = true;

  onMount(async () => {
    await loadRepositories();
  });

  async function loadRepositories(resetPage = true) {
    if (resetPage) {
      page = 1;
      repositories = [];
    }
    loading = true;
    error = null;

    try {
      const result = await api.workspaceSCM.getAvailableRepos(workspaceId, connection.id, {
        page,
        per_page: perPage
      });

      if (result.error) {
        error = result.error;
        repositories = [];
      } else {
        const newRepos = result.repositories || [];
        if (resetPage) {
          repositories = newRepos;
        } else {
          repositories = [...repositories, ...newRepos];
        }
        hasMore = newRepos.length === perPage;
      }
    } catch (err) {
      console.error('Failed to load repositories:', err);
      error = 'Failed to load repositories';
      repositories = [];
    } finally {
      loading = false;
    }
  }

  async function loadMore() {
    page += 1;
    await loadRepositories(false);
  }

  function toggleRepo(repo) {
    if (repo.is_linked) return; // Already linked, can't select

    if (selectedRepos.has(repo.id)) {
      selectedRepos.delete(repo.id);
    } else {
      selectedRepos.add(repo.id);
    }
    selectedRepos = selectedRepos;
  }

  async function linkSelectedRepos() {
    if (selectedRepos.size === 0) return;

    linking = true;
    const linkedRepos = [];

    try {
      for (const repoId of selectedRepos) {
        const repo = repositories.find(r => r.id === repoId);
        if (repo) {
          await api.workspaceSCM.linkRepo(workspaceId, connection.id, {
            repository_external_id: repo.id,
            repository_name: repo.full_name,
            repository_url: repo.url,
            default_branch: repo.default_branch || 'main'
          });
          linkedRepos.push(repo);
        }
      }

      dispatch('linked', { repos: linkedRepos });
    } catch (err) {
      console.error('Failed to link repositories:', err);
      error = 'Failed to link some repositories';
    } finally {
      linking = false;
    }
  }

  function close() {
    dispatch('close');
  }

  // Filter repos by search
  $: filteredRepos = searchQuery
    ? repositories.filter(r =>
        r.full_name.toLowerCase().includes(searchQuery.toLowerCase()) ||
        (r.description && r.description.toLowerCase().includes(searchQuery.toLowerCase()))
      )
    : repositories;

  function getProviderLabel(providerType) {
    const labels = {
      github: 'GitHub',
      gitlab: 'GitLab',
      gitea: 'Gitea',
      bitbucket: 'Bitbucket'
    };
    return labels[providerType] || providerType;
  }
</script>

<!-- Modal Backdrop -->
<div
  class="fixed inset-0 flex items-center justify-center p-4 z-50"
  style="background-color: rgba(0, 0, 0, 0.3); backdrop-filter: blur(2px);"
  onclick={(e) => e.target === e.currentTarget && close()}
  onkeypress={(e) => e.key === 'Escape' && close()}
  role="dialog"
  aria-modal="true"
  aria-labelledby="repo-selector-title"
>
  <div
    class="w-full max-w-2xl max-h-[80vh] flex flex-col rounded-xl shadow-xl border overflow-hidden"
    style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
  >
    <!-- Header -->
    <div class="flex items-center justify-between px-6 py-4 border-b" style="border-color: var(--ds-border);">
      <div>
        <h2 id="repo-selector-title" class="text-lg font-semibold" style="color: var(--ds-text);">
          Link Repositories
        </h2>
        <p class="text-sm" style="color: var(--ds-text-subtle);">
          Select repositories from {connection.provider_name} ({getProviderLabel(connection.provider_type)})
        </p>
      </div>
      <button
        class="p-2 rounded-lg transition-colors"
        style="color: var(--ds-text-subtle);"
        onclick={close}
      >
        <X class="w-5 h-5" />
      </button>
    </div>

    <!-- Search -->
    <div class="px-6 py-3 border-b" style="border-color: var(--ds-border);">
      <div class="relative">
        <Search class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4" style="color: var(--ds-text-subtle);" />
        <input
          type="text"
          bind:value={searchQuery}
          placeholder="Search repositories..."
          class="w-full pl-10 pr-4 py-2 rounded-lg border text-sm"
          style="border-color: var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
        />
      </div>
    </div>

    <!-- Repository List -->
    <div class="flex-1 overflow-y-auto px-6 py-3">
      {#if loading && repositories.length === 0}
        <div class="flex items-center justify-center py-12">
          <Loader2 class="w-6 h-6 animate-spin" style="color: var(--ds-text-subtle);" />
          <span class="ml-2 text-sm" style="color: var(--ds-text-subtle);">Loading repositories...</span>
        </div>
      {:else if error}
        <div class="text-center py-12">
          <p class="text-sm" style="color: var(--ds-text-danger);">{error}</p>
          <Button size="sm" variant="secondary" class="mt-3" onclick={() => loadRepositories()}>
            Try Again
          </Button>
        </div>
      {:else if filteredRepos.length === 0}
        <div class="text-center py-12">
          <GitBranch class="w-8 h-8 mx-auto mb-2" style="color: var(--ds-text-subtlest);" />
          <p class="text-sm" style="color: var(--ds-text-subtle);">
            {searchQuery ? 'No repositories match your search' : 'No repositories available'}
          </p>
        </div>
      {:else}
        <div class="space-y-2">
          {#each filteredRepos as repo}
            <button
              class="w-full flex items-start gap-3 px-3 py-3 rounded-lg border text-left transition-colors"
              class:opacity-50={repo.is_linked}
              class:cursor-not-allowed={repo.is_linked}
              style="
                border-color: {selectedRepos.has(repo.id) ? 'var(--ds-interactive)' : 'var(--ds-border)'};
                background-color: {selectedRepos.has(repo.id) ? 'var(--ds-background-selected)' : 'var(--ds-surface)'};
              "
              onclick={() => toggleRepo(repo)}
              disabled={repo.is_linked}
            >
              <!-- Checkbox -->
              <div
                class="flex-shrink-0 w-5 h-5 rounded border flex items-center justify-center mt-0.5"
                style="
                  border-color: {selectedRepos.has(repo.id) || repo.is_linked ? 'var(--ds-interactive)' : 'var(--ds-border)'};
                  background-color: {selectedRepos.has(repo.id) || repo.is_linked ? 'var(--ds-interactive)' : 'transparent'};
                "
              >
                {#if selectedRepos.has(repo.id) || repo.is_linked}
                  <Check class="w-3 h-3" style="color: white;" />
                {/if}
              </div>

              <!-- Repo Info -->
              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-2">
                  <span class="font-medium truncate" style="color: var(--ds-text);">{repo.full_name}</span>
                  {#if repo.is_private}
                    <Lock class="w-3.5 h-3.5 flex-shrink-0" style="color: var(--ds-text-subtle);" />
                  {:else}
                    <Globe class="w-3.5 h-3.5 flex-shrink-0" style="color: var(--ds-text-subtle);" />
                  {/if}
                  {#if repo.is_linked}
                    <span class="text-xs px-1.5 py-0.5 rounded" style="background-color: var(--ds-background-success); color: var(--ds-text-success);">
                      Already linked
                    </span>
                  {/if}
                </div>
                {#if repo.description}
                  <p class="text-sm truncate mt-0.5" style="color: var(--ds-text-subtle);">{repo.description}</p>
                {/if}
                <div class="flex items-center gap-3 mt-1 text-xs" style="color: var(--ds-text-subtlest);">
                  <span class="flex items-center gap-1">
                    <GitBranch class="w-3 h-3" />
                    {repo.default_branch || 'main'}
                  </span>
                </div>
              </div>
            </button>
          {/each}
        </div>

        <!-- Load More -->
        {#if hasMore && !searchQuery}
          <div class="flex justify-center py-4">
            {#if loading}
              <Loader2 class="w-5 h-5 animate-spin" style="color: var(--ds-text-subtle);" />
            {:else}
              <Button size="sm" variant="secondary" onclick={loadMore}>
                Load More
              </Button>
            {/if}
          </div>
        {/if}
      {/if}
    </div>

    <!-- Footer -->
    <DialogFooter
      onCancel={close}
      onConfirm={linkSelectedRepos}
      confirmLabel="Link Selected"
      loadingLabel="Linking..."
      loading={linking}
      disabled={selectedRepos.size === 0}
    >
      {#snippet extra()}
        <span class="text-sm" style="color: var(--ds-text-subtle);">
          {selectedRepos.size} {selectedRepos.size === 1 ? 'repository' : 'repositories'} selected
        </span>
      {/snippet}
    </DialogFooter>
  </div>
</div>
