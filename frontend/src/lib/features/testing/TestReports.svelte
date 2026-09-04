<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { currentRoute } from '../../router.js';
  import Button from '../../components/Button.svelte';
  import Select from '../../components/Select.svelte';
  import PageHeader from '../../layout/PageHeader.svelte';
  import TabNav from '../../components/TabNav.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import Card from '../../components/Card.svelte';
  import StatCard from '../../components/StatCard.svelte';
  import StateDisplay from '../../components/StateDisplay.svelte';
  import FormField from '../../components/FormField.svelte';
  import MilestoneCombobox from '../../pickers/MilestoneCombobox.svelte';
  import Chart from '../../widgets/Chart.svelte';
  import { IconChartBar, IconRefresh, IconCircleCheck, IconCircleX, IconAlertTriangle, IconPlayerSkipForward, IconClock, IconTrendingUp, IconSettings } from '@tabler/icons-svelte-runes';
  import TestCoverageReport from './TestCoverageReport.svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { formatAuthenticatedDateTime } from '../../utils/authenticatedDateFormatter.js';

  let { workspaceId = null } = $props();

  let loading = $state(true);
  let reportData = $state(null);
  let selectedMilestoneId = $state(null);
  let days = $state(30);

  // Coverage report reference for accessing its state/methods
  let coverageReportRef = $state(null);
  let coverageCollections = $state([]);
  let coverageSelectedCollectionId = $state(null);
  let coverageFilterCovered = $state('all');

  // Get subtab from URL query params, default to 'test-runs'
  const subtab = $derived($currentRoute.query?.subtab || 'test-runs');

  const tabs = $derived([
    { id: 'test-runs', label: t('testing.testRunReport') },
    { id: 'coverage', label: t('testing.requirementsCoverage') }
  ]);

  // Compute basePath for TabNav
  const basePath = $derived(workspaceId ? `/workspaces/${workspaceId}/tests/reports` : '/tests/reports');

  const workspaceTestBase = $derived.by(() => workspaceId ? `/workspaces/${workspaceId}/tests` : '/workspaces');

  // Update coverage state from the component ref
  function updateCoverageState() {
    if (coverageReportRef) {
      coverageCollections = coverageReportRef.getCollections();
      coverageSelectedCollectionId = coverageReportRef.getSelectedCollectionId();
      coverageFilterCovered = coverageReportRef.getFilterCovered();
    }
  }

  function handleCoverageCollectionChange(event) {
    const value = event.target.value;
    const id = value === '' ? null : parseInt(value, 10);
    coverageReportRef?.setSelectedCollectionId(id);
    coverageSelectedCollectionId = id;
  }

  function handleCoverageFilterChange(event) {
    const value = event.target.value;
    coverageReportRef?.setFilterCovered(value);
    coverageFilterCovered = value;
  }

  function handleOpenCoverageConfig() {
    coverageReportRef?.triggerOpenConfigModal();
  }

  // Update coverage state when the ref becomes available
  $effect(() => {
    if (coverageReportRef && subtab === 'coverage') {
      // Small delay to ensure the child component has initialized
      setTimeout(updateCoverageState, 100);
    }
  });

  // Transform trend data for the chart
  const chartData = $derived.by(() => {
    if (!reportData?.trend || reportData.trend.length === 0) return [];
    return reportData.trend.map(point => ({
      date: point.date,
      count: point.pass_rate,
      label: `${point.date} - ${point.pass_rate.toFixed(1)}%`
    }));
  });

  // Failure/blocked rows can repeat the same test_case_id (a flaky test that
  // failed across several runs), so test_case_id is NOT a unique row key. Derive
  // a composite key from (run_id, test_case_id) — unique per test result — so the
  // DataTable's keyed {#each} doesn't crash on duplicate keys.
  const failureRows = $derived.by(() =>
    (reportData?.recent_failures ?? []).map((f) => ({ ...f, row_key: `${f.run_id}:${f.test_case_id}` }))
  );
  const blockedRows = $derived.by(() =>
    (reportData?.recent_blocked ?? []).map((b) => ({ ...b, row_key: `${b.run_id}:${b.test_case_id}` }))
  );

  // Columns for the failures table
  const failuresColumns = $derived.by(() => [
    {
      key: 'test_case_title',
      label: t('testing.testCase'),
      slot: 'test_case_link'
    },
    {
      key: 'run_name',
      label: t('testing.run'),
      slot: 'run_link'
    },
    {
      key: 'failed_at',
      label: t('testing.failedAt'),
      render: (failure) => failure.failed_at ? formatAuthenticatedDateTime(failure.failed_at) : '-'
    }
  ]);

  // Columns for the blocked table
  const blockedColumns = $derived.by(() => [
    {
      key: 'test_case_title',
      label: t('testing.testCase'),
      slot: 'test_case_link'
    },
    {
      key: 'run_name',
      label: t('testing.run'),
      slot: 'run_link'
    },
    {
      key: 'reason',
      label: t('testing.reason'),
      render: (blocked) => blocked.reason || t('testing.noReasonProvided'),
      textColor: (blocked) => blocked.reason ? 'var(--ds-text)' : 'var(--ds-text-subtle)'
    },
    {
      key: 'blocked_at',
      label: t('testing.blockedAt'),
      render: (blocked) => blocked.blocked_at ? formatAuthenticatedDateTime(blocked.blocked_at) : '-'
    }
  ]);

  onMount(() => {
    loadReportData();
  });

  async function loadReportData() {
    try {
      loading = true;
      const options = { days };
      if (selectedMilestoneId) {
        options.milestoneId = selectedMilestoneId;
      }
      reportData = await api.tests.reports.getSummary(workspaceId, options);
    } catch (error) {
      console.error('Failed to load report data:', error);
      reportData = null;
    } finally {
      loading = false;
    }
  }

  function handleMilestoneSelect(result) {
    selectedMilestoneId = result.value;
    loadReportData();
  }
