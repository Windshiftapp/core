// Centralized reactive state for the work-item create form.
import { api } from '../api.js';
import { isBooleanCustomFieldType } from '../utils/customFieldTypes.js';
import { dateInputToISOString } from '../utils/dateFormatter.js';
import { isGenericSubtaskType, sortItemTypesByHierarchy } from '../utils/hierarchy.js';
import {
  isCreateSystemFieldAutoManaged,
  isCreateSystemFieldRenderable,
  isSystemFieldConfigured,
  resolveEffectiveScreenIds,
  systemFieldIdentifiers,
} from '../utils/screenFields.js';
import { parseDuration } from '../utils/timeUtils.js';
import { getSystemFieldName } from './fieldConfig.js';

const STORAGE_KEYS = {
  workspace: 'vertex_create_modal_workspace',
  itemType: 'vertex_create_modal_item_type',
};

// System fields managed or rendered outside generated form sections.
const EXCLUDED_SYSTEM_FIELDS = ['status'];
const FIXED_SYSTEM_FIELDS = ['title', 'description'];

/**
 * Default work-item form values. Single source of truth for both initial
 * state and form resets so the create modal starts and restarts the same way.
 * @param {{ itemTypeId?: number|null }} [opts]
 */
function defaultFormData({ itemTypeId = null } = {}) {
  return {
    name: '',
    description: '',
    due_date: '',
    start_date: '',
    end_date: '',
    workspace_id: null,
    priority_id: null,
    milestone_ids: [],
    assignee_id: null,
    iteration_id: null,
    project_id: null,
    label_names: [],
    story_points: '',
    estimate: '',
    item_type_id: itemTypeId,
  };
}

class WorkItemFormStore {
  // === Form Data ===
  formData = $state(defaultFormData());
  customFieldValues = $state({});
  selectedLabels = $state([]);
  validationErrors = $state([]);
  pendingDescriptionImages = $state([]);

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
  milestonesLoadedForKey = $state(null);

  iterations = $state([]);
  iterationsLoading = $state(false);
  iterationsLoaded = $state(false);
  iterationsLoadedForKey = $state(null);

  timeProjects = $state([]);
  timeProjectsLoading = $state(false);
  timeProjectsLoaded = $state(false);
  timeProjectsLoadedForKey = $state(null);

  itemTypes = $state([]);
  hierarchyLevels = $state([]);
  availableItemTypes = $state([]);
  itemTypesLoaded = $state(false);

  // Templates for the selected type; mandatory bodies lock the picker.
  templateOptions = $state([]);
  mandatoryTemplate = $state(null);
  selectedTemplateId = $state(null);
  templatesLoading = $state(false);
  templateApplyNonce = $state(0);
  // Deduplicates only the current template fetch.
  #templatesInFlightKey = null;

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
  #milestonesLoadToken = 0;
  #iterationsLoadToken = 0;
  #timeProjectsLoadToken = 0;

  // === Derived Values (getters) ===

  /**
   * Get the currently selected item type object.
   */
  get selectedItemType() {
    return this.availableItemTypes.find((t) => t.id === this.formData.item_type_id) || null;
  }

  /**
   * Whether the current item type enforces a mandatory template (so the
   * create-modal template picker is disabled and its body auto-applied).
   */
  get templateLocked() {
    return !!this.mandatoryTemplate;
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
    return this.users.find((u) => u.id === this.formData.assignee_id) || null;
  }

  /**
   * Get the currently selected milestone objects (multi-select).
   */
  get selectedMilestones() {
    const ids = this.formData.milestone_ids || [];
    return ids
      .map((id) => this.milestones.find((milestone) => String(milestone.id) === String(id)))
      .filter(Boolean);
  }

  /**
   * Get the currently selected iteration object.
   */
  get selectedIteration() {
    return this.iterations.find((iteration) => iteration.id === this.formData.iteration_id) || null;
  }

  /**
   * Get the currently selected time project object.
   */
  get selectedProject() {
    return this.timeProjects.find((project) => project.id === this.formData.project_id) || null;
  }

  /**
   * Get selected global label IDs for post-create assignment.
   */
  get selectedLabelIds() {
    return (this.selectedLabels || [])
      .map((label) => label?.id)
      .filter((id) => Number.isFinite(id));
  }

  /**
   * Get non-required custom fields for the overflow menu.
   */
  get nonRequiredCustomFields() {
    return this.customFields.filter((cf) => {
      const screenField = this.screenFields.find(
        (f) => f.field_type === 'custom' && parseInt(f.field_identifier, 10) === cf.id
      );
      return !screenField?.is_required;
    });
  }

