import { stopTimerAndReview } from '../../features/time/stopTimerAndReview.js';
import { timeLogStore } from '../../stores/timeLogStore.svelte.js';
import { timerStore } from '../../stores/timerStore.svelte.js';
import { errorToast, infoToast, warningToast } from '../../stores/toasts.svelte.js';
import { BUCKET } from '../buckets.js';
import { createCommand } from '../types.js';

/** Time navigation and actions using canonical worklogs and projects routes. */
export function timeProvider(ctx) {
  const { t, activeTimer } = ctx;

  const out = [
    createCommand({
      id: 'time-tracking',
      label: t('commandPalette.commands.timeTracking.label'),
      description: t('commandPalette.commands.timeTracking.description'),
      bucket: BUCKET.GLOBAL_NAVIGATION,
      keywords: ['time', 'tracking', 'hours', 'work', 'log', 'timesheet'],
      url: '/time',
    }),
    createCommand({
      id: 'time-worklogs',
      label: t('commandPalette.commands.timeReports.label'),
      description: t('commandPalette.commands.timeReports.description'),
      bucket: BUCKET.GLOBAL_NAVIGATION,
      keywords: ['time', 'reports', 'worklogs', 'analytics', 'hours'],
      url: '/time/worklogs',
    }),
    createCommand({
      id: 'time-projects',
      label: t('commandPalette.commands.timeProjects.label'),
      description: t('commandPalette.commands.timeProjects.description'),
      bucket: BUCKET.GLOBAL_NAVIGATION,
      keywords: ['time', 'projects', 'clients', 'billing'],
      url: '/time/projects',
    }),
    createCommand({
      id: 'log-time',
      label: t('commandPalette.commands.logTime.label'),
      description: t('commandPalette.commands.logTime.description'),
      bucket: BUCKET.MODULE_ACTIONS,
      keywords: ['log', 'time', 'entry', 'hours', 'work', 'track', 'add'],
      execute: () => {
        timeLogStore.open();
      },
    }),
  ];

  if (!activeTimer) {
    out.push(
      createCommand({
        id: 'start-timer',
        label: t('commandPalette.commands.startTimer.label'),
        description: t('commandPalette.commands.startTimer.description'),
        bucket: BUCKET.MODULE_ACTIONS,
        keywords: ['start', 'timer', 'track', 'time', 'begin'],
        execute: () => {
          if (timerStore.canStart) {
            infoToast(t('dialogs.alerts.startTimerFromItem'));
          } else if (timerStore.activeTimer) {
            warningToast(t('dialogs.alerts.timerAlreadyRunning'));
          }
        },
      })
    );
  } else {
    out.push(
      createCommand({
        id: 'stop-timer',
        label: t('commandPalette.commands.stopTimer.label'),
        description: t('commandPalette.commands.stopTimer.description'),
        bucket: BUCKET.MODULE_ACTIONS,
        keywords: ['stop', 'timer', 'end', 'finish', 'complete'],
        execute: async () => {
          if (timerStore.canStop) {
            try {
              await stopTimerAndReview(timerStore, timeLogStore);
            } catch (err) {
              errorToast(t('dialogs.alerts.stopTimerFailed', { error: err.message }));
            }
          } else if (timerStore.syncing) {
            infoToast(t('dialogs.alerts.timerSyncing'));
          } else {
            infoToast(t('dialogs.alerts.noTimerRunning'));
          }
        },
      })
    );
  }

  return out;
}
