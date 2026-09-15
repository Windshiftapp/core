<script>
  import { api } from '../../api.js';
  import { formatRelativeTime } from '../../utils/dateFormatter.js';
  import { t } from '../../stores/i18n.svelte.js';
  import Spinner from '../../components/Spinner.svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import { IconX, IconHistory } from '@tabler/icons-svelte-runes';

  let {
    workspaceId,
    requirementNumber = null,
    open = $bindable(false),
  } = $props();

  let history = $state([]);
  let loading = $state(false);
  let error = $state('');
  let lastLoadedFor = $state(null);

  $effect(() => {
    if (!open || requirementNumber == null) return;
    const key = `${workspaceId}:${requirementNumber}`;
    if (lastLoadedFor === key) return;
    lastLoadedFor = key;
    void loadHistory();
  });

  async function loadHistory() {
    loading = true;
    error = '';
    try {
      history = await api.requirements.getHistory(workspaceId, requirementNumber);
    } catch (e) {
      error = e?.message || t('requirements.historyLoadError');
      history = [];
    } finally {
      loading = false;
    }
  }

  function formatValue(value) {
    if (value == null || value === '') return '—';
    return String(value);
  }
</script>

{#if open}
  <div class="history-drawer-backdrop" role="presentation" onclick={() => (open = false)}></div>
  <aside class="history-drawer" aria-label={t('requirements.historyTitle')}>
    <header class="history-drawer__header">
      <div class="flex items-center gap-2">
        <IconHistory size={18} />
        <h2>{t('requirements.historyTitle')}</h2>
      </div>
      <button type="button" class="history-drawer__close" onclick={() => (open = false)} aria-label={t('common.close')}>
        <IconX size={18} />
      </button>
    </header>
    <div class="history-drawer__body">
      {#if loading}
        <div class="flex justify-center py-8"><Spinner /></div>
      {:else if error}
        <p class="text-sm text-[var(--ds-text-danger)] px-4">{error}</p>
      {:else if history.length === 0}
        <EmptyState title={t('requirements.historyEmpty')} />
      {:else}
        <ul class="history-list">
          {#each history as entry (entry.id)}
            <li class="history-list__item">
              <div class="history-list__field">{entry.field_name}</div>
              <div class="history-list__change">
                <span class="history-list__old">{formatValue(entry.old_value)}</span>
                <span aria-hidden="true">→</span>
                <span class="history-list__new">{formatValue(entry.new_value)}</span>
              </div>
              <time class="history-list__time">{formatRelativeTime(entry.changed_at)}</time>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  </aside>
{/if}

<style>
  .history-drawer-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.35);
    z-index: 40;
  }
  .history-drawer {
    position: fixed;
    top: 0;
    right: 0;
    bottom: 0;
    width: min(420px, 100vw);
    background: var(--ds-surface-raised);
    border-left: 1px solid var(--ds-border);
    z-index: 41;
    display: flex;
    flex-direction: column;
  }
  .history-drawer__header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 1rem 1.25rem;
    border-bottom: 1px solid var(--ds-border);
  }
  .history-drawer__header h2 {
    font-size: 1rem;
    font-weight: 600;
    margin: 0;
  }
  .history-drawer__close {
    display: flex;
    padding: 0.25rem;
    border: none;
    background: transparent;
    cursor: pointer;
    color: var(--ds-text-subtle);
  }
  .history-drawer__body {
    flex: 1;
    overflow-y: auto;
    padding: 0.5rem 0;
  }
  .history-list {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  .history-list__item {
    padding: 0.75rem 1.25rem;
    border-bottom: 1px solid var(--ds-border-subtle);
  }
  .history-list__field {
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
    color: var(--ds-text-subtle);
    margin-bottom: 0.25rem;
  }
  .history-list__change {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
    font-size: 0.875rem;
  }
  .history-list__old {
    color: var(--ds-text-subtle);
    text-decoration: line-through;
  }
  .history-list__time {
    display: block;
    margin-top: 0.35rem;
    font-size: 0.75rem;
    color: var(--ds-text-subtle);
  }
</style>
