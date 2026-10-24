<script>
  import { onMount } from 'svelte';
  import StateDisplay from '../../components/StateDisplay.svelte';
  import IconLifebuoy from '@tabler/icons-svelte-runes/icons/lifebuoy';
  import { channels as channelsApi } from '../../api/channels.js';
  import Button from '../../components/Button.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import SearchInput from '../../components/SearchInput.svelte';
  import PageHeader from '../../layout/PageHeader.svelte';
  import { navigate } from '../../router.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { workspacesStore } from '../../stores/workspaces.svelte.js';
  import ChannelConfigModal from '../../dialogs/ChannelConfigModal.svelte';
  import { getChannelTypeIcon } from './channelTypes.js';
  import {
    managerChannelPurpose,
    managerChannelStatusColor,
  } from './managerChannelPresentation.js';

  let channels = $state([]);
  let loading = $state(true);
  let error = $state('');
  let search = $state('');
  let selectedChannel = $state(null);
  let showConfigModal = $state(false);

  const columns = [
    { key: 'name', label: t('channel.channelName'), slot: 'name', sortable: true },
    { key: 'purpose', label: t('channels.manager.purpose'), slot: 'purpose' },
    { key: 'status', label: t('common.status'), slot: 'status', width: 'w-32' },
  ];

  let filteredChannels = $derived.by(() => {
    const query = search.trim().toLowerCase();
    if (!query) return channels;
    return channels.filter(
      (channel) =>
        channel.name?.toLowerCase().includes(query) ||
        channel.description?.toLowerCase().includes(query)
    );
  });

  onMount(loadChannels);

  async function loadChannels() {
    loading = true;
    error = '';
    try {
      channels = (await channelsApi.getAll({ include_disabled: true })) || [];
    } catch (err) {
      console.error('Failed to load managed channels:', err);
      channels = [];
      error = t('channels.manager.loadFailed');
    } finally {
      loading = false;
    }
  }

  function workspaceName(channel) {
    let config = {};
    try {
      config =
        typeof channel.config === 'string' ? JSON.parse(channel.config || '{}') : channel.config || {};
    } catch {
      return '';
    }
    const workspaceID = Number(config.email_workspace_id);
    return $workspacesStore.allWorkspaces.find((workspace) => workspace.id === workspaceID)?.name || '';
  }

  function channelPurpose(channel) {
    const presentation = managerChannelPurpose(channel, workspaceName(channel));
    return t(presentation.key, presentation.params);
  }

  function openChannel(channel) {
    if (channel.type === 'form') {
      navigate(`/admin/channels/${channel.id}/forms`);
    } else if (channel.type === 'portal') {
      navigate(`/admin/channels/${channel.id}/portal`);
    } else {
      selectedChannel = channel;
      showConfigModal = true;
    }
  }

  async function handleConfigSave() {
    const selectedID = selectedChannel?.id;
    await loadChannels();
    selectedChannel = channels.find((channel) => channel.id === selectedID) || null;
  }
</script>

<div class="space-y-6">
  <PageHeader
    icon={IconLifebuoy}
    title={t('channels.title')}
    subtitle={t('channels.manager.subtitle')}
  />

  <SearchInput
    bind:value={search}
    placeholder={t('channels.searchChannels')}
    dataTestid="manager-channels-search"
    class="max-w-md"
  />

  {#if error}
    <div class="rounded-lg border px-6 py-10 text-center" style="border-color: var(--ds-border);">
      <p class="mb-4 text-sm" style="color: var(--ds-text-danger);">{error}</p>
      <Button onclick={loadChannels} variant="default" size="small" dataTestid="manager-channels-retry">
        {t('common.retry')}
      </Button>
    </div>
  {:else if loading}
    <StateDisplay type="loading" dataTestid="manager-channels-loading" />
  {:else}
    <DataTable
      {columns}
      data={filteredChannels}
      keyField="id"
      emptyMessage={t('channels.manager.noChannels')}
      emptyDescription={t('channels.manager.noChannelsDescription')}
      emptyIcon={IconLifebuoy}
      onRowClick={openChannel}
      rowAttrs={(channel) => ({ 'data-testid': `manager-channel-row-${channel.id}` })}
    >
      {#snippet name(item)}
        {@const ChannelIcon = getChannelTypeIcon(item.type)}
        <div class="flex items-center gap-3">
          <ChannelIcon class="h-4 w-4 flex-shrink-0" style="color: var(--ds-icon-subtle);" />
          <div class="min-w-0">
            <div class="truncate font-medium" style="color: var(--ds-text);">{item.name}</div>
            {#if item.description}
              <div class="truncate text-xs" style="color: var(--ds-text-subtle);">
                {item.description}
              </div>
            {/if}
          </div>
        </div>
      {/snippet}

      {#snippet purpose(item)}
        <span class="text-sm" style="color: var(--ds-text-subtle);">{channelPurpose(item)}</span>
      {/snippet}

      {#snippet status(item)}
        <Lozenge
          color={managerChannelStatusColor(item.status)}
          text={t(`channels.status.${item.status || 'disabled'}`)}
        />
      {/snippet}
    </DataTable>
  {/if}
</div>

<ChannelConfigModal
  isOpen={showConfigModal}
  channel={selectedChannel}
  onClose={() => {
    showConfigModal = false;
    selectedChannel = null;
  }}
  onSave={handleConfigSave}
/>
