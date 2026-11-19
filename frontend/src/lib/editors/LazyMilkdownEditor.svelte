<script>
  import { onMount } from 'svelte';
  import Button from '../components/Button.svelte';
  import StateDisplay from '../components/StateDisplay.svelte';
  import { t } from '../stores/i18n.svelte.js';

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
  let loadError = $state(false);

  // Load immediately: idle callbacks can starve on busy pages. A failed chunk
  // must offer recovery rather than leaving a permanent loading placeholder.
  async function loadEditor() {
    loading = true;
    loadError = false;
    try {
      const module = await import('./MilkdownEditor.svelte');
      MilkdownEditor = module.default;
    } catch (error) {
      console.error('Failed to load MilkdownEditor:', error);
      loadError = true;
    } finally {
      loading = false;
    }
  }

  onMount(() => { void loadEditor(); });

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

  export function getMarkdown() {
    return editorInstance?.getMarkdown?.() ?? '';
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
{:else if loadError}
  <StateDisplay type="error" size="sm" dataTestid="milkdown-load-error">
    {#snippet action()}
      <!-- Browsers cache failed ESM loads for this document; reload clears them. -->
      <Button variant="default" onclick={() => window.location.reload()} dataTestid="milkdown-load-reload">
        {t('common.reload')}
      </Button>
    {/snippet}
  </StateDisplay>
{:else if loading}
  <StateDisplay type="loading" size="sm" dataTestid="milkdown-loading" />
{/if}
