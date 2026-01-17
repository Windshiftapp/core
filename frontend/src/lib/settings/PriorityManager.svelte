<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { createEventDispatcher } from 'svelte';
  import { Plus, Edit, Trash2, AlertCircle } from 'lucide-svelte';
  import { priorityIconMap, priorityIconOptions } from '../utils/icons.js';
  import Button from '../components/Button.svelte';
  import DataTable from '../components/DataTable.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';
  import Select from '../components/Select.svelte';
  import Textarea from '../components/Textarea.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import Input from '../components/Input.svelte';
  import ColorPicker from '../editors/ColorPicker.svelte';
  import Toggle from '../components/Toggle.svelte';
  import { createShortcutHandler } from '../utils/keyboardShortcuts.js';
  import { t } from '../stores/i18n.svelte.js';

  const dispatch = createEventDispatcher();

  let priorities = $state([]);
  let isLoading = $state(true);
  let error = $state(null);
  let editingId = $state(null);
  let showCreateForm = $state(false);

  // Form data
  let formData = $state({
    name: '',
    description: '',
    icon: 'AlertCircle',
    color: '#dc2626',
    sort_order: 1,
    is_default: false
  });

  onMount(async () => {
    await loadPriorities();
  });

  function handleGlobalKeydown(event) {
    createShortcutHandler({
      add: startCreate
    }, 'priorities', { guard: () => !showCreateForm })(event);
  }

  async function loadPriorities() {
    try {
      isLoading = true;
      error = null;
      priorities = await api.priorities.getAll();
      // Sort by sort_order
      priorities = priorities.sort((a, b) => a.sort_order - b.sort_order);
    } catch (err) {
      error = 'Failed to load priorities: ' + err.message;
    } finally {
      isLoading = false;
    }
  }

  function startCreate() {
    formData = {
      name: '',
      description: '',
      icon: 'AlertCircle',
      color: colorOptions[0],
      sort_order: getNextSortOrder(),
      is_default: false
    };
    editingId = null;
    showCreateForm = true;
  }

  function startEdit(priority) {
    formData = {
      name: priority.name,
      description: priority.description,
      icon: priority.icon,
      color: priority.color,
      sort_order: priority.sort_order,
      is_default: priority.is_default || false
    };
    editingId = priority.id;
    showCreateForm = true;
  }

  function cancelEdit() {
    showCreateForm = false;
    editingId = null;
    formData = {
      name: '',
      description: '',
      icon: 'AlertCircle',
      color: '#dc2626',
      sort_order: 1,
      is_default: false
    };
  }

  function getNextSortOrder() {
    return priorities.length > 0 ? Math.max(...priorities.map(p => p.sort_order)) + 1 : 1;
  }

  async function savePriority() {
    try {
      if (!formData.name.trim()) {
        error = 'Priority name is required';
        return;
      }

      if (editingId) {
        await api.priorities.update(editingId, formData);
      } else {
        await api.priorities.create(formData);
      }

      await loadPriorities();
      cancelEdit();
      error = null;
    } catch (err) {
      error = err.message;
    }
  }

  async function deletePriority(id, name) {
    if (!confirm(`Are you sure you want to delete "${name}"? This action cannot be undone.`)) {
      return;
    }

    try {
      await api.priorities.delete(id);
      await loadPriorities();
      error = null;
    } catch (err) {
      error = err.message;
    }
  }

  // Column definitions for DataTable
  const priorityColumns = $derived([
    {
      key: 'icon',
      label: '',
      width: '40px',
      slot: 'icon'
    },
    {
      key: 'name',
      label: t('common.name')
    },
    {
      key: 'is_default',
      label: t('common.default'),
      width: '80px',
      slot: 'is_default'
    },
    {
      key: 'sort_order',
      label: t('common.order')
    },
    {
      key: 'configuration_set_names',
      label: t('configuration.title'),
      slot: 'configuration_set_names'
    },
    {
      key: 'actions',
      label: t('common.actions')
    }
  ]);

  function buildPriorityDropdownItems(priority) {
    return [
      {
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: t('common.edit'),
        hoverClass: 'hover-bg',
        onClick: () => startEdit(priority)
      },
      {
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: t('common.delete'),
        color: '#dc2626',
        hoverClass: 'hover:bg-red-50',
        onClick: () => deletePriority(priority.id, priority.name)
      }
    ];
  }
