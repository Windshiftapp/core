<script>
  import { onMount } from 'svelte';
  import ModalBackdrop from '../../components/ModalBackdrop.svelte';
  import { currentRoute, navigate } from '../../router.js';
  import { items } from '../../api/items.js';
  import { tests } from '../../api/tests/index.js';
  import { isExpectedBackgroundSyncError } from '../../utils/backgroundSync.js';
  import IconCheck from '@tabler/icons-svelte-runes/icons/check';
  import IconX from '@tabler/icons-svelte-runes/icons/x';
  import IconBug from '@tabler/icons-svelte-runes/icons/bug';
  import IconChevronRight from '@tabler/icons-svelte-runes/icons/chevron-right';
  import IconChevronLeft from '@tabler/icons-svelte-runes/icons/chevron-left';
  import IconPlus from '@tabler/icons-svelte-runes/icons/plus';
  import IconLink from '@tabler/icons-svelte-runes/icons/link';
  import IconPlayerSkipForward from '@tabler/icons-svelte-runes/icons/player-skip-forward';
  import { confirm } from '../../composables/useConfirm.js';
  import Button from '../../components/Button.svelte';
  import BackButton from '../../components/BackButton.svelte';
  import Card from '../../components/Card.svelte';
  import Panel from '../../components/Panel.svelte';
  import Progress from '../../components/Progress.svelte';
  import StateDisplay from '../../components/StateDisplay.svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import AlertBox from '../../components/AlertBox.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import StatusButton from '../../components/StatusButton.svelte';
  import MilkdownEditor from '../../editors/LazyMilkdownEditor.svelte';
  import ItemPicker from '../../pickers/ItemPicker.svelte';
  import CreateModal from '../../dialogs/CreateModal.svelte';
  import Label from '../../components/Label.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import { getStatusLabel, getTestStatusColor } from '../../utils/statusColors.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { workspacePermissions } from '../../stores/workspacePermissions.svelte.js';
  import DescriptionText from '../../components/DescriptionText.svelte';
  import { loadTestRunDetail } from './testRunDetailData.js';
