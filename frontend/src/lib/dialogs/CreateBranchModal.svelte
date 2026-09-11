<script>
  import { api } from '../api.js';
  import Input from '../components/Input.svelte';
  import Label from '../components/Label.svelte';
  import { GitBranch } from '@lucide/svelte';
  import { successToast, errorToast } from '../stores/toasts.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import DescriptionText from '../components/DescriptionText.svelte';
  import SCMRepositoryModal from './SCMRepositoryModal.svelte';

  let { itemId, itemKey = '', itemTitle = '', oncreated, onclose } = $props();

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

  function generateBranchName(key, title) {
    const slug = title
      .toLowerCase()
      .replace(/[^a-z0-9\s-]/g, '')
      .replace(/\s+/g, '-')
      .substring(0, 50);
    return `feature/${key.toLowerCase()}-${slug}`;
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

<SCMRepositoryModal
  {itemId}
  title={t('scm.createBranch')}
  subtitle={t('scm.createBranchFor', { itemKey: itemKey || 'this item' })}
  emptyIcon={GitBranch}
  bind:repositories
  bind:selectedRepoId
  bind:error
  {submitting}
  confirmLabel={t('scm.createBranch')}
  loadingLabel={t('scm.creating')}
  confirmDisabled={!branchName}
  onclose={close}
  onsubmit={submit}
>
  {#snippet fields()}
        <!-- Branch Name -->
        <div>
          <Label color="default" required class="mb-1.5">{t('scm.branchName')}</Label>
          <div class="flex items-center gap-2">
            <GitBranch class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
            <Input
              type="text"
              bind:value={branchName}
              placeholder="feature/PROJ-123-add-login"
              class="flex-1 font-mono"
              size="small"
            />
          </div>
        </div>

        <!-- Base Branch -->
        <div>
          <Label color="default" class="mb-1.5">{t('scm.baseBranch')}</Label>
          <Input
            type="text"
            bind:value={baseBranch}
            placeholder={selectedRepo?.default_branch || 'main'}
            class="font-mono"
            size="small"
          />
          <DescriptionText variant="subtlest">
            {t('scm.baseBranchHelp')}
          </DescriptionText>
        </div>

  {/snippet}
</SCMRepositoryModal>
