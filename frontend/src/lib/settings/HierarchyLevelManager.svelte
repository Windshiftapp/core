<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';
  import { Plus, Edit, Trash2, Move, ChevronUp, ChevronDown, Circle, MoreHorizontal, Network } from 'lucide-svelte';
  import Button from '../components/Button.svelte';
  import DataTable from '../components/DataTable.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';
  import { errorToast } from '../stores/toasts.svelte.js';
  import Textarea from '../components/Textarea.svelte';
  import { confirm } from '../composables/useConfirm.js';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';

  let hierarchyLevels = $state([]);
  let isLoading = $state(true);
  let error = $state(null);
  let editingId = $state(null);
  let showCreateForm = $state(false);

  // Form data
  let formData = $state({
    level: 0,
    name: '',
    description: ''
  });


  onMount(() => {
    loadHierarchyLevels();
  });

  async function loadHierarchyLevels() {
    try {
      isLoading = true;
      error = null;
      const data = await api.hierarchyLevels.getAll();
      hierarchyLevels = data.sort((a, b) => a.level - b.level);
    } catch (err) {
      error = 'Failed to load hierarchy levels: ' + err.message;
    } finally {
      isLoading = false;
    }
  }

  function startCreate() {
    formData = {
      level: hierarchyLevels.length > 0 ? Math.max(...hierarchyLevels.map(h => h.level)) + 1 : 0,
      name: '',
      description: ''
    };
    editingId = null;
    showCreateForm = true;
  }

  function startEdit(hierarchyLevel) {
    formData = {
      level: hierarchyLevel.level,
      name: hierarchyLevel.name,
      description: hierarchyLevel.description
    };
    editingId = hierarchyLevel.id;
    showCreateForm = true;
  }

  function cancelEdit() {
    showCreateForm = false;
    editingId = null;
    formData = { level: 0, name: '', description: '' };
  }

  async function saveHierarchyLevel(event) {
    event.preventDefault();
    try {
      if (!formData.name.trim()) {
        error = t('settings.hierarchyLevels.nameRequired');
        return;
      }

      if (editingId) {
        await api.hierarchyLevels.update(editingId, formData);
      } else {
        await api.hierarchyLevels.create(formData);
      }

      await loadHierarchyLevels();
      cancelEdit();
      error = null;
    } catch (err) {
      error = err.message;
    }
  }

  async function deleteHierarchyLevel(id, name) {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: `${t('settings.hierarchyLevels.confirmDelete')} "${name}"?`,
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger',
      icon: Trash2
    });

    if (!confirmed) return;

    try {
      await api.hierarchyLevels.delete(id);
      await loadHierarchyLevels();
    } catch (err) {
      errorToast(err.message || 'Failed to delete hierarchy level', 'Cannot Delete Hierarchy Level');
    }
  }

  async function moveLevel(id, direction) {
    const currentLevel = hierarchyLevels.find(h => h.id === id);
    if (!currentLevel) return;

    const newLevel = direction === 'up' ? currentLevel.level - 1 : currentLevel.level + 1;

    // Check if the new level already exists
    const conflictLevel = hierarchyLevels.find(h => h.level === newLevel);
    if (conflictLevel) {
      error = `Level ${newLevel} is already occupied by "${conflictLevel.name}"`;
      return;
    }

    if (newLevel < 0) {
      error = 'Cannot move to negative level';
      return;
    }

    try {
      await api.hierarchyLevels.update(id, {
        ...currentLevel,
        level: newLevel
      });
      await loadHierarchyLevels();
      error = null;
    } catch (err) {
      error = err.message;
    }
  }

  function getLevelDescription(level) {
    const descriptions = {
      0: 'Top-level strategic work',
      1: 'Large features or capabilities',
      2: 'User stories and requirements',
      3: 'Development tasks and bugs',
      4: 'Sub-tasks and smaller work items'
    };
    return descriptions[level] || 'Work items at this level';
  }

  // Column definitions for DataTable
  const hierarchyColumns = $derived([
    {
      key: 'level',
      label: t('settings.hierarchyLevels.level'),
      width: '60px'
    },
    {
      key: 'name',
      label: t('settings.hierarchyLevels.name')
    },
    {
      key: 'description',
      label: t('settings.hierarchyLevels.description')
    },
    {
      key: 'actions',
      label: t('common.actions')
    }
  ]);

  function buildHierarchyDropdownItems(hierarchyLevel) {
    return [
      {
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: t('common.edit'),
        hoverClass: 'hover-bg',
        onClick: () => startEdit(hierarchyLevel)
      },
      {
        id: 'move-up',
        type: 'regular',
        icon: ChevronUp,
        title: t('common.moveUp'),
        hoverClass: 'hover-bg',
        disabled: hierarchyLevel.level === 0,
        onClick: () => moveLevel(hierarchyLevel.id, 'up')
      },
      {
        id: 'move-down',
        type: 'regular',
        icon: ChevronDown,
        title: t('common.moveDown'),
        hoverClass: 'hover-bg',
        onClick: () => moveLevel(hierarchyLevel.id, 'down')
      },
      {
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: t('common.delete'),
        color: '#dc2626',
        hoverClass: 'hover:bg-red-50',
        onClick: () => deleteHierarchyLevel(hierarchyLevel.id, hierarchyLevel.name)
      }
    ];
  }
