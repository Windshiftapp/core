<script>
  import { ChevronLeft, Plus } from '@lucide/svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { systemStatusName } from '../../utils/systemLabels.js';

  let {
    column,
    itemCount = 0,
    wipCount = itemCount,
    visibleItemCount = itemCount,
    hiddenItemCount = 0,
    isOverWip = false,
    statusColumnKey = '',
    swimlaneParentId = '',
    statusId = column?.status_ids?.[0] ?? '',
    quickAddOpen = false,
    columnStyle = 'background-color: var(--ds-surface-raised); border-color: var(--ds-border);',
    textStyle = 'color: var(--ds-text);',
    subtleTextStyle = 'color: var(--ds-text-subtle);',
    onadd = null,
    oncollapse = null,
    children,
  } = $props();
</script>

<div
  class="relative rounded border shadow-sm transition-colors"
  style="{columnStyle} {quickAddOpen ? 'z-index: 30;' : ''}"
  data-testid="board-column"
  id={`board-column-status-${statusId}`}
  data-status-column
  data-status-column-key={statusColumnKey}
  data-swimlane-parent-id={swimlaneParentId}
  data-status-id={statusId}
>
  <div
    class="border-b border-t-4 p-4"
    style="border-bottom-color: var(--ctx-border, var(--ds-border)); border-top-color: {column.color};"
  >
    <div class="flex items-center justify-between">
      <h3 class="font-semibold" data-testid="column-header" style={textStyle}>{systemStatusName(column.name)}</h3>
      <div class="flex items-center gap-0.5">
        {#if onadd}
          <button
            type="button"
            onclick={onadd}
            class="rounded p-1 text-[var(--ds-text-subtle)] transition-colors hover:text-[var(--ds-text)]"
            data-testid={`board-column-add-${column.id}`}
            title={t('collections.addCard')}
            aria-label={t('collections.addCard')}
          >
            <Plus class="h-4 w-4" />
          </button>
        {/if}
        {#if oncollapse}
          <button
            type="button"
            onclick={oncollapse}
            class="rounded p-1 text-[var(--ds-text-subtle)] transition-colors hover:text-[var(--ds-text)]"
            title={t('collections.collapseColumn')}
            aria-label={t('collections.collapseColumn')}
            aria-expanded="true"
          >
            <ChevronLeft class="h-4 w-4" />
          </button>
        {/if}
      </div>
    </div>
    <div class="flex items-center justify-between">
      <span class="text-sm" style={subtleTextStyle}>
        {#if hiddenItemCount > 0}
          {visibleItemCount} of {itemCount} {t('items.item')}
        {:else}
          {itemCount} {t('items.item')}
        {/if}
      </span>
      {#if column.wip_limit}
        <span
          class="rounded px-2 py-0.5 text-xs"
          style={isOverWip
            ? 'background-color: var(--ds-danger-subtle); color: var(--ds-text-danger);'
            : 'background-color: var(--ds-background-neutral, #091e420f); color: var(--ds-text-subtle, #6b778c);'}
        >
          WIP: {wipCount}/{column.wip_limit}
        </span>
      {/if}
    </div>
  </div>
  <div class="min-h-32 p-4">
    {@render children?.()}
  </div>
</div>
