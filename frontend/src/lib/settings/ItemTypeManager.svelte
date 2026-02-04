<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';
  import { createEventDispatcher } from 'svelte';
  import { Plus, Edit, Trash2, FileText } from 'lucide-svelte';
  import { itemTypeIconMap, itemTypeIconOptions } from '../utils/icons.js';
  import Button from '../components/Button.svelte';
  import DataTable from '../components/DataTable.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';
  import Select from '../components/Select.svelte';
  import Textarea from '../components/Textarea.svelte';
  import Input from '../components/Input.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import ColorPicker from '../editors/ColorPicker.svelte';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';

  const dispatch = createEventDispatcher();

  let itemTypes = $state([]);
  let hierarchyLevels = $state([]);
  let isLoading = $state(true);
  let error = $state(null);
  let editingId = $state(null);
  let showCreateForm = $state(false);

  // Form data
  let formData = $state({
    name: '',
    description: '',
    icon: 'FileText',
    color: '#3b82f6',
    hierarchy_level: 0, // Default to level 0 (Initiative level)
    sort_order: 1
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
      itemTypes = itemTypes.sort((a, b) => {
        if (a.hierarchy_level !== b.hierarchy_level) {
          return a.hierarchy_level - b.hierarchy_level;
        }
        return a.sort_order - b.sort_order;
      });
    } catch (err) {
      error = 'Failed to load item types: ' + err.message;
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
      sort_order: getNextSortOrder(defaultHierarchyLevel)
    };
    editingId = null;
    showCreateForm = true;
  }

  function startEdit(itemType) {
    formData = {
      name: itemType.name,
      description: itemType.description,
      icon: itemType.icon,
      color: itemType.color,
      hierarchy_level: itemType.hierarchy_level,
      sort_order: itemType.sort_order
    };
    editingId = itemType.id;
    showCreateForm = true;
  }

  function cancelEdit() {
    showCreateForm = false;
    editingId = null;
    formData = {
      name: '',
      description: '',
      icon: 'FileText',
      color: '#3b82f6',
      hierarchy_level: 0, // Default to level 0 (Initiative level)
      sort_order: 1
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

  async function saveItemType() {
    try {
      if (!formData.name.trim()) {
        error = t('settings.itemTypes.nameRequired');
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
      error = t('settings.itemTypes.failedToSave') + ' ' + err.message;
    }
  }

  async function deleteItemType(id, name) {
    if (!confirm(`Are you sure you want to delete "${name}"? This action cannot be undone.`)) {
      return;
    }

    try {
      await api.itemTypes.delete(id);
      await loadItemTypes();
      error = null;
      window.dispatchEvent(new CustomEvent('refresh-workspace-data'));
    } catch (err) {
      error = err.message;
    }
  }

  function getHierarchyLevelName(level) {
    const hierarchyLevel = hierarchyLevels.find(hl => hl.level === level);
    return hierarchyLevel ? `Level ${level} - ${hierarchyLevel.name}` : `Level ${level}`;
  }

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
      label: t('settings.itemTypes.name')
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
        color: '#dc2626',
        hoverClass: 'hover:bg-red-50',
        onClick: () => deleteItemType(itemType.id, itemType.name)
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
    emptyMessage={t('settings.itemTypes.noItemTypes') || 'No work item types configured yet.'}
    emptyIcon={FileText}
    actionItems={buildItemTypeDropdownItems}
  >
    <div slot="icon" let:item={itemType} class="flex items-center justify-center">
      <div class="w-6 h-6 rounded flex items-center justify-center" style="background-color: {itemType.color}">
        <svelte:component this={itemTypeIconMap[itemType.icon] || FileText} size={12} color="white" />
      </div>
    </div>

    <Lozenge slot="hierarchy_level" let:item={itemType} color="blue" text={getHierarchyLevelName(itemType.hierarchy_level)} />

    <div slot="configuration_set_names" let:item={itemType} class="flex flex-wrap gap-1">
      {#if itemType.configuration_set_names && itemType.configuration_set_names.length > 0}
        {#each itemType.configuration_set_names as configSetName}
          <Lozenge color="gray" text={configSetName} />
        {/each}
      {:else}
        <span class="text-xs text-gray-500">No configuration sets</span>
      {/if}
    </div>
  </DataTable>

  <Modal isOpen={showCreateForm} onclose={cancelEdit} maxWidth="max-w-2xl">
    <!-- Modal header -->
    <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
      <h3 class="text-lg font-semibold" style="color: var(--ds-text);">
        {editingId ? t('itemTypes.editItemType') : t('itemTypes.createItemType')}
      </h3>
    </div>

    <!-- Modal content -->
    <div class="px-6 py-4">
      <form onsubmit={(e) => { e.preventDefault(); saveItemType(); }}>
        <div class="form-group">
          <label for="name">{t('settings.itemTypes.name')}</label>
          <input
            type="text"
            id="name"
            placeholder="e.g. Epic, Story, Task, Bug"
            bind:value={formData.name}
            required
          />
        </div>

        <div class="form-group">
          <label for="description">{t('settings.itemTypes.description')}</label>
          <Textarea
            id="description"
            placeholder="Brief description of this item type"
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
            >
              {#each hierarchyLevels as level}
                <option value={level.level}>{level.name} (Level {level.level})</option>
              {/each}
            </Select>
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

        <div class="form-row">
          <div class="form-group">
            <label for="icon">{t('settings.itemTypes.icon')}</label>
            <Select id="icon" bind:value={formData.icon} required>
              {#each itemTypeIconOptions as icon}
                <option value={icon}>{icon}</option>
              {/each}
            </Select>
            <div class="icon-preview">
              <div class="preview-icon" style="background-color: {formData.color}">
                <svelte:component this={itemTypeIconMap[formData.icon] || FileText} size={16} color="white" />
              </div>
              {t('common.preview')}
            </div>
          </div>

          <div class="form-group">
            <ColorPicker bind:value={formData.color} label={t('settings.itemTypes.color')} />
          </div>
        </div>

      </form>
    </div>

    <DialogFooter
      onCancel={cancelEdit}
      onConfirm={saveItemType}
      confirmLabel={editingId ? t('common.update') : t('common.create')}
    />
  </Modal>

<style>
  .form-row {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 1rem;
    margin-bottom: 1.5rem;
  }

  .form-group {
    margin-bottom: 1.5rem;
  }

  .form-row .form-group {
    margin-bottom: 0;
  }

  .form-group label {
    display: block;
    margin-bottom: 0.5rem;
    font-weight: 500;
    color: var(--ds-text);
  }

  .form-group input,
  .form-group textarea,
  .form-group select {
    width: 100%;
    padding: 0.75rem;
    border: 1px solid var(--ds-border);
    border-radius: 6px;
    font-size: 0.9rem;
    background: var(--ds-surface);
    color: var(--ds-text);
    transition: border-color 0.2s ease;
  }

  .form-group input:focus,
  .form-group textarea:focus,
  .form-group select:focus {
    outline: none;
    border-color: var(--ds-border-focused);
    box-shadow: 0 0 0 3px var(--ds-focus-ring);
  }

  .icon-preview {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-top: 0.5rem;
    font-size: 0.85rem;
    color: var(--ds-text-subtle);
  }

  .preview-icon {
    width: 24px;
    height: 24px;
    border-radius: 4px;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .error {
    background: var(--ds-danger-subtle);
    color: var(--ds-text-danger);
    padding: 1rem;
    border-radius: 6px;
    margin-bottom: 1rem;
    border: 1px solid var(--ds-border-danger);
  }
</style>