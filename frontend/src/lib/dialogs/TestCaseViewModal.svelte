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
    History,
    Copy,
    FileText
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
  import BDDScenarioView from '../features/testing/BDDScenarioView.svelte';
  import { parseScenarioSpec } from '../features/testing/bddSpec.js';
  import { formatAuthenticatedDateTime as formatDateTimeLocale } from '../utils/authenticatedDateFormatter.js';
  import { t } from '../stores/i18n.svelte.js';
  import { buildRequirementKeyByPageId, pageHref } from '../features/requirements/requirementKeyMap.js';

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
  let pageLinks = $state([]);
  let requirementKeyByPageId = $state(new Map());
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
    pageLinks = [];
    requirementKeyByPageId = new Map();

    try {
      const caseData = await api.tests.testCases.get(workspaceId, numericId);
      const stepsData = caseData?.format === 'bdd'
        ? []
        : await api.tests.testCases.steps.getAll(workspaceId, numericId);

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

      if (workspaceId) {
        const [keys, linksResp] = await Promise.all([
          buildRequirementKeyByPageId(workspaceId),
          api.links.getForItem('test-cases', numericId).catch((linkErr) => {
            console.warn('Failed to load test case page links:', linkErr);
            return null;
          }),
        ]);
        requirementKeyByPageId = keys;
        pageLinks = linksResp ? pageLinksForTestCase(linksResp, numericId) : [];
      }
    } catch (err) {
      console.error('Failed to load test case detail:', err);
      error = err?.message || 'Failed to load test case';
    } finally {
      loading = false;
    }
  }

  function pageLinksForTestCase(linksResp, testCaseId) {
    const outgoing = Array.isArray(linksResp?.outgoing) ? linksResp.outgoing : [];
    const incoming = Array.isArray(linksResp?.incoming) ? linksResp.incoming : [];
    const seen = new Set();
    const merged = [];
    for (const link of [...incoming, ...outgoing]) {
      if (!link || link.id == null || seen.has(link.id)) continue;
      const otherType = link.source_type === 'test_case' && link.source_id === testCaseId
        ? link.target_type
        : link.target_type === 'test_case' && link.target_id === testCaseId
          ? link.source_type
          : null;
      if (otherType !== 'page') continue;
      seen.add(link.id);
      merged.push(link);
    }
    return merged;
  }

  function linkedPageFromTestLink(link) {
    const testIsSource = link.source_type === 'test_case' && link.source_id === testCase?.id;
    const testIsTarget = link.target_type === 'test_case' && link.target_id === testCase?.id;
    if (testIsSource) {
      return {
        id: link.target_id,
        title: link.target_title,
        workspaceId: link.target_workspace_id,
      };
    }
    if (testIsTarget) {
      return {
        id: link.source_id,
        title: link.source_title,
        workspaceId: link.source_workspace_id,
      };
    }
    return null;
  }

  let isBdd = $derived(testCase?.format === 'bdd');
  let bddSpec = $derived(isBdd ? parseScenarioSpec(testCase?.bdd?.spec) : null);
  let copyState = $state('');

  // Export the authored feature file and copy it. The backend serializes the
  // stored source; no client-side reconstruction needed.
  async function copyFeatureFile() {
    if (!workspaceId || !testCase?.id) return;
    try {
      const feature = await api.tests.testCases.feature(workspaceId, testCase.id);
      await navigator.clipboard.writeText(feature);
      copyState = 'copied';
    } catch (err) {
      console.error('Failed to copy feature file:', err);
      copyState = 'failed';
    } finally {
      setTimeout(() => (copyState = ''), 2000);
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
          {#if !isBdd}
            <Button
              variant="primary"
              icon={Edit}
              size="medium"
              href={`${workspaceTestsBasePath}/cases/${testCase.id}/steps`}
              onclick={closeOnPlainClick}
            >
              {t('testCase.editTestSteps')}
            </Button>
          {/if}
          {#if isBdd}
            <Button
              variant="primary"
              icon={Copy}
              size="medium"
              onclick={copyFeatureFile}
              dataTestid="test-case-copy-feature"
            >
              {copyState === 'copied' ? t('testing.featureCopied') : copyState === 'failed' ? t('testing.featureCopyFailed') : t('testing.copyFeatureFile')}
            </Button>
          {/if}
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

        <!-- BDD Scenario Section -->
        {#if isBdd}
          <Card variant="raised" padding="none" rounded="xl" shadow class="overflow-hidden">
            {#snippet header()}
              <h2 class="text-lg font-semibold flex items-center gap-2" style="color: var(--ds-text);">
                <ListOrdered class="w-5 h-5" style="color: var(--ds-interactive);" />
                {t('testing.scenario')}
              </h2>
            {/snippet}
            <div class="p-6">
              {#if bddSpec}
                <BDDScenarioView spec={bddSpec} raw={testCase.bdd?.gherkin || ''} dataTestid="test-case-bdd-view" />
              {:else}
                <EmptyState icon={ClipboardList} title={t('testing.bddSpecUnavailable')}>
                  {#snippet action()}
                    <Button
                      variant="default"
                      size="medium"
                      onclick={copyFeatureFile}
                      dataTestid="test-case-copy-feature-fallback"
                    >
                      {t('testing.copyFeatureFile')}
                    </Button>
                  {/snippet}
                </EmptyState>
              {/if}
            </div>
          </Card>
        {:else}
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
        {/if}

        <!-- Linked requirements / pages -->
        <Card variant="raised" padding="none" rounded="xl" shadow class="overflow-hidden">
          {#snippet header()}
            <h2 class="text-lg font-semibold flex items-center gap-2" style="color: var(--ds-text);">
              <FileText class="w-5 h-5" style="color: var(--ds-interactive);" />
              {t('requirements.traceability.linkedPages')} ({pageLinks.length})
            </h2>
          {/snippet}
          <div class="p-6">
            {#if pageLinks.length === 0}
              <EmptyState icon={FileText} title={t('requirements.traceability.noPages')} />
            {:else}
              <div class="space-y-2">
                {#each pageLinks as link (link.id)}
                  {@const linkedPage = linkedPageFromTestLink(link)}
                  {#if linkedPage}
                    {@const reqMeta = requirementKeyByPageId.get(linkedPage.id)}
                    {@const href = pageHref(linkedPage.workspaceId || workspaceId, linkedPage.id, reqMeta)}
                    <Panel
                      href={href}
                      style="border-color: var(--ds-border); background-color: var(--ds-surface);"
                      padding="default"
                      rounded="lg"
                      interactive
                      onclick={closeOnPlainClick}
                    >
                      <div class="flex items-center gap-2 min-w-0">
                        {#if reqMeta?.key}
                          <Lozenge color="blue">{reqMeta.key}</Lozenge>
                        {/if}
                        <span class="text-sm font-medium truncate" style="color: var(--ds-text);">
                          {linkedPage.title}
                        </span>
                      </div>
                    </Panel>
                  {/if}
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
