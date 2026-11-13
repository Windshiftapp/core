<script>
  import { onMount } from 'svelte';
  import StateDisplay from '../../components/StateDisplay.svelte';
  import { actions } from '../../api/actions.js';
  import { t } from '../../stores/i18n.svelte.js';
  import DataTable from '../../components/DataTable.svelte';
  import Button from '../../components/Button.svelte';
  import BackButton from '../../components/BackButton.svelte';
  import Badge from '../../components/Badge.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import Eye from '@lucide/svelte/icons/eye';
  import History from '@lucide/svelte/icons/history';
  import { formatAuthenticatedDateTime } from '../../utils/authenticatedDateFormatter.js';
  import { surfaceError } from '../../stores/toasts.svelte.js';

  let { workspaceId, action, onBack } = $props();

  let revisions = $state([]);
  let loading = $state(true);
  let selectedRevision = $state(null);
  let detailLoading = $state(false);

  const columns = [
    { key: 'revision_number', label: t('actions.history.revision'), render: (item) => `#${item.revision_number}`, width: '90px' },
    { key: 'source', label: t('actions.history.source'), slot: 'source', width: '150px' },
    { key: 'editor_username', label: t('actions.history.editor'), slot: 'editor' },
    { key: 'changed_fields', label: t('actions.history.changedFields'), slot: 'changed' },
    { key: 'created_at', label: t('actions.history.createdAt'), slot: 'date', width: '180px' },
    { key: 'details', label: t('actions.logs.details'), slot: 'details', width: '100px' },
  ];

  onMount(loadRevisions);

  async function loadRevisions() {
    loading = true;
    try {
      revisions = (await actions.getRevisions(workspaceId, action.id)) || [];
    } catch (err) {
      surfaceError(err, { context: 'load action revisions' });
      revisions = [];
    } finally {
      loading = false;
    }
  }

  async function showDetails(revision) {
    selectedRevision = revision;
    detailLoading = true;
    try {
      selectedRevision = await actions.getRevision(workspaceId, action.id, revision.id);
    } catch (err) {
      surfaceError(err, { context: 'load action revision' });
    } finally {
      detailLoading = false;
    }
  }

  function closeDetails() {
    selectedRevision = null;
  }

  function formatDate(dateStr) {
    if (!dateStr) return '-';
    return formatAuthenticatedDateTime(dateStr);
  }

  function changedFields(revision) {
    const fields = revision?.changed_fields || [];
    return fields.length > 0 ? fields.join(', ') : t('actions.history.noChangedFields');
  }

  function sourceLabel(source) {
    return source || 'update';
  }
</script>

