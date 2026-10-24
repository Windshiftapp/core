import { system } from '../../api/admin.js';
import { confirm } from '../../composables/useConfirm.js';
import { errorToast, infoToast } from '../../stores/toasts.svelte.js';
import { BUCKET } from '../buckets.js';
import { createCommand } from '../types.js';

export function systemProvider(ctx) {
  const { t } = ctx;
  return [
    createCommand({
      id: 'quit-app',
      label: t('commandPalette.commands.quitApp.label'),
      description: t('commandPalette.commands.quitApp.description'),
      bucket: BUCKET.SYSTEM,
      keywords: ['quit', 'exit', 'shutdown', 'close', 'stop'],
      execute: async () => {
        const ok = await confirm({
          title: t('common.confirm'),
          message: t('dialogs.confirmations.quitApplication'),
          confirmText: t('common.confirm'),
          cancelText: t('common.cancel'),
          variant: 'warning',
        });
        if (!ok) return;
        try {
          await system.shutdown();
          infoToast(t('dialogs.alerts.applicationShuttingDown'));
        } catch (err) {
          console.error('Failed to shutdown:', err);
          errorToast(t('dialogs.alerts.shutdownFailed'));
        }
      },
    }),
  ];
}
