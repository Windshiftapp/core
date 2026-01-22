<script>
  import { onMount } from 'svelte';

  // All props that MilkdownEditor accepts
  export let content = '';
  export let placeholder = '';
  export let readonly = false;
  export let showToolbar = false;
  export let itemId = null;
  export let entityType = null;
  export let entityId = null;
  export let onImageInsert = null;
  export let isPersonalWorkspace = false;
  export let compact = false;

  let MilkdownEditor = null;
  let editorInstance = null;
  let loading = true;

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
    if (editorInstance?.focus) {
      editorInstance.focus();
    }
  }

  export function focusEnd() {
    if (editorInstance?.focusEnd) {
      editorInstance.focusEnd();
    }
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
  <svelte:component
    this={MilkdownEditor}
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
