import { confirm } from '../composables/useConfirm.js';
import { t } from '../stores/i18n.svelte.js';
import { errorToast } from '../stores/toasts.svelte.js';

/**
 * Run a bulk transition, and when the server refuses the whole batch because
 * some items need a transition screen (409 screen_required), offer to retry
 * without them. A bulk caller cannot collect screen field values, so the server
 * refuses the batch rather than partially applying it.
 *
 * @param {(ids: number[], toStatusId: number) => Promise<any>} transition
 * @param {number[]} ids
 * @param {number} toStatusId
 * @returns {Promise<any | null>} the transition result, or null when nothing moved.
 */
export async function runBulkTransitionWithScreenRetry(transition, ids, toStatusId) {
  try {
    return await transition(ids, toStatusId);
  } catch (error) {
    if (error?.status !== 409 || error?.code !== 'screen_required') {
      throw error;
    }
    const blocked = Array.isArray(error?.details?.items) ? error.details.items : [];
    const blockedIds = new Set(blocked.map((item) => Number(item?.item_id)));
    const remaining = ids.filter((id) => !blockedIds.has(Number(id)));
    if (remaining.length === 0) {
      errorToast(t('bulkActions.screenRequiredAll'));
      return null;
    }
    const proceed = await confirm({
      title: t('bulkActions.screenRequiredTitle'),
      message: t('bulkActions.screenRequiredMessage', {
        blocked: blocked.length,
        total: ids.length,
        remaining: remaining.length,
      }),
      confirmText: t('bulkActions.screenRequiredConfirm'),
      variant: 'warning',
    });
    if (!proceed) return null;
    return transition(remaining, toStatusId);
  }
}
