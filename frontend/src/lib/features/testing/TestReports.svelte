<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { navigate } from '../../router.js';
  import Button from '../../components/Button.svelte';
  import PageHeader from '../../layout/PageHeader.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import MilestoneCombobox from '../../pickers/MilestoneCombobox.svelte';
  import ResponsiveLineChart from '../../widgets/ResponsiveLineChart.svelte';
  import { BarChart3, RefreshCw, CheckCircle, XCircle, AlertTriangle, SkipForward, Clock, TrendingUp, ShieldCheck } from 'lucide-svelte';
  import TestCoverageReport from './TestCoverageReport.svelte';

  let { workspaceId = null } = $props();

  let loading = $state(true);
  let reportData = $state(null);
  let selectedMilestoneId = $state(null);
  let days = $state(30);

  const workspaceTestBase = $derived.by(() => workspaceId ? `/workspaces/${workspaceId}/tests` : '/workspaces');

  // Transform trend data for the chart
  const chartData = $derived.by(() => {
    if (!reportData?.trend || reportData.trend.length === 0) return [];
    return reportData.trend.map(point => ({
      date: point.date,
      count: point.pass_rate,
      label: `${point.date} - ${point.pass_rate.toFixed(1)}%`
    }));
  });

  // Columns for the failures table
  const failuresColumns = $derived.by(() => [
    {
      key: 'test_case_title',
      label: 'Test Case',
      render: (failure) => `<a href="${workspaceTestBase}/cases/${failure.test_case_id}" style="color: var(--ds-text-link);" class="hover:underline">${failure.test_case_title}</a>`
    },
    {
      key: 'run_name',
      label: 'Run',
      render: (failure) => `<a href="${workspaceTestBase}/runs/${failure.run_id}?from=reports" style="color: var(--ds-text-link);" class="hover:underline">${failure.run_name}</a>`
    },
    {
      key: 'failed_at',
      label: 'Failed At',
      render: (failure) => failure.failed_at ? new Date(failure.failed_at).toLocaleString() : '-'
    }
  ]);

  // Columns for the blocked table
  const blockedColumns = $derived.by(() => [
    {
      key: 'test_case_title',
      label: 'Test Case',
      render: (blocked) => `<a href="${workspaceTestBase}/cases/${blocked.test_case_id}" style="color: var(--ds-text-link);" class="hover:underline">${blocked.test_case_title}</a>`
    },
    {
      key: 'run_name',
      label: 'Run',
      render: (blocked) => `<a href="${workspaceTestBase}/runs/${blocked.run_id}?from=reports" style="color: var(--ds-text-link);" class="hover:underline">${blocked.run_name}</a>`
    },
    {
      key: 'reason',
      label: 'Reason',
      render: (blocked) => blocked.reason || '<span style="color: var(--ds-text-subtle);">No reason provided</span>'
    },
    {
      key: 'blocked_at',
      label: 'Blocked At',
      render: (blocked) => blocked.blocked_at ? new Date(blocked.blocked_at).toLocaleString() : '-'
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

  function handleMilestoneSelect(event) {
    selectedMilestoneId = event.detail.value;
    loadReportData();
  }
</script>

<div class="min-h-screen flex flex-col p-6" style="background-color: var(--ds-surface-raised);">
  <PageHeader
    title="Test Reports"
    subtitle="View test execution metrics and trends"
  >
    {#snippet actions()}
      <div class="flex items-center gap-3">
        <div class="w-48">
          <MilestoneCombobox
            bind:value={selectedMilestoneId}
            placeholder="All milestones"
            on:select={handleMilestoneSelect}
          />
        </div>
        <Button
          onclick={loadReportData}
          variant="primary"
          size="medium"
          disabled={loading}
        >
          <RefreshCw class="w-4 h-4 {loading ? 'animate-spin' : ''}" />
          {loading ? 'Loading...' : 'Refresh'}
        </Button>
      </div>
    {/snippet}
  </PageHeader>

  <!-- Content wrapper -->
  <div class="flex-1 -mx-6 -mb-6 px-10 py-6 space-y-6">
  {#if loading}
    <div class="text-center py-16">
      <RefreshCw class="w-8 h-8 mx-auto mb-4 animate-spin" style="color: var(--ds-text-subtle);" />
      <p style="color: var(--ds-text-subtle);">Loading report data...</p>
    </div>
  {:else if !reportData || reportData.overall?.total_runs === 0}
    <EmptyState
      icon={BarChart3}
      title="No test data found"
      description="Complete some test runs to see reports here."
    />
  {:else}
    <!-- Stats Cards -->
    <div class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-4">
      <!-- Total Tests -->
      <div class="p-4">
        <div class="flex items-center gap-2 mb-2">
          <Clock class="w-4 h-4" style="color: var(--ds-text-subtle);" />
          <span class="text-xs font-medium uppercase tracking-wide" style="color: var(--ds-text-subtle);">Total Tests</span>
        </div>
        <div class="text-2xl font-bold" style="color: var(--ds-text);">
          {reportData.overall.total_tests}
        </div>
        <div class="text-xs mt-1" style="color: var(--ds-text-subtle);">
          {reportData.overall.total_runs} runs
        </div>
      </div>

      <!-- Passed -->
      <div class="p-4">
        <div class="flex items-center gap-2 mb-2">
          <CheckCircle class="w-4 h-4" style="color: var(--ds-text-subtle);" />
          <span class="text-xs font-medium uppercase tracking-wide" style="color: var(--ds-text-subtle);">Passed</span>
        </div>
        <div class="text-2xl font-bold" style="color: var(--ds-text);">
          {reportData.overall.passed}
        </div>
      </div>

      <!-- Failed -->
      <div class="p-4">
        <div class="flex items-center gap-2 mb-2">
          <XCircle class="w-4 h-4" style="color: var(--ds-text-subtle);" />
          <span class="text-xs font-medium uppercase tracking-wide" style="color: var(--ds-text-subtle);">Failed</span>
        </div>
        <div class="text-2xl font-bold" style="color: var(--ds-text);">
          {reportData.overall.failed}
        </div>
      </div>

      <!-- Blocked -->
      <div class="p-4">
        <div class="flex items-center gap-2 mb-2">
          <AlertTriangle class="w-4 h-4" style="color: var(--ds-text-subtle);" />
          <span class="text-xs font-medium uppercase tracking-wide" style="color: var(--ds-text-subtle);">Blocked</span>
        </div>
        <div class="text-2xl font-bold" style="color: var(--ds-text);">
          {reportData.overall.blocked}
        </div>
      </div>

      <!-- Skipped -->
      <div class="p-4">
        <div class="flex items-center gap-2 mb-2">
          <SkipForward class="w-4 h-4" style="color: var(--ds-text-subtle);" />
          <span class="text-xs font-medium uppercase tracking-wide" style="color: var(--ds-text-subtle);">Skipped</span>
        </div>
        <div class="text-2xl font-bold" style="color: var(--ds-text);">
          {reportData.overall.skipped}
        </div>
      </div>

      <!-- Pass Rate -->
      <div class="p-4">
        <div class="flex items-center gap-2 mb-2">
          <TrendingUp class="w-4 h-4" style="color: var(--ds-text-subtle);" />
          <span class="text-xs font-medium uppercase tracking-wide" style="color: var(--ds-text-subtle);">Pass Rate</span>
        </div>
        <div class="text-2xl font-bold" style="color: var(--ds-text);">
          {reportData.overall.pass_rate.toFixed(1)}%
        </div>
      </div>
    </div>

    <!-- Pass Rate Trend Chart -->
    <div>
      <div class="px-5 py-4 border-b flex items-center gap-2" style="border-color: var(--ds-border);">
        <TrendingUp class="w-5 h-5" style="color: var(--ds-text-subtle);" />
        <h3 class="text-lg font-semibold" style="color: var(--ds-text);">
          Pass Rate Trend (Last {days} Days)
        </h3>
      </div>
      <div class="p-5">
        {#if chartData.length > 0}
          <ResponsiveLineChart
            {chartData}
            color="var(--ds-status-success-solid)"
            emptyMessage="No trend data available"
            gradientPrefix="pass-rate"
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
            No trend data available for the selected period
          </div>
        {/if}
      </div>
    </div>

    <!-- Recent Failures and Blocked Tables - Side by Side -->
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
      <!-- Recent Failures Table -->
      <div>
        <div class="px-5 py-4 border-b flex items-center gap-2" style="border-color: var(--ds-border);">
          <XCircle class="w-5 h-5" style="color: var(--ds-text-subtle);" />
          <div>
            <h3 class="text-lg font-semibold" style="color: var(--ds-text);">Recent Failures</h3>
            <p class="text-sm" style="color: var(--ds-text-subtle);">
              Latest test failures from the last {days} days
            </p>
          </div>
        </div>
        {#if reportData.recent_failures && reportData.recent_failures.length > 0}
          <DataTable
            columns={failuresColumns}
            data={reportData.recent_failures}
            keyField="test_case_id"
            emptyMessage="No failures to show"
          />
        {:else}
          <EmptyState
            icon={CheckCircle}
            title="No failures!"
            description="All tests are passing"
          />
        {/if}
      </div>

      <!-- Blocked Tests Table -->
      <div>
        <div class="px-5 py-4 border-b flex items-center gap-2" style="border-color: var(--ds-border);">
          <AlertTriangle class="w-5 h-5" style="color: var(--ds-text-subtle);" />
          <div>
            <h3 class="text-lg font-semibold" style="color: var(--ds-text);">Blocked Tests</h3>
            <p class="text-sm" style="color: var(--ds-text-subtle);">
              Tests blocked with reasons from the last {days} days
            </p>
          </div>
        </div>
        {#if reportData.recent_blocked && reportData.recent_blocked.length > 0}
          <DataTable
            columns={blockedColumns}
            data={reportData.recent_blocked}
            keyField="test_case_id"
            emptyMessage="No blocked tests"
          />
        {:else}
          <EmptyState
            icon={CheckCircle}
            title="No blocked tests!"
            description="All tests are unblocked"
          />
        {/if}
      </div>
    </div>
  {/if}

  <!-- Requirements Coverage Section - always shown, handles its own empty state -->
  {#if !loading}
    <div class="rounded-lg border shadow-sm overflow-hidden" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
      <TestCoverageReport {workspaceId} />
    </div>
  {/if}
  </div>
</div>
