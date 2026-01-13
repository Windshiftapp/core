<script>
  import { createEventDispatcher, onMount } from 'svelte';
  import { fade } from 'svelte/transition';
  import { navigate, currentRoute } from '../router.js';
  import { milestonesStore } from '../stores/milestones.js';
  import { workspacesStore, shouldNavigateAfterCreate } from '../stores';
  import { api } from '../api.js';
  import { X, MoreHorizontal, Calendar, Target, Flag, User, Tag, Milestone as MilestoneIcon, Building, FolderOpen, Layers, ChevronRight, ChevronDown, FileText } from 'lucide-svelte';
  import MilkdownEditor from '../editors/MilkdownEditor.svelte';
  import Button from '../components/Button.svelte';
  import CompactWorkspaceSelector from '../pickers/CompactWorkspaceSelector.svelte';
  import FieldChip from '../components/FieldChip.svelte';
  import CustomFieldRenderer from '../features/items/CustomFieldRenderer.svelte';
  import PriorityPicker from '../pickers/PriorityPicker.svelte';
  import MilestoneCombobox from '../pickers/MilestoneCombobox.svelte';
  import UserPicker from '../pickers/UserPicker.svelte';
  import BasePicker from '../pickers/BasePicker.svelte';
  import Select from '../components/Select.svelte';
  import Label from '../components/Label.svelte';
  import { collectionCategoriesStore } from '../stores/collectionCategories.js';
  import { getShortcut, matchesShortcut, getDisplayString } from '../utils/keyboardShortcuts.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import { createPopover, melt } from '@melt-ui/svelte';

  // Status options for milestones
  const milestoneStatusOptions = [
    { value: 'planning', label: 'Planning' },
    { value: 'in-progress', label: 'In Progress' },
    { value: 'completed', label: 'Completed' },
    { value: 'cancelled', label: 'Cancelled' }
  ];

  // Type icons and options for the type selector
  const typeIcons = {
    'work-item': FileText,
    'milestone': Target,
    'workspace': Building,
    'collection': FolderOpen
  };

  const typeOptions = [
    { value: 'work-item', label: 'Work Item', icon: FileText },
    { value: 'milestone', label: 'Milestone', icon: Target },
    { value: 'workspace', label: 'Workspace', icon: Building },
    { value: 'collection', label: 'Collection', icon: FolderOpen }
  ];

  const dispatch = createEventDispatcher();

  // Get shortcut configurations
  const submitShortcut = getShortcut('modal', 'submit');
  const cancelShortcut = getShortcut('modal', 'cancel');

  let {
    isOpen = $bindable(false),
    compactMode = false // Hides type selection tabs, forces work-item type
  } = $props();

  let selectedType = $state('work-item'); // work-item, milestone, workspace, collection
  let selectedWorkspace = $state(null);
  let allMilestones = $state([]);
  let milestones = $state([]);
  let milestonesLoading = $state(false);
  let milestonesLoaded = $state(false);
  let workspaceDetails = $state(null);
  let parentItem = $state(null);
  let restrictedItemTypes = $state(null);
  let customFields = $state([]);
  let allCustomFields = $state([]);
  let screenFields = $state([]);
  let screenSystemFields = $state([]);
  let customFieldValues = $state({});
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
  let formData = $state({
    name: '',
    description: '',
    target_date: '',
    due_date: '',
    status: 'planning',
    workspace_id: null,
    priority_id: null,
    milestone_id: null,
    assignee_id: null,
    key: '',
    item_type_id: null
  });

  // Collection-specific state
  let collectionCategoryId = $state(null);

  // Active field picker state
  let activeField = $state(null);
  let itemTypeChipRef = null;
  let statusChipRef = null;
  let categoryChipRef = null;

  // Track selected priority object for display in chip trigger
  let selectedPriorityObj = $state(null);

  // Computed required/non-required field lists
  let nonRequiredCustomFields = $derived(customFields.filter(cf => {
    const screenField = screenFields.find(f => f.field_type === 'custom' && parseInt(f.field_identifier) === cf.id);
    return !screenField?.required;
  }));

  let requiredSystemFields = $derived(screenFields.filter(f => f.required && f.field_type === 'system'));

  let requiredCustomFields = $derived(customFields.filter(cf => {
    const screenField = screenFields.find(f => f.field_type === 'custom' && parseInt(f.field_identifier) === cf.id);
    return screenField?.required === true;
  }));

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

  const STORAGE_KEYS = {
    workspace: 'vertex_create_modal_workspace',
    itemType: 'vertex_create_modal_item_type'
  };

  let storedWorkspaceId = $state(null);
  let storedItemTypeId = $state(null);
  let lastPersistedWorkspaceId = $state(null);
  let lastPersistedItemTypeId = $state(null);
  let storedItemTypeApplied = $state(false);
  let configSetDefaultApplied = $state(false);

  function loadStoredSelections() {
    if (typeof window === 'undefined') return;

    try {
      const workspaceValue = window.localStorage.getItem(STORAGE_KEYS.workspace);
      if (workspaceValue) {
        const parsedWorkspace = parseInt(workspaceValue, 10);
        storedWorkspaceId = Number.isNaN(parsedWorkspace) ? null : parsedWorkspace;
      }
    } catch (error) {
      console.warn('Failed to read stored workspace selection:', error);
      storedWorkspaceId = null;
    }

    try {
      const itemTypeValue = window.localStorage.getItem(STORAGE_KEYS.itemType);
      if (itemTypeValue) {
        const parsedItemType = parseInt(itemTypeValue, 10);
        storedItemTypeId = Number.isNaN(parsedItemType) ? null : parsedItemType;
      }
    } catch (error) {
      console.warn('Failed to read stored item type selection:', error);
      storedItemTypeId = null;
    }
  }

  function persistWorkspaceSelection(workspaceId) {
    if (typeof window === 'undefined' || !workspaceId) return;
    try {
      window.localStorage.setItem(STORAGE_KEYS.workspace, String(workspaceId));
      storedWorkspaceId = workspaceId;
    } catch (error) {
      console.warn('Failed to store workspace selection:', error);
    }
  }

  function persistItemTypeSelection(itemTypeId) {
    if (typeof window === 'undefined' || !itemTypeId) return;
    try {
      window.localStorage.setItem(STORAGE_KEYS.itemType, String(itemTypeId));
      storedItemTypeId = itemTypeId;
    } catch (error) {
      console.warn('Failed to store item type selection:', error);
    }
  }

  // Type display names
  const typeLabels = {
    'work-item': 'Work Item',
    'milestone': 'Milestone',
    'workspace': 'Workspace',
    'collection': 'Collection'
  };

  // Helper functions for screen field configuration
  function isFieldRequired(fieldIdentifier) {
    const screenField = screenFields.find(f => f.field_identifier === fieldIdentifier);
    return screenField?.required === true;
  }

  function isFieldConfigured(fieldIdentifier) {
    return screenSystemFields.includes(fieldIdentifier);
  }

  async function loadWorkspaces() {
    await workspacesStore.load();
  }

  async function loadUsers() {
    if (usersLoaded) return;
    try {
      const result = await api.getUsers();
      users = result || [];
      usersLoaded = true;
    } catch (error) {
      console.error('Failed to load users:', error);
      users = [];
      usersLoaded = true;
    }
  }

  async function loadCustomFields() {
    try {
      const result = await api.customFields.getAll();
      allCustomFields = result || [];
      customFields = allCustomFields;
      customFieldsLoaded = true;
    } catch (error) {
      console.error('Failed to load custom fields:', error);
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
      console.error('Failed to load milestones:', error);
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
      console.error('Failed to load workspace details:', error);
      workspaceDetails = null;
      filterMilestones();
    }
  }

  $effect(() => {
    if (selectedWorkspace) {
      loadWorkspaceDetails(selectedWorkspace.id);
    } else {
      workspaceDetails = null;
      filterMilestones();
    }
  });

  async function loadItemTypes(forceReload = false) {
    if (itemTypesLoaded && !forceReload) {
      return;
    }

    try {
      const [itemTypesResult, hierarchyLevelsResult] = await Promise.all([
        api.itemTypes.getAll(),
        api.hierarchyLevels.getAll()
      ]);

      itemTypes = itemTypesResult || [];
      hierarchyLevels = hierarchyLevelsResult || [];

      if (restrictedItemTypes && restrictedItemTypes.length > 0) {
        availableItemTypes = restrictedItemTypes.sort((a, b) => a.hierarchy_level - b.hierarchy_level || a.sort_order - b.sort_order);
      } else {
        availableItemTypes = itemTypes.sort((a, b) => a.hierarchy_level - b.hierarchy_level || a.sort_order - b.sort_order);
      }

      if (availableItemTypes.length > 0 && !formData.item_type_id) {
        formData.item_type_id = availableItemTypes[0].id;
      }

      itemTypesLoaded = true;
    } catch (error) {
      console.error('Failed to load item types:', error);
      itemTypes = [];
      hierarchyLevels = [];
      availableItemTypes = [];
      itemTypesLoaded = true;
    }
  }

  function resolveCreateScreenId(itemTypeId) {
    if (currentConfigSet) {
      const itemTypeConfig = currentConfigSet.item_type_configs?.find(
        c => c.item_type_id === itemTypeId
      );
      if (itemTypeConfig?.create_screen_id) {
        return itemTypeConfig.create_screen_id;
      }
      if (currentConfigSet.create_screen_id) {
        return currentConfigSet.create_screen_id;
      }
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
        if (configSet.is_default) {
          defaultConfigSet = configSet;
        }
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
      console.error('Failed to load config set for workspace:', error);
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

      const filteredCustomFields = allCustomFields.filter(field =>
        customFieldIds.includes(field.id)
      );

      customFieldValues = {};
      filteredCustomFields.forEach(field => {
        customFieldValues[field.id] = '';
      });

      customFields = filteredCustomFields;
      screenFieldsLoadedForKey = key;
      console.log('[CreateModal] Screen fields loaded:', {
        screenFields,
        screenSystemFields,
        customFieldsCount: customFields.length
      });
    } catch (error) {
      console.error('Failed to load screen fields:', error);
      screenSystemFields = ['priority', 'milestone'];
      screenFields = [];
      customFields = [];
      customFieldValues = {};
      screenFieldsLoadedForKey = key;
    } finally {
      loadingScreenFields = false;
    }
  }

  function close() {
    isOpen = false;
    selectedType = 'work-item';
    selectedWorkspace = null;
    parentItem = null;
    restrictedItemTypes = null;
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
    storedItemTypeApplied = false;
    configSetDefaultApplied = false;
    collectionCategoryId = null;
    activeField = null;
    resetForm();
    dispatch('close');
  }

  function resetForm() {
    formData = {
      name: '',
      description: '',
      target_date: '',
      due_date: '',
      status: 'planning',
      workspace_id: null,
      priority_id: null,
      milestone_id: null,
      assignee_id: null,
      key: '',
      item_type_id: availableItemTypes.length > 0 ? availableItemTypes[0].id : null,
    };
    customFieldValues = {};
    collectionCategoryId = null;
    customFields.forEach(field => {
      customFieldValues[field.id] = '';
    });
  }

  function selectType(type) {
    selectedType = type;
    resetForm();
    if (type === 'work-item') {
      if (!$workspacesStore.loaded) {
        loadWorkspaces();
      }
      if (allCustomFields.length === 0) {
        loadCustomFields();
      }
      if (!itemTypesLoaded) {
        loadItemTypes();
      }
      if (!usersLoaded) {
        loadUsers();
      }
    } else if (type === 'collection') {
      collectionCategoriesStore.init();
    }
  }

  async function handleSubmit() {
    try {
      if (selectedType === 'work-item') {
        const validationErrors = [];

        for (const field of screenFields) {
          if (field.required) {
            if (field.field_type === 'system') {
              const identifier = field.field_identifier;
              if (identifier === 'priority' && !formData.priority_id) {
                validationErrors.push('Priority is required');
              }
              if (identifier === 'due_date' && !formData.due_date) {
                validationErrors.push('Due Date is required');
              }
              if (identifier === 'milestone' && !formData.milestone_id) {
                validationErrors.push('Milestone is required');
              }
            } else if (field.field_type === 'custom') {
              const fieldId = parseInt(field.field_identifier);
              const value = customFieldValues[fieldId];
              if (value === undefined || value === null || value === '') {
                const fieldDef = allCustomFields.find(f => f.id === fieldId);
                validationErrors.push(`${fieldDef?.name || 'Custom field'} is required`);
              }
            }
          }
        }

        if (validationErrors.length > 0) {
          alert('Please fill in required fields:\n' + validationErrors.join('\n'));
          return;
        }

        if (selectedWorkspace) {
          const itemData = {
            workspace_id: selectedWorkspace.id,
            title: formData.name,
            description: formData.description || '',
            priority_id: formData.priority_id || null,
            milestone_id: formData.milestone_id || null,
            assignee_id: formData.assignee_id || null,
            due_date: formData.due_date || null,
            status: 'open',
            item_type_id: formData.item_type_id,
            parent_id: parentItem ? parentItem.id : null,
            custom_field_values: customFieldValues,
          };

          let result;
          try {
            result = await api.items.create(itemData);
          } catch (error) {
            console.error('API create error:', error);
            throw error;
          }

          window.dispatchEvent(new CustomEvent('refresh-work-items', { detail: { itemId: result.id } }));
          dispatch('created', result);

          if (shouldNavigateAfterCreate($currentRoute.view)) {
            navigate(`/workspaces/${selectedWorkspace.id}/items/${result.id}`);
          }
        }
        close();
      } else if (selectedType === 'milestone') {
        await milestonesStore.add({
          name: formData.name,
          description: formData.description,
          target_date: formData.target_date,
          status: formData.status,
          category_id: null
        });

        navigate('/milestones');
        close();
      } else if (selectedType === 'workspace') {
        const workspaceData = {
          name: formData.name,
          key: formData.key,
          description: formData.description || '',
          active: true
        };

        const result = await api.workspaces.create(workspaceData);
        window.dispatchEvent(new CustomEvent('refresh-workspaces'));
        navigate(`/workspaces/${result.id}`);
        close();
      } else if (selectedType === 'collection') {
        const collectionData = {
          name: formData.name,
          description: formData.description || '',
          cql_query: '',
          is_public: false,
          workspace_id: formData.workspace_id,
          category_id: collectionCategoryId
        };

        const result = await api.collections.create(collectionData);
        navigate(`/collections/${result.id}`);
        close();
      }
    } catch (error) {
      console.error('Failed to create item:', error);
      const errorMsg = error.message || String(error);
      if (errorMsg.includes('UNIQUE constraint failed: workspaces.key')) {
        errorToast('A workspace with this key already exists. Please choose a different key.');
      } else {
        errorToast(`Failed to create ${currentTypeName.toLowerCase()}: ${errorMsg}`);
      }
    }
  }

  function handleBackdropClick(e) {
    if (e.target === e.currentTarget) {
      close();
    }
  }

  function handleKeydown(e) {
    if (matchesShortcut(e, cancelShortcut)) {
      close();
    }
    if (matchesShortcut(e, submitShortcut)) {
      e.preventDefault();
      if (!formData.name.trim() ||
          (selectedType === 'milestone' && !formData.target_date) ||
          (selectedType === 'work-item' && !formData.workspace_id) ||
          (selectedType === 'workspace' && !formData.key.trim())) {
        return;
      }
      handleSubmit();
    }
  }

  // Focus first input when modal opens
  let nameInputRef;
  $effect(() => {
    if (isOpen && nameInputRef) {
      setTimeout(() => {
        nameInputRef.focus();
      }, 100);
    }
  });

  // Derived state for display
  let currentTypeName = $derived(typeLabels[selectedType] || 'Item');
  let selectedItemType = $derived(availableItemTypes.find(t => t.id === formData.item_type_id));
  let selectedPriority = $derived.by(() => {
    if (!formData.priority_id || !selectedWorkspace) return null;
    // Priority is loaded via PriorityPicker, we'll handle display there
    return null;
  });
  let selectedAssignee = $derived(users.find(u => u.id === formData.assignee_id));
  let selectedMilestone = $derived(milestones.find(m => m.id === formData.milestone_id));

  // Load workspaces when modal opens
  $effect(() => {
    if (isOpen && !$workspacesStore.loaded && $workspacesStore.regularWorkspaces.length === 0) {
      loadWorkspaces();
    }
  });

  // Load custom fields when modal opens for work items
  $effect(() => {
    if (isOpen && selectedType === 'work-item' && allCustomFields.length === 0) {
      loadCustomFields();
    }
  });

  // Load milestones when modal opens for work items
  $effect(() => {
    if (isOpen && selectedType === 'work-item' && !milestonesLoaded) {
      loadMilestones();
    }
  });

  // Load item types when modal opens for work items
  $effect(() => {
    if (isOpen && selectedType === 'work-item' && !itemTypesLoaded) {
      loadItemTypes();
    }
  });

  // Load users when modal opens for work items
  $effect(() => {
    if (isOpen && selectedType === 'work-item' && !usersLoaded) {
      loadUsers();
    }
  });

  // Force work-item type when compact mode is enabled
  $effect(() => {
    if (compactMode && selectedType !== 'work-item') {
      selectedType = 'work-item';
    }
  });

  // Auto-select first workspace if only one exists
  $effect(() => {
    if (selectedType === 'work-item' && $workspacesStore.regularWorkspaces.length === 1 && !formData.workspace_id) {
      selectedWorkspace = $workspacesStore.regularWorkspaces[0];
      formData.workspace_id = $workspacesStore.regularWorkspaces[0].id;
    }
  });

  // Load config set when workspace changes
  $effect(() => {
    if (selectedType === 'work-item' && formData.workspace_id && configSetLoadedForWorkspace !== formData.workspace_id) {
      loadConfigSetForWorkspace(formData.workspace_id);
    }
  });

  // Load screen fields when workspace or item type changes
  $effect(() => {
    console.log('[CreateModal] Screen field effect check:', {
      selectedType,
      workspace_id: formData.workspace_id,
      item_type_id: formData.item_type_id,
      customFieldsLoaded,
      configSetLoadedForWorkspace,
      match: configSetLoadedForWorkspace === formData.workspace_id
    });
    if (selectedType === 'work-item' && formData.workspace_id && formData.item_type_id && customFieldsLoaded && configSetLoadedForWorkspace === formData.workspace_id) {
      const key = `${formData.workspace_id}-${formData.item_type_id}`;
      if (screenFieldsLoadedForKey !== key) {
        loadScreenFieldsForItemType(formData.workspace_id, formData.item_type_id);
      }
    }
  });

  // Debug: Log field configuration status
  $effect(() => {
    if (selectedType === 'work-item' && screenSystemFields.length > 0) {
      console.log('[CreateModal] Field config check:', {
        priority: isFieldConfigured('priority'),
        assignee: isFieldConfigured('assignee'),
        milestone: isFieldConfigured('milestone'),
        due_date: isFieldConfigured('due_date'),
        screenSystemFields
      });
    }
  });

  // Apply stored workspace preference
  $effect(() => {
    if (
      isOpen &&
      (selectedType === 'work-item' || parentItem) &&
      !formData.workspace_id &&
      storedWorkspaceId &&
      $workspacesStore.regularWorkspaces.length > 0
    ) {
      const storedWorkspace = $workspacesStore.regularWorkspaces.find(w => w.id === storedWorkspaceId);
      if (storedWorkspace) {
        selectedWorkspace = storedWorkspace;
        formData.workspace_id = storedWorkspace.id;
      }
    }
  });

  // Apply stored item type preference
  $effect(() => {
    if (
      isOpen &&
      (selectedType === 'work-item' || parentItem) &&
      storedItemTypeId &&
      availableItemTypes.length > 0 &&
      !storedItemTypeApplied
    ) {
      const storedItemType = availableItemTypes.find(type => type.id === storedItemTypeId);
      if (storedItemType) {
        formData.item_type_id = storedItemType.id;
      }
      storedItemTypeApplied = true;
    }
  });

  // Apply config set default item type
  $effect(() => {
    if (
      isOpen &&
      (selectedType === 'work-item' || parentItem) &&
      availableItemTypes.length > 0 &&
      currentConfigSet?.default_item_type_id &&
      !configSetDefaultApplied
    ) {
      const hasValidStoredType = storedItemTypeId && availableItemTypes.find(type => type.id === storedItemTypeId);
      if (!hasValidStoredType) {
        const configDefault = availableItemTypes.find(type => type.id === currentConfigSet.default_item_type_id);
        if (configDefault) {
          formData.item_type_id = configDefault.id;
        }
      }
      configSetDefaultApplied = true;
    }
  });

  // Persist workspace selection
  $effect(() => {
    if (
      isOpen &&
      (selectedType === 'work-item' || parentItem) &&
      selectedWorkspace?.id &&
      selectedWorkspace.id !== lastPersistedWorkspaceId
    ) {
      lastPersistedWorkspaceId = selectedWorkspace.id;
      persistWorkspaceSelection(selectedWorkspace.id);
    }
  });

  // Persist item type selection
  $effect(() => {
    if (
      isOpen &&
      (selectedType === 'work-item' || parentItem) &&
      formData.item_type_id &&
      formData.item_type_id !== lastPersistedItemTypeId
    ) {
      lastPersistedItemTypeId = formData.item_type_id;
      persistItemTypeSelection(formData.item_type_id);
    }
  });

  // Listen for create type changes from command palette
  function handleSetCreateType(event) {
    if (event.detail?.type) {
      selectedType = event.detail.type;
      resetForm();
      if (event.detail.type === 'work-item') {
        if ($workspacesStore.regularWorkspaces.length === 0) {
          loadWorkspaces();
        }
        if (!itemTypesLoaded) {
          loadItemTypes();
        }
        if (!usersLoaded) {
          loadUsers();
        }
      }
    }
  }

  function handleSetCreateWorkspace(event) {
    if (event.detail?.workspaceId) {
      const workspaceId = event.detail.workspaceId;
      const workspaceIdNum = typeof workspaceId === 'string' ? parseInt(workspaceId, 10) : workspaceId;
      formData.workspace_id = workspaceIdNum;

      if ($workspacesStore.regularWorkspaces.length === 0) {
        loadWorkspaces().then(() => {
          selectedWorkspace = $workspacesStore.regularWorkspaces.find(w => w.id === workspaceIdNum);
          loadConfigSetForWorkspace(workspaceIdNum);
        });
      } else {
        selectedWorkspace = $workspacesStore.regularWorkspaces.find(w => w.id === workspaceIdNum);
        loadConfigSetForWorkspace(workspaceIdNum);
      }
    }
  }

  function handleSetCreateParent(event) {
    if (event.detail?.parentId) {
      parentItem = {
        id: event.detail.parentId,
        title: event.detail.parentTitle
      };
      restrictedItemTypes = event.detail.availableItemTypes || null;

      if (itemTypesLoaded) {
        loadItemTypes(true);
      }
    }
  }

  async function handleOpenCreateModal(event) {
    isOpen = true;
    if ($workspacesStore.regularWorkspaces.length === 0) {
      await loadWorkspaces();
    }
  }

  onMount(() => {
    loadStoredSelections();
    window.addEventListener('open-create-modal', handleOpenCreateModal);
    window.addEventListener('set-create-type', handleSetCreateType);
    window.addEventListener('set-create-workspace', handleSetCreateWorkspace);
    window.addEventListener('set-create-parent', handleSetCreateParent);

    return () => {
      window.removeEventListener('open-create-modal', handleOpenCreateModal);
      window.removeEventListener('set-create-type', handleSetCreateType);
      window.removeEventListener('set-create-workspace', handleSetCreateWorkspace);
      window.removeEventListener('set-create-parent', handleSetCreateParent);
    };
  });

  // Format due date for display
  function formatDueDate(dateStr) {
    if (!dateStr) return null;
    const date = new Date(dateStr);
    const today = new Date();
    const diffDays = Math.ceil((date - today) / (1000 * 60 * 60 * 24));
    if (diffDays === 0) return 'Today';
    if (diffDays === 1) return 'Tomorrow';
    if (diffDays === -1) return 'Yesterday';
    if (diffDays > 0 && diffDays <= 7) return `${diffDays} days`;
    return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
  }
