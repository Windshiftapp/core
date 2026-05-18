<script>
  import { currentRoute } from '../router.js';
  import PermissionGuard from '../layout/PermissionGuard.svelte';
  import SectionHeader from '../layout/SectionHeader.svelte';
  import TabNav from '../components/TabNav.svelte';
  import ServerClockSection from './diagnostics/ServerClockSection.svelte';
  import ActionLogsSection from './diagnostics/ActionLogsSection.svelte';
  import WebhookDeliveriesSection from './diagnostics/WebhookDeliveriesSection.svelte';
  import SchedulerRunsSection from './diagnostics/SchedulerRunsSection.svelte';
  import FracIndexSection from './diagnostics/FracIndexSection.svelte';

  const tabs = [
    { id: 'clock', label: 'Server clock' },
    { id: 'actions', label: 'Action executions' },
    { id: 'webhooks', label: 'Webhook deliveries' },
    { id: 'schedulers', label: 'Background jobs' },
    { id: 'fracindex', label: 'Frac index' },
  ];

  const subtab = $derived($currentRoute.query?.subtab || 'clock');
</script>

<PermissionGuard requireSystemAdmin={true}>
  <div class="space-y-6" data-testid="diagnostics-page">
    <SectionHeader
      title="Diagnostics"
      subtitle="Operational signals from the running system. Read-only — no actions taken from this page."
    />

    <TabNav {tabs} basePath="/admin/diagnostics" defaultTab="clock" />

    <div>
      {#if subtab === 'clock'}
        <ServerClockSection />
      {:else if subtab === 'actions'}
        <ActionLogsSection />
      {:else if subtab === 'webhooks'}
        <WebhookDeliveriesSection />
      {:else if subtab === 'schedulers'}
        <SchedulerRunsSection />
      {:else if subtab === 'fracindex'}
        <FracIndexSection />
      {/if}
    </div>
  </div>
</PermissionGuard>
