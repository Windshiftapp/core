<script>
  import ResponsiveLineChart from './ResponsiveLineChart.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let { chartData = [] } = $props();
</script>

<ResponsiveLineChart
  {chartData}
  color="#3b82f6"
  emptyMessage={t('widgets.createdChart.emptyMessage')}
  gradientPrefix="created"
  minHeight={220}
/>
