<script>
  import { t } from '../stores/i18n.svelte.js';

  let {
    workspaceName = '',
    collection = '',
    viewName = '',
    itemCount = null,
    actionButtons = null,
    hasGradient = false,
    textStyle = 'color: var(--ds-text);',
    subtleTextStyle = 'color: var(--ds-text-subtle);',
    actions
  } = $props();
</script>

<div class="flex items-center justify-between mb-4">
  <div>
    <div class="flex items-baseline gap-2 mb-1">
      <h1 class="text-xl font-medium" style={textStyle}>
        {viewName}
      </h1>
      {#if collection}
        <span class="text-xs font-medium px-1.5 py-0.5 rounded" style={hasGradient ? 'background-color: var(--ds-glass-bg); color: var(--ds-text); backdrop-filter: blur(4px); border: 1px solid var(--ds-glass-border);' : 'background-color: var(--ds-accent-blue-subtler); color: var(--ds-accent-blue);'}>
          {collection}
        </span>
      {/if}
    </div>
    <div class="flex items-center">
      <span class="text-sm" style={subtleTextStyle}>
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
