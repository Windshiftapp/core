<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { GitMerge, GitBranch, GitCommit, ExternalLink, Plus, RefreshCw, Trash2, ChevronDown, ChevronRight, Loader2, GitBranchPlus, Link2 } from 'lucide-svelte';
  import Button from '../../components/Button.svelte';
  import Text from '../../components/Text.svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { confirm } from '../../composables/useConfirm.js';

  export let itemId;

  let loading = true;
  let links = [];
  let expanded = true;
  let refreshing = false;
  let error = null;

  // SCM connection status
  let connectionStatus = null;
  let checkingConnection = true;

  // Event dispatcher for opening the add link modal
  import { createEventDispatcher } from 'svelte';
  const dispatch = createEventDispatcher();

  onMount(async () => {
    await checkConnectionStatus();
    if (connectionStatus?.connected) {
      await loadLinks();
    } else {
      loading = false;
    }
  });

  async function checkConnectionStatus() {
    if (!itemId) {
      checkingConnection = false;
      return;
    }

    try {
      connectionStatus = await api.itemSCMLinks.getConnectionStatus(itemId);
    } catch (err) {
      console.error('Failed to check SCM connection status:', err);
      // If we can't check connection status, assume no repos configured
      connectionStatus = { has_repositories: false };
    } finally {
      checkingConnection = false;
    }
  }

  function startOAuthConnect() {
    if (!connectionStatus?.provider_slug) return;

    // Store return URL so we come back to this item
    const returnUrl = window.location.href;
    sessionStorage.setItem('scm_oauth_return', returnUrl);

    // Start OAuth flow
    api.scmProviders.startOAuth(connectionStatus.provider_slug).then(result => {
      if (result?.auth_url) {
        window.location.href = result.auth_url;
      }
    }).catch(err => {
      console.error('Failed to start OAuth:', err);
      error = t('scm.failedToStartConnection');
    });
  }

  // Export for parent component to call
  export async function loadLinks() {
    if (!itemId) return;
    loading = true;
    error = null;

    try {
      links = await api.itemSCMLinks.get(itemId) || [];
      // Re-fetch after short delay to pick up background OAuth refresh
      if (links.some(l => l.link_type === 'pull_request' && l.state !== 'merged')) {
        setTimeout(async () => {
          try {
            const updated = await api.itemSCMLinks.get(itemId) || [];
            if (JSON.stringify(updated) !== JSON.stringify(links)) {
              links = updated;
            }
          } catch (_) { /* silent */ }
        }, 3000);
      }
    } catch (err) {
      console.error('Failed to load SCM links:', err);
      error = t('scm.failedToLoadLinks');
      links = [];
    } finally {
      loading = false;
    }
  }

  async function refreshLink(linkId) {
    refreshing = true;
    try {
      const updatedLink = await api.itemSCMLinks.refresh(linkId);
      // Update the link in our list
      links = links.map(l => l.id === linkId ? updatedLink : l);
    } catch (err) {
      console.error('Failed to refresh link:', err);
    } finally {
      refreshing = false;
    }
  }

  async function deleteLink(linkId) {
    const confirmed = await confirm({
      title: t('common.remove'),
      message: t('scm.confirmRemoveLink'),
      confirmText: t('common.remove'),
      cancelText: t('common.cancel'),
      variant: 'danger'
    });
    if (!confirmed) return;

    try {
      await api.itemSCMLinks.delete(linkId);
      links = links.filter(l => l.id !== linkId);
    } catch (err) {
      console.error('Failed to delete link:', err);
    }
  }

  function openAddLinkModal() {
    dispatch('add-link');
  }

  function openCreateBranchModal() {
    dispatch('create-branch');
  }

  function getLinkIcon(linkType) {
    switch (linkType) {
      case 'pull_request': return GitMerge;
      case 'branch': return GitBranch;
      case 'commit': return GitCommit;
      default: return GitBranch;
    }
  }

  function getLinkTypeLabel(linkType) {
    switch (linkType) {
      case 'pull_request': return 'PR';
      case 'branch': return 'Branch';
      case 'commit': return 'Commit';
      default: return linkType;
    }
  }

  function getStateColor(state) {
    switch (state) {
      case 'open': return { bg: 'var(--ds-background-success)', text: 'var(--ds-text-success)' };
      case 'merged': return { bg: 'var(--ds-background-accent-purple)', text: 'var(--ds-accent-purple)' };
      case 'closed': return { bg: 'var(--ds-background-danger)', text: 'var(--ds-text-danger)' };
      default: return { bg: 'var(--ds-background-neutral)', text: 'var(--ds-text-subtle)' };
    }
  }

  function getDisplayText(link) {
    if (link.link_type === 'pull_request') {
      return link.title || `#${link.external_id}`;
    }
    if (link.link_type === 'commit') {
      return link.title || link.external_id.substring(0, 7);
    }
    return link.title || link.external_id;
  }

  function getRepoName(link) {
    // Extract short repo name (last part of org/repo)
    const parts = (link.repository_name || '').split('/');
    return parts[parts.length - 1] || link.repository_name;
  }

  function openCreatePRModal(link) {
    dispatch('create-pr', { link });
  }
