<script>
  import BaseChartWidget from './BaseChartWidget.svelte';
  import Text from '../components/Text.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let { chartData = [] } = $props();
</script>

<Text as="p" size="xs" variant="subtle">{t('widgets.completionChart.definition')}</Text>
<BaseChartWidget
  {chartData}
  seriesKey="completion"
  seriesLabel={t('widgets.completionChart.title')}
  seriesColor="#10b981"
/>
