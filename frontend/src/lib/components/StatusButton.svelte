<script>
  import { cn } from '../utils/cn.js';

  /**
   * Selectable pass/fail/blocked/skipped status button used by the test
   * execution views. Tints with the status tokens while unselected and fills
   * solid when selected.
   *
   * @type {{
   *   status: 'passed' | 'failed' | 'blocked' | 'skipped',
   *   selected?: boolean,
   *   onclick?: (event: MouseEvent) => void,
   *   dataTestid?: string,
   *   class?: string,
   *   children?: import('svelte').Snippet,
   * }}
   */
  let { status, selected = false, onclick = undefined, dataTestid = undefined, class: className = '', children } = $props();
</script>

<button
  type="button"
  class={cn('status-btn', className)}
  data-status={status}
  class:selected
  data-testid={dataTestid}
  {onclick}
>
  {@render children?.()}
</button>

<style>
  .status-btn {
    background-color: transparent;
    border: 1px solid;
  }

  .status-btn[data-status='passed'] {
    color: var(--ds-status-success-text);
    border-color: var(--ds-status-success-border);
  }

  .status-btn[data-status='failed'] {
    color: var(--ds-status-danger-text);
    border-color: var(--ds-status-danger-border);
  }

  .status-btn[data-status='blocked'] {
    color: var(--ds-status-warning-text);
    border-color: var(--ds-status-warning-border);
  }

  .status-btn[data-status='skipped'] {
    color: var(--ds-status-neutral-text);
    border-color: var(--ds-status-neutral-border);
  }

  .status-btn.selected {
    color: white;
  }

  .status-btn[data-status='passed'].selected {
    background-color: var(--ds-status-success-solid);
    border-color: var(--ds-status-success-solid);
  }

  .status-btn[data-status='failed'].selected {
    background-color: var(--ds-status-danger-solid);
    border-color: var(--ds-status-danger-solid);
  }

  .status-btn[data-status='blocked'].selected {
    background-color: var(--ds-status-warning-solid);
    border-color: var(--ds-status-warning-solid);
  }

  .status-btn[data-status='skipped'].selected {
    background-color: var(--ds-status-neutral-solid);
    border-color: var(--ds-status-neutral-solid);
  }

  .status-btn[data-status='passed']:hover:not(.selected) {
    background-color: var(--ds-status-success-bg);
  }

  .status-btn[data-status='failed']:hover:not(.selected) {
    background-color: var(--ds-status-danger-bg);
  }

  .status-btn[data-status='blocked']:hover:not(.selected) {
    background-color: var(--ds-status-warning-bg);
  }

  .status-btn[data-status='skipped']:hover:not(.selected) {
    background-color: var(--ds-status-neutral-bg);
  }
</style>
