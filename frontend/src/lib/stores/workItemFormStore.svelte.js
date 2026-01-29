/**
 * Store for managing Work Item Form state.
 * Uses Svelte 5 class-based reactive state pattern.
 * Centralizes form data, validation, data loading, and selection persistence.
 */
import { api } from '../api.js';
import { workspacesStore } from './workspaces.svelte.js';
import { getSystemFieldName } from './fieldConfig.js';

const STORAGE_KEYS = {
  workspace: 'vertex_create_modal_workspace',
  itemType: 'vertex_create_modal_item_type'
};

// System fields that are auto-managed and should not be shown in create form
const EXCLUDED_SYSTEM_FIELDS = ['status'];

class WorkItemFormStore {
  // === Form Data ===
  formData = $state({
    name: '',
    description: '',
    due_date: '',
    workspace_id: null,
    priority_id: null,
    milestone_id: null,
    assignee_id: null,
    item_type_id: null
  });
  customFieldValues = $state({});
  validationErrors = $state([]);

  // === Selection Context ===
  selectedWorkspace = $state(null);
  parentItem = $state(null);
  restrictedItemTypes = $state(null);

  // === Data Loading State ===
  users = $state([]);
  usersLoaded = $state(false);

  allMilestones = $state([]);
  milestones = $state([]);
  milestonesLoading = $state(false);
  milestonesLoaded = $state(false);

  itemTypes = $state([]);
  hierarchyLevels = $state([]);
  availableItemTypes = $state([]);
  itemTypesLoaded = $state(false);

  allCustomFields = $state([]);
  customFields = $state([]);
  customFieldsLoaded = $state(false);

  screenFields = $state([]);
  screenSystemFields = $state([]);
  loadingScreenFields = $state(false);

  workspaceDetails = $state(null);
  currentConfigSet = $state(null);

  // === Cache Keys ===
  configSetLoadedForWorkspace = $state(null);
  screenFieldsLoadedForKey = $state(null);

  // === Persistence State ===
  storedWorkspaceId = $state(null);
  storedItemTypeId = $state(null);
  lastPersistedWorkspaceId = $state(null);
  lastPersistedItemTypeId = $state(null);
  storedItemTypeApplied = $state(false);
  configSetDefaultApplied = $state(false);

  // === Initialization Flag ===
  #initialized = false;

  // === Derived Values (getters) ===

  /**
   * Get the currently selected item type object.
   */
  get selectedItemType() {
    return this.availableItemTypes.find(t => t.id === this.formData.item_type_id) || null;
  }

  /**
   * Get priorities from the loaded config set.
   */
  get configSetPriorities() {
    return this.currentConfigSet?.priorities_detailed?.length > 0
      ? this.currentConfigSet.priorities_detailed
      : null;
  }

  /**
   * Get the currently selected assignee object.
   */
  get selectedAssignee() {
    return this.users.find(u => u.id === this.formData.assignee_id) || null;
  }

  /**
   * Get the currently selected milestone object.
   */
  get selectedMilestone() {
    return this.milestones.find(m => m.id === this.formData.milestone_id) || null;
  }

  /**
   * Get non-required custom fields for the overflow menu.
   */
  get nonRequiredCustomFields() {
    return this.customFields.filter(cf => {
      const screenField = this.screenFields.find(
        f => f.field_type === 'custom' && parseInt(f.field_identifier) === cf.id
      );
      return !screenField?.is_required;
    });
  }

  /**
   * Get required system fields that should be shown as full inputs.
   */
  get requiredSystemFields() {
    return this.screenFields.filter(f =>
      f.is_required &&
      f.field_type === 'system' &&
      !EXCLUDED_SYSTEM_FIELDS.includes(f.field_identifier)
    );
  }

  /**
   * Get required custom fields that should be shown as full inputs.
   */
  get requiredCustomFields() {
    return this.customFields.filter(cf => {
      const screenField = this.screenFields.find(
        f => f.field_type === 'custom' && parseInt(f.field_identifier) === cf.id
      );
      return screenField?.is_required === true;
    });
  }

