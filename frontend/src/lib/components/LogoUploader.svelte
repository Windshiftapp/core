<script>
  import { Upload, Trash2, Image } from 'lucide-svelte';
  import Button from './Button.svelte';
  import Label from './Label.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { attachmentStatus } from '../stores';

  let {
    currentLogoUrl = null,
    onUpload = () => {},
    onRemove = () => {},
    uploading = false,
    label = null,
    helpText = null,
    maxHeight = '50px'
  } = $props();

  function handleFileChange(e) {
    const files = e.target.files;
    if (files && files.length > 0) {
      onUpload(files);
    }
    // Reset the input so the same file can be selected again
    e.target.value = '';
  }
</script>

<div class="logo-uploader">
  <Label class="mb-3">{label || t('lookAndFeel.logo', 'Logo')}</Label>

  <!-- Current logo preview -->
  {#if currentLogoUrl}
    <div class="mb-4 p-3 rounded-lg border flex items-center gap-4" style="border-color: var(--ds-border-focused); background-color: var(--ds-surface);">
      <div class="flex-shrink-0 flex items-center justify-center" style="max-height: {maxHeight};">
        <img src={currentLogoUrl} alt="Current logo" class="object-contain" style="max-height: {maxHeight}; max-width: 150px;" />
      </div>
      <div class="flex-1 min-w-0">
        <div class="text-sm font-medium" style="color: var(--ds-text);">{t('lookAndFeel.currentLogo', 'Current Logo')}</div>
        <div class="text-xs truncate" style="color: var(--ds-text-subtle);">{currentLogoUrl}</div>
      </div>
      <Button
        variant="default"
        size="sm"
        onclick={onRemove}
        icon={Trash2}
      >
        {t('workspaceSettings.remove', 'Remove')}
      </Button>
    </div>
  {:else}
    <div class="mb-4 p-4 rounded-lg border border-dashed flex items-center justify-center gap-3" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
      <Image class="w-8 h-8" style="color: var(--ds-text-subtle);" />
      <span class="text-sm" style="color: var(--ds-text-subtle);">{t('lookAndFeel.noLogoSet', 'No logo set')}</span>
    </div>
  {/if}

  <!-- Upload button and input -->
  <div class="flex items-center gap-3">
    <label class="cursor-pointer">
      <input
        type="file"
        accept="image/png,image/jpeg,image/svg+xml,image/gif,image/webp"
        onchange={handleFileChange}
        disabled={uploading || !attachmentStatus.enabled}
        class="hidden"
      />
      <Button
        variant="default"
        size="sm"
        icon={Upload}
        disabled={uploading || !attachmentStatus.enabled}
        onclick={(e) => e.target.closest('label').querySelector('input').click()}
      >
        {uploading ? t('workspaceSettings.uploading', 'Uploading...') : t('lookAndFeel.uploadLogo', 'Upload Logo')}
      </Button>
    </label>
    {#if !attachmentStatus.enabled}
      <span class="text-xs" style="color: var(--ds-text-warning);">
        {t('workspaceSettings.attachmentsRequired', 'Attachments must be enabled')}
      </span>
    {/if}
  </div>

  {#if helpText}
    <p class="text-xs mt-2" style="color: var(--ds-text-subtle);">
      {helpText}
    </p>
  {:else}
    <p class="text-xs mt-2" style="color: var(--ds-text-subtle);">
      {t('lookAndFeel.logoRecommendation', 'Recommended: PNG or SVG with transparent background. Max height in header: 40-50px.')}
    </p>
  {/if}
</div>
