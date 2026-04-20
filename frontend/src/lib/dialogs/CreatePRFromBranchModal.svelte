<script>
  import { api } from '../api.js';
  import Label from '../components/Label.svelte';
  import Modal from './Modal.svelte';
  import ModalHeader from './ModalHeader.svelte';
  import DialogFooter from './DialogFooter.svelte';
  import { successToast, errorToast } from '../stores/toasts.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import DescriptionText from '../components/DescriptionText.svelte';

  let { branchLink, itemKey = '', itemTitle = '', oncreated, onclose } = $props();

  let submitting = $state(false);
  let error = $state(null);

  // Form state
  let prTitle = $state(itemKey ? `${itemKey}: ${itemTitle}` : '');
  let prBody = $state(itemKey ? `Linked to ${itemKey}` : '');
  let baseBranch = $state('');

  async function submit() {
    if (!branchLink?.id) {
      error = t('scm.noBranchLink');
      return;
    }

    submitting = true;
    error = null;

    try {
      const data = {
        pr_title: prTitle.trim() || undefined,
        pr_body: prBody.trim() || undefined,
        base_branch: baseBranch.trim() || undefined,
      };

      const result = await api.itemSCMLinks.createPRFromBranch(branchLink.id, data);
      successToast(t('scm.prCreatedSuccess', { prNumber: result.pr_number }));
      oncreated?.(result);
    } catch (err) {
      console.error('Failed to create PR:', err);
      error = err.message || t('scm.failedToCreatePR');
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
    title={t('scm.createPullRequest')}
    subtitle={`${t('scm.createPRFrom', { branch: '' })} ${branchLink?.external_id || branchLink?.title || 'branch'}`}
    onClose={close}
  />

    <!-- Content -->
    <div class="px-6 py-4 space-y-4">
      <!-- PR Title -->
      <div>
        <Label color="default" class="mb-1.5">{t('scm.prTitle')}</Label>
        <input
          type="text"
          bind:value={prTitle}
          placeholder={itemKey ? `${itemKey}: ${itemTitle}` : 'Pull request title'}
          class="w-full px-3 py-2 rounded-lg border text-sm"
          style="border-color: var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
        />
      </div>

      <!-- PR Body -->
      <div>
        <Label color="default" class="mb-1.5">{t('scm.description')}</Label>
        <textarea
          bind:value={prBody}
          placeholder={itemKey ? `Linked to ${itemKey}` : 'Pull request description'}
          rows="3"
          class="w-full px-3 py-2 rounded-lg border text-sm resize-none"
          style="border-color: var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
        ></textarea>
      </div>

      <!-- Base Branch -->
      <div>
        <Label color="default" class="mb-1.5">{t('scm.baseBranchPR')}</Label>
        <input
          type="text"
          bind:value={baseBranch}
          placeholder="main"
          class="w-full px-3 py-2 rounded-lg border text-sm font-mono"
          style="border-color: var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
        />
        <DescriptionText variant="subtlest">
          {t('scm.baseBranchPRHelp')}
        </DescriptionText>
      </div>

      <!-- Error -->
      {#if error}
        <p class="text-sm" style="color: var(--ds-text-danger);">{error}</p>
      {/if}
    </div>

  <DialogFooter
    onCancel={close}
    onConfirm={submit}
    confirmLabel={t('scm.createPR')}
    loading={submitting}
    loadingLabel={t('scm.creating')}
  />
</Modal>
