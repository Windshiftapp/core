<script>
  import { currentRoute, navigate, isWorkspaceRoute } from '../router.js';
  import { permissionStore, uiStore, workspacesStore } from '../stores';
  import { t } from '../stores/i18n.svelte.js';
  import { getShortcutDisplay } from '../utils/keyboardShortcuts.js';
  import { workspaceIconMap } from '../utils/icons.js';
  import DropdownMenu from './DropdownMenu.svelte';
  import Tooltip from '../components/Tooltip.svelte';
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
      const workspaceItems = filteredWorkspaces.map(workspace => {
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

      items.push(
        { type: 'group', items: workspaceItems },
        { type: 'divider' }
      );
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

    <!-- Collections -->
    <Tooltip content={t('nav.collections')} placement="right" disabled={$uiStore.navExpanded}>
      <a
        href="/collections"
        class="{$uiStore.navExpanded ? 'w-full px-3' : 'w-10'} h-10 rounded flex items-center {$uiStore.navExpanded ? '' : 'justify-center'} cursor-pointer nav-button {$currentRoute.view === 'collections-list' ? 'nav-button-selected' : ''}"
        aria-current={$currentRoute.view === 'collections-list' ? 'page' : undefined}
      >
        <Library class="w-5 h-5 flex-shrink-0" />
        {#if $uiStore.navExpanded}<span class="ml-3 text-sm whitespace-nowrap">{t('nav.collections')}</span>{/if}
      </a>
    </Tooltip>

    <!-- Time & Projects -->
    <Tooltip content={t('nav.timeAndProjects')} placement="right" disabled={$uiStore.navExpanded}>
      <a
        href="/time"
        class="{$uiStore.navExpanded ? 'w-full px-3' : 'w-10'} h-10 rounded flex items-center {$uiStore.navExpanded ? '' : 'justify-center'} cursor-pointer nav-button {$currentRoute.view === 'time' ? 'nav-button-selected' : ''}"
        aria-current={$currentRoute.view === 'time' ? 'page' : undefined}
      >
        <Clock class="w-5 h-5 flex-shrink-0" />
        {#if $uiStore.navExpanded}<span class="ml-3 text-sm whitespace-nowrap">{t('nav.timeAndProjects')}</span>{/if}
      </a>
    </Tooltip>

    <!-- Milestones -->
    <Tooltip content={t('nav.milestones')} placement="right" disabled={$uiStore.navExpanded}>
      <a
        href="/milestones"
        class="{$uiStore.navExpanded ? 'w-full px-3' : 'w-10'} h-10 rounded flex items-center {$uiStore.navExpanded ? '' : 'justify-center'} cursor-pointer nav-button {$currentRoute.view === 'milestones' || $currentRoute.view === 'milestone-detail' ? 'nav-button-selected' : ''}"
        aria-current={$currentRoute.view === 'milestones' ? 'page' : undefined}
      >
        <Milestone class="w-5 h-5 flex-shrink-0" />
        {#if $uiStore.navExpanded}<span class="ml-3 text-sm whitespace-nowrap">{t('nav.milestones')}</span>{/if}
      </a>
    </Tooltip>

    <!-- Iterations -->
    <Tooltip content={t('nav.iterations')} placement="right" disabled={$uiStore.navExpanded}>
      <a
        href="/iterations"
        class="{$uiStore.navExpanded ? 'w-full px-3' : 'w-10'} h-10 rounded flex items-center {$uiStore.navExpanded ? '' : 'justify-center'} cursor-pointer nav-button {$currentRoute.view === 'iterations' || $currentRoute.view === 'iteration-detail' ? 'nav-button-selected' : ''}"
        aria-current={$currentRoute.view === 'iterations' ? 'page' : undefined}
      >
        <Calendar class="w-5 h-5 flex-shrink-0" />
        {#if $uiStore.navExpanded}<span class="ml-3 text-sm whitespace-nowrap">{t('nav.iterations')}</span>{/if}
      </a>
    </Tooltip>

    <!-- Assets -->
    <Tooltip content={t('nav.assets')} placement="right" disabled={$uiStore.navExpanded}>
      <a
        href="/assets"
        class="{$uiStore.navExpanded ? 'w-full px-3' : 'w-10'} h-10 rounded flex items-center {$uiStore.navExpanded ? '' : 'justify-center'} cursor-pointer nav-button {$currentRoute.view === 'assets' || $currentRoute.view === 'asset-detail' ? 'nav-button-selected' : ''}"
        aria-current={$currentRoute.view === 'assets' ? 'page' : undefined}
      >
        <Package class="w-5 h-5 flex-shrink-0" />
        {#if $uiStore.navExpanded}<span class="ml-3 text-sm whitespace-nowrap">{t('nav.assets')}</span>{/if}
      </a>
    </Tooltip>

    <!-- Channels -->
    <Tooltip content={t('nav.channels')} placement="right" disabled={$uiStore.navExpanded}>
      <a
        href="/channels"
        class="{$uiStore.navExpanded ? 'w-full px-3' : 'w-10'} h-10 rounded flex items-center {$uiStore.navExpanded ? '' : 'justify-center'} cursor-pointer nav-button {$currentRoute.view === 'channels' ? 'nav-button-selected' : ''}"
        aria-current={$currentRoute.view === 'channels' ? 'page' : undefined}
      >
        <LifeBuoy class="w-5 h-5 flex-shrink-0" />
        {#if $uiStore.navExpanded}<span class="ml-3 text-sm whitespace-nowrap">{t('nav.channels')}</span>{/if}
      </a>
    </Tooltip>

    <!-- Customers (conditional based on permission) -->
    {#if $permissionStore.canAccessCustomers}
      <Tooltip content={t('nav.customers')} placement="right" disabled={$uiStore.navExpanded}>
        <a
          href="/customers"
          class="{$uiStore.navExpanded ? 'w-full px-3' : 'w-10'} h-10 rounded flex items-center {$uiStore.navExpanded ? '' : 'justify-center'} cursor-pointer nav-button {$currentRoute.view === 'customers' ? 'nav-button-selected' : ''}"
          aria-current={$currentRoute.view === 'customers' ? 'page' : undefined}
        >
          <Users class="w-5 h-5 flex-shrink-0" />
          {#if $uiStore.navExpanded}<span class="ml-3 text-sm whitespace-nowrap">{t('nav.customers')}</span>{/if}
        </a>
      </Tooltip>
    {/if}

    <!-- Top Actions Section - "Notch" style centered positioning -->
    <div class="flex flex-col items-stretch space-y-2 my-6 py-4">
      <!-- Create button -->
      <Tooltip content="{t('nav.create')} (C)" placement="right" disabled={$uiStore.navExpanded}>
        <button
          onclick={showCreateDropdown}
          class="{$uiStore.navExpanded ? 'w-full' : 'w-10'} h-10 bg-[var(--ds-interactive)] bg-primary text-white rounded flex items-center {$uiStore.navExpanded ? 'px-3' : 'justify-center'} text-sm font-medium transition cursor-pointer"
        >
          <Plus class="w-5 h-5 flex-shrink-0" />
          {#if $uiStore.navExpanded}<span class="ml-3 whitespace-nowrap">{t('nav.create')}</span>{/if}
        </button>
      </Tooltip>

      <!-- Search button -->
      <Tooltip content="{t('nav.search')} ({getShortcutDisplay('global', 'commandPalette')} or Space Space)" placement="right" disabled={$uiStore.navExpanded}>
        <button
          onclick={onShowCommandPalette}
          class="{$uiStore.navExpanded ? 'w-full' : 'w-10'} h-10 rounded flex items-center {$uiStore.navExpanded ? 'px-3' : 'justify-center'} cursor-pointer nav-button"
        >
          <Search class="w-5 h-5 flex-shrink-0" />
          {#if $uiStore.navExpanded}<span class="ml-3 text-sm whitespace-nowrap">{t('nav.search')}</span>{/if}
        </button>
      </Tooltip>
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
    <!-- Admin (conditional) -->
    {#if $permissionStore.canAccessAdmin}
      <Tooltip content={t('nav.admin')} placement="right" disabled={$uiStore.navExpanded}>
        <a
          href="/admin"
          class="{$uiStore.navExpanded ? 'w-full px-3' : 'w-10 justify-center'} h-10 rounded flex items-center cursor-pointer nav-button {$currentRoute.view === 'admin' ? 'nav-button-selected' : ''}"
          aria-current={$currentRoute.view === 'admin' ? 'page' : undefined}
        >
          <Settings class="w-5 h-5 flex-shrink-0" />
          {#if $uiStore.navExpanded}<span class="ml-3 text-sm whitespace-nowrap">{t('nav.admin')}</span>{/if}
        </a>
      </Tooltip>
    {/if}

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
