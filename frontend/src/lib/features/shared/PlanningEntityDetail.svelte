<script>
  import StateDisplay from '../../components/StateDisplay.svelte';
  import BackButton from '../../components/BackButton.svelte';
  import Button from '../../components/Button.svelte';
  import DropdownMenu from '../../layout/DropdownMenu.svelte';
  import ProgressSummary from '../../components/ProgressSummary.svelte';
  import ItemsByStatusCategory from '../../components/ItemsByStatusCategory.svelte';
  import { t } from '../../stores/i18n.svelte.js';

  let {
    loading = false,
    error = null,
    progress = null,
    backLabel = '',
    onBack,
    dropdownIcon,
    dropdownItems = [],
    onRetry,
    summary = {},
    items = {},
    expandedCategories = {},
    ontoggle,
    header = null,
    afterSummary = null,
  } = $props();
</script>

<div class="flex min-h-screen" style="background-color: var(--ds-surface);">
  <div class="flex-1 max-w-5xl mx-auto p-6">
    <!-- Header -->
    <div class="flex items-center justify-between mb-6">
      <BackButton onclick={onBack} label={backLabel} />

      {#if progress}
        <DropdownMenu
          triggerIcon={dropdownIcon}
          triggerClass="w-8 h-8 flex items-center justify-center rounded-md transition-colors"
          triggerStyle="background-color: var(--ds-surface); color: var(--ds-text-subtle);"
          items={dropdownItems}
          maxWidth="max-w-48"
          showChevron={false}
          iconOnly={true}
        />
      {/if}
    </div>

    {#if loading}
      <StateDisplay type="loading" />
    {:else if error}
      <div class="text-center py-20">
        <p class="text-ds-text-danger">{error}</p>
        <Button onclick={onRetry} class="mt-4">{t('common.retry')}</Button>
      </div>
    {:else if progress}
      <!-- Entity Header Card -->
      <div
        class="rounded-xl border p-6 mb-6"
        style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
      >
        {@render header?.()}
      </div>

      <ProgressSummary
        {progress}
        ariaLabel={summary.ariaLabel}
        completeLabel={summary.completeLabel}
        noItemsLabel={summary.noItemsLabel}
        summaryLabel={summary.summaryLabel}
        totalLabel={summary.totalLabel}
        completedLabel={summary.completedLabel}
        remainingLabel={summary.remainingLabel}
        statusLabel={summary.statusLabel}
        noStatusDataLabel={summary.noStatusDataLabel}
      />

      {@render afterSummary?.()}

      <!-- Items Grouped by Category -->
      <ItemsByStatusCategory
        statusBreakdown={progress.status_breakdown}
        itemsByCategory={progress.items_by_category}
        {expandedCategories}
        title={items.title}
        emptyIcon={items.emptyIcon}
        emptyTitle={items.emptyTitle}
        emptyDescription={items.emptyDescription}
        ontoggle={ontoggle}
      />
    {/if}
  </div>
</div>
