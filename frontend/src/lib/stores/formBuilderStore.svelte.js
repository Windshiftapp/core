/**
 * Store for managing Form Builder state.
 * Uses Svelte 5 class-based reactive state pattern.
 * Manages form (request type) fields, drag-and-drop, and per-form config.
 */
import { api } from '../api.js';
import { createSearchFilteredFields } from '../utils/fieldSearchUtils.js';
import { getSystemFieldName } from './fieldConfig.js';
import { createFieldObject, fieldExists } from './fieldUtils.js';
import { applyDragStateMixin } from './storeUtils.js';

class FormBuilderStore {
  // === Form List ===
  forms = $state([]);
  loading = $state(false);
  channelId = $state(null);

  // === Selected Form for Field Editing ===
  editingForm = $state(null);
  formFields = $state([]);
  showFieldEditor = $state(false);

  // === Available Fields ===
  availableFields = $state([]);

  // === Per-form Config ===
  formConfig = $state({
    require_auth: false,
    success_message: '',
    submit_button_text: 'Submit',
    redirect_url: '',
  });

  // === Field Search ===
  fieldSearchQuery = $state('');

  // === Drag State ===
  draggedField = $state(null);
  fieldDragState = $state(new Map());

  // === Derived Values ===

  get availableFieldsFiltered() {
    return this.availableFields.filter(
      (field) =>
        !this.formFields.some(
          (ff) => ff.field_type === field.type && ff.field_identifier === field.identifier
        )
    );
  }

  get searchFilteredFields() {
    return createSearchFilteredFields(
      () => this.availableFieldsFiltered,
      () => this.fieldSearchQuery
    );
  }

  // === Data Loading ===

  async loadForms(channelId) {
    try {
      this.loading = true;
      this.channelId = channelId;
      const result = await api.requestTypes.getAllForChannel(channelId);
      this.forms = result || [];
    } catch (err) {
      console.error('Failed to load forms:', err);
      this.forms = [];
    } finally {
      this.loading = false;
    }
  }

  async deleteForm(formId) {
    await api.requestTypes.delete(this.channelId, formId);
    this.forms = this.forms.filter((f) => f.id !== formId);
  }

  // === Field Editor ===

  async startEditFields(form) {
    this.editingForm = form;
    this.showFieldEditor = true;

    try {
      const [fields, available] = await Promise.all([
        api.requestTypes.getFields(form.id),
        api.requestTypes.getAvailableFields(form.id),
      ]);

      this.formFields = fields || [];

      // Map available fields to a consistent format
      this.availableFields = (available || []).map((f) => ({
        identifier: f.identifier,
        name: f.type === 'default' ? getSystemFieldName(f.identifier) : f.name,
        type: f.type,
        fieldType: f.field_type || null,
        category: f.type === 'default' ? 'Default Fields' : 'Custom Fields',
      }));

      // Add virtual field options
      this.availableFields.push(
        {
          identifier: `virtual_text_${Date.now()}`,
          name: 'Text Field',
          type: 'virtual',
          fieldType: 'text',
          category: 'Virtual Fields',
        },
        {
          identifier: `virtual_textarea_${Date.now()}`,
          name: 'Text Area',
          type: 'virtual',
          fieldType: 'textarea',
          category: 'Virtual Fields',
        },
        {
          identifier: `virtual_select_${Date.now()}`,
          name: 'Dropdown',
          type: 'virtual',
          fieldType: 'select',
          category: 'Virtual Fields',
        },
        {
          identifier: `virtual_checkbox_${Date.now()}`,
          name: 'Checkbox',
          type: 'virtual',
          fieldType: 'checkbox',
          category: 'Virtual Fields',
        }
      );

      // Load per-form config
      if (form.config) {
        try {
          const config = typeof form.config === 'string' ? JSON.parse(form.config) : form.config;
          this.formConfig = {
            require_auth: config.require_auth || false,
            success_message: config.success_message || '',
            submit_button_text: config.submit_button_text || 'Submit',
            redirect_url: config.redirect_url || '',
          };
        } catch {
          this.resetFormConfig();
        }
      } else {
        this.resetFormConfig();
      }
    } catch (err) {
      console.error('Failed to load form fields:', err);
      this.formFields = [];
      this.availableFields = [];
    }
  }

