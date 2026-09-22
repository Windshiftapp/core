import { aiStore } from '../../stores';
import { BUCKET } from '../buckets.js';
import { createCommand } from '../types.js';

/**
 * Mobile shell destinations. Only surfaces that exist under /m are offered —
 * desktop-only modules (boards, admin, time reports) have no phone surface
 * yet and would strand the user in desktop chrome.
 */
export function mobileNavigationProvider({ t }) {
  const out = [
    createCommand({
      id: 'm-my-work',
      label: t('mobile.myWork.title'),
      description: t('mobile.nav.myWorkDescription'),
      bucket: BUCKET.GLOBAL_NAVIGATION,
      keywords: ['my work', 'home', 'items', 'assigned', 'start'],
      url: '/m',
    }),
    createCommand({
      id: 'm-personal',
      label: t('workspaces.personal'),
      description: t('personal.personalTasks'),
      bucket: BUCKET.GLOBAL_NAVIGATION,
      keywords: ['personal', 'tasks', 'todo'],
      url: '/m/personal',
    }),
    createCommand({
      id: 'm-pages',
      label: t('pages.treeHeading'),
      description: t('mobile.nav.pagesDescription'),
      bucket: BUCKET.GLOBAL_NAVIGATION,
      keywords: ['pages', 'wiki', 'knowledge', 'docs', 'notes'],
      url: '/m/pages',
    }),
    createCommand({
      id: 'm-timer',
      label: t('time.pomodoro.timer'),
      description: t('mobile.nav.timerDescription'),
      bucket: BUCKET.GLOBAL_NAVIGATION,
      keywords: ['timer', 'time', 'tracking', 'worklog'],
      url: '/m/timer',
    }),
    createCommand({
      id: 'm-notifications',
      label: t('mobile.nav.alerts'),
      description: t('notifications.title'),
      bucket: BUCKET.GLOBAL_NAVIGATION,
      keywords: ['alerts', 'notifications', 'inbox', 'unread'],
      url: '/m/notifications',
    }),
    createCommand({
      id: 'm-search',
      label: t('common.search'),
      description: t('mobile.nav.searchDescription'),
      bucket: BUCKET.GLOBAL_NAVIGATION,
      keywords: ['search', 'find', 'look'],
      url: '/m/search',
    }),
  ];

  if (aiStore.chatAvailable) {
    out.push(
      createCommand({
        id: 'm-chat',
        label: t('mobile.chat.title'),
        description: t('mobile.nav.chatDescription'),
        bucket: BUCKET.GLOBAL_NAVIGATION,
        keywords: ['assistant', 'ai', 'chat', 'ask'],
        url: '/m/chat',
      })
    );
  }

  return out;
}
