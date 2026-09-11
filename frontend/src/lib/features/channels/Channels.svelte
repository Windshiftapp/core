<script>
  import { onMount } from 'svelte';
  import { useEventListener } from 'runed';
  import { t } from '../../stores/i18n.svelte.js';
  import { IconLifebuoy, IconPlus, IconTrash, IconSettings, IconSearch, IconTag, IconPower, IconFileText } from '@tabler/icons-svelte-runes';
  import { api } from '../../api.js';
  import { currentRoute, navigate } from '../../router.js';
  import { channelCategoriesStore } from '../../stores/channelCategories.js';
  import { toHotkeyString, getShortcutDisplay } from '../../utils/keyboardShortcuts.js';
  import Button from '../../components/Button.svelte';
  import Input from '../../components/Input.svelte';
  import Select from '../../components/Select.svelte';
  import ItemPicker from '../../pickers/ItemPicker.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import { confirm } from '../../composables/useConfirm.js';
  import { errorToast, successToast } from '../../stores/toasts.svelte.js';
  import Lozenge from '../../components/Lozenge.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import Spinner from '../../components/Spinner.svelte';
  import ChannelNavigation from './ChannelNavigation.svelte';
  import { channelTypes as channelTypeDefs, allTypesEntry, getChannelTypeIcon } from './channelTypes.js';
  import { channelAdminRoute } from './channelRoutes.js';
  import CategoryModal from '../../dialogs/CategoryModal.svelte';
  import ChannelConfigModal from '../../dialogs/ChannelConfigModal.svelte';
  import EmailLogModal from '../../dialogs/EmailLogModal.svelte';
  import Label from '../../components/Label.svelte';
  import DescriptionText from '../../components/DescriptionText.svelte';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';
  import { isSystemAdmin } from '../../stores/permissions.svelte.js';

  // Props
  let { embedded = false } = $props();

  // Channel type definitions for embedded tab navigation
  const channelTypes = [
    { ...allTypesEntry, label: t('channels.allTypes', 'All') },
    ...channelTypeDefs.map(ct => ({ ...ct, label: t(`channels.${ct.id}`, ct.id) }))
  ];

  let channels = $state([]);
  let loading = $state(true);
  let error = $state(null);
  let channelSearch = $state('');

  // Filters are URL-backed in both layouts so deep links and picker/tab
  // changes cannot disagree about which channels are visible.
  let activeCategoryId = $derived($currentRoute.params?.categoryId || null);
  let activeTypeFilter = $derived($currentRoute.params?.type || null);

  // Handlers for embedded tab navigation
  function handleTypeClick(typeId) {
    if (typeId === null) {
      navigate('/admin/channels');
    } else {
      navigate(`/admin/channels/type/${typeId}`);
    }
  }

  async function deleteCategory(id) {
    await channelCategoriesStore.delete(id);
    if (Number(activeCategoryId) === Number(id)) {
      navigate('/admin/channels');
    }
    // The category FK is SET NULL server-side; refresh channel rows so an
    // edit modal cannot retain the deleted ID in its local form state.
    await loadChannels();
  }

  // Filtered channels based on type, category, and search
  let filteredChannels = $derived(() => {
    let result = channels;

    // Filter by type
    if (activeTypeFilter !== null) {
      result = result.filter(c => c.type === activeTypeFilter);
    }

    // Filter by category
    if (activeCategoryId !== null) {
      result = result.filter(c => c.category_id === parseInt(activeCategoryId));
    }

    // Filter by search
    if (channelSearch.trim()) {
      result = result.filter(c => c.name.toLowerCase().includes(channelSearch.toLowerCase()));
    }

    return result;
  });

  // Modal states
  let showAddForm = $state(false);
  let showCategoryModal = $state(false);
  let showConfigModal = $state(false);
  let selectedChannel = $state(null);
  let showEmailLog = $state(false);
  let emailLogChannel = $state(null);
  let creating = $state(false);


  // Form data for new channel
  let channelFormData = $state({
    name: '',
    type: 'portal',
    description: '',
    category_id: null,
    slug: ''
  });


  // DataTable columns
  const channelColumns = [
    { key: 'name', label: 'Name', slot: 'name' },
    { key: 'type', label: 'Type', width: 'w-32', slot: 'type' },
    { key: 'direction', label: 'Direction', width: 'w-32' },
    { key: 'status', label: 'Status', width: 'w-32', slot: 'status' },
    { key: 'actions', label: '', width: 'w-16' }
  ];

  async function toggleChannelEnabled(channel) {
    try {
      await api.channels.toggle(channel.id);
      await loadChannels();
      successToast(`Channel ${channel.status === 'enabled' ? 'disabled' : 'enabled'} successfully`);
    } catch (err) {
      console.error('Failed to toggle channel:', err);
      errorToast('Failed to toggle channel: ' + (err.message || err));
    }
  }

  function openEmailLog(channel) {
    emailLogChannel = channel;
    showEmailLog = true;
  }

  function getChannelActionItems(channel) {
    const items = [
      { title: 'Configure', icon: IconSettings, onClick: () => {
        const route = channelAdminRoute(channel);
        if (route) {
          navigate(route);
        } else {
          openConfigModal(channel);
        }
      }}
    ];

    if (channel.type === 'email') {
      items.push({
        title: t('channel.processingLog', 'Processing Log'),
        icon: IconFileText,
        onClick: () => openEmailLog(channel)
      });
    }

    if (!isPluginOwned(channel)) {
      items.push({
        title: channel.status === 'enabled' ? 'Disable' : 'Enable',
        icon: IconPower,
        onClick: () => toggleChannelEnabled(channel)
      });
    }

    if ($isSystemAdmin && !channel.is_default && !isPluginOwned(channel)) {
      items.push({
        title: 'Delete',
        icon: IconTrash,
        onClick: () => deleteChannel(channel),
        color: 'var(--ds-text-danger)',
        testid: 'channel-delete',
      });
    }

    return items;
  }

  onMount(async () => {
    await loadChannels();
    await channelCategoriesStore.init();

    // Handle OAuth callback parameters (after channels are loaded)
    handleOAuthCallback();
    openChannelFromRoute();
  });

  useEventListener(() => document, 'manage-channel-categories', handleManageCategories);

  function handleOAuthCallback() {
    const urlParams = new URLSearchParams(window.location.search);
    const oauthSuccess = urlParams.get('oauth_success');
    const oauthError = urlParams.get('oauth_error');
    const channelIdFromPath = $currentRoute.params?.id;

    if (oauthSuccess === 'true' && channelIdFromPath) {
      // OAuth was successful - open the channel config modal and show success
      const channelId = parseInt(channelIdFromPath);
      const channel = channels.find(c => c.id === channelId);
      if (channel) {
        selectedChannel = channel;
        showConfigModal = true;
        successToast('Email OAuth connected successfully!');
      }
      // Clear URL params
      window.history.replaceState({}, '', window.location.pathname);
    } else if (oauthError) {
      // OAuth failed - show error
      const errorMessages = {
        'exchange_failed': 'Failed to exchange OAuth code for tokens',
        'save_failed': 'Failed to save OAuth tokens',
        'channel_not_found': 'Channel not found',
        'invalid_config': 'Invalid channel configuration',
        'unsupported_provider': 'Unsupported OAuth provider',
        'decrypt_failed': 'Could not decrypt the OAuth client secret',
        'identity_failed': 'Could not read the connected mailbox identity',
        'authorization_failed': 'You no longer have permission to connect this channel',
        'config_changed': 'The channel changed while OAuth was open. Review the latest settings and reconnect.'
      };
      errorToast(errorMessages[oauthError] || `OAuth error: ${oauthError}`);
      // Clear URL params
      window.history.replaceState({}, '', window.location.pathname);
    }
  }

  function openChannelFromRoute() {
    const match = window.location.pathname.match(/^\/admin\/channels\/(\d+)$/);
    if (!match || showConfigModal) return;
    const channel = channels.find(candidate => candidate.id === Number(match[1]));
    if (channel) {
      const route = channelAdminRoute(channel);
      if (route) {
        navigate(route);
        return;
      }
      selectedChannel = channel;
      showConfigModal = true;
    }
  }

  function handleManageCategories() {
    showCategoryModal = true;
  }

  async function loadChannels() {
    try {
      loading = true;
      error = null;
      channels = await api.channels.getAll({ include_disabled: true });
    } catch (err) {
      console.error('Failed to load channels:', err);
      error = 'Failed to load channels';
      channels = [];
    } finally {
      loading = false;
    }
  }

  function isPluginOwned(channel) {
    return channel?.plugin_name != null;
  }

  function getChannelStatus(channel) {
    return channel.status || 'disabled';
  }

  function getChannelStatusLabel(channel) {
    const status = channel.status || 'disabled';
    return t(`channels.status.${status}`, status);
  }

  function getChannelStatusColor(status) {
    const colors = {
      'enabled': 'green',
      'disabled': 'gray',
      'active': 'green',
      'configured': 'green',
      'pending': 'gray',
      'inactive': 'gray'
    };
    return colors[status] || 'gray';
  }

  function showAddChannelForm() {
    const filteredType = channelTypeDefs.some(type => type.id === activeTypeFilter)
      ? activeTypeFilter
      : 'portal';
    channelFormData = {
      name: '',
      type: filteredType,
      description: '',
      category_id: activeCategoryId ? parseInt(activeCategoryId) : null,
      slug: ''
    };
    showAddForm = true;
  }

  function cancelChannelForm() {
    showAddForm = false;
    channelFormData = {
      name: '',
      type: 'portal',
      description: '',
      category_id: null,
      slug: ''
    };
  }

  async function handleChannelSubmit() {
    if (creating) return;
    try {
      creating = true;
      // Auto-determine direction based on type
      const directionMap = {
        'portal': 'inbound',
        'form': 'inbound',
        'webhook': 'outbound',
        'email': 'inbound',
        'smtp': 'outbound'
      };
      const direction = directionMap[channelFormData.type] || 'outbound';

      const channelData = {
        ...channelFormData,
        direction,
        category_id: channelFormData.category_id || null
      };

      const newChannel = await api.channels.create(channelData);

      await loadChannels();
      cancelChannelForm();

      const route = channelAdminRoute(newChannel);
      if (route) {
        navigate(route);
      } else {
        selectedChannel = channels.find(channel => channel.id === newChannel.id) || newChannel;
        showConfigModal = true;
      }
    } catch (error) {
      console.error('Failed to save channel:', error);
      errorToast('Failed to save channel: ' + (error.message || error));
    } finally {
      creating = false;
    }
  }

  function openConfigModal(channel) {
    selectedChannel = channel;
    showConfigModal = true;
  }

  function closeConfigModal() {
    showConfigModal = false;
    selectedChannel = null;
  }

  async function handleConfigSave() {
    const selectedID = selectedChannel?.id;
    await loadChannels();
    if (selectedID) {
      selectedChannel = channels.find(channel => channel.id === selectedID) || selectedChannel;
    }
  }

  async function deleteChannel(channel) {
    if (channel.is_default) {
      errorToast('Cannot delete the default notification channel');
      return;
    }
    if (isPluginOwned(channel)) {
      errorToast('Cannot delete a plugin-owned channel');
      return;
    }
    let impact;
    try {
      impact = await api.channels.getDeleteImpact(channel.id);
    } catch (error) {
      console.error('Failed to load channel delete impact:', error);
      errorToast('Could not verify what this deletion would remove. Please try again.');
      return;
    }
    const impactLabels = [
      ['request types', impact.request_types],
      ['asset reports', impact.asset_reports],
      ['customer access grants', impact.portal_customer_channels],
      ['email log entries', impact.email_message_tracking],
      ['queued email replies', impact.email_reply_outbox],
      ['pending email authorizations', impact.email_oauth_states],
      ['email credential leases', impact.email_credential_leases],
      ['active mailbox-processing leases', impact.email_processing_leases],
      ['mailbox sync states', impact.email_channel_state],
      ['webhook delivery records', impact.webhook_deliveries],
      ['manager assignments', impact.channel_managers],
      ['in-progress portal drafts', impact.portal_request_drafts],
      ['portal sign-in links', impact.portal_magic_links],
      ['portal sessions', impact.portal_sessions],
      ['linked items', impact.items]
    ].filter(([, count]) => count > 0).map(([label, count]) => `${count} ${label}`);
    const impactMessage = impactLabels.length > 0
      ? ` This will affect ${impactLabels.join(', ')}.`
      : '';
    const ok = await confirm({
      title: 'Delete Channel',
      message: `Are you sure you want to delete this channel?${impactMessage} This action cannot be undone.`,
      confirmText: 'Delete Channel',
      variant: 'danger',
    });
    if (!ok) return;
    try {
      await api.channels.delete(channel.id);
      await loadChannels();

      if (selectedChannel?.id === channel.id) {
        closeConfigModal();
      }

      successToast('Channel deleted successfully');
    } catch (error) {
      console.error('Failed to delete channel:', error);
      errorToast('Failed to delete channel: ' + (error.message || error));
    }
  }

  function handleRowClick(channel) {
    const route = channelAdminRoute(channel);
    if (route) {
      navigate(route);
    } else {
      openConfigModal(channel);
    }
  }
