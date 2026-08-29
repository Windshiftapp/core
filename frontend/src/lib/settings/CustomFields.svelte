<script>
  import { onMount, untrack } from 'svelte';
  import { api } from '../api.js';
  import { currentRoute, navigate } from '../router.js';
  import { Plus, Edit, Trash2, MoreHorizontal, Circle, Database, Settings, Type, AlignLeft, ChevronDownCircle, ListChecks, Hash, Calendar, User, Repeat, Flag, Box, Globe, Building2, Link2, CheckSquare } from '@lucide/svelte';
  import Button from '../components/Button.svelte';
  import Input from '../components/Input.svelte';
  import Select from '../components/Select.svelte';
  import Textarea from '../components/Textarea.svelte';
  import DataTable from '../components/DataTable.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
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
  import { errorToast, infoToast } from '../stores/toasts.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import { formatDateSimple } from '../utils/dateFormatter.js';
  import BasePicker from '../pickers/BasePicker.svelte';
  import { parseFieldOptions, serializeOptions } from '../utils/optionUtils.js';
  import { X as XIcon } from '@lucide/svelte';
  import DescriptionText from '../components/DescriptionText.svelte';
  import { customFieldFormData, loadCustomFieldsOverview } from './customFieldsData.js';
  import { BOOLEAN_CUSTOM_FIELD_TYPE, canonicalCustomFieldType, isBooleanCustomFieldType } from '../utils/customFieldTypes.js';
  import { workspaceDataStore } from '../stores/workspaceDataStore.svelte.js';

  const entityTypeOptions = [
    { id: 'item', name: 'Items' },
    { id: 'test_case', name: 'Test Cases' },
    { id: 'asset', name: 'Assets' }
  ];

  let customFields = $state([]);
  let indexCounts = $state({ items: { current: 0, max: 20 }, assets: { current: 0, max: 20 } });
  let screens = $state([]);
  let showCreateForm = $state(false);
  let editingField = $state(null);
  // Validated board-configuration route to return to after a create/cancel
  // flow, or to surface as a "Back to board fields" action in the manage flow.
  let boardReturnTarget = $state(null);
  // True only when this page was opened via action=create from board
  // configuration, in which case cancel/success should return automatically.
  let boardCreateFlow = $state(false);
  /** @type {{ field_name: string, field_type: string, field_config: Record<string, any>, applies_to_portal_customers: boolean, applies_to_customer_organisations: boolean, description?: string, required?: boolean }} */
  let formData = $state(customFieldFormData());

  // Settings modal state
  let showSettingsModal = $state(false);
  let settingsMaxIndexes = $state(20);

  // Indexing state for edit modal
  let indexedItems = $state(false);
  let indexedAssets = $state(false);

  let optionItems = $state([]); // Array of {id, label} for select/multiselect options
  let nextOptionId = $state(1); // Next ID to assign to a new option

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
    { value: 'multi_user', label: 'Multi User', icon: User, iconColor: '#4D9DC5' },
    { value: 'iteration', label: 'Iteration', icon: Repeat, iconColor: '#D95B5B' },
    { value: 'milestone', label: 'Milestone', icon: Flag, iconColor: '#C9A84C' },
    { value: 'asset', label: 'Asset', icon: Box, iconColor: '#7B8A9E' },
    { value: 'portalcustomer', label: 'Portal Customer', icon: Globe, iconColor: '#E07BAF' },
    { value: 'customerorganisation', label: 'Customer Organisation', icon: Building2, iconColor: '#8B7EC8' },
    { value: BOOLEAN_CUSTOM_FIELD_TYPE, label: t('fields.checkbox'), icon: CheckSquare, iconColor: '#2F9E8F' },
    { value: 'linking', label: 'Linking', icon: Link2, iconColor: '#3B82F6' }
  ];

  const selectedFieldType = $derived(fieldTypes.find(ft => ft.value === formData.field_type));

  // Asset field configuration
  let assetSetId = $state(null);
  let assetQlQuery = $state('');
  let assetMulti = $state(false);
  let assetSets = $state([]);

  // Linking field configuration
  let linkingLinkTypeId = $state(null);
  let linkingAllowedItemTypeIds = $state([]);
  let linkingAllowedEntityTypes = $state(['item']);
  let linkingMulti = $state(true);
  let linkingMirrorName = $state('');
  let linkingMirrorAllowedItemTypeIds = $state([]);
  let linkTypes = $state([]);
  let itemTypes = $state([]);

  // When a link type is selected, auto-populate entity types if the link type constrains them
  const selectedLinkType = $derived(linkTypes.find(l => l.id == linkingLinkTypeId));
  const linkTypeConstrainsEntities = $derived(selectedLinkType?.allowed_entity_types?.length > 0);

  $effect(() => {
    if (linkingLinkTypeId) {
      const lt = linkTypes.find(l => l.id == linkingLinkTypeId);
      if (lt?.allowed_entity_types?.length > 0) {
        linkingAllowedEntityTypes = [...lt.allowed_entity_types];
      }
    }
  });

  onMount(async () => {
    // Board-configuration entry: validate `returnTo` and consume `action=create`
    // exactly once (removing it from browser history so back/forward never
    // re-opens the create form).
    const query = $currentRoute.query || {};
    const returnPath = validateReturnTo(query.returnTo);
    if (query.action === 'create') {
      boardCreateFlow = true;
      try {
        const url = new URL(window.location.href);
        url.searchParams.delete('action');
        history.replaceState(history.state, '', url.toString());
      } catch { /* history rewrite is best-effort */ }
      boardReturnTarget = returnPath;
      if (returnPath) startCreate();
    } else if (returnPath) {
      boardCreateFlow = false;
      boardReturnTarget = returnPath;
    }

    const loadCatalog = async (loader, label) => {
      try {
        return (await loader()) || [];
      } catch (error) {
        console.error(`Failed to load ${label}:`, error);
        return [];
      }
    };
    const [, loadedAssetSets, loadedLinkTypes, loadedItemTypes] = await Promise.all([
      loadCustomFields(),
      loadCatalog(() => api.assetSets.getAll(), 'asset sets'),
      loadCatalog(() => api.linkTypes.getAll(), 'link types'),
      loadCatalog(() => api.itemTypes.getAll(), 'item types'),
    ]);
    assetSets = loadedAssetSets;
    linkTypes = loadedLinkTypes;
    itemTypes = loadedItemTypes;
  });

  async function loadCustomFields() {
    try {
      const overview = await loadCustomFieldsOverview(api);
      customFields = overview.customFields;
      indexCounts = overview.indexCounts;
      screens = overview.screens;
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
      errorToast(t('dialogs.alerts.failedToSave', { error: error.message || error }));
    }
  }

  function startCreate() {
    showCreateForm = true;
    editingField = null;
    resetForm();
  }

  function startEdit(field) {
    editingField = field;
    formData = customFieldFormData(field);

    // Parse options using the unified parser (handles both legacy and new format)
    if ((field.field_type === 'select' || field.field_type === 'multiselect') && field.options) {
      const parsed = parseFieldOptions(field.options);
      optionItems = parsed.items.map(item => ({ id: item.id, label: item.label }));
      nextOptionId = parsed.nextId;
    } else {
      optionItems = [];
      nextOptionId = 1;
    }

    // Parse asset field config
    if (field.field_type === 'asset' && field.options) {
      try {
        const config = JSON.parse(field.options);
        assetSetId = config.asset_set_id || null;
        assetQlQuery = config.ql_query || config.cql_query || '';
        assetMulti = config.multi === true;
      } catch (e) {
        assetSetId = null;
        assetQlQuery = '';
        assetMulti = false;
      }
    } else {
      assetSetId = null;
      assetQlQuery = '';
      assetMulti = false;
    }

    // Parse linking field config
    if (field.field_type === 'linking' && field.options) {
      try {
        const config = JSON.parse(field.options);
        linkingLinkTypeId = config.link_type_id || null;
        linkingAllowedItemTypeIds = config.allowed_item_type_ids || [];
        linkingAllowedEntityTypes = config.allowed_entity_types || ['item'];
        linkingMulti = config.multi !== false;
        linkingMirrorName = '';
        linkingMirrorAllowedItemTypeIds = [];
      } catch (e) {
        linkingLinkTypeId = null;
        linkingAllowedItemTypeIds = [];
        linkingAllowedEntityTypes = ['item'];
        linkingMulti = true;
      }
    } else {
      linkingLinkTypeId = null;
      linkingAllowedItemTypeIds = [];
      linkingAllowedEntityTypes = ['item'];
      linkingMulti = true;
      linkingMirrorName = '';
      linkingMirrorAllowedItemTypeIds = [];
    }

    // Load indexing state
    indexedItems = field.indexed?.items || false;
    indexedAssets = field.indexed?.assets || false;

    showCreateForm = true;
  }

  function resetForm() {
    formData = customFieldFormData();
    optionItems = [];
    nextOptionId = 1;
    assetSetId = null;
    assetQlQuery = '';
    assetMulti = false;
    indexedItems = false;
    indexedAssets = false;
    linkingLinkTypeId = null;
    linkingAllowedItemTypeIds = [];
    linkingAllowedEntityTypes = ['item'];
    linkingMulti = true;
    linkingMirrorName = '';
    linkingMirrorAllowedItemTypeIds = [];
  }

  // Board-configuration routes this page may return to. External, protocol-
  // relative, and unrelated paths are rejected rather than trusted.
  function validateReturnTo(raw) {
    if (typeof raw !== 'string' || !raw) return null;
    let p = raw;
    try { p = decodeURIComponent(raw); } catch { /* keep raw */ }
    if (!p.startsWith('/') || p.startsWith('//') || /[?#\s:]/.test(p)) return null;
    const ok = /^\/(?:collections\/\d+\/board\/configure|workspaces\/\d+(?:\/collections\/\d+)?\/board\/configure)$/.test(p);
    return ok ? p : null;
  }

  function returnToBoard() {
    if (!boardReturnTarget) return;
    navigate(boardReturnTarget);
    boardReturnTarget = null;
    boardCreateFlow = false;
  }

  function cancelForm() {
    const navigateAway = boardCreateFlow && boardReturnTarget;
    showCreateForm = false;
    editingField = null;
    resetForm();
    if (navigateAway) returnToBoard();
  }

  function processFieldConfig() {
    const config = { ...formData.field_config };
    
    if (formData.field_type === 'select' || formData.field_type === 'multiselect') {
      // Filter out empty labels
      const validItems = optionItems.filter(item => item.label.trim().length > 0);

      if (validItems.length === 0) {
        throw new Error('At least one option is required for select fields');
      }

      config.selectOptions = { next_id: nextOptionId, items: validItems };
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
      config.multi = assetMulti;
    } else if (formData.field_type === 'linking') {
      if (!linkingLinkTypeId) {
        throw new Error('Linking fields require a link type');
      }
      config.link_type_id = parseInt(linkingLinkTypeId);
      config.allowed_entity_types = linkingAllowedEntityTypes;
      config.multi = linkingMulti;
      if (linkingAllowedItemTypeIds.length > 0) {
        config.allowed_item_type_ids = linkingAllowedItemTypeIds.map(Number);
      }
      if (linkingMirrorName.trim()) {
        config.mirror_name = linkingMirrorName.trim();
        if (linkingMirrorAllowedItemTypeIds.length > 0) {
          config.mirror_allowed_item_type_ids = linkingMirrorAllowedItemTypeIds.map(Number);
        }
      }
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
      if (processedConfig.selectOptions) {
        data.options = JSON.stringify(processedConfig.selectOptions);
      } else if (formData.field_type === 'asset') {
        // Asset fields store config as JSON in options
        data.options = JSON.stringify({
          asset_set_id: processedConfig.asset_set_id,
          ql_query: processedConfig.ql_query,
          multi: processedConfig.multi
        });
      } else if (formData.field_type === 'linking') {
        const linkOpts = {
          link_type_id: processedConfig.link_type_id,
          allowed_entity_types: processedConfig.allowed_entity_types,
          multi: processedConfig.multi
        };
        if (processedConfig.allowed_item_type_ids) {
          linkOpts.allowed_item_type_ids = processedConfig.allowed_item_type_ids;
        }
        if (processedConfig.mirror_name) {
          linkOpts.mirror_name = processedConfig.mirror_name;
          if (processedConfig.mirror_allowed_item_type_ids) {
            linkOpts.mirror_allowed_item_type_ids = processedConfig.mirror_allowed_item_type_ids;
          }
        }
        data.options = JSON.stringify(linkOpts);
      }

      let saveResult = null;
      if (editingField) {
        // Include indexing state if field type supports it
        if (isIndexableType(formData.field_type)) {
          data.indexed = { items: indexedItems, assets: indexedAssets };
        }
        saveResult = await api.customFields.update(editingField.id, data);
      } else {
        saveResult = await api.customFields.create(data);
      }

      if (saveResult?.indexing_deferred) {
        const tables = [];
        if (saveResult.indexing_deferred.items) tables.push('items');
        if (saveResult.indexing_deferred.assets) tables.push('assets');
        infoToast(
          `Custom field index creation for ${tables.join(' and ')} has been scheduled and will run during the next server restart. Searches may not speed up until then.`
        );
      }

      await loadCustomFields();
      await workspaceDataStore.invalidate('customFieldDefinitions');
      cancelForm();
      window.dispatchEvent(new CustomEvent('refresh-workspace-data'));
    } catch (error) {
      console.error('Failed to save custom field:', error);
      errorToast(t('dialogs.alerts.failedToSave', { error: error.message || error }));
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
        errorToast(t('dialogs.alerts.failedToDelete', { error: error.message || error }));
      }
    }
  }

  function getFieldTypeLabel(type) {
    return fieldTypes.find(t => t.value === canonicalCustomFieldType(type))?.label || type;
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
  const isLinkingField = $derived(formData.field_type === 'linking');
  const isLinkingMirror = $derived(isLinkingField && editingField && (() => { try { const o = JSON.parse(editingField.options || '{}'); return !!o.mirror_of_field_id; } catch { return false; } })());
  const hideAppliesToSection = $derived(formData.field_type === 'portalcustomer' || formData.field_type === 'customerorganisation' || formData.field_type === 'linking');

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
        testid: 'custom-field-edit',
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
        testid: 'custom-field-delete',
        color: 'var(--ds-text-danger)',
        hoverClass: 'hover-danger',
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
            if (options && options.items && Array.isArray(options.items)) {
              return `${options.items.length} options`;
            } else if (Array.isArray(options)) {
              return `${options.length} options`;
            } else if (field.field_type === 'linking' && options.link_type_id) {
              const lt = linkTypes.find(l => l.id === options.link_type_id);
              const ltName = lt ? lt.name : `Type #${options.link_type_id}`;
              const parts = [ltName];
              if (options.multi === false) parts.push('single');
              if (options.mirror_field_id) parts.push('mirrored');
              if (options.mirror_of_field_id) parts.push('mirror');
              return parts.join(', ');
            } else if (field.field_type === 'asset' && options.asset_set_id) {
              const set = assetSets.find(s => s.id === options.asset_set_id);
              const setName = set ? set.name : `Set #${options.asset_set_id}`;
              const parts = [setName];
              if (options.multi === true) parts.push('multiple');
              if (options.ql_query || options.cql_query) parts.push('filtered');
              return parts.join(', ');
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
      {#if boardReturnTarget && !boardCreateFlow}
        <Button
          variant="default"
          dataTestid="custom-fields-return-to-board"
          onclick={returnToBoard}
        >
          {t('fields.returnToBoard')}
        </Button>
      {/if}
      <Button
        id="create-field-button"
        variant="primary"
        icon={Plus}
        dataTestid="custom-field-create"
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


<Modal isOpen={showCreateForm} onclose={cancelForm} onSubmit={saveField} maxWidth="max-w-2xl" dataTestid="custom-field-dialog">
  <ModalHeader title={editingField ? t('fields.editField') : t('fields.createField')} showCloseButton={false} />

  <!-- Modal content -->
  <div class="px-6 py-4">
    <form onsubmit={(e) => { e.preventDefault(); saveField(); }}>
      <div class="grid grid-cols-1 md:grid-cols-2 gap-6">

        <div>
          <Label for="field-name" required class="mb-2">{t('fields.fieldName')}</Label>
          <Input
            id="field-name"
            dataTestid="custom-field-name"
            bind:value={formData.field_name}
            placeholder="e.g., Sprint, Epic, Customer Impact"
            required
          />
        </div>

        <div>
          <Label for="field-type" required class="mb-2">{t('fields.fieldType')}</Label>
          <DropdownMenu
            triggerTestid="custom-field-type-trigger"
            triggerIcon={selectedFieldType?.icon}
            triggerIconBgColor={selectedFieldType?.iconColor}
            triggerText={selectedFieldType?.label || 'Select type...'}
            triggerClass="w-full h-[38px] rounded-lg border px-3 text-sm"
            triggerStyle="border-color: var(--ds-border); background: var(--ds-surface); color: var(--ds-text);"
            triggerAlignment="between"
            showChevron={true}
            disabled={!!editingField}
            maxWidth="max-w-72"
            items={fieldTypes.map(type => ({
              id: type.value,
              type: 'regular',
              icon: type.icon,
              iconColor: type.iconColor,
              title: type.label,
              testid: `custom-field-type-${type.value}`,
              onClick: () => {
                if (isBooleanCustomFieldType(type.value)) {
                  optionItems = [];
                  nextOptionId = 1;
                }
                formData.field_type = type.value;
              }
            }))}
          />
          {#if editingField}
            <p class="mt-2 text-xs" style="color: var(--ds-text-subtle);">
              Field type cannot be changed after creation.
            </p>
          {/if}
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
        <div class="mt-6" data-testid="custom-field-options">
          <Label required class="mb-2">Options</Label>
          <div class="flex flex-col gap-2">
            {#each optionItems as item, index}
              <div class="flex items-center gap-2">
                <span class="text-xs tabular-nums w-6 text-right flex-shrink-0" style="color: var(--ds-text-subtle);">
                  {item.id}
                </span>
                <Input
                  bind:value={item.label}
                  placeholder="Option label"
                  class="flex-1"
                />
                <button
                  type="button"
                  onclick={() => { optionItems = optionItems.filter((_, i) => i !== index); }}
                  class="p-1.5 rounded transition-colors hover-danger"
                  style="color: var(--ds-text-subtle);"
                  title="Remove option"
                >
                  <XIcon class="w-4 h-4" />
                </button>
              </div>
            {/each}
          </div>
          <Button
            variant="ghost"
            size="sm"
            icon={Plus}
            class="mt-2"
            onclick={() => {
              optionItems = [...optionItems, { id: nextOptionId, label: '' }];
              nextOptionId = nextOptionId + 1;
            }}
          >
            Add Option
          </Button>
        </div>
      {/if}

      {#if isAssetField}
        <div class="mt-6">
          <Label for="asset-set" required class="mb-2">Asset Set</Label>
          <Select id="asset-set" bind:value={assetSetId} required options={[{ value: null, label: 'Select asset set...' }, ...assetSets.map(set => ({ value: set.id, label: set.name }))]} />
        </div>

        <div class="mt-4 flex items-center gap-4">
          <Toggle
            bind:checked={assetMulti}
            label="Allow multiple values"
            size="small"
          />
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

      {#if isLinkingField}
        <div class="mt-6 space-y-4">
          {#if isLinkingMirror}
            <p class="text-sm p-3 rounded" style="color: var(--ds-text); background: var(--ds-surface); border: 1px solid var(--ds-border);">
              This is a mirror field. Configuration is managed through its primary field.
            </p>
          {:else}
            <div>
              <Label for="linking-link-type" required class="mb-2">Link Type</Label>
              <Select id="linking-link-type" bind:value={linkingLinkTypeId} required options={[{ value: null, label: 'Select link type...' }, ...linkTypes.filter(lt => lt.active !== false).map(lt => ({ value: lt.id, label: `${lt.name} (${lt.forward_label} / ${lt.reverse_label})` }))]} />
            </div>

            <div>
              <Label class="mb-2">Allowed Entity Types</Label>
              <BasePicker
                bind:value={linkingAllowedEntityTypes}
                items={entityTypeOptions}
                multiple={true}
                placeholder="Select entity types..."
                getValue={(item) => item.id}
                getLabel={(item) => item.name}
                disabled={linkTypeConstrainsEntities}
              />
              <DescriptionText>
                {linkTypeConstrainsEntities ? 'Constrained by link type' : 'Choose which entity types can be linked'}
              </DescriptionText>
            </div>

            {#if linkingAllowedEntityTypes.includes('item')}
              <div>
                <Label class="mb-2">Allowed Item Types (optional)</Label>
                <BasePicker
                  bind:value={linkingAllowedItemTypeIds}
                  items={itemTypes}
                  multiple={true}
                  placeholder="All item types"
                  getValue={(item) => item.id}
                  getLabel={(item) => item.name}
                />
                <DescriptionText>Leave empty to allow all item types</DescriptionText>
              </div>
            {/if}

            <div class="flex items-center gap-4">
              <Toggle
                bind:checked={linkingMulti}
                label="Allow multiple values"
                size="small"
              />
            </div>

            <div class="p-4 rounded-lg" style="background: var(--ds-surface); border: 1px solid var(--ds-border);">
              <Label for="linking-mirror-name" class="mb-2">Mirror Field Name (optional)</Label>
              <Input
                id="linking-mirror-name"
                bind:value={linkingMirrorName}
                placeholder='e.g., "Blocks" (reverse of "Blocked By")'
              />
              <DescriptionText>
                Creates a reverse field that shows the other side of the relationship. Leave empty for no mirror.
              </DescriptionText>

              {#if linkingMirrorName.trim() && linkingAllowedEntityTypes.includes('item')}
                <div class="mt-3">
                  <Label class="mb-2">Mirror Allowed Item Types (optional)</Label>
                  <BasePicker
                    bind:value={linkingMirrorAllowedItemTypeIds}
                    items={itemTypes}
                    multiple={true}
                    placeholder="All item types"
                    getValue={(item) => item.id}
                    getLabel={(item) => item.name}
                  />
                </div>
              {/if}
            </div>
          {/if}
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
    confirmTestid="custom-field-save"
    showKeyboardHint={true}
    confirmKeyboardHint="⏎"
    disabled={!formData.field_name.trim() || (needsOptions && optionItems.filter(i => i.label.trim()).length === 0) || (isAssetField && !assetSetId) || (isLinkingField && !isLinkingMirror && !linkingLinkTypeId)}
  />
</Modal>

<Modal isOpen={showSettingsModal} onclose={() => showSettingsModal = false} maxWidth="max-w-md">
  <ModalHeader title={t('fields.indexSettings')} showCloseButton={false} />
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

  <div class="mb-6" data-testid="custom-fields-table">
    <DataTable
      columns={fieldColumns}
      data={paginatedCustomFields}
      keyField="id"
      emptyMessage={t('fields.noFields')}
      emptyIcon={Circle}
      actionItems={buildFieldDropdownItems}
      actionTriggerTestid={() => 'custom-field-actions'}
      rowAttrs={() => ({ 'data-testid': 'custom-field-row' })}
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
