<script>
  import { Copy, Check } from 'lucide-svelte';

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
    {@html renderMarkdown(briefing)}
  </div>
</div>

<script context="module">
  function renderMarkdown(text) {
    if (!text) return '';
    return text
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/^### (.+)$/gm, '<h4 class="text-sm font-semibold mt-3 mb-1" style="color: var(--ds-text);">$1</h4>')
      .replace(/^## (.+)$/gm, '<h3 class="text-base font-semibold mt-4 mb-1" style="color: var(--ds-text);">$1</h3>')
      .replace(/^# (.+)$/gm, '<h2 class="text-lg font-semibold mt-4 mb-2" style="color: var(--ds-text);">$1</h2>')
      .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
      .replace(/\*(.+?)\*/g, '<em>$1</em>')
      .replace(/`(.+?)`/g, '<code class="px-1 py-0.5 rounded text-xs" style="background-color: var(--ds-surface-sunken);">$1</code>')
      .replace(/^- (.+)$/gm, '<li class="ml-4 list-disc">$1</li>')
      .replace(/^(\d+)\. (.+)$/gm, '<li class="ml-4 list-decimal">$2</li>')
      .replace(/\n\n/g, '</p><p class="mt-2">')
      .replace(/\n/g, '<br>');
  }
</script>
