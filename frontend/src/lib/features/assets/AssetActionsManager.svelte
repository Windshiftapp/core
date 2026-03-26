<script>
  import { Zap, Eye, ToggleLeft, ToggleRight, Pencil, Trash2, Plus } from 'lucide-svelte';
  import Button from '../../components/Button.svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import { confirm } from '../../composables/useConfirm.js';
  import { t } from '../../stores/i18n.svelte.js';

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

  async function handleDelete(action) {
    const confirmed = await confirm({
      title: 'Delete Action',
      message: `Are you sure you want to delete "${action.name}"? This cannot be undone.`,
      confirmLabel: t('common.delete'),
      variant: 'danger',
    });
    if (confirmed) {
      ondelete?.(action);
    }
  }
</script>

<div class="actions-manager">
  <div class="header">
    <div class="header-text">
      <h3>Automations</h3>
      <p class="subtitle">Automate workflows triggered by asset events</p>
    </div>
    {#if oncreate}
      <Button variant="primary" size="small" onclick={oncreate}>
        <Plus size={14} />
        New Action
      </Button>
    {/if}
  </div>

  {#if loading}
    <div class="loading">Loading actions...</div>
  {:else if actions.length === 0}
    <EmptyState
      icon={Zap}
      title="No actions yet"
      description="Create an action to automate workflows when assets are created or updated."
    />
  {:else}
    <div class="actions-list">
      {#each actions as action (action.id)}
        <div class="action-card">
          <div class="action-info">
            <div class="action-header">
              <span
                class="status-badge"
                class:enabled={action.is_enabled}
                class:disabled={!action.is_enabled}
              >
                {action.is_enabled ? 'Enabled' : 'Disabled'}
              </span>
              <span class="action-name">{action.name}</span>
            </div>
            {#if action.description}
              <p class="action-description">{action.description}</p>
            {/if}
            <div class="action-meta">
              <span class="trigger-type">
                <Zap size={12} />
                {triggerLabels[action.trigger_type] || action.trigger_type}
              </span>
            </div>
          </div>
          <div class="action-controls">
            {#if onviewlogs}
              <Button
                variant="ghost"
                size="small"
                title="View Logs"
                onclick={() => onviewlogs(action)}
              >
                <Eye size={14} />
              </Button>
            {/if}
            {#if ontoggle}
              <Button
                variant="ghost"
                size="small"
                title={action.is_enabled ? 'Disable' : 'Enable'}
                onclick={() => ontoggle(action)}
              >
                {#if action.is_enabled}
                  <ToggleRight size={14} class="text-success" />
                {:else}
                  <ToggleLeft size={14} />
                {/if}
              </Button>
            {/if}
            {#if onedit}
              <Button variant="ghost" size="small" title="Edit" onclick={() => onedit(action)}>
                <Pencil size={14} />
              </Button>
            {/if}
            <Button variant="ghost" size="small" title="Delete" onclick={() => handleDelete(action)}>
              <Trash2 size={14} class="text-danger" />
            </Button>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .actions-manager {
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  .header {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .header-text h3 {
    font-size: 16px;
    font-weight: 600;
    color: var(--ds-text);
    margin: 0;
  }

  .subtitle {
    font-size: 13px;
    color: var(--ds-text-subtle);
    margin: 4px 0 0;
  }

  .loading {
    text-align: center;
    padding: 32px;
    color: var(--ds-text-subtle);
  }

  .actions-list {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .action-card {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 12px 16px;
    background: var(--ds-surface-raised);
    border: 1px solid var(--ds-border);
    border-radius: 8px;
    gap: 16px;
  }

  .action-info {
    flex: 1;
    min-width: 0;
  }

  .action-header {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .status-badge {
    font-size: 11px;
    font-weight: 500;
    padding: 2px 8px;
    border-radius: 10px;
    flex-shrink: 0;
  }

  .status-badge.enabled {
    background-color: var(--ds-success-subtle);
    color: var(--ds-success);
  }

  .status-badge.disabled {
    background-color: var(--ds-surface-sunken);
    color: var(--ds-text-subtlest);
  }

  .action-name {
    font-size: 14px;
    font-weight: 500;
    color: var(--ds-text);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .action-description {
    font-size: 12px;
    color: var(--ds-text-subtle);
    margin: 4px 0 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .action-meta {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-top: 6px;
  }

  .trigger-type {
    display: flex;
    align-items: center;
    gap: 4px;
    font-size: 11px;
    color: var(--ds-text-subtlest);
  }

  .action-controls {
    display: flex;
    align-items: center;
    gap: 2px;
    flex-shrink: 0;
  }

  :global(.text-success) {
    color: var(--ds-success);
  }

  :global(.text-danger) {
    color: var(--ds-danger);
  }
</style>
