<script>
  import { ChevronLeft } from '@lucide/svelte';
  import { t } from '../stores/i18n.svelte.js';

  /**
   * @type {{
   *   title?: string,
   *   onback?: (() => void) | null,
   *   right?: import('svelte').Snippet | null,
   *   children?: import('svelte').Snippet | null,
   * }}
   */
  let { title = '', onback = null, right = null, children = null } = $props();
</script>

<header class="mobile-header" data-testid="mobile-header">
  <div class="left">
    {#if onback}
      <button class="back" onclick={onback} data-testid="mobile-header-back" aria-label={t('common.back')} type="button">
        <ChevronLeft size={24} />
      </button>
    {/if}
    {#if title}
      <h1 class="title" data-testid="mobile-header-title">{title}</h1>
    {/if}
  </div>
  {#if right}
    <div class="right">{@render right()}</div>
  {/if}
</header>

{#if children}
  <div class="mobile-header-extra">{@render children()}</div>
{/if}

<style>
  .mobile-header {
    position: sticky;
    top: 0;
    /* Above scrolling content but BELOW portaled dropdown menus (DropdownMenu
       z-60, picker z-70) so the user menu / pickers aren't clipped by the bar. */
    z-index: 30;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
    min-height: 52px;
    padding: 0.5rem 0.75rem;
    padding-top: calc(env(safe-area-inset-top, 0px) + 0.5rem);
    background-color: var(--ds-surface);
    border-bottom: 1px solid var(--ds-border);
  }

  .left {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    min-width: 0;
  }

  .back {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 36px;
    height: 36px;
    margin-left: -6px;
    border: none;
    background: transparent;
    color: var(--ds-text);
    cursor: pointer;
  }

  .title {
    font-size: 1.0625rem;
    font-weight: var(--font-semibold, 600);
    color: var(--ds-text);
    margin: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .right {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    flex-shrink: 0;
  }

  .mobile-header-extra {
    position: sticky;
    top: 52px;
    z-index: 30;
    background-color: var(--ds-surface);
    border-bottom: 1px solid var(--ds-border);
  }
</style>
