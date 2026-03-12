// Asset CSV Import Store - State management for the import wizard
// Uses Svelte 5 runes for reactivity

import {
  assetCategories,
  assetStatuses,
  assetTypes,
  assetImport as importApi,
} from '../../../api/assets.js';

// Upload state
let uploadState = $state({
  uploadId: null,
  fileName: null,
  fileSize: null,
  headers: [],
  previewRows: [],
  totalRows: 0,
  delimiter: ',',
  hasHeaderRow: true,
  isUploading: false,
  error: null,
  headerWarning: null,
});

// Target state (set and type selection)
let targetState = $state({
  setId: null,
  assetTypeId: null,
  typeFields: [],
  categories: [],
  statuses: [],
  types: [],
  isLoadingMeta: false,
});

// Mapping state
let mappingState = $state({
  title: -1,
  description: -1,
  assetTag: -1,
  categoryId: -1,
  statusId: -1,
  customFields: {},
  categoryMap: {},
  statusMap: {},
});

// Import state
let importState = $state({
  jobId: null,
  isImporting: false,
  phase: 'idle',
  totalRows: 0,
  importedCount: 0,
  failedCount: 0,
  errors: [],
  result: null,
  error: null,
});

// Create type state
let createTypeState = $state({
  isOpen: false,
  name: '',
  description: '',
  icon: 'Box',
  color: '#6b7280',
  suggestedFields: [],
  editedFields: [],
  isLoadingSuggestions: false,
  isCreating: false,
  error: null,
});

// Wizard state
let wizardState = $state({
  currentStep: 0,
  steps: [
    { id: 'upload', label: 'Upload', completed: false },
    { id: 'mapping', label: 'Mapping', completed: false },
    { id: 'preview', label: 'Preview', completed: false },
    { id: 'import', label: 'Import', completed: false },
  ],
});

