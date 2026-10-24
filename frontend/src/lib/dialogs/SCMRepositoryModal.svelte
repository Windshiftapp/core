<script>
  import { onMount } from 'svelte';
  import StateDisplay from '../components/StateDisplay.svelte';
  import { itemSCMLinks } from '../api/scm.js';
  import EmptyState from '../components/EmptyState.svelte';
  import Label from '../components/Label.svelte';
  import BasePicker from '../pickers/BasePicker.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import DialogFooter from './DialogFooter.svelte';
  import Modal from './Modal.svelte';
  import ModalHeader from './ModalHeader.svelte';

  let {
    itemId,
    title,
    subtitle = '',
    emptyIcon,
    repositories = $bindable([]),
    selectedRepoId = $bindable(null),
    error = $bindable(null),
    submitting = false,
    confirmLabel,
    loadingLabel,
    confirmDisabled = false,
    onclose,
    onsubmit,
    fields,
  } = $props();

  let loading = $state(true);

  onMount(async () => {
    loading = true;
    error = null;
    try {
      repositories = (await itemSCMLinks.getRepositories(itemId)) || [];
      if (repositories.length === 1) selectedRepoId = repositories[0].id;
    } catch (loadError) {
      console.error('Failed to load repositories:', loadError);
      repositories = [];
      error = t('scm.failedToLoadRepos');
    } finally {
      loading = false;
    }
  });
</script>

<Modal
  isOpen={true}
  maxWidth="max-w-md"
  {onclose}
  onSubmit={onsubmit}
  submitDisabled={loading || repositories.length === 0 || !selectedRepoId || confirmDisabled}
>
  <ModalHeader {title} {subtitle} onClose={onclose} />

  <div class="px-6 py-4 space-y-4">
    {#if loading}
      <StateDisplay type="loading" />
    {:else if repositories.length === 0}
      <EmptyState
        icon={emptyIcon}
        title={t('scm.noReposLinked')}
        description={t('scm.linkReposHelp')}
      />
    {:else}
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

      {@render fields()}

      {#if error}
        <p class="text-sm" style="color: var(--ds-text-danger);">{error}</p>
      {/if}
    {/if}
  </div>

  <DialogFooter
    onCancel={onclose}
    onConfirm={onsubmit}
    {confirmLabel}
    loading={submitting}
    {loadingLabel}
    disabled={loading || repositories.length === 0 || !selectedRepoId || confirmDisabled}
    showKeyboardHint={true}
  />
</Modal>
