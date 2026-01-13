<script>
  import { onMount } from 'svelte';
  import { currentRoute, navigate } from '../router.js';
  import { api } from '../api.js';
  import { ArrowLeft } from 'lucide-svelte';
  import Button from '../components/Button.svelte';
  import Textarea from '../components/Textarea.svelte';
  import Tabs from '../components/Tabs.svelte';
  import ConfigurationSetWorkspaces from './ConfigurationSetWorkspaces.svelte';
  import ConfigurationSetItemTypes from './ConfigurationSetItemTypes.svelte';
  import ConfigurationSetPriorities from './ConfigurationSetPriorities.svelte';
  import ScreenPicker from '../pickers/ScreenPicker.svelte';
  import WorkflowPicker from '../pickers/WorkflowPicker.svelte';
  import Toggle from '../components/Toggle.svelte';
  import BasePicker from '../pickers/BasePicker.svelte';
  import Label from '../components/Label.svelte';

  // Tab configuration
  let activeTab = 'general';

  // Tabs are dynamic - default config set doesn't show workspaces tab
  // (it automatically includes all unassigned workspaces)
  $: tabs = formData.is_default
    ? [
        { id: 'general', label: 'General' },
        { id: 'priorities', label: 'Priorities' },
        { id: 'item-types', label: 'Item Types' }
      ]
    : [
        { id: 'general', label: 'General' },
        { id: 'priorities', label: 'Priorities' },
        { id: 'item-types', label: 'Item Types' },
        { id: 'workspaces', label: 'Workspaces' }
      ];

  let configSetId = null;
  let isNewMode = false;
  let configSet = null;
  let loading = true;
  let saving = false;

  // Reference data
  let workflows = [];
  let screens = [];
  let notificationSettings = [];
  let workspaces = [];
  let itemTypes = [];
  let priorities = [];

  // Form state
  let formData = {
    name: '',
    description: '',
    is_default: false,
    differentiate_by_item_type: false,
    workflow_id: null,
    notification_setting_id: null,
    create_screen_id: null,
    edit_screen_id: null,
    view_screen_id: null,
    default_item_type_id: null,
    workspace_ids: [],
    priority_ids: [],
    item_type_configs: []
  };

  // Original form data for change tracking
  let originalFormData = {};

  // Reactive: Check if form has unsaved changes
  $: hasUnsavedChanges = JSON.stringify(formData) !== JSON.stringify(originalFormData);

  // If user toggles is_default while on workspaces tab, switch to general
  $: if (formData.is_default && activeTab === 'workspaces') {
    activeTab = 'general';
  }

  // Subscribe to route changes
  $: {
    const id = $currentRoute.params?.id;
    if (id === 'new') {
      isNewMode = true;
      configSetId = null;
      resetForm();
      loading = false;
    } else if (id) {
      const newId = parseInt(id);
      if (newId && newId !== configSetId) {
        isNewMode = false;
        configSetId = newId;
        loadData();
      }
    }
  }

  onMount(() => {
    loadReferenceData();
  });

  function resetForm() {
    formData = {
      name: '',
      description: '',
      default_item_type_id: null,
      is_default: false,
      differentiate_by_item_type: false,
      workflow_id: null,
      notification_setting_id: null,
      create_screen_id: null,
      edit_screen_id: null,
      view_screen_id: null,
      workspace_ids: [],
      priority_ids: [],
      item_type_configs: []
    };
    originalFormData = { ...formData };
  }

  async function loadReferenceData() {
    try {
      const [workflowsData, screensData, notifData, workspacesData, itemTypesData, prioritiesData] = await Promise.all([
        api.workflows.getAll(),
        api.screens.getAll(),
        api.notificationSettings.getAll(),
        api.workspaces.getAll(),
        api.itemTypes.getAll(),
        api.priorities.getAll()
      ]);
      workflows = workflowsData || [];
      screens = screensData || [];
      notificationSettings = notifData || [];
      workspaces = (workspacesData || []).filter(w => !w.is_personal);
      itemTypes = itemTypesData || [];
      priorities = prioritiesData || [];
    } catch (error) {
      console.error('Failed to load reference data:', error);
    }
  }

  async function loadData() {
    if (!configSetId) {
      loading = false;
      return;
    }

    try {
      loading = true;
      const data = await api.configurationSets.get(configSetId);
      configSet = data;

      formData = {
        name: data.name || '',
        description: data.description || '',
        is_default: data.is_default || false,
        differentiate_by_item_type: data.differentiate_by_item_type || false,
        workflow_id: data.workflow_id || null,
        notification_setting_id: data.notification_setting_id || null,
        create_screen_id: data.create_screen_id || null,
        edit_screen_id: data.edit_screen_id || null,
        view_screen_id: data.view_screen_id || null,
        default_item_type_id: data.default_item_type_id || null,
        workspace_ids: data.workspace_ids || [],
        priority_ids: data.priority_ids || [],
        item_type_configs: data.item_type_configs || []
      };

      originalFormData = JSON.parse(JSON.stringify(formData));
    } catch (error) {
      console.error('Failed to load configuration set:', error);
      alert('Failed to load configuration set: ' + (error.message || JSON.stringify(error)));
    } finally {
      loading = false;
    }
  }

  async function save() {
    if (!formData.name.trim()) {
      alert('Name is required');
      return;
    }

    try {
      saving = true;

      const payload = {
        name: formData.name,
        description: formData.description,
        is_default: formData.is_default,
        differentiate_by_item_type: formData.differentiate_by_item_type,
        workflow_id: formData.workflow_id,
        notification_setting_id: formData.notification_setting_id,
        create_screen_id: formData.create_screen_id,
        edit_screen_id: formData.edit_screen_id,
        view_screen_id: formData.view_screen_id,
        default_item_type_id: formData.default_item_type_id,
        workspace_ids: formData.workspace_ids,
        priority_ids: formData.priority_ids,
        item_type_configs: formData.item_type_configs
      };

      if (isNewMode) {
        const created = await api.configurationSets.create(payload);
        configSet = created;
        configSetId = created.id;
        isNewMode = false;
        navigate(`/admin/configuration-sets/${created.id}`);
      } else {
        const updated = await api.configurationSets.update(configSetId, payload);
        configSet = updated;
      }

      originalFormData = JSON.parse(JSON.stringify(formData));
    } catch (error) {
      console.error('Failed to save configuration set:', error);
      alert('Failed to save: ' + (error.message || JSON.stringify(error)));
    } finally {
      saving = false;
    }
  }

  function goBack() {
    navigate('/admin/configuration-sets');
  }

  function handleWorkspacesChange(event) {
    formData.workspace_ids = event.detail;
  }

  function handleItemTypeConfigsChange(event) {
    formData.item_type_configs = event.detail;
  }

  function handlePrioritiesChange(event) {
    formData.priority_ids = event.detail;
  }
