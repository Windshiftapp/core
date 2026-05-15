<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { ExternalLink, Plus, RefreshCw, Trash2, ChevronDown, ChevronRight, Loader2, Link2 } from '@lucide/svelte';
  import Button from '../../components/Button.svelte';
  import Text from '../../components/Text.svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { confirm } from '../../composables/useConfirm.js';
  import { successToast, errorToast } from '../../stores/toasts.svelte.js';
  import { safeHref } from '../../utils/sanitize';

  let { itemId, onaddlink } = $props();

  let loading = $state(true);
  let links = $state([]);
  let expanded = $state(true);
  let refreshingId = $state(null);
  let error = $state(null);
  let hasProviders = $state(false);
  let hasConnection = $state(false);
  let checkingStatus = $state(true);

  onMount(async () => {
    await checkStatus();
    if (hasConnection) {
      await loadLinks();
    } else {
      loading = false;
    }
  });

  async function checkStatus() {
    try {
      const available = await api.userIntegrations.getAvailableProviders() || [];
      hasProviders = available.length > 0;
      if (hasProviders) {
        const connections = await api.userIntegrations.getConnections() || [];
        hasConnection = connections.length > 0;
      }
    } catch (err) {
      console.error('Failed to check integration status:', err);
    } finally {
      checkingStatus = false;
    }
  }

  export async function loadLinks() {
    if (!itemId) return;
    loading = true;
    error = null;

    try {
      links = await api.itemIntegrationLinks.get(itemId) || [];
    } catch (err) {
      console.error('Failed to load integration links:', err);
      error = t('integrations.failedToLoadLinks');
      links = [];
    } finally {
      loading = false;
    }
  }

  async function refreshLink(linkId) {
    refreshingId = linkId;
    try {
      const updated = await api.itemIntegrationLinks.refresh(linkId);
      links = links.map(l => l.id === linkId ? updated : l);
      successToast(t('integrations.refreshed'));
    } catch (err) {
      console.error('Failed to refresh link:', err);
      errorToast('Failed to refresh link');
    } finally {
      refreshingId = null;
    }
  }

  async function deleteLink(linkId) {
    const confirmed = await confirm({
      title: t('common.remove'),
      message: t('integrations.confirmRemoveLink'),
      confirmText: t('common.remove'),
      cancelText: t('common.cancel'),
      variant: 'danger'
    });
    if (!confirmed) return;

    try {
      await api.itemIntegrationLinks.delete(linkId);
      links = links.filter(l => l.id !== linkId);
      successToast(t('integrations.removed'));
    } catch (err) {
      console.error('Failed to delete link:', err);
      errorToast('Failed to delete link');
    }
  }

  function getProviderTypeLabel(type) {
    switch (type) {
      case 'notion': return t('integrations.notion');
      default: return type;
    }
  }

  function connectIntegration() {
    // Redirect to connected accounts page
    window.location.href = '/profile?tab=connected-accounts';
  }
</script>

{#if hasProviders || checkingStatus}
<div class="mb-4">
  <div class="border-t my-4" style="border-color: var(--ds-border);"></div>

  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="w-full flex items-center justify-between mb-3 group cursor-pointer"
    onclick={() => expanded = !expanded}
  >
    <div class="flex items-center gap-2">
      <Text variant="subtle" size="xs" weight="semibold" class="uppercase tracking-wider">{t('integrations.title')}</Text>
      {#if links.length > 0}
        <span class="text-xs px-1.5 py-0.5 rounded" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
          {links.length}
        </span>
      {/if}
    </div>
    <div class="flex items-center gap-1">
      {#if hasConnection}
        <button
          class="p-1 rounded transition-colors opacity-0 group-hover:opacity-100"
          class:invisible={!expanded}
          onclick={e => { e.stopPropagation(); onaddlink?.(); }}
          title={t('integrations.linkPage')}
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
      {#if checkingStatus || loading}
        <div class="flex items-center justify-center py-3">
          <Loader2 class="w-4 h-4 animate-spin" style="color: var(--ds-text-subtle);" />
        </div>
      {:else if !hasConnection && hasProviders}
        <div class="py-3 px-3 rounded-md" style="background-color: var(--ds-background-neutral);">
          <div class="flex items-center gap-2 mb-2">
            <Link2 class="w-4 h-4" style="color: var(--ds-text-subtle);" />
            <Text size="sm" weight="medium">{t('integrations.connectAccount')}</Text>
          </div>
          <p class="text-xs mb-3" style="color: var(--ds-text-subtle);">
            {t('integrations.connectToLink')}
          </p>
          <Button size="sm" variant="primary" onclick={connectIntegration}>
            {t('integrations.connect')}
          </Button>
        </div>
      {:else if error}
        <p class="text-xs py-2" style="color: var(--ds-text-danger);">{error}</p>
      {:else if links.length === 0}
        <p class="text-xs py-2" style="color: var(--ds-text-subtle);">{t('integrations.noLinksYet')}</p>
      {:else}
        {#each links as link}
          <div
            class="group flex items-center gap-2 py-1.5 px-2 rounded-md transition-colors hover:bg-[var(--ds-background-neutral)]"
          >
            {#if link.icon}
              <span class="text-sm flex-shrink-0">{link.icon}</span>
            {:else}
              <ExternalLink class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
            {/if}

            <div class="flex-1 min-w-0">
              <a
                href={safeHref(link.external_url)}
                target="_blank"
                rel="noopener noreferrer"
                class="text-sm truncate block hover:underline"
                style="color: var(--ds-link);"
                onclick={e => e.stopPropagation()}
              >
                {link.title}
              </a>
              <div class="flex items-center gap-1.5 mt-0.5">
                <span
                  class="text-xs px-1.5 py-0.5 rounded"
                  style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);"
                >
                  {getProviderTypeLabel(link.provider_type)}
                </span>
                {#if link.link_type && link.link_type !== 'page'}
                  <span class="text-xs" style="color: var(--ds-text-subtlest);">{link.link_type}</span>
                {/if}
              </div>
            </div>

            <div class="flex items-center gap-0.5 opacity-0 group-hover:opacity-100 transition-opacity">
              <button
                class="p-1 rounded hover:bg-[var(--ds-background-neutral-hovered)]"
                onclick={() => refreshLink(link.id)}
                title="Refresh"
                disabled={refreshingId === link.id}
              >
                {#if refreshingId === link.id}
                  <Loader2 class="w-3.5 h-3.5 animate-spin" style="color: var(--ds-text-subtle);" />
                {:else}
                  <RefreshCw class="w-3.5 h-3.5" style="color: var(--ds-text-subtle);" />
                {/if}
              </button>
              <button
                class="p-1 rounded hover:bg-[var(--ds-background-neutral-hovered)]"
                onclick={() => deleteLink(link.id)}
                title={t('common.remove')}
              >
                <Trash2 class="w-3.5 h-3.5" style="color: var(--ds-text-danger);" />
              </button>
            </div>
          </div>
        {/each}
      {/if}
    </div>
  {/if}
</div>
{/if}
