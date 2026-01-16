<script>
  import TimeCustomers from './TimeCustomers.svelte';
  import TimeProjects from './TimeProjects.svelte';
  import TimeEntry from '../time/TimeEntry.svelte';
  import TimeReports from './TimeReports.svelte';
  import { User, Briefcase, Clock, BarChart3 } from 'lucide-svelte';
  import { currentRoute, navigate } from '../../router.js';

  let activeTab = 'time-entry';

  const tabs = [
    { id: 'time-entry', label: 'Time Entry', icon: Clock, component: TimeEntry, route: '/time' },
    { id: 'customers', label: 'Organizations', icon: User, component: TimeCustomers, route: '/time/customers' },
    { id: 'projects', label: 'Projects', icon: Briefcase, component: TimeProjects, route: '/time/projects' },
    { id: 'reports', label: 'Reports', icon: BarChart3, component: TimeReports, route: '/time/worklogs' }
  ];

  // Update active tab based on current route
  $: {
    const path = $currentRoute.path;
    if (path === '/time') {
      activeTab = 'time-entry';
    } else if (path === '/time/customers') {
      activeTab = 'customers';
    } else if (path === '/time/categories') {
      activeTab = 'categories';
    } else if (path === '/time/projects') {
      activeTab = 'projects';
    } else if (path === '/time/worklogs') {
      activeTab = 'reports';
    }
  }

  function handleTabClick(tab) {
    navigate(tab.route);
  }
</script>

<!-- Main container with sidebar layout -->
<div class="flex min-h-screen" style="background-color: var(--ds-surface);">
  <!-- Left Sidebar -->
  <div class="w-64 border-r flex-shrink-0" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
    <div class="p-6">
      <div class="mb-6">
        <h1 class="text-xl font-semibold" style="color: var(--ds-text);">Time & Projects</h1>
        <p class="mt-1 text-sm" style="color: var(--ds-text-subtle);">Manage organizations, projects, and time tracking</p>
      </div>
      
      <!-- Navigation -->
      <nav class="space-y-1">
        {#each tabs as tab}
          {@const isTabActive = activeTab === tab.id}
          <button
            onclick={() => handleTabClick(tab)}
            class="w-full group flex items-center px-3 py-2 text-sm font-medium rounded-lg transition-all cursor-pointer"
            style={isTabActive ? 'background: var(--ds-surface-selected); color: var(--ds-text);' : 'color: var(--ds-text-subtle);'}
            onmouseenter={(e) => { if (!isTabActive) e.currentTarget.style.cssText = 'background: var(--ds-surface-hovered); color: var(--ds-text);'; }}
            onmouseleave={(e) => { if (!isTabActive) e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'; }}
          >
            <svelte:component this={tab.icon} class="flex-shrink-0 -ml-1 mr-3 w-5 h-5" />
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
        <div class="p-6">
          <svelte:component this={tab.component} />
        </div>
      {/if}
    {/each}
  </div>
</div>