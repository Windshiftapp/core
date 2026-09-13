import { aiStore } from '../../stores';
import { BUCKET } from '../buckets.js';
import { createCommand } from '../types.js';

/**
 * Mobile shell destinations. Only surfaces that exist under /m are offered —
 * desktop-only modules (boards, admin, time reports) have no phone surface
 * yet and would strand the user in desktop chrome.
 */
export function mobileNavigationProvider(_ctx) {
  const out = [
    createCommand({
      id: 'm-my-work',
      label: 'My Work',
      description: 'Assigned, watched, recent',
      bucket: BUCKET.GLOBAL_NAVIGATION,
      keywords: ['my work', 'home', 'items', 'assigned', 'start'],
      url: '/m',
    }),
    createCommand({
      id: 'm-personal',
      label: 'Personal',
      description: 'Personal tasks',
      bucket: BUCKET.GLOBAL_NAVIGATION,
      keywords: ['personal', 'tasks', 'todo'],
      url: '/m/personal',
    }),
    createCommand({
      id: 'm-pages',
      label: 'Pages',
      description: 'Workspace knowledge base',
      bucket: BUCKET.GLOBAL_NAVIGATION,
      keywords: ['pages', 'wiki', 'knowledge', 'docs', 'notes'],
      url: '/m/pages',
    }),
    createCommand({
      id: 'm-timer',
      label: 'Timer',
      description: 'Time tracking',
      bucket: BUCKET.GLOBAL_NAVIGATION,
      keywords: ['timer', 'time', 'tracking', 'worklog'],
      url: '/m/timer',
    }),
    createCommand({
      id: 'm-notifications',
      label: 'Alerts',
      description: 'Notifications',
      bucket: BUCKET.GLOBAL_NAVIGATION,
      keywords: ['alerts', 'notifications', 'inbox', 'unread'],
      url: '/m/notifications',
    }),
    createCommand({
      id: 'm-search',
      label: 'Search',
      description: 'Find items and pages',
      bucket: BUCKET.GLOBAL_NAVIGATION,
      keywords: ['search', 'find', 'look'],
      url: '/m/search',
    }),
  ];

  if (aiStore.chatAvailable) {
    out.push(
      createCommand({
        id: 'm-chat',
        label: 'Assistant',
        description: 'AI chat',
        bucket: BUCKET.GLOBAL_NAVIGATION,
        keywords: ['assistant', 'ai', 'chat', 'ask'],
        url: '/m/chat',
      })
    );
  }

  return out;
}
