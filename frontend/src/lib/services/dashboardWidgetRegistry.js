// Dashboard Widget Registry
// Defines widget types available on the main user homepage (distinct from the
// per-workspace widget registry in widgetRegistry.js).

export const dashboardWidgetCategories = {
  ACTIVITY: 'activity',
  WORK: 'work',
  NAVIGATION: 'navigation',
};

export const DASHBOARD_GRID_COLUMNS = 12;

export const dashboardWidgetRegistry = [
  // Activity & news
  {
    type: 'daily-briefing',
    name: 'Daily Briefing',
    description: 'AI-generated summary of what matters to you today',
    nameKey: 'dashboard.widgets.dailyBriefing.name',
    descriptionKey: 'dashboard.widgets.dailyBriefing.description',
    category: dashboardWidgetCategories.ACTIVITY,
    icon: 'Sparkles',
    defaultWidth: 12,
    minWidth: 6,
  },
  {
    type: 'your-activity',
    name: 'Your Activity',
    description: 'Items you recently viewed, edited, or commented on',
    nameKey: 'dashboard.widgets.yourActivity.name',
    descriptionKey: 'dashboard.widgets.yourActivity.description',
    category: dashboardWidgetCategories.ACTIVITY,
    icon: 'Clock',
    defaultWidth: 8,
    minWidth: 4,
  },
  {
    type: 'whats-new',
    name: "What's New",
    description: 'Latest notifications and unread updates',
    nameKey: 'dashboard.widgets.whatsNew.name',
    descriptionKey: 'dashboard.widgets.whatsNew.description',
    category: dashboardWidgetCategories.ACTIVITY,
    icon: 'Bell',
    defaultWidth: 4,
    minWidth: 3,
  },

  // Work items
  {
    type: 'personal-tasks',
    name: 'Personal Tasks',
    description: 'Items from your personal todo list',
    nameKey: 'dashboard.widgets.personalTasks.name',
    descriptionKey: 'dashboard.widgets.personalTasks.description',
    category: dashboardWidgetCategories.WORK,
    icon: 'ListChecks',
    defaultWidth: 6,
    minWidth: 3,
  },
  {
    type: 'saved-search',
    name: 'Saved Search',
    description: 'Display work items from a saved collection',
    nameKey: 'dashboard.widgets.savedSearch.name',
    descriptionKey: 'dashboard.widgets.savedSearch.description',
    category: dashboardWidgetCategories.WORK,
    icon: 'Search',
    defaultWidth: 6,
    minWidth: 4,
  },
  {
    type: 'assigned-to-me',
    name: 'Assigned to Me',
    description: 'Open items assigned to you across all workspaces',
    nameKey: 'dashboard.widgets.assignedToMe.name',
    descriptionKey: 'dashboard.widgets.assignedToMe.description',
    category: dashboardWidgetCategories.WORK,
    icon: 'CheckSquare',
    defaultWidth: 6,
    minWidth: 3,
  },
  {
    type: 'watched-items',
    name: 'Watched Items',
    description: 'Items you are following',
    nameKey: 'dashboard.widgets.watchedItems.name',
    descriptionKey: 'dashboard.widgets.watchedItems.description',
    category: dashboardWidgetCategories.WORK,
    icon: 'Eye',
    defaultWidth: 4,
    minWidth: 3,
  },
  {
    type: 'upcoming-milestones',
    name: 'Upcoming Milestones',
    description: 'Milestones with approaching target dates',
    nameKey: 'dashboard.widgets.upcomingMilestones.name',
    descriptionKey: 'dashboard.widgets.upcomingMilestones.description',
    category: dashboardWidgetCategories.WORK,
    icon: 'Target',
    defaultWidth: 12,
    minWidth: 4,
  },

  // Navigation
  {
    type: 'recent-workspaces',
    name: 'Recent Workspaces',
    description: 'Workspaces you recently visited',
    nameKey: 'dashboard.widgets.recentWorkspaces.name',
    descriptionKey: 'dashboard.widgets.recentWorkspaces.description',
    category: dashboardWidgetCategories.NAVIGATION,
    icon: 'Briefcase',
    defaultWidth: 8,
    minWidth: 3,
  },
  {
    type: 'quick-access',
    name: 'Quick Access',
    description: 'Quick links to workspaces you can reach',
    nameKey: 'dashboard.widgets.quickAccess.name',
    descriptionKey: 'dashboard.widgets.quickAccess.description',
    category: dashboardWidgetCategories.NAVIGATION,
    icon: 'Grip',
    defaultWidth: 4,
    minWidth: 3,
  },
];

export function getDashboardWidgetMetadata(type) {
  return dashboardWidgetRegistry.find((w) => w.type === type);
}

export function getDashboardWidgetsByCategory(category) {
  return dashboardWidgetRegistry.filter((w) => w.category === category);
}

export function getDashboardWidgetDefaultWidth(type) {
  const widget = getDashboardWidgetMetadata(type);
  return widget ? widget.defaultWidth : DASHBOARD_GRID_COLUMNS;
}

export function getDashboardWidgetMinWidth(type) {
  const widget = getDashboardWidgetMetadata(type);
  return widget?.minWidth ?? 3;
}

/**
 * Build the default three-section layout shown to users who have never
 * customized their dashboard (or whose saved layout is empty).
 */
export function buildDefaultDashboardLayout() {
  const sections = [
    {
      id: 'default-your-day',
      title: 'Your Day',
      subtitle: 'A quick read on what needs your attention',
      display_order: 0,
      widget_ids: ['default-daily-briefing', 'default-your-activity', 'default-whats-new'],
    },
    {
      id: 'default-work',
      title: 'Work',
      subtitle: 'Your personal list and items assigned to you',
      display_order: 1,
      widget_ids: ['default-personal-tasks', 'default-assigned-to-me'],
    },
    {
      id: 'default-workspaces',
      title: 'Workspaces',
      subtitle: 'Jump back in',
      display_order: 2,
      widget_ids: ['default-recent-workspaces', 'default-quick-access'],
    },
  ];

  const widget = (id, type, sectionId, position, width) => ({
    id,
    type,
    section_id: sectionId,
    position,
    width: width ?? getDashboardWidgetDefaultWidth(type),
    config: {},
  });

  const widgets = [
    widget('default-daily-briefing', 'daily-briefing', 'default-your-day', 0),
    widget('default-your-activity', 'your-activity', 'default-your-day', 1),
    widget('default-whats-new', 'whats-new', 'default-your-day', 2),
    widget('default-personal-tasks', 'personal-tasks', 'default-work', 0, 6),
    widget('default-assigned-to-me', 'assigned-to-me', 'default-work', 1, 6),
    widget('default-recent-workspaces', 'recent-workspaces', 'default-workspaces', 0),
    widget('default-quick-access', 'quick-access', 'default-workspaces', 1),
  ];

  return { grid_columns: DASHBOARD_GRID_COLUMNS, sections, widgets };
}