</script>

<div class="flex flex-col h-full" style="background-color: var(--ds-surface);">
    <!-- Header -->
    <div class="border-b px-6 py-4" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-4">
          <button
            onclick={goBack}
            class="transition-colors"
            style="color: var(--ds-text-subtle);"
            title="Back to Configuration Sets"
          >
            <ArrowLeft class="w-5 h-5" />
          </button>
          <div>
            <h1 class="text-xl font-semibold" style="color: var(--ds-text);">
              {#if loading}
                Loading...
              {:else if isNewMode}
                New Configuration Set
              {:else}
                {configSet?.name || 'Configuration Set'}
              {/if}
            </h1>
            <p class="text-sm mt-0.5" style="color: var(--ds-text-subtle);">
              Configure workflows, screens, and workspace assignments
            </p>
          </div>
        </div>
        <div class="flex items-center gap-3">
          {#if hasUnsavedChanges}
            <span class="text-sm" style="color: var(--ds-text-subtle);">Unsaved changes</span>
          {/if}
          <Button variant="ghost" onclick={goBack}>
            Cancel
          </Button>
          <Button variant="primary" onclick={save} disabled={saving || !hasUnsavedChanges}>
            {saving ? 'Saving...' : 'Save'}
          </Button>
        </div>
      </div>
    </div>

    <!-- Content -->
    <div class="flex-1 overflow-y-auto p-6" style="background-color: var(--ds-surface);">
      {#if loading}
        <div class="flex items-center justify-center h-64">
          <div style="color: var(--ds-text-subtle);">Loading configuration set...</div>
        </div>
      {:else}
        <div class="max-w-6xl mx-auto">
          <Tabs {tabs} bind:activeTab>
            {#if activeTab === 'general'}
              <!-- Basic Information -->
              <div class="space-y-6">
                <div>
                  <h3 class="text-base font-medium mb-4" style="color: var(--ds-text);">Basic Information</h3>
                  <div class="space-y-4">
                    <div>
                      <Label color="default" required class="mb-1">Name</Label>
                      <input
                        type="text"
                        bind:value={formData.name}
                        class="w-full px-3 py-2 border rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
                        style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text);"
                        placeholder="e.g., Development Config"
                      />
                    </div>

                    <div>
                      <Label color="default" class="mb-1">Description</Label>
                      <Textarea
                        bind:value={formData.description}
                        rows={3}
                        placeholder="Optional description of this configuration set"
                      />
                    </div>

                    <div>
                      <label class="flex items-center gap-2 cursor-pointer">
                        <input
                          type="checkbox"
                          bind:checked={formData.is_default}
                          class="w-4 h-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
                        />
                        <span class="text-sm" style="color: var(--ds-text);">Set as default configuration set</span>
                      </label>
                    </div>
                  </div>
                </div>

                <!-- Default Settings -->
                <div class="border-t pt-6" style="border-color: var(--ds-border);">
                  <h3 class="text-base font-medium mb-4" style="color: var(--ds-text);">Default Settings</h3>
                  <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <div>
                      <Label color="default" class="mb-1">Workflow</Label>
                      <WorkflowPicker
                        value={formData.workflow_id}
                        items={workflows}
                        unassignedLabel="No workflow"
                        placeholder="Select workflow..."
                        onSelect={(workflow) => formData.workflow_id = workflow?.id || null}
                      />
                    </div>

                    <div>
                      <Label color="default" class="mb-1">Notification Settings</Label>
                      <BasePicker
                        bind:value={formData.notification_setting_id}
                        items={notificationSettings}
                        placeholder="Select notification settings..."
                        showUnassigned={true}
                        unassignedLabel="No notification settings"
                        getValue={(item) => item.id}
                        getLabel={(item) => item.name}
                      />
                    </div>

                    <div>
                      <Label color="default" class="mb-1">Create Screen</Label>
                      <ScreenPicker
                        value={formData.create_screen_id}
                        items={screens}
                        unassignedLabel="No screen"
                        placeholder="Select screen..."
                        onSelect={(screen) => formData.create_screen_id = screen?.id || null}
                      />
                    </div>

                    <div>
                      <Label color="default" class="mb-1">Edit Screen</Label>
                      <ScreenPicker
                        value={formData.edit_screen_id}
                        items={screens}
                        unassignedLabel="No screen"
                        placeholder="Select screen..."
                        onSelect={(screen) => formData.edit_screen_id = screen?.id || null}
                      />
                    </div>

                    <div>
                      <Label color="default" class="mb-1">View Screen</Label>
                      <ScreenPicker
                        value={formData.view_screen_id}
                        items={screens}
                        unassignedLabel="No screen"
                        placeholder="Select screen..."
                        onSelect={(screen) => formData.view_screen_id = screen?.id || null}
                      />
                    </div>

                    <div>
                      <Label color="default" class="mb-1">Default Item Type</Label>
                      <BasePicker
                        bind:value={formData.default_item_type_id}
                        items={itemTypes.filter(t => formData.item_type_configs.some(c => c.item_type_id === t.id))}
                        placeholder="Select default item type..."
                        showUnassigned={true}
                        unassignedLabel="First available"
                        getValue={(item) => item.id}
                        getLabel={(item) => item.name}
                      />
                      <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
                        Pre-selected item type when creating new items (if user has no preference)
                      </p>
                    </div>
                  </div>
                </div>
              </div>

            {:else if activeTab === 'priorities'}
              <!-- Priorities -->
              <ConfigurationSetPriorities
                {priorities}
                selectedPriorityIds={formData.priority_ids}
                configurationSetId={configSetId}
                onchange={handlePrioritiesChange}
              />

            {:else if activeTab === 'item-types'}
              <!-- Item Types -->
              <div>
                <ConfigurationSetItemTypes
                  {itemTypes}
                  {workflows}
                  {screens}
                  itemTypeConfigs={formData.item_type_configs}
                  defaultWorkflowId={formData.workflow_id}
                  defaultCreateScreenId={formData.create_screen_id}
                  defaultEditScreenId={formData.edit_screen_id}
                  defaultViewScreenId={formData.view_screen_id}
                  showOverrides={formData.differentiate_by_item_type}
                  onchange={handleItemTypeConfigsChange}
                />

                <div class="flex items-center justify-between mt-6 pt-4 border-t" style="border-color: var(--ds-border);">
                  <div>
                    <p class="text-sm font-medium" style="color: var(--ds-text);">
                      Configure per item type
                    </p>
                    <p class="text-sm" style="color: var(--ds-text-subtle);">
                      {#if formData.differentiate_by_item_type}
                        Configure different workflows and screens for each item type above.
                      {:else}
                        All item types use the default workflow and screens from the General tab.
                      {/if}
                    </p>
                  </div>
                  <Toggle bind:checked={formData.differentiate_by_item_type} />
                </div>
              </div>

            {:else if activeTab === 'workspaces'}
              <!-- Workspaces -->
              <ConfigurationSetWorkspaces
                allWorkspaces={workspaces}
                selectedWorkspaceIds={formData.workspace_ids}
                configurationSetId={configSetId}
                onchange={handleWorkspacesChange}
              />
            {/if}
          </Tabs>
        </div>
      {/if}
    </div>
</div>
