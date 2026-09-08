<script>
  import { onMount } from 'svelte';
  import { Square, Clock, Plus, ExternalLink, Pencil, Trash2 } from '@lucide/svelte';
  import { timerStore } from '../stores/timerStore.svelte.js';
  import { timeEntryStore } from '../stores';
  import { confirm } from '../composables/useConfirm.js';
  import { t } from '../stores/i18n.svelte.js';
  import { formatItemKey } from '../utils/itemKey.js';
  import MobileHeader from './MobileHeader.svelte';
  import TimeLogModal from '../dialogs/TimeLogModal.svelte';
  import { formatAuthenticatedInstant } from '../utils/authenticatedDateFormatter.js';

  const activeTimer = $derived(timerStore.activeTimer);
  // Most-recent-first; the worklog list/edit/delete + modal all come from the
  // same store the desktop Time view uses, so behaviour is identical.
  const worklogs = $derived(
    [...timeEntryStore.worklogs].sort((a, b) => (b.start_time ?? b.date ?? 0) - (a.start_time ?? a.date ?? 0)).slice(0, 20)
  );

  function fmtDay(epochSeconds) {
    if (!epochSeconds) return '';
    return formatAuthenticatedInstant(epochSeconds * 1000, { month: 'short', day: 'numeric' });
  }

  async function stopTimer() {
    try {
      await timerStore.stop();
      await timeEntryStore.loadWorklogs();
    } catch (err) {
      console.error('Failed to stop timer:', err);
    }
  }

  async function removeWorklog(w, e) {
    e.stopPropagation();
    const ok = await confirm({
      title: t('common.delete'),
      message: t('time.entry.confirmDelete'),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger',
    });
    if (ok) await timeEntryStore.deleteWorklog(w);
  }

  onMount(() => {
    timeEntryStore.init();
  });
</script>

<MobileHeader title="Timer" />

