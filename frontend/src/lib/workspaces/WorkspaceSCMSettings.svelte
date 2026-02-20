<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import Button from '../components/Button.svelte';
  import { GitMerge, Plus, Trash2, ExternalLink, ChevronDown, ChevronRight, Settings, RefreshCw, Loader2, Check, X, KeyRound } from 'lucide-svelte';
  import RepositorySelector from '../pickers/RepositorySelector.svelte';
  import { successToast, errorToast } from '../stores/toasts.svelte.js';

  export let workspaceId;

  let loading = true;
  let availableProviders = [];
  let connections = [];
  let expandedConnections = new Set();
  let linkedRepos = {}; // connId -> repos array
  let loadingRepos = new Set();
  let authStatuses = {}; // connId -> auth status object

  // Modal state
  let showRepoSelector = false;
  let selectedConnection = null;


  onMount(async () => {
    await loadData();
  });

  async function loadData() {
    loading = true;
    try {
      const [providersRes, connectionsRes] = await Promise.all([
        api.workspaceSCM.getAvailableProviders(workspaceId),
        api.workspaceSCM.getConnections(workspaceId)
      ]);
      availableProviders = providersRes || [];
      connections = connectionsRes || [];
      if (connections.length > 0) {
        await loadAuthStatuses(connections);
      }
    } catch (error) {
      console.error('Failed to load SCM data:', error);
      showNotification('Failed to load SCM settings', 'error');
    } finally {
      loading = false;
    }
  }

  async function loadAuthStatuses(conns) {
    const results = await Promise.allSettled(
      conns.map(c => api.workspaceSCM.getAuthStatus(workspaceId, c.id))
    );
    results.forEach((res, i) => {
      if (res.status === 'fulfilled' && res.value) {
        authStatuses[conns[i].id] = res.value;
      }
    });
    authStatuses = authStatuses;
  }

  async function reconnectOAuth(conn) {
    try {
      sessionStorage.setItem('scm_oauth_return', window.location.href);
      const result = await api.workspaceSCM.startOAuth(workspaceId, conn.id);
      if (result?.auth_url) {
        window.location.href = result.auth_url;
      }
    } catch (error) {
      console.error('Failed to start OAuth:', error);
      showNotification('Failed to start OAuth reconnection', 'error');
    }
  }

  async function connectProvider(provider) {
    try {
      const newConn = await api.workspaceSCM.createConnection(workspaceId, {
        scm_provider_id: provider.id
      });
      connections = [...connections, newConn];
      // Update provider status
      availableProviders = availableProviders.map(p =>
        p.id === provider.id ? { ...p, is_connected: true } : p
      );
      showNotification(`Connected to ${provider.name}`, 'success');
    } catch (error) {
      console.error('Failed to connect provider:', error);
      showNotification('Failed to connect provider', 'error');
    }
  }

  async function disconnectProvider(conn) {
    if (!confirm(`Are you sure you want to disconnect ${conn.provider_name}? This will also unlink all repositories.`)) {
      return;
    }
    try {
      await api.workspaceSCM.deleteConnection(workspaceId, conn.id);
      connections = connections.filter(c => c.id !== conn.id);
      // Update provider status
      availableProviders = availableProviders.map(p =>
        p.id === conn.scm_provider_id ? { ...p, is_connected: false } : p
      );
      // Clean up expanded state and repos
      expandedConnections.delete(conn.id);
      expandedConnections = expandedConnections;
      delete linkedRepos[conn.id];
      linkedRepos = linkedRepos;
      showNotification(`Disconnected from ${conn.provider_name}`, 'success');
    } catch (error) {
      console.error('Failed to disconnect provider:', error);
      showNotification('Failed to disconnect provider', 'error');
    }
  }

  async function toggleExpanded(connId) {
    if (expandedConnections.has(connId)) {
      expandedConnections.delete(connId);
      expandedConnections = expandedConnections;
    } else {
      expandedConnections.add(connId);
      expandedConnections = expandedConnections;
      // Load repos if not already loaded
      if (!linkedRepos[connId]) {
        await loadLinkedRepos(connId);
      }
    }
  }

  async function loadLinkedRepos(connId) {
    loadingRepos.add(connId);
    loadingRepos = loadingRepos;
    try {
      const repos = await api.workspaceSCM.getLinkedRepos(workspaceId, connId);
      linkedRepos[connId] = repos || [];
      linkedRepos = linkedRepos;
    } catch (error) {
      console.error('Failed to load repositories:', error);
      linkedRepos[connId] = [];
      linkedRepos = linkedRepos;
    } finally {
      loadingRepos.delete(connId);
      loadingRepos = loadingRepos;
    }
  }

  function openRepoSelector(conn) {
    selectedConnection = conn;
    showRepoSelector = true;
  }

  async function handleReposLinked({ repos }) {
    if (selectedConnection && repos.length > 0) {
      // Refresh the linked repos for this connection
      await loadLinkedRepos(selectedConnection.id);
      // Update the connection's repo count
      connections = connections.map(c =>
        c.id === selectedConnection.id
          ? { ...c, repository_count: (linkedRepos[selectedConnection.id] || []).length }
          : c
      );
      showNotification(`Linked ${repos.length} repositor${repos.length === 1 ? 'y' : 'ies'}`, 'success');
    }
    showRepoSelector = false;
    selectedConnection = null;
  }

  async function unlinkRepo(connId, repo) {
    if (!confirm(`Are you sure you want to unlink ${repo.repository_name}?`)) {
      return;
    }
    try {
      await api.workspaceSCM.unlinkRepo(repo.id);
      linkedRepos[connId] = linkedRepos[connId].filter(r => r.id !== repo.id);
      linkedRepos = linkedRepos;
      // Update connection repo count
      connections = connections.map(c =>
        c.id === connId
          ? { ...c, repository_count: c.repository_count - 1 }
          : c
      );
      showNotification(`Unlinked ${repo.repository_name}`, 'success');
    } catch (error) {
      console.error('Failed to unlink repository:', error);
      showNotification('Failed to unlink repository', 'error');
    }
  }

  function showNotification(message, type = 'success') {
    if (type === 'success') {
      successToast(message);
    } else {
      errorToast(message);
    }
  }

  function getProviderIcon(providerType) {
    // Return appropriate styling based on provider type
    const colors = {
      github: '#333',
      gitlab: '#FC6D26',
      gitea: '#609926',
      bitbucket: '#0052CC'
    };
    return colors[providerType] || '#666';
  }

  function getProviderLabel(providerType) {
    const labels = {
      github: 'GitHub',
      gitlab: 'GitLab',
      gitea: 'Gitea',
      bitbucket: 'Bitbucket'
    };
    return labels[providerType] || providerType;
  }
