<script>
  import { FileText, Package, Trash2 } from '@lucide/svelte';
  import Spinner from '../components/Spinner.svelte';
  import Panel from '../components/Panel.svelte';
  import { portalStore, iconMap } from '../stores/portal.svelte.js';
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
    await portalStore.deleteDraft(draft.request_type_id);
  }
</script>

<div class="space-y-6">
  <div>
    <h2 class="text-2xl font-semibold" style="color: var(--ds-text);">
      {t('portal.draftsTitle')}
    </h2>
    <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
      {t('portal.draftsSubtitle')}
    </p>
  </div>

  {#if portalStore.loadingDrafts}
    <div class="flex justify-center py-12">
      <Spinner size="lg" />
    </div>
  {:else if portalStore.myDrafts.length === 0}
    <Panel padding="spacious">
      <div class="text-center py-8">
        <div class="w-16 h-16 rounded-full mx-auto mb-4 flex items-center justify-center" style="background-color: var(--ds-background-neutral);">
          <FileText class="w-8 h-8" style="color: var(--ds-text-subtle);" />
        </div>
        <p class="text-sm" style="color: var(--ds-text-subtle);">
          {t('portal.draftsEmpty')}
        </p>
      </div>
    </Panel>
  {:else}
    <div class="space-y-4">
      {#each portalStore.myDrafts as draft (draft.id)}
        {@const Icon = iconMap[draft.request_type_icon] || Package}
        <div
          class="w-full p-4 rounded text-left transition-all hover:shadow-md flex items-start gap-4"
          style="background-color: var(--ds-surface-card); border: 1px solid var(--ds-border);"
        >
          <button
            type="button"
            onclick={() => resume(draft)}
            class="flex-1 flex items-start gap-4 text-left cursor-pointer"
            data-testid="portal-draft-row"
          >
            <div class="w-10 h-10 rounded-full flex items-center justify-center flex-shrink-0" style="background-color: var(--ds-background-neutral);">
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
