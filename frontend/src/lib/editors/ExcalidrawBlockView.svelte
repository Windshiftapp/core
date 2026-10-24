<script>
  import Pencil from '@lucide/svelte/icons/pencil';
  import AlertTriangle from '@lucide/svelte/icons/alert-triangle';
  import Loader2 from '@lucide/svelte/icons/loader-2';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import { t } from '../stores/i18n.svelte.js';
  import { preparePageDiagramScene } from '../features/pages/pageDiagramScene.js';

  let {
    attachmentId,
    name = '',
    readonly = false,
    onEdit = () => {},
    onDelete = () => {},
  } = $props();

  let svgHost = $state(null);
  let status = $state('idle'); // idle | loading | ready | missing | error
  let errorMsg = $state('');

  // Re-render whenever the underlying attachment changes (edit saves a new one).
  $effect(() => {
    const id = attachmentId;
    if (!id || !svgHost) return;
    void renderDiagram(id);
  });

  async function renderDiagram(id) {
    status = 'loading';
    errorMsg = '';
    try {
      const res = await fetch(`/api/attachments/${id}/download`, { credentials: 'same-origin' });
      if (res.status === 404) {
        status = 'missing';
        return;
      }
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const scene = await preparePageDiagramScene(await res.json());
      const { exportToSvg } = await import('@excalidraw/excalidraw');
      const svg = await exportToSvg({
        elements: scene.elements || [],
        appState: { ...(scene.appState || {}), exportBackground: true, exportWithDarkMode: false },
        files: scene.files || null,
        exportPadding: 16,
      });
      svg.setAttribute('width', '100%');
      svg.style.maxWidth = '100%';
      svg.style.height = 'auto';
      if (svgHost) {
        svgHost.replaceChildren(svg);
        status = 'ready';
      }
    } catch (err) {
      console.error('Failed to render diagram', err);
      errorMsg = err?.message || String(err);
      status = 'error';
    }
  }

  function handleClick() {
    if (readonly) return;
    onEdit();
  }

  function handleKeydown(event) {
    if (event.key !== 'Enter' && event.key !== ' ') return;
    event.preventDefault();
    handleClick();
  }
</script>

<figure
  class="excalidraw-block"
  class:readonly
  data-excalidraw-block
  data-status={status}
  data-testid="page-diagram-block"
>
  <div
    class="excalidraw-block__canvas"
    bind:this={svgHost}
    onclick={handleClick}
    onkeydown={handleKeydown}
    role="button"
    tabindex={readonly ? -1 : 0}
    aria-label={name || t('editors.diagramOpen')}
    data-testid="page-diagram-canvas"
  >
    {#if status === 'loading' || status === 'idle'}
      <div class="excalidraw-block__placeholder">
        <Loader2 size={18} class="spin" />
      </div>
    {:else if status === 'missing'}
      <div class="excalidraw-block__placeholder error">
        <AlertTriangle size={18} />
        <span>{t('editors.diagramDeleted')}</span>
      </div>
    {:else if status === 'error'}
      <div class="excalidraw-block__placeholder error">
        <AlertTriangle size={18} />
        <span>{t('editors.diagramRenderError')}</span>
        {#if errorMsg}
          <span class="excalidraw-block__error-msg" title={errorMsg}>{errorMsg}</span>
        {/if}
      </div>
    {/if}
  </div>
  <figcaption class="excalidraw-block__caption">
    <span class="excalidraw-block__name" data-testid="page-diagram-caption">{name || t('editors.diagramUntitled')}</span>
    {#if !readonly}
      <div class="excalidraw-block__actions">
        <button
          type="button"
          class="excalidraw-block__action"
          onclick={() => onEdit()}
          title={t('editors.diagramEdit')}
          aria-label={t('editors.diagramEdit')}
          data-testid="excalidraw-block-edit"
        >
          <Pencil size={14} />
        </button>
        <button
          type="button"
          class="excalidraw-block__action excalidraw-block__delete"
          onclick={() => onDelete()}
          title={t('common.delete')}
          aria-label={t('common.delete')}
          data-testid="excalidraw-block-delete"
        >
          <Trash2 size={14} />
        </button>
      </div>
    {/if}
  </figcaption>
</figure>

<style>
  .excalidraw-block {
    display: block;
    margin: 0.75rem 0;
    border: 1px solid var(--ds-border);
    border-radius: 6px;
    background: var(--ds-surface-raised);
    overflow: hidden;
  }
  .excalidraw-block__canvas {
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 120px;
    padding: 8px;
    cursor: pointer;
  }
  .excalidraw-block.readonly .excalidraw-block__canvas {
    cursor: default;
  }
  .excalidraw-block__canvas :global(svg) {
    max-width: 100%;
    height: auto;
  }
  .excalidraw-block__placeholder {
    display: flex;
    gap: 6px;
    align-items: center;
    color: var(--ds-text-subtle);
    font-size: 0.875rem;
  }
  .excalidraw-block__placeholder.error {
    color: var(--ds-text-danger);
  }
  .excalidraw-block__error-msg {
    overflow: hidden;
    max-width: 60ch;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.75rem;
  }
  .excalidraw-block__caption {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 6px 10px;
    border-top: 1px solid var(--ds-border);
    background: var(--ds-surface-hovered);
    font-size: 0.8125rem;
    color: var(--ds-text-subtle);
  }
  .excalidraw-block__name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .excalidraw-block__actions {
    display: inline-flex;
    align-items: center;
    gap: 2px;
    flex-shrink: 0;
  }
  .excalidraw-block__action {
    background: none;
    border: 1px solid transparent;
    border-radius: 4px;
    padding: 4px;
    cursor: pointer;
    color: inherit;
    display: inline-flex;
    align-items: center;
  }
  .excalidraw-block__action:hover {
    background: var(--ds-surface-raised-hovered);
    border-color: var(--ds-border);
  }
  .excalidraw-block__delete {
    color: var(--ds-text-danger, #b91c1c);
  }
  :global(.spin) {
    animation: excal-spin 1s linear infinite;
  }
  @keyframes excal-spin {
    to { transform: rotate(360deg); }
  }
</style>
