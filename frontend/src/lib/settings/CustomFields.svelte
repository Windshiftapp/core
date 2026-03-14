<script>
  import { onMount, untrack } from 'svelte';
  import { api } from '../api.js';
  import { currentRoute, navigate } from '../router.js';
  import { Plus, Edit, Trash2, MoreHorizontal, Circle, Database, Settings, Type, AlignLeft, ChevronDownCircle, ListChecks, Hash, Calendar, User, Repeat, Flag, Box, Globe, Building2 } from 'lucide-svelte';
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
  import Tooltip from '../components/Tooltip.svelte';
  import Toggle from '../components/Toggle.svelte';
  import Label from '../components/Label.svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';
  import DropdownMenu from '../layout/DropdownMenu.svelte';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';
  import { t } from '../stores/i18n.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import { formatDateSimple } from '../utils/dateFormatter.js';

  let customFields = $state([]);
  let indexCounts = $state({ items: { current: 0, max: 20 }, assets: { current: 0, max: 20 } });
  let screens = $state([]);
  let showCreateForm = $state(false);
  let editingField = $state(null);
  let formData = $state({
    field_name: '',
    field_type: 'text',
    field_config: { max_length: '' },
    applies_to_portal_customers: false,
    applies_to_customer_organisations: false
  });

  // Settings modal state
  let showSettingsModal = $state(false);
  let settingsMaxIndexes = $state(20);

  // Indexing state for edit modal
  let indexedItems = $state(false);
  let indexedAssets = $state(false);

  let optionsText = $state(''); // For managing select/multiselect options

  // Search state
  let searchQuery = $state('');

  // Pagination state derived from URL
  let currentPage = $derived(parseInt($currentRoute.query?.page) || 1);
  let itemsPerPage = $derived(parseInt($currentRoute.query?.pageSize) || 25);

  const fieldTypes = [
    { value: 'text', label: 'Single Line Text', icon: Type, iconColor: '#4A90D9' },
    { value: 'textarea', label: 'Multi Line Text', icon: AlignLeft, iconColor: '#5B6ABF' },
    { value: 'select', label: 'Single Select', icon: ChevronDownCircle, iconColor: '#E8853D' },
    { value: 'multiselect', label: 'Multi Select', icon: ListChecks, iconColor: '#D46B2F' },
    { value: 'number', label: 'Number', icon: Hash, iconColor: '#4CAF7D' },
    { value: 'date', label: 'Date', icon: Calendar, iconColor: '#9B6DB7' },
    { value: 'user', label: 'User', icon: User, iconColor: '#5BA4C9' },
    { value: 'iteration', label: 'Iteration', icon: Repeat, iconColor: '#D95B5B' },
    { value: 'milestone', label: 'Milestone', icon: Flag, iconColor: '#C9A84C' },
    { value: 'asset', label: 'Asset', icon: Box, iconColor: '#7B8A9E' },
    { value: 'portalcustomer', label: 'Portal Customer', icon: Globe, iconColor: '#E07BAF' },
    { value: 'customerorganisation', label: 'Customer Organisation', icon: Building2, iconColor: '#8B7EC8' }
  ];

  const selectedFieldType = $derived(fieldTypes.find(ft => ft.value === formData.field_type));

  // Asset field configuration
  let assetSetId = $state(null);
  let assetQlQuery = $state('');
  let assetSets = $state([]);

  onMount(async () => {
    await loadCustomFields();

    // Load asset sets for asset field type
    try {
      assetSets = await api.assetSets.getAll() || [];
    } catch (error) {
      console.error('Failed to load asset sets:', error);
      assetSets = [];
    }
  });

  async function loadCustomFields() {
    try {
      const [fieldsResult, screensResult] = await Promise.all([
        api.customFields.getAll(),
        api.screens.getAll()
      ]);
      // Handle new response format with data array and index_counts
      if (fieldsResult && fieldsResult.data) {
        customFields = fieldsResult.data || [];
        indexCounts = fieldsResult.index_counts || { items: { current: 0, max: 20 }, assets: { current: 0, max: 20 } };
      } else {
        customFields = fieldsResult || [];
      }
      
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


  function openSettings() {
    settingsMaxIndexes = indexCounts.items?.max || 20;
    showSettingsModal = true;
  }

  async function saveSettings() {
    try {
      await api.customFields.updateSettings({ max_indexes_per_table: settingsMaxIndexes });
      showSettingsModal = false;
      await loadCustomFields();
    } catch (error) {
      console.error('Failed to save settings:', error);
      alert(t('dialogs.alerts.failedToSave', { error: error.message || error }));
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

    // Load indexing state
    indexedItems = field.indexed?.items || false;
    indexedAssets = field.indexed?.assets || false;

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
    indexedItems = false;
    indexedAssets = false;
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
        // Include indexing state if field type supports it
        if (isIndexableType(formData.field_type)) {
          data.indexed = { items: indexedItems, assets: indexedAssets };
        }
        await api.customFields.update(editingField.id, data);
      } else {
        await api.customFields.create(data);
      }

      await loadCustomFields();
      cancelForm();
      window.dispatchEvent(new CustomEvent('refresh-workspace-data'));
    } catch (error) {
      console.error('Failed to save custom field:', error);
      alert(t('dialogs.alerts.failedToSave', { error: error.message || error }));
    }
  }

  async function deleteField(field) {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('dialogs.confirmations.deleteCustomField', { name: field.name }),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger'
    });
    if (confirmed) {
      try {
        await api.customFields.delete(field.id);
        await loadCustomFields();
        window.dispatchEvent(new CustomEvent('refresh-workspace-data'));
      } catch (error) {
        console.error('Failed to delete custom field:', error);
        alert(t('dialogs.alerts.failedToDelete', { error: error.message || error }));
      }
    }
  }

  function getFieldTypeLabel(type) {
    return fieldTypes.find(t => t.value === type)?.label || type;
  }

  function getScreenCount(fieldId) {
    if (!screens || screens.length === 0) {
      return 0;
    }
    return getFieldScreens(fieldId).length;
  }

  function getFieldScreens(fieldId) {
    if (!screens || screens.length === 0) {
      return [];
    }
    return screens.filter(screen => {
      if (!screen.fields || screen.fields.length === 0) return false;
      const fieldIdStr = fieldId.toString();
      return screen.fields.some(f => f.field_type === 'custom' && f.field_identifier.toString() === fieldIdStr);
    });
  }

  const indexableTypes = ['number', 'date', 'text'];
  function isIndexableType(type) {
    return indexableTypes.includes(type);
  }
  const showIndexingSection = $derived(editingField && isIndexableType(formData.field_type));

  const needsOptions = $derived(formData.field_type === 'select' || formData.field_type === 'multiselect');
  const needsMaxLength = $derived(formData.field_type === 'text' || formData.field_type === 'textarea');
  const isMilestoneField = $derived(formData.field_type === 'milestone');
  const isDateField = $derived(formData.field_type === 'date');
  const isAssetField = $derived(formData.field_type === 'asset');
  const isPortalCustomerField = $derived(formData.field_type === 'portalcustomer');
  const isCustomerOrganisationField = $derived(formData.field_type === 'customerorganisation');
  const hideAppliesToSection = $derived(formData.field_type === 'portalcustomer' || formData.field_type === 'customerorganisation');

  // Reactive statement to trigger re-calculation when screens data changes
  const screensLoaded = $derived(screens && screens.length > 0);

  // Reactive computed screen counts for all fields - triggers when screens or customFields change
  const fieldScreenCounts = $derived(customFields.reduce((acc, field) => {
    if (screensLoaded) {
      acc[field.id] = getScreenCount(field.id);
    } else {
      acc[field.id] = 0;
    }
    return acc;
  }, {}));

  // Search filtering - filters custom fields by name, type, or description
  const filteredCustomFields = $derived(customFields.filter(field => {
    if (!searchQuery.trim()) return true;

    const query = searchQuery.toLowerCase();
    return (
      field.name?.toLowerCase().includes(query) ||
      field.field_type?.toLowerCase().includes(query) ||
      field.description?.toLowerCase().includes(query) ||
      getFieldTypeLabel(field.field_type)?.toLowerCase().includes(query)
    );
  }));

  // Reset to page 1 when search query changes
  let searchInitialized = false;
  $effect(() => {
    const _ = searchQuery;
    if (!searchInitialized) {
      searchInitialized = true;
      return;
    }
    untrack(() => {
      if (currentPage !== 1) {
        updatePagination(1, itemsPerPage);
      }
    });
  });

  // Pagination logic - slice filtered results based on current page
  const totalPages = $derived(Math.ceil(filteredCustomFields.length / itemsPerPage));
  const paginatedCustomFields = $derived(filteredCustomFields.slice(
    (currentPage - 1) * itemsPerPage,
    currentPage * itemsPerPage
  ));

  // Update pagination via URL
  function updatePagination(page, pageSize) {
    const params = new URLSearchParams(window.location.search);
    if (page > 1) {
      params.set('page', page);
    } else {
      params.delete('page');
    }
    if (pageSize && pageSize !== 25) {
      params.set('pageSize', pageSize);
    } else {
      params.delete('pageSize');
    }
    const qs = params.toString();
    navigate(`/admin/custom-fields${qs ? '?' + qs : ''}`);
  }

  // Handle page change from Pagination component
  function handlePageChange(event) {
    updatePagination(event.detail.page, itemsPerPage);
  }

  // Handle page size change from Pagination component
  function handlePageSizeChange(event) {
    updatePagination(event.detail.page, event.detail.itemsPerPage);
  }

  function buildFieldDropdownItems(field) {
    const items = [
      {
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: t('common.edit'),
        hoverClass: 'hover-bg',
        onClick: () => startEdit(field)
      }
    ];

    // Only add delete option for non-system default fields
    if (!field.system_default) {
      items.push({
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: t('common.delete'),
        color: '#dc2626',
        hoverClass: 'hover:bg-red-50',
        onClick: () => deleteField(field)
      });
    }

    return items;
  }

  // Table column definitions
  const fieldColumns = $derived([
    {
      key: 'id',
      label: 'ID',
      render: (field) => field.id,
      textColor: '#3b82f6'
    },
    {
      key: 'name',
      label: t('fields.fieldName'),
      slot: 'name'
    },
    {
      key: 'field_type',
      label: t('common.type'),
      slot: 'type'
    },
    {
      key: 'options',
      label: t('common.options'),
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
      label: t('fields.usedIn'),
      slot: 'usage'
    },
    {
      key: 'created_at',
      label: t('common.created'),
      render: (field) => formatDateSimple(field.created_at),
      textColor: 'var(--ds-text-subtle)'
    },
    {
      key: 'actions',
      label: t('common.actions')
    }
  ]);
