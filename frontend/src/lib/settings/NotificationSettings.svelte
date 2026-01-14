<script>
  import { onMount, createEventDispatcher } from 'svelte';
  import { writable } from 'svelte/store';
  import { api } from '../api.js';
  import { authStore } from '../stores/auth.svelte.js';
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
  import DialogFooter from '../dialogs/DialogFooter.svelte';
  import { getShortcut, matchesShortcut } from '../utils/keyboardShortcuts.js';

  const submitShortcut = getShortcut('modal', 'submit');

  const dispatch = createEventDispatcher();

  let notificationSettings = [];
  let availableEvents = [];
  let loading = true;
  let error = null;
  let showCreateModal = false;
  let showEditModal = false;
  let editingSetting = null;

  // Form state
  let formData = {
    name: '',
    description: '',
    is_active: true,
    event_rules: []
  };

  // Load notification settings and available events
  onMount(async () => {
    await loadNotificationSettings();
    await loadAvailableEvents();
    
    // Add keyboard event listener
    document.addEventListener('keydown', handleKeydown);
    return () => {
      document.removeEventListener('keydown', handleKeydown);
    };
  });

  async function loadNotificationSettings() {
    try {
      loading = true;
      const data = await api.notificationSettings.getAll();
      notificationSettings = data || [];
    } catch (err) {
      console.error('Failed to load notification settings:', err);
      error = 'Failed to load notification settings';
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
      alert('Name is required');
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
      alert('Failed to save notification setting: ' + err.message);
    }
  }

  async function handleDelete(setting) {
    if (!confirm(`Are you sure you want to delete "${setting.name}"? This action cannot be undone.`)) {
      return;
    }

    try {
      await api.notificationSettings.delete(setting.id);
      await loadNotificationSettings();
    } catch (err) {
      console.error('Failed to delete notification setting:', err);
      alert('Failed to delete notification setting: ' + err.message);
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
      alert('Failed to update notification setting: ' + err.message);
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
        title: 'Edit',
        hoverClass: 'hover-bg',
        onClick: () => openEditModal(setting)
      },
      {
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: 'Delete',
        color: '#dc2626',
        hoverClass: 'hover:bg-red-50',
        onClick: () => handleDelete(setting)
      }
    ];
  }

  // Table column definitions
  const notificationColumns = [
    {
      key: 'setting_info',
      label: 'Setting',
      render: (setting) => setting.name + (setting.description ? ` - ${setting.description}` : '')
    },
    {
      key: 'status_info',
      label: 'Status',
      slot: 'status'
    },
    {
      key: 'event_rules_info',
      label: 'Event Rules',
      render: (setting) => {
        const rulesCount = setting.event_rules?.length || 0;
        const enabledCount = setting.event_rules?.filter(rule => rule.is_enabled).length || 0;
        return rulesCount > 0 ? `${rulesCount} rules configured (${enabledCount} enabled)` : '0 rules configured';
      }
    },
    {
      key: 'created_by_name',
      label: 'Created By',
      render: (setting) => setting.created_by_name || `User ${setting.created_by}`
    },
    {
      key: 'actions',
      label: 'Actions'
    }
  ];

  // Handle keyboard shortcuts
  function handleKeydown(event) {
    if (matchesShortcut(event, { key: 'a' }) && !showCreateModal && !showEditModal) {
      const target = event.target;
      if (target.tagName !== 'INPUT' && target.tagName !== 'TEXTAREA' && !target.contentEditable.includes('true')) {
        event.preventDefault();
        openCreateModal();
      }
    } else if (event.key === 'Escape' && (showCreateModal || showEditModal)) {
      event.preventDefault();
      closeModals();
    } else if (matchesShortcut(event, submitShortcut) && (showCreateModal || showEditModal)) {
      event.preventDefault();
      handleSubmit();
    }
  }

</script>

