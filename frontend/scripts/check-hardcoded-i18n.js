import { readFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

// This list is an incremental ratchet: once a user-facing screen has been
// migrated, literal English UI copy must not return to it.
const guardedFiles = [
  'src/lib/layout/DashboardCustomizationSidebar.svelte',
  'src/lib/pages/Homepage.svelte',
  'src/lib/features/workflows/WorkflowBuilder.svelte',
  'src/lib/pages/Screens.svelte',
  'src/lib/pickers/ConfigurationSetPicker.svelte',
  'src/lib/pickers/ScreenPicker.svelte',
  'src/lib/pickers/WorkflowPicker.svelte',
  'src/lib/settings/ConfigurationSetManager.svelte',
  'src/lib/settings/ConfigurationSetItemTypes.svelte',
  'src/lib/settings/ActionCapabilitiesManager.svelte',
  'src/lib/settings/ActionCredentialManager.svelte',
  'src/lib/settings/AgentTemplateManager.svelte',
  'src/lib/settings/CapabilityManager.svelte',
  'src/lib/settings/HierarchyLevelManager.svelte',
  'src/lib/settings/ItemTypeManager.svelte',
  'src/lib/settings/IntegrationProviderManager.svelte',
  'src/lib/settings/IntegrationsManager.svelte',
  'src/lib/settings/LLMConnectionManager.svelte',
  'src/lib/settings/LinkTypeManager.svelte',
  'src/lib/settings/OAuthClientManager.svelte',
  'src/lib/settings/PriorityManager.svelte',
  'src/lib/settings/StatusCategoryManager.svelte',
  'src/lib/settings/StatusManager.svelte',
  'src/lib/settings/ThemeManager.svelte',
  'src/lib/settings/RunnerPoolManager.svelte',
  'src/lib/workspaces/WorkspaceConfigurationAssigner.svelte',
  'src/lib/workspaces/WorkspaceConfigurationPreview.svelte',
  'src/lib/workspaces/WorkspaceMembers.svelte',
  'src/lib/workspaces/WorkspaceCustomizationSidebar.svelte',
  'src/lib/workspaces/WorkspaceNavigation.svelte',
  'src/lib/workspaces/WorkspaceWelcome.svelte',
  'src/lib/workspaces/Workspaces.svelte',
  'src/lib/widgets/CompletionChartWidget.svelte',
  'src/lib/widgets/CreatedChartWidget.svelte',
  'src/lib/widgets/IterationTimelineWidget.svelte',
  'src/lib/widgets/MilestoneProgressWidget.svelte',
  'src/lib/widgets/MyTasksWidget.svelte',
  'src/lib/widgets/OverdueItemsWidget.svelte',
  'src/lib/widgets/RecentItemsWidget.svelte',
  'src/lib/widgets/StatsCardWidget.svelte',
  'src/lib/widgets/TestCoverageWidget.svelte',
  'src/lib/widgets/UpcomingDeadlinesWidget.svelte',
  'src/lib/widgets/WidgetState.svelte',
  'src/lib/widgets/WidgetWrapper.svelte',
  'src/lib/widgets/dashboard/AssignedToMeWidget.svelte',
  'src/lib/widgets/dashboard/DailyBriefingWidget.svelte',
  'src/lib/widgets/dashboard/DashboardItemRow.svelte',
  'src/lib/widgets/dashboard/DashboardTaskList.svelte',
  'src/lib/widgets/dashboard/DueMark.svelte',
  'src/lib/widgets/dashboard/PersonalTasksWidget.svelte',
  'src/lib/widgets/dashboard/QuickAccessWidget.svelte',
  'src/lib/widgets/dashboard/RecentWorkspacesWidget.svelte',
  'src/lib/widgets/dashboard/SavedSearchWidget.svelte',
  'src/lib/widgets/dashboard/UpcomingMilestonesWidget.svelte',
  'src/lib/widgets/dashboard/WatchedItemsWidget.svelte',
  'src/lib/widgets/dashboard/WhatsNewWidget.svelte',
  'src/lib/widgets/dashboard/YourActivityWidget.svelte',
];

const rules = [
  {
    name: 'visible text',
    pattern: />\s*([A-Z][^<{\n]*?)\s*</g,
  },
  {
    name: 'localizable attribute',
    pattern:
      /(?:placeholder|title|aria-label|emptyMessage|emptyDescription|confirmLabel|cancelLabel|subtitle)=["']([A-Z][^"']+)["']/g,
  },
];

const intentionalLiterals = new Set(['Esc']);

const violations = [];

for (const relativeFile of guardedFiles) {
  // Script blocks contain operators such as `=>` that look like HTML text to
  // the lightweight regex guard. UI copy lives in the component markup.
  const source = readFileSync(path.join(root, relativeFile), 'utf8').replace(
    /<script\b[^>]*>[\s\S]*?<\/script>/g,
    (script) => '\n'.repeat(script.split('\n').length - 1)
  );
  for (const rule of rules) {
    for (const match of source.matchAll(rule.pattern)) {
      if (intentionalLiterals.has(match[1].trim())) continue;
      const line = source.slice(0, match.index).split('\n').length;
      violations.push(`${relativeFile}:${line} ${rule.name}: ${JSON.stringify(match[1].trim())}`);
    }
  }
}

if (violations.length > 0) {
  console.error('Hardcoded i18n guard failed. Move this copy into the locale catalog:');
  for (const violation of violations) console.error(`  ${violation}`);
  process.exit(1);
}

console.log(`Hardcoded i18n guard passed (${guardedFiles.length} migrated screens).`);
