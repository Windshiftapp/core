<script>
  import {
    ArrowLeft,
    Edit,
    Play,
    CheckCircle,
    XCircle,
    AlertCircle,
    Clock,
    ListOrdered,
    ClipboardList,
    History
  } from '@lucide/svelte';
  import Modal from './Modal.svelte';
  import Button from '../components/Button.svelte';
  import Card from '../components/Card.svelte';
  import Panel from '../components/Panel.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import StateDisplay from '../components/StateDisplay.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import { api } from '../api.js';
  import { formatAuthenticatedDateTime as formatDateTimeLocale } from '../utils/authenticatedDateFormatter.js';
  import { t } from '../stores/i18n.svelte.js';

  let {
    isOpen = $bindable(false),
    testCaseId = null,
    workspaceId = null,
    embedded = false,
    backLabel = null,
    onclose = null
  } = $props();

  let loading = $state(false);
  let error = $state(null);
  let testCase = $state(null);
  let testSteps = $state([]);
  let executions = $state([]);
  let lastLoadedId = $state(null);
  const workspaceTestsBasePath = $derived.by(() => workspaceId ? `/workspaces/${workspaceId}/tests` : '/workspaces');

  // Close the modal on a plain click; let cmd/ctrl/middle clicks open in a new
  // tab with the modal left open behind them.
  function closeOnPlainClick(e) {
    if (e && (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey || e.button !== 0)) return;
    isOpen = false;
  }

  $effect(() => {
    if (isOpen && testCaseId && testCaseId !== lastLoadedId) {
      loadTestCaseData(testCaseId);
    }
  });

  async function loadTestCaseData(id) {
    if (!id) return;

    const numericId = Number(id);
    if (!Number.isFinite(numericId)) {
      console.warn('Invalid test case ID provided to TestCaseViewModal:', id);
      error = 'Unable to load linked test case.';
      return;
    }

    loading = true;
    error = null;

    try {
      const [caseData, stepsData] = await Promise.all([
        api.tests.testCases.get(workspaceId, numericId),
        api.tests.testCases.steps.getAll(workspaceId, numericId)
      ]);

      let connections = null;
      try {
        connections = await api.tests.testCases.connections(workspaceId, numericId);
      } catch (connErr) {
        if (!(connErr?.status === 404)) {
          throw connErr;
        }
        console.warn('Test case connections unavailable:', connErr);
      }

      testCase = caseData;
      testSteps = Array.isArray(stepsData) ? stepsData : [];
      executions = connections?.executions || [];
      lastLoadedId = numericId;
    } catch (err) {
      console.error('Failed to load test case detail:', err);
      error = err?.message || 'Failed to load test case';
    } finally {
      loading = false;
    }
  }

  function handleClose() {
    isOpen = false;
    onclose?.();
  }

  function getStatusColor(status) {
    if (!status) return 'gray';
    const normalized = status.toLowerCase();
    if (normalized === 'passed') return 'green';
    if (normalized === 'failed') return 'red';
    if (normalized === 'blocked') return 'amber';
    if (normalized === 'in_progress') return 'blue';
    return 'gray';
  }

  function getStatusIcon(status) {
    if (!status) return Clock;
    const normalized = status.toLowerCase();
    if (normalized === 'passed') return CheckCircle;
    if (normalized === 'failed') return XCircle;
    if (normalized === 'blocked') return AlertCircle;
    if (normalized === 'in_progress') return Clock;
    return Clock;
  }

  function getStatusIconColor(status) {
    if (!status) return 'var(--ds-icon)';
    const normalized = status.toLowerCase();
    if (normalized === 'passed') return 'var(--ds-text-success)';
    if (normalized === 'failed') return 'var(--ds-text-danger)';
    if (normalized === 'blocked') return 'var(--ds-text-warning)';
    if (normalized === 'in_progress') return 'var(--ds-text-information)';
    return 'var(--ds-icon)';
  }

  // Priority color helper
  function getPriorityColor(priority) {
    const colors = {
      low: '#6B7280',
      medium: '#3B82F6',
      high: '#F59E0B',
      critical: '#EF4444'
    };
    return colors[priority] || colors.medium;
  }

  // Format duration helper
  function formatDuration(seconds) {
    if (!seconds || seconds === 0) return null;
    const hours = Math.floor(seconds / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    if (hours > 0 && minutes > 0) return `${hours}h ${minutes}m`;
    if (hours > 0) return `${hours}h`;
    return `${minutes}m`;
  }

  function getTestCaseStatusColor(status) {
    if (status === 'inactive') return 'gray';
    if (status === 'draft') return 'amber';
    return 'green';
  }
</script>

{#if embedded}
  <Card variant="raised" padding="none" rounded="xl" shadow class="w-full max-h-full flex flex-col overflow-hidden">
    <div class="flex-1 overflow-y-auto">
      {@render previewContent()}
    </div>
  </Card>
{:else}
  <Modal
    bind:isOpen
    maxWidth="max-w-4xl"
    zIndexClass="z-60"
    onclose={handleClose}
  >
    {@render previewContent()}
  </Modal>
{/if}

{#snippet previewContent()}
  <div class="p-6 space-y-6">
    <PageHeader
      title={testCase ? testCase.title : t('common.loading')}
      subtitle={testCase?.folder_name ? `${t('testCase.folder')}: ${testCase.folder_name}` : t('testCase.preview')}
      marginClass="mb-0"
    >
      {#snippet actions()}
        {#if embedded}
          <Button variant="ghost" icon={ArrowLeft} onclick={handleClose} dataTestid="test-case-detail-back">
            {backLabel || t('testCase.backToItem')}
          </Button>
        {/if}
      {/snippet}
    </PageHeader>
    {#if testCase}
      <div class="flex flex-wrap items-center gap-2">
        <Lozenge customBg={getPriorityColor(testCase.priority || 'medium')} text={`${testCase.priority || 'medium'} ${t('testCase.priority')}`} />
        <Lozenge color={getTestCaseStatusColor(testCase.status || 'active')} text={testCase.status || 'active'} />
        {#if formatDuration(testCase.estimated_duration)}
          <Lozenge color="gray" icon={Clock} text={formatDuration(testCase.estimated_duration)} />
        {/if}
      </div>
    {/if}

    {#if loading}
      <StateDisplay type="loading" message={t('common.loading')} />
    {:else if error}
      <StateDisplay type="error" title={t('common.error')} message={error} />
    {:else if testCase}
      <div class="space-y-6">
        <!-- Action Buttons -->
        <div class="flex flex-wrap gap-3">
          <Button
            variant="primary"
            icon={Edit}
            size="medium"
            href={`${workspaceTestsBasePath}/cases/${testCase.id}/steps`}
            onclick={closeOnPlainClick}
          >
            {t('testCase.editTestSteps')}
          </Button>
          <Button
            variant="default"
            icon={Play}
            size="medium"
            href={`${workspaceTestsBasePath}/runs`}
            onclick={closeOnPlainClick}
          >
            {t('testCase.viewTestRuns')}
          </Button>
        </div>

        {#if testCase.preconditions}
          <AlertBox variant="info">
            <strong>{t('testCase.preconditions')}:</strong> {testCase.preconditions}
          </AlertBox>
        {/if}

        <!-- Test Steps Section -->
        <Card variant="raised" padding="none" rounded="xl" shadow class="overflow-hidden">
          {#snippet header()}
            <h2 class="text-lg font-semibold flex items-center gap-2" style="color: var(--ds-text);">
              <ListOrdered class="w-5 h-5" style="color: var(--ds-interactive);" />
              {t('testCase.testSteps')} ({testSteps.length})
            </h2>
          {/snippet}
          <div class="p-6">
            {#if testSteps.length === 0}
              <EmptyState icon={ClipboardList} title={t('testCase.noStepsDefined')} description={t('testCase.noStepsHelp')}>
                {#snippet action()}
                  <Button
                    variant="primary"
                    icon={Edit}
                    size="medium"
                    href={`${workspaceTestsBasePath}/cases/${testCase.id}/steps`}
                    onclick={closeOnPlainClick}
                  >
                    {t('testCase.addSteps')}
                  </Button>
                {/snippet}
              </EmptyState>
            {:else}
              <div class="space-y-4">
                {#each testSteps as step}
                  <Panel padding="default" rounded="lg" style="background-color: var(--ds-surface);">
                    <div class="flex items-start gap-4">
                      <div
                        class="flex-shrink-0 w-10 h-10 rounded-full text-white font-semibold text-base flex items-center justify-center"
                        style="background-color: var(--ds-interactive);"
                      >
                        {step.step_number}
                      </div>
                      <div class="flex-1 space-y-4">
                        <div>
                          <p class="text-xs font-semibold uppercase tracking-wider mb-1" style="color: var(--ds-interactive);">
                            {t('testCase.action')}
                          </p>
                          <p class="text-sm" style="color: var(--ds-text);">
                            {step.action || '—'}
                          </p>
                        </div>

                        {#if step.data}
                          <div>
                            <p
                              class="text-xs font-semibold uppercase tracking-wider mb-1"
                              style="color: var(--ds-icon-accent-purple);"
                            >
                              {t('testCase.data')}
                            </p>
                            <p class="text-sm" style="color: var(--ds-text);">
                              {step.data}
                            </p>
                          </div>
                        {/if}

                        <div>
                          <p
                            class="text-xs font-semibold uppercase tracking-wider mb-1"
                            style="color: var(--ds-icon-accent-green);"
                          >
                            {t('testCase.expectedResult')}
                          </p>
                          <p class="text-sm" style="color: var(--ds-text);">
                            {step.expected || '—'}
                          </p>
                        </div>
                      </div>
                    </div>
                  </Panel>
                {/each}
              </div>
            {/if}
          </div>
        </Card>

        <!-- Recent Executions Section -->
        <Card variant="raised" padding="none" rounded="xl" shadow class="overflow-hidden">
          {#snippet header()}
            <h2 class="text-lg font-semibold flex items-center gap-2" style="color: var(--ds-text);">
              <Play class="w-5 h-5" style="color: var(--ds-icon-accent-green);" />
              {t('testCase.recentExecutions')} ({executions.length})
            </h2>
          {/snippet}
          <div class="p-6">
            {#if executions.length === 0}
              <EmptyState icon={History} title={t('testCase.noExecutions')} />
            {:else}
              <div class="space-y-3">
                {#each executions as execution}
                  {@const StatusIcon = getStatusIcon(execution.status)}
                  <Panel
                    href={`${workspaceTestsBasePath}/runs/${execution.run_id}`}
                    style="border-color: var(--ds-border); background-color: var(--ds-surface);"
                    padding="default"
                    rounded="lg"
                    interactive
                  >
                    <div class="flex items-start gap-3">
                      <div class="flex-shrink-0 mt-0.5">
                      <StatusIcon
                        class="w-5 h-5"
                        style={`color: ${getStatusIconColor(execution.status)};`}
                      />
                      </div>
                      <div class="flex-1 min-w-0">
                      <div class="flex items-center justify-between gap-3 mb-1">
                        <div class="font-semibold text-sm truncate" style="color: var(--ds-text);">
                          {execution.run_name}
                        </div>
                        <Lozenge color={getStatusColor(execution.status)} text={execution.status || 'not_run'} />
                      </div>
                      <div class="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs" style="color: var(--ds-text-subtle);">
                        <span class="flex items-center gap-1">
                          <Clock class="w-3.5 h-3.5" />
                          {formatDateTimeLocale(execution.started_at) || '—'}
                        </span>
                        {#if execution.set_name}
                          <span>• {t('testCase.set')}: {execution.set_name}</span>
                        {/if}
                        {#if execution.template_name}
                          <span>• {t('testCase.template')}: {execution.template_name}</span>
                        {/if}
                      </div>
                      </div>
                    </div>
                  </Panel>
                {/each}
              </div>
            {/if}
          </div>
        </Card>
      </div>
    {/if}
  </div>
{/snippet}
