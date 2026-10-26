<script>
  import { onDestroy, untrack } from 'svelte';
  import { navigate } from '../../router.js';
  import { logbookStore } from '../../stores/logbook.svelte.js';
  import { canRunBackgroundSync, onBackgroundSyncAvailable } from '../../utils/backgroundSync.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { logbook } from '../../api/logbook.js';
  import { successToast, errorToast } from '../../stores/toasts.svelte.js';
  import { confirm } from '../../composables/useConfirm.js';
  import Button from '../../components/Button.svelte';
  import Spinner from '../../components/Spinner.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import Progress from '../../components/Progress.svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import SearchInput from '../../components/SearchInput.svelte';
  import Input from '../../components/Input.svelte';
  import DocumentUpload from './DocumentUpload.svelte';
  import { computeDocumentHealth } from './healthScore.js';
  import { getDocumentStatusColor } from '../../utils/statusMaps.js';
  import Plus from '@tabler/icons-svelte-runes/icons/plus';
  import Upload from '@tabler/icons-svelte-runes/icons/upload';
  import FileText from '@tabler/icons-svelte-runes/icons/file-text';
  import StickyNote from '@tabler/icons-svelte-runes/icons/note';
  import Mail from '@tabler/icons-svelte-runes/icons/mail';
  import BookOpen from '@tabler/icons-svelte-runes/icons/book';
  import Trash2 from '@tabler/icons-svelte-runes/icons/trash';
  import ExternalLink from '@tabler/icons-svelte-runes/icons/external-link';
  import PageHeader from '../../layout/PageHeader.svelte';
  import { formatDateShort } from '../../utils/dateFormatter.js';
  import LazyMilkdownEditor from '../../editors/LazyMilkdownEditor.svelte';
  import RunActionMenu from './RunActionMenu.svelte';

  let { activeBucketId = null } = $props();

  let searchQuery = $state('');
  let showUploadModal = $state(false);
  let showNoteModal = $state(false);
  let noteFormData = $state({ title: '', content: '' });

  // Get active bucket info
  let activeBucket = $derived(
    activeBucketId ? logbookStore.buckets.find(b => b.id === activeBucketId) : null
  );

  // Search handler. A monotonic version plus the bucket captured at request
  // time keep a superseded query from replacing the newest results.
  let searchTimeout;
  let searchVersion = 0;
  let searchMatchIds = $state(null); // null = no active search, Set = active filter

  function runSearch() {
    const version = ++searchVersion;
    const query = searchQuery.trim();
    const bucketId = activeBucketId;
    if (!query) {
      searchMatchIds = null;
      return;
    }
    searchTimeout = setTimeout(async () => {
      try {
        const params = {};
        if (bucketId) params.bucket_id = bucketId;
        const result = await logbook.keywordSearch(query, params);
        if (version !== searchVersion) return;
        const results = result?.data ?? result;
        if (Array.isArray(results)) {
          searchMatchIds = new Set(results.map(r => r.document_id));
        }
      } catch (error) {
        if (version !== searchVersion) return;
        console.error('Search failed:', error);
      }
    }, 300);
  }

  function handleSearch() {
    clearTimeout(searchTimeout);
    runSearch();
  }

  // A bucket change invalidates any in-flight search and restarts it for the
  // new scope so results cannot carry over from the previous bucket.
  $effect(() => {
    activeBucketId;
    untrack(() => {
      clearTimeout(searchTimeout);
      searchMatchIds = null;
      if (searchQuery.trim()) runSearch();
    });
  });

  onDestroy(() => {
    clearTimeout(searchTimeout);
    searchVersion += 1;
  });

  let filteredDocuments = $derived(
    searchMatchIds ? logbookStore.documents.filter(d => searchMatchIds.has(d.id)) : logbookStore.documents
  );

  // Poll while any visible documents are still processing. The next refresh is
  // scheduled only after the previous one settles so requests never overlap.
  let hasProcessingDocs = $derived(
    filteredDocuments.some(d => d.status === 'pending' || d.status === 'processing')
  );

  $effect(() => {
    const bucketId = activeBucketId;
    if (!hasProcessingDocs) return;

    let cancelled = false;
    let timer = null;

    const refresh = async () => {
      if (cancelled) return;
      if (timer) {
        clearTimeout(timer);
        timer = null;
      }
      if (canRunBackgroundSync()) {
        if (bucketId) {
          await logbookStore.loadDocuments(bucketId, {}, { silent: true });
        } else {
          await logbookStore.loadAllDocuments({}, { silent: true });
        }
      }
      if (cancelled) return;
      timer = setTimeout(refresh, 3000);
    };

    timer = setTimeout(refresh, 3000);
    const stopRecovery = onBackgroundSyncAvailable(() => {
      if (!cancelled) void refresh();
    });

    return () => {
      cancelled = true;
      if (timer) clearTimeout(timer);
      stopRecovery();
    };
  });

  function getSourceIcon(sourceType) {
    switch (sourceType) {
      case 'upload': return FileText;
      case 'note': return StickyNote;
      case 'email': return Mail;
      default: return FileText;
    }
  }

  function getContentTypeColor(contentType) {
    switch (contentType) {
      case 'knowledge': return 'blue';
      case 'record': return 'grey';
      case 'correspondence': return 'purple';
      default: return 'grey';
    }
  }

  async function createNote() {
    if (!activeBucketId || !noteFormData.title.trim()) return;
    try {
      await logbook.createNote(activeBucketId, noteFormData);
      successToast(t('logbook.noteCreated'));
      showNoteModal = false;
      noteFormData = { title: '', content: '' };
      await logbookStore.loadDocuments(activeBucketId);
    } catch (error) {
      errorToast(error.message || String(error));
    }
  }

  async function deleteDocument(e, docId) {
    e.preventDefault();
    e.stopPropagation();
    const confirmed = await confirm({
      title: t('logbook.delete'),
      message: t('logbook.confirmDelete'),
      confirmText: t('logbook.delete'),
      variant: 'danger',
    });
    if (!confirmed) return;
    try {
      await logbook.archiveDocument(docId);
      successToast(t('logbook.documentDeleted'));
      if (activeBucketId) {
        await logbookStore.loadDocuments(activeBucketId);
      } else {
        await logbookStore.loadAllDocuments();
      }
    } catch (error) {
      errorToast(error.message || String(error));
    }
  }

  function openFile(e, docId) {
    e.preventDefault();
    e.stopPropagation();
    window.open(logbook.getDocumentFileUrl(docId), '_blank');
  }

  function handleUploadComplete() {
    showUploadModal = false;
    if (activeBucketId) {
      logbookStore.loadDocuments(activeBucketId);
    } else {
      logbookStore.loadAllDocuments();
    }
  }
