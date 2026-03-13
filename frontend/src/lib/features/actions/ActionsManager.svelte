<script>
  import { createEventDispatcher } from 'svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { confirm } from '../../composables/useConfirm.js';
  import { toHotkeyString, getShortcutDisplay } from '../../utils/keyboardShortcuts.js';
  import Button from '../../components/Button.svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import TestActionModal from './TestActionModal.svelte';
  import { Plus, Play, Zap } from 'lucide-svelte';
  import PageHeader from '../../layout/PageHeader.svelte';

  export let workspaceId;
  export let actions = [];
  export let loading = false;

  const dispatch = createEventDispatcher();

  // Test action modal state
  let showTestModal = false;
  let testAction = null;

  function handleTest(action) {
    testAction = action;
    showTestModal = true;
  }

  function closeTestModal() {
    showTestModal = false;
    testAction = null;
  }

  function getTriggerTypeLabel(triggerType) {
    const labels = {
      'status_transition': t('actions.trigger.statusTransition'),
      'item_created': t('actions.trigger.itemCreated'),
      'item_updated': t('actions.trigger.itemUpdated'),
      'item_linked': t('actions.trigger.itemLinked')
    };
    return labels[triggerType] || triggerType;
  }

  function handleCreate() {
    dispatch('create');
  }

  function handleEdit(action) {
    dispatch('edit', action);
  }

  function handleToggle(action) {
    dispatch('toggle', action);
  }

  async function handleDelete(action) {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('actions.confirmDelete', { name: action.name }),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger'
    });
    if (confirmed) {
      dispatch('delete', action);
    }
  }

  function handleViewLogs(action) {
    dispatch('viewLogs', action);
  }
</script>

<div class="actions-manager">
  <PageHeader
    title={t('actions.title')}
    subtitle={t('actions.description')}
  >
    {#snippet actions()}
      <Button
        variant="primary"
        icon={Plus}
        keyboardHint={getShortcutDisplay('actions', 'add')}
        hotkeyConfig={{ key: toHotkeyString('actions', 'add') }}
        onclick={handleCreate}
      >
        {t('actions.create')}
      </Button>
    {/snippet}
  </PageHeader>

  {#if loading}
    <div class="flex items-center justify-center py-12">
      <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600"></div>
    </div>
  {:else if actions.length === 0}
    <EmptyState
      icon={Zap}
      title={t('actions.noActions')}
      description={t('actions.noActionsDescription')}
    >
      {#snippet action()}
        <Button
          variant="primary"
          icon={Plus}
          keyboardHint={getShortcutDisplay('actions', 'add')}
          onclick={handleCreate}
        >
          {t('actions.createFirst')}
        </Button>
      {/snippet}
    </EmptyState>
  {:else}
    <div class="actions-list space-y-3">
      {#each actions as action}
        <div class="action-card p-4 border rounded-lg">
          <div class="flex items-center justify-between">
            <div class="flex-1">
              <div class="flex items-center gap-3">
                <span
                  class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium {action.is_enabled ? 'bg-green-100 text-green-800' : 'bg-gray-100 text-gray-800'}"
                >
                  {action.is_enabled ? t('actions.enabled') : t('actions.disabled')}
                </span>
                <h3 class="text-base font-medium action-name">{action.name}</h3>
              </div>
              {#if action.description}
                <p class="mt-1 text-sm action-description">{action.description}</p>
              {/if}
              <div class="mt-2 flex items-center gap-4 text-xs action-meta">
                <span class="flex items-center gap-1">
                  <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
                  </svg>
                  {getTriggerTypeLabel(action.trigger_type)}
                </span>
                {#if action.creator_name}
                  <span class="flex items-center gap-1">
                    <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
                    </svg>
                    {action.creator_name}
                  </span>
                {/if}
              </div>
            </div>
            <div class="flex items-center gap-2">
              <button
                class="p-2 rounded-md action-button"
                onclick={() => handleTest(action)}
                title={t('actions.test.run')}
              >
                <Play class="h-5 w-5" />
              </button>
              <button
                class="p-2 rounded-md action-button"
                onclick={() => handleViewLogs(action)}
                title={t('actions.viewLogs')}
              >
                <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
                </svg>
              </button>
              <button
                class="p-2 rounded-md action-button"
                onclick={() => handleToggle(action)}
                title={action.is_enabled ? t('actions.disable') : t('actions.enable')}
              >
                {#if action.is_enabled}
                  <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 9v6m4-6v6m7-3a9 9 0 11-18 0 9 9 0 0118 0z" />
                  </svg>
                {:else}
                  <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z" />
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                  </svg>
                {/if}
              </button>
              <button
                class="p-2 rounded-md action-button"
                onclick={() => handleEdit(action)}
                title={t('common.edit')}
              >
                <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
                </svg>
              </button>
              <button
                class="p-2 rounded-md action-button-danger"
                onclick={() => handleDelete(action)}
                title={t('common.delete')}
              >
                <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                </svg>
              </button>
            </div>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>

{#if showTestModal && testAction}
  <TestActionModal
    action={testAction}
    {workspaceId}
    onclose={closeTestModal}
    onsuccess={() => {
      // Optionally refresh logs or show toast
      dispatch('testExecuted', testAction);
    }}
  />
{/if}

<style>
  .actions-manager {
    padding: 1.5rem;
  }

  .action-card {
    background-color: var(--ds-surface-raised);
    border-color: var(--ds-border);
  }

  .action-card:hover {
    border-color: var(--ds-border-bold);
  }

  .action-name {
    color: var(--ds-text);
  }

  .action-description {
    color: var(--ds-text-subtle);
  }

  .action-meta {
    color: var(--ds-text-subtlest);
  }

  .action-button {
    color: var(--ds-text-subtle);
    background-color: transparent;
  }

  .action-button:hover {
    color: var(--ds-text);
    background-color: var(--ds-surface-hovered);
  }

  .action-button-danger {
    color: var(--ds-text-subtle);
    background-color: transparent;
  }

  .action-button-danger:hover {
    color: var(--ds-danger);
    background-color: var(--ds-danger-subtle);
  }
</style>
