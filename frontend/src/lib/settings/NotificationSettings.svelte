<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { authStore } from '../stores/auth.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import {
    Bell, Plus, Edit, Trash2, Save, X, Check,
    AlertCircle, Settings
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
    event_rules: [],
    created_by: undefined
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

  // Recipient picker helpers
  const recipientItems = $derived([
    { id: 'assignee', name: t('settings.notifications.assignee') },
    { id: 'creator', name: t('settings.notifications.creator') },
    { id: 'watchers', name: t('settings.notifications.watchers') },
    { id: 'workspace_admins', name: t('settings.notifications.workspaceAdmins') }
  ]);

  function getRecipientValue(rule) {
    const result = [];
    if (rule.notify_assignee) result.push('assignee');
    if (rule.notify_creator) result.push('creator');
    if (rule.notify_watchers) result.push('watchers');
    if (rule.notify_workspace_admins) result.push('workspace_admins');
    return result;
  }

  function handleRecipientsChange(index, selectedIds) {
    formData.event_rules[index] = {
      ...formData.event_rules[index],
      notify_assignee: selectedIds.includes('assignee'),
      notify_creator: selectedIds.includes('creator'),
      notify_watchers: selectedIds.includes('watchers'),
      notify_workspace_admins: selectedIds.includes('workspace_admins')
    };
  }

  function openCreateModal() {
    formData = {
      name: '',
      description: '',
      is_active: true,
      event_rules: [],
      created_by: undefined
    };
    showCustomMessage = [];
    showCreateModal = true;
  }

  function openEditModal(setting) {
    editingSetting = setting;
    formData = {
      name: setting.name,
      description: setting.description || '',
      is_active: true,
      event_rules: [...(setting.event_rules || [])],
      created_by: setting.created_by
    };
    showCustomMessage = (setting.event_rules || []).map(rule => !!(rule.message_template));
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
      event_rules: [],
      created_by: undefined
    };
    showCustomMessage = [];
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
        formData.created_by = /** @type {any} */ (authStore.currentUser)?.id;
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
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('settings.notifications.confirmDelete', { name: setting.name }),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger'
    });
    if (!confirmed) return;

    try {
      await api.notificationSettings.delete(setting.id);
      await loadNotificationSettings();
    } catch (err) {
      console.error('Failed to delete notification setting:', err);
      alert(t('settings.notifications.failedToDelete') + ': ' + err.message);
    }
  }

  // Custom message toggle state
  let showCustomMessage = $state([]);

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
    showCustomMessage = [...showCustomMessage, false];
  }

  function removeEventRule(index) {
    formData.event_rules = formData.event_rules.filter((_, i) => i !== index);
    showCustomMessage = showCustomMessage.filter((_, i) => i !== index);
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
    />

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
                  <!-- Event Type + Enable + Delete inline -->
                  <div class="flex items-end gap-3">
                    <div class="flex-1">
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
                    <div class="flex items-center pb-1.5">
                      <Checkbox
                        bind:checked={rule.is_enabled}
                        label={t('common.enable')}
                        size="small"
                      />
                    </div>
                    <button
                      type="button"
                      onclick={() => removeEventRule(index)}
                      class="text-red-600 hover:text-red-800 transition-colors pb-1.5"
                    >
                      <Trash2 class="w-4 h-4" />
                    </button>
                  </div>

                  <!-- Notification Recipients (multiselect) -->
                  <div class="mt-3">
                    <Label color="default" class="mb-1">{t('settings.notifications.notifyRecipients')}</Label>
                    <BasePicker
                      value={getRecipientValue(rule)}
                      items={recipientItems}
                      multiple={true}
                      placeholder={t('settings.notifications.selectRecipients')}
                      onChange={(selectedIds) => handleRecipientsChange(index, selectedIds)}
                    />
                  </div>

                  <!-- Custom Message Toggle -->
                  <div class="mt-3">
                    <Checkbox
                      checked={showCustomMessage[index] || false}
                      label={t('settings.notifications.customizeMessage')}
                      size="small"
                      onchange={(val) => {
                        showCustomMessage[index] = val;
                        if (!val) {
                          formData.event_rules[index] = { ...formData.event_rules[index], message_template: '' };
                        }
                      }}
                    />
                    {#if showCustomMessage[index]}
                      <div class="mt-2">
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
                    {/if}
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