<script>
  import { onMount, createEventDispatcher } from 'svelte';
  import { writable } from 'svelte/store';
  import { api } from '../api.js';
  import { authStore } from '../stores/auth.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import {
    Bell, Plus, Edit, Trash2, Save, X, Check,
    AlertCircle, Settings, Power, PowerOff
  } from 'lucide-svelte';
  import DataTable from '../components/DataTable.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Button from '../components/Button.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import Spinner from '../components/Spinner.svelte';
  import Textarea from '../components/Textarea.svelte';
  import BasePicker from '../pickers/BasePicker.svelte';
  import Label from '../components/Label.svelte';
  import Checkbox from '../components/Checkbox.svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';

  const dispatch = createEventDispatcher();

  let notificationSettings = $state([]);
  let availableEvents = $state([]);
  let loading = $state(true);
  let error = $state(null);
  let showCreateModal = $state(false);
  let showEditModal = $state(false);
  let editingSetting = $state(null);

  // Form state
  let formData = $state({
    name: '',
    description: '',
    is_active: true,
    event_rules: []
  });

  // Load notification settings and available events
  onMount(async () => {
    await loadNotificationSettings();
    await loadAvailableEvents();
  });

  async function loadNotificationSettings() {
    try {
      loading = true;
      const data = await api.notificationSettings.getAll();
      notificationSettings = data || [];
    } catch (err) {
      console.error('Failed to load notification settings:', err);
      error = t('settings.notifications.failedToLoad');
    } finally {
      loading = false;
    }
  }

  async function loadAvailableEvents() {
    try {
      const data = await api.notificationSettings.getAvailableEvents();
      availableEvents = data || [];
    } catch (err) {
      console.error('Failed to load available events:', err);
    }
  }

  function openCreateModal() {
    formData = {
      name: '',
      description: '',
      is_active: true,
      event_rules: []
    };
    showCreateModal = true;
  }

  function openEditModal(setting) {
    editingSetting = setting;
    formData = {
      name: setting.name,
      description: setting.description || '',
      is_active: setting.is_active,
      event_rules: [...(setting.event_rules || [])]
    };
    showEditModal = true;
  }

  function closeModals() {
    showCreateModal = false;
    showEditModal = false;
    editingSetting = null;
    formData = {
      name: '',
      description: '',
      is_active: true,
      event_rules: []
    };
  }

  async function handleSubmit() {
    if (!formData.name.trim()) {
      alert(t('settings.notifications.nameRequired'));
      return;
    }

    try {
      if (editingSetting) {
        await api.notificationSettings.update(editingSetting.id, formData);
      } else {
        // Add created_by from current user
        formData.created_by = authStore.currentUser?.id;
        await api.notificationSettings.create(formData);
      }

      await loadNotificationSettings();
      closeModals();
    } catch (err) {
      console.error('Failed to save notification setting:', err);
      alert(t('settings.notifications.failedToSave') + ': ' + err.message);
    }
  }

  async function handleDelete(setting) {
    if (!confirm(t('settings.notifications.confirmDelete', { name: setting.name }))) {
      return;
    }

    try {
      await api.notificationSettings.delete(setting.id);
      await loadNotificationSettings();
    } catch (err) {
      console.error('Failed to delete notification setting:', err);
      alert(t('settings.notifications.failedToDelete') + ': ' + err.message);
    }
  }

  async function toggleActive(setting) {
    try {
      const updatedSetting = {
        ...setting,
        is_active: !setting.is_active
      };
      await api.notificationSettings.update(setting.id, updatedSetting);
      await loadNotificationSettings();
    } catch (err) {
      console.error('Failed to toggle notification setting:', err);
      alert(t('settings.notifications.failedToUpdate') + ': ' + err.message);
    }
  }

  function addEventRule() {
    formData.event_rules = [
      ...formData.event_rules,
      {
        event_type: '',
        is_enabled: true,
        notify_assignee: false,
        notify_creator: false,
        notify_watchers: false,
        notify_workspace_admins: false,
        message_template: ''
      }
    ];
  }

  function removeEventRule(index) {
    formData.event_rules = formData.event_rules.filter((_, i) => i !== index);
  }

  function groupEventsByCategory(events) {
    if (!events || events.length === 0) {
      return {};
    }
    const grouped = {};
    events.forEach(event => {
      if (!event || !event.category) return; // Safety check
      if (!grouped[event.category]) {
        grouped[event.category] = [];
      }
      grouped[event.category].push(event);
    });
    return grouped;
  }

  function buildNotificationDropdownItems(setting) {
    return [
      {
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: t('common.edit'),
        hoverClass: 'hover-bg',
        onClick: () => openEditModal(setting)
      },
      {
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: t('common.delete'),
        color: '#dc2626',
        hoverClass: 'hover:bg-red-50',
        onClick: () => handleDelete(setting)
      }
    ];
  }

  // Table column definitions - reactive for i18n
  const notificationColumns = $derived([
    {
      key: 'setting_info',
      label: t('settings.notifications.setting'),
      render: (setting) => setting.name + (setting.description ? ` - ${setting.description}` : '')
    },
    {
      key: 'status_info',
      label: t('common.status'),
      slot: 'status'
    },
    {
      key: 'event_rules_info',
      label: t('settings.notifications.eventRules'),
      render: (setting) => {
        const rulesCount = setting.event_rules?.length || 0;
        const enabledCount = setting.event_rules?.filter(rule => rule.is_enabled).length || 0;
        return rulesCount > 0 ? t('settings.notifications.rulesConfigured', { count: rulesCount, enabled: enabledCount }) : t('settings.notifications.noRules');
      }
    },
    {
      key: 'created_by_name',
      label: t('settings.notifications.createdBy'),
      render: (setting) => setting.created_by_name || `User ${setting.created_by}`
    },
    {
      key: 'actions',
      label: t('common.actions')
    }
  ]);