</script>

<div class="min-h-screen flex flex-col" style="background-color: var(--ds-surface);" data-testid="test-reports">
  <!-- Tab Navigation - Always at top -->
  <div class="px-6">
    <TabNav {tabs} {basePath} defaultTab="test-runs" />
  </div>

  <!-- Tab Content -->
  {#if subtab === 'test-runs'}
    <div class="flex flex-col flex-1 p-6">
      <PageHeader
        title={t('testing.testRunReport')}
        subtitle={t('testing.testRunReportSubtitle')}
      >
        {#snippet actions()}
          <div class="flex items-center gap-3">
            <div class="w-48">
              <MilestoneCombobox
                bind:value={selectedMilestoneId}
                placeholder={t('milestones.allMilestones')}
                onSelect={handleMilestoneSelect}
              />
            </div>
            <Button
              onclick={loadReportData}
              variant="primary"
              size="medium"
              disabled={loading}
            >
              <IconRefresh class="w-4 h-4 {loading ? 'animate-spin' : ''}" />
              {loading ? t('common.loading') : t('common.refresh')}
            </Button>
          </div>
        {/snippet}
      </PageHeader>

      <!-- Content wrapper -->
      <div class="flex-1 space-y-6">
        {#if loading}
          <StateDisplay type="loading" message={t('testing.loadingReportData')} size="lg" />
        {:else if !reportData || reportData.overall?.total_runs === 0}
          <EmptyState
            icon={IconChartBar}
            title={t('testing.noTestDataFound')}
            description={t('testing.completeTestRunsToSeeReports')}
          />
        {:else}
    <div class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-4">
      <StatCard icon={IconClock} label={t('testing.totalTests')} value={reportData.overall.total_tests} color="blue" dataTestid="test-report-total" />
      <StatCard icon={IconCircleCheck} label={t('testing.passed')} value={reportData.overall.passed} color="green" dataTestid="test-report-passed" />
      <StatCard icon={IconCircleX} label={t('testing.failed')} value={reportData.overall.failed} color="orange" dataTestid="test-report-failed" />
      <StatCard icon={IconAlertTriangle} label={t('testing.blocked')} value={reportData.overall.blocked} color="orange" dataTestid="test-report-blocked" />
      <StatCard icon={IconPlayerSkipForward} label={t('testing.skipped')} value={reportData.overall.skipped} color="purple" dataTestid="test-report-skipped" />
      <StatCard icon={IconTrendingUp} label={t('testing.passRate')} value={`${reportData.overall.pass_rate.toFixed(1)}%`} color="green" dataTestid="test-report-pass-rate" />
    </div>

    <!-- Pass Rate Trend Chart -->
    <Card padding="none" shadow>
      {#snippet header()}
      <div class="flex items-center gap-2">
        <IconTrendingUp class="w-5 h-5" style="color: var(--ds-text-subtle);" />
        <h3 class="font-semibold" style="color: var(--ds-text);">
          {t('testing.passRateTrend', { days })}
        </h3>
      </div>
      {/snippet}
      <div class="p-5">
        {#if chartData.length > 0}
          <Chart
            type="line"
            series={[{ key: 'rate', label: t('testing.passRate'), color: 'var(--ds-status-success-solid)', values: chartData.map(d => d.count ?? 0) }]}
            categories={chartData.map(d => d.label || d.date)}
            emptyMessage={t('testing.noTrendData')}
            minHeight={150}
            maxHeight={250}
            showYAxis={true}
            minValue={0}
            maxValue={100}
            valueFormat={(v) => `${v.toFixed(1)}%`}
            yAxisFormat={(v) => `${Math.round(v)}%`}
          />
        {:else}
          <div class="flex items-center justify-center h-40 text-sm" style="color: var(--ds-text-subtle);">
            {t('testing.noTrendDataForPeriod')}
          </div>
        {/if}
      </div>
    </Card>

    <!-- Recent Failures and Blocked Tables - Side by Side -->
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
      <!-- Recent Failures Table -->
      <Card padding="none" shadow>
        {#snippet header()}
        <div class="flex items-center gap-2">
          <IconCircleX class="w-5 h-5" style="color: var(--ds-text-subtle);" />
          <div>
            <h3 class="text-lg font-semibold" style="color: var(--ds-text);">{t('testing.recentFailures')}</h3>
            <p class="text-sm" style="color: var(--ds-text-subtle);">
              {t('testing.recentFailuresSubtitle', { days })}
            </p>
          </div>
        </div>
        {/snippet}
        {#if failureRows.length > 0}
          <DataTable
            columns={failuresColumns}
            data={failureRows}
            keyField="row_key"
            class="border-0 rounded-none"
            emptyMessage={t('testing.noFailuresToShow')}
          >
            {#snippet test_case_link(item)}
              <a href="{workspaceTestBase}/cases/{item.test_case_id}?from=reports" data-testid={`test-report-failure-case-${item.test_case_id}`} style="color: var(--ds-text-link);" class="hover:underline">{item.test_case_title}</a>
            {/snippet}
            {#snippet run_link(item)}
              <a href="{workspaceTestBase}/runs/{item.run_id}?from=reports" data-testid={`test-report-failure-run-${item.run_id}`} style="color: var(--ds-text-link);" class="hover:underline">{item.run_name}</a>
            {/snippet}
          </DataTable>
        {:else}
          <EmptyState
            icon={IconCircleCheck}
            title={t('testing.noFailures')}
            description={t('testing.allTestsPassing')}
          />
        {/if}
      </Card>

      <!-- Blocked Tests Table -->
      <Card padding="none" shadow>
        {#snippet header()}
        <div class="flex items-center gap-2">
          <IconAlertTriangle class="w-5 h-5" style="color: var(--ds-text-subtle);" />
          <div>
            <h3 class="text-lg font-semibold" style="color: var(--ds-text);">{t('testing.blockedTests')}</h3>
            <p class="text-sm" style="color: var(--ds-text-subtle);">
              {t('testing.blockedTestsSubtitle', { days })}
            </p>
          </div>
        </div>
        {/snippet}
        {#if blockedRows.length > 0}
          <DataTable
            columns={blockedColumns}
            data={blockedRows}
            keyField="row_key"
            class="border-0 rounded-none"
            emptyMessage={t('testing.noBlockedTests')}
          >
            {#snippet test_case_link(item)}
              <a href="{workspaceTestBase}/cases/{item.test_case_id}?from=reports" style="color: var(--ds-text-link);" class="hover:underline">{item.test_case_title}</a>
            {/snippet}
            {#snippet run_link(item)}
              <a href="{workspaceTestBase}/runs/{item.run_id}?from=reports" style="color: var(--ds-text-link);" class="hover:underline">{item.run_name}</a>
            {/snippet}
          </DataTable>
        {:else}
          <EmptyState
            icon={IconCircleCheck}
            title={t('testing.noBlockedTestsTitle')}
            description={t('testing.allTestsUnblocked')}
          />
        {/if}
      </Card>
    </div>
        {/if}
      </div>
    </div>
  {/if}

  {#if subtab === 'coverage'}
    <div class="flex flex-col flex-1 p-6">
      <PageHeader
        title={t('testing.requirementsCoverage')}
        subtitle={t('testing.requirementsCoverageSubtitle')}
      >
        {#snippet actions()}
          <div class="flex items-center gap-3">
            <!-- Collection selector -->
            <FormField id="coverage-collection-select" label={t('collections.collection')} class="mb-0 min-w-48">
              <Select
                id="coverage-collection-select"
                options={[{ value: '', label: t('common.default') }, ...coverageCollections.map(c => ({ value: c.id, label: c.name }))]}
                value={coverageSelectedCollectionId ?? ''}
                onchange={(v) => handleCoverageCollectionChange({ target: { value: v } })}
              />
            </FormField>

            <!-- Filter -->
            <FormField id="coverage-filter-select" label={t('common.filter')} class="mb-0 min-w-48">
              <Select
                id="coverage-filter-select"
                options={[
                  { value: 'all', label: t('testing.allRequirements') },
                  { value: 'true', label: t('testing.coveredOnly') },
                  { value: 'false', label: t('testing.notCoveredOnly') },
                ]}
                value={coverageFilterCovered}
                onchange={(v) => handleCoverageFilterChange({ target: { value: v } })}
              />
            </FormField>

            <!-- Configure button -->
            <div class="flex flex-col gap-1">
              <span class="text-xs font-medium invisible">{t('common.action')}</span>
              <Button variant="default" onclick={handleOpenCoverageConfig}>
                <IconSettings class="w-4 h-4" />
                {t('common.configure')}
              </Button>
            </div>
          </div>
        {/snippet}
      </PageHeader>

      <!-- Content wrapper -->
      <div class="flex-1">
        <TestCoverageReport
          {workspaceId}
          hideHeader={true}
          bind:this={coverageReportRef}
        />
      </div>
    </div>
  {/if}
</div>
