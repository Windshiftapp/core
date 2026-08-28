<script>
  import Tabs from '../components/Tabs.svelte';
  import CapabilityManager from './CapabilityManager.svelte';
  import ActionCredentialManager from './ActionCredentialManager.svelte';
  import RunnerPoolManager from './RunnerPoolManager.svelte';
  import { currentRoute, updateQueryParams } from '../router.js';
  import { Bolt, KeyRound, Server } from '@lucide/svelte';
  import { useEventListener } from 'runed';
  import { t } from '../stores/i18n.svelte.js';

  const VALID_TABS = new Set(['capabilities', 'credentials', 'runners']);

  let activeTab = $state('capabilities');

  const tabs = $derived([
    { id: 'capabilities', label: t('settings.adminOperations.tabs.capabilities'), icon: Bolt },
    { id: 'credentials', label: t('settings.adminOperations.tabs.credentials'), icon: KeyRound },
    { id: 'runners', label: t('settings.adminOperations.tabs.runnerPools'), icon: Server },
  ]);

  function tabFromRoute(route) {
    const requestedTab = route.query?.tab;
    return VALID_TABS.has(requestedTab) ? requestedTab : 'capabilities';
  }

  function setRouteTab(tab) {
    if (!VALID_TABS.has(tab)) return;
    updateQueryParams({ tab: tab === 'capabilities' ? null : tab });
  }

  $effect(() => {
    activeTab = tabFromRoute($currentRoute);
  });

  useEventListener(() => window, 'action-capabilities-tab-switch', (/** @type {CustomEvent<{tab?: string}>} */ event) => {
    setRouteTab(event.detail?.tab);
  });
</script>

<div class="space-y-4">
  <Tabs {tabs} bind:activeTab onTabChange={({ tab }) => setRouteTab(tab)}>
    {#if activeTab === 'capabilities'}
      <CapabilityManager />
    {:else if activeTab === 'credentials'}
      <ActionCredentialManager />
    {:else if activeTab === 'runners'}
      <RunnerPoolManager />
    {/if}
  </Tabs>
</div>
