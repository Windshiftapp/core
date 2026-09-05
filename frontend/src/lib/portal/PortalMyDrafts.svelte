<script>
  import { FileText, Package, Trash2 } from '@lucide/svelte';
  import Spinner from '../components/Spinner.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import { portalCatalogStore as portalStore } from '../stores/portal.svelte.js';
  import { portalDraftsStore } from '../stores/portalActivity.svelte.js';
  import { iconMap } from '../stores/portalPresentation.js';
  import { t } from '../stores/i18n.svelte.js';
  import { formatRelativeTime } from '../utils/dateFormatter.js';

  // onresume({ requestType }) is wired by Portal.svelte to open
  // RequestFormModal, which then auto-loads the draft and jumps to the saved
  // step via its own open-effect (api.portal.drafts.getForRequestType).
  let { onresume = () => {} } = $props();

  function resume(draft) {
    const matched = portalStore.requestTypes?.find((rt) => rt.id === draft.request_type_id) || {
      id: draft.request_type_id,
      name: draft.request_type_name,
      icon: draft.request_type_icon || 'FileText',
    };
    onresume({ requestType: matched });
  }

  async function deleteDraft(draft) {
    if (!window.confirm(t('portal.draftDeleteConfirm'))) return;
    await portalDraftsStore.delete(draft.request_type_id);
  }
</script>

<div>
  <PageHeader title={t('portal.draftsTitle')} subtitle={t('portal.draftsSubtitle')} />

  {#if portalDraftsStore.loading}
    <div class="flex justify-center py-12">
      <Spinner size="lg" />
    </div>
  {:else if portalDraftsStore.drafts.length === 0}
    <div class="max-w-xl mt-7 py-8 border-t" style="border-color: var(--ds-border);">
      <div class="flex items-start gap-3">
        <FileText class="w-5 h-5 mt-0.5" style="color: var(--ds-text-subtle);" />
        <div>
          <h2 class="text-base font-medium" style="color: var(--ds-text);">No drafts yet</h2>
          <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
            {t('portal.draftsEmpty')}
          </p>
        </div>
      </div>
    </div>
  {:else}
    <div class="mt-7 border rounded-md overflow-hidden" style="border-color: var(--ds-border); background-color: var(--ds-surface-card);">
      {#each portalDraftsStore.drafts as draft (draft.id)}
        {@const Icon = iconMap[draft.request_type_icon] || Package}
        <div
          class="w-full p-4 border-b last:border-b-0 text-left flex items-start gap-4"
          style="border-color: var(--ds-border);"
        >
          <button
            type="button"
            onclick={() => resume(draft)}
            class="flex-1 flex items-start gap-4 text-left cursor-pointer"
            data-testid="portal-draft-row"
          >
            <div class="w-9 h-9 rounded-md flex items-center justify-center flex-shrink-0" style="background-color: var(--ds-background-neutral);">
              <Icon class="w-5 h-5" style="color: var(--ds-text);" />
            </div>
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2 mb-1">
                <span class="text-xs uppercase tracking-wide font-medium" style="color: var(--ds-text-subtle);">
                  {draft.request_type_name}
                </span>
                <span
                  class="text-xs px-2 py-0.5 rounded-full"
                  style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);"
                >
                  {t('portal.draftStepProgress', { current: draft.current_step })}
                </span>
              </div>
              <h4 class="font-semibold mb-1 truncate" style="color: var(--ds-text);">
                {draft.title?.trim() || t('portal.draftUntitled')}
              </h4>
              <p class="text-xs" style="color: var(--ds-text-subtle);">
                {t('portal.draftUpdatedAgo', { time: formatRelativeTime(draft.updated_at) })}
              </p>
            </div>
          </button>
          <button
            type="button"
            onclick={() => deleteDraft(draft)}
            aria-label={t('portal.draftDelete')}
            title={t('portal.draftDelete')}
            class="p-2 rounded hover:bg-black/5 transition-colors flex-shrink-0"
            style="color: var(--ds-text-subtle);"
            data-testid="portal-draft-delete"
          >
            <Trash2 class="w-4 h-4" />
          </button>
        </div>
      {/each}
    </div>
  {/if}
</div>
