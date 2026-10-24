<script>
  import { currentRoute } from '../router.js';
  import { workspacePermissions } from '../stores/workspacePermissions.svelte.js';
  import {
    visibleWorkspaceSettingsItems,
    workspaceSettingsRoute,
  } from '../navigation/workspaceNavigation.js';
  import { t } from '../stores/i18n.svelte.js';

  let { workspaceId = null } = $props();

  const canAdmin = $derived(workspacePermissions.canAdminWorkspace(workspaceId));
  const items = $derived(
    visibleWorkspaceSettingsItems(workspaceId, (wsId, permission) =>
      workspacePermissions.hasPermission(wsId, permission)
    )
  );
</script>

{#if canAdmin}
  <nav class="px-4 pt-2 pb-2 space-y-1" data-testid="workspace-admin-nav" aria-label={t('workspaceSettings.title')}>
    {#each items as item (item.id)}
      {@const ItemIcon = item.icon}
      {@const active = $currentRoute.view === item.view}
      <a
        href={workspaceSettingsRoute(workspaceId, item.id)}
        class="ws-nav-item w-full text-left cursor-pointer px-3 py-2 rounded-lg text-sm font-medium flex items-center gap-2 workspace-nav-item no-underline"
        class:active={active}
        class:danger={item.danger}
        data-testid="workspace-admin-nav-{item.id}"
        aria-current={active ? 'page' : undefined}
      >
        <ItemIcon class="w-4 h-4" />
        {t(item.labelKey)}
      </a>
    {/each}
  </nav>
{/if}

<style>
  .ws-nav-item {
    color: var(--ds-text-subtle);
  }

  .ws-nav-item.danger {
    color: var(--ds-text-danger);
  }

  .ws-nav-item:hover:not(.active):not(.danger) {
    background: var(--ds-background-neutral-hovered);
    color: var(--ds-text);
  }

  .ws-nav-item.danger:hover:not(.active) {
    background: var(--ds-background-neutral-hovered);
  }

  .ws-nav-item.active:not(.danger) {
    background: var(--ds-surface-selected);
    color: var(--ds-text);
  }
</style>