<div class="content">
  <!-- Active timer card -->
  <section class="timer-card" class:running={!!activeTimer} data-testid="timer-card">
    {#if activeTimer}
      <div class="t-top">
        <Clock size={18} />
        {#if formatItemKey(activeTimer)}
          <a class="t-key" href={`/m/items/${activeTimer.item_id}`} data-testid="timer-item-link">
            {formatItemKey(activeTimer)} <ExternalLink size={12} />
          </a>
        {:else}
          <span class="t-key">{activeTimer.project_name ?? 'Running'}</span>
        {/if}
      </div>
      <div class="t-duration" data-testid="timer-duration">{timerStore.durationFormatted}</div>
      {#if activeTimer.item_title}
        <div class="t-title">{activeTimer.item_title}</div>
      {/if}
      <button class="btn-stop" onclick={stopTimer} disabled={timerStore.syncing} data-testid="timer-stop" type="button">
        <Square size={16} /> Stop
      </button>
    {:else}
      <div class="t-idle" data-testid="timer-idle">
        <Clock size={20} />
        <p>No timer running</p>
        <span>Start one from a work item's detail screen.</span>
      </div>
    {/if}
  </section>

  <!-- Recent worklogs + manual log -->
  <section class="block">
    <div class="block-head">
      <h2>Recent worklogs</h2>
      <button class="btn-log" onclick={() => timeEntryStore.openTimeLogModal()} data-testid="quick-log-open" type="button">
        <Plus size={16} /> Log time
      </button>
    </div>

    {#if timeEntryStore.worklogsLoading && worklogs.length === 0}
      <p class="msg">Loading…</p>
    {:else if worklogs.length === 0}
      <p class="msg" data-testid="worklogs-empty">No worklogs yet.</p>
    {:else}
      <ul class="worklogs" data-testid="worklogs-list">
        {#each worklogs as w (w.id)}
          <li class="wl">
            <button class="wl-main" onclick={() => timeEntryStore.editWorklog(w)} data-testid="worklog-edit" type="button">
              <span class="wl-desc">{w.description || w.project_name || 'Worklog'}</span>
              <span class="wl-sub">
                {#if formatItemKey(w)}<span class="wl-key">{formatItemKey(w)}</span>{/if}
                <span class="wl-dur">{timeEntryStore.formatDuration(w.duration_minutes)}</span>
                <span class="wl-day">{fmtDay(w.start_time || w.date)}</span>
              </span>
            </button>
            <button class="wl-del" onclick={(e) => removeWorklog(w, e)} data-testid="worklog-delete" aria-label="Delete worklog" type="button">
              <Trash2 size={16} />
            </button>
          </li>
        {/each}
      </ul>
    {/if}
  </section>
</div>

<!-- Full worklog dialog — identical to the desktop Time view (project/item/
     date/start/end/duration with sync, create + edit). -->
{#if timeEntryStore.showTimeLogModal}
  <TimeLogModal
    projects={timeEntryStore.projects}
    customers={timeEntryStore.customers}
    workItems={timeEntryStore.workItems}
    workspaces={timeEntryStore.workspaces}
    editingWorklog={timeEntryStore.editingWorklog}
    onsave={(e) => timeEntryStore.saveWorklog(e.detail)}
    oncancel={() => timeEntryStore.closeTimeLogModal()}
  />
{/if}

<style>
  .content { padding: 0.75rem; display: flex; flex-direction: column; gap: 1rem; }

  .timer-card {
    border: 1px solid var(--ds-border);
    border-radius: var(--radius-xl, 12px);
    padding: 1.25rem;
    text-align: center;
    background-color: var(--ds-surface-card, var(--ds-surface-raised));
    box-shadow: var(--ds-shadow-raised);
  }
  .timer-card.running { border-color: var(--ds-interactive); }

  .t-top { display: flex; align-items: center; justify-content: center; gap: 0.5rem; color: var(--ds-text-subtle); }
  .t-key {
    display: inline-flex; align-items: center; gap: 4px;
    font-family: var(--font-mono, monospace); font-size: 0.8125rem;
    color: var(--ds-text-link, var(--ds-interactive)); text-decoration: none;
  }
  .t-duration { font-size: 2.5rem; font-weight: var(--font-bold, 700); font-variant-numeric: tabular-nums; margin: 0.5rem 0; color: var(--ds-text); }
  .t-title { color: var(--ds-text-subtle); font-size: 0.875rem; margin-bottom: 0.75rem; }

  .btn-stop {
    display: inline-flex; align-items: center; gap: 0.4rem;
    padding: 0.6rem 1.5rem; border: none; border-radius: var(--radius-lg, 8px);
    background-color: var(--ds-danger, #e5484d); color: #fff; font-weight: var(--font-semibold, 600); cursor: pointer;
  }
  .btn-stop:disabled { opacity: 0.6; }

  .t-idle { display: flex; flex-direction: column; align-items: center; gap: 0.25rem; color: var(--ds-text-subtle); }
  .t-idle p { margin: 0.25rem 0 0; font-weight: var(--font-medium, 500); color: var(--ds-text); }
  .t-idle span { font-size: 0.8125rem; }

  .block-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 0.5rem; }
  .block-head h2 { font-size: 0.9375rem; font-weight: var(--font-semibold, 600); color: var(--ds-text); margin: 0; }

  .btn-log {
    display: inline-flex; align-items: center; gap: 0.3rem;
    padding: 0.4rem 0.75rem; border: 1px solid var(--ds-border); border-radius: var(--radius-md, 6px);
    background-color: var(--ds-surface); color: var(--ds-text); font-size: 0.8125rem; cursor: pointer;
  }

  .worklogs { list-style: none; margin: 0; padding: 0; border: 1px solid var(--ds-border); border-radius: var(--radius-lg, 8px); overflow: hidden; }
  .wl { display: flex; align-items: center; }
  .wl:not(:last-child) { border-bottom: 1px solid var(--ds-border); }
  .wl-main {
    flex: 1 1 auto; min-width: 0; display: flex; flex-direction: column; gap: 3px;
    padding: 0.7rem 0.85rem; text-align: left; background: none; border: none; cursor: pointer;
  }
  .wl-main:active { background-color: var(--ds-background-neutral-hovered); }
  .wl-desc { font-size: 0.875rem; color: var(--ds-text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .wl-sub { display: flex; align-items: center; gap: 0.6rem; font-size: 0.6875rem; color: var(--ds-text-subtle); }
  .wl-key { font-family: var(--font-mono, monospace); }
  .wl-dur { font-weight: var(--font-semibold, 600); color: var(--ds-text); }
  .wl-del {
    flex-shrink: 0; display: inline-flex; align-items: center; justify-content: center;
    width: 44px; align-self: stretch; border: none; border-left: 1px solid var(--ds-border);
    background: none; color: var(--ds-text-subtle); cursor: pointer;
  }
  .wl-del:active { background-color: var(--ds-background-neutral-hovered); color: var(--ds-text-danger, var(--ds-danger)); }

  .msg { padding: 1.5rem; text-align: center; color: var(--ds-text-subtle); font-size: 0.875rem; }
</style>
