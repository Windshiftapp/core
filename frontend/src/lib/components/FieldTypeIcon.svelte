<script>
  import { fieldTypeMeta } from '../utils/fieldTypeMeta.js';

  // Renders the icon for a field type, with the human-readable type name as
  // the tooltip/accessible label. Used wherever fields are listed so a custom
  // field's type is visible at a glance.
  let { fieldType } = $props();

  const meta = $derived(fieldTypeMeta(fieldType));
</script>

{#if meta}
  {@const Icon = meta.icon}
  <span
    title={meta.label}
    aria-label={meta.label}
    class="inline-flex flex-shrink-0 items-center"
  >
    <Icon class="w-4 h-4 flex-shrink-0" style="color: {meta.iconColor};" />
  </span>
{/if}
