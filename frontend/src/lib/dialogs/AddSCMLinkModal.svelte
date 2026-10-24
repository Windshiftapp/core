<script>
  import { itemSCMLinks } from '../api/scm.js';
  import Label from '../components/Label.svelte';
  import GitMerge from '@lucide/svelte/icons/git-merge';
  import GitBranch from '@lucide/svelte/icons/git-branch';
  import GitCommit from '@lucide/svelte/icons/git-commit';
  import { t } from '../stores/i18n.svelte.js';
  import SCMRepositoryModal from './SCMRepositoryModal.svelte';
	import TextField from '../components/TextField.svelte';

  let { itemId, oncreated, onclose } = $props();

  let submitting = $state(false);
  let repositories = $state([]);
  let error = $state(null);

  // Form state
  let selectedRepoId = $state(null);
  let linkType = $state('pull_request');
  let externalId = $state('');
  let title = $state('');
  let externalUrl = $state('');

  async function submit() {
    if (!selectedRepoId || !linkType || !externalId) {
      error = t('scm.fillAllRequired');
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

      await itemSCMLinks.create(itemId, data);
      oncreated?.();
    } catch (err) {
      console.error('Failed to create link:', err);
      error = err.message || t('scm.failedToCreateLink');
    } finally {
      submitting = false;
    }
  }

  function buildExternalUrl(repo, type, id) {
    const baseUrl = repo.repository_url.replace(/\.git$/, '');

    switch (type) {
      case 'pull_request':
        // GitHub/GitLab/Gitea pattern
        if (repo.provider_type === 'github') {
          return `${baseUrl}/pull/${id}`;
        } else if (repo.provider_type === 'gitea') {
          return `${baseUrl}/pulls/${id}`;
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
    onclose?.();
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
      case 'pull_request': return t('scm.prNumber');
      case 'branch': return t('scm.branchName');
      case 'commit': return t('scm.commitSha');
      default: return 'ID';
    }
  }
</script>

<SCMRepositoryModal
  {itemId}
  title={t('scm.linkDevResource')}
  subtitle={t('scm.linkDevResourceDesc')}
  emptyIcon={GitMerge}
  bind:repositories
  bind:selectedRepoId
  bind:error
  {submitting}
  confirmLabel={t('scm.linkResource')}
  loadingLabel={t('scm.linking')}
  confirmDisabled={!externalId}
  onclose={close}
  onsubmit={submit}
>
  {#snippet fields()}
        <!-- Link Type -->
        <div>
          <Label color="default" required class="mb-1.5">{t('scm.type')}</Label>
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
              {t('scm.pr')}
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
              {t('scm.branch')}
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
              {t('scm.commit')}
            </button>
          </div>
        </div>

        <!-- External ID -->
        <div>
          <TextField
            label={getIdLabel(linkType)}
            required
            labelColor="default"
            type="text"
            placeholder={getPlaceholder(linkType)}
            size="small"
            bind:value={externalId}
          />
        </div>

        <!-- Title (optional) -->
        <div>
          <TextField
            label={t('scm.titleOptional')}
            labelColor="default"
            type="text"
            placeholder="e.g., Add user authentication"
            size="small"
            bind:value={title}
          />
        </div>

  {/snippet}
</SCMRepositoryModal>
