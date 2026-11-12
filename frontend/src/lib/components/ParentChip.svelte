<script>
  import CornerLeftUp from '@lucide/svelte/icons/corner-left-up';
  import Chip from './Chip.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { hasParentReference, resolveParentKey } from '../utils/parentReference.js';

  /**
   * Parent reference chip for a work item. Renders nothing when the item has
   * no renderable parent (a masked parent, or summary data that omits it).
   */
  let {
    item,
    maxWidth = 'clamp(6rem, 20%, 16rem)',
    testId = null,
    containerTestId = null,
  } = $props();

  const parentKey = $derived(resolveParentKey(item));
  const parentTitle = $derived(item.parent_title || '');
  const parentLabel = $derived([parentKey, parentTitle].filter(Boolean).join(' · '));
  const resolvedTestId = $derived(testId ?? `work-item-parent-${item.id}`);
  const resolvedContainerTestId = $derived(containerTestId ?? `work-item-parent-container-${item.id}`);
</script>

{#if hasParentReference(item)}
  <span
    class="inline-flex min-w-0 shrink overflow-hidden"
    style="max-width: {maxWidth};"
    data-testid={resolvedContainerTestId}
  >
    <Chip
      appearance="metadata"
      icon={CornerLeftUp}
      class="max-w-full min-w-0 overflow-hidden"
      title={`${t('items.parent')}: ${parentLabel}`}
    >
      <span data-testid={resolvedTestId} class="inline-flex min-w-0 items-center gap-1 overflow-hidden">
        {#if parentKey}
          <span class="shrink-0 font-mono">{parentKey}</span>
        {/if}
        {#if parentTitle}
          <span class="min-w-0 truncate">{parentTitle}</span>
        {/if}
      </span>
    </Chip>
  </span>
{/if}
