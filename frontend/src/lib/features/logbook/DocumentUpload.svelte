<script>
  import { t } from '../../stores/i18n.svelte.js';
  import { logbook } from '../../api/logbook.js';
  import { successToast, errorToast } from '../../stores/toasts.svelte.js';
  import Button from '../../components/Button.svelte';
  import FileInput from '../../components/FileInput.svelte';
  import Input from '../../components/Input.svelte';
  import FormField from '../../components/FormField.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import Upload from '@tabler/icons-svelte-runes/icons/upload';
  import X from '@tabler/icons-svelte-runes/icons/x';
  import FileText from '@tabler/icons-svelte-runes/icons/file-text';

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
      await logbook.uploadDocument(bucketId, formData);
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
<Modal isOpen={true} preventClose={uploading} maxWidth="max-w-lg" onclose={onclose}>
    <ModalHeader title={t('logbook.uploadDocument')} onClose={onclose} />

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

      <FileInput
        id="file-input"
        accept={acceptedTypes}
        onchange={handleFileSelect}
        class="hidden"
      />

      <!-- Title field -->
      <FormField label={t('logbook.documentTitle')} id="doc-title">
        <Input
          id="doc-title"
          type="text"
          bind:value={title}
          placeholder={t('logbook.documentTitlePlaceholder')}
        />
      </FormField>
    </div>

    <!-- Footer -->
    <div class="px-6 py-4 border-t flex justify-end gap-3" style="border-color: var(--ds-border);">
      <Button variant="default" onclick={onclose} disabled={uploading}>
        {t('common.cancel')}
      </Button>
      <Button
        variant="primary"
        icon={Upload}
        onclick={uploadFile}
        loading={uploading}
        disabled={!selectedFile}
      >
        {uploading ? t('logbook.uploading') : t('logbook.uploadDocument')}
      </Button>
    </div>
</Modal>
