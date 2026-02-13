<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { writable } from 'svelte/store';
  import { navigate } from '../../router.js';
  import { Trash2, Play, Eye, PlayCircle, User } from 'lucide-svelte';
  import { escapeHtml } from '../../utils/sanitize.ts';
  import Button from '../../components/Button.svelte';
  import PageHeader from '../../layout/PageHeader.svelte';
  import Input from '../../components/Input.svelte';
  import Select from '../../components/Select.svelte';
  import ConfirmDialog from '../../dialogs/ConfirmDialog.svelte';
  import MilestoneCombobox from '../../pickers/MilestoneCombobox.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import Label from '../../components/Label.svelte';
  import UserPicker from '../../pickers/UserPicker.svelte';
  import { renderStatusBadge, renderMilestoneBadge } from '../../utils/statusColors.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { useEventListener } from 'runed';

  let { workspaceId = null } = $props();

  const testSets = writable([]);
  const testRuns = writable([]);
  const milestones = writable([]);
  const users = writable([]);

  let showForm = $state(false);
  let selectedSetId = $state('');
  let runName = $state('');
  let selectedAssigneeId = $state(null);

  // Filtering
  let selectedMilestoneFilter = $state(null);
  let selectedAssigneeFilter = $state('');

  onMount(async () => {
    await loadData();

    // Check for URL parameters
    const urlParams = new URLSearchParams(window.location.search);
    const milestoneParam = urlParams.get('milestone');
    if (milestoneParam) {
      selectedMilestoneFilter = parseInt(milestoneParam);
    }
  });

  useEventListener(() => document, 'keydown', (e) => {
    if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA' || e.target.tagName === 'SELECT') return;
    if (e.key === 'a' || e.key === 'A') { e.preventDefault(); showAddForm(); }
  });
  useEventListener(() => window, 'trigger-test-run-form', () => showAddForm());

  async function loadData() {
    try {
      // Build query params for assignee filter
      const params = {};
      if (selectedAssigneeFilter) {
        params.assignee_id = selectedAssigneeFilter;
      }

      const [sets, runs, milestonesData, usersData] = await Promise.all([
        api.tests.testSets.getAll(workspaceId),
        api.tests.testRuns.getAll(workspaceId, params),
        api.milestones.getAll(),
        api.getUsers()
      ]);
      const safeSets = sets || [];
      const safeRuns = runs || [];

      testSets.set(safeSets);
      testRuns.set(safeRuns);
      milestones.set(milestonesData || []);
      users.set(usersData || []);
    } catch (error) {
      console.error('Failed to load data:', error);
    }
  }

  function showAddForm() {
    showForm = true;
    selectedSetId = '';
    runName = '';
    selectedAssigneeId = null;
    // Focus the first input after the form is rendered
    setTimeout(() => {
      const firstInput = document.getElementById('set-select');
      if (firstInput) firstInput.focus();
    }, 100);
  }

  async function createRun() {
    if (!selectedSetId || !runName) {
      alert(t('testing.selectPlanAndEnterName'));
      return;
    }

    try {
      await api.tests.testRuns.create(workspaceId, {
        set_id: parseInt(selectedSetId),
        name: runName,
        assignee_id: selectedAssigneeId || null
      });
      await loadData();
      showForm = false;
    } catch (error) {
      console.error('Failed to create test run:', error);
    }
  }

  // Handle assignee filter change
  async function handleAssigneeFilterChange(event) {
    selectedAssigneeFilter = event.target.value;
    await loadData();
    updateURL();
  }

  // Status rendering now handled by imported utility (renderStatusBadge)

  function viewRunDetails(run) {
    navigate(testPath(`/runs/${run.id}?from=runs`));
  }

  function continueExecution(run) {
    // Navigate directly to the execution page to continue where left off
    navigate(testPath(`/runs/${run.id}/execute?from=runs`));
  }

  // Delete confirmation
  let showDeleteConfirm = $state(false);
  let runToDelete = $state(null);

  function testPath(suffix = '') {
    const base = workspaceId ? `/workspaces/${workspaceId}/tests` : '/workspaces';
    return `${base}${suffix}`;
  }

  const workspaceTestBase = $derived.by(() => testPath(''));
  const filteredTestSets = $derived.by(() => selectedMilestoneFilter
    ? $testSets.filter(set => set.milestone_id === selectedMilestoneFilter)
    : $testSets);

  const runColumns = $derived.by(() => [
    {
      key: 'name',
      label: t('testing.runName'),
      html: true,
      render: (run) => `<a href="${workspaceTestBase}/runs/${run.id}?from=runs" style="color: var(--ds-text-link);" class="hover:underline">${escapeHtml(run.name)}</a>`
    },
    {
      key: 'testSetName',
      label: t('testing.testPlan'),
      html: true,
      render: (run) => `<a href="${workspaceTestBase}/sets?milestone=${run.milestoneId || ''}" style="color: var(--ds-text-link);" class="hover:underline">${escapeHtml(run.testSetName)}</a>`
    },
    {
      key: 'assignee',
      label: t('common.assignee'),
      html: true,
      render: (run) => {
        if (run.assignee_id && run.assignee_name) {
          const safeName = escapeHtml(run.assignee_name);
          const initials = escapeHtml(run.assignee_name.split(' ').map(n => n[0]).join('').toUpperCase().slice(0, 2));
          return `<div class="flex items-center gap-2">
            ${run.assignee_avatar
              ? `<img src="${escapeHtml(run.assignee_avatar)}" alt="${safeName}" class="w-6 h-6 rounded-full" />`
              : `<div class="w-6 h-6 rounded-full flex items-center justify-center text-xs font-medium" style="background-color: var(--ds-background-accent-blue-subtler); color: var(--ds-text-accent-blue);">${initials}</div>`
            }
            <span>${safeName}</span>
          </div>`;
        }
        return `<span style="color: var(--ds-text-subtle);">${t('common.unassigned')}</span>`;
      }
    },
    {
      key: 'milestoneName',
      label: t('milestones.milestone'),
      html: true,
      render: (run) => run.milestoneId
        ? `<a href="/milestones" style="color: var(--ds-text-link);" class="hover:underline">${escapeHtml(run.milestoneName)}</a>`
        : `<span style="color: var(--ds-text-subtle);">${t('testing.noMilestone')}</span>`
    },
    {
      key: 'started_at',
      label: t('testing.started'),
      render: (run) => run.started_at ? new Date(run.started_at).toLocaleString() : '-'
    },
    {
      key: 'ended_at',
      label: t('testing.ended'),
      render: (run) => run.ended_at ? new Date(run.ended_at).toLocaleString() : '-'
    },
    {
      key: 'status',
      label: t('common.status'),
      html: true,
      render: (run) => {
        const status = run.ended_at ? 'completed' : 'in_progress';
        return renderStatusBadge(status);
      }
    },
    { key: 'actions', label: t('common.actions'), width: 'w-16', align: 'text-right' }
  ]);

  function confirmDelete(run) {
    runToDelete = run;
    showDeleteConfirm = true;
  }

  async function deleteRun() {
    if (!runToDelete) return;

    try {
      await api.tests.testRuns.delete(workspaceId, runToDelete.id);
      runToDelete = null;
      await loadData();
    } catch (error) {
      console.error('Failed to delete test run:', error);
      alert(t('testing.failedToDeleteRun') + ': ' + error.message);
    }
  }

  function buildRunDropdownItems(run) {
    const items = [];

    // Add "Continue" option for in-progress runs
    if (!run.ended_at) {
      items.push({
        id: 'continue',
        type: 'regular',
        icon: Play,
        title: t('testing.continueExecution'),
        color: 'var(--ds-status-success-text)',
        onClick: () => continueExecution(run)
      });
    }

    // Add "View" option
    items.push({
      id: 'view',
      type: 'regular',
      icon: Eye,
      title: run.ended_at ? t('testing.viewResults') : t('testing.viewDetails'),
      onClick: () => viewRunDetails(run)
    });

    // Add "Delete" option
    items.push({
      id: 'delete',
      type: 'regular',
      icon: Trash2,
      title: t('common.delete'),
      color: 'var(--ds-text-danger)',
      onClick: () => setTimeout(() => confirmDelete(run), 0)
    });

    return items;
  }

  // Create a list of all test runs with their test set and milestone info
  const allTestRuns = $derived.by(() => {
    // Filter by milestone if selected
    const filteredSetIds = new Set(filteredTestSets.map(s => s.id));

    return $testRuns
      .filter(run => !selectedMilestoneFilter || filteredSetIds.has(run.set_id))
      .map(run => {
        const set = $testSets.find(s => s.id === run.set_id);
        const milestone = set ? $milestones.find(m => m.id === set.milestone_id) : null;
        return {
          ...run,
          testSetName: set?.name || 'Unknown',
          testSetId: run.set_id,
          milestoneName: milestone?.name || 'No milestone',
          milestoneId: set?.milestone_id
        };
      });
  });

  // Handle milestone selection and update URL
  function handleMilestoneSelect(event) {
    selectedMilestoneFilter = event.detail.value;
    updateURL();
  }

  function updateURL() {
    const url = new URL(window.location);
    if (selectedMilestoneFilter) {
      url.searchParams.set('milestone', selectedMilestoneFilter.toString());
    } else {
      url.searchParams.delete('milestone');
    }
    window.history.replaceState({}, '', url);
  }
