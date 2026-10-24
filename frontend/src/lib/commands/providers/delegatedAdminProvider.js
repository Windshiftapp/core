import { filterDelegatedAdminItems } from '../../navigation/delegatedAdminNavigation.js';
import { BUCKET } from '../buckets.js';
import { createCommand } from '../types.js';

/** Reuse the descriptions of the matching global commands. */
const DESCRIPTION_KEYS = {
  channels: 'commandPalette.commands.channels.description',
  assets: 'commandPalette.commands.assets.description',
};

/**
 * Management surfaces for non-system-admins (`/manage/*`). The delegated-admin
 * registry already excludes system admins, so these never overlap with
 * adminProvider's `/admin/*` entries.
 */
export function delegatedAdminProvider(ctx) {
  const { t, permissions } = ctx;
  return filterDelegatedAdminItems(permissions).map((item) =>
    createCommand({
      id: `manage-${item.id}`,
      label: t(item.labelKey),
      description: DESCRIPTION_KEYS[item.id] ? t(DESCRIPTION_KEYS[item.id]) : '',
      bucket: BUCKET.ADMIN,
      keywords: ['manage', 'admin', item.id],
      url: item.href,
    })
  );
}
