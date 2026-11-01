<script>
  import { cn } from '../utils/cn.js';

  /**
   * Surface for a row of related toolbar controls (view switcher, board
   * grouping/sorting, ...). The group owns the container surface and the
   * segment metrics, so every group lines up wherever it sits. A segment marks
   * itself active with `data-active="true"` (or `aria-pressed` for toggles) and
   * gets the shared flat selected treatment from the group.
   */
  let {
    /** Renders the segments; receives the shared segment class. */
    children,
    testid = undefined,
    ariaLabel = undefined,
    class: className = ''
  } = $props();

  const segment = 'inline-flex items-center gap-2 whitespace-nowrap px-3 py-1.5 text-sm font-medium text-[var(--ds-text)] transition-colors';
</script>

<div
  class={cn('ws-button-group inline-flex items-center overflow-hidden rounded-md', className)}
  style="background-color: var(--ctx-surface, var(--ds-background-neutral)); backdrop-filter: var(--ctx-backdrop, none);"
  data-testid={testid}
  role={ariaLabel ? 'group' : undefined}
  aria-label={ariaLabel}
>
  {@render children(segment)}
</div>

<style>
  /* Active segments use the flat selected surface from the design system;
     hover previews it with the context active tint. No per-segment rounding
     or shadow. */
  .ws-button-group :global([data-active='true']),
  .ws-button-group :global([aria-pressed='true']) {
    background-color: var(--ds-surface-selected);
    color: var(--ds-text);
  }

  .ws-button-group :global(button:not([data-active='true']):not([aria-pressed='true']):hover),
  .ws-button-group :global(a:not([data-active='true']):hover) {
    background-color: var(--ctx-active-bg, var(--ds-background-neutral-hovered));
  }
</style>
