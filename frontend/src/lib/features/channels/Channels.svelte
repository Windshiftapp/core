<script>
  import { onMount, onDestroy } from 'svelte';
  import { LifeBuoy, Plus, Webhook, Globe, Trash2, Settings, Search, Mail } from 'lucide-svelte';
  import { api } from '../../api.js';
  import { currentRoute, navigate } from '../../router.js';
  import { channelCategoriesStore } from '../../stores/channelCategories.js';
  import { createShortcutHandler, getShortcutDisplay } from '../../utils/keyboardShortcuts.js';
  import Button from '../../components/Button.svelte';
  import Input from '../../components/Input.svelte';
  import Select from '../../components/Select.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import ConfirmDialog from '../../dialogs/ConfirmDialog.svelte';
  import { errorToast, successToast } from '../../stores/toasts.svelte.js';
  import Lozenge from '../../components/Lozenge.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import Spinner from '../../components/Spinner.svelte';
  import ChannelNavigation from './ChannelNavigation.svelte';
  import CategoryModal from '../../dialogs/CategoryModal.svelte';
  import ChannelConfigModal from '../../dialogs/ChannelConfigModal.svelte';
  import Label from '../../components/Label.svelte';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';

  let channels = $state([]);
  let loading = $state(true);
  let error = $state(null);
  let channelSearch = $state('');

  // Filters from URL
  let activeCategoryId = $derived($currentRoute.params?.categoryId || null);
  let activeTypeFilter = $derived($currentRoute.params?.type || null);

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
  let showDeleteConfirmation = $state(false);
  let channelToDelete = $state(null);


  // Form data for new channel
  let channelFormData = $state({
    name: '',
    type: 'portal',
    description: '',
    category_id: null
  });


  // DataTable columns
  const channelColumns = [
    { key: 'name', label: 'Name', slot: 'name' },
    { key: 'type', label: 'Type', width: 'w-32', slot: 'type' },
    { key: 'direction', label: 'Direction', width: 'w-32' },
    { key: 'status', label: 'Status', width: 'w-32', slot: 'status' },
    { key: 'actions', label: '', width: 'w-16' }
  ];

  function getChannelActionItems(channel) {
    const items = [
      { title: 'Configure', icon: Settings, onClick: () => openConfigModal(channel) }
    ];

    if (!channel.is_default && !isPluginOwned(channel)) {
      items.push({ title: 'Delete', icon: Trash2, onClick: () => deleteChannel(channel), color: 'var(--ds-text-danger)' });
    }

    return items;
  }

  // Keyboard shortcut handler
  const shortcutHandler = createShortcutHandler({
    addChannel: () => {
      if (!showAddForm && !showConfigModal && !showCategoryModal) {
        showAddChannelForm();
      }
    }
  }, 'channels');

  onMount(async () => {
    await loadChannels();
    await channelCategoriesStore.init();

    // Listen for manage-channel-categories event
    document.addEventListener('manage-channel-categories', handleManageCategories);
    // Listen for keyboard shortcuts
    document.addEventListener('keydown', shortcutHandler);

    // Handle OAuth callback parameters (after channels are loaded)
    handleOAuthCallback();
  });

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
        'unsupported_provider': 'Unsupported OAuth provider'
      };
      errorToast(errorMessages[oauthError] || `OAuth error: ${oauthError}`);
      // Clear URL params
      window.history.replaceState({}, '', window.location.pathname);
    }
  }

  onDestroy(() => {
    document.removeEventListener('manage-channel-categories', handleManageCategories);
    document.removeEventListener('keydown', shortcutHandler);
  });

  function handleManageCategories() {
    showCategoryModal = true;
  }

  async function loadChannels() {
    try {
      loading = true;
      error = null;
      channels = await api.channels.getAll();
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

  function getChannelTypeIcon(type) {
    const icons = {
      'webhook': Webhook,
      'portal': Globe
    };
    return icons[type] || LifeBuoy;
  }

  function showAddChannelForm() {
    channelFormData = {
      name: '',
      type: 'portal',
      description: '',
      category_id: activeCategoryId ? parseInt(activeCategoryId) : null
    };
    showAddForm = true;
  }

  function cancelChannelForm() {
    showAddForm = false;
    channelFormData = {
      name: '',
      type: 'portal',
      description: '',
      category_id: null
    };
  }

  async function handleChannelSubmit() {
    try {
      // Auto-determine direction based on type
      const directionMap = {
        'portal': 'inbound',
        'webhook': 'outbound',
        'email': 'inbound'
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

      // Open config modal for the new channel
      selectedChannel = newChannel;
      showConfigModal = true;
    } catch (error) {
      console.error('Failed to save channel:', error);
      errorToast('Failed to save channel: ' + (error.message || error));
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

  function handleConfigSave() {
    loadChannels();
  }

  function deleteChannel(channel) {
    if (channel.is_default) {
      errorToast('Cannot delete the default notification channel');
      return;
    }
    if (isPluginOwned(channel)) {
      errorToast('Cannot delete a plugin-owned channel');
      return;
    }
    channelToDelete = channel;
    showDeleteConfirmation = true;
  }

  async function confirmDeleteChannel() {
    try {
      await api.channels.delete(channelToDelete.id);
      await loadChannels();

      // Close config modal if this channel was being configured
      if (selectedChannel?.id === channelToDelete.id) {
        closeConfigModal();
      }

      successToast('Channel deleted successfully');
    } catch (error) {
      console.error('Failed to delete channel:', error);
      errorToast('Failed to delete channel: ' + (error.message || error));
    } finally {
      channelToDelete = null;
      showDeleteConfirmation = false;
    }
  }

  function handleRowClick(channel) {
    openConfigModal(channel);
  }
</script>

<!-- Main container with sidebar layout -->
<div class="flex min-h-screen" style="background-color: var(--ds-surface);">
  <!-- Left Sidebar - Category Navigation -->
  <ChannelNavigation />

  <!-- Main Content -->
  <div class="flex-1 p-6">
    <!-- Header -->
    <div class="flex items-center justify-between mb-6">
      <div>
        <h1 class="text-2xl font-semibold" style="color: var(--ds-text);">
          {#if activeTypeFilter}
            {activeTypeFilter === 'portal' ? 'Portal' : activeTypeFilter === 'webhook' ? 'Webhook' : activeTypeFilter} Channels
          {:else if activeCategoryId}
            {@const category = $channelCategoriesStore.find(c => c.id === parseInt(activeCategoryId))}
            {category?.name || 'Category'}
          {:else}
            All Channels
          {/if}
        </h1>
        <p class="mt-1 text-sm" style="color: var(--ds-text-subtle);">
          {filteredChannels().length} channel{filteredChannels().length !== 1 ? 's' : ''}
        </p>
      </div>
      <Button
        onclick={showAddChannelForm}
        variant="primary"
        icon={Plus}
        size="medium"
        keyboardHint={getShortcutDisplay('channels', 'addChannel')}
      >
        Add Channel
      </Button>
    </div>

    <!-- Search Bar -->
    <div class="mb-6">
      <div class="relative max-w-md">
        <Search class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4" style="color: var(--ds-text-subtle);" />
        <input
          type="text"
          bind:value={channelSearch}
          placeholder="Search channels..."
          class="w-full pl-9 pr-3 py-2 text-sm rounded-lg border"
          style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
        />
      </div>
    </div>

    <!-- Data Table -->
    {#if loading}
      <div class="flex items-center justify-center py-16">
        <Spinner />
      </div>
    {:else if error}
      <div class="text-center py-16">
        <div class="text-red-600 text-sm font-medium mb-2">{error}</div>
        <Button onclick={loadChannels} variant="default" size="small">
          Retry
        </Button>
      </div>
    {:else}
      <DataTable
        columns={channelColumns}
        data={filteredChannels()}
        keyField="id"
        emptyMessage="No channels found"
        emptyDescription={channelSearch ? `No channels match "${channelSearch}"` : 'Create a channel to get started'}
        emptyIcon={LifeBuoy}
        actionItems={getChannelActionItems}
        onRowClick={handleRowClick}
      >
        {#snippet name({ item })}
          <div class="flex items-center gap-3">
            <svelte:component this={getChannelTypeIcon(item.type)} class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
            <div>
              <div class="font-medium" style="color: var(--ds-text);">{item.name}</div>
              {#if item.description}
                <div class="text-xs" style="color: var(--ds-text-subtle);">{item.description}</div>
              {/if}
            </div>
          </div>
        {/snippet}

        {#snippet type({ item })}
          <span class="capitalize" style="color: var(--ds-text);">{item.type}</span>
        {/snippet}

        {#snippet status({ item })}
          <div class="flex items-center gap-2">
            <Lozenge
              color={getChannelStatusColor(getChannelStatus(item))}
              text={getChannelStatus(item)}
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
  on:close={cancelChannelForm}
  onSubmit={handleChannelSubmit}
  submitDisabled={!channelFormData.name.trim()}
  maxWidth="max-w-lg"
  autoFocus={true}
  let:submitHint
>
  <!-- Header -->
  <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
    <h3 class="text-lg font-semibold" style="color: var(--ds-text);">
      Add New Channel
    </h3>
  </div>

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
        <div class="space-y-2">
          {#each [
            { id: 'portal', label: 'Portal', icon: Globe, color: 'var(--ds-icon-accent-green)' },
            { id: 'webhook', label: 'Webhook', icon: Webhook, color: 'var(--ds-icon-accent-purple)' },
            { id: 'email', label: 'Email', icon: Mail, color: 'var(--ds-icon-accent-blue)' }
          ] as option}
            <button
              type="button"
              onclick={() => channelFormData.type = option.id}
              class="w-full flex items-center gap-3 p-3 rounded-lg border transition-all text-left"
              style={channelFormData.type === option.id
                ? 'border-color: var(--ds-border-focused); background: var(--ds-surface-selected);'
                : 'border-color: var(--ds-border); background: var(--ds-surface);'}
            >
              <svelte:component this={option.icon} class="w-5 h-5 flex-shrink-0" style="color: {option.color};" />
              <span class="font-medium" style="color: var(--ds-text);">{option.label}</span>
            </button>
          {/each}
        </div>
      </div>

      <div>
        <Label for="channelCategory" color="default" class="mb-2">Category</Label>
        <Select id="channelCategory" bind:value={channelFormData.category_id}>
          <option value={null}>No Category</option>
          {#each $channelCategoriesStore as category}
            <option value={category.id}>{category.name}</option>
          {/each}
        </Select>
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
    </div>
  </div>

  <!-- Actions -->
  <DialogFooter
    onCancel={cancelChannelForm}
    onConfirm={handleChannelSubmit}
    confirmLabel="Create Channel"
    disabled={!channelFormData.name.trim()}
    showKeyboardHint={true}
    confirmKeyboardHint={submitHint}
  />
</Modal>

<!-- Channel Category Modal -->
<CategoryModal
  isOpen={showCategoryModal}
  onClose={() => showCategoryModal = false}
  title="Manage Channel Categories"
  categories={$channelCategoriesStore}
  onAdd={async (data) => await channelCategoriesStore.add(data)}
  onDelete={async (id) => await channelCategoriesStore.delete(id)}
  showColorPicker={true}
/>

<!-- Channel Configuration Modal -->
<ChannelConfigModal
  isOpen={showConfigModal}
  channel={selectedChannel}
  onClose={closeConfigModal}
  onSave={handleConfigSave}
/>

<!-- Delete Confirmation Dialog -->
<ConfirmDialog
  bind:show={showDeleteConfirmation}
  title="Delete Channel"
  message="Are you sure you want to delete this channel? This action cannot be undone."
  confirmText="Delete Channel"
  cancelText="Cancel"
  variant="danger"
  on:confirm={confirmDeleteChannel}
  on:cancel={() => {
    showDeleteConfirmation = false;
    channelToDelete = null;
  }}
/>

