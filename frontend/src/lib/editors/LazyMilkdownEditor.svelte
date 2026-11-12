<script>
  import { onMount } from 'svelte';

  // All props that MilkdownEditor accepts
  let {
    content = $bindable(''), placeholder = '', readonly = false,
    showToolbar = false, hideToolbarUntilFocus = false, itemId = null, entityType = null,
    entityId = null, onImageInsert = null, onContentChange = null, isPersonalWorkspace = false, compact = false,
    customUploadFn = null, downloadUrlBase = '/api/attachments', deferImageUploads = false,
    onDeferredImageUpload = null,
    allowImageUpload = true,
    enableDiagrams = false,
    workspaceId = null,
    enablePageLinks = false,
    expectedContentHash = '',
    onBeforeDiagramOpen = async () => {},
    onDiagramPersisted = (_payload) => {},
    testId = null
  } = $props();

  let MilkdownEditor = $state(null);
  let editorInstance = $state(null);
  let loading = $state(true);

  // Start loading the editor chunk on mount. The dynamic import is async and
  // does not block render, and requestIdleCallback can starve under load
  // (CI, busy pages), leaving the description blank for seconds.
  onMount(() => {
    void (async () => {
      try {
        const module = await import('./MilkdownEditor.svelte');
        MilkdownEditor = module.default;
      } catch (error) {
        console.error('Failed to load MilkdownEditor:', error);
      } finally {
        loading = false;
      }
    })();
  });

  // Expose methods from the underlying editor
  export function focus() {
    let attempts = 0;
    const maxAttempts = 20; // 20 * 50ms = 1 second max wait

    const attemptFocus = () => {
      if (editorInstance?.focus) {
        editorInstance.focus();
      } else if (attempts < maxAttempts) {
        attempts++;
        setTimeout(attemptFocus, 50);
      }
    };

    attemptFocus();
  }

  export function focusEnd() {
    let attempts = 0;
    const maxAttempts = 20; // 20 * 50ms = 1 second max wait

    const attemptFocus = () => {
      if (editorInstance?.focusEnd) {
        editorInstance.focusEnd();
      } else if (attempts < maxAttempts) {
        attempts++;
        setTimeout(attemptFocus, 50);
      }
    };

    attemptFocus();
  }

  export function clear() {
    if (editorInstance?.clear) {
      editorInstance.clear();
    }
  }

  export function insertImage(src, alt, title) {
    if (editorInstance?.insertImage) {
      editorInstance.insertImage(src, alt, title);
    }
  }

  export function insertMarkdown(markdown) {
    if (editorInstance?.insertMarkdown) {
      editorInstance.insertMarkdown(markdown);
    }
  }
</script>

{#if MilkdownEditor}
  {#key readonly}
    <MilkdownEditor
      bind:this={editorInstance}
      bind:content
      {placeholder}
      {readonly}
      {showToolbar}
      {hideToolbarUntilFocus}
      {itemId}
      {entityType}
      {entityId}
      {onImageInsert}
      {onContentChange}
      {isPersonalWorkspace}
      {compact}
      {customUploadFn}
      {downloadUrlBase}
      {deferImageUploads}
      {onDeferredImageUpload}
      {allowImageUpload}
      {enableDiagrams}
      {workspaceId}
      {enablePageLinks}
      {expectedContentHash}
      {onBeforeDiagramOpen}
      {onDiagramPersisted}
      {testId}
    />
  {/key}
{:else}
  <!-- Skeleton placeholder while loading -->
  <div
    class="animate-pulse rounded"
    style="min-height: {compact ? '80px' : '150px'}; background: var(--ds-background-neutral); border: 1px solid var(--ds-border); border-radius: 0.375rem;"
  ></div>
{/if}
