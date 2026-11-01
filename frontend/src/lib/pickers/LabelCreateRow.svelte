<script>
  import Plus from '@lucide/svelte/icons/plus';
  import { t } from '../stores/i18n.svelte.js';

  // The "no results" block of a label picker, offering inline creation.
  let { searchQuery, oncreate, testid = 'label-picker-create' } = $props();
</script>

<div class="p-3 text-sm text-center" style="color: var(--ds-text-subtle);">
  <div class="space-y-2">
    <div>{t('pickers.noLabelsFoundFor', { query: searchQuery })}</div>
    <button
      type="button"
      data-testid={testid}
      class="flex items-center gap-2 px-3 py-1 rounded transition-colors mx-auto"
      style="background-color: var(--ds-background-accent-blue-subtlest); color: var(--ds-interactive);"
      onclick={() => oncreate(searchQuery)}
    >
      <Plus class="w-4 h-4" />
      {t('pickers.createItem', { value: searchQuery })}
    </button>
  </div>
</div>
