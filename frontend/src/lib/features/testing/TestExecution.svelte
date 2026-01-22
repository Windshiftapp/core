<script>
  import { onMount } from 'svelte';
  import { currentRoute, navigate } from '../../router.js';
  import { api } from '../../api.js';
  import { Check, X, Bug, ArrowLeft, ChevronRight, ChevronLeft, AlertTriangle, Plus, Link, SkipForward } from 'lucide-svelte';
  import ConfirmDialog from '../../dialogs/ConfirmDialog.svelte';
  import Button from '../../components/Button.svelte';
  import Spinner from '../../components/Spinner.svelte';
  import MilkdownEditor from '../../editors/LazyMilkdownEditor.svelte';
  import ItemPicker from '../../pickers/ItemPicker.svelte';
  import CreateModal from '../../dialogs/CreateModal.svelte';
  import Label from '../../components/Label.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import { getStatusBadgeCSS, getStatusLabel, getStatusButtonStyle, getStatusButtonHoverStyle } from '../../utils/statusColors.js';
  import { t } from '../../stores/i18n.svelte.js';

  let testRun = null;
  let testCases = [];
  let currentCaseIndex = 0;
  let currentStepIndex = 0;
  let testResults = {};
  let stepResults = {};
  let loading = true;
  let workspaceItems = [];
  let showCreateModal = false;
  let pendingLinkStepId = null;
  let sidebarCollapsed = false;
  let showFinishConfirmation = false;
  let showErrorDialog = false;
  let errorMessage = '';
  let previewImage = null;

  function handleRenderedContentClick(event) {
    if (event.target.tagName === 'IMG') {
      previewImage = {
        src: event.target.src,
        alt: event.target.alt || 'Image preview'
      };
    }
  }

  $: workspaceId = $currentRoute.params.id;
  $: runId = $currentRoute.params.runId;
  $: fromPage = $currentRoute.query?.from;
  $: currentCase = (Array.isArray(testCases) && testCases[currentCaseIndex]) || null;
  $: currentStep = currentCase?.test_steps?.[currentStepIndex] || null;

  function testPath(suffix = '') {
    const base = workspaceId ? `/workspaces/${workspaceId}/tests` : '/workspaces';
    return `${base}${suffix}`;
  }

  onMount(async () => {
    if (runId) {
      await loadTestRun(runId);
      await loadWorkspaceItems();
    }
  });

  async function loadWorkspaceItems() {
    try {
      const response = await api.items.getAll({ workspace_id: workspaceId });
      // API returns { items: [], total_count, page, limit }
      workspaceItems = response?.items || [];
    } catch (error) {
      console.error('Failed to load workspace items:', error);
      workspaceItems = [];
    }
  }

  async function loadTestRun(runId) {
    try {
      loading = true;

      // Load test run details
      testRun = await api.tests.testRuns.get(workspaceId, runId);
      if (!testRun) {
        throw new Error('Test run not found');
      }

      // Load test cases for this run's test plan
      const testSet = await api.tests.testSets.get(workspaceId, testRun.set_id);
      if (!testSet) {
        throw new Error('Test set not found');
      }

      const loadedTestCases = await api.tests.testSets.getTestCases(workspaceId, testRun.set_id);

      // Ensure we have an array
      if (!Array.isArray(loadedTestCases)) {
        console.error('getTestCases did not return an array:', loadedTestCases);
        testCases = [];
        return;
      }

      // Load test steps for each test case
      // Create a new array with steps to trigger proper reactivity
      const testCasesWithSteps = [];
      for (let testCase of loadedTestCases) {
        try {
          const steps = await api.tests.testCases.steps.getAll(workspaceId, testCase.id);
          const testCaseWithSteps = {
            ...testCase,
            test_steps: Array.isArray(steps) ? steps : []
          };
          testCasesWithSteps.push(testCaseWithSteps);
        } catch (error) {
          console.error(`Failed to load steps for test case ${testCase.id}:`, error);
          testCasesWithSteps.push({
            ...testCase,
            test_steps: []
          });
        }
      }

      // Update the testCases array all at once to trigger proper reactivity
      testCases = testCasesWithSteps;

      // Initialize results
      initializeResults();

      // Load existing step results from database
      await loadExistingResults();

    } catch (error) {
      console.error('Failed to load test run:', error);
      errorMessage = error.message || 'Failed to load test run';
      showErrorDialog = true;
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
            defect_id: null
          };
        }
      });
    });
  }

  async function loadExistingResults() {
    try {
      // Load step results from the database
      const existingStepResults = await api.tests.testRuns.getStepResults(workspaceId, runId);
      
      // Merge existing results with initialized results
      Object.keys(existingStepResults).forEach(stepId => {
        const existingResult = existingStepResults[stepId];
        stepResults[stepId] = {
          status: existingResult.status || 'not_run',
          actual_result: existingResult.actual_result || '',
          notes: existingResult.notes || '',
          defect_id: existingResult.defect_id || null
        };
      });
      
      // Trigger reactivity
      stepResults = { ...stepResults };
    } catch (error) {
      console.error('Failed to load existing step results:', error);
    }
  }

  function setInitialTestCasePosition() {
    // Safety check
    if (!Array.isArray(testCases) || testCases.length === 0) {
      return;
    }

    // Find the first test case that has steps and incomplete work
    for (let caseIndex = 0; caseIndex < testCases.length; caseIndex++) {
      const testCase = testCases[caseIndex];

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

    // If all test cases are complete or no steps found, find first case with steps
    for (let caseIndex = 0; caseIndex < testCases.length; caseIndex++) {
      const testCase = testCases[caseIndex];
      if (testCase.test_steps && testCase.test_steps.length > 0) {
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

  function goToStep(index) {
    currentStepIndex = index;
  }

  function nextStep() {
    // If current case has steps and we're not at the last step
    if (currentCase?.test_steps?.length > 0 && currentStepIndex < currentCase.test_steps.length - 1) {
      currentStepIndex++;
    } else {
      // Move to next test case
      if (currentCaseIndex < testCases.length - 1) {
        currentCaseIndex++;
        currentStepIndex = 0;
        // If the next test case has no steps, keep moving forward
        while (currentCaseIndex < testCases.length && (!testCases[currentCaseIndex].test_steps || testCases[currentCaseIndex].test_steps.length === 0)) {
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
    if (currentStepIndex > 0) {
      currentStepIndex--;
    } else if (currentCaseIndex > 0) {
      // Move to previous test case
      currentCaseIndex--;
      // If the previous test case has no steps, keep moving backward
      while (currentCaseIndex >= 0 && (!testCases[currentCaseIndex].test_steps || testCases[currentCaseIndex].test_steps.length === 0)) {
        if (currentCaseIndex > 0) {
          currentCaseIndex--;
        } else {
          break;
        }
      }
      // Set to last step of the previous test case, or 0 if no steps
      const prevCase = testCases[currentCaseIndex];
      currentStepIndex = Math.max(0, (prevCase?.test_steps?.length || 1) - 1);
    }
  }

  async function markStepStatus(stepId, status) {
    // Create a new object to trigger reactivity
    stepResults = {
      ...stepResults,
      [stepId]: { ...stepResults[stepId], status }
    };
    
    // Save to backend
    try {
      const resultData = stepResults[stepId];
      await api.tests.testRuns.updateStepResult(workspaceId, runId, stepId, {
        status: status,
        actual_result: resultData.actual_result || '',
        notes: resultData.notes || '',
        defect_id: resultData.defect_id || null
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

    // Save to backend
    try {
      const resultData = stepResults[stepId];
      await api.tests.testRuns.updateStepResult(workspaceId, runId, stepId, {
        status: resultData.status || 'not_run',
        actual_result: resultData.actual_result || '',
        notes: resultData.notes || '',
        defect_id: resultData.defect_id || null
      });
    } catch (error) {
      console.error('Failed to save step result:', error);
    }
  }

  function finishExecution() {
    showFinishConfirmation = true;
  }

  async function confirmFinishExecution() {
    try {
      // End the test run
      await api.tests.testRuns.end(workspaceId, runId);
      
      // Navigate back to appropriate page
      if (fromPage === 'reports') {
        navigate(testPath('/reports'));
      } else {
        navigate(testPath('/runs'));
      }
    } catch (error) {
      console.error('Failed to finish test execution:', error);
      errorMessage = t('testing.failedToFinish');
      showErrorDialog = true;
    }
  }

  function goBack() {
    if (fromPage === 'reports') {
      navigate(testPath('/reports'));
    } else {
      navigate(testPath('/runs'));
    }
  }

  // Status colors now handled by imported utility (getStatusBadgeCSS, getStatusLabel)

  function getCaseProgress(testCase, currentStepResults = stepResults) {
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

  async function handleItemCreated(event) {
    const item = event.detail;
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
      await api.tests.testRuns.updateStepResult(workspaceId, runId, stepId, {
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
  <div class="flex items-center justify-center h-96">
    <Spinner size="lg" />
  </div>
{:else if testRun && currentCase}
  <div class="flex min-h-screen" style="background-color: var(--ds-surface-raised);">
    <!-- Left Sidebar - Test Cases (Collapsible) -->
    <div class="{sidebarCollapsed ? 'w-14' : 'w-64'} border-r flex flex-col transition-all duration-200" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
      <!-- Header -->
      <div class="p-3 border-b flex items-center {sidebarCollapsed ? 'justify-center' : 'justify-between'}" style="border-color: var(--ds-border);">
        {#if !sidebarCollapsed}
          <div class="flex items-center gap-2 min-w-0">
            <button
              onclick={goBack}
              class="p-1 rounded cursor-pointer flex-shrink-0"
              style="color: var(--ds-icon);"
              onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
              onmouseleave={(e) => e.currentTarget.style.backgroundColor = ''}
            >
              <ArrowLeft class="w-4 h-4" />
            </button>
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
          class="p-1 rounded cursor-pointer flex-shrink-0"
          style="color: var(--ds-icon);"
          title={sidebarCollapsed ? t('testing.expandSidebar') : t('testing.collapseSidebar')}
          onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
          onmouseleave={(e) => e.currentTarget.style.backgroundColor = ''}
        >
          <ChevronLeft class="w-4 h-4 transition-transform {sidebarCollapsed ? 'rotate-180' : ''}" />
        </button>
      </div>

      <!-- Test Cases List -->
      <div class="flex-1 overflow-y-auto p-2">
        {#each testCases as testCase, index}
          {@const progress = getCaseProgress(testCase, stepResults)}
          {#if sidebarCollapsed}
            <!-- Collapsed: show only progress indicator -->
            {@const isCollapsedActive = currentCaseIndex === index}
            <div
              class="mb-2 p-1 rounded-lg cursor-pointer transition-all"
              style={isCollapsedActive ? 'background: var(--ds-surface); box-shadow: 0 1px 3px rgba(0,0,0,0.1), 0 1px 2px rgba(0,0,0,0.06);' : ''}
              onmouseenter={(e) => { if (!isCollapsedActive) e.currentTarget.style.background = 'var(--ds-background-neutral-hovered)'; }}
              onmouseleave={(e) => { if (!isCollapsedActive) e.currentTarget.style.background = ''; }}
              onclick={() => goToCase(index)}
              role="button"
              tabindex="0"
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
            </div>
          {:else}
            <!-- Expanded: show full card -->
            {@const isExpandedActive = currentCaseIndex === index}
            <div
              class="p-3 mb-2 rounded-lg border cursor-pointer transition-all"
              style={isExpandedActive ? 'border-color: var(--ds-interactive); background: var(--ds-surface); box-shadow: 0 1px 3px rgba(0,0,0,0.1), 0 1px 2px rgba(0,0,0,0.06);' : 'border-color: var(--ds-border);'}
              onmouseenter={(e) => { if (!isExpandedActive) e.currentTarget.style.background = 'var(--ds-background-neutral-hovered)'; }}
              onmouseleave={(e) => { if (!isExpandedActive) e.currentTarget.style.background = ''; }}
              onclick={() => goToCase(index)}
              role="button"
              tabindex="0"
            >
              <div class="font-medium text-sm mb-1 truncate" style="color: var(--ds-text);">
                {testCase.title}
              </div>
              <div class="text-xs mb-2" style="color: var(--ds-text-subtle);">
                {t('testing.stepsProgress', { completed: progress.completed, total: progress.total, percent: progress.percent })}
              </div>
              <div class="w-full rounded-full h-1" style="background-color: var(--ds-progress-track);">
                <div
                  class="h-1 rounded-full transition-all duration-300"
                  style="width: {progress.percent}%; background-color: var(--ds-progress-fill);"
                ></div>
              </div>
            </div>
          {/if}
        {/each}
      </div>

      <!-- Footer Actions -->
      <div class="p-2 border-t" style="border-color: var(--ds-border);">
        {#if sidebarCollapsed}
          <Button
            onclick={finishExecution}
            variant="primary"
            size="small"
            class="w-full"
            title={t('testing.finishExecution')}
          >
            <Check class="w-4 h-4" />
          </Button>
        {:else}
          <Button
            onclick={finishExecution}
            variant="primary"
            size="medium"
            class="w-full"
          >
            {t('testing.finishExecution')}
          </Button>
        {/if}
      </div>
    </div>

    <!-- Main Content - Step Execution -->
    <div class="flex-1 flex flex-col">
      <!-- Step Header -->
      <div class="p-6 border-b" style="border-color: var(--ds-border);">
        <div class="flex items-center justify-between mb-4">
          <div>
            <h1 class="text-xl font-semibold" style="color: var(--ds-text);">
              {currentCase.title}
            </h1>
            {#if currentCase.preconditions}
              <div class="text-sm mt-2 px-3 py-2 rounded border-l-4" style="background-color: var(--ds-background-neutral); border-left-color: var(--ds-status-info-solid);">
                <strong style="color: var(--ds-text);">{t('testing.preconditions')}</strong> <span style="color: var(--ds-text-subtle);">{currentCase.preconditions}</span>
              </div>
            {/if}
          </div>
          <div class="text-sm" style="color: var(--ds-text-subtle);">
            {#if currentCase.test_steps && currentCase.test_steps.length > 0}
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
            {#if currentCase.test_steps && currentCase.test_steps.length > 0}
              {#each currentCase.test_steps as step, index}
                <button
                  onclick={() => goToStep(index)}
                  class="flex-1 h-2 rounded transition cursor-pointer"
                  style="background-color: {currentStepIndex === index ? 'var(--ds-progress-fill)' : 'var(--ds-progress-track)'};"
                  onmouseenter={(e) => { if (currentStepIndex !== index) e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'; }}
                  onmouseleave={(e) => { if (currentStepIndex !== index) e.currentTarget.style.backgroundColor = 'var(--ds-progress-track)'; }}
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

      <!-- Step Content -->
      {#if currentStep}
        <div class="flex-1 p-6 overflow-y-auto">
          <div class="max-w-4xl">
            <!-- Step Details -->
            <div class="grid grid-cols-3 gap-6 mb-8">
              <div>
                <h3 class="font-medium mb-2" style="color: var(--ds-status-info-text);">{t('testing.action')}</h3>
                <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
                <div class="p-4 border rounded test-step-rendered" style="border-color: var(--ds-border); background-color: var(--ds-background-neutral);" onclick={handleRenderedContentClick}>
                  <MilkdownEditor content={currentStep.action || ''} readonly={true} showToolbar={false} />
                </div>
              </div>

              <div>
                <h3 class="font-medium mb-2" style="color: var(--ds-accent-purple);">{t('testing.data')}</h3>
                <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
                <div class="p-4 border rounded test-step-rendered" style="border-color: var(--ds-border); background-color: var(--ds-background-neutral);" onclick={handleRenderedContentClick}>
                  <MilkdownEditor content={currentStep.data || t('testing.noDataSpecified')} readonly={true} showToolbar={false} />
                </div>
              </div>

              <div>
                <h3 class="font-medium mb-2" style="color: var(--ds-status-success-text);">{t('testing.expected')}</h3>
                <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
                <div class="p-4 border rounded test-step-rendered" style="border-color: var(--ds-border); background-color: var(--ds-background-neutral);" onclick={handleRenderedContentClick}>
                  <MilkdownEditor content={currentStep.expected || ''} readonly={true} showToolbar={false} />
                </div>
              </div>
            </div>

            <!-- Result Recording -->
            <div class="mb-6">
              <h3 class="font-medium mb-4" style="color: var(--ds-text);">{t('testing.recordResult')}</h3>
              
              <!-- Status Buttons -->
              <div class="flex gap-3 mb-4">
                <button
                  onclick={() => markStepStatus(currentStep.id, 'passed')}
                  class="flex items-center gap-2 px-4 py-2 rounded transition cursor-pointer"
                  style={getStatusButtonStyle('passed', stepResults[currentStep.id]?.status === 'passed')}
                  onmouseenter={(e) => { if (stepResults[currentStep.id]?.status !== 'passed') e.currentTarget.style.backgroundColor = 'var(--ds-status-success-bg)'; }}
                  onmouseleave={(e) => { if (stepResults[currentStep.id]?.status !== 'passed') e.currentTarget.style.backgroundColor = 'transparent'; }}
                >
                  <Check class="w-4 h-4" />
                  {t('testing.pass')}
                </button>

                <button
                  onclick={() => markStepStatus(currentStep.id, 'failed')}
                  class="flex items-center gap-2 px-4 py-2 rounded transition cursor-pointer"
                  style={getStatusButtonStyle('failed', stepResults[currentStep.id]?.status === 'failed')}
                  onmouseenter={(e) => { if (stepResults[currentStep.id]?.status !== 'failed') e.currentTarget.style.backgroundColor = 'var(--ds-status-danger-bg)'; }}
                  onmouseleave={(e) => { if (stepResults[currentStep.id]?.status !== 'failed') e.currentTarget.style.backgroundColor = 'transparent'; }}
                >
                  <X class="w-4 h-4" />
                  {t('testing.fail')}
                </button>

                <button
                  onclick={() => markStepStatus(currentStep.id, 'blocked')}
                  class="flex items-center gap-2 px-4 py-2 rounded transition cursor-pointer"
                  style={getStatusButtonStyle('blocked', stepResults[currentStep.id]?.status === 'blocked')}
                  onmouseenter={(e) => { if (stepResults[currentStep.id]?.status !== 'blocked') e.currentTarget.style.backgroundColor = 'var(--ds-status-warning-bg)'; }}
                  onmouseleave={(e) => { if (stepResults[currentStep.id]?.status !== 'blocked') e.currentTarget.style.backgroundColor = 'transparent'; }}
                >
                  <Bug class="w-4 h-4" />
                  {t('testing.blocked')}
                </button>

                <button
                  onclick={() => markStepStatus(currentStep.id, 'skipped')}
                  class="flex items-center gap-2 px-4 py-2 rounded transition cursor-pointer"
                  style={getStatusButtonStyle('skipped', stepResults[currentStep.id]?.status === 'skipped')}
                  onmouseenter={(e) => { if (stepResults[currentStep.id]?.status !== 'skipped') e.currentTarget.style.backgroundColor = 'var(--ds-status-neutral-bg)'; }}
                  onmouseleave={(e) => { if (stepResults[currentStep.id]?.status !== 'skipped') e.currentTarget.style.backgroundColor = 'transparent'; }}
                >
                  <SkipForward class="w-4 h-4" />
                  {t('testing.skip')}
                </button>
              </div>

              <!-- Actual Result -->
              <div class="mb-4">
                <Label color="default" class="mb-2">{t('testing.actual')}</Label>
                <Textarea
                  value={stepResults[currentStep.id]?.actual_result || ''}
                  on:input={(e) => updateStepResult(currentStep.id, 'actual_result', e.target.value)}
                  rows={3}
                  placeholder={t('testing.actualResultPlaceholder')}
                />
              </div>

              <!-- Notes -->
              <div class="mb-4">
                <Label color="default" class="mb-2">{t('common.notes')}</Label>
                <Textarea
                  value={stepResults[currentStep.id]?.notes || ''}
                  on:input={(e) => updateStepResult(currentStep.id, 'notes', e.target.value)}
                  rows={2}
                  placeholder={t('testing.notesPlaceholder')}
                />
              </div>

              <!-- Link Issue (shown when failed) -->
              {#if stepResults[currentStep.id]?.status === 'failed'}
                <div class="mb-4 p-4 rounded" style="background-color: var(--ds-status-danger-bg); border: 1px solid var(--ds-status-danger-border);">
                  <div class="flex items-center gap-2 mb-3">
                    <AlertTriangle class="w-4 h-4" style="color: var(--ds-status-danger-text);" />
                    <h4 class="font-medium" style="color: var(--ds-status-danger-text);">{t('testing.linkIssue')}</h4>
                  </div>

                  {#if stepResults[currentStep.id]?.item_id}
                    {@const linkedItem = workspaceItems.find(i => i.id === stepResults[currentStep.id]?.item_id)}
                    <div class="p-3 rounded border" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
                      <div class="flex items-center justify-between">
                        <div>
                          <div class="font-medium text-sm" style="color: var(--ds-text);">{linkedItem?.name || linkedItem?.title || t('testing.unknownItem')}</div>
                          {#if linkedItem?.workspace_item_number}
                            <div class="text-xs mt-1" style="color: var(--ds-text-subtle);">
                              #{linkedItem.workspace_item_number}
                            </div>
                          {/if}
                        </div>
                        <span class="px-2 py-1 text-xs rounded flex items-center gap-1" style="background-color: var(--ds-status-success-bg); color: var(--ds-status-success-text);">
                          <Link class="w-3 h-3" />
                          {t('testing.linked')}
                        </span>
                      </div>
                    </div>
                  {:else}
                    <div class="space-y-3">
                      <div class="text-sm" style="color: var(--ds-status-danger-text);">{t('testing.stepFailedNoIssue')}</div>

                      <!-- Link existing item -->
                      <div>
                        <label class="block text-sm font-medium mb-1" style="color: var(--ds-status-danger-text);">{t('testing.linkExistingItem')}</label>
                        <ItemPicker
                          items={workspaceItems}
                          placeholder={t('testing.searchItemsToLink')}
                          config={{
                            primary: { text: (item) => item.name || item.title || '' },
                            secondary: { text: (item) => item.workspace_item_number ? `#${item.workspace_item_number}` : '' },
                            searchFields: ['name', 'title', 'workspace_item_number']
                          }}
                          on:select={(e) => linkItemToStep(currentStep.id, e.detail)}
                        />
                      </div>

                      <!-- Or create new -->
                      <div class="text-center text-sm" style="color: var(--ds-text-subtle);">{t('testing.or')}</div>
                      <Button
                        onclick={() => openCreateModalForStep(currentStep.id)}
                        variant="default"
                        size="small"
                        icon={Plus}
                      >
                        {t('testing.createNewIssue')}
                      </Button>
                    </div>
                  {/if}
                </div>
              {/if}

              <!-- Quick Navigation -->
              <div class="flex justify-between items-center pt-4 border-t" style="border-color: var(--ds-border);">
                <span class="text-sm px-2 py-1 rounded" style={getStatusBadgeCSS(stepResults[currentStep.id]?.status || 'not_run')}>
                  {t('common.status')}: {getStatusLabel(stepResults[currentStep.id]?.status || 'not_run')}
                </span>
                
                {#if currentCaseIndex < testCases.length - 1 || (currentCase.test_steps?.length && currentStepIndex < currentCase.test_steps.length - 1)}
                  <Button
                    onclick={nextStep}
                    variant="primary"
                    size="medium"
                  >
                    {t('testing.nextStep')}
                    <ChevronRight slot="icon-right" class="w-4 h-4" />
                  </Button>
                {:else}
                  <Button
                    onclick={finishExecution}
                    variant="primary"
                    size="medium"
                  >
                    {t('testing.finishExecution')}
                    <ChevronRight slot="icon-right" class="w-4 h-4" />
                  </Button>
                {/if}
              </div>
            </div>
          </div>
        </div>
      {:else if currentCase}
        <!-- No steps content -->
        <div class="flex-1 p-6 overflow-y-auto flex items-center justify-center">
          <div class="max-w-md text-center">
            <div class="text-6xl mb-4">📝</div>
            <h3 class="text-lg font-medium mb-2" style="color: var(--ds-text);">{t('testing.noTestSteps')}</h3>
            <p class="text-sm mb-6" style="color: var(--ds-text-subtle);">
              {t('testing.noTestStepsDescription')}
            </p>
              
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
                >
                  {t('testing.nextCase')}
                  <ChevronRight slot="icon-right" class="w-4 h-4" />
                </Button>
              {:else}
                <Button
                  onclick={finishExecution}
                  variant="primary"
                  size="medium"
                >
                  {t('testing.finishExecution')}
                  <ChevronRight slot="icon-right" class="w-4 h-4" />
                </Button>
              {/if}
            </div>
          </div>
        </div>
      {/if}
    </div>
  </div>
{:else}
  <div class="text-center py-12">
    <div style="color: var(--ds-text-subtle);">{t('testing.testRunNotFound')}</div>
    <Button
      onclick={goBack}
      variant="primary"
      size="medium"
      class="mt-4"
    >
      {t('testing.backToTestRuns')}
    </Button>
  </div>
{/if}

<!-- Create Item Modal -->
<CreateModal
  bind:isOpen={showCreateModal}
  compactMode={true}
  on:created={handleItemCreated}
/>

<!-- Finish Execution Confirmation Dialog -->
<ConfirmDialog
  bind:show={showFinishConfirmation}
  title={t('testing.finishTestExecution')}
  message={t('testing.finishConfirmMessage')}
  confirmText={t('testing.finishExecution')}
  cancelText={t('common.cancel')}
  variant="info"
  onconfirm={confirmFinishExecution}
  oncancel={() => showFinishConfirmation = false}
/>

<!-- Error Dialog -->
<ConfirmDialog
  bind:show={showErrorDialog}
  title={t('notifications.error')}
  message={errorMessage}
  confirmText={t('common.ok')}
  cancelText=""
  variant="danger"
  onconfirm={() => showErrorDialog = false}
  oncancel={() => showErrorDialog = false}
/>

<!-- Image Preview Modal -->
{#if previewImage}
  <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
  <div class="fixed inset-0 bg-black/80 flex items-center justify-center z-50 p-4"
       onclick={() => previewImage = null}>
    <div class="relative max-w-4xl max-h-full">
      <img src={previewImage.src} alt={previewImage.alt} class="max-w-full max-h-[90vh] object-contain rounded" />
      <p class="text-white text-center mt-2 text-sm">{previewImage.alt}</p>
    </div>
  </div>
{/if}

<style>
  :global(.test-step-rendered img) {
    max-width: 300px;
    width: 100%;
    height: auto;
    cursor: pointer;
    border-radius: 6px;
    box-shadow: 0 1px 4px rgba(0, 0, 0, 0.12);
  }
</style>