</script>

<!-- Development Section -->
<div class="mb-4">
  <!-- Divider -->
  <div class="border-t my-4" style="border-color: var(--ds-border);"></div>

  <!-- Section Header -->
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="w-full flex items-center justify-between mb-3 group cursor-pointer"
    onclick={() => expanded = !expanded}
  >
    <div class="flex items-center gap-2">
      <Text variant="subtle" size="xs" weight="semibold" class="uppercase tracking-wider">{t('scm.development')}</Text>
      {#if links.length > 0}
        <span class="text-xs px-1.5 py-0.5 rounded" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
          {links.length}
        </span>
      {/if}
    </div>
    <div class="flex items-center gap-1">
      {#if connectionStatus?.connected}
        <button
          class="p-1 rounded transition-colors opacity-0 group-hover:opacity-100"
          class:invisible={!expanded}
          onclick={e => { e.stopPropagation(); openCreateBranchModal(); }}
          title={t('scm.createBranch')}
        >
          <GitBranchPlus class="w-4 h-4" style="color: var(--ds-text-subtle);" />
        </button>
        <button
          class="p-1 rounded transition-colors opacity-0 group-hover:opacity-100"
          class:invisible={!expanded}
          onclick={e => { e.stopPropagation(); openAddLinkModal(); }}
          title={t('scm.linkExisting')}
        >
          <Plus class="w-4 h-4" style="color: var(--ds-text-subtle);" />
        </button>
      {/if}
      {#if expanded}
        <ChevronDown class="w-4 h-4" style="color: var(--ds-text-subtle);" />
      {:else}
        <ChevronRight class="w-4 h-4" style="color: var(--ds-text-subtle);" />
      {/if}
    </div>
  </div>

  {#if expanded}
    <div class="space-y-2 mt-1">
      {#if checkingConnection || loading}
        <div class="flex items-center justify-center py-3">
          <Loader2 class="w-4 h-4 animate-spin" style="color: var(--ds-text-subtle);" />
        </div>
      {:else if !connectionStatus?.has_repositories}
        <!-- No SCM repositories configured for this workspace -->
        <p class="text-xs py-2" style="color: var(--ds-text-subtle);">{t('scm.noRepositoriesLinked')}</p>
      {:else if !connectionStatus?.connected}
        <!-- User hasn't connected their SCM account -->
        <div class="py-3 px-3 rounded-md" style="background-color: var(--ds-background-neutral);">
          <div class="flex items-center gap-2 mb-2">
            <Link2 class="w-4 h-4" style="color: var(--ds-text-subtle);" />
            <Text size="sm" weight="medium">{t('scm.connectYourAccount', { provider: connectionStatus?.provider_name || 'Git' })}</Text>
          </div>
          <p class="text-xs mb-3" style="color: var(--ds-text-subtle);">
            {t('scm.connectToCreate')}
          </p>
          <Button size="sm" variant="primary" onclick={startOAuthConnect}>
            {t('scm.connect', { provider: connectionStatus?.provider_name || t('common.account') })}
          </Button>
        </div>
      {:else if error}
        <p class="text-xs py-2" style="color: var(--ds-text-danger);">{error}</p>
      {:else if links.length === 0}
        <p class="text-xs py-2" style="color: var(--ds-text-subtle);">{t('scm.noLinksYet')}</p>
      {:else}
        {#each links as link}
          <div
            class="flex items-start gap-2 px-2 py-2 rounded-md group transition-colors"
            style="background-color: var(--ds-surface);"
          >
            <!-- Icon -->
            <svelte:component
              this={getLinkIcon(link.link_type)}
              class="w-4 h-4 flex-shrink-0 mt-0.5"
              style="color: var(--ds-text-subtle);"
            />

            <!-- Content -->
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2 flex-wrap">
                <!-- Title/Number -->
                <a
                  href={link.external_url}
                  target="_blank"
                  rel="noopener noreferrer"
                  class="text-sm font-medium hover:underline truncate"
                  style="color: var(--ds-text);"
                  title={link.title || link.external_id}
                >
                  {#if link.link_type === 'pull_request'}
                    #{link.external_id}
                  {:else if link.link_type === 'commit'}
                    {link.external_id.substring(0, 7)}
                  {:else}
                    {link.external_id}
                  {/if}
                </a>

                <!-- State badge for PRs -->
                {#if link.link_type === 'pull_request' && link.state}
                  {@const colors = getStateColor(link.state)}
                  <span
                    class="text-xs px-1.5 py-0.5 rounded capitalize"
                    style="background-color: {colors.bg}; color: {colors.text};"
                  >
                    {link.state}
                  </span>
                {/if}
              </div>

              <!-- Title (if different from external_id) -->
              {#if link.title && link.link_type !== 'branch'}
                <p class="text-xs truncate mt-0.5" style="color: var(--ds-text-subtle);" title={link.title}>
                  {link.title}
                </p>
              {/if}

              <!-- Repository info -->
              <div class="flex items-center gap-2 mt-1 text-xs" style="color: var(--ds-text-subtlest);">
                <span>{getRepoName(link)}</span>
                {#if link.author_name}
                  <span>·</span>
                  <span>{link.author_name}</span>
                {/if}
              </div>
            </div>

            <!-- Actions -->
            <div class="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
              {#if link.link_type === 'branch'}
                <button
                  class="p-1 rounded hover:bg-opacity-50"
                  style="color: var(--ds-text-subtle);"
                  onclick={() => openCreatePRModal(link)}
                  title={t('scm.createPullRequest')}
                >
                  <GitMerge class="w-3 h-3" />
                </button>
              {/if}
              <button
                class="p-1 rounded hover:bg-opacity-50"
                style="color: var(--ds-text-subtle);"
                onclick={() => refreshLink(link.id)}
                title={t('common.refresh')}
                disabled={refreshing}
              >
                <RefreshCw class="w-3 h-3 {refreshing ? 'animate-spin' : ''}" />
              </button>
              <a
                href={link.external_url}
                target="_blank"
                rel="noopener noreferrer"
                class="p-1 rounded hover:bg-opacity-50"
                style="color: var(--ds-text-subtle);"
                title={t('common.openInNewTab')}
              >
                <ExternalLink class="w-3 h-3" />
              </a>
              <button
                class="p-1 rounded hover:bg-opacity-50"
                style="color: var(--ds-text-danger);"
                onclick={() => deleteLink(link.id)}
                title={t('items.removeLink')}
              >
                <Trash2 class="w-3 h-3" />
              </button>
            </div>
          </div>
        {/each}
      {/if}
    </div>
  {/if}
</div>
