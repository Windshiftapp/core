<script>
  import { onMount } from 'svelte';
  import { navigate } from '../../router.js';
  import { logbookStore } from '../../stores/logbook.svelte.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { api } from '../../api.js';
  import { successToast, errorToast } from '../../stores/toasts.svelte.js';
  import { confirm } from '../../composables/useConfirm.js';
  import Button from '../../components/Button.svelte';
  import Spinner from '../../components/Spinner.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import Progress from '../../components/Progress.svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import SearchInput from '../../components/SearchInput.svelte';
  import DocumentUpload from './DocumentUpload.svelte';
  import { computeDocumentHealth } from './healthScore.js';
  import {
    Plus, Upload, FileText, StickyNote, Mail, BookOpen, Trash2, ExternalLink
  } from 'lucide-svelte';
  import PageHeader from '../../layout/PageHeader.svelte';
  import { formatDateShort } from '../../utils/dateFormatter.js';
  import LazyMilkdownEditor from '../../editors/LazyMilkdownEditor.svelte';

  let { activeBucketId = null } = $props();

  let searchQuery = $state('');
  let showUploadModal = $state(false);
  let showNoteModal = $state(false);
  let noteFormData = $state({ title: '', content: '' });

  // Get active bucket info
  let activeBucket = $derived(
    activeBucketId ? logbookStore.buckets.find(b => b.id === activeBucketId) : null
  );

  // Search handler
  let searchTimeout;
  let searchMatchIds = $state(null); // null = no active search, Set = active filter
  function handleSearch(e) {
    clearTimeout(searchTimeout);
    searchTimeout = setTimeout(async () => {
      if (searchQuery.trim()) {
        try {
          const params = {};
          if (activeBucketId) params.bucket_id = activeBucketId;
          const result = await api.logbook.keywordSearch(searchQuery, params);
          const results = result?.data ?? result;
          if (Array.isArray(results)) {
            searchMatchIds = new Set(results.map(r => r.document_id));
          }
        } catch (error) {
          console.error('Search failed:', error);
        }
      } else {
        searchMatchIds = null;
      }
    }, 300);
  }

  let filteredDocuments = $derived(
    searchMatchIds ? logbookStore.documents.filter(d => searchMatchIds.has(d.id)) : logbookStore.documents
  );

  // Poll while any visible documents are still processing
  let hasProcessingDocs = $derived(
    filteredDocuments.some(d => d.status === 'pending' || d.status === 'processing')
  );

  $effect(() => {
    if (!hasProcessingDocs) return;
    const interval = setInterval(() => {
      if (activeBucketId) {
        logbookStore.loadDocuments(activeBucketId, {}, { silent: true });
      } else {
        logbookStore.loadAllDocuments({}, { silent: true });
      }
    }, 3000);
    return () => clearInterval(interval);
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

  function getStatusColor(status) {
    switch (status) {
      case 'pending': return 'grey';
      case 'processing': return 'blue';
      case 'ready': return 'green';
      case 'error': return 'red';
      default: return 'grey';
    }
  }

  async function createNote() {
    if (!activeBucketId || !noteFormData.title.trim()) return;
    try {
      await api.logbook.createNote(activeBucketId, noteFormData);
      successToast(t('logbook.noteCreated'));
      showNoteModal = false;
      noteFormData = { title: '', content: '' };
      await logbookStore.loadDocuments(activeBucketId);
    } catch (error) {
      errorToast(error.message || String(error));
    }
  }

  async function deleteDocument(e, docId) {
    e.stopPropagation();
    const confirmed = await confirm({
      title: t('logbook.delete'),
      message: t('logbook.confirmDelete'),
      confirmText: t('logbook.delete'),
      variant: 'danger',
    });
    if (!confirmed) return;
    try {
      await api.logbook.archiveDocument(docId);
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
    e.stopPropagation();
    window.open(api.logbook.getDocumentFileUrl(docId), '_blank');
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
        <!-- svelte-ignore a11y_click_events_have_key_events -->
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div
          onclick={() => navigate(`/logbook/documents/${doc.id}`)}
          class="group text-left rounded-xl border transition-all duration-200 hover:shadow-md cursor-pointer overflow-hidden flex flex-col"
          style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
          onmouseenter={(e) => e.currentTarget.style.borderColor = 'var(--ds-border-focused)'}
          onmouseleave={(e) => e.currentTarget.style.borderColor = 'var(--ds-border)'}
        >
          <div class="relative aspect-[210/297] w-full overflow-hidden" style="background-color: var(--ds-surface);">
            {#if doc.status === 'pending' || doc.status === 'processing'}
              <!-- Processing shimmer overlay -->
              <div class="w-full h-full flex items-center justify-center doc-shimmer">
                <Spinner />
              </div>
            {:else if doc.has_thumbnail}
              <img
                src={api.logbook.getDocumentThumbnailUrl(doc.id)}
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
            <div class="absolute top-2 right-2 flex gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
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
              <h3 class="text-sm font-medium truncate" style="color: var(--ds-text);">
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
              <Lozenge color={getStatusColor(doc.status)} text={t(`logbook.status.${doc.status}`)} />
              <span class="text-xs" style="color: var(--ds-text-subtlest);">
                {formatDateShort(doc.created_at)}
              </span>
            </div>
          </div>
        </div>
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
{#if showNoteModal}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center"
    style="background-color: rgba(0, 0, 0, 0.4); backdrop-filter: blur(2px);"
    onclick={(e) => { if (e.target === e.currentTarget) { showNoteModal = false; } }}
    onkeydown={(e) => { if (e.key === 'Escape') showNoteModal = false; }}
    role="dialog"
    aria-modal="true"
    tabindex="-1"
  >
    <div
      class="w-full max-w-2xl rounded-xl border shadow-xl"
      style="background-color: var(--ds-surface-overlay); border-color: var(--ds-border);"
    >
      <!-- Header -->
      <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
        <h3 class="text-lg font-semibold" style="color: var(--ds-text);">
          {t('logbook.newNote')}
        </h3>
      </div>

      <!-- Content -->
      <div class="px-6 py-4 space-y-4">
        <div>
          <label for="note-title" class="block text-sm font-medium mb-1" style="color: var(--ds-text);">
            {t('logbook.noteTitle')} <span class="text-red-500">*</span>
          </label>
          <input
            id="note-title"
            type="text"
            bind:value={noteFormData.title}
            class="w-full px-4 py-3 rounded border transition-all duration-200 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-opacity-50"
            style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
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
        <Button
          variant="primary"
          onclick={createNote}
          disabled={!noteFormData.title.trim()}
        >
          {t('common.create')}
        </Button>
      </div>
    </div>
  </div>
{/if}

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
</style>
