<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { writable } from 'svelte/store';
  import { navigate } from '../../router.js';
  import { Trash2, Play, Eye, PlayCircle, User } from 'lucide-svelte';
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
  import { renderStatusBadge, renderMilestoneBadge } from '../../utils/testStatusColors.js';

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

    // Add keyboard shortcuts
    const handleKeyDown = (e) => {
      // Only handle shortcuts when not typing in inputs or textareas
      if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA' || e.target.tagName === 'SELECT') {
        return;
      }

      // 'a' key to add test run
      if (e.key === 'a' || e.key === 'A') {
        e.preventDefault();
        showAddForm();
      }
    };

    document.addEventListener('keydown', handleKeyDown);

    // Cleanup
    return () => {
      document.removeEventListener('keydown', handleKeyDown);
    };
  });

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
      alert('Please select a test plan and enter a run name');
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
      label: 'Run Name',
      html: true,
      render: (run) => `<a href="${workspaceTestBase}/runs/${run.id}?from=runs" style="color: var(--ds-text-link);" class="hover:underline">${run.name}</a>`
    },
    {
      key: 'testSetName',
      label: 'Test Plan',
      html: true,
      render: (run) => `<a href="${workspaceTestBase}/sets?milestone=${run.milestoneId || ''}" style="color: var(--ds-text-link);" class="hover:underline">${run.testSetName}</a>`
    },
    {
      key: 'assignee',
      label: 'Assignee',
      html: true,
      render: (run) => {
        if (run.assignee_id && run.assignee_name) {
          const initials = run.assignee_name.split(' ').map(n => n[0]).join('').toUpperCase().slice(0, 2);
          return `<div class="flex items-center gap-2">
            ${run.assignee_avatar
              ? `<img src="${run.assignee_avatar}" alt="${run.assignee_name}" class="w-6 h-6 rounded-full" />`
              : `<div class="w-6 h-6 rounded-full flex items-center justify-center text-xs font-medium" style="background-color: var(--ds-background-accent-blue-subtler); color: var(--ds-text-accent-blue);">${initials}</div>`
            }
            <span>${run.assignee_name}</span>
          </div>`;
        }
        return `<span style="color: var(--ds-text-subtle);">Unassigned</span>`;
      }
    },
    {
      key: 'milestoneName',
      label: 'Milestone',
      html: true,
      render: (run) => run.milestoneId
        ? `<a href="/milestones" style="color: var(--ds-text-link);" class="hover:underline">${run.milestoneName}</a>`
        : `<span style="color: var(--ds-text-subtle);">No milestone</span>`
    },
    {
      key: 'started_at',
      label: 'Started',
      render: (run) => run.started_at ? new Date(run.started_at).toLocaleString() : '-'
    },
    {
      key: 'ended_at',
      label: 'Ended',
      render: (run) => run.ended_at ? new Date(run.ended_at).toLocaleString() : '-'
    },
    {
      key: 'status',
      label: 'Status',
      html: true,
      render: (run) => {
        const status = run.ended_at ? 'completed' : 'in_progress';
        return renderStatusBadge(status);
      }
    },
    { key: 'actions', label: 'Actions', width: 'w-16', align: 'text-right' }
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
      alert('Failed to delete test run: ' + error.message);
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
        title: 'Continue Execution',
        color: 'var(--ds-status-success-text)',
        onClick: () => continueExecution(run)
      });
    }

    // Add "View" option
    items.push({
      id: 'view',
      type: 'regular',
      icon: Eye,
      title: run.ended_at ? 'View Results' : 'View Details',
      onClick: () => viewRunDetails(run)
    });

    // Add "Delete" option
    items.push({
      id: 'delete',
      type: 'regular',
      icon: Trash2,
      title: 'Delete',
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
    title="Test Runs"
    subtitle="Execute and track test case results"
  >
    {#snippet actions()}
      <div class="flex items-center gap-3">
        <div class="w-40">
          <Select value={selectedAssigneeFilter} onchange={handleAssigneeFilterChange}>
            <option value="">All assignees</option>
            <option value="unassigned">Unassigned</option>
            {#each $users as user}
              <option value={user.id}>{user.first_name} {user.last_name}</option>
            {/each}
          </Select>
        </div>
        <div class="w-48">
          <MilestoneCombobox
            bind:value={selectedMilestoneFilter}
            placeholder="All milestones"
            onselect={handleMilestoneSelect}
          />
        </div>
        <Button
          onclick={showAddForm}
          variant="primary"
          size="medium"
          keyboardHint="A"
        >
          Create Test Run
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
            <h3 class="text-xl font-semibold" style="color: var(--ds-text);">Create Test Run</h3>
            <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">Select a plan and name this execution.</p>
          </div>
        </div>

        <div class="space-y-4">
          <div>
            <Label for="set-select" color="default" class="mb-2">Select Test Plan</Label>
            <Select id="set-select" bind:value={selectedSetId}>
              <option value="">-- Select a test plan --</option>
              {#each filteredTestSets as set}
                <option value={set.id}>{set.name}</option>
              {/each}
            </Select>
          </div>
          <div>
            <Label for="run-name" color="default" class="mb-2">Run Name</Label>
            <Input
              id="run-name"
              bind:value={runName}
              placeholder="e.g., Sprint 2 Build 123, UAT Testing"
            />
          </div>
          <div>
            <Label color="default" class="mb-2">Assign To</Label>
            <UserPicker
              bind:value={selectedAssigneeId}
              showUnassigned={true}
              placeholder="Select assignee (optional)"
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
            Cancel
          </Button>
          <Button
            onclick={createRun}
            variant="primary"
            disabled={!selectedSetId || !runName}
            keyboardHint="↵"
          >
            Create Run
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
      emptyMessage="No test runs yet"
      emptyDescription="Create a test run to execute your test plans."
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
  title="Delete Test Run"
  message="Are you sure you want to delete '{runToDelete?.name}'? This will permanently delete all test results and cannot be undone."
  confirmText="Delete"
  cancelText="Cancel"
/>
