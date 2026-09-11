<script>
  import { authStore } from '../stores';
  import { api } from '../api.js';
  import { navigate } from '../router.js';
  import { Check } from '@lucide/svelte';
  import MobileHeader from './MobileHeader.svelte';
  import MobileListState from './MobileListState.svelte';

  // Personal-workspace status ids: 1 = Open, 3 = Done (workspace default taxonomy).
  const STATUS_OPEN = 1;
  const STATUS_DONE = 3;

  let tasks = $state([]);
  let loading = $state(false);
  let errored = $state(false);
  let personalWorkspaceId = $state(null);
  let toggling = $state(new Set());
  let version = 0;

  const currentUserId = $derived($authStore?.currentUser?.id ?? null);

  function isDone(task) {
    return Boolean(task.status_completed) || task.status_id === STATUS_DONE;
  }

  async function load() {
    const v = ++version;
    loading = true;
    errored = false;
    try {
      if (!personalWorkspaceId) {
        const ws = await api.workspaces.getOrCreatePersonal();
        if (v !== version) return;
        personalWorkspaceId = ws?.id ?? null;
      }
      if (!personalWorkspaceId) {
        tasks = [];
        return;
      }
      const res = await api.items.getAll({
        ql: `workspace_id = ${personalWorkspaceId}`,
        limit: 50,
        order_by: 'updated_at',
      });
      if (v !== version) return;
      tasks = res?.data ?? [];
    } catch (err) {
      if (v !== version) return;
      console.error('Failed to load personal tasks:', err);
      errored = true;
      tasks = [];
    } finally {
      if (v === version) loading = false;
    }
  }

  async function toggleDone(task, e) {
    e.stopPropagation();
    if (toggling.has(task.id)) return;
    toggling = new Set(toggling).add(task.id);
    const target = isDone(task) ? STATUS_OPEN : STATUS_DONE;
    try {
      const updated = await api.items.transition(task.id, target);
      tasks = tasks.map((t) => (t.id === task.id ? { ...t, ...updated } : t));
    } catch (err) {
      console.error('Failed to toggle personal task:', err);
    } finally {
      const next = new Set(toggling);
      next.delete(task.id);
      toggling = next;
    }
  }

  let bootstrapped = false;
  $effect(() => {
    if (!bootstrapped && currentUserId) {
      bootstrapped = true;
      load();
    }
  });

  // A personal task created in this tab (mobile create dialog) won't be
  // caught by the cross-tab BroadcastChannel, which excludes the posting
  // tab. Refresh the list directly when one is added.
  function onPersonalTaskCreated() {
    load();
  }
  $effect(() => {
    window.addEventListener('personal-task-created', onPersonalTaskCreated);
    return () => window.removeEventListener('personal-task-created', onPersonalTaskCreated);
  });
</script>

<MobileHeader title="Personal" />

<div class="list" data-testid="personal-list">
  <MobileListState
    {loading}
    {errored}
    rowCount={tasks.length}
    skeletonRowHeight={52}
    errorTestId="personal-error"
    emptyTestId="personal-empty"
    errorMessage="Couldn't load your personal tasks."
    emptyMessage="Your personal todo list is empty."
    onretry={load}
  >
    {#each tasks as task (task.id)}
      {@const done = isDone(task)}
      <div class="row" data-testid="personal-row">
        <button
          class="check"
          class:checked={done}
          onclick={(e) => toggleDone(task, e)}
          disabled={toggling.has(task.id)}
          data-testid="personal-toggle"
          aria-label={done ? 'Mark not done' : 'Mark done'}
          type="button"
        >
          {#if done}<Check size={14} strokeWidth={3} />{/if}
        </button>
        <button class="body" onclick={() => navigate(`/m/items/${task.id}`)} type="button">
          <span class="title" class:done>{task.title}</span>
        </button>
      </div>
    {/each}
  </MobileListState>
</div>

<style>
  .row {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 0.625rem 0.75rem;
    border-bottom: 1px solid var(--ds-border);
    min-height: 52px;
  }

  .check {
    flex-shrink: 0;
    width: 24px;
    height: 24px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border: 2px solid var(--ds-border-bold, var(--ds-border));
    border-radius: var(--radius-full, 9999px);
    background: transparent;
    color: #fff;
    cursor: pointer;
  }

  .check.checked {
    background-color: var(--ds-success, #4cb782);
    border-color: var(--ds-success, #4cb782);
  }

  .body {
    flex: 1 1 auto;
    min-width: 0;
    text-align: left;
    background: none;
    border: none;
    cursor: pointer;
    padding: 0;
  }

  .title {
    font-size: 0.9375rem;
    color: var(--ds-text);
  }

  .title.done {
    text-decoration: line-through;
    color: var(--ds-text-subtlest, var(--ds-text-subtle));
  }
</style>
