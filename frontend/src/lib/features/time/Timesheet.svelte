<script>
  import { onMount } from 'svelte';
  import { ChevronLeft, ChevronRight, X } from 'lucide-svelte';
  import { api } from '../../api.js';
  import { timeEntryStore } from '../../stores';
  import { BasePicker } from '../../pickers';
  import Button from '../../components/Button.svelte';
  import Card from '../../components/Card.svelte';
  import TimeLogModal from '../../dialogs/TimeLogModal.svelte';
  import PageHeader from '../../layout/PageHeader.svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { formatDate } from '../../utils/dateFormatter.js';

  const STORAGE_KEY = 'windshift-timesheet-projects';
  const WEEKENDS_KEY = 'windshift-timesheet-show-weekends';

  // Shared data from store
  let customers = $derived(timeEntryStore.customers);
  let projects = $derived(timeEntryStore.projects);
  let workItems = $derived(timeEntryStore.workItems);
  let workspaces = $derived(timeEntryStore.workspaces);
  let activeProjects = $derived(timeEntryStore.activeProjects);

  // Local state
  let weekWorklogs = $state([]);
  let prevWeekWorklogs = $state([]);
  let loading = $state(false);
  let currentWeekStart = $state(getMonday(new Date()));
  let pinnedProjectIds = $state(loadPinnedProjects());
  let showWeekends = $state(loadShowWeekends());
  let addProjectPickerValue = $state(null);

  // Modal state
  let showModal = $state(false);
  let modalProjectId = $state(null);
  let modalItemId = $state(null);
  let modalDate = $state(null);

  // Persist show weekends to localStorage
  function loadShowWeekends() {
    try {
      if (typeof localStorage !== 'undefined') {
        return localStorage.getItem(WEEKENDS_KEY) === 'true';
      }
    } catch { /* ignore */ }
    return false;
  }

  function toggleWeekends() {
    showWeekends = !showWeekends;
    try {
      if (typeof localStorage !== 'undefined') {
        localStorage.setItem(WEEKENDS_KEY, String(showWeekends));
      }
    } catch { /* ignore */ }
  }

  // Persist pinned projects to localStorage
  function loadPinnedProjects() {
    try {
      if (typeof localStorage !== 'undefined') {
        const stored = localStorage.getItem(STORAGE_KEY);
        return stored ? JSON.parse(stored) : [];
      }
    } catch { /* ignore */ }
    return [];
  }

  function savePinnedProjects() {
    try {
      if (typeof localStorage !== 'undefined') {
        localStorage.setItem(STORAGE_KEY, JSON.stringify(pinnedProjectIds));
      }
    } catch { /* ignore */ }
  }

  function addProject(projectId) {
    if (projectId && !pinnedProjectIds.includes(projectId)) {
      pinnedProjectIds = [...pinnedProjectIds, projectId];
      savePinnedProjects();
    }
    addProjectPickerValue = null;
  }

  function removeProject(projectId) {
    pinnedProjectIds = pinnedProjectIds.filter(id => id !== projectId);
    savePinnedProjects();
  }

  function getMonday(date) {
    const d = new Date(date);
    const day = d.getDay();
    const diff = d.getDate() - day + (day === 0 ? -6 : 1);
    d.setDate(diff);
    d.setHours(0, 0, 0, 0);
    return d;
  }

  // Full 7-day week (always Mon-Sun for data loading)
  const fullWeekDays = $derived.by(() => {
    const days = [];
    for (let i = 0; i < 7; i++) {
      const d = new Date(currentWeekStart);
      d.setDate(d.getDate() + i);
      days.push(d);
    }
    return days;
  });

  // Check if any worklogs exist on weekends (Sat/Sun = index 5,6)
  const hasWeekendWorklogs = $derived.by(() => {
    const satKey = toDateKey(fullWeekDays[5]);
    const sunKey = toDateKey(fullWeekDays[6]);
    return weekWorklogs.some(wl => {
      const dk = toDateKey(new Date(wl.date * 1000));
      return dk === satKey || dk === sunKey;
    });
  });

  // Weekends forced on when there are weekend worklogs
  const effectiveShowWeekends = $derived(showWeekends || hasWeekendWorklogs);

  // Visible weekdays based on effective setting
  const weekDays = $derived(effectiveShowWeekends ? fullWeekDays : fullWeekDays.slice(0, 5));

  // Previous week start
  const prevWeekStart = $derived.by(() => {
    const d = new Date(currentWeekStart);
    d.setDate(d.getDate() - 7);
    return d;
  });

  // Format date as YYYY-MM-DD
  function toDateKey(date) {
    return formatDate(date);
  }

  // Check if date is today
  function isToday(date) {
    const today = new Date();
    return toDateKey(date) === toDateKey(today);
  }

  // Week range label
  const weekLabel = $derived.by(() => {
    const start = weekDays[0];
    const end = weekDays[weekDays.length - 1];
    const opts = { month: 'short', day: 'numeric' };
    const startStr = start.toLocaleDateString('en-US', opts);
    const endStr = end.toLocaleDateString('en-US', { ...opts, year: 'numeric' });
    return `${startStr} – ${endStr}`;
  });

  // Project IDs that have worklogs in current or previous week
  const worklogProjectIds = $derived.by(() => {
    const ids = new Set();
    for (const wl of weekWorklogs) ids.add(wl.project_id);
    for (const wl of prevWeekWorklogs) ids.add(wl.project_id);
    return ids;
  });

  // Visible project IDs = pinned + those with worklogs in current/prev week
  const visibleProjectIds = $derived.by(() => {
    const ids = new Set(pinnedProjectIds);
    for (const id of worklogProjectIds) ids.add(id);
    return ids;
  });

  // Projects available to add (active, not already visible)
  const addableProjects = $derived(
    activeProjects
      .filter(p => !visibleProjectIds.has(p.id))
      .map(p => {
        const customer = customers.find(c => c.id === p.customer_id);
        return { ...p, subtitle: customer?.name || '' };
      })
  );

  // Build grid: group worklogs by project (and optionally item)
  const gridRows = $derived.by(() => {
    const rowMap = new Map();

    // Add visible projects as base rows
    for (const projectId of visibleProjectIds) {
      const project = projects.find(p => p.id === projectId);
      if (!project) continue;
      const key = `${project.id}:`;
      rowMap.set(key, {
        key,
        projectId: project.id,
        projectName: project.name,
        itemId: null,
        itemTitle: null,
        customerName: customers.find(c => c.id === project.customer_id)?.name || '',
        isPinned: pinnedProjectIds.includes(project.id),
        days: new Map(),
      });
    }

    // Add rows from current week worklogs (including project+item combos)
    for (const wl of weekWorklogs) {
      const key = wl.item_id ? `${wl.project_id}:${wl.item_id}` : `${wl.project_id}:`;
      if (!rowMap.has(key)) {
        rowMap.set(key, {
          key,
          projectId: wl.project_id,
          projectName: wl.project_name || 'Unknown Project',
          itemId: wl.item_id || null,
          itemTitle: wl.item_title || null,
          customerName: wl.customer_name || '',
          isPinned: pinnedProjectIds.includes(wl.project_id),
          days: new Map(),
        });
      }
      const row = rowMap.get(key);
      const dateKey = toDateKey(new Date(wl.date * 1000));
      row.days.set(dateKey, (row.days.get(dateKey) || 0) + wl.duration_minutes);
    }

    // Sort: projects alphabetically, item sub-rows under their project
    const rows = Array.from(rowMap.values());
    rows.sort((a, b) => {
      const nameCompare = a.projectName.localeCompare(b.projectName);
      if (nameCompare !== 0) return nameCompare;
      if (!a.itemId && b.itemId) return -1;
      if (a.itemId && !b.itemId) return 1;
      return (a.itemTitle || '').localeCompare(b.itemTitle || '');
    });

    return rows;
  });

  // Daily totals
  const dailyTotals = $derived.by(() => {
    const totals = new Map();
    for (const day of weekDays) {
      const dateKey = toDateKey(day);
      let total = 0;
      for (const row of gridRows) {
        total += row.days.get(dateKey) || 0;
      }
      totals.set(dateKey, total);
    }
    return totals;
  });

  // Grand total
  const grandTotal = $derived.by(() => {
    let total = 0;
    for (const [, val] of dailyTotals) {
      total += val;
    }
    return total;
  });

  // Row total
  function getRowTotal(row) {
    let total = 0;
    for (const [, val] of row.days) {
      total += val;
    }
    return total;
  }

  function formatDuration(minutes) {
    if (!minutes) return '';
    const hours = Math.floor(minutes / 60);
    const mins = minutes % 60;
    if (hours === 0) return `${mins}m`;
    if (mins === 0) return `${hours}h`;
    return `${hours}h ${mins}m`;
  }

  // Navigation
  function navigateWeek(offset) {
    const d = new Date(currentWeekStart);
    d.setDate(d.getDate() + offset * 7);
    currentWeekStart = d;
  }

  function goToThisWeek() {
    currentWeekStart = getMonday(new Date());
  }

  // Load week data (always full Mon-Sun + previous week for visibility rules)
  async function loadWeekWorklogs() {
    loading = true;
    try {
      const dateFrom = toDateKey(fullWeekDays[0]);
      const dateTo = toDateKey(fullWeekDays[6]);
      const prevFrom = toDateKey(prevWeekStart);
      const prevEnd = new Date(prevWeekStart);
      prevEnd.setDate(prevEnd.getDate() + 6);
      const prevTo = toDateKey(prevEnd);

      const [current, prev] = await Promise.all([
        api.time.worklogs.getAll({ date_from: dateFrom, date_to: dateTo }),
        api.time.worklogs.getAll({ date_from: prevFrom, date_to: prevTo }),
      ]);
      weekWorklogs = current || [];
      prevWeekWorklogs = prev || [];
    } catch (err) {
      console.error('Failed to load week worklogs:', err);
      weekWorklogs = [];
      prevWeekWorklogs = [];
    } finally {
      loading = false;
    }
  }

  // Reload when week changes
  $effect(() => {
    if (fullWeekDays.length > 0) {
      loadWeekWorklogs();
    }
  });

  onMount(async () => {
    await timeEntryStore.init();
  });

  // Cell click handler
  function handleCellClick(projectId, itemId, date) {
    modalProjectId = projectId;
    modalItemId = itemId;
    modalDate = toDateKey(date);
    showModal = true;
  }

  async function handleModalSave(event) {
    try {
      const data = event.detail;
      await api.time.worklogs.create(data);
      await loadWeekWorklogs();
    } catch (error) {
      console.error('Failed to save worklog:', error);
    } finally {
      showModal = false;
      modalProjectId = null;
      modalItemId = null;
      modalDate = null;
    }
  }

  function handleModalCancel() {
    showModal = false;
    modalProjectId = null;
    modalItemId = null;
    modalDate = null;
  }

  function getCurrentTime() {
    const now = new Date();
    return now.toTimeString().substring(0, 5);
  }

  // Format day header
  function formatDayHeader(date) {
    const dayName = date.toLocaleDateString('en-US', { weekday: 'short' });
    const dayNum = date.getDate();
    const month = date.toLocaleDateString('en-US', { month: 'short' });
    return `${dayName} ${dayNum} ${month}`;
  }