  // === Data Loading Methods ===

  /**
   * Load all users.
   */
  async loadUsers() {
    if (this.usersLoaded) return;
    try {
      const result = await api.getUsers();
      this.users = result || [];
      this.usersLoaded = true;
    } catch (error) {
      console.error('Failed to load users:', error);
      this.users = [];
      this.usersLoaded = true;
    }
  }

  /**
   * Load all custom fields.
   */
  async loadCustomFields() {
    if (this.customFieldsLoaded) return;
    try {
      const result = await api.customFields.getAll();
      this.allCustomFields = result || [];
      this.customFieldsLoaded = true;
    } catch (error) {
      console.error('Failed to load custom fields:', error);
      this.allCustomFields = [];
      this.customFields = [];
      this.customFieldsLoaded = true;
    }
  }

  /**
   * Load all milestones.
   */
  async loadMilestones() {
    if (this.milestonesLoading || this.milestonesLoaded) return;
    try {
      this.milestonesLoading = true;
      const result = await api.milestones.getAll();
      this.allMilestones = result || [];
      this.#filterMilestones();
      this.milestonesLoaded = true;
    } catch (error) {
      console.error('Failed to load milestones:', error);
      this.allMilestones = [];
      this.milestones = [];
      this.milestonesLoaded = true;
    } finally {
      this.milestonesLoading = false;
    }
  }

