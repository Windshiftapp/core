<script>
  import CheckSquare from '@lucide/svelte/icons/check-square';
  import PenLine from '@lucide/svelte/icons/pen-line';
  import { authStore } from '../../stores/auth.svelte.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { items } from '../../api/items.js';
  import DashboardTaskList from './DashboardTaskList.svelte';
  import {
    TASK_LIST_MODES,
    normalizeTaskResponse,
    openTask,
    resolveRowCount,
    resolveDensity,
    rowCountToLimit,
  } from './taskWidgetState.js';

  let { config = {}, mode } = $props();

  const MODE_ICONS = { assigned: CheckSquare, created: PenLine };

  const modeConfig = $derived(TASK_LIST_MODES[mode]);

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
      const response = await items.getAll(modeConfig.query(currentUserId, fetchLimit));
      if (v !== version) return;
      tasks = normalizeTaskResponse(response, rowCount);
    } catch (err) {
      if (v !== version) return;
      if (err?.name === 'AbortError') return;
      console.error(`Failed to load ${modeConfig.logLabel}:`, err);
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
  icon={MODE_ICONS[mode]}
  errorMessage={t(modeConfig.errorKey)}
  emptyMessage={t(modeConfig.emptyKey)}
  {density}
  {openTask}
/>
