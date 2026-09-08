<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import Button from '../../components/Button.svelte';
  import Card from '../../components/Card.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import Input from '../../components/Input.svelte';
  import BasePicker from '../../pickers/BasePicker.svelte';
  import PageHeader from '../../layout/PageHeader.svelte';
  import { Filter, Download, FileText, Clock, Hash, TrendingUp, Briefcase, Users, PieChart } from '@lucide/svelte';
  import StatCard from '../../components/StatCard.svelte';
  import Chart from '../../widgets/Chart.svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { formatDateOnly, getUserTimezone } from '../../utils/dateFormatter.js';
  import { formatAuthenticatedInstant } from '../../utils/authenticatedDateFormatter.js';
  import { authStore } from '../../stores/auth.svelte.js';
  import { dateKeyInZone, monthBoundsInZone, splitWorklogMinutesByDay } from '../../utils/worklogTimezone.js';
  import { openMarkdownPrintView } from '../print/markdownPrintWindow.js';
  import { buildTimeReportMarkdown } from './timeReportMarkdown.js';

  // Reports group and label worklogs by civil date in the reporting timezone,
  // derived from the stored timestamps and split at local midnight.
  const reportTimezone = $derived(getUserTimezone(authStore?.currentUser));

  let worklogs = $state([]);
  let customers = $state([]);
  let projects = $state([]);
  let loading = $state(false);
  let exportLoading = $state(false);

  // Mode: 'personal' or 'project'
  let mode = $state('personal');

  // Personal mode filters
  let filters = $state({
    customer_id: '',
    project_id: '',
    date_from: '',
    date_to: '',
    description_filter: ''
  });

  // Project mode state
  let selectedProjectId = $state('');
  let projectDateFrom = $state('');
  let projectDateTo = $state('');
  let projectWorklogs = $state([]);
  let projectLoading = $state(false);

  // Summary data (personal mode)
  let summary = $state({
    totalHours: 0,
    totalEntries: 0,
    averageHoursPerDay: 0,
    topProject: null,
    topCustomer: null
  });

  // Derived: managed projects
  const managedProjects = $derived(projects.filter(p => p.is_manager));
  const hasManagerAccess = $derived(managedProjects.length > 0);

  // Derived: selected project details
  const selectedProject = $derived(managedProjects.find(p => p.id === parseInt(selectedProjectId)));

  // Project mode computed data
  const projectSummary = $derived.by(() => {
    if (projectWorklogs.length === 0) {
      return { totalHours: 0, budgetPercent: null, budgetLabel: '', contributors: 0, avgPerDay: 0 };
    }

    const totalMinutes = projectWorklogs.reduce((sum, w) => sum + w.duration_minutes, 0);
    const totalHours = Math.round((totalMinutes / 60) * 100) / 100;

    // All-time total hours from backend (unaffected by date filters)
    const allTimeTotal = projectWorklogs[0]?.project_total_hours
      ? Math.round(projectWorklogs[0].project_total_hours * 100) / 100
      : totalHours;

    // Budget from project settings
    let budgetPercent = null;
    let budgetLabel = '';
    const maxHours = selectedProject?.settings?.max_hours;
    if (maxHours && maxHours > 0) {
      budgetPercent = Math.round((allTimeTotal / maxHours) * 100);
      budgetLabel = `${allTimeTotal} / ${maxHours}h (${budgetPercent}%)`;
    }

    // Unique contributors
    const uniqueUsers = new Set(projectWorklogs.map(w => w.user_id).filter(Boolean));
    const contributors = uniqueUsers.size;

    // Avg hours per day
    let avgPerDay = 0;
    if (projectDateFrom && projectDateTo) {
      const daysDiff = Math.ceil((new Date(projectDateTo).getTime() - new Date(projectDateFrom).getTime()) / (1000 * 60 * 60 * 24)) + 1;
      avgPerDay = Math.round((totalHours / daysDiff) * 100) / 100;
    }

    return { totalHours, budgetPercent, budgetLabel, contributors, avgPerDay };
  });

  // Member breakdown data
  const memberBreakdown = $derived.by(() => {
    if (projectWorklogs.length === 0) return [];

    const memberMap = {};
    const dateSet = new Set();

    projectWorklogs.forEach(w => {
      const key = w.user_id || 0;
      if (!memberMap[key]) {
        memberMap[key] = { user_name: w.user_name || 'Unknown', totalMinutes: 0, entries: 0, dates: new Set() };
      }
      memberMap[key].totalMinutes += w.duration_minutes;
      memberMap[key].entries += 1;
      for (const dateStr of splitWorklogMinutesByDay(w.start_time, w.end_time, reportTimezone).keys()) {
        memberMap[key].dates.add(dateStr);
        dateSet.add(dateStr);
      }
    });

    return Object.values(memberMap)
      .map(m => ({
        user_name: m.user_name,
        hours: Math.round((m.totalMinutes / 60) * 100) / 100,
        entries: m.entries,
        avgPerDay: m.dates.size > 0 ? Math.round(((m.totalMinutes / 60) / m.dates.size) * 100) / 100 : 0
      }))
      .sort((a, b) => b.hours - a.hours);
  });

  // Daily hours chart data
  const dailyChartData = $derived.by(() => {
    if (projectWorklogs.length === 0) return [];

    const dailyMap = {};
    projectWorklogs.forEach(w => {
      for (const [dateStr, minutes] of splitWorklogMinutesByDay(w.start_time, w.end_time, reportTimezone)) {
        dailyMap[dateStr] = (dailyMap[dateStr] || 0) + minutes;
      }
    });

    return Object.keys(dailyMap)
      .sort()
      .map(date => ({
        date: new Date(date),
        count: Math.round((dailyMap[date] / 60) * 100) / 100,
        label: formatDateOnly(date)
      }));
  });

  const memberColumns = $derived([
    { key: 'user_name', label: t('time.reports.member') },
    { key: 'hours', label: t('time.reports.hoursLogged'), render: (m) => `${m.hours}h` },
    { key: 'entries', label: t('time.reports.entries') },
    { key: 'avgPerDay', label: t('time.reports.avgPerDay'), render: (m) => `${m.avgPerDay}h` }
  ]);

  const reportColumns = $derived([
    { key: 'date', label: t('common.date'), render: (w) => formatDateOnly(dateKeyInZone(w.start_time, reportTimezone)) },
    { key: 'customer_name', label: t('time.reports.customer') },
    { key: 'project_name', label: t('time.reports.project'), slot: 'project' },
    { key: 'description', label: t('common.description') },
    { key: 'time', label: t('common.time'), slot: 'time' },
    { key: 'duration_minutes', label: t('time.duration'), slot: 'duration' }
  ]);

  onMount(async () => {
    await Promise.all([loadCustomers(), loadProjects()]);

    // Set default date range to current month in the reporting timezone
    const { from, to } = monthBoundsInZone(reportTimezone);

    filters.date_from = from;
    filters.date_to = to;
    projectDateFrom = from;
    projectDateTo = to;

    await loadReports();
  });

  async function loadCustomers() {
    try {
      customers = (await api.customerOrganisations.getAll()) || [];
    } catch (error) {
      console.error('Failed to load customers:', error);
      customers = [];
    }
  }

  async function loadProjects() {
    try {
      projects = (await api.time.projects.getAll()) || [];
    } catch (error) {
      console.error('Failed to load projects:', error);
      projects = [];
    }
  }

  async function loadReports() {
    loading = true;
    try {
      worklogs = (await api.time.worklogs.getAll({ ...filters, timezone: reportTimezone })) || [];
      calculateSummary();
    } catch (error) {
      console.error('Failed to load reports:', error);
      worklogs = [];
    } finally {
      loading = false;
    }
  }

  async function loadProjectWorklogs() {
    if (!selectedProjectId) {
      projectWorklogs = [];
      return;
    }
    projectLoading = true;
    try {
      const dateFilters = { timezone: reportTimezone };
      if (projectDateFrom) dateFilters.date_from = projectDateFrom;
      if (projectDateTo) dateFilters.date_to = projectDateTo;
      projectWorklogs = (await api.time.projects.getWorklogs(selectedProjectId, dateFilters)) || [];
    } catch (error) {
      console.error('Failed to load project worklogs:', error);
      projectWorklogs = [];
    } finally {
      projectLoading = false;
    }
  }

  function calculateSummary() {
    if (worklogs.length === 0) {
      summary = { totalHours: 0, totalEntries: 0, averageHoursPerDay: 0, topProject: null, topCustomer: null };
      return;
    }

    const totalMinutes = worklogs.reduce((sum, w) => sum + w.duration_minutes, 0);
    summary.totalHours = Math.round((totalMinutes / 60) * 100) / 100;
    summary.totalEntries = worklogs.length;

    if (filters.date_from && filters.date_to) {
      const daysDiff = Math.ceil((new Date(filters.date_to).getTime() - new Date(filters.date_from).getTime()) / (1000 * 60 * 60 * 24)) + 1;
      summary.averageHoursPerDay = Math.round((summary.totalHours / daysDiff) * 100) / 100;
    }

    // Top project
    const projectHours = {};
    worklogs.forEach(w => {
      projectHours[w.project_name] = (projectHours[w.project_name] || 0) + w.duration_minutes / 60;
    });
    const topProjectName = Object.keys(projectHours).reduce((a, b) =>
      projectHours[a] > projectHours[b] ? a : b, Object.keys(projectHours)[0]);
    summary.topProject = { name: topProjectName, hours: Math.round(projectHours[topProjectName] * 100) / 100 };

    // Top customer
    const customerHours = {};
    worklogs.forEach(w => {
      customerHours[w.customer_name] = (customerHours[w.customer_name] || 0) + w.duration_minutes / 60;
    });
    const topCustomerName = Object.keys(customerHours).reduce((a, b) =>
      customerHours[a] > customerHours[b] ? a : b, Object.keys(customerHours)[0]);
    summary.topCustomer = { name: topCustomerName, hours: Math.round(customerHours[topCustomerName] * 100) / 100 };
  }

  async function applyFilters() {
    await loadReports();
  }

  function clearFilters() {
    const { from, to } = monthBoundsInZone(reportTimezone);
    filters = {
      customer_id: '',
      project_id: '',
      date_from: from,
      date_to: to,
      description_filter: ''
    };
    loadReports();
  }

  function formatDuration(minutes) {
    const hours = Math.floor(minutes / 60);
    const mins = minutes % 60;
    if (hours === 0) return `${mins}m`;
    if (mins === 0) return `${hours}h`;
    return `${hours}h ${mins}m`;
  }

  function formatTime(unixTimestamp) {
    return formatAuthenticatedInstant(unixTimestamp * 1000, { hour: '2-digit', minute: '2-digit', hour12: false });
  }

  // Export functions
  function exportToCSV() {
    exportLoading = true;

    if (mode === 'project') {
      exportProjectCSV();
    } else {
      exportPersonalCSV();
    }

    exportLoading = false;
  }

  function exportPersonalCSV() {
    const headers = ['Date', 'Customer', 'Project', 'Description', 'Start Time', 'End Time', 'Duration (hours)'];
    /** @type {(string | number)[][]} */
    const csvData = [headers];

    worklogs.forEach(worklog => {
      csvData.push([
        dateKeyInZone(worklog.start_time, reportTimezone),
        worklog.customer_name,
        worklog.project_name,
        worklog.description,
        formatTime(worklog.start_time),
        formatTime(worklog.end_time),
        (worklog.duration_minutes / 60).toFixed(2)
      ]);
    });

    csvData.push([]);
    csvData.push(['Summary']);
    csvData.push(['Total Hours', '', '', '', '', '', summary.totalHours]);
    csvData.push(['Total Entries', '', '', '', '', '', summary.totalEntries]);
    if (summary.topProject) {
      csvData.push(['Top Project', '', summary.topProject.name, '', '', '', summary.topProject.hours]);
    }
    if (summary.topCustomer) {
      csvData.push(['Top Customer', summary.topCustomer.name, '', '', '', '', summary.topCustomer.hours]);
    }

    downloadCSV(csvData, `time-report-${filters.date_from}-to-${filters.date_to}.csv`);
  }

  function exportProjectCSV() {
    const headers = ['Date', 'Member', 'Customer', 'Project', 'Description', 'Start Time', 'End Time', 'Duration (hours)'];
    /** @type {(string | number)[][]} */
    const csvData = [headers];

    projectWorklogs.forEach(worklog => {
      csvData.push([
        dateKeyInZone(worklog.start_time, reportTimezone),
        worklog.user_name || 'Unknown',
        worklog.customer_name,
        worklog.project_name,
        worklog.description,
        formatTime(worklog.start_time),
        formatTime(worklog.end_time),
        (worklog.duration_minutes / 60).toFixed(2)
      ]);
    });

    csvData.push([]);
    csvData.push(['Summary']);
    csvData.push(['Total Hours', '', '', '', '', '', '', projectSummary.totalHours]);
    csvData.push(['Contributors', '', '', '', '', '', '', projectSummary.contributors]);

    csvData.push([]);
    csvData.push(['Member Breakdown']);
    csvData.push(['Member', 'Hours', 'Entries', 'Avg/Day']);
    memberBreakdown.forEach(m => {
      csvData.push([m.user_name, m.hours, m.entries, m.avgPerDay]);
    });

    const projectName = selectedProject?.name || 'project';
    downloadCSV(csvData, `project-report-${projectName}-${projectDateFrom}-to-${projectDateTo}.csv`);
  }

  function downloadCSV(csvData, filename) {
    const csvContent = csvData.map(row => row.map(field => `"${field}"`).join(',')).join('\n');
    const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
    const link = document.createElement('a');
    const url = URL.createObjectURL(blob);
    link.setAttribute('href', url);
    link.setAttribute('download', filename);
    link.style.visibility = 'hidden';
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
  }

  async function exportToPDF() {
    exportLoading = true;

    try {
      if (mode === 'project') {
        exportProjectPDF();
      } else {
        exportPersonalPDF();
      }
    } catch (error) {
      console.error('PDF export failed:', error);
    } finally {
      exportLoading = false;
    }
  }

  function exportPersonalPDF() {
    const report = buildTimeReportMarkdown({
      title: 'Time Tracking Report',
      period: {
        from: filters.date_from || 'All time',
        to: filters.date_to || 'Present',
      },
      generated: formatAuthenticatedInstant(new Date(), { year: 'numeric', month: 'short', day: 'numeric' }),
      summary: [
        { label: 'Total Hours', value: `${summary.totalHours}h` },
        { label: 'Total Entries', value: summary.totalEntries },
        { label: 'Average Hours per Day', value: `${summary.averageHoursPerDay}h` },
        { label: 'Top Project', value: `${summary.topProject?.name || 'N/A'} (${summary.topProject?.hours || 0}h)` },
        { label: 'Top Customer', value: `${summary.topCustomer?.name || 'N/A'} (${summary.topCustomer?.hours || 0}h)` },
      ],
      entries: worklogs.map((worklog) => ({
        heading: `${formatDateOnly(dateKeyInZone(worklog.start_time, reportTimezone))} — ${worklog.project_name}`,
        fields: [
          { label: 'Customer', value: worklog.customer_name },
          { label: 'Duration', value: formatDuration(worklog.duration_minutes) },
          { label: 'Description', value: worklog.description },
          { label: 'Time', value: `${formatTime(worklog.start_time)} – ${formatTime(worklog.end_time)}` },
        ],
      })),
      totalSummary: `Grand Total: ${summary.totalHours} hours across ${summary.totalEntries} entries.`,
    });

    openTimeReport(report, 'Time Tracking Report');
  }

  function exportProjectPDF() {
    const projectName = selectedProject?.name || 'Project';
    const summaryRows = [
      { label: 'Total Hours', value: `${projectSummary.totalHours}h` },
      { label: 'Contributors', value: projectSummary.contributors },
      { label: 'Avg Hours/Day', value: `${projectSummary.avgPerDay}h` },
    ];
    if (projectSummary.budgetLabel) {
      summaryRows.push({ label: 'Budget', value: projectSummary.budgetLabel });
    }

    const report = buildTimeReportMarkdown({
      title: `Project Time Report: ${projectName}`,
      period: {
        from: projectDateFrom || 'All time',
        to: projectDateTo || 'Present',
      },
      generated: formatAuthenticatedInstant(new Date(), { year: 'numeric', month: 'short', day: 'numeric' }),
      summary: summaryRows,
      team: memberBreakdown.map((member) => ({
        name: member.user_name,
        hours: `${member.hours}h`,
        entries: member.entries,
        average: `${member.avgPerDay}h`,
      })),
      entries: projectWorklogs.map((worklog) => ({
        heading: `${formatDateOnly(dateKeyInZone(worklog.start_time, reportTimezone))} — ${worklog.user_name || 'Unknown'}`,
        fields: [
          { label: 'Duration', value: formatDuration(worklog.duration_minutes) },
          { label: 'Description', value: worklog.description },
          { label: 'Time', value: `${formatTime(worklog.start_time)} – ${formatTime(worklog.end_time)}` },
        ],
      })),
    });

    openTimeReport(report, `Project Time Report - ${projectName}`);
  }

  function openTimeReport(content, title) {
    if (!openMarkdownPrintView('/time/worklogs/print', 'time-report', { content, title })) {
      throw new Error('The browser blocked the report window');
    }
  }

  // Reactive filtering for projects based on selected customer
  const filteredProjects = $derived(filters.customer_id
    ? projects.filter(p => p.customer_id === parseInt(filters.customer_id))
    : projects);

  // Filter worklogs by description if filter is set
  const filteredWorklogs = $derived(filters.description_filter
    ? worklogs.filter(w => w.description?.toLowerCase().includes(filters.description_filter.toLowerCase()))
    : worklogs);

  // Current export data source
  const currentExportDisabled = $derived(
    mode === 'personal' ? worklogs.length === 0 : projectWorklogs.length === 0
  );
