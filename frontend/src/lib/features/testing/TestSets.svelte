<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { writable } from 'svelte/store';
  import ConfirmDialog from '../../dialogs/ConfirmDialog.svelte';
  import { X, Package, FileText, Play } from 'lucide-svelte';
  import { navigate } from '../../router.js';
  import Button from '../../components/Button.svelte';
  import PageHeader from '../../layout/PageHeader.svelte';
  import Input from '../../components/Input.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import MilestoneCombobox from '../../pickers/MilestoneCombobox.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import Label from '../../components/Label.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import TestCasePicker from '../../pickers/TestCasePicker.svelte';
  import { renderStatusBadge, renderMilestoneBadge } from '../../utils/statusColors.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { useEventListener } from 'runed';

  let { workspaceId = null } = $props();

  const testSets = writable([]);
  const selectedSet = writable(null);
  const milestones = writable([]);

  let showForm = $state(false);
  let editingSet = $state(null);
  let showTestCaseSelector = $state(false);
  let setTestCases = $state([]);

  // Filtering
  let selectedMilestoneFilter = $state(null);

  // Confirmation dialog
  let showDeleteConfirmation = $state(false);
  let setToDelete = null;

  let formData = $state({
    name: '',
    description: '',
    milestone_id: null
  });

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
  useEventListener(() => window, 'trigger-test-plan-form', () => showAddForm());

  async function loadData() {
    try {
      const [sets, milestonesData] = await Promise.all([
        api.tests.testSets.getAll(workspaceId),
        api.milestones.getAll() // Get all milestones
      ]);
      testSets.set(sets || []);
      milestones.set(milestonesData || []);
    } catch (error) {
      console.error('Failed to load data:', error);
    }
  }

  function showAddForm() {
    showForm = true;
    editingSet = null;
    formData = {
      name: '',
      description: '',
      milestone_id: null
    };
  }

  function showEditForm(set) {
    showForm = true;
    editingSet = set;
    formData = {
      name: set.name,
      description: set.description,
      milestone_id: set.milestone_id
    };
  }

  function testPath(suffix = '') {
    const base = workspaceId ? `/workspaces/${workspaceId}/tests` : '/workspaces';
    return `${base}${suffix}`;
  }

  async function handleSubmit() {
    try {
      const data = { ...formData };

      if (editingSet) {
        await api.tests.testSets.update(workspaceId, editingSet.id, data);
      } else {
        await api.tests.testSets.create(workspaceId, data);
      }
      await loadData();
      showForm = false;
    } catch (error) {
      console.error('Failed to save test plan:', error);
    }
  }

  async function handleStartRun() {
    if (!$selectedSet || setTestCases.length === 0) return;

    try {
      // Create a test run for this plan
      const runName = `${$selectedSet.name} - ${new Date().toLocaleDateString()}`;
      const newRun = await api.tests.testRuns.create(workspaceId, {
        set_id: $selectedSet.id,
        name: runName
      });

      showTestCaseSelector = false;

      // Navigate to the execution page
      navigate(testPath(`/runs/${newRun.id}/execute`));
    } catch (error) {
      console.error('Failed to start test run:', error);
    }
  }

  function deleteSet(id) {
    setToDelete = id;
    showDeleteConfirmation = true;
  }

  async function confirmDeleteSet() {
    try {
      await api.tests.testSets.delete(workspaceId, setToDelete);
      await loadData();
    } catch (error) {
      console.error('Failed to delete test plan:', error);
    } finally {
      setToDelete = null;
    }
  }

  async function manageSetTests(set) {
    selectedSet.set(set);
    showTestCaseSelector = true;
    await loadSetTestCases(set.id);
  }

  async function loadSetTestCases(setId) {
    try {
      const cases = await api.tests.testSets.getTestCases(workspaceId, setId);
      setTestCases = cases || [];
    } catch (error) {
      console.error('Failed to load set test cases:', error);
    }
  }

  async function handleAddTestCase(event) {
    const testCase = event.detail;
    if (!testCase || !testCase.id) return;

    try {
      await api.tests.testSets.addTestCase(workspaceId, $selectedSet.id, testCase.id);
      await loadSetTestCases($selectedSet.id);
    } catch (error) {
      console.error('Failed to add test case to set:', error);
      alert(t('dialogs.alerts.errorAddingTestCase', { error: error.message }));
    }
  }

  async function removeTestCaseFromSet(testCaseId) {
    try {
      await api.tests.testSets.removeTestCase(workspaceId, $selectedSet.id, testCaseId);
      await loadSetTestCases($selectedSet.id);
    } catch (error) {
      console.error('Failed to remove test case from set:', error);
    }
  }

  // Computed property for filtered test sets
  const filteredTestSets = $derived.by(() => selectedMilestoneFilter
    ? $testSets.filter(set => set.milestone_id === selectedMilestoneFilter)
    : $testSets);

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

  const workspaceTestBase = $derived.by(() => workspaceId ? `/workspaces/${workspaceId}/tests` : '/workspaces');
  const testSetColumns = $derived.by(() => [
    { key: 'id', label: t('common.id'), width: 'w-16' },
    {
      key: 'name',
      label: t('common.name'),
      html: true,
      render: (set) => {
        const description = set.description ? `<div class="text-xs mt-1" style="color: var(--ds-text-subtle);">${set.description}</div>` : '';
        return `<div class="font-medium" style="color: var(--ds-text);">${set.name}</div>${description}`;
      }
    },
    {
      key: 'milestone',
      label: t('milestones.milestone'),
      html: true,
      render: (set) => renderMilestoneBadge(set.milestone_name)
    },
    {
      key: 'test_case_count',
      label: t('testing.testCases'),
      html: true,
      render: (set) => {
        const count = set.test_case_count || 0;
        return `<span style="color: var(--ds-text);">${count}</span>`;
      }
    },
    {
      key: 'total_runs',
      label: t('testing.testRuns'),
      html: true,
      render: (set) => {
        const total = set.total_runs || 0;
        const success = set.successful_runs || 0;
        const failed = set.failed_runs || 0;
        const summary = total > 0
          ? `<div class="text-xs"><span style="color: var(--ds-text-success);">${success} ✓</span>${failed > 0 ? `<span style="color: var(--ds-text-danger);" class="ml-1">${failed} ✗</span>` : ''}</div>`
          : '';
        return `<div class="flex items-center space-x-2"><span class="font-medium" style="color: var(--ds-text);">${total} ${t('common.total').toLowerCase()}</span>${summary}</div>`;
      }
    },
    {
      key: 'last_run_status',
      label: t('testing.lastRun'),
      html: true,
      render: (set) => {
        if (!set.last_run_status) return `<span style="color: var(--ds-text-subtle);">${t('testing.neverRun')}</span>`;
        const datePart = set.last_run_date ? `<span class="text-xs mt-1" style="color: var(--ds-text-subtle);">${new Date(set.last_run_date).toLocaleDateString()}</span>` : '';
        return `<div class="flex flex-col">${renderStatusBadge(set.last_run_status)}${datePart}</div>`;
      }
    },
    { key: 'actions', label: t('common.actions'), width: 'w-24', align: 'text-right' }
  ]);

  function testSetActions(set) {
    return [
      {
        id: 'manage-tests',
        title: t('testing.tests'),
        onClick: () => manageSetTests(set)
      },
      {
        id: 'edit',
        title: t('common.edit'),
        onClick: () => showEditForm(set)
      },
      {
        id: 'delete',
        title: t('common.delete'),
        color: 'var(--ds-text-danger)',
        onClick: () => deleteSet(set.id)
      }
    ];
  }
