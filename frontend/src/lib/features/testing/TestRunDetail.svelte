<script>
  import { onMount } from 'svelte';
  import { currentRoute, navigate } from '../../router.js';
  import { api } from '../../api.js';
  import { ArrowLeft, Play, Check, X, Bug, AlertTriangle, FileText, Trash2 } from 'lucide-svelte';
  import Button from '../../components/Button.svelte';
  import ConfirmDialog from '../../dialogs/ConfirmDialog.svelte';
  import Spinner from '../../components/Spinner.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import { getStatusBadgeCSS, getStatusLabel } from '../../utils/testStatusColors.js';

  let testRun = null;
  let testResults = [];
  let loading = true;

  $: workspaceId = $currentRoute.params.id;
  $: runId = $currentRoute.params.runId;
  $: fromPage = $currentRoute.query?.from;

  onMount(async () => {
    if (runId) {
      await loadTestRun(runId);
    }
  });

  async function loadTestRun(runId) {
    try {
      loading = true;
      testRun = await api.tests.testRuns.get(workspaceId, runId);
      
      // Load test results if the run has been executed
      if (testRun.ended_at) {
        const results = await api.tests.testRuns.getResults(workspaceId, runId);
        const stepResults = await api.tests.testRuns.getStepResults(workspaceId, runId);
        
        // Load test cases to get their steps for proper association
        const testSet = await api.tests.testSets.get(workspaceId, testRun.set_id);
        const testCases = await api.tests.testSets.getTestCases(workspaceId, testRun.set_id);
        
        // Load test steps for each test case
        for (let testCase of testCases) {
          testCase.test_steps = await api.tests.testCases.steps.getAll(workspaceId, testCase.id) || [];
        }
        
        // Combine results with step results for display
        testResults = results.map(result => {
          // Find the corresponding test case
          const testCase = testCases.find(tc => tc.id === result.test_case_id);
          
          // Get step results that belong to this test case
          const caseStepResults = {};
          if (testCase && testCase.test_steps) {
            testCase.test_steps.forEach(step => {
              // Use composite key to avoid conflicts between test cases
              const compositeKey = `${testCase.id}_${step.id}`;
              if (stepResults[compositeKey]) {
                caseStepResults[step.id] = stepResults[compositeKey];
              }
            });
          }
          
          return {
            ...result,
            test_steps: testCase?.test_steps || [],
            stepResults: caseStepResults
          };
        });
      }
    } catch (error) {
      console.error('Failed to load test run:', error);
    } finally {
      loading = false;
    }
  }

  function goBack() {
    if (fromPage === 'reports') {
      navigate(testPath('/reports'));
    } else {
      navigate(testPath('/runs'));
    }
  }

  function testPath(suffix = '') {
    const base = workspaceId ? `/workspaces/${workspaceId}/tests` : '/workspaces';
    return `${base}${suffix}`;
  }

  async function exportResults() {
    if (!testRun || !testRun.ended_at) {
      alert('No test results available for export. Please execute the test run first.');
      return;
    }
    
    try {
      const response = await fetch(`/api/test-runs/${runId}/summary`);
      const data = await response.json();
      
      // Open summary in a new window/tab with formatted markdown
      const summaryWindow = window.open('', '_blank');
      summaryWindow.document.write(`
        <!DOCTYPE html>
        <html>
        <head>
          <title>Test Run Summary - ${testRun.name}</title>
          <style>
            body { font-family: Arial, sans-serif; max-width: 800px; margin: 0 auto; padding: 20px; }
            pre { background: #f5f5f5; padding: 15px; border-radius: 5px; white-space: pre-wrap; }
            table { border-collapse: collapse; width: 100%; margin: 15px 0; }
            th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }
            th { background-color: #f2f2f2; }
          </style>
        </head>
        <body>
          <pre>${data.markdown}</pre>
        </body>
        </html>
      `);
      summaryWindow.document.close();
    } catch (error) {
      console.error('Failed to get summary:', error);
      alert('Failed to load test run summary');
    }
  }

  async function executeRun() {
    try {
      // Get existing runs for this test plan to generate sequential numbering
      const setRuns = await api.tests.testSets.getRuns(workspaceId, testRun.set_id);
      const executionCount = setRuns.length;
      
      const newRunName = prompt(
        `Enter name for this test execution:`, 
        `${testRun.name} - Run ${executionCount + 1}`
      );
      
      if (!newRunName) {
        return; // User cancelled
      }
      
      // Create a new test run instance for this execution
      const newRun = await api.tests.testRuns.create(workspaceId, {
        set_id: testRun.set_id,
        name: newRunName
      });
      
      // Navigate to execute the new run
      navigate(testPath(`/runs/${newRun.id}/execute`));
    } catch (error) {
      console.error('Failed to create execution run:', error);
    }
  }

  // Status colors now handled by imported utility (getStatusBadgeCSS, getStatusLabel)

  function getStatusIcon(status) {
    switch (status) {
      case 'passed': return '✓';
      case 'failed': return '✗';
      case 'blocked': return '⚠';
      case 'skipped': return '⊘';
      default: return '○';
    }
  }

  function getStepStatusStyle(status) {
    const styles = {
      'passed': 'var(--ds-status-success-solid)',
      'failed': 'var(--ds-status-danger-solid)',
      'blocked': 'var(--ds-status-warning-solid)',
      'skipped': 'var(--ds-status-neutral-solid)',
      'not_run': 'var(--ds-status-neutral-border)'
    };
    return styles[status] || styles['not_run'];
  }

  function getResultsSummary(results) {
    const summary = {
      total: results.length,
      passed: 0,
      failed: 0,
      blocked: 0,
      skipped: 0,
      not_run: 0
    };

    results.forEach(result => {
      summary[result.status] = (summary[result.status] || 0) + 1;
    });

    const executedTests = summary.total - summary.not_run;
    summary.successRate = executedTests > 0 ? Math.round((summary.passed / executedTests) * 100) : 0;

    return summary;
  }

  function getDuration(startTime, endTime) {
    const start = new Date(startTime);
    const end = new Date(endTime);
    const diffMs = end - start;

    const hours = Math.floor(diffMs / (1000 * 60 * 60));
    const minutes = Math.floor((diffMs % (1000 * 60 * 60)) / (1000 * 60));
    const seconds = Math.floor((diffMs % (1000 * 60)) / 1000);

    if (hours > 0) {
      return `${hours}h ${minutes}m ${seconds}s`;
    } else if (minutes > 0) {
      return `${minutes}m ${seconds}s`;
    } else {
      return `${seconds}s`;
    }
  }

  // Delete confirmation
  let showDeleteConfirm = false;

  function confirmDelete() {
    showDeleteConfirm = true;
  }

  async function deleteRun() {
    if (!testRun) return;

    try {
      await api.tests.testRuns.delete(workspaceId, testRun.id);
      showDeleteConfirm = false;
      // Navigate back to the test runs list
      if (fromPage === 'reports') {
        navigate(testPath('/reports'));
      } else {
        navigate(testPath('/runs'));
      }
    } catch (error) {
      console.error('Failed to delete test run:', error);
      alert('Failed to delete test run: ' + error.message);
    }
  }
