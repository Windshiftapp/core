<script>
  import { currentRoute, navigate } from '../router.js';
  import PermissionManager from '../settings/PermissionManager.svelte';
  import PermissionSetManager from '../settings/PermissionSetManager.svelte';

  // Get active subtab from URL query params, default to 'permissions'
  $: subtab = $currentRoute.query?.subtab || 'permissions';

  function switchSubtab(newSubtab) {
    navigate(`/admin/permissions?subtab=${newSubtab}`);
  }

  const tabs = [
    { id: 'permissions', label: 'Permissions' },
    { id: 'permission-sets', label: 'Permission Sets' }
  ];
</script>

<div class="space-y-6">
  <!-- Tab Navigation -->
  <div class="border-b" style="border-color: var(--ds-border);">
    <nav class="-mb-px flex space-x-8" aria-label="Tabs">
      {#each tabs as tab}
        <button
          onclick={() => switchSubtab(tab.id)}
          class="whitespace-nowrap py-4 px-1 border-b-2 font-medium text-sm transition-colors {
            subtab === tab.id
              ? 'border-blue-500 text-blue-600'
              : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'
          }"
          aria-current={subtab === tab.id ? 'page' : undefined}
        >
          {tab.label}
        </button>
      {/each}
    </nav>
  </div>

  <!-- Tab Content -->
  <div>
    {#if subtab === 'permissions'}
      <PermissionManager />
    {:else if subtab === 'permission-sets'}
      <PermissionSetManager />
    {/if}
  </div>
</div>
