<script>
  import Button from '../../components/Button.svelte';
  import Card from '../../components/Card.svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast } from '../../stores/toasts.svelte.js';
  import { timeLogStore } from '../../stores/timeLogStore.svelte.js';

  let { class: className = '' } = $props();

  function formatDuration(minutes) {
    if (!minutes) return '';
    const hours = Math.floor(minutes / 60);
    const mins = minutes % 60;
    if (hours === 0) return `${mins}m`;
    if (mins === 0) return `${hours}h`;
    return `${hours}h ${mins}m`;
  }

  function draftLabel(draft) {
    return draft.item_title || draft.description || draft.project_name || t('time.drafts');
  }

  async function complete(draft) {
    try {
      await timeLogStore.openDraft(draft.id);
    } catch (error) {
      console.error('Failed to open draft:', error);
      errorToast(t('time.bookFailed'));
    }
  }

  async function discard(draft) {
    try {
      await timeLogStore.removeDraft(draft.id);
    } catch (error) {
      console.error('Failed to discard draft:', error);
      errorToast(t('time.discardFailed'));
    }
  }
</script>

<Card rounded="xl" shadow padding="spacious" class="mb-6 {className}" dataTestid="time-drafts-card">
  {#if timeLogStore.drafts.length === 0}
    <EmptyState
      title={t('time.draftsEmptyTitle')}
      description={t('time.draftsEmptyHint')}
    />
  {:else}
    <ul class="space-y-2">
      {#each timeLogStore.drafts as draft (draft.id)}
        <li
          class="flex items-center gap-3 py-2 border-b border-ds-border last:border-b-0"
          data-testid={`time-draft-${draft.id}`}
        >
          <div class="flex-1 min-w-0">
            <div class="text-sm font-medium truncate text-ds-text">{draftLabel(draft)}</div>
            <div class="text-xs text-ds-text-subtle">
              {#if draft.project_name}{draft.project_name} · {/if}
              {formatDuration(draft.duration_minutes)}
            </div>
          </div>
          <Button size="small" variant="primary" onclick={() => complete(draft)} dataTestid={`time-draft-complete-${draft.id}`}>
            {t('time.completeTimeEntry')}
          </Button>
          <Button size="small" variant="ghost" onclick={() => discard(draft)} dataTestid={`time-draft-discard-${draft.id}`}>
            {t('common.discard')}
          </Button>
        </li>
      {/each}
    </ul>
  {/if}
</Card>