  /**
   * Get optional system fields that should be rendered as full-width controls
   * instead of compact chips.
   */
  get nonRequiredFullSystemFields() {
    return this.screenFields.filter(
      (f) => f.field_type === 'system' && !f.is_required && f.field_identifier === 'labels'
    );
  }

  /**
   * Get required system fields that should be shown as full inputs.
   */
  get requiredSystemFields() {
    return this.screenFields.filter(
      (f) =>
        f.is_required &&
        f.field_type === 'system' &&
        isCreateSystemFieldRenderable(f.field_identifier) &&
        !EXCLUDED_SYSTEM_FIELDS.includes(f.field_identifier) &&
        !FIXED_SYSTEM_FIELDS.includes(f.field_identifier)
    );
  }

  /**
   * Get required custom fields that should be shown as full inputs.
   */
  get requiredCustomFields() {
    return this.customFields.filter((cf) => {
      const screenField = this.screenFields.find(
        (f) => f.field_type === 'custom' && parseInt(f.field_identifier, 10) === cf.id
      );
      return screenField?.is_required === true;
    });
  }

  // === Data Loading Methods ===

  /**
   * Load assignable users. When workspaceId is provided, fetches only active users
   * via the assignable-users endpoint; otherwise falls back to the general users endpoint.
   */
  async loadUsers(workspaceId = null) {
    if (this.usersLoaded) return;
    try {
      const result = workspaceId ? await api.getAssignableUsers(workspaceId) : await api.getUsers();
      this.users = result || [];
      this.usersLoaded = true;
    } catch (error) {
      if (error?.name === 'AbortError') return;
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
      this.allCustomFields = await api.customFields.getAll();
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
  async loadMilestones(workspaceId = null, forceReload = false) {
    const numericWorkspaceId = workspaceId ? Number(workspaceId) : null;
    const loadKey = numericWorkspaceId || 'global';
    if (!forceReload && this.milestonesLoaded && this.milestonesLoadedForKey === loadKey) return;

    const token = ++this.#milestonesLoadToken;
    try {
      this.milestonesLoading = true;
      const filters = numericWorkspaceId
        ? { workspace_id: numericWorkspaceId, include_global: true }
        : {};
      const result = await api.milestones.getAll(filters);
      if (token !== this.#milestonesLoadToken) return;

      this.allMilestones = result || [];
      this.milestonesLoaded = true;
      this.milestonesLoadedForKey = loadKey;
      this.#filterMilestones();
    } catch (error) {
      if (token !== this.#milestonesLoadToken) return;
      console.error('Failed to load milestones:', error);
      this.allMilestones = [];
      this.milestones = [];
      this.milestonesLoaded = true;
      this.milestonesLoadedForKey = loadKey;
    } finally {
      if (token === this.#milestonesLoadToken) {
        this.milestonesLoading = false;
      }
    }
  }

  /**
   * Load iterations scoped to the current workspace plus globals.
   */
  async loadIterations(workspaceId = null, forceReload = false) {
    const numericWorkspaceId = workspaceId ? Number(workspaceId) : null;
    const loadKey = numericWorkspaceId || 'global';
    if (!forceReload && this.iterationsLoaded && this.iterationsLoadedForKey === loadKey) return;

    const token = ++this.#iterationsLoadToken;
    try {
      this.iterationsLoading = true;
      const filters = numericWorkspaceId
        ? { workspace_id: numericWorkspaceId, include_global: true }
        : {};
      const result = await api.iterations.getAll(filters);
      if (token !== this.#iterationsLoadToken) return;

      this.iterations = result || [];
      this.iterationsLoaded = true;
      this.iterationsLoadedForKey = loadKey;
    } catch (error) {
      if (token !== this.#iterationsLoadToken) return;
      console.error('Failed to load iterations:', error);
      this.iterations = [];
      this.iterationsLoaded = true;
      this.iterationsLoadedForKey = loadKey;
    } finally {
      if (token === this.#iterationsLoadToken) {
        this.iterationsLoading = false;
      }
    }
  }

  /**
   * Load time projects available to the selected workspace.
   */
  async loadTimeProjects(workspaceId = null, forceReload = false) {
    const numericWorkspaceId = workspaceId ? Number(workspaceId) : null;
    const loadKey = numericWorkspaceId || 'none';
    if (!numericWorkspaceId) {
      this.timeProjects = [];
      this.timeProjectsLoaded = false;
      this.timeProjectsLoadedForKey = null;
      return;
    }
    if (!forceReload && this.timeProjectsLoaded && this.timeProjectsLoadedForKey === loadKey)
      return;

    const token = ++this.#timeProjectsLoadToken;
    try {
      this.timeProjectsLoading = true;
      const result = await api.time.projects.getByWorkspace(numericWorkspaceId);
      if (token !== this.#timeProjectsLoadToken) return;

      this.timeProjects = result || [];
      this.timeProjectsLoaded = true;
      this.timeProjectsLoadedForKey = loadKey;
    } catch (error) {
      if (token !== this.#timeProjectsLoadToken) return;
      console.error('Failed to load time projects:', error);
      this.timeProjects = [];
      this.timeProjectsLoaded = true;
      this.timeProjectsLoadedForKey = loadKey;
    } finally {
      if (token === this.#timeProjectsLoadToken) {
        this.timeProjectsLoading = false;
      }
    }
  }

  /**
   * Filter milestones based on workspace categories, while always keeping
   * global milestones available in workspace-scoped forms.
   */
  #filterMilestones() {
    if (!this.workspaceDetails?.milestone_categories?.length) {
      this.milestones = this.allMilestones;
    } else {
      const allowedCategoryIds = this.workspaceDetails.milestone_categories;
      this.milestones = this.allMilestones.filter(
        (m) => m.is_global || allowedCategoryIds.includes(m.category_id)
      );
    }
  }

  /**
   * Load workspace details and filter milestones.
   */
  async loadWorkspaceDetails(workspaceId) {
    if (!workspaceId) {
      this.workspaceDetails = null;
      this.iterations = [];
      this.timeProjects = [];
      this.#filterMilestones();
      return;
    }
    try {
      this.workspaceDetails = await api.workspaces.get(workspaceId);
      await Promise.all([
        this.loadMilestones(workspaceId),
        this.loadIterations(workspaceId),
        this.loadTimeProjects(workspaceId),
      ]);
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
        api.hierarchyLevels.getAll(),
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
    } else if (!this.parentItem) {
      // A level -1 generic sub-task is invalid without a parent. The API also
      // enforces this, but hiding it prevents an avoidable failed submission.
      baseTypes = baseTypes.filter((type) => !isGenericSubtaskType(type));
    }

    // Apply config set item type restrictions
    if (this.currentConfigSet?.item_type_configs?.length > 0) {
      const allowedItemTypeIds = this.currentConfigSet.item_type_configs.map((c) => c.item_type_id);
      baseTypes = baseTypes.filter((t) => allowedItemTypeIds.includes(t.id));
    }

    this.availableItemTypes = sortItemTypesByHierarchy(baseTypes);

    // Auto-select first item type if current is invalid
    if (
      this.availableItemTypes.length > 0 &&
      !this.availableItemTypes.find((t) => t.id === this.formData.item_type_id)
    ) {
      this.formData.item_type_id = this.availableItemTypes[0].id;
    }

    // Load templates for the resolved type so a mandatory template auto-applies
    // before the create modal's editor mounts (WI-438).
    this.loadTemplatesForCurrentType();
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
    return resolveEffectiveScreenIds(this.currentConfigSet, itemTypeId, 1).create;
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
        .filter((field) => field.field_type === 'system')
        .map((field) => field.field_identifier);

      const customFieldIds = this.screenFields
        .filter((field) => field.field_type === 'custom')
        .map((field) => parseInt(field.field_identifier, 10));

      const filteredCustomFields = this.allCustomFields.filter((field) =>
        customFieldIds.includes(field.id)
      );

      // Keep values the user already entered for fields that remain configured
      // across the workspace/type change; only fields new to the screen get
      // their default. Resetting wholesale silently discarded picks when the
      // item type changed after a field was filled.
      const previousValues = this.customFieldValues;
      this.customFieldValues = {};
      filteredCustomFields.forEach((field) => {
        const previous = previousValues[field.id];
        this.customFieldValues[field.id] =
          previous !== undefined && previous !== null && previous !== ''
            ? previous
            : isBooleanCustomFieldType(field.field_type)
              ? false
              : '';
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
    const screenField = this.screenFields.find(
      (f) =>
        f.field_type === 'system' &&
        systemFieldIdentifiers(fieldIdentifier).includes(f.field_identifier)
    );
    return screenField?.is_required === true;
  }

  /**
   * Check if a system field is configured (in screen fields).
   */
  isFieldConfigured(fieldIdentifier) {
    return isSystemFieldConfigured(this.screenSystemFields, fieldIdentifier);
  }

  // === Selection Methods ===

  /**
   * Set the selected workspace.
   */
  setWorkspace(workspace) {
    const nextWorkspaceID = workspace?.id || null;
    if (this.formData.workspace_id && this.formData.workspace_id !== nextWorkspaceID) {
      this.formData.label_names = [];
      this.selectedLabels = [];
    }
    this.selectedWorkspace = workspace;
    this.formData.workspace_id = nextWorkspaceID;

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
    this.loadTemplatesForCurrentType();
  }

  // Load selectable and mandatory templates for the current workspace and type.
  async loadTemplatesForCurrentType() {
    const workspaceId = this.formData.workspace_id;
    const itemTypeId = this.formData.item_type_id;
    if (!workspaceId || !itemTypeId) {
      this.templateOptions = [];
      this.mandatoryTemplate = null;
      this.selectedTemplateId = null;
      this.#templatesInFlightKey = null;
      return;
    }
    const key = `${workspaceId}:${itemTypeId}`;
    // Never cache resolved keys: templates can change while the modal is open.
    if (this.#templatesInFlightKey === key) return;
    this.#templatesInFlightKey = key;

    this.templatesLoading = true;
    try {
      const list =
        (await api.itemTemplates.getAll(workspaceId, { item_type_id: itemTypeId })) ?? [];
      // Guard against an out-of-order response after another type change.
      if (`${this.formData.workspace_id}:${this.formData.item_type_id}` !== key) return;

      const mandatory = list.find((t) => t.mode === 'mandatory') || null;
      this.templateOptions = list.filter((t) => t.mode === 'selectable');
      this.mandatoryTemplate = mandatory;
      if (mandatory) {
        this.selectedTemplateId = mandatory.id;
        // Never overwrite description text entered during the async load.
        if (!this.formData.description?.trim()) {
          this.formData.description = mandatory.description_body || '';
          this.templateApplyNonce += 1;
        }
      } else {
        this.selectedTemplateId = null;
      }
    } catch (err) {
      console.error('Failed to load item templates:', err);
      this.templateOptions = [];
      this.mandatoryTemplate = null;
    } finally {
      // Preserve a newer fetch marker.
      if (this.#templatesInFlightKey === key) this.#templatesInFlightKey = null;
      this.templatesLoading = false;
    }
  }

  /**
   * Apply a selectable template's body into the description (from the picker).
   */
  applyTemplate(templateId) {
    const tmpl = this.templateOptions.find((t) => t.id === templateId);
    if (!tmpl) return;
    this.formData.description = tmpl.description_body || '';
    this.selectedTemplateId = templateId;
    this.templateApplyNonce += 1;
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
      const storedWorkspace = workspaces.find((w) => w.id === this.storedWorkspaceId);
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
    if (
      this.storedItemTypeId &&
      this.availableItemTypes.length > 0 &&
      !this.storedItemTypeApplied &&
      !this.restrictedItemTypes
    ) {
      const storedItemType = this.availableItemTypes.find(
        (type) => type.id === this.storedItemTypeId
      );
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
    if (
      this.availableItemTypes.length > 0 &&
      this.currentConfigSet?.default_item_type_id &&
      !this.configSetDefaultApplied
    ) {
      const hasValidStoredType =
        this.storedItemTypeId &&
        this.availableItemTypes.find((type) => type.id === this.storedItemTypeId);
      if (!hasValidStoredType) {
        const configDefault = this.availableItemTypes.find(
          (type) => type.id === this.currentConfigSet.default_item_type_id
        );
        if (configDefault) {
          this.formData.item_type_id = configDefault.id;
        }
      }
      this.configSetDefaultApplied = true;
    }
  }

  // === Deferred Description Image Uploads ===

  /**
   * Track an image inserted before the item exists so it can be uploaded after creation.
   */
  addPendingDescriptionImage(image) {
    if (!image?.file || !image?.url) return;
    this.pendingDescriptionImages = [...this.pendingDescriptionImages, image];
  }

  /**
   * Clear tracked pending images and optionally revoke their local preview URLs.
   */
  clearPendingDescriptionImages(revokeUrls = true) {
    if (revokeUrls && typeof URL !== 'undefined') {
      this.pendingDescriptionImages.forEach((image) => {
        if (image?.url?.startsWith('blob:')) {
          URL.revokeObjectURL(image.url);
        }
      });
    }
    this.pendingDescriptionImages = [];
  }

  // === Validation ===

  #getSystemFieldValue(identifier) {
    switch (identifier) {
      case 'title':
        return this.formData.name;
      case 'milestone':
        return this.formData.milestone_ids;
      case 'labels':
        return this.selectedLabelIds;
      case 'estimate_minutes':
      case 'estimate':
        return this.formData.estimate;
      default:
        return this.formData[identifier] ?? this.formData[`${identifier}_id`];
    }
  }

  #isEmptyValue(value) {
    if (Array.isArray(value)) return value.length === 0;
    return value === undefined || value === null || value === '';
  }

  #parsedStoryPoints() {
    const raw = this.formData.story_points;
    if (raw === undefined || raw === null || raw === '') return null;
    const parsed = parseFloat(raw);
    return Number.isFinite(parsed) && parsed >= 0 ? parsed : null;
  }

  #parsedEstimateMinutes() {
    const raw = (this.formData.estimate || '').trim();
    if (!raw) return null;
    const minutes = parseDuration(raw);
    return Number.isFinite(minutes) && minutes > 0 ? Math.round(minutes) : null;
  }

  /**
   * Validate the form and return whether it's valid.
   */
  validate() {
    const errors = [];

    for (const field of this.screenFields) {
      if (field.is_required) {
        if (field.field_type === 'system') {
          const identifier = field.field_identifier;
          // Skip fields that create manages automatically or cannot currently
          // submit. The screen editor is being tightened separately so admins
          // cannot create impossible required-field states.
          if (
            isCreateSystemFieldAutoManaged(identifier) ||
            !isCreateSystemFieldRenderable(identifier)
          ) {
            continue;
          }
          const value = this.#getSystemFieldValue(identifier);
          const labelIdentifier = identifier === 'estimate_minutes' ? 'estimate' : identifier;
          const label = getSystemFieldName(labelIdentifier);
          if (this.#isEmptyValue(value)) {
            errors.push(`${label} is required`);
          } else if (identifier === 'story_points' && this.#parsedStoryPoints() === null) {
            errors.push(`${label} must be a valid number`);
          } else if (
            (identifier === 'estimate' || identifier === 'estimate_minutes') &&
            this.#parsedEstimateMinutes() === null
          ) {
            errors.push(`${label} must be a valid duration`);
          }
        } else if (field.field_type === 'custom') {
          const fieldId = parseInt(field.field_identifier, 10);
          const value = this.customFieldValues[fieldId];
          const fieldDef = this.allCustomFields.find((f) => f.id === fieldId);
          if (isBooleanCustomFieldType(fieldDef?.field_type)) continue;
          if (value === undefined || value === null || value === '') {
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
      milestone_ids: Array.isArray(this.formData.milestone_ids) ? this.formData.milestone_ids : [],
      assignee_id: this.formData.assignee_id || null,
      label_ids: this.selectedLabelIds,
      iteration_id: this.formData.iteration_id || null,
      project_id: this.formData.project_id || null,
      story_points: this.#parsedStoryPoints(),
      estimate_minutes: this.#parsedEstimateMinutes(),
      due_date: dateInputToISOString(this.formData.due_date),
      start_date: dateInputToISOString(this.formData.start_date),
      end_date: dateInputToISOString(this.formData.end_date),
      item_type_id: this.formData.item_type_id,
      parent_id: this.parentItem?.id || null,
      custom_field_values: this.customFieldValues,
    };
  }

  // === Reset ===

  /**
   * Reset form state while keeping loaded reference data.
   */
  resetForm() {
    this.formData = defaultFormData({
      itemTypeId: this.availableItemTypes.length > 0 ? this.availableItemTypes[0].id : null,
    });
    this.customFieldValues = {};
    this.selectedLabels = [];
    this.validationErrors = [];
    this.clearPendingDescriptionImages();
    this.selectedWorkspace = null;
    this.parentItem = null;
    this.restrictedItemTypes = null;
    this.workspaceDetails = null;

    // Reset cache keys to force reload for new workspace/item type
    this.configSetLoadedForWorkspace = null;
    this.screenFieldsLoadedForKey = null;
    this.storedItemTypeApplied = false;
    this.configSetDefaultApplied = false;

    // Reset template state (WI-438) so a new modal doesn't inherit the prior
    // type's picker options / mandatory lock.
    this.templateOptions = [];
    this.mandatoryTemplate = null;
    this.selectedTemplateId = null;
    this.#templatesInFlightKey = null;

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
    this.milestonesLoadedForKey = null;
    this.iterations = [];
    this.iterationsLoading = false;
    this.iterationsLoaded = false;
    this.iterationsLoadedForKey = null;
    this.timeProjects = [];
    this.timeProjectsLoading = false;
    this.timeProjectsLoaded = false;
    this.timeProjectsLoadedForKey = null;
    this.itemTypes = [];
    this.hierarchyLevels = [];
    this.availableItemTypes = [];
    this.itemTypesLoaded = false;
    this.allCustomFields = [];
    this.customFields = [];
    this.selectedLabels = [];
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
      this.loadCustomFields(),
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
