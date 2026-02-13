<script>
  import { onMount } from 'svelte';
  import { navigate, currentRoute } from '../../router.js';
  import { logbookStore } from '../../stores/logbook.svelte.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { api } from '../../api.js';
  import { successToast, errorToast } from '../../stores/toasts.svelte.js';
  import { confirm } from '../../composables/useConfirm.js';
  import Button from '../../components/Button.svelte';
  import Spinner from '../../components/Spinner.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import Progress from '../../components/Progress.svelte';
  import { computeDocumentHealth } from './healthScore.js';
  import {
    ArrowLeft, Save, FileText, StickyNote, Mail, Clock, Hash, Eye, Trash2, ExternalLink, CheckCircle, Loader
  } from 'lucide-svelte';
  import { formatDateShort } from '../../utils/dateFormatter.js';
  import LazyMilkdownEditor from '../../editors/LazyMilkdownEditor.svelte';

  let { documentId = null } = $props();

  // Resolve documentId from prop or route
  let resolvedDocumentId = $derived(documentId || $currentRoute.params?.documentId);

  let activeTab = $state('article');
  let articleContent = $state('');
  let titleValue = $state('');
  let saving = $state(false);
  let hasChanges = $state(false);

  onMount(async () => {
    if (resolvedDocumentId) {
      await logbookStore.loadDocument(resolvedDocumentId);
      if (logbookStore.activeDocument) {
        articleContent = logbookStore.activeDocument.article || '';
        titleValue = logbookStore.activeDocument.title || '';
      }
    }

    return () => {
      logbookStore.clearActiveDocument();
    };
  });

  // Sync when document loads
  $effect(() => {
    if (logbookStore.activeDocument) {
      articleContent = logbookStore.activeDocument.article || '';
      titleValue = logbookStore.activeDocument.title || '';
      hasChanges = false;
    }
  });

  // Track article content changes from the editor
  $effect(() => {
    if (logbookStore.activeDocument) {
      const original = logbookStore.activeDocument.article || '';
      if (articleContent !== original) {
        hasChanges = true;
      }
    }
  });

  function handleTitleInput() {
    hasChanges = true;
  }

  async function saveDocument() {
    if (!resolvedDocumentId || saving) return;
    saving = true;
    try {
      const payload = { title: titleValue };
      if (isNote) {
        payload.article = articleContent;
      } else {
        payload.content = articleContent;
      }
      await api.logbook.updateDocument(resolvedDocumentId, payload);
      successToast(t('logbook.saved'));
      hasChanges = false;
      // Reload to get updated metadata
      await logbookStore.loadDocument(resolvedDocumentId);
    } catch (error) {
      errorToast(error.message || String(error));
    } finally {
      saving = false;
    }
  }

  function goBack() {
    const doc = logbookStore.activeDocument;
    if (doc?.bucket_id) {
      navigate(`/logbook/bucket/${doc.bucket_id}`);
    } else {
      navigate('/logbook');
    }
  }

  function getSourceIcon(sourceType) {
    switch (sourceType) {
      case 'upload': return FileText;
      case 'note': return StickyNote;
      case 'email': return Mail;
      default: return FileText;
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

  let deleting = $state(false);

  async function deleteDocument() {
    if (!resolvedDocumentId || deleting) return;
    const confirmed = await confirm({
      title: t('logbook.delete'),
      message: t('logbook.confirmDelete'),
      confirmText: t('logbook.delete'),
      variant: 'danger',
    });
    if (!confirmed) return;
    deleting = true;
    try {
      await api.logbook.archiveDocument(resolvedDocumentId);
      successToast(t('logbook.documentDeleted'));
      goBack();
    } catch (error) {
      errorToast(error.message || String(error));
    } finally {
      deleting = false;
    }
  }

  function viewOriginal() {
    if (!resolvedDocumentId) return;
    window.open(api.logbook.getDocumentFileUrl(resolvedDocumentId), '_blank');
  }

  let doc = $derived(logbookStore.activeDocument);
  let isNote = $derived(doc?.source_type === 'note');
  let health = $derived(doc ? computeDocumentHealth(doc) : null);
  let isProcessing = $derived(doc?.status === 'pending' || doc?.status === 'processing');

  // Poll while document is still processing
  $effect(() => {
    if (!isProcessing || !resolvedDocumentId) return;
    const interval = setInterval(() => {
      logbookStore.loadDocument(resolvedDocumentId, { silent: true });
    }, 3000);
    return () => clearInterval(interval);
  });
</script>

<div class="max-w-5xl mx-auto p-6">
  {#if logbookStore.activeDocumentLoading}
    <div class="flex items-center justify-center h-64">
      <Spinner />
    </div>
  {:else if doc}
    <!-- Back + Header -->
    <div class="mb-6">
      <button
        onclick={goBack}
        class="flex items-center gap-2 text-sm mb-4 cursor-pointer transition-colors"
        style="color: var(--ds-text-subtle);"
        onmouseenter={(e) => e.currentTarget.style.color = 'var(--ds-text)'}
        onmouseleave={(e) => e.currentTarget.style.color = 'var(--ds-text-subtle)'}
      >
        <ArrowLeft class="w-4 h-4" />
        {t('logbook.back')}
      </button>

      <div class="flex items-start justify-between gap-4">
        <div class="flex-1">
          {#if isNote}
            <input
              type="text"
              bind:value={titleValue}
              oninput={handleTitleInput}
              class="text-xl font-semibold w-full bg-transparent border-none outline-none focus:ring-0 p-0"
              style="color: var(--ds-text);"
              placeholder="Untitled"
            />
          {:else}
            <h1 class="text-xl font-semibold" style="color: var(--ds-text);">{doc.title}</h1>
          {/if}

          <div class="flex items-center gap-3 mt-2 flex-wrap">
            <Lozenge color={getStatusColor(doc.status)} text={t(`logbook.status.${doc.status}`)} />
            <span class="text-sm" style="color: var(--ds-text-subtle);">
              {t(`logbook.sourceType.${doc.source_type}`)}
            </span>
            {#if doc.content_type}
              <Lozenge color="blue" text={t(`logbook.contentType.${doc.content_type}`)} />
            {/if}
            {#if doc.author}
              <span class="text-sm" style="color: var(--ds-text-subtle);">{doc.author}</span>
            {/if}
            <span class="text-sm" style="color: var(--ds-text-subtle);">
              {formatDateShort(doc.created_at)}
            </span>
            {#if doc.chunk_count > 0}
              <span class="text-sm" style="color: var(--ds-text-subtlest);">
                {doc.chunk_count} chunks
              </span>
            {/if}
          </div>

          {#if health}
            <div class="mt-3">
              <Progress value={health.score} size="sm" color={health.color} showLabel label={t('logbook.health')} />
            </div>
          {/if}
        </div>

        <div class="flex items-center gap-2">
          {#if doc.source_type === 'upload'}
            <Button
              variant="default"
              icon={ExternalLink}
              onclick={viewOriginal}
            >
              {t('logbook.viewOriginal')}
            </Button>
          {/if}
          <Button
            variant="danger"
            icon={Trash2}
            onclick={deleteDocument}
            disabled={deleting}
          >
            {t('logbook.delete')}
          </Button>
          <Button
            variant="primary"
            icon={Save}
            onclick={saveDocument}
            disabled={!hasChanges || saving}
          >
            {saving ? t('logbook.saving') : t('logbook.save')}
          </Button>
        </div>
      </div>
    </div>

    <!-- Processing banner -->
    {#if isProcessing}
      <div
        class="flex items-center gap-3 px-4 py-3 rounded-lg mb-6"
        style="background-color: var(--ds-background-information, var(--ds-surface-raised)); border: 1px solid var(--ds-border-information, var(--ds-border));"
      >
        <Loader class="w-4 h-4 animate-spin" style="color: var(--ds-icon-information, var(--ds-icon));" />
        <span class="text-sm" style="color: var(--ds-text-information, var(--ds-text));">
          {t('logbook.processingMessage')}
        </span>
      </div>
    {/if}

    <!-- Tab Navigation -->
    <div class="flex border-b mb-6 gap-1" style="border-color: var(--ds-border);">
      <button
        onclick={() => activeTab = 'article'}
        class="px-4 py-2 text-sm font-medium transition-colors cursor-pointer -mb-px"
        style={activeTab === 'article'
          ? 'color: var(--ds-text); border-bottom: 2px solid var(--ds-interactive);'
          : 'color: var(--ds-text-subtle); border-bottom: 2px solid transparent;'}
      >
        {t('logbook.article')}
      </button>
      {#if !isNote}
        <button
          onclick={() => activeTab = 'raw'}
          class="px-4 py-2 text-sm font-medium transition-colors cursor-pointer -mb-px"
          style={activeTab === 'raw'
            ? 'color: var(--ds-text); border-bottom: 2px solid var(--ds-interactive);'
            : 'color: var(--ds-text-subtle); border-bottom: 2px solid transparent;'}
        >
          {t('logbook.rawContent')}
        </button>
      {/if}
      <button
        onclick={() => activeTab = 'info'}
        class="px-4 py-2 text-sm font-medium transition-colors cursor-pointer -mb-px"
        style={activeTab === 'info'
          ? 'color: var(--ds-text); border-bottom: 2px solid var(--ds-interactive);'
          : 'color: var(--ds-text-subtle); border-bottom: 2px solid transparent;'}
      >
        {t('logbook.info')}
      </button>
    </div>

    <!-- Tab Content -->
    {#if activeTab === 'article'}
      <div class="rounded-xl border p-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <LazyMilkdownEditor
          bind:content={articleContent}
          placeholder={isNote ? t('logbook.noteContentPlaceholder') : 'Article content...'}
          showToolbar={true}
          customUploadFn={(formData) => api.logbook.uploadAttachment(resolvedDocumentId, formData)}
          downloadUrlBase="/api/logbook/attachments"
        />
      </div>
    {:else if activeTab === 'raw'}
      <div class="rounded-xl border p-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        {#if doc.raw_content}
          <pre class="whitespace-pre-wrap font-mono text-sm leading-relaxed" style="color: var(--ds-text);">{doc.raw_content}</pre>
        {:else}
          <p class="text-sm italic" style="color: var(--ds-text-subtle);">No raw content available.</p>
        {/if}
      </div>
    {:else if activeTab === 'info'}
      <div class="rounded-xl border divide-y" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border); --tw-divide-color: var(--ds-border);">
        {#each [
          { icon: FileText, label: t('logbook.mimeType'), value: doc.mime_type || '-' },
          { icon: Hash, label: t('logbook.contentHash'), value: doc.content_hash ? doc.content_hash.substring(0, 16) + '...' : '-' },
          { icon: Eye, label: t('logbook.retrievalCount'), value: String(doc.retrieval_count || 0) },
          { icon: Hash, label: t('logbook.chunkCount'), value: String(doc.chunk_count || 0) },
          { icon: Clock, label: t('logbook.createdAt'), value: formatDateShort(doc.created_at) || '-' },
          { icon: Clock, label: t('logbook.updatedAt'), value: formatDateShort(doc.updated_at) || '-' },
          { icon: CheckCircle, label: t('logbook.reviewedAt'), value: doc.reviewed_at ? formatDateShort(doc.reviewed_at) : '-' },
        ] as item (item.label)}
          {@const IconComp = item.icon}
          <div class="flex items-center px-6 py-3">
            <div class="flex items-center gap-3 w-48 flex-shrink-0">
              <IconComp class="w-4 h-4" style="color: var(--ds-icon);" />
              <span class="text-sm font-medium" style="color: var(--ds-text-subtle);">{item.label}</span>
            </div>
            <span class="text-sm" style="color: var(--ds-text);">{item.value}</span>
          </div>
        {/each}
      </div>
    {/if}
  {:else}
    <div class="flex items-center justify-center h-64">
      <p class="text-sm" style="color: var(--ds-text-subtle);">Document not found.</p>
    </div>
  {/if}
</div>
