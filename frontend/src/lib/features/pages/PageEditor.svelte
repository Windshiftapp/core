<script>
  import LazyMilkdownEditor from '../../editors/LazyMilkdownEditor.svelte';
  import { t } from '../../stores/i18n.svelte.js';

  /** Frameless page body editor. */
  let {
    entityId,
    workspaceId,
    content = $bindable(''),
    readonly = false,
    expectedContentHash = '',
    onBeforeDiagramOpen = async () => {},
    onDiagramPersisted = (_payload) => {},
    onContentChange = () => {},
  } = $props();
</script>

<div class="editor-frame flex min-h-0 min-w-0 flex-1 flex-col" data-testid="page-editor">
  <LazyMilkdownEditor
    bind:content
    placeholder={t('pages.editorPlaceholder')}
    showToolbar={true}
    {readonly}
    entityType="page"
    {entityId}
    enableDiagrams={true}
    {workspaceId}
    {expectedContentHash}
    {onBeforeDiagramOpen}
    {onDiagramPersisted}
    {onContentChange}
  />
</div>

<style>
  /* The embedded MilkdownEditor ships its own border + tinted card +
     150px min-height that paint as a small box. Inside the pages
     surface we strip the card and stretch every wrapper in the chain
     so the ProseMirror surface fills the whole page-pane.
     ----------------------------------------------------------------
     Specificity note: MilkdownEditor's scoped CSS produces selectors
     like `.milkdown-toolbar.svelte-HASH` (specificity 0,2,0). A plain
     `.editor-frame .milkdown-toolbar` ties at 0,2,0 and loses on
     source order because the dynamically-imported MilkdownEditor
     bundle is appended to <head> AFTER this component's stylesheet.
     Every override below chains through `.milkdown-wrapper` (or doubles
     the `.editor-frame` class) to lift specificity above the scoped
     originals; without that, the cascade tie silently restores the
     bordered card. Scope stays inside `.editor-frame` so inline
     editors and item descriptions keep their boxed look. */
  :global(.editor-frame.editor-frame .milkdown-wrapper) {
    flex: 1;
    display: flex;
    flex-direction: column;
    min-height: 0;
    /* Flatten the wrapper's own rounded card so the editor disappears
       into the page background regardless of focus state. */
    border-radius: 0;
    overflow: visible;
  }

  /* Kill the 2px blue focus halo MilkdownEditor draws on the wrapper
     for inline contexts. Doubled class bumps specificity above the
     scoped `.milkdown-wrapper:focus-within.svelte-HASH` (0,3,0). */
  :global(.editor-frame.editor-frame .milkdown-wrapper:focus-within) {
    box-shadow: none;
  }

  :global(.editor-frame .milkdown-wrapper .milkdown-editor),
  :global(.editor-frame .milkdown-wrapper .milkdown-editor.has-toolbar) {
    flex: 1;
    display: flex;
    flex-direction: column;
    border: none;
    border-radius: 0;
    background: transparent;
    overflow: visible;
  }

  /* The formatting toolbar stays frameless with one divider across the canvas. */
  :global(.editor-frame .milkdown-wrapper .milkdown-toolbar) {
    border: none;
    border-radius: 0;
    background: transparent;
    padding: 0 var(--page-gutter) 0.375rem;
    border-bottom: 1px solid var(--ds-border);
  }

  :global(.editor-frame .milkdown-wrapper .milkdown-editor .milkdown) {
    flex: 1;
    min-height: 0;
    /* Keep the prose aligned with the title and below the toolbar divider. */
    padding: 1.5rem var(--page-gutter) 0;
    display: flex;
    flex-direction: column;
    font-size: 1rem;
    line-height: 1.6;
  }

  /* ProseMirror itself must grow so the entire empty column is
     clickable + focusable, not just the first text node. */
  :global(.editor-frame .milkdown-wrapper .milkdown-editor .ProseMirror) {
    flex: 1;
    min-height: 50vh;
    outline: none;
  }
</style>
