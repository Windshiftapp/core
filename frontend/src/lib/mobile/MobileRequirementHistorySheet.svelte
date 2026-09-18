<script>
  import { Loader } from '@lucide/svelte';
  import { api } from '../api.js';
  import { formatRelativeTime } from '../utils/dateFormatter.js';
  import { t } from '../stores/i18n.svelte.js';
  import MobileSheet from './MobileSheet.svelte';

  let {
    workspaceId,
    requirementNumber = null,
    isOpen = $bindable(false),
    dataTestid = 'mobile-requirement-history-sheet',
  } = $props();

  let history = $state([]);
  let loading = $state(false);
  let error = $state('');
  let lastLoadedFor = $state(null);

  $effect(() => {
    if (!isOpen || requirementNumber == null) return;
    const key = `${workspaceId}:${requirementNumber}`;
    if (lastLoadedFor === key) return;
    lastLoadedFor = key;
    void loadHistory();
  });

  $effect(() => {
    if (!isOpen) {
      error = '';
      lastLoadedFor = null;
    }
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

<MobileSheet bind:isOpen title={t('requirements.historyTitle')} {dataTestid}>
  {#if loading}
    <div class="center" data-testid="mobile-requirement-history-loading">
      <Loader class="spin" size={22} />
    </div>
  {:else if error}
    <p class="error" data-testid="mobile-requirement-history-error">{error}</p>
  {:else if history.length === 0}
    <p class="empty" data-testid="mobile-requirement-history-empty">{t('requirements.historyEmpty')}</p>
  {:else}
    <ul class="history-list" data-testid="mobile-requirement-history-list">
      {#each history as entry (entry.id)}
        <li class="history-item">
          <div class="field">{entry.field_name}</div>
          <div class="change">
            <span class="old">{formatValue(entry.old_value)}</span>
            <span aria-hidden="true">→</span>
            <span class="new">{formatValue(entry.new_value)}</span>
          </div>
          <time class="time">{formatRelativeTime(entry.changed_at)}</time>
        </li>
      {/each}
    </ul>
  {/if}
</MobileSheet>

<style>
  .center {
    display: flex;
    justify-content: center;
    padding: 2rem;
    color: var(--ds-text-subtle);
  }

  :global(.spin) {
    animation: spin 1s linear infinite;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  .error {
    margin: 0;
    padding: 0.5rem 0;
    font-size: 0.875rem;
    color: var(--ds-text-danger);
  }

  .empty {
    margin: 0;
    padding: 0.5rem 0;
    font-size: 0.875rem;
    color: var(--ds-text-subtle);
  }

  .history-list {
    list-style: none;
    margin: 0;
    padding: 0;
  }

  .history-item {
    padding: 0.75rem 0;
    border-bottom: 1px solid var(--ds-border-subtle);
  }

  .history-item:last-child {
    border-bottom: none;
  }

  .field {
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
    color: var(--ds-text-subtle);
    margin-bottom: 0.25rem;
  }

  .change {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
    font-size: 0.875rem;
  }

  .old {
    color: var(--ds-text-subtle);
    text-decoration: line-through;
  }

  .time {
    display: block;
    margin-top: 0.35rem;
    font-size: 0.75rem;
    color: var(--ds-text-subtle);
  }
</style>
