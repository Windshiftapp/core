<script>
  import ResponsiveLineChart from './ResponsiveLineChart.svelte';

  export let chartData = [];
</script>

<ResponsiveLineChart
  {chartData}
  color="#3b82f6"
  emptyMessage="No creation data available"
  gradientPrefix="created"
  minHeight={220}
/>
