<script>
  import { api } from '../api.js';
  import ScopedLabelCombobox from './ScopedLabelCombobox.svelte';

  let {
    workspaceId,
    id = undefined,
    testidPrefix = 'workspace-label-picker',
    value = $bindable(/** @type {string[] | string} */ ([])),
    placeholder = '',
    class: className = '',
    disabled = false,
    labels = null,
    loading = false,
    onOpen = null,
    onClose = null,
    onSelect = () => {},
    onCancel = () => {},
  } = $props();

  function loadLabels() {
    return workspaceId ? api.labels.getAll(workspaceId) : [];
  }

  function createLabel(name) {
    const scopedWorkspaceId = Number(workspaceId);
    if (!Number.isFinite(scopedWorkspaceId) || scopedWorkspaceId <= 0) return null;
    return api.labels.create({ name, workspace_id: scopedWorkspaceId });
  }
</script>

<ScopedLabelCombobox
  {id}
  {testidPrefix}
  bind:value
  {placeholder}
  class={className}
  disabled={disabled || !workspaceId}
  {labels}
  {loading}
  loadKey={workspaceId}
  {loadLabels}
  {createLabel}
  {onOpen}
  {onClose}
  {onSelect}
  {onCancel}
/>
