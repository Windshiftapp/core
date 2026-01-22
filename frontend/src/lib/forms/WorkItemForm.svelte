<script>
  import { onMount } from 'svelte';
  import { X, MoreHorizontal, Calendar, Flag, User, Layers, ChevronDown } from 'lucide-svelte';
  import { api } from '../api.js';
  import { workspacesStore } from '../stores';
  import { t } from '../stores/i18n.svelte.js';
  import MilkdownEditor from '../editors/MilkdownEditor.svelte';
  import CompactWorkspaceSelector from '../pickers/CompactWorkspaceSelector.svelte';
  import FieldChip from '../components/FieldChip.svelte';
  import CustomFieldRenderer from '../features/items/CustomFieldRenderer.svelte';
  import PriorityPicker from '../pickers/PriorityPicker.svelte';
  import MilestoneCombobox from '../pickers/MilestoneCombobox.svelte';
  import UserPicker from '../pickers/UserPicker.svelte';
  import Label from '../components/Label.svelte';
  import { getSystemFieldName } from '../stores/fieldConfig.js';
  import { createPopover, melt } from '@melt-ui/svelte';
  import { Milestone as MilestoneIcon } from 'lucide-svelte';

  const STORAGE_KEYS = {
    workspace: 'vertex_create_modal_workspace',
    itemType: 'vertex_create_modal_item_type'
  };

  let {
    formData = $bindable({
      name: '',
      description: '',
      due_date: '',
      workspace_id: null,
      priority_id: null,
      milestone_id: null,
      assignee_id: null,
      item_type_id: null
    }),
    customFieldValues = $bindable({}),
    validationErrorMessages = $bindable([]),
    parentItem = null,
    restrictedItemTypes = null,
    onWorkspaceChange = () => {},
    nameInputRef = $bindable(null)
  } = $props();

  // State management
  let selectedWorkspace = $state(null);
  let allMilestones = $state([]);
  let milestones = $state([]);
  let milestonesLoading = $state(false);
  let milestonesLoaded = $state(false);
  let workspaceDetails = $state(null);
  let customFields = $state([]);
  let allCustomFields = $state([]);
  let screenFields = $state([]);
  let screenSystemFields = $state([]);
  let loadingScreenFields = $state(false);
  let currentConfigSet = $state(null);
  let configSetLoadedForWorkspace = $state(null);
  let screenFieldsLoadedForKey = $state(null);
  let itemTypes = $state([]);
  let hierarchyLevels = $state([]);
  let availableItemTypes = $state([]);
  let itemTypesLoaded = $state(false);
  let users = $state([]);
  let usersLoaded = $state(false);
  let customFieldsLoaded = $state(false);
  let selectedPriorityObj = $state(null);
  let storedWorkspaceId = $state(null);
  let storedItemTypeId = $state(null);
  let lastPersistedWorkspaceId = $state(null);
  let lastPersistedItemTypeId = $state(null);
  let storedItemTypeApplied = $state(false);
  let configSetDefaultApplied = $state(false);

  // Computed required/non-required field lists
  let nonRequiredCustomFields = $derived(customFields.filter(cf => {
    const screenField = screenFields.find(f => f.field_type === 'custom' && parseInt(f.field_identifier) === cf.id);
    return !screenField?.is_required;
  }));

  // System fields that are auto-managed and should not be shown in create form
  const excludedSystemFields = ['status'];
  let requiredSystemFields = $derived(screenFields.filter(f =>
    f.is_required &&
    f.field_type === 'system' &&
    !excludedSystemFields.includes(f.field_identifier)
  ));

  let requiredCustomFields = $derived(customFields.filter(cf => {
    const screenField = screenFields.find(f => f.field_type === 'custom' && parseInt(f.field_identifier) === cf.id);
    return screenField?.is_required === true;
  }));

  // Derived state
  let selectedItemType = $derived(availableItemTypes.find(t => t.id === formData.item_type_id));
  let selectedAssignee = $derived(users.find(u => u.id === formData.assignee_id));
  let selectedMilestone = $derived(milestones.find(m => m.id === formData.milestone_id));

  // Overflow menu popover
  const {
    elements: { trigger: overflowTrigger, content: overflowContent },
    states: { open: overflowOpen }
  } = createPopover({
    positioning: { placement: 'bottom-end', gutter: 4 },
    portal: 'body',
    forceVisible: true
  });

  // Due date popover
  const {
    elements: { trigger: dueDateTrigger, content: dueDateContent },
    states: { open: dueDateOpen }
  } = createPopover({
    positioning: { placement: 'bottom-start', gutter: 4 },
    portal: 'body',
    forceVisible: true
  });

  // Helper functions
  function isFieldRequired(fieldIdentifier) {
    const screenField = screenFields.find(f => f.field_identifier === fieldIdentifier);
    return screenField?.is_required === true;
  }

  function isFieldConfigured(fieldIdentifier) {
    return screenSystemFields.includes(fieldIdentifier);
  }

  function formatDueDate(dateStr) {
    if (!dateStr) return null;
    const date = new Date(dateStr);
    const today = new Date();
    const diffDays = Math.ceil((date - today) / (1000 * 60 * 60 * 24));
    if (diffDays === 0) return t('common.today');
    if (diffDays === 1) return t('common.tomorrow');
    if (diffDays === -1) return t('common.yesterday');
    if (diffDays > 0 && diffDays <= 7) return `${diffDays} days`;
    return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
  }

  function loadStoredSelections() {
    if (typeof window === 'undefined') return;
    try {
      const workspaceValue = window.localStorage.getItem(STORAGE_KEYS.workspace);
      if (workspaceValue) {
        const parsedWorkspace = parseInt(workspaceValue, 10);
        storedWorkspaceId = Number.isNaN(parsedWorkspace) ? null : parsedWorkspace;
      }
    } catch (error) {
      storedWorkspaceId = null;
    }
    try {
      const itemTypeValue = window.localStorage.getItem(STORAGE_KEYS.itemType);
      if (itemTypeValue) {
        const parsedItemType = parseInt(itemTypeValue, 10);
        storedItemTypeId = Number.isNaN(parsedItemType) ? null : parsedItemType;
      }
    } catch (error) {
      storedItemTypeId = null;
    }
  }

  function persistWorkspaceSelection(workspaceId) {
    if (typeof window === 'undefined' || !workspaceId) return;
    try {
      window.localStorage.setItem(STORAGE_KEYS.workspace, String(workspaceId));
      storedWorkspaceId = workspaceId;
    } catch (error) {}
  }

  function persistItemTypeSelection(itemTypeId) {
    if (typeof window === 'undefined' || !itemTypeId) return;
    try {
      window.localStorage.setItem(STORAGE_KEYS.itemType, String(itemTypeId));
      storedItemTypeId = itemTypeId;
    } catch (error) {}
  }

  // Data loading functions
  async function loadUsers() {
    if (usersLoaded) return;
    try {
      const result = await api.getUsers();
      users = result || [];
      usersLoaded = true;
    } catch (error) {
      users = [];
      usersLoaded = true;
    }
  }

  async function loadCustomFields() {
    try {
      const result = await api.customFields.getAll();
      allCustomFields = result || [];
      customFieldsLoaded = true;
    } catch (error) {
      allCustomFields = [];
      customFields = [];
      customFieldsLoaded = true;
    }
  }

  async function loadMilestones() {
    if (milestonesLoading || milestonesLoaded) return;
    try {
      milestonesLoading = true;
      const result = await api.milestones.getAll();
      allMilestones = result || [];
      filterMilestones();
      milestonesLoaded = true;
    } catch (error) {
      allMilestones = [];
      milestones = [];
      milestonesLoaded = true;
    } finally {
      milestonesLoading = false;
    }
  }

  function filterMilestones() {
    if (!workspaceDetails || !workspaceDetails.milestone_categories || workspaceDetails.milestone_categories.length === 0) {
      milestones = allMilestones;
    } else {
      const allowedCategoryIds = workspaceDetails.milestone_categories;
      milestones = allMilestones.filter(m => allowedCategoryIds.includes(m.category_id));
    }
  }

  async function loadWorkspaceDetails(workspaceId) {
    if (!workspaceId) {
      workspaceDetails = null;
      filterMilestones();
      return;
    }
    try {
      workspaceDetails = await api.workspaces.get(workspaceId);
      filterMilestones();
    } catch (error) {
      workspaceDetails = null;
      filterMilestones();
    }
  }

  async function loadItemTypes(forceReload = false) {
    if (itemTypesLoaded && !forceReload) return;
    try {
      const [itemTypesResult, hierarchyLevelsResult] = await Promise.all([
        api.itemTypes.getAll(),
        api.hierarchyLevels.getAll()
      ]);
      itemTypes = itemTypesResult || [];
      hierarchyLevels = hierarchyLevelsResult || [];

      if (restrictedItemTypes && restrictedItemTypes.length > 0) {
        availableItemTypes = restrictedItemTypes.sort((a, b) => a.hierarchy_level - b.hierarchy_level || a.sort_order - b.sort_order);
        const currentTypeValid = formData.item_type_id && availableItemTypes.some(t => t.id === formData.item_type_id);
        if (!currentTypeValid && availableItemTypes.length > 0) {
          formData.item_type_id = availableItemTypes[0].id;
        }
      } else {
        availableItemTypes = itemTypes.sort((a, b) => a.hierarchy_level - b.hierarchy_level || a.sort_order - b.sort_order);
        if (availableItemTypes.length > 0 && !formData.item_type_id) {
          formData.item_type_id = availableItemTypes[0].id;
        }
      }
      itemTypesLoaded = true;
    } catch (error) {
      itemTypes = [];
      hierarchyLevels = [];
      availableItemTypes = [];
      itemTypesLoaded = true;
    }
  }

  function resolveCreateScreenId(itemTypeId) {
    if (currentConfigSet) {
      const itemTypeConfig = currentConfigSet.item_type_configs?.find(c => c.item_type_id === itemTypeId);
      if (itemTypeConfig?.create_screen_id) return itemTypeConfig.create_screen_id;
      if (currentConfigSet.create_screen_id) return currentConfigSet.create_screen_id;
    }
    return 1;
  }

  async function loadConfigSetForWorkspace(workspaceId) {
    if (configSetLoadedForWorkspace === workspaceId) return;
    try {
      const response = await api.configurationSets.getAll();
      const configSets = response?.configuration_sets || [];
      currentConfigSet = null;
      let defaultConfigSet = null;

      for (const configSet of configSets) {
        if (configSet.is_default) defaultConfigSet = configSet;
        if (configSet.workspace_ids && configSet.workspace_ids.includes(workspaceId)) {
          currentConfigSet = await api.configurationSets.get(configSet.id);
          break;
        }
      }

      if (!currentConfigSet && defaultConfigSet) {
        currentConfigSet = await api.configurationSets.get(defaultConfigSet.id);
      }

      configSetLoadedForWorkspace = workspaceId;

      if (currentConfigSet?.item_type_configs?.length > 0) {
        const allowedItemTypeIds = currentConfigSet.item_type_configs.map(c => c.item_type_id);
        const baseTypes = restrictedItemTypes && restrictedItemTypes.length > 0 ? restrictedItemTypes : itemTypes;
        availableItemTypes = baseTypes
          .filter(t => allowedItemTypeIds.includes(t.id))
          .sort((a, b) => a.hierarchy_level - b.hierarchy_level || a.sort_order - b.sort_order);
      } else if (restrictedItemTypes && restrictedItemTypes.length > 0) {
        availableItemTypes = restrictedItemTypes.sort((a, b) => a.hierarchy_level - b.hierarchy_level || a.sort_order - b.sort_order);
      } else {
        availableItemTypes = itemTypes.sort((a, b) => a.hierarchy_level - b.hierarchy_level || a.sort_order - b.sort_order);
      }

      if (availableItemTypes.length > 0 && !availableItemTypes.find(t => t.id === formData.item_type_id)) {
        formData.item_type_id = availableItemTypes[0].id;
      }
    } catch (error) {
      currentConfigSet = null;
      configSetLoadedForWorkspace = workspaceId;
      const baseTypes = restrictedItemTypes && restrictedItemTypes.length > 0 ? restrictedItemTypes : itemTypes;
      availableItemTypes = baseTypes.sort((a, b) => a.hierarchy_level - b.hierarchy_level || a.sort_order - b.sort_order);
    }
  }

  async function loadScreenFieldsForItemType(workspaceId, itemTypeId) {
    const key = `${workspaceId}-${itemTypeId}`;
    if (loadingScreenFields || screenFieldsLoadedForKey === key) return;
    try {
      loadingScreenFields = true;
      const createScreenId = resolveCreateScreenId(itemTypeId);
      const fields = await api.screens.getFields(createScreenId);
      screenFields = fields || [];

      screenSystemFields = screenFields
        .filter(field => field.field_type === 'system')
        .map(field => field.field_identifier);

      const customFieldIds = screenFields
        .filter(field => field.field_type === 'custom')
        .map(field => parseInt(field.field_identifier));

      const filteredCustomFields = allCustomFields.filter(field => customFieldIds.includes(field.id));

      customFieldValues = {};
      filteredCustomFields.forEach(field => {
        customFieldValues[field.id] = '';
      });

      customFields = filteredCustomFields;
      screenFieldsLoadedForKey = key;
    } catch (error) {
      screenSystemFields = ['priority', 'milestone'];
      screenFields = [];
      customFields = [];
      customFieldValues = {};
      screenFieldsLoadedForKey = key;
    } finally {
      loadingScreenFields = false;
    }
  }

  // Validation
  export function validate() {
    const errors = [];

    for (const field of screenFields) {
      if (field.is_required) {
        if (field.field_type === 'system') {
          const identifier = field.field_identifier;
          // Skip system-managed fields that are auto-assigned
          if (excludedSystemFields.includes(identifier)) {
            continue;
          }
          const fieldKeyMap = { 'title': 'name' };
          const formKey = fieldKeyMap[identifier] || identifier;
          const value = formData[formKey] ?? formData[`${formKey}_id`];
          if (!value) {
            errors.push(`${getSystemFieldName(identifier)} is required`);
          }
        } else if (field.field_type === 'custom') {
          const fieldId = parseInt(field.field_identifier);
          const value = customFieldValues[fieldId];
          if (value === undefined || value === null || value === '') {
            const fieldDef = allCustomFields.find(f => f.id === fieldId);
            errors.push(`${fieldDef?.name || 'Custom field'} is required`);
          }
        }
      }
    }

    validationErrorMessages = errors;
    return errors.length === 0;
  }

  // Export functions for parent component
  export function getFormData() {
    return {
      workspace_id: selectedWorkspace?.id,
      title: formData.name,
      description: formData.description || '',
      priority_id: formData.priority_id || null,
      milestone_id: formData.milestone_id || null,
      assignee_id: formData.assignee_id || null,
      due_date: formData.due_date ? new Date(formData.due_date).toISOString() : null,
      status: 'open',
      item_type_id: formData.item_type_id,
      parent_id: parentItem ? parentItem.id : null,
      custom_field_values: customFieldValues
    };
  }

  export function getSelectedWorkspace() {
    return selectedWorkspace;
  }

  export function reset() {
    selectedWorkspace = null;
    milestonesLoaded = false;
    milestonesLoading = false;
    allMilestones = [];
    milestones = [];
    workspaceDetails = null;
    itemTypesLoaded = false;
    customFieldsLoaded = false;
    itemTypes = [];
    hierarchyLevels = [];
    availableItemTypes = [];
    currentConfigSet = null;
    configSetLoadedForWorkspace = null;
    screenFieldsLoadedForKey = null;
    loadingScreenFields = false;
    screenFields = [];
    screenSystemFields = [];
    customFields = [];
    customFieldValues = {};
    validationErrorMessages = [];
    storedItemTypeApplied = false;
    configSetDefaultApplied = false;

    formData = {
      name: '',
      description: '',
      due_date: '',
      workspace_id: null,
      priority_id: null,
      milestone_id: null,
      assignee_id: null,
      item_type_id: availableItemTypes.length > 0 ? availableItemTypes[0].id : null
    };
  }

  // Effects
  $effect(() => {
    if (selectedWorkspace) {
      loadWorkspaceDetails(selectedWorkspace.id);
    } else {
      workspaceDetails = null;
      filterMilestones();
    }
  });

  $effect(() => {
    if (allCustomFields.length === 0) loadCustomFields();
  });

  $effect(() => {
    if (!milestonesLoaded) loadMilestones();
  });

  $effect(() => {
    if (!itemTypesLoaded) loadItemTypes();
  });

  $effect(() => {
    if (!usersLoaded) loadUsers();
  });

  $effect(() => {
    if (formData.workspace_id && configSetLoadedForWorkspace !== formData.workspace_id) {
      loadConfigSetForWorkspace(formData.workspace_id);
    }
  });

  $effect(() => {
    if (formData.workspace_id && formData.item_type_id && customFieldsLoaded && configSetLoadedForWorkspace === formData.workspace_id) {
      const key = `${formData.workspace_id}-${formData.item_type_id}`;
      if (screenFieldsLoadedForKey !== key) {
        loadScreenFieldsForItemType(formData.workspace_id, formData.item_type_id);
      }
    }
  });

  $effect(() => {
    if (!formData.workspace_id && storedWorkspaceId && $workspacesStore.regularWorkspaces.length > 0) {
      const storedWorkspace = $workspacesStore.regularWorkspaces.find(w => w.id === storedWorkspaceId);
      if (storedWorkspace) {
        selectedWorkspace = storedWorkspace;
        formData.workspace_id = storedWorkspace.id;
      }
    }
  });

  $effect(() => {
    // Don't apply stored item type when creating child items (restrictedItemTypes is set)
    if (storedItemTypeId && availableItemTypes.length > 0 && !storedItemTypeApplied && !restrictedItemTypes) {
      const storedItemType = availableItemTypes.find(type => type.id === storedItemTypeId);
      if (storedItemType) formData.item_type_id = storedItemType.id;
      storedItemTypeApplied = true;
    }
  });

  $effect(() => {
    if (availableItemTypes.length > 0 && currentConfigSet?.default_item_type_id && !configSetDefaultApplied) {
      const hasValidStoredType = storedItemTypeId && availableItemTypes.find(type => type.id === storedItemTypeId);
      if (!hasValidStoredType) {
        const configDefault = availableItemTypes.find(type => type.id === currentConfigSet.default_item_type_id);
        if (configDefault) formData.item_type_id = configDefault.id;
      }
      configSetDefaultApplied = true;
    }
  });

  $effect(() => {
    if (selectedWorkspace?.id && selectedWorkspace.id !== lastPersistedWorkspaceId) {
      lastPersistedWorkspaceId = selectedWorkspace.id;
      persistWorkspaceSelection(selectedWorkspace.id);
    }
  });

  $effect(() => {
    if (formData.item_type_id && formData.item_type_id !== lastPersistedItemTypeId) {
      lastPersistedItemTypeId = formData.item_type_id;
      persistItemTypeSelection(formData.item_type_id);
    }
  });

  // Auto-select first workspace if only one exists
  $effect(() => {
    if ($workspacesStore.regularWorkspaces.length === 1 && !formData.workspace_id) {
      selectedWorkspace = $workspacesStore.regularWorkspaces[0];
      formData.workspace_id = $workspacesStore.regularWorkspaces[0].id;
    }
  });

  onMount(() => {
    loadStoredSelections();
  });
