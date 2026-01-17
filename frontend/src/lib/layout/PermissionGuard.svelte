<script>
  import { permissionStore } from '../stores';
  import UnauthorizedAccess from '../pages/UnauthorizedAccess.svelte';
  import { t } from '../stores/i18n.svelte.js';

  export let permissionKey = null;
  export let permissionId = null;
  export let requireSystemAdmin = false;

  $: hasAccess = (() => {
    if (requireSystemAdmin) {
      return $permissionStore.isSystemAdmin;
    }

    if (permissionKey) {
      return permissionStore.hasPermissionKey(permissionKey);
    }

    if (permissionId) {
      return permissionStore.hasPermission(permissionId);
    }

    // If no specific permission is required, allow access
    return true;
  })();

  $: requiredPermissionDisplay = permissionKey || (requireSystemAdmin ? 'system.admin' : null);
</script>

{#if hasAccess}
  <slot />
{:else}
  <slot name="fallback" {requiredPermissionDisplay}>
    <!-- Default fallback if no custom fallback provided -->
    <UnauthorizedAccess
      message={t('permissions.noAccessMessage')}
      requiredPermission={requiredPermissionDisplay}
    />
  </slot>
{/if}