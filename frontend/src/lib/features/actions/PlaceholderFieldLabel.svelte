<script>
  import HelpCircle from '@lucide/svelte/icons/help-circle';
  import { t } from '../../stores/i18n.svelte.js';

  /**
   * Label row with the "show placeholder reference" help button used by the
   * action config panels.
   *
   * @type {{
   *   forId?: string,
   *   label: string,
   *   required?: boolean,
   *   onShowReference: () => void,
   * }}
   */
  let { forId = undefined, label, required = false, onShowReference } = $props();
</script>

<div class="flex items-center gap-1 mb-1">
  <label for={forId} class="block text-xs font-medium">{label}</label>
  {#if required}
    <span class="text-ds-text-danger text-xs">*</span>
  {/if}
  <button
    type="button"
    onclick={onShowReference}
    class="text-[var(--ds-text-subtlest)] hover:text-[var(--ds-interactive)] transition-colors"
    title={t('actions.placeholders.showReference')}
  >
    <HelpCircle class="w-3.5 h-3.5" />
  </button>
</div>
