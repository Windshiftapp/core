<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import Button from '../components/Button.svelte';
  import Label from '../components/Label.svelte';
  import Modal from './Modal.svelte';
  import ModalHeader from './ModalHeader.svelte';
  import { Search, ExternalLink, Loader2 } from '@lucide/svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { successToast, errorToast } from '../stores/toasts.svelte.js';

  let { itemId, oncreated, onclose } = $props();

  let loading = $state(true);
  let providers = $state([]);
  let connections = $state([]);
  let selectedProviderId = $state(null);
  let searchQuery = $state('');
  let searchResults = $state([]);
  let searching = $state(false);
  let linking = $state(null);
  let error = $state(null);
  let searchTimeout = null;

  onMount(async () => {
    await loadProviders();
  });

  async function loadProviders() {
    loading = true;
    try {
      const [avail, conns] = await Promise.all([
        api.userIntegrations.getAvailableProviders(),
        api.userIntegrations.getConnections(),
      ]);
      providers = avail || [];
      connections = conns || [];

      // Only show providers the user is connected to
      const connectedProviderIds = new Set(connections.map(c => c.integration_provider_id));
      providers = providers.filter(p => connectedProviderIds.has(p.id));

      if (providers.length === 1) {
        selectedProviderId = providers[0].id;
      }
    } catch (err) {
      console.error('Failed to load providers:', err);
      error = t('integrations.failedToLoadLinks');
    } finally {
      loading = false;
    }
  }

  function onSearchInput(e) {
    searchQuery = e.target.value;
    if (searchTimeout) clearTimeout(searchTimeout);
    if (!searchQuery.trim() || !selectedProviderId) {
      searchResults = [];
      return;
    }
    searchTimeout = setTimeout(() => doSearch(), 300);
  }

  async function doSearch() {
    if (!searchQuery.trim() || !selectedProviderId) return;
    searching = true;
    error = null;

    try {
      searchResults = await api.itemIntegrationLinks.search(itemId, searchQuery, selectedProviderId) || [];
    } catch (err) {
      console.error('Failed to search:', err);
      error = t('integrations.failedToSearch');
      searchResults = [];
    } finally {
      searching = false;
    }
  }

  async function linkPage(result) {
    linking = result.external_id;
    error = null;

    try {
      await api.itemIntegrationLinks.create(itemId, {
        provider_id: selectedProviderId,
        external_id: result.external_id,
        external_url: result.external_url,
        title: result.title,
        icon: result.icon || '',
        link_type: result.page_type || 'page',
      });
      successToast(t('integrations.linked'));
      oncreated?.();
    } catch (err) {
      console.error('Failed to link page:', err);
      if (err.message?.includes('already linked')) {
        error = 'This page is already linked to this item';
      } else {
        error = err.message || 'Failed to link page';
      }
    } finally {
      linking = null;
    }
  }

  function close() {
    onclose?.();
  }
</script>

<Modal isOpen={true} maxWidth="max-w-md" onclose={close}>
  <ModalHeader title={t('integrations.linkPage')} onClose={close} />

    <!-- Content -->
    <div class="px-6 py-4 space-y-4">
      {#if loading}
        <div class="flex items-center justify-center py-8">
          <Loader2 class="w-6 h-6 animate-spin" style="color: var(--ds-text-subtle);" />
        </div>
      {:else if providers.length === 0}
        <EmptyState
          icon={ExternalLink}
          title={t('integrations.connectAccount')}
          description={t('integrations.connectToLink')}
        />
      {:else}
        <!-- Provider Selection (if multiple) -->
        {#if providers.length > 1}
          <div>
            <Label color="default" required class="mb-1.5">{t('integrations.selectProvider')}</Label>
            <select
              bind:value={selectedProviderId}
              class="w-full h-9 px-3 text-sm border rounded-md"
              style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text);"
              onchange={() => { searchResults = []; searchQuery = ''; }}
            >
              <option value={null}>{t('integrations.selectProvider')}</option>
              {#each providers as provider}
                <option value={provider.id}>{provider.name}</option>
              {/each}
            </select>
          </div>
        {/if}

        <!-- Search -->
        {#if selectedProviderId}
          <div>
            <div class="relative">
              <Search class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4" style="color: var(--ds-text-subtle);" />
              <!-- svelte-ignore a11y_autofocus -->
              <input
                type="text"
                value={searchQuery}
                oninput={onSearchInput}
                placeholder={t('integrations.searchPages')}
                class="w-full pl-10 pr-3 py-2 rounded-lg border text-sm"
                style="border-color: var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
                autofocus
              />
              {#if searching}
                <Loader2 class="absolute right-3 top-1/2 -translate-y-1/2 w-4 h-4 animate-spin" style="color: var(--ds-text-subtle);" />
              {/if}
            </div>
          </div>

          <!-- Results -->
          {#if searchResults.length > 0}
            <div class="max-h-64 overflow-y-auto -mx-2 space-y-0.5">
              {#each searchResults as result}
                <button
                  class="w-full flex items-center gap-3 px-3 py-2.5 rounded-lg text-left transition-colors hover:bg-[var(--ds-background-neutral)]"
                  onclick={() => linkPage(result)}
                  disabled={linking === result.external_id}
                >
                  {#if result.icon}
                    <span class="text-lg flex-shrink-0">{result.icon}</span>
                  {:else}
                    <ExternalLink class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
                  {/if}
                  <div class="flex-1 min-w-0">
                    <span class="text-sm block truncate" style="color: var(--ds-text);">
                      {result.title}
                    </span>
                    <span class="text-xs" style="color: var(--ds-text-subtlest);">
                      {result.page_type === 'database' ? t('integrations.database') : t('integrations.page')}
                    </span>
                  </div>
                  {#if linking === result.external_id}
                    <Loader2 class="w-4 h-4 animate-spin flex-shrink-0" style="color: var(--ds-text-subtle);" />
                  {/if}
                </button>
              {/each}
            </div>
          {:else if searchQuery && !searching}
            <p class="text-sm text-center py-4" style="color: var(--ds-text-subtle);">
              No pages found
            </p>
          {/if}
        {/if}

        {#if error}
          <p class="text-sm" style="color: var(--ds-text-danger);">{error}</p>
        {/if}
      {/if}
    </div>

  <div class="px-6 py-3 border-t flex justify-end" style="border-color: var(--ds-border);">
    <Button variant="ghost" onclick={close}>{t('common.close')}</Button>
  </div>
</Modal>
