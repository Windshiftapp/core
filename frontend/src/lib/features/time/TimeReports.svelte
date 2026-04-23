<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import Button from '../../components/Button.svelte';
  import Card from '../../components/Card.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import Input from '../../components/Input.svelte';
  import BasePicker from '../../pickers/BasePicker.svelte';
  import PageHeader from '../../layout/PageHeader.svelte';
  import { Filter, Download, FileText, Clock, Hash, TrendingUp, Briefcase, Users, PieChart } from 'lucide-svelte';
  import StatCard from '../../components/StatCard.svelte';
  import Chart from '../../widgets/Chart.svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { formatDate, formatDateSimple, formatDateWithOptions } from '../../utils/dateFormatter.js';
  import { escapeHtml } from '../../utils/sanitize.ts';

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
      const daysDiff = Math.ceil((new Date(projectDateTo) - new Date(projectDateFrom)) / (1000 * 60 * 60 * 24)) + 1;
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
      const dateStr = formatDateSimple(new Date(w.date * 1000));
      memberMap[key].dates.add(dateStr);
      dateSet.add(dateStr);
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
      const dateStr = formatDate(new Date(w.date * 1000));
      dailyMap[dateStr] = (dailyMap[dateStr] || 0) + w.duration_minutes;
    });

    return Object.keys(dailyMap)
      .sort()
      .map(date => ({
        date: new Date(date),
        count: Math.round((dailyMap[date] / 60) * 100) / 100,
        label: formatDateSimple(new Date(date))
      }));
  });

  const memberColumns = $derived([
    { key: 'user_name', label: t('time.reports.member') },
    { key: 'hours', label: t('time.reports.hoursLogged'), render: (m) => `${m.hours}h` },
    { key: 'entries', label: t('time.reports.entries') },
    { key: 'avgPerDay', label: t('time.reports.avgPerDay'), render: (m) => `${m.avgPerDay}h` }
  ]);

  const reportColumns = $derived([
    { key: 'date', label: t('common.date'), render: (w) => formatDateSimple(new Date(w.date * 1000)) },
    { key: 'customer_name', label: t('time.reports.customer') },
    { key: 'project_name', label: t('time.reports.project'), slot: 'project' },
    { key: 'description', label: t('common.description') },
    { key: 'time', label: t('common.time'), slot: 'time' },
    { key: 'duration_minutes', label: t('time.duration'), slot: 'duration' }
  ]);

  onMount(async () => {
    await Promise.all([loadCustomers(), loadProjects()]);

    // Set default date range to current month
    const now = new Date();
    const monthStart = formatDate(new Date(now.getFullYear(), now.getMonth(), 1));
    const monthEnd = formatDate(new Date(now.getFullYear(), now.getMonth() + 1, 0));

    filters.date_from = monthStart;
    filters.date_to = monthEnd;
    projectDateFrom = monthStart;
    projectDateTo = monthEnd;

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
      worklogs = (await api.time.worklogs.getAll(filters)) || [];
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
      const dateFilters = {};
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
      const daysDiff = Math.ceil((new Date(filters.date_to) - new Date(filters.date_from)) / (1000 * 60 * 60 * 24)) + 1;
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
    const now = new Date();
    filters = {
      customer_id: '',
      project_id: '',
      date_from: formatDate(new Date(now.getFullYear(), now.getMonth(), 1)),
      date_to: formatDate(new Date(now.getFullYear(), now.getMonth() + 1, 0)),
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
    const date = new Date(unixTimestamp * 1000);
    return formatDateWithOptions(date, { hour: '2-digit', minute: '2-digit', hour12: false });
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
    const csvData = [headers];

    worklogs.forEach(worklog => {
      csvData.push([
        formatDateSimple(new Date(worklog.date * 1000)),
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
    const csvData = [headers];

    projectWorklogs.forEach(worklog => {
      csvData.push([
        formatDateSimple(new Date(worklog.date * 1000)),
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
    const template = localStorage.getItem('ostime_export_template') || getDefaultTemplate();

    const templateData = {
      date_from: filters.date_from || 'All time',
      date_to: filters.date_to || 'Present',
      generated_date: formatDateSimple(new Date()),
      total_hours: summary.totalHours.toString(),
      total_entries: summary.totalEntries.toString(),
      average_hours_per_day: summary.averageHoursPerDay.toString(),
      top_project_name: summary.topProject?.name || 'N/A',
      top_project_hours: summary.topProject?.hours?.toString() || '0',
      top_customer_name: summary.topCustomer?.name || 'N/A',
      top_customer_hours: summary.topCustomer?.hours?.toString() || '0',
      entries: worklogs.map(worklog => ({
        date: formatDateSimple(new Date(worklog.date * 1000)),
        project_name: worklog.project_name,
        customer_name: worklog.customer_name,
        duration: formatDuration(worklog.duration_minutes),
        description: worklog.description,
        start_time: formatTime(worklog.start_time),
        end_time: formatTime(worklog.end_time)
      }))
    };

    let processedContent = processTemplate(template, templateData);
    openPrintWindow(processedContent);
  }

  function exportProjectPDF() {
    const projectName = selectedProject?.name || 'Project';
    let html = `<h1>Project Time Report: ${escapeHtml(projectName)}</h1>`;
    html += `<p><strong>Report Period:</strong> ${escapeHtml(projectDateFrom || 'All time')} to ${escapeHtml(projectDateTo || 'Present')}<br>`;
    html += `<strong>Generated:</strong> ${escapeHtml(formatDateSimple(new Date()))}</p><hr>`;
    html += `<h2>Summary</h2><ul>`;
    html += `<li><strong>Total Hours:</strong> ${projectSummary.totalHours}h</li>`;
    html += `<li><strong>Contributors:</strong> ${projectSummary.contributors}</li>`;
    html += `<li><strong>Avg Hours/Day:</strong> ${projectSummary.avgPerDay}h</li>`;
    if (projectSummary.budgetLabel) {
      html += `<li><strong>Budget:</strong> ${escapeHtml(projectSummary.budgetLabel)}</li>`;
    }
    html += `</ul><hr>`;
    html += `<h2>Team Breakdown</h2>`;
    html += `<table style="width:100%;border-collapse:collapse;"><thead><tr>`;
    html += `<th style="text-align:left;padding:8px;border-bottom:2px solid #e5e7eb;">Member</th>`;
    html += `<th style="text-align:right;padding:8px;border-bottom:2px solid #e5e7eb;">Hours</th>`;
    html += `<th style="text-align:right;padding:8px;border-bottom:2px solid #e5e7eb;">Entries</th>`;
    html += `<th style="text-align:right;padding:8px;border-bottom:2px solid #e5e7eb;">Avg/Day</th>`;
    html += `</tr></thead><tbody>`;
    memberBreakdown.forEach(m => {
      html += `<tr>`;
      html += `<td style="padding:8px;border-bottom:1px solid #e5e7eb;">${escapeHtml(m.user_name)}</td>`;
      html += `<td style="text-align:right;padding:8px;border-bottom:1px solid #e5e7eb;">${m.hours}h</td>`;
      html += `<td style="text-align:right;padding:8px;border-bottom:1px solid #e5e7eb;">${m.entries}</td>`;
      html += `<td style="text-align:right;padding:8px;border-bottom:1px solid #e5e7eb;">${m.avgPerDay}h</td>`;
      html += `</tr>`;
    });
    html += `</tbody></table><hr>`;
    html += `<h2>Time Entries</h2>`;
    projectWorklogs.forEach(w => {
      html += `<h3>${escapeHtml(formatDateSimple(new Date(w.date * 1000)))} - ${escapeHtml(w.user_name || 'Unknown')}</h3>`;
      html += `<p><strong>Duration:</strong> ${escapeHtml(formatDuration(w.duration_minutes))}<br>`;
      html += `<strong>Description:</strong> ${escapeHtml(w.description)}<br>`;
      html += `<strong>Time:</strong> ${escapeHtml(formatTime(w.start_time))} - ${escapeHtml(formatTime(w.end_time))}</p><hr>`;
    });
    html += `<p><em>Generated by ostime Time Management System</em></p>`;

    openPrintWindow(html);
  }

  function openPrintWindow(content) {
    const printWindow = window.open('', '_blank');
    const printContent = `
      <!DOCTYPE html>
      <html>
      <head>
        <title>Time Tracking Report</title>
        <style>
          body { font-family: Arial, sans-serif; margin: 20px; line-height: 1.6; }
          h1 { color: #2563eb; font-size: 2em; margin-bottom: 0.5em; }
          h2 { color: #2563eb; font-size: 1.5em; margin: 1em 0 0.5em 0; }
          h3 { color: #374151; font-size: 1.2em; margin: 0.8em 0 0.3em 0; }
          hr { border: none; border-top: 2px solid #e5e7eb; margin: 1.5em 0; }
          strong { color: #374151; }
          ul, ol { padding-left: 1.5em; }
          li { margin-bottom: 0.3em; }
          table { font-size: 0.9em; }
          .page-break { page-break-before: always; }
        </style>
      </head>
      <body>
        ${content}
      </body>
      </html>
    `;

    printWindow.document.write(printContent);
    printWindow.document.close();

    setTimeout(() => {
      printWindow.print();
    }, 500);
  }

  function getDefaultTemplate() {
    return `<h1>Time Tracking Report</h1>
<p><strong>Report Period:</strong> {{date_from}} to {{date_to}}<br><strong>Generated:</strong> {{generated_date}}</p>
<hr>
<h2>Summary</h2>
<ul>
<li><strong>Total Hours:</strong> {{total_hours}}h</li>
<li><strong>Total Entries:</strong> {{total_entries}}</li>
<li><strong>Average Hours per Day:</strong> {{average_hours_per_day}}h</li>
<li><strong>Top Project:</strong> {{top_project_name}} ({{top_project_hours}}h)</li>
<li><strong>Top Customer:</strong> {{top_customer_name}} ({{top_customer_hours}}h)</li>
</ul>
<hr>
<h2>Time Entries</h2>
{{#each entries}}
<h3>{{date}} - {{project_name}}</h3>
<p><strong>Customer:</strong> {{customer_name}}<br><strong>Duration:</strong> {{duration}}<br><strong>Description:</strong> {{description}}<br><strong>Time:</strong> {{start_time}} - {{end_time}}</p>
<hr>
{{/each}}
<h2>Total Summary</h2>
<p><strong>Grand Total:</strong> {{total_hours}} hours across {{total_entries}} entries.</p>
<hr>
<p><em>Generated by ostime Time Management System</em></p>`;
  }

  function processTemplate(template, data) {
    let processed = template;

    Object.keys(data).forEach(key => {
      if (key !== 'entries') {
        const regex = new RegExp(`{{${key}}}`, 'g');
        processed = processed.replace(regex, escapeHtml(data[key]));
      }
    });

    const entriesMatch = processed.match(/{{#each entries}}(.*?){{\/each}}/s);
    if (entriesMatch && data.entries) {
      const entryTemplate = entriesMatch[1];
      let entriesContent = '';

      data.entries.forEach(entry => {
        let entryContent = entryTemplate;
        Object.keys(entry).forEach(key => {
          const regex = new RegExp(`{{${key}}}`, 'g');
          entryContent = entryContent.replace(regex, escapeHtml(entry[key]));
        });
        entriesContent += entryContent;
      });

      processed = processed.replace(/{{#each entries}}.*?{{\/each}}/s, entriesContent);
    }

    return processed;
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
      <div class="px-6 py-4 border-t" style="background-color: var(--ds-background-neutral); border-color: var(--ds-border);">
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
          categories={dailyChartData.map(d => d.label || formatDateSimple(d.date))}
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
        <div class="px-6 py-4 border-t" style="background-color: var(--ds-background-neutral); border-color: var(--ds-border);">
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