</script>

{#if isOpen}
  <!-- Backdrop -->
  <div
    transition:fade={{ duration: 150 }}
    class="fixed inset-0 flex items-start justify-center pt-16 overflow-y-auto z-50"
    style="background-color: rgba(0, 0, 0, 0.4); backdrop-filter: blur(2px);"
    tabindex="-1"
    onclick={handleBackdropClick}
    onkeydown={handleKeydown}
    role="dialog"
    aria-modal="true"
  >
    <!-- Modal -->
    <div class="rounded-xl shadow-2xl w-full max-w-lg mx-4 mb-8 flex flex-col" style="background-color: var(--ds-surface-raised);">
      <!-- Compact Header -->
      <div class="flex items-center gap-2 px-4 py-3 border-b" style="border-color: var(--ds-border);">
        <!-- Type Selector FIRST (independent of workspace) -->
        {#if !parentItem && !compactMode}
          <FieldChip
            label="Type"
            value={selectedType}
            displayValue={typeLabels[selectedType]}
            icon={typeIcons[selectedType]}
            placeholder="Type"
          >
            {#snippet children({ close: closePopover })}
              <div class="py-1">
                {#each typeOptions as type}
                  <button
                    type="button"
                    class="w-full flex items-center gap-3 px-3 py-2.5 text-left transition-colors"
                    style="color: var(--ds-text); background-color: {selectedType === type.value ? 'var(--ds-background-selected)' : 'transparent'};"
                    onmouseover={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-selected)'}
                    onmouseout={(e) => e.currentTarget.style.backgroundColor = selectedType === type.value ? 'var(--ds-background-selected)' : 'transparent'}
                    onclick={() => {
                      selectType(type.value);
                      closePopover();
                    }}
                  >
                    <svelte:component this={type.icon} size={16} style="color: var(--ds-text-subtle);" />
                    <span class="font-medium">{type.label}</span>
                  </button>
                {/each}
              </div>
            {/snippet}
          </FieldChip>
          <ChevronRight size={14} style="color: var(--ds-text-subtle);" />
        {/if}

        <!-- Workspace Selector (only for work-items) -->
        {#if selectedType === 'work-item' && !parentItem}
          <CompactWorkspaceSelector
            bind:value={formData.workspace_id}
            workspaces={$workspacesStore.regularWorkspaces}
            onSelect={(workspace) => {
              if (workspace) {
                selectedWorkspace = workspace;
                formData.workspace_id = workspace.id;
                if (workspace.id) {
                  loadConfigSetForWorkspace(workspace.id);
                }
              }
            }}
          />
          <ChevronRight size={14} style="color: var(--ds-text-subtle);" />
        {/if}

        <span class="font-medium" style="color: var(--ds-text);">
          {#if parentItem}
            New Child Item
          {:else}
            New {currentTypeName}
          {/if}
        </span>

        <button
          onclick={close}
          class="ml-auto p-1.5 rounded transition-colors"
          style="color: var(--ds-text-subtle);"
          onmouseover={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
          onmouseout={(e) => e.currentTarget.style.backgroundColor = 'transparent'}
          aria-label="Close"
        >
          <X size={16} />
        </button>
      </div>

      <!-- Body -->
      <div class="px-4 py-3 space-y-3">
        <!-- Parent Item Info (for sub-issues) -->
        {#if parentItem}
          <div class="text-xs px-2 py-1.5 rounded" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
            Parent: {parentItem.title}
          </div>
        {/if}

        <!-- Borderless Title Input -->
        <input
          bind:this={nameInputRef}
          bind:value={formData.name}
          type="text"
          class="w-full text-lg font-medium border-0 outline-none bg-transparent"
          style="color: var(--ds-text);"
          placeholder={selectedType === 'work-item' ? 'Issue title' : `${currentTypeName} name`}
        />

        <!-- Workspace Key (for workspace creation) -->
        {#if selectedType === 'workspace'}
          <input
            bind:value={formData.key}
            type="text"
            class="w-full text-sm border-0 outline-none bg-transparent"
            style="color: var(--ds-text-subtle);"
            placeholder="Workspace key (e.g., PROJ, TEAM)"
          />
        {/if}

        <!-- Compact Description -->
        <div class="min-h-[60px]">
          <MilkdownEditor
            bind:content={formData.description}
            placeholder="Add description..."
            compact={true}
            showToolbar={false}
            readonly={false}
            itemId={null}
          />
        </div>

        <!-- Field Chips Row (for work items) -->
        {#if selectedType === 'work-item'}
          <div class="flex flex-wrap items-center gap-2 pt-2 border-t" style="border-color: var(--ds-border);">
            <!-- Item Type Chip -->
            {#if availableItemTypes.length >= 1}
              <FieldChip
                bind:this={itemTypeChipRef}
                label="Type"
                value={formData.item_type_id}
                displayValue={selectedItemType?.name || ''}
                icon={Layers}
                placeholder="Type"
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

            <!-- Priority Chip (only if configured and NOT required) -->
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
                  unassignedLabel="No priority"
                >
                  {#snippet children()}
                    <div
                      class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-sm transition-colors"
                      style="
                        background-color: var(--ds-surface);
                        border: 1px solid var(--ds-border);
                        color: {formData.priority_id ? 'var(--ds-text)' : 'var(--ds-text-subtle)'};
                      "
                      onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-surface-hovered, var(--ds-background-neutral-hovered))'}
                      onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-surface)'}
                    >
                      <Flag size={14} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
                      <span class="truncate max-w-[120px]">
                        {selectedPriorityObj?.name || 'Priority'}
                      </span>
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
                  <span>Priority</span>
                  <ChevronDown size={12} style="flex-shrink: 0;" />
                </div>
              {/if}
            {/if}

            <!-- Assignee Chip (only if configured and NOT required) -->
            {#if isFieldConfigured('assignee') && !isFieldRequired('assignee')}
              <UserPicker
                bind:value={formData.assignee_id}
                showUnassigned={true}
                unassignedLabel="Unassigned"
              >
                {#snippet children()}
                  <div
                    class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-sm transition-colors"
                    style="
                      background-color: var(--ds-surface);
                      border: 1px solid var(--ds-border);
                      color: {formData.assignee_id ? 'var(--ds-text)' : 'var(--ds-text-subtle)'};
                    "
                    onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-surface-hovered, var(--ds-background-neutral-hovered))'}
                    onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-surface)'}
                  >
                    <User size={14} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
                    <span class="truncate max-w-[120px]">
                      {selectedAssignee?.name || selectedAssignee?.email || 'Assignee'}
                    </span>
                    <ChevronDown size={12} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
                  </div>
                {/snippet}
              </UserPicker>
            {/if}

            <!-- Due Date Chip (only if configured and NOT required) -->
            {#if isFieldConfigured('due_date') && !isFieldRequired('due_date')}
              <button
                use:melt={$dueDateTrigger}
                class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-sm transition-colors"
                style="
                  background-color: var(--ds-surface);
                  border: 1px solid var(--ds-border);
                  color: {formData.due_date ? 'var(--ds-text)' : 'var(--ds-text-subtle)'};
                "
                onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-surface-hovered, var(--ds-background-neutral-hovered))'}
                onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-surface)'}
              >
                <Calendar size={14} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
                <span class="truncate max-w-[120px]">
                  {formData.due_date ? formatDueDate(formData.due_date) : 'Due date'}
                </span>
                <ChevronDown size={12} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
              </button>

              {#if $dueDateOpen}
                <div
                  use:melt={$dueDateContent}
                  class="z-50 rounded-lg shadow-lg p-3"
                  style="
                    background-color: var(--ds-surface-raised);
                    border: 1px solid var(--ds-border);
                  "
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

            <!-- Milestone Chip (only if configured and NOT required) -->
            {#if isFieldConfigured('milestone') && !isFieldRequired('milestone')}
              <MilestoneCombobox
                bind:value={formData.milestone_id}
                workspaceId={selectedWorkspace?.id}
                showUnassigned={true}
                unassignedLabel="No milestone"
              >
                {#snippet children()}
                  <div
                    class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-sm transition-colors"
                    style="
                      background-color: var(--ds-surface);
                      border: 1px solid var(--ds-border);
                      color: {formData.milestone_id ? 'var(--ds-text)' : 'var(--ds-text-subtle)'};
                    "
                    onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-surface-hovered, var(--ds-background-neutral-hovered))'}
                    onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-surface)'}
                  >
                    {#if selectedMilestone?.category_color}
                      <div class="w-2 h-2 rounded-full flex-shrink-0" style="background-color: {selectedMilestone.category_color};"></div>
                    {:else}
                      <MilestoneIcon size={14} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
                    {/if}
                    <span class="truncate max-w-[120px]">
                      {selectedMilestone?.name || 'Milestone'}
                    </span>
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
                    Additional Fields
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
                      />
                    </div>
                  {/each}
                </div>
              {/if}
            {/if}
          </div>

          <!-- Required System Fields Section (old style, full-width) -->
          {#if requiredSystemFields.length > 0}
            <div class="space-y-3 pt-3 border-t" style="border-color: var(--ds-border);">
              {#each requiredSystemFields as field}
                {#if field.field_identifier === 'priority'}
                  <div class="space-y-1">
                    <Label color="default">
                      Priority <span style="color: var(--ds-text-danger, #ef4444);">*</span>
                    </Label>
                    {#if selectedWorkspace}
                      <PriorityPicker
                        workspaceId={selectedWorkspace.id}
                        selectedPriorityId={formData.priority_id}
                        onChange={(priorityId) => formData.priority_id = priorityId}
                        placeholder="Select priority"
                      />
                    {:else}
                      <div class="px-3 py-2 text-sm rounded border" style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text-subtle);">
                        Select a workspace first
                      </div>
                    {/if}
                  </div>
                {:else if field.field_identifier === 'due_date'}
                  <div class="space-y-1">
                    <Label color="default">
                      Due Date <span style="color: var(--ds-text-danger, #ef4444);">*</span>
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
                      Milestone <span style="color: var(--ds-text-danger, #ef4444);">*</span>
                    </Label>
                    <MilestoneCombobox
                      bind:value={formData.milestone_id}
                      workspaceId={selectedWorkspace?.id}
                      placeholder="Select milestone"
                    />
                  </div>
                {:else if field.field_identifier === 'assignee'}
                  <div class="space-y-1">
                    <Label color="default">
                      Assignee <span style="color: var(--ds-text-danger, #ef4444);">*</span>
                    </Label>
                    <UserPicker
                      bind:value={formData.assignee_id}
                      placeholder="Select assignee"
                    />
                  </div>
                {/if}
              {/each}
            </div>
          {/if}

          <!-- Required Custom Fields Section (old style, full-width) -->
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
        {/if}

        <!-- Milestone-specific chips -->
        {#if selectedType === 'milestone'}
          <div class="flex flex-wrap items-center gap-2 pt-2 border-t" style="border-color: var(--ds-border);">
            <!-- Target Date Chip -->
            <FieldChip
              label="Target Date"
              value={formData.target_date}
              displayValue={formData.target_date ? new Date(formData.target_date).toLocaleDateString('en-US', { month: 'short', day: 'numeric' }) : ''}
              icon={Calendar}
              placeholder="Target date"
              required={true}
            >
              {#snippet children({ close: closePopover })}
                <div class="p-3">
                  <input
                    type="date"
                    bind:value={formData.target_date}
                    class="w-full px-3 py-2 rounded border text-sm"
                    style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
                    onchange={() => closePopover()}
                  />
                </div>
              {/snippet}
            </FieldChip>

            <!-- Status Chip -->
            <FieldChip
              bind:this={statusChipRef}
              label="Status"
              value={formData.status}
              displayValue={milestoneStatusOptions.find(s => s.value === formData.status)?.label || 'Planning'}
              icon={Target}
              placeholder="Status"
            >
              {#snippet children({ close: closePopover })}
                <div class="p-2 max-h-48 overflow-y-auto">
                  {#each milestoneStatusOptions as status}
                    <button
                      type="button"
                      class="w-full px-3 py-2 text-left text-sm rounded transition-colors"
                      style="color: var(--ds-text);"
                      onmouseover={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-selected)'}
                      onmouseout={(e) => e.currentTarget.style.backgroundColor = 'transparent'}
                      onclick={() => {
                        formData.status = status.value;
                        closePopover();
                      }}
                    >
                      {status.label}
                    </button>
                  {/each}
                </div>
              {/snippet}
            </FieldChip>
          </div>
        {/if}

        <!-- Collection-specific chips -->
        {#if selectedType === 'collection'}
          <div class="flex flex-wrap items-center gap-2 pt-2 border-t" style="border-color: var(--ds-border);">
            <FieldChip
              bind:this={categoryChipRef}
              label="Category"
              value={collectionCategoryId}
              displayValue={$collectionCategoriesStore.find(c => c.id === collectionCategoryId)?.name || ''}
              icon={FolderOpen}
              placeholder="Category"
            >
              {#snippet children({ close: closePopover })}
                <div class="p-2 max-h-48 overflow-y-auto">
                  <button
                    type="button"
                    class="w-full px-3 py-2 text-left text-sm rounded transition-colors"
                    style="color: var(--ds-text-subtle);"
                    onmouseover={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-selected)'}
                    onmouseout={(e) => e.currentTarget.style.backgroundColor = 'transparent'}
                    onclick={() => {
                      collectionCategoryId = null;
                      closePopover();
                    }}
                  >
                    No Category
                  </button>
                  {#each $collectionCategoriesStore as category}
                    <button
                      type="button"
                      class="w-full px-3 py-2 text-left text-sm rounded transition-colors"
                      style="color: var(--ds-text);"
                      onmouseover={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-selected)'}
                      onmouseout={(e) => e.currentTarget.style.backgroundColor = 'transparent'}
                      onclick={() => {
                        collectionCategoryId = category.id;
                        closePopover();
                      }}
                    >
                      {category.name}
                    </button>
                  {/each}
                </div>
              {/snippet}
            </FieldChip>
          </div>
        {/if}
      </div>

      <!-- Footer -->
      <div class="flex items-center justify-end px-4 py-3 border-t" style="border-color: var(--ds-border);">
        <Button
          onclick={handleSubmit}
          variant="primary"
          size="medium"
          keyboardHint={getDisplayString(submitShortcut)}
          disabled={!formData.name.trim() ||
                   (selectedType === 'milestone' && !formData.target_date) ||
                   (selectedType === 'work-item' && !formData.workspace_id) ||
                   (selectedType === 'workspace' && !formData.key.trim())}
        >
          Create {currentTypeName}
        </Button>
      </div>
    </div>
  </div>
{/if}
