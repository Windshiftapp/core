<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import Label from '../components/Label.svelte';
  import BasePicker from '../pickers/BasePicker.svelte';
  import Modal from './Modal.svelte';
  import ModalHeader from './ModalHeader.svelte';
  import DialogFooter from './DialogFooter.svelte';
  import { GitBranch, Loader2 } from 'lucide-svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import { successToast, errorToast } from '../stores/toasts.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import DescriptionText from '../components/DescriptionText.svelte';

  let { itemId, itemKey = '', itemTitle = '', oncreated, onclose } = $props();

  let loading = $state(true);
  let submitting = $state(false);
  let repositories = $state([]);
  let error = $state(null);

  // Form state
  let selectedRepoId = $state(null);
  let branchName = $state('');
  let baseBranch = $state('');

  let selectedRepo = $derived(repositories.find(r => r.id === selectedRepoId));

  // Generate default branch name when item key changes or repo is selected
  $effect(() => {
    if (itemKey && !branchName) {
      branchName = generateBranchName(itemKey, itemTitle);
    }
  });

  // Set default base branch when repo changes
  $effect(() => {
    if (selectedRepo && !baseBranch) {
      baseBranch = selectedRepo.default_branch || 'main';
    }
  });

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
      error = t('scm.failedToLoadRepos');
      repositories = [];
    } finally {
      loading = false;
    }
  }

  async function submit() {
    if (!selectedRepoId || !branchName) {
      error = t('scm.fillAllRequired');
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
      successToast(t('scm.branchCreatedSuccess'));
      oncreated?.(result);
    } catch (err) {
      console.error('Failed to create branch:', err);
      error = err.message || t('scm.failedToCreateBranch');
      errorToast(error);
    } finally {
      submitting = false;
    }
  }

  function close() {
    onclose?.();
  }
</script>

<Modal isOpen={true} maxWidth="max-w-md" onclose={close}>
  <ModalHeader
    title={t('scm.createBranch')}
    subtitle={t('scm.createBranchFor', { itemKey: itemKey || 'this item' })}
    onClose={close}
  />

    <!-- Content -->
    <div class="px-6 py-4 space-y-4">
      {#if loading}
        <div class="flex items-center justify-center py-8">
          <Loader2 class="w-6 h-6 animate-spin" style="color: var(--ds-text-subtle);" />
        </div>
      {:else if repositories.length === 0}
        <EmptyState
          icon={GitBranch}
          title={t('scm.noReposLinked')}
          description={t('scm.linkReposHelp')}
        />
      {:else}
        <!-- Repository Selection -->
        <div>
          <Label color="default" required class="mb-1.5">{t('scm.repository')}</Label>
          <BasePicker
            bind:value={selectedRepoId}
            items={repositories}
            placeholder={t('scm.selectRepository')}
            showUnassigned={true}
            unassignedLabel={t('scm.selectRepository')}
            getValue={(repo) => repo.id}
            getLabel={(repo) => `${repo.repository_name} (${repo.provider_name})`}
          />
        </div>

        <!-- Branch Name -->
        <div>
          <Label color="default" required class="mb-1.5">{t('scm.branchName')}</Label>
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
          <Label color="default" class="mb-1.5">{t('scm.baseBranch')}</Label>
          <input
            type="text"
            bind:value={baseBranch}
            placeholder={selectedRepo?.default_branch || 'main'}
            class="w-full px-3 py-2 rounded-lg border text-sm font-mono"
            style="border-color: var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
          />
          <DescriptionText variant="subtlest">
            {t('scm.baseBranchHelp')}
          </DescriptionText>
        </div>

        <!-- Error -->
        {#if error}
          <p class="text-sm" style="color: var(--ds-text-danger);">{error}</p>
        {/if}
      {/if}
    </div>

  <DialogFooter
    onCancel={close}
    onConfirm={submit}
    confirmLabel={t('scm.createBranch')}
    loading={submitting}
    loadingLabel={t('scm.creating')}
    disabled={loading || repositories.length === 0 || !selectedRepoId || !branchName}
  />
</Modal>
