<script>
  import { onMount } from 'svelte';
  import { User, Users, X, Plus, Shield } from 'lucide-svelte';
  import Button from '../components/Button.svelte';
  import AssigneePicker from '../pickers/AssigneePicker.svelte';
  import ConfirmDialog from '../dialogs/ConfirmDialog.svelte';
  import { api } from '../api.js';
  import { formatDateTimeLocale } from '../utils/dateFormatter.js';
  import Spinner from '../components/Spinner.svelte';
  import { t } from '../stores/i18n.svelte.js';

  // Props
  let { channelId = null, channelName = '', isDefault = false } = $props();

  // State
  let managers = $state([]);
  let showAddManager = $state(false);
  let addManagerType = $state('user');
  let selectedUserId = $state(null);
  let selectedGroupId = $state(null);
  let loading = $state(false);
  let saving = $state(false);
  let showRemoveConfirmation = $state(false);
  let managerToRemove = $state(null);

  onMount(async () => {
    await loadManagers();
  });

  async function loadManagers() {
    try {
      loading = true;
      managers = await api.channels.getManagers(channelId) || [];
    } catch (err) {
      console.error('Failed to load managers:', err);
      managers = [];
    } finally {
      loading = false;
    }
  }

  function toggleAddManager() {
    showAddManager = !showAddManager;
    if (!showAddManager) {
      selectedUserId = null;
      selectedGroupId = null;
      addManagerType = 'user';
    }
  }

  async function handleAddManager() {
    try {
      saving = true;
      const managerId = addManagerType === 'user' ? selectedUserId : selectedGroupId;
      await api.channels.addManagers(channelId, addManagerType, [managerId]);
      await loadManagers();
      toggleAddManager();
    } catch (err) {
      console.error('Failed to add manager:', err);
      alert(t('dialogs.alerts.failedToAddManager', { error: err.message || err }));
    } finally {
      saving = false;
    }
  }

  function removeManager(manager) {
    managerToRemove = manager;
    showRemoveConfirmation = true;
  }

  async function confirmRemoveManager() {
    try {
      await api.channels.removeManager(channelId, managerToRemove.id);
      await loadManagers();
    } catch (err) {
      console.error('Failed to remove manager:', err);
      alert(t('dialogs.alerts.failedToRemoveManager', { error: err.message || err }));
    } finally {
      showRemoveConfirmation = false;
      managerToRemove = null;
    }
  }

</script>

