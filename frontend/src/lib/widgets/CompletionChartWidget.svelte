<script>
  import ResponsiveLineChart from './ResponsiveLineChart.svelte';

  export let chartData = [];
</script>

<ResponsiveLineChart
  {chartData}
  color="#10b981"
  emptyMessage="No completion data available"
  gradientPrefix="completion"
  minHeight={220}
/>
