<script>
  import { currentRoute } from '../router.js';
  import TabNav from '../components/TabNav.svelte';
  import LLMConnectionManager from './LLMConnectionManager.svelte';
  import AIFeaturesSettings from './AIFeaturesSettings.svelte';
  import AgentTemplateManager from './AgentTemplateManager.svelte';
  import WorkItemStalenessSettings from './WorkItemStalenessSettings.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { capabilitiesStore } from '../stores/capabilities.svelte.js';
  import AlertBox from '../components/AlertBox.svelte';

  let subtab = $derived($currentRoute.query?.subtab || 'connections');

  let tabs = $derived([
    { id: 'connections', label: t('settings.adminItems.llmConnections.title') },
    { id: 'features', label: t('settings.adminItems.aiFeatures.title') },
    { id: 'agent-templates', label: t('settings.adminItems.agentTemplates.title') },
    { id: 'staleness', label: t('settings.adminItems.workItemStaleness.title') },
  ]);
</script>

<div class="space-y-6">
  <TabNav {tabs} basePath="/admin/llm-connections" defaultTab="connections" />

  <div>
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
  </div>
</div>
