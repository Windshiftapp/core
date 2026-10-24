<script>
  import User from '@tabler/icons-svelte-runes/icons/user';
  import Briefcase from '@tabler/icons-svelte-runes/icons/briefcase';
  import Clock from '@tabler/icons-svelte-runes/icons/clock';
  import CalendarDays from '@tabler/icons-svelte-runes/icons/calendar-event';
  import BarChart3 from '@tabler/icons-svelte-runes/icons/chart-bar';
  import { currentRoute, navigate } from '../../router.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { permissionStore, isSystemAdmin } from '../../stores/permissions.svelte.js';
  import NavigationSidebar from '../../layout/NavigationSidebar.svelte';
  import Spinner from '../../components/Spinner.svelte';
  import Button from '../../components/Button.svelte';

  // Each tab is a literal dynamic import so entering Time ships only the active
  // tab's code; loaded modules are cached for later visits.
  const TAB_LOADERS = {
    'time-entry': () => import('./TimeEntry.svelte'),
    timesheet: () => import('./Timesheet.svelte'),
    organizations: () => import('./TimeCustomers.svelte'),
    projects: () => import('./TimeProjects.svelte'),
    reports: () => import('./TimeReports.svelte'),
  };

  let activeTab = $state('time-entry');
  let tabComponents = $state({});
  let tabErrors = $state({});

  async function loadTab(id) {
    try {
      const module = await TAB_LOADERS[id]();
      tabComponents = { ...tabComponents, [id]: module.default };
    } catch (error) {
      console.error(`Failed to load time tab "${id}":`, error);
      tabErrors = { ...tabErrors, [id]: true };
    }
  }

  function retryTab(id) {
    tabErrors = { ...tabErrors, [id]: false };
    void loadTab(id);
  }

  const canManageCustomers = $derived($permissionStore.userPermissionKeys?.has('customers.manage') || $isSystemAdmin);
  const canManageProjects = $derived($permissionStore.userPermissionKeys?.has('project.manage') || $isSystemAdmin);

  const tabs = $derived.by(() => {
    const allTabs = [
      { id: 'time-entry', label: t('time.entry.title'), icon: Clock, route: '/time' },
      { id: 'timesheet', label: t('time.timesheet.title'), icon: CalendarDays, route: '/time/timesheet' },
      { id: 'organizations', label: t('time.organizations.title'), icon: User, route: '/time/organizations', permission: canManageCustomers },
      { id: 'projects', label: t('time.projects.title'), icon: Briefcase, route: '/time/projects', permission: canManageProjects },
      { id: 'reports', label: t('time.reports.title'), icon: BarChart3, route: '/time/worklogs', permission: canManageProjects }
    ];

    return allTabs.filter(tab => !tab.permission || tab.permission);
  });

  // Update active tab based on current route
  $effect(() => {
    const path = $currentRoute.path;
    if (path === '/time') {
      activeTab = 'time-entry';
    } else if (path === '/time/timesheet') {
      activeTab = 'timesheet';
    } else if (path === '/time/organizations') {
      activeTab = 'organizations';
    } else if (path === '/time/categories') {
      activeTab = 'categories';
    } else if (path === '/time/projects') {
      activeTab = 'projects';
    } else if (path === '/time/worklogs') {
      activeTab = 'reports';
    }
  });

  // Load the active tab's code on demand and keep it for later visits.
  $effect(() => {
    const id = activeTab;
    if (!TAB_LOADERS[id] || tabComponents[id] || tabErrors[id]) return;
    void loadTab(id);
  });

  function handleTabClick(tab) {
    navigate(tab.route);
  }
</script>

<!-- Main container with sidebar layout -->
<div class="flex h-full min-h-0 overflow-hidden" style="background-color: var(--ds-surface);">
  <!-- Left Sidebar -->
  <NavigationSidebar title={t('time.title')} description={t('time.subtitle')}>
    <nav class="space-y-1">
      {#each tabs as tab (tab.id)}
        {@const isTabActive = activeTab === tab.id}
        {@const TabIcon = tab.icon}
        <button
          onclick={() => handleTabClick(tab)}
          class="nav-tab w-full group flex items-center px-3 py-2 text-sm font-medium rounded-lg transition-all cursor-pointer"
          class:active={isTabActive}
        >
          <TabIcon class="flex-shrink-0 -ml-1 mr-3 w-5 h-5" />
          {tab.label}
        </button>
      {/each}
    </nav>
  </NavigationSidebar>

  <!-- Main Content -->
  <div class="flex-1 min-h-0 overflow-y-auto">
    {#if !TAB_LOADERS[activeTab]}
      <!-- Unknown/legacy tab (e.g. /time/categories): render nothing. -->
    {:else if tabComponents[activeTab]}
      {@const TabComponent = tabComponents[activeTab]}
      <div class="p-6">
        <TabComponent />
      </div>
    {:else if tabErrors[activeTab]}
      <div class="p-6 text-center" data-testid="time-tab-error">
        <p class="mb-4" style="color: var(--ds-text-subtle);">{t('errors.failedToLoad')}</p>
        <Button
          variant="primary"
          onclick={() => retryTab(activeTab)}
          dataTestid="time-tab-retry"
        >
          {t('common.retry')}
        </Button>
      </div>
    {:else}
      <div class="p-6 flex justify-center" data-testid="time-tab-loading">
        <Spinner />
      </div>
    {/if}
  </div>
</div>

<style>
  .nav-tab {
    color: var(--ds-text-subtle);
  }

  .nav-tab:hover:not(.active) {
    background: var(--ds-background-neutral-hovered);
    color: var(--ds-text);
  }

  .nav-tab.active {
    background: var(--ds-surface-selected);
    color: var(--ds-text);
  }
</style>
