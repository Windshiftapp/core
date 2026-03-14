<script>
  import ResponsiveLineChart from './ResponsiveLineChart.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let { chartData = [] } = $props();
</script>

<ResponsiveLineChart
  {chartData}
  color="#10b981"
  emptyMessage={t('widgets.completionChart.emptyMessage')}
  gradientPrefix="completion"
  minHeight={220}
/>
