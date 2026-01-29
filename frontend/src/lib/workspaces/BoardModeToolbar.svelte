<script>
  import { onMount } from 'svelte';
  import { navigate, currentRoute } from '../router.js';
  import { currentWorkspace } from '../stores';
  import { api } from '../api.js';
  import { workspaceIconMap } from '../utils/icons.js';
  import DropdownMenu from '../layout/DropdownMenu.svelte';
  import { Package, List, MapPin, Calendar, Milestone, Settings, Palette } from 'lucide-svelte';
  import Rows_3 from 'lucide-svelte/icons/rows-3';
  import ListTree from 'lucide-svelte/icons/list-tree';
  import { t } from '../stores/i18n.svelte.js';

  let { workspaceId = null } = $props();

  let collections = $state([]);
  let currentCollectionId = $state(null);
  let currentCollectionName = $state('All Items');
  let collectionMenuItems = $state([]);

  onMount(async () => {
    if (workspaceId) {
      await loadCollections();
    }
  });

  $effect(() => {
    if (workspaceId) {
      loadCollections();
    }
  });

  $effect(() => {
    syncCollectionWithRoute($currentRoute.params?.collectionId);
  });

  function syncCollectionWithRoute(routeCollectionId) {
    if (routeCollectionId) {
      currentCollectionId = routeCollectionId;
      const collection = collections.find(c => c.id == routeCollectionId);
      currentCollectionName = collection ? collection.name : 'All Items';
    } else {
      currentCollectionId = null;
      currentCollectionName = 'All Items';
    }
    buildCollectionMenuItems();
  }

  async function loadCollections() {
    try {
      const allCollections = await api.collections.getAll();
      collections = (allCollections || []).filter(c => c.workspace_id == workspaceId);
      buildCollectionMenuItems();
    } catch (error) {
      console.error('Failed to load collections:', error);
      collections = [];
    }
  }

  function buildCollectionMenuItems() {
    const items = [
      {
        id: 'default',
        title: 'All Items',
        subtitle: 'Show all items in workspace',
        badge: currentCollectionId === null ? '✓' : null,
        badgeStyle: currentCollectionId === null ? 'color: var(--ds-text-link);' : '',
        style: currentCollectionId === null ? 'background-color: var(--ds-background-selected); font-weight: 600;' : '',
        onClick: () => selectCollection(null)
      }
    ];

    if (collections.length > 0) {
      items.push({ type: 'divider' });
      for (const collection of collections) {
        items.push({
          id: `collection-${collection.id}`,
          title: collection.name,
          subtitle: collection.description || undefined,
          badge: currentCollectionId == collection.id ? '✓' : null,
          badgeStyle: currentCollectionId == collection.id ? 'color: var(--ds-text-link);' : '',
          style: currentCollectionId == collection.id ? 'background-color: var(--ds-background-selected); font-weight: 600;' : '',
          onClick: () => selectCollection(collection)
        });
      }
    }

    collectionMenuItems = items;
  }

  function selectCollection(collection) {
    if (collection) {
      currentCollectionId = collection.id;
      currentCollectionName = collection.name;
      navigate(`/workspaces/${workspaceId}/collections/${collection.id}/board`);
    } else {
      currentCollectionId = null;
      currentCollectionName = 'All Items';
      navigate(`/workspaces/${workspaceId}/board`);
    }
  }

  // Build navigation dropdown menu items
  const navMenuItems = $derived.by(() => {
    return [
      { title: 'Backlog', icon: Rows_3, onClick: () => navigateToView('backlog') },
      { title: 'List', icon: List, onClick: () => navigateToView('list') },
      { title: 'Tree', icon: ListTree, onClick: () => navigateToView('tree') },
      { title: 'Map', icon: MapPin, onClick: () => navigateToView('map') },
      { type: 'divider' },
      { title: 'Iterations', icon: Calendar, onClick: () => navigate(`/workspaces/${workspaceId}/iterations`) },
      { title: 'Milestones', icon: Milestone, onClick: () => navigate(`/workspaces/${workspaceId}/milestones`) },
      { type: 'divider' },
      { title: 'Look and Feel', icon: Palette, onClick: () => navigate(`/workspaces/${workspaceId}/look-and-feel`) },
      { title: 'Settings', icon: Settings, onClick: () => navigate(`/workspaces/${workspaceId}/settings`) }
    ];
  });

  function navigateToView(viewId) {
    if (currentCollectionId) {
      navigate(`/workspaces/${workspaceId}/collections/${currentCollectionId}/${viewId}`);
    } else {
      navigate(`/workspaces/${workspaceId}/${viewId}`);
    }
  }

  // Get workspace icon component
  const WorkspaceIcon = $derived.by(() => {
    const iconName = $currentWorkspace?.icon;
    return iconName ? (workspaceIconMap[iconName] || Package) : Package;
  });

  const workspaceColor = $derived($currentWorkspace?.color || '#3b82f6');
</script>

<div class="board-mode-toolbar flex items-center px-4 py-4 border-b" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
  <!-- Left side: workspace dropdown + collection switcher -->
  <div class="flex items-center gap-2 min-w-0">
    <!-- Workspace dropdown with navigation -->
    <DropdownMenu
      items={navMenuItems}
      showChevron={true}
      placement="bottom-start"
      triggerClass="!px-2 !py-1 !text-sm font-medium rounded transition-colors"
      triggerStyle="color: var(--ds-text);"
    >
      <div class="flex items-center gap-1.5">
        {#if $currentWorkspace?.avatar_url}
          <img src={$currentWorkspace.avatar_url} alt="" class="w-5 h-5 rounded object-cover" />
        {:else}
          <div class="w-5 h-5 rounded flex items-center justify-center" style="background-color: {workspaceColor};">
            <svelte:component this={WorkspaceIcon} size={12} color="white" />
          </div>
        {/if}
        <span class="truncate max-w-[140px]">{$currentWorkspace?.name || 'Workspace'}</span>
      </div>
    </DropdownMenu>

    <span class="text-xs" style="color: var(--ds-text-subtle);">/</span>

    <!-- Collection switcher -->
    <DropdownMenu
      triggerText={currentCollectionName}
      items={collectionMenuItems}
      showChevron={true}
      placement="bottom-start"
      triggerClass="!px-2 !py-1 !text-sm font-medium rounded transition-colors"
      triggerStyle="color: var(--ds-text);"
    />
  </div>
</div>

<style>
  .board-mode-toolbar {
    min-height: 36px;
  }
</style>
