<script>
  import { createEventDispatcher } from 'svelte';
  import { api } from '../api.js';
  import Button from '../components/Button.svelte';
  import Label from '../components/Label.svelte';
  import DialogFooter from './DialogFooter.svelte';
  import { X, GitMerge, Loader2 } from 'lucide-svelte';
  import { successToast, errorToast } from '../stores/toasts.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import { portal } from '../actions/portal.js';

  export let branchLink;
  export let itemKey = '';
  export let itemTitle = '';

  const dispatch = createEventDispatcher();

  let submitting = false;
  let error = null;

  // Form state
  let prTitle = itemKey ? `${itemKey}: ${itemTitle}` : '';
  let prBody = itemKey ? `Linked to ${itemKey}` : '';
  let baseBranch = '';

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
      dispatch('created', result);
    } catch (err) {
      console.error('Failed to create PR:', err);
      error = err.message || t('scm.failedToCreatePR');
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
  use:portal
  class="fixed inset-0 flex items-center justify-center p-4 z-50"
  style="background-color: rgba(0, 0, 0, 0.3); backdrop-filter: blur(2px);"
  onclick={(e) => e.target === e.currentTarget && close()}
  onkeypress={(e) => e.key === 'Escape' && close()}
  role="dialog"
  aria-modal="true"
  tabindex="-1"
  aria-labelledby="create-pr-title"
>
  <div
    class="w-full max-w-md rounded-xl shadow-xl border overflow-hidden"
    style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
  >
    <!-- Header -->
    <div class="flex items-center justify-between px-6 py-4 border-b" style="border-color: var(--ds-border);">
      <div>
        <h2 id="create-pr-title" class="text-lg font-semibold" style="color: var(--ds-text);">
          {t('scm.createPullRequest')}
        </h2>
        <p class="text-sm" style="color: var(--ds-text-subtle);">
          {t('scm.createPRFrom', { branch: '' })} <span class="font-mono">{branchLink?.external_id || branchLink?.title || 'branch'}</span>
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
        <p class="text-xs mt-1" style="color: var(--ds-text-subtlest);">
          {t('scm.baseBranchPRHelp')}
        </p>
      </div>

      <!-- Error -->
      {#if error}
        <p class="text-sm" style="color: var(--ds-text-danger);">{error}</p>
      {/if}
    </div>

    <!-- Footer -->
    <DialogFooter
      onCancel={close}
      onConfirm={submit}
      confirmLabel={t('scm.createPR')}
      loading={submitting}
      loadingLabel={t('scm.creating')}
    />
  </div>
</div>
