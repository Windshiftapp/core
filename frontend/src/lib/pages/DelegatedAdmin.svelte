<script>
  import { untrack } from 'svelte';
  import { currentRoute, navigate } from '../router.js';
  import LinkComponent from '../components/Link.svelte';
  import LazyRootView from '../components/LazyRootView.svelte';
  import Spinner from '../components/Spinner.svelte';
  import ScrollableSidebar from '../layout/ScrollableSidebar.svelte';
  import SidebarHeader from '../layout/SidebarHeader.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { permissionStore } from '../stores/permissions.svelte.js';
  import { visibleDelegatedAdminItems } from '../navigation/delegatedAdminNavigation.js';
  import { DELEGATED_ADMIN_COMPONENT_LOADERS } from '../admin/delegatedAdminComponentRoutes.js';
  import UnauthorizedAccess from './UnauthorizedAccess.svelte';

  const items = $derived($visibleDelegatedAdminItems);
  const activeTab = $derived($currentRoute.params?.tab || null);
  const activeItem = $derived(items.find((item) => item.id === activeTab) || null);
  const loading = $derived($permissionStore.loading && items.length === 0);

  // Keep the URL and the rendered surface in sync: `/manage` and unknown tabs
  // fall back to the first surface the user can actually use.
  // The shell stays mounted across sections (#542), so a new section would
  // otherwise open at the previous one's scroll position.
  let contentEl = $state(null);
  $effect(() => {
    void activeItem?.id;
    untrack(() => {
      if (contentEl) contentEl.scrollTop = 0;
    });
  });

  $effect(() => {
    if (items.length === 0) return;
    if (!activeItem) {
      navigate(items[0].href, { replace: true });
    }
  });
</script>

{#snippet sidebarHeader()}
  <div class="px-6 pt-6 pb-3">
    <SidebarHeader
      title={t('nav.delegatedAdmin')}
      description={t('nav.delegatedAdminSubtitle')}
      noBorder
    />
  </div>
{/snippet}

{#if loading}
  <div class="flex h-full items-center justify-center" style="background-color: var(--ds-surface);">
    <Spinner />
  </div>
{:else if items.length === 0}
  <UnauthorizedAccess message={t('nav.delegatedAdminUnauthorized')} />
{:else}
  <div
    class="delegated-admin-shell flex h-full min-h-0 min-w-0 flex-col overflow-hidden md:flex-row"
    style="background-color: var(--ds-surface);"
    data-testid="delegated-admin-shell"
  >
    <ScrollableSidebar
      as="aside"
      id="delegated-admin-navigation"
      class="delegated-admin-sidebar w-full flex-shrink-0 border-b md:w-56 md:border-b-0 md:border-r"
      style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);"
      aria-label={t('nav.delegatedAdmin')}
      header={sidebarHeader}
      scrollClass="px-4 md:px-6"
      scrollTestid="delegated-admin-navigation-scroll"
    >
      <nav
        class="flex gap-1 overflow-x-auto pb-4 md:flex-col md:space-y-1 md:overflow-visible"
        aria-label={t('nav.delegatedAdmin')}
      >
        {#each items as item (item.id)}
          {@const Icon = item.icon}
          {@const isActive = item.id === activeTab}
          <LinkComponent
            data-testid="delegated-admin-navigation-item"
            id="delegated-admin-nav-{item.id}"
            href={item.href}
            active={isActive}
            class="delegated-admin-nav-item flex w-full flex-shrink-0 items-center whitespace-nowrap rounded-lg px-3 py-2 text-sm font-medium transition-all cursor-pointer {isActive
              ? 'active'
              : ''}"
          >
            <Icon size={16} stroke={1.5} class="-ml-1 mr-3 flex-shrink-0" aria-hidden="true" />
            <span>{t(item.labelKey)}</span>
          </LinkComponent>
        {/each}
      </nav>
    </ScrollableSidebar>

    <div class="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
      <div bind:this={contentEl} class="delegated-admin-content min-w-0 flex-1 overflow-y-auto px-6 py-6 lg:px-16 lg:py-12">
        {#if activeItem}
          {#key activeItem.id}
            <LazyRootView
              loader={DELEGATED_ADMIN_COMPONENT_LOADERS[activeItem.id]}
              label={t(activeItem.labelKey)}
            />
          {/key}
        {/if}
      </div>
    </div>
  </div>
{/if}

<style>
  /* Rendered inside the Link component, so the rules must be global. */
  :global(.delegated-admin-nav-item) {
    color: var(--ds-text-subtle);
  }

  :global(.delegated-admin-nav-item:hover:not(.active)) {
    background: var(--ds-background-neutral-hovered);
    color: var(--ds-text);
  }

  :global(.delegated-admin-nav-item.active) {
    background: var(--ds-surface-selected);
    color: var(--ds-text);
  }
</style>