  /**
   * Filter milestones based on workspace categories.
   */
  #filterMilestones() {
    if (!this.workspaceDetails?.milestone_categories?.length) {
      this.milestones = this.allMilestones;
    } else {
      const allowedCategoryIds = this.workspaceDetails.milestone_categories;
      this.milestones = this.allMilestones.filter(m => allowedCategoryIds.includes(m.category_id));
    }
  }

  /**
   * Load workspace details and filter milestones.
   */
  async loadWorkspaceDetails(workspaceId) {
    if (!workspaceId) {
      this.workspaceDetails = null;
      this.#filterMilestones();
      return;
    }
    try {
      this.workspaceDetails = await api.workspaces.get(workspaceId);
      this.#filterMilestones();
    } catch (error) {
      console.error('Failed to load workspace details:', error);
      this.workspaceDetails = null;
      this.#filterMilestones();
    }
  }

  /**
   * Load all item types and hierarchy levels.
   */
  async loadItemTypes(forceReload = false) {
    if (this.itemTypesLoaded && !forceReload) return;
    try {
      const [itemTypesResult, hierarchyLevelsResult] = await Promise.all([
        api.itemTypes.getAll(),
        api.hierarchyLevels.getAll()
      ]);
      this.itemTypes = itemTypesResult || [];
      this.hierarchyLevels = hierarchyLevelsResult || [];

      this.#updateAvailableItemTypes();
      this.itemTypesLoaded = true;
    } catch (error) {
      console.error('Failed to load item types:', error);
      this.itemTypes = [];
      this.hierarchyLevels = [];
      this.availableItemTypes = [];
      this.itemTypesLoaded = true;
    }
  }

  /**
   * Update available item types based on restrictions and config set.
   */
  #updateAvailableItemTypes() {
    let baseTypes = this.itemTypes;

    // Apply restricted item types if set (child item creation)
    if (this.restrictedItemTypes?.length > 0) {
      baseTypes = this.restrictedItemTypes;
    }

    // Apply config set item type restrictions
    if (this.currentConfigSet?.item_type_configs?.length > 0) {
      const allowedItemTypeIds = this.currentConfigSet.item_type_configs.map(c => c.item_type_id);
      baseTypes = baseTypes.filter(t => allowedItemTypeIds.includes(t.id));
    }

    this.availableItemTypes = baseTypes.sort(
      (a, b) => a.hierarchy_level - b.hierarchy_level || a.sort_order - b.sort_order
    );

    // Auto-select first item type if current is invalid
    if (this.availableItemTypes.length > 0 && !this.availableItemTypes.find(t => t.id === this.formData.item_type_id)) {
      this.formData.item_type_id = this.availableItemTypes[0].id;
    }
  }

  /**
   * Load configuration set for a workspace.
   */
  async loadConfigSetForWorkspace(workspaceId) {
    if (this.configSetLoadedForWorkspace === workspaceId) return;
    try {
      const response = await api.configurationSets.getAll();
      const configSets = response?.configuration_sets || [];
      this.currentConfigSet = null;
      let defaultConfigSet = null;

      for (const configSet of configSets) {
        if (configSet.is_default) defaultConfigSet = configSet;
        if (configSet.workspace_ids?.includes(workspaceId)) {
          this.currentConfigSet = await api.configurationSets.get(configSet.id);
          break;
        }
      }

      if (!this.currentConfigSet && defaultConfigSet) {
        this.currentConfigSet = await api.configurationSets.get(defaultConfigSet.id);
      }

      this.configSetLoadedForWorkspace = workspaceId;
      this.#updateAvailableItemTypes();
    } catch (error) {
      console.error('Failed to load config set:', error);
      this.currentConfigSet = null;
      this.configSetLoadedForWorkspace = workspaceId;
      this.#updateAvailableItemTypes();
    }
  }

  /**
   * Resolve the create screen ID for an item type.
   */
  #resolveCreateScreenId(itemTypeId) {
    if (this.currentConfigSet) {
      const itemTypeConfig = this.currentConfigSet.item_type_configs?.find(c => c.item_type_id === itemTypeId);
      if (itemTypeConfig?.create_screen_id) return itemTypeConfig.create_screen_id;
      if (this.currentConfigSet.create_screen_id) return this.currentConfigSet.create_screen_id;
    }
    return 1;
  }

  /**
   * Load screen fields for a specific workspace/item type combination.
   */
  async loadScreenFieldsForItemType(workspaceId, itemTypeId) {
    const key = `${workspaceId}-${itemTypeId}`;
    if (this.loadingScreenFields || this.screenFieldsLoadedForKey === key) return;
    try {
      this.loadingScreenFields = true;
      const createScreenId = this.#resolveCreateScreenId(itemTypeId);
      const fields = await api.screens.getFields(createScreenId);
      this.screenFields = fields || [];

      this.screenSystemFields = this.screenFields
        .filter(field => field.field_type === 'system')
        .map(field => field.field_identifier);

      const customFieldIds = this.screenFields
        .filter(field => field.field_type === 'custom')
        .map(field => parseInt(field.field_identifier));

      const filteredCustomFields = this.allCustomFields.filter(field => customFieldIds.includes(field.id));

      // Reset custom field values for new fields
      this.customFieldValues = {};
      filteredCustomFields.forEach(field => {
        this.customFieldValues[field.id] = '';
      });

      this.customFields = filteredCustomFields;
      this.screenFieldsLoadedForKey = key;
    } catch (error) {
      console.error('Failed to load screen fields:', error);
      this.screenSystemFields = ['priority', 'milestone'];
      this.screenFields = [];
      this.customFields = [];
      this.customFieldValues = {};
      this.screenFieldsLoadedForKey = key;
    } finally {
      this.loadingScreenFields = false;
    }
  }

  // === Field Helpers ===

  /**
   * Check if a system field is required.
   */
  isFieldRequired(fieldIdentifier) {
    const screenField = this.screenFields.find(f => f.field_identifier === fieldIdentifier);
    return screenField?.is_required === true;
  }

  /**
   * Check if a system field is configured (in screen fields).
   */
  isFieldConfigured(fieldIdentifier) {
    return this.screenSystemFields.includes(fieldIdentifier);
  }

  // === Selection Methods ===

  /**
   * Set the selected workspace.
   */
  setWorkspace(workspace) {
    this.selectedWorkspace = workspace;
    this.formData.workspace_id = workspace?.id || null;

    if (workspace?.id) {
      this.#persistWorkspaceSelection(workspace.id);
      this.loadWorkspaceDetails(workspace.id);
      this.loadConfigSetForWorkspace(workspace.id);
    } else {
      this.workspaceDetails = null;
      this.#filterMilestones();
    }
  }

  /**
   * Set the selected item type.
   */
  setItemType(itemTypeId) {
    this.formData.item_type_id = itemTypeId;
    this.#persistItemTypeSelection(itemTypeId);
  }

  /**
   * Set the parent item context (for child item creation).
   */
  setParentItem(parent, allowedItemTypes = null) {
    this.parentItem = parent;
    this.restrictedItemTypes = allowedItemTypes;
    this.#updateAvailableItemTypes();
  }

  // === Persistence ===

  /**
   * Load stored selections from localStorage.
   */
  loadStoredSelections() {
    if (typeof window === 'undefined') return;
    try {
      const workspaceValue = window.localStorage.getItem(STORAGE_KEYS.workspace);
      if (workspaceValue) {
        const parsedWorkspace = parseInt(workspaceValue, 10);
        this.storedWorkspaceId = Number.isNaN(parsedWorkspace) ? null : parsedWorkspace;
      }
    } catch {
      this.storedWorkspaceId = null;
    }
    try {
      const itemTypeValue = window.localStorage.getItem(STORAGE_KEYS.itemType);
      if (itemTypeValue) {
        const parsedItemType = parseInt(itemTypeValue, 10);
        this.storedItemTypeId = Number.isNaN(parsedItemType) ? null : parsedItemType;
      }
    } catch {
      this.storedItemTypeId = null;
    }
  }

  #persistWorkspaceSelection(workspaceId) {
    if (typeof window === 'undefined' || !workspaceId) return;
    if (workspaceId === this.lastPersistedWorkspaceId) return;
    try {
      window.localStorage.setItem(STORAGE_KEYS.workspace, String(workspaceId));
      this.storedWorkspaceId = workspaceId;
      this.lastPersistedWorkspaceId = workspaceId;
    } catch {
      // Ignore localStorage errors
    }
  }

  #persistItemTypeSelection(itemTypeId) {
    if (typeof window === 'undefined' || !itemTypeId) return;
    if (itemTypeId === this.lastPersistedItemTypeId) return;
    try {
      window.localStorage.setItem(STORAGE_KEYS.itemType, String(itemTypeId));
      this.storedItemTypeId = itemTypeId;
      this.lastPersistedItemTypeId = itemTypeId;
    } catch {
      // Ignore localStorage errors
    }
  }

  /**
   * Apply stored workspace selection if available.
   */
  applyStoredWorkspace(workspaces) {
    if (!this.formData.workspace_id && this.storedWorkspaceId && workspaces.length > 0) {
      const storedWorkspace = workspaces.find(w => w.id === this.storedWorkspaceId);
      if (storedWorkspace) {
        this.setWorkspace(storedWorkspace);
      }
    }
  }

  /**
   * Apply stored item type selection if available.
   */
  applyStoredItemType() {
    // Don't apply stored item type when creating child items
    if (this.storedItemTypeId && this.availableItemTypes.length > 0 && !this.storedItemTypeApplied && !this.restrictedItemTypes) {
      const storedItemType = this.availableItemTypes.find(type => type.id === this.storedItemTypeId);
      if (storedItemType) {
        this.formData.item_type_id = storedItemType.id;
      }
      this.storedItemTypeApplied = true;
    }
  }

  /**
   * Apply config set default item type if no valid stored type.
   */
  applyConfigSetDefault() {
    if (this.availableItemTypes.length > 0 && this.currentConfigSet?.default_item_type_id && !this.configSetDefaultApplied) {
      const hasValidStoredType = this.storedItemTypeId && this.availableItemTypes.find(type => type.id === this.storedItemTypeId);
      if (!hasValidStoredType) {
        const configDefault = this.availableItemTypes.find(type => type.id === this.currentConfigSet.default_item_type_id);
        if (configDefault) {
          this.formData.item_type_id = configDefault.id;
        }
      }
      this.configSetDefaultApplied = true;
    }
  }

  // === Validation ===

  /**
   * Validate the form and return whether it's valid.
   */
  validate() {
    const errors = [];

    for (const field of this.screenFields) {
      if (field.is_required) {
        if (field.field_type === 'system') {
          const identifier = field.field_identifier;
          // Skip system-managed fields that are auto-assigned
          if (EXCLUDED_SYSTEM_FIELDS.includes(identifier)) {
            continue;
          }
          const fieldKeyMap = { 'title': 'name' };
          const formKey = fieldKeyMap[identifier] || identifier;
          const value = this.formData[formKey] ?? this.formData[`${formKey}_id`];
          if (!value) {
            errors.push(`${getSystemFieldName(identifier)} is required`);
          }
        } else if (field.field_type === 'custom') {
          const fieldId = parseInt(field.field_identifier);
          const value = this.customFieldValues[fieldId];
          if (value === undefined || value === null || value === '') {
            const fieldDef = this.allCustomFields.find(f => f.id === fieldId);
            errors.push(`${fieldDef?.name || 'Custom field'} is required`);
          }
        }
      }
    }

    this.validationErrors = errors;
    return errors.length === 0;
  }

  // === Form Data for API ===

  /**
   * Get form data formatted for the API.
   */
  getFormData() {
    return {
      workspace_id: this.selectedWorkspace?.id || this.formData.workspace_id,
      title: this.formData.name,
      description: this.formData.description || '',
      priority_id: this.formData.priority_id || null,
      milestone_id: this.formData.milestone_id || null,
      assignee_id: this.formData.assignee_id || null,
      due_date: this.formData.due_date ? new Date(this.formData.due_date).toISOString() : null,
      status: 'open',
      item_type_id: this.formData.item_type_id,
      parent_id: this.parentItem?.id || null,
      custom_field_values: this.customFieldValues
    };
  }

  // === Reset ===

  /**
   * Reset form state while keeping loaded reference data.
   */
  resetForm() {
    this.formData = {
      name: '',
      description: '',
      due_date: '',
      workspace_id: null,
      priority_id: null,
      milestone_id: null,
      assignee_id: null,
      item_type_id: this.availableItemTypes.length > 0 ? this.availableItemTypes[0].id : null
    };
    this.customFieldValues = {};
    this.validationErrors = [];
    this.selectedWorkspace = null;
    this.parentItem = null;
    this.restrictedItemTypes = null;
    this.workspaceDetails = null;

    // Reset cache keys to force reload for new workspace/item type
    this.configSetLoadedForWorkspace = null;
    this.screenFieldsLoadedForKey = null;
    this.storedItemTypeApplied = false;
    this.configSetDefaultApplied = false;

    // Keep loaded data (users, milestones, itemTypes, customFields, etc.)
  }

  /**
   * Full reset including all loaded data.
   */
  reset() {
    this.resetForm();

    // Reset all loaded data
    this.users = [];
    this.usersLoaded = false;
    this.allMilestones = [];
    this.milestones = [];
    this.milestonesLoading = false;
    this.milestonesLoaded = false;
    this.itemTypes = [];
    this.hierarchyLevels = [];
    this.availableItemTypes = [];
    this.itemTypesLoaded = false;
    this.allCustomFields = [];
    this.customFields = [];
    this.customFieldsLoaded = false;
    this.screenFields = [];
    this.screenSystemFields = [];
    this.loadingScreenFields = false;
    this.currentConfigSet = null;
    this.#initialized = false;
  }

  // === Initialize ===

  /**
   * Initialize the store (called when form opens).
   * Loads reference data if not already loaded.
   */
  async init() {
    if (this.#initialized) return;

    this.loadStoredSelections();
    await Promise.all([
      this.loadUsers(),
      this.loadMilestones(),
      this.loadItemTypes(),
      this.loadCustomFields()
    ]);
    this.#initialized = true;
  }

  /**
   * Ensure store is ready to use (call before rendering form).
   */
  async ensureReady() {
    await this.init();
  }
}

export const workItemFormStore = new WorkItemFormStore();
