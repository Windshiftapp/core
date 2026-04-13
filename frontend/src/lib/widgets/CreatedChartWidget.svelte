<script>
  import Chart from './Chart.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let { chartData = [] } = $props();

  const fmtDate = (v) => {
    const d = v instanceof Date ? v : new Date(v);
    return `${String(d.getMonth() + 1).padStart(2, '0')}/${String(d.getDate()).padStart(2, '0')}`;
  };

  const categories = $derived(chartData.map(d => d.label || fmtDate(d.date)));
  const series = $derived([{
    key: 'created', label: t('widgets.createdChart.title'), color: '#3b82f6',
    values: chartData.map(d => d.count ?? 0)
  }]);
</script>

<Chart
  type="line"
  {series}
  {categories}
  minHeight={220}
  showYAxis={false}
  gridLineCount={4}
  gridDashed={true}
  emptyMessage={t('widgets.createdChart.emptyMessage')}
/>