export const assetImportStore = {
  get upload() {
    return uploadState;
  },
  get target() {
    return targetState;
  },
  get mapping() {
    return mappingState;
  },
  get import() {
    return importState;
  },
  get wizard() {
    return wizardState;
  },
  get createType() {
    return createTypeState;
  },

  // Set target set ID and load metadata
  async setTarget(setId) {
    targetState.setId = setId;
    targetState.isLoadingMeta = true;

    try {
      const [types, categories, statuses] = await Promise.all([
        assetTypes.getAll(setId),
        assetCategories.getAll(setId),
        assetStatuses.getAll(setId),
      ]);
      targetState.types = types || [];
      targetState.categories = categories || [];
      targetState.statuses = statuses || [];
    } catch (err) {
      console.error('Failed to load asset metadata:', err);
    } finally {
      targetState.isLoadingMeta = false;
    }
  },

  // Set asset type and load its fields
  async setAssetType(typeId) {
    targetState.assetTypeId = typeId;
    if (typeId) {
      try {
        const fields = await assetTypes.getFields(typeId);
        targetState.typeFields = fields || [];
      } catch (err) {
        console.error('Failed to load type fields:', err);
        targetState.typeFields = [];
      }
    } else {
      targetState.typeFields = [];
    }
  },

  // Upload CSV file
  async uploadFile(file) {
    if (!targetState.setId) return { success: false, error: 'No set selected' };

    uploadState.isUploading = true;
    uploadState.error = null;

    try {
      const formData = new FormData();
      formData.append('file', file);
      formData.append('has_header', uploadState.hasHeaderRow.toString());

      const result = await importApi.upload(targetState.setId, formData);

      uploadState.uploadId = result.upload_id;
      uploadState.fileName = file.name;
      uploadState.fileSize = file.size;
      uploadState.headers = result.headers || [];
      uploadState.previewRows = result.preview_rows || [];
      uploadState.totalRows = result.total_rows || 0;
      uploadState.delimiter = result.delimiter || ',';
      uploadState.headerWarning = result.header_warning || null;

      wizardState.steps[0].completed = true;
      return { success: true };
    } catch (err) {
      uploadState.error = err.message || 'Failed to upload file';
      return { success: false, error: uploadState.error };
    } finally {
      uploadState.isUploading = false;
    }
  },

  // Auto-map CSV columns to fields
  autoMap() {
    const headers = uploadState.headers.map((h) => h.toLowerCase().trim());

    const findColumn = (...names) => {
      for (const name of names) {
        const idx = headers.findIndex((h) => h === name || h.includes(name) || name.includes(h));
        if (idx >= 0) return idx;
      }
      return -1;
    };

    mappingState.title = findColumn('title', 'name', 'asset name', 'asset_name');
    mappingState.description = findColumn('description', 'desc', 'details', 'notes');
    mappingState.assetTag = findColumn(
      'asset tag',
      'asset_tag',
      'tag',
      'serial',
      'serial number',
      'serial_number',
      'id'
    );
    mappingState.categoryId = findColumn('category', 'group', 'type', 'class');
    mappingState.statusId = findColumn('status', 'state', 'condition');

    // Auto-map custom fields
    for (const field of targetState.typeFields) {
      const fieldName = (field.name || field.field_name || '').toLowerCase();
      const idx = findColumn(fieldName);
      if (idx >= 0) {
        mappingState.customFields[String(field.custom_field_id || field.id)] = idx;
      }
    }
  },

  // Set a field mapping
  setFieldMapping(field, columnIndex) {
    mappingState[field] = columnIndex;
  },

  // Set a custom field mapping
  setCustomFieldMapping(fieldId, columnIndex) {
    mappingState.customFields[String(fieldId)] = columnIndex;
  },

  // Set category value mapping
  setCategoryValueMapping(csvValue, categoryId) {
    mappingState.categoryMap[csvValue] = categoryId;
  },

  // Set status value mapping
  setStatusValueMapping(csvValue, statusId) {
    mappingState.statusMap[csvValue] = statusId;
  },

  // Get unique values from a CSV column for value mapping
  getUniqueColumnValues(columnIndex) {
    if (columnIndex < 0) return [];
    const values = new Set();
    for (const row of uploadState.previewRows) {
      if (row[columnIndex]) {
        values.add(row[columnIndex].trim());
      }
    }
    return Array.from(values).sort();
  },

  // Check if mapping is valid (title must be mapped)
  isMappingValid() {
    return mappingState.title >= 0 && targetState.assetTypeId > 0;
  },

  // Start the import
  async startImport() {
    if (!uploadState.uploadId || !targetState.setId) return;

    importState.isImporting = true;
    importState.error = null;

    try {
      const response = await importApi.start(targetState.setId, {
        upload_id: uploadState.uploadId,
        asset_type_id: targetState.assetTypeId,
        has_header: uploadState.hasHeaderRow,
        delimiter: uploadState.delimiter,
        mappings: {
          title: mappingState.title,
          description: mappingState.description,
          asset_tag: mappingState.assetTag,
          category_id: mappingState.categoryId,
          status_id: mappingState.statusId,
          custom_fields: mappingState.customFields,
        },
        category_map: mappingState.categoryMap,
        status_map: mappingState.statusMap,
      });

      importState.jobId = response.job_id;
      wizardState.steps[2].completed = true;
      wizardState.currentStep = 3;

      this.pollJobStatus();
      return { success: true };
    } catch (err) {
      importState.error = err.message || 'Failed to start import';
      return { success: false, error: importState.error };
    } finally {
      importState.isImporting = false;
    }
  },

  // Poll for job status
  async pollJobStatus() {
    if (!importState.jobId || !targetState.setId) return;

    const poll = async () => {
      try {
        const status = await importApi.getJob(targetState.setId, importState.jobId);
        importState.phase = status.phase || 'running';

        if (status.progress) {
          importState.totalRows = status.progress.total_rows || 0;
          importState.importedCount = status.progress.imported_count || 0;
          importState.failedCount = status.progress.failed_count || 0;
          importState.errors = status.progress.errors || [];
        }

        if (status.status === 'completed') {
          importState.result = status;
          wizardState.steps[3].completed = true;
          return;
        } else if (status.status === 'failed') {
          importState.error = status.error_message || 'Import failed';
          return;
        }

        setTimeout(poll, 2000);
      } catch (err) {
        console.error('Failed to poll import status:', err);
        setTimeout(poll, 5000);
      }
    };

    poll();
  },

  // Create type from import
  toggleCreateType() {
    createTypeState.isOpen = !createTypeState.isOpen;
    if (!createTypeState.isOpen) {
      createTypeState.error = null;
    }
  },

  async suggestFields() {
    if (!uploadState.uploadId || !targetState.setId) return;

    createTypeState.isLoadingSuggestions = true;
    createTypeState.error = null;

    try {
      const result = await importApi.suggestFields(targetState.setId, {
        upload_id: uploadState.uploadId,
        has_header: uploadState.hasHeaderRow,
        delimiter: uploadState.delimiter,
      });

      createTypeState.suggestedFields = result.suggested_fields || [];
      createTypeState.editedFields = (result.suggested_fields || [])
        .filter((f) => !f.is_standard)
        .map((f, i) => ({
          name: f.suggested_name,
          field_type: f.suggested_type,
          options: f.options || [],
          is_required: false,
          display_order: i,
          sample_values: f.sample_values || [],
          enabled: true,
        }));
    } catch (err) {
      createTypeState.error = err.message || 'Failed to suggest fields';
    } finally {
      createTypeState.isLoadingSuggestions = false;
    }
  },

  async createTypeFromImport() {
    if (!targetState.setId || !createTypeState.name) return;

    createTypeState.isCreating = true;
    createTypeState.error = null;

    try {
      const enabledFields = createTypeState.editedFields
        .filter((f) => f.enabled)
        .map((f, i) => ({
          name: f.name,
          field_type: f.field_type,
          options: f.field_type === 'select' ? f.options : undefined,
          is_required: f.is_required,
          display_order: i,
        }));

      const result = await importApi.createType(targetState.setId, {
        name: createTypeState.name,
        description: createTypeState.description,
        icon: createTypeState.icon,
        color: createTypeState.color,
        fields: enabledFields,
      });

      // Add new type to the list and select it
      const newType = result.asset_type;
      targetState.types = [...targetState.types, newType];
      targetState.assetTypeId = newType.id;
      targetState.typeFields = result.fields || [];

      // Close the panel
      createTypeState.isOpen = false;

      return { success: true };
    } catch (err) {
      createTypeState.error = err.message || 'Failed to create type';
      return { success: false, error: createTypeState.error };
    } finally {
      createTypeState.isCreating = false;
    }
  },

  // Navigation
  nextStep() {
    if (wizardState.currentStep < wizardState.steps.length - 1) {
      wizardState.currentStep++;
    }
  },

  prevStep() {
    if (wizardState.currentStep > 0) {
      wizardState.currentStep--;
    }
  },

  goToStep(stepIndex) {
    if (stepIndex >= 0 && stepIndex < wizardState.steps.length) {
      wizardState.currentStep = stepIndex;
    }
  },

  // Reset
  reset() {
    uploadState = {
      uploadId: null,
      fileName: null,
      fileSize: null,
      headers: [],
      previewRows: [],
      totalRows: 0,
      delimiter: ',',
      hasHeaderRow: true,
      isUploading: false,
      error: null,
      headerWarning: null,
    };

    targetState = {
      setId: null,
      assetTypeId: null,
      typeFields: [],
      categories: [],
      statuses: [],
      types: [],
      isLoadingMeta: false,
    };

    mappingState = {
      title: -1,
      description: -1,
      assetTag: -1,
      categoryId: -1,
      statusId: -1,
      customFields: {},
      categoryMap: {},
      statusMap: {},
    };

    importState = {
      jobId: null,
      isImporting: false,
      phase: 'idle',
      totalRows: 0,
      importedCount: 0,
      failedCount: 0,
      errors: [],
      result: null,
      error: null,
    };

    createTypeState = {
      isOpen: false,
      name: '',
      description: '',
      icon: 'Box',
      color: '#6b7280',
      suggestedFields: [],
      editedFields: [],
      isLoadingSuggestions: false,
      isCreating: false,
      error: null,
    };

    wizardState = {
      currentStep: 0,
      steps: wizardState.steps.map((s) => ({ ...s, completed: false })),
    };
  },
};

export default assetImportStore;