</script>

<!-- Main container with sidebar layout -->
<div class="flex h-full min-h-0 overflow-hidden" style="background-color: var(--ds-surface);">
  <!-- Left Sidebar - Category Navigation (only when not embedded in Admin) -->
  {#if !embedded}
    <ChannelNavigation />
  {/if}

  <!-- Main Content -->
  <div class="flex-1 min-h-0 overflow-y-auto {embedded ? '' : 'p-6'}">
    <!-- Embedded Tab Navigation -->
    {#if embedded}
      <div class="border-b mb-6" style="border-color: var(--ds-border);">
        <!-- Type Tabs with Category filter and Manage button on right -->
        <div class="flex items-center justify-between">
          <div class="flex gap-1">
            {#each channelTypes as type (type.id)}
              {@const isActive = activeTypeFilter === type.id}
              <button
                onclick={() => handleTypeClick(type.id)}
                class="px-4 py-2.5 text-sm font-medium border-b-2 transition-colors flex items-center gap-2"
                style={isActive
                  ? 'border-color: var(--ds-border-focused); color: var(--ds-text);'
                  : 'border-color: transparent; color: var(--ds-text-subtle);'}
                onmouseenter={(e) => { if (!isActive) e.currentTarget.style.color = 'var(--ds-text)'; }}
                onmouseleave={(e) => { if (!isActive) e.currentTarget.style.color = 'var(--ds-text-subtle)'; }}
              >
                <type.icon class="w-4 h-4" />
                {type.label}
              </button>
            {/each}
          </div>
          <div class="flex items-center gap-3 flex-shrink-0">
            <span class="text-sm whitespace-nowrap" style="color: var(--ds-text-subtle);">{t('channels.category', 'Category')}:</span>
            <ItemPicker
              value={activeCategoryId ? parseInt(activeCategoryId) : null}
              items={$channelCategoriesStore}
              placeholder={t('channels.allCategories', 'All Categories')}
              showUnassigned={true}
              unassignedLabel={t('channels.allCategories', 'All Categories')}
              allowClear={false}
              class="w-48"
              onSelect={(item) => {
                if (!item) {
                  navigate('/admin/channels');
                } else {
                  navigate(`/admin/channels/category/${item.id}`);
                }
              }}
            />
            {#if $isSystemAdmin}
              <Button
                onclick={() => showCategoryModal = true}
                variant="ghost"
                size="small"
                icon={IconTag}
                class="whitespace-nowrap flex-shrink-0"
              >
                {t('channels.manageCategories', 'Manage')}
              </Button>
            {/if}
          </div>
        </div>
      </div>
    {/if}

    <!-- Header -->
    <div class="flex items-center justify-between mb-6">
      <div>
        {#if !embedded}
          <h1 class="text-2xl font-semibold" style="color: var(--ds-text);">
            {#if activeTypeFilter}
              {activeTypeFilter === 'portal' ? 'Portal' : activeTypeFilter === 'webhook' ? 'Webhook' : activeTypeFilter} {t('channels.title')}
            {:else if activeCategoryId}
              {@const category = $channelCategoriesStore.find(c => c.id === parseInt(activeCategoryId))}
              {category?.name || t('common.category')}
            {:else}
              {t('channels.title')}
            {/if}
          </h1>
          <p class="mt-1 text-sm" style="color: var(--ds-text-subtle);">
            {filteredChannels().length} channel{filteredChannels().length !== 1 ? 's' : ''}
          </p>
        {:else}
          <!-- Search Bar (embedded) -->
          <div class="relative w-64">
            <IconSearch class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4" style="color: var(--ds-text-subtle);" />
            <Input
              type="text"
              bind:value={channelSearch}
              placeholder={t('channels.searchChannels')}
              class="pl-9"
              size="small"
            />
          </div>
        {/if}
      </div>
      {#if $isSystemAdmin}
        <Button
          onclick={showAddChannelForm}
          variant="primary"
          icon={IconPlus}
          size="medium"
          dataTestid="channel-create-open"
          keyboardHint={getShortcutDisplay('channels', 'addChannel')}
          hotkeyConfig={{ key: toHotkeyString('channels', 'addChannel'), guard: () => !showAddForm && !showConfigModal && !showCategoryModal }}
        >
          {t('channels.createChannel')}
        </Button>
      {/if}
    </div>

    <!-- Search Bar (non-embedded) -->
    {#if !embedded}
      <div class="mb-6">
        <div class="relative max-w-md">
          <IconSearch class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4" style="color: var(--ds-text-subtle);" />
          <Input
            type="text"
            bind:value={channelSearch}
            placeholder={t('channels.searchChannels')}
            class="pl-9"
            size="small"
          />
        </div>
      </div>
    {/if}

    <!-- Data Table -->
    {#if loading}
      <div class="flex items-center justify-center py-16">
        <Spinner />
      </div>
    {:else if error}
      <div class="text-center py-16">
        <div class="text-ds-text-danger text-sm font-medium mb-2">{error}</div>
        <Button onclick={loadChannels} variant="default" size="small">
          {t('common.retry')}
        </Button>
      </div>
    {:else}
      <DataTable
        columns={channelColumns}
        data={filteredChannels()}
        keyField="id"
        emptyMessage={t('channels.noChannels')}
        emptyDescription={channelSearch ? t('channels.noChannels') : t('channels.noChannels')}
        emptyIcon={IconLifebuoy}
        actionItems={getChannelActionItems}
        actionTriggerTestid={(channel) => `channel-actions-${channel.id}`}
        onRowClick={handleRowClick}
        rowAttrs={(channel) => ({ 'data-testid': `admin-channel-row-${channel.id}` })}
      >
        {#snippet name(item)}
          {@const ChannelIcon = getChannelTypeIcon(item.type)}
          <div class="flex items-center gap-3">
            <ChannelIcon class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
            <div>
              <div class="font-medium" style="color: var(--ds-text);">{item.name}</div>
              {#if item.description}
                <div class="text-xs" style="color: var(--ds-text-subtle);">{item.description}</div>
              {/if}
            </div>
          </div>
        {/snippet}

        {#snippet type(item)}
          <span class="capitalize" style="color: var(--ds-text);">{item.type}</span>
        {/snippet}

        {#snippet status(item)}
          <div class="flex items-center gap-2">
            <Lozenge
              color={getChannelStatusColor(getChannelStatus(item))}
              text={getChannelStatusLabel(item)}
            />
            {#if item.is_default}
              <Lozenge color="blue" text="System" />
            {/if}
            {#if isPluginOwned(item)}
              <Lozenge color="purple" text="Plugin" />
            {/if}
          </div>
        {/snippet}
      </DataTable>
    {/if}
  </div>
</div>

<!-- Add Channel Modal -->
<Modal
  isOpen={showAddForm}
  onclose={cancelChannelForm}
  onSubmit={handleChannelSubmit}
  submitDisabled={!channelFormData.name.trim() ||
    ((channelFormData.type === 'portal' || channelFormData.type === 'form') && !channelFormData.slug.trim()) || creating}
  maxWidth="max-w-xl"
  autoFocus={true}
>
  {#snippet children(submitHint)}
  <!-- Header -->
  <ModalHeader title={t('channels.createChannel')} showCloseButton={false} />

  <!-- Content -->
  <div class="p-6">
    <div class="space-y-4">
      <div>
        <Label for="channelName" required color="default" class="mb-2">Channel Name</Label>
        <Input
          id="channelName"
          bind:value={channelFormData.name}
          required
          placeholder="e.g., Customer Support Portal"
        />
      </div>

      <div>
        <Label color="default" class="mb-2">Type</Label>
        <div class="flex flex-wrap gap-2">
          {#each channelTypeDefs as option (option.id)}
            <button
              type="button"
              onclick={() => channelFormData.type = option.id}
              data-testid={`channel-type-${option.id}`}
              aria-pressed={channelFormData.type === option.id}
              class="flex items-center gap-2 px-3 py-2 rounded-lg border transition-all"
              style={channelFormData.type === option.id
                ? 'border-color: var(--ds-border-focused); background: var(--ds-surface-selected);'
                : 'border-color: var(--ds-border); background: var(--ds-surface);'}
            >
              <option.icon class="w-4 h-4 flex-shrink-0" style="color: {option.formColor};" />
              <span class="text-sm font-medium" style="color: var(--ds-text);">{t(`channels.${option.id}`, option.id)}</span>
            </button>
          {/each}
        </div>
      </div>

      {#if channelFormData.type === 'portal' || channelFormData.type === 'form'}
        <div>
          <Label for="channelSlug" required color="default" class="mb-2">
            {channelFormData.type === 'form' ? 'Public URL' : 'Slug'}
          </Label>
          <Input
            id="channelSlug"
            bind:value={channelFormData.slug}
            required
            minlength={3}
            maxlength={64}
            pattern={'[a-z0-9](?:[a-z0-9-]{1,62}[a-z0-9])'}
            placeholder="e.g., support"
          />
          <DescriptionText>
            {#if channelFormData.type === 'form'}
              Forms will be shared at /forms/{channelFormData.slug || 'your-url'}.
            {:else}
              /portal/{channelFormData.slug || '...'}
            {/if}
          </DescriptionText>
        </div>
      {/if}

      {#if channelFormData.type !== 'form'}
        <div>
          <Label for="channelCategory" color="default" class="mb-2">Category</Label>
          <Select id="channelCategory" bind:value={channelFormData.category_id} options={[{ value: null, label: 'No Category' }, ...$channelCategoriesStore.map(c => ({ value: c.id, label: c.name }))]} />
        </div>

        <div>
          <Label for="channelDescription" color="default" class="mb-2">Description</Label>
          <Textarea
            id="channelDescription"
            bind:value={channelFormData.description}
            rows={3}
            placeholder="Brief description of this channel's purpose"
          />
        </div>
      {/if}
    </div>
  </div>

  <!-- Actions -->
  <DialogFooter
    onCancel={cancelChannelForm}
    onConfirm={handleChannelSubmit}
    confirmLabel={t('channels.createChannel')}
    confirmTestid="channel-create-confirm"
    disabled={!channelFormData.name.trim() ||
      ((channelFormData.type === 'portal' || channelFormData.type === 'form') && !channelFormData.slug.trim()) || creating}
    showKeyboardHint={true}
    confirmKeyboardHint={submitHint}
  />
  {/snippet}
</Modal>

<!-- Channel Category Modal -->
<CategoryModal
  isOpen={showCategoryModal}
  onClose={() => showCategoryModal = false}
  title="Manage Channel Categories"
  categories={$channelCategoriesStore}
  onAdd={async (data) => await channelCategoriesStore.add(data)}
  onDelete={deleteCategory}
  showColorPicker={true}
/>

<!-- Channel Configuration Modal -->
<ChannelConfigModal
  isOpen={showConfigModal}
  channel={selectedChannel}
  onClose={closeConfigModal}
  onSave={handleConfigSave}
/>

<!-- Email Processing Log Modal -->
<EmailLogModal
  isOpen={showEmailLog}
  channel={emailLogChannel}
  onClose={() => { showEmailLog = false; emailLogChannel = null; }}
/>
