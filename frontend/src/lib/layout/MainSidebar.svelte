<script>
  import { currentRoute, navigate, isWorkspaceRoute } from '../router.js';
  import { permissionStore, uiStore, workspacesStore } from '../stores';
  import { t } from '../stores/i18n.svelte.js';
  import { getShortcutDisplay } from '../utils/keyboardShortcuts.js';
  import { workspaceIconMap } from '../utils/icons.js';
  import DropdownMenu from './DropdownMenu.svelte';
  import Tooltip from '../components/Tooltip.svelte';
  import NavLink from './NavLink.svelte';
  import UserAvatar from '../components/UserAvatar.svelte';
  import NotificationTray from '../features/notifications/NotificationTray.svelte';
  import {
    Search, Settings, Plus, Grip, Clock, Calendar, LifeBuoy,
    Milestone, Library, Package, Users, PanelLeftOpen, PanelLeftClose
  } from 'lucide-svelte';

  let {
    onShowCommandPalette = () => {},
    onShowCreateModal = () => {}
  } = $props();

  let workspaceSearchQuery = $state('');

  // Derived workspace dropdown items that automatically updates when store or search changes
  const workspacesDropdownItems = $derived.by(() => {
    const items = [];

    // Add search input at the top
    items.push({
      type: 'search',
      id: 'search',
      placeholder: t('nav.searchWorkspaces'),
      value: workspaceSearchQuery,
      onInput: (value) => {
        workspaceSearchQuery = value;
      }
    });

    // Filter workspaces based on search query
    const search = workspaceSearchQuery?.trim().toLowerCase();
    const filteredWorkspaces = !search
      ? $workspacesStore.regularWorkspaces
      : $workspacesStore.regularWorkspaces.filter(workspace => {
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
          icon: hasAvatar ? null : workspaceIcon,
          iconColor: hasAvatar ? null : workspace.color,
          avatarUrl: hasAvatar ? workspace.avatar_url : null,
          title: workspace.name,
          subtitle: workspace.description,
          onClick: () => navigateToWorkspace(workspace.id)
        };
      });

      items.push({ type: 'group', items: workspaceItems });
      if (hasMore) {
        items.push({ type: 'text', text: t('nav.searchToFindMore') });
      }
      items.push({ type: 'divider' });
    } else if ($workspacesStore.regularWorkspaces.length > 0 && workspaceSearchQuery) {
      // Show "no results" only if there are workspaces but search didn't match
      items.push(
        { type: 'text', text: t('nav.noWorkspacesMatch') },
        { type: 'divider' }
      );
    } else if ($workspacesStore.regularWorkspaces.length === 0) {
      items.push(
        { type: 'text', text: t('nav.noWorkspacesFound') },
        { type: 'divider' }
      );
    }

    // Add combined manage workspaces action
    items.push({
      id: 'manage',
      type: 'regular',
      icon: Settings,
      title: t('nav.manageWorkspaces'),
      subtitle: t('nav.manageWorkspacesSubtitle'),
      color: 'var(--ds-text-link)',
      class: 'font-medium',
      onClick: () => navigate('/workspaces')
    });

    return items;
  });

  // Navigation items data
  const mainNavItems = [
    { id: 'collections', icon: Library, labelKey: 'nav.collections', href: '/collections', activeViews: ['collections-list'] },
    { id: 'time', icon: Clock, labelKey: 'nav.timeAndProjects', href: '/time', activeViews: ['time'] },
    { id: 'milestones', icon: Milestone, labelKey: 'nav.milestones', href: '/milestones', activeViews: ['milestones', 'milestone-detail'] },
    { id: 'iterations', icon: Calendar, labelKey: 'nav.iterations', href: '/iterations', activeViews: ['iterations', 'iteration-detail'] },
    { id: 'assets', icon: Package, labelKey: 'nav.assets', href: '/assets', activeViews: ['assets', 'asset-detail'], permission: 'canAccessAssets' },
    { id: 'channels', icon: LifeBuoy, labelKey: 'nav.channels', href: '/channels', activeViews: ['hub', 'hub-inbox', 'channels'] },
    { id: 'customers', icon: Users, labelKey: 'nav.customers', href: '/customers', activeViews: ['customers'], permission: 'canAccessCustomers' }
  ];

  const bottomNavItems = [
    { id: 'admin', icon: Settings, labelKey: 'nav.admin', href: '/admin', activeViews: ['admin'], permission: 'canAccessAdmin' }
  ];

  // Filter nav items based on permissions
  const filteredMainNav = $derived(
    mainNavItems.filter(item => !item.permission || $permissionStore[item.permission])
  );

  const filteredBottomNav = $derived(
    bottomNavItems.filter(item => !item.permission || $permissionStore[item.permission])
  );

  function showCreateDropdown() {
    onShowCreateModal();
  }

  function navigateToWorkspace(workspaceId) {
    navigate(`/workspaces/${workspaceId}`);
  }
</script>

<nav class="{$uiStore.navExpanded ? 'w-[200px]' : 'w-16'} shadow-lg border-r flex flex-col py-4 fixed h-full z-40 themed-nav transition-all duration-200" style="border-color: var(--ds-border);" aria-label="Main navigation">
  <!-- Logo -->
  <Tooltip content="Windshift" placement="right" disabled={$uiStore.navExpanded}>
    <a
      href="/"
      class="flex items-center {$uiStore.navExpanded ? 'px-4' : 'justify-center'} w-full h-10 mb-2 hover:opacity-80 transition-opacity cursor-pointer"
    >
      <img src="/cmicon-2.svg" alt="Windshift" class="w-8 h-8 flex-shrink-0" />
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
          triggerIcon={Grip}
          triggerIconClass="w-5 h-5"
          triggerGap="gap-3"
          triggerText={$uiStore.navExpanded ? t('nav.workspaces') : ''}
          triggerClass="{$uiStore.navExpanded ? 'w-full px-3' : 'w-10'} h-10 rounded flex items-center {$uiStore.navExpanded ? '' : 'justify-center'} cursor-pointer nav-button {isWorkspaceRoute($currentRoute.view) ? 'nav-button-selected' : ''} {!$workspacesStore.loaded ? 'opacity-50 cursor-wait' : ''}"
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
        icon={Plus}
        label={t('nav.create')}
        onclick={showCreateDropdown}
        expanded={$uiStore.navExpanded}
        variant="primary"
        tooltipSuffix=" (C)"
      />
      <NavLink
        icon={Search}
        label={t('nav.search')}
        onclick={onShowCommandPalette}
        expanded={$uiStore.navExpanded}
        tooltipSuffix=" ({getShortcutDisplay('global', 'commandPalette')} or Space Space)"
      />
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
        <PanelLeftClose class="w-5 h-5 flex-shrink-0" />
        <span class="ml-3 text-sm whitespace-nowrap">{t('nav.collapse')}</span>
      {:else}
        <PanelLeftOpen class="w-5 h-5" />
      {/if}
    </button>
    <!-- Bottom Nav Links -->
    {#each filteredBottomNav as item (item.id)}
      <NavLink
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
