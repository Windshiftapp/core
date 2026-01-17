<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import {
    Briefcase,
    FolderOpen,
    AlertCircle,
    CheckCircle2,
    Clock,
    TrendingUp,
    Plus,
    Settings,
    Users,
    BarChart3,
    Target,
    TestTube,
    Play,
    BarChart3 as DashboardIcon
  } from 'lucide-svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let stats = {
    workspaces: 0,
    projects: 0,
    items: 0,
    milestones: 0,
    openItems: 0,
    inProgressItems: 0,
    closedItems: 0,
    // Test statistics (only when test management is enabled)
    testSets: 0,
    testRuns: 0,
    testCases: 0
  };

  let recentItems = [];
  let loading = true;
  let moduleSettings = null;
  let isTestManagementEnabled = false;

  onMount(async () => {
    await loadDashboardData();
    loading = false;
  });

  async function loadDashboardData() {
    try {
      // Load module settings first
      moduleSettings = await api.setup.getModuleSettings();
      isTestManagementEnabled = moduleSettings?.test_management || false;

      const promises = [
        api.workspaces.getAll(),
        api.projects.getAll(),
        api.items.getAll(),
        api.milestones.getAll()
      ];

      // Add test-related promises if test management is enabled
      if (isTestManagementEnabled) {
        promises.push(
          api.tests.testSets.getAll(),
          api.tests.testRuns.getAll(),
          api.tests.testCases.getAll()
        );
      }

      const results = await Promise.all(promises);
      const [workspaces, projects, allItems, milestones, ...testResults] = results;

      // Ensure all responses are arrays or null/undefined
      const safeWorkspaces = Array.isArray(workspaces) ? workspaces : [];
      const safeProjects = Array.isArray(projects) ? projects : [];
      const safeItems = Array.isArray(allItems) ? allItems : [];
      const safeMilestones = Array.isArray(milestones) ? milestones : [];

      stats.workspaces = safeWorkspaces.length;
      stats.projects = safeProjects.length;
      stats.items = safeItems.length;
      stats.milestones = safeMilestones.length;
      
      if (safeItems.length > 0) {
        // Count items by status - assuming these are the common statuses
        stats.openItems = safeItems.filter(item => 
          !item.status_name || item.status_name.toLowerCase().includes('open') || 
          item.status_name.toLowerCase().includes('new') ||
          item.status_name.toLowerCase().includes('to do')
        ).length;
        stats.inProgressItems = safeItems.filter(item => 
          item.status_name && (
            item.status_name.toLowerCase().includes('progress') ||
            item.status_name.toLowerCase().includes('doing') ||
            item.status_name.toLowerCase().includes('active')
          )
        ).length;
        stats.closedItems = safeItems.filter(item => 
          item.status_name && (
            item.status_name.toLowerCase().includes('done') ||
            item.status_name.toLowerCase().includes('closed') ||
            item.status_name.toLowerCase().includes('resolved') ||
            item.status_name.toLowerCase().includes('completed')
          )
        ).length;
        
        // Get recent items (last 10)
        recentItems = safeItems
          .sort((a, b) => new Date(b.created_at) - new Date(a.created_at))
          .slice(0, 10);
      }

      // Set test statistics if test management is enabled
      if (isTestManagementEnabled && testResults.length >= 3) {
        const [testSets, testRuns, testCases] = testResults;
        const safeTestSets = Array.isArray(testSets) ? testSets : [];
        const safeTestRuns = Array.isArray(testRuns) ? testRuns : [];
        const safeTestCases = Array.isArray(testCases) ? testCases : [];
        
        stats.testSets = safeTestSets.length;
        stats.testRuns = safeTestRuns.length;
        stats.testCases = safeTestCases.length;
      }
    } catch (error) {
      console.error('Failed to load dashboard data:', error);
    }
  }

  function getPriorityColor(priority) {
    const colors = {
      low: 'bg-green-100 text-green-800',
      medium: 'bg-yellow-100 text-yellow-800',
      high: 'bg-orange-100 text-orange-800',
      critical: 'bg-red-100 text-red-800'
    };
    return colors[priority.toLowerCase()] || 'bg-gray-100 text-gray-800';
  }