<div class="action-history h-full flex flex-col" data-testid="action-history">
  <div class="flex items-center gap-4 p-4 border-b header">
    <BackButton onclick={onBack} tooltip={t('common.back')} class="p-1" />
    <div class="flex-1">
      <h1 class="text-xl font-medium title">{t('actions.history.title')}</h1>
      <p class="text-sm subtitle">{action.name}</p>
    </div>
  </div>

  <div class="flex-1 p-4 overflow-auto">
    {#if loading}
      <StateDisplay type="loading" />
    {:else}
      <DataTable
        {columns}
        data={revisions}
        keyField="id"
        emptyMessage={t('actions.history.noRevisions')}
        emptyIcon={History}
        pagination={true}
        pageSize={25}
        rowAttrs={(item) => ({ 'data-testid': `action-history-row-${item.id}` })}
      >
        {#snippet source(item)}
          <Badge variant="neutral">{sourceLabel(item.source)}</Badge>
        {/snippet}

        {#snippet editor(item)}
          {#if item.editor_username}
            <span>{item.editor_username}</span>
          {:else}
            <span class="text-subtle">{t('actions.history.system')}</span>
          {/if}
        {/snippet}

        {#snippet changed(item)}
          <span class="changed-fields">{changedFields(item)}</span>
        {/snippet}

        {#snippet date(item)}
          <span class="date-text whitespace-nowrap">{formatDate(item.created_at)}</span>
        {/snippet}

        {#snippet details(item)}
          <Button
            variant="ghost"
            size="sm"
            onclick={() => showDetails(item)}
            title={t('actions.history.viewDetails')}
            dataTestid={`action-history-details-${item.id}`}
          >
            <Eye class="w-4 h-4" />
          </Button>
        {/snippet}
      </DataTable>
    {/if}
  </div>
</div>

{#if selectedRevision}
  <Modal isOpen={true} onclose={closeDetails} maxWidth="max-w-2xl">
    <ModalHeader
      title={t('actions.history.detailTitle')}
      subtitle={`${action.name} — #${selectedRevision.revision_number}`}
      onClose={closeDetails}
    />
    <div class="p-6 space-y-4" data-testid="action-revision-detail">
      {#if detailLoading}
        <StateDisplay type="loading" />
      {:else}
        <div class="flex flex-wrap items-center gap-2">
          <Badge variant="neutral">{sourceLabel(selectedRevision.source)}</Badge>
          <span class="text-sm text-subtle">
            {selectedRevision.editor_username || t('actions.history.system')}
          </span>
          <span class="text-sm text-subtle">{formatDate(selectedRevision.created_at)}</span>
        </div>

        <div>
          <div class="field-label">{t('actions.history.changedFields')}</div>
          <div class="changed-fields" data-testid="action-revision-changed-fields">
            {changedFields(selectedRevision)}
          </div>
        </div>

        <div>
          <div class="field-label">{t('actions.history.snapshot')}</div>
          <div class="snapshot">
            <div class="snapshot-row">
              <span class="snapshot-key">{t('common.name')}</span>
              <span class="snapshot-value" data-testid="action-revision-name">
                {selectedRevision.snapshot?.name}
              </span>
            </div>
            <div class="snapshot-row">
              <span class="snapshot-key">{t('actions.history.triggerType', 'Trigger')}</span>
              <span class="snapshot-value">{selectedRevision.snapshot?.trigger_type}</span>
            </div>
            <div class="snapshot-row">
              <span class="snapshot-key">{t('actions.history.nodes')}</span>
              <span class="snapshot-value">
                {selectedRevision.snapshot?.nodes?.length ?? 0}
              </span>
            </div>
            <div class="snapshot-row">
              <span class="snapshot-key">{t('actions.history.edges')}</span>
              <span class="snapshot-value">
                {selectedRevision.snapshot?.edges?.length ?? 0}
              </span>
            </div>
          </div>
        </div>

        <div>
          <div class="field-label">{t('actions.history.nodes')}</div>
          <ul class="node-list">
            {#each selectedRevision.snapshot?.nodes ?? [] as node, index}
              <li class="node-item" data-testid={`action-revision-node-${index}`}>
                <span class="node-type">{node.node_type}</span>
                <code class="node-config">{node.node_config}</code>
              </li>
            {/each}
          </ul>
        </div>
      {/if}
    </div>
  </Modal>
{/if}

<style>
  .action-history {
    background-color: var(--ds-surface);
  }

  .header {
    border-color: var(--ds-border);
    background-color: var(--ds-surface-raised);
  }

  .title {
    color: var(--ds-text);
  }

  .subtitle {
    color: var(--ds-text-subtle);
  }

  .text-subtle {
    color: var(--ds-text-subtle);
  }

  .changed-fields {
    color: var(--ds-text-subtle);
    font-size: 0.8125rem;
  }

  .date-text {
    color: var(--ds-text);
  }

  .field-label {
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.03em;
    color: var(--ds-text-subtlest);
    margin-bottom: 0.375rem;
  }

  .snapshot {
    border: 1px solid var(--ds-border);
    border-radius: 0.375rem;
    overflow: hidden;
  }

  .snapshot-row {
    display: flex;
    justify-content: space-between;
    gap: 1rem;
    padding: 0.5rem 0.75rem;
    border-bottom: 1px solid var(--ds-border);
    font-size: 0.8125rem;
  }

  .snapshot-row:last-child {
    border-bottom: none;
  }

  .snapshot-key {
    color: var(--ds-text-subtle);
  }

  .snapshot-value {
    color: var(--ds-text);
    font-weight: 500;
  }

  .node-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .node-item {
    display: flex;
    align-items: flex-start;
    gap: 0.75rem;
    padding: 0.5rem 0.75rem;
    border: 1px solid var(--ds-border);
    border-radius: 0.375rem;
  }

  .node-type {
    font-weight: 600;
    font-size: 0.8125rem;
    color: var(--ds-text);
    flex-shrink: 0;
  }

  .node-config {
    font-family: ui-monospace, monospace;
    font-size: 0.75rem;
    color: var(--ds-text-subtle);
    word-break: break-all;
    white-space: pre-wrap;
  }
</style>
