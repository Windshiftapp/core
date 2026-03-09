<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import Button from '../../components/Button.svelte';
  import TimeProjectCategoryModal from '../../dialogs/TimeProjectCategoryModal.svelte';
  import { Plus, GripVertical, Edit, Trash2 } from 'lucide-svelte';
  import { toHotkeyString } from '../../utils/keyboardShortcuts.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { confirm } from '../../composables/useConfirm.js';

  let categories = $state([]);
  let showCreateForm = $state(false);
  let editingCategory = $state(null);
  let formData = $state({
    name: '',
    description: ''
  });

  const colors = ['#3b82f6', '#10b981', '#f59e0b', '#8b5cf6', '#ef4444', '#ec4899', '#06b6d4', '#84cc16'];

  onMount(async () => {
    await loadCategories();
  });

  async function loadCategories() {
    try {
      const result = await api.time.projectCategories.getAll();
      categories = result || [];
    } catch (error) {
      console.error('Failed to load categories:', error);
      categories = [];
    }
  }

  function startCreate() {
    showCreateForm = true;
    editingCategory = null;
    resetForm();
  }

  function startEdit(category) {
    editingCategory = category;
    formData = {
      name: category.name,
      description: category.description || ''
    };
    showCreateForm = true;
  }

  function resetForm() {
    formData = {
      name: '',
      description: ''
    };
  }

  function cancelForm() {
    showCreateForm = false;
    editingCategory = null;
    resetForm();
  }

  async function saveCategory() {
    try {
      if (editingCategory) {
        await api.time.projectCategories.update(editingCategory.id, {
          ...formData,
          display_order: editingCategory.display_order,
          color: editingCategory.color // Keep existing color on edit
        });
      } else {
        // Assign random color for new categories
        const randomColor = colors[Math.floor(Math.random() * colors.length)];
        await api.time.projectCategories.create({
          ...formData,
          color: randomColor
        });
      }
      await loadCategories();
      cancelForm();
    } catch (error) {
      console.error('Failed to save category:', error);
      alert(t('time.categories.failedToSave') + ': ' + (error.message || error));
    }
  }

  async function deleteCategory(category) {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('time.categories.confirmDelete', { name: category.name }),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger'
    });
    if (confirmed) {
      try {
        await api.time.projectCategories.delete(category.id);
        await loadCategories();
      } catch (error) {
        console.error('Failed to delete category:', error);
        alert(t('time.categories.failedToDelete') + ': ' + (error.message || error));
      }
    }
  }

  // Drag and drop reordering
  let draggedItem = $state(null);

  function handleDragStart(event, category) {
    draggedItem = category;
    event.dataTransfer.effectAllowed = 'move';
  }

  function handleDragOver(event) {
    event.preventDefault();
    event.dataTransfer.dropEffect = 'move';
  }

  function handleDrop(event, targetCategory) {
    event.preventDefault();
    if (draggedItem && draggedItem.id !== targetCategory.id) {
      const draggedIndex = categories.findIndex(c => c.id === draggedItem.id);
      const targetIndex = categories.findIndex(c => c.id === targetCategory.id);

      // Reorder array
      const newCategories = [...categories];
      newCategories.splice(draggedIndex, 1);
      newCategories.splice(targetIndex, 0, draggedItem);

      // Update display_order for all categories
      const updates = newCategories.map((cat, index) => ({
        id: cat.id,
        display_order: index
      }));

      // Optimistically update UI
      categories = newCategories.map((cat, index) => ({
        ...cat,
        display_order: index
      }));

      // Save to backend
      api.time.projectCategories.reorder(updates).catch(error => {
        console.error('Failed to reorder categories:', error);
        loadCategories(); // Reload on error
      });
    }
    draggedItem = null;
  }


</script>

<!-- Header -->
<div class="mb-6 flex justify-between items-start">
  <div>
    <h2 class="text-lg font-semibold" style="color: var(--ds-text);">{t('time.categories.title')}</h2>
    <div class="text-xs mt-1" style="color: var(--ds-text-subtle);">
      {t('time.categories.subtitle')}
    </div>
  </div>
  <Button
    variant="primary"
    onclick={startCreate}
    icon={Plus}
    size="medium"
    keyboardHint="A"
    hotkeyConfig={{ key: toHotkeyString('timeProjects', 'addCategory'), guard: () => !showCreateForm }}
  >
    {t('time.categories.newCategory')}
  </Button>
</div>

<!-- Categories List -->
<div class="space-y-2">
  {#if categories.length === 0}
    <div class="text-center py-12" style="color: var(--ds-text-subtle);">
      <p class="text-sm">{t('time.categories.noCategories')}</p>
      <p class="text-xs mt-1">{t('time.categories.createFirstHint')}</p>
    </div>
  {:else}
    {#each categories as category (category.id)}
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div
        class="flex items-center gap-3 p-3 rounded transition-colors"
        style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
        draggable="true"
        ondragstart={(e) => handleDragStart(e, category)}
        ondragover={handleDragOver}
        ondrop={(e) => handleDrop(e, category)}
      >
        <!-- Drag Handle -->
        <div class="cursor-move" style="color: var(--ds-text-subtle);">
          <GripVertical class="w-4 h-4" />
        </div>

        <!-- Color Indicator -->
        <div
          class="w-3 h-3 rounded-full flex-shrink-0"
          style="background-color: {category.color || '#3b82f6'};"
        ></div>

        <!-- Category Info -->
        <div class="flex-1 min-w-0">
          <div class="text-sm font-medium" style="color: var(--ds-text);">
            {category.name}
          </div>
          {#if category.description}
            <div class="text-xs mt-0.5 truncate" style="color: var(--ds-text-subtle);">
              {category.description}
            </div>
          {/if}
        </div>

        <!-- Actions -->
        <div class="flex items-center gap-1 flex-shrink-0">
          <button
            onclick={() => startEdit(category)}
            class="p-1.5 rounded hover-bg transition-colors"
            title={t('common.edit')}
          >
            <Edit class="w-4 h-4" style="color: var(--ds-text-subtle);" />
          </button>
          <button
            onclick={() => deleteCategory(category)}
            class="p-1.5 rounded hover:bg-red-50 transition-colors"
            title={t('common.delete')}
          >
            <Trash2 class="w-4 h-4 text-red-600" />
          </button>
        </div>
      </div>
    {/each}
  {/if}
</div>

<!-- Category Modal -->
<TimeProjectCategoryModal
  isOpen={showCreateForm}
  bind:formData
  isEditing={!!editingCategory}
  onsave={saveCategory}
  oncancel={cancelForm}
/>

