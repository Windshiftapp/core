<script>
  import { t } from '../stores/i18n.svelte.js';
  import { formatItemCount } from '../utils/itemCount.js';
  import BaseHeader from './BaseHeader.svelte';

  let {
    workspaceName = '',
    collection = '',
    viewName = '',
    itemCount = null,
    shownCount = null,
    actionButtons = null,
    hasGradient = false,
    textStyle = '',
    subtleTextStyle = '',
    actions = null,
  } = $props();

  let countText = $derived(formatItemCount(itemCount, shownCount, t));
  let subtitle = $derived([workspaceName, countText].filter(Boolean).join(' • '));
</script>

<BaseHeader
  title={viewName}
  badge={collection}
  {subtitle}
  {actions}
  textStyle={textStyle || 'color: var(--ctx-text, var(--ds-text));'}
  subtitleStyle={subtleTextStyle || 'color: var(--ctx-text-subtle, var(--ds-text-subtle));'}
  icon={null}
  count={null}
  children={null}
  marginClass="mb-4"
/>