<script>
  import { t } from '../stores/i18n.svelte.js';

  let {
    workspaceName = '',
    collection = '',
    viewName = '',
    itemCount = null,
    actionButtons = null,
    hasGradient = false,
    textStyle = '',
    subtleTextStyle = '',
    actions = null,
  } = $props();
  void hasGradient; void textStyle; void subtleTextStyle;
</script>

<div class="flex items-center justify-between mb-4">
  <div>
    <div class="flex items-baseline gap-2 mb-1">
      <h1 class="text-xl font-medium" style="color: var(--ctx-text, var(--ds-text));">
        {viewName}
      </h1>
      {#if collection}
        <span class="text-xs font-medium px-1.5 py-0.5 rounded" style="background-color: var(--ctx-active-bg, var(--ds-accent-blue-subtler)); color: var(--ctx-active-text, var(--ds-accent-blue)); backdrop-filter: var(--ctx-backdrop, none);">
          {collection}
        </span>
      {/if}
    </div>
    <div class="flex items-center">
      <span class="text-sm" style="color: var(--ctx-text-subtle, var(--ds-text-subtle));">
        {workspaceName}{#if itemCount !== null} • {itemCount} {t('layout.items')}{/if}
      </span>
    </div>
  </div>

  {#if actions}
    <div>
      {@render actions()}
    </div>
  {/if}
</div>
