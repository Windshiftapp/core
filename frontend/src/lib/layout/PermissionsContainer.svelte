<script>
  import { currentRoute } from '../router.js';
  import PermissionManager from '../settings/PermissionManager.svelte';
  import TabNav from '../components/TabNav.svelte';
  import { t } from '../stores/i18n.svelte.js';

  // Active subtab is read from the URL query so deep links keep working.
  let subtab = $derived($currentRoute.query?.subtab || 'permissions');

  let tabs = $derived([
    { id: 'permissions', label: t('permissions.title') }
    // Permission sets hidden for now - using role-based permissions instead
    // { id: 'permission-sets', label: t('permissions.permissionSet') + 's' }
  ]);
</script>

<div class="space-y-6">
  <TabNav {tabs} basePath="/admin/permissions" paramName="subtab" defaultTab="permissions" />

  <!-- Tab Content -->
  <div>
    {#if subtab === 'permissions'}
      <PermissionManager />
    {/if}
    <!-- Permission sets hidden for now - using role-based permissions instead -->
    <!-- {:else if subtab === 'permission-sets'}
      <PermissionSetManager />
    -->
  </div>
</div>
