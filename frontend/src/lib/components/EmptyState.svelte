<script>
  import { Inbox } from 'lucide-svelte';
  import { t } from '../stores/i18n.svelte.js';

  let {
    icon: IconComponent = Inbox,
    title = '',
    description = '',
    action = null,
    class: className = ''
  } = $props();
</script>

<div class="text-center py-12 {className}">
  {#if IconComponent}
    <IconComponent class="w-6 h-6 mx-auto mb-3" style="color: var(--ds-icon-disabled);" />
  {/if}
  <h3 class="text-lg font-medium mb-2" style="color: var(--ds-text-subtle);">
    {title || t('common.noData')}
  </h3>
  {#if description}
    <p class="text-sm" style="color: var(--ds-text-subtle);">
      {description}
    </p>
  {/if}
  {#if action}
    <div class="mt-4">
      {@render action()}
    </div>
  {/if}
</div>
