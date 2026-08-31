<script>
  import { currentRoute, isWorkspaceRoute } from '../router.js';
  import { permissionStore, uiStore, workspacesStore } from '../stores';
  import { t } from '../stores/i18n.svelte.js';
  import { aiStore } from '../stores/aiStore.svelte.js';
  import { getShortcutDisplay } from '../utils/keyboardShortcuts.js';
  import { workspaceIconMap } from '../utils/icons.js';
  import { isTauri as getIsTauri } from '../utils/isTauri.js';
  import DropdownMenu from './DropdownMenu.svelte';
  import Tooltip from '../components/Tooltip.svelte';
  import NavLink from './NavLink.svelte';
  import UserAvatar from '../components/UserAvatar.svelte';
  import NotificationTray from '../features/notifications/NotificationTray.svelte';
  import ScrollableSidebar from './ScrollableSidebar.svelte';
  import {
    IconSearch, IconSettings, IconPlus, IconGridDots, IconUserScan,
    IconFolders, IconLayoutSidebarLeftExpand, IconLayoutSidebarLeftCollapse,
    IconMessage, IconTerminal2,
  } from '@tabler/icons-svelte-runes';
  import { mainNavItems, bottomNavItems } from '../navigation/mainNavigation.js';

  let {
    onShowCommandPalette = () => {},
    onShowCreateModal = () => {},
    onShowChatPanel = () => {},
    onToggleTerminal = () => {},
    activeSurface = null,
    onSurfaceChange = () => {},
  } = $props();

  const isTauri = getIsTauri();

  let workspaceSearchQuery = $state('');

  // Derived workspace dropdown items that automatically updates when store or search changes
  const workspacesDropdownItems = $derived.by(() => {
    const items = [];

    // Add search input at the top
    items.push({
      type: 'search',
      id: 'search',
      testid: 'workspaces-search',
      placeholder: t('nav.searchWorkspaces'),
      value: workspaceSearchQuery,
      onInput: (value) => {
        workspaceSearchQuery = value;
      }
    });

    // Filter workspaces based on search query (inactive workspaces are
    // hidden here even for admins — Manage Workspaces is the only surface
    // that shows them).
    const activeRegularWorkspaces = $workspacesStore.regularWorkspaces.filter(ws => ws.active);
    const search = workspaceSearchQuery?.trim().toLowerCase();
    const filteredWorkspaces = !search
      ? activeRegularWorkspaces
      : activeRegularWorkspaces.filter(workspace => {
          const nameMatch = workspace.name?.toLowerCase().includes(search);
          const keyMatch = workspace.key?.toLowerCase().includes(search);
          const descriptionMatch = workspace.description?.toLowerCase().includes(search);
          return nameMatch || keyMatch || descriptionMatch;
        });

    // Add workspace items
    if (filteredWorkspaces.length > 0) {
      const maxVisible = 10;
      const hasMore = filteredWorkspaces.length > maxVisible;
      const visibleWorkspaces = filteredWorkspaces.slice(0, maxVisible);
      const workspaceItems = visibleWorkspaces.map(workspace => {
        const hasAvatar = workspace.avatar_url;
        const workspaceIcon = workspaceIconMap[workspace.icon] || workspaceIconMap.Package;

        return {
          id: workspace.id,
          type: 'regular',
          testid: 'workspace-dropdown-item',
          icon: hasAvatar ? null : workspaceIcon,
          iconColor: hasAvatar ? null : workspace.color,
          avatarUrl: hasAvatar ? workspace.avatar_url : null,
          title: workspace.name,
          subtitle: workspace.description,
          href: `/workspaces/${workspace.id}`
        };
      });

      items.push({ type: 'group', items: workspaceItems });
      if (hasMore) {
        items.push({ type: 'text', text: t('nav.searchToFindMore') });
      }
      items.push({ type: 'divider' });
    } else if (activeRegularWorkspaces.length > 0 && workspaceSearchQuery) {
      // Show "no results" only if there are workspaces but search didn't match
      items.push(
        { type: 'text', text: t('nav.noWorkspacesMatch') },
        { type: 'divider' }
      );
    } else if (activeRegularWorkspaces.length === 0) {
      items.push(
        { type: 'text', text: t('nav.noWorkspacesFound') },
        { type: 'divider' }
      );
    }

    // Add combined manage workspaces action
    items.push({
      id: 'manage',
      type: 'regular',
      icon: IconSettings,
      title: t('nav.manageWorkspaces'),
      subtitle: t('nav.manageWorkspacesSubtitle'),
      color: 'var(--ds-text-link)',
      class: 'font-medium',
      href: '/workspaces'
    });

    return items;
  });

  // Filter nav items based on permissions (registry: navigation/mainNavigation.js)
  const filteredMainNav = $derived(
    mainNavItems.filter(item => !item.permission || $permissionStore[item.permission])
  );

  const filteredBottomNav = $derived(
    bottomNavItems.filter(item => !item.permission || $permissionStore[item.permission])
  );

  function showCreateDropdown() {
    onShowCreateModal();
  }

  function setPopoverSurface(surface, open) {
    if (open) {
      onSurfaceChange(surface);
    } else if (activeSurface === surface) {
      onSurfaceChange(null);
    }
  }

  function closePopoverSurface() {
    if (activeSurface === 'workspaces' || activeSurface === 'notifications' || activeSurface === 'profile') {
      onSurfaceChange(null);
    }
  }
