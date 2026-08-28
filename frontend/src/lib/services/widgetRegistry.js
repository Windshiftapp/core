// Widget Registry
// Defines all available widget types with metadata for the workspace homepage

export const widgetCategories = {
  BUILT_IN: 'built-in',
  ADDITIONAL: 'additional',
};

export const WORKSPACE_WIDGET_GRID_COLUMNS = 3;

export const widgetRegistry = [
  // Built-in widgets (core functionality)
  {
    type: 'stats',
    name: 'Statistics Overview',
    description: 'Collections and item counts by status category',
    nameKey: 'workspaceDashboard.widgets.stats.name',
    descriptionKey: 'workspaceDashboard.widgets.stats.description',
    category: widgetCategories.BUILT_IN,
    icon: 'BarChart3',
    minWidth: 1,
    defaultWidth: 3,
    maxWidth: 3,
  },
  {
    type: 'completion-chart',
    name: 'Completion Chart',
    description: 'Items completed over last 4 weeks',
    nameKey: 'workspaceDashboard.widgets.completionChart.name',
    descriptionKey: 'workspaceDashboard.widgets.completionChart.description',
    category: widgetCategories.BUILT_IN,
    icon: 'TrendingUp',
    minWidth: 1,
    defaultWidth: 2,
    maxWidth: 3,
  },
  {
    type: 'created-chart',
    name: 'Creation Chart',
    description: 'Items created over last 7 days',
    nameKey: 'workspaceDashboard.widgets.createdChart.name',
    descriptionKey: 'workspaceDashboard.widgets.createdChart.description',
    category: widgetCategories.BUILT_IN,
    icon: 'Activity',
    minWidth: 1,
    defaultWidth: 1,
    maxWidth: 3,
  },
  {
    type: 'milestone-progress',
    name: 'Milestone Progress',
    description: 'Active milestones and their progress',
    nameKey: 'workspaceDashboard.widgets.milestoneProgress.name',
    descriptionKey: 'workspaceDashboard.widgets.milestoneProgress.description',
    category: widgetCategories.BUILT_IN,
    icon: 'Flag',
    minWidth: 1,
    defaultWidth: 3,
    maxWidth: 3,
  },

  // Additional widgets (list widgets)
  {
    type: 'recent-items',
    name: 'Recent Items',
    description: 'Recently updated items in this workspace',
    nameKey: 'workspaceDashboard.widgets.recentItems.name',
    descriptionKey: 'workspaceDashboard.widgets.recentItems.description',
    category: widgetCategories.ADDITIONAL,
    icon: 'Clock',
    minWidth: 1,
    defaultWidth: 2,
    maxWidth: 3,
  },
  {
    type: 'my-tasks',
    name: 'My Tasks',
    description: 'Items assigned to you',
    nameKey: 'workspaceDashboard.widgets.myTasks.name',
    descriptionKey: 'workspaceDashboard.widgets.myTasks.description',
    category: widgetCategories.ADDITIONAL,
    icon: 'User',
    minWidth: 1,
    defaultWidth: 2,
    maxWidth: 3,
  },
  {
    type: 'saved-search',
    name: 'Saved Search',
    description: 'Display work items from a saved collection',
    nameKey: 'workspaceDashboard.widgets.savedSearch.name',
    descriptionKey: 'workspaceDashboard.widgets.savedSearch.description',
    category: widgetCategories.ADDITIONAL,
    icon: 'Search',
    minWidth: 1,
    defaultWidth: 2,
    maxWidth: 3,
  },
  {
    type: 'overdue-items',
    name: 'Overdue Items',
    description: 'Items past their due date',
    nameKey: 'workspaceDashboard.widgets.overdueItems.name',
    descriptionKey: 'workspaceDashboard.widgets.overdueItems.description',
    category: widgetCategories.ADDITIONAL,
    icon: 'AlertCircle',
    minWidth: 1,
    defaultWidth: 2,
    maxWidth: 3,
  },

  // Additional widgets (calendar/timeline widgets)
  {
    type: 'upcoming-deadlines',
    name: 'Upcoming Deadlines',
    description: 'Items with approaching due dates',
    nameKey: 'workspaceDashboard.widgets.upcomingDeadlines.name',
    descriptionKey: 'workspaceDashboard.widgets.upcomingDeadlines.description',
    category: widgetCategories.ADDITIONAL,
    icon: 'Calendar',
    minWidth: 1,
    defaultWidth: 2,
    maxWidth: 3,
  },
  {
    type: 'iteration-timeline',
    name: 'Iteration Timeline',
    description: 'Current and upcoming iteration schedule',
    nameKey: 'workspaceDashboard.widgets.iterationTimeline.name',
    descriptionKey: 'workspaceDashboard.widgets.iterationTimeline.description',
    category: widgetCategories.ADDITIONAL,
    icon: 'CalendarDays',
    minWidth: 1,
    defaultWidth: 3,
    maxWidth: 3,
  },

  // Additional widgets (test management)
  {
    type: 'test-coverage',
    name: 'Test Coverage',
    description: 'Requirements covered by test cases',
    nameKey: 'workspaceDashboard.widgets.testCoverage.name',
    descriptionKey: 'workspaceDashboard.widgets.testCoverage.description',
    category: widgetCategories.ADDITIONAL,
    icon: 'ShieldCheck',
    minWidth: 1,
    defaultWidth: 2,
    maxWidth: 3,
  },
];

// Helper functions

/**
 * Get widget metadata by type
 * @param {string} type - Widget type
 * @returns {object|undefined} Widget metadata
 */
export function getWidgetMetadata(type) {
  return widgetRegistry.find((widget) => widget.type === type);
}

/**
 * Get widgets by category
 * @param {string} category - Category name
 * @returns {Array} Filtered widgets
 */
export function getWidgetsByCategory(category) {
  return widgetRegistry.filter((widget) => widget.category === category);
}

/**
 * Get default width for a widget type
 * @param {string} type - Widget type
 * @returns {number} Default width (1-3)
 */
export function getDefaultWidth(type) {
  const widget = getWidgetMetadata(type);
  return widget ? widget.defaultWidth : WORKSPACE_WIDGET_GRID_COLUMNS;
}

/**
 * Get the minimum width for a widget type.
 * @param {string} type - Widget type
 * @returns {number} Minimum width (1-3)
 */
export function getWidgetMinWidth(type) {
  return getWidgetMetadata(type)?.minWidth ?? 1;
}

/**
 * Get the maximum width for a widget type.
 * @param {string} type - Widget type
 * @returns {number} Maximum width (1-3)
 */
export function getWidgetMaxWidth(type) {
  return getWidgetMetadata(type)?.maxWidth ?? WORKSPACE_WIDGET_GRID_COLUMNS;
}

/**
 * Keep a widget width within its registry bounds.
 * @param {string} type - Widget type
 * @param {unknown} width - Requested width
 * @returns {number} Valid integer width
 */
export function clampWidgetWidth(type, width) {
  const minWidth = getWidgetMinWidth(type);
  const maxWidth = getWidgetMaxWidth(type);
  const numericWidth = Number(width);
  const resolvedWidth = Number.isFinite(numericWidth)
    ? Math.round(numericWidth)
    : getDefaultWidth(type);
  return Math.min(maxWidth, Math.max(minWidth, resolvedWidth));
}

/**
 * Check if a widget type exists
 * @param {string} type - Widget type
 * @returns {boolean} True if widget exists
 */
export function isValidWidgetType(type) {
  return widgetRegistry.some((widget) => widget.type === type);
}
