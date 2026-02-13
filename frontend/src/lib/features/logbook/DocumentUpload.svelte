<script>
  import { t } from '../../stores/i18n.svelte.js';
  import { api } from '../../api.js';
  import { successToast, errorToast } from '../../stores/toasts.svelte.js';
  import Button from '../../components/Button.svelte';
  import Spinner from '../../components/Spinner.svelte';
  import { Upload, X, FileText } from 'lucide-svelte';

  let { bucketId, onclose = () => {}, onupload = () => {} } = $props();

  let dragOver = $state(false);
  let selectedFile = $state(null);
  let title = $state('');
  let uploading = $state(false);

  const acceptedTypes = '.pdf,.docx,.pptx,.xlsx,.txt,.md,.html,.htm,.png,.jpg,.jpeg,.gif,.webp';

  function handleDragOver(e) {
    e.preventDefault();
    dragOver = true;
  }

  function handleDragLeave() {
    dragOver = false;
  }

  function handleDrop(e) {
    e.preventDefault();
    dragOver = false;
    const files = e.dataTransfer?.files;
    if (files?.length > 0) {
      selectedFile = files[0];
      if (!title) {
        title = files[0].name.replace(/\.[^/.]+$/, '');
      }
    }
  }

  function handleFileSelect(e) {
    const files = e.target.files;
    if (files?.length > 0) {
      selectedFile = files[0];
      if (!title) {
        title = files[0].name.replace(/\.[^/.]+$/, '');
      }
    }
  }

  async function uploadFile() {
    if (!selectedFile || !bucketId || uploading) return;
    uploading = true;
    try {
      const formData = new FormData();
      formData.append('file', selectedFile);
      if (title.trim()) {
        formData.append('title', title.trim());
      }
      await api.logbook.uploadDocument(bucketId, formData);
      successToast(t('logbook.uploadSuccess'));
      onupload();
    } catch (error) {
      errorToast(error.message || String(error));
    } finally {
      uploading = false;
    }
  }

  function formatFileSize(bytes) {
    if (!bytes) return '';
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  }
</script>

<!-- Modal overlay -->
<div
  class="fixed inset-0 z-50 flex items-center justify-center"
  style="background-color: rgba(0, 0, 0, 0.4); backdrop-filter: blur(2px);"
  onclick={(e) => { if (e.target === e.currentTarget && !uploading) onclose(); }}
  onkeydown={(e) => { if (e.key === 'Escape' && !uploading) onclose(); }}
  role="dialog"
  aria-modal="true"
  tabindex="-1"
>
  <div
    class="w-full max-w-lg rounded-xl border shadow-xl"
    style="background-color: var(--ds-surface-overlay); border-color: var(--ds-border);"
  >
    <!-- Header -->
    <div class="px-6 py-4 border-b flex justify-between items-center" style="border-color: var(--ds-border);">
      <h3 class="text-lg font-semibold" style="color: var(--ds-text);">
        {t('logbook.uploadDocument')}
      </h3>
      <button
        onclick={onclose}
        class="p-1 rounded hover:bg-gray-100 cursor-pointer"
        disabled={uploading}
      >
        <X class="w-5 h-5" style="color: var(--ds-text-subtle);" />
      </button>
    </div>

    <!-- Content -->
    <div class="px-6 py-4 space-y-4">
      <!-- Dropzone -->
      <div
        class="border-2 border-dashed rounded-xl p-8 text-center transition-colors cursor-pointer"
        style="border-color: {dragOver ? 'var(--ds-interactive)' : 'var(--ds-border)'}; background-color: {dragOver ? 'var(--ds-surface-selected)' : 'var(--ds-surface)'};"
        ondragover={handleDragOver}
        ondragleave={handleDragLeave}
        ondrop={handleDrop}
        onclick={() => document.getElementById('file-input')?.click()}
        onkeydown={(e) => { if (e.key === 'Enter') document.getElementById('file-input')?.click(); }}
        role="button"
        tabindex="0"
      >
        {#if selectedFile}
          <div class="flex items-center justify-center gap-3">
            <FileText class="w-8 h-8" style="color: var(--ds-interactive);" />
            <div class="text-left">
              <p class="text-sm font-medium" style="color: var(--ds-text);">{selectedFile.name}</p>
              <p class="text-xs" style="color: var(--ds-text-subtle);">{formatFileSize(selectedFile.size)}</p>
            </div>
          </div>
        {:else}
          <Upload class="w-8 h-8 mx-auto mb-3" style="color: var(--ds-text-subtle);" />
          <p class="text-sm font-medium mb-1" style="color: var(--ds-text);">
            {t('logbook.dropzoneTitle')}
          </p>
          <p class="text-xs" style="color: var(--ds-text-subtle);">
            {t('logbook.dropzoneDescription')}
          </p>
        {/if}
      </div>

      <input
        id="file-input"
        type="file"
        accept={acceptedTypes}
        onchange={handleFileSelect}
        class="hidden"
      />

      <!-- Title field -->
      <div>
        <label for="doc-title" class="block text-sm font-medium mb-1" style="color: var(--ds-text);">
          {t('logbook.documentTitle')}
        </label>
        <input
          id="doc-title"
          type="text"
          bind:value={title}
          class="w-full px-4 py-3 rounded border transition-all duration-200 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-opacity-50"
          style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
          placeholder={t('logbook.documentTitlePlaceholder')}
        />
      </div>
    </div>

    <!-- Footer -->
    <div class="px-6 py-4 border-t flex justify-end gap-3" style="border-color: var(--ds-border);">
      <Button variant="default" onclick={onclose} disabled={uploading}>
        {t('common.cancel')}
      </Button>
      <Button
        variant="primary"
        icon={uploading ? null : Upload}
        onclick={uploadFile}
        disabled={!selectedFile || uploading}
      >
        {#if uploading}
          <Spinner size="sm" class="mr-2" />
          {t('logbook.uploading')}
        {:else}
          {t('logbook.uploadDocument')}
        {/if}
      </Button>
    </div>
  </div>
</div>
