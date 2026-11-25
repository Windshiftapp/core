<script>
  import { currentRoute, updateQueryParams } from '../../router.js';
  import Tabs from '../../components/Tabs.svelte';
  import StatusCategoryManager from '../../settings/StatusCategoryManager.svelte';
  import StatusManager from '../../settings/StatusManager.svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import CircleDot from '@lucide/svelte/icons/circle-dot';
  import Tags from '@lucide/svelte/icons/tags';

  // Active subtab lives in the URL query so deep links keep working, matching
  // the local-state-plus-URL-sync pattern used by ActionCapabilitiesManager.
  const VALID_TABS = new Set(['statuses', 'status-categories']);

  let subtab = $state('statuses');

  const tabs = $derived([
    { id: 'statuses', label: t('statuses.statuses'), icon: CircleDot },
    { id: 'status-categories', label: t('statuses.statusCategories'), icon: Tags }
  ]);

  function tabFromRoute(route) {
    const requestedTab = route.query?.subtab;
    return VALID_TABS.has(requestedTab) ? requestedTab : 'statuses';
  }

  function setRouteTab(tab) {
    if (!VALID_TABS.has(tab)) return;
    updateQueryParams({ subtab: tab === 'statuses' ? null : tab });
  }

  $effect(() => {
    subtab = tabFromRoute($currentRoute);
  });
</script>

<div class="space-y-4">
  <Tabs {tabs} bind:activeTab={subtab} onTabChange={({ tab }) => setRouteTab(tab)}>
    {#if subtab === 'status-categories'}
      <StatusCategoryManager />
    {:else}
      <StatusManager />
    {/if}
  </Tabs>
</div>
