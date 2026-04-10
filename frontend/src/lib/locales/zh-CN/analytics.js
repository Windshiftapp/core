/**
 * Analytics translations for Simplified Chinese locale locale
 */
export default {
  analytics: {
    title: '分析',
    loading: '正在加载分析...',
    noData: '暂无数据',
    dateRange: '日期范围',
    collection: '集合',
    allItems: '所有工作区项目',
    datasetBasis: '{items} 个项目，跨 {iterations, plural, one {# 个迭代} other {# 个迭代}}',
    datasetBasisNoIterations: '{items} 个项目，范围内无迭代',
    velocity: {
      title: '速率趋势',
      completed: '已完成',
      average: '平均值（已完成迭代）',
    },
    cumulativeFlow: {
      title: '累积流量',
    },
    cycleTime: {
      title: '周期时间',
      itemsAnalyzed: '已分析项目',
      avgTotal: '平均总计',
      median: '中位数',
      p85: '第85百分位',
    },
    forecast: {
      title: '完成预测',
      remainingItems: '剩余项目',
      remainingPoints: '剩余点数',
      throughput: '吞吐量（每迭代）',
      avg: '平均',
      min: '最小',
      max: '最大',
      predictions: '预测',
      confidence: '置信度',
      iterations: '迭代',
      estimatedDate: '预计日期',
      method: '方法',
      lowDataWarning: '预测基于有限的历史数据，结果可能不可靠。',
    },
    storyPoints: '故事点',
    insufficientData: {
      no_items: '此工作区暂无项目。创建项目以查看分析。',
      no_iterations: '所选日期范围内未找到已完成的迭代。创建并完成迭代以查看趋势。',
      no_iteration_items: '范围内存在迭代但没有匹配的项目。将项目分配到迭代以查看速率。',
      no_workflow: '未配置工作流。为此工作区设置状态以查看流数据。',
      no_history: '存在项目但尚未经过状态变更。在状态之间移动项目以构建流图。',
      no_completed_items: '所选日期范围内没有完成的项目。完成项目以分析周期时间。',
      few_completed_items: '仅完成了少量项目。更多数据将使周期时间统计更可靠。',
      few_iterations: '范围内已完成的迭代少于3个。完成更多迭代以启用蒙特卡洛预测。',
    },
  },
};
