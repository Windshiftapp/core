<script>
  import Button from '../components/Button.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let {
    hasUnsavedChanges = false,
    showUnsaved = true,
    saving = false,
    onCancel,
    onSave,
    saveTestid = undefined,
    saveHotkey = undefined,
  } = $props();
</script>

{#if showUnsaved && hasUnsavedChanges}
  <span class="text-sm" style="color: var(--ds-text-subtle);">
    {t('settings.configSets.unsavedChanges')}
  </span>
{/if}
<Button variant="ghost" onclick={onCancel}>
  {t('common.cancel')}
</Button>
<Button
  variant="primary"
  onclick={onSave}
  disabled={saving || !hasUnsavedChanges}
  dataTestid={saveTestid}
  hotkeyConfig={saveHotkey}
>
  {saving ? t('common.saving') : t('common.save')}
</Button>
