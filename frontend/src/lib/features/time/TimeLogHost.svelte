<script>
  import { time } from '../../api/time.js';
  import { errorToast, successToast } from '../../stores/toasts.svelte.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { timeLogStore } from '../../stores/timeLogStore.svelte.js';
  import TimeLogModal from '../../dialogs/TimeLogModal.svelte';

  const prefill = $derived(/** @type {any} */ (timeLogStore.prefill ?? {}));

  async function handleSave(event) {
    const data = event.detail;
    const draft = timeLogStore.editingDraft;
    try {
      if (draft) {
        await time.worklogDrafts.book(draft.id, { ...data, version: draft.version });
      } else if (timeLogStore.editingWorklog) {
        await time.worklogs.update(timeLogStore.editingWorklog.id, data);
      } else {
        await time.worklogs.create(data);
      }
      timeLogStore.markSaved();
      await timeLogStore.refreshDrafts();
      timeLogStore.close();
    } catch (error) {
      console.error('Failed to save time entry:', error);
      errorToast(draft ? t('time.bookFailed') : t('time.entry.failedToSave'));
    }
  }

  async function handleDiscard() {
    const draft = timeLogStore.editingDraft;
    if (!draft) return;
    try {
      await timeLogStore.removeDraft(draft.id);
      successToast(t('time.draftDiscarded'));
      timeLogStore.close();
    } catch (error) {
      console.error('Failed to discard draft:', error);
      errorToast(t('time.discardFailed'));
    }
  }
</script>

{#if timeLogStore.show}
  <TimeLogModal
    projects={timeLogStore.projects}
    customers={timeLogStore.customers}
    workItems={timeLogStore.workItems}
    workspaces={timeLogStore.workspaces}
    editingWorklog={timeLogStore.editingWorklog}
    draft={timeLogStore.editingDraft}
    defaultProjectId={prefill.defaultProjectId ?? null}
    defaultItemId={prefill.itemId ?? null}
    defaultDate={prefill.date ?? null}
    defaultStartTime={prefill.startTime ?? null}
    showProjectField={prefill.showProjectField ?? true}
    showWorkItemField={prefill.showWorkItemField ?? !prefill.itemId}
    allowProjectChange={prefill.allowProjectChange ?? true}
    onsave={handleSave}
    oncancel={() => timeLogStore.close()}
    ondiscard={handleDiscard}
  />
{/if}