</script>

<div class="space-y-6">
  <PageHeader
    icon={Bell}
    title={t('settings.notifications.title')}
    subtitle={t('settings.notifications.subtitle')}
  >
    {#snippet actions()}
      <Button
        variant="primary"
        icon={Plus}
        onclick={openCreateModal}
        keyboardHint="A"
        hotkeyConfig={{ key: toHotkeyString('notifications', 'add'), guard: () => !showCreateModal && !showEditModal }}
      >
        {t('settings.notifications.createSetting')}
      </Button>
    {/snippet}
  </PageHeader>

  {#if loading}
    <div class="flex justify-center py-12">
      <Spinner />
    </div>
  {:else if error}
    <div class="bg-red-50 border border-red-200 rounded p-4 flex items-center gap-2">
      <AlertCircle class="w-5 h-5 text-red-600" />
      <span class="text-red-700">{error}</span>
    </div>
  {:else}
    <DataTable
      columns={notificationColumns}
      data={notificationSettings}
      keyField="id"
      emptyMessage={t('settings.notifications.noSettingsFound')}
      emptyIcon={Bell}
      actionItems={buildNotificationDropdownItems}
    >

      <div slot="status" let:item={setting}>
        <button
          onclick={() => toggleActive(setting)}
          class="flex items-center gap-2 px-3 py-1 rounded-full text-sm font-medium transition-colors
                 {setting.is_active
                   ? 'bg-green-100 text-green-800 hover:bg-green-200'
                   : 'bg-gray-100 text-gray-800 hover:bg-gray-200'}"
        >
          {#if setting.is_active}
            <Power class="w-3 h-3" />
            {t('common.active')}
          {:else}
            <PowerOff class="w-3 h-3" />
            {t('common.inactive')}
          {/if}
        </button>
      </div>

    </DataTable>
  {/if}
</div>

<Modal isOpen={showCreateModal || showEditModal} onclose={closeModals} maxWidth="max-w-4xl">
  <!-- Modal header -->
  <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
    <h3 class="text-lg font-semibold flex items-center gap-2" style="color: var(--ds-text);">
      <Bell class="w-5 h-5" />
      {editingSetting ? t('settings.notifications.editSetting') : t('settings.notifications.createSetting')}
    </h3>
  </div>

  <!-- Modal content -->
  <div class="px-6 py-4 space-y-6 max-h-[60vh] overflow-y-auto">
    <form onsubmit={(e) => { e.preventDefault(); handleSubmit(); }}>
      <div class="space-y-6">
        <!-- Basic Information -->
        <div class="grid grid-cols-1 gap-4">
          <div>
            <Label for="name" color="default" required class="mb-1">{t('settings.notifications.name')}</Label>
            <input
              id="name"
              type="text"
              bind:value={formData.name}
              placeholder={t('settings.notifications.namePlaceholder')}
              class="w-full px-3 py-2 border rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
              style="border-color: var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
              required
            />
          </div>

          <div>
            <Label for="description" color="default" class="mb-1">{t('settings.notifications.description')}</Label>
            <Textarea
              id="description"
              bind:value={formData.description}
              placeholder={t('settings.notifications.descriptionPlaceholder')}
              rows={3}
            />
          </div>

          <Checkbox
            bind:checked={formData.is_active}
            label={t('settings.notifications.activeCanBeAssigned')}
            size="small"
          />
        </div>

        <!-- Event Rules -->
        <div>
          <div class="flex items-center justify-between mb-4">
            <h4 class="text-sm font-medium" style="color: var(--ds-text)">{t('settings.notifications.eventRules')}</h4>
            <Button
              variant="primary"
              size="sm"
              icon={Plus}
              onclick={addEventRule}
            >
              {t('settings.notifications.addRule')}
            </Button>
          </div>

          {#if formData.event_rules.length === 0}
            <div class="text-center py-8 rounded" style="background-color: var(--ds-surface)">
              <Settings class="w-8 h-8 mx-auto mb-2" style="color: var(--ds-icon-subtle)" />
              <p class="text-sm" style="color: var(--ds-text)">{t('settings.notifications.noEventRulesConfigured')}</p>
              <p class="text-xs mt-1" style="color: var(--ds-text-subtle)">{t('settings.notifications.noEventRulesDesc')}</p>
            </div>
          {:else}
            <div class="space-y-4">
              {#each formData.event_rules as rule, index (index)}
                <div class="border rounded p-4" style="border-color: var(--ds-border)">
                  <div class="flex items-center justify-between mb-4">
                    <h5 class="font-medium" style="color: var(--ds-text)">{t('settings.notifications.rule')} {index + 1}</h5>
                    <button
                      type="button"
                      onclick={() => removeEventRule(index)}
                      class="text-red-600 hover:text-red-800 transition-colors"
                    >
                      <Trash2 class="w-4 h-4" />
                    </button>
                  </div>

                  <div class="grid grid-cols-2 gap-4">
                    <!-- Event Type -->
                    <div>
                      <Label color="default" required class="mb-1">{t('settings.notifications.eventType')}</Label>
                      <BasePicker
                        bind:value={rule.event_type}
                        items={availableEvents || []}
                        placeholder={t('settings.notifications.selectEventType')}
                        showUnassigned={true}
                        unassignedLabel={t('settings.notifications.selectEventType')}
                        getValue={(event) => event.type}
                        getLabel={(event) => `${event.category ? event.category.charAt(0).toUpperCase() + event.category.slice(1) + ': ' : ''}${event.name}`}
                      />
                    </div>

                    <!-- Enabled -->
                    <div class="flex items-center pt-7">
                      <Checkbox
                        bind:checked={rule.is_enabled}
                        label={t('common.enable')}
                        size="small"
                      />
                    </div>
                  </div>

                  <!-- Notification Recipients -->
                  <div class="mt-4">
                    <Label color="default" class="mb-2">{t('settings.notifications.notifyRecipients')}</Label>
                    <div class="grid grid-cols-2 gap-4">
                      <Checkbox
                        bind:checked={rule.notify_assignee}
                        label={t('settings.notifications.assignee')}
                        size="small"
                      />

                      <Checkbox
                        bind:checked={rule.notify_creator}
                        label={t('settings.notifications.creator')}
                        size="small"
                      />

                      <Checkbox
                        bind:checked={rule.notify_watchers}
                        label={t('settings.notifications.watchers')}
                        size="small"
                      />

                      <Checkbox
                        bind:checked={rule.notify_workspace_admins}
                        label={t('settings.notifications.workspaceAdmins')}
                        size="small"
                      />
                    </div>
                  </div>

                  <!-- Message Template -->
                  <div class="mt-4">
                    <Label color="default" class="mb-1">{t('settings.notifications.customMessageTemplate')}</Label>
                    <Textarea
                      bind:value={rule.message_template}
                      placeholder={t('settings.notifications.messageTemplatePlaceholder')}
                      rows={3}
                      class="text-sm"
                    />
                    <div class="mt-1 text-xs" style="color: var(--ds-text-subtle)">
                      <strong>{t('settings.notifications.availableVariables')}:</strong> &#123;item.title&#125;, &#123;item.key&#125;, &#123;user.name&#125;, &#123;workspace.name&#125;, &#123;event.type&#125;<br>
                      <strong>{t('settings.notifications.example')}:</strong> "Work item &#123;item.key&#125; '&#123;item.title&#125;' was updated in &#123;workspace.name&#125; by &#123;user.name&#125;"
                    </div>
                  </div>
                </div>
              {/each}
            </div>
          {/if}
        </div>
      </div>
    </form>
  </div>

  <DialogFooter
    onCancel={closeModals}
    onConfirm={handleSubmit}
    confirmLabel={editingSetting ? t('settings.notifications.updateSetting') : t('settings.notifications.createSetting')}
  />
</Modal>