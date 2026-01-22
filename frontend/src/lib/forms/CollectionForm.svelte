<script>
  import { onMount } from 'svelte';
  import { FolderOpen } from 'lucide-svelte';
  import { t } from '../stores/i18n.svelte.js';
  import MilkdownEditor from '../editors/LazyMilkdownEditor.svelte';
  import FieldChip from '../components/FieldChip.svelte';
  import { collectionCategoriesStore } from '../stores/collectionCategories.js';

  let {
    formData = $bindable({
      name: '',
      description: '',
      workspace_id: null
    }),
    categoryId = $bindable(null),
    nameInputRef = $bindable(null)
  } = $props();

  onMount(() => {
    collectionCategoriesStore.init();
  });

  export function validate() {
    return formData.name.trim() !== '';
  }

  export function getFormData() {
    return {
      name: formData.name,
      description: formData.description || '',
      cql_query: '',
      is_public: false,
      workspace_id: formData.workspace_id,
      category_id: categoryId
    };
  }

  export function reset() {
    formData = {
      name: '',
      description: '',
      workspace_id: null
    };
    categoryId = null;
  }

  export function isValid() {
    return formData.name.trim() !== '';
  }
</script>

<div class="space-y-3">
  <!-- Title Input -->
  <input
    bind:this={nameInputRef}
    bind:value={formData.name}
    type="text"
    class="w-full text-lg font-medium border-0 outline-none bg-transparent"
    style="color: var(--ds-text);"
    placeholder={t('createModal.workspaceName', { type: t('createModal.collection') })}
  />

  <!-- Description -->
  <div class="min-h-[60px]">
    <MilkdownEditor
      bind:content={formData.description}
      placeholder={t('createModal.addDescription')}
      compact={true}
      showToolbar={false}
      readonly={false}
      itemId={null}
    />
  </div>

  <!-- Field Chips Row -->
  <div class="flex flex-wrap items-center gap-2 pt-2 border-t" style="border-color: var(--ds-border);">
    <FieldChip
      label={t('createModal.category')}
      value={categoryId}
      displayValue={$collectionCategoriesStore.find(c => c.id === categoryId)?.name || ''}
      icon={FolderOpen}
      placeholder={t('createModal.category')}
    >
      {#snippet children({ close: closePopover })}
        <div class="p-2 max-h-48 overflow-y-auto">
          <button
            type="button"
            class="w-full px-3 py-2 text-left text-sm rounded transition-colors"
            style="color: var(--ds-text-subtle);"
            onmouseover={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-selected)'}
            onmouseout={(e) => e.currentTarget.style.backgroundColor = 'transparent'}
            onclick={() => {
              categoryId = null;
              closePopover();
            }}
          >
            {t('createModal.noCategory')}
          </button>
          {#each $collectionCategoriesStore as category}
            <button
              type="button"
              class="w-full px-3 py-2 text-left text-sm rounded transition-colors"
              style="color: var(--ds-text);"
              onmouseover={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-selected)'}
              onmouseout={(e) => e.currentTarget.style.backgroundColor = 'transparent'}
              onclick={() => {
                categoryId = category.id;
                closePopover();
              }}
            >
              {category.name}
            </button>
          {/each}
        </div>
      {/snippet}
    </FieldChip>
  </div>
</div>
