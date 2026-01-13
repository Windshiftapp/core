<script>
  import { createEventDispatcher, onMount } from 'svelte';
  import { api } from '../api.js';
  import Button from '../components/Button.svelte';
  import BasePicker from '../pickers/BasePicker.svelte';
  import Label from '../components/Label.svelte';
  import DialogFooter from './DialogFooter.svelte';
  import { X, GitMerge, GitBranch, GitCommit, Loader2, Search } from 'lucide-svelte';
  import EmptyState from '../components/EmptyState.svelte';

  export let itemId;

  const dispatch = createEventDispatcher();

  let loading = true;
  let submitting = false;
  let repositories = [];
  let error = null;

  // Form state
  let selectedRepoId = null;
  let linkType = 'pull_request';
  let externalId = '';
  let title = '';
  let externalUrl = '';

  onMount(async () => {
    await loadRepositories();
  });

  async function loadRepositories() {
    loading = true;
    error = null;

    try {
      repositories = await api.itemSCMLinks.getRepositories(itemId) || [];
      // Auto-select first repository if only one
      if (repositories.length === 1) {
        selectedRepoId = repositories[0].id;
      }
    } catch (err) {
      console.error('Failed to load repositories:', err);
      error = 'Failed to load repositories';
      repositories = [];
    } finally {
      loading = false;
    }
  }

  async function submit() {
    if (!selectedRepoId || !linkType || !externalId) {
      error = 'Please fill in all required fields';
      return;
    }

    submitting = true;
    error = null;

    try {
      // Build URL if not provided
      let url = externalUrl;
      if (!url && selectedRepoId) {
        const repo = repositories.find(r => r.id === selectedRepoId);
        if (repo) {
          url = buildExternalUrl(repo, linkType, externalId);
        }
      }

      const data = {
        workspace_repository_id: selectedRepoId,
        link_type: linkType,
        external_id: externalId.trim(),
        external_url: url,
        title: title.trim() || undefined,
      };

      await api.itemSCMLinks.create(itemId, data);
      dispatch('created');
    } catch (err) {
      console.error('Failed to create link:', err);
      error = err.message || 'Failed to create link';
    } finally {
      submitting = false;
    }
  }

  function buildExternalUrl(repo, type, id) {
    const baseUrl = repo.repository_url.replace(/\.git$/, '');

    switch (type) {
      case 'pull_request':
        // GitHub/GitLab/Gitea pattern
        if (repo.provider_type === 'github' || repo.provider_type === 'gitea') {
          return `${baseUrl}/pull/${id}`;
        } else if (repo.provider_type === 'gitlab') {
          return `${baseUrl}/-/merge_requests/${id}`;
        } else if (repo.provider_type === 'bitbucket') {
          return `${baseUrl}/pull-requests/${id}`;
        }
        return `${baseUrl}/pull/${id}`;

      case 'branch':
        if (repo.provider_type === 'github' || repo.provider_type === 'gitea') {
          return `${baseUrl}/tree/${id}`;
        } else if (repo.provider_type === 'gitlab') {
          return `${baseUrl}/-/tree/${id}`;
        } else if (repo.provider_type === 'bitbucket') {
          return `${baseUrl}/branch/${id}`;
        }
        return `${baseUrl}/tree/${id}`;

      case 'commit':
        if (repo.provider_type === 'github' || repo.provider_type === 'gitea') {
          return `${baseUrl}/commit/${id}`;
        } else if (repo.provider_type === 'gitlab') {
          return `${baseUrl}/-/commit/${id}`;
        } else if (repo.provider_type === 'bitbucket') {
          return `${baseUrl}/commits/${id}`;
        }
        return `${baseUrl}/commit/${id}`;

      default:
        return baseUrl;
    }
  }

  function close() {
    dispatch('close');
  }

  function getPlaceholder(type) {
    switch (type) {
      case 'pull_request': return 'e.g., 123';
      case 'branch': return 'e.g., feature/PROJ-123-add-login';
      case 'commit': return 'e.g., abc1234 or full SHA';
      default: return '';
    }
  }

  function getIdLabel(type) {
    switch (type) {
      case 'pull_request': return 'PR Number';
      case 'branch': return 'Branch Name';
      case 'commit': return 'Commit SHA';
      default: return 'ID';
    }
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
  aria-labelledby="add-scm-link-title"
>
  <div
    class="w-full max-w-md rounded-xl shadow-xl border overflow-hidden"
    style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
  >
    <!-- Header -->
    <div class="flex items-center justify-between px-6 py-4 border-b" style="border-color: var(--ds-border);">
      <div>
        <h2 id="add-scm-link-title" class="text-lg font-semibold" style="color: var(--ds-text);">
          Link Development Resource
        </h2>
        <p class="text-sm" style="color: var(--ds-text-subtle);">
          Connect a PR, branch, or commit to this item
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

    <!-- Content -->
    <div class="px-6 py-4 space-y-4">
      {#if loading}
        <div class="flex items-center justify-center py-8">
          <Loader2 class="w-6 h-6 animate-spin" style="color: var(--ds-text-subtle);" />
        </div>
      {:else if repositories.length === 0}
        <EmptyState
          icon={GitMerge}
          title="No repositories linked to this workspace"
          description="Link repositories in Workspace Settings → Source Control"
        />
      {:else}
        <!-- Repository Selection -->
        <div>
          <Label color="default" required class="mb-1.5">Repository</Label>
          <BasePicker
            bind:value={selectedRepoId}
            items={repositories}
            placeholder="Select a repository..."
            showUnassigned={true}
            unassignedLabel="Select a repository..."
            getValue={(repo) => repo.id}
            getLabel={(repo) => `${repo.repository_name} (${repo.provider_name})`}
          />
        </div>

        <!-- Link Type -->
        <div>
          <Label color="default" required class="mb-1.5">Type</Label>
          <div class="flex gap-2">
            <button
              class="flex-1 flex items-center justify-center gap-2 px-3 py-2 rounded-lg border text-sm transition-colors"
              class:ring-2={linkType === 'pull_request'}
              style="
                border-color: {linkType === 'pull_request' ? 'var(--ds-interactive)' : 'var(--ds-border)'};
                background-color: {linkType === 'pull_request' ? 'var(--ds-background-selected)' : 'var(--ds-surface)'};
                color: var(--ds-text);
              "
              onclick={() => linkType = 'pull_request'}
            >
              <GitMerge class="w-4 h-4" />
              PR
            </button>
            <button
              class="flex-1 flex items-center justify-center gap-2 px-3 py-2 rounded-lg border text-sm transition-colors"
              class:ring-2={linkType === 'branch'}
              style="
                border-color: {linkType === 'branch' ? 'var(--ds-interactive)' : 'var(--ds-border)'};
                background-color: {linkType === 'branch' ? 'var(--ds-background-selected)' : 'var(--ds-surface)'};
                color: var(--ds-text);
              "
              onclick={() => linkType = 'branch'}
            >
              <GitBranch class="w-4 h-4" />
              Branch
            </button>
            <button
              class="flex-1 flex items-center justify-center gap-2 px-3 py-2 rounded-lg border text-sm transition-colors"
              class:ring-2={linkType === 'commit'}
              style="
                border-color: {linkType === 'commit' ? 'var(--ds-interactive)' : 'var(--ds-border)'};
                background-color: {linkType === 'commit' ? 'var(--ds-background-selected)' : 'var(--ds-surface)'};
                color: var(--ds-text);
              "
              onclick={() => linkType = 'commit'}
            >
              <GitCommit class="w-4 h-4" />
              Commit
            </button>
          </div>
        </div>

        <!-- External ID -->
        <div>
          <Label color="default" required class="mb-1.5">{getIdLabel(linkType)}</Label>
          <input
            type="text"
            bind:value={externalId}
            placeholder={getPlaceholder(linkType)}
            class="w-full px-3 py-2 rounded-lg border text-sm"
            style="border-color: var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
          />
        </div>

        <!-- Title (optional) -->
        <div>
          <Label color="default" class="mb-1.5">Title (optional)</Label>
          <input
            type="text"
            bind:value={title}
            placeholder="e.g., Add user authentication"
            class="w-full px-3 py-2 rounded-lg border text-sm"
            style="border-color: var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
          />
        </div>

        <!-- Error -->
        {#if error}
          <p class="text-sm" style="color: var(--ds-text-danger);">{error}</p>
        {/if}
      {/if}
    </div>

    <!-- Footer -->
    <DialogFooter
      onCancel={close}
      onConfirm={submit}
      confirmLabel="Link Resource"
      loading={submitting}
      loadingLabel="Linking..."
      disabled={loading || repositories.length === 0 || !selectedRepoId || !externalId}
    />
  </div>
</div>
