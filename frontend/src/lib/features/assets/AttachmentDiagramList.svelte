<script>
  import { Paperclip, PenTool, Trash2 } from 'lucide-svelte';
  import { t } from '../../stores/i18n.svelte.js';

  let {
    attachments = [],
    diagrams = [],
    canDelete = true,
    ondelete,
    oneditdiagram,
    ondeletediagram
  } = $props();

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
      alert(t('assets.failedToDownload') + ': ' + error.message);
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
        <Paperclip class="w-3.5 h-3.5 flex-shrink-0" style="color: var(--ds-text-subtle);" />
        <button
          onclick={() => handleDownload(attachment)}
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
            <Trash2 class="w-3.5 h-3.5" />
          </button>
        {/if}
      </div>
    {/each}
    {#each diagrams as diagram}
      <div class="flex items-center gap-2 py-1 px-2 -mx-2 rounded group hover:bg-[var(--ds-background-neutral-hovered)] transition-colors">
        <PenTool class="w-3.5 h-3.5 flex-shrink-0" style="color: var(--ds-text-subtle);" />
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
            <Trash2 class="w-3.5 h-3.5" />
          </button>
        {/if}
      </div>
    {/each}
  </div>
{/if}
