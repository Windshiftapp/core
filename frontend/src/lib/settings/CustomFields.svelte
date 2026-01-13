<script>
  import { onMount, onDestroy } from 'svelte';
  import { api } from '../api.js';
  import { Plus, Edit, Trash2, MoreHorizontal, Circle, Database } from 'lucide-svelte';
  import Button from '../components/Button.svelte';
  import Input from '../components/Input.svelte';
  import Select from '../components/Select.svelte';
  import Textarea from '../components/Textarea.svelte';
  import DataTable from '../components/DataTable.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import SearchInput from '../components/SearchInput.svelte';
  import Pagination from '../components/Pagination.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import Toggle from '../components/Toggle.svelte';
  import Label from '../components/Label.svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';
  import { matchesShortcut } from '../utils/keyboardShortcuts.js';

  let customFields = [];
  let screens = [];
  let showCreateForm = false;
  let editingField = null;
  let formData = {
    field_name: '',
    field_type: 'text',
    field_config: { max_length: '' },
    applies_to_portal_customers: false,
    applies_to_customer_organisations: false
  };

  let optionsText = ''; // For managing select/multiselect options

  // Search and pagination state
  let searchQuery = '';
  let currentPage = 1;
  let itemsPerPage = 25;

  const fieldTypes = [
    { value: 'text', label: 'Single Line Text' },
    { value: 'textarea', label: 'Multi Line Text' },
    { value: 'select', label: 'Single Select' },
    { value: 'multiselect', label: 'Multi Select' },
    { value: 'number', label: 'Number' },
    { value: 'date', label: 'Date' },
    { value: 'user', label: 'User' },
    { value: 'iteration', label: 'Iteration' },
    { value: 'milestone', label: 'Milestone' },
    { value: 'asset', label: 'Asset' }
  ];

  // Asset field configuration
  let assetSetId = null;
  let assetQlQuery = '';
  let assetSets = [];

  onMount(async () => {
    await loadCustomFields();

    // Load asset sets for asset field type
    try {
      assetSets = await api.assetSets.getAll() || [];
    } catch (error) {
      console.error('Failed to load asset sets:', error);
      assetSets = [];
    }

    // Add global keyboard listener
    window.addEventListener('keydown', handleGlobalKeydown);
  });

  onDestroy(() => {
    window.removeEventListener('keydown', handleGlobalKeydown);
  });

  function handleGlobalKeydown(event) {
    // 'a' to add/create new custom field
    if (matchesShortcut(event, { key: 'a' }) && !showCreateForm) {
      const target = event.target;
      if (target.tagName !== 'INPUT' && target.tagName !== 'TEXTAREA' && !target.contentEditable.includes('true')) {
        event.preventDefault();
        startCreate();
      }
    }
  }

  async function loadCustomFields() {
    try {
      const [fieldsResult, screensResult] = await Promise.all([
        api.customFields.getAll(),
        api.screens.getAll()
      ]);
      customFields = fieldsResult || [];
      
      // Load screen fields for each screen
      const screensWithFields = await Promise.all(
        (screensResult || []).map(async (screen) => {
          try {
            const fields = await api.screens.getFields(screen.id);
            return { ...screen, fields: fields || [] };
          } catch (error) {
            console.error(`Failed to load fields for screen ${screen.id}:`, error);
            return { ...screen, fields: [] };
          }
        })
      );
      
      screens = screensWithFields;
    } catch (error) {
      console.error('Failed to load custom fields:', error);
      customFields = [];
      screens = [];
    }
  }


  function startCreate() {
    showCreateForm = true;
    editingField = null;
    resetForm();
  }

  function startEdit(field) {
    editingField = field;
    formData = {
      field_name: field.name,
      field_type: field.field_type,
      field_config: { max_length: '' },
      applies_to_portal_customers: field.applies_to_portal_customers || false,
      applies_to_customer_organisations: field.applies_to_customer_organisations || false
    };

    // Parse options from JSON string for editing
    if (field.options) {
      try {
        const options = JSON.parse(field.options);
        if (Array.isArray(options)) {
          optionsText = options.join('\n');
        } else {
          optionsText = '';
        }
      } catch (e) {
        optionsText = '';
      }
    } else {
      optionsText = '';
    }

    // Parse asset field config
    if (field.field_type === 'asset' && field.options) {
      try {
        const config = JSON.parse(field.options);
        assetSetId = config.asset_set_id || null;
        assetQlQuery = config.ql_query || '';
      } catch (e) {
        assetSetId = null;
        assetQlQuery = '';
      }
    } else {
      assetSetId = null;
      assetQlQuery = '';
    }

    showCreateForm = true;
  }

  function resetForm() {
    formData = {
      field_name: '',
      field_type: 'text',
      field_config: { max_length: '' },
      applies_to_portal_customers: false,
      applies_to_customer_organisations: false
    };
    optionsText = '';
    assetSetId = null;
    assetQlQuery = '';
  }

  function cancelForm() {
    showCreateForm = false;
    editingField = null;
    resetForm();
  }

  function processFieldConfig() {
    const config = { ...formData.field_config };
    
    if (formData.field_type === 'select' || formData.field_type === 'multiselect') {
      // Process options from textarea
      const options = optionsText
        .split('\n')
        .map(line => line.trim())
        .filter(line => line.length > 0);
      
      if (options.length === 0) {
        throw new Error('At least one option is required for select fields');
      }
      
      config.options = options;
    } else if (formData.field_type === 'text' || formData.field_type === 'textarea') {
      // Handle text field configuration
      if (formData.field_config.max_length) {
        config.max_length = parseInt(formData.field_config.max_length);
      }
    } else if (formData.field_type === 'milestone') {
      // Milestone fields don't need special configuration
      // They reference existing milestones from the system
    } else if (formData.field_type === 'date') {
      // Date fields don't need special configuration
      // They store dates in YYYY-MM-DD format
    } else if (formData.field_type === 'asset') {
      // Asset fields require asset_set_id and optional ql_query
      if (!assetSetId) {
        throw new Error('Asset fields require an asset set');
      }
      config.asset_set_id = assetSetId;
      config.ql_query = assetQlQuery || '';
    }

    return config;
  }

  async function saveField() {
    try {
      // Process field configuration based on type
      const processedConfig = processFieldConfig();

      const data = {
        name: formData.field_name,
        field_type: formData.field_type,
        description: formData.description || '',
        required: formData.required || false,
        applies_to_portal_customers: formData.applies_to_portal_customers || false,
        applies_to_customer_organisations: formData.applies_to_customer_organisations || false
      };

      // Convert config to options format expected by backend
      if (processedConfig.options) {
        data.options = JSON.stringify(processedConfig.options);
      } else if (formData.field_type === 'asset') {
        // Asset fields store config as JSON in options
        data.options = JSON.stringify({
          asset_set_id: processedConfig.asset_set_id,
          ql_query: processedConfig.ql_query
        });
      }

      if (editingField) {
        await api.customFields.update(editingField.id, data);
      } else {
        await api.customFields.create(data);
      }

      await loadCustomFields();
      cancelForm();
    } catch (error) {
      console.error('Failed to save custom field:', error);
      alert('Failed to save custom field: ' + (error.message || error));
    }
  }

  async function deleteField(field) {
    if (confirm(`Are you sure you want to delete the custom field "${field.name}"? This will remove it from all projects.`)) {
      try {
        await api.customFields.delete(field.id);
        await loadCustomFields();
      } catch (error) {
        console.error('Failed to delete custom field:', error);
        alert('Failed to delete custom field: ' + (error.message || error));
      }
    }
  }

  function getFieldTypeLabel(type) {
    return fieldTypes.find(t => t.value === type)?.label || type;
  }

  function getScreenCount(fieldId) {
    if (!screens || screens.length === 0) {
      console.warn('No screens loaded');
      return 0;
    }
    
    const count = screens.filter(screen => {
      if (!screen.fields || screen.fields.length === 0) {
        return false;
      }
      
      return screen.fields.some(field => {
        // Convert both to strings for comparison to handle type mismatches
        const fieldIdStr = fieldId.toString();
        const identifierStr = field.field_identifier.toString();
        const isMatch = field.field_type === 'custom' && identifierStr === fieldIdStr;
        
        if (isMatch) {
        }
        
        // Debug: log comparison details
        if (field.field_type === 'custom') {
        }
        
        return isMatch;
      });
    }).length;
    
    return count;
  }

  $: needsOptions = formData.field_type === 'select' || formData.field_type === 'multiselect';
  $: needsMaxLength = formData.field_type === 'text' || formData.field_type === 'textarea';
  $: isMilestoneField = formData.field_type === 'milestone';
  $: isDateField = formData.field_type === 'date';
  $: isAssetField = formData.field_type === 'asset';
  
  // Reactive statement to trigger re-calculation when screens data changes
  $: screensLoaded = screens && screens.length > 0;
  
  // Reactive computed screen counts for all fields - triggers when screens or customFields change
  $: fieldScreenCounts = customFields.reduce((acc, field) => {
    if (screensLoaded) {
      acc[field.id] = getScreenCount(field.id);
    } else {
      acc[field.id] = 0;
    }
    return acc;
  }, {});

  // Search filtering - filters custom fields by name, type, or description
  $: filteredCustomFields = customFields.filter(field => {
    if (!searchQuery.trim()) return true;

    const query = searchQuery.toLowerCase();
    return (
      field.name?.toLowerCase().includes(query) ||
      field.field_type?.toLowerCase().includes(query) ||
      field.description?.toLowerCase().includes(query) ||
      getFieldTypeLabel(field.field_type)?.toLowerCase().includes(query)
    );
  });

  // Reset to page 1 when search query changes
  $: if (searchQuery) {
    currentPage = 1;
  }

  // Pagination logic - slice filtered results based on current page
  $: totalPages = Math.ceil(filteredCustomFields.length / itemsPerPage);
  $: paginatedCustomFields = filteredCustomFields.slice(
    (currentPage - 1) * itemsPerPage,
    currentPage * itemsPerPage
  );

  // Handle page change
  function handlePageChange(event) {
    currentPage = event.detail;
  }

  // Handle page size change
  function handlePageSizeChange(event) {
    itemsPerPage = event.detail;
    currentPage = 1; // Reset to first page when changing page size
  }

  function buildFieldDropdownItems(field) {
    const items = [
      {
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: 'Edit',
        hoverClass: 'hover:bg-gray-100',
        onClick: () => startEdit(field)
      }
    ];

    // Only add delete option for non-system default fields
    if (!field.system_default) {
      items.push({
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: 'Delete',
        color: '#dc2626',
        hoverClass: 'hover:bg-red-50',
        onClick: () => deleteField(field)
      });
    }

    return items;
  }

  // Table column definitions
  const fieldColumns = [
    {
      key: 'id',
      label: 'Field ID',
      render: (field) => field.id,
      textColor: '#3b82f6'
    },
    {
      key: 'name',
      label: 'Field Name',
      slot: 'name'
    },
    {
      key: 'field_type',
      label: 'Type',
      slot: 'type'
    },
    {
      key: 'options',
      label: 'Configuration',
      render: (field) => {
        if (field.options) {
          try {
            const options = JSON.parse(field.options);
            if (Array.isArray(options)) {
              return `${options.length} options`;
            } else if (field.field_type === 'asset' && options.asset_set_id) {
              const set = assetSets.find(s => s.id === options.asset_set_id);
              const setName = set ? set.name : `Set #${options.asset_set_id}`;
              return options.ql_query ? `${setName} (filtered)` : setName;
            }
            return '—';
          } catch (e) {
            return '—';
          }
        }
        return '—';
      },
      textColor: 'var(--ds-text-subtle)'
    },
    {
      key: 'screen_usage',
      label: 'Used in Screens',
      slot: 'usage'
    },
    {
      key: 'created_at',
      label: 'Created',
      render: (field) => new Date(field.created_at).toLocaleDateString(),
      textColor: 'var(--ds-text-subtle)'
    },
    {
      key: 'actions',
      label: 'Actions'
    }
  ];
