<script>
  import { api } from '../api.js';
  import Input from '../components/Input.svelte';
  import Label from '../components/Label.svelte';
  import Textarea from '../components/Textarea.svelte';
  import Modal from './Modal.svelte';
  import ModalHeader from './ModalHeader.svelte';
  import DialogFooter from './DialogFooter.svelte';
  import { successToast, errorToast } from '../stores/toasts.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import DescriptionText from '../components/DescriptionText.svelte';

  let { branchLink, itemKey = '', itemTitle = '', oncreated, onclose } = $props();

  let submitting = $state(false);
  let error = $state(null);

  // Form state (initialised from props; subsequent edits are user-driven)
  // svelte-ignore state_referenced_locally
  let prTitle = $state(itemKey ? `${itemKey}: ${itemTitle}` : '');
  // svelte-ignore state_referenced_locally
  let prBody = $state(itemKey ? `Linked to ${itemKey}` : '');
  let baseBranch = $state('');
  const isGitLab = $derived(branchLink?.provider_type === 'gitlab');

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
      successToast(t(isGitLab ? 'scm.mrCreatedSuccess' : 'scm.prCreatedSuccess', { prNumber: result.pr_number }));
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

<Modal
  isOpen={true}
  maxWidth="max-w-md"
  onSubmit={submit}
  submitDisabled={submitting}
  onclose={close}
>
  <ModalHeader
    title={t(isGitLab ? 'scm.createMergeRequest' : 'scm.createPullRequest')}
    subtitle={`${t('scm.createPRFrom', { branch: '' })} ${branchLink?.external_id || branchLink?.title || 'branch'}`}
    onClose={close}
  />

    <!-- Content -->
    <div class="px-6 py-4 space-y-4">
      <!-- PR Title -->
      <div>
        <Label color="default" class="mb-1.5">{t('scm.prTitle')}</Label>
        <Input
          type="text"
          bind:value={prTitle}
          placeholder={itemKey ? `${itemKey}: ${itemTitle}` : 'Pull request title'}
          size="small"
        />
      </div>

      <!-- PR Body -->
      <div>
        <Label color="default" class="mb-1.5">{t('scm.description')}</Label>
        <Textarea
          bind:value={prBody}
          placeholder={itemKey ? `Linked to ${itemKey}` : 'Pull request description'}
          rows={3}
          size="small"
        />
      </div>

      <!-- Base Branch -->
      <div>
        <Label color="default" class="mb-1.5">{t('scm.baseBranchPR')}</Label>
        <Input
          type="text"
          bind:value={baseBranch}
          placeholder="main"
          class="font-mono"
          size="small"
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
    confirmLabel={isGitLab ? 'Create MR' : t('scm.createPR')}
    loading={submitting}
    loadingLabel={t('scm.creating')}
    showKeyboardHint={true}
  />
</Modal>
