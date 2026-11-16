<script>
  import BaseChartWidget from './BaseChartWidget.svelte';
  import Text from '../components/Text.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let { chartData = [] } = $props();
</script>

<Text as="p" size="xs" variant="subtle">{t('widgets.createdChart.definition')}</Text>
<BaseChartWidget
  {chartData}
  seriesKey="created"
  seriesLabel={t('widgets.createdChart.title')}
  seriesColor="#3b82f6"
/>
