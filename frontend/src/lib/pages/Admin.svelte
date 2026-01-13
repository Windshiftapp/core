<script>
  import { onMount } from 'svelte';
  import { currentRoute, navigate } from '../router.js';
  import CustomFields from '../settings/CustomFields.svelte';
  import Workspaces from '../workspaces/Workspaces.svelte';
  import Screens from './Screens.svelte';
  import StatusContainer from '../features/workflows/StatusContainer.svelte';
  import WorkflowBuilder from '../features/workflows/WorkflowBuilder.svelte';
  import ConfigurationSetManager from '../settings/ConfigurationSetManager.svelte';
  import LinkTypeManager from '../settings/LinkTypeManager.svelte';
  import UserManager from '../settings/UserManager.svelte';
  import GroupManager from '../settings/GroupManager.svelte';
  import PermissionsContainer from '../layout/PermissionsContainer.svelte';
  import RoleManager from '../settings/RoleManager.svelte';
  import AttachmentSettings from '../settings/AttachmentSettings.svelte';
  import ModuleSettings from '../settings/ModuleSettings.svelte';
  import HierarchyLevelManager from '../settings/HierarchyLevelManager.svelte';
  import ItemTypeManager from '../settings/ItemTypeManager.svelte';
  import PriorityManager from '../settings/PriorityManager.svelte';
  import NotificationSettings from '../settings/NotificationSettings.svelte';
  import ThemeManager from '../settings/ThemeManager.svelte';
  import AuditLog from './AuditLog.svelte';
  import SSOSettings from '../settings/SSOSettings.svelte';
  import SCMProviderManager from '../settings/SCMProviderManager.svelte';
  import SecuritySettings from '../settings/SecuritySettings.svelte';
  import AssetManager from '../features/assets/AssetManager.svelte';
  import PermissionSetEdit from '../settings/PermissionSetEdit.svelte';
  import ConfigurationSetDetail from '../settings/ConfigurationSetDetail.svelte';
  import SystemImportPage from '../jira-import/SystemImportPage.svelte';
  import { extensions, loadExtensions, getExtensionsForPoint } from '../stores/extensions.js';
  import IframePluginLoader from '../services/IframePluginLoader.svelte';
  import PluginModalContainer from '../layout/PluginModalContainer.svelte';
  import LinkComponent from '../components/Link.svelte';
  import {
    Settings, UserStar, Layout, Database, GitBranch,
    Workflow, Package, Link, Paperclip, Puzzle,
    Network, FileText, Shield, Bell, Search, X,
    Layers, Cog, LinkIcon, UserCheck, MessageSquare, Folder, UsersRound, Palette, Notebook, Grip, ScrollText, AlertCircle, KeyRound, BadgeCheck, GitMerge, CloudDownload
  } from 'lucide-svelte';

  // Check if we're on a nested detail route (not a tab)
  const isNestedRoute = $derived(
    $currentRoute.path.startsWith('/admin/permission-sets/') ||
    $currentRoute.path.startsWith('/admin/configuration-sets/')
  );
  const isPermissionSetRoute = $derived($currentRoute.path.startsWith('/admin/permission-sets/'));
  const isConfigSetRoute = $derived($currentRoute.path.startsWith('/admin/configuration-sets/'));

  // Get active tab from URL - supports both /admin/:tab path and ?tab= query param
  const activeTab = $derived($currentRoute.params?.tab || $currentRoute.query?.tab || 'custom-fields');

  // Search functionality
  let searchQuery = $state('');
  let searchInput;

  // Navigation focus management
  let navButtons = $state([]);
  let focusedIndex = $state(-1);

  // Define admin navigation groups
  const adminGroups = [
    {
      id: 'content-structure',
      label: 'Content & Structure',
      icon: Layers,
      items: [
        { id: 'custom-fields', label: 'Custom Fields', icon: Database, description: 'Manage custom field definitions' },
        { id: 'screens', label: 'Screens', icon: Layout, description: 'Configure form screens and layouts' },
        { id: 'hierarchy-levels', label: 'Hierarchy Levels', icon: Network, description: 'Configure work item hierarchy levels' },
        { id: 'item-types', label: 'Item Types', icon: FileText, description: 'Manage work item types with icons and colors' },
        { id: 'priorities', label: 'Priorities', icon: AlertCircle, description: 'Configure priority levels with icons and colors' },
        { id: 'configuration-sets', label: 'Configuration Sets', icon: Settings, description: 'Manage configuration sets' },
      ]
    },
    {
      id: 'workflow-process',
      label: 'Workflow & Process',
      icon: Cog,
      items: [
        { id: 'statuses', label: 'Statuses', icon: GitBranch, description: 'Manage status categories and individual statuses' },
        { id: 'workflows', label: 'Workflows', icon: Workflow, description: 'Design and manage workflow transitions' },
      ]
    },
    {
      id: 'integration-links',
      label: 'Integration & Links',
      icon: LinkIcon,
      items: [
        { id: 'scm-providers', label: 'SCM Providers', icon: GitMerge, description: 'Configure GitHub, GitLab, Gitea, and Bitbucket integrations' },
        { id: 'system-import', label: 'System Import', icon: CloudDownload, description: 'Import data from external systems' },
        { id: 'link-types', label: 'Link Types', icon: Link, description: 'Manage link types between items' },
        { id: 'attachments', label: 'Attachments', icon: Paperclip, description: 'Manage attachment settings' },
        { id: 'modules', label: 'Modules', icon: Puzzle, description: 'Enable or disable system modules' },
        { id: 'themes', label: 'Themes', icon: Palette, description: 'Manage application themes and appearance' },
      ]
    },
    {
      id: 'users-access',
      label: 'Users & Access',
      icon: UserCheck,
      items: [
        { id: 'users', label: 'Users', icon: UsersRound, description: 'Manage user accounts and profiles' },
        { id: 'groups', label: 'Groups', icon: UserStar, description: 'Manage user groups and memberships' },
        { id: 'permissions', label: 'Permissions & Sets', icon: Shield, description: 'Manage permissions and permission sets' },
        { id: 'workspace-roles', label: 'Workspace Roles', icon: BadgeCheck, description: 'View workspace roles and their permissions' },
        { id: 'sso', label: 'Single Sign-On', icon: KeyRound, description: 'Configure OIDC identity providers for SSO' },
        { id: 'security', label: 'Security', icon: Shield, description: 'Manage security settings and feature controls' },
        { id: 'workspaces', label: 'Workspaces', icon: Grip, description: 'Manage workspaces and settings' },
      ]
    },
    {
      id: 'communication',
      label: 'Communication',
      icon: MessageSquare,
      items: [
        { id: 'notification-settings', label: 'Notification Settings', icon: Bell, description: 'Manage notification configurations' },
      ]
    },
    {
      id: 'asset-management',
      label: 'Asset Management',
      icon: Package,
      items: [
        { id: 'assets', label: 'Assets', icon: Package, description: 'Manage asset sets, types, categories, and assets' },
      ]
    },
    // Core audit log hidden - will be provided by plugin
    // {
    //   id: 'security-audit',
    //   label: 'Security & Audit',
    //   icon: ScrollText,
    //   items: [
    //     { id: 'audit-log', label: 'Audit Log', icon: FileText, description: 'Track and review all administrative actions and security events' },
    //   ]
    // }
  ];

  // Merge plugin extensions into admin groups
  const adminGroupsWithPlugins = $derived.by(() => {
    const groups = [...adminGroups];
    const adminTabExtensions = getExtensionsForPoint($extensions, 'admin.tab');

    // Group extensions by their group property
    const extensionsByGroup = {};
    adminTabExtensions.forEach(ext => {
      const groupName = ext.group || 'Plugins';
      if (!extensionsByGroup[groupName]) {
        extensionsByGroup[groupName] = [];
      }
      extensionsByGroup[groupName].push({
        id: ext.id,
        label: ext.label,
        icon: FileText, // Default icon, could be mapped from ext.icon
        description: ext.description,
        isPlugin: true,
        pluginData: ext
      });
    });

    // Add or merge extension groups
    Object.entries(extensionsByGroup).forEach(([groupName, items]) => {
      const existingGroup = groups.find(g => g.label === groupName);
      if (existingGroup) {
        // Merge into existing group
        existingGroup.items = [...existingGroup.items, ...items];
      } else {
        // Create new group for plugins
        groups.push({
          id: groupName.toLowerCase().replace(/\s+/g, '-'),
          label: groupName,
          icon: Puzzle,
          items
        });
      }
    });

    return groups;
  });

  // Create flat list of all items for search
  const allAdminItems = $derived(adminGroupsWithPlugins.flatMap(group => group.items));

  // Filter groups and items based on search
  const filteredGroups = $derived(
    searchQuery.trim() === ''
      ? adminGroupsWithPlugins
      : adminGroupsWithPlugins
          .map(group => ({
            ...group,
            items: group.items.filter(item =>
              item.label.toLowerCase().includes(searchQuery.toLowerCase()) ||
              item.description.toLowerCase().includes(searchQuery.toLowerCase())
            )
          }))
          .filter(group => group.items.length > 0)
  );

  function clearSearch() {
    searchQuery = '';
    searchInput?.focus();
  }

  // Keyboard navigation for search
  function handleSearchKeydown(event) {
    if (event.key === 'Escape') {
      if (searchQuery) {
        clearSearch();
      } else {
        // If search is already empty, blur the input
        searchInput?.blur();
      }
    } else if (event.key === 'Enter' && filteredGroups.length > 0 && filteredGroups[0].items.length > 0) {
      // Navigate to first item when pressing Enter
      const firstItem = filteredGroups[0].items[0];
      handleTabClick(firstItem.id);
    } else if (event.key === 'ArrowDown') {
      event.preventDefault();
      // Focus first navigation item
      if (navButtons.length > 0) {
        navButtons[0]?.focus();
        focusedIndex = 0;
      }
    }
  }

  // Arrow key navigation for menu items
  function handleNavKeydown(event, currentIndex) {
    if (event.key === 'ArrowDown') {
      event.preventDefault();
      const nextIndex = currentIndex + 1;
      if (nextIndex < navButtons.length) {
        navButtons[nextIndex]?.focus();
        focusedIndex = nextIndex;
      }
    } else if (event.key === 'ArrowUp') {
      event.preventDefault();
      const prevIndex = currentIndex - 1;
      if (prevIndex >= 0) {
        navButtons[prevIndex]?.focus();
        focusedIndex = prevIndex;
      } else {
        // Go back to search
        searchInput?.focus();
        focusedIndex = -1;
      }
    } else if (event.key === 'Home') {
      event.preventDefault();
      if (navButtons.length > 0) {
        navButtons[0]?.focus();
        focusedIndex = 0;
      }
    } else if (event.key === 'End') {
      event.preventDefault();
      const lastIndex = navButtons.length - 1;
      if (lastIndex >= 0) {
        navButtons[lastIndex]?.focus();
        focusedIndex = lastIndex;
      }
    }
  }

  // Global keyboard shortcut handler
  function handleGlobalKeydown(event) {
    // Focus search on '/' key (unless already in an input)
    if (event.key === '/' && document.activeElement?.tagName !== 'INPUT' && document.activeElement?.tagName !== 'TEXTAREA') {
      event.preventDefault();
      searchInput?.focus();
    }
  }

  function handleTabClick(tabId) {
    navigate(`/admin/${tabId}`);
  }

  onMount(async () => {
    // Load plugin extensions
    await loadExtensions();

    // If no tab in URL and not on a nested detail route, redirect to default tab
    const path = $currentRoute.path;
    const isNested = path.startsWith('/admin/permission-sets/') || path.startsWith('/admin/configuration-sets/');
    if (!$currentRoute.params?.tab && !isNested) {
      navigate('/admin/custom-fields');
    }

    // Listen for admin tab switch events from command palette
    function handleAdminTabSwitch(event) {
      if (event.detail && event.detail.tab) {
        navigate(`/admin/${event.detail.tab}`);
      }
    }

    window.addEventListener('admin-tab-switch', handleAdminTabSwitch);
    window.addEventListener('keydown', handleGlobalKeydown);

    // Cleanup event listeners
    return () => {
      window.removeEventListener('admin-tab-switch', handleAdminTabSwitch);
      window.removeEventListener('keydown', handleGlobalKeydown);
    };
  });

  // Calculate button indices for arrow navigation
  const buttonIndices = $derived.by(() => {
    const indices = new Map();
    let globalIndex = 0;
    filteredGroups.forEach(group => {
      group.items.forEach(item => {
        indices.set(item.id, globalIndex);
        globalIndex++;
      });
    });
    return indices;
  });

  // Total button count for validation
  const totalButtons = $derived(filteredGroups.reduce((count, group) => count + group.items.length, 0));

  function switchTab(tab) {
    navigate(`/admin/${tab}`);
  }
