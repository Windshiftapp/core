<script>
  import GenericActionListManager from '../actions/shared/GenericActionListManager.svelte';

  let {
    actions = [],
    loading = false,
    oncreate,
    onedit,
    ontoggle,
    ondelete,
    onviewlogs,
  } = $props();

  const triggerLabels = {
    asset_created: 'Asset Created',
    asset_updated: 'Asset Updated',
    asset_status_changed: 'Status Changed',
    manual: 'Manual',
  };
</script>

<GenericActionListManager
  {actions}
  {loading}
  {triggerLabels}
  headerTitle="Automations"
  headerSubtitle="Automate workflows triggered by asset events"
  emptyStateDescription="Create an action to automate workflows when assets are created or updated."
  {oncreate}
  {onedit}
  {ontoggle}
  {ondelete}
  {onviewlogs}
/>
