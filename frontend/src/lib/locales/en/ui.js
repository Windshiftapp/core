/**
 * UI-related translations for English locale
 * Contains: pickers, editors, dialogs, components, aria, layout, widgets, footer
 */

export default {
  pickers: {
    // General
    select: 'Select',
    search: 'Search',
    options: 'Options',
    clearSelection: 'Clear selection',
    noResultsFor: 'No results for "{query}"',
    createItem: 'Create "{value}"',
    noItemsFound: 'No items found',
    noItemsAvailable: 'No items available',

    // Asset Picker
    selectAsset: 'Select asset',
    noTag: 'No tag',

    // User/Assignee Picker
    selectUser: 'Select user',
    searchUsers: 'Search users...',
    users: 'Users',
    noUsersFound: 'No users found',
    noUsersAvailable: 'No users available',
    assignTo: 'Assign to',
    unassigned: 'Unassigned',
    user: 'User',
    group: 'Group',
    searchUser: 'Search user...',
    searchGroup: 'Search group...',

    // Group Picker
    selectGroup: 'Select group',

    // Category Picker
    selectCategories: 'Select categories',
    removeCategory: 'Remove category',
    categoriesSelected: '{count} categories selected',
    searchCategories: 'Search categories...',
    noCategoriesFound: 'No categories found',

    // Collection Picker
    selectCollections: 'Select collections',

    // Workspace Picker
    selectWorkspaces: 'Select workspaces',
    searchWorkspaces: 'Search workspaces...',
    noWorkspacesFound: 'No workspaces found',

    // Configuration Set Picker
    selectConfigurationSet: 'Select configuration set',
    searchConfigurationSets: 'Search configuration sets...',
    configurationSets: 'Configuration sets',
    defaultConfiguration: 'Default Configuration',
    defaultConfigurationDescription: 'Uses the workspace default settings',
    noConfigurationSetsFound: 'No configuration sets found',

    // Configuration Set Entity Picker
    entityAlreadyAssigned: '{label} is already assigned',
    itemType: 'Item Type',
    priorities: 'Priorities',
    itemTypes: 'Item Types',
    level: 'Level {level}',
    assigned: 'Assigned',
    noEntitiesAssigned: 'No {label} assigned',
    available: 'Available',
    noEntitiesMatchSearch: 'No {label} match your search',
    allEntitiesAssigned: 'All {label} are assigned',
    inConfigSet: 'In config set',
    searchEntities: 'Search {label}...',

    // Field Selector
    selectField: 'Select field',
    searchFields: 'Search fields...',
    noFieldsFound: 'No fields found',
    customFields: 'Custom Fields',
    custom: 'Custom',
    customFieldDesc: 'Custom field',
    fieldTypes: {
      text: 'Text',
      number: 'Number',
      date: 'Date',
      select: 'Select',
      multiselect: 'Multi-select',
      checkbox: 'Checkbox',
      url: 'URL',
      email: 'Email',
      phone: 'Phone',
      textarea: 'Text Area',
      user: 'User',
      rating: 'Rating'
    },
    fieldCategories: {
      basic: 'Basic Fields',
      dates: 'Date Fields',
      people: 'People',
      workflow: 'Workflow',
      custom: 'Custom Fields'
    },
    fields: {
      title: { name: 'Title', description: 'Item title' },
      description: { name: 'Description', description: 'Item description' },
      status: { name: 'Status', description: 'Current status' },
      priority: { name: 'Priority', description: 'Priority level' },
      type: { name: 'Type', description: 'Item type' },
      assignee: { name: 'Assignee', description: 'Assigned user' },
      reporter: { name: 'Reporter', description: 'Who reported the item' },
      createdAt: { name: 'Created At', description: 'When the item was created' },
      updatedAt: { name: 'Updated At', description: 'When the item was last updated' },
      dueDate: { name: 'Due Date', description: 'When the item is due' },
      startDate: { name: 'Start Date', description: 'When work begins' },
      estimate: { name: 'Estimate', description: 'Estimated effort' },
      labels: { name: 'Labels', description: 'Item labels' },
      sprint: { name: 'Sprint', description: 'Associated sprint' },
      milestone: { name: 'Milestone', description: 'Target milestone' },
      parent: { name: 'Parent', description: 'Parent item' },
      children: { name: 'Children', description: 'Child items' },
      links: { name: 'Links', description: 'Related items' },
      attachments: { name: 'Attachments', description: 'File attachments' },
      comments: { name: 'Comments', description: 'Discussion comments' },
      watchers: { name: 'Watchers', description: 'Users watching this item' }
    },

    // Icon Selector
    iconAndColor: 'Icon & Color',
    searchIcons: 'Search icons...',
    icons: 'Icons',
    colors: 'Colors',
    icon: 'Icon',
    color: 'Color',

    // Label Combobox
    allLabels: 'All labels',
    selectLabels: 'Select labels',
    noLabelsFoundFor: 'No labels found for "{query}"',

    // Mention Picker
    mentionUsers: 'Mention users',
    searching: 'Searching...',
    noNotificationPersonalTask: 'Personal tasks do not send notifications',

    // Milestone Combobox
    selectMilestone: 'Select milestone',
    noMilestone: 'No milestone',
    milestones: 'Milestones',
    noMilestonesFound: 'No milestones found',

    // Priority Picker
    selectPriority: 'Select priority',
    noPriority: 'No priority',
    loadingPriorities: 'Loading priorities...',
    noPrioritiesConfigured: 'No priorities configured',

    // Repository Selector
    linkRepositories: 'Link Repositories',
    selectRepositoriesFrom: 'Select repositories from {provider}',
    searchRepositories: 'Search repositories...',
    loadingRepositories: 'Loading repositories...',
    noRepositoriesMatchSearch: 'No repositories match your search',
    noRepositoriesAvailable: 'No repositories available',
    alreadyLinked: 'Already linked',
    linkSelected: 'Link Selected',
    linking: 'Linking...',
    repositoriesSelected: '{count} selected',

    // Role Picker
    selectRole: 'Select role',

    // Screen Picker
    selectScreen: 'Select screen',

    // Test Case Picker
    searchTestCases: 'Search test cases...',

    // Workflow Picker
    selectWorkflow: 'Select workflow'
  },

  editors: {
    enterText: 'Enter text...',
    selectDate: 'Select date...',
    clickToChangeColor: 'Click to change color',
    saveEnter: 'Save (Enter)',
    cancelEscape: 'Cancel (Escape)',
    availableFields: 'Available Fields',
    selectedFields: 'Selected Fields',
    dragFieldsToAdd: 'Drag fields to add them',
    dragToReorderOrDrop: 'Drag to reorder or drop fields here',
    dropFieldsHere: 'Drop fields here to configure',
    noFieldsMatchSearch: 'No fields match your search',
    noFieldsAvailable: 'No fields available',
    allFieldsAdded: 'All available fields have been added',
    bold: 'Bold (Ctrl+B)',
    italic: 'Italic (Ctrl+I)',
    strikethrough: 'Strikethrough',
    inlineCode: 'Inline Code',
    bulletList: 'Bullet List',
    numberedList: 'Numbered List',
    insertImage: 'Insert Image',
    userNotFound: 'User not found'
  },

  dialogs: {
    cancel: 'Cancel',
    confirm: 'Confirm',
    save: 'Save',
    close: 'Close',
    delete: 'Delete',
    update: 'Update',
    // Confirmation messages for confirm() dialogs
    confirmations: {
      deleteItem: 'Are you sure you want to delete "{name}"? This cannot be undone.',
      deleteSection: 'Are you sure you want to delete this section?',
      discardChanges: 'You have unsaved changes. Are you sure you want to cancel?',
      dismissAllNotifications: 'Are you sure you want to dismiss all notifications? This cannot be undone.',
      removeAvatar: 'Are you sure you want to remove your profile picture?',
      revokeCalendarFeed: 'Are you sure you want to revoke your calendar feed URL? Any calendars using this URL will stop syncing.',
      deleteTheme: 'Are you sure you want to delete this theme? This cannot be undone.',
      resetBoardConfig: 'Are you sure you want to reset to default board configuration? This will delete your custom configuration.',
      deleteCustomField: 'Are you sure you want to delete the custom field "{name}"? This will remove it from all projects.',
      deleteLinkType: 'Are you sure you want to delete this link type? This will also remove all links of this type.',
      deleteAsset: 'Are you sure you want to delete this asset?',
      deleteAssetSet: 'Are you sure you want to delete this asset set? This will delete all assets, types, and categories within it.',
      deleteAssetType: 'Are you sure you want to delete this asset type? Assets using this type will no longer have a type assigned.',
      deleteCategory: 'Are you sure you want to delete this category? Child categories will be moved to the parent.',
      revokeRole: 'Are you sure you want to revoke this role?',
      quitApplication: 'Are you sure you want to quit the application? The server will shut down.',
      deleteConnection: 'Are you sure you want to delete this connection? This action cannot be undone.',
      deleteWidget: 'Delete this section? All widgets in this section will be removed.',
      deleteScreen: 'Are you sure you want to delete screen "{name}"? This will affect all workspaces using this screen.'
    },
    // Alert messages for alert() dialogs
    alerts: {
      nameRequired: 'Name is required',
      pleaseSelectImage: 'Please select an image file',
      timerAlreadyRunning: 'A timer is already running. Please stop it before starting a new one.',
      noTimerRunning: 'No timer is currently running.',
      timerSyncing: 'Timer is currently syncing. Please wait and try again.',
      startTimerFromItem: 'Please start a timer from within a work item to provide context.',
      cannotDeleteDefaultScreen: 'Cannot delete the default screen. This screen is required for workspaces without a configuration set.',
      applicationShuttingDown: 'Application is shutting down...',
      pdfExportComingSoon: 'PDF export coming soon for time-block view',
      configUpdatedSuccess: 'Configuration set updated successfully. All work items are already using statuses from the new workflow.',
      failedToSave: 'Failed to save: {error}',
      failedToDelete: 'Failed to delete: {error}',
      failedToUpdate: 'Failed to update: {error}',
      failedToLoad: 'Failed to load: {error}',
      failedToCreate: 'Failed to create: {error}',
      failedToUpload: 'Failed to upload: {error}',
      failedToGeneratePdf: 'Failed to generate PDF. Please try again.',
      failedToApplyConfig: 'Failed to apply configuration change: {error}',
      failedToAddManager: 'Failed to add manager: {error}',
      failedToRemoveManager: 'Failed to remove manager: {error}',
      failedToSaveWorkspace: 'Failed to save project. Please check your input and try again.',
      failedToResetConfig: 'Failed to reset configuration: {error}',
      failedToToggleStatus: 'Failed to toggle link type status: {error}',
      failedToAssignRole: 'Failed to assign role: {error}',
      failedToRevokeRole: 'Failed to revoke role: {error}',
      failedToUpdateRole: 'Failed to update everyone role: {error}',
      failedToLoadFields: 'Failed to load fields: {error}',
      failedToSaveFields: 'Failed to save field assignments: {error}',
      errorAddingTestCase: 'Error adding test case: {error}',
      failedToCreateLabel: 'Failed to create label: {error}',
      failedToSaveLayout: 'Failed to save layout changes',
      statusInUseByTransitions: 'Cannot delete "{name}" because it is being used in {count} workflow transition(s). To delete this status, go to Workflow Management, remove all transitions that use this status, then try deleting the status again.'
    }
  },

  components: {
    // Avatar component
    avatar: {
      defaultAlt: 'Avatar'
    },

    // DataTable component
    dataTable: {
      showingRange: 'Showing {start}–{end} of {total}'
    },

    // Diagram components
    diagram: {
      loading: 'Loading diagrams...',
      loadError: 'Failed to load diagrams',
      deleteError: 'Failed to delete diagram',
      confirmDelete: 'Are you sure you want to delete this diagram?',
      edit: 'Edit diagram',
      untitled: 'Untitled Diagram',
      namePlaceholder: 'Diagram name',
      nameRequired: 'Please enter a diagram name',
      saveError: 'Failed to save diagram',
      unsavedChanges: 'Unsaved changes',
      unsavedChangesConfirm: 'You have unsaved changes. Are you sure you want to close?'
    },

    // ErrorState component
    errorState: {
      title: 'Something went wrong'
    },

    // Pagination component
    pagination: {
      showingRange: 'Showing {start}-{end} of {total}',
      limitedTo: 'limited to {max} items',
      itemsPerPage: 'Items per page:',
      previousPage: 'Previous page',
      nextPage: 'Next page',
      goToPage: 'Go to page {page}',
      pageOf: 'Page {current} of {total}'
    },

    // UserAvatar component
    userAvatar: {
      myWorkspace: 'My Workspace',
      myWorkspaceSubtitle: 'Personal workspace for todos and notes',
      profileSubtitle: 'Manage your profile and settings',
      security: 'Security',
      securitySubtitle: 'Manage passwords, 2FA, and API tokens',
      themeTitle: 'Theme: {mode}',
      themeCycle: 'Click to cycle: Light → Dark → System',
      themeLight: 'Light',
      themeDark: 'Dark',
      themeSystem: 'System'
    }
  },

  aria: {
    close: 'Close',
    dragToReorder: 'Drag to reorder',
    refresh: 'Refresh',
    removeField: 'Remove field',
    removeFromSection: 'Remove from section',
    addNewStep: 'Add new step',
    removeCurrentStep: 'Remove current step',
    dismissNotification: 'Dismiss notification',
    mainNavigation: 'Main navigation',
    mentionUsers: 'Mention users',
    notifications: 'Notifications',
    adminSettings: 'Admin settings',
    userMenu: 'User menu',
    clearSearch: 'Clear search'
  },

  layout: {
    addSection: 'Add Section',
    moveUp: 'Move section up',
    moveDown: 'Move section down',
    deleteSection: 'Delete section',
    editMode: 'Edit Mode',
    editDisplaySettings: 'Edit display settings',
    items: 'items'
  },

  widgets: {
    removeWidget: 'Remove widget',
    narrowWidth: 'Narrow (1/3 width)',
    mediumWidth: 'Medium (2/3 width)',
    fullWidth: 'Full width',
    chart: {
      items: 'items'
    },
    completionChart: {
      emptyMessage: 'No completion data available'
    },
    createdChart: {
      emptyMessage: 'No creation data available'
    },
    milestoneProgress: {
      emptyTitle: 'No milestones',
      emptySubtitle: 'Create milestones to track progress'
    }
  },

  footer: {
    platformName: 'Windshift Work Management Platform',
    aboutWindshift: 'About Windshift',
    reportProblem: 'Report a problem'
  }
};
