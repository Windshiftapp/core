<script>
  import { permissionStore } from '../stores';
  import UnauthorizedAccess from '../pages/UnauthorizedAccess.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let { permissionKey = null, permissionId = null, requireSystemAdmin = false, children, fallback } = $props();

  let hasAccess = $derived((() => {
    if (requireSystemAdmin) {
      return $permissionStore.isSystemAdmin;
    }

    if (permissionKey) {
      return permissionStore.hasPermissionKey(permissionKey);
    }

    if (permissionId) {
      return permissionStore.hasPermission(permissionId);
    }

    return true;
  })());

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