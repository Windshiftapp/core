<script>
  import { createEventDispatcher } from 'svelte';
  import { t } from '../stores/i18n.svelte.js';
  import ConfigurationSetEntityPicker from '../pickers/ConfigurationSetEntityPicker.svelte';

  let {
    priorities = [],
    selectedPriorityIds = [],
    configurationSetId = null
  } = $props();

  const dispatch = createEventDispatcher();

  function handleChange(event) {
    dispatch('change', event.detail);
  }
</script>

<div>
  <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">
    {t('settings.configSets.selectPriorities')}
  </p>

  <ConfigurationSetEntityPicker
    entityType="priorities"
    allEntities={priorities}
    selectedIds={selectedPriorityIds}
    {configurationSetId}
    on:change={handleChange}
  />
</div>
