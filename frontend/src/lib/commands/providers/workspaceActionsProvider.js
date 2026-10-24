import { workspacePermissions } from '../../stores/workspacePermissions.svelte.js';
import { BUCKET } from '../buckets.js';
import { createCommand } from '../types.js';

/**
 * Workspace-scoped actions: board configuration, settings, test management
 * create actions. Permission-gated where applicable.
 */
export function workspaceActionsProvider(ctx) {
  const { t, workspaceId, workspace, collectionId, modules } = ctx;
  if (!workspaceId) return [];

  const name = workspace?.name || 'Workspace';
  const out = [];

  // Board configuration saves are gated server-side: workspace scope requires
  // workspace.admin, collection scope requires the creator. Match the
  // CollectionViewSwitcher entry point and gate both commands on
  // canAdminWorkspace so non-admins don't reach a page they can't save.
  const canConfigureBoard = workspacePermissions.canAdminWorkspace(workspaceId);

  if (collectionId && canConfigureBoard) {
    out.push(
      createCommand({
        id: 'collection-configure-board',
        label: 'Configure Collection Board',
        description: 'Open board configuration for this collection',
        bucket: BUCKET.WORKSPACE_ACTIONS,
        keywords: ['configure', 'configuration', 'board', 'columns', 'collection'],
        url: `/workspaces/${workspaceId}/collections/${collectionId}/board/configure`,
      })
    );
  } else if (canConfigureBoard) {
    out.push(
      createCommand({
        id: 'workspace-configure-board',
        label: `Configure ${name} Board`,
        description: 'Open board configuration for this workspace',
        bucket: BUCKET.WORKSPACE_ACTIONS,
        keywords: [
          'configure',
          'configuration',
          'board',
          'columns',
          'workspace',
          name.toLowerCase(),
        ],
        url: `/workspaces/${workspaceId}/board/configure`,
      })
    );
  }

  if (workspacePermissions.canAdminWorkspace(workspaceId)) {
    out.push(
      createCommand({
        id: 'workspace-settings',
        label: `Open ${name} Settings`,
        description: 'Configure this workspace',
        bucket: BUCKET.WORKSPACE_ACTIONS,
        keywords: ['settings', 'configuration', 'workspace', name.toLowerCase()],
        url: `/workspaces/${workspaceId}/settings/general`,
      })
    );
  }

  // Test management create actions when module enabled in this workspace
  if (modules?.test_management_enabled && workspacePermissions.canViewTests(workspaceId)) {
    const testBase = `/workspaces/${workspaceId}/tests`;
    const makeTrigger = (label, descriptionKey, eventName, route, keywords) =>
      createCommand({
        id: `create-${label}`,
        label: t(
          `commandPalette.commands.create${label.replace(/(^|-)\w/g, (s) => s.replace('-', '').toUpperCase())}.label`
        ),
        description: t(descriptionKey),
        bucket: BUCKET.WORKSPACE_ACTIONS,
        keywords,
        execute: () => {
          import('../../router.js').then((r) => r.navigate(route));
          setTimeout(() => {
            window.dispatchEvent(new CustomEvent(eventName));
          }, 100);
        },
      });

    out.push(
      makeTrigger(
        'test-case',
        'commandPalette.commands.createTestCase.description',
        'trigger-test-case-form',
        testBase,
        ['create', 'new', 'test', 'case', 'testing', 'qa']
      ),
      makeTrigger(
        'test-plan',
        'commandPalette.commands.createTestPlan.description',
        'trigger-test-plan-form',
        `${testBase}/sets`,
        ['create', 'new', 'test', 'plan', 'testing', 'qa', 'suite']
      ),
      makeTrigger(
        'test-run',
        'commandPalette.commands.createTestRun.description',
        'trigger-test-run-form',
        `${testBase}/runs`,
        ['create', 'new', 'test', 'run', 'execution']
      )
    );
  }

  return out;
}
