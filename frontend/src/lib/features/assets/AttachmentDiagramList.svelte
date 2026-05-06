<script>
  import { IconPaperclip, IconPencil, IconTrash, IconPhoto } from '@tabler/icons-svelte-runes';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast } from '../../stores/toasts.svelte.js';
  import { portal } from '../../actions/portal.js';
  import { fade, scale } from 'svelte/transition';

  let {
    attachments = [],
    diagrams = [],
    canDelete = true,
    ondelete,
    oneditdiagram,
    ondeletediagram
  } = $props();

  // Lightbox state. Holds the attachment currently being previewed full-size
  // in an overlay. Set to null to close.
  let lightbox = $state(null);

  function isImage(att) {
    return typeof att?.mime_type === 'string' && att.mime_type.startsWith('image/');
  }

  function isPDF(att) {
    return att?.mime_type === 'application/pdf';
  }

  function previewable(att) {
    return isImage(att) || isPDF(att);
  }

  function handlePreview(att) {
    if (previewable(att)) {
      lightbox = att;
    } else {
      handleDownload(att);
    }
  }

  function closeLightbox() {
    lightbox = null;
  }

  function handleLightboxKeydown(e) {
    if (e.key === 'Escape') closeLightbox();
  }

  function thumbUrl(att) {
    return `/api/attachments/${att.id}/thumbnail`;
  }

  function contentUrl(att) {
    return `/api/attachments/${att.id}/download`;
  }

  function formatFileSize(bytes) {
    if (!bytes) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
  }

  async function handleDownload(attachment) {
    try {
      const downloadUrl = `/api/attachments/${attachment.id}/download`;
      const response = await fetch(downloadUrl);
      if (!response.ok) {
        throw new Error(`Download failed: ${response.statusText}`);
      }

      const blob = await response.blob();
      const blobUrl = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = blobUrl;
      link.download = attachment.original_filename;
      link.style.display = 'none';

      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);

      URL.revokeObjectURL(blobUrl);
    } catch (error) {
      console.error('Download failed:', error);
      errorToast(t('assets.failedToDownload') + ': ' + error.message);
    }
  }

  function handleDelete(attachment) {
    ondelete?.(attachment);
  }

  function handleEditDiagram(diagram) {
    oneditdiagram?.(diagram);
  }

  function handleDeleteDiagram(diagram) {
    ondeletediagram?.(diagram);
  }
</script>

{#if attachments.length > 0 || diagrams.length > 0}
  <div class="space-y-1">
    {#each attachments as attachment}
      <div class="flex items-center gap-2 py-1 px-2 -mx-2 rounded group hover:bg-[var(--ds-background-neutral-hovered)] transition-colors">
        {#if attachment.has_thumbnail}
          <button
            type="button"
            class="flex-shrink-0 rounded overflow-hidden border"
            style="border-color: var(--ds-border); width: 2.5rem; height: 2.5rem; background: var(--ds-surface);"
            onclick={() => handlePreview(attachment)}
            title={t('assets.preview') || 'Preview'}
          >
            <img
              src={thumbUrl(attachment)}
              alt=""
              class="w-full h-full object-cover"
              loading="lazy"
            />
          </button>
        {:else if isPDF(attachment)}
          <button
            type="button"
            class="flex-shrink-0 rounded border flex items-center justify-center text-[0.625rem] font-bold tracking-wider"
            style="border-color: var(--ds-border); width: 2.5rem; height: 2.5rem; background: var(--ds-surface); color: #d63b2a;"
            onclick={() => handlePreview(attachment)}
            title={t('assets.preview') || 'Preview'}
          >
            PDF
          </button>
        {:else if isImage(attachment)}
          <IconPhoto class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
        {:else}
          <IconPaperclip class="w-3.5 h-3.5 flex-shrink-0" style="color: var(--ds-text-subtle);" />
        {/if}
        <button
          onclick={() => previewable(attachment) ? handlePreview(attachment) : handleDownload(attachment)}
          class="flex-1 text-sm truncate hover:underline text-left"
          style="color: var(--ds-text);"
          title={attachment.original_filename}
        >
          {attachment.original_filename}
        </button>
        <span class="text-xs" style="color: var(--ds-text-subtlest);">
          {formatFileSize(attachment.file_size)}
        </span>
        {#if canDelete}
          <button
            class="p-1 rounded opacity-0 group-hover:opacity-100 transition-opacity hover:bg-[var(--ds-background-danger-hovered)]"
            style="color: var(--ds-text-danger);"
            onclick={() => handleDelete(attachment)}
            title={t('common.delete')}
          >
            <IconTrash class="w-3.5 h-3.5" />
          </button>
        {/if}
      </div>
    {/each}
    {#each diagrams as diagram}
      <div class="flex items-center gap-2 py-1 px-2 -mx-2 rounded group hover:bg-[var(--ds-background-neutral-hovered)] transition-colors">
        <IconPencil class="w-3.5 h-3.5 flex-shrink-0" style="color: var(--ds-text-subtle);" />
        <button
          class="flex-1 text-sm truncate text-left hover:underline"
          style="color: var(--ds-text);"
          onclick={() => handleEditDiagram(diagram)}
          title={t('assets.editDiagram')}
        >
          {diagram.name || t('assets.untitledDiagram')}
        </button>
        <span class="text-xs" style="color: var(--ds-text-subtlest);">
          {diagram.type || t('assets.diagram')}
        </span>
        {#if canDelete}
          <button
            class="p-1 rounded opacity-0 group-hover:opacity-100 transition-opacity hover:bg-[var(--ds-background-danger-hovered)]"
            style="color: var(--ds-text-danger);"
            onclick={() => handleDeleteDiagram(diagram)}
            title={t('common.delete')}
          >
            <IconTrash class="w-3.5 h-3.5" />
          </button>
        {/if}
      </div>
    {/each}
  </div>
{/if}

{#if lightbox}
  <!-- Fullscreen lightbox overlay. Backdrop click + Escape both close. PDFs
       render in an iframe; images render at their native size, capped to the
       viewport. -->
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    use:portal
    transition:fade={{ duration: 120 }}
    class="fixed inset-0 z-50 flex items-center justify-center p-6"
    style="background-color: rgba(0,0,0,0.85);"
    onclick={(e) => { if (e.target === e.currentTarget) closeLightbox(); }}
    onkeydown={handleLightboxKeydown}
    role="dialog"
    aria-modal="true"
    tabindex="-1"
  >
    <button
      type="button"
      class="absolute top-4 right-4 text-white/80 hover:text-white text-3xl leading-none"
      onclick={closeLightbox}
      aria-label={t('common.close') || 'Close'}
    >
      ×
    </button>
    <div class="max-w-full max-h-full" transition:scale={{ duration: 150, start: 0.97 }}>
      {#if isImage(lightbox)}
        <img
          src={contentUrl(lightbox)}
          alt={lightbox.original_filename}
          class="block max-w-[95vw] max-h-[90vh] object-contain rounded shadow-2xl"
        />
      {:else if isPDF(lightbox)}
        <iframe
          src={contentUrl(lightbox)}
          title={lightbox.original_filename}
          class="block bg-white rounded shadow-2xl"
          style="width: 90vw; height: 90vh;"
        ></iframe>
      {/if}
      <div class="mt-2 text-center text-xs text-white/70 truncate">
        {lightbox.original_filename}
      </div>
    </div>
  </div>
{/if}
