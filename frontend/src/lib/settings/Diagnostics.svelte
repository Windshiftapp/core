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
  import LLMHealthSection from './diagnostics/LLMHealthSection.svelte';
  import RunnerPoolsSection from './diagnostics/RunnerPoolsSection.svelte';
  import DatabasePoolsSection from './diagnostics/DatabasePoolsSection.svelte';
  import CacheMemorySection from './diagnostics/CacheMemorySection.svelte';
  import RecurrenceVolumeSection from './diagnostics/RecurrenceVolumeSection.svelte';
  import DomainEventsSection from './diagnostics/DomainEventsSection.svelte';
  import SCMHealthSection from './diagnostics/SCMHealthSection.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let diagnosticGroups = $derived([
    { label: t('diagnostics.groups.overview'), tabs: [{ id: 'clock', label: t('diagnostics.tabs.clock') }] },
    {
      label: t('diagnostics.groups.automation'),
      tabs: [
        { id: 'actions', label: t('diagnostics.tabs.actions') },
        { id: 'webhooks', label: t('diagnostics.tabs.webhooks') },
        { id: 'schedulers', label: t('diagnostics.tabs.schedulers') },
        { id: 'recurrence-volume', label: t('diagnostics.tabs.recurrence') },
        { id: 'domain-events', label: t('diagnostics.tabs.domainEvents') },
      ],
    },
    { label: t('diagnostics.groups.data'), tabs: [{ id: 'frac-index', label: t('diagnostics.tabs.fracIndex') }] },
    { label: 'AI / LLM', tabs: [{ id: 'llm-health', label: 'AI / LLM' }] },
    {
      label: t('diagnostics.groups.infrastructure'),
      tabs: [
        { id: 'runner-pools', label: t('diagnostics.tabs.runnerPools') },
        { id: 'database-pools', label: t('diagnostics.tabs.databasePools') },
        { id: 'cache-memory', label: t('diagnostics.tabs.cacheMemory') },
      ],
    },
    { label: 'SCM', tabs: [{ id: 'scm-health', label: t('diagnostics.tabs.scmConnections') }] },
  ]);

  let tabs = $derived(diagnosticGroups.map((group) => ({
    id: group.tabs[0].id,
    label: group.label,
    matches: group.tabs.map((tab) => tab.id),
  })));

  const subtab = $derived($currentRoute.query?.subtab || 'clock');
  const activeGroup = $derived(
    diagnosticGroups.find((group) => group.tabs.some((tab) => tab.id === subtab)) ?? diagnosticGroups[0]
  );
</script>

<PermissionGuard requireSystemAdmin={true}>
  <div class="space-y-6" data-testid="diagnostics-page">
    <SectionHeader
      title={t('diagnostics.title')}
      subtitle={t('diagnostics.subtitle')}
    />

    <TabNav {tabs} basePath="/admin/diagnostics" defaultTab="clock" />

    {#if activeGroup.tabs.length > 1}
      <TabNav tabs={activeGroup.tabs} basePath="/admin/diagnostics" defaultTab={activeGroup.tabs[0].id} />
    {/if}

    <div>
      {#if subtab === 'clock'}
        <ServerClockSection />
      {:else if subtab === 'actions'}
        <ActionLogsSection />
      {:else if subtab === 'webhooks'}
        <WebhookDeliveriesSection />
      {:else if subtab === 'schedulers'}
        <SchedulerRunsSection />
      {:else if subtab === 'frac-index'}
        <FracIndexSection />
      {:else if subtab === 'llm-health'}
        <LLMHealthSection />
      {:else if subtab === 'runner-pools'}
        <RunnerPoolsSection />
      {:else if subtab === 'database-pools'}
        <DatabasePoolsSection />
      {:else if subtab === 'cache-memory'}
        <CacheMemorySection />
      {:else if subtab === 'recurrence-volume'}
        <RecurrenceVolumeSection />
      {:else if subtab === 'domain-events'}
        <DomainEventsSection />
      {:else if subtab === 'scm-health'}
        <SCMHealthSection />
      {/if}
    </div>
  </div>
</PermissionGuard>
