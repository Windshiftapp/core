<script>
  import { onMount } from 'svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { api } from '../api.js';
  import { navigate } from '../router.js';
  import {
    Plus, Edit, Trash2, Save, X, Settings, Workflow,
    FileText, Target, Zap, BookOpen, CheckSquare, Bug, Minus, Star, Flag,
    Lightbulb, User, Users, Calendar, Clock, MapPin, Search, Filter, Tag,
    Bookmark, Heart, Shield, Key, Lock, Globe, Wifi, Database, Server,
    Code, Terminal, Folder, Image, Video, Music, Download, Upload, Send,
    Mail, Phone, MessageSquare, AlertCircle, Info, CheckCircle, XCircle,
    HelpCircle, Archive, Trash, Copy, Scissors, Paperclip, Link, ExternalLink,
    Circle, Layers
  } from 'lucide-svelte';
  import Button from '../components/Button.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import MigrationAssistant from '../pages/MigrationAssistant.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import Pagination from '../components/Pagination.svelte';
  import Textarea from '../components/Textarea.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import BasePicker from '../pickers/BasePicker.svelte';
  import Label from '../components/Label.svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';

  // Icon mapping for item types
  const iconMap = {
    Target, Zap, BookOpen, CheckSquare, Bug, Minus, Star, Flag, Lightbulb,
    Settings, User, Users, Calendar, Clock, MapPin, Search, Filter, Tag,
    Bookmark, Heart, Shield, Key, Lock, Globe, Wifi, Database, Server,
    Code, Terminal, FileText, Folder, Image, Video, Music, Download,
    Upload, Send, Mail, Phone, MessageSquare, AlertCircle, Info,
    CheckCircle, XCircle, HelpCircle, Archive, Trash, Edit, Copy,
    Scissors, Paperclip, Link, ExternalLink, Circle, Layers
  };

  let configurationSets = $state([]);
  let workspaces = $state([]);
  let workflows = $state([]);
  let screens = $state([]);
  let notificationSettings = $state([]);
  let loading = $state(true);
  let creating = $state(false);
  let editingId = $state(null);
  let showEditModal = $state(false);

  // Search and pagination state
  let searchQuery = $state('');
  let currentPage = $state(1);
  let itemsPerPage = $state(10);
  let totalConfigSets = $state(0);
  let searchTimeout;

  // Migration assistant state
  let showMigrationAssistant = $state(false);
  let migrationConfigSet = $state(null);

  // Form state
  let newConfigSet = $state({
    name: '',
    description: '',
    workspace_ids: [],
    workflow_id: null,
    create_screen_id: null,
    edit_screen_id: null,
    view_screen_id: null,
    notification_setting_id: null,
    is_default: false
  });

  let editConfigSet = $state({
    name: '',
    description: '',
    workspace_ids: [],
    workflow_id: null,
    create_screen_id: null,
    edit_screen_id: null,
    view_screen_id: null,
    notification_setting_id: null,
    is_default: false
  });

  onMount(async () => {
    await loadData(currentPage, itemsPerPage, searchQuery);
  });

  async function loadData(page = 1, limit = 10, search = '') {
    try {
      loading = true;

      // Build query string for pagination and search
      const params = new URLSearchParams({
        page: page.toString(),
        limit: limit.toString()
      });
      if (search) {
        params.append('search', search);
      }

      const [configSetsResponse, workspacesData, workflowsData, screensData, notificationSettingsData] = await Promise.all([
        api.get(`/configuration-sets?${params.toString()}`),
        api.workspaces.getAll(),
        api.get('/workflows'),
        api.get('/screens'),
        api.notificationSettings.getAll()
      ]);

      // Debug logging
      console.log('Configuration Sets Response:', configSetsResponse);
      console.log('Response structure:', {
        hasConfigurationSets: !!configSetsResponse.configuration_sets,
        hasPagination: !!configSetsResponse.pagination,
        configSetsLength: configSetsResponse.configuration_sets?.length || 0
      });

      // Extract pagination data from response
      configurationSets = configSetsResponse.configuration_sets || [];
      if (configSetsResponse.pagination) {
        totalConfigSets = configSetsResponse.pagination.total;
        currentPage = configSetsResponse.pagination.page;
        itemsPerPage = configSetsResponse.pagination.limit;
      } else {
        console.warn('No pagination metadata in response');
        totalConfigSets = configurationSets.length;
      }

      console.log('After assignment:', {
        configurationSetsLength: configurationSets.length,
        totalConfigSets,
        configSets: configurationSets
      });

      workspaces = workspacesData || [];
      workflows = workflowsData || [];
      screens = screensData || [];
      notificationSettings = notificationSettingsData || [];
    } catch (error) {
      console.error('Failed to load data:', error);
      configurationSets = [];
      workspaces = [];
      workflows = [];
      screens = [];
      notificationSettings = [];
      totalConfigSets = 0;
    } finally {
      loading = false;
    }
  }

  function startCreating() {
    navigate('/admin/configuration-sets/new');
  }

  function cancelCreating() {
    creating = false;
    newConfigSet = {
      name: '',
      description: '',
      workspace_ids: [],
      workflow_id: null,
      create_screen_id: null,
      edit_screen_id: null,
      view_screen_id: null,
      notification_setting_id: null,
      is_default: false
    };
  }

  async function createConfigurationSet() {
    try {
      if (!newConfigSet.name.trim()) {
        alert(t('dialogs.alerts.nameRequired'));
        return;
      }

      const payload = {
        ...newConfigSet,
        workspace_ids: newConfigSet.workspace_ids.map(id => parseInt(id)),
        workflow_id: newConfigSet.workflow_id ? parseInt(newConfigSet.workflow_id) : null
      };

      const created = await api.configurationSets.create(payload);
      
      // If the API doesn't return the created item, reload the data
      if (created && created.id) {
        configurationSets = [...configurationSets, created];
      } else {
        // Reload all data if create response is incomplete
        await loadData();
      }
      cancelCreating();
    } catch (error) {
      console.error('Failed to create configuration set:', error);
      alert(t('dialogs.alerts.failedToCreate', { error: error.message || error }));
    }
  }

  function startEditing(configSet) {
    if (!configSet) {
      console.error('startEditing called with null/undefined configuration set');
      return;
    }
    navigate(`/admin/configuration-sets/${configSet.id}`);
  }

  function cancelEditing() {
    editingId = null;
    showEditModal = false;
    editConfigSet = {
      name: '',
      description: '',
      workspace_ids: [],
      workflow_id: null,
      create_screen_id: null,
      edit_screen_id: null,
      view_screen_id: null,
      notification_setting_id: null,
      is_default: false
    };
  }

  async function updateConfigurationSet() {
    try {
      if (!editConfigSet.name.trim()) {
        alert(t('dialogs.alerts.nameRequired'));
        return;
      }

      // Check if workflow has changed
      const originalConfigSet = configurationSets.find(cs => cs.id === editingId);
      const oldWorkflowId = originalConfigSet ? originalConfigSet.workflow_id : null;
      const newWorkflowId = editConfigSet.workflow_id ? parseInt(editConfigSet.workflow_id) : null;
      const workflowChanged = oldWorkflowId !== newWorkflowId;

      const payload = {
        ...editConfigSet,
        workspace_ids: editConfigSet.workspace_ids.map(id => parseInt(id)),
        workflow_id: newWorkflowId
      };

      const updated = await api.configurationSets.update(editingId, payload);
      
      // If the API doesn't return the updated item, reload the data
      if (updated && updated.id) {
        configurationSets = configurationSets.map(cs => 
          cs.id === editingId ? updated : cs
        );
      } else {
        // Reload all data if update response is incomplete
        await loadData();
      }

      // If workflow changed and there are affected workspaces, check if migration is needed
      if (workflowChanged && payload.workspace_ids.length > 0) {
        const configSetToCheck = updated || configurationSets.find(cs => cs.id === editingId);

        // Analyze if migration is actually required
        const analysis = await api.configurationSets.analyzeMigration(configSetToCheck.id);

        if (analysis.requires_migration) {
          // Migration needed - show assistant
          migrationConfigSet = configSetToCheck;
          showMigrationAssistant = true;
        } else {
          // No migration needed - just show success message
          alert(t('dialogs.alerts.configUpdatedSuccess'));
        }
      }

      cancelEditing();
    } catch (error) {
      console.error('Failed to update configuration set:', error);
      alert(t('dialogs.alerts.failedToUpdate', { error: error.message || error }));
    }
  }

  async function deleteConfigurationSet(configSet) {
    if (!configSet) {
      console.error('deleteConfigurationSet called with null/undefined configuration set');
      return;
    }

    if (!confirm(t('dialogs.confirmations.deleteItem', { name: configSet.name }))) {
      return;
    }

    try {
      await api.configurationSets.delete(configSet.id);
      configurationSets = configurationSets.filter(cs => cs.id !== configSet.id);
    } catch (error) {
      console.error('Failed to delete configuration set:', error);
      alert(t('dialogs.alerts.failedToDelete', { error: error.message || error }));
    }
  }

  function handleMigrationAssistantClose() {
    showMigrationAssistant = false;
    migrationConfigSet = null;
  }

  function showMigrationAssistantForConfigSet(configSet) {
    migrationConfigSet = configSet;
    showMigrationAssistant = true;
  }

  function getWorkspaceName(workspaceId) {
    const workspace = workspaces.find(w => w.id === workspaceId);
    return workspace ? workspace.name : 'Unknown';
  }

  function getWorkflowName(workflowId) {
    if (!workflowId) return 'None';
    const workflow = workflows.find(w => w.id === workflowId);
    return workflow ? workflow.name : 'Unknown';
  }

  function getNotificationSettingName(notificationSettingId) {
    if (!notificationSettingId) return 'None';
    const setting = notificationSettings.find(s => s.id === notificationSettingId);
    return setting ? setting.name : 'Unknown';
  }

  // Helper functions for workspace selection
  function toggleWorkspaceSelection(workspaceId, isEditing = false) {
    const targetConfig = isEditing ? editConfigSet : newConfigSet;
    const currentIds = targetConfig.workspace_ids || [];
    
    if (currentIds.includes(workspaceId)) {
      // Remove workspace
      targetConfig.workspace_ids = currentIds.filter(id => id !== workspaceId);
    } else {
      // Add workspace
      targetConfig.workspace_ids = [...currentIds, workspaceId];
    }
    
    // Trigger reactivity
    if (isEditing) {
      editConfigSet = { ...editConfigSet };
    } else {
      newConfigSet = { ...newConfigSet };
    }
  }

  function isWorkspaceSelected(workspaceId, isEditing = false) {
    const targetConfig = isEditing ? editConfigSet : newConfigSet;
    return (targetConfig.workspace_ids || []).includes(workspaceId);
  }

  // Pagination handlers
  function handlePageChange(event) {
    const { page } = event.detail;
    loadData(page, itemsPerPage, searchQuery);
  }

  function handlePageSizeChange(event) {
    const { page, itemsPerPage: newItemsPerPage } = event.detail;
    itemsPerPage = newItemsPerPage;
    loadData(page, newItemsPerPage, searchQuery);
  }

  // Search handler with debounce
  function handleSearch(event) {
    const value = event.target.value;
    searchQuery = value;

    // Clear existing timeout
    if (searchTimeout) {
      clearTimeout(searchTimeout);
    }

    // Debounce search for 300ms
    searchTimeout = setTimeout(() => {
      currentPage = 1; // Reset to first page on search
      loadData(1, itemsPerPage, searchQuery);
    }, 300);
  }
