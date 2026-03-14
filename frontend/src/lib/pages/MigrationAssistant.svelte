<script>
  import { api } from '../api.js';
  import Spinner from '../components/Spinner.svelte';
  import Button from '../components/Button.svelte';
  import BasePicker from '../pickers/BasePicker.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import { X, ArrowRight, Type, FileText, Flag, Activity } from 'lucide-svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let {
    configurationSet = null,
    targetConfigurationSet = null,
    isVisible = $bindable(false),
    workspaceId = null,
    comprehensive = false,
    onclose = null
  } = $props();

  let migrationAnalysis = $state(null);
  let isAnalyzing = $state(false);
  let isMigrating = $state(false);
  let analysisError = $state(null);
  let migrationError = $state(null);
  let migrationSuccess = $state(false);

  // Mappings for each dimension
  let statusMappings = $state([]);
  let itemTypeMappings = $state([]);
  let customFieldMappings = $state([]);
  let priorityMappings = $state([]);

  // Tab state
  let activeTab = $state('status');

  $effect(() => {
    if ((configurationSet || targetConfigurationSet) && isVisible) {
      analyzeMigration();
    }
  });

  // Compute tab counts
  let statusCount = $derived(statusMappings.filter(m => m.requires_migration).length);
  let itemTypeCount = $derived(itemTypeMappings.filter(m => m.requires_migration).length);
  let fieldCount = $derived(customFieldMappings.filter(m => m.requires_default).length);
  let priorityCount = $derived(priorityMappings.filter(m => m.requires_migration).length);

  async function analyzeMigration() {
    if (!workspaceId) return;
    if (!comprehensive && !configurationSet) return;
    if (comprehensive && !targetConfigurationSet) return;

    isAnalyzing = true;
    analysisError = null;
    migrationAnalysis = null;
    activeTab = 'status';

    try {
      let response;
      if (comprehensive) {
        response = await api.configurationSets.analyzeComprehensiveMigration(
          targetConfigurationSet.id,
          workspaceId
        );
      } else {
        response = await api.configurationSets.analyzeMigration(configurationSet.id, workspaceId);
      }
      migrationAnalysis = response;

      // Initialize status mappings
      statusMappings = (migrationAnalysis.status_migrations || []).map(migration => ({
        from_status: migration.current_status,
        from_status_id: migration.current_status_id,
        item_type_id: migration.item_type_id || null,
        item_type_name: migration.item_type_name || null,
        to_status_id: migration.suggested_status_id || null,
        item_count: migration.item_count,
        requires_migration: migration.requires_migration
      }));

      // Initialize item type mappings (comprehensive only)
      if (comprehensive) {
        itemTypeMappings = (migrationAnalysis.item_type_migrations || []).map(migration => ({
          from_item_type_id: migration.current_item_type_id,
          from_item_type_name: migration.current_item_type_name,
          to_item_type_id: migration.suggested_item_type_id || null,
          to_item_type_name: migration.suggested_item_type_name || '',
          item_count: migration.item_count,
          requires_migration: migration.requires_migration,
          available_targets: migration.available_targets || migrationAnalysis.available_item_types || []
        }));

        customFieldMappings = (migrationAnalysis.custom_field_migrations || []).map(migration => ({
          field_id: migration.field_id,
          field_name: migration.field_name,
          field_type: migration.field_type,
          item_count: migration.item_count,
          action: migration.action,
          requires_default: migration.requires_default,
          default_value: null
        }));

        priorityMappings = (migrationAnalysis.priority_migrations || []).map(migration => ({
          from_priority_id: migration.current_priority_id,
          from_priority_name: migration.current_priority_name,
          to_priority_id: migration.suggested_priority_id || null,
          to_priority_name: migration.suggested_priority_name || '',
          item_count: migration.item_count,
          requires_migration: migration.requires_migration,
          available_targets: migrationAnalysis.available_priorities || []
        }));

        // Set active tab to first one with required migrations
        if (itemTypeCount > 0) activeTab = 'itemType';
        else if (fieldCount > 0) activeTab = 'fields';
        else if (priorityCount > 0) activeTab = 'priority';
        else activeTab = 'status';
      }
    } catch (error) {
      console.error('Migration analysis failed:', error);
      analysisError = error.message || 'Failed to analyze migration requirements';
    } finally {
      isAnalyzing = false;
    }
  }

  async function executeMigration() {
    if (!migrationAnalysis) return;

    // Validate status mappings
    const invalidStatusMappings = statusMappings.filter(mapping =>
      mapping.requires_migration && !mapping.to_status_id
    );
    if (invalidStatusMappings.length > 0) {
      migrationError = 'Please select target statuses for all items that require migration.';
      activeTab = 'status';
      return;
    }

    if (comprehensive) {
      // Validate item type mappings
      const invalidItemTypeMappings = itemTypeMappings.filter(mapping =>
        mapping.requires_migration && !mapping.to_item_type_id
      );
      if (invalidItemTypeMappings.length > 0) {
        migrationError = 'Please select target item types for all items that require migration.';
        activeTab = 'itemType';
        return;
      }

      // Validate custom field mappings (default values for required new fields)
      const invalidFieldMappings = customFieldMappings.filter(mapping =>
        mapping.requires_default && mapping.default_value === null
      );
      if (invalidFieldMappings.length > 0) {
        migrationError = 'Please provide default values for all new required fields.';
        activeTab = 'fields';
        return;
      }

      // Validate priority mappings
      const invalidPriorityMappings = priorityMappings.filter(mapping =>
        mapping.requires_migration && !mapping.to_priority_id
      );
      if (invalidPriorityMappings.length > 0) {
        migrationError = 'Please select target priorities for all items that require migration.';
        activeTab = 'priority';
        return;
      }
    }

    isMigrating = true;
    migrationError = null;
    migrationSuccess = false;

    try {
      if (comprehensive) {
        const migrationRequest = {
          old_configuration_set_id: migrationAnalysis.old_config_set_id,
          new_configuration_set_id: migrationAnalysis.new_config_set_id,
          workspace_ids: migrationAnalysis.affected_workspaces,
          status_mappings: statusMappings
            .filter(mapping => mapping.to_status_id)
            .map(mapping => ({
              from_status: mapping.from_status,
              from_status_id: mapping.from_status_id,
              to_status_id: mapping.to_status_id,
              item_type_id: mapping.item_type_id,
              item_count: mapping.item_count
            })),
          item_type_mappings: itemTypeMappings
            .filter(mapping => mapping.requires_migration && mapping.to_item_type_id)
            .map(mapping => ({
              from_item_type_id: mapping.from_item_type_id,
              to_item_type_id: mapping.to_item_type_id
            })),
          custom_field_mappings: customFieldMappings
            .filter(mapping => mapping.requires_default || mapping.action === 'add_default')
            .map(mapping => ({
              field_id: mapping.field_id,
              action: mapping.action,
              default_value: mapping.default_value
            })),
          priority_mappings: priorityMappings
            .filter(mapping => mapping.requires_migration && mapping.to_priority_id)
            .map(mapping => ({
              from_priority_id: mapping.from_priority_id,
              to_priority_id: mapping.to_priority_id
            }))
        };

        await api.configurationSets.executeComprehensiveMigration(migrationRequest);
      } else {
        const migrationRequest = {
          configuration_set_id: configurationSet.id,
          workspace_ids: migrationAnalysis.affected_workspaces,
          status_mappings: statusMappings
            .filter(mapping => mapping.to_status_id)
            .map(mapping => ({
              from_status: mapping.from_status,
              from_status_id: mapping.from_status_id,
              to_status_id: mapping.to_status_id,
              item_type_id: mapping.item_type_id,
              item_count: mapping.item_count
            }))
        };

        await api.configurationSets.executeMigration(migrationRequest);
      }

      migrationSuccess = true;

      // Close the assistant after a brief delay
      setTimeout(() => {
        closeAssistant();
      }, 2000);

    } catch (error) {
      console.error('Migration execution failed:', error);
      migrationError = error.message || 'Failed to execute migration';
    } finally {
      isMigrating = false;
    }
  }

  function updateStatusMapping(fromStatus, itemTypeId, toStatusId) {
    statusMappings = statusMappings.map(mapping =>
      mapping.from_status === fromStatus && mapping.item_type_id === itemTypeId
        ? { ...mapping, to_status_id: toStatusId }
        : mapping
    );
  }

  function updateItemTypeMapping(fromItemTypeId, toItemTypeId, toItemTypeName) {
    itemTypeMappings = itemTypeMappings.map(mapping =>
      mapping.from_item_type_id === fromItemTypeId
        ? { ...mapping, to_item_type_id: toItemTypeId, to_item_type_name: toItemTypeName }
        : mapping
    );
  }

  function updatePriorityMapping(fromPriorityId, toPriorityId, toPriorityName) {
    priorityMappings = priorityMappings.map(mapping =>
      mapping.from_priority_id === fromPriorityId
        ? { ...mapping, to_priority_id: toPriorityId, to_priority_name: toPriorityName }
        : mapping
    );
  }

  function updateFieldDefaultValue(fieldId, value) {
    customFieldMappings = customFieldMappings.map(mapping =>
      mapping.field_id === fieldId
        ? { ...mapping, default_value: value }
        : mapping
    );
  }

  function closeAssistant(cancelled = false) {
    const wasSuccessful = migrationSuccess;

    // Clean up state
    isVisible = false;
    migrationAnalysis = null;
    statusMappings = [];
    itemTypeMappings = [];
    customFieldMappings = [];
    priorityMappings = [];
    analysisError = null;
    migrationError = null;
    migrationSuccess = false;
    activeTab = 'status';

    // Call close callback with success/cancelled information
    onclose?.({
      success: wasSuccessful && !cancelled,
      cancelled: cancelled
    });
  }

  // Get available workflow statuses for dropdowns
  let workflowStatuses = $state([]);
  $effect(() => {
    if (migrationAnalysis && migrationAnalysis.new_workflow_id) {
      loadWorkflowStatuses();
    }
  });

  async function loadWorkflowStatuses() {
    try {
      const workflow = await api.workflows.get(migrationAnalysis.new_workflow_id);
      // Extract unique statuses from workflow transitions
      const statusSet = new Set();
      workflow.transitions.forEach(transition => {
        if (transition.from_status_id && transition.from_status_name) {
          statusSet.add(JSON.stringify({
            id: transition.from_status_id,
            name: transition.from_status_name
          }));
        }
        if (transition.to_status_id && transition.to_status_name) {
          statusSet.add(JSON.stringify({
            id: transition.to_status_id,
            name: transition.to_status_name
          }));
        }
      });

      workflowStatuses = Array.from(statusSet).map(status => JSON.parse(status))
        .sort((a, b) => a.name.localeCompare(b.name));
    } catch (error) {
      console.error('Failed to load workflow statuses:', error);
    }
  }

  // Check if migration is needed
  let requiresMigration = $derived(comprehensive
    ? (migrationAnalysis?.requires_migration ?? false)
    : (migrationAnalysis?.requires_migration ?? false));

  // Check if any tab has pending migrations
  let hasPendingMigrations = $derived(statusCount > 0 || itemTypeCount > 0 || fieldCount > 0 || priorityCount > 0);
