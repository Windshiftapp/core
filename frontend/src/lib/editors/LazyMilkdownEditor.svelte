<script>
  import { onMount } from 'svelte';

  // All props that MilkdownEditor accepts
  let {
    content = $bindable(''), placeholder = '', readonly = false,
    showToolbar = false, itemId = null, entityType = null,
    entityId = null, onImageInsert = null, isPersonalWorkspace = false, compact = false
  } = $props();

  let MilkdownEditor = $state(null);
  let editorInstance = $state(null);
  let loading = $state(true);

  // Start preloading immediately on mount (background, non-blocking)
  onMount(() => {
    const preload = async () => {
      try {
        const module = await import('./MilkdownEditor.svelte');
        MilkdownEditor = module.default;
      } catch (error) {
        console.error('Failed to load MilkdownEditor:', error);
      } finally {
        loading = false;
      }
    };

    // Preload immediately but don't block render
    if ('requestIdleCallback' in window) {
      requestIdleCallback(preload);
    } else {
      // Fallback: use microtask to start loading after paint
      queueMicrotask(preload);
    }
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
</script>

{#if MilkdownEditor}
  <MilkdownEditor
    bind:this={editorInstance}
    bind:content
    {placeholder}
    {readonly}
    {showToolbar}
    {itemId}
    {entityType}
    {entityId}
    {onImageInsert}
    {isPersonalWorkspace}
    {compact}
  />
{:else}
  <!-- Skeleton placeholder while loading -->
  <div
    class="animate-pulse rounded"
    style="min-height: {compact ? '80px' : '150px'}; background: var(--ds-background-neutral); border: 1px solid var(--ds-border); border-radius: 0.375rem;"
  ></div>
{/if}
