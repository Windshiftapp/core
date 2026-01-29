/**
 * Store for managing Item Detail state.
 * Uses Svelte 5 class-based reactive state pattern.
 * Centralizes item data, editing states, modals, and related data loading.
 */
import { api } from '../api.js';

class ItemDetailStore {
  // === Current Item ===
  item = $state(null);
  itemId = $state(null);
  workspaceId = $state(null);
  loading = $state(true);
  error = $state(null);
  saving = $state(false);

  // Workspace
  workspace = $state(null);

  // === Editing State (unified flag + value) ===
  editing = $state({
    title: { active: false, value: '' },
    description: { active: false, value: '' },
    status: { active: false, value: null },
    priority: { active: false, value: null },
    dueDate: { active: false, value: null },
    milestone: { active: false, value: null },
    iteration: { active: false, value: null },
    project: { active: false, value: null },
    assignee: { active: false, value: null },
    customFields: { active: {}, values: {} }
  });

  // === Related Data (cached) ===
  parentHierarchy = $state([]);
  childItems = $state([]);
  loadingChildItems = $state(false);
  milestones = $state([]);
  iterations = $state([]);
  priorities = $state([]);

  // Item types
  itemTypes = $state([]);
  currentItemType = $state(null);
  currentHierarchyLevel = $state(null);
  availableSubIssueTypes = $state([]);

  // Screen configuration (cached per workspace)
  customFieldDefinitions = $state([]);
  workspaceScreenFields = $state([]);
  workspaceScreenSystemFields = $state([]);

  // Status
  availableStatusTransitions = $state([]);
  loadingStatusTransitions = $state(false);

  // Links
  itemLinks = $state([]);
  linkTypes = $state([]);
  loadingLinks = $state(false);

  // Watch
  isWatching = $state(false);
  loadingWatchStatus = $state(false);

  // Time tracking
  timeProjects = $state([]);
  timeWorklogs = $state([]);
  customers = $state([]);
  workItems = $state([]);
  workspaces = $state([]);

  // Diagrams & Actions
  diagrams = $state([]);
  loadingDiagrams = $state(false);
  manualActions = $state([]);

  // Modals
  showDeleteDialog = $state(false);
  showLinkModal = $state(false);
  showTestCaseModal = $state(false);
  selectedTestCaseId = $state(null);
  showTimeLogModal = $state(false);
  editingWorklog = $state(null);

  // Track changes
  hasChanges = $state(false);

  // Modal/Fullscreen state
  isFullscreen = $state(false);

  // Animation state
  transitioning = $state(false);

  // Dropdown items (computed from item state)
  dropdownItems = $state([]);

  // === Derived Values (getters) ===

  /**
   * Get status options based on loaded transitions.
   */
  get statusOptions() {
    if (this.availableStatusTransitions.length > 0) {
      return this.availableStatusTransitions.map(transition => ({
        id: transition.id,
        value: transition.value,
        label: transition.name,
        categoryColor: transition.category_color || null
      }));
    }
    return this.loadingStatusTransitions ? [{ value: '', label: 'Loading...' }] : [];
  }

  /**
   * Get filtered link types (excluding test link type for item-to-item linking).
   */
  get filteredLinkTypes() {
    return this.linkTypes;
  }

  // === Data Loading Methods ===

