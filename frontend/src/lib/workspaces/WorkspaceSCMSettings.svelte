<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import Button from '../components/Button.svelte';
  import { GitMerge, Plus, Trash2, ExternalLink, ChevronDown, ChevronRight, Loader2, Check, X, KeyRound, AlertTriangle, Settings } from '@lucide/svelte';
  import RepositorySelector from '../pickers/RepositorySelector.svelte';
  import { successToast, errorToast } from '../stores/toasts.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import DescriptionText from '../components/DescriptionText.svelte';
  import Checkbox from '../components/Checkbox.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import Input from '../components/Input.svelte';
  import { safeHref } from '../utils/sanitize';
  import { loadWorkspaceSCMOverview } from './workspaceSCMData.js';

  let { workspaceId } = $props();

  let loading = $state(true);
  let availableProviders = $state([]);
  let connections = $state([]);
  let expandedConnections = $state(new Set());
  let linkedRepos = $state({}); // connId -> repos array
  let loadingRepos = $state(new Set());
  let authStatuses = $state({}); // connId -> auth status object

  // Modal state
  let showRepoSelector = $state(false);
  let selectedConnection = $state(null);

  // Repo automation settings (per-repo glob patterns) inline editor.
  // Keyed by repo id; null means "not currently open."
  let editingRepoSettings = $state(null);

  function openRepoSettings(repo) {
    editingRepoSettings = {
      id: repo.id,
      milestone_tag_pattern: repo.milestone_tag_pattern || 'v*',
      milestone_branch_pattern: repo.milestone_branch_pattern || 'release/*'
    };
  }
  function closeRepoSettings() {
    editingRepoSettings = null;
  }
  async function saveRepoSettings(connId) {
    if (!editingRepoSettings) return;
    try {
      const updated = await api.workspaceSCM.updateRepo(editingRepoSettings.id, {
        milestone_tag_pattern: editingRepoSettings.milestone_tag_pattern,
        milestone_branch_pattern: editingRepoSettings.milestone_branch_pattern
      });
      // Patch the local list in place so the row reflects the new
      // values without a full refetch.
      const list = linkedRepos[connId] || [];
      const idx = list.findIndex((r) => r.id === updated.id);
      if (idx >= 0) {
        const nextList = [...list];
        nextList[idx] = updated;
        linkedRepos = { ...linkedRepos, [connId]: nextList };
      }
      successToast(t('scmSettings.repoSettingsSaved'));
      closeRepoSettings();
    } catch (err) {
      console.error(err);
      errorToast(t('scmSettings.repoSettingsSaveFailed'));
    }
  }


  onMount(async () => {
    await loadData();
  });

  async function loadData() {
    loading = true;
    try {
      const overview = await loadWorkspaceSCMOverview(api, workspaceId);
      availableProviders = overview.availableProviders;
      connections = overview.connections;
      authStatuses = overview.authStatuses;
    } catch (error) {
      console.error('Failed to load SCM data:', error);
      showNotification(t('scmSettings.loadFailed'), 'error');
    } finally {
      loading = false;
    }
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
      showNotification(t('scmSettings.oauthFailed'), 'error');
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
      showNotification(t('scmSettings.connectedTo', { name: provider.name }), 'success');
    } catch (error) {
      console.error('Failed to connect provider:', error);
      showNotification(t('scmSettings.connectFailed'), 'error');
    }
  }

  async function disconnectProvider(conn) {
    const confirmed = await confirm({
      title: t('common.disconnect'),
      message: t('scmSettings.disconnectMessage', { name: conn.provider_name }),
      confirmText: t('common.disconnect'),
      cancelText: t('common.cancel'),
      variant: 'danger'
    });
    if (!confirmed) return;
    try {
      await api.workspaceSCM.deleteConnection(workspaceId, conn.id);
      connections = connections.filter(c => c.id !== conn.id);
      // Update provider status
      availableProviders = availableProviders.map(p =>
        p.id === conn.scm_provider_id ? { ...p, is_connected: false } : p
      );
      // Clean up expanded state and repos
      expandedConnections = new Set([...expandedConnections].filter(id => id !== conn.id));
      linkedRepos = Object.fromEntries(
        Object.entries(linkedRepos).filter(([id]) => id !== String(conn.id))
      );
      showNotification(t('scmSettings.disconnectedFrom', { name: conn.provider_name }), 'success');
    } catch (error) {
      console.error('Failed to disconnect provider:', error);
      showNotification(t('scmSettings.disconnectFailed'), 'error');
    }
  }

  async function toggleExpanded(connId) {
    if (expandedConnections.has(connId)) {
      expandedConnections = new Set([...expandedConnections].filter(id => id !== connId));
    } else {
      expandedConnections = new Set(expandedConnections).add(connId);
      // Load repos if not already loaded
      if (!linkedRepos[connId]) {
        await loadLinkedRepos(connId);
      }
    }
  }

  async function loadLinkedRepos(connId) {
    loadingRepos = new Set(loadingRepos).add(connId);
    try {
      const repos = await api.workspaceSCM.getLinkedRepos(workspaceId, connId);
      linkedRepos = { ...linkedRepos, [connId]: repos || [] };
    } catch (error) {
      if (error?.name === 'AbortError') return;
      console.error('Failed to load repositories:', error);
      linkedRepos = { ...linkedRepos, [connId]: [] };
    } finally {
      loadingRepos = new Set([...loadingRepos].filter(id => id !== connId));
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
      showNotification(t('scmSettings.linkedRepositories', { count: repos.length }), 'success');
    }
    showRepoSelector = false;
    selectedConnection = null;
  }

  async function toggleSmartCommits(conn, next) {
    const prev = conn.smart_commits_enabled;
    // Optimistic update so the warning renders immediately.
    connections = connections.map(c =>
      c.id === conn.id ? { ...c, smart_commits_enabled: next } : c
    );
    try {
      const updated = await api.workspaceSCM.updateConnection(workspaceId, conn.id, {
        smart_commits_enabled: next,
      });
      connections = connections.map(c => (c.id === conn.id ? { ...c, ...updated } : c));
    } catch (error) {
      console.error('Failed to update smart commits setting:', error);
      // Revert on failure.
      connections = connections.map(c =>
        c.id === conn.id ? { ...c, smart_commits_enabled: prev } : c
      );
      showNotification(t('scm.smartCommitsUpdateFailed'), 'error');
    }
  }

  async function unlinkRepo(connId, repo) {
    const confirmed = await confirm({
      title: t('common.remove'),
      message: t('scmSettings.unlinkMessage', { name: repo.repository_name }),
      confirmText: t('common.remove'),
      cancelText: t('common.cancel'),
      variant: 'danger'
    });
    if (!confirmed) return;
    try {
      await api.workspaceSCM.unlinkRepo(repo.id);
      linkedRepos = {
        ...linkedRepos,
        [connId]: linkedRepos[connId].filter(r => r.id !== repo.id)
      };
      // Update connection repo count
      connections = connections.map(c =>
        c.id === connId
          ? { ...c, repository_count: c.repository_count - 1 }
          : c
      );
      showNotification(t('scmSettings.unlinkedRepository', { name: repo.repository_name }), 'success');
    } catch (error) {
      console.error('Failed to unlink repository:', error);
      showNotification(t('scmSettings.unlinkFailed'), 'error');
    }
  }

  function showNotification(message, type = 'success') {
    if (type === 'success') {
      successToast(message);
    } else {
      errorToast(message);
    }
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

<div class="space-y-6" data-testid="workspace-scm-settings">
  <!-- Header -->
  <div class="flex items-center justify-between">
    <div class="flex items-center gap-3">
      <GitMerge class="w-5 h-5" style="color: var(--ds-text-subtle);" />
      <div>
        <h3 class="text-lg font-medium" style="color: var(--ds-text);">{t('scmSettings.title')}</h3>
        <p class="text-sm" style="color: var(--ds-text-subtle);">{t('scmSettings.subtitle')}</p>
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
        <h4 class="text-sm font-medium mb-3" style="color: var(--ds-text);">{t('scmSettings.availableProviders')}</h4>
        <div class="flex flex-wrap gap-2">
          {#each availableProviders as provider}
            <div
              data-testid={`scm-provider-${provider.id}`}
              class="flex items-center gap-2 px-3 py-2 rounded-lg border text-sm"
              style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);"
            >
              <span style="color: var(--ds-text);">{provider.name}</span>
              <span class="text-xs px-1.5 py-0.5 rounded" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                {getProviderLabel(provider.provider_type)}
              </span>
              {#if provider.is_connected}
                <span class="flex items-center gap-1 text-xs" style="color: var(--ds-text-success);">
                  <Check class="w-3 h-3" />
                  {t('scmSettings.connected')}
                </span>
              {:else}
                <Button dataTestid={`scm-connect-${provider.id}`} size="xs" variant="ghost" onclick={() => connectProvider(provider)}>
                  <Plus class="w-3 h-3 mr-1" />
                  {t('scmSettings.connect')}
                </Button>
              {/if}
            </div>
          {/each}
        </div>
      </div>
    {:else}
      <div class="rounded-lg border p-6 text-center" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
        <GitMerge class="w-8 h-8 mx-auto mb-2" style="color: var(--ds-text-subtlest);" />
        <p class="text-sm" style="color: var(--ds-text-subtle);">{t('scmSettings.noProviders')}</p>
        <DescriptionText variant="subtlest">
          {t('scmSettings.noProvidersDescription')}
        </DescriptionText>
      </div>
    {/if}

    <!-- Connected Providers & Repositories -->
    {#if connections.length > 0}
      <div class="space-y-3">
        <h4 class="text-sm font-medium" style="color: var(--ds-text);">{t('scmSettings.connectedProviders')}</h4>

        {#each connections as conn}
          <div data-testid={`scm-connection-${conn.id}`} class="rounded-lg border overflow-hidden" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
            <!-- Connection Header -->
            <div
              class="flex items-center justify-between px-4 py-3 cursor-pointer hover:bg-opacity-50"
              style="background-color: var(--ds-surface);"
              onclick={() => toggleExpanded(conn.id)}
              onkeydown={(e) => (e.key === 'Enter' || e.key === ' ') && toggleExpanded(conn.id)}
              role="button"
              tabindex="0"
            >
              <div class="flex items-center gap-3">
                {#if expandedConnections.has(conn.id)}
                  <ChevronDown class="w-4 h-4" style="color: var(--ds-text-subtle);" />
                {:else}
                  <ChevronRight class="w-4 h-4" style="color: var(--ds-text-subtle);" />
                {/if}
                <span class="font-medium" style="color: var(--ds-text);">{conn.provider_name}</span>
                <span class="text-xs px-1.5 py-0.5 rounded" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                  {t('scmSettings.repositoryCount', { count: conn.repository_count })}
                </span>
              </div>
              <!-- svelte-ignore a11y_click_events_have_key_events -->
              <!-- svelte-ignore a11y_no_static_element_interactions -->
              <div class="flex items-center gap-2" onclick={e => e.stopPropagation()}>
                {#if authStatuses[conn.id]?.auth_method === 'oauth' && !authStatuses[conn.id]?.has_workspace_token}
                  <Button size="sm" variant="ghost" onclick={() => reconnectOAuth(conn)}>
                    <KeyRound class="w-4 h-4 mr-1" />
                    {t('scmSettings.reconnect')}
                  </Button>
                {/if}
                <Button size="sm" variant="ghost" onclick={() => openRepoSelector(conn)}>
                  <Plus class="w-4 h-4 mr-1" />
                  {t('scmSettings.linkRepositories')}
                </Button>
                <Button size="sm" variant="danger-ghost" icon={Trash2} title={t('common.disconnect')} onclick={() => disconnectProvider(conn)}></Button>
              </div>
            </div>

            <!-- Expanded Content - Settings + Linked Repositories -->
            {#if expandedConnections.has(conn.id)}
              <div class="border-t px-4 py-3 space-y-4" style="border-color: var(--ds-border);">
                <!-- Smart-commit toggle -->
                <div class="rounded-md p-3" style="background-color: var(--ds-surface);">
                  <Checkbox
                    checked={conn.smart_commits_enabled}
                    onchange={(checked) => toggleSmartCommits(conn, checked)}
                    dataTestid={`scm-smart-commits-${conn.id}`}
                    label={t('scm.smartCommitsTitle')}
                    hint={t('scm.smartCommitsDescription')}
                    size="small"
                  />
                  {#if conn.smart_commits_enabled}
                    <div class="mt-3 flex items-start gap-2 p-2 rounded text-xs" style="background-color: var(--ds-background-warning, #fff7e6); color: var(--ds-text-warning, #b35900); border: 1px solid var(--ds-border-warning, #f0b64d);">
                      <AlertTriangle class="w-4 h-4 flex-shrink-0 mt-0.5" />
                      <div>
                        <div class="font-medium">{t('scm.smartCommitsWarningTitle')}</div>
                        <div class="mt-0.5">{t('scm.smartCommitsWarningBody')}</div>
                      </div>
                    </div>
                  {/if}
                </div>

                <!-- Linked repositories -->
                {#if loadingRepos.has(conn.id)}
                  <div class="flex items-center justify-center py-4">
                    <Loader2 class="w-5 h-5 animate-spin" style="color: var(--ds-text-subtle);" />
                  </div>
                {:else if !linkedRepos[conn.id] || linkedRepos[conn.id].length === 0}
                  <EmptyState title={t('scmSettings.noRepositories')}>
                    {#snippet action()}
                      <!-- shortcut-guard-exempt: contextual repository linking is not a global create action -->
                      <Button size="sm" variant="secondary" icon={Plus} onclick={() => openRepoSelector(conn)}>
                        {t('scmSettings.linkRepositories')}
                      </Button>
                    {/snippet}
                  </EmptyState>
                {:else}
                  <div class="space-y-2">
                    {#each linkedRepos[conn.id] as repo}
                      <div class="rounded-md" style="background-color: var(--ds-surface);">
                        <div class="flex items-center justify-between px-3 py-2">
                          <div class="flex items-center gap-2">
                            <span class="font-mono text-sm" style="color: var(--ds-text);">{repo.repository_name}</span>
                            <span class="text-xs px-1.5 py-0.5 rounded" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                              {repo.default_branch}
                            </span>
                            <span class="text-xs px-1.5 py-0.5 rounded font-mono" title={t('scmSettings.tagPattern')} style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                              {t('scmSettings.tags')}: {repo.milestone_tag_pattern || 'v*'}
                            </span>
                            <span class="text-xs px-1.5 py-0.5 rounded font-mono" title={t('scmSettings.branchPattern')} style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                              {t('scmSettings.branches')}: {repo.milestone_branch_pattern || 'release/*'}
                            </span>
                          </div>
                          <div class="flex items-center gap-2">
                            <button
                              class="p-1 rounded hover:bg-opacity-50"
                              style="color: var(--ds-text-subtle);"
                              title={t('scmSettings.repositorySettings')}
                              onclick={() => editingRepoSettings?.id === repo.id ? closeRepoSettings() : openRepoSettings(repo)}
                            >
                              <Settings class="w-4 h-4" />
                            </button>
                            <a
                              href={safeHref(repo.repository_url)}
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
                        {#if editingRepoSettings?.id === repo.id}
                          <div class="px-3 py-3 border-t" style="border-color: var(--ds-border);">
                            <p class="text-xs mb-3" style="color: var(--ds-text-subtle);">
                              {t('scmSettings.patternsHelp')}
                            </p>
                            <div class="grid grid-cols-2 gap-3">
                              <div>
                                <label for="ws-scm-tag-pattern-{repo.id}" class="block text-xs font-medium mb-1">{t('scmSettings.tagPattern')}</label>
                                <Input
                                  id="ws-scm-tag-pattern-{repo.id}"
                                  type="text"
                                  class="font-mono"
                                  bind:value={editingRepoSettings.milestone_tag_pattern}
                                  placeholder="v*"
                                />
                              </div>
                              <div>
                                <label for="ws-scm-branch-pattern-{repo.id}" class="block text-xs font-medium mb-1">{t('scmSettings.branchPattern')}</label>
                                <Input
                                  id="ws-scm-branch-pattern-{repo.id}"
                                  type="text"
                                  class="font-mono"
                                  bind:value={editingRepoSettings.milestone_branch_pattern}
                                  placeholder="release/*"
                                />
                              </div>
                            </div>
                            <div class="flex justify-end gap-2 mt-3">
                              <Button size="sm" variant="ghost" onclick={closeRepoSettings}>{t('common.cancel')}</Button>
                              <Button size="sm" variant="primary" onclick={() => saveRepoSettings(conn.id)}>{t('common.save')}</Button>
                            </div>
                          </div>
                        {/if}
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