</script>

<div class="space-y-3">
  <!-- Validation Errors -->
  {#if validationErrorMessages.length > 0}
    <div class="p-3 rounded text-sm" style="background-color: var(--ds-background-danger-subtle, #fef2f2); border: 1px solid var(--ds-border-danger, #fecaca); color: var(--ds-text-danger, #dc2626);">
      <p class="font-medium mb-1">{t('createModal.fillRequiredFields')}</p>
      <ul class="list-disc list-inside">
        {#each validationErrorMessages as error}
          <li>{error}</li>
        {/each}
      </ul>
    </div>
  {/if}

  <!-- Parent Item Info -->
  {#if parentItem}
    <div class="text-xs px-2 py-1.5 rounded" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
      {t('createModal.parent')}: {parentItem.title}
    </div>
  {/if}

  <!-- Title Input -->
  <input
    bind:this={nameInputRef}
    bind:value={formData.name}
    type="text"
    class="w-full text-lg font-medium border-0 outline-none bg-transparent"
    style="color: var(--ds-text);"
    placeholder={t('createModal.issueTitle')}
  />

  <!-- Description -->
  <div class="min-h-[60px]">
    <MilkdownEditor
      bind:content={formData.description}
      placeholder={t('createModal.addDescription')}
      compact={true}
      showToolbar={false}
      readonly={false}
      itemId={null}
    />
  </div>

  <!-- Field Chips Row -->
  <div class="flex flex-wrap items-center gap-2 pt-2 border-t" style="border-color: var(--ds-border);">
    <!-- Item Type Chip -->
    {#if availableItemTypes.length >= 1}
      <FieldChip
        label={t('createModal.type')}
        value={formData.item_type_id}
        displayValue={selectedItemType?.name || ''}
        icon={Layers}
        placeholder={t('createModal.type')}
      >
        {#snippet children({ close: closePopover })}
          <div class="p-2 max-h-48 overflow-y-auto">
            {#each availableItemTypes as itemType}
              <button
                type="button"
                class="w-full px-3 py-2 text-left text-sm rounded transition-colors"
                style="color: var(--ds-text);"
                onmouseover={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-selected)'}
                onmouseout={(e) => e.currentTarget.style.backgroundColor = 'transparent'}
                onclick={() => {
                  formData.item_type_id = itemType.id;
                  closePopover();
                }}
              >
                {itemType.name}
              </button>
            {/each}
          </div>
        {/snippet}
      </FieldChip>
    {/if}

    <!-- Priority Chip -->
    {#if isFieldConfigured('priority') && !isFieldRequired('priority')}
      {#if selectedWorkspace}
        <PriorityPicker
          workspaceId={selectedWorkspace.id}
          selectedPriorityId={formData.priority_id}
          onChange={(priorityId, priority) => {
            formData.priority_id = priorityId;
            selectedPriorityObj = priority;
          }}
          showUnassigned={true}
          unassignedLabel={t('createModal.noPriority')}
        >
          {#snippet children()}
            <div
              class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-sm transition-colors"
              style="background-color: var(--ds-surface); border: 1px solid var(--ds-border); color: {formData.priority_id ? 'var(--ds-text)' : 'var(--ds-text-subtle)'};"
              onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-surface-hovered, var(--ds-background-neutral-hovered))'}
              onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-surface)'}
            >
              <Flag size={14} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
              <span class="truncate max-w-[120px]">{selectedPriorityObj?.name || t('createModal.priority')}</span>
              <ChevronDown size={12} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
            </div>
          {/snippet}
        </PriorityPicker>
      {:else}
        <div
          class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-sm cursor-not-allowed opacity-50"
          style="background-color: var(--ds-surface); border: 1px solid var(--ds-border); color: var(--ds-text-subtle);"
        >
          <Flag size={14} style="flex-shrink: 0;" />
          <span>{t('createModal.priority')}</span>
          <ChevronDown size={12} style="flex-shrink: 0;" />
        </div>
      {/if}
    {/if}

    <!-- Assignee Chip -->
    {#if isFieldConfigured('assignee') && !isFieldRequired('assignee')}
      <UserPicker
        bind:value={formData.assignee_id}
        showUnassigned={true}
        unassignedLabel={t('createModal.unassigned')}
      >
        {#snippet children()}
          <div
            class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-sm transition-colors"
            style="background-color: var(--ds-surface); border: 1px solid var(--ds-border); color: {formData.assignee_id ? 'var(--ds-text)' : 'var(--ds-text-subtle)'};"
            onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-surface-hovered, var(--ds-background-neutral-hovered))'}
            onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-surface)'}
          >
            <User size={14} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
            <span class="truncate max-w-[120px]">{selectedAssignee?.name || selectedAssignee?.email || t('createModal.assignee')}</span>
            <ChevronDown size={12} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
          </div>
        {/snippet}
      </UserPicker>
    {/if}

    <!-- Due Date Chip -->
    {#if isFieldConfigured('due_date') && !isFieldRequired('due_date')}
      <button
        use:melt={$dueDateTrigger}
        class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-sm transition-colors"
        style="background-color: var(--ds-surface); border: 1px solid var(--ds-border); color: {formData.due_date ? 'var(--ds-text)' : 'var(--ds-text-subtle)'};"
        onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-surface-hovered, var(--ds-background-neutral-hovered))'}
        onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-surface)'}
      >
        <Calendar size={14} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
        <span class="truncate max-w-[120px]">{formData.due_date ? formatDueDate(formData.due_date) : t('createModal.dueDate')}</span>
        <ChevronDown size={12} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
      </button>

      {#if $dueDateOpen}
        <div
          use:melt={$dueDateContent}
          class="z-50 rounded-lg shadow-lg p-3"
          style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
        >
          <input
            type="date"
            bind:value={formData.due_date}
            class="w-full px-3 py-2 rounded border text-sm"
            style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
            onchange={() => $dueDateOpen = false}
          />
        </div>
      {/if}
    {/if}

    <!-- Milestone Chip -->
    {#if isFieldConfigured('milestone') && !isFieldRequired('milestone')}
      <MilestoneCombobox
        bind:value={formData.milestone_id}
        workspaceId={selectedWorkspace?.id}
        showUnassigned={true}
        unassignedLabel={t('createModal.noMilestone')}
      >
        {#snippet children()}
          <div
            class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-sm transition-colors"
            style="background-color: var(--ds-surface); border: 1px solid var(--ds-border); color: {formData.milestone_id ? 'var(--ds-text)' : 'var(--ds-text-subtle)'};"
            onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-surface-hovered, var(--ds-background-neutral-hovered))'}
            onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-surface)'}
          >
            {#if selectedMilestone?.category_color}
              <div class="w-2 h-2 rounded-full flex-shrink-0" style="background-color: {selectedMilestone.category_color};"></div>
            {:else}
              <MilestoneIcon size={14} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
            {/if}
            <span class="truncate max-w-[120px]">{selectedMilestone?.name || t('createModal.milestoneField')}</span>
            <ChevronDown size={12} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
          </div>
        {/snippet}
      </MilestoneCombobox>
    {/if}

    <!-- Overflow Menu for Non-Required Custom Fields -->
    {#if nonRequiredCustomFields.length > 0}
      <button
        use:melt={$overflowTrigger}
        class="inline-flex items-center px-2 py-1 rounded-full text-sm transition-colors"
        style="background-color: var(--ds-surface); border: 1px solid var(--ds-border); color: var(--ds-text-subtle);"
        onmouseover={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-surface-hovered, var(--ds-background-neutral-hovered))'}
        onmouseout={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-surface)'}
      >
        <MoreHorizontal size={14} />
      </button>

      {#if $overflowOpen}
        <div
          use:melt={$overflowContent}
          class="z-50 rounded-lg shadow-lg overflow-hidden p-2"
          style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border); min-width: 200px; max-width: 300px;"
        >
          <div class="text-xs font-medium px-2 py-1 mb-1" style="color: var(--ds-text-subtle);">
            {t('createModal.additionalFields')}
          </div>
          {#each nonRequiredCustomFields as field}
            <div class="px-2 py-2">
              <CustomFieldRenderer
                {field}
                bind:value={customFieldValues[field.id]}
                readonly={false}
                onChange={(val) => customFieldValues[field.id] = val}
                {milestones}
                isDarkMode={false}
                autoOpenPickers={false}
              />
            </div>
          {/each}
        </div>
      {/if}
    {/if}
  </div>

  <!-- Required System Fields Section -->
  {#if requiredSystemFields.length > 0}
    <div class="space-y-3 pt-3 border-t" style="border-color: var(--ds-border);">
      {#each requiredSystemFields as field}
        {#if field.field_identifier === 'priority'}
          <div class="space-y-1">
            <Label color="default">
              {t('createModal.priority')} <span style="color: var(--ds-text-danger, #ef4444);">*</span>
            </Label>
            {#if selectedWorkspace}
              <PriorityPicker
                workspaceId={selectedWorkspace.id}
                selectedPriorityId={formData.priority_id}
                onChange={(priorityId) => formData.priority_id = priorityId}
                placeholder={t('createModal.noPriority')}
              />
            {:else}
              <div class="px-3 py-2 text-sm rounded border" style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text-subtle);">
                {t('createModal.selectWorkspaceFirst')}
              </div>
            {/if}
          </div>
        {:else if field.field_identifier === 'due_date'}
          <div class="space-y-1">
            <Label color="default">
              {t('createModal.dueDate')} <span style="color: var(--ds-text-danger, #ef4444);">*</span>
            </Label>
            <input
              type="date"
              bind:value={formData.due_date}
              class="w-full px-3 py-2 rounded border text-sm"
              style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
            />
          </div>
        {:else if field.field_identifier === 'milestone'}
          <div class="space-y-1">
            <Label color="default">
              {t('createModal.milestoneField')} <span style="color: var(--ds-text-danger, #ef4444);">*</span>
            </Label>
            <MilestoneCombobox
              bind:value={formData.milestone_id}
              workspaceId={selectedWorkspace?.id}
              placeholder={t('createModal.noMilestone')}
            />
          </div>
        {:else if field.field_identifier === 'assignee'}
          <div class="space-y-1">
            <Label color="default">
              {t('createModal.assignee')} <span style="color: var(--ds-text-danger, #ef4444);">*</span>
            </Label>
            <UserPicker
              bind:value={formData.assignee_id}
              placeholder={t('createModal.unassigned')}
            />
          </div>
        {/if}
      {/each}
    </div>
  {/if}

  <!-- Required Custom Fields Section -->
  {#if requiredCustomFields.length > 0}
    <div class="space-y-3 pt-3 border-t" style="border-color: var(--ds-border);">
      {#each requiredCustomFields as field}
        <div class="space-y-1">
          <Label color="default">
            {field.name} <span style="color: var(--ds-text-danger, #ef4444);">*</span>
          </Label>
          <CustomFieldRenderer
            {field}
            bind:value={customFieldValues[field.id]}
            readonly={false}
            onChange={(val) => customFieldValues[field.id] = val}
            {milestones}
            isDarkMode={false}
          />
        </div>
      {/each}
    </div>
  {/if}
</div>
