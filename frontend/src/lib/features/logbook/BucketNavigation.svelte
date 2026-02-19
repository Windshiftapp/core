<script>
  import { navigate } from '../../router.js';
  import { logbookStore } from '../../stores/logbook.svelte.js';
  import { permissionStore } from '../../stores';
  import { t } from '../../stores/i18n.svelte.js';
  import { api } from '../../api.js';
  import { successToast, errorToast } from '../../stores/toasts.svelte.js';
  import Button from '../../components/Button.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';
  import Label from '../../components/Label.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import { Plus, FolderOpen } from 'lucide-svelte';
  import SidebarHeader from '../../layout/SidebarHeader.svelte';

  let { activeBucketId = null } = $props();

  let showCreateForm = $state(false);
  let formData = $state({ name: '', description: '' });

  function handleBucketClick(bucketId) {
    if (bucketId === null) {
      navigate('/logbook');
    } else {
      navigate(`/logbook/bucket/${bucketId}`);
    }
  }

  async function createBucket() {
    try {
      await api.logbook.createBucket(formData);
      successToast(t('logbook.bucketCreated'));
      showCreateForm = false;
      formData = { name: '', description: '' };
      await logbookStore.loadBuckets();
    } catch (error) {
      errorToast(error.message || String(error));
    }
  }

  function cancelForm() {
    showCreateForm = false;
    formData = { name: '', description: '' };
  }

  let isAllActive = $derived(activeBucketId === null);
</script>

<!-- Bucket Navigation Sidebar -->
<div class="w-64 border-r flex flex-col p-6 flex-shrink-0" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
  <!-- Header -->
  <SidebarHeader title={t('logbook.title')} description={t('logbook.subtitle')} noBorder />

  <!-- Navigation -->
  <nav class="flex-1 space-y-1">
    <!-- All Documents -->
    <button
      onclick={() => handleBucketClick(null)}
      class="w-full text-left cursor-pointer px-3 py-2 rounded-lg text-sm font-medium transition-all flex items-center gap-3"
      style={isAllActive ? 'background: var(--ds-surface-selected); color: var(--ds-text);' : 'color: var(--ds-text-subtle);'}
      onmouseenter={(e) => { if (!isAllActive) e.currentTarget.style.cssText = 'background: var(--ds-surface-hovered); color: var(--ds-text);'; }}
      onmouseleave={(e) => { if (!isAllActive) e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'; }}
    >
      <div class="w-4 h-4 rounded bg-gradient-to-br from-blue-400 to-blue-600 flex-shrink-0"></div>
      <span>{t('logbook.allDocuments')}</span>
    </button>

    <!-- Bucket List -->
    {#each logbookStore.buckets as bucket (bucket.id)}
      {@const isBucketActive = activeBucketId === bucket.id}
      <button
        onclick={() => handleBucketClick(bucket.id)}
        class="w-full text-left cursor-pointer px-3 py-2 rounded-lg text-sm font-medium transition-all flex items-center gap-3"
        style={isBucketActive ? 'background: var(--ds-surface-selected); color: var(--ds-text);' : 'color: var(--ds-text-subtle);'}
        onmouseenter={(e) => { if (!isBucketActive) e.currentTarget.style.cssText = 'background: var(--ds-surface-hovered); color: var(--ds-text);'; }}
        onmouseleave={(e) => { if (!isBucketActive) e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'; }}
        title={bucket.description || bucket.name}
      >
        <FolderOpen class="w-4 h-4 flex-shrink-0" style="color: var(--ds-icon);" />
        <span class="truncate">{bucket.name}</span>
        {#if bucket.document_count > 0}
          <span class="ml-auto text-xs opacity-60">{bucket.document_count}</span>
        {/if}
      </button>
    {/each}
  </nav>

  <!-- Footer - Create Bucket (admin only) -->
  {#if $permissionStore.isSystemAdmin}
    <div class="pt-4 border-t" style="border-color: var(--ds-border);">
      <Button
        variant="default"
        icon={Plus}
        onclick={() => showCreateForm = true}
        class="w-full justify-center"
      >
        {t('logbook.createBucket')}
      </Button>
    </div>
  {/if}
</div>

<!-- Create Bucket Modal -->
<Modal
  isOpen={showCreateForm}
  onclose={cancelForm}
  onSubmit={createBucket}
  submitDisabled={!formData.name.trim()}
  maxWidth="max-w-md"
  let:submitHint
>
  <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
    <h3 class="text-lg font-semibold" style="color: var(--ds-text);">
      {t('logbook.createBucket')}
    </h3>
  </div>

  <div class="px-6 py-4">
    <form onsubmit={(e) => { e.preventDefault(); createBucket(); }}>
      <div class="space-y-4">
        <div>
          <Label for="bucket-name" required class="mb-2">{t('logbook.bucketName')}</Label>
          <input
            id="bucket-name"
            type="text"
            bind:value={formData.name}
            class="w-full px-4 py-3 rounded border transition-all duration-200 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-opacity-50"
            style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
            placeholder={t('logbook.bucketNamePlaceholder')}
            required
          />
        </div>

        <div>
          <Label for="bucket-description" class="mb-2">{t('logbook.bucketDescription')}</Label>
          <Textarea
            id="bucket-description"
            bind:value={formData.description}
            rows={3}
            placeholder={t('logbook.bucketDescriptionPlaceholder')}
          />
        </div>
      </div>
    </form>
  </div>

  <DialogFooter
    onCancel={cancelForm}
    onConfirm={createBucket}
    confirmLabel={t('common.create')}
    disabled={!formData.name.trim()}
    showKeyboardHint={true}
    confirmKeyboardHint={submitHint}
  />
</Modal>
