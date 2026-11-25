<script>
  import { currentRoute, updateQueryParams } from '../router.js';
  import Tabs from '../components/Tabs.svelte';
  import LLMConnectionManager from './LLMConnectionManager.svelte';
  import AIFeaturesSettings from './AIFeaturesSettings.svelte';
  import AgentTemplateManager from './AgentTemplateManager.svelte';
  import WorkItemStalenessSettings from './WorkItemStalenessSettings.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { capabilitiesStore } from '../stores/capabilities.svelte.js';
  import AlertBox from '../components/AlertBox.svelte';
  import Plug from '@lucide/svelte/icons/plug';
  import Sparkles from '@lucide/svelte/icons/sparkles';
  import Bot from '@lucide/svelte/icons/bot';
  import Gauge from '@lucide/svelte/icons/gauge';

  // Active subtab lives in the URL query so deep links keep working, matching
  // the local-state-plus-URL-sync pattern used by ActionCapabilitiesManager.
  const VALID_TABS = new Set(['connections', 'features', 'agent-templates', 'staleness']);

  let subtab = $state('connections');

  const tabs = $derived([
    { id: 'connections', label: t('settings.adminItems.llmConnections.title'), icon: Plug },
    { id: 'features', label: t('settings.adminItems.aiFeatures.title'), icon: Sparkles },
    { id: 'agent-templates', label: t('settings.adminItems.agentTemplates.title'), icon: Bot },
    { id: 'staleness', label: t('settings.adminItems.workItemStaleness.title'), icon: Gauge },
  ]);

  function tabFromRoute(route) {
    const requestedTab = route.query?.subtab;
    return VALID_TABS.has(requestedTab) ? requestedTab : 'connections';
  }

  function setRouteTab(tab) {
    if (!VALID_TABS.has(tab)) return;
    updateQueryParams({ subtab: tab === 'connections' ? null : tab });
  }

  $effect(() => {
    subtab = tabFromRoute($currentRoute);
  });
</script>

<div class="space-y-4">
  <Tabs {tabs} bind:activeTab={subtab} onTabChange={({ tab }) => setRouteTab(tab)}>
    {#if subtab === 'features'}
      <AIFeaturesSettings />
    {:else if subtab === 'agent-templates'}
      <AgentTemplateManager />
    {:else if subtab === 'staleness'}
      <WorkItemStalenessSettings />
    {:else}
      {#if capabilitiesStore.operatorAllows('ai_connection_management')}
        <LLMConnectionManager />
      {:else}
        <div data-testid="operator-controls-ai-restricted"><AlertBox variant="info" message={t('operatorControls.restricted', 'This capability is managed by the instance operator.')} /></div>
      {/if}
    {/if}
  </Tabs>
</div>
