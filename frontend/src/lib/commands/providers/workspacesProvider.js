import { BUCKET } from '../buckets.js';
import { createCommand } from '../types.js';

/** Match all active or personal workspaces before ranking caps the display. */
export function workspacesProvider(ctx) {
  const { workspaces, t } = ctx;
  const active = (workspaces || []).filter((ws) => ws.is_personal || ws.active !== false);

  return active.map((ws) => {
    const url = ws.is_personal ? '/personal' : `/workspaces/${ws.id}`;
    return createCommand({
      id: `goto-workspace-${ws.id}`,
      label: t('commandPalette.commands.goToWorkspace.label', { name: ws.name }),
      description: t('commandPalette.commands.goToWorkspace.description', { name: ws.name }),
      bucket: BUCKET.GLOBAL_NAVIGATION,
      keywords: [
        'goto',
        'workspace',
        'navigate',
        ws.name?.toLowerCase(),
        ws.key,
        ws.description,
      ].filter(Boolean),
      url,
    });
  });
}
