/**
 * Composable for managing item field editing state.
 * Centralizes the editing mode tracking and value management for all item fields.
 */

/**
 * Creates an item editing state manager.
 *
 * @param {Function} getItem - Function that returns the current item
 * @param {Function} onSave - Callback when a field is saved
 * @param {Function} onError - Callback when an error occurs
 * @returns {Object} Editing state and functions
 */
export function useItemEditing(getItem, onSave, onError) {
  // Editing mode flags for each field
  let editingTitle = $state(false);
  let editingDescription = $state(false);
  let editingStatus = $state(false);
  let editingPriority = $state(false);
  let editingDueDate = $state(false);
  let editingMilestone = $state(false);
  let editingIteration = $state(false);
  let editingProject = $state(false);
  let editingAssignee = $state(false);
  let editingCustomFields = $state({});

  // Temporary edit values
  let editTitle = $state('');
  let editDescription = $state('');
  let editStatus = $state('');
  let editPriority = $state('');
  let editMilestone = $state(null);
  let editIteration = $state(null);
  let editProject = $state(null);
  let editAssignee = $state(null);
  let editCustomFieldValues = $state({});

  // Saving state
  let saving = $state(false);

  /**
   * Starts editing a field.
   */
  function startEditing(field, customFieldId = null) {
    const item = getItem();
    if (!item) return;

    switch (field) {
      case 'title':
        editTitle = item.title || '';
        editingTitle = true;
        break;
      case 'description':
        editDescription = item.description || '';
        editingDescription = true;
        break;
      case 'status':
        editStatus = item.status || '';
        editingStatus = true;
        break;
      case 'priority':
        editPriority = item.priority || '';
        editingPriority = true;
        break;
      case 'dueDate':
        editingDueDate = true;
        break;
      case 'milestone':
        editMilestone = item.milestone_id || null;
        editingMilestone = true;
        break;
      case 'iteration':
        editIteration = item.iteration_id || null;
        editingIteration = true;
        break;
      case 'project':
        editProject = item.project_id || null;
        editingProject = true;
        break;
      case 'assignee':
        editAssignee = item.assignee_id || null;
        editingAssignee = true;
        break;
      case 'customField':
        if (customFieldId) {
          const value = item.custom_field_values?.[customFieldId] || '';
          editCustomFieldValues = { ...editCustomFieldValues, [customFieldId]: value };
          editingCustomFields = { ...editingCustomFields, [customFieldId]: true };
        }
        break;
    }
  }

  /**
   * Cancels editing a field.
   */
  function cancelEdit(field, customFieldId = null) {
    switch (field) {
      case 'title':
        editingTitle = false;
        editTitle = '';
        break;
      case 'description':
        editingDescription = false;
        editDescription = '';
        break;
      case 'status':
      case 'status_id':
        editingStatus = false;
        editStatus = '';
        break;
      case 'priority':
      case 'priority_id':
        editingPriority = false;
        editPriority = '';
        break;
      case 'due_date':
        editingDueDate = false;
        break;
      case 'milestone':
        editingMilestone = false;
        editMilestone = null;
        break;
      case 'iteration':
        editingIteration = false;
        editIteration = null;
        break;
      case 'project':
        editingProject = false;
        editProject = null;
        break;
      case 'assignee':
        editingAssignee = false;
        editAssignee = null;
        break;
      default:
        // Handle custom fields
        if (field.startsWith('custom_field_')) {
          const fieldId = field.replace('custom_field_', '');
          const newEditingCustomFields = { ...editingCustomFields };
          delete newEditingCustomFields[fieldId];
          editingCustomFields = newEditingCustomFields;

          const newEditCustomFieldValues = { ...editCustomFieldValues };
          delete newEditCustomFieldValues[fieldId];
          editCustomFieldValues = newEditCustomFieldValues;
        }
        break;
    }
  }

  /**
   * Checks if any field is currently being edited.
   */
  function isAnyFieldEditing() {
    return editingTitle || editingDescription || editingStatus ||
           editingPriority || editingMilestone || editingIteration ||
           editingProject || editingAssignee ||
           Object.keys(editingCustomFields).length > 0;
  }

  /**
   * Resets all editing state.
   */
  function resetAllEditing() {
    editingTitle = false;
    editingDescription = false;
    editingStatus = false;
    editingPriority = false;
    editingDueDate = false;
    editingMilestone = false;
    editingIteration = false;
    editingProject = false;
    editingAssignee = false;
    editingCustomFields = {};

    editTitle = '';
    editDescription = '';
    editStatus = '';
    editPriority = '';
    editMilestone = null;
    editIteration = null;
    editProject = null;
    editAssignee = null;
    editCustomFieldValues = {};
  }

  return {
    // Editing mode flags (getters)
    get editingTitle() { return editingTitle; },
    get editingDescription() { return editingDescription; },
    get editingStatus() { return editingStatus; },
    get editingPriority() { return editingPriority; },
    get editingDueDate() { return editingDueDate; },
    get editingMilestone() { return editingMilestone; },
    get editingIteration() { return editingIteration; },
    get editingProject() { return editingProject; },
    get editingAssignee() { return editingAssignee; },
    get editingCustomFields() { return editingCustomFields; },

    // Edit values (getters)
    get editTitle() { return editTitle; },
    get editDescription() { return editDescription; },
    get editStatus() { return editStatus; },
    get editPriority() { return editPriority; },
    get editMilestone() { return editMilestone; },
    get editIteration() { return editIteration; },
    get editProject() { return editProject; },
    get editAssignee() { return editAssignee; },
    get editCustomFieldValues() { return editCustomFieldValues; },

    // Setters for edit values
    set editTitle(value) { editTitle = value; },
    set editDescription(value) { editDescription = value; },
    set editStatus(value) { editStatus = value; },
    set editPriority(value) { editPriority = value; },
    set editMilestone(value) { editMilestone = value; },
    set editIteration(value) { editIteration = value; },
    set editProject(value) { editProject = value; },
    set editAssignee(value) { editAssignee = value; },

    // Setters for editing mode
    set editingTitle(value) { editingTitle = value; },
    set editingDescription(value) { editingDescription = value; },
    set editingStatus(value) { editingStatus = value; },
    set editingPriority(value) { editingPriority = value; },
    set editingDueDate(value) { editingDueDate = value; },
    set editingMilestone(value) { editingMilestone = value; },
    set editingIteration(value) { editingIteration = value; },
    set editingProject(value) { editingProject = value; },
    set editingAssignee(value) { editingAssignee = value; },
    set editingCustomFields(value) { editingCustomFields = value; },
    set editCustomFieldValues(value) { editCustomFieldValues = value; },

    // Saving state
    get saving() { return saving; },
    set saving(value) { saving = value; },

    // Methods
    startEditing,
    cancelEdit,
    isAnyFieldEditing,
    resetAllEditing
  };
}
