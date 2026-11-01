<script>
  // Error bubble shared by the inline editors, anchored under the editor.
  let { error = '' } = $props();
</script>

{#if error}
  <div
    class="absolute top-full left-0 mt-1 text-xs px-2 py-1 border rounded shadow-sm z-10 inline-editor-error"
  >
    {error}
  </div>
{/if}

<style>
  .inline-editor-error {
    color: var(--ds-text-danger);
    background-color: var(--ds-surface-raised);
    border-color: var(--ds-border-danger);
  }
</style>
