<script>
  import { createEventDispatcher } from 'svelte';
  import ConfigurationSetEntityPicker from '../pickers/ConfigurationSetEntityPicker.svelte';

  export let priorities = [];
  export let selectedPriorityIds = [];
  export let configurationSetId = null;

  const dispatch = createEventDispatcher();

  function handleChange(event) {
    dispatch('change', event.detail);
  }
</script>

<div>
  <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">
    Select which priorities are available in workspaces using this configuration set.
  </p>

  <ConfigurationSetEntityPicker
    entityType="priorities"
    allEntities={priorities}
    selectedIds={selectedPriorityIds}
    {configurationSetId}
    on:change={handleChange}
  />
</div>
