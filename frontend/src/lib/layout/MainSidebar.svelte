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
    onToggleTerminal = () => {}
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
</script>

<nav class="main-sidebar {$uiStore.navExpanded ? 'w-[200px]' : 'w-16'} shadow-lg border-r flex flex-col py-4 fixed h-full z-40 themed-nav transition-all duration-200 overflow-x-hidden" style="border-color: var(--ds-border);" aria-label={t('aria.mainNavigation')}>
  <!-- Logo -->
  <Tooltip content="Windshift" placement="right" disabled={$uiStore.navExpanded}>
    <a
      href="/"
      class="flex items-center {$uiStore.navExpanded ? 'px-4' : 'justify-center'} w-full h-10 mb-2 hover:opacity-80 transition-opacity cursor-pointer"
    >
      <img src="windshift-3.svg" alt="Windshift" class="w-8 h-8 flex-shrink-0" />
      {#if $uiStore.navExpanded}
        <span class="ml-3 font-semibold text-sm whitespace-nowrap">Windshift</span>
      {/if}
    </a>
  </Tooltip>

  <!-- Main Navigation -->
  <div class="flex mt-6 flex-col {$uiStore.navExpanded ? 'items-stretch px-2.5' : 'items-center'} space-y-1 flex-1">

    <!-- Workspaces -->
    <Tooltip content={t('nav.workspaces')} placement="right" disabled={$uiStore.navExpanded}>
      <div class="{$uiStore.navExpanded ? 'w-full' : ''}">
        <DropdownMenu
          triggerIcon={IconGridDots}
          triggerIconClass="w-5 h-5"
          triggerGap="gap-3"
          triggerText={$uiStore.navExpanded ? t('nav.workspaces') : ''}
          triggerLabel={t('nav.workspaces')}
          triggerClass="{$uiStore.navExpanded ? 'w-full px-3' : 'w-10'} h-10 rounded flex items-center {$uiStore.navExpanded ? '' : 'justify-center'} cursor-pointer nav-button nav-button-emphasized {isWorkspaceRoute($currentRoute.view) ? 'nav-button-selected' : ''} {!$workspacesStore.loaded ? 'opacity-50 cursor-wait' : ''}"
          triggerTestid="workspaces-dropdown-trigger"
          items={workspacesDropdownItems}
          maxWidth="max-w-xs"
          showChevron={false}
          placement="right-start"
          iconOnly={!$uiStore.navExpanded}
          triggerAlignment={$uiStore.navExpanded ? 'start' : 'center'}
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
      />
    {/each}

    <!-- Top Actions Section - "Notch" style centered positioning -->
    <div class="flex flex-col items-stretch space-y-2 my-6 py-4">
      <NavLink
        id="global-create-button"
        icon={IconPlus}
        label={t('nav.create')}
        onclick={showCreateDropdown}
        expanded={$uiStore.navExpanded}
        variant="primary"
        tooltipSuffix=" (C)"
      />
      <NavLink
        icon={IconSearch}
        label={t('nav.search')}
        onclick={onShowCommandPalette}
        expanded={$uiStore.navExpanded}
        tooltipSuffix=" ({getShortcutDisplay('global', 'commandPalette')} or Space Space)"
      />
      {#if aiStore.chatAvailable}
        <NavLink
          id="chat-toggle-button"
          icon={IconMessage}
          label={t('nav.aiChat')}
          onclick={onShowChatPanel}
          expanded={$uiStore.navExpanded}
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

  <!-- Bottom Section -->
  <div class="flex flex-col {$uiStore.navExpanded ? 'items-stretch px-3' : 'items-center'} space-y-1 mt-auto">
    <!-- Nav Toggle Button -->
    <button
      onclick={() => uiStore.toggleNavExpanded()}
      class="flex items-center {$uiStore.navExpanded ? 'w-full px-3' : 'w-10 justify-center'} h-10 mb-2 rounded cursor-pointer nav-button"
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
      />
    {/each}

    <!-- Notification Tray -->
    <Tooltip content={t('nav.notifications')} placement="right" disabled={$uiStore.navExpanded}>
      <NotificationTray expanded={$uiStore.navExpanded} label={t('nav.notifications')} />
    </Tooltip>

    <!-- User Profile Avatar -->
    <Tooltip content={t('nav.profile')} placement="right" disabled={$uiStore.navExpanded}>
      <UserAvatar expanded={$uiStore.navExpanded} label={t('nav.profile')} />
    </Tooltip>
  </div>
</nav>

<style>
  @media (max-width: 767px) {
    .main-sidebar {
      width: 4rem;
    }
  }
</style>
