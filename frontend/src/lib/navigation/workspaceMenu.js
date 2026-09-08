import { IconSettings } from '@tabler/icons-svelte-runes';
import { workspaceIconMap } from '../utils/icons.js';

export function workspaceMenuItems(workspaces, searchQuery, onSearch, t) {
  const items = [];

  // Add search input at the top
  items.push({
    type: 'search',
    id: 'search',
    testid: 'workspaces-search',
    placeholder: t('nav.searchWorkspaces'),
    value: searchQuery,
    onInput: (value) => {
      onSearch(value);
    },
  });

  // Filter workspaces based on search query (inactive workspaces are
  // hidden here even for admins — Manage Workspaces is the only surface
  // that shows them).
  const activeRegularWorkspaces = workspaces.filter((ws) => ws.active);
  const search = searchQuery?.trim().toLowerCase();
  const filteredWorkspaces = !search
    ? activeRegularWorkspaces
    : activeRegularWorkspaces.filter((workspace) => {
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
    const workspaceItems = visibleWorkspaces.map((workspace) => {
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
        href: `/workspaces/${workspace.id}`,
      };
    });

    items.push({ type: 'group', items: workspaceItems });
    if (hasMore) {
      items.push({ type: 'text', text: t('nav.searchToFindMore') });
    }
    items.push({ type: 'divider' });
  } else if (activeRegularWorkspaces.length > 0 && searchQuery) {
    // Show "no results" only if there are workspaces but search didn't match
    items.push({ type: 'text', text: t('nav.noWorkspacesMatch') }, { type: 'divider' });
  } else if (activeRegularWorkspaces.length === 0) {
    items.push({ type: 'text', text: t('nav.noWorkspacesFound') }, { type: 'divider' });
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
    href: '/workspaces',
  });

  return items;
}
