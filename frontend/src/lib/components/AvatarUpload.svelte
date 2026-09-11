<script>
  import FileInput from './FileInput.svelte';
  /** Reusable image-avatar upload control with availability/MIME checks and
   * fallback tile. Categories require matching backend attachment support. */
  import { Camera, Trash2, Package } from '@lucide/svelte';
  import { api } from '../api.js';
  import { attachmentStatus } from '../stores';
  import { t } from '../stores/i18n.svelte.js';
  import { successToast, errorToast } from '../stores/toasts.svelte.js';
  import Button from './Button.svelte';
  import Label from './Label.svelte';

  let {
    avatarUrl = $bindable(null),
    category,
    itemId,
    fallbackIcon: FallbackIcon = Package,
    fallbackColor = '#3b82f6',
    label = '',
    onUploaded = null,
    onRemoved = null,
  } = $props();

  let uploading = $state(false);
  let showUpload = $state(false);

  async function handleFiles(files) {
    if (!files || files.length === 0) return;

    if (!attachmentStatus.enabled) {
      errorToast(t('workspaceSettings.attachmentsRequired'));
      return;
    }

    const file = files[0];
    if (!file.type.startsWith('image/')) {
      errorToast(t('workspaceSettings.pleaseSelectImage'));
      return;
    }

    uploading = true;
    try {
      const formData = new FormData();
      formData.append('file', file);
      if (itemId != null) formData.append('item_id', String(itemId));
      formData.append('category', category);

      const result = await api.attachments.upload(formData);

      if (result?.success && result.avatar_url) {
        avatarUrl = result.avatar_url;
        showUpload = false;
        successToast(t('workspaceSettings.avatarUploadedSuccess'));
        onUploaded?.(result.avatar_url);
      }
    } catch (err) {
      errorToast(t('workspaceSettings.failedToUploadAvatar', { error: err.message || err }));
    } finally {
      uploading = false;
    }
  }

  function handleRemove() {
    avatarUrl = null;
    onRemoved?.();
  }
</script>

<div class="space-y-4">
  {#if label}
    <Label class="mb-2">{label}</Label>
  {/if}

  {#if avatarUrl}
    <div class="flex items-center gap-4 p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
      <img src={avatarUrl} alt="Avatar" class="w-16 h-16 rounded object-cover" />
      <div class="flex-1">
        <div class="text-sm font-medium" style="color: var(--ds-text);">{t('workspaceSettings.customAvatar')}</div>
        <div class="text-xs" style="color: var(--ds-text-subtle);">{t('workspaceSettings.imageUploadedSuccessfully')}</div>
      </div>
      <Button variant="default" size="sm" onclick={handleRemove} icon={Trash2}>
        {t('workspaceSettings.remove')}
      </Button>
    </div>
  {:else}
    <div class="flex items-center gap-4 p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
      <div class="w-16 h-16 rounded flex items-center justify-center" style="background-color: {fallbackColor};">
        <FallbackIcon size={32} color="white" />
      </div>
      <div class="flex-1">
        <div class="text-sm font-medium" style="color: var(--ds-text);">{t('workspaceSettings.defaultIcon')}</div>
        <div class="text-xs" style="color: var(--ds-text-subtle);">{t('workspaceSettings.usingSelectedIconColor')}</div>
      </div>
    </div>
  {/if}

  <div>
    <Button
      variant="default"
      size="sm"
      onclick={() => (showUpload = !showUpload)}
      icon={Camera}
      disabled={!attachmentStatus.enabled}
    >
      {avatarUrl ? t('workspaceSettings.changeAvatar') : t('workspaceSettings.uploadAvatar')}
    </Button>
    {#if !attachmentStatus.enabled}
      <p class="text-xs mt-1" style="color: var(--ds-text-warning);">
        {t('workspaceSettings.attachmentsRequired')}
      </p>
    {/if}
  </div>

  {#if showUpload && attachmentStatus.enabled}
    <div class="p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
      <FileInput
        accept="image/*"
        onchange={(e) => handleFiles(e.currentTarget.files)}
        disabled={uploading}
        class="block w-full text-sm text-ds-text-subtle file:mr-4 file:py-2 file:px-4 file:rounded-md file:border-0 file:text-sm file:font-medium file:bg-ds-background-neutral file:text-ds-text-link hover:file:bg-ds-background-neutral-hovered disabled:opacity-50"
      />
      {#if uploading}
        <div class="mt-2 text-sm text-ds-interactive">{t('workspaceSettings.uploading')}</div>
      {/if}
      <p class="text-xs mt-2" style="color: var(--ds-text-subtle);">
        {t('workspaceSettings.uploadRecommendation')}
      </p>
    </div>
  {/if}
</div>
