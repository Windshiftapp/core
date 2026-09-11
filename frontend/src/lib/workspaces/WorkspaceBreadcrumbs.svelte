<script>
  import { ChevronRight } from '@lucide/svelte';
  import DropdownMenu from '../layout/DropdownMenu.svelte';
  import { workspaceMenuItems } from '../navigation/workspaceMenu.js';
  import { currentRoute } from '../router.js';
  import { workspacesStore } from '../stores';
  import { t } from '../stores/i18n.svelte.js';

  let { workspace = null, onOpen = () => {} } = $props();
  let search = $state('');
  let open = $state(false);
  const items = $derived(workspaceMenuItems(
    $workspacesStore.regularWorkspaces,
    search,
    (value) => search = value,
    t
  ));

  $effect(() => {
    $currentRoute.path;
    open = false;
    search = '';
  });
</script>

<nav class="workspace-breadcrumbs" aria-label={t('nav.workspaces')} data-testid="workspace-breadcrumbs">
  <ol>
    <li class="shrink-0">
      <DropdownMenu
        triggerText={t('nav.workspaces')}
        triggerTestid="workspace-breadcrumb-picker"
        triggerClass="!px-2 !py-1.5 !text-sm rounded hover:bg-[var(--ds-background-neutral-hovered)]"
        triggerStyle="color: var(--ds-text-subtle);"
        placement="bottom-start"
        maxWidth="max-w-xs"
        {items}
        bind:isOpen={open}
        onOpenChange={(value) => { if (value) onOpen(); }}
      />
    </li>
    {#if workspace}
      <li class="min-w-0 flex items-center gap-2">
        <ChevronRight size={14} class="shrink-0" aria-hidden="true" />
        <a
          href={`/workspaces/${workspace.id}`}
          aria-current="page"
          data-testid="workspace-breadcrumb-current"
          title={workspace.name}
          class="truncate rounded px-2 py-1.5 font-medium hover:bg-[var(--ds-background-neutral-hovered)]"
        >{workspace.name}</a>
      </li>
    {/if}
  </ol>
</nav>

<style>
  .workspace-breadcrumbs {
    display: flex;
    align-items: center;
    flex-shrink: 0;
    height: 3rem;
    min-width: 0;
    padding: 0 1rem;
    border-bottom: 1px solid var(--ds-border);
    background: var(--ds-surface);
    color: var(--ds-text-subtle);
    font-size: 0.875rem;
  }
  ol {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    min-width: 0;
    margin: 0;
    padding: 0;
    list-style: none;
  }
  a { color: var(--ds-text); text-decoration: none; }
</style>