</script>

<!-- Header -->
<PageHeader
  title={t('time.reports.title')}
  subtitle={mode === 'project' ? t('time.reports.projectReports') : t('time.reports.subtitle')}
>
  {#snippet actions()}
    <div class="flex gap-3">
      <Button
        variant="default"
        onclick={exportToCSV}
        disabled={exportLoading || currentExportDisabled}
        loading={exportLoading}
        icon={Download}
        size="medium"
      >
        {t('time.reports.exportCSV')}
      </Button>
      <Button
        variant="default"
        onclick={exportToPDF}
        disabled={exportLoading || currentExportDisabled}
        loading={exportLoading}
        icon={FileText}
        size="medium"
        dataTestid="time-report-export-pdf"
      >
        {t('time.reports.exportPDF')}
      </Button>
    </div>
  {/snippet}
</PageHeader>

<!-- Mode Toggle -->
{#if hasManagerAccess}
  <div class="mb-6">
    <div class="inline-flex rounded-lg p-1" style="background-color: var(--ds-background-neutral);">
      <button
        class="px-4 py-2 text-sm font-medium rounded-md transition-colors"
        class:mode-active={mode === 'personal'}
        class:mode-inactive={mode !== 'personal'}
        onclick={() => mode = 'personal'}
      >
        {t('time.reports.personal')}
      </button>
      <button
        class="px-4 py-2 text-sm font-medium rounded-md transition-colors"
        class:mode-active={mode === 'project'}
        class:mode-inactive={mode !== 'project'}
        onclick={() => mode = 'project'}
      >
        {t('time.reports.project')}
      </button>
    </div>
  </div>
{/if}

{#if mode === 'personal'}
  <!-- PERSONAL MODE -->

  <!-- Filters -->
  <Card rounded="xl" shadow padding="spacious" class="mb-8">
    <h3 class="text-sm font-semibold mb-4" style="color: var(--ds-text);">{t('time.reports.filters')}</h3>
    <div class="grid grid-cols-1 md:grid-cols-3 gap-4 mb-4">
      <div>
        <label for="report-customer-picker" class="block text-xs font-medium mb-2" style="color: var(--ds-text-subtle);">{t('time.reports.customer')}</label>
        <BasePicker
          id="report-customer-picker"
          bind:value={filters.customer_id}
          items={customers}
          placeholder={t('time.reports.allCustomers')}
          showUnassigned={true}
          unassignedLabel={t('time.reports.allCustomers')}
          getValue={(item) => item.id}
          getLabel={(item) => item.name}
        />
      </div>
      <div>
        <label for="report-project-picker" class="block text-xs font-medium mb-2" style="color: var(--ds-text-subtle);">{t('time.reports.project')}</label>
        <BasePicker
          id="report-project-picker"
          bind:value={filters.project_id}
          items={filteredProjects}
          placeholder={t('time.reports.allProjects')}
          showUnassigned={true}
          unassignedLabel={t('time.reports.allProjects')}
          getValue={(item) => item.id}
          getLabel={(item) => item.name}
        />
      </div>
      <div>
        <label for="report-description-filter" class="block text-xs font-medium mb-2" style="color: var(--ds-text-subtle);">{t('time.reports.descriptionFilter')}</label>
        <Input id="report-description-filter" bind:value={filters.description_filter} placeholder={t('time.reports.searchDescriptions')} size="small" />
      </div>
    </div>
    <div class="grid grid-cols-1 md:grid-cols-2 gap-4 mb-4">
      <div>
        <label for="report-date-from" class="block text-xs font-medium mb-2" style="color: var(--ds-text-subtle);">{t('time.reports.fromDate')}</label>
        <Input id="report-date-from" type="date" bind:value={filters.date_from} size="small" />
      </div>
      <div>
        <label for="report-date-to" class="block text-xs font-medium mb-2" style="color: var(--ds-text-subtle);">{t('time.reports.toDate')}</label>
        <Input id="report-date-to" type="date" bind:value={filters.date_to} size="small" />
      </div>
    </div>
    <div class="flex gap-3">
      <Button
        variant="primary"
        onclick={applyFilters}
        disabled={loading}
        loading={loading}
        icon={Filter}
        size="medium"
      >
        {t('time.reports.applyFilters')}
      </Button>
      <Button
        variant="default"
        onclick={clearFilters}
        size="medium"
      >
        {t('common.clear')}
      </Button>
    </div>
  </Card>

  <!-- Summary Cards -->
  <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6 mb-8">
    <StatCard icon={Clock} label={t('time.reports.totalHours')} value="{summary.totalHours}h" color="blue" />
    <StatCard icon={Hash} label={t('time.reports.totalEntries')} value={summary.totalEntries} color="green" />
    <StatCard icon={TrendingUp} label={t('time.reports.averagePerDay')} value="{summary.averageHoursPerDay}h" color="purple" />
    <StatCard icon={Briefcase} label={t('time.reports.topProject')} value={summary.topProject?.name ?? t('common.noData')} color="orange" />
  </div>

  <!-- Results Table -->
  <Card rounded="xl" shadow padding="none" class="overflow-hidden">
    <DataTable
      columns={reportColumns}
      data={filteredWorklogs}
      keyField="id"
      {loading}
      emptyMessage={t('time.reports.noEntriesFound')}
      pagination={true}
      pageSize={25}
      class="rounded-none border-0 shadow-none overflow-hidden"
    >
      {#snippet project(worklog)}
        <span class="text-sm font-medium" style="color: var(--ds-text);">{worklog.project_name}</span>
      {/snippet}

      {#snippet time(worklog)}
        <span class="text-sm font-mono" style="color: var(--ds-text-subtle);">
          {formatTime(worklog.start_time)} – {formatTime(worklog.end_time)}
        </span>
      {/snippet}

      {#snippet duration(worklog)}
        <span class="text-sm" style="color: var(--ds-text);">
          {formatDuration(worklog.duration_minutes)}
        </span>
      {/snippet}
    </DataTable>

    <!-- Summary Footer -->
    {#if filteredWorklogs.length > 0}
      <div class="px-6 py-4 border-t" style="background-color: var(--ds-surface); border-color: var(--ds-border);">
        <div class="text-sm font-semibold" style="color: var(--ds-text);">
          {t('time.reports.totalTime')}: {summary.totalHours}h
          <span class="ml-2 font-normal" style="color: var(--ds-text-subtle);">({t('time.reports.entriesShown', { count: filteredWorklogs.length })})</span>
        </div>
      </div>
    {/if}
  </Card>

{:else}
  <!-- PROJECT MODE -->

  <!-- Project Picker & Date Range -->
  <Card rounded="xl" shadow padding="spacious" class="mb-8">
    <h3 class="text-sm font-semibold mb-4" style="color: var(--ds-text);">{t('time.reports.filters')}</h3>
    <div class="grid grid-cols-1 md:grid-cols-3 gap-4 mb-4">
      <div>
        <label for="project-report-picker" class="block text-xs font-medium mb-2" style="color: var(--ds-text-subtle);">{t('time.reports.project')}</label>
        <BasePicker
          id="project-report-picker"
          bind:value={selectedProjectId}
          items={managedProjects}
          placeholder={t('time.reports.selectProject')}
          showUnassigned={false}
          getValue={(item) => item.id}
          getLabel={(item) => item.name}
        />
      </div>
      <div>
        <label for="project-date-from" class="block text-xs font-medium mb-2" style="color: var(--ds-text-subtle);">{t('time.reports.fromDate')}</label>
        <Input id="project-date-from" type="date" bind:value={projectDateFrom} size="small" />
      </div>
      <div>
        <label for="project-date-to" class="block text-xs font-medium mb-2" style="color: var(--ds-text-subtle);">{t('time.reports.toDate')}</label>
        <Input id="project-date-to" type="date" bind:value={projectDateTo} size="small" />
      </div>
    </div>
    <div class="flex gap-3">
      <Button
        variant="primary"
        onclick={loadProjectWorklogs}
        disabled={projectLoading || !selectedProjectId}
        loading={projectLoading}
        icon={Filter}
        size="medium"
      >
        {t('time.reports.applyFilters')}
      </Button>
    </div>
  </Card>

  {#if !selectedProjectId}
    <Card rounded="xl" shadow padding="spacious">
      <div class="text-center py-12" style="color: var(--ds-text-subtle);">
        <Briefcase class="w-12 h-12 mx-auto mb-4 opacity-40" />
        <p class="text-sm">{t('time.reports.noProjectSelected')}</p>
      </div>
    </Card>
  {:else if projectWorklogs.length === 0 && !projectLoading}
    <Card rounded="xl" shadow padding="spacious">
      <div class="text-center py-12" style="color: var(--ds-text-subtle);">
        <p class="text-sm">{t('time.reports.noEntriesFound')}</p>
      </div>
    </Card>
  {:else}
    <!-- Summary Cards -->
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6 mb-8">
      <StatCard icon={Clock} label={t('time.reports.totalHours')} value="{projectSummary.totalHours}h" color="blue" />
      <StatCard
        icon={PieChart}
        label={t('time.reports.budgetUsage')}
        value={projectSummary.budgetLabel || t('time.reports.noBudgetSet')}
        color="green"
      />
      <StatCard icon={Users} label={t('time.reports.contributors')} value={projectSummary.contributors} color="purple" />
      <StatCard icon={TrendingUp} label={t('time.reports.avgPerDay')} value="{projectSummary.avgPerDay}h" color="orange" />
    </div>

    <!-- Daily Hours Chart -->
    {#if dailyChartData.length > 1}
      <Card rounded="xl" shadow padding="spacious" class="mb-8">
        <h3 class="text-sm font-semibold mb-4" style="color: var(--ds-text);">{t('time.reports.dailyHours')}</h3>
        <Chart
          type="line"
          series={[{ key: 'hours', label: t('time.reports.hoursLogged'), color: 'var(--ds-accent-blue)', values: dailyChartData.map(d => d.count ?? 0) }]}
          categories={dailyChartData.map(d => d.label)}
          valueFormat={(v) => `${v}h`}
          showYAxis={true}
          yAxisFormat={(v) => `${Math.round(v)}h`}
          minHeight={140}
          maxHeight={260}
        />
      </Card>
    {/if}

    <!-- Member Breakdown -->
    <Card rounded="xl" shadow padding="none" class="mb-8 overflow-hidden">
      <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
        <h3 class="text-sm font-semibold" style="color: var(--ds-text);">{t('time.reports.memberBreakdown')}</h3>
      </div>
      <DataTable
        columns={memberColumns}
        data={memberBreakdown}
        keyField="user_name"
        loading={projectLoading}
        emptyMessage={t('time.reports.noEntriesFound')}
        pagination={false}
        class="rounded-none border-0 shadow-none overflow-hidden"
      />
      {#if memberBreakdown.length > 0}
        <div class="px-6 py-4 border-t" style="background-color: var(--ds-surface); border-color: var(--ds-border);">
          <div class="text-sm font-semibold" style="color: var(--ds-text);">
            {t('time.reports.totalTime')}: {projectSummary.totalHours}h
            <span class="ml-2 font-normal" style="color: var(--ds-text-subtle);">({t('time.reports.contributors')}: {projectSummary.contributors})</span>
          </div>
        </div>
      {/if}
    </Card>
  {/if}
{/if}

<style>
  .mode-active {
    background-color: var(--ds-surface-raised);
    color: var(--ds-text);
    box-shadow: 0 1px 2px rgba(0, 0, 0, 0.05);
  }
  .mode-inactive {
    background-color: transparent;
    color: var(--ds-text-subtle);
  }
  .mode-inactive:hover {
    color: var(--ds-text);
  }
</style>