</script>

<PageHeader
  icon={Database}
  title="Custom Fields"
  subtitle="Define custom fields for issues and projects"
>
  {#snippet actions()}
    <div class="flex items-center gap-3">
      <SearchInput
        bind:value={searchQuery}
        placeholder="Search custom fields..."
        class="w-64"
      />
      <Button
        variant="primary"
        icon={Plus}
        onclick={startCreate}
        keyboardHint="A"
      >
        Add Custom Field
      </Button>
    </div>
  {/snippet}
</PageHeader>


<Modal isOpen={showCreateForm} onclose={cancelForm} maxWidth="max-w-2xl">
  <!-- Modal header -->
  <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
    <h3 class="text-lg font-semibold" style="color: var(--ds-text);">
      {editingField ? 'Edit Custom Field' : 'Create Custom Field'}
    </h3>
  </div>

  <!-- Modal content -->
  <div class="px-6 py-4">
    <form onsubmit={(e) => { e.preventDefault(); saveField(); }}>
      <div class="grid grid-cols-1 md:grid-cols-2 gap-6">

        <div>
          <Label for="field-name" required class="mb-2">Field Name</Label>
          <Input
            id="field-name"
            bind:value={formData.field_name}
            placeholder="e.g., Sprint, Epic, Customer Impact"
            required
          />
        </div>

        <div>
          <Label for="field-type" required class="mb-2">Field Type</Label>
          <Select
            id="field-type"
            bind:value={formData.field_type}
            required
          >
            {#each fieldTypes as type}
              <option value={type.value}>{type.label}</option>
            {/each}
          </Select>
          {#if isMilestoneField}
            <p class="text-sm mt-2 text-blue-600 bg-blue-50 p-2 rounded">
              📌 Milestone fields automatically reference system milestones. Users will be able to select from existing milestones when filling out this field.
            </p>
          {/if}
          {#if isDateField}
            <p class="text-sm mt-2 text-green-600 bg-green-50 p-2 rounded">
              📅 Date fields allow users to select dates using a date picker. Values are stored in YYYY-MM-DD format.
            </p>
          {/if}
          {#if isAssetField}
            <p class="text-sm mt-2 text-purple-600 bg-purple-50 p-2 rounded">
              📦 Asset fields allow users to select assets from a specified asset set. You can optionally filter available assets using a QL query.
            </p>
          {/if}
        </div>
      </div>

      <!-- Applies To Section -->
      <div class="col-span-2 mt-4">
        <Label class="mb-3">Applies To</Label>
        <div class="flex flex-col gap-3">
          <Toggle
            bind:checked={formData.applies_to_portal_customers}
            label="Portal Customers"
            size="small"
          />
          <Toggle
            bind:checked={formData.applies_to_customer_organisations}
            label="Customer Organisations"
            size="small"
          />
        </div>
        <p class="text-xs mt-2" style="color: var(--ds-text-subtle);">
          Note: Work items use screen-based field configuration
        </p>
      </div>

      {#if needsMaxLength}
        <div class="mt-6">
          <Label for="field-max-length" class="mb-2">Maximum Length (optional)</Label>
          <Input
            id="field-max-length"
            type="number"
            bind:value={formData.field_config.max_length}
            min={1}
            placeholder="Leave empty for no limit"
          />
        </div>
      {/if}

      {#if needsOptions}
        <div class="mt-6">
          <Label for="field-options" required class="mb-2">Options (one per line)</Label>
          <Textarea
            id="field-options"
            bind:value={optionsText}
            rows={6}
            placeholder="Option 1&#10;Option 2&#10;Option 3"
            required
          />
          <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">Enter each option on a separate line</p>
        </div>
      {/if}

      {#if isAssetField}
        <div class="mt-6">
          <Label for="asset-set" required class="mb-2">Asset Set</Label>
          <Select id="asset-set" bind:value={assetSetId} required>
            <option value={null}>Select asset set...</option>
            {#each assetSets as set}
              <option value={set.id}>{set.name}</option>
            {/each}
          </Select>
        </div>

        <div class="mt-4">
          <Label for="asset-ql" class="mb-2">Filter Query (QL)</Label>
          <Textarea
            id="asset-ql"
            bind:value={assetQlQuery}
            rows={3}
            placeholder='e.g., type = "Laptop" AND status = "Active"'
          />
          <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
            Optional: Filter assets shown to users. Leave empty to show all assets in the set.
          </p>
        </div>
      {/if}
    </form>
  </div>

  <DialogFooter
    onCancel={cancelForm}
    onConfirm={saveField}
    confirmLabel={editingField ? 'Update Field' : 'Create Field'}
    disabled={!formData.field_name.trim() || (needsOptions && !optionsText.trim()) || (isAssetField && !assetSetId)}
  />
</Modal>

  <div class="mb-6">
    <DataTable
      columns={fieldColumns}
      data={paginatedCustomFields}
      keyField="id"
      emptyMessage="No custom fields found. Create your first custom field to get started."
      emptyIcon={Circle}
      actionItems={buildFieldDropdownItems}
    >
      <div slot="name" let:item={field}>
        <span>{field.name}</span>
      </div>

      <Lozenge slot="type" let:item={field} color="blue" text={getFieldTypeLabel(field.field_type)} />

      <div slot="usage" let:item={field} class="text-sm">
        {#if screensLoaded}
          {(() => {
            const count = fieldScreenCounts[field.id] || 0;
            return count === 0 ? 'Not used' : `${count} screen${count === 1 ? '' : 's'}`;
          })()}
        {:else}
          <span class="text-gray-400">Loading...</span>
        {/if}
      </div>
    </DataTable>
  </div>

  {#if filteredCustomFields.length > 0}
    <Pagination
      currentPage={currentPage}
      totalItems={filteredCustomFields.length}
      itemsPerPage={itemsPerPage}
      showPageSizes={true}
      onpageChange={handlePageChange}
      onpageSizeChange={handlePageSizeChange}
    />
  {/if}