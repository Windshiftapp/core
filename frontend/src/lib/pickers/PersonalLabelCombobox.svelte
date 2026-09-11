<script>
  import { api } from '../api.js';
  import { authStore } from '../stores';
  import ScopedLabelCombobox from './ScopedLabelCombobox.svelte';

  let {
    value = $bindable(/** @type {string[] | string} */ ([])),
    placeholder = '',
    class: className = '',
    disabled = false,
    userId = undefined,
    labels = null,
    loading = false,
    onOpen = null,
    onClose = null,
    onSelect = () => {},
    onCancel = () => {},
  } = $props();

  const loadKey = $derived(userId === undefined ? 'current' : (userId ?? 'shared'));

  function createLabel(name) {
    const createUserId = userId === undefined ? (authStore.currentUser?.id ?? null) : userId;
    return api.personalLabels.create({ name, user_id: createUserId });
  }
</script>

<ScopedLabelCombobox
  bind:value
  {placeholder}
  class={className}
  {disabled}
  {labels}
  {loading}
  {loadKey}
  loadLabels={() => api.personalLabels.getAll(userId)}
  {createLabel}
  {onOpen}
  {onClose}
  {onSelect}
  {onCancel}
/>