</script>

<div class="space-y-6">
  <!-- Header -->
  <div class="flex items-center justify-between">
    <div class="flex items-center gap-3">
      <GitMerge class="w-5 h-5" style="color: var(--ds-text-subtle);" />
      <div>
        <h3 class="text-lg font-medium" style="color: var(--ds-text);">Source Control</h3>
        <p class="text-sm" style="color: var(--ds-text-subtle);">Connect SCM providers and link repositories to this workspace</p>
      </div>
    </div>
  </div>

  {#if loading}
    <div class="flex items-center justify-center py-12">
      <Loader2 class="w-6 h-6 animate-spin" style="color: var(--ds-text-subtle);" />
    </div>
  {:else}
    <!-- Available Providers Section -->
    {#if availableProviders.length > 0}
      <div class="rounded-lg border p-4" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
        <h4 class="text-sm font-medium mb-3" style="color: var(--ds-text);">Available Providers</h4>
        <div class="flex flex-wrap gap-2">
          {#each availableProviders as provider}
            <div
              class="flex items-center gap-2 px-3 py-2 rounded-lg border text-sm"
              style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);"
            >
              <span
                class="w-2 h-2 rounded-full"
                style="background-color: {getProviderIcon(provider.provider_type)};"
              ></span>
              <span style="color: var(--ds-text);">{provider.name}</span>
              <span class="text-xs px-1.5 py-0.5 rounded" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                {getProviderLabel(provider.provider_type)}
              </span>
              {#if provider.is_connected}
                <span class="flex items-center gap-1 text-xs" style="color: var(--ds-text-success);">
                  <Check class="w-3 h-3" />
                  Connected
                </span>
              {:else}
                <Button size="xs" variant="ghost" onclick={() => connectProvider(provider)}>
                  <Plus class="w-3 h-3 mr-1" />
                  Connect
                </Button>
              {/if}
            </div>
          {/each}
        </div>
      </div>
    {:else}
      <div class="rounded-lg border p-6 text-center" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
        <GitMerge class="w-8 h-8 mx-auto mb-2" style="color: var(--ds-text-subtlest);" />
        <p class="text-sm" style="color: var(--ds-text-subtle);">No SCM providers configured</p>
        <p class="text-xs mt-1" style="color: var(--ds-text-subtlest);">
          Ask a system administrator to configure SCM providers in the Admin panel
        </p>
      </div>
    {/if}

    <!-- Connected Providers & Repositories -->
    {#if connections.length > 0}
      <div class="space-y-3">
        <h4 class="text-sm font-medium" style="color: var(--ds-text);">Connected Providers</h4>

        {#each connections as conn}
          <div class="rounded-lg border overflow-hidden" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
            <!-- Connection Header -->
            <div
              class="flex items-center justify-between px-4 py-3 cursor-pointer hover:bg-opacity-50"
              style="background-color: var(--ds-surface);"
              onclick={() => toggleExpanded(conn.id)}
              onkeypress={(e) => e.key === 'Enter' && toggleExpanded(conn.id)}
              role="button"
              tabindex="0"
            >
              <div class="flex items-center gap-3">
                {#if expandedConnections.has(conn.id)}
                  <ChevronDown class="w-4 h-4" style="color: var(--ds-text-subtle);" />
                {:else}
                  <ChevronRight class="w-4 h-4" style="color: var(--ds-text-subtle);" />
                {/if}
                <span
                  class="w-3 h-3 rounded-full"
                  style="background-color: {getProviderIcon(conn.provider_type)};"
                ></span>
                <span class="font-medium" style="color: var(--ds-text);">{conn.provider_name}</span>
                <span class="text-xs px-1.5 py-0.5 rounded" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                  {conn.repository_count} {conn.repository_count === 1 ? 'repository' : 'repositories'}
                </span>
              </div>
              <div class="flex items-center gap-2" onclick={e => e.stopPropagation()}>
                {#if authStatuses[conn.id]?.auth_method === 'oauth' && !authStatuses[conn.id]?.has_workspace_token}
                  <Button size="sm" variant="ghost" onclick={() => reconnectOAuth(conn)}>
                    <KeyRound class="w-4 h-4 mr-1" />
                    Reconnect
                  </Button>
                {/if}
                <Button size="sm" variant="ghost" onclick={() => openRepoSelector(conn)}>
                  <Plus class="w-4 h-4 mr-1" />
                  Link Repositories
                </Button>
                <Button size="sm" variant="ghost" onclick={() => disconnectProvider(conn)}>
                  <Trash2 class="w-4 h-4" style="color: var(--ds-text-danger);" />
                </Button>
              </div>
            </div>

            <!-- Expanded Content - Linked Repositories -->
            {#if expandedConnections.has(conn.id)}
              <div class="border-t px-4 py-3" style="border-color: var(--ds-border);">
                {#if loadingRepos.has(conn.id)}
                  <div class="flex items-center justify-center py-4">
                    <Loader2 class="w-5 h-5 animate-spin" style="color: var(--ds-text-subtle);" />
                  </div>
                {:else if !linkedRepos[conn.id] || linkedRepos[conn.id].length === 0}
                  <div class="text-center py-4">
                    <p class="text-sm" style="color: var(--ds-text-subtle);">No repositories linked yet</p>
                    <Button size="sm" variant="secondary" class="mt-2" onclick={() => openRepoSelector(conn)}>
                      <Plus class="w-4 h-4 mr-1" />
                      Link Repositories
                    </Button>
                  </div>
                {:else}
                  <div class="space-y-2">
                    {#each linkedRepos[conn.id] as repo}
                      <div
                        class="flex items-center justify-between px-3 py-2 rounded-md"
                        style="background-color: var(--ds-surface);"
                      >
                        <div class="flex items-center gap-2">
                          <span class="font-mono text-sm" style="color: var(--ds-text);">{repo.repository_name}</span>
                          <span class="text-xs px-1.5 py-0.5 rounded" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                            {repo.default_branch}
                          </span>
                        </div>
                        <div class="flex items-center gap-2">
                          <a
                            href={repo.repository_url}
                            target="_blank"
                            rel="noopener noreferrer"
                            class="p-1 rounded hover:bg-opacity-50"
                            style="color: var(--ds-text-subtle);"
                          >
                            <ExternalLink class="w-4 h-4" />
                          </a>
                          <button
                            class="p-1 rounded hover:bg-opacity-50"
                            style="color: var(--ds-text-danger);"
                            onclick={() => unlinkRepo(conn.id, repo)}
                          >
                            <X class="w-4 h-4" />
                          </button>
                        </div>
                      </div>
                    {/each}
                  </div>
                {/if}
              </div>
            {/if}
          </div>
        {/each}
      </div>
    {/if}
  {/if}
</div>

<!-- Repository Selector Modal -->
{#if showRepoSelector && selectedConnection}
  <RepositorySelector
    {workspaceId}
    connection={selectedConnection}
    onclose={() => { showRepoSelector = false; selectedConnection = null; }}
    onlinked={handleReposLinked}
  />
{/if}

