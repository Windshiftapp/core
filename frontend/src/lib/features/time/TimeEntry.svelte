<script>
  import { onMount } from 'svelte';
  import { Filter, Plus, Edit, Trash2 } from 'lucide-svelte';
  import AlertBox from '../../components/AlertBox.svelte';
  import { navigate } from '../../router.js';
  import { timeEntryStore } from '../../stores';
  import Button from '../../components/Button.svelte';
  import Input from '../../components/Input.svelte';
  import BasePicker from '../../pickers/BasePicker.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import Card from '../../components/Card.svelte';
  import TimeTrackingOnboarding from './TimeTrackingOnboarding.svelte';
  import TimeLogModal from '../../dialogs/TimeLogModal.svelte';
  import { toHotkeyString } from '../../utils/keyboardShortcuts.js';
  import { t } from '../../stores/i18n.svelte.js';

  // Bind to store values
  let worklogs = $derived(timeEntryStore.worklogs);
  let customers = $derived(timeEntryStore.customers);
  let projects = $derived(timeEntryStore.projects);
  let workItems = $derived(timeEntryStore.workItems);
  let workspaces = $derived(timeEntryStore.workspaces);
  let editingWorklog = $derived(timeEntryStore.editingWorklog);
  let showOnboarding = $derived(timeEntryStore.showOnboarding);
  let showTimeLogModal = $derived(timeEntryStore.showTimeLogModal);
  let filters = $derived(timeEntryStore.filters);
  let activeProjects = $derived(timeEntryStore.activeProjects);
  let filteredProjects = $derived(timeEntryStore.filteredProjects);

  const worklogColumns = $derived([
    { key: 'date', label: t('common.date'), render: (w) => new Date(w.date * 1000).toLocaleDateString() },
    { key: 'project_name', label: t('time.reports.project'), slot: 'project' },
    { key: 'item_title', label: t('items.workItem'), slot: 'item' },
    { key: 'description', label: t('common.description') },
    { key: 'time', label: t('common.time'), slot: 'details' },
    { key: 'duration_minutes', label: t('time.duration'), render: (w) => formatDuration(w.duration_minutes) },
    { key: 'actions', label: t('common.actions') }
  ]);

  onMount(async () => {
    await timeEntryStore.init();
  });

  function buildWorklogDropdownItems(worklog) {
    return [
      {
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: t('common.edit'),
        hoverClass: 'hover-bg',
        onClick: () => timeEntryStore.editWorklog(worklog)
      },
      {
        id: 'delete',
        type: 'danger',
        icon: Trash2,
        title: t('common.delete'),
        hoverClass: 'hover:bg-red-50',
        onClick: () => deleteWorklog(worklog)
      }
    ];
  }

  async function handleModalSave(event) {
    try {
      const data = event.detail;
      await timeEntryStore.saveWorklog(data);
    } catch (error) {
      console.error('Failed to save worklog:', error);
      alert(t('time.entry.failedToSave'));
    }
  }

  function handleModalCancel() {
    timeEntryStore.closeTimeLogModal();
  }

  function openLogTimeModal() {
    timeEntryStore.openTimeLogModal();
  }

  async function deleteWorklog(worklog) {
    if (confirm(t('time.entry.confirmDelete'))) {
      try {
        await timeEntryStore.deleteWorklog(worklog);
      } catch (error) {
        console.error('Failed to delete worklog:', error);
      }
    }
  }

  function formatTime(unixTimestamp) {
    return timeEntryStore.formatTime(unixTimestamp);
  }

  function formatDuration(minutes) {
    return timeEntryStore.formatDuration(minutes);
  }

  function isProjectOverBudget(worklog) {
    return timeEntryStore.isProjectOverBudget(worklog);
  }

  async function applyFilters() {
    await timeEntryStore.applyFilters();
  }

  function clearFilters() {
    timeEntryStore.clearFilters();
  }

  function navigateToItem(workspaceId, itemId) {
    if (workspaceId && itemId) {
      navigate(`/workspaces/${workspaceId}/items/${itemId}`);
    }
  }

  function handleOnboardingCancel() {
    timeEntryStore.closeOnboarding();
  }

  async function handleOnboardingCompleted(event) {
    await timeEntryStore.handleOnboardingCompleted();
  }

  function setFilter(key, value) {
    timeEntryStore.setFilter(key, value);
  }
</script>

<!-- Header -->
<div class="mb-6 flex justify-between items-start">
  <div>
    <h2 class="text-lg font-semibold" style="color: var(--ds-text);">{t('time.entry.title')}</h2>
    <div class="text-xs mt-1" style="color: var(--ds-text-subtle);">
      {t('time.entry.subtitle')}
    </div>
  </div>
  <Button
    variant="primary"
    size="medium"
    icon={Plus}
    onclick={openLogTimeModal}
    keyboardHint="A"
    hotkeyConfig={{ key: toHotkeyString('timeEntry', 'openLog'), guard: () => !showTimeLogModal }}
    title={t('time.entry.addTimeEntry')}
  >
    {t('time.logTime')}
  </Button>
</div>

