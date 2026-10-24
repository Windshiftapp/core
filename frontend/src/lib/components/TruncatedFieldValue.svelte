<script>
  import ExternalLink from '@lucide/svelte/icons/external-link';
  import Tooltip from './Tooltip.svelte';

  let {
    value,
    href = null,
    onactivate = null,
    disabled = false,
    subtle = false,
    testId = undefined,
  } = $props();

  let isTruncated = $state(false);

  function trackOverflow(node, _value) {
    const measure = () => {
      isTruncated = node.scrollWidth > node.clientWidth;
    };

    measure();
    const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(measure);
    observer?.observe(node);
    window.addEventListener('resize', measure);

    return {
      update() {
        measure();
      },
      destroy() {
        observer?.disconnect();
        window.removeEventListener('resize', measure);
      },
    };
  }
</script>

<Tooltip
  content={String(value)}
  disabled={!isTruncated}
  class="block min-w-0 max-w-full"
  contentClass="max-w-sm break-words px-2.5 py-1.5 text-xs"
>
  {#if href}
    <a
      {href}
      target="_blank"
      rel="noopener noreferrer"
      class="flex min-w-0 max-w-full items-center justify-end gap-1.5 rounded-sm hover:underline focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2"
      style="color: var(--ds-link, var(--ds-text)); outline-color: var(--ds-border-focused);"
      data-testid={testId}
    >
      <span use:trackOverflow={value} class="min-w-0 truncate">{value}</span>
      <ExternalLink class="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
    </a>
  {:else if onactivate}
    <button
      type="button"
      onclick={onactivate}
      {disabled}
      use:trackOverflow={value}
      class="block w-full min-w-0 truncate rounded-sm text-right focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 disabled:cursor-default"
      style="color: {subtle ? 'var(--ds-text-subtle)' : 'var(--ds-text)'}; outline-color: var(--ds-border-focused);"
      data-testid={testId}
    >
      {value}
    </button>
  {:else}
    <span
      use:trackOverflow={value}
      class="block min-w-0 truncate text-right"
      style="color: {subtle ? 'var(--ds-text-subtle)' : 'var(--ds-text)'};"
      data-testid={testId}
    >
      {value}
    </span>
  {/if}
</Tooltip>