</script>

<div class="min-h-screen flex flex-col p-6" style="background-color: var(--ds-surface-raised);">
  <div class="flex-1 -mx-6 -mb-6 px-10 py-6">
    {#if loading}
      <div class="flex items-center justify-center py-12">
        <Spinner />
      </div>
    {:else if testRun}
      <!-- Header -->
      <div class="flex items-center justify-between mb-6">
        <div class="flex items-center gap-3">
          <button
            onclick={goBack}
            class="p-2 rounded cursor-pointer"
            style="color: var(--ds-icon);"
            onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
            onmouseleave={(e) => e.currentTarget.style.backgroundColor = ''}
          >
            <ArrowLeft class="w-5 h-5" />
          </button>
          <div>
            <h1 class="text-2xl font-semibold" style="color: var(--ds-text);">
              {testRun.name}
            </h1>
            <div class="text-sm mt-1" style="color: var(--ds-text-subtle);">
              Started: {new Date(testRun.started_at).toLocaleString()}
              {#if testRun.ended_at}
                • Ended: {new Date(testRun.ended_at).toLocaleString()}
              {/if}
            </div>
          </div>
        </div>

        <div class="flex items-center gap-3">
          {#if testRun.ended_at}
            <Button
              onclick={exportResults}
              variant="primary"
              size="medium"
              icon={FileText}
            >
              Export Results
            </Button>
          {:else}
            <Button
              variant="primary"
              onclick={() => navigate(testPath(`/runs/${runId}/execute`))}
              icon={Play}
              size="medium"
            >
              Continue Execution
            </Button>
          {/if}
          <Button
            onclick={confirmDelete}
            variant="danger"
            size="medium"
            icon={Trash2}
            title="Delete test run"
          >
            Delete
          </Button>
        </div>
      </div>

      <!-- Test Run Details -->
      <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <!-- Status Overview -->
        <div class="lg:col-span-2">
          <div>
            <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Test Results</h2>
            
            {#if testResults.length > 0}
              <!-- Test Results Display -->
              <div class="space-y-4">
                {#each testResults as result}
                  <div class="border rounded p-4" style="border-color: var(--ds-border);">
                    <div class="flex items-center justify-between mb-3">
                      <h3 class="font-medium" style="color: var(--ds-text);">
                        {result.test_case_title}
                      </h3>
                      <span class="px-3 py-1 text-sm rounded-full" style={getStatusBadgeCSS(result.status)}>
                        {getStatusIcon(result.status)}
                        {getStatusLabel(result.status)}
                      </span>
                    </div>


                    {#if result.actual_result}
                      <div class="mb-3">
                        <h4 class="text-sm font-medium mb-1" style="color: var(--ds-text);">Actual Result</h4>
                        <p class="text-sm p-2 rounded" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                          {result.actual_result}
                        </p>
                      </div>
                    {/if}

                    {#if result.notes}
                      <div class="mb-3">
                        <h4 class="text-sm font-medium mb-1" style="color: var(--ds-text);">Notes</h4>
                        <p class="text-sm p-2 rounded" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                          {result.notes}
                        </p>
                      </div>
                    {/if}
                    
                    <!-- Step Results - only show if test case has steps and has step results -->
                    {#if result.test_steps && result.test_steps.length > 0}
                      {#if result.stepResults && Object.keys(result.stepResults).length > 0}
                        <div class="mt-4 pt-3 border-t" style="border-color: var(--ds-border);">
                          <h4 class="text-sm font-medium mb-2" style="color: var(--ds-text);">Step Results</h4>
                          <div class="space-y-3">
                            {#each result.test_steps as step, index}
                              {@const stepResult = result.stepResults[step.id]}
                              <div class="border rounded p-3" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
                                <div class="flex items-center gap-2 text-sm mb-2">
                                  <span class="w-2 h-2 rounded-full" style="background-color: {getStepStatusStyle(stepResult?.status || 'not_run')};"></span>
                                  <span class="font-medium" style="color: var(--ds-text);">Step {index + 1}: {getStatusLabel(stepResult?.status || 'not_run')}</span>
                                  {#if stepResult?.defect_id}
                                    <AlertTriangle class="w-3 h-3" style="color: var(--ds-status-warning-text);" />
                                  {/if}
                                </div>

                                <div class="text-xs mb-2" style="color: var(--ds-text-subtle);">
                                  <strong style="color: var(--ds-text);">Action:</strong> {step.action}
                                  {#if step.data}
                                    <br><strong style="color: var(--ds-text);">Data:</strong> {step.data}
                                  {/if}
                                  <br><strong style="color: var(--ds-text);">Expected:</strong> {step.expected}
                                </div>

                                {#if stepResult?.actual_result}
                                  <div class="mt-2">
                                    <div class="text-xs font-medium mb-1" style="color: var(--ds-text);">Actual Result:</div>
                                    <div class="text-xs p-2 rounded" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                                      {stepResult.actual_result}
                                    </div>
                                  </div>
                                {/if}

                                {#if stepResult?.notes}
                                  <div class="mt-2">
                                    <div class="text-xs font-medium mb-1" style="color: var(--ds-text);">Notes:</div>
                                    <div class="text-xs p-2 rounded" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                                      {stepResult.notes}
                                    </div>
                                  </div>
                                {/if}
                              </div>
                            {/each}
                          </div>
                        </div>
                      {:else}
                        <div class="mt-4 pt-3 border-t" style="border-color: var(--ds-border);">
                          <h4 class="text-sm font-medium mb-2" style="color: var(--ds-text);">Step Results</h4>
                          <div class="text-sm" style="color: var(--ds-text-subtle);">
                            {result.test_steps.length} steps - Not executed
                          </div>
                        </div>
                      {/if}
                    {:else}
                      <!-- Test case has no steps -->
                      <div class="mt-4 pt-3 border-t" style="border-color: var(--ds-border);">
                        <div class="text-sm italic" style="color: var(--ds-text-subtle);">
                          This test case has no defined steps
                        </div>
                      </div>
                    {/if}
                    
                    {#if result.executed_at}
                      <div class="text-xs mt-3 pt-2 border-t" style="border-color: var(--ds-border); color: var(--ds-text-subtle);">
                        Executed: {new Date(result.executed_at).toLocaleString()}
                      </div>
                    {/if}
                  </div>
                {/each}
              </div>
            {:else}
              <div class="text-center py-8">
                <div class="text-6xl mb-4">🧪</div>
                <div class="text-lg font-medium mb-2" style="color: var(--ds-text);">No results yet</div>
                <div class="text-sm" style="color: var(--ds-text-subtle);">
                  Execute this test run to see results
                </div>
                <Button
                  variant="primary"
                  onclick={() => navigate(testPath(`/runs/${runId}/execute`))}
                  icon={Play}
                  size="medium"
                  class="mt-4"
                >
                  Start Execution
                </Button>
              </div>
            {/if}
          </div>
        </div>

        <!-- Run Information -->
        <div class="space-y-6">
          <!-- Summary Stats -->
          {#if testResults.length > 0}
            <div>
              <h3 class="font-semibold mb-4" style="color: var(--ds-text);">Results Summary</h3>
              
              {#if testResults.length > 0}
                {@const summary = getResultsSummary(testResults)}
                <div class="space-y-3">
                <div class="flex justify-between">
                  <span class="text-sm" style="color: var(--ds-text-subtle);">Total</span>
                  <span class="text-sm font-medium" style="color: var(--ds-text);">{summary.total}</span>
                </div>
                <div class="flex justify-between">
                  <span class="text-sm" style="color: var(--ds-status-success-text);">Passed</span>
                  <span class="text-sm font-medium" style="color: var(--ds-status-success-text);">{summary.passed}</span>
                </div>
                <div class="flex justify-between">
                  <span class="text-sm" style="color: var(--ds-status-danger-text);">Failed</span>
                  <span class="text-sm font-medium" style="color: var(--ds-status-danger-text);">{summary.failed}</span>
                </div>
                <div class="flex justify-between">
                  <span class="text-sm" style="color: var(--ds-status-warning-text);">Blocked</span>
                  <span class="text-sm font-medium" style="color: var(--ds-status-warning-text);">{summary.blocked}</span>
                </div>
                <div class="flex justify-between">
                  <span class="text-sm" style="color: var(--ds-status-neutral-text);">Skipped</span>
                  <span class="text-sm font-medium" style="color: var(--ds-status-neutral-text);">{summary.skipped}</span>
                </div>
                <div class="flex justify-between">
                  <span class="text-sm" style="color: var(--ds-text-subtle);">Not Run</span>
                  <span class="text-sm font-medium" style="color: var(--ds-text-subtle);">{summary.not_run}</span>
                </div>
                <div class="pt-2 border-t" style="border-color: var(--ds-border);">
                  <div class="flex justify-between">
                    <span class="text-sm font-medium" style="color: var(--ds-text);">Success Rate</span>
                    <span class="text-sm font-medium" style="color: {summary.successRate >= 80 ? 'var(--ds-status-success-text)' : summary.successRate >= 60 ? 'var(--ds-status-warning-text)' : 'var(--ds-status-danger-text)'};">
                      {summary.successRate}%
                    </span>
                  </div>
                </div>
              </div>
              {/if}
            </div>
          {/if}

          <!-- Run Information -->
          <div>
            <h3 class="font-semibold mb-4" style="color: var(--ds-text);">Run Information</h3>
            
            <div class="space-y-3">
              <div>
                <div class="text-sm font-medium" style="color: var(--ds-text-subtle);">Status</div>
                <div class="mt-1">
                  <Lozenge color={testRun.ended_at ? 'green' : 'blue'} text={testRun.ended_at ? 'Completed' : 'In Progress'} />
                </div>
              </div>
              
              <div>
                <div class="text-sm font-medium" style="color: var(--ds-text-subtle);">Started</div>
                <div class="text-sm" style="color: var(--ds-text);">
                  {new Date(testRun.started_at).toLocaleString()}
                </div>
              </div>
              
              {#if testRun.ended_at}
                <div>
                  <div class="text-sm font-medium" style="color: var(--ds-text-subtle);">Ended</div>
                  <div class="text-sm" style="color: var(--ds-text);">
                    {new Date(testRun.ended_at).toLocaleString()}
                  </div>
                </div>
                
                <div>
                  <div class="text-sm font-medium" style="color: var(--ds-text-subtle);">Duration</div>
                  <div class="text-sm" style="color: var(--ds-text);">
                    {getDuration(testRun.started_at, testRun.ended_at)}
                  </div>
                </div>
              {/if}
            </div>
          </div>
        </div>
      </div>
    {:else}
      <div class="text-center py-12">
        <div style="color: var(--ds-text-subtle);">Test run not found</div>
        <Button
          onclick={goBack}
          variant="primary"
          size="medium"
          class="mt-4"
        >
          Back to Test Runs
        </Button>
      </div>
    {/if}
  </div>
</div>

<!-- Delete Confirmation Dialog -->
<ConfirmDialog
  bind:show={showDeleteConfirm}
  onconfirm={deleteRun}
  oncancel={() => {}}
  title="Delete Test Run"
  message="Are you sure you want to delete '{testRun?.name}'? This will permanently delete all test results and cannot be undone."
  confirmText="Delete"
  cancelText="Cancel"
  variant="danger"
/>
