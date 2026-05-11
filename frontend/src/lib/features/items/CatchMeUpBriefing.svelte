<script>
  import { Copy, Check } from '@lucide/svelte';
  import MilkdownEditor from '../../editors/LazyMilkdownEditor.svelte';

  let { briefing = '', itemKey = '' } = $props();

  let copied = $state(false);

  async function copyToClipboard() {
    try {
      await navigator.clipboard.writeText(briefing);
      copied = true;
      setTimeout(() => { copied = false; }, 2000);
    } catch (err) {
      console.error('Failed to copy:', err);
    }
  }
</script>

<div class="space-y-3">
  <div class="flex items-center justify-between">
    <span class="text-xs font-medium px-2 py-1 rounded" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
      {itemKey}
    </span>
    <button
      class="inline-flex items-center gap-1.5 px-2 py-1 text-xs rounded transition-colors"
      style="color: var(--ds-text-subtle);"
      onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
      onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'transparent'}
      onclick={copyToClipboard}
    >
      {#if copied}
        <Check class="w-3.5 h-3.5" style="color: var(--ds-icon-success);" />
        <span style="color: var(--ds-text-success);">Copied</span>
      {:else}
        <Copy class="w-3.5 h-3.5" />
        Copy
      {/if}
    </button>
  </div>

  <div class="prose-sm max-w-none text-sm leading-relaxed" style="color: var(--ds-text);">
    <MilkdownEditor content={briefing} readonly={true} showToolbar={false} compact={true} />
  </div>
</div>
