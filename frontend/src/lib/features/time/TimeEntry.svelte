<script>
  import { onMount } from 'svelte';
  import { Filter, Plus, Edit, Trash2, MoreHorizontal } from 'lucide-svelte';
  import AlertBox from '../../components/AlertBox.svelte';
  import { navigate } from '../../router.js';
  import { timeEntryStore } from '../../stores';
  import Button from '../../components/Button.svelte';
  import Input from '../../components/Input.svelte';
  import Select from '../../components/Select.svelte';
  import TimeTrackingOnboarding from './TimeTrackingOnboarding.svelte';
  import TimeLogModal from '../../dialogs/TimeLogModal.svelte';
  import DropdownMenu from '../../layout/DropdownMenu.svelte';
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
<div class="rounded-xl p-6 mb-6 border shadow-sm" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
  <h3 class="text-sm font-semibold mb-4" style="color: var(--ds-text);">{t('time.reports.filters')}</h3>
  <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
    <div>
      <label class="block text-xs font-medium mb-2" style="color: var(--ds-text-subtle);">{t('time.reports.customer')}</label>
      <Select value={filters.customer_id} onchange={(e) => setFilter('customer_id', e.target.value)} size="small">
        <option value="">{t('time.reports.allCustomers')}</option>
        {#each customers as customer}
          <option value={customer.id}>{customer.name}</option>
        {/each}
      </Select>
    </div>
    <div>
      <label class="block text-xs font-medium mb-2" style="color: var(--ds-text-subtle);">{t('time.reports.project')}</label>
      <Select value={filters.project_id} onchange={(e) => setFilter('project_id', e.target.value)} size="small">
        <option value="">{t('time.reports.allProjects')}</option>
        {#each filteredProjects as project}
          <option value={project.id}>{project.name}</option>
        {/each}
      </Select>
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
</div>

<!-- Time Entries -->
<div class="rounded-xl border shadow-sm overflow-hidden" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
  {#if worklogs.length === 0}
    <div class="p-8 text-center" style="color: var(--ds-text-subtle);">
      {t('time.entry.noEntries')}
    </div>
  {:else}
    <div class="overflow-x-auto">
      <table class="w-full">
        <thead style="background-color: var(--ds-background-neutral);">
          <tr>
            <th class="px-6 py-4 text-left text-xs font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle);">{t('common.date')}</th>
            <th class="px-6 py-4 text-left text-xs font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle);">{t('time.reports.project')}</th>
            <th class="px-6 py-4 text-left text-xs font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle);">{t('items.workItem')}</th>
            <th class="px-6 py-4 text-left text-xs font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle);">{t('common.description')}</th>
            <th class="px-6 py-4 text-left text-xs font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle);">{t('common.time')}</th>
            <th class="px-6 py-4 text-left text-xs font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle);">{t('time.duration')}</th>
            <th class="px-6 py-4 text-right text-xs font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle);">{t('common.actions')}</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-200">
          {#each worklogs as worklog (worklog.id)}
            <tr class="transition-colors duration-150 hover:bg-opacity-50" style="hover:background-color: var(--ds-background-neutral-hovered);">
              <td class="px-6 py-4 text-sm" style="color: var(--ds-text);">
                {new Date(worklog.date * 1000).toLocaleDateString()}
              </td>
              <td class="px-6 py-4">
                <div class="flex items-center gap-2">
                  <div class="text-sm">
                    <div class="font-semibold" style="color: var(--ds-text);">{worklog.project_name}</div>
                    <div class="text-xs mt-1" style="color: var(--ds-text-subtle);">{worklog.customer_name}</div>
                  </div>
                  {#if isProjectOverBudget(worklog)}
                    <div title="{worklog.project_total_hours?.toFixed(1)}h / {worklog.project_max_hours?.toFixed(1)}h {t('time.entry.budgetExceeded')}">
                      <AlertTriangle size={16} class="text-amber-500" />
                    </div>
                  {/if}
                </div>
              </td>
              <td class="px-6 py-4 text-sm" style="color: var(--ds-text);">
                {#if worklog.item_title && worklog.workspace_key && worklog.workspace_item_number}
                  <button
                    class="font-medium text-blue-600 hover:text-blue-800 cursor-pointer text-left hover:underline"
                    onclick={() => navigateToItem(worklog.workspace_id, worklog.item_id)}
                    title={t('time.entry.clickToView', { key: worklog.workspace_key, number: worklog.workspace_item_number })}
                  >
                    {worklog.workspace_key}-{worklog.workspace_item_number}: {worklog.item_title}
                  </button>
                {:else}
                  <span class="text-gray-400 text-xs">—</span>
                {/if}
              </td>
              <td class="px-6 py-4 text-sm" style="color: var(--ds-text);">
                {worklog.description}
              </td>
              <td class="px-6 py-4 text-sm font-mono" style="color: var(--ds-text-subtle);">
                {formatTime(worklog.start_time)} - {formatTime(worklog.end_time)}
              </td>
              <td class="px-6 py-4 text-sm font-semibold" style="color: var(--ds-text);">
                {formatDuration(worklog.duration_minutes)}
              </td>
              <td class="px-6 py-4 text-right text-sm font-medium">
                <DropdownMenu
                  items={buildWorklogDropdownItems(worklog)}
                  triggerIcon={MoreHorizontal}
                  showChevron={false}
                  iconOnly={true}
                  triggerClass="p-2 rounded-md hover-bg transition-colors duration-150"
                />
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    <!-- Summary -->
    <div class="px-6 py-4 border-t" style="background-color: var(--ds-background-neutral); border-color: var(--ds-border);">
      <div class="text-sm font-semibold" style="color: var(--ds-text);">
        {t('time.reports.totalTime')}: {formatDuration(timeEntryStore.totalDuration)}
        <span class="ml-2 font-normal" style="color: var(--ds-text-subtle);">({t('time.reports.entriesShown', { count: worklogs.length })})</span>
      </div>
    </div>
  {/if}
</div>

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