<div class="space-y-6">
  <PageHeader
    icon={Bell}
    title="Notification Settings"
    subtitle="Create and manage notification configurations that can be assigned to configuration sets"
  >
    {#snippet actions()}
      <Button
        variant="primary"
        icon={Plus}
        onclick={openCreateModal}
        keyboardHint="A"
      >
        Create Notification Setting
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
      emptyMessage="No notification settings found. Create your first notification setting to get started."
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
            Active
          {:else}
            <PowerOff class="w-3 h-3" />
            Inactive
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
      {editingSetting ? 'Edit Notification Setting' : 'Create Notification Setting'}
    </h3>
  </div>

  <!-- Modal content -->
  <div class="px-6 py-4 space-y-6 max-h-[60vh] overflow-y-auto">
    <form onsubmit={(e) => { e.preventDefault(); handleSubmit(); }}>
      <div class="space-y-6">
        <!-- Basic Information -->
        <div class="grid grid-cols-1 gap-4">
          <div>
            <Label for="name" color="default" required class="mb-1">Name</Label>
            <input
              id="name"
              type="text"
              bind:value={formData.name}
              placeholder="e.g., Development Team Notifications"
              class="w-full px-3 py-2 border rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
              style="border-color: var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
              required
            />
          </div>

          <div>
            <Label for="description" color="default" class="mb-1">Description</Label>
            <Textarea
              id="description"
              bind:value={formData.description}
              placeholder="Description of this notification setting..."
              rows={3}
            />
          </div>

          <div class="flex items-center">
            <input
              id="is_active"
              type="checkbox"
              bind:checked={formData.is_active}
              class="h-4 w-4 text-blue-600 focus:ring-blue-500 border-gray-300 rounded"
            />
            <label for="is_active" class="ml-2 block text-sm" style="color: var(--ds-text)">
              Active (can be assigned to configuration sets)
            </label>
          </div>
        </div>

        <!-- Event Rules -->
        <div>
          <div class="flex items-center justify-between mb-4">
            <h4 class="text-sm font-medium" style="color: var(--ds-text)">Event Rules</h4>
            <Button
              variant="primary"
              size="sm"
              icon={Plus}
              onclick={addEventRule}
            >
              Add Rule
            </Button>
          </div>

          {#if formData.event_rules.length === 0}
            <div class="text-center py-8 rounded" style="background-color: var(--ds-surface)">
              <Settings class="w-8 h-8 mx-auto mb-2" style="color: var(--ds-icon-subtle)" />
              <p class="text-sm" style="color: var(--ds-text)">No event rules configured</p>
              <p class="text-xs mt-1" style="color: var(--ds-text-subtle)">Add rules to define when notifications should be sent</p>
            </div>
          {:else}
            <div class="space-y-4">
              {#each formData.event_rules as rule, index (index)}
                <div class="border rounded p-4" style="border-color: var(--ds-border)">
                  <div class="flex items-center justify-between mb-4">
                    <h5 class="font-medium" style="color: var(--ds-text)">Rule {index + 1}</h5>
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
                      <Label color="default" required class="mb-1">Event Type</Label>
                      <BasePicker
                        bind:value={rule.event_type}
                        items={availableEvents || []}
                        placeholder="Select event type..."
                        showUnassigned={true}
                        unassignedLabel="Select event type..."
                        getValue={(event) => event.type}
                        getLabel={(event) => `${event.category ? event.category.charAt(0).toUpperCase() + event.category.slice(1) + ': ' : ''}${event.name}`}
                      />
                    </div>

                    <!-- Enabled -->
                    <div class="flex items-center pt-7">
                      <input
                        type="checkbox"
                        bind:checked={rule.is_enabled}
                        class="h-4 w-4 text-blue-600 focus:ring-blue-500 border-gray-300 rounded"
                      />
                      <label class="ml-2 block text-sm" style="color: var(--ds-text)">
                        Enabled
                      </label>
                    </div>
                  </div>

                  <!-- Notification Recipients -->
                  <div class="mt-4">
                    <Label color="default" class="mb-2">Notify Recipients</Label>
                    <div class="grid grid-cols-2 gap-4">
                      <label class="flex items-center">
                        <input
                          type="checkbox"
                          bind:checked={rule.notify_assignee}
                          class="h-4 w-4 text-blue-600 focus:ring-blue-500 border-gray-300 rounded"
                        />
                        <span class="ml-2 text-sm" style="color: var(--ds-text)">Assignee</span>
                      </label>

                      <label class="flex items-center">
                        <input
                          type="checkbox"
                          bind:checked={rule.notify_creator}
                          class="h-4 w-4 text-blue-600 focus:ring-blue-500 border-gray-300 rounded"
                        />
                        <span class="ml-2 text-sm" style="color: var(--ds-text)">Creator</span>
                      </label>

                      <label class="flex items-center">
                        <input
                          type="checkbox"
                          bind:checked={rule.notify_watchers}
                          class="h-4 w-4 text-blue-600 focus:ring-blue-500 border-gray-300 rounded"
                        />
                        <span class="ml-2 text-sm" style="color: var(--ds-text)">Watchers</span>
                      </label>

                      <label class="flex items-center">
                        <input
                          type="checkbox"
                          bind:checked={rule.notify_workspace_admins}
                          class="h-4 w-4 text-blue-600 focus:ring-blue-500 border-gray-300 rounded"
                        />
                        <span class="ml-2 text-sm" style="color: var(--ds-text)">Workspace Admins</span>
                      </label>
                    </div>
                  </div>

                  <!-- Message Template -->
                  <div class="mt-4">
                    <Label color="default" class="mb-1">Custom Message Template (optional)</Label>
                    <Textarea
                      bind:value={rule.message_template}
                      placeholder="Leave empty for default message, or customize using variables like &#123;item.title&#125;, &#123;user.name&#125;, &#123;workspace.name&#125;"
                      rows={3}
                      class="text-sm"
                    />
                    <div class="mt-1 text-xs" style="color: var(--ds-text-subtle)">
                      <strong>Available variables:</strong> &#123;item.title&#125;, &#123;item.key&#125;, &#123;user.name&#125;, &#123;workspace.name&#125;, &#123;event.type&#125;<br>
                      <strong>Example:</strong> "Work item &#123;item.key&#125; '&#123;item.title&#125;' was updated in &#123;workspace.name&#125; by &#123;user.name&#125;"
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
    confirmLabel={editingSetting ? 'Update Setting' : 'Create Setting'}
  />
</Modal>