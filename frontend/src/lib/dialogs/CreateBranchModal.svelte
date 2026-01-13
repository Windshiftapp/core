<script>
  import { createEventDispatcher, onMount } from 'svelte';
  import { api } from '../api.js';
  import Button from '../components/Button.svelte';
  import Label from '../components/Label.svelte';
  import BasePicker from '../pickers/BasePicker.svelte';
  import DialogFooter from './DialogFooter.svelte';
  import { X, GitBranch, Loader2 } from 'lucide-svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import { successToast, errorToast } from '../stores/toasts.svelte.js';

  export let itemId;
  export let itemKey = '';
  export let itemTitle = '';

  const dispatch = createEventDispatcher();

  let loading = true;
  let submitting = false;
  let repositories = [];
  let error = null;

  // Form state
  let selectedRepoId = null;
  let branchName = '';
  let baseBranch = '';

  $: selectedRepo = repositories.find(r => r.id === selectedRepoId);

  // Generate default branch name when item key changes or repo is selected
  $: if (itemKey && !branchName) {
    branchName = generateBranchName(itemKey, itemTitle);
  }

  // Set default base branch when repo changes
  $: if (selectedRepo && !baseBranch) {
    baseBranch = selectedRepo.default_branch || 'main';
  }

  onMount(async () => {
    await loadRepositories();
  });

  function generateBranchName(key, title) {
    const slug = title
      .toLowerCase()
      .replace(/[^a-z0-9\s-]/g, '')
      .replace(/\s+/g, '-')
      .substring(0, 50);
    return `feature/${key.toLowerCase()}-${slug}`;
  }

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
    if (!selectedRepoId || !branchName) {
      error = 'Please fill in all required fields';
      return;
    }

    submitting = true;
    error = null;

    try {
      const data = {
        workspace_repository_id: selectedRepoId,
        branch_name: branchName.trim(),
        base_branch: baseBranch.trim() || undefined,
      };

      const result = await api.itemSCMLinks.createBranch(itemId, data);
      successToast(`Branch created successfully`);
      dispatch('created', result);
    } catch (err) {
      console.error('Failed to create branch:', err);
      error = err.message || 'Failed to create branch';
      errorToast(error);
    } finally {
      submitting = false;
    }
  }

  function close() {
    dispatch('close');
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
  aria-labelledby="create-branch-title"
>
  <div
    class="w-full max-w-md rounded-xl shadow-xl border overflow-hidden"
    style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
  >
    <!-- Header -->
    <div class="flex items-center justify-between px-6 py-4 border-b" style="border-color: var(--ds-border);">
      <div>
        <h2 id="create-branch-title" class="text-lg font-semibold" style="color: var(--ds-text);">
          Create Branch
        </h2>
        <p class="text-sm" style="color: var(--ds-text-subtle);">
          Create a new branch for {itemKey || 'this item'}
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
          icon={GitBranch}
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

        <!-- Branch Name -->
        <div>
          <Label color="default" required class="mb-1.5">Branch Name</Label>
          <div class="flex items-center gap-2">
            <GitBranch class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
            <input
              type="text"
              bind:value={branchName}
              placeholder="feature/PROJ-123-add-login"
              class="flex-1 px-3 py-2 rounded-lg border text-sm font-mono"
              style="border-color: var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
            />
          </div>
        </div>

        <!-- Base Branch -->
        <div>
          <Label color="default" class="mb-1.5">Base Branch</Label>
          <input
            type="text"
            bind:value={baseBranch}
            placeholder={selectedRepo?.default_branch || 'main'}
            class="w-full px-3 py-2 rounded-lg border text-sm font-mono"
            style="border-color: var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
          />
          <p class="text-xs mt-1" style="color: var(--ds-text-subtlest);">
            The branch to create from. Defaults to the repository's default branch.
          </p>
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
      confirmLabel="Create Branch"
      loading={submitting}
      loadingLabel="Creating..."
      disabled={loading || repositories.length === 0 || !selectedRepoId || !branchName}
    />
  </div>
</div>
