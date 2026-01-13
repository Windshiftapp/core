// Widget Registry
// Defines all available widget types with metadata for the workspace homepage

export const widgetCategories = {
  BUILT_IN: 'built-in',
  ADDITIONAL: 'additional'
};

export const widgetRegistry = [
  // Built-in widgets (core functionality)
  {
    type: 'stats',
    name: 'Statistics Overview',
    description: 'Collections and item counts by status category',
    category: widgetCategories.BUILT_IN,
    icon: 'BarChart3',
    defaultWidth: 3,
  },
  {
    type: 'completion-chart',
    name: 'Completion Chart',
    description: 'Items completed over last 4 weeks',
    category: widgetCategories.BUILT_IN,
    icon: 'TrendingUp',
    defaultWidth: 2,
  },
  {
    type: 'created-chart',
    name: 'Creation Chart',
    description: 'Items created over last 7 days',
    category: widgetCategories.BUILT_IN,
    icon: 'Activity',
    defaultWidth: 1,
  },
  {
    type: 'milestone-progress',
    name: 'Milestone Progress',
    description: 'Active milestones and their progress',
    category: widgetCategories.BUILT_IN,
    icon: 'Flag',
    defaultWidth: 3,
  },

  // Additional widgets (list widgets)
  {
    type: 'recent-items',
    name: 'Recent Items',
    description: 'Recently updated items in this workspace',
    category: widgetCategories.ADDITIONAL,
    icon: 'Clock',
    defaultWidth: 2,
  },
  {
    type: 'my-tasks',
    name: 'My Tasks',
    description: 'Items assigned to you',
    category: widgetCategories.ADDITIONAL,
    icon: 'User',
    defaultWidth: 2,
  },
  {
    type: 'overdue-items',
    name: 'Overdue Items',
    description: 'Items past their due date',
    category: widgetCategories.ADDITIONAL,
    icon: 'AlertCircle',
    defaultWidth: 2,
  },

  // Additional widgets (filter/search widgets)
  {
    type: 'item-filter',
    name: 'Item Filter',
    description: 'Custom filter for items',
    category: widgetCategories.ADDITIONAL,
    icon: 'Filter',
    defaultWidth: 3,
  },
  {
    type: 'saved-search',
    name: 'Saved Search',
    description: 'Execute and display a saved search',
    category: widgetCategories.ADDITIONAL,
    icon: 'Search',
    defaultWidth: 3,
  },

  // Additional widgets (calendar/timeline widgets)
  {
    type: 'upcoming-deadlines',
    name: 'Upcoming Deadlines',
    description: 'Items with approaching due dates',
    category: widgetCategories.ADDITIONAL,
    icon: 'Calendar',
    defaultWidth: 2,
  },
  {
    type: 'sprint-timeline',
    name: 'Sprint Timeline',
    description: 'Current and upcoming sprint schedule',
    category: widgetCategories.ADDITIONAL,
    icon: 'CalendarDays',
    defaultWidth: 3,
  },
];

// Helper functions

/**
 * Get widget metadata by type
 * @param {string} type - Widget type
 * @returns {object|undefined} Widget metadata
 */
export function getWidgetMetadata(type) {
  return widgetRegistry.find(widget => widget.type === type);
}

/**
 * Get widgets by category
 * @param {string} category - Category name
 * @returns {Array} Filtered widgets
 */
export function getWidgetsByCategory(category) {
  return widgetRegistry.filter(widget => widget.category === category);
}

/**
 * Get default width for a widget type
 * @param {string} type - Widget type
 * @returns {number} Default width (1-3)
 */
export function getDefaultWidth(type) {
  const widget = getWidgetMetadata(type);
  return widget ? widget.defaultWidth : 3;
}

/**
 * Check if a widget type exists
 * @param {string} type - Widget type
 * @returns {boolean} True if widget exists
 */
export function isValidWidgetType(type) {
  return widgetRegistry.some(widget => widget.type === type);
}