import BDDExampleExecution from './BDDExampleExecution.svelte';
import { parseScenarioSpec, flattenExamples } from './bddSpec.js';

  let testRun = $state(null);
  let testCases = $state([]);
  let currentCaseIndex = $state(0);
  let currentStepIndex = $state(0);
  let testResults = $state({});
  let stepResults = $state({});
  let loading = $state(true);
  let workspaceItems = $state([]);
  let showCreateModal = $state(false);
  let pendingLinkStepId = $state(null);
  let sidebarCollapsed = $state(false);
  let previewImage = $state(null);
  // BDD execution state: frozen spec snapshots and example results, keyed by
  // test case id. Both come with the run detail payload.
  let bddSnapshots = $state({});
  let bddExampleResults = $state({});

  function handleRenderedContentClick(event) {
    if (event.target.tagName === 'IMG') {
      previewImage = {
        src: event.target.src,
        alt: event.target.alt || 'Image preview'
      };
    }
  }

  let workspaceId = $derived($currentRoute.params.id);
  let runId = $derived($currentRoute.params.runId);
  // Recording results and finishing a run are the test.execute tier (denied
  // server-side with 404 otherwise); read-only users just follow along.
  let canExecuteTests = $derived(workspacePermissions.canExecuteTests(workspaceId));
  let fromPage = $derived($currentRoute.query?.from);
  let currentCase = $derived((Array.isArray(testCases) && testCases[currentCaseIndex]) || null);
  let currentStep = $derived(currentCase?.test_steps?.[currentStepIndex] || null);
  let currentCaseIsBdd = $derived(isBddCase(currentCase));
  let currentExampleRows = $derived(currentCaseIsBdd ? bddRowsFor(currentCase) : []);

  function testPath(suffix = '') {
    const base = workspaceId ? `/workspaces/${workspaceId}/tests` : '/workspaces';
    return `${base}${suffix}`;
  }

  onMount(async () => {
    if (runId) {
      await Promise.all([loadTestRun(runId), loadWorkspaceItems()]);
    }
  });

  async function loadWorkspaceItems() {
    try {
      const response = await items.getAll({ workspace_id: workspaceId });
      workspaceItems = response?.data ?? [];
    } catch (error) {
      // Navigating away aborts the in-flight fetch — expected control flow,
      // not a failure to surface in the console.
      if (!isExpectedBackgroundSyncError(error)) {
        console.error('Failed to load workspace items:', error);
      }
      workspaceItems = [];
    }
  }

  async function loadTestRun(runId) {
    try {
      loading = true;

      const detail = await loadTestRunDetail(workspaceId, runId);
      testRun = detail.run;
      bddSnapshots = detail.bddSnapshots;
      const groupedExampleResults = {};
      for (const result of detail.bddExampleResults) {
        (groupedExampleResults[result.test_case_id] ??= []).push(result);
      }
      bddExampleResults = groupedExampleResults;
      testCases = detail.testCases;
      testResults = {};
      stepResults = {};

      // Initialize results
      initializeResults();
      applyExistingResults(detail.stepResults, detail.results);

    } catch (error) {
      console.error('Failed to load test run:', error);
      await confirm({
        title: t('notifications.error'),
        message: error.message || 'Failed to load test run',
        confirmText: t('common.ok'),
        cancelText: '',
        variant: 'danger',
      });
      testCases = [];
    } finally {
      loading = false;

      // Set initial position after everything is loaded and loading is complete
      // Use a small timeout to ensure all reactive statements have updated
      setTimeout(() => {
        if (testCases && testCases.length > 0) {
          setInitialTestCasePosition();
        }
      }, 100);
    }
  }

  function initializeResults() {
    // Initialize test case results
    testCases.forEach(testCase => {
      if (!testResults[testCase.id]) {
        testResults[testCase.id] = {
          status: 'not_run',
          actual_result: '',
          notes: ''
        };
      }
      
      // Initialize step results
      testCase.test_steps.forEach(step => {
        if (!stepResults[step.id]) {
          stepResults[step.id] = {
            status: 'not_run',
            actual_result: '',
            notes: '',
            item_id: null
          };
        }
      });
    });
  }

  function applyExistingResults(existingStepResults, resultRows) {
    // Backend keys step results by `${testCaseID}_${stepID}`. Remap to the
    // globally unique step id used by the execution UI.
    testCases.forEach(testCase => {
      (testCase.test_steps || []).forEach(step => {
        const existingResult = existingStepResults[`${testCase.id}_${step.id}`];
        if (!existingResult) return;
        stepResults[step.id] = {
          status: existingResult.status || 'not_run',
          actual_result: existingResult.actual_result || '',
          notes: existingResult.notes || '',
          item_id: existingResult.item_id ?? null
        };
      });
    });
    stepResults = { ...stepResults };

    for (const row of resultRows) {
      testResults[row.test_case_id] = {
        ...(testResults[row.test_case_id] ?? {}),
        id: row.id,
        status: row.status || testResults[row.test_case_id]?.status || 'not_run',
      };
    }
    testResults = { ...testResults };
  }

  function setInitialTestCasePosition() {
    // Safety check
    if (!Array.isArray(testCases) || testCases.length === 0) {
      return;
    }

    // Find the first test case with incomplete work (steps or BDD examples)
    for (let caseIndex = 0; caseIndex < testCases.length; caseIndex++) {
      const testCase = testCases[caseIndex];

      if (isBddCase(testCase)) {
        const rows = bddExampleResults[testCase.id] || [];
        const hasIncompleteExamples = rows.some(row => !row.status || row.status === 'not_run');
        if (rows.length === 0 || hasIncompleteExamples) {
          currentCaseIndex = caseIndex;
          currentStepIndex = 0;
          return;
        }
        continue;
      }

      if (testCase.test_steps && testCase.test_steps.length > 0) {
        // Check if this test case has any incomplete steps
        const hasIncompleteSteps = testCase.test_steps.some(step => {
          const stepResult = stepResults[step.id];
          return !stepResult || stepResult.status === 'not_run';
        });

        if (hasIncompleteSteps) {
          currentCaseIndex = caseIndex;
          // Find the first incomplete step in this case
          for (let stepIndex = 0; stepIndex < testCase.test_steps.length; stepIndex++) {
            const step = testCase.test_steps[stepIndex];
            const stepResult = stepResults[step.id];
            if (!stepResult || stepResult.status === 'not_run') {
              currentStepIndex = stepIndex;
              return; // Found the position, exit
            }
          }
          // If all steps are complete in this case, start at first step
          currentStepIndex = 0;
          return;
        }
      }
    }

    // If all test cases are complete, find first execution target
    for (let caseIndex = 0; caseIndex < testCases.length; caseIndex++) {
      const testCase = testCases[caseIndex];
      if (isExecutionTarget(testCase)) {
        currentCaseIndex = caseIndex;
        currentStepIndex = 0;
        return;
      }
    }

    // Final fallback
    currentCaseIndex = 0;
    currentStepIndex = 0;
  }

  function goToCase(index) {
    currentCaseIndex = index;
    currentStepIndex = 0;
  }

  // A case is an execution target when it has steps (step format) or a
  // frozen BDD spec with example rows (bdd format).
  function isBddCase(testCase) {
    return testCase?.format === 'bdd';
  }

  function bddStatusColor(status) {
    switch (status) {
      case 'passed': return 'var(--ds-status-success-solid)';
      case 'failed': return 'var(--ds-status-danger-solid)';
      case 'blocked': return 'var(--ds-status-warning-solid)';
      case 'skipped': return 'var(--ds-status-neutral-solid)';
      default: return 'var(--ds-progress-track)';
    }
  }

  function isExecutionTarget(testCase) {
    return isBddCase(testCase) || (testCase?.test_steps && testCase.test_steps.length > 0);
  }

  function bddRowsFor(testCase) {
    return bddExampleResults[testCase?.id] || [];
  }

  function currentCaseSnapshot() {
    return bddSnapshots[currentCase?.id] || null;
  }

  function handleBddResultsChange(updatedResults) {
    bddExampleResults = {
      ...bddExampleResults,
      [currentCase.id]: updatedResults,
    };
  }

  function goToStep(index) {
    currentStepIndex = index;
  }

  function nextStep() {
    // BDD cases execute as a whole; navigation is case-level.
    if (isBddCase(currentCase)) {
      if (currentCaseIndex < testCases.length - 1) {
        currentCaseIndex++;
        currentStepIndex = 0;
      }
      return;
    }
    // If current case has steps and we're not at the last step
    if (currentCase?.test_steps?.length > 0 && currentStepIndex < currentCase.test_steps.length - 1) {
      currentStepIndex++;
    } else {
      // Move to next test case
      if (currentCaseIndex < testCases.length - 1) {
        currentCaseIndex++;
        currentStepIndex = 0;
        // If the next test case has no steps, keep moving forward
        while (currentCaseIndex < testCases.length && !isExecutionTarget(testCases[currentCaseIndex])) {
          if (currentCaseIndex < testCases.length - 1) {
            currentCaseIndex++;
          } else {
            break;
          }
        }
      }
    }
  }

  function previousStep() {
    if (isBddCase(currentCase)) {
      if (currentCaseIndex > 0) {
        currentCaseIndex--;
        currentStepIndex = 0;
      }
      return;
    }
    if (currentStepIndex > 0) {
      currentStepIndex--;
    } else if (currentCaseIndex > 0) {
      // Move to previous test case
      currentCaseIndex--;
      // If the previous test case has no steps, keep moving backward
      while (currentCaseIndex >= 0 && !isExecutionTarget(testCases[currentCaseIndex])) {
        if (currentCaseIndex > 0) {
          currentCaseIndex--;
        } else {
          break;
        }
      }
      // Set to last step of the previous test case, or 0 if no steps
      const prevCase = testCases[currentCaseIndex];
      if (isBddCase(prevCase)) {
        currentStepIndex = 0;
      } else {
        currentStepIndex = Math.max(0, (prevCase?.test_steps?.length || 1) - 1);
      }
    }
  }

  async function markStepStatus(stepId, status) {
    // Create a new object to trigger reactivity
    stepResults = {
      ...stepResults,
      [stepId]: { ...stepResults[stepId], status }
    };
    
    // Save to backend. Always send the current item_id so a previously
    // linked defect/item isn't cleared by a follow-up status change — the
    // backend overwrites items.item_id with whatever this payload carries.
    try {
      const resultData = stepResults[stepId];
      await tests.testRuns.updateStepResult(workspaceId, runId, stepId, {
        status: status,
        actual_result: resultData.actual_result || '',
        notes: resultData.notes || '',
        item_id: resultData.item_id ?? null
      });
    } catch (error) {
      console.error('Failed to save step result:', error);
    }

    // Auto-advance to next step on pass/skip (but not fail/blocked so user can create defects)
    if (status === 'passed' || status === 'skipped') {
      setTimeout(() => nextStep(), 500);
    }
  }

  async function updateStepResult(stepId, field, value) {
    // Create a new object to trigger reactivity
    stepResults = {
      ...stepResults,
      [stepId]: { ...stepResults[stepId], [field]: value }
    };

    // Save to backend, preserving any previously linked item_id.
    try {
      const resultData = stepResults[stepId];
      await tests.testRuns.updateStepResult(workspaceId, runId, stepId, {
        status: resultData.status || 'not_run',
        actual_result: resultData.actual_result || '',
        notes: resultData.notes || '',
        item_id: resultData.item_id ?? null
      });
    } catch (error) {
      console.error('Failed to save step result:', error);
    }
  }

  async function finishExecution() {
    const ok = await confirm({
      title: t('testing.finishTestExecution'),
      message: t('testing.finishConfirmMessage'),
      confirmText: t('testing.finishExecution'),
      variant: 'info',
    });
    if (!ok) return;
    try {
      await tests.testRuns.end(workspaceId, runId);
      if (fromPage === 'reports') {
        navigate(testPath('/reports'));
      } else {
        navigate(testPath('/runs'));
      }
    } catch (error) {
      console.error('Failed to finish test execution:', error);
      await confirm({
        title: t('notifications.error'),
        message: t('testing.failedToFinish'),
        confirmText: t('common.ok'),
        cancelText: '',
        variant: 'danger',
      });
    }
  }

  function goBack() {
    if (fromPage === 'reports') {
      navigate(testPath('/reports'));
    } else {
      navigate(testPath('/runs'));
    }
  }

  // Status labels and button styles are shared with the other test views.

  function getCaseProgress(testCase, currentStepResults = stepResults) {
    if (isBddCase(testCase)) {
      const rows = bddExampleResults[testCase.id] || [];
      const total = flattenExamples(parseScenarioSpec(bddSnapshots[testCase.id]?.spec))?.length || rows.length;
      const completed = rows.filter(row => row.status && row.status !== 'not_run').length;
      return {
        completed,
        total,
        percent: total > 0 ? Math.round((completed / total) * 100) : 0
      };
    }

    const steps = testCase.test_steps || [];
    if (steps.length === 0) return { completed: 0, total: 0, percent: 0 };
    
    const completed = steps.filter(step => {
      const result = currentStepResults[step.id];
      return result && result.status !== 'not_run';
    }).length;
    
    return {
      completed,
      total: steps.length,
      percent: steps.length > 0 ? Math.round((completed / steps.length) * 100) : 0
    };
  }

  function openCreateModalForStep(stepId) {
    pendingLinkStepId = stepId;
    showCreateModal = true;
  }

  async function handleItemCreated(item) {
    if (pendingLinkStepId && item) {
      await linkItemToStep(pendingLinkStepId, item);
      pendingLinkStepId = null;
    }
    await loadWorkspaceItems();
  }

  async function linkItemToStep(stepId, item) {
    // Update local state
    stepResults = {
      ...stepResults,
      [stepId]: { ...stepResults[stepId], item_id: item.id }
    };

    // Save to backend
    try {
      const resultData = stepResults[stepId];
      await tests.testRuns.updateStepResult(workspaceId, runId, stepId, {
        status: resultData.status || 'not_run',
        actual_result: resultData.actual_result || '',
        notes: resultData.notes || '',
        item_id: item.id
      });
    } catch (error) {
      console.error('Failed to link item:', error);
    }
  }
