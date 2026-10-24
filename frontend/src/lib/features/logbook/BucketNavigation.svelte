<script>
  import { navigate } from '../../router.js';
  import { logbookStore } from '../../stores/logbook.svelte.js';
  import { permissionStore } from '../../stores/permissions.svelte.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { logbook } from '../../api/logbook.js';
  import { successToast, errorToast } from '../../stores/toasts.svelte.js';
  import Button from '../../components/Button.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';
  import Plus from '@tabler/icons-svelte-runes/icons/plus';
  import FolderOpen from '@tabler/icons-svelte-runes/icons/folder-open';
  import NavigationSidebar from '../../layout/NavigationSidebar.svelte';
	import TextareaField from '../../components/TextareaField.svelte';
	import TextField from '../../components/TextField.svelte';

  let { activeBucketId = null } = $props();

  let showCreateForm = $state(false);
  let formData = $state({ name: '', description: '' });

  function bucketHref(bucketId) {
    return bucketId === null ? '/logbook' : `/logbook/bucket/${bucketId}`;
  }

  async function createBucket() {
    try {
      await logbook.createBucket(formData);
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

{#snippet sidebarFooter()}
  {#if $permissionStore.isSystemAdmin}
    <div class="px-6 pb-6 pt-4 border-t" style="border-color: var(--ds-border);">
      <!-- shortcut-guard-exempt: contextual sidebar action; the modal owns its submit behavior -->
      <Button
        dataTestid="logbook-create-bucket"
        variant="default"
        icon={Plus}
        onclick={() => showCreateForm = true}
        class="w-full justify-center"
      >
        {t('logbook.createBucket')}
      </Button>
    </div>
  {/if}
{/snippet}

<!-- Bucket Navigation Sidebar -->
<NavigationSidebar title={t('logbook.title')} description={t('logbook.subtitle')} footer={sidebarFooter}>
  <!-- Navigation -->
  <nav class="flex-1 space-y-1">
    <!-- All Documents -->
    <a
      href={bucketHref(null)}
      class="nav-link w-full text-left cursor-pointer px-3 py-2 rounded-lg text-sm font-medium transition-all flex items-center gap-3 no-underline"
      class:active={isAllActive}
    >
      <div class="w-4 h-4 rounded bg-gradient-to-br from-ds-nav-blue-from to-ds-nav-blue-to flex-shrink-0"></div>
      <span>{t('logbook.allDocuments')}</span>
    </a>

    <!-- Bucket List -->
    {#each logbookStore.buckets as bucket (bucket.id)}
      {@const isBucketActive = activeBucketId === bucket.id}
      <a
        href={bucketHref(bucket.id)}
        data-testid={`logbook-bucket-${bucket.id}`}
        class="nav-link w-full text-left cursor-pointer px-3 py-2 rounded-lg text-sm font-medium transition-all flex items-center gap-3 no-underline"
        class:active={isBucketActive}
        title={bucket.description || bucket.name}
      >
        <FolderOpen class="w-4 h-4 flex-shrink-0" style="color: var(--ds-icon);" />
        <span class="truncate">{bucket.name}</span>
        {#if bucket.document_count > 0}
          <span class="ml-auto text-xs opacity-60">{bucket.document_count}</span>
        {/if}
      </a>
    {/each}
  </nav>

</NavigationSidebar>

<!-- Create Bucket Modal -->
<Modal
  isOpen={showCreateForm}
  onclose={cancelForm}
  onSubmit={createBucket}
  submitDisabled={!formData.name.trim()}
  maxWidth="max-w-md"
>
  {#snippet children(submitHint)}
  <ModalHeader title={t('logbook.createBucket')} showCloseButton={false} />

  <div class="px-6 py-4">
    <form onsubmit={(e) => { e.preventDefault(); createBucket(); }}>
      <div class="space-y-4">
        <div>
          <TextField
            label={t('logbook.bucketName')}
            id="bucket-name"
            required
            dataTestid="logbook-bucket-name"
            type="text"
            placeholder={t('logbook.bucketNamePlaceholder')}
            bind:value={formData.name}
          />
        </div>

        <div>
          <TextareaField
            label={t('logbook.bucketDescription')}
            id="bucket-description"
            rows={3}
            placeholder={t('logbook.bucketDescriptionPlaceholder')}
            bind:value={formData.description}
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
  {/snippet}
</Modal>

<style>
  .nav-link {
    color: var(--ds-text-subtle);
  }

  .nav-link:hover:not(.active) {
    background: var(--ds-background-neutral-hovered);
    color: var(--ds-text);
  }

  .nav-link.active {
    background: var(--ds-surface-selected);
    color: var(--ds-text);
  }
</style>
