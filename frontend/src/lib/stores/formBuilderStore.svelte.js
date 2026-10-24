/**
 * Store for managing Form Builder state.
 * Uses Svelte 5 class-based reactive state pattern.
 * Manages form (request type) fields, drag-and-drop, and per-form config.
 */
import { requestTypes } from '../api/channels.js';
import { createSearchFilteredFields } from '../utils/fieldSearchUtils.js';
import { DragStateStore } from './DragStateStore.svelte.js';
import { getSystemFieldName } from './fieldConfig.js';
import { createFieldObject, fieldExists } from './fieldUtils.js';

class FormBuilderStore extends DragStateStore {
  // === Form List ===
  forms = $state([]);
  loading = $state(false);
  channelId = $state(null);

  // === Selected Form for Field Editing ===
  editingForm = $state(null);
  formFields = $state([]);
  showFieldEditor = $state(false);
  savedFieldsSnapshot = $state('');
  savedConfigSnapshot = $state('');
  savedRoutingSnapshot = $state('');

  // === Available Fields ===
  availableFields = $state([]);

  // === Per-form Config ===
  formConfig = $state({
    require_auth: false,
    allow_attachments: false,
    success_message: '',
    submit_button_text: 'Submit',
    redirect_url: '',
  });

  // === Routing Metadata (name/description/icon/color + routing target) ===
  // Editable identity + routing fields, persisted via the request-type Update
  // endpoint. workspace_id / item_type_id determine where submissions land and
  // which fields resolve, so the builder needs to correct them after create.
  routingMeta = $state({
    name: '',
    description: '',
    icon: 'FileText',
    color: '#6b7280',
    workspace_id: null,
    item_type_id: null,
  });

  // === Field Search ===
  fieldSearchQuery = $state('');

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

  get hasUnsavedChanges() {
    if (!this.showFieldEditor) return false;
    return (
      JSON.stringify(this.formFields) !== this.savedFieldsSnapshot ||
      JSON.stringify(this.formConfig) !== this.savedConfigSnapshot ||
      this.hasUnsavedRoutingChanges
    );
  }

  get hasUnsavedRoutingChanges() {
    return JSON.stringify(this.routingMeta) !== this.savedRoutingSnapshot;
  }

  // === Data Loading ===

  async loadForms(channelId) {
    try {
      this.loading = true;
      this.channelId = channelId;
      const result = await requestTypes.getAllForChannel(channelId);
      this.forms = result || [];
    } catch (err) {
      console.error('Failed to load forms:', err);
      this.forms = [];
    } finally {
      this.loading = false;
    }
  }

  async deleteForm(formId) {
    await requestTypes.delete(this.channelId, formId);
    this.forms = this.forms.filter((f) => f.id !== formId);
  }

  // === Field Editor ===

  async startEditFields(form) {
    this.editingForm = form;
    this.showFieldEditor = true;
    this.routingMeta = {
      name: form.name || '',
      description: form.description || '',
      icon: form.icon || 'FileText',
      color: form.color || '#6b7280',
      workspace_id: form.workspace_id ?? null,
      item_type_id: form.item_type_id ?? null,
    };

    try {
      const [fields, available] = await Promise.all([
        requestTypes.getFields(form.id),
        requestTypes.getAvailableFields(form.id),
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
            allow_attachments: config.allow_attachments === true,
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
      this.markBuilderSaved();
    } catch (err) {
      console.error('Failed to load form fields:', err);
      this.formFields = [];
      this.availableFields = [];
    }
  }

  async saveFormFields() {
    try {
      await requestTypes.updateFields(this.channelId, this.editingForm.id, this.formFields);
      this.savedFieldsSnapshot = JSON.stringify(this.formFields);
    } catch (err) {
      console.error('Failed to save form fields:', err);
      throw err;
    }
  }

  async saveFormConfig() {
    try {
      await requestTypes.updateConfig(this.editingForm.id, this.formConfig);
      this.savedConfigSnapshot = JSON.stringify(this.formConfig);
    } catch (err) {
      console.error('Failed to save form config:', err);
      throw err;
    }
  }

  async saveRoutingMetadata() {
    try {
      // The Update endpoint preserves omitted fields; send the editable
      // identity and routing fields the builder exposes.
      const updated = await requestTypes.update(this.channelId, this.editingForm.id, {
        name: this.routingMeta.name.trim(),
        description: (this.routingMeta.description || '').trim(),
        icon: this.routingMeta.icon,
        color: this.routingMeta.color,
        item_type_id: this.routingMeta.item_type_id,
        workspace_id: this.routingMeta.workspace_id || null,
        is_active: this.editingForm.is_active ?? true,
      });
      // Reflect the saved values locally so the list + header stay in sync
      // without a full reload.
      this.editingForm = { ...this.editingForm, ...this.routingMeta };
      this.forms = this.forms.map((f) =>
        f.id === this.editingForm.id ? { ...f, ...this.routingMeta } : f
      );
      this.savedRoutingSnapshot = JSON.stringify(this.routingMeta);
      return updated;
    } catch (err) {
      console.error('Failed to save routing metadata:', err);
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

  // === Helpers ===

  resetFormConfig() {
    this.formConfig = {
      require_auth: false,
      allow_attachments: false,
      success_message: '',
      submit_button_text: 'Submit',
      redirect_url: '',
    };
  }

  resetRoutingMeta() {
    this.routingMeta = {
      name: '',
      description: '',
      icon: 'FileText',
      color: '#6b7280',
      workspace_id: null,
      item_type_id: null,
    };
  }

  markBuilderSaved() {
    this.savedFieldsSnapshot = JSON.stringify(this.formFields);
    this.savedConfigSnapshot = JSON.stringify(this.formConfig);
    this.savedRoutingSnapshot = JSON.stringify(this.routingMeta);
  }

  cancelFieldEditor() {
    this.showFieldEditor = false;
    this.editingForm = null;
    this.formFields = [];
    this.availableFields = [];
    this.fieldSearchQuery = '';
    this.clearDragState();
    this.savedFieldsSnapshot = '';
    this.savedConfigSnapshot = '';
    this.savedRoutingSnapshot = '';
    this.resetFormConfig();
    this.resetRoutingMeta();
  }

  reset() {
    this.forms = [];
    this.loading = false;
    this.channelId = null;
    this.editingForm = null;
    this.formFields = [];
    this.showFieldEditor = false;
    this.availableFields = [];
    this.savedFieldsSnapshot = '';
    this.savedConfigSnapshot = '';
    this.savedRoutingSnapshot = '';
    this.fieldSearchQuery = '';
    this.resetDragState();
    this.resetFormConfig();
    this.resetRoutingMeta();
  }
}

export const formBuilderStore = new FormBuilderStore();
