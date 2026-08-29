export default {
  dashboard: {
    salutation: {
      withName: '{salutation}, {name}!',
      withoutName: '{salutation}!',
    },
    sections: {
      yourDay: {
        title: 'Your Day',
        subtitle: 'A quick read on what needs your attention',
      },
      work: {
        title: 'Work',
        subtitle: 'Your personal list and items assigned to you',
      },
      workspaces: {
        title: 'Workspaces',
        subtitle: 'Jump back in',
      },
    },
    widgetCatalog: {
      dailyBriefing: {
        name: 'Daily Briefing',
        description: 'AI-generated summary of what matters to you today',
      },
      yourActivity: {
        name: 'Your Activity',
        description: 'Items you recently viewed, edited, or commented on',
      },
      whatsNew: {
        name: "What's New",
        description: 'Latest notifications and unread updates',
      },
      personalTasks: {
        name: 'Personal Tasks',
        description: 'Items from your personal todo list',
      },
      savedSearch: {
        name: 'Saved Search',
        description: 'Display work items from a saved collection',
      },
      assignedToMe: {
        name: 'Assigned to Me',
        description: 'Open items assigned to you across all workspaces',
      },
      watchedItems: {
        name: 'Watched Items',
        description: 'Items you are following',
      },
      upcomingMilestones: {
        name: 'Upcoming Milestones',
        description: 'Milestones with approaching target dates',
      },
      recentWorkspaces: {
        name: 'Recent Workspaces',
        description: 'Workspaces you recently visited',
      },
      quickAccess: {
        name: 'Quick Access',
        description: 'Quick links to workspaces you can reach',
      },
    },
    customization: {
      widgets: 'Widgets',
      activity: {
        name: 'Activity',
        description: 'Briefings, activity streams, and notifications',
      },
      work: {
        name: 'Work',
        description: 'Items, milestones, and things assigned to you',
      },
      navigation: {
        name: 'Navigation',
        description: 'Quick access to workspaces',
      },
      tipLabel: 'Tip',
      tip: 'Drag widgets from here into any section on your dashboard.',
    },
    editor: {
      newSection: 'New Section',
      deleteSectionConfirm: 'Delete this section? All widgets in this section will be removed.',
      doneEditing: 'Done Editing',
      customize: 'Customize',
      editModeDescription: 'Edit mode: add, rename, reorder, or delete sections and widgets',
      addSection: 'Add Section',
      sectionLabel: 'Dashboard section',
      sectionTitlePlaceholder: 'Section title',
      sectionSubtitlePlaceholder: 'Subtitle (optional)',
      renameSection: 'Rename section',
      deleteSection: 'Delete section',
      unknownWidgetType: 'Unknown widget type: {type}',
      noWidgets: 'No widgets in this section yet',
      addWidgetsHint: 'Select Customize to add widgets',
      noSections: 'No sections configured',
      addSectionsHint: 'Select Edit to add sections to your dashboard',
    },
    states: {
      assignedLoadError: "Couldn't load your assigned items",
      assignedEmpty: 'Nothing assigned to you right now',
      personalTasksLoadError: "Couldn't load your personal tasks",
      personalTasksEmpty: 'Your personal todo list is empty',
      dailyBriefingUnavailable: "Your daily briefing isn't available right now. It relies on an AI integration. If you've just set one up, check back in a bit.",
      updatedAt: 'Updated {time}',
      priorityLabel: 'Priority: {priority}',
      noWorkspaces: 'No workspaces yet',
      createWorkspace: 'Create one',
      workspaceAvatarAlt: '{name} avatar',
      visited: 'visited {time}',
      noUpcomingMilestones: 'No upcoming milestones',
      milestoneProgress: '{done} of {total} done',
      daysOverdue: '{days} days overdue',
      daysLeft: '{days} days left',
      watchedItemsEmpty: "You aren't watching any items",
      workspaceWithId: 'Workspace {id}',
    },
  },
};
