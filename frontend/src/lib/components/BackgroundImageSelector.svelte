<script>
  import { Upload, Trash2 } from 'lucide-svelte';
  import { backgroundCategories, getPresetsByCategory } from '../utils/backgroundImages.js';
  import Button from './Button.svelte';
  import Label from './Label.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { attachmentStatus } from '../stores';

  let {
    currentImageUrl = null,
    selectedCategory = 'abstract',
    onSelectImage = () => {},
    onRemoveImage = () => {},
    onUploadImage = () => {},
    uploading = false,
    uploadCategory = 'portal_background',
    label = null
  } = $props();

  let showUploadSection = $state(false);
  let localSelectedCategory = $state(selectedCategory);

  // Sync local category with prop
  $effect(() => {
    localSelectedCategory = selectedCategory;
  });

  function selectCategory(categoryId) {
    localSelectedCategory = categoryId;
  }

  function handleFileChange(e) {
    const files = e.target.files;
    if (files && files.length > 0) {
      onUploadImage(files);
    }
  }
</script>

<div class="border-t pt-6" style="border-color: var(--ds-border);">
  <Label class="mb-4">{label || t('lookAndFeel.backgroundImages')}</Label>

  <!-- Current background image preview -->
  {#if currentImageUrl}
    <div class="mb-4 p-3 rounded-lg border flex items-center gap-4" style="border-color: var(--ds-border-focused); background-color: var(--ds-surface);">
      <div class="w-20 h-12 rounded overflow-hidden flex-shrink-0">
        <img src={currentImageUrl} alt="Current background" class="w-full h-full object-cover" />
      </div>
      <div class="flex-1 min-w-0">
        <div class="text-sm font-medium" style="color: var(--ds-text);">{t('lookAndFeel.currentBackground')}</div>
        <div class="text-xs truncate" style="color: var(--ds-text-subtle);">{currentImageUrl}</div>
      </div>
      <Button
        variant="default"
        size="sm"
        onclick={onRemoveImage}
        icon={Trash2}
      >
        {t('workspaceSettings.remove')}
      </Button>
    </div>
  {/if}

  <!-- Category tabs -->
  <div class="flex gap-2 mb-5">
    {#each backgroundCategories as category}
      <button
        class="category-tab px-3 py-1.5 text-sm font-medium rounded-md transition-colors"
        class:category-tab-selected={localSelectedCategory === category.id}
        onclick={() => selectCategory(category.id)}
      >
        {category.name}
      </button>
    {/each}
  </div>

  <!-- Preset images grid -->
  <div class="grid grid-cols-4 gap-3 mb-6">
    {#each getPresetsByCategory(localSelectedCategory) as preset}
      <button
        onclick={() => onSelectImage(preset.url)}
        class="group relative aspect-[4/3] rounded-lg overflow-hidden transition-all hover:scale-105"
        style={currentImageUrl === preset.url ? 'box-shadow: 0 0 0 2px var(--ds-border-focused);' : ''}
        title={preset.name}
      >
        <img
          src={preset.thumbnail}
          alt={preset.name}
          class="w-full h-full object-cover"
          loading="lazy"
        />
        <div class="absolute inset-0 bg-black/0 group-hover:bg-black/20 transition-colors"></div>
        <div class="absolute bottom-0 left-0 right-0 p-2 bg-gradient-to-t from-black/60 to-transparent">
          <span class="text-xs text-white font-medium">{preset.name}</span>
        </div>
      </button>
    {/each}
  </div>

  <!-- Custom upload section -->
  <div class="border-t pt-5" style="border-color: var(--ds-border);">
    <div class="flex items-center gap-3">
      <Button
        variant="default"
        size="sm"
        onclick={() => showUploadSection = !showUploadSection}
        icon={Upload}
        disabled={!attachmentStatus.enabled}
      >
        {t('lookAndFeel.uploadCustomImage')}
      </Button>
      {#if !attachmentStatus.enabled}
        <span class="text-xs" style="color: var(--ds-text-warning);">
          {t('workspaceSettings.attachmentsRequired')}
        </span>
      {/if}
    </div>

    {#if showUploadSection && attachmentStatus.enabled}
      <div class="mt-3 p-4 rounded-lg border" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
        <input
          type="file"
          accept="image/*"
          onchange={handleFileChange}
          disabled={uploading}
          class="block w-full text-sm text-gray-500 file:mr-4 file:py-2 file:px-4 file:rounded-md file:border-0 file:text-sm file:font-medium file:bg-blue-50 file:text-blue-700 hover:file:bg-blue-100 disabled:opacity-50"
        />
        {#if uploading}
          <div class="mt-2 text-sm text-blue-600">{t('workspaceSettings.uploading')}</div>
        {/if}
        <p class="text-xs mt-2" style="color: var(--ds-text-subtle);">
          {t('lookAndFeel.backgroundUploadRecommendation')}
        </p>
      </div>
    {/if}
  </div>
</div>

<style>
  .category-tab {
    background-color: var(--ds-surface);
    color: var(--ds-text-subtle);
    border: 1px solid var(--ds-border);
  }

  .category-tab:hover {
    background-color: var(--ds-surface-hovered);
    color: var(--ds-text);
  }

  .category-tab-selected {
    background-color: var(--ds-interactive) !important;
    color: white !important;
    border-color: var(--ds-interactive) !important;
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
  }
</style>
