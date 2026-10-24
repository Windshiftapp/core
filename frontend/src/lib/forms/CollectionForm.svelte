<script>
  import { onMount } from 'svelte';
  import FolderOpen from '@lucide/svelte/icons/folder-open';
  import { t } from '../stores/i18n.svelte.js';
  import MilkdownEditor from '../editors/LazyMilkdownEditor.svelte';
  import ChipPicker from '../pickers/ChipPicker.svelte';
  import { collectionCategoriesStore } from '../stores/collectionCategories.js';
  import Input from '../components/Input.svelte';

  let {
    formData = $bindable({
      name: '',
      description: '',
      workspace_id: null
    }),
    categoryId = $bindable(null),
    nameInputRef = $bindable(null),
    categories = null,
  } = $props();

  onMount(() => {
    if (categories === null) {
      collectionCategoriesStore.init();
    }
  });

  let availableCategories = $derived(categories ?? $collectionCategoriesStore);

  export function validate() {
    return formData.name.trim() !== '';
  }

  export function getFormData() {
    return {
      name: formData.name,
      description: formData.description || '',
      ql_query: '',
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
  <Input
    dataTestid="collection-create-name"
    bind:inputRef={nameInputRef}
    bind:value={formData.name}
    type="text"
    variant="ghost"
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
    <ChipPicker
      value={categoryId}
      items={[{ id: null, name: t('createModal.noCategory') }, ...availableCategories]}
      getValue={(c) => c.id}
      getLabel={(c) => c.name}
      icon={FolderOpen}
      placeholder={t('createModal.category')}
      onSelect={(category) => categoryId = category.id}
    />
  </div>
</div>
