<script>
  import { onMount } from 'svelte';
  import Spinner from '../components/Spinner.svelte';
  import { setOAuthReturnURL } from '../utils/oauthReturn.js';
  import StateDisplay from '../components/StateDisplay.svelte';
  import { useDebounce } from 'runed';
  import { workspaceSCM } from '../api/scm.js';
  import Button from '../components/Button.svelte';
  import Input from '../components/Input.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import ModalBackdrop from '../components/ModalBackdrop.svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';
  import X from '@lucide/svelte/icons/x';
  import Search from '@lucide/svelte/icons/search';
  import Check from '@lucide/svelte/icons/check';
  import GitBranch from '@lucide/svelte/icons/git-branch';
  import Lock from '@lucide/svelte/icons/lock';
  import Globe from '@lucide/svelte/icons/globe';
  import { t } from '../stores/i18n.svelte.js';

  let { workspaceId, connection, onclose, onlinked } = $props();

  let loading = $state(true);
  let linking = $state(false);
  let searchQuery = $state('');
  let repositories = $state([]);
  let selectedRepos = $state(new Set());
  let error = $state(null);
  let errorCode = $state(null);

  // Pagination
  let page = $state(1);
  let perPage = 30;
  let hasMore = $state(true);

  onMount(async () => {
    await loadRepositories();
  });

  // Search goes to the SCM provider so results come from all projects, not
  // just the pages already fetched. Providers without native search ignore
  // the param and return the unfiltered page; filteredRepos narrows those.
  async function loadRepositories(resetPage = true) {
    if (resetPage) {
      page = 1;
      repositories = [];
    }
    loading = true;
    error = null;
    errorCode = null;

    try {
      const params = { page, per_page: perPage };
      const query = searchQuery.trim();
      if (query) params.search = query;
      const result = await workspaceSCM.getAvailableRepos(workspaceId, connection.id, params);

      if (result.error) {
        error = result.error;
        errorCode = result.error_code || null;
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

  // Debounced re-fetch on search input so each keystroke doesn't hit the SCM
  // API — the query goes to the provider (page 1) rather than only filtering
  // whatever pages happen to be loaded already.
  const debouncedSearch = useDebounce(() => loadRepositories(true), 350);

  function toggleRepo(repo) {
    if (repo.is_linked) return; // Already linked, can't select

    const next = new Set(selectedRepos);
    if (next.has(repo.id)) {
      next.delete(repo.id);
    } else {
      next.add(repo.id);
    }
    selectedRepos = next;
  }

  async function linkSelectedRepos() {
    if (selectedRepos.size === 0) return;

    linking = true;
    const linkedRepos = [];

    try {
      for (const repoId of selectedRepos) {
        const repo = repositories.find(r => r.id === repoId);
        if (repo) {
          await workspaceSCM.linkRepo(workspaceId, connection.id, {
            repository_external_id: repo.id,
            repository_name: repo.full_name,
            repository_url: repo.url,
            default_branch: repo.default_branch || 'main'
          });
          linkedRepos.push(repo);
        }
      }

      onlinked?.({ repos: linkedRepos });
    } catch (err) {
      console.error('Failed to link repositories:', err);
      error = 'Failed to link some repositories';
    } finally {
      linking = false;
    }
  }

  function startOAuthConnect() {
    setOAuthReturnURL();
    workspaceSCM.startOAuth(workspaceId, connection.id).then(result => {
      if (result?.auth_url) {
        window.location.href = result.auth_url;
      }
    }).catch(err => {
      error = 'Failed to start OAuth connection';
    });
  }

  function close() {
    onclose?.();
  }

  // Filter repos by search
  let filteredRepos = $derived(searchQuery
    ? repositories.filter(r =>
        r.full_name.toLowerCase().includes(searchQuery.toLowerCase()) ||
        (r.description && r.description.toLowerCase().includes(searchQuery.toLowerCase()))
      )
    : repositories);

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
<!-- shortcut-guard-exempt: picker confirms via its explicit footer button; no submit shortcut owned -->
<ModalBackdrop show={true} onclose={close} ariaLabelledBy="repo-selector-title">
  <div
    class="w-full max-w-2xl max-h-[80vh] flex flex-col rounded-lg shadow-xl border overflow-hidden"
    style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
  >
    <!-- Header -->
    <div class="flex items-center justify-between px-6 py-4 border-b" style="border-color: var(--ds-border);">
      <div>
        <h2 id="repo-selector-title" class="text-lg font-semibold" style="color: var(--ds-text);">
          {t('pickers.linkRepositories')}
        </h2>
        <p class="text-sm" style="color: var(--ds-text-subtle);">
          {t('pickers.selectRepositoriesFrom', { provider: connection.provider_name, type: getProviderLabel(connection.provider_type) })}
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
        <Input
          type="text"
          bind:value={searchQuery}
          oninput={debouncedSearch}
          dataTestid="repository-selector-search"
          placeholder={t('pickers.searchRepositories')}
          class="pl-10"
          size="small"
        />
      </div>
    </div>

    <!-- Repository List -->
    <div class="flex-1 overflow-y-auto px-6 py-3">
      {#if loading && repositories.length === 0}
        <StateDisplay type="loading" message={t('pickers.loadingRepositories')} />
      {:else if error}
        <div class="text-center py-12">
          {#if errorCode === 'user_scm_not_connected'}
            <GitBranch class="w-8 h-8 mx-auto mb-2" style="color: var(--ds-text-subtlest);" />
            <p class="text-sm mb-3" style="color: var(--ds-text-subtle);">
              Connect your account to browse repositories
            </p>
            <Button size="sm" variant="primary" onclick={startOAuthConnect}>
              Connect {getProviderLabel(connection.provider_type)} Account
            </Button>
          {:else}
            <p class="text-sm" style="color: var(--ds-text-danger);">{error}</p>
            <Button size="sm" variant="secondary" class="mt-3" onclick={() => loadRepositories()}>
              {t('common.retry')}
            </Button>
          {/if}
        </div>
      {:else if filteredRepos.length === 0}
        <EmptyState
          icon={GitBranch}
          title={searchQuery ? t('pickers.noRepositoriesMatchSearch') : t('pickers.noRepositoriesAvailable')}
        />
      {:else}
        <div class="space-y-2">
          {#each filteredRepos as repo}
            <button
              data-testid="repository-selector-repo"
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
                      {t('pickers.alreadyLinked')}
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
        {#if hasMore}
          <div class="flex justify-center py-4">
            {#if loading}
              <Spinner size="sm" />
            {:else}
              <Button size="sm" variant="secondary" onclick={loadMore}>
                {t('common.loadMore')}
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
      confirmLabel={t('pickers.linkSelected')}
      loadingLabel={t('pickers.linking')}
      loading={linking}
      disabled={selectedRepos.size === 0}
    >
      {#snippet extra()}
        <span class="text-sm" style="color: var(--ds-text-subtle);">
          {t('pickers.repositoriesSelected', { count: selectedRepos.size })}
        </span>
      {/snippet}
    </DialogFooter>
  </div>
</ModalBackdrop>
