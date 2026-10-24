import { resolveAdminGroups } from '../../admin/adminNavigation.js';
import { extensions, getExtensionsForPoint } from '../../stores/extensions.svelte.js';
import { BUCKET } from '../buckets.js';
import { createCommand } from '../types.js';

/**
 * Admin tab commands, gated by canAccessAdmin. Mirrors the admin sidebar:
 * the static registry plus plugin-provided `admin.tab` extensions, which
 * Admin.svelte merges from the same extension point.
 *
 * The extension store is hydrated by CommandPalette.loadData before the
 * provider first runs for a system admin.
 */
export function adminProvider(ctx) {
  const { t, permissions } = ctx;
  if (!permissions?.canAccessAdmin) return [];

  const out = [];
  for (const group of resolveAdminGroups(t)) {
    for (const item of group.items) {
      out.push(
        createCommand({
          id: `admin-${item.id}`,
          label: item.label,
          description: item.description || '',
          bucket: BUCKET.ADMIN,
          keywords: ['admin', item.id.replace(/-/g, ' ')],
          url: `/admin/${item.id}`,
        })
      );
    }
  }

  for (const ext of getExtensionsForPoint(extensions.value, 'admin.tab')) {
    if (!ext?.id || !ext?.label) continue;
    out.push(
      createCommand({
        id: `admin-plugin-${ext.id}`,
        label: ext.label,
        description: ext.description || '',
        bucket: BUCKET.ADMIN,
        keywords: ['admin', 'plugin', ext.id.replace(/-/g, ' '), ext.group].filter(Boolean),
        url: `/admin/${ext.id}`,
      })
    );
  }

  return out;
}