</script>

<!-- Header -->
<PageHeader
  title={t('time.timesheet.title')}
  subtitle={weekLabel}
>
  {#snippet actions()}
    <div class="flex items-center gap-4">
      <label class="flex items-center gap-2 text-sm select-none {hasWeekendWorklogs ? '' : 'cursor-pointer'}" style="color: var(--ds-text-subtle);">
        <input
          type="checkbox"
          checked={effectiveShowWeekends}
          onchange={toggleWeekends}
          disabled={hasWeekendWorklogs}
          class="rounded {hasWeekendWorklogs ? 'opacity-50' : 'cursor-pointer'}"
        />
        {t('time.timesheet.showWeekends')}
      </label>
      <div class="flex items-center gap-2">
        <Button
          variant="default"
          size="small"
          icon={ChevronLeft}
          onclick={() => navigateWeek(-1)}
          title={t('time.calendar.previousWeek')}
        />
        <Button
          variant="default"
          size="small"
          onclick={goToThisWeek}
        >
          {t('time.calendar.thisWeek')}
        </Button>
        <Button
          variant="default"
          size="small"
          icon={ChevronRight}
          onclick={() => navigateWeek(1)}
          title={t('time.calendar.nextWeek')}
        />
      </div>
    </div>
  {/snippet}
</PageHeader>

<!-- Add Project -->
<div class="mb-4 flex items-center gap-3">
  <div class="w-72">
    <BasePicker
      bind:value={addProjectPickerValue}
      items={addableProjects}
      placeholder={t('time.timesheet.addProject')}
      searchFields={['name', 'subtitle']}
      getValue={(p) => p?.id}
      getLabel={(p) => p?.name ?? ''}
      onSelect={(item) => { if (item) addProject(item.id); }}
    >
      {#snippet itemSnippet({ item: project })}
        <div class="flex flex-col min-w-0">
          <span class="font-medium text-sm">{project.name}</span>
          {#if project.subtitle}
            <span class="text-xs" style="color: var(--ds-text-subtle);">{project.subtitle}</span>
          {/if}
        </div>
      {/snippet}
    </BasePicker>
  </div>
</div>

<Card rounded="xl" shadow padding="none" class="overflow-hidden">
  {#if loading}
    <div class="p-12 text-center" style="color: var(--ds-text-subtle);">
      Loading...
    </div>
  {:else if gridRows.length === 0}
    <div class="p-12 text-center" style="color: var(--ds-text-subtle);">
      {t('time.timesheet.noEntries')}
    </div>
  {:else}
    <div class="overflow-x-auto">
      <table class="w-full border-collapse">
        <thead>
          <tr style="background-color: var(--ds-surface-raised);">
            <th class="text-left text-xs font-semibold px-4 py-3 border-b min-w-[220px]" style="color: var(--ds-text-subtle); border-color: var(--ds-border);">
              {t('time.timesheet.projectItem')}
            </th>
            {#each weekDays as day}
              <th
                class="text-center text-xs font-semibold px-3 py-3 border-b min-w-[100px]"
                style="color: var(--ds-text-subtle); border-color: var(--ds-border); {isToday(day) ? 'background-color: var(--ds-surface-selected);' : ''}"
              >
                {formatDayHeader(day)}
              </th>
            {/each}
            <th class="text-center text-xs font-semibold px-4 py-3 border-b min-w-[80px]" style="color: var(--ds-text-subtle); border-color: var(--ds-border);">
              {t('time.timesheet.total')}
            </th>
          </tr>
        </thead>
        <tbody>
          {#each gridRows as row (row.key)}
            <tr class="group" style="border-color: var(--ds-border);">
              <td class="px-4 py-2.5 border-b text-sm" style="border-color: var(--ds-border);">
                <div class="flex items-center gap-2 min-w-0">
                  <div class="flex flex-col min-w-0 flex-1">
                    {#if row.itemId}
                      <span class="text-xs truncate" style="color: var(--ds-text-subtle);">{row.projectName}</span>
                      <span class="truncate" style="color: var(--ds-text);">{row.itemTitle}</span>
                    {:else}
                      <span class="truncate" style="color: var(--ds-text);">{row.projectName}</span>
                      {#if row.customerName}
                        <span class="text-xs truncate" style="color: var(--ds-text-subtle);">{row.customerName}</span>
                      {/if}
                    {/if}
                  </div>
                  {#if row.isPinned && !row.itemId}
                    <button
                      class="opacity-0 group-hover:opacity-100 transition-opacity flex-shrink-0 p-0.5 rounded hover:bg-red-100 cursor-pointer"
                      style="color: var(--ds-text-subtle);"
                      onclick={() => removeProject(row.projectId)}
                      title={t('time.timesheet.removeProject')}
                    >
                      <X class="w-3.5 h-3.5" />
                    </button>
                  {/if}
                </div>
              </td>
              {#each weekDays as day}
                {@const dateKey = toDateKey(day)}
                {@const minutes = row.days.get(dateKey) || 0}
                <td
                  class="text-center px-3 py-2.5 border-b cursor-pointer transition-colors"
                  style="border-color: var(--ds-border); {isToday(day) ? 'background-color: var(--ds-surface-selected);' : ''}"
                  onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
                  onmouseleave={(e) => e.currentTarget.style.backgroundColor = isToday(day) ? 'var(--ds-surface-selected)' : ''}
                  onclick={() => handleCellClick(row.projectId, row.itemId, day)}
                  role="button"
                  tabindex="0"
                  onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); handleCellClick(row.projectId, row.itemId, day); } }}
                >
                  <span class="text-sm font-mono {minutes ? 'font-medium' : ''}" style="color: {minutes ? 'var(--ds-text)' : 'var(--ds-text-disabled)'};">
                    {minutes ? formatDuration(minutes) : '—'}
                  </span>
                </td>
              {/each}
              <td class="text-center px-4 py-2.5 border-b" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
                <span class="text-sm font-mono font-semibold" style="color: {getRowTotal(row) ? 'var(--ds-text)' : 'var(--ds-text-disabled)'};">
                  {getRowTotal(row) ? formatDuration(getRowTotal(row)) : '—'}
                </span>
              </td>
            </tr>
          {/each}
        </tbody>
        <tfoot>
          <tr style="background-color: var(--ds-background-neutral);">
            <td class="px-4 py-3 text-sm font-semibold" style="color: var(--ds-text);">
              {t('time.timesheet.total')}
            </td>
            {#each weekDays as day}
              {@const dateKey = toDateKey(day)}
              {@const dayTotal = dailyTotals.get(dateKey) || 0}
              <td class="text-center px-3 py-3" style="{isToday(day) ? 'background-color: var(--ds-surface-selected);' : ''}">
                <span class="text-sm font-mono font-semibold" style="color: {dayTotal ? 'var(--ds-text)' : 'var(--ds-text-disabled)'};">
                  {dayTotal ? formatDuration(dayTotal) : '—'}
                </span>
              </td>
            {/each}
            <td class="text-center px-4 py-3" style="background-color: var(--ds-surface-raised);">
              <span class="text-sm font-mono font-bold" style="color: var(--ds-text);">
                {grandTotal ? formatDuration(grandTotal) : '—'}
              </span>
            </td>
          </tr>
        </tfoot>
      </table>
    </div>
  {/if}
</Card>

<!-- Time Log Modal -->
{#if showModal}
  <TimeLogModal
    defaultProjectId={modalProjectId}
    defaultItemId={modalItemId}
    defaultDate={modalDate}
    defaultStartTime={getCurrentTime()}
    {projects}
    {customers}
    {workItems}
    {workspaces}
    onsave={handleModalSave}
    oncancel={handleModalCancel}
  />
{/if}