</script>

<div class="min-h-screen flex flex-col p-6" style="background-color: var(--ds-surface-raised);">
  <PageHeader
    title={t('testing.testRuns')}
    subtitle={t('testing.testRunsSubtitle')}
  >
    {#snippet actions()}
      <div class="flex items-center gap-3">
        <div class="w-40">
          <Select value={selectedAssigneeFilter} onchange={handleAssigneeFilterChange}>
            <option value="">{t('common.allAssignees')}</option>
            <option value="unassigned">{t('common.unassigned')}</option>
            {#each $users as user}
              <option value={user.id}>{user.first_name} {user.last_name}</option>
            {/each}
          </Select>
        </div>
        <div class="w-48">
          <MilestoneCombobox
            bind:value={selectedMilestoneFilter}
            placeholder={t('milestones.allMilestones')}
            onselect={handleMilestoneSelect}
          />
        </div>
        <Button
          onclick={showAddForm}
          variant="primary"
          size="medium"
          keyboardHint="A"
        >
          {t('testing.createTestRun')}
        </Button>
      </div>
    {/snippet}
  </PageHeader>

  {#if showForm}
    <Modal
      isOpen={showForm}
      onclose={() => showForm = false}
      onSubmit={createRun}
      submitDisabled={!selectedSetId || !runName}
    >
      <div class="p-6 space-y-6">
        <div class="flex items-start justify-between">
          <div>
            <h3 class="text-xl font-semibold" style="color: var(--ds-text);">{t('testing.createTestRun')}</h3>
            <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">{t('testing.createTestRunSubtitle')}</p>
          </div>
        </div>

        <div class="space-y-4">
          <div>
            <Label for="set-select" color="default" class="mb-2">{t('testing.selectTestPlan')}</Label>
            <Select id="set-select" bind:value={selectedSetId}>
              <option value="">{t('testing.selectTestPlanPlaceholder')}</option>
              {#each filteredTestSets as set}
                <option value={set.id}>{set.name}</option>
              {/each}
            </Select>
          </div>
          <div>
            <Label for="run-name" color="default" class="mb-2">{t('testing.runName')}</Label>
            <Input
              id="run-name"
              bind:value={runName}
              placeholder={t('testing.runNamePlaceholder')}
            />
          </div>
          <div>
            <Label color="default" class="mb-2">{t('common.assignTo')}</Label>
            <UserPicker
              bind:value={selectedAssigneeId}
              showUnassigned={true}
              placeholder={t('testing.selectAssigneeOptional')}
            />
          </div>
        </div>

        <div class="flex gap-3 justify-end pt-2">
          <Button
            type="button"
            variant="outline"
            onclick={() => showForm = false}
            keyboardHint="Esc"
          >
            {t('common.cancel')}
          </Button>
          <Button
            onclick={createRun}
            variant="primary"
            disabled={!selectedSetId || !runName}
            keyboardHint="↵"
          >
            {t('testing.createRun')}
          </Button>
        </div>
      </div>
    </Modal>
  {/if}

  <!-- Content wrapper -->
  <div class="flex-1 -mx-6 -mb-6 px-10 py-6">
    <DataTable
      columns={runColumns}
      data={allTestRuns}
      keyField="id"
      actionItems={buildRunDropdownItems}
      emptyMessage={t('testing.noTestRunsYet')}
      emptyDescription={t('testing.createTestRunToExecute')}
      emptyIcon={Play}
    />
  </div>
</div>

<!-- Delete Confirmation Dialog -->
<ConfirmDialog
  bind:show={showDeleteConfirm}
  variant="danger"
  onconfirm={deleteRun}
  oncancel={() => { runToDelete = null; }}
  title={t('testing.deleteTestRun')}
  message={t('testing.deleteRunConfirm', { name: runToDelete?.name })}
  confirmText={t('common.delete')}
  cancelText={t('common.cancel')}
/>