<div class="space-y-6">
  <!-- Header with Add Button -->
  <div class="flex items-center justify-between">
    <div>
      <h3 class="text-lg font-semibold" style="color: var(--ds-text);">
        {t('settings.channelManagers.title')}
      </h3>
      <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
        {#if isDefault}
          <span class="flex items-center gap-2">
            <Shield class="w-4 h-4 text-amber-500" />
            {t('settings.channelManagers.systemChannelNote')}
          </span>
        {:else}
          {t('settings.channelManagers.description')}
        {/if}
      </p>
    </div>

    {#if !isDefault}
      <Button
        onclick={toggleAddManager}
        variant="primary"
        size="medium"
        icon={Plus}
      >
        {t('settings.channelManagers.addManager')}
      </Button>
    {/if}
  </div>

  <!-- Add Manager Form -->
  {#if showAddManager && !isDefault}
    <div
      class="p-4 rounded border"
      style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
    >
      <h4 class="text-sm font-medium mb-4" style="color: var(--ds-text);">
        {t('settings.channelManagers.addChannelManager')}
      </h4>

      <AssigneePicker
        bind:type={addManagerType}
        bind:userId={selectedUserId}
        bind:groupId={selectedGroupId}
        confirmText={t('settings.channelManagers.addManager')}
        cancelText={t('common.cancel')}
        on_confirm={handleAddManager}
        on_cancel={toggleAddManager}
      />
    </div>
  {/if}

  <!-- Managers List -->
  <div class="space-y-2">
    {#if loading}
      <div class="flex items-center justify-center py-12">
        <Spinner />
      </div>
    {:else if managers.length === 0}
      <div
        class="text-center py-12 rounded border-2 border-dashed"
        style="border-color: var(--ds-border);"
      >
        <div class="flex justify-center mb-3">
          {#if isDefault}
            <Shield class="w-12 h-12" style="color: var(--ds-text-subtle);" />
          {:else}
            <Users class="w-12 h-12" style="color: var(--ds-text-subtle);" />
          {/if}
        </div>
        <p class="text-sm font-medium" style="color: var(--ds-text);">
          {isDefault ? t('settings.channelManagers.systemAdminsCanManage') : t('settings.channelManagers.noManagersAssigned')}
        </p>
        <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
          {isDefault ? t('settings.channelManagers.defaultChannelsManaged') : t('settings.channelManagers.addUsersOrGroups')}
        </p>
      </div>
    {:else}
      {#each managers as manager}
        <div
          class="flex items-center gap-4 p-4 rounded border"
          style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
        >
          <!-- Icon -->
          <div
            class="flex-shrink-0 w-10 h-10 rounded-full flex items-center justify-center"
            style="background-color: {manager.manager_type === 'user' ? 'var(--ds-interactive-subtle)' : 'var(--ds-surface)'};"
          >
            {#if manager.manager_type === 'user'}
              <User class="w-5 h-5" style="color: var(--ds-interactive);" />
            {:else}
              <Users class="w-5 h-5" style="color: var(--ds-text-subtle);" />
            {/if}
          </div>

          <!-- Manager Info -->
          <div class="flex-1 min-w-0">
            <div class="flex items-center gap-2">
              <h4 class="text-sm font-medium" style="color: var(--ds-text);">
                {manager.manager_name}
              </h4>
              <span
                class="px-2 py-0.5 text-xs font-medium rounded"
                style="background-color: {manager.manager_type === 'user' ? 'var(--ds-interactive-subtle)' : 'var(--ds-surface)'}; color: {manager.manager_type === 'user' ? 'var(--ds-interactive-pressed)' : 'var(--ds-text-subtle)'};"
              >
                {manager.manager_type === 'user' ? t('settings.channelManagers.user') : t('settings.channelManagers.group')}
              </span>
            </div>
            {#if manager.manager_email}
              <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
                {manager.manager_email}
              </p>
            {/if}
            <p class="text-xs mt-1" style="color: var(--ds-text-disabled);">
              {t('settings.channelManagers.addedBy')} {manager.added_by_name} {t('settings.channelManagers.on')} {formatDateTimeLocale(manager.created_at) || '-'}
            </p>
          </div>

          <!-- Remove Button -->
          {#if !isDefault}
            <button
              onclick={() => removeManager(manager)}
              class="flex-shrink-0 p-2 rounded hover:bg-red-50 transition-colors"
              title={t('settings.channelManagers.removeManager')}
              style="color: var(--ds-text-subtle);"
            >
              <X class="w-4 h-4" />
            </button>
          {/if}
        </div>
      {/each}
    {/if}
  </div>

  <!-- Help Text -->
  {#if !isDefault}
    <div
      class="p-4 rounded"
      style="background-color: var(--ds-surface);"
    >
      <p class="text-sm" style="color: var(--ds-text-subtle);">
        <strong style="color: var(--ds-text);">{t('settings.channelManagers.note')}</strong> {t('settings.channelManagers.noteText')}
      </p>
    </div>
  {/if}
</div>

<!-- Remove Manager Confirmation Dialog -->
<ConfirmDialog
  bind:show={showRemoveConfirmation}
  title={t('settings.channelManagers.removeManager')}
  message={t('settings.channelManagers.confirmRemoveMessage', { name: managerToRemove?.manager_name })}
  confirmText={t('settings.channelManagers.removeManager')}
  cancelText={t('common.cancel')}
  variant="danger"
  onconfirm={confirmRemoveManager}
  oncancel={() => {
    showRemoveConfirmation = false;
    managerToRemove = null;
  }}
/>
