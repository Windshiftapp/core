import { bottomNavItems, mainNavItems } from '../../navigation/mainNavigation.js';
import { pluginNavItems } from '../../navigation/pluginNavigation.js';
import { extensions } from '../../stores/extensions.svelte.js';
import { BUCKET } from '../buckets.js';
import { createCommand } from '../types.js';

/**
 * Global navigation commands derived from the same registry that drives the
 * sidebar. Excludes /dashboard (dead route — Phase 0 audit decision).
 * Adds Search and Workspaces entries which are not in the sidebar registry
 * but are surfaced as palette commands.
 */
export function globalNavigationProvider(ctx) {
  const { t, permissions } = ctx;
  const out = [];

  // Built-ins not in mainNavItems
  out.push(
    createCommand({
      id: 'nav-workspaces',
      label: t('commandPalette.commands.workspaces.label'),
      description: t('commandPalette.commands.workspaces.description'),
      bucket: BUCKET.GLOBAL_NAVIGATION,
      keywords: ['workspace', 'projects', 'organize'],
      url: '/workspaces',
    }),
    createCommand({
      id: 'nav-search',
      label: t('commandPalette.commands.search.label'),
      description: t('commandPalette.commands.search.description'),
      bucket: BUCKET.GLOBAL_NAVIGATION,
      keywords: ['search', 'find', 'items'],
      url: '/search',
    })
  );

  // Sidebar nav, gated by the same permissionStore keys
  for (const item of [...mainNavItems, ...bottomNavItems]) {
    if (item.permission && !permissions[item.permission]) continue;
    if (item.id === 'admin') continue; // admin tabs handled by adminProvider
    out.push(
      createCommand({
        id: `nav-${item.id}`,
        label: t(item.labelKey),
        description: '',
        bucket: BUCKET.GLOBAL_NAVIGATION,
        keywords: [item.id],
        url: item.href,
      })
    );
  }

  // Plugin pages from `main.nav` extensions (hydrated by the sidebar and CommandPalette)
  for (const item of pluginNavItems(extensions.value)) {
    out.push(
      createCommand({
        id: `nav-${item.id}`,
        label: item.label,
        description: item.description || '',
        bucket: BUCKET.GLOBAL_NAVIGATION,
        keywords: ['plugin', item.pluginName, item.extensionId],
        url: item.href,
      })
    );
  }

  return out;
}