</script>

{#snippet sidebarHeader()}
  <!-- Logo -->
  <Tooltip content="Windshift" placement="right" disabled={$uiStore.navExpanded}>
    <a
      href="/"
      onclick={closePopoverSurface}
      class="flex items-center justify-start px-4 w-full h-10 mb-2 hover:opacity-80 transition-opacity cursor-pointer"
    >
      <img src="windshift-3.svg" alt="Windshift" class="w-8 h-8 flex-shrink-0" />
      {#if $uiStore.navExpanded}
        <span class="ml-3 font-semibold text-sm whitespace-nowrap">Windshift</span>
      {/if}
    </a>
  </Tooltip>
{/snippet}

{#snippet sidebarContent()}
  <!-- Main Navigation -->
  <div class="flex flex-col items-stretch px-2.5 space-y-1 py-4">

    <!-- Workspaces -->
    <Tooltip content={t('nav.workspaces')} placement="right" disabled={$uiStore.navExpanded}>
      <div class="w-full">
        <DropdownMenu
          triggerIcon={IconGridDots}
          triggerIconClass="w-5 h-5"
          triggerGap="gap-3"
          triggerText={$uiStore.navExpanded ? t('nav.workspaces') : ''}
          triggerLabel={t('nav.workspaces')}
          triggerClass="w-full px-3 h-10 rounded flex items-center justify-start cursor-pointer nav-button nav-button-emphasized {isWorkspaceRoute($currentRoute.view) || activeSurface === 'workspaces' ? 'nav-button-selected' : ''} {!$workspacesStore.loaded ? 'opacity-50 cursor-wait' : ''}"
          triggerTestid="workspaces-dropdown-trigger"
          items={workspacesDropdownItems}
          maxWidth="max-w-xs"
          showChevron={false}
          placement="right-start"
          iconOnly={!$uiStore.navExpanded}
          triggerAlignment={$uiStore.navExpanded ? 'start' : 'center'}
          isOpen={activeSurface === 'workspaces'}
          onOpenChange={(open) => setPopoverSurface('workspaces', open)}
        />
      </div>
    </Tooltip>

    <!-- Main Nav Links -->
    {#each filteredMainNav as item (item.id)}
      <NavLink
        id="nav-{item.id}"
        icon={item.icon}
        label={t(item.labelKey)}
        href={item.href}
        isActive={item.activeViews.includes($currentRoute.view)}
        expanded={$uiStore.navExpanded}
        onclick={closePopoverSurface}
      />
    {/each}

    <!-- Global actions share the same rhythm as navigation, but remain a
         distinct task group instead of a second navigation section. -->
    <div class="sidebar-quick-actions flex flex-col items-stretch space-y-1 my-3 py-3 border-y">
      <NavLink
        id="global-create-button"
        icon={IconPlus}
        label={t('nav.create')}
        onclick={showCreateDropdown}
        expanded={$uiStore.navExpanded}
        variant="accent"
        isActive={activeSurface === 'create'}
        shortcut={getShortcutDisplay('global', 'create')}
        tooltipSuffix=" ({getShortcutDisplay('global', 'create')})"
      />
      <NavLink
        icon={IconSearch}
        label={t('nav.search')}
        onclick={onShowCommandPalette}
        expanded={$uiStore.navExpanded}
        isActive={activeSurface === 'search'}
        shortcut={getShortcutDisplay('global', 'commandPalette')}
        tooltipSuffix=" ({getShortcutDisplay('global', 'commandPalette')} or Space Space)"
      />
      {#if aiStore.chatAvailable}
        <NavLink
          id="chat-toggle-button"
          icon={IconMessage}
          label={t('nav.aiChat')}
          onclick={onShowChatPanel}
          expanded={$uiStore.navExpanded}
          isActive={activeSurface === 'chat'}
          shortcut={getShortcutDisplay('global', 'aiChat')}
          tooltipSuffix=" ({getShortcutDisplay('global', 'aiChat')})"
        />
      {/if}
      {#if isTauri}
      <NavLink
        icon={IconTerminal2}
        label={t('nav.terminal')}
        onclick={onToggleTerminal}
        expanded={$uiStore.navExpanded}
        tooltipSuffix=" (Cmd+`)"
      />
      {/if}
    </div>
  </div>
{/snippet}

{#snippet sidebarFooter()}
  <!-- Bottom Section -->
  <div class="flex flex-col items-stretch px-2.5 space-y-1 pt-2">
    <!-- Nav Toggle Button -->
    <button
      onclick={() => uiStore.toggleNavExpanded()}
      class="flex items-center justify-start w-full px-3 h-10 mb-2 rounded cursor-pointer nav-button"
      aria-label={$uiStore.navExpanded ? t('nav.collapse') : t('nav.expand')}
    >
      {#if $uiStore.navExpanded}
        <IconLayoutSidebarLeftCollapse class="w-5 h-5 flex-shrink-0" />
        <span class="ml-3 text-sm whitespace-nowrap">{t('nav.collapse')}</span>
      {:else}
        <IconLayoutSidebarLeftExpand class="w-5 h-5" />
      {/if}
    </button>
    <!-- Bottom Nav Links -->
    {#each filteredBottomNav as item (item.id)}
      <NavLink
        id="nav-{item.id}"
        icon={item.icon}
        label={t(item.labelKey)}
        href={item.href}
        isActive={item.activeViews.includes($currentRoute.view)}
        expanded={$uiStore.navExpanded}
        onclick={closePopoverSurface}
      />
    {/each}

    <!-- Notification Tray -->
    <Tooltip content={t('nav.notifications')} placement="right" disabled={$uiStore.navExpanded}>
      <NotificationTray
        expanded={$uiStore.navExpanded}
        label={t('nav.notifications')}
        isOpen={activeSurface === 'notifications'}
        onOpenChange={(open) => setPopoverSurface('notifications', open)}
      />
    </Tooltip>

    <!-- User Profile Avatar -->
    <Tooltip content={t('nav.profile')} placement="right" disabled={$uiStore.navExpanded}>
      <UserAvatar
        expanded={$uiStore.navExpanded}
        label={t('nav.profile')}
        isOpen={activeSurface === 'profile'}
        onOpenChange={(open) => setPopoverSurface('profile', open)}
      />
    </Tooltip>
  </div>
{/snippet}

<ScrollableSidebar
  as="nav"
  class="main-sidebar {$uiStore.navExpanded ? 'w-[200px]' : 'w-16'} shadow-lg border-r py-4 fixed inset-y-0 z-40 themed-nav transition-[width] duration-200 ease-[cubic-bezier(0.25,1,0.5,1)] motion-reduce:transition-none"
  style="border-color: var(--ds-border);"
  aria-label={t('aria.mainNavigation')}
  header={sidebarHeader}
  footer={sidebarFooter}
  scrollTestid="main-navigation-scroll"
>
  {@render sidebarContent()}
</ScrollableSidebar>

<style>
  :global(.main-sidebar) {
    height: 100vh;
    height: 100dvh;
  }

  .sidebar-quick-actions {
    border-color: color-mix(in srgb, var(--ds-border) 75%, transparent);
  }

  @media (max-width: 767px) {
    :global(.main-sidebar) {
      width: 4rem;
    }
  }
</style>
