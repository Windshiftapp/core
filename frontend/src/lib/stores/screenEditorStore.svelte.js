/**
 * Store for managing Screen Editor state.
 * Uses Svelte 5 class-based reactive state pattern.
 * Centralizes screen list, field editing, and drag-and-drop state.
 */
import { api } from '../api.js';
import { SYSTEM_FIELDS, getSystemFieldName } from './fieldConfig.js';

class ScreenEditorStore {
  // === Screens List ===
  screens = $state([]);
  loading = $state(false);

  // === Custom Fields Reference ===
  customFields = $state([]);

  // === Selected Screen for Field Editing ===
  editingScreenFields = $state(null);
  screenFields = $state([]);
  showFieldEditor = $state(false);

  // === Form State ===
  showCreateForm = $state(false);
  editingScreen = $state(null);
  formData = $state({
    name: '',
    description: ''
  });

  // === Field Search ===
  fieldSearchQuery = $state('');

  // === Drag State ===
  draggedField = $state(null);
  fieldDragState = $state(new Map());

  // === Field Width Options ===
  fieldWidths = [
    { value: 'full', label: 'Full width' },
    { value: 'half', label: 'Half width' },
    { value: 'third', label: 'Third width' },
    { value: 'quarter', label: 'Quarter width' }
  ];

  // === Derived Values ===

  /**
   * Combined available fields list (system + custom).
   */
  get allAvailableFields() {
    return [
      // System fields from shared config
      ...SYSTEM_FIELDS.map(field => ({
        ...field,
        type: 'system',
        category: 'System Fields'
      })),
      // Custom fields
      ...this.customFields.map(field => ({
        identifier: field.id.toString(),
        name: field.field_name || field.name,
        type: 'custom',
        category: 'Custom Fields',
        fieldType: field.field_type,
        config: field.field_config
      }))
    ];
  }

  /**
   * Available fields filtered to exclude already-added fields.
   */
  get availableFieldsFiltered() {
    return this.allAvailableFields
      .filter(field =>
        !this.screenFields.some(sf => sf.field_type === field.type && sf.field_identifier === field.identifier)
      )
      .filter(field =>
        // Filter out Title and Status fields since they're always auto-added
        !(field.type === 'system' && (field.identifier === 'title' || field.identifier === 'status'))
      );
  }

  /**
   * Search-filtered available fields.
   */
  get searchFilteredFields() {
    return this.availableFieldsFiltered.filter(field => {
      if (!this.fieldSearchQuery.trim()) return true;
      const query = this.fieldSearchQuery.toLowerCase();
      return field.name.toLowerCase().includes(query) ||
             field.identifier.toLowerCase().includes(query);
    });
  }

  // === Data Loading ===

  async loadScreens() {
    try {
      this.loading = true;
      const result = await api.screens.getAll();
      this.screens = result || [];
    } catch (err) {
      console.error('Failed to load screens:', err);
      this.screens = [];
    } finally {
      this.loading = false;
    }
  }

  async loadCustomFields() {
    try {
      const result = await api.customFields.getAll();
      this.customFields = result || [];
    } catch (err) {
      console.error('Failed to load custom fields:', err);
      this.customFields = [];
    }
  }

  // === Screen CRUD ===

  startCreate() {
    this.showCreateForm = true;
    this.editingScreen = null;
    this.resetForm();
  }

  startEdit(screen) {
    this.editingScreen = screen;
    this.formData = {
      name: screen.name,
      description: screen.description || ''
    };
    this.showCreateForm = true;
  }

  async saveScreen() {
    try {
      if (this.editingScreen) {
        await api.screens.update(this.editingScreen.id, this.formData);
      } else {
        await api.screens.create(this.formData);
      }
      await this.loadScreens();
      this.cancelForm();
    } catch (err) {
      console.error('Failed to save screen:', err);
      throw err;
    }
  }

  async deleteScreen(screen) {
    // Prevent deletion of default screen (ID 1)
    if (screen.id === 1) {
      throw new Error('Cannot delete the default screen');
    }

    try {
      await api.screens.delete(screen.id);
      await this.loadScreens();
    } catch (err) {
      console.error('Failed to delete screen:', err);
      throw err;
    }
  }

  // === Field Editor ===

  async startEditFields(screen) {
    this.editingScreenFields = screen;
    this.showFieldEditor = true;

    try {
      const fields = await api.screens.getFields(screen.id);
      this.screenFields = fields || [];

      // Ensure Title field is always present and first
      const titleField = this.screenFields.find(f => f.field_type === 'system' && f.field_identifier === 'title');
      if (!titleField) {
        const newTitleField = {
          screen_id: screen.id,
          field_type: 'system',
          field_identifier: 'title',
          display_order: 0,
          is_required: true,
          field_width: 'full',
          field_name: 'Title',
          field_label: 'Title'
        };
        this.screenFields = [newTitleField, ...this.screenFields.map(f => ({ ...f, display_order: f.display_order + 1 }))];
      }

      // Ensure Status field is always present (after title)
      const statusField = this.screenFields.find(f => f.field_type === 'system' && f.field_identifier === 'status');
      if (!statusField) {
        const newStatusField = {
          screen_id: screen.id,
          field_type: 'system',
          field_identifier: 'status',
          display_order: 1,
          is_required: false,
          field_width: 'half',
          field_name: 'Status',
          field_label: 'Status'
        };
        const titleIndex = this.screenFields.findIndex(f => f.field_type === 'system' && f.field_identifier === 'title');
        const insertIndex = titleIndex >= 0 ? titleIndex + 1 : 0;
        this.screenFields = [
          ...this.screenFields.slice(0, insertIndex),
          newStatusField,
          ...this.screenFields.slice(insertIndex).map(f => ({ ...f, display_order: f.display_order + 1 }))
        ];
      }

      await this.loadCustomFields();
    } catch (err) {
      console.error('Failed to load screen fields:', err);
      this.screenFields = [];
    }
  }

  async saveScreenFields() {
    try {
      await api.screens.updateFields(this.editingScreenFields.id, this.screenFields);
      this.cancelFieldEditor();
    } catch (err) {
      console.error('Failed to save screen fields:', err);
      throw err;
    }
  }

  // === Field Manipulation ===

  addFieldToScreen(fieldData) {
    // Check if field already exists
    if (this.screenFields.some(f => f.field_type === fieldData.type && f.field_identifier === fieldData.identifier)) {
      return;
    }

    const newField = {
      screen_id: this.editingScreenFields.id,
      field_type: fieldData.type,
      field_identifier: fieldData.identifier,
      display_order: this.screenFields.length,
      is_required: fieldData.identifier === 'title',
      field_width: 'full',
      field_name: fieldData.name,
      field_label: fieldData.name
    };

    if (fieldData.type === 'custom') {
      newField.field_config = fieldData.config;
    }

    this.screenFields = [...this.screenFields, newField];
  }

  addFieldAtPosition(fieldData, targetIndex, closestEdge) {
    // Check if field already exists
    if (this.screenFields.some(f => f.field_type === fieldData.type && f.field_identifier === fieldData.identifier)) {
      return;
    }

    const insertIndex = closestEdge === 'bottom' ? targetIndex + 1 : targetIndex;

    const newField = {
      screen_id: this.editingScreenFields.id,
      field_type: fieldData.type,
      field_identifier: fieldData.identifier,
      display_order: insertIndex,
      is_required: fieldData.identifier === 'title',
      field_width: 'full',
      field_name: fieldData.name,
      field_label: fieldData.name
    };

    if (fieldData.type === 'custom') {
      newField.field_config = fieldData.config;
    }

    const newFields = [...this.screenFields];
    newFields.splice(insertIndex, 0, newField);
    this.screenFields = newFields.map((f, i) => ({ ...f, display_order: i }));
  }

  reorderFieldWithEdge(fromIndex, toIndex, closestEdge) {
    if (fromIndex === toIndex) return;

    const insertIndex = closestEdge === 'bottom' ? toIndex + 1 : toIndex;
    const adjustedInsertIndex = fromIndex < insertIndex ? insertIndex - 1 : insertIndex;

    const newFields = [...this.screenFields];
    const [movedField] = newFields.splice(fromIndex, 1);
    newFields.splice(adjustedInsertIndex, 0, movedField);

    this.screenFields = newFields.map((f, i) => ({ ...f, display_order: i }));
  }

  removeField(index) {
    const field = this.screenFields[index];

    // Prevent removing the Title and Status fields
    if (field.field_type === 'system' && (field.field_identifier === 'title' || field.field_identifier === 'status')) {
      return;
    }

    this.screenFields = this.screenFields
      .filter((_, i) => i !== index)
      .map((field, i) => ({ ...field, display_order: i }));
  }

  toggleFieldRequired(index) {
    const field = this.screenFields[index];
    field.is_required = !field.is_required;
    this.screenFields = [...this.screenFields];
  }

  // === Drag State Management ===

  setDragState(fieldId, state) {
    this.fieldDragState.set(fieldId, state);
    this.fieldDragState = new Map(this.fieldDragState);
  }

  clearDragState() {
    this.fieldDragState.forEach((_, id) => {
      this.fieldDragState.set(id, { closestEdge: null });
    });
    this.fieldDragState = new Map(this.fieldDragState);
  }

  setDraggedField(field) {
    this.draggedField = field;
  }

  clearDraggedField() {
    this.draggedField = null;
  }

  // === Helpers ===

  getFieldWidthLabel(width) {
    return this.fieldWidths.find(w => w.value === width)?.label || width;
  }

  getFieldDisplayName(field) {
    if (field.field_type === 'system') {
      return getSystemFieldName(field.field_identifier);
    }
    return field.field_name || field.field_identifier;
  }

  // === Form Controls ===

  resetForm() {
    this.formData = {
      name: '',
      description: ''
    };
  }

  cancelForm() {
    this.showCreateForm = false;
    this.editingScreen = null;
    this.resetForm();
  }

  cancelFieldEditor() {
    this.showFieldEditor = false;
    this.editingScreenFields = null;
    this.screenFields = [];
    this.customFields = [];
    this.fieldSearchQuery = '';
    this.clearDragState();
  }

  // === Full Reset ===

  reset() {
    this.screens = [];
    this.loading = false;
    this.customFields = [];
    this.editingScreenFields = null;
    this.screenFields = [];
    this.showFieldEditor = false;
    this.showCreateForm = false;
    this.editingScreen = null;
    this.formData = { name: '', description: '' };
    this.fieldSearchQuery = '';
    this.draggedField = null;
    this.fieldDragState = new Map();
  }
}

export const screenEditorStore = new ScreenEditorStore();
