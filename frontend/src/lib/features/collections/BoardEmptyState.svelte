<script>
  import Plus from '@lucide/svelte/icons/plus';
  import { t } from '../../stores/i18n.svelte.js';

  let {
    text = t('items.noItems'),
    textStyle = 'color: var(--ds-text-subtle);',
  } = $props();
</script>

<div class="py-8 text-center" style={textStyle}>
  <Plus class="mx-auto mb-2 h-8 w-8" />
  <p class="text-sm">{text}</p>
</div>