  async saveFormFields() {
    try {
      await api.requestTypes.updateFields(this.channelId, this.editingForm.id, this.formFields);
    } catch (err) {
      console.error('Failed to save form fields:', err);
      throw err;
    }
  }

  async saveFormConfig() {
    try {
      await api.requestTypes.updateConfig(this.editingForm.id, this.formConfig);
    } catch (err) {
      console.error('Failed to save form config:', err);
      throw err;
    }
  }

  // === Field Manipulation ===

  addField(fieldData) {
    let identifier = fieldData.identifier;
    if (fieldData.type === 'virtual') {
      identifier = `vf_${fieldData.fieldType}_${Date.now()}`;
    }

    if (fieldExists(this.formFields, fieldData, identifier)) {
      return;
    }

    const newField = createFieldObject({
      fieldData,
      parentId: this.editingForm.id,
      parentType: 'request_type',
      displayOrder: this.formFields.length,
    });

    this.formFields = [...this.formFields, newField];
  }

  addFieldAtPosition(fieldData, targetIndex, closestEdge) {
    let identifier = fieldData.identifier;
    if (fieldData.type === 'virtual') {
      identifier = `vf_${fieldData.fieldType}_${Date.now()}`;
    }

    if (fieldExists(this.formFields, fieldData, identifier)) {
      return;
    }

    const insertIndex = closestEdge === 'bottom' ? targetIndex + 1 : targetIndex;

    const newField = createFieldObject({
      fieldData,
      parentId: this.editingForm.id,
      parentType: 'request_type',
      displayOrder: insertIndex,
    });

    const newFields = [...this.formFields];
    newFields.splice(insertIndex, 0, newField);
    this.formFields = newFields.map((f, i) => ({ ...f, display_order: i }));
  }

  reorderField(fromIndex, toIndex, closestEdge) {
    if (fromIndex === toIndex) return;

    const insertIndex = closestEdge === 'bottom' ? toIndex + 1 : toIndex;
    const adjustedInsertIndex = fromIndex < insertIndex ? insertIndex - 1 : insertIndex;

    const newFields = [...this.formFields];
    const [movedField] = newFields.splice(fromIndex, 1);
    newFields.splice(adjustedInsertIndex, 0, movedField);

    this.formFields = newFields.map((f, i) => ({ ...f, display_order: i }));
  }

  removeField(index) {
    this.formFields = this.formFields
      .filter((_, i) => i !== index)
      .map((field, i) => ({ ...field, display_order: i }));
  }

  toggleFieldRequired(index) {
    const field = this.formFields[index];
    field.is_required = !field.is_required;
    this.formFields = [...this.formFields];
  }

  updateFieldProperty(index, property, value) {
    const field = this.formFields[index];
    field[property] = value;
    this.formFields = [...this.formFields];
  }

  // Drag state methods are added via applyDragStateMixin below.

  // === Helpers ===

  resetFormConfig() {
    this.formConfig = {
      require_auth: false,
      success_message: '',
      submit_button_text: 'Submit',
      redirect_url: '',
    };
  }

  cancelFieldEditor() {
    this.showFieldEditor = false;
    this.editingForm = null;
    this.formFields = [];
    this.availableFields = [];
    this.fieldSearchQuery = '';
    this.clearDragState();
    this.resetFormConfig();
  }

  reset() {
    this.forms = [];
    this.loading = false;
    this.channelId = null;
    this.editingForm = null;
    this.formFields = [];
    this.showFieldEditor = false;
    this.availableFields = [];
    this.fieldSearchQuery = '';
    this.draggedField = null;
    this.fieldDragState = new Map();
    this.resetFormConfig();
  }
}

applyDragStateMixin(FormBuilderStore);

export const formBuilderStore = new FormBuilderStore();