</script>

<PageHeader
  icon={Network}
  title={t('settings.hierarchyLevels.title')}
  subtitle={t('settings.hierarchyLevels.subtitle')}
>
  {#snippet actions()}
    <Button
      variant="primary"
      icon={Plus}
      onclick={startCreate}
      disabled={isLoading}
      keyboardHint="A"
      hotkeyConfig={{ key: toHotkeyString('hierarchyLevels', 'add'), guard: () => !showCreateForm }}
    >
      {t('settings.hierarchyLevels.addHierarchyLevel')}
    </Button>
  {/snippet}
</PageHeader>

{#if error}
  <div class="error">
    {error}
  </div>
{/if}

<DataTable
  columns={hierarchyColumns}
  data={hierarchyLevels}
  keyField="id"
  emptyMessage="No hierarchy levels configured yet."
  emptyIcon={Circle}
  actionItems={buildHierarchyDropdownItems}
/>

<Modal isOpen={showCreateForm} onclose={cancelEdit} maxWidth="max-w-lg" onSubmit={saveHierarchyLevel}>
  {#snippet children(submitHint)}
  <!-- Modal header -->
  <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
    <h3 class="text-lg font-semibold" style="color: var(--ds-text);">
      {editingId ? t('common.edit') : t('common.create')} {t('settings.hierarchyLevels.level')}
    </h3>
  </div>

  <!-- Modal content -->
  <div class="px-6 py-4">
    <form onsubmit={saveHierarchyLevel}>
      <div class="form-group">
        <label for="level">{t('settings.hierarchyLevels.level')}</label>
        <input
          type="number"
          id="level"
          min="0"
          max="10"
          bind:value={formData.level}
          required
        />
        <small>Numeric hierarchy level (0 = highest)</small>
      </div>

      <div class="form-group">
        <label for="name">{t('settings.hierarchyLevels.name')}</label>
        <input
          type="text"
          id="name"
          placeholder="e.g. Initiative, Epic, Story, Task"
          bind:value={formData.name}
          required
        />
      </div>

      <div class="form-group">
        <label for="description">{t('settings.hierarchyLevels.description')}</label>
        <Textarea
          id="description"
          placeholder="Brief description of this hierarchy level"
          bind:value={formData.description}
          rows={3}
        />
      </div>

    </form>
  </div>

  <DialogFooter
    onCancel={cancelEdit}
    onConfirm={saveHierarchyLevel}
    confirmLabel={editingId ? t('common.update') : t('common.create')}
    showKeyboardHint={true}
    confirmKeyboardHint={submitHint}
  />
  {/snippet}
</Modal>


<style>
  .error {
    background: var(--ds-danger-subtle);
    color: var(--ds-text-danger);
    padding: 1rem;
    border-radius: 6px;
    margin-bottom: 1rem;
    border: 1px solid var(--ds-border-danger);
  }

  .form-group {
    margin-bottom: 1.5rem;
  }

  .form-group label {
    display: block;
    margin-bottom: 0.5rem;
    font-weight: 500;
    color: var(--ds-text);
  }

  .form-group input,
  .form-group textarea {
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
  .form-group textarea:focus {
    outline: none;
    border-color: var(--ds-border-focused);
    box-shadow: 0 0 0 3px var(--ds-focus-ring);
  }

  .form-group small {
    display: block;
    margin-top: 0.25rem;
    font-size: 0.8rem;
    color: var(--ds-text-subtle);
  }

  .loading {
    text-align: center;
    padding: 2rem;
    color: var(--ds-text-subtle);
  }
</style>