</script>

{#snippet headerActions()}
  <Button variant="primary" icon={Plus} onclick={startCreating} keyboardHint="A" hotkeyConfig={{ key: toHotkeyString('configurationSets', 'add'), guard: () => !creating }}>
    {t('settings.configSets.addConfigSet')}
  </Button>
{/snippet}

<PageHeader
  icon={Settings}
  title={t('settings.configSets.title')}
  subtitle={t('settings.configSets.subtitle')}
  actions={headerActions}
/>

<!-- Search Bar -->
<div class="mb-6">
  <div class="relative max-w-md">
    <Search class="absolute left-3 top-1/2 transform -translate-y-1/2 w-4 h-4" style="color: var(--ds-icon-subtle);" />
    <input
      type="text"
      placeholder={t('settings.configSets.searchPlaceholder')}
      value={searchQuery}
      oninput={handleSearch}
      class="w-full pl-9 pr-4 py-2 border rounded text-sm focus:outline-none focus:ring-2"
      style="border-color: var(--ds-border); background-color: var(--ds-surface-raised); color: var(--ds-text);"
    />
  </div>
</div>

  {#if loading}
    <div class="rounded-xl border shadow-sm p-8 text-center" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
      <div class="animate-pulse" style="color: var(--ds-text-subtle);">{t('settings.configSets.loading')}</div>
    </div>
  {:else}
    <!-- Create Form -->
    <Modal isOpen={creating} onclose={cancelCreating} maxWidth="max-w-2xl" onSubmit={createConfigurationSet} let:submitHint>
      <!-- Modal header -->
      <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
        <h3 class="text-lg font-semibold" style="color: var(--ds-text);">
          {t('settings.configSets.createConfigSet')}
        </h3>
      </div>

      <!-- Modal content -->
      <div class="px-6 py-4">
        <form onsubmit={(e) => { e.preventDefault(); createConfigurationSet(); }}>
          <div class="space-y-4">
            <div>
              <Label color="default" required class="mb-2">{t('settings.configSets.name')}</Label>
              <input
                type="text"
                bind:value={newConfigSet.name}
                placeholder={t('settings.configSets.namePlaceholder')}
                class="w-full px-3 py-2 border rounded focus:outline-none focus:ring-2"
                style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text);"
              />
            </div>

            <div>
              <Label color="default" class="mb-3">{t('settings.configSets.workspaces')}</Label>
              <div class="space-y-2 max-h-48 overflow-y-auto border rounded p-3" style="border-color: var(--ds-border);">
                {#each workspaces as workspace}
                  <label class="flex items-center gap-3 p-2 rounded cursor-pointer workspace-option">
                    <input
                      type="checkbox"
                      checked={isWorkspaceSelected(workspace.id, false)}
                      onchange={() => toggleWorkspaceSelection(workspace.id, false)}
                      class="rounded"
                      style="border-color: var(--ds-border);"
                    />
                    <span class="text-sm" style="color: var(--ds-text);">{workspace.name}</span>
                  </label>
                {/each}
                {#if workspaces.length === 0}
                  <p class="text-sm italic" style="color: var(--ds-text-subtle);">{t('settings.configSets.noWorkspacesAvailable')}</p>
                {/if}
              </div>
              {#if newConfigSet.workspace_ids && newConfigSet.workspace_ids.length > 0}
                <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
                  {newConfigSet.workspace_ids.length} workspace{newConfigSet.workspace_ids.length === 1 ? '' : 's'} selected
                </p>
              {/if}
            </div>

            <div>
              <Label color="default" class="mb-2">{t('settings.configSets.workflow')}</Label>
              <BasePicker
                bind:value={newConfigSet.workflow_id}
                items={workflows}
                placeholder={t('settings.configSets.noWorkflow')}
                showUnassigned={true}
                unassignedLabel={t('settings.configSets.noWorkflow')}
                getValue={(item) => item.id}
                getLabel={(item) => item.name}
              />
            </div>

            <div>
              <Label color="default" class="mb-2">{t('settings.configSets.notificationSettings')}</Label>
              <BasePicker
                bind:value={newConfigSet.notification_setting_id}
                items={notificationSettings.filter(s => s.is_active)}
                placeholder={t('settings.configSets.notificationSettings')}
                showUnassigned={true}
                unassignedLabel={t('settings.configSets.notificationSettings')}
                getValue={(item) => item.id}
                getLabel={(item) => item.name}
              />
            </div>

            <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
              <div>
                <Label color="default" class="mb-2">{t('settings.configSets.createScreen')}</Label>
                <BasePicker
                  bind:value={newConfigSet.create_screen_id}
                  items={screens}
                  placeholder={t('settings.configSets.none')}
                  showUnassigned={true}
                  unassignedLabel={t('settings.configSets.none')}
                  getValue={(item) => item.id}
                  getLabel={(item) => item.name}
                />
              </div>

              <div>
                <Label color="default" class="mb-2">{t('settings.configSets.editScreen')}</Label>
                <BasePicker
                  bind:value={newConfigSet.edit_screen_id}
                  items={screens}
                  placeholder={t('settings.configSets.none')}
                  showUnassigned={true}
                  unassignedLabel={t('settings.configSets.none')}
                  getValue={(item) => item.id}
                  getLabel={(item) => item.name}
                />
              </div>

              <div>
                <Label color="default" class="mb-2">{t('settings.configSets.viewScreen')}</Label>
                <BasePicker
                  bind:value={newConfigSet.view_screen_id}
                  items={screens}
                  placeholder={t('settings.configSets.none')}
                  showUnassigned={true}
                  unassignedLabel={t('settings.configSets.none')}
                  getValue={(item) => item.id}
                  getLabel={(item) => item.name}
                />
              </div>
            </div>

            <div>
              <Label color="default" class="mb-2">{t('settings.configSets.description')}</Label>
              <Textarea
                bind:value={newConfigSet.description}
                placeholder={t('settings.configSets.description')}
                rows={2}
              />
            </div>

            <div class="flex items-center gap-2">
              <input
                type="checkbox"
                bind:checked={newConfigSet.is_default}
                id="new-default"
                class="rounded"
                style="border-color: var(--ds-border);"
              />
              <label for="new-default" class="text-sm" style="color: var(--ds-text);">{t('settings.configSets.setAsDefault')}</label>
            </div>
          </div>
        </form>
      </div>

      <DialogFooter
        onCancel={cancelCreating}
        onConfirm={createConfigurationSet}
        confirmLabel={t('settings.configSets.createConfigSet')}
        showKeyboardHint={true}
        confirmKeyboardHint={submitHint}
      />
    </Modal>

    <!-- Configuration Sets List -->
    {#if configurationSets.filter(cs => cs && cs.id && cs.name !== 'Personal Tasks Configuration').length === 0}
      <div class="rounded-xl border shadow-sm p-12" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <EmptyState
          icon={Settings}
          title={t('settings.configSets.noConfigSets')}
          description={t('settings.configSets.getStarted')}
        >
          {#snippet action()}
            <Button variant="primary" icon={Plus} onclick={startCreating}>
              {t('settings.configSets.createFirst')}
            </Button>
          {/snippet}
        </EmptyState>
      </div>
    {:else}
      <div class="space-y-3">
        {#each (configurationSets || []).filter(cs => cs && cs.id && cs.name !== 'Personal Tasks Configuration') as configSet (configSet.id)}
            <div class="rounded-xl border shadow-sm p-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
              <!-- Display Mode -->
              <div class="flex items-center justify-between">
                <div class="flex-1">
                  <div class="flex items-center gap-3 mb-2">
                    <h3 class="text-lg font-medium" style="color: var(--ds-text);">{configSet.name}</h3>
                    {#if configSet.is_default}
                      <Lozenge color="blue" text="Default" />
                    {/if}
                  </div>

                  <!-- Main sections with better spacing -->
                  <div class="space-y-5 mt-4">
                    <!-- Workspaces Section -->
                    <div>
                      <div class="flex items-center gap-2 mb-2">
                        <Layers class="w-4 h-4" style="color: var(--ds-icon-subtle);" />
                        <span class="text-xs font-medium uppercase tracking-wide" style="color: var(--ds-text-subtle);">{t('settings.configSets.workspaces')}</span>
                      </div>
                      {#if configSet.workspaces && configSet.workspaces.length > 0}
                        <div class="flex flex-wrap gap-2">
                          {#each configSet.workspaces as workspaceName}
                            <Lozenge color="gray" text={workspaceName} size="md" />
                          {/each}
                        </div>
                      {:else}
                        <span class="text-sm italic" style="color: var(--ds-text-disabled);">{t('settings.configSets.noWorkspacesAssigned')}</span>
                      {/if}
                    </div>

                    <!-- Item Types Section with icons and colors -->
                    <div>
                      <div class="flex items-center gap-2 mb-2">
                        <FileText class="w-4 h-4" style="color: var(--ds-icon-subtle);" />
                        <span class="text-xs font-medium uppercase tracking-wide" style="color: var(--ds-text-subtle);">{t('settings.configSets.itemTypes')}</span>
                      </div>
                      {#if configSet.item_types_detailed && configSet.item_types_detailed.length > 0}
                        <div class="flex flex-wrap gap-2">
                          {#each configSet.item_types_detailed as itemType}
                            <Lozenge customBg={itemType.color} size="md">
                              <span class="flex items-center justify-center w-4 h-4 rounded" style="background-color: {itemType.color};">
                                <svelte:component this={iconMap[itemType.icon] || FileText} size={10} color="white" />
                              </span>
                              {itemType.name}
                            </Lozenge>
                          {/each}
                        </div>
                      {:else}
                        <span class="text-sm italic" style="color: var(--ds-text-disabled);">{t('settings.configSets.noItemTypesAssigned')}</span>
                      {/if}
                    </div>

                    <!-- Workflow and Notifications Row -->
                    <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
                      <div>
                        <div class="flex items-center gap-2 mb-2">
                          <Workflow class="w-4 h-4" style="color: var(--ds-icon-subtle);" />
                          <span class="text-xs font-medium uppercase tracking-wide" style="color: var(--ds-text-subtle);">{t('settings.configSets.workflow')}</span>
                        </div>
                        {#if configSet.workflow_id}
                          <span class="text-sm font-medium" style="color: var(--ds-text);">{configSet.workflow_name || getWorkflowName(configSet.workflow_id)}</span>
                        {:else}
                          <span class="text-sm italic" style="color: var(--ds-text-disabled);">{t('settings.configSets.noneAssigned')}</span>
                        {/if}
                      </div>

                      <div>
                        <div class="flex items-center gap-2 mb-2">
                          <AlertCircle class="w-4 h-4" style="color: var(--ds-icon-subtle);" />
                          <span class="text-xs font-medium uppercase tracking-wide" style="color: var(--ds-text-subtle);">{t('settings.configSets.notifications')}</span>
                        </div>
                        {#if configSet.notification_setting_id}
                          <span class="text-sm font-medium" style="color: var(--ds-text);">{configSet.notification_setting_name || getNotificationSettingName(configSet.notification_setting_id)}</span>
                        {:else}
                          <span class="text-sm italic" style="color: var(--ds-text-disabled);">{t('settings.configSets.noneAssigned')}</span>
                        {/if}
                      </div>
                    </div>

                    <!-- Screens Section - Compact -->
                    <div>
                      <div class="flex items-center gap-2 mb-2">
                        <Copy class="w-4 h-4" style="color: var(--ds-icon-subtle);" />
                        <span class="text-xs font-medium uppercase tracking-wide" style="color: var(--ds-text-subtle);">{t('settings.configSets.screens')}</span>
                      </div>
                      <div class="flex flex-wrap gap-4 text-sm">
                        <div>
                          <span class="text-xs" style="color: var(--ds-text-subtle);">{t('settings.configSets.createScreen')}</span>
                          <span class="ml-1 font-medium" style="color: var(--ds-text);">{configSet.create_screen_name || t('settings.configSets.none')}</span>
                        </div>
                        <div>
                          <span class="text-xs" style="color: var(--ds-text-subtle);">{t('settings.configSets.editScreen')}</span>
                          <span class="ml-1 font-medium" style="color: var(--ds-text);">{configSet.edit_screen_name || t('settings.configSets.none')}</span>
                        </div>
                        <div>
                          <span class="text-xs" style="color: var(--ds-text-subtle);">{t('settings.configSets.viewScreen')}</span>
                          <span class="ml-1 font-medium" style="color: var(--ds-text);">{configSet.view_screen_name || t('settings.configSets.none')}</span>
                        </div>
                      </div>
                    </div>
                  </div>

                  <!-- Footer with metadata -->
                  <div class="mt-5 pt-4 border-t" style="border-color: var(--ds-border);">
                    <span class="text-xs" style="color: var(--ds-text-subtle);">{t('settings.configSets.created')} {new Date(configSet.created_at).toLocaleDateString()}</span>
                  </div>

                  {#if configSet.description}
                    <p class="text-sm mt-2" style="color: var(--ds-text-subtle);">{configSet.description}</p>
                  {/if}
                </div>

                <div class="flex items-center gap-2 ml-4">
                  <Button
                    variant="default"
                    size="small"
                    icon={Edit}
                    onclick={() => startEditing(configSet)}
                  >
                    {t('common.edit')}
                  </Button>
                  <Button
                    variant="default"
                    size="small"
                    icon={Trash2}
                    onclick={() => deleteConfigurationSet(configSet)}
                  >
                    {t('common.delete')}
                  </Button>
                </div>
              </div>
            </div>
        {/each}
      </div>

      <!-- Pagination -->
      {#if !loading && totalConfigSets > 0}
        <div class="mt-6">
          <Pagination
            {currentPage}
            {itemsPerPage}
            totalItems={totalConfigSets}
            pageSizeOptions={[10, 25, 50]}
            onpageChange={handlePageChange}
            onpageSizeChange={handlePageSizeChange}
          />
        </div>
      {/if}
    {/if}
  {/if}

<!-- Edit Modal -->
<Modal isOpen={showEditModal} onclose={cancelEditing} maxWidth="max-w-2xl" onSubmit={updateConfigurationSet} let:submitHint>
  <!-- Modal header -->
  <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
    <h3 class="text-lg font-semibold" style="color: var(--ds-text);">
      {t('settings.configSets.editConfigSet')}
    </h3>
  </div>

  <!-- Modal content -->
  <div class="px-6 py-4">
    <form onsubmit={(e) => { e.preventDefault(); updateConfigurationSet(); }}>
      <div class="space-y-4">
        <div>
          <Label color="default" required class="mb-2">{t('settings.configSets.name')}</Label>
          <input
            type="text"
            bind:value={editConfigSet.name}
            class="w-full px-3 py-2 border rounded focus:outline-none focus:ring-2"
            style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text);"
          />
        </div>

        <div>
          <Label color="default" class="mb-3">{t('settings.configSets.workspaces')}</Label>
          <div class="space-y-2 max-h-48 overflow-y-auto border rounded p-3" style="border-color: var(--ds-border);">
            {#each workspaces as workspace}
              <label class="flex items-center gap-3 p-2 rounded cursor-pointer workspace-option">
                <input
                  type="checkbox"
                  checked={isWorkspaceSelected(workspace.id, true)}
                  onchange={() => toggleWorkspaceSelection(workspace.id, true)}
                  class="rounded"
                  style="border-color: var(--ds-border);"
                />
                <span class="text-sm" style="color: var(--ds-text);">{workspace.name}</span>
              </label>
            {/each}
            {#if workspaces.length === 0}
              <p class="text-sm italic" style="color: var(--ds-text-subtle);">{t('settings.configSets.noWorkspacesAvailable')}</p>
            {/if}
          </div>
          {#if editConfigSet.workspace_ids && editConfigSet.workspace_ids.length > 0}
            <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
              {editConfigSet.workspace_ids.length} workspace{editConfigSet.workspace_ids.length === 1 ? '' : 's'} selected
            </p>
          {/if}
        </div>

        <div>
          <Label color="default" class="mb-2">{t('settings.configSets.workflow')}</Label>
          <BasePicker
            bind:value={editConfigSet.workflow_id}
            items={workflows}
            placeholder={t('settings.configSets.noWorkflow')}
            showUnassigned={true}
            unassignedLabel={t('settings.configSets.noWorkflow')}
            getValue={(item) => item.id}
            getLabel={(item) => item.name}
          />
        </div>

        <div>
          <Label color="default" class="mb-2">{t('settings.configSets.notificationSettings')}</Label>
          <BasePicker
            bind:value={editConfigSet.notification_setting_id}
            items={notificationSettings.filter(s => s.is_active)}
            placeholder={t('settings.configSets.notificationSettings')}
            showUnassigned={true}
            unassignedLabel={t('settings.configSets.notificationSettings')}
            getValue={(item) => item.id}
            getLabel={(item) => item.name}
          />
        </div>

        <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
          <div>
            <Label color="default" class="mb-2">{t('settings.configSets.createScreen')}</Label>
            <BasePicker
              bind:value={editConfigSet.create_screen_id}
              items={screens}
              placeholder={t('settings.configSets.none')}
              showUnassigned={true}
              unassignedLabel={t('settings.configSets.none')}
              getValue={(item) => item.id}
              getLabel={(item) => item.name}
            />
          </div>

          <div>
            <Label color="default" class="mb-2">{t('settings.configSets.editScreen')}</Label>
            <BasePicker
              bind:value={editConfigSet.edit_screen_id}
              items={screens}
              placeholder={t('settings.configSets.none')}
              showUnassigned={true}
              unassignedLabel={t('settings.configSets.none')}
              getValue={(item) => item.id}
              getLabel={(item) => item.name}
            />
          </div>

          <div>
            <Label color="default" class="mb-2">{t('settings.configSets.viewScreen')}</Label>
            <BasePicker
              bind:value={editConfigSet.view_screen_id}
              items={screens}
              placeholder={t('settings.configSets.none')}
              showUnassigned={true}
              unassignedLabel={t('settings.configSets.none')}
              getValue={(item) => item.id}
              getLabel={(item) => item.name}
            />
          </div>
        </div>

        <div>
          <Label color="default" class="mb-2">{t('settings.configSets.description')}</Label>
          <Textarea
            bind:value={editConfigSet.description}
            rows={2}
          />
        </div>

        <div class="flex items-center gap-2">
          <input
            type="checkbox"
            bind:checked={editConfigSet.is_default}
            id="edit-default"
            class="rounded"
            style="border-color: var(--ds-border);"
          />
          <label for="edit-default" class="text-sm" style="color: var(--ds-text);">{t('settings.configSets.setAsDefault')}</label>
        </div>
      </div>
    </form>
  </div>

  <DialogFooter
    onCancel={cancelEditing}
    onConfirm={updateConfigurationSet}
    confirmLabel={t('common.saveChanges')}
    showKeyboardHint={true}
    confirmKeyboardHint={submitHint}
  />
</Modal>

<!-- Migration Assistant -->
<MigrationAssistant
  configurationSet={migrationConfigSet}
  isVisible={showMigrationAssistant}
  onclose={handleMigrationAssistantClose}
/>

<style>
  .workspace-option:hover {
    background-color: var(--ds-surface-hovered);
  }
</style>