  /**
   * Load all item data and related data.
   */
  async loadItem(workspaceId, itemId) {
    this.workspaceId = workspaceId;
    this.itemId = itemId;
    this.loading = true;
    this.error = null;
    this.loadingLinks = true;

    try {
      const [
        itemData,
        workspaceData,
        linkTypesData,
        linksData,
        customFieldsData,
        milestonesData,
        iterationsData,
        projectsData,
        worklogsData,
        customersData,
        workItemsData,
        workspacesData
      ] = await Promise.all([
        api.items.get(itemId),
        api.workspaces.get(workspaceId),
        api.linkTypes.getAll(),
        api.links.getForItem('items', itemId),
        api.customFields.getAll(),
        api.milestones.getAll(),
        api.iterations.getAll({ workspace_id: workspaceId, include_global: true }),
        api.time.projects.getByWorkspace(workspaceId),
        api.time.worklogs.getByItem(itemId),
        api.time.customers.getAll(),
        api.items.getAll({ limit: 100 }),
        api.workspaces.getAll()
      ]);

      this.item = itemData;
      // Ensure assignee_id is never undefined
      if (this.item.assignee_id === undefined) {
        this.item.assignee_id = null;
      }

      this.workspace = workspaceData;
      this.customFieldDefinitions = customFieldsData || [];

      // Filter milestones by workspace restrictions
      let allMilestones = milestonesData || [];
      if (this.workspace?.milestone_categories?.length > 0) {
        const allowedCategoryIds = this.workspace.milestone_categories;
        this.milestones = allMilestones.filter(m => allowedCategoryIds.includes(m.category_id));
      } else {
        this.milestones = allMilestones;
      }

      this.iterations = iterationsData || [];
      this.timeProjects = projectsData || [];
      this.timeWorklogs = worklogsData || [];
      this.customers = customersData || [];
      this.workItems = workItemsData?.items || workItemsData || [];
      this.workspaces = workspacesData || [];

      // Load priorities based on workspace configuration
      await this.#loadPriorities();

      // Load status transitions and watch status
      this.#loadAvailableStatusTransitions();
      this.#loadWatchStatus();

      this.linkTypes = linkTypesData;

      // Process links data
      const allLinks = [];
      if (linksData.outgoing) allLinks.push(...linksData.outgoing);
      if (linksData.incoming) allLinks.push(...linksData.incoming);
      this.itemLinks = allLinks;

      // Sync editing state from item
      this.#syncEditingFromItem();

      // Load parent hierarchy if item has parents
      if (this.item.parent_id) {
        await this.#loadParentHierarchy();
      } else {
        this.parentHierarchy = [];
      }

      // Load child items and hierarchy data
      await this.loadChildItems();
      await this.#loadItemTypeData();

      // Load workspace screen configuration
      await this.#loadWorkspaceScreenFields();

      // Load diagrams
      await this.loadDiagrams();

      // Load manual actions
      await this.#loadManualActions();

      this.loading = false;
      this.loadingLinks = false;
    } catch (err) {
      console.error('Failed to load item or workspace:', err);
      this.error = err.message || 'Failed to load data';
      this.loading = false;
      this.loadingLinks = false;
    }
  }

  /**
   * Reload worklogs (e.g., after timer stops).
   */
  async reloadWorklogs() {
    if (!this.itemId) return;
    try {
      this.timeWorklogs = await api.time.worklogs.getByItem(this.itemId) || [];
    } catch (err) {
      console.error('Failed to reload worklogs:', err);
    }
  }

  /**
   * Load child items.
   */
  async loadChildItems() {
    if (!this.itemId) return;
    try {
      this.loadingChildItems = true;
      const response = await api.items.getChildren(this.itemId);
      if (Array.isArray(response)) {
        this.childItems = response;
      } else if (response?.items) {
        this.childItems = response.items;
      } else if (response?.data) {
        this.childItems = response.data;
      } else {
        this.childItems = [];
      }
    } catch (err) {
      console.error('Failed to load child items:', err);
      this.childItems = [];
    } finally {
      this.loadingChildItems = false;
    }
  }

  /**
   * Load diagrams for the item.
   */
  async loadDiagrams() {
    if (!this.item?.id) return;
    try {
      this.loadingDiagrams = true;
      this.diagrams = await api.getDiagrams(this.item.id) || [];
    } catch (err) {
      console.error('Failed to load diagrams:', err);
      this.diagrams = [];
    } finally {
      this.loadingDiagrams = false;
    }
  }

  // === Private Data Loading Methods ===

