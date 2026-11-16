<script>
  import { itemTypeIconMap } from '../utils/icons.js';
  import { objectDisplayName } from '../utils/systemLabels.js';

  let {
    itemType = null,
    icon = null,
    color = null,
    title = undefined,
    ariaLabel = undefined,
    testId = undefined,
    class: className = ''
  } = $props();

  const resolvedIconName = $derived(String(icon || itemType?.icon || ''));
  const ResolvedIcon = $derived(itemTypeIconMap[resolvedIconName] || itemTypeIconMap.FileText);
  const resolvedColor = $derived(color || itemType?.color || 'var(--ds-icon-subtle, #6b7280)');
  const resolvedTitle = $derived(title ?? (objectDisplayName(itemType, 'item_type') || undefined));
</script>

<span
  class="item-type-icon {className}"
  style="color: {resolvedColor};"
  title={resolvedTitle}
  aria-label={ariaLabel}
  aria-hidden={ariaLabel ? undefined : 'true'}
  data-testid={testId}
>
  <ResolvedIcon size={16} strokeWidth={2} />
</span>

<style>
  .item-type-icon {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 16px;
    height: 16px;
    flex-shrink: 0;
    line-height: 0;
  }
</style>
