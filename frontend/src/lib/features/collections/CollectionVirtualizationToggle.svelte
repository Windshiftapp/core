<script>
  import Checkbox from '../../components/Checkbox.svelte';
  import { t } from '../../stores/i18n.svelte.js';
  let { preference } = $props();
</script>

<Checkbox
  checked={preference.enabled}
  onchange={(enabled) => preference.save(enabled)}
  label={t('common.virtualizeRows')}
  hint={t('common.virtualizeRowsHint')}
  size="small"
  dataTestid="virtualize-rows"
/>
