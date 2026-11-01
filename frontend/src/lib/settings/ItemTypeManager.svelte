<script>
  import { hierarchyLevels as hierarchyLevelsApi, itemTypes as itemTypesApi } from '../api/configuration.js';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import Edit from '@lucide/svelte/icons/edit';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import FileText from '@lucide/svelte/icons/file-text';
  import { itemTypeIconMap, itemTypeIconOptions } from '../utils/icons.js';
  import ItemTypeIcon from '../components/ItemTypeIcon.svelte';
  import Select from '../components/Select.svelte';
  import Textarea from '../components/Textarea.svelte';
  import Input from '../components/Input.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import LocalizedObjectFields from './LocalizedObjectFields.svelte';
  import IconSelector from '../pickers/IconSelector.svelte';
  import {
    GENERIC_SUBTASK_HIERARCHY_LEVEL,
    isGenericSubtaskType,
    sortItemTypesByHierarchy,
  } from '../utils/hierarchy.js';
  import { objectDisplayName } from '../utils/systemLabels.js';
  import EntityManagerPage from './EntityManagerPage.svelte';

  let itemTypes = $state([]);
  let hierarchyLevels = $state([]);
  let editingId = $state(null);
  let originalHierarchyLevel = $state(null);
  let editingObject = $state(null);
  let translationEditor = $state(null);

  let formData = $state({
    name: '',
    description: '',
    icon: 'FileText',
    color: '#3b82f6',
    hierarchy_level: 0,
    sort_order: 1,
    is_default: false,
  });

  function getItemTypeDisplayName(itemType) {
    return objectDisplayName(itemType);
  }

  function getHierarchyLevelDisplayName(hierarchyLevel) {
    return objectDisplayName(hierarchyLevel);
  }

  function getConfigurationSetDisplayName(name, builtinKey = '') {
    return builtinKey === 'default' ? t('settings.itemTypes.defaultConfiguration') : name;
  }

  function getNextSortOrder(hierarchyLevel, itemTypes) {
    const itemsAtLevel = itemTypes.filter((it) => it.hierarchy_level === hierarchyLevel);
    return itemsAtLevel.length > 0 ? Math.max(...itemsAtLevel.map((it) => it.sort_order)) + 1 : 1;
  }

  async function loadItemTypes() {
    const [loaded] = await Promise.all([itemTypesApi.getAll(), loadHierarchyLevels()]);
    itemTypes = sortItemTypesByHierarchy(loaded);
    return itemTypes;
  }

  async function loadHierarchyLevels() {
    try {
      hierarchyLevels = (await hierarchyLevelsApi.getAll()).sort((a, b) => a.level - b.level);
    } catch {
      hierarchyLevels = [];
    }
  }

  function openCreate(itemTypes) {
    const defaultHierarchyLevel = 3;
    formData = {
      name: '',
      description: '',
      icon: 'FileText',
      color: '#3b82f6',
      hierarchy_level: defaultHierarchyLevel,
      sort_order: getNextSortOrder(defaultHierarchyLevel, itemTypes),
      is_default: false,
    };
    editingId = null;
    editingObject = null;
    originalHierarchyLevel = null;
  }

  function openEdit(itemType) {
    editingObject = itemType;
    formData = {
      name: itemType.name,
      description: itemType.description,
      icon: itemType.icon,
      color: itemType.color,
      hierarchy_level: itemType.hierarchy_level,
      sort_order: itemType.sort_order,
      is_default: itemType.is_default,
    };
    editingId = itemType.id;
    originalHierarchyLevel = itemType.hierarchy_level;
  }

  // Already surfaced via a toast; the scaffold should not also show its banner.
  class ValidationError extends Error {}

  async function saveItemType() {
    if (!formData.name.trim()) {
      errorToast(t('settings.itemTypes.nameRequired'));
      throw new ValidationError();
    }
    if (editingId) {
      translationEditor?.validate();
      await itemTypesApi.update(editingId, formData);
      await translationEditor?.save();
    } else {
      await itemTypesApi.create(formData);
    }
    window.dispatchEvent(new CustomEvent('refresh-workspace-data'));
  }

  function handleError(err, phase) {
    if (err instanceof ValidationError) return true;
    if (phase === 'save') {
      errorToast(`${t('settings.itemTypes.failedToSave')} ${err?.message || err}`);
      return true;
    }
    // Load and delete errors fall back to the scaffold's banner.
    return false;
  }

  async function deleteItemType(itemType) {
    await itemTypesApi.delete(itemType.id);
    window.dispatchEvent(new CustomEvent('refresh-workspace-data'));
  }

  function onHierarchyLevelChange() {
    formData.sort_order = getNextSortOrder(formData.hierarchy_level, itemTypes);
  }

  const hierarchyLevelChanged = $derived(
    editingId !== null &&
      originalHierarchyLevel !== null &&
      Number(formData.hierarchy_level) !== Number(originalHierarchyLevel)
  );

  function getHierarchyLevelName(level) {
    if (Number(level) === GENERIC_SUBTASK_HIERARCHY_LEVEL) {
      return t('settings.itemTypes.genericSubtaskLevel');
    }
    const hierarchyLevel = hierarchyLevels.find((hl) => hl.level === level);
    return hierarchyLevel
      ? t('settings.itemTypes.levelLabel', {
          level,
          name: getHierarchyLevelDisplayName(hierarchyLevel),
        })
      : t('settings.itemTypes.levelOnly', { level });
  }

  const hierarchyLevelOptions = $derived([
    ...hierarchyLevels.map((level) => ({
      value: level.level,
      label: t('settings.itemTypes.levelOption', {
        name: getHierarchyLevelDisplayName(level),
        level: level.level,
      }),
    })),
    {
      value: GENERIC_SUBTASK_HIERARCHY_LEVEL,
      label: t('settings.itemTypes.genericSubtaskLevel'),
    },
  ]);

  const itemTypeColumns = [
    { key: 'icon', label: '', width: '40px', slot: 'icon' },
    { key: 'name', label: t('settings.itemTypes.name'), render: (itemType) => getItemTypeDisplayName(itemType) },
    { key: 'hierarchy_level', label: t('settings.itemTypes.hierarchyLevel'), slot: 'hierarchy_level' },
    { key: 'sort_order', label: t('common.order') },
    {
      key: 'configuration_set_names',
      label: t('settings.configSets.title'),
      slot: 'configuration_set_names',
    },
    { key: 'actions', label: t('common.actions') },
  ];

  function buildItemTypeDropdownItems(itemType, { edit, remove }) {
    return [
      {
        id: 'edit',
        type: 'regular',
        testid: 'item-type-edit',
        icon: Edit,
        title: t('common.edit'),
        hoverClass: 'hover-bg',
        onClick: () => edit(itemType),
      },
      {
        id: 'delete',
        type: 'regular',
        testid: 'item-type-delete',
        icon: Trash2,
        title: t('common.delete'),
        color: 'var(--ds-text-danger)',
        hoverClass: 'hover-danger',
        onClick: () => remove(itemType),
      },
    ];
  }
