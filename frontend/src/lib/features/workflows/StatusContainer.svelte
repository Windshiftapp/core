<script>
  import { currentRoute } from '../../router.js';
  import TabNav from '../../components/TabNav.svelte';
  import StatusCategoryManager from '../../settings/StatusCategoryManager.svelte';
  import StatusManager from '../../settings/StatusManager.svelte';

  // Get active subtab from URL query params, default to 'statuses'
  $: subtab = $currentRoute.query?.subtab || 'statuses';

  const tabs = [
    { id: 'statuses', label: 'Statuses' },
    { id: 'status-categories', label: 'Status Categories' }
  ];
</script>

<div class="space-y-6">
  <!-- Tab Navigation -->
  <TabNav {tabs} basePath="/admin/statuses" defaultTab="statuses" />

  <!-- Tab Content -->
  <div>
    {#if subtab === 'status-categories'}
      <StatusCategoryManager />
    {:else if subtab === 'statuses'}
      <StatusManager />
    {/if}
  </div>
</div>
