<script>
  import { onDestroy, onMount } from 'svelte';
  import Spinner from '../../components/Spinner.svelte';
  import { itemIntegrationLinks, userIntegrations } from '../../api/integrations.js';
  import ExternalLink from '@lucide/svelte/icons/external-link';
  import Plus from '@lucide/svelte/icons/plus';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import Link2 from '@lucide/svelte/icons/link-2';
  import Button from '../../components/Button.svelte';
  import Text from '../../components/Text.svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { confirm } from '../../composables/useConfirm.js';
  import { successToast, errorToast } from '../../stores/toasts.svelte.js';
  import { safeHref } from '../../utils/sanitize';
  import { toExternal } from '../../runtime/contextPath.js';

  let { itemId, onaddlink } = $props();

  const LINKABLE_PROVIDER_TYPES = new Set(['notion']);

  let loading = $state(true);
  let links = $state([]);
  let expanded = $state(true);
  let refreshingId = $state(null);
  let error = $state(null);
  let hasProviders = $state(false);
  let hasConnection = $state(false);
  let checkingStatus = $state(true);
  const loadController = new AbortController();

  onMount(async () => {
    await checkStatus(loadController.signal);
    if (!loadController.signal.aborted) {
      await loadLinks(loadController.signal);
    }
  });

  onDestroy(() => loadController.abort());

  async function checkStatus(signal) {
    try {
      const available = await userIntegrations.getAvailableProviders({ signal }) || [];
      const linkableProviders = available.filter((provider) =>
        LINKABLE_PROVIDER_TYPES.has(provider.provider_type?.toLowerCase())
      );
      hasProviders = linkableProviders.length > 0;
      if (hasProviders) {
        const connections = await userIntegrations.getConnections({ signal }) || [];
        const connectedProviderIds = new Set(
          connections.map((connection) => connection.integration_provider_id)
        );
        hasConnection = linkableProviders.some((provider) =>
          connectedProviderIds.has(provider.id)
        );
      }
    } catch (err) {
      if (err?.name === 'AbortError') return;
      console.error('Failed to check integration status:', err);
    } finally {
      checkingStatus = false;
    }
  }

  export async function loadLinks(signal) {
    if (!itemId) return;
    loading = true;
    error = null;

    try {
      links = await itemIntegrationLinks.get(itemId, signal ? { signal } : undefined) || [];
    } catch (err) {
      if (signal?.aborted || err?.name === 'AbortError' || err?.status === 404) return;
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
      const updated = await itemIntegrationLinks.refresh(linkId);
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
      await itemIntegrationLinks.delete(linkId);
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
    window.location.href = toExternal('/profile?tab=connected-accounts');
  }
</script>

{#if hasProviders || links.length > 0 || checkingStatus}
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
          class="p-1 rounded transition-colors opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 pointer-coarse:opacity-100"
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
          <Spinner size="sm" />
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

            <div class="flex items-center gap-0.5 opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 pointer-coarse:opacity-100 transition-opacity">
              <button
                class="p-1 rounded hover:bg-[var(--ds-background-neutral-hovered)]"
                onclick={() => refreshLink(link.id)}
                title="Refresh"
                disabled={refreshingId === link.id}
              >
                {#if refreshingId === link.id}
                  <Spinner size="sm" />
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
