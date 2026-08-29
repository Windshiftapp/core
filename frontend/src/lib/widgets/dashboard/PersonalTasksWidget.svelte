<script>
  import { ListChecks } from '@lucide/svelte';
  import { authStore, workspacesStore } from '../../stores';
  import { t } from '../../stores/i18n.svelte.js';
  import { api } from '../../api.js';
  import DashboardTaskList from './DashboardTaskList.svelte';
  import {
    completedSinceCutoff,
    normalizeTaskResponse,
    openTask,
    resolveRowCount,
    resolveDensity,
    rowCountToLimit,
  } from './taskWidgetState.js';

  let { config = {} } = $props();

  let tasks = $state([]);
  let loading = $state(false);
  let errored = $state(false);
  let lastLoadKey = null;
  let version = 0;

  const currentUserId = $derived($authStore?.currentUser?.id ?? null);
  const personalWorkspaceId = $derived($workspacesStore.personalWorkspace?.id ?? null);
  const rowCount = $derived(resolveRowCount(config, 12));
  const density = $derived(resolveDensity(config));
  const fetchLimit = $derived(rowCountToLimit(rowCount));

  $effect(() => {
    if (currentUserId && !personalWorkspaceId) {
      void workspacesStore.loadPersonalWorkspace();
    }

    const loadKey = currentUserId && personalWorkspaceId
      ? `${currentUserId}:${personalWorkspaceId}:${rowCount}`
      : null;
    if (loadKey && loadKey !== lastLoadKey) {
      lastLoadKey = loadKey;
      load(personalWorkspaceId);
    } else if (!currentUserId && lastLoadKey !== null) {
      lastLoadKey = null;
      tasks = [];
    }
  });

  async function load(workspaceId) {
    const v = ++version;
    loading = true;
    errored = false;
    try {
      const response = await api.items.getAll({
        ql: `workspace_id = ${workspaceId}`,
        limit: fetchLimit,
        order_by: 'updated_at',
        // Hide tasks completed more than the default window ago, matching the
        // per-workspace TodoList done-range default.
        completed_since: completedSinceCutoff(),
      });
      if (v !== version) return;
      tasks = normalizeTaskResponse(response, rowCount);
    } catch (err) {
      if (v !== version) return;
      if (err?.name === 'AbortError') return;
      console.error('Failed to load personal tasks:', err);
      errored = true;
      tasks = [];
    } finally {
      if (v === version) loading = false;
    }
  }
</script>

<DashboardTaskList
  {loading}
  {errored}
  {tasks}
  icon={ListChecks}
  errorMessage={t('dashboard.states.personalTasksLoadError')}
  emptyMessage={t('dashboard.states.personalTasksEmpty')}
  {density}
  {openTask}
/>