</script>

<EntityManagerPage
  icon={FileText}
  title={t('settings.itemTypes.title')}
  subtitle={t('settings.itemTypes.subtitle')}
  addLabel={t('settings.itemTypes.addItemType')}
  addTestid="item-type-add"
  hotkeyScope="itemTypes"
  emptyMessage={t('settings.itemTypes.noItemTypes')}
  emptyIcon={FileText}
  columns={itemTypeColumns}
  load={loadItemTypes}
  create={saveItemType}
  update={saveItemType}
  remove={deleteItemType}
  onOpenCreate={openCreate}
  onOpenEdit={openEdit}
  confirmDeleteMessage={(itemType) =>
    t('settings.itemTypes.confirmDelete', { name: getItemTypeDisplayName(itemType) })}
  rowTestid={(itemType) => `item-type-row-${itemType.id}`}
  rowAttrs={(itemType) => ({ 'data-hierarchy-level': itemType.hierarchy_level })}
  actionItems={buildItemTypeDropdownItems}
  actionTriggerTestid={(itemType) => `item-type-actions-${itemType.id}`}
  createTitle={t('itemTypes.createItemType')}
  editTitle={t('itemTypes.editItemType')}
  confirmTestid="dialog-confirm"
  errorTestid="item-type-error"
  onError={handleError}
  errorLabels={{
    load: t('settings.itemTypes.failedToLoad'),
    save: t('settings.itemTypes.failedToSave'),
    delete: t('settings.itemTypes.deleteFailed'),
  }}
  cellSnippets={{
    icon: itemTypeIconCell,
    hierarchy_level: itemTypeHierarchyCell,
    configuration_set_names: itemTypeConfigurationSetsCell,
  }}
  formFields={itemTypeForm}
>
</EntityManagerPage>

{#snippet itemTypeIconCell(itemType)}
  <div class="flex items-center justify-center">
    <ItemTypeIcon {itemType} title={getItemTypeDisplayName(itemType)} />
  </div>
{/snippet}

{#snippet itemTypeHierarchyCell(itemType)}
  <Lozenge
    color={isGenericSubtaskType(itemType) ? 'gray' : 'blue'}
    text={getHierarchyLevelName(itemType.hierarchy_level)}
  />
{/snippet}

{#snippet itemTypeConfigurationSetsCell(itemType)}
  <div class="flex flex-wrap gap-1">
    {#if itemType.configuration_set_names && itemType.configuration_set_names.length > 0}
      {#each itemType.configuration_set_names as configSetName, index}
        <Lozenge
          color="gray"
          text={getConfigurationSetDisplayName(
            configSetName,
            itemType.configuration_set_builtin_keys?.[index]
          )}
        />
      {/each}
    {:else}
      <span class="text-xs text-ds-text-subtle">{t('settings.itemTypes.noConfigurationSets')}</span>
    {/if}
  </div>
{/snippet}

{#snippet itemTypeForm()}
  {#if editingId}
    {#key editingId}
      <LocalizedObjectFields
        bind:this={translationEditor}
        objectType="item_type"
        objectId={editingId}
        bind:canonicalName={formData.name}
        bind:canonicalDescription={formData.description}
        displayName={editingObject?.display_name || editingObject?.name}
        displayDescription={editingObject?.display_description || editingObject?.description}
      />
    {/key}
  {:else}
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
  {/if}

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
      <Input type="number" id="sort_order" min={1} bind:value={formData.sort_order} required />
    </div>
  </div>

  {#if hierarchyLevelChanged}
    <AlertBox variant="warning" class="mb-4">
      <div data-testid="admin-hierarchy-change-warning">
        <p class="font-medium">{t('settings.itemTypes.hierarchyChangeWarningTitle')}</p>
        <p class="mt-1" style="color: var(--ds-text-subtle);">
          {t('settings.itemTypes.hierarchyChangeWarningDescription', {
            fromLevel: originalHierarchyLevel,
            toLevel: formData.hierarchy_level,
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
{/snippet}
