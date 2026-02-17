<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import Button from '../../components/Button.svelte';
  import Card from '../../components/Card.svelte';
  import Input from '../../components/Input.svelte';
  import BasePicker from '../../pickers/BasePicker.svelte';
  import { Filter, Download, FileText } from 'lucide-svelte';
  import { t } from '../../stores/i18n.svelte.js';

  // Escape HTML to prevent XSS in print templates
  function escapeHtml(text) {
    if (text == null) return '';
    const div = document.createElement('div');
    div.textContent = String(text);
    return div.innerHTML;
  }

  let worklogs = $state([]);
  let customers = $state([]);
  let projects = $state([]);
  let loading = $state(false);
  let exportLoading = $state(false);

  // Filters
  let filters = $state({
    customer_id: '',
    project_id: '',
    date_from: '',
    date_to: '',
    description_filter: ''
  });

  // Summary data
  let summary = $state({
    totalHours: 0,
    totalEntries: 0,
    averageHoursPerDay: 0,
    topProject: null,
    topCustomer: null
  });

  onMount(async () => {
    await Promise.all([loadCustomers(), loadProjects()]);
    
    // Set default date range to current month
    const now = new Date();
    filters.date_from = new Date(now.getFullYear(), now.getMonth(), 1).toISOString().split('T')[0];
    filters.date_to = new Date(now.getFullYear(), now.getMonth() + 1, 0).toISOString().split('T')[0];
    
    await loadReports();
  });

  async function loadCustomers() {
    try {
      const result = await api.customerOrganisations.getAll();
      customers = result || [];
    } catch (error) {
      console.error('Failed to load customers:', error);
      customers = [];
    }
  }

  async function loadProjects() {
    try {
      const result = await api.time.projects.getAll();
      projects = result || [];
    } catch (error) {
      console.error('Failed to load projects:', error);
      projects = [];
    }
  }

  async function loadReports() {
    loading = true;
    try {
      const result = await api.time.worklogs.getAll(filters);
      worklogs = result || [];
      calculateSummary();
    } catch (error) {
      console.error('Failed to load reports:', error);
      worklogs = [];
    } finally {
      loading = false;
    }
  }

  function calculateSummary() {
    if (worklogs.length === 0) {
      summary = {
        totalHours: 0,
        totalEntries: 0,
        averageHoursPerDay: 0,
        topProject: null,
        topCustomer: null
      };
      return;
    }

    // Total hours and entries
    const totalMinutes = worklogs.reduce((sum, w) => sum + w.duration_minutes, 0);
    summary.totalHours = Math.round((totalMinutes / 60) * 100) / 100;
    summary.totalEntries = worklogs.length;

    // Average hours per day
    if (filters.date_from && filters.date_to) {
      const startDate = new Date(filters.date_from);
      const endDate = new Date(filters.date_to);
      const daysDiff = Math.ceil((endDate - startDate) / (1000 * 60 * 60 * 24)) + 1;
      summary.averageHoursPerDay = Math.round((summary.totalHours / daysDiff) * 100) / 100;
    }

    // Top project by time
    const projectHours = {};
    worklogs.forEach(w => {
      if (!projectHours[w.project_name]) {
        projectHours[w.project_name] = 0;
      }
      projectHours[w.project_name] += w.duration_minutes / 60;
    });
    const topProjectName = Object.keys(projectHours).reduce((a, b) => 
      projectHours[a] > projectHours[b] ? a : b, Object.keys(projectHours)[0]);
    summary.topProject = {
      name: topProjectName,
      hours: Math.round(projectHours[topProjectName] * 100) / 100
    };

    // Top customer by time
    const customerHours = {};
    worklogs.forEach(w => {
      if (!customerHours[w.customer_name]) {
        customerHours[w.customer_name] = 0;
      }
      customerHours[w.customer_name] += w.duration_minutes / 60;
    });
    const topCustomerName = Object.keys(customerHours).reduce((a, b) => 
      customerHours[a] > customerHours[b] ? a : b, Object.keys(customerHours)[0]);
    summary.topCustomer = {
      name: topCustomerName,
      hours: Math.round(customerHours[topCustomerName] * 100) / 100
    };
  }

  async function applyFilters() {
    await loadReports();
  }

  function clearFilters() {
    const now = new Date();
    filters = {
      customer_id: '',
      project_id: '',
      date_from: new Date(now.getFullYear(), now.getMonth(), 1).toISOString().split('T')[0],
      date_to: new Date(now.getFullYear(), now.getMonth() + 1, 0).toISOString().split('T')[0],
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
    return date.toLocaleTimeString('en-US', { 
      hour: '2-digit', 
      minute: '2-digit',
      hour12: false 
    });
  }

  // Export functions
  function exportToCSV() {
    exportLoading = true;
    
    const headers = ['Date', 'Customer', 'Project', 'Description', 'Start Time', 'End Time', 'Duration (hours)'];
    const csvData = [headers];
    
    worklogs.forEach(worklog => {
      csvData.push([
        new Date(worklog.date * 1000).toLocaleDateString(),
        worklog.customer_name,
        worklog.project_name,
        worklog.description,
        formatTime(worklog.start_time),
        formatTime(worklog.end_time),
        (worklog.duration_minutes / 60).toFixed(2)
      ]);
    });
    
    // Add summary row
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
    
    const csvContent = csvData.map(row => row.map(field => `"${field}"`).join(',')).join('\n');
    const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
    const link = document.createElement('a');
    const url = URL.createObjectURL(blob);
    link.setAttribute('href', url);
    link.setAttribute('download', `time-report-${filters.date_from}-to-${filters.date_to}.csv`);
    link.style.visibility = 'hidden';
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    
    exportLoading = false;
  }

  async function exportToPDF() {
    exportLoading = true;
    
    try {
      // Get the custom template from localStorage
      const template = localStorage.getItem('ostime_export_template') || getDefaultTemplate();
      
      // Prepare template data
      const templateData = {
        date_from: filters.date_from || 'All time',
        date_to: filters.date_to || 'Present',
        generated_date: new Date().toLocaleDateString(),
        total_hours: summary.totalHours.toString(),
        total_entries: summary.totalEntries.toString(),
        average_hours_per_day: summary.averageHoursPerDay.toString(),
        top_project_name: summary.topProject?.name || 'N/A',
        top_project_hours: summary.topProject?.hours?.toString() || '0',
        top_customer_name: summary.topCustomer?.name || 'N/A',
        top_customer_hours: summary.topCustomer?.hours?.toString() || '0',
        entries: worklogs.map(worklog => ({
          date: new Date(worklog.date * 1000).toLocaleDateString(),
          project_name: worklog.project_name,
          customer_name: worklog.customer_name,
          duration: formatDuration(worklog.duration_minutes),
          description: worklog.description,
          start_time: formatTime(worklog.start_time),
          end_time: formatTime(worklog.end_time)
        }))
      };
      
      // Process template (template is now HTML from Tiptap)
      let processedContent = processTemplate(template, templateData);
      
      // Create print window with processed template
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
            code { background: #f3f4f6; padding: 2px 4px; border-radius: 3px; font-family: monospace; }
            .page-break { page-break-before: always; }
          </style>
        </head>
        <body>
          ${processedContent}
        </body>
        </html>
      `;
      
      printWindow.document.write(printContent);
      printWindow.document.close();
      
      setTimeout(() => {
        printWindow.print();
        exportLoading = false;
      }, 500);
    } catch (error) {
      console.error('PDF export failed:', error);
      exportLoading = false;
      alert('Failed to generate PDF. Please try again.');
    }
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

    // Replace simple variables (escape HTML to prevent XSS)
    Object.keys(data).forEach(key => {
      if (key !== 'entries') {
        const regex = new RegExp(`{{${key}}}`, 'g');
        processed = processed.replace(regex, escapeHtml(data[key]));
      }
    });

    // Handle entries loop
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
</script>

<!-- Header -->
<div class="mb-6 flex justify-between items-start">
  <div>
    <h2 class="text-lg font-semibold" style="color: var(--ds-text);">{t('time.reports.title')}</h2>
    <div class="text-xs mt-1" style="color: var(--ds-text-subtle);">
      {t('time.reports.subtitle')}
    </div>
  </div>
    <div class="flex gap-3">
      <Button
        variant="default"
        onclick={exportToCSV}
        disabled={exportLoading || worklogs.length === 0}
        loading={exportLoading}
        icon={Download}
        size="medium"
      >
        {t('time.reports.exportCSV')}
      </Button>
      <Button
        variant="default"
        onclick={exportToPDF}
        disabled={exportLoading || worklogs.length === 0}
        loading={exportLoading}
        icon={FileText}
        size="medium"
      >
        {t('time.reports.exportPDF')}
      </Button>
    </div>
  </div>

  <!-- Filters -->
  <Card rounded="xl" shadow padding="spacious" class="mb-8">
    <h3 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">{t('time.reports.filters')}</h3>
    <div class="grid grid-cols-1 md:grid-cols-3 gap-4 mb-4">
      <div>
        <label class="block text-xs font-medium mb-2" style="color: var(--ds-text-subtle);">{t('time.reports.customer')}</label>
        <BasePicker
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
        <label class="block text-xs font-medium mb-2" style="color: var(--ds-text-subtle);">{t('time.reports.project')}</label>
        <BasePicker
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
        <label class="block text-xs font-medium mb-2" style="color: var(--ds-text-subtle);">{t('time.reports.descriptionFilter')}</label>
        <Input bind:value={filters.description_filter} placeholder={t('time.reports.searchDescriptions')} size="small" />
      </div>
    </div>
    <div class="grid grid-cols-1 md:grid-cols-2 gap-4 mb-4">
      <div>
        <label class="block text-xs font-medium mb-2" style="color: var(--ds-text-subtle);">{t('time.reports.fromDate')}</label>
        <Input type="date" bind:value={filters.date_from} size="small" />
      </div>
      <div>
        <label class="block text-xs font-medium mb-2" style="color: var(--ds-text-subtle);">{t('time.reports.toDate')}</label>
        <Input type="date" bind:value={filters.date_to} size="small" />
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
    <Card rounded="xl" shadow padding="spacious" class="text-center">
      <div class="text-3xl font-bold text-blue-600 mb-2">{summary.totalHours}h</div>
      <div class="text-sm" style="color: var(--ds-text-subtle);">{t('time.reports.totalHours')}</div>
    </Card>
    <Card rounded="xl" shadow padding="spacious" class="text-center">
      <div class="text-3xl font-bold text-green-600 mb-2">{summary.totalEntries}</div>
      <div class="text-sm" style="color: var(--ds-text-subtle);">{t('time.reports.totalEntries')}</div>
    </Card>
    <Card rounded="xl" shadow padding="spacious" class="text-center">
      <div class="text-3xl font-bold text-purple-600 mb-2">{summary.averageHoursPerDay}h</div>
      <div class="text-sm" style="color: var(--ds-text-subtle);">{t('time.reports.averagePerDay')}</div>
    </Card>
    <Card rounded="xl" shadow padding="spacious" class="text-center">
      {#if summary.topProject}
        <div class="text-lg font-semibold mb-1" style="color: var(--ds-text);">{summary.topProject.name}</div>
        <div class="text-sm text-orange-600 font-medium">{summary.topProject.hours}h</div>
        <div class="text-xs" style="color: var(--ds-text-subtle);">{t('time.reports.topProject')}</div>
      {:else}
        <div class="text-lg" style="color: var(--ds-text-subtle);">{t('common.noData')}</div>
        <div class="text-xs" style="color: var(--ds-text-subtle);">{t('time.reports.topProject')}</div>
      {/if}
    </Card>
  </div>

  <!-- Results Table -->
  <Card rounded="xl" shadow padding="none" class="overflow-hidden">
    {#if filteredWorklogs.length === 0}
      <div class="p-8 text-center" style="color: var(--ds-text-subtle);">
        {#if loading}
          {t('time.reports.loadingReports')}
        {:else}
          {t('time.reports.noEntriesFound')}
        {/if}
      </div>
    {:else}
      <div class="overflow-x-auto">
        <table class="w-full">
          <thead style="background-color: var(--ds-background-neutral);">
            <tr>
              <th class="px-6 py-4 text-left text-xs font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle);">{t('common.date')}</th>
              <th class="px-6 py-4 text-left text-xs font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle);">{t('time.reports.customer')}</th>
              <th class="px-6 py-4 text-left text-xs font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle);">{t('time.reports.project')}</th>
              <th class="px-6 py-4 text-left text-xs font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle);">{t('common.description')}</th>
              <th class="px-6 py-4 text-left text-xs font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle);">{t('common.time')}</th>
              <th class="px-6 py-4 text-left text-xs font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle);">{t('time.duration')}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100">
            {#each filteredWorklogs as worklog (worklog.id)}
              <tr class="transition-colors duration-150 hover:bg-opacity-50" style="hover:background-color: var(--ds-background-neutral-hovered);">
                <td class="px-6 py-4 text-sm" style="color: var(--ds-text);">
                  {new Date(worklog.date * 1000).toLocaleDateString()}
                </td>
                <td class="px-6 py-4 text-sm" style="color: var(--ds-text);">
                  {worklog.customer_name}
                </td>
                <td class="px-6 py-4 text-sm font-medium" style="color: var(--ds-text);">
                  {worklog.project_name}
                </td>
                <td class="px-6 py-4 text-sm" style="color: var(--ds-text);">
                  {worklog.description}
                </td>
                <td class="px-6 py-4 text-sm font-mono" style="color: var(--ds-text-subtle);">
                  {formatTime(worklog.start_time)} - {formatTime(worklog.end_time)}
                </td>
                <td class="px-6 py-4 text-sm font-semibold" style="color: var(--ds-text);">
                  {formatDuration(worklog.duration_minutes)}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
      
      <!-- Summary Footer -->
      <div class="px-6 py-4 border-t" style="background-color: var(--ds-background-neutral); border-color: var(--ds-border);">
        <div class="text-sm font-semibold" style="color: var(--ds-text);">
          {t('time.reports.totalTime')}: {summary.totalHours}h
          <span class="ml-2 font-normal" style="color: var(--ds-text-subtle);">({t('time.reports.entriesShown', { count: filteredWorklogs.length })})</span>
        </div>
      </div>
    {/if}
  </Card>