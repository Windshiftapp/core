<script>
  import TimeCustomers from './TimeCustomers.svelte';
  import TimeProjects from './TimeProjects.svelte';
  import TimeEntry from '../time/TimeEntry.svelte';
  import Timesheet from './Timesheet.svelte';
  import TimeReports from './TimeReports.svelte';
  import { IconUser as User, IconBriefcase as Briefcase, IconClock as Clock, IconCalendarEvent as CalendarDays, IconChartBar as BarChart3 } from '@tabler/icons-svelte-runes';
  import { currentRoute, navigate } from '../../router.js';
  import { t } from '../../stores/i18n.svelte.js';
  import SidebarHeader from '../../layout/SidebarHeader.svelte';

  let activeTab = $state('time-entry');

  const tabs = $derived([
    { id: 'time-entry', label: t('time.entry.title'), icon: Clock, component: TimeEntry, route: '/time' },
    { id: 'timesheet', label: t('time.timesheet.title'), icon: CalendarDays, component: Timesheet, route: '/time/timesheet' },
    { id: 'customers', label: t('time.organizations.title'), icon: User, component: TimeCustomers, route: '/time/customers' },
    { id: 'projects', label: t('time.projects.title'), icon: Briefcase, component: TimeProjects, route: '/time/projects' },
    { id: 'reports', label: t('time.reports.title'), icon: BarChart3, component: TimeReports, route: '/time/worklogs' }
  ]);

  // Update active tab based on current route
  $effect(() => {
    const path = $currentRoute.path;
    if (path === '/time') {
      activeTab = 'time-entry';
    } else if (path === '/time/timesheet') {
      activeTab = 'timesheet';
    } else if (path === '/time/customers') {
      activeTab = 'customers';
    } else if (path === '/time/categories') {
      activeTab = 'categories';
    } else if (path === '/time/projects') {
      activeTab = 'projects';
    } else if (path === '/time/worklogs') {
      activeTab = 'reports';
    }
  });

  function handleTabClick(tab) {
    navigate(tab.route);
  }
</script>

<!-- Main container with sidebar layout -->
<div class="flex min-h-screen" style="background-color: var(--ds-surface);">
  <!-- Left Sidebar -->
  <div class="w-64 border-r flex-shrink-0" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
    <div class="p-6">
      <SidebarHeader title={t('time.title')} description={t('time.subtitle')} noBorder />
      
      <!-- Navigation -->
      <nav class="space-y-1">
        {#each tabs as tab}
          {@const isTabActive = activeTab === tab.id}
          {@const TabIcon = tab.icon}
          <button
            onclick={() => handleTabClick(tab)}
            class="w-full group flex items-center px-3 py-2 text-sm font-medium rounded-lg transition-all cursor-pointer"
            style={isTabActive ? 'background: var(--ds-surface-selected); color: var(--ds-text);' : 'color: var(--ds-text-subtle);'}
            onmouseenter={(e) => { if (!isTabActive) e.currentTarget.style.cssText = 'background: var(--ds-surface-hovered); color: var(--ds-text);'; }}
            onmouseleave={(e) => { if (!isTabActive) e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'; }}
          >
            <TabIcon class="flex-shrink-0 -ml-1 mr-3 w-5 h-5" />
            {tab.label}
          </button>
        {/each}
      </nav>
    </div>
  </div>

  <!-- Main Content -->
  <div class="flex-1">
    {#each tabs as tab}
      {#if activeTab === tab.id}
        {@const TabComponent = tab.component}
        <div class="p-6">
          <TabComponent />
        </div>
      {/if}
    {/each}
  </div>
</div>