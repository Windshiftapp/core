<script>
  import { currentRoute, updateQueryParams } from '../router.js';
  import PermissionManager from '../settings/PermissionManager.svelte';
  import Tabs from '../components/Tabs.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import ShieldCheck from '@lucide/svelte/icons/shield-check';

  // Active subtab is read from the URL query so deep links keep working.
  const VALID_TABS = new Set(['permissions']);
  // Permission sets hidden for now - using role-based permissions instead
  // { id: 'permission-sets', label: t('permissions.permissionSet') + 's' }

  let subtab = $state('permissions');

  const tabs = $derived([
    { id: 'permissions', label: t('permissions.title'), icon: ShieldCheck }
  ]);

  function tabFromRoute(route) {
    const requestedTab = route.query?.subtab;
    return VALID_TABS.has(requestedTab) ? requestedTab : 'permissions';
  }

  function setRouteTab(tab) {
    if (!VALID_TABS.has(tab)) return;
    updateQueryParams({ subtab: tab === 'permissions' ? null : tab });
  }

  $effect(() => {
    subtab = tabFromRoute($currentRoute);
  });
</script>

<div class="space-y-4">
  <Tabs {tabs} bind:activeTab={subtab} onTabChange={({ tab }) => setRouteTab(tab)}>
    {#if subtab === 'permissions'}
      <PermissionManager />
    {/if}
    <!-- Permission sets hidden for now - using role-based permissions instead -->
    <!-- {:else if subtab === 'permission-sets'}
      <PermissionSetManager />
    -->
  </Tabs>
</div>
