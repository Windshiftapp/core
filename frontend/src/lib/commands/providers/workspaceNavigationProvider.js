import {
  testNavigationItems,
  visibleWorkspaceSettingsItems,
  workspaceOnlyViews,
  workspaceSettingsRoute,
  workspaceSettingsSubtitleKeys,
  workspaceViewItems,
} from '../../navigation/workspaceNavigation.js';
import { t } from '../../stores/i18n.svelte.js';
import { viewSettingsStore } from '../../stores/viewSettings.svelte.js';
import { workspacePermissions } from '../../stores/workspacePermissions.svelte.js';
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

  const enabledViews = new Set(viewSettingsStore.enabledViewIds(workspaceId, collectionId));
  for (const view of workspaceViewItems) {
    if (!enabledViews.has(view.id)) continue;
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
    const enabledNav = new Set(viewSettingsStore.enabledNavIds(workspaceId));
    for (const view of workspaceOnlyViews) {
      if (!enabledNav.has(view.id)) continue;
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
    // Test entries are not nav-configurable; module and permission gating
    // decide their visibility.
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

  // Workspace settings modules. The settings area itself needs workspace.admin;
  // permissioned modules are filtered exactly like the settings sidebar.
  if (!collectionId && workspacePermissions.canAdminWorkspace(workspaceId)) {
    const settings = visibleWorkspaceSettingsItems(workspaceId, (wsId, permission) =>
      workspacePermissions.hasPermission(wsId, permission)
    );
    for (const item of settings) {
      const label = t(item.labelKey);
      out.push(
        createCommand({
          id: `workspace-settings-${item.id}`,
          label: `${name}: ${label}`,
          description: t(workspaceSettingsSubtitleKeys[item.id] || 'workspaceSettings.title'),
          bucket: BUCKET.WORKSPACE_NAVIGATION,
          keywords: [
            'settings',
            'admin',
            'workspace',
            item.id,
            label.toLowerCase(),
            name.toLowerCase(),
          ],
          url: workspaceSettingsRoute(workspaceId, item.id),
        })
      );
    }
  }

  // Filter out the command for the current view to reduce noise.
  const here = `${route?.path || ''}${typeof window !== 'undefined' ? window.location.search : ''}`;
  return out.filter((c) => c.url !== here);
}