  async #loadPriorities() {
    if (!this.workspace) return;
    try {
      if (this.workspace.configuration_set_id) {
        const configSet = await api.configurationSets.get(this.workspace.configuration_set_id);
        this.priorities = configSet.priorities_detailed || [];
      } else {
        this.priorities = await api.priorities.getAll();
      }
      this.priorities = this.priorities.sort((a, b) => a.sort_order - b.sort_order);
    } catch (err) {
      console.error('Failed to load priorities:', err);
      this.priorities = [];
    }
  }

  async #loadAvailableStatusTransitions() {
    if (!this.item?.id || this.loadingStatusTransitions) return;
    try {
      this.loadingStatusTransitions = true;
      const result = await api.items.getAvailableStatusTransitions(this.item.id);
      this.availableStatusTransitions = result.available_transitions || [];
    } catch (err) {
      console.error('Failed to load status transitions:', err);
      this.availableStatusTransitions = [];
    } finally {
      this.loadingStatusTransitions = false;
    }
  }

  async #loadWatchStatus() {
    if (!this.item?.id || this.loadingWatchStatus) return;
    try {
      this.loadingWatchStatus = true;
      const result = await api.items.getWatchStatus(this.item.id);
      this.isWatching = result.watching || false;
    } catch (err) {
      console.error('Failed to load watch status:', err);
      this.isWatching = false;
    } finally {
      this.loadingWatchStatus = false;
    }
  }

  async #loadParentHierarchy() {
    try {
      const ancestors = await api.items.getAncestors(this.item.id);
      try {
        const itemTypesData = await api.itemTypes.getAll();
        this.parentHierarchy = ancestors.map(ancestor => {
          if (ancestor.item_type_id) {
            const itemType = itemTypesData.find(type => type.id === ancestor.item_type_id);
            return { ...ancestor, itemType };
          }
          return ancestor;
        });
      } catch (err) {
        console.warn('Failed to load item types for parent hierarchy:', err);
        this.parentHierarchy = ancestors;
      }
    } catch (err) {
      console.error('Failed to load ancestors:', err);
      this.parentHierarchy = [];
    }
  }

  async #loadItemTypeData() {
    try {
      const [itemTypesData, hierarchyLevels] = await Promise.all([
        api.itemTypes.getAll(),
        api.hierarchyLevels.getAll()
      ]);

      this.itemTypes = itemTypesData || [];

      if (this.item.item_type_id) {
        this.currentItemType = this.itemTypes.find(type => type.id === this.item.item_type_id);
        if (this.currentItemType) {
          this.currentHierarchyLevel = hierarchyLevels.find(level => level.level === this.currentItemType.hierarchy_level);
        }
      }

      // Find available sub-issue types (next level down)
      if (this.currentItemType && this.currentHierarchyLevel) {
        const nextLevel = this.currentHierarchyLevel.level + 1;
        this.availableSubIssueTypes = this.itemTypes.filter(type => type.hierarchy_level === nextLevel);
      } else {
        this.availableSubIssueTypes = [];
      }
    } catch (err) {
      console.error('Failed to load item type data:', err);
      this.currentItemType = null;
      this.currentHierarchyLevel = null;
      this.availableSubIssueTypes = [];
    }
  }

  async #loadWorkspaceScreenFields() {
    try {
      let screenId = null;

      if (this.workspace?.configuration_set_id) {
        const configSet = await api.configurationSets.get(this.workspace.configuration_set_id);
        screenId = configSet?.edit_screen_id || configSet?.create_screen_id || configSet?.view_screen_id;
      }

      if (!screenId) screenId = 1;

      const screen = await api.screens.get(screenId);
      const screenFields = screen?.fields || [];

      this.workspaceScreenFields = screenFields.filter(field => field.field_type === 'custom');

      const configuredSystemFields = screenFields
        .filter(field => field.field_type === 'system')
        .map(field => field.field_identifier);

      if (configuredSystemFields.length > 0) {
        this.workspaceScreenSystemFields = configuredSystemFields;
      } else {
        this.workspaceScreenSystemFields = screen?.system_fields || [];
      }
    } catch (err) {
      console.error('Failed to load workspace screen fields:', err);
      this.workspaceScreenFields = [];
      this.workspaceScreenSystemFields = [];
    }
  }

  async #loadManualActions() {
    if (!this.workspaceId) return;
    try {
      const allActions = await api.actions.getAll(this.workspaceId);
      this.manualActions = (allActions || []).filter(a => a.trigger_type === 'manual' && a.is_enabled);
    } catch (err) {
      console.error('Failed to load manual actions:', err);
      this.manualActions = [];
    }
  }

  // === Editing Methods ===

  /**
   * Start editing a field.
   */
  startEditing(field) {
    if (field.startsWith('custom_field_')) {
      const fieldId = field.replace('custom_field_', '');
      this.editing.customFields.active[fieldId] = true;
      const currentValue = this.item.custom_field_values?.[fieldId];
      this.editing.customFields.values[fieldId] = currentValue !== null && currentValue !== undefined ? currentValue : '';
      // Trigger reactivity
      this.editing = { ...this.editing };
    } else {
      // Sync value from item before activating edit mode
      this.#syncFieldFromItem(field);
      this.editing[field].active = true;
      // Trigger reactivity
      this.editing = { ...this.editing };
    }
  }

  /**
   * Cancel editing a field.
   */
  cancelEditing(field) {
    if (field.startsWith('custom_field_')) {
      const fieldId = field.replace('custom_field_', '');
      delete this.editing.customFields.active[fieldId];
      delete this.editing.customFields.values[fieldId];
      this.editing = { ...this.editing };
    } else if (this.editing[field]) {
      this.editing[field].active = false;
      this.#syncFieldFromItem(field);
      // Trigger reactivity
      this.editing = { ...this.editing };
    }
  }

  /**
   * Save a field value.
   */
  async saveField(field, directValue = null, assigneeName = null, iterationName = null) {
    if (this.saving) return;

    try {
      this.saving = true;
      let updateData = {};

      if (field === 'title') {
        const newTitle = directValue || this.editing.title.value.trim();
        if (newTitle === this.item.title) {
          this.cancelEditing('title');
          return;
        }
        updateData.title = newTitle;
      } else if (field === 'description') {
        const newDescription = directValue !== null ? directValue : this.editing.description.value;
        if (newDescription === (this.item.description || '')) {
          this.cancelEditing('description');
          return;
        }
        updateData.description = newDescription;
      } else if (field === 'status_id') {
        const newStatusId = directValue !== null ? directValue : null;
        if (newStatusId === this.item.status_id) {
          this.cancelEditing('status');
          return;
        }
        updateData.status_id = newStatusId;
        this.item = { ...this.item, status_id: newStatusId };
      } else if (field === 'priority_id') {
        const newPriorityId = directValue !== null ? directValue : null;
        if (newPriorityId === this.item.priority_id) {
          this.cancelEditing('priority');
          return;
        }
        updateData.priority_id = newPriorityId;
        this.item = { ...this.item, priority_id: newPriorityId };
      } else if (field === 'due_date') {
        const newDueDate = directValue !== null ? directValue : null;
        if (newDueDate === this.item.due_date) {
          this.cancelEditing('dueDate');
          return;
        }
        updateData.due_date = newDueDate;
        this.item = { ...this.item, due_date: newDueDate };
      } else if (field === 'milestone') {
        const newMilestone = directValue !== null ? directValue : this.editing.milestone.value;
        if (newMilestone === this.item.milestone_id) {
          this.cancelEditing('milestone');
          return;
        }
        updateData.milestone_id = newMilestone;
        this.item = { ...this.item, milestone_id: newMilestone };
      } else if (field === 'iteration') {
        const newIteration = directValue !== null ? directValue : null;
        if (newIteration === this.item.iteration_id) {
          return;
        }
        updateData.iteration_id = newIteration;
        this.item = {
          ...this.item,
          iteration_id: newIteration,
          iteration_name: iterationName !== undefined ? iterationName : this.item.iteration_name
        };
      } else if (field === 'project') {
        const newProject = directValue !== null ? directValue : this.editing.project.value;
        if (typeof newProject === 'object' && newProject !== null) {
          updateData.project_id = newProject.project_id;
          updateData.inherit_project = newProject.inherit_project;
          this.item = { ...this.item, project_id: newProject.project_id, inherit_project: newProject.inherit_project };
        } else {
          if (newProject === this.item.project_id) {
            this.cancelEditing('project');
            return;
          }
          updateData.project_id = newProject;
          this.item = { ...this.item, project_id: newProject };
        }
      } else if (field === 'assignee') {
        const newAssignee = directValue !== undefined ? directValue : this.editing.assignee.value;
        if (newAssignee === this.item.assignee_id) {
          this.cancelEditing('assignee');
          return;
        }
        updateData.assignee_id = newAssignee;
        this.item = {
          ...this.item,
          assignee_id: newAssignee,
          assignee_name: assigneeName !== undefined ? assigneeName : this.item.assignee_name
        };
      } else if (field.startsWith('custom_field_')) {
        const fieldId = field.replace('custom_field_', '');
        let newValue = directValue !== null ? directValue : this.editing.customFields.values[fieldId];
        const currentValue = this.item.custom_field_values?.[fieldId] || '';

        // Convert number fields
        const fieldDef = this.customFieldDefinitions.find(f => f.id === parseInt(fieldId));
        if (fieldDef?.field_type === 'number' && newValue !== null && newValue !== undefined && newValue !== '') {
          newValue = parseFloat(newValue);
          if (isNaN(newValue)) {
            newValue = directValue !== null ? directValue : this.editing.customFields.values[fieldId];
          }
        }

        if (newValue === currentValue) {
          this.cancelEditing(field);
          return;
        }

        updateData.custom_field_values = {
          ...(this.item.custom_field_values || {}),
          [fieldId]: newValue
        };
      }

      // Update via API
      const updatedItem = await api.items.update(this.item.id, updateData);
      this.item = { ...this.item, ...updatedItem };

      // Update assignee/iteration names if provided
      if (field === 'assignee' && assigneeName !== null) {
        this.item = { ...this.item, assignee_name: assigneeName };
      }
      if (field === 'iteration' && iterationName !== undefined) {
        this.item = { ...this.item, iteration_name: iterationName };
      }

      this.hasChanges = true;
      this.cancelEditing(field);
    } catch (err) {
      console.error('Failed to update item:', err);
      throw err;
    } finally {
      this.saving = false;
    }
  }

  // === Auto-sync editing values when item loads/changes ===

  #syncEditingFromItem() {
    if (!this.item) return;
    this.editing.title.value = this.item.title || '';
    this.editing.description.value = this.item.description || '';
    this.editing.status.value = this.item.status_id;
    this.editing.priority.value = this.item.priority_id;
    this.editing.dueDate.value = this.item.due_date;
    this.editing.milestone.value = this.item.milestone_id;
    this.editing.iteration.value = this.item.iteration_id;
    this.editing.assignee.value = this.item.assignee_id;
    this.editing.project.value = this.item.project_id;
    this.editing.customFields.values = { ...(this.item.custom_field_values || {}) };
  }

  #syncFieldFromItem(field) {
    if (!this.item) return;
    const fieldMap = {
      title: 'title',
      description: 'description',
      status: 'status_id',
      priority: 'priority_id',
      dueDate: 'due_date',
      milestone: 'milestone_id',
      iteration: 'iteration_id',
      assignee: 'assignee_id',
      project: 'project_id'
    };
    if (fieldMap[field] && this.editing[field]) {
      this.editing[field].value = this.item[fieldMap[field]];
    }
  }

  // === Watch Actions ===

  async toggleWatch() {
    if (!this.item?.id) return;
    try {
      if (this.isWatching) {
        await api.items.removeWatch(this.item.id);
        this.isWatching = false;
      } else {
        await api.items.addWatch(this.item.id);
        this.isWatching = true;
      }
      this.hasChanges = true;
    } catch (err) {
      console.error('Failed to toggle watch:', err);
      throw err;
    }
  }

  // === Link Actions ===

  async createLink(linkTypeId, targetId, targetType = 'item') {
    try {
      await api.links.create({
        source_type: 'item',
        source_id: parseInt(this.itemId),
        target_type: targetType,
        target_id: parseInt(targetId),
        link_type_id: parseInt(linkTypeId)
      });
      // Reload to refresh links
      await this.loadItem(this.workspaceId, this.itemId);
    } catch (err) {
      console.error('Error creating link:', err);
      throw err;
    }
  }

  async removeLink(linkId) {
    try {
      await api.links.delete(linkId);
      await this.loadItem(this.workspaceId, this.itemId);
    } catch (err) {
      console.error('Error removing link:', err);
      throw err;
    }
  }

  // === Copy Item ===

  async copyItem() {
    try {
      const copiedItem = await api.items.copy(this.item.id);
      return copiedItem;
    } catch (err) {
      console.error('Failed to copy item:', err);
      throw err;
    }
  }

  // === Execute Action ===

  async executeAction(actionId) {
    try {
      await api.actions.execute(this.workspaceId, actionId, this.item.id);
    } catch (err) {
      console.error('Failed to execute action:', err);
      throw err;
    }
  }

  // === Modal Controls ===

  openDeleteDialog() {
    this.showDeleteDialog = true;
  }

  closeDeleteDialog() {
    this.showDeleteDialog = false;
  }

  openLinkModal() {
    this.showLinkModal = true;
  }

  closeLinkModal() {
    this.showLinkModal = false;
  }

  openTestCaseModal(testCaseId) {
    this.selectedTestCaseId = testCaseId;
    this.showTestCaseModal = true;
  }

  closeTestCaseModal() {
    this.showTestCaseModal = false;
    this.selectedTestCaseId = null;
  }

  openTimeLogModal(worklog = null) {
    this.editingWorklog = worklog;
    this.showTimeLogModal = true;
  }

  closeTimeLogModal() {
    this.showTimeLogModal = false;
    this.editingWorklog = null;
  }

  toggleFullscreen() {
    this.isFullscreen = !this.isFullscreen;
  }

  // === Get Default Project for Time Logging ===

  getDefaultProjectForTimeLogging() {
    if (this.item?.time_project_id) return this.item.time_project_id;
    if (this.item?.effective_project_id) return this.item.effective_project_id;
    if (this.workspace?.time_project_id) return this.workspace.time_project_id;
    return null;
  }

  // === Reset ===

  /**
   * Clear state for navigation to new item.
   */
  clearForNavigation() {
    this.item = null;
    this.parentHierarchy = [];
    this.childItems = [];
    this.availableStatusTransitions = [];
    this.customFieldDefinitions = [];
    this.workspaceScreenFields = [];
    this.itemLinks = [];
  }

  /**
   * Full reset.
   */
  reset() {
    this.item = null;
    this.itemId = null;
    this.workspaceId = null;
    this.loading = true;
    this.error = null;
    this.saving = false;
    this.workspace = null;

    this.editing = {
      title: { active: false, value: '' },
      description: { active: false, value: '' },
      status: { active: false, value: null },
      priority: { active: false, value: null },
      dueDate: { active: false, value: null },
      milestone: { active: false, value: null },
      iteration: { active: false, value: null },
      project: { active: false, value: null },
      assignee: { active: false, value: null },
      customFields: { active: {}, values: {} }
    };

    this.parentHierarchy = [];
    this.childItems = [];
    this.loadingChildItems = false;
    this.milestones = [];
    this.iterations = [];
    this.priorities = [];
    this.itemTypes = [];
    this.currentItemType = null;
    this.currentHierarchyLevel = null;
    this.availableSubIssueTypes = [];
    this.customFieldDefinitions = [];
    this.workspaceScreenFields = [];
    this.workspaceScreenSystemFields = [];
    this.availableStatusTransitions = [];
    this.loadingStatusTransitions = false;
    this.itemLinks = [];
    this.linkTypes = [];
    this.loadingLinks = false;
    this.isWatching = false;
    this.loadingWatchStatus = false;
    this.timeProjects = [];
    this.timeWorklogs = [];
    this.customers = [];
    this.workItems = [];
    this.workspaces = [];
    this.diagrams = [];
    this.loadingDiagrams = false;
    this.manualActions = [];
    this.showDeleteDialog = false;
    this.showLinkModal = false;
    this.showTestCaseModal = false;
    this.selectedTestCaseId = null;
    this.showTimeLogModal = false;
    this.editingWorklog = null;
    this.hasChanges = false;
    this.isFullscreen = false;
    this.transitioning = false;
    this.dropdownItems = [];
  }
}

export const itemDetailStore = new ItemDetailStore();