</script>

<Modal
  isOpen={isVisible}
  maxWidth="max-w-4xl"
  onclose={() => closeAssistant(true)}
>
  <div class="max-h-[90vh] overflow-hidden flex flex-col">
    <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
      <div class="flex items-center justify-between">
        <h2 class="text-xl font-semibold" style="color: var(--ds-text);">
          {comprehensive ? t('migrationAssistant.configSetMigration') : t('migrationAssistant.workflowMigration')}
        </h2>
        <Button
          variant="ghost"
          icon={X}
          onclick={() => closeAssistant(true)}
          title={t('common.close')}
        />
      </div>
      {#if comprehensive && migrationAnalysis}
        <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
          {t('migrationAssistant.migratingFrom')} <span class="font-medium">{migrationAnalysis.old_config_set_name}</span>
          {t('migrationAssistant.to')} <span class="font-medium">{migrationAnalysis.new_config_set_name}</span>
        </p>
      {:else if configurationSet}
        <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
          {t('migrationAssistant.configurationSet')}: <span class="font-medium">{configurationSet.name}</span>
        </p>
      {/if}
    </div>

    <div class="migration-assistant-content p-6 overflow-y-auto flex-1 min-h-0">
      {#if migrationError}
        <div class="mb-6">
          <AlertBox variant="error" message={migrationError} />
        </div>
      {/if}

      {#if isAnalyzing}
        <div class="flex items-center justify-center py-8">
          <Spinner />
          <span class="ml-3" style="color: var(--ds-text-subtle);">{t('migrationAssistant.analyzingMigration')}</span>
        </div>
      {:else if analysisError}
        <AlertBox variant="error" message="{t('migrationAssistant.analysisFailed')}: {analysisError}" />
      {:else if migrationAnalysis}
        {#if !requiresMigration}
          <AlertBox variant="success" message="No Migration Required - All items ({migrationAnalysis.total_affected_items}) are compatible with the new configuration." />
        {:else}
          <div class="space-y-6">
            <AlertBox variant="warning" message="Migration Required - {migrationAnalysis.total_affected_items} items need migration. Please review the mappings below." />

            {#if comprehensive}
              <!-- Tabs for each dimension -->
              <div class="flex border-b" style="border-color: var(--ds-border);">
                <button
                  class="px-4 py-2 text-sm font-medium flex items-center gap-2 border-b-2 -mb-px transition-colors"
                  class:border-blue-500={activeTab === 'itemType'}
                  class:text-blue-600={activeTab === 'itemType'}
                  class:border-transparent={activeTab !== 'itemType'}
                  style={activeTab !== 'itemType' ? 'color: var(--ds-text-subtle);' : ''}
                  onclick={() => activeTab = 'itemType'}
                >
                  <Type size={16} />
                  {t('migrationAssistant.itemTypes')}
                  {#if itemTypeCount > 0}
                    <span class="px-1.5 py-0.5 text-xs rounded-full bg-yellow-100 text-yellow-800">{itemTypeCount}</span>
                  {/if}
                </button>
                <button
                  class="px-4 py-2 text-sm font-medium flex items-center gap-2 border-b-2 -mb-px transition-colors"
                  class:border-blue-500={activeTab === 'fields'}
                  class:text-blue-600={activeTab === 'fields'}
                  class:border-transparent={activeTab !== 'fields'}
                  style={activeTab !== 'fields' ? 'color: var(--ds-text-subtle);' : ''}
                  onclick={() => activeTab = 'fields'}
                >
                  <FileText size={16} />
                  {t('migrationAssistant.fields')}
                  {#if fieldCount > 0}
                    <span class="px-1.5 py-0.5 text-xs rounded-full bg-yellow-100 text-yellow-800">{fieldCount}</span>
                  {/if}
                </button>
                <button
                  class="px-4 py-2 text-sm font-medium flex items-center gap-2 border-b-2 -mb-px transition-colors"
                  class:border-blue-500={activeTab === 'status'}
                  class:text-blue-600={activeTab === 'status'}
                  class:border-transparent={activeTab !== 'status'}
                  style={activeTab !== 'status' ? 'color: var(--ds-text-subtle);' : ''}
                  onclick={() => activeTab = 'status'}
                >
                  <Activity size={16} />
                  {t('migrationAssistant.status')}
                  {#if statusCount > 0}
                    <span class="px-1.5 py-0.5 text-xs rounded-full bg-yellow-100 text-yellow-800">{statusCount}</span>
                  {/if}
                </button>
                <button
                  class="px-4 py-2 text-sm font-medium flex items-center gap-2 border-b-2 -mb-px transition-colors"
                  class:border-blue-500={activeTab === 'priority'}
                  class:text-blue-600={activeTab === 'priority'}
                  class:border-transparent={activeTab !== 'priority'}
                  style={activeTab !== 'priority' ? 'color: var(--ds-text-subtle);' : ''}
                  onclick={() => activeTab = 'priority'}
                >
                  <Flag size={16} />
                  {t('migrationAssistant.priority')}
                  {#if priorityCount > 0}
                    <span class="px-1.5 py-0.5 text-xs rounded-full bg-yellow-100 text-yellow-800">{priorityCount}</span>
                  {/if}
                </button>
              </div>
            {/if}

            <!-- Item Type Migrations -->
            {#if activeTab === 'itemType' && comprehensive}
              <div>
                <h3 class="text-lg font-medium mb-4" style="color: var(--ds-text);">{t('migrationAssistant.itemTypeMigrations')}</h3>
                {#if itemTypeMappings.length === 0}
                  <p class="text-sm" style="color: var(--ds-text-subtle);">{t('migrationAssistant.noItemsToMigrate')}</p>
                {:else}
                  <div class="space-y-4">
                    {#each itemTypeMappings as mapping}
                      <div class="rounded p-4" style="background-color: var(--ds-surface-raised);">
                        <div class="flex items-center justify-between">
                          <div class="flex-1">
                            <div class="flex items-center space-x-3">
                              <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium" style="background-color: var(--ds-background-neutral); color: var(--ds-text);">
                                {mapping.from_item_type_name}
                              </span>
                              <ArrowRight size={16} style="color: var(--ds-text-subtle);" />
                              {#if mapping.requires_migration}
                                <div class="w-48">
                                  <BasePicker
                                    bind:value={mapping.to_item_type_id}
                                    items={mapping.available_targets}
                                    placeholder={t('migrationAssistant.selectTargetType')}
                                    showUnassigned={true}
                                    unassignedLabel={t('migrationAssistant.selectTargetType')}
                                    getValue={(type) => type.id}
                                    getLabel={(type) => type.name}
                                    onSelect={(type) => {
                                      if (type) {
                                        updateItemTypeMapping(mapping.from_item_type_id, type.id, type.name);
                                      }
                                    }}
                                  />
                                </div>
                              {:else}
                                <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-800">
                                  {mapping.to_item_type_name || t('migrationAssistant.compatible')}
                                </span>
                              {/if}
                            </div>
                            <p class="text-sm mt-2" style="color: var(--ds-text-subtle);">
                              {mapping.item_count} item{mapping.item_count !== 1 ? 's' : ''}
                              {#if mapping.requires_migration}
                                <span class="text-yellow-600 font-medium"> - {t('migrationAssistant.requiresMigration')}</span>
                              {/if}
                            </p>
                          </div>
                        </div>
                      </div>
                    {/each}
                  </div>
                {/if}
              </div>
            {/if}

            <!-- Custom Field Migrations -->
            {#if activeTab === 'fields' && comprehensive}
              <div>
                <h3 class="text-lg font-medium mb-4" style="color: var(--ds-text);">{t('migrationAssistant.customFieldChanges')}</h3>
                {#if customFieldMappings.length === 0}
                  <p class="text-sm" style="color: var(--ds-text-subtle);">No field changes detected.</p>
                {:else}
                  <div class="space-y-4">
                    {#each customFieldMappings as mapping}
                      <div class="rounded p-4" style="background-color: var(--ds-surface-raised);">
                        <div class="flex items-center justify-between">
                          <div class="flex-1">
                            <div class="flex items-center space-x-3">
                              <span class="font-medium" style="color: var(--ds-text);">{mapping.field_name}</span>
                              <span class="text-xs px-2 py-0.5 rounded" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                                {mapping.field_type}
                              </span>
                            </div>
                            <div class="mt-2">
                              {#if mapping.action === 'keep'}
                                <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-800">
                                  Kept - {mapping.item_count} values
                                </span>
                              {:else if mapping.action === 'orphan'}
                                <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-blue-100 text-blue-800">
                                  Hidden (data preserved) - {mapping.item_count} values
                                </span>
                              {:else if mapping.action === 'add_default'}
                                <div class="flex items-center gap-3">
                                  <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-yellow-100 text-yellow-800">
                                    New required field
                                  </span>
                                  <input
                                    type="text"
                                    class="px-3 py-1.5 text-sm border rounded-md w-48"
                                    style="border-color: var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
                                    placeholder="Enter default value..."
                                    bind:value={mapping.default_value}
                                    oninput={(e) => updateFieldDefaultValue(mapping.field_id, e.target.value)}
                                  />
                                </div>
                              {/if}
                            </div>
                          </div>
                        </div>
                      </div>
                    {/each}
                  </div>
                {/if}
              </div>
            {/if}

            <!-- Status Migrations -->
            {#if activeTab === 'status' || !comprehensive}
              <div>
                <h3 class="text-lg font-medium mb-4" style="color: var(--ds-text);">{t('migrationAssistant.statusMigrations')}</h3>
                {#if statusMappings.length === 0}
                  <p class="text-sm" style="color: var(--ds-text-subtle);">No status changes detected.</p>
                {:else}
                  <div class="space-y-4">
                    {#each statusMappings as mapping}
                      <div class="rounded p-4" style="background-color: var(--ds-surface-raised);">
                        <div class="flex items-center justify-between">
                          <div class="flex-1">
                            {#if mapping.item_type_name}
                              <span class="text-xs font-medium mb-2 block" style="color: var(--ds-text-subtle);">
                                {mapping.item_type_name}
                              </span>
                            {/if}
                            <div class="flex items-center space-x-3">
                              <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium" style="background-color: var(--ds-background-neutral); color: var(--ds-text);">
                                {mapping.from_status}
                              </span>
                              <ArrowRight size={16} style="color: var(--ds-text-subtle);" />
                              {#if mapping.requires_migration}
                                <div class="w-48">
                                  <BasePicker
                                    bind:value={mapping.to_status_id}
                                    items={workflowStatuses}
                                    placeholder="Select target status..."
                                    showUnassigned={true}
                                    unassignedLabel="Select target status..."
                                    getValue={(status) => status.id}
                                    getLabel={(status) => status.name}
                                    onSelect={(status) => {
                                      if (status) {
                                        updateStatusMapping(mapping.from_status, mapping.item_type_id, status.id);
                                      }
                                    }}
                                  />
                                </div>
                              {:else}
                                <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-800">
                                  {workflowStatuses.find(s => s.id === mapping.to_status_id)?.name || t('migrationAssistant.compatible')}
                                </span>
                              {/if}
                            </div>
                            <p class="text-sm mt-2" style="color: var(--ds-text-subtle);">
                              {mapping.item_count} item{mapping.item_count !== 1 ? 's' : ''}
                              {#if mapping.requires_migration}
                                <span class="text-yellow-600 font-medium"> - {t('migrationAssistant.requiresMigration')}</span>
                              {/if}
                            </p>
                          </div>
                        </div>
                      </div>
                    {/each}
                  </div>
                {/if}
              </div>
            {/if}

            <!-- Priority Migrations -->
            {#if activeTab === 'priority' && comprehensive}
              <div>
                <h3 class="text-lg font-medium mb-4" style="color: var(--ds-text);">{t('migrationAssistant.priorityMigrations')}</h3>
                {#if priorityMappings.length === 0}
                  <p class="text-sm" style="color: var(--ds-text-subtle);">{t('migrationAssistant.noItemsToMigrate')}</p>
                {:else}
                  <div class="space-y-4">
                    {#each priorityMappings as mapping}
                      <div class="rounded p-4" style="background-color: var(--ds-surface-raised);">
                        <div class="flex items-center justify-between">
                          <div class="flex-1">
                            <div class="flex items-center space-x-3">
                              <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium" style="background-color: var(--ds-background-neutral); color: var(--ds-text);">
                                {mapping.from_priority_name}
                              </span>
                              <ArrowRight size={16} style="color: var(--ds-text-subtle);" />
                              {#if mapping.requires_migration}
                                <div class="w-48">
                                  <BasePicker
                                    bind:value={mapping.to_priority_id}
                                    items={mapping.available_targets}
                                    placeholder="Select target priority..."
                                    showUnassigned={true}
                                    unassignedLabel="Select target priority..."
                                    getValue={(priority) => priority.id}
                                    getLabel={(priority) => priority.name}
                                    onSelect={(priority) => {
                                      if (priority) {
                                        updatePriorityMapping(mapping.from_priority_id, priority.id, priority.name);
                                      }
                                    }}
                                  />
                                </div>
                              {:else}
                                <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-800">
                                  {mapping.to_priority_name || t('migrationAssistant.compatible')}
                                </span>
                              {/if}
                            </div>
                            <p class="text-sm mt-2" style="color: var(--ds-text-subtle);">
                              {mapping.item_count} item{mapping.item_count !== 1 ? 's' : ''}
                              {#if mapping.requires_migration}
                                <span class="text-yellow-600 font-medium"> - {t('migrationAssistant.requiresMigration')}</span>
                              {/if}
                            </p>
                          </div>
                        </div>
                      </div>
                    {/each}
                  </div>
                {/if}
              </div>
            {/if}

            {#if migrationSuccess}
              <AlertBox variant="success" message="Migration Completed - All items have been successfully migrated." />
            {/if}
          </div>
        {/if}
      {/if}
    </div>

    {#if migrationAnalysis && requiresMigration && !migrationSuccess}
      <DialogFooter
        class="flex-shrink-0"
        onCancel={() => closeAssistant(true)}
        onConfirm={executeMigration}
        confirmLabel={t('migrationAssistant.executeMigration')}
        loading={isMigrating}
        loadingLabel={t('migrationAssistant.migrating')}
      />
    {/if}
  </div>
</Modal>
