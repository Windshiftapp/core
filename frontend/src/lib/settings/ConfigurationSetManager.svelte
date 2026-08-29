<script>
  import { onMount } from 'svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast, successToast } from '../stores/toasts.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import { api } from '../api.js';
  import { navigate } from '../router.js';
  import { formatDateSimple } from '../utils/dateFormatter.js';
  import {
    Plus, Edit, Trash2, Settings, Workflow,
    FileText, Search, AlertCircle, Upload, Layers, Copy, Download
  } from '@lucide/svelte';
  import ItemTypeIcon from '../components/ItemTypeIcon.svelte';
  import Button from '../components/Button.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import Panel from '../components/Panel.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import MigrationAssistant from '../pages/MigrationAssistant.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import Pagination from '../components/Pagination.svelte';
  import Textarea from '../components/Textarea.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import BasePicker from '../pickers/BasePicker.svelte';
  import Label from '../components/Label.svelte';
  import Checkbox from '../components/Checkbox.svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';
  import Input from '../components/Input.svelte';
  import FileInput from '../components/FileInput.svelte';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';
  import DescriptionText from '../components/DescriptionText.svelte';
  import { builtinLocaleKey } from '../utils/systemLabels.js';

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

  // Import / unresolved-references modal state. unresolvedRefs is the
  // structured list returned by the backend on a 422; null hides the modal.
  let importFileInput = $state(null);
  let importing = $state(false);
  let unresolvedRefs = $state(null);
  let unresolvedHeading = $state('');

  // Migration assistant state
  let showMigrationAssistant = $state(false);
  let migrationConfigSet = $state(null);
  // Pre-supplied analysis from a 409 (intra-set workflow change) — when set,
  // the assistant skips its own analyze call and uses this directly.
  let migrationPreloadedAnalysis = $state(null);
  // When set, the assistant will request that the server atomically update
  // the target configuration set's workflow_id inside the migration tx.
  let migrationApplyWorkflowId = $state(null);
  // The PUT payload that triggered the 409, replayed after the migration
  // completes so the rest of the configuration-set fields (name, screens,
  // assignments, etc.) get persisted.
  let pendingWorkflowChangePayload = $state(null);
  let pendingWorkflowChangeId = $state(null);

  function getConfigurationSetDisplayValue(configSet, field) {
    return configSet?.builtin_key
      ? t(`settings.configSets.defaults.configuration.${field}`)
      : (configSet?.[field] || '');
  }

  function getItemTypeDisplayName(itemType) {
    const key = builtinLocaleKey(itemType);
    return key ? t(`settings.itemTypes.defaults.${key}`) : itemType?.name;
  }

  function getScreenDisplayName(screenId, fallbackName) {
    const screen = screens.find(candidate => candidate.id === screenId) ||
      screens.find(candidate => candidate.name === fallbackName);
    return screen?.builtin_key
      ? t('screensPage.defaults.default.name')
      : (fallbackName || screen?.name || t('settings.configSets.none'));
  }

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
        errorToast(t('dialogs.alerts.nameRequired'));
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
      errorToast(t('dialogs.alerts.failedToCreate', { error: error.message || error }));
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
        errorToast(t('dialogs.alerts.nameRequired'));
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

      let updated;
      try {
        updated = await api.configurationSets.update(editingId, payload);
      } catch (error) {
        // The server returns 409 with { error: 'migration_required', analysis }
        // when an intra-set workflow change would orphan items. Replay the
        // PUT after the user completes the migration assistant.
        if (isMigrationRequired(error)) {
          pendingWorkflowChangeId = editingId;
          pendingWorkflowChangePayload = payload;
          migrationConfigSet = originalConfigSet;
          migrationPreloadedAnalysis = error.body.analysis;
          migrationApplyWorkflowId = newWorkflowId;
          showMigrationAssistant = true;
          return;
        }
        throw error;
      }

      // If the API doesn't return the updated item, reload the data
      if (updated && updated.id) {
        configurationSets = configurationSets.map(cs =>
          cs.id === editingId ? updated : cs
        );
      } else {
        // Reload all data if update response is incomplete
        await loadData();
      }

      cancelEditing();
    } catch (error) {
      console.error('Failed to update configuration set:', error);
      errorToast(t('dialogs.alerts.failedToUpdate', { error: error.message || error }));
    }
  }

  // Recognize the server's 409 migration-required response. fetchAPI is
  // expected to have parsed the JSON body onto error.body — any error that
  // doesn't match this shape is treated as a real error.
  function isMigrationRequired(error) {
    return error && error.status === 409 && error.body && error.body.error === 'migration_required';
  }

  async function deleteConfigurationSet(configSet) {
    if (!configSet) {
      console.error('deleteConfigurationSet called with null/undefined configuration set');
      return;
    }

    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('dialogs.confirmations.deleteItem', { name: getConfigurationSetDisplayValue(configSet, 'name') }),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger'
    });
    if (!confirmed) return;

    try {
      await api.configurationSets.delete(configSet.id);
      configurationSets = configurationSets.filter(cs => cs.id !== configSet.id);
    } catch (error) {
      console.error('Failed to delete configuration set:', error);
      errorToast(t('dialogs.alerts.failedToDelete', { error: error.message || error }));
    }
  }

  async function handleMigrationAssistantClose(data) {
    const { success, cancelled } = data || {};
    showMigrationAssistant = false;
    migrationConfigSet = null;
    migrationPreloadedAnalysis = null;
    migrationApplyWorkflowId = null;

    if (cancelled || !success) {
      pendingWorkflowChangePayload = null;
      pendingWorkflowChangeId = null;
      return;
    }

    // After a successful intra-set workflow migration, the server has already
    // applied the new workflow_id. Reload data so the FE reflects the change.
    // If a PUT was pending (other fields the user changed in the same edit),
    // replay it now — the migration check will pass because items match.
    if (pendingWorkflowChangePayload && pendingWorkflowChangeId) {
      const id = pendingWorkflowChangeId;
      const payload = pendingWorkflowChangePayload;
      pendingWorkflowChangeId = null;
      pendingWorkflowChangePayload = null;
      try {
        await api.configurationSets.update(id, payload);
      } catch (error) {
        console.error('Failed to apply pending update after migration:', error);
        errorToast(t('dialogs.alerts.failedToUpdate', { error: error.message || error }));
      }
    }
    await loadData(currentPage, itemsPerPage, searchQuery);
    cancelEditing();
  }

  function showMigrationAssistantForConfigSet(configSet) {
    migrationConfigSet = configSet;
    showMigrationAssistant = true;
  }

  function getWorkspaceName(workspaceId) {
    const workspace = workspaces.find(w => w.id === workspaceId);
    return workspace ? workspace.name : t('common.unknown');
  }

  function getWorkflowName(workflowId) {
    if (!workflowId) return t('settings.configSets.none');
    const workflow = workflows.find(w => w.id === workflowId);
    if (!workflow) return t('common.unknown');
    return workflow.builtin_key ? t('workflows.defaults.default.name') : workflow.name;
  }

  function getNotificationSettingName(notificationSettingId) {
    if (!notificationSettingId) return t('settings.configSets.none');
    const setting = notificationSettings.find(s => s.id === notificationSettingId);
    if (!setting) return t('common.unknown');
    return setting.builtin_key ? t('settings.configSets.defaults.notifications.name') : setting.name;
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

  // ---- Export / Import ---------------------------------------------------

  function exportConfigurationSet(configSet) {
    if (!configSet || !configSet.id) return;
    // Browser-native download: hits the GET endpoint with the session cookie
    // and streams the JSON to a file. No JS-side parsing required.
    const a = document.createElement('a');
    a.href = api.configurationSets.exportUrl(configSet.id);
    a.download = '';
    document.body.appendChild(a);
    a.click();
    a.remove();
  }

  function pickImportFile() {
    if (importing) return;
    if (importFileInput) importFileInput.click();
  }

  async function handleImportFileChange(event) {
    const file = event.target.files && event.target.files[0];
    // Reset the input so the same file can be re-selected after a failure.
    if (event.target) event.target.value = '';
    if (!file) return;
    importing = true;
    try {
      const result = await api.configurationSets.import(file);
      // Backend may return either the bare configuration set, or
      // { data, warnings } when warnings were emitted.
      const created = result && result.data ? result.data : result;
      const warnings = (result && result.warnings) || [];
      if (created && created.id) {
        successToast(t('settings.configSets.importSuccess', { name: created.name }));
      }
      for (const warning of warnings) {
        errorToast(warning);
      }
      await loadData(currentPage, itemsPerPage, searchQuery);
    } catch (err) {
      if (err && err.status === 422 && err.code === 'unresolved_references') {
        unresolvedHeading = t('settings.configSets.unresolvedReferencesHeading');
        unresolvedRefs = (err.details && err.details.unresolved) || [];
      } else if (err && err.status === 409 && err.code === 'default_entity_conflict') {
        // Same modal handles both cases — the items share a {kind, name}
        // shape; default-conflict entries just lack the `at` breadcrumb.
        unresolvedHeading = t('settings.configSets.defaultConflictHeading');
        unresolvedRefs = (err.details && err.details.conflicts) || [];
      } else {
        errorToast(t('settings.configSets.importFailed'));
      }
    } finally {
      importing = false;
    }
  }

  function dismissUnresolved() {
    unresolvedRefs = null;
    unresolvedHeading = '';
  }

  function unresolvedLabel(ref) {
    const kindKey = ['user', 'role', 'group', 'status_category'].includes(ref.kind) ? ref.kind : 'entity';
    return t(`settings.configSets.unresolvedKinds.${kindKey}`, { name: ref.email || ref.name || '' });
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
  <Button variant="default" icon={Upload} onclick={pickImportFile} disabled={importing}>
    {importing ? t('settings.configSets.importing') : t('settings.configSets.import')}
  </Button>
  <Button variant="primary" icon={Plus} onclick={startCreating} keyboardHint="A" hotkeyConfig={{ key: toHotkeyString('configurationSets', 'add'), guard: () => !creating }}>
    {t('settings.configSets.addConfigSet')}
  </Button>
{/snippet}

<FileInput
  accept="application/json,.json"
  bind:inputRef={importFileInput}
  onchange={handleImportFileChange}
  style="display: none;"
/>

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
    <Input
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
    <Panel padding="spacious" class="text-center">
      <div class="animate-pulse" style="color: var(--ds-text-subtle);">{t('settings.configSets.loading')}</div>
    </Panel>
  {:else}
    <!-- Create Form -->
    <Modal isOpen={creating} onclose={cancelCreating} maxWidth="max-w-2xl" onSubmit={createConfigurationSet}>
      {#snippet children(submitHint)}
      <ModalHeader title={t('settings.configSets.createConfigSet')} showCloseButton={false} />

      <!-- Modal content -->
      <div class="px-6 py-4">
        <form onsubmit={(e) => { e.preventDefault(); createConfigurationSet(); }}>
          <div class="space-y-4">
            <div>
              <Label color="default" required class="mb-2">{t('settings.configSets.name')}</Label>
              <Input
                type="text"
                bind:value={newConfigSet.name}
                placeholder={t('settings.configSets.namePlaceholder')}
                size="small"
              />
            </div>

            <div>
              <Label color="default" class="mb-3">{t('settings.configSets.workspaces')}</Label>
              <div class="space-y-2 max-h-48 overflow-y-auto border rounded p-3" style="border-color: var(--ds-border);">
                {#each workspaces as workspace}
                  <div class="p-2 rounded workspace-option">
                    <Checkbox
                      checked={isWorkspaceSelected(workspace.id, false)}
                      onchange={() => toggleWorkspaceSelection(workspace.id, false)}
                      label={workspace.name}
                      size="small"
                    />
                  </div>
                {/each}
                {#if workspaces.length === 0}
                  <p class="text-sm italic" style="color: var(--ds-text-subtle);">{t('settings.configSets.noWorkspacesAvailable')}</p>
                {/if}
              </div>
              {#if newConfigSet.workspace_ids && newConfigSet.workspace_ids.length > 0}
                <DescriptionText>
                  {newConfigSet.workspace_ids.length} workspace{newConfigSet.workspace_ids.length === 1 ? '' : 's'} selected
                </DescriptionText>
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

            <Checkbox
              bind:checked={newConfigSet.is_default}
              label={t('settings.configSets.setAsDefault')}
              size="small"
            />
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
      {/snippet}
    </Modal>

    <!-- Configuration Sets List -->
    {#if configurationSets.filter(cs => cs && cs.id && cs.name !== 'Personal Tasks Configuration').length === 0}
      <Panel padding="spacious">
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
      </Panel>
    {:else}
      <div class="space-y-3">
        {#each (configurationSets || []).filter(cs => cs && cs.id && cs.name !== 'Personal Tasks Configuration') as configSet (configSet.id)}
            <Panel padding="spacious" hoverable>
              <!-- Display Mode -->
              <div class="flex items-center justify-between">
                <div class="flex-1">
                  <div class="flex items-center gap-3 mb-2">
                    <h3 class="text-lg font-medium" style="color: var(--ds-text);">{getConfigurationSetDisplayValue(configSet, 'name')}</h3>
                    {#if configSet.is_default}
                      <Lozenge color="blue" text={t('common.default')} />
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
                              <ItemTypeIcon icon={itemType.icon} color={itemType.color} />
                              {getItemTypeDisplayName(itemType)}
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
                          <span class="text-sm font-medium" style="color: var(--ds-text);">{getWorkflowName(configSet.workflow_id)}</span>
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
                          <span class="text-sm font-medium" style="color: var(--ds-text);">{getNotificationSettingName(configSet.notification_setting_id)}</span>
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
                          <span class="ml-1 font-medium" style="color: var(--ds-text);">{getScreenDisplayName(configSet.create_screen_id, configSet.create_screen_name)}</span>
                        </div>
                        <div>
                          <span class="text-xs" style="color: var(--ds-text-subtle);">{t('settings.configSets.editScreen')}</span>
                          <span class="ml-1 font-medium" style="color: var(--ds-text);">{getScreenDisplayName(configSet.edit_screen_id, configSet.edit_screen_name)}</span>
                        </div>
                        <div>
                          <span class="text-xs" style="color: var(--ds-text-subtle);">{t('settings.configSets.viewScreen')}</span>
                          <span class="ml-1 font-medium" style="color: var(--ds-text);">{getScreenDisplayName(configSet.view_screen_id, configSet.view_screen_name)}</span>
                        </div>
                      </div>
                    </div>
                  </div>

                  <!-- Footer with metadata -->
                  <div class="mt-5 pt-4 border-t" style="border-color: var(--ds-border);">
                    <span class="text-xs" style="color: var(--ds-text-subtle);">{t('settings.configSets.created')} {formatDateSimple(configSet.created_at)}</span>
                  </div>

                  {#if getConfigurationSetDisplayValue(configSet, 'description')}
                    <p class="text-sm mt-2" style="color: var(--ds-text-subtle);">{getConfigurationSetDisplayValue(configSet, 'description')}</p>
                  {/if}
                </div>

                <div class="flex items-center gap-2 ml-4">
                  <Button
                    variant="default"
                    size="small"
                    icon={Download}
                    disabled={configSet.is_default}
                    title={configSet.is_default ? t('settings.configSets.exportDefaultDisabled') : t('settings.configSets.exportTitle')}
                    onclick={() => exportConfigurationSet(configSet)}
                  >
                    {t('settings.configSets.export')}
                  </Button>
                  <Button
                    variant="default"
                    size="small"
                    icon={Edit}
                    onclick={() => startEditing(configSet)}
                  >
                    {t('common.edit')}
                  </Button>
                  <Button
                    variant="danger-ghost"
                    size="small"
                    icon={Trash2}
                    onclick={() => deleteConfigurationSet(configSet)}
                  >
                    {t('common.delete')}
                  </Button>
                </div>
              </div>
            </Panel>
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
<Modal isOpen={showEditModal} onclose={cancelEditing} maxWidth="max-w-2xl" onSubmit={updateConfigurationSet}>
  {#snippet children(submitHint)}
  <ModalHeader title={t('settings.configSets.editConfigSet')} showCloseButton={false} />

  <!-- Modal content -->
  <div class="px-6 py-4">
    <form onsubmit={(e) => { e.preventDefault(); updateConfigurationSet(); }}>
      <div class="space-y-4">
        <div>
          <Label color="default" required class="mb-2">{t('settings.configSets.name')}</Label>
          <Input
            type="text"
            bind:value={editConfigSet.name}
            size="small"
          />
        </div>

        <div>
          <Label color="default" class="mb-3">{t('settings.configSets.workspaces')}</Label>
          <div class="space-y-2 max-h-48 overflow-y-auto border rounded p-3" style="border-color: var(--ds-border);">
            {#each workspaces as workspace}
              <div class="p-2 rounded workspace-option">
                <Checkbox
                  checked={isWorkspaceSelected(workspace.id, true)}
                  onchange={() => toggleWorkspaceSelection(workspace.id, true)}
                  label={workspace.name}
                  size="small"
                />
              </div>
            {/each}
            {#if workspaces.length === 0}
              <p class="text-sm italic" style="color: var(--ds-text-subtle);">{t('settings.configSets.noWorkspacesAvailable')}</p>
            {/if}
          </div>
          {#if editConfigSet.workspace_ids && editConfigSet.workspace_ids.length > 0}
            <DescriptionText>
              {editConfigSet.workspace_ids.length} workspace{editConfigSet.workspace_ids.length === 1 ? '' : 's'} selected
            </DescriptionText>
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

        <Checkbox
          bind:checked={editConfigSet.is_default}
          label={t('settings.configSets.setAsDefault')}
          size="small"
        />
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
  {/snippet}
</Modal>

<!-- Unresolved References Modal — surfaces 422 from /configuration-sets/import.
     Lists every role/group/user/status_category the bundle expected and the
     target instance does not have, so the operator can fix the source bundle
     or provision the missing identities before retrying. No write happened. -->
<Modal isOpen={!!unresolvedRefs} onclose={dismissUnresolved} maxWidth="max-w-xl">
  {#snippet children()}
  <ModalHeader title={t('settings.configSets.unresolvedImportTitle')} showCloseButton={true} onclose={dismissUnresolved} />
  <div class="px-6 py-4">
    <p class="text-sm mb-3" style="color: var(--ds-text);">{unresolvedHeading}</p>
    <p class="text-xs mb-3" style="color: var(--ds-text-subtle);">
      {t('settings.configSets.unresolvedImportDescription')}
    </p>
    <ul class="space-y-1 text-sm" style="color: var(--ds-text);">
      {#each (unresolvedRefs || []) as ref ((ref.at || '') + ref.kind + (ref.name || ref.email || ''))}
        <li class="flex items-start gap-2 border rounded p-2" style="border-color: var(--ds-border);">
          <Lozenge color="red" text={ref.kind} size="sm" />
          <div class="flex-1">
            <div class="font-medium">{unresolvedLabel(ref)}</div>
            {#if ref.at}
              <div class="text-xs" style="color: var(--ds-text-subtle);">{t('settings.configSets.referenceLocation', { path: ref.at })}</div>
            {/if}
          </div>
        </li>
      {/each}
    </ul>
  </div>
  <DialogFooter
    onCancel={dismissUnresolved}
    cancelLabel={t('common.close')}
  />
  {/snippet}
</Modal>

<!-- Migration Assistant -->
<MigrationAssistant
  configurationSet={migrationConfigSet}
  targetConfigurationSet={migrationConfigSet}
  isVisible={showMigrationAssistant}
  workspaceId={migrationPreloadedAnalysis?.affected_workspaces?.[0] ?? null}
  comprehensive={!!migrationPreloadedAnalysis}
  preloadedAnalysis={migrationPreloadedAnalysis}
  applyWorkflowId={migrationApplyWorkflowId}
  onclose={handleMigrationAssistantClose}
/>

<style>
  .workspace-option:hover {
    background-color: var(--ds-background-neutral-hovered);
  }
</style>
