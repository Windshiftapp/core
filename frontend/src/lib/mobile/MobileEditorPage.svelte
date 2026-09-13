<script>
  import { Loader } from '@lucide/svelte';

  /**
   * Full-screen editor layout for /m composition flows (create item, edit
   * title/description): sticky Cancel/Save header, scrollable body. The page
   * renders inside the shell's scroll surface (`.mobile-scroll`), which owns
   * scrolling — the header sticks to it. Pages own their state and dirty
   * guard; this component only renders the chrome.
   *
   * @type {{
   *   title?: string,
   *   saveLabel?: string,
   *   cancelLabel?: string,
   *   canSave?: boolean,
   *   saving?: boolean,
   *   error?: string,
   *   onsave?: () => void,
   *   oncancel?: () => void,
   *   dataTestid?: string,
   *   children?: import('svelte').Snippet,
   * }}
   */
  let {
    title = '',
    saveLabel = 'Save',
    cancelLabel = 'Cancel',
    canSave = true,
    saving = false,
    error = '',
    onsave = () => {},
    oncancel = () => {},
    dataTestid = undefined,
    children,
  } = $props();
</script>

<div class="editor-page" data-testid={dataTestid}>
  <header class="editor-header">
    <button
      class="bar-btn cancel"
      onclick={oncancel}
      disabled={saving}
      type="button"
      data-testid="editor-cancel"
    >
      {cancelLabel}
    </button>
    <h1 class="bar-title" data-testid="editor-title">{title}</h1>
    <button
      class="bar-btn save"
      onclick={onsave}
      disabled={!canSave || saving}
      type="button"
      data-testid="editor-save"
    >
      {#if saving}
        <Loader size={16} class="spin" aria-label="Saving" />
      {:else}
        {saveLabel}
      {/if}
    </button>
  </header>

  <div class="editor-body">
    {@render children?.()}
    {#if error}
      <p class="error" data-testid="editor-error">{error}</p>
    {/if}
  </div>
</div>

<style>
  .editor-header {
    position: sticky;
    top: 0;
    z-index: 30;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    min-height: 52px;
    padding: 0.5rem 0.75rem;
    padding-top: calc(env(safe-area-inset-top, 0px) + 0.5rem);
    background-color: var(--ds-surface);
    border-bottom: 1px solid var(--ds-border);
  }
  .bar-title {
    flex: 1;
    margin: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    text-align: center;
    font-size: 1.0625rem;
    font-weight: var(--font-semibold, 600);
    color: var(--ds-text);
  }
  .bar-btn {
    flex-shrink: 0;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 44px;
    min-height: 44px;
    padding: 0 0.5rem;
    border: none;
    background: transparent;
    font-size: 1rem;
    cursor: pointer;
  }
  .bar-btn.cancel {
    color: var(--ds-text-subtle);
  }
  .bar-btn.save {
    color: var(--ds-interactive);
    font-weight: var(--font-semibold, 600);
  }
  .bar-btn.save:disabled {
    opacity: 0.45;
    cursor: default;
  }
  :global(.spin) {
    animation: spin 1s linear infinite;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }

  .editor-body {
    padding: 1rem 1rem calc(env(safe-area-inset-bottom, 0px) + 2rem);
  }

  .error {
    margin: 0.75rem 0 0;
    font-size: 0.875rem;
    color: var(--ds-text-danger, var(--ds-danger));
  }
</style>
