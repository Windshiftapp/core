<script>
  import SlaCalendarsTab from './SlaCalendarsTab.svelte';
  import SlaMetricsTab from './SlaMetricsTab.svelte';
  import SlaWarningThresholdsTab from './SlaWarningThresholdsTab.svelte';
  import SlaRecalculationTab from './SlaRecalculationTab.svelte';
  import SlaTeamBindingsTab from './SlaTeamBindingsTab.svelte';
  import SlaComplianceReportTab from './SlaComplianceReportTab.svelte';
  import TabStrip from '../../components/TabStrip.svelte';
  import BarChart3 from '@lucide/svelte/icons/bar-chart-3';
  import BellRing from '@lucide/svelte/icons/bell-ring';
  import Calendar from '@lucide/svelte/icons/calendar';
  import Gauge from '@lucide/svelte/icons/gauge';
  import Handshake from '@lucide/svelte/icons/handshake';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import { t } from '../../stores/i18n.svelte.js';

  let { workspaceId = null } = $props();

  // Sub-sections of the Service levels area. Each entry maps to a component
  // that reads the active workspace from the prop.
  const tabs = [
    {
      id: 'calendars',
      labelKey: 'workspaceSettings.serviceLevels.tabs.calendars',
      icon: Calendar,
      component: SlaCalendarsTab,
    },
    {
      id: 'metrics',
      labelKey: 'workspaceSettings.serviceLevels.tabs.metrics',
      icon: Gauge,
      component: SlaMetricsTab,
    },
    {
      id: 'warnings',
      labelKey: 'workspaceSettings.serviceLevels.tabs.warnings',
      icon: BellRing,
      component: SlaWarningThresholdsTab,
    },
    {
      id: 'recalculations',
      labelKey: 'workspaceSettings.serviceLevels.tabs.recalculations',
      icon: RefreshCw,
      component: SlaRecalculationTab,
    },
    {
      id: 'teams',
      labelKey: 'workspaceSettings.serviceLevels.tabs.teams',
      icon: Handshake,
      component: SlaTeamBindingsTab,
    },
    {
      id: 'report',
      labelKey: 'workspaceSettings.serviceLevels.tabs.report',
      icon: BarChart3,
      component: SlaComplianceReportTab,
    },
  ];

  function initialTab() {
    if (typeof window === 'undefined') return tabs[0].id;
    const requested = new URLSearchParams(window.location.search).get('subtab');
    return tabs.some((tab) => tab.id === requested) ? requested : tabs[0].id;
  }

  let active = $state(initialTab());
</script>

<div class="space-y-4" data-testid="service-levels">
  {#if tabs.length > 1}
    <TabStrip
      tabs={tabs.map((tab) => ({
        id: tab.id,
        label: t(tab.labelKey),
        icon: tab.icon,
        testid: `service-levels-tab-${tab.id}`,
      }))}
      bind:activeTab={active}
    />
  {/if}

  {#each tabs as tab (tab.id)}
    {#if active === tab.id}
      {@const TabComponent = tab.component}
      <TabComponent {workspaceId} />
    {/if}
  {/each}
</div>
