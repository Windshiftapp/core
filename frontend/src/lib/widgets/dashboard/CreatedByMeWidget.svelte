<script>
  import PenLine from '@lucide/svelte/icons/pen-line';
  import { authStore } from '../../stores/auth.svelte.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { items } from '../../api/items.js';
  import DashboardTaskList from './DashboardTaskList.svelte';
  import {
    createdByMeQuery,
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
      const response = await items.getAll(createdByMeQuery(currentUserId, fetchLimit));
      if (v !== version) return;
      tasks = normalizeTaskResponse(response, rowCount);
    } catch (err) {
      if (v !== version) return;
      if (err?.name === 'AbortError') return;
      console.error('Failed to load created items:', err);
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
  icon={PenLine}
  errorMessage={t('dashboard.states.createdLoadError')}
  emptyMessage={t('dashboard.states.createdEmpty')}
  {density}
  {openTask}
/>
