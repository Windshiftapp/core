/**
 * Analytics translations for English locale
 */
export default {
  analytics: {
    title: 'Analytics',
    loading: 'Loading analytics...',
    noData: 'No data available',
    dateRange: 'Date Range',
    collection: 'Collection',
    allItems: 'All workspace items',
    datasetBasis: '{items} items across {iterations, plural, one {# iteration} other {# iterations}}',
    datasetBasisNoIterations: '{items} items, no iterations in range',
    velocity: {
      title: 'Velocity Trend',
      completed: 'Completed',
      average: 'Average (completed iterations)',
    },
    cumulativeFlow: {
      title: 'Cumulative Flow',
    },
    cycleTime: {
      title: 'Cycle Time',
      itemsAnalyzed: 'items analyzed',
      avgTotal: 'Avg Total',
      median: 'Median',
      p85: '85th %ile',
    },
    forecast: {
      title: 'Completion Forecast',
      remainingItems: 'Remaining Items',
      remainingPoints: 'Remaining Points',
      throughput: 'Throughput (per iteration)',
      avg: 'Avg',
      min: 'Min',
      max: 'Max',
      predictions: 'Predictions',
      confidence: 'Confidence',
      iterations: 'Iterations',
      estimatedDate: 'Est. Date',
      method: 'Method',
      lowDataWarning: 'Forecast is based on limited historical data. Predictions may be unreliable.',
    },
    storyPoints: 'Story Points',
    insufficientData: {
      no_items: 'This workspace has no items yet. Create items to see analytics.',
      no_iterations: 'No completed iterations found in the selected date range. Create and complete iterations to see trends.',
      no_iteration_items: 'Iterations exist in the range but have no matching items. Assign items to iterations to see velocity.',
      no_workflow: 'No workflow configured. Set up statuses for this workspace to see flow data.',
      no_history: 'Items exist but haven\'t moved through statuses yet. Transition items between statuses to build a flow diagram.',
      no_completed_items: 'No items were completed in the selected date range. Complete items to analyze cycle times.',
      few_completed_items: 'Only a few items have been completed. Cycle time statistics become more reliable with more data.',
      few_iterations: 'Less than 3 completed iterations in range. Complete more iterations to enable Monte Carlo forecasting.',
    },
  },
};
