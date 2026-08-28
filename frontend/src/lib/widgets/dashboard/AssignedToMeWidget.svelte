<script>
  import { CheckSquare } from '@lucide/svelte';
  import { authStore } from '../../stores';
  import { api } from '../../api.js';
  import DashboardTaskList from './DashboardTaskList.svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import {
    assignedToMeQuery,
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
  let lastUserId = null;
  let lastLoadKey = null;
  let version = 0;

  const currentUserId = $derived($authStore?.currentUser?.id ?? null);
  const rowCount = $derived(resolveRowCount(config, 12));
  const density = $derived(resolveDensity(config));
  const fetchLimit = $derived(rowCountToLimit(rowCount));
  const loadKey = $derived(`${currentUserId}:${rowCount}`);

  $effect(() => {
    if (currentUserId && (currentUserId !== lastUserId || loadKey !== lastLoadKey)) {
      lastUserId = currentUserId;
      lastLoadKey = loadKey;
      load();
    } else if (!currentUserId && lastUserId !== null) {
      lastUserId = null;
      lastLoadKey = null;
      tasks = [];
    }
  });

  async function load() {
    const v = ++version;
    loading = true;
    errored = false;
    try {
      const response = await api.items.getAll(assignedToMeQuery(currentUserId, fetchLimit));
      if (v !== version) return;
      tasks = normalizeTaskResponse(response, rowCount);
    } catch (err) {
      if (v !== version) return;
      if (err?.name === 'AbortError') return;
      console.error('Failed to load assigned items:', err);
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
  icon={CheckSquare}
  errorMessage={t('dashboard.widgetContent.assignedLoadError')}
  emptyMessage={t('dashboard.widgetContent.assignedEmpty')}
  {density}
  {openTask}
/>
