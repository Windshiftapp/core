<script>
  import { currentRoute } from '../../router.js';
  import TabNav from '../../components/TabNav.svelte';
  import StatusCategoryManager from '../../settings/StatusCategoryManager.svelte';
  import StatusManager from '../../settings/StatusManager.svelte';
  import { t } from '../../stores/i18n.svelte.js';

  // Get active subtab from URL query params, default to 'statuses'
  let subtab = $derived($currentRoute.query?.subtab || 'statuses');

  let tabs = $derived([
    { id: 'statuses', label: t('statuses.statuses') },
    { id: 'status-categories', label: t('statuses.statusCategories') }
  ]);
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