</script>

<svelte:window onkeydown={handleGlobalKeydown} />

<PageHeader
  icon={AlertCircle}
  title={t('priorities.title')}
  subtitle={t('priorities.subtitle')}
>
  {#snippet actions()}
    <Button
      variant="primary"
      icon={Plus}
      onclick={startCreate}
      disabled={isLoading}
      keyboardHint="A"
    >
      {t('priorities.createPriority')}
    </Button>
  {/snippet}
</PageHeader>

  {#if error}
    <div class="error">
      {error}
    </div>
  {/if}

  <DataTable
    columns={priorityColumns}
    data={priorities}
    keyField="id"
    emptyMessage={t('priorities.noPriorities')}
    emptyIcon={AlertCircle}
    actionItems={buildPriorityDropdownItems}
  >
    <div slot="icon" let:item={priority} class="flex items-center justify-center">
      <div class="w-6 h-6 rounded flex items-center justify-center" style="background-color: {priority.color}">
        <svelte:component this={priorityIconMap[priority.icon] || AlertCircle} size={12} color="white" />
      </div>
    </div>

    <div slot="is_default" let:item={priority} class="flex items-center">
      {#if priority.is_default}
        <Lozenge color="green" text={t('common.default')} />
      {/if}
    </div>

    <div slot="configuration_set_names" let:item={priority} class="flex flex-wrap gap-1">
      {#if priority.configuration_set_names && priority.configuration_set_names.length > 0}
        {#each priority.configuration_set_names as configSetName}
          <Lozenge color="gray" text={configSetName} />
        {/each}
      {:else}
        <span class="text-xs text-gray-500">{t('common.noData')}</span>
      {/if}
    </div>
  </DataTable>

  <Modal isOpen={showCreateForm} onclose={cancelEdit} maxWidth="max-w-2xl" onSubmit={savePriority} let:submitHint>
    <!-- Modal header -->
    <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
      <h3 class="text-lg font-semibold" style="color: var(--ds-text);">
        {editingId ? t('priorities.editPriority') : t('priorities.createPriority')}
      </h3>
    </div>

    <!-- Modal content -->
    <div class="px-6 py-4">
      <form onsubmit={(e) => { e.preventDefault(); savePriority(); }}>
        <div class="form-group">
          <label for="name">{t('common.name')}</label>
          <input
            type="text"
            id="name"
            placeholder="e.g. Critical, High, Medium, Low"
            bind:value={formData.name}
            required
          />
        </div>

        <div class="form-group">
          <label for="description">{t('common.description')}</label>
          <Textarea
            id="description"
            placeholder={t('placeholders.optionalDescription')}
            bind:value={formData.description}
            rows={2}
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

        <div class="form-row">
          <div class="form-group">
            <label for="icon">{t('common.icon')}</label>
            <Select id="icon" bind:value={formData.icon} required>
              {#each priorityIconOptions as icon}
                <option value={icon}>{icon}</option>
              {/each}
            </Select>
            <div class="icon-preview">
              <div class="preview-icon" style="background-color: {formData.color}">
                <svelte:component this={priorityIconMap[formData.icon] || AlertCircle} size={16} color="white" />
              </div>
              {t('common.preview')}
            </div>
          </div>

          <div class="form-group">
            <ColorPicker bind:value={formData.color} label={t('common.color')} />
          </div>
        </div>

        <div class="form-group">
          <Toggle
            bind:checked={formData.is_default}
            label={t('common.default')}
            size="small"
          />
        </div>

      </form>
    </div>

    <DialogFooter
      onCancel={cancelEdit}
      onConfirm={savePriority}
      confirmLabel={editingId ? t('common.update') : t('common.create')}
      showKeyboardHint={true}
      confirmKeyboardHint={submitHint}
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
