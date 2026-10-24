<script module>
  // Module-scoped counter for unique IDs across all mounted views — mermaid
  // requires a DOM-unique id per render() call. nanoid would also work but
  // a counter avoids pulling another dep into the editor chunk.
  let _mermaidIdCounter = 0;
  function nextMermaidId() {
    _mermaidIdCounter += 1;
    return `mmd-${Date.now().toString(36)}-${_mermaidIdCounter}`;
  }

  // Mermaid is a ~180KB chunk; load it once and share the module promise.
  let _mermaidModulePromise = null;
  function loadMermaid() {
    if (!_mermaidModulePromise) {
      _mermaidModulePromise = import('mermaid').then((m) => m.default ?? m);
    }
    return _mermaidModulePromise;
  }

  // Track the theme we initialized mermaid with so we re-initialize when it
  // changes (mermaid.initialize is global state).
  let _currentMermaidTheme = null;
  async function ensureInitialized(theme) {
    const mermaid = await loadMermaid();
    if (_currentMermaidTheme !== theme) {
      mermaid.initialize({
        startOnLoad: false,
        theme: theme === 'dark' ? 'dark' : 'default',
        securityLevel: 'strict',
        fontFamily: 'inherit',
      });
      _currentMermaidTheme = theme;
    }
    return mermaid;
  }
</script>

<script>
  import AlertTriangle from '@lucide/svelte/icons/alert-triangle';
  import Loader2 from '@lucide/svelte/icons/loader-2';
  import { themeStore } from '../stores/theme.svelte.js';
  import { t } from '../stores/i18n.svelte.js';

  let { source = '', readonly = false } = $props();

  let host = $state(null);
  let status = $state('idle'); // idle | rendering | ready | error
  let errorMsg = $state('');

  // Re-render when source or theme changes. `host` becomes non-null after
  // bind:this resolves; the effect fires again to do the first render then.
  $effect(() => {
    const _theme = themeStore.resolvedTheme;
    const trimmed = (source || '').trim();
    if (!host) return;
    if (!trimmed) {
      host.replaceChildren();
      status = 'idle';
      return;
    }
    void render(trimmed, _theme);
  });

  async function render(text, theme) {
    status = 'rendering';
    errorMsg = '';
    try {
      const mermaid = await ensureInitialized(theme);
      // parse() throws on invalid syntax with a useful message; do it first
      // so the user sees a readable error rather than mermaid's render-time
      // SVG-of-error.
      await mermaid.parse(text);
      const { svg } = await mermaid.render(nextMermaidId(), text);
      if (!host) return;
      host.innerHTML = svg;
      status = 'ready';
    } catch (err) {
      console.warn('Mermaid render failed', err);
      errorMsg = (err && (err.str || err.message)) ? String(err.str || err.message) : String(err);
      status = 'error';
      if (host) host.replaceChildren();
    }
  }
</script>

<figure class="mermaid-block" class:readonly data-testid="page-mermaid-block">
  <div
    class="mermaid-block__canvas"
    bind:this={host}
    aria-label={t('editors.diagramOpen')}
    data-testid="page-mermaid-canvas"
    data-status={status}
  >
    {#if status === 'rendering' || status === 'idle'}
      <div class="mermaid-block__placeholder">
        <Loader2 size={18} class="spin" />
        <span class="sr-only">{t('editors.mermaidRendering')}</span>
      </div>
    {/if}
  </div>
  {#if status === 'error'}
    <div class="mermaid-block__error">
      <div class="mermaid-block__error-header">
        <AlertTriangle size={14} />
        <span>{t('editors.mermaidParseError')}</span>
      </div>
      <pre class="mermaid-block__error-msg">{errorMsg}</pre>
      <pre class="mermaid-block__error-source">{source}</pre>
    </div>
  {/if}
</figure>

<style>
  .mermaid-block {
    display: block;
    margin: 0.75rem 0;
    border: 1px solid var(--border-color, #d1d5db);
    border-radius: 6px;
    background: var(--surface-1, #fff);
    overflow: hidden;
    padding: 12px;
  }
  .mermaid-block__canvas {
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 80px;
  }
  .mermaid-block__canvas :global(svg) {
    max-width: 100%;
    height: auto;
  }
  .mermaid-block__placeholder {
    display: flex;
    gap: 6px;
    align-items: center;
    color: var(--text-muted, #6b7280);
    font-size: 0.875rem;
  }
  .mermaid-block__error {
    margin-top: 8px;
    padding: 8px 10px;
    border: 1px solid var(--danger-border, #fecaca);
    background: var(--danger-surface, #fef2f2);
    color: var(--danger-color, #b91c1c);
    border-radius: 4px;
    font-size: 0.8125rem;
  }
  .mermaid-block__error-header {
    display: flex;
    gap: 6px;
    align-items: center;
    font-weight: 600;
    margin-bottom: 4px;
  }
  .mermaid-block__error-msg,
  .mermaid-block__error-source {
    margin: 4px 0 0;
    padding: 6px 8px;
    background: var(--surface-2, #fff);
    color: var(--text-muted, #4b5563);
    border-radius: 3px;
    white-space: pre-wrap;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.75rem;
  }
  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border: 0;
  }
  :global(.spin) {
    animation: mmd-spin 1s linear infinite;
  }
  @keyframes mmd-spin {
    to { transform: rotate(360deg); }
  }
</style>
