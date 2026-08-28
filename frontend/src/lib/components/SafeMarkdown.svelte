<script lang="ts">
  import { sanitizeMarkdownHtml } from '../utils/sanitize.ts';

  let {
    html = '',
    source = '',
    compact = false,
    testid = undefined,
  }: {
    html?: string;
    source?: string;
    compact?: boolean;
    testid?: string;
  } = $props();

  const sanitized = $derived(sanitizeMarkdownHtml(html));
</script>

{#if sanitized}
  <div class:compact class="safe-markdown" data-testid={testid}>{@html sanitized}</div>
{:else if source}
  <div class:compact class="safe-markdown plaintext" data-testid={testid}>{source}</div>
{/if}

<style>
  .safe-markdown {
    min-width: 0;
    overflow-wrap: anywhere;
    color: inherit;
    line-height: 1.6;
  }

  .safe-markdown.compact {
    line-height: 1.45;
  }

  .safe-markdown.plaintext {
    white-space: pre-wrap;
  }

  .safe-markdown :global(:first-child) {
    margin-top: 0;
  }

  .safe-markdown :global(:last-child) {
    margin-bottom: 0;
  }

  .safe-markdown :global(p),
  .safe-markdown :global(ul),
  .safe-markdown :global(ol),
  .safe-markdown :global(blockquote),
  .safe-markdown :global(pre),
  .safe-markdown :global(table) {
    margin-block: 0.65em;
  }

  .safe-markdown :global(ul),
  .safe-markdown :global(ol) {
    padding-inline-start: 1.5rem;
  }

  .safe-markdown :global(ul) {
    list-style-type: disc;
  }

  .safe-markdown :global(ol) {
    list-style-type: decimal;
  }

  .safe-markdown :global(li) {
    display: list-item;
    margin-block: 0.25em;
  }

  .safe-markdown :global(a) {
    color: var(--ds-text-link);
    text-decoration: underline;
    text-underline-offset: 2px;
  }

  .safe-markdown :global(code) {
    border-radius: 4px;
    background: var(--ds-surface-sunken);
    padding: 0.1em 0.3em;
    font-family: var(--font-mono, ui-monospace, monospace);
    font-size: 0.9em;
  }

  .safe-markdown :global(pre) {
    max-width: 100%;
    overflow-x: auto;
    border-radius: 8px;
    background: var(--ds-surface-sunken);
    padding: 0.85rem 1rem;
  }

  .safe-markdown :global(pre code) {
    background: transparent;
    padding: 0;
  }

  .safe-markdown :global(blockquote) {
    margin-inline: 0;
    border-inline-start: 1px solid var(--ds-border);
    padding-inline-start: 1rem;
    color: var(--ds-text-subtle);
  }

  .safe-markdown :global(img) {
    max-width: 100%;
    height: auto;
  }

  .safe-markdown :global(table) {
    display: block;
    max-width: 100%;
    overflow-x: auto;
    border-collapse: collapse;
  }

  .safe-markdown :global(th),
  .safe-markdown :global(td) {
    border: 1px solid var(--ds-border);
    padding: 0.4rem 0.6rem;
    text-align: start;
  }

  .safe-markdown :global(input[type='checkbox']) {
    margin-inline-end: 0.4rem;
  }
</style>
