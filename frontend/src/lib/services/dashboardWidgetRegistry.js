// Dashboard Widget Registry
// Defines widget types available on the main user homepage (distinct from the
// per-workspace widget registry in widgetRegistry.js).

export const dashboardWidgetCategories = {
  ACTIVITY: 'activity',
  WORK: 'work',
  NAVIGATION: 'navigation',
};

export const dashboardWidgetRegistry = [
  // Activity & news
  {
    type: 'daily-briefing',
    name: 'Daily Briefing',
    description: 'AI-generated summary of what matters to you today',
    category: dashboardWidgetCategories.ACTIVITY,
    icon: 'Sparkles',
    defaultWidth: 3,
  },
  {
    type: 'your-activity',
    name: 'Your Activity',
    description: 'Items you recently viewed, edited, or commented on',
    category: dashboardWidgetCategories.ACTIVITY,
    icon: 'Clock',
    defaultWidth: 2,
  },
  {
    type: 'whats-new',
    name: "What's New",
    description: 'Latest notifications and unread updates',
    category: dashboardWidgetCategories.ACTIVITY,
    icon: 'Bell',
    defaultWidth: 1,
  },

  // Work items
  {
    type: 'personal-tasks',
    name: 'Personal Tasks',
    description: 'Items from your personal todo list',
    category: dashboardWidgetCategories.WORK,
    icon: 'ListChecks',
    defaultWidth: 1,
  },
  {
    type: 'assigned-to-me',
    name: 'Assigned to Me',
    description: 'Open items assigned to you across all workspaces',
    category: dashboardWidgetCategories.WORK,
    icon: 'CheckSquare',
    defaultWidth: 2,
  },
  {
    type: 'watched-items',
    name: 'Watched Items',
    description: 'Items you are following',
    category: dashboardWidgetCategories.WORK,
    icon: 'Eye',
    defaultWidth: 1,
  },
  {
    type: 'upcoming-milestones',
    name: 'Upcoming Milestones',
    description: 'Milestones with approaching target dates',
    category: dashboardWidgetCategories.WORK,
    icon: 'Target',
    defaultWidth: 3,
  },

  // Navigation
  {
    type: 'recent-workspaces',
    name: 'Recent Workspaces',
    description: 'Workspaces you recently visited',
    category: dashboardWidgetCategories.NAVIGATION,
    icon: 'Briefcase',
    defaultWidth: 2,
  },
  {
    type: 'quick-access',
    name: 'Quick Access',
    description: 'Quick links to workspaces you can reach',
    category: dashboardWidgetCategories.NAVIGATION,
    icon: 'Grip',
    defaultWidth: 1,
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
  return widget ? widget.defaultWidth : 3;
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
      subtitle: 'Items and milestones assigned to or watched by you',
      display_order: 1,
      widget_ids: [
        'default-personal-tasks',
        'default-assigned-to-me',
        'default-upcoming-milestones',
        'default-watched-items',
      ],
    },
    {
      id: 'default-workspaces',
      title: 'Workspaces',
      subtitle: 'Jump back in',
      display_order: 2,
      widget_ids: ['default-recent-workspaces', 'default-quick-access'],
    },
  ];

  const widget = (id, type, sectionId, position) => ({
    id,
    type,
    section_id: sectionId,
    position,
    width: getDashboardWidgetDefaultWidth(type),
    config: {},
  });

  const widgets = [
    widget('default-daily-briefing', 'daily-briefing', 'default-your-day', 0),
    widget('default-your-activity', 'your-activity', 'default-your-day', 1),
    widget('default-whats-new', 'whats-new', 'default-your-day', 2),
    widget('default-personal-tasks', 'personal-tasks', 'default-work', 0),
    widget('default-assigned-to-me', 'assigned-to-me', 'default-work', 1),
    widget('default-upcoming-milestones', 'upcoming-milestones', 'default-work', 2),
    widget('default-watched-items', 'watched-items', 'default-work', 3),
    widget('default-recent-workspaces', 'recent-workspaces', 'default-workspaces', 0),
    widget('default-quick-access', 'quick-access', 'default-workspaces', 1),
  ];

  return { sections, widgets };
}
