<script>
  import ResponsiveLineChart from './ResponsiveLineChart.svelte';
  import { t } from '../stores/i18n.svelte.js';

  export let chartData = [];
</script>

<ResponsiveLineChart
  {chartData}
  color="#10b981"
  emptyMessage={t('widgets.completionChart.emptyMessage')}
  gradientPrefix="completion"
  minHeight={220}
/>
