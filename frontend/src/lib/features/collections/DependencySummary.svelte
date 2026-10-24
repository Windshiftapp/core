<script>
  import AlertTriangle from '@lucide/svelte/icons/alert-triangle';
  import GitBranch from '@lucide/svelte/icons/git-branch';
  import { t } from '../../stores/i18n.svelte.js';
  import Tooltip from '../../components/Tooltip.svelte';
  import { splitDependencies, dependencyKey } from './dependencySummary.js';

  // Board-card hover summary for Depends On links, rendered only when present.
  let {
    item,
    links = [],
    maxTitles = 4,
  } = $props();

  let currentItemId = $derived(Number(item?.id));

  let dependency = $derived(splitDependencies(links, currentItemId));

  let hasDependencies = $derived(dependency.blockers.length > 0 || dependency.blocking.length > 0);

  function keyOf(linked) {
    return dependencyKey(linked);
  }
</script>

{#if hasDependencies}
  <Tooltip
    placement="top"
    delay={{ open: 200, close: 0 }}
    class="inline-flex"
    contentClass="px-3 py-2 text-xs max-w-xs"
  >
    {#snippet tip()}
      {#if dependency.blockers.length > 0}
        <div class="mb-1.5">
          <div class="font-semibold mb-1" style="color: #fbbf24;">
            {t('collections.blockedBy')} ({dependency.blockers.length})
          </div>
          <ul class="space-y-0.5">
            {#each dependency.blockers.slice(0, maxTitles) as linked (linked.id)}
              <li class="flex items-center gap-1.5">
                <span class="font-mono opacity-70 flex-shrink-0">{keyOf(linked)}</span>
                <span class="truncate">{linked.title}</span>
              </li>
            {/each}
            {#if dependency.blockers.length > maxTitles}
              <li class="opacity-70">+{dependency.blockers.length - maxTitles}</li>
            {/if}
          </ul>
        </div>
      {/if}
      {#if dependency.blocking.length > 0}
        <div>
          <div class="font-semibold mb-1" style="color: #93c5fd;">
            {t('collections.blocking')} ({dependency.blocking.length})
          </div>
          <ul class="space-y-0.5">
            {#each dependency.blocking.slice(0, maxTitles) as linked (linked.id)}
              <li class="flex items-center gap-1.5">
                <span class="font-mono opacity-70 flex-shrink-0">{keyOf(linked)}</span>
                <span class="truncate">{linked.title}</span>
              </li>
            {/each}
            {#if dependency.blocking.length > maxTitles}
              <li class="opacity-70">+{dependency.blocking.length - maxTitles}</li>
            {/if}
          </ul>
        </div>
      {/if}
    {/snippet}

    <span
      class="inline-flex items-center gap-2 text-[11px] leading-4"
      style="color: var(--ds-text-subtle);"
      data-testid={`board-card-dependency-summary-${currentItemId}`}
    >
      {#if dependency.blockers.length > 0}
        <span class="inline-flex items-center gap-1">
          <AlertTriangle class="h-3 w-3 shrink-0" style="color: var(--ds-icon-warning);" />
          <span>{t('collections.blockersCount', { count: dependency.blockers.length })}</span>
        </span>
      {/if}
      {#if dependency.blocking.length > 0}
        <span class="inline-flex items-center gap-1">
          <GitBranch class="h-3 w-3 shrink-0" style="color: var(--ds-icon-info);" />
          <span>{t('collections.blockingCount', { count: dependency.blocking.length })}</span>
        </span>
      {/if}
    </span>
  </Tooltip>
{/if}