</script>

<!-- Main container with sidebar layout -->
<div class="flex min-h-screen" style="background-color: var(--ds-surface);">
  <!-- Left Sidebar -->
  <div class="w-64 border-r flex-shrink-0" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
    <div class="p-6">
      <div class="mb-6">
        <h1 class="text-xl font-semibold" style="color: var(--ds-text);">Admin Panel</h1>
        <p class="mt-1 text-sm" style="color: var(--ds-text-subtle);">System administration and configuration</p>
      </div>
      
      <!-- Search -->
      <div class="mb-4 relative">
        <label for="admin-search" class="sr-only">Search admin settings</label>
        <div class="relative">
          <Search class="absolute left-3 top-1/2 transform -translate-y-1/2 w-4 h-4" style="color: var(--ds-icon-subtle);" aria-hidden="true" />
          <input
            id="admin-search"
            bind:this={searchInput}
            bind:value={searchQuery}
            onkeydown={handleSearchKeydown}
            type="search"
            placeholder="Search"
            class="w-full pl-10 pr-8 py-2 text-sm border rounded-md focus:outline-none focus:ring-2"
            style="border-color: var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text); --tw-ring-color: var(--ds-interactive);"
            aria-describedby={searchQuery && filteredGroups.length === 0 ? 'search-no-results' : undefined}
          />
          {#if searchQuery}
            <button
              onclick={clearSearch}
              class="absolute right-2 top-1/2 transform -translate-y-1/2 p-1 transition-colors"
              style="color: var(--ds-icon-subtle);"
              onmouseenter={(e) => e.currentTarget.style.color = 'var(--ds-icon)'}
              onmouseleave={(e) => e.currentTarget.style.color = 'var(--ds-icon-subtle)'}
              aria-label="Clear search"
            >
              <X class="w-3 h-3" aria-hidden="true" />
            </button>
          {/if}
        </div>
      </div>

      <!-- Navigation -->
      <nav class="space-y-6 pb-6" aria-label="Admin settings">
        {#each filteredGroups as group}
          <div role="group" aria-labelledby="group-{group.id}">
            <!-- Group Header -->
            <div class="px-2 pt-3 pb-1 mb-1">
              <h3 id="group-{group.id}" class="text-xs font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle);">
                {group.label}
              </h3>
            </div>

            <!-- Group Items -->
            <div class="space-y-1">
              {#each group.items as item}
                {@const buttonIndex = buttonIndices.get(item.id)}
                {@const isItemActive = activeTab === item.id}
                <LinkComponent
                  bind:element={navButtons[buttonIndex]}
                  href="/admin/{item.id}"
                  active={isItemActive}
                  onkeydown={(e) => handleNavKeydown(e, buttonIndex)}
                  class="w-full group flex items-center px-3 py-2 text-sm font-medium rounded-lg transition-all cursor-pointer"
                  style={isItemActive ? 'background: var(--ds-surface-selected); color: var(--ds-text);' : 'color: var(--ds-text-subtle);'}
                  onmouseenter={(e) => { if (!isItemActive) e.currentTarget.style.cssText = 'background: var(--ds-surface-hovered); color: var(--ds-text);'; }}
                  onmouseleave={(e) => { if (!isItemActive) e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'; }}
                >
                  <svelte:component this={item.icon} class="flex-shrink-0 -ml-1 mr-3 w-4 h-4" aria-hidden="true" />
                  <span>{item.label}</span>
                </LinkComponent>
              {/each}
            </div>
          </div>
        {/each}
        
        {#if filteredGroups.length === 0 && searchQuery}
          <div class="text-center py-4" role="status" id="search-no-results">
            <p class="text-sm" style="color: var(--ds-text-subtle);">No admin functions found</p>
            <button
              onclick={clearSearch}
              class="text-xs mt-1"
              style="color: var(--ds-link);"
              onmouseenter={(e) => e.currentTarget.style.color = 'var(--ds-link-pressed)'}
              onmouseleave={(e) => e.currentTarget.style.color = 'var(--ds-link)'}
            >
              Clear search
            </button>
          </div>
        {/if}

        <!-- Live region for search results announcements -->
        <div class="sr-only" role="status" aria-live="polite" aria-atomic="true">
          {#if searchQuery && filteredGroups.length > 0}
            {filteredGroups.reduce((count, group) => count + group.items.length, 0)} result{filteredGroups.reduce((count, group) => count + group.items.length, 0) === 1 ? '' : 's'} found
          {:else if searchQuery && filteredGroups.length === 0}
            No results found
          {/if}
        </div>
      </nav>
    </div>
  </div>

  <!-- Main Content -->
  <div class="flex-1 flex flex-col overflow-hidden">
    <!-- Nested detail routes (no padding) -->
    {#if isPermissionSetRoute}
      <PermissionSetEdit />
    {:else if isConfigSetRoute}
      <ConfigurationSetDetail />
    {:else}
    <div class="px-16 py-12 pb-0 flex-1 overflow-y-auto">
      <div class="pr-0 pl-0">
      <!-- Custom Fields Tab -->
  {#if activeTab === 'custom-fields'}
    <CustomFields />
  {/if}

  <!-- Workspaces Tab -->
  {#if activeTab === 'workspaces'}
    <Workspaces noPadding />
  {/if}

  <!-- Screens Tab -->
  {#if activeTab === 'screens'}
    <Screens />
  {/if}

  <!-- Statuses (Categories & Individual) Tab -->
  {#if activeTab === 'statuses'}
    <StatusContainer />
  {/if}

  <!-- Workflows Tab -->
  {#if activeTab === 'workflows'}
    <WorkflowBuilder />
  {/if}

  <!-- Configuration Sets Tab -->
  {#if activeTab === 'configuration-sets'}
    <ConfigurationSetManager />
  {/if}

  <!-- Notification Settings Tab -->
  {#if activeTab === 'notification-settings'}
    <NotificationSettings />
  {/if}

  <!-- Link Types Tab -->
  {#if activeTab === 'link-types'}
    <LinkTypeManager />
  {/if}

  <!-- User Management Tab -->
  {#if activeTab === 'users'}
    <UserManager />
  {/if}

  <!-- Group Management Tab -->
  {#if activeTab === 'groups'}
    <GroupManager />
  {/if}

  <!-- Permissions & Permission Sets Tab -->
  {#if activeTab === 'permissions'}
    <PermissionsContainer />
  {/if}

  <!-- Workspace Roles Tab -->
  {#if activeTab === 'workspace-roles'}
    <RoleManager />
  {/if}

  <!-- Attachment Settings Tab -->
  {#if activeTab === 'attachments'}
    <AttachmentSettings />
  {/if}

  <!-- Module Settings Tab -->
  {#if activeTab === 'modules'}
    <ModuleSettings />
  {/if}

  <!-- Theme Settings Tab -->
  {#if activeTab === 'themes'}
    <ThemeManager />
  {/if}

  <!-- Hierarchy Levels Tab -->
  {#if activeTab === 'hierarchy-levels'}
    <HierarchyLevelManager />
  {/if}

  <!-- Item Types Tab -->
  {#if activeTab === 'item-types'}
    <ItemTypeManager />
  {/if}

  <!-- Priorities Tab -->
  {#if activeTab === 'priorities'}
    <PriorityManager />
  {/if}

  <!-- Audit Log Tab (core - kept for backward compatibility) -->
  {#if activeTab === 'audit-log'}
    <AuditLog />
  {/if}

  <!-- SSO Settings Tab -->
  {#if activeTab === 'sso'}
    <SSOSettings />
  {/if}

  <!-- SCM Providers Tab -->
  {#if activeTab === 'scm-providers'}
    <SCMProviderManager />
  {/if}

  <!-- System Import Tab -->
  {#if activeTab === 'system-import'}
    <SystemImportPage />
  {/if}

  <!-- Security Settings Tab -->
  {#if activeTab === 'security'}
    <SecuritySettings />
  {/if}

  <!-- Asset Management Tab -->
  {#if activeTab === 'assets'}
    <AssetManager />
  {/if}

  <!-- Plugin Components -->
  {#each allAdminItems.filter(item => item.isPlugin) as pluginItem}
    {#if activeTab === pluginItem.id}
      {@const pluginName = pluginItem.pluginData?.pluginName || 'unknown'}
      {@const iframeSrc = `/api/plugins/${pluginName}/assets/${pluginItem.pluginData?.component || 'index.html'}`}

      <div class="plugin-component-container">
        <!-- All plugins use iframe mode -->
        <IframePluginLoader
          pluginName={pluginItem.label}
          src={iframeSrc}
        />
      </div>
    {/if}
  {/each}
      </div>
    </div>
    {/if}
  </div>
</div>

<!-- Plugin Modal Container - renders modals requested by iframe plugins -->
<PluginModalContainer />

<style>
  :global(.tiptap-editor) {
    outline: none;
    white-space: pre-wrap;
  }

  :global(.tiptap-editor .ProseMirror) {
    outline: none;
    padding: 1rem;
    min-height: 350px;
  }

  :global(.tiptap-editor h1) {
    font-size: 1.875rem;
    font-weight: 600;
    margin: 1.5rem 0 1rem 0;
    color: var(--ds-text);
    line-height: 1.2;
  }

  :global(.tiptap-editor h2) {
    font-size: 1.5rem;
    font-weight: 600;
    margin: 1.25rem 0 0.75rem 0;
    color: var(--ds-text);
    line-height: 1.3;
  }

  :global(.tiptap-editor h3) {
    font-size: 1.25rem;
    font-weight: 600;
    margin: 1rem 0 0.5rem 0;
    color: var(--ds-text);
    line-height: 1.4;
  }

  :global(.tiptap-editor p) {
    margin: 0.75rem 0;
    line-height: 1.6;
  }

  :global(.tiptap-editor p:first-child) {
    margin-top: 0;
  }

  :global(.tiptap-editor p:last-child) {
    margin-bottom: 0;
  }

  :global(.tiptap-editor ul, .tiptap-editor ol) {
    padding-left: 1.5rem;
    margin: 0.75rem 0;
  }

  :global(.tiptap-editor li) {
    margin: 0.25rem 0;
    line-height: 1.5;
  }

  :global(.tiptap-editor strong) {
    font-weight: 600;
  }

  :global(.tiptap-editor em) {
    font-style: italic;
  }

  :global(.tiptap-editor code) {
    background: var(--ds-background-neutral);
    padding: 2px 4px;
    border-radius: 3px;
    font-family: monospace;
    font-size: 0.875rem;
    color: var(--ds-text);
  }

  :global(.tiptap-editor hr) {
    border: none;
    border-top: 2px solid var(--ds-border);
    margin: 1.5rem 0;
  }

  :global(.tiptap-editor blockquote) {
    border-left: 4px solid var(--ds-border);
    padding-left: 1rem;
    margin: 1rem 0;
    font-style: italic;
  }

  /* Placeholder styling */
  :global(.tiptap-editor .ProseMirror p.is-editor-empty:first-child::before) {
    content: attr(data-placeholder);
    float: left;
    color: var(--ds-text-subtlest);
    pointer-events: none;
    height: 0;
  }

  /* Ensure proper spacing and line breaks */
  :global(.tiptap-editor br) {
    display: block;
    content: "";
    margin-top: 0.5rem;
  }
</style>