</script>

<div class="p-6">
  <!-- Header -->
  <PageHeader
    title={activeBucket ? activeBucket.name : t('logbook.allDocuments')}
    subtitle="{logbookStore.totalDocuments} document{logbookStore.totalDocuments !== 1 ? 's' : ''}{activeBucket?.description ? ` · ${activeBucket.description}` : ''}"
  >
    {#snippet actions()}
      {#if activeBucketId}
        <div class="flex items-center gap-2">
          <Button
            dataTestid="logbook-new-note"
            variant="default"
            icon={StickyNote}
            onclick={() => { showNoteModal = true; }}
          >
            {t('logbook.newNote')}
          </Button>
          <Button
            variant="primary"
            icon={Upload}
            onclick={() => { showUploadModal = true; }}
          >
            {t('logbook.uploadDocument')}
          </Button>
        </div>
      {/if}
    {/snippet}
  </PageHeader>

  <!-- Search -->
  <div class="mb-6">
    <SearchInput
      bind:value={searchQuery}
      dataTestid="logbook-search"
      placeholder={t('logbook.search')}
      on_input={handleSearch}
    />
  </div>

  <!-- Document Grid -->
  {#if logbookStore.documentsLoading}
    <div class="flex items-center justify-center h-48">
      <Spinner />
    </div>
  {:else if filteredDocuments.length === 0}
    <EmptyState
      icon={BookOpen}
      title={t('logbook.noDocuments')}
      description={activeBucketId ? t('logbook.noDocumentsDescription') : t('logbook.noDocumentsAllDescription')}
    />
  {:else}
    <div class="grid gap-4 mb-8" style="grid-template-columns: repeat(auto-fill, minmax(150px, 200px));">
      {#each filteredDocuments as doc (doc.id)}
        {@const SourceIcon = getSourceIcon(doc.source_type)}
        {@const health = computeDocumentHealth(doc)}
        <a
          href={`/logbook/documents/${doc.id}`}
          data-testid={`logbook-document-${doc.id}`}
          class="doc-card group text-left rounded-xl border transition-all duration-200 hover:shadow-md cursor-pointer overflow-hidden flex flex-col no-underline"
          style="background-color: var(--ds-surface-raised); color: inherit;"
        >
          <div class="relative aspect-[210/297] w-full overflow-hidden" style="background-color: var(--ds-surface);">
            {#if doc.status === 'pending' || doc.status === 'processing'}
              <!-- Processing shimmer overlay -->
              <div class="w-full h-full flex items-center justify-center doc-shimmer">
                <Spinner />
              </div>
            {:else if doc.has_thumbnail}
              <img
                src={logbook.getDocumentThumbnailUrl(doc.id)}
                alt=""
                class="w-full h-full object-contain"
                loading="lazy"
              />
            {:else}
              <div class="w-full h-full flex items-center justify-center">
                <SourceIcon class="w-10 h-10" style="color: var(--ds-icon-subtle);" />
              </div>
            {/if}

            <!-- Content type lozenge -->
            {#if doc.content_type}
              <div class="absolute top-2 left-2">
                <Lozenge size="sm" color={getContentTypeColor(doc.content_type)} text={t(`logbook.contentType.${doc.content_type}`)} />
              </div>
            {/if}

            <!-- Hover action buttons -->
            <div class="absolute top-2 right-2 flex gap-1 opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 pointer-coarse:opacity-100 transition-opacity">
              {#if doc.source_type === 'upload'}
                <button
                  onclick={(e) => openFile(e, doc.id)}
                  class="p-1.5 rounded-lg shadow-sm border transition-colors hover:bg-opacity-90"
                  style="background-color: var(--ds-surface-overlay); border-color: var(--ds-border);"
                  title={t('logbook.viewOriginal')}
                >
                  <ExternalLink class="w-3.5 h-3.5" style="color: var(--ds-text-subtle);" />
                </button>
              {/if}
              {#if doc.status === 'ready' && doc.bucket_id}
                <RunActionMenu bucketId={doc.bucket_id} documentId={doc.id} variant="icon" />
              {/if}
              <button
                onclick={(e) => deleteDocument(e, doc.id)}
                class="p-1.5 rounded-lg shadow-sm border transition-colors hover:bg-opacity-90"
                style="background-color: var(--ds-surface-overlay); border-color: var(--ds-border);"
                title={t('logbook.delete')}
              >
                <Trash2 class="w-3.5 h-3.5" style="color: var(--ds-text-danger, #ef4444);" />
              </button>
            </div>
          </div>

          {#if health}
            <div class="px-3 pt-2">
              <Progress value={health.score} size="sm" color={health.color} />
            </div>
          {/if}

          <div class="p-3 flex-1 flex flex-col justify-between">
            <div class="mb-2">
              <h3 class="text-sm truncate" style="color: var(--ds-text);">
                {doc.title || 'Untitled'}
              </h3>
              <p class="text-xs mt-0.5" style="color: var(--ds-text-subtle);">
                {t(`logbook.sourceType.${doc.source_type}`)}
                {#if !activeBucketId && doc.bucket_name}
                  &middot; {doc.bucket_name}
                {/if}
                {#if doc.author}
                  &middot; {doc.author}
                {/if}
              </p>
            </div>

            <div class="flex items-center justify-between">
              <Lozenge color={getDocumentStatusColor(doc.status)} text={t(`logbook.status.${doc.status}`)} />
              <span class="text-xs" style="color: var(--ds-text-subtlest);">
                {formatDateShort(doc.created_at)}
              </span>
            </div>
          </div>
        </a>
      {/each}
    </div>
  {/if}

</div>

<!-- Upload Modal -->
{#if showUploadModal && activeBucketId}
  <DocumentUpload
    bucketId={activeBucketId}
    onclose={() => showUploadModal = false}
    onupload={handleUploadComplete}
  />
{/if}

<!-- Create Note Modal -->
<Modal bind:isOpen={showNoteModal} maxWidth="max-w-2xl" dataTestid="logbook-note-dialog">
      <!-- Header -->
      <ModalHeader title={t('logbook.newNote')} showCloseButton={false} />

      <!-- Content -->
      <div class="px-6 py-4 space-y-4">
        <div>
          <label for="note-title" class="block text-sm font-medium mb-1" style="color: var(--ds-text);">
            {t('logbook.noteTitle')} <span class="text-ds-text-danger">*</span>
          </label>
          <Input
            id="note-title"
            dataTestid="logbook-note-title"
            type="text"
            bind:value={noteFormData.title}
            placeholder={t('logbook.noteTitlePlaceholder')}
          />
        </div>

        <div>
          <span class="block text-sm font-medium mb-1" style="color: var(--ds-text);">
            {t('logbook.noteContent')}
          </span>
          <div style="min-height: 300px;">
            <LazyMilkdownEditor
              bind:content={noteFormData.content}
              testId="logbook-note-editor"
              placeholder={t('logbook.noteContentPlaceholder')}
              showToolbar={true}
            />
          </div>
        </div>
      </div>

      <!-- Footer -->
      <div class="px-6 py-4 border-t flex justify-end gap-3" style="border-color: var(--ds-border);">
        <Button variant="default" onclick={() => { showNoteModal = false; noteFormData = { title: '', content: '' }; }}>
          {t('common.cancel')}
        </Button>
        <!-- shortcut-guard-exempt: dialog submit action; this is not a global create shortcut -->
        <Button
          dataTestid="logbook-note-create"
          variant="primary"
          onclick={createNote}
          disabled={!noteFormData.title.trim()}
        >
          {t('common.create')}
        </Button>
      </div>
</Modal>

<style>
  .doc-shimmer {
    animation: shimmer-pulse 2s ease-in-out infinite;
    background: linear-gradient(
      110deg,
      var(--ds-surface) 40%,
      var(--ds-surface-raised) 50%,
      var(--ds-surface) 60%
    );
    background-size: 200% 100%;
  }

  @keyframes shimmer-pulse {
    0% { background-position: 200% 0; }
    100% { background-position: -200% 0; }
  }
  .doc-card {
    border-color: var(--ds-border);
  }

  .doc-card:hover {
    border-color: var(--ds-border-focused);
  }
</style>
