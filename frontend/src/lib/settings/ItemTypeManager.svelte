<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import { Plus, Edit, Trash2, FileText } from '@lucide/svelte';
  import { itemTypeIconMap, itemTypeIconOptions } from '../utils/icons.js';
  import Button from '../components/Button.svelte';
  import ItemTypeIcon from '../components/ItemTypeIcon.svelte';
  import DataTable from '../components/DataTable.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';
  import Select from '../components/Select.svelte';
  import Textarea from '../components/Textarea.svelte';
  import Input from '../components/Input.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import IconSelector from '../pickers/IconSelector.svelte';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';
  import './settings-form.css';
  import {
    GENERIC_SUBTASK_HIERARCHY_LEVEL,
    isGenericSubtaskType,
    sortItemTypesByHierarchy,
  } from '../utils/hierarchy.js';

  let itemTypes = $state([]);
  let hierarchyLevels = $state([]);
  let isLoading = $state(true);
  let error = $state(null);
  let editingId = $state(null);
  let originalHierarchyLevel = $state(null);
  let showCreateForm = $state(false);

  const systemItemTypes = [
    {
      key: 'initiative',
      name: 'Initiative',
      description: 'Strategic initiative spanning multiple teams',
      icon: 'Target',
      color: '#7c3aed',
      hierarchyLevel: 0,
      sortOrder: 1
    },
    {
      key: 'epic',
      name: 'Epic',
      description: 'Large feature or capability',
      icon: 'Zap',
      color: '#2563eb',
      hierarchyLevel: 1,
      sortOrder: 1
    },
    {
      key: 'story',
      name: 'Story',
      description: 'User story delivering value to end users',
      icon: 'BookOpen',
      color: '#059669',
      hierarchyLevel: 2,
      sortOrder: 1
    },
    {
      key: 'task',
      name: 'Task',
      description: 'Development or operational task',
      icon: 'CheckSquare',
      color: '#dc2626',
      hierarchyLevel: 3,
      sortOrder: 1
    },
    {
      key: 'bug',
      name: 'Bug',
      description: 'Software defect that needs fixing',
      icon: 'Bug',
      color: '#ea580c',
      hierarchyLevel: 3,
      sortOrder: 2
    },
    {
      key: 'subtask',
      name: 'Sub-task',
      description: 'Small work item below any regular hierarchy level',
      icon: 'Minus',
      color: '#6b7280',
      hierarchyLevel: GENERIC_SUBTASK_HIERARCHY_LEVEL,
      sortOrder: 1
    }
  ];

  const systemHierarchyLevels = {
    0: {
      key: 'initiative',
      name: 'Initiative',
      description: 'High-level strategic work spanning multiple epics'
    },
    1: {
      key: 'epic',
      name: 'Epic',
      description: 'Large work item that can be broken down into stories'
    },
    2: {
      key: 'story',
      name: 'Story',
      description: 'User story or feature that delivers value'
    },
    3: {
      key: 'task',
      name: 'Task',
      description: 'Individual work item or technical task'
    },
    4: {
      key: 'activity',
      name: 'Activity',
      description: 'Discrete activity within a task'
    }
  };

  function getSystemItemTypeDefinition(itemType) {
    if (!itemType.is_default) return null;

    return systemItemTypes.find(definition => (
      itemType.name === definition.name &&
      itemType.description === definition.description &&
      itemType.icon === definition.icon &&
      itemType.color?.toLowerCase() === definition.color &&
      Number(itemType.hierarchy_level) === definition.hierarchyLevel &&
      Number(itemType.sort_order) === definition.sortOrder
    )) ?? null;
  }

  function getItemTypeDisplayName(itemType) {
    const definition = getSystemItemTypeDefinition(itemType);
    return definition
      ? t(`settings.itemTypes.defaults.${definition.key}`)
      : itemType.name;
  }

  function getHierarchyLevelDisplayName(hierarchyLevel) {
    const definition = systemHierarchyLevels[hierarchyLevel.level];
    const isUnchangedSystemLevel = definition &&
      hierarchyLevel.name === definition.name &&
      hierarchyLevel.description === definition.description;

    return isUnchangedSystemLevel
      ? t(`settings.hierarchyLevels.defaults.${definition.key}.name`)
      : hierarchyLevel.name;
  }

  function getConfigurationSetDisplayName(name) {
    return name === 'Default Configuration'
      ? t('settings.itemTypes.defaultConfiguration')
      : name;
  }

  // Form data
  let formData = $state({
    name: '',
    description: '',
    icon: 'FileText',
    color: '#3b82f6',
    hierarchy_level: 0, // Default to level 0 (Initiative level)
    sort_order: 1,
    is_default: false
  });

  onMount(async () => {
    await Promise.all([
      loadItemTypes(),
      loadHierarchyLevels()
    ]);
  });

  async function loadItemTypes() {
    try {
      isLoading = true;
      error = null;
      itemTypes = await api.itemTypes.getAll();
      // Group by hierarchy level for better display
      itemTypes = sortItemTypesByHierarchy(itemTypes);
    } catch (err) {
      error = t('settings.itemTypes.failedToLoad');
    } finally {
      isLoading = false;
    }
  }

  async function loadHierarchyLevels() {
    try {
      hierarchyLevels = await api.hierarchyLevels.getAll();
      hierarchyLevels.sort((a, b) => a.level - b.level);
    } catch (err) {
    }
  }

  function startCreate() {
    const defaultHierarchyLevel = 3; // Default to level 3 (Task level)
    formData = {
      name: '',
      description: '',
      icon: 'FileText',
      color: '#3b82f6',
      hierarchy_level: defaultHierarchyLevel,
      sort_order: getNextSortOrder(defaultHierarchyLevel),
      is_default: false
    };
    editingId = null;
    originalHierarchyLevel = null;
    showCreateForm = true;
  }

  function startEdit(itemType) {
    formData = {
      name: itemType.name,
      description: itemType.description,
      icon: itemType.icon,
      color: itemType.color,
      hierarchy_level: itemType.hierarchy_level,
      sort_order: itemType.sort_order,
      is_default: itemType.is_default
    };
    editingId = itemType.id;
    originalHierarchyLevel = itemType.hierarchy_level;
    showCreateForm = true;
  }

  function cancelEdit() {
    showCreateForm = false;
    editingId = null;
    originalHierarchyLevel = null;
    formData = {
      name: '',
      description: '',
      icon: 'FileText',
      color: '#3b82f6',
      hierarchy_level: 0, // Default to level 0 (Initiative level)
      sort_order: 1,
      is_default: false
    };
  }

  function getNextSortOrder(hierarchyLevel) {
    const itemsAtLevel = itemTypes.filter(it => it.hierarchy_level === hierarchyLevel);
    return itemsAtLevel.length > 0 ? Math.max(...itemsAtLevel.map(it => it.sort_order)) + 1 : 1;
  }

  // Update sort order when hierarchy level changes
  function onHierarchyLevelChange() {
    formData.sort_order = getNextSortOrder(formData.hierarchy_level);
  }

  let hierarchyLevelChanged = $derived(
    editingId !== null &&
    originalHierarchyLevel !== null &&
    Number(formData.hierarchy_level) !== Number(originalHierarchyLevel)
  );

  async function saveItemType() {
    try {
      if (!formData.name.trim()) {
        errorToast(t('settings.itemTypes.nameRequired'));
        return;
      }

      if (editingId) {
        await api.itemTypes.update(editingId, formData);
      } else {
        await api.itemTypes.create(formData);
      }

      await loadItemTypes();
      cancelEdit();
      error = null;
      window.dispatchEvent(new CustomEvent('refresh-workspace-data'));
    } catch (err) {
      errorToast(t('settings.itemTypes.failedToSave'));
    }
  }

  async function deleteItemType(id, name) {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('settings.itemTypes.confirmDelete', { name }),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger'
    });
    if (!confirmed) return;

    try {
      await api.itemTypes.delete(id);
      await loadItemTypes();
      error = null;
      window.dispatchEvent(new CustomEvent('refresh-workspace-data'));
    } catch (err) {
      error = t('settings.itemTypes.deleteFailed');
    }
  }

  function getHierarchyLevelName(level) {
    if (Number(level) === GENERIC_SUBTASK_HIERARCHY_LEVEL) {
      return t('settings.itemTypes.genericSubtaskLevel');
    }
    const hierarchyLevel = hierarchyLevels.find(hl => hl.level === level);
    return hierarchyLevel
      ? t('settings.itemTypes.levelLabel', {
          level,
          name: getHierarchyLevelDisplayName(hierarchyLevel)
        })
      : t('settings.itemTypes.levelOnly', { level });
  }

  let hierarchyLevelOptions = $derived([
    ...hierarchyLevels.map(level => ({
      value: level.level,
      label: t('settings.itemTypes.levelOption', {
        name: getHierarchyLevelDisplayName(level),
        level: level.level
      })
    })),
    {
      value: GENERIC_SUBTASK_HIERARCHY_LEVEL,
      label: t('settings.itemTypes.genericSubtaskLevel')
    }
  ]);

  // Column definitions for DataTable
  const itemTypeColumns = $derived([
    {
      key: 'icon',
      label: '',
      width: '40px',
      slot: 'icon'
    },
    {
      key: 'name',
      label: t('settings.itemTypes.name'),
      render: itemType => getItemTypeDisplayName(itemType)
    },
    {
      key: 'hierarchy_level',
      label: t('settings.itemTypes.hierarchyLevel'),
      slot: 'hierarchy_level'
    },
    {
      key: 'sort_order',
      label: t('common.order')
    },
    {
      key: 'configuration_set_names',
      label: t('settings.configSets.title'),
      slot: 'configuration_set_names'
    },
    {
      key: 'actions',
      label: t('common.actions')
    }
  ]);

  function buildItemTypeDropdownItems(itemType) {
    return [
      {
        id: 'edit',
        type: 'regular',
        testid: 'item-type-edit',
        icon: Edit,
        title: t('common.edit'),
        hoverClass: 'hover-bg',
        onClick: () => startEdit(itemType)
      },
      {
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: t('common.delete'),
        color: 'var(--ds-text-danger)',
        hoverClass: 'hover-danger',
        onClick: () => deleteItemType(itemType.id, getItemTypeDisplayName(itemType))
      }
    ];
  }
