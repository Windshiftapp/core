<script>
  import { onMount, onDestroy } from 'svelte';
  import { pages } from '../../api/pages.js';
  import { navigate } from '../../router.js';
  import { t } from '../../stores/i18n.svelte.js';
  import LazyMilkdownEditor from '../../editors/LazyMilkdownEditor.svelte';

  /** Standalone print/PDF page preserving read-only Milkdown fidelity and
   * auto-opening print after content and diagrams settle. */
  let { workspaceId, pageId } = $props();

  let page = $state(null);
  let loading = $state(true);
  let error = $state('');
  let printRootEl = $state(null);

  // Fire window.print() at most once from the readiness gate.
  let autoPrinted = false;

  // Restore the user's theme and title after printing.
  let savedColorMode = null;
  let savedTitle = null;

  // Force light mode so browser and headless prints avoid dark sheets.
  function forceLightForPrint() {
    const root = document.documentElement;
    savedColorMode = root.dataset.colorMode ?? null;
    root.dataset.colorMode = 'light';
  }
  function restoreColorMode() {
    const root = document.documentElement;
    if (savedColorMode === null) delete root.dataset.colorMode;
    else root.dataset.colorMode = savedColorMode;
  }

  onMount(async () => {
    forceLightForPrint();
    window.addEventListener('beforeprint', forceLightForPrint);
    window.addEventListener('afterprint', restoreColorMode);

    try {
      page = await pages.getPage(workspaceId, pageId);
      // Default the Save-as-PDF filename to the page title.
      savedTitle = document.title;
      if (page?.title) document.title = page.title;
    } catch (err) {
      error = err?.message || t('pages.print.error');
    } finally {
      loading = false;
    }

    if (page) {
      await waitForRenderReady();
      if (!autoPrinted) {
        autoPrinted = true;
        // Print after the final layout and paint.
        requestAnimationFrame(() => requestAnimationFrame(() => window.print()));
      }
    }
  });

  onDestroy(() => {
    window.removeEventListener('beforeprint', forceLightForPrint);
    window.removeEventListener('afterprint', restoreColorMode);
    restoreColorMode();
    if (savedTitle !== null) document.title = savedTitle;
  });

  /** Wait for the body, diagrams, images, and fonts, bounded so stuck assets
   * cannot block printing indefinitely. */
  function waitForRenderReady() {
    const MAX_WAIT_MS = 4000;
    const POLL_MS = 100;
    const expectBody = ((page?.content || '').trim().length) > 0;
    const start = performance.now();

    const domSettled = () =>
      new Promise((resolve) => {
        const tick = () => {
          if (performance.now() - start >= MAX_WAIT_MS) return resolve();
          const rootEl = printRootEl;
          if (!rootEl) return setTimeout(tick, POLL_MS);

          const body = rootEl.querySelector('.ProseMirror');
          const bodyReady = !expectBody || (body && (body.textContent || '').trim().length > 0);

          const diagrams = Array.from(rootEl.querySelectorAll('[data-excalidraw-block]'));
          const diagramsReady = diagrams.every((d) =>
            ['ready', 'missing', 'error'].includes(d.getAttribute('data-status'))
          );

          const imgs = Array.from(rootEl.querySelectorAll('img'));
          const imgsReady = imgs.every((img) => img.complete);

          if (bodyReady && diagramsReady && imgsReady) return resolve();
          setTimeout(tick, POLL_MS);
        };
        tick();
      });

    const fontsReady =
      document.fonts && document.fonts.ready
        ? document.fonts.ready.catch(() => {})
        : Promise.resolve();

    return Promise.all([domSettled(), fontsReady]);
  }

  function printNow() {
    window.print();
  }

  function backToPage() {
    navigate(`/workspaces/${workspaceId}/pages/${pageId}`);
  }
</script>

