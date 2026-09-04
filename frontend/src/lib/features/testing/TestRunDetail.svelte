<script>
  import { onMount } from 'svelte';
  import { currentRoute, navigate } from '../../router.js';
  import { api } from '../../api.js';
  import { IconArrowLeft, IconPlayerPlay, IconAlertTriangle, IconFileText, IconTrash } from '@tabler/icons-svelte-runes';
  import Button from '../../components/Button.svelte';
  import { confirm } from '../../composables/useConfirm.js';
  import Lozenge from '../../components/Lozenge.svelte';
  import Card from '../../components/Card.svelte';
  import Panel from '../../components/Panel.svelte';
  import Progress from '../../components/Progress.svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import StateDisplay from '../../components/StateDisplay.svelte';
  import PageHeader from '../../layout/PageHeader.svelte';
  import SectionHeader from '../../layout/SectionHeader.svelte';
  import { getStatusLabel } from '../../utils/statusColors.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast, infoToast } from '../../stores/toasts.svelte.js';
  import { loadTestRunDetail } from './testRunDetailData.js';
  import { formatAuthenticatedDateTime } from '../../utils/authenticatedDateFormatter.js';

  let testRun = $state(null);
  let testResults = $state([]);
  let loading = $state(true);

  let workspaceId = $derived($currentRoute.params.id);
  let runId = $derived($currentRoute.params.runId);
  let fromPage = $derived($currentRoute.query?.from);

  onMount(async () => {
    if (runId) {
      await loadTestRun(runId);
    }
  });

  async function loadTestRun(runId) {
    try {
      loading = true;
      const detail = await loadTestRunDetail(api, workspaceId, runId);
      testRun = detail.run;
      
      // Load test results if the run has been executed
      if (testRun.ended_at) {
        const testCases = detail.testCases;
        const stepResults = detail.stepResults;
        
        // Combine results with step results for display
        testResults = detail.results.map(result => {
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

  function exportResults() {
    if (!testRun || !testRun.ended_at) {
      infoToast(t('testing.noResultsForExport'));
      return;
    }
    
    window.open(testPath(`/runs/${runId}/print`), '_blank');
  }

  async function executeRun() {
    try {
      // Get existing runs for this test plan to generate sequential numbering
      const setRuns = await api.tests.testPlans.getRuns(workspaceId, testRun.plan_id);
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
        plan_id: testRun.plan_id,
        name: newRunName
      });
      
      // Navigate to execute the new run
      navigate(testPath(`/runs/${newRun.id}/execute`));
    } catch (error) {
      console.error('Failed to create execution run:', error);
    }
  }

  // Status colors now handled by imported utility (getStatusBadgeCSS, getStatusLabel)

  function getStatusColor(status) {
    return {
      passed: 'green',
      failed: 'red',
      blocked: 'amber',
      skipped: 'gray',
      not_run: 'gray'
    }[status] || 'gray';
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
    const diffMs = end.getTime() - start.getTime();

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

  async function confirmDelete() {
    const ok = await confirm({
      title: t('testing.deleteTestRun'),
      message: t('testing.deleteRunConfirm', { name: testRun?.name }),
      confirmText: t('common.delete'),
      variant: 'danger',
    });
    if (!ok) return;

    try {
      await api.tests.testRuns.delete(workspaceId, testRun.id);
      if (fromPage === 'reports') {
        navigate(testPath('/reports'));
      } else {
        navigate(testPath('/runs'));
      }
    } catch (error) {
      console.error('Failed to delete test run:', error);
      errorToast(t('testing.failedToDeleteRun') + ': ' + error.message);
    }
  }
</script>

<div class="min-h-screen flex flex-col p-6" style="background-color: var(--ds-surface);" data-testid="test-run-detail">
  <div class="flex-1">
    {#if loading}
      <StateDisplay type="loading" message={t('common.loading')} size="lg" />
    {:else if testRun}
      <!-- Header -->
      <Button
        onclick={goBack}
        dataTestid="test-run-detail-back"
        variant="ghost"
        size="small"
        icon={IconArrowLeft}
        class="mb-4"
      >
        {fromPage === 'reports' ? t('testing.testRunReport') : t('testing.testRuns')}
      </Button>
      <PageHeader
        title={testRun.name}
        subtitle={`${t('testing.started')}: ${formatAuthenticatedDateTime(testRun.started_at)}${testRun.ended_at ? ` • ${t('testing.ended')}: ${formatAuthenticatedDateTime(testRun.ended_at)}` : ''}`}
      >
        {#snippet actions()}
        <div class="flex flex-wrap items-center gap-3">
          {#if testRun.ended_at}
            <Button
              onclick={executeRun}
              variant="default"
              size="medium"
              icon={IconPlayerPlay}
              dataTestid="test-run-rerun"
            >
              {t('testing.startExecution')}
            </Button>
            <Button
              onclick={exportResults}
              variant="primary"
              size="medium"
              icon={IconFileText}
              dataTestid="test-run-export-results"
            >
              {t('testing.exportResults')}
            </Button>
          {:else}
            <Button
              variant="primary"
              onclick={() => navigate(testPath(`/runs/${runId}/execute`))}
              icon={IconPlayerPlay}
              size="medium"
            >
              {t('testing.continueExecution')}
            </Button>
          {/if}
          <Button
            onclick={confirmDelete}
            variant="danger"
            size="medium"
            icon={IconTrash}
            title={t('testing.deleteTestRun')}
          >
            {t('common.delete')}
          </Button>
        </div>
        {/snippet}
      </PageHeader>

      <!-- Test Run Details -->
      <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <!-- Status Overview -->
        <div class="lg:col-span-2">
          <div>
            <SectionHeader title={t('testing.testResults')} />
            
            {#if testResults.length > 0}
              <!-- Test Results Display -->
              <div class="space-y-4">
                {#each testResults as result}
                  <Card variant="outlined" padding="default" dataTestid={`test-run-result-${result.test_case_id}`}>
                    <div class="flex items-center justify-between mb-3">
                      <h3 class="font-medium" style="color: var(--ds-text);">
                        {result.test_case_title}
                      </h3>
                      <Lozenge color={getStatusColor(result.status)} text={getStatusLabel(result.status)} size="md" />
                    </div>


                    {#if result.actual_result}
                      <div class="mb-3">
                        <h4 class="text-sm font-medium mb-1" style="color: var(--ds-text);">{t('testing.actualResult')}</h4>
                        <Panel padding="compact" rounded="md" class="text-sm" style="color: var(--ds-text-subtle);">
                          {result.actual_result}
                        </Panel>
                      </div>
                    {/if}

                    {#if result.notes}
                      <div class="mb-3">
                        <h4 class="text-sm font-medium mb-1" style="color: var(--ds-text);">{t('common.notes')}</h4>
                        <Panel padding="compact" rounded="md" class="text-sm" style="color: var(--ds-text-subtle);">
                          {result.notes}
                        </Panel>
                      </div>
                    {/if}
                    
                    <!-- Step Results - only show if test case has steps and has step results -->
                    {#if result.test_steps && result.test_steps.length > 0}
                      {#if result.stepResults && Object.keys(result.stepResults).length > 0}
                        <div class="mt-4 pt-3 border-t" style="border-color: var(--ds-border);">
                          <h4 class="text-sm font-medium mb-2" style="color: var(--ds-text);">{t('testing.stepResults')}</h4>
                          <div class="space-y-3">
                            {#each result.test_steps as step, index}
                              {@const stepResult = result.stepResults[step.id]}
                              <Panel padding="compact" rounded="md" style="background-color: var(--ds-surface);">
                                <div data-testid={`test-run-step-result-${step.id}`}>
                                <div class="flex items-center gap-2 text-sm mb-2">
                                  <span class="w-2 h-2 rounded-full" style="background-color: {getStepStatusStyle(stepResult?.status || 'not_run')};"></span>
                                  <span class="font-medium" style="color: var(--ds-text);">{t('testing.stepNumber', { number: index + 1 })}: {getStatusLabel(stepResult?.status || 'not_run')}</span>
                                  {#if stepResult?.item_id}
                                    <a
                                      href={`/workspaces/${workspaceId}/items/${stepResult.item_id}`}
                                      data-testid={`test-run-step-item-${step.id}`}
                                      aria-label={t('testing.linked')}
                                      title={t('testing.linked')}
                                    >
                                      <IconAlertTriangle class="w-3 h-3" style="color: var(--ds-status-warning-text);" />
                                    </a>
                                  {/if}
                                </div>

                                <div class="text-xs mb-2" style="color: var(--ds-text-subtle);">
                                  <strong style="color: var(--ds-text);">{t('testing.action')}:</strong> {step.action}
                                  {#if step.data}
                                    <br><strong style="color: var(--ds-text);">{t('testing.data')}:</strong> {step.data}
                                  {/if}
                                  <br><strong style="color: var(--ds-text);">{t('testing.expected')}:</strong> {step.expected}
                                </div>

                                {#if stepResult?.actual_result}
                                  <div class="mt-2">
                                    <div class="text-xs font-medium mb-1" style="color: var(--ds-text);">{t('testing.actualResult')}:</div>
                                    <div data-testid={`test-run-step-actual-${step.id}`} class="text-xs p-2 rounded" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                                      {stepResult.actual_result}
                                    </div>
                                  </div>
                                {/if}

                                {#if stepResult?.notes}
                                  <div class="mt-2">
                                    <div class="text-xs font-medium mb-1" style="color: var(--ds-text);">{t('common.notes')}:</div>
                                    <div data-testid={`test-run-step-notes-${step.id}`} class="text-xs p-2 rounded" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                                      {stepResult.notes}
                                    </div>
                                  </div>
                                {/if}
                                </div>
                              </Panel>
                            {/each}
                          </div>
                        </div>
                      {:else}
                        <div class="mt-4 pt-3 border-t" style="border-color: var(--ds-border);">
                          <h4 class="text-sm font-medium mb-2" style="color: var(--ds-text);">{t('testing.stepResults')}</h4>
                          <div class="text-sm" style="color: var(--ds-text-subtle);">
                            {t('testing.stepsNotExecuted', { count: result.test_steps.length })}
                          </div>
                        </div>
                      {/if}
                    {:else}
                      <!-- Test case has no steps -->
                      <div class="mt-4 pt-3 border-t" style="border-color: var(--ds-border);">
                        <div class="text-sm italic" style="color: var(--ds-text-subtle);">
                          {t('testing.noDefinedSteps')}
                        </div>
                      </div>
                    {/if}

                    {#if result.executed_at}
                      <div class="text-xs mt-3 pt-2 border-t" style="border-color: var(--ds-border); color: var(--ds-text-subtle);">
                        {t('testing.executed')}: {formatAuthenticatedDateTime(result.executed_at)}
                      </div>
                    {/if}
                  </Card>
                {/each}
              </div>
            {:else}
              <EmptyState icon={IconPlayerPlay} title={t('testing.noResultsYet')} description={t('testing.executeToSeeResults')}>
                {#snippet action()}
                  <Button
                    variant="primary"
                    onclick={() => navigate(testPath(`/runs/${runId}/execute`))}
                    icon={IconPlayerPlay}
                    size="medium"
                  >
                    {t('testing.startExecution')}
                  </Button>
                {/snippet}
              </EmptyState>
            {/if}
          </div>
        </div>

        <!-- Run Information -->
        <div class="space-y-6">
          <!-- Summary Stats -->
          {#if testResults.length > 0}
            <Card variant="raised" padding="spacious" shadow>
              <SectionHeader title={t('testing.resultsSummary')} />

              {#if testResults.length > 0}
                {@const summary = getResultsSummary(testResults)}
                <div class="space-y-3">
                <div class="flex justify-between">
                  <span class="text-sm" style="color: var(--ds-text-subtle);">{t('common.total')}</span>
                  <span class="text-sm font-medium" style="color: var(--ds-text);">{summary.total}</span>
                </div>
                <div class="flex justify-between">
                  <span class="text-sm" style="color: var(--ds-status-success-text);">{t('testing.passed')}</span>
                  <span class="text-sm font-medium" style="color: var(--ds-status-success-text);">{summary.passed}</span>
                </div>
                <div class="flex justify-between">
                  <span class="text-sm" style="color: var(--ds-status-danger-text);">{t('testing.failed')}</span>
                  <span class="text-sm font-medium" style="color: var(--ds-status-danger-text);">{summary.failed}</span>
                </div>
                <div class="flex justify-between">
                  <span class="text-sm" style="color: var(--ds-status-warning-text);">{t('testing.blocked')}</span>
                  <span class="text-sm font-medium" style="color: var(--ds-status-warning-text);">{summary.blocked}</span>
                </div>
                <div class="flex justify-between">
                  <span class="text-sm" style="color: var(--ds-status-neutral-text);">{t('testing.skipped')}</span>
                  <span class="text-sm font-medium" style="color: var(--ds-status-neutral-text);">{summary.skipped}</span>
                </div>
                <div class="flex justify-between">
                  <span class="text-sm" style="color: var(--ds-text-subtle);">{t('testing.notRun')}</span>
                  <span class="text-sm font-medium" style="color: var(--ds-text-subtle);">{summary.not_run}</span>
                </div>
                <div class="pt-2 border-t" style="border-color: var(--ds-border);">
                  <div class="flex justify-between">
                    <span class="text-sm font-medium" style="color: var(--ds-text);">{t('testing.successRate')}</span>
                    <span class="text-sm font-medium" style="color: {summary.successRate >= 80 ? 'var(--ds-status-success-text)' : summary.successRate >= 60 ? 'var(--ds-status-warning-text)' : 'var(--ds-status-danger-text)'};">
                      {summary.successRate}%
                    </span>
                  </div>
                </div>
                <Progress
                  value={summary.successRate}
                  color={summary.successRate >= 80 ? 'success' : summary.successRate >= 60 ? 'warning' : 'danger'}
                  class="pt-1"
                />
              </div>
              {/if}
            </Card>
          {/if}

          <!-- Run Information -->
          <Card variant="raised" padding="spacious" shadow>
            <SectionHeader title={t('testing.runInformation')} />

            <div class="space-y-3">
              <div>
                <div class="text-sm font-medium" style="color: var(--ds-text-subtle);">{t('common.status')}</div>
                <div class="mt-1">
                  <Lozenge color={testRun.ended_at ? 'green' : 'blue'} text={testRun.ended_at ? t('testing.completed') : t('testing.inProgress')} />
                </div>
              </div>

              <div>
                <div class="text-sm font-medium" style="color: var(--ds-text-subtle);">{t('testing.started')}</div>
                <div class="text-sm" style="color: var(--ds-text);">
                  {formatAuthenticatedDateTime(testRun.started_at)}
                </div>
              </div>
              
              {#if testRun.ended_at}
                <div>
                  <div class="text-sm font-medium" style="color: var(--ds-text-subtle);">{t('testing.ended')}</div>
                  <div class="text-sm" style="color: var(--ds-text);">
                    {formatAuthenticatedDateTime(testRun.ended_at)}
                  </div>
                </div>

                <div>
                  <div class="text-sm font-medium" style="color: var(--ds-text-subtle);">{t('testing.duration')}</div>
                  <div class="text-sm" style="color: var(--ds-text);">
                    {getDuration(testRun.started_at, testRun.ended_at)}
                  </div>
                </div>
              {/if}
            </div>
          </Card>
        </div>
      </div>
    {:else}
      <StateDisplay type="empty" icon={IconFileText} title={t('testing.testRunNotFound')}>
        {#snippet action()}
          <Button onclick={goBack} variant="primary" size="medium">
            {t('testing.backToTestRuns')}
          </Button>
        {/snippet}
      </StateDisplay>
    {/if}
  </div>
</div>
