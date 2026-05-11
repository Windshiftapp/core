<script>
  import { permissionStore } from '../stores';
  import UnauthorizedAccess from '../pages/UnauthorizedAccess.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let { permissionKey = null, permissionId = null, requireSystemAdmin = false, children, fallback = null } = $props();

  let hasAccess = $derived.by(() => {
    if (requireSystemAdmin) {
      return $permissionStore.isSystemAdmin;
    }

    if (permissionKey) {
      return $permissionStore.userPermissionKeys?.has(permissionKey) || $permissionStore.isSystemAdmin;
    }

    if (permissionId) {
      return $permissionStore.userPermissions?.has(permissionId) || $permissionStore.isSystemAdmin;
    }

    return true;
  });

  let requiredPermissionDisplay = $derived(permissionKey || (requireSystemAdmin ? 'system.admin' : null));
</script>

{#if hasAccess}
  {@render children?.()}
{:else if fallback}
  {@render fallback(requiredPermissionDisplay)}
{:else}
  <UnauthorizedAccess
    message={t('permissions.noAccessMessage')}
    requiredPermission={requiredPermissionDisplay}
  />
{/if}