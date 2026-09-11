/**
 * Store for managing Screen Editor state.
 * Uses Svelte 5 class-based reactive state pattern.
 * Centralizes screen list, field editing, and drag-and-drop state.
 */
import { api } from '../api.js';
import { createSearchFilteredFields } from '../utils/fieldSearchUtils.js';
import {
  ALWAYS_VISIBLE_SYSTEM_FIELDS,
  canSystemFieldBeRequiredOnCreate,
  isAlwaysVisibleSystemField,
} from '../utils/screenFields.js';
import { DragStateStore } from './DragStateStore.svelte.js';
import { getSystemFieldName, SYSTEM_FIELDS } from './fieldConfig.js';

const ALWAYS_VISIBLE_SYSTEM_FIELD_DEFAULTS = {
  title: { is_required: true, field_width: 'full' },
  description: { is_required: false, field_width: 'full' },
  status: { is_required: false, field_width: 'half' },
};

class ScreenEditorStore extends DragStateStore {
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
    description: '',
  });

  // === Field Search ===
  fieldSearchQuery = $state('');

  // === Field Width Options ===
  fieldWidths = [
    { value: 'full', label: 'Full width' },
    { value: 'half', label: 'Half width' },
    { value: 'third', label: 'Third width' },
    { value: 'quarter', label: 'Quarter width' },
  ];

  // === Derived Values ===

  /**
   * Combined available fields list (system + custom).
   */
  get allAvailableFields() {
    return [
      // System fields from shared config
      ...SYSTEM_FIELDS.map((field) => ({
        ...field,
        type: 'system',
        category: 'System Fields',
      })),
      // Custom fields
      ...this.customFields.map((field) => ({
        identifier: field.id.toString(),
        name: field.field_name || field.name,
        type: 'custom',
        category: 'Custom Fields',
        fieldType: field.field_type,
        config: field.field_config,
      })),
    ];
  }

  /**
   * Available fields filtered to exclude already-added fields.
   */
  get availableFieldsFiltered() {
    return this.allAvailableFields
      .filter(
        (field) =>
          !this.screenFields.some(
            (sf) => sf.field_type === field.type && sf.field_identifier === field.identifier
          )
      )
      .filter(
        (field) =>
          // Filter out always-visible fields since they're auto-added and locked.
          !(field.type === 'system' && isAlwaysVisibleSystemField(field.identifier))
      );
  }

  /**
   * Search-filtered available fields.
   */
  get searchFilteredFields() {
    return createSearchFilteredFields(
      () => this.availableFieldsFiltered,
      () => this.fieldSearchQuery
    );
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
      this.customFields = await api.customFields.getAll();
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
      description: screen.description || '',
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
      this.ensureAlwaysVisibleSystemFields(screen.id);

      await this.loadCustomFields();
    } catch (err) {
      console.error('Failed to load screen fields:', err);
      this.screenFields = [];
    }
  }

  ensureAlwaysVisibleSystemFields(screenId) {
    let fields = [...this.screenFields];

    for (const identifier of ALWAYS_VISIBLE_SYSTEM_FIELDS) {
      const fieldName = getSystemFieldName(identifier);
      const defaults = ALWAYS_VISIBLE_SYSTEM_FIELD_DEFAULTS[identifier];
      const existingIndex = fields.findIndex(
        (field) => field.field_type === 'system' && field.field_identifier === identifier
      );
      if (existingIndex >= 0) {
        fields[existingIndex] = {
          ...fields[existingIndex],
          is_required: defaults.is_required,
          field_width: defaults.field_width,
        };
        continue;
      }

      const insertIndex = this.#alwaysVisibleFieldInsertIndex(fields, identifier);
      fields.splice(insertIndex, 0, {
        screen_id: screenId,
        field_type: 'system',
        field_identifier: identifier,
        display_order: insertIndex,
        is_required: defaults.is_required,
        field_width: defaults.field_width,
        field_name: fieldName,
        field_label: fieldName,
      });
    }

    this.screenFields = fields.map((field, index) => ({ ...field, display_order: index }));
  }

  #alwaysVisibleFieldInsertIndex(fields, identifier) {
    const orderIndex = ALWAYS_VISIBLE_SYSTEM_FIELDS.indexOf(identifier);

    for (let i = orderIndex + 1; i < ALWAYS_VISIBLE_SYSTEM_FIELDS.length; i += 1) {
      const nextIndex = fields.findIndex(
        (field) =>
          field.field_type === 'system' &&
          field.field_identifier === ALWAYS_VISIBLE_SYSTEM_FIELDS[i]
      );
      if (nextIndex >= 0) return nextIndex;
    }

    for (let i = orderIndex - 1; i >= 0; i -= 1) {
      const previousIndex = fields.findIndex(
        (field) =>
          field.field_type === 'system' &&
          field.field_identifier === ALWAYS_VISIBLE_SYSTEM_FIELDS[i]
      );
      if (previousIndex >= 0) return previousIndex + 1;
    }

    return Math.min(orderIndex, fields.length);
  }

  async saveScreenFields() {
    try {
      this.ensureAlwaysVisibleSystemFields(this.editingScreenFields.id);
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
    if (
      this.screenFields.some(
        (f) => f.field_type === fieldData.type && f.field_identifier === fieldData.identifier
      )
    ) {
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
      field_label: fieldData.name,
    };

    if (fieldData.type === 'custom') {
      newField.field_config = fieldData.config;
    }

    this.screenFields = [...this.screenFields, newField];
  }

  addFieldAtPosition(fieldData, targetIndex, closestEdge) {
    // Check if field already exists
    if (
      this.screenFields.some(
        (f) => f.field_type === fieldData.type && f.field_identifier === fieldData.identifier
      )
    ) {
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
      field_label: fieldData.name,
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

    // Prevent removing fields that are always visible on item screens.
    if (field.field_type === 'system' && isAlwaysVisibleSystemField(field.field_identifier)) {
      return;
    }

    this.screenFields = this.screenFields
      .filter((_, i) => i !== index)
      .map((field, i) => ({ ...field, display_order: i }));
  }

  canFieldBeRequired(field) {
    if (!field) return false;
    if (field.field_type !== 'system') return true;
    return canSystemFieldBeRequiredOnCreate(field.field_identifier);
  }

  toggleFieldRequired(index) {
    const field = this.screenFields[index];
    // Allow clearing legacy invalid required flags, but do not allow enabling
    // required for fields the create form cannot satisfy.
    if (!field.is_required && !this.canFieldBeRequired(field)) return;
    field.is_required = !field.is_required;
    this.screenFields = [...this.screenFields];
  }

  // === Helpers ===

  getFieldWidthLabel(width) {
    return this.fieldWidths.find((w) => w.value === width)?.label || width;
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
      description: '',
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
    this.resetDragState();
  }
}

export const screenEditorStore = new ScreenEditorStore();