</script>

<PageHeader
  icon={FileText}
  title={t('settings.itemTypes.title')}
  subtitle={t('settings.itemTypes.subtitle')}
>
  {#snippet actions()}
    <Button
      variant="primary"
      icon={Plus}
      onclick={startCreate}
      disabled={isLoading}
      dataTestid="item-type-add"
      keyboardHint="A"
      hotkeyConfig={{ key: toHotkeyString('itemTypes', 'add'), guard: () => !showCreateForm }}
    >
      {t('settings.itemTypes.addItemType')}
    </Button>
  {/snippet}
</PageHeader>

  {#if error}
    <div class="error">
      {error}
    </div>
  {/if}

  <DataTable
    columns={itemTypeColumns}
    data={itemTypes}
    keyField="id"
    loading={isLoading}
    emptyMessage={t('settings.itemTypes.noItemTypes')}
    emptyIcon={FileText}
    actionItems={buildItemTypeDropdownItems}
    actionTriggerTestid={(itemType) => `item-type-actions-${itemType.id}`}
    rowAttrs={(itemType) => ({
      'data-testid': `item-type-row-${itemType.id}`,
      'data-hierarchy-level': itemType.hierarchy_level
    })}
  >
    {#snippet icon(itemType)}
      <div class="flex items-center justify-center">
        <ItemTypeIcon itemType={itemType} title={getItemTypeDisplayName(itemType)} />
      </div>
    {/snippet}

    {#snippet hierarchy_level(itemType)}
      <Lozenge
        color={isGenericSubtaskType(itemType) ? 'gray' : 'blue'}
        text={getHierarchyLevelName(itemType.hierarchy_level)}
      />
    {/snippet}

    {#snippet configuration_set_names(itemType)}
      <div class="flex flex-wrap gap-1">
        {#if itemType.configuration_set_names && itemType.configuration_set_names.length > 0}
          {#each itemType.configuration_set_names as configSetName}
            <Lozenge color="gray" text={getConfigurationSetDisplayName(configSetName)} />
          {/each}
        {:else}
          <span class="text-xs text-gray-500">{t('settings.itemTypes.noConfigurationSets')}</span>
        {/if}
      </div>
    {/snippet}
  </DataTable>

  <Modal isOpen={showCreateForm} onclose={cancelEdit} maxWidth="max-w-2xl">
    <!-- Modal header -->
    <ModalHeader title={editingId ? t('itemTypes.editItemType') : t('itemTypes.createItemType')} showCloseButton={false} />

    <!-- Modal content -->
    <div class="px-6 py-4">
      <form onsubmit={(e) => { e.preventDefault(); saveItemType(); }}>
        <div class="form-group">
          <label for="name">{t('settings.itemTypes.name')}</label>
          <Input
            type="text"
            id="name"
            placeholder={t('settings.itemTypes.namePlaceholder')}
            bind:value={formData.name}
            required
          />
        </div>

        <div class="form-group">
          <label for="description">{t('settings.itemTypes.description')}</label>
          <Textarea
            id="description"
            placeholder={t('settings.itemTypes.descriptionPlaceholder')}
            bind:value={formData.description}
            rows={2}
          />
        </div>

        <div class="form-row">
          <div class="form-group">
            <label for="hierarchy_level">{t('settings.itemTypes.hierarchyLevel')}</label>
            <Select
              id="hierarchy_level"
              bind:value={formData.hierarchy_level}
              onchange={onHierarchyLevelChange}
              required
              options={hierarchyLevelOptions}
            />
          </div>

          <div class="form-group">
            <label for="sort_order">{t('common.order')}</label>
            <Input
              type="number"
              id="sort_order"
              min={1}
              bind:value={formData.sort_order}
              required
            />
          </div>
        </div>

        {#if hierarchyLevelChanged}
          <AlertBox variant="warning" class="mb-4">
            <div data-testid="admin-hierarchy-change-warning">
              <p class="font-medium">{t('settings.itemTypes.hierarchyChangeWarningTitle')}</p>
              <p class="mt-1" style="color: var(--ds-text-subtle);">
                {t('settings.itemTypes.hierarchyChangeWarningDescription', {
                  fromLevel: originalHierarchyLevel,
                  toLevel: formData.hierarchy_level
                })}
              </p>
            </div>
          </AlertBox>
        {/if}

        <div class="form-group">
          <IconSelector
            bind:selectedIcon={formData.icon}
            bind:selectedColor={formData.color}
            iconMap={itemTypeIconMap}
            iconOptions={itemTypeIconOptions}
            compact
          />
        </div>

      </form>
    </div>

    <DialogFooter
      onCancel={cancelEdit}
      onConfirm={saveItemType}
      confirmLabel={editingId ? t('common.update') : t('common.create')}
    />
  </Modal>