</script>

<PageHeader
  icon={Database}
  title={t('fields.title')}
  subtitle={t('fields.subtitle')}
>
  {#snippet actions()}
    <div class="flex items-center gap-3">
      <SearchInput
        bind:value={searchQuery}
        placeholder={t('fields.searchFields')}
        class="w-64"
      />
      <Button
        variant="primary"
        icon={Plus}
        onclick={startCreate}
        keyboardHint="A"
        hotkeyConfig={{ key: toHotkeyString('customFields', 'add'), guard: () => !showCreateForm }}
      >
        {t('fields.createField')}
      </Button>
      <DropdownMenu
        triggerIcon={MoreHorizontal}
        items={[
          {
            id: 'index-settings',
            type: 'regular',
            icon: Settings,
            title: t('fields.indexSettings'),
            onClick: openSettings
          }
        ]}
        maxWidth="max-w-48"
        showChevron={false}
        iconOnly={true}
      />
    </div>
  {/snippet}
</PageHeader>


<Modal isOpen={showCreateForm} onclose={cancelForm} maxWidth="max-w-2xl">
  <!-- Modal header -->
  <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
    <h3 class="text-lg font-semibold" style="color: var(--ds-text);">
      {editingField ? t('fields.editField') : t('fields.createField')}
    </h3>
  </div>

  <!-- Modal content -->
  <div class="px-6 py-4">
    <form onsubmit={(e) => { e.preventDefault(); saveField(); }}>
      <div class="grid grid-cols-1 md:grid-cols-2 gap-6">

        <div>
          <Label for="field-name" required class="mb-2">{t('fields.fieldName')}</Label>
          <Input
            id="field-name"
            bind:value={formData.field_name}
            placeholder="e.g., Sprint, Epic, Customer Impact"
            required
          />
        </div>

        <div>
          <Label for="field-type" required class="mb-2">{t('fields.fieldType')}</Label>
          <DropdownMenu
            triggerIcon={selectedFieldType?.icon}
            triggerIconBgColor={selectedFieldType?.iconColor}
            triggerText={selectedFieldType?.label || 'Select type...'}
            triggerClass="w-full h-[38px] rounded-lg border px-3 text-sm"
            triggerStyle="border-color: var(--ds-border); background: var(--ds-surface); color: var(--ds-text);"
            triggerAlignment="between"
            showChevron={true}
            maxWidth="max-w-72"
            items={fieldTypes.map(type => ({
              id: type.value,
              type: 'regular',
              icon: type.icon,
              iconColor: type.iconColor,
              title: type.label,
              onClick: () => { formData.field_type = type.value; }
            }))}
          />
          {#if isMilestoneField}
            <p class="text-sm mt-2 p-2 rounded" style="color: var(--ds-text); background: var(--ds-surface); border: 1px solid var(--ds-border);">
              {t('fields.milestoneHint')}
            </p>
          {/if}
          {#if isDateField}
            <p class="text-sm mt-2 p-2 rounded" style="color: var(--ds-text); background: var(--ds-surface); border: 1px solid var(--ds-border);">
              {t('fields.dateHint')}
            </p>
          {/if}
          {#if isAssetField}
            <p class="text-sm mt-2 p-2 rounded" style="color: var(--ds-text); background: var(--ds-surface); border: 1px solid var(--ds-border);">
              {t('fields.assetHint')}
            </p>
          {/if}
          {#if isPortalCustomerField}
            <p class="text-sm mt-2 p-2 rounded" style="color: var(--ds-text); background: var(--ds-surface); border: 1px solid var(--ds-border);">
              {t('fields.portalCustomerHint')}
            </p>
          {/if}
          {#if isCustomerOrganisationField}
            <p class="text-sm mt-2 p-2 rounded" style="color: var(--ds-text); background: var(--ds-surface); border: 1px solid var(--ds-border);">
              {t('fields.customerOrganisationHint')}
            </p>
          {/if}
        </div>
      </div>

      {#if !hideAppliesToSection}
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
      {/if}

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

      {#if showIndexingSection}
        <div class="mt-6 p-4 rounded-lg" style="background: var(--ds-surface); border: 1px solid var(--ds-border);">
          <Label class="mb-3">Database Indexing</Label>
          <div class="flex flex-col gap-3">
            <div class="flex items-center justify-between">
              <Toggle
                bind:checked={indexedItems}
                label="Index on Items"
                size="small"
              />
              <span class="text-xs" style="color: var(--ds-text-subtle);">
                {indexCounts.items?.current || 0} of {indexCounts.items?.max || 20} used
              </span>
            </div>
            <div class="flex items-center justify-between">
              <Toggle
                bind:checked={indexedAssets}
                label="Index on Assets"
                size="small"
              />
              <span class="text-xs" style="color: var(--ds-text-subtle);">
                {indexCounts.assets?.current || 0} of {indexCounts.assets?.max || 20} used
              </span>
            </div>
          </div>
          <p class="text-xs mt-3" style="color: var(--ds-text-subtle);">
            Indexing improves sort and filter performance but adds overhead to every write operation on this table.
          </p>
        </div>
      {/if}
    </form>
  </div>

  <DialogFooter
    onCancel={cancelForm}
    onConfirm={saveField}
    confirmLabel={editingField ? t('common.update') : t('common.create')}
    disabled={!formData.field_name.trim() || (needsOptions && !optionsText.trim()) || (isAssetField && !assetSetId)}
  />
</Modal>

<Modal isOpen={showSettingsModal} onclose={() => showSettingsModal = false} maxWidth="max-w-md">
  <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
    <h3 class="text-lg font-semibold" style="color: var(--ds-text);">{t('fields.indexSettings')}</h3>
  </div>
  <div class="px-6 py-4">
    <Label for="max-indexes" class="mb-2">Maximum indexes per table</Label>
    <Input
      id="max-indexes"
      type="number"
      bind:value={settingsMaxIndexes}
      min={1}
      max={100}
    />
    <p class="text-xs mt-2" style="color: var(--ds-text-subtle);">
      Controls how many custom field indexes can be created per table (items, assets). Higher values allow more indexed fields but may impact write performance. Currently using {indexCounts.items?.current || 0} on items and {indexCounts.assets?.current || 0} on assets.
    </p>
  </div>
  <DialogFooter
    onCancel={() => showSettingsModal = false}
    onConfirm={saveSettings}
    confirmLabel={t('common.save')}
    disabled={!settingsMaxIndexes || settingsMaxIndexes < 1 || settingsMaxIndexes > 100}
  />
</Modal>

  <div class="mb-6">
    <DataTable
      columns={fieldColumns}
      data={paginatedCustomFields}
      keyField="id"
      emptyMessage={t('fields.noFields')}
      emptyIcon={Circle}
      actionItems={buildFieldDropdownItems}
    >
      {#snippet name(field)}
        <div>
          <span>{field.name}</span>
        </div>
      {/snippet}

      {#snippet type(field)}
        <Lozenge color="blue" text={getFieldTypeLabel(field.field_type)} />
      {/snippet}

      {#snippet usage(field)}
        <div class="text-sm">
        {#if screensLoaded}
          {@const matchingScreens = getFieldScreens(field.id)}
          {@const assetTypes = field.asset_type_usages || []}
          {@const hasPortal = field.applies_to_portal_customers}
          {@const hasOrgs = field.applies_to_customer_organisations}
          {@const hasUsage = matchingScreens.length > 0 || assetTypes.length > 0 || hasPortal || hasOrgs}
          {#if hasUsage}
            <div class="flex flex-wrap gap-1">
              {#if matchingScreens.length > 0}
                <Tooltip content={matchingScreens.map(s => s.name).join(', ')}>
                  <Lozenge color="blue" text={t('screens.screens', { count: matchingScreens.length })} size="sm" />
                </Tooltip>
              {/if}
              {#each assetTypes as at}
                <Tooltip content={at.set_name}>
                  <Lozenge color="teal" text={at.asset_type_name} size="sm" />
                </Tooltip>
              {/each}
              {#if hasPortal}
                <Lozenge color="purple" text={t('fields.portalCustomers')} size="sm" />
              {/if}
              {#if hasOrgs}
                <Lozenge color="green" text={t('fields.customerOrganisations')} size="sm" />
              {/if}
            </div>
          {:else}
            {t('common.noData')}
          {/if}
        {:else}
          <span class="text-gray-400">{t('common.loading')}</span>
        {/if}
        </div>
      {/snippet}
    </DataTable>
  </div>

  {#if filteredCustomFields.length > 0}
    <div class="pb-6">
      <Pagination
        currentPage={currentPage}
        totalItems={filteredCustomFields.length}
        itemsPerPage={itemsPerPage}
        showPageSizes={true}
        onpageChange={handlePageChange}
        onpageSizeChange={handlePageSizeChange}
      />
    </div>
  {/if}