<div class="print-root" bind:this={printRootEl}>
  <div class="print-controls" role="toolbar" aria-label={t('pages.print.button')}>
    <button type="button" class="print-btn print-btn--primary" onclick={printNow} data-testid="page-print-button">
      {t('pages.print.button')}
    </button>
    <button type="button" class="print-btn" onclick={backToPage} data-testid="page-print-back">
      {t('pages.print.back')}
    </button>
  </div>

  {#if error}
    <p class="print-error" role="alert">{error}</p>
  {:else if loading}
    <p class="print-loading">{t('pages.print.loading')}</p>
  {:else if page}
    <h1 class="print-title">{page.title}</h1>

    {#if (page.labels || []).length > 0}
      <div class="print-labels">
        {#each page.labels as label (label.id)}
          <span
            class="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs"
            style="background-color: {label.color || '#3B82F6'}1A; color: var(--ds-text); border: 1px solid {label.color || '#3B82F6'};"
          >
            <span
              class="inline-block w-2 h-2 rounded-full"
              style="background-color: {label.color || '#3B82F6'};"
              aria-hidden="true"
            ></span>
            {label.name}
          </span>
        {/each}
      </div>
    {/if}

    <div class="print-body" data-testid="page-print-body">
      <LazyMilkdownEditor
        content={page.content}
        readonly={true}
        showToolbar={false}
        entityType="page"
        entityId={page.id}
        enableDiagrams={true}
        {workspaceId}
      />
    </div>
  {/if}
</div>

<style>
  .print-root {
    max-width: 60rem;
    margin: 0 auto;
    padding: 2rem 1.5rem 4rem;
    background: var(--ds-surface);
    color: var(--ds-text);
    min-height: 100vh;
  }

  .print-controls {
    display: flex;
    gap: 0.75rem;
    margin-bottom: 1.5rem;
  }

  .print-btn {
    display: inline-flex;
    align-items: center;
    padding: 0.4rem 0.9rem;
    border: 1px solid var(--ds-border);
    border-radius: 0.375rem;
    background: var(--ds-surface-raised);
    color: var(--ds-text);
    font-size: 0.875rem;
    cursor: pointer;
  }

  .print-btn:hover {
    background: var(--ds-background-neutral-hovered);
  }

  .print-btn--primary {
    background: var(--ds-interactive);
    border-color: var(--ds-interactive);
    color: var(--ds-text-inverse);
  }

  .print-btn--primary:hover {
    background: var(--ds-interactive-hovered);
  }

  .print-title {
    font-size: 2rem;
    font-weight: 700;
    line-height: 1.2;
    margin: 0 0 0.5rem;
  }

  .print-labels {
    display: flex;
    flex-wrap: wrap;
    gap: 0.375rem;
    margin-bottom: 1rem;
  }

  .print-error {
    color: var(--ds-text-danger);
  }

  .print-loading {
    color: var(--ds-text-subtle);
  }

  /* Strip the embedded MilkdownEditor's card (border, tinted bg, min-height,
     radius) on screen and paper so the body reads as a plain document.
     `!important` wins over the editor's scoped, non-important styles
     regardless of stylesheet source order. */
  :global(.print-body .milkdown-wrapper),
  :global(.print-body .milkdown-editor),
  :global(.print-body .milkdown-editor .milkdown),
  :global(.print-body .milkdown-editor .ProseMirror) {
    min-height: 0 !important;
    border: none !important;
    border-radius: 0 !important;
    box-shadow: none !important;
    background: transparent !important;
    padding-left: 0 !important;
    padding-right: 0 !important;
  }

  @media print {
    /* Paper geometry */
    @page {
      margin: 18mm 16mm;
    }

    /* Force a white canvas / light scheme regardless of theme. */
    :global(html),
    :global(body) {
      background: #ffffff !important;
      color: #111111 !important;
      color-scheme: light !important;
    }

    /* Hide screen-only controls + any residual editor chrome. */
    .print-controls {
      display: none !important;
    }
    :global(.milkdown-toolbar) {
      display: none !important;
    }

    /* Let the document flow on paper instead of a scroll viewport. */
    .print-root {
      max-width: none;
      margin: 0;
      padding: 0;
      min-height: 0;
      background: #ffffff !important;
      color: #111111 !important;
    }

    /* Never orphan a heading at the bottom of a sheet — keep it with the
       content that follows. */
    .print-title,
    :global(.ProseMirror h1),
    :global(.ProseMirror h2),
    :global(.ProseMirror h3),
    :global(.ProseMirror h4),
    :global(.ProseMirror h5),
    :global(.ProseMirror h6) {
      break-after: avoid;
      page-break-after: avoid;
      break-inside: avoid;
    }

    /* Atomic blocks: don't split across a page boundary. */
    :global(.ProseMirror pre),
    :global(.ProseMirror blockquote),
    :global(.ProseMirror table),
    :global(.ProseMirror figure),
    :global(.ProseMirror img),
    :global(.ProseMirror [data-excalidraw-block]),
    :global(.excalidraw-block) {
      break-inside: avoid;
      page-break-inside: avoid;
    }

    /* Scale media down to one page rather than overflow the sheet. */
    :global(.ProseMirror img),
    :global(.excalidraw-block),
    :global(.excalidraw-block svg) {
      max-width: 100% !important;
      height: auto !important;
    }

    /* Orphan / widow control for running text. */
    :global(.ProseMirror p),
    :global(.ProseMirror li) {
      orphans: 3;
      widows: 3;
    }

    /* Repeat table headers on every printed page. */
    :global(.ProseMirror thead) {
      display: table-header-group;
    }
    :global(.ProseMirror tr),
    :global(.ProseMirror td),
    :global(.ProseMirror th) {
      break-inside: avoid;
    }
  }
</style>