</script>

{#if loading}
  <StateDisplay type="loading" message={t('common.loading')} size="lg" />
{:else if testRun && currentCase}
  <div
    class="flex min-h-screen"
    style="background-color: var(--ds-surface);"
    data-testid="test-execution"
    data-run-id={runId}
    data-current-case-id={currentCase?.id}
    data-current-step-id={currentStep?.id}
  >
    <!-- Left Sidebar - Test Cases (Collapsible) -->
    <div class="{sidebarCollapsed ? 'w-14' : 'w-64'} border-r flex flex-col transition-all duration-200" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
      <!-- Header -->
      <div class="p-3 border-b flex items-center {sidebarCollapsed ? 'justify-center' : 'justify-between'}" style="border-color: var(--ds-border);">
        {#if !sidebarCollapsed}
          <div class="flex items-center gap-2 min-w-0">
            <BackButton
              onclick={goBack}
              testid="test-execution-back"
              tooltip={t('common.back')}
              class="p-1"
            />
            <div class="min-w-0">
              <h2 class="font-semibold text-sm truncate" style="color: var(--ds-text);">{t('testing.testExecution')}</h2>
              <div class="text-xs truncate" style="color: var(--ds-text-subtle);">
                {testRun.name}
              </div>
            </div>
          </div>
        {/if}
        <button
          onclick={() => sidebarCollapsed = !sidebarCollapsed}
          class="hover-bg p-1 rounded cursor-pointer flex-shrink-0"
          style="color: var(--ds-icon);"
          title={sidebarCollapsed ? t('testing.expandSidebar') : t('testing.collapseSidebar')}
        >
          <IconChevronLeft class="w-4 h-4 transition-transform {sidebarCollapsed ? 'rotate-180' : ''}" />
        </button>
      </div>

      <!-- Test Cases List -->
      <div class="flex-1 overflow-y-auto p-2">
        {#each testCases as testCase, index}
          {@const progress = getCaseProgress(testCase, stepResults)}
          {#if sidebarCollapsed}
            <!-- Collapsed: show only progress indicator -->
            {@const isCollapsedActive = currentCaseIndex === index}
            <button
              type="button"
              data-testid={`test-execution-case-${testCase.id}`}
              data-progress={progress.percent}
              class="case-pill appearance-none bg-transparent border-none font-[inherit] text-[inherit] text-left w-full m-0 cursor-pointer mb-2 p-1 rounded-lg transition-all"
              class:active={isCollapsedActive}
              onclick={() => goToCase(index)}
              title="{testCase.title} ({progress.percent}%)"
            >
              <div class="w-full rounded-full h-6 relative" style="background-color: var(--ds-progress-track);">
                <div
                  class="h-6 rounded-full transition-all duration-300"
                  style="width: {progress.percent}%; background-color: var(--ds-progress-fill);"
                ></div>
                <span class="absolute inset-0 flex items-center justify-center text-xs font-medium" style="color: var(--ds-text);">
                  {index + 1}
                </span>
              </div>
            </button>
          {:else}
            <!-- Expanded: show full card -->
            {@const isExpandedActive = currentCaseIndex === index}
            <button
              type="button"
              data-testid={`test-execution-case-${testCase.id}`}
              data-progress={progress.percent}
              class="case-card appearance-none bg-transparent font-[inherit] text-[inherit] text-left w-full m-0 cursor-pointer p-3 mb-2 rounded-lg border transition-all"
              class:active={isExpandedActive}
              onclick={() => goToCase(index)}
            >
              <div class="font-medium text-sm mb-1 truncate" style="color: var(--ds-text);">
                {testCase.title}
              </div>
              <div class="text-xs mb-2" style="color: var(--ds-text-subtle);">
                {t('testing.stepsProgress', { completed: progress.completed, total: progress.total, percent: progress.percent })}
              </div>
              <Progress value={progress.percent} size="sm" />
            </button>
          {/if}
        {/each}
      </div>

      <!-- Footer Actions -->
      {#if canExecuteTests}
        <div class="p-2 border-t" style="border-color: var(--ds-border);">
          {#if sidebarCollapsed}
            <Button
              onclick={finishExecution}
              variant="primary"
              size="small"
              class="w-full"
              title={t('testing.finishExecution')}
              dataTestid="test-execution-finish-sidebar"
            >
              <IconCheck class="w-4 h-4" />
            </Button>
          {:else}
            <Button
              onclick={finishExecution}
              variant="primary"
              size="medium"
              class="w-full"
              dataTestid="test-execution-finish-sidebar"
            >
              {t('testing.finishExecution')}
            </Button>
          {/if}
        </div>
      {/if}
    </div>

    <!-- Main Content - Step Execution -->
    <div class="min-w-0 flex-1 flex flex-col">
      <!-- Step Header -->
      <div class="p-6 border-b" style="border-color: var(--ds-border);">
        <div class="flex items-center justify-between gap-6 mb-4">
          <div class="min-w-0 flex-1 [overflow-wrap:anywhere]">
            <h1 class="text-xl font-semibold" style="color: var(--ds-text);">
              {currentCase.title}
            </h1>
            {#if currentCase.preconditions}
              <AlertBox variant="info" class="mt-3">
                <strong>{t('testing.preconditions')}:</strong> {currentCase.preconditions}
              </AlertBox>
            {/if}
          </div>
          <div class="shrink-0 whitespace-nowrap text-sm" style="color: var(--ds-text-subtle);">
            {#if currentCaseIsBdd}
              {t('testing.examplesCount', { count: currentExampleRows.length })}
            {:else if currentCase.test_steps && currentCase.test_steps.length > 0}
              {t('testing.stepOfTotal', { current: currentStepIndex + 1, total: currentCase.test_steps.length })}
            {:else}
              {t('testing.noStepsDefined')}
            {/if}
          </div>
        </div>

        <!-- Step Navigation -->
        <div class="flex items-center gap-2">
          <Button
            onclick={previousStep}
            disabled={currentCaseIndex === 0 && currentStepIndex === 0}
            variant="default"
            size="small"
          >
            {t('common.previous')}
          </Button>
          
          <div class="flex-1 flex gap-1">
            {#if currentCaseIsBdd}
              {#each currentExampleRows as row (row.example_index)}
                <div
                  class="step-seg flex-1 h-2 rounded"
                  style={`background-color: ${bddStatusColor(row.status)}`}
                  title={t('testing.exampleN', { n: row.example_index + 1 })}
                ></div>
              {/each}
              {#if currentExampleRows.length === 0}
                <div class="flex-1 h-2 rounded" style="background-color: var(--ds-progress-track);"></div>
              {/if}
            {:else if currentCase.test_steps && currentCase.test_steps.length > 0}
              {#each currentCase.test_steps as step, index}
                <button
                  onclick={() => goToStep(index)}
                  data-testid={`test-execution-step-${step.id}`}
                  class="step-seg flex-1 h-2 rounded transition cursor-pointer"
                  class:current={currentStepIndex === index}
                  aria-label="Step {index + 1}"
                ></button>
              {/each}
            {:else}
              <div class="flex-1 h-2 rounded" style="background-color: var(--ds-progress-track);"></div>
            {/if}
          </div>

          <Button
            onclick={nextStep}
            disabled={currentCaseIndex === testCases.length - 1 && (!currentCase.test_steps?.length || currentStepIndex === currentCase.test_steps.length - 1)}
            variant="default"
            size="small"
          >
            {t('common.next')}
          </Button>
        </div>
      </div>

      <!-- Case Content -->
      {#if currentCaseIsBdd}
        <div class="flex-1 p-6 overflow-y-auto">
          <div class="max-w-4xl">
            <BDDExampleExecution
              {workspaceId}
              {runId}
              testCase={currentCase}
              snapshot={currentCaseSnapshot()}
              initialResults={currentExampleRows}
              canExecute={canExecuteTests}
              onResultsChange={handleBddResultsChange}
            />
          </div>
        </div>
      {:else if currentStep}
        <div class="flex-1 p-6 overflow-y-auto">
          <div class="max-w-4xl">
            <!-- Step Details -->
            <div class="grid grid-cols-3 gap-6 mb-8">
              <div>
                <h3 class="font-medium mb-2" style="color: var(--ds-status-info-text);">{t('testing.action')}</h3>
                <!-- svelte-ignore a11y_click_events_have_key_events -->
                <!-- svelte-ignore a11y_no_static_element_interactions -->
                <Panel padding="default" rounded="md" class="test-step-rendered" onclick={handleRenderedContentClick}>
                  <MilkdownEditor content={currentStep.action || ''} readonly={true} showToolbar={false} />
                </Panel>
              </div>

              <div>
                <h3 class="font-medium mb-2" style="color: var(--ds-accent-purple);">{t('testing.data')}</h3>
                <!-- svelte-ignore a11y_click_events_have_key_events -->
                <!-- svelte-ignore a11y_no_static_element_interactions -->
                <Panel padding="default" rounded="md" class="test-step-rendered" onclick={handleRenderedContentClick}>
                  <MilkdownEditor content={currentStep.data || t('testing.noDataSpecified')} readonly={true} showToolbar={false} />
                </Panel>
              </div>

              <div>
                <h3 class="font-medium mb-2" style="color: var(--ds-status-success-text);">{t('testing.expected')}</h3>
                <!-- svelte-ignore a11y_click_events_have_key_events -->
                <!-- svelte-ignore a11y_no_static_element_interactions -->
                <Panel padding="default" rounded="md" class="test-step-rendered" onclick={handleRenderedContentClick}>
                  <MilkdownEditor content={currentStep.expected || ''} readonly={true} showToolbar={false} />
                </Panel>
              </div>
            </div>

            <!-- Result Recording -->
            <Card variant="raised" padding="spacious" shadow class="mb-6">
              <h3 class="font-medium mb-4" style="color: var(--ds-text);">{t('testing.recordResult')}</h3>

              {#if canExecuteTests}
              <!-- Status Buttons -->
              <div class="flex gap-3 mb-4">
                <StatusButton
                  status="passed"
                  selected={stepResults[currentStep.id]?.status === 'passed'}
                  dataTestid="test-execution-status-passed"
                  onclick={() => markStepStatus(currentStep.id, 'passed')}
                  class="flex items-center gap-2 px-4 py-2 rounded transition cursor-pointer"
                >
                  <IconCheck class="w-4 h-4" />
                  {t('testing.pass')}
                </StatusButton>

                <StatusButton
                  status="failed"
                  selected={stepResults[currentStep.id]?.status === 'failed'}
                  dataTestid="test-execution-status-failed"
                  onclick={() => markStepStatus(currentStep.id, 'failed')}
                  class="flex items-center gap-2 px-4 py-2 rounded transition cursor-pointer"
                >
                  <IconX class="w-4 h-4" />
                  {t('testing.fail')}
                </StatusButton>

                <StatusButton
                  status="blocked"
                  selected={stepResults[currentStep.id]?.status === 'blocked'}
                  dataTestid="test-execution-status-blocked"
                  onclick={() => markStepStatus(currentStep.id, 'blocked')}
                  class="flex items-center gap-2 px-4 py-2 rounded transition cursor-pointer"
                >
                  <IconBug class="w-4 h-4" />
                  {t('testing.blocked')}
                </StatusButton>

                <StatusButton
                  status="skipped"
                  selected={stepResults[currentStep.id]?.status === 'skipped'}
                  dataTestid="test-execution-status-skipped"
                  onclick={() => markStepStatus(currentStep.id, 'skipped')}
                  class="flex items-center gap-2 px-4 py-2 rounded transition cursor-pointer"
                >
                  <IconPlayerSkipForward class="w-4 h-4" />
                  {t('testing.skip')}
                </StatusButton>
              </div>

              <!-- Actual Result -->
              <div class="mb-4">
                <Label color="default" class="mb-2">{t('testing.actual')}</Label>
                {#if testResults[currentCase.id]?.id}
                  {#key currentStep.id}
                    <div data-testid="test-execution-actual-result" class="border rounded overflow-hidden" style="border-color: var(--ds-border); min-height: 80px;">
                      <MilkdownEditor
                        content={stepResults[currentStep.id]?.actual_result || ''}
                        testId="test-execution-actual-result-editor"
                        entityType="test_result"
                        entityId={testResults[currentCase.id].id}
                        showToolbar={true}
                        placeholder={t('testing.actualResultPlaceholder')}
                        onContentChange={(value) => updateStepResult(currentStep.id, 'actual_result', value)}
                      />
                    </div>
                  {/key}
                {:else}
                  <Textarea
                    value={stepResults[currentStep.id]?.actual_result || ''}
                    oninput={(e) => updateStepResult(currentStep.id, 'actual_result', e.target.value)}
                    rows={3}
                    placeholder={t('testing.actualResultPlaceholder')}
                    data-testid="test-execution-actual-result"
                  />
                {/if}
              </div>

              <!-- Notes -->
              <div class="mb-4">
                <Label color="default" class="mb-2">{t('common.notes')}</Label>
                {#if testResults[currentCase.id]?.id}
                  {#key `notes-${currentStep.id}`}
                    <div data-testid="test-execution-notes" class="border rounded overflow-hidden" style="border-color: var(--ds-border); min-height: 60px;">
                      <MilkdownEditor
                        content={stepResults[currentStep.id]?.notes || ''}
                        testId="test-execution-notes-editor"
                        entityType="test_result"
                        entityId={testResults[currentCase.id].id}
                        showToolbar={true}
                        placeholder={t('testing.notesPlaceholder')}
                        onContentChange={(value) => updateStepResult(currentStep.id, 'notes', value)}
                      />
                    </div>
                  {/key}
                {:else}
                  <Textarea
                    value={stepResults[currentStep.id]?.notes || ''}
                    oninput={(e) => updateStepResult(currentStep.id, 'notes', e.target.value)}
                    rows={2}
                    placeholder={t('testing.notesPlaceholder')}
                    data-testid="test-execution-notes"
                  />
                {/if}
              </div>

              <!-- Link Issue (shown when failed) -->
              {#if stepResults[currentStep.id]?.status === 'failed'}
                <AlertBox variant="error" class="mb-4">
                  <div class="w-full">
                  <div class="flex items-center gap-2 mb-3">
                    <h4 class="font-medium" style="color: var(--ds-status-danger-text);">{t('testing.linkIssue')}</h4>
                  </div>

                  {#if stepResults[currentStep.id]?.item_id}
                    {@const linkedItem = workspaceItems.find(i => i.id === stepResults[currentStep.id]?.item_id)}
                    <Panel padding="compact" rounded="md">
                      <div class="flex items-center justify-between">
                        <div>
                          <a
                            href={`/workspaces/${workspaceId}/items/${stepResults[currentStep.id].item_id}`}
                            data-testid="test-execution-linked-item"
                            class="font-medium text-sm hover:underline"
                            style="color: var(--ds-text-link);"
                          >
                            {linkedItem?.name || linkedItem?.title || t('testing.unknownItem')}
                          </a>
                          {#if linkedItem?.workspace_item_number}
                            <DescriptionText as="div">
                              #{linkedItem.workspace_item_number}
                            </DescriptionText>
                          {/if}
                        </div>
                        <Lozenge color="green" icon={IconLink} text={t('testing.linked')} />
                      </div>
                    </Panel>
                  {:else}
                    <div class="space-y-3">
                      <div class="text-sm" style="color: var(--ds-status-danger-text);">{t('testing.stepFailedNoIssue')}</div>

                      <!-- Link existing item -->
                      <div>
                        <span class="block text-sm font-medium mb-1" style="color: var(--ds-status-danger-text);">{t('testing.linkExistingItem')}</span>
                        <ItemPicker
                          items={workspaceItems}
                          placeholder={t('testing.searchItemsToLink')}
                          searchTestid="test-execution-item-search"
                          optionTestid={(option) => `test-execution-item-${option.value}`}
                          config={{
                            primary: { text: (item) => item.name || item.title || '' },
                            secondary: { text: (item) => item.workspace_item_number ? `#${item.workspace_item_number}` : '' },
                            searchFields: ['name', 'title', 'workspace_item_number']
                          }}
                          onSelect={(item) => linkItemToStep(currentStep.id, item)}
                        />
                      </div>

                      <!-- Or create new -->
                      <div class="text-center text-sm" style="color: var(--ds-text-subtle);">{t('testing.or')}</div>
                      <Button
                        onclick={() => openCreateModalForStep(currentStep.id)}
                        variant="default"
                        size="small"
                        icon={IconPlus}
                      >
                        {t('testing.createNewIssue')}
                      </Button>
                    </div>
                  {/if}
                  </div>
                </AlertBox>
              {/if}
              {/if}

              <!-- Quick Navigation -->
              <div class="flex justify-between items-center pt-4 border-t" style="border-color: var(--ds-border);">
                <span data-testid="test-execution-current-status">
                  <Lozenge
                    color={getTestStatusColor(stepResults[currentStep.id]?.status || 'not_run')}
                    text={`${t('common.status')}: ${getStatusLabel(stepResults[currentStep.id]?.status || 'not_run')}`}
                  />
                </span>
                
                {#if currentCaseIndex < testCases.length - 1 || (currentCase.test_steps?.length && currentStepIndex < currentCase.test_steps.length - 1)}
                  <Button
                    onclick={nextStep}
                    variant="primary"
                    size="medium"
                    icon={IconChevronRight}
                    iconPosition="right"
                  >
                    {t('testing.nextStep')}
                  </Button>
                {:else if canExecuteTests}
                  <Button
                    onclick={finishExecution}
                    variant="primary"
                    size="medium"
                    icon={IconChevronRight}
                    iconPosition="right"
                    dataTestid="test-execution-finish-current"
                  >
                    {t('testing.finishExecution')}
                  </Button>
                {/if}
              </div>
            </Card>
          </div>
        </div>
      {:else if currentCase}
        <!-- No steps content -->
        <div class="flex-1 p-6 overflow-y-auto flex items-center justify-center">
          <div class="max-w-md w-full">
            <EmptyState icon={IconPlayerSkipForward} title={t('testing.noTestSteps')} description={t('testing.noTestStepsDescription')} />
              
            <!-- Navigation for cases without steps -->
            <div class="flex justify-center items-center gap-4 pt-4 border-t" style="border-color: var(--ds-border);">
              <Button
                onclick={previousStep}
                disabled={currentCaseIndex === 0}
                variant="default"
                size="medium"
              >
                {t('testing.previousCase')}
              </Button>

              <span class="px-4 py-2 rounded text-sm" style="background-color: var(--ds-background-neutral); color: var(--ds-text);">
                {t('testing.caseOfTotal', { current: currentCaseIndex + 1, total: testCases.length })}
              </span>

              {#if currentCaseIndex < testCases.length - 1}
                <Button
                  onclick={nextStep}
                  variant="primary"
                  size="medium"
                  icon={IconChevronRight}
                  iconPosition="right"
                >
                  {t('testing.nextCase')}
                </Button>
              {:else if canExecuteTests}
                <Button
                  onclick={finishExecution}
                  variant="primary"
                  size="medium"
                  icon={IconChevronRight}
                  iconPosition="right"
                >
                  {t('testing.finishExecution')}
                </Button>
              {/if}
            </div>
          </div>
        </div>
      {/if}
    </div>
  </div>
{:else}
  <StateDisplay type="empty" title={t('testing.testRunNotFound')}>
    {#snippet action()}
      <Button onclick={goBack} variant="primary" size="medium">
        {t('testing.backToTestRuns')}
      </Button>
    {/snippet}
  </StateDisplay>
{/if}

<!-- Create Item Modal -->
<CreateModal
  bind:isOpen={showCreateModal}
  compactMode={true}
  oncreated={handleItemCreated}
/>


<!-- Image Preview Modal -->
<ModalBackdrop opacity={0.8} blur={0} bind:show={previewImage} onclose={() => (previewImage = null)}>
  <div class="relative max-w-4xl max-h-full">
    <img src={previewImage?.src} alt={previewImage?.alt} class="max-w-full max-h-[90vh] object-contain rounded" />
    <p class="text-white text-center mt-2 text-sm">{previewImage?.alt}</p>
  </div>
</ModalBackdrop>

<style>
  :global(.test-step-rendered img) {
    max-width: 300px;
    width: 100%;
    height: auto;
    cursor: pointer;
    border-radius: 6px;
    box-shadow: 0 1px 4px rgba(0, 0, 0, 0.12);
  }
  .case-pill:hover:not(.active) {
    background: var(--ds-background-neutral-hovered);
  }

  .case-card {
    border-color: var(--ds-border);
  }

  .case-card:hover:not(.active) {
    background: var(--ds-background-neutral-hovered);
  }

  .case-card.active {
    border-color: var(--ds-interactive);
    background: var(--ds-surface);
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1), 0 1px 2px rgba(0, 0, 0, 0.06);
  }

  .step-seg {
    background-color: var(--ds-progress-track);
  }

  .step-seg.current {
    background-color: var(--ds-progress-fill);
  }

  .step-seg:hover:not(.current) {
    background-color: var(--ds-background-neutral-hovered);
  }
</style>
