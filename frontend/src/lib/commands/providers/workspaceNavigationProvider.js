import {
  testNavigationItems,
  workspaceOnlyViews,
  workspaceViewItems,
} from '../../navigation/workspaceNavigation.js';
import { workspacePermissions } from '../../stores';
import { t } from '../../stores/i18n.svelte.js';
import { BUCKET } from '../buckets.js';
import { createCommand } from '../types.js';

function buildViewUrl(workspaceId, view, collectionId) {
  const prefix = collectionId ? `/collections/${collectionId}` : '';
  return `/workspaces/${workspaceId}${prefix}/${view}`;
}

/** Workspace navigation for every workspace route, without the overview
 * dashboard keyword that polluted board searches. */
export function workspaceNavigationProvider(ctx) {
  const { workspaceId, workspace, collectionId, route, modules } = ctx;
  if (!workspaceId) return [];

  const name = workspace?.name || t('common.workspace');
  const out = [];

  out.push(
    createCommand({
      id: 'workspace-overview',
      label: t('commandPalette.commands.workspaceOverview.label', { name }),
      description: t('commandPalette.commands.workspaceOverview.description'),
      bucket: BUCKET.WORKSPACE_NAVIGATION,
      keywords: ['overview', 'workspace', 'stats', name.toLowerCase()],
      url: collectionId
        ? buildViewUrl(workspaceId, 'overview', collectionId)
        : `/workspaces/${workspaceId}`,
    })
  );

  for (const view of workspaceViewItems) {
    const label = t(view.labelKey);
    out.push(
      createCommand({
        id: `workspace-${view.id}-view`,
        label: `${name}: ${label}`,
        description: t(view.tooltipKey || view.labelKey),
        bucket: BUCKET.WORKSPACE_NAVIGATION,
        keywords: [view.id, label.toLowerCase(), name.toLowerCase()],
        url: buildViewUrl(workspaceId, view.id, collectionId),
      })
    );
  }

  if (!collectionId) {
    for (const view of workspaceOnlyViews) {
      if (view.id === 'agents' && !workspacePermissions.canAdminWorkspace(workspaceId)) continue;
      const label = t(view.labelKey);
      out.push(
        createCommand({
          id: `workspace-${view.id}-view`,
          label: `${name}: ${label}`,
          description: t(view.tooltipKey || view.labelKey),
          bucket: BUCKET.WORKSPACE_NAVIGATION,
          keywords: [view.id, label.toLowerCase(), name.toLowerCase()],
          url: buildViewUrl(workspaceId, view.id, null),
        })
      );
    }
  }

  if (
    modules?.test_management_enabled &&
    workspacePermissions.canViewTests(workspaceId) &&
    !collectionId
  ) {
    for (const view of testNavigationItems) {
      const slug = view.id === 'test-cases' ? 'tests' : `tests/${view.id.replace(/^test-/, '')}`;
      const label = t(view.labelKey);
      out.push(
        createCommand({
          id: `workspace-${view.id}`,
          label: `${name}: ${label}`,
          description: t(view.tooltipKey || view.labelKey),
          bucket: BUCKET.WORKSPACE_NAVIGATION,
          keywords: ['test', 'testing', 'qa', view.id, label.toLowerCase()],
          url: `/workspaces/${workspaceId}/${slug}`,
        })
      );
    }
  }

  // Filter out the command for the current view to reduce noise.
  const here = `${route?.path || ''}${typeof window !== 'undefined' ? window.location.search : ''}`;
  return out.filter((c) => c.url !== here);
}