{#if activeProjects.length === 0 && !showOnboarding}
  <AlertBox variant="warning" class="mb-6">
    <p class="font-medium" style="color: var(--ds-text-warning);">{t('time.entry.needProjects')}</p>
    <p class="mt-1" style="color: var(--ds-text-subtle);">
      <a href="/time/projects" class="font-medium underline hover:opacity-80 transition-opacity" style="color: var(--ds-link);">{t('time.entry.goToProjects')}</a>
      {#if customers.length === 0}
        {t('common.or')} <button onclick={() => timeEntryStore.openOnboarding()} class="font-medium underline hover:opacity-80 transition-opacity" style="color: var(--ds-link);">{t('time.entry.startSetupWizard')}</button>
      {/if}
    </p>
  </AlertBox>
{/if}

<!-- Filters -->
<Card rounded="xl" shadow padding="spacious" class="mb-6">
  <h3 class="text-sm font-semibold mb-4" style="color: var(--ds-text);">{t('time.reports.filters')}</h3>
  <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
    <div>
      <label class="block text-xs font-medium mb-2" style="color: var(--ds-text-subtle);">{t('time.reports.customer')}</label>
      <BasePicker
        value={filters.customer_id}
        items={customers}
        placeholder={t('time.reports.allCustomers')}
        showUnassigned={true}
        unassignedLabel={t('time.reports.allCustomers')}
        getValue={(item) => item.id}
        getLabel={(item) => item.name}
        onSelect={(item) => setFilter('customer_id', item ? item.id : '')}
      />
    </div>
    <div>
      <label class="block text-xs font-medium mb-2" style="color: var(--ds-text-subtle);">{t('time.reports.project')}</label>
      <BasePicker
        value={filters.project_id}
        items={filteredProjects}
        placeholder={t('time.reports.allProjects')}
        showUnassigned={true}
        unassignedLabel={t('time.reports.allProjects')}
        getValue={(item) => item.id}
        getLabel={(item) => item.name}
        onSelect={(item) => setFilter('project_id', item ? item.id : '')}
      />
    </div>
    <div>
      <label class="block text-xs font-medium mb-2" style="color: var(--ds-text-subtle);">{t('time.reports.fromDate')}</label>
      <Input type="date" value={filters.date_from} oninput={(e) => setFilter('date_from', e.target.value)} size="small" />
    </div>
    <div>
      <label class="block text-xs font-medium mb-2" style="color: var(--ds-text-subtle);">{t('time.reports.toDate')}</label>
      <Input type="date" value={filters.date_to} oninput={(e) => setFilter('date_to', e.target.value)} size="small" />
    </div>
  </div>
  <div class="mt-5 flex gap-3">
    <Button
      variant="primary"
      onclick={applyFilters}
      icon={Filter}
      size="medium"
      title={t('time.entry.applyFiltersTitle')}
    >
      {t('time.reports.applyFilters')}
    </Button>
    <Button
      variant="default"
      onclick={clearFilters}
      size="medium"
      title={t('time.entry.clearFiltersTitle')}
    >
      {t('common.clear')}
    </Button>
  </div>
</Card>

<!-- Time Entries -->
<Card rounded="xl" shadow padding="none" class="overflow-hidden">
  <DataTable
    columns={worklogColumns}
    data={worklogs}
    keyField="id"
    emptyMessage={t('time.entry.noEntries')}
    actionItems={buildWorklogDropdownItems}
    class="rounded-none border-0 shadow-none overflow-hidden"
  >
    <!-- Project: name + customer -->
    <div slot="project" let:item={worklog}>
      <div class="text-sm">
        <div class="font-semibold" style="color: var(--ds-text);">{worklog.project_name}</div>
        <div class="text-xs mt-1" style="color: var(--ds-text-subtle);">{worklog.customer_name}</div>
      </div>
    </div>

    <!-- Work Item: clickable link -->
    <div slot="item" let:item={worklog}>
      {#if worklog.item_title && worklog.workspace_key && worklog.workspace_item_number}
        <button
          class="font-medium text-blue-600 hover:text-blue-800 cursor-pointer text-left hover:underline text-sm"
          onclick={() => navigateToItem(worklog.workspace_id, worklog.item_id)}
          title={t('time.entry.clickToView', { key: worklog.workspace_key, number: worklog.workspace_item_number })}
        >
          {worklog.workspace_key}-{worklog.workspace_item_number}: {worklog.item_title}
        </button>
      {:else}
        <span class="text-gray-400 text-xs">—</span>
      {/if}
    </div>

    <!-- Time range with mono font -->
    <div slot="details" let:item={worklog}>
      <span class="text-sm font-mono" style="color: var(--ds-text-subtle);">
        {formatTime(worklog.start_time)} — {formatTime(worklog.end_time)}
      </span>
    </div>
  </DataTable>

  <!-- Summary footer -->
  {#if worklogs.length > 0}
    <div class="px-6 py-4 border-t" style="background-color: var(--ds-background-neutral); border-color: var(--ds-border);">
      <div class="text-sm font-semibold" style="color: var(--ds-text);">
        {t('time.reports.totalTime')}: {formatDuration(timeEntryStore.totalDuration)}
        <span class="ml-2 font-normal" style="color: var(--ds-text-subtle);">({t('time.reports.entriesShown', { count: worklogs.length })})</span>
      </div>
    </div>
  {/if}
</Card>

<!-- Onboarding Modal -->
{#if showOnboarding}
  <TimeTrackingOnboarding oncompleted={handleOnboardingCompleted} oncancel={handleOnboardingCancel} />
{/if}

<!-- Time Log Modal -->
{#if showTimeLogModal}
  <TimeLogModal
    projects={projects}
    {customers}
    {workItems}
    {workspaces}
    {editingWorklog}
    onsave={handleModalSave}
    oncancel={handleModalCancel}
  />
{/if}