</script>

<div class="min-h-screen flex flex-col p-6" style="background-color: var(--ds-surface-raised);">
  <PageHeader
    title={t('testing.testPlans')}
    subtitle={t('testing.testPlansSubtitle')}
  >
    {#snippet actions()}
      <div class="flex items-center gap-3">
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
          {t('testing.addTestPlan')}
        </Button>
      </div>
    {/snippet}
  </PageHeader>

  <!-- Add/Edit Test Plan Modal -->
  <Modal
    isOpen={showForm}
    onSubmit={handleSubmit}
    submitDisabled={!formData.name.trim()}
    maxWidth="max-w-2xl"
    onclose={() => showForm = false}
    let:submitHint
  >
    <div class="p-6">
      <h3 class="text-xl font-semibold mb-6" style="color: var(--ds-text);">
        {editingSet ? t('testing.editTestPlan') : t('testing.addTestPlan')}
      </h3>

      <div class="space-y-4">
        <div>
          <Label color="default" class="mb-2">{t('common.name')}</Label>
          <Input bind:value={formData.name} required />
        </div>

        <div>
          <Label color="default" class="mb-2">{t('common.description')}</Label>
          <Textarea bind:value={formData.description} rows={3} />
        </div>

        <div>
          <Label color="default" class="mb-2">{t('testing.milestoneOptional')}</Label>
          <MilestoneCombobox
            bind:value={formData.milestone_id}
            placeholder={t('testing.noMilestone')}
          />
        </div>
      </div>

      <div class="flex gap-2 justify-end mt-6">
        <Button
          type="button"
          variant="outline"
          onclick={() => showForm = false}
          keyboardHint="Esc"
        >
          {t('common.cancel')}
        </Button>
        <Button
          variant="primary"
          onclick={handleSubmit}
          disabled={!formData.name.trim()}
          keyboardHint={submitHint}
        >
          {editingSet ? t('common.save') : t('common.create')}
        </Button>
      </div>
    </div>
  </Modal>

  <Modal
    isOpen={showTestCaseSelector && $selectedSet}
    maxWidth="max-w-2xl"
    onclose={() => showTestCaseSelector = false}
  >
    <div class="p-6 max-h-[80vh] overflow-y-auto">
      <div class="flex justify-between items-center mb-6">
        <h3 class="text-xl font-semibold" style="color: var(--ds-text);">{t('testing.manageTestCasesFor', { name: $selectedSet?.name })}</h3>
        <button
          onclick={() => showTestCaseSelector = false}
          class="p-1 rounded transition-colors hover:bg-[var(--ds-background-neutral-hovered)]"
          style="color: var(--ds-text-subtle);"
        >
          <X size={20} />
        </button>
      </div>

      <!-- Add Test Case Picker -->
      <div class="mb-6">
        <span class="block text-sm font-medium mb-2" style="color: var(--ds-text);">{t('testing.addTestCase')}</span>
        <TestCasePicker
          {workspaceId}
          excludeIds={setTestCases.map(tc => tc.id)}
          onselect={handleAddTestCase}
          placeholder={t('testing.searchTestCasesToAdd')}
        />
      </div>

      <!-- Assigned Test Cases List -->
      <div>
        <h4 class="font-medium mb-3" style="color: var(--ds-text);">
          {t('testing.assignedTestCases', { count: setTestCases.length })}
        </h4>
        <div class="border rounded overflow-hidden" style="border-color: var(--ds-border);">
          {#if setTestCases.length === 0}
            <div class="p-8 text-center" style="background-color: var(--ds-surface);">
              <FileText size={32} style="color: var(--ds-text-subtle); margin: 0 auto 8px;" />
              <p class="text-sm" style="color: var(--ds-text-subtle);">{t('testing.noTestCasesAssigned')}</p>
              <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">{t('testing.useSearchToAddTestCases')}</p>
            </div>
          {:else}
            <div class="max-h-80 overflow-y-auto" style="background-color: var(--ds-surface);">
              {#each setTestCases as tc (tc.id)}
                <div
                  class="flex justify-between items-center px-3 py-2.5 border-b transition-colors"
                  style="border-color: var(--ds-border);"
                >
                  <div class="flex items-center gap-3 flex-1 min-w-0">
                    <FileText size={16} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
                    <div class="flex-1 min-w-0">
                      <span class="text-sm font-medium truncate block" style="color: var(--ds-text);">{tc.title}</span>
                      {#if tc.folder_name}
                        <span class="text-xs" style="color: var(--ds-text-subtle);">{tc.folder_name}</span>
                      {/if}
                    </div>
                  </div>
                  <button
                    onclick={() => removeTestCaseFromSet(tc.id)}
                    class="p-1.5 rounded transition-colors flex-shrink-0 hover:bg-[var(--ds-background-danger-hovered)] hover:text-[var(--ds-text-danger)]"
                    style="color: var(--ds-text-subtle);"
                    title={t('testing.removeTestCase')}
                  >
                    <X size={16} />
                  </button>
                </div>
              {/each}
            </div>
          {/if}
        </div>
      </div>

      <div class="mt-6 flex justify-end gap-3">
        <Button
          variant="ghost"
          onclick={() => showTestCaseSelector = false}
        >
          {t('common.done')}
        </Button>
        <Button
          variant="primary"
          onclick={handleStartRun}
          disabled={setTestCases.length === 0}
          icon={Play}
        >
          {t('testing.startRun')}
        </Button>
      </div>
    </div>
  </Modal>

  <!-- Content wrapper -->
  <div class="flex-1 -mx-6 -mb-6 px-10 py-6">
    <DataTable
      columns={testSetColumns}
      data={filteredTestSets}
      keyField="id"
      actionItems={testSetActions}
      emptyMessage={t('testing.noTestPlansYet')}
      emptyDescription={t('testing.createFirstTestPlan')}
      emptyIcon={Package}
    />
  </div>
</div>

<!-- Delete Test Plan Confirmation Dialog -->
<ConfirmDialog
  bind:show={showDeleteConfirmation}
  title={t('testing.deleteTestPlan')}
  message={t('testing.deleteTestPlanConfirm')}
  confirmText={t('testing.deleteTestPlan')}
  cancelText={t('common.cancel')}
  variant="danger"
  onconfirm={confirmDeleteSet}
  oncancel={() => {
    showDeleteConfirmation = false;
    setToDelete = null;
  }}
/>
