import { t } from '../../stores/i18n.svelte.js';
import { successToast } from '../../stores/toasts.svelte.js';

/**
 * Stop the active timer and route the result: a complete entry is booked
 * silently, an incomplete one is persisted as a draft and opened in the shared
 * time-log modal for completion. Shared by the floating timer, the palette
 * stop command, and the mobile timer view.
 */
export async function stopTimerAndReview(timerStore, timeLogStore) {
  const result = await timerStore.stop();
  if (!result) return null;
  if (result.booked) {
    successToast(t('time.timeBooked'));
  } else if (result.draft_id) {
    await timeLogStore.openDraft(result.draft_id);
  }
  return result;
}