</script>

<div class="min-h-screen" style="background-color: var(--ds-surface);">
    <PageHeader
      icon={DashboardIcon}
      title={t('dashboard.title')}
      subtitle={t('dashboard.subtitle')}
    />

  {#if loading}
    <div class="flex justify-center items-center h-64">
      <div class="text-lg" style="color: var(--ds-text-subtle);">{t('common.loading')}</div>
    </div>
  {:else}
    <!-- Stats Cards -->
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-{isTestManagementEnabled ? '4' : '3'} gap-4 mb-6">
      <div class="rounded p-4 border shadow-sm hover:shadow-md transition-shadow" style="background: linear-gradient(135deg, var(--ds-surface-raised) 0%, rgba(59, 130, 246, 0.02) 100%); border-color: var(--ds-border);">
        <div class="flex items-center">
          <div class="flex-shrink-0">
            <div class="w-6 h-6 bg-blue-50 rounded-md flex items-center justify-center">
              <Briefcase class="w-3.5 h-3.5 text-blue-600" />
            </div>
          </div>
          <div class="ml-3 w-0 flex-1">
            <dl>
              <dt class="text-xs font-medium truncate" style="color: var(--ds-text-subtle);">{t('workspaces.title')}</dt>
              <dd class="text-xl font-semibold" style="color: var(--ds-text);">{stats.workspaces}</dd>
            </dl>
          </div>
        </div>
      </div>

      <div class="rounded p-4 border shadow-sm hover:shadow-md transition-shadow" style="background: linear-gradient(135deg, var(--ds-surface-raised) 0%, rgba(34, 197, 94, 0.02) 100%); border-color: var(--ds-border);">
        <div class="flex items-center">
          <div class="flex-shrink-0">
            <div class="w-6 h-6 bg-green-50 rounded-md flex items-center justify-center">
              <Target class="w-3.5 h-3.5 text-green-600" />
            </div>
          </div>
          <div class="ml-3 w-0 flex-1">
            <dl>
              <dt class="text-xs font-medium truncate" style="color: var(--ds-text-subtle);">{t('nav.milestones')}</dt>
              <dd class="text-xl font-semibold" style="color: var(--ds-text);">{stats.milestones}</dd>
            </dl>
          </div>
        </div>
      </div>

      <div class="rounded p-4 border shadow-sm hover:shadow-md transition-shadow" style="background: linear-gradient(135deg, var(--ds-surface-raised) 0%, rgba(245, 101, 101, 0.02) 100%); border-color: var(--ds-border);">
        <div class="flex items-center">
          <div class="flex-shrink-0">
            <div class="w-6 h-6 bg-orange-50 rounded-md flex items-center justify-center">
              <AlertCircle class="w-3.5 h-3.5 text-orange-600" />
            </div>
          </div>
          <div class="ml-3 w-0 flex-1">
            <dl>
              <dt class="text-xs font-medium truncate" style="color: var(--ds-text-subtle);">{t('items.title')}</dt>
              <dd class="text-xl font-semibold" style="color: var(--ds-text);">{stats.items}</dd>
            </dl>
          </div>
        </div>
      </div>

      {#if isTestManagementEnabled}
        <div class="rounded p-4 border shadow-sm hover:shadow-md transition-shadow" style="background: linear-gradient(135deg, var(--ds-surface-raised) 0%, rgba(139, 69, 196, 0.02) 100%); border-color: var(--ds-border);">
          <div class="flex items-center">
            <div class="flex-shrink-0">
              <div class="w-6 h-6 bg-purple-50 rounded-md flex items-center justify-center">
                <TestTube class="w-3.5 h-3.5 text-purple-600" />
              </div>
            </div>
            <div class="ml-3 w-0 flex-1">
              <dl>
                <dt class="text-xs font-medium truncate" style="color: var(--ds-text-subtle);">{t('testing.testPlans')}</dt>
                <dd class="text-xl font-semibold" style="color: var(--ds-text);">{stats.testSets}</dd>
              </dl>
            </div>
          </div>
        </div>
      {/if}
    </div>

    <!-- Work Item Status Overview -->
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-4 mb-6">
      <div class="rounded p-5 border shadow-sm" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <div class="flex items-center mb-4">
          <BarChart3 class="w-4 h-4 mr-2" style="color: var(--ds-text-subtle);" />
          <h3 class="text-base font-semibold" style="color: var(--ds-text);">{t('dashboard.workItemStatusOverview')}</h3>
        </div>
        <div class="space-y-3">
          <div class="flex justify-between items-center">
            <span class="text-sm font-medium" style="color: var(--ds-text);">{t('dashboard.statusOpen')}</span>
            <div class="flex items-center">
              <div class="w-32 bg-gray-200 rounded-full h-2 mr-3">
                <div class="bg-blue-500 h-2 rounded-full" style="width: {stats.items > 0 ? (stats.openItems / stats.items * 100) : 0}%"></div>
              </div>
              <span class="text-sm font-medium" style="color: var(--ds-text);">{stats.openItems}</span>
            </div>
          </div>
          <div class="flex justify-between items-center">
            <span class="text-sm font-medium" style="color: var(--ds-text);">{t('dashboard.statusInProgress')}</span>
            <div class="flex items-center">
              <div class="w-32 bg-gray-200 rounded-full h-2 mr-3">
                <div class="bg-yellow-500 h-2 rounded-full" style="width: {stats.items > 0 ? (stats.inProgressItems / stats.items * 100) : 0}%"></div>
              </div>
              <span class="text-sm font-medium" style="color: var(--ds-text);">{stats.inProgressItems}</span>
            </div>
          </div>
          <div class="flex justify-between items-center">
            <span class="text-sm font-medium" style="color: var(--ds-text);">{t('dashboard.statusClosed')}</span>
            <div class="flex items-center">
              <div class="w-32 bg-gray-200 rounded-full h-2 mr-3">
                <div class="bg-green-500 h-2 rounded-full" style="width: {stats.items > 0 ? (stats.closedItems / stats.items * 100) : 0}%"></div>
              </div>
              <span class="text-sm font-medium" style="color: var(--ds-text);">{stats.closedItems}</span>
            </div>
          </div>
        </div>
      </div>

      <div class="rounded p-5 border shadow-sm" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <div class="flex items-center mb-4">
          <Plus class="w-4 h-4 mr-2" style="color: var(--ds-text-subtle);" />
          <h3 class="text-base font-semibold" style="color: var(--ds-text);">{t('dashboard.quickActions')}</h3>
        </div>
        <div class="space-y-2">
          <button
            onclick={() => {
              window.dispatchEvent(new CustomEvent('open-create-modal'));
              setTimeout(() => {
                window.dispatchEvent(new CustomEvent('set-create-type', {
                  detail: { type: 'work-item' }
                }));
              }, 50);
            }}
            class="w-full text-left px-3 py-2.5 rounded-md border hover:bg-gray-50 transition-colors group"
            style="border-color: var(--ds-border);"
          >
            <div class="flex items-center">
              <Plus class="w-3.5 h-3.5 mr-2 opacity-60 group-hover:opacity-100" style="color: var(--ds-text-subtle);" />
              <div class="flex-1">
                <div class="text-sm font-medium" style="color: var(--ds-text);">{t('dashboard.createWorkItem')}</div>
                <div class="text-xs mt-0.5" style="color: var(--ds-text-subtle);">{t('dashboard.createWorkItemDesc')}</div>
              </div>
            </div>
          </button>
          <button
            onclick={() => window.location.href = '/milestones'}
            class="w-full text-left px-3 py-2.5 rounded-md border hover:bg-gray-50 transition-colors group"
            style="border-color: var(--ds-border);"
          >
            <div class="flex items-center">
              <Target class="w-3.5 h-3.5 mr-2 opacity-60 group-hover:opacity-100" style="color: var(--ds-text-subtle);" />
              <div class="flex-1">
                <div class="text-sm font-medium" style="color: var(--ds-text);">{t('dashboard.manageMilestones')}</div>
                <div class="text-xs mt-0.5" style="color: var(--ds-text-subtle);">{t('dashboard.manageMilestonesDesc')}</div>
              </div>
            </div>
          </button>
          <button
            onclick={() => window.location.href = '/workspaces'}
            class="w-full text-left px-3 py-2.5 rounded-md border hover:bg-gray-50 transition-colors group"
            style="border-color: var(--ds-border);"
          >
            <div class="flex items-center">
              <Settings class="w-3.5 h-3.5 mr-2 opacity-60 group-hover:opacity-100" style="color: var(--ds-text-subtle);" />
              <div class="flex-1">
                <div class="text-sm font-medium" style="color: var(--ds-text);">{t('dashboard.manageWorkspaces')}</div>
                <div class="text-xs mt-0.5" style="color: var(--ds-text-subtle);">{t('dashboard.manageWorkspacesDesc')}</div>
              </div>
            </div>
          </button>
        </div>
      </div>
    </div>

    <!-- Recent Work Items -->
    {#if recentItems.length > 0}
      <div class="rounded border shadow-sm" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <div class="px-5 py-3 border-b" style="border-color: var(--ds-border);">
          <div class="flex items-center">
            <Clock class="w-4 h-4 mr-2" style="color: var(--ds-text-subtle);" />
            <h3 class="text-base font-semibold" style="color: var(--ds-text);">{t('dashboard.recentWorkItems')}</h3>
          </div>
        </div>
        <div class="overflow-x-auto">
          <table class="w-full">
            <thead style="background-color: var(--ds-background-neutral);">
              <tr>
                <th class="px-4 py-2.5 text-left text-xs font-medium uppercase tracking-wider" style="color: var(--ds-text-subtle);">{t('items.item')}</th>
                <th class="px-4 py-2.5 text-left text-xs font-medium uppercase tracking-wider" style="color: var(--ds-text-subtle);">{t('workspaces.workspace')}</th>
                <th class="px-4 py-2.5 text-left text-xs font-medium uppercase tracking-wider" style="color: var(--ds-text-subtle);">{t('common.status')}</th>
                <th class="px-4 py-2.5 text-left text-xs font-medium uppercase tracking-wider" style="color: var(--ds-text-subtle);">{t('common.priority')}</th>
                <th class="px-4 py-2.5 text-left text-xs font-medium uppercase tracking-wider" style="color: var(--ds-text-subtle);">{t('common.created')}</th>
              </tr>
            </thead>
            <tbody class="divide-y" style="divide-color: var(--ds-border);">
              {#each recentItems as item (item.id)}
                <tr class="hover:bg-gray-50">
                  <td class="px-4 py-3">
                    <div class="font-medium text-sm" style="color: var(--ds-text);">{item.title}</div>
                    {#if item.description}
                      <div class="text-xs mt-0.5 line-clamp-1" style="color: var(--ds-text-subtle);">{item.description}</div>
                    {/if}
                  </td>
                  <td class="px-4 py-3 text-sm" style="color: var(--ds-text);">{item.workspace_name || '—'}</td>
                  <td class="px-4 py-3">
                    <span class="inline-flex px-2 py-0.5 text-xs font-medium rounded-md bg-gray-100 text-gray-800">
                      {item.status_name || 'No Status'}
                    </span>
                  </td>
                  <td class="px-4 py-3">
                    <span class="inline-flex px-2 py-0.5 text-xs font-medium rounded-md {getPriorityColor(item.priority || 'medium')}">
                      {(item.priority || 'medium').charAt(0).toUpperCase() + (item.priority || 'medium').slice(1)}
                    </span>
                  </td>
                  <td class="px-4 py-3 text-sm" style="color: var(--ds-text-subtle);">
                    {new Date(item.created_at).toLocaleDateString()}
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </div>
    {/if}
  {/if}
</div>