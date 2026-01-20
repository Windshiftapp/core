<script>
  import { Tag, Trash2 } from 'lucide-svelte';
  import Modal from './Modal.svelte';
  import Button from '../components/Button.svelte';
  import ColorPicker from '../editors/ColorPicker.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let {
    isOpen = false,
    onClose = () => {},
    title = 'Manage Categories',
    categories = [],
    onAdd = async () => {},
    onDelete = async () => {},
    showColorPicker = true
  } = $props();

  let newCategoryName = $state('');
  let newCategoryColor = $state('#3b82f6');
  let loading = $state(false);

  async function addCategory() {
    if (!newCategoryName.trim()) return;

    loading = true;
    try {
      const data = { name: newCategoryName.trim() };
      if (showColorPicker) {
        data.color = newCategoryColor;
      }
      await onAdd(data);
      newCategoryName = '';
      newCategoryColor = '#3b82f6';
    } catch (error) {
      console.error('Failed to add category:', error);
    } finally {
      loading = false;
    }
  }

  async function deleteCategory(category) {
    if (!confirm(t('categories.confirmDeleteCategory', { name: category.name }))) {
      return;
    }

    loading = true;
    try {
      await onDelete(category.id);
    } catch (error) {
      console.error('Failed to delete category:', error);
      alert(t('categories.failedToDeleteCategory'));
    } finally {
      loading = false;
    }
  }

  function handleClose() {
    newCategoryName = '';
    newCategoryColor = '#3b82f6';
    onClose();
  }
</script>

<Modal
  {isOpen}
  onclose={handleClose}
  maxWidth="max-w-2xl"
>
  <div class="p-6">
    <h3 class="text-xl font-semibold mb-6" style="color: var(--ds-text);">
      {title}
    </h3>

    <!-- Add New Category Form -->
    <div class="mb-6 p-4 rounded border" style="background-color: var(--ds-background-neutral); border-color: var(--ds-border);">
      <h4 class="text-sm font-medium mb-3" style="color: var(--ds-text);">{t('categories.addNewCategory')}</h4>
      <div class="flex gap-3 items-center">
        <div class="flex-1">
          <input
            type="text"
            bind:value={newCategoryName}
            class="w-full px-3 py-2 rounded border focus:outline-none focus:ring-2 focus:ring-blue-500"
            style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
            placeholder={t('categories.categoryNamePlaceholder')}
            onkeydown={(e) => e.key === 'Enter' && addCategory()}
            disabled={loading}
          />
        </div>
        {#if showColorPicker}
          <ColorPicker bind:value={newCategoryColor} compact={true} />
        {/if}
        <Button
          variant="primary"
          onclick={addCategory}
          disabled={!newCategoryName.trim() || loading}
        >
          {t('categories.addCategory')}
        </Button>
      </div>
    </div>

    <!-- Existing Categories List -->
    <div>
      <h4 class="text-sm font-medium mb-3" style="color: var(--ds-text);">
        {t('categories.existingCategories')} ({categories.length})
      </h4>

      {#if categories.length > 0}
        <div class="space-y-2 max-h-80 overflow-y-auto">
          {#each categories as category (category.id)}
            <div class="flex items-center justify-between p-3 rounded border" style="background-color: var(--ds-surface); border-color: var(--ds-border);">
              <div class="flex items-center gap-3">
                <div
                  class="w-4 h-4 rounded-full flex-shrink-0"
                  style="background-color: {category.color || '#6b7280'};"
                ></div>
                <span class="font-medium" style="color: var(--ds-text);">
                  {category.name}
                </span>
              </div>
              <Button
                variant="danger"
                size="small"
                icon={Trash2}
                onclick={() => deleteCategory(category)}
                disabled={loading}
              >
                {t('common.delete')}
              </Button>
            </div>
          {/each}
        </div>
      {:else}
        <EmptyState
          icon={Tag}
          title={t('categories.noCategoriesYet')}
          description={t('categories.addFirstCategoryHint')}
        />
      {/if}
    </div>

    <!-- Modal Footer -->
    <div class="mt-6 pt-4 border-t flex justify-end" style="border-color: var(--ds-border);">
      <Button
        variant="default"
        onclick={handleClose}
      >
        {t('common.close')}
      </Button>
    </div>
  </div>
</Modal>
