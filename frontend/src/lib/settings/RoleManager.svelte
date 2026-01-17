<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { BadgeCheck, Eye, CheckCircle } from 'lucide-svelte';
  import DataTable from '../components/DataTable.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let roles = $state([]);
  let loading = $state(true);
  let selectedRole = $state(null);
  let rolePermissions = $state([]);

  const columns = $derived([
    { key: 'name', label: t('roles.roleName'), sortable: true },
    { key: 'description', label: t('common.description') },
    {
      key: 'is_system',
      label: t('common.type'),
      render: (item) => item.is_system ? t('common.default') : t('common.custom'),
      sortable: true
    },
    { key: 'actions', label: '', width: 'w-16' }
  ]);

  onMount(async () => {
    await loadRoles();
  });

  async function loadRoles() {
    try {
      loading = true;
      const data = await api.get('/workspace-roles') || [];
      roles = data;
    } catch (error) {
      console.error('Failed to load workspace roles:', error);
      roles = [];
    } finally {
      loading = false;
    }
  }

  async function viewRoleDetails(role) {
    try {
      const fullRole = await api.get(`/workspace-roles/${role.id}`);
      selectedRole = fullRole;
      rolePermissions = fullRole.permissions || [];
    } catch (error) {
      console.error('Failed to load role details:', error);
      alert(t('dialogs.alerts.failedToLoad', { error: error.message || error }));
    }
  }

  function closeDetails() {
    selectedRole = null;
    rolePermissions = [];
  }

  function buildRoleDropdownItems(role) {
    return [
      {
        id: 'view',
        label: t('common.view'),
        icon: Eye,
        action: () => viewRoleDetails(role)
      }
    ];
  }
</script>

<div class="space-y-6">
  <PageHeader
    title={t('roles.title')}
    description={t('roles.subtitle')}
    icon={BadgeCheck}
  />

  {#if selectedRole}
    <div class="border rounded p-6 shadow-sm" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
      <div class="flex justify-between items-start mb-4">
        <div>
          <h3 class="text-lg font-semibold" style="color: var(--ds-text);">{selectedRole.name}</h3>
          <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">{selectedRole.description}</p>
        </div>
        <button
          onclick={closeDetails}
          class="close-btn"
          style="color: var(--ds-icon-subtle);"
        >
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>

      <div class="border-t pt-4" style="border-color: var(--ds-border);">
        <h4 class="font-medium mb-3" style="color: var(--ds-text);">{t('roles.permissions')}</h4>
        <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
          {#each rolePermissions as permission}
            <div class="flex items-start space-x-2 p-3 rounded-md" style="background-color: var(--ds-interactive-subtle);">
              <CheckCircle class="w-5 h-5 mt-0.5" style="color: var(--ds-text-success);" />
              <div>
                <div class="font-medium text-sm" style="color: var(--ds-text);">{permission.permission_name}</div>
                <div class="text-xs" style="color: var(--ds-text-subtle);">{permission.description}</div>
                <div class="text-xs mt-0.5" style="color: var(--ds-text-subtlest);">{permission.permission_key}</div>
              </div>
            </div>
          {/each}
        </div>
      </div>
    </div>
  {/if}

  <DataTable
    data={roles}
    {columns}
    {loading}
    actionItems={buildRoleDropdownItems}
    emptyMessage={t('roles.noRoles')}
  />

  <AlertBox type="info">
    <h4 class="font-semibold mb-2">{t('roles.title')}</h4>
    <p class="text-sm mb-2">
      {t('roles.subtitle')}
    </p>
  </AlertBox>
</div>

<style>
  .close-btn:hover {
    color: var(--ds-icon);
  }
</style>
