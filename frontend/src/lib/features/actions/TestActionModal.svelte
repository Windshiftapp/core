<script>
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import Button from '../../components/Button.svelte';
  import ItemPicker from '../../pickers/ItemPicker.svelte';
  import { api } from '../../api.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { Play } from 'lucide-svelte';

  let { action, workspaceId, onclose, onsuccess } = $props();

  let selectedItemId = $state(null);
  let items = $state([]);
  let loading = $state(false);
  let executing = $state(false);
  let error = $state(null);

  // Search for items when the picker's search term changes
  async function handleSearchChange(term) {
    if (!term || term.length < 2) {
      items = [];
      return;
    }

    loading = true;
    try {
      const results = await api.search.items({
        query: term,
        workspaceIds: [workspaceId],
        limit: 20
      });
      items = results || [];
    } catch (err) {
      console.error('Failed to search items:', err);
      items = [];
    } finally {
      loading = false;
    }
  }

  async function handleExecute() {
    if (!selectedItemId) return;

    executing = true;
    error = null;

    try {
      await api.post(`/workspaces/${workspaceId}/actions/${action.id}/execute`, {
        item_id: selectedItemId
      });
      onsuccess?.();
      onclose();
    } catch (err) {
      error = err.message || t('actions.test.executionFailed');
    } finally {
      executing = false;
    }
  }

  const itemConfig = {
    primary: { text: (item) => item.title || `#${item.id}` },
    secondary: { text: (item) => item.status_name || '' },
    searchFields: ['title', 'id'],
    getValue: (item) => item.id,
    getLabel: (item) => item.title || `#${item.id}`
  };
</script>

<Modal isOpen={true} onclose={onclose} maxWidth="max-w-md">
  <ModalHeader
    title={t('actions.test.title')}
    subtitle={action.name}
    icon={Play}
    onClose={onclose}
  />

  <div class="p-6">
    <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">
      {t('actions.test.description')}
    </p>

    <div class="mb-4">
      <label class="block text-sm font-medium mb-2" style="color: var(--ds-text);">
        {t('actions.test.selectItem')}
      </label>
      <ItemPicker
        bind:value={selectedItemId}
        {items}
        config={itemConfig}
        placeholder={t('actions.test.itemPlaceholder')}
        {loading}
        onSearchChange={handleSearchChange}
      />
    </div>

    {#if error}
      <div class="mb-4 p-3 rounded text-sm" style="background: var(--ds-error-subtle); color: var(--ds-error);">
        {error}
      </div>
    {/if}

    <div class="flex justify-end gap-3">
      <Button variant="ghost" onclick={onclose}>
        {t('common.cancel')}
      </Button>
      <Button
        variant="primary"
        onclick={handleExecute}
        disabled={!selectedItemId || executing}
        loading={executing}
      >
        {t('actions.test.execute')}
      </Button>
    </div>
  </div>
</Modal>
