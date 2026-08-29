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
    nameKey: 'dashboard.widgetCatalog.dailyBriefing.name',
    descriptionKey: 'dashboard.widgetCatalog.dailyBriefing.description',
    category: dashboardWidgetCategories.ACTIVITY,
    icon: 'Sparkles',
    defaultWidth: 12,
    minWidth: 6,
  },
  {
    type: 'your-activity',
    nameKey: 'dashboard.widgetCatalog.yourActivity.name',
    descriptionKey: 'dashboard.widgetCatalog.yourActivity.description',
    category: dashboardWidgetCategories.ACTIVITY,
    icon: 'Clock',
    defaultWidth: 8,
    minWidth: 4,
  },
  {
    type: 'whats-new',
    nameKey: 'dashboard.widgetCatalog.whatsNew.name',
    descriptionKey: 'dashboard.widgetCatalog.whatsNew.description',
    category: dashboardWidgetCategories.ACTIVITY,
    icon: 'Bell',
    defaultWidth: 4,
    minWidth: 3,
  },

  // Work items
  {
    type: 'personal-tasks',
    nameKey: 'dashboard.widgetCatalog.personalTasks.name',
    descriptionKey: 'dashboard.widgetCatalog.personalTasks.description',
    category: dashboardWidgetCategories.WORK,
    icon: 'ListChecks',
    defaultWidth: 6,
    minWidth: 3,
  },
  {
    type: 'saved-search',
    nameKey: 'dashboard.widgetCatalog.savedSearch.name',
    descriptionKey: 'dashboard.widgetCatalog.savedSearch.description',
    category: dashboardWidgetCategories.WORK,
    icon: 'Search',
    defaultWidth: 6,
    minWidth: 4,
  },
  {
    type: 'assigned-to-me',
    nameKey: 'dashboard.widgetCatalog.assignedToMe.name',
    descriptionKey: 'dashboard.widgetCatalog.assignedToMe.description',
    category: dashboardWidgetCategories.WORK,
    icon: 'CheckSquare',
    defaultWidth: 6,
    minWidth: 3,
  },
  {
    type: 'watched-items',
    nameKey: 'dashboard.widgetCatalog.watchedItems.name',
    descriptionKey: 'dashboard.widgetCatalog.watchedItems.description',
    category: dashboardWidgetCategories.WORK,
    icon: 'Eye',
    defaultWidth: 4,
    minWidth: 3,
  },
  {
    type: 'upcoming-milestones',
    nameKey: 'dashboard.widgetCatalog.upcomingMilestones.name',
    descriptionKey: 'dashboard.widgetCatalog.upcomingMilestones.description',
    category: dashboardWidgetCategories.WORK,
    icon: 'Target',
    defaultWidth: 12,
    minWidth: 4,
  },

  // Navigation
  {
    type: 'recent-workspaces',
    nameKey: 'dashboard.widgetCatalog.recentWorkspaces.name',
    descriptionKey: 'dashboard.widgetCatalog.recentWorkspaces.description',
    category: dashboardWidgetCategories.NAVIGATION,
    icon: 'Briefcase',
    defaultWidth: 8,
    minWidth: 3,
  },
  {
    type: 'quick-access',
    nameKey: 'dashboard.widgetCatalog.quickAccess.name',
    descriptionKey: 'dashboard.widgetCatalog.quickAccess.description',
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

export const defaultDashboardSections = {
  'default-your-day': {
    title: 'Your Day',
    subtitle: 'A quick read on what needs your attention',
    titleKey: 'dashboard.sections.yourDay.title',
    subtitleKey: 'dashboard.sections.yourDay.subtitle',
  },
  'default-work': {
    title: 'Work',
    subtitle: 'Your personal list and items assigned to you',
    titleKey: 'dashboard.sections.work.title',
    subtitleKey: 'dashboard.sections.work.subtitle',
  },
  'default-workspaces': {
    title: 'Workspaces',
    subtitle: 'Jump back in',
    titleKey: 'dashboard.sections.workspaces.title',
    subtitleKey: 'dashboard.sections.workspaces.subtitle',
  },
};

/**
 * Translate untouched built-in section headings while preserving user edits.
 */
export function getDashboardSectionDisplay(section, translate) {
  const defaults = defaultDashboardSections[section.id];
  if (!defaults) return { title: section.title, subtitle: section.subtitle };
  return {
    title: section.title === defaults.title ? translate(defaults.titleKey) : section.title,
    subtitle:
      section.subtitle === defaults.subtitle ? translate(defaults.subtitleKey) : section.subtitle,
  };
}

/**
 * Preserve canonical default values when a localized section editor is saved
 * without changing its translated display text.
 */
export function getDashboardSectionSaveValues(section, draft, translate) {
  const display = getDashboardSectionDisplay(section, translate);
  return {
    title: draft.title === display.title ? section.title : draft.title,
    subtitle: draft.subtitle === (display.subtitle || '') ? section.subtitle : draft.subtitle,
  };
}

/**
 * Build the default three-section layout shown to users who have never
 * customized their dashboard (or whose saved layout is empty).
 */
export function buildDefaultDashboardLayout() {
  const section = (id, displayOrder, widgetIds) => ({
    id,
    title: defaultDashboardSections[id].title,
    subtitle: defaultDashboardSections[id].subtitle,
    display_order: displayOrder,
    widget_ids: widgetIds,
  });

  const sections = [
    section('default-your-day', 0, [
      'default-daily-briefing',
      'default-your-activity',
      'default-whats-new',
    ]),
    section('default-work', 1, ['default-personal-tasks', 'default-assigned-to-me']),
    section('default-workspaces', 2, ['default-recent-workspaces', 'default-quick-access']),
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
