/**
 * Workspace-related translations for English locale
 *
 * This module contains the following sections:
 * - workspaces: Workspace management translations
 * - items: Work items (issues, tasks, etc.) translations
 * - comments: Comment-related translations
 * - todo: Personal tasks and todo list translations
 * - collectionTree: Tree view translations
 * - collections: Saved queries and filters translations
 * - links: Item links translations
 */

export default {
  workspaces: {
    title: 'Workspaces',
    subtitle: 'Manage your workspaces and projects',
    workspace: 'Workspace',
    workspaces_one: '{count} workspace',
    workspaces_other: '{count} workspaces',
    createWorkspace: 'Create Workspace',
    editWorkspace: 'Edit Workspace',
    deleteWorkspace: 'Delete Workspace',
    switchWorkspace: 'Switch Workspace',
    workspaceName: 'Workspace Name',
    workspaceKey: 'Workspace Key',
    workspaceDescription: 'Description',
    members: 'Members',
    settings: 'Workspace Settings',
    noWorkspaces: 'No workspaces found',
    selectWorkspace: 'Select a workspace',
    currentWorkspace: 'Current Workspace',
    workspaceCreated: 'Workspace created successfully',
    workspaceUpdated: 'Workspace updated successfully',
    workspaceDeleted: 'Workspace deleted successfully',
    customers: {
      title: 'Customers',
      subtitle: 'Manage portal customers and organisations',
      addCustomer: 'Add Customer',
      unassignedCustomers: 'Unassigned Customers',
      customerCount_one: '{count} customer',
      customerCount_other: '{count} customers',
      failedToLoadCustomers: 'Failed to load customers',
      failedToLoadOrganisations: 'Failed to load organisations',
      failedToAssignCustomer: 'Failed to assign customer to organisation',
      deleteCustomer: 'Delete Customer',
      confirmDeleteCustomer: 'Are you sure you want to delete "{name}"?',
      manageOrganisations: 'Manage Organisations',
      searchOrganisations: 'Search organisations...',
      noOrganisationsFound: 'No organisations found',
      noCustomersFound: 'No customers found',
      unassigned: 'Unassigned',
      allCustomersAssigned: 'All customers are assigned to organisations',
      searchCustomers: 'Search customers...',
      tryAdjustingSearch: 'Try adjusting your search',
      dragCustomersHere: 'Drag customers here to assign them to this organisation',
      linked: 'Linked: ',
      loadMore: 'Load more ({count} remaining)',
      addPortalCustomer: 'Add Portal Customer',
      editPortalCustomer: 'Edit Portal Customer',
      createCustomer: 'Create Customer',
      noneUnassigned: 'None (Unassigned)',
      customFields: 'Custom Fields',
      fields: {
        name: 'Name',
        email: 'Email',
        phone: 'Phone',
        customerOrganisation: 'Customer Organisation'
      },
      placeholders: {
        name: 'Enter customer name',
        email: 'customer@example.com',
        phone: '+1 (555) 123-4567'
      },
      metadata: {
        created: 'Created:',
        updated: 'Updated:',
        linkedUser: 'Linked User:'
      }
    }
  },

  items: {
    title: 'Items',
    subtitle: 'View and manage work items',
    item: 'Item',
    items_one: '{count} item',
    items_other: '{count} items',
    createItem: 'Create Item',
    editItem: 'Edit Item',
    deleteItem: 'Delete Item',
    viewItem: 'View Item',
    itemKey: 'Key',
    itemTitle: 'Title',
    itemDescription: 'Description',
    itemType: 'Item Type',
    itemStatus: 'Status',
    itemPriority: 'Priority',
    assignee: 'Assignee',
    reporter: 'Reporter',
    dueDate: 'Due Date',
    startDate: 'Start Date',
    estimate: 'Estimate',
    timeSpent: 'Time Spent',
    remaining: 'Remaining',
    parent: 'Parent',
    children: 'Children',
    subtasks: 'Subtasks',
    linkedItems: 'Linked Items',
    attachments: 'Attachments',
    comments: 'Comments',
    activity: 'Activity',
    history: 'History',
    noItems: 'No items found',
    noItemsInFilter: 'No items match the current filter',
    createToStart: 'Create an item to get started',
    itemCreated: 'Item created successfully',
    itemUpdated: 'Item updated successfully',
    itemDeleted: 'Item deleted successfully',
    assignedToYou: 'Assigned to you',
    createdByYou: 'Created by you',
    recentlyViewed: 'Recently viewed',
    recentlyUpdated: 'Recently updated',
    // Item Detail Tabs
    timeTracking: 'Time Tracking',
    details: 'Details',
    created: 'Created',
    lastUpdated: 'Last Updated',
    by: 'by',
    workItemInformation: 'Work Item Information',
    id: 'ID',
    type: 'Type',
    workItem: 'Work Item',
    noProjectConfigured: 'No project configured for time tracking',
    setDefaultProject: 'Set a default project in workspace or item settings to log time',
    timeEntries: 'Time Entries',
    startTimer: 'Start Timer',
    logTime: 'Log Time',
    startTimerTitle: 'Start tracking time for this work item',
    logTimeTitle: 'Manually log time worked on this item',
    noTimeLogged: 'No time logged yet',

    // Item Detail additional translations
    workItemDetails: 'Work Item Details',
    fullDetails: 'Full Details',
    errorLoadingWorkItem: 'Error Loading Work Item',
    workItemNotFound: 'Work item not found',
    timerBusy: 'Timer busy',
    timerSyncingMessage: 'Timer is currently syncing, please wait a moment and try again.',
    timerAlreadyRunning: 'Timer already running',
    stopTimerFirst: 'Please stop the current timer before starting a new one.',
    workingOn: 'Working on {title}',
    failedToStartTimer: 'Failed to start timer',
    failedToSaveTimeEntry: 'Failed to save time entry',
    failedToDeleteTimeEntry: 'Failed to delete time entry',
    deleteTimeEntry: 'Delete Time Entry',
    deleteTimeEntryConfirm: 'Are you sure you want to delete this time entry? This action cannot be undone.',
    noDescription: 'No description',
    itemCopiedAs: 'Work item copied successfully as {key}',
    clickToViewCopied: 'Click to view copied item',
    failedToCopy: 'Failed to copy item',
    deleteWorkItem: 'Delete Work Item',
    confirmDeleteItem: 'Are you sure you want to delete "{title}"? This action cannot be undone.',
    failedToDelete: 'Failed to delete item',

    // Cascade delete dialog
    deleteItemWithChildren: 'Delete Item with Children',
    itemHasChildren: 'This item has {count} child items.',
    itemHasChildrenSingular: 'This item has 1 child item.',
    deleteAllOption: 'Delete all ({count} items)',
    deleteAllDescription: 'Permanently delete this item and all its descendants',
    reparentOption: 'Reparent children',
    reparentDescription: 'Move children to this item\'s parent, then delete only this item',
    typeToConfirm: 'Type "{title}" to confirm deletion',
    confirmationPlaceholder: 'Type the title to confirm...',
    deleteAllItems: 'Delete All Items',
    reparentAndDelete: 'Reparent & Delete',
    reparentFailed: 'Failed to reparent children',
    cascadeDeleteFailed: 'Failed to delete item tree',
    deletedItemsCount: 'Deleted {count} items',
    reparentedAndDeleted: 'Children reparented and item deleted',
    selectNewParent: 'Select new parent for children',
    selectNewParentPlaceholder: 'Choose a parent item...',
    makeRootItem: 'Make root items (no parent)',
    reparentLevelHint: 'Only showing items at the same hierarchy level',
    noOtherItemsAtLevel: 'No other items at this level - select "Make root items" or choose from above',
    reparentToGrandparent: 'Children will be moved to the grandparent',
    childrenWillBecomeRoot: 'Children will become root items',
    failedToUpdateWatchStatus: 'Failed to update watch status',
    copyWorkItem: 'Copy Work Item',
    unwatchWorkItem: 'Unwatch Work Item',
    watchWorkItem: 'Watch Work Item',
    noSubIssueTypes: 'No sub-issue types available',
    cannotCreateChildItems: 'Cannot create child work items for this item level.',

    // Item Detail Breadcrumbs
    workItems: 'Work Items',
    linkedTo: 'linked to',
    goToLinkedWorkItem: 'Go to linked work item',
    goTo: 'Go to {title}',
    noParent: 'No parent',
    setParent: 'Set parent',
    changeParent: 'Change Parent',
    searchForParentItem: 'Search for parent item...',
    showingItemsFromLevel: 'Only showing items from hierarchy level {level}',
    oneLevelAbove: 'one level above {name}',
    searchParentAcrossWorkspaces: 'Search for parent item across workspaces',
    removeParent: 'Remove parent',
    noItemsAtLevel: 'No items found at hierarchy level {level}',
    failedToUpdateParent: 'Failed to update parent',
    failedToRemoveParent: 'Failed to remove parent',
    clickToCopyKey: 'Click to copy key to clipboard',

    // Item Detail Description
    enterDescription: 'Enter description...',
    clickToEditDescription: 'Click to edit description',
    clickToAddDescription: 'Click to add description',
    noDescriptionProvided: 'No description provided - click to add one',
    addLink: 'Add Link',
    createChild: 'Create Child',
    child: 'Child',
    attachFile: 'Attach File',
    attach: 'Attach',
    newDiagram: 'New Diagram',
    diagram: 'Diagram',

    // Item Detail Header
    previousValueRemains: 'Previous value remains unchanged',
    titleCannotBeEmpty: 'Title cannot be empty',
    enterTitle: 'Enter title...',
    clickToEditTitle: 'Click to edit title',

    // Item Detail Links
    searchTestCases: 'Search test cases...',
    searchWorkItems: 'Search work items...',
    loadingLinks: 'Loading links...',
    removeLink: 'Remove link',
    linkType: 'Link Type',
    chooseRelationshipType: 'Choose relationship type...',
    linkToTestCase: 'Link this item to a test case.',
    targetItem: 'Target Item',
    testCase: 'Test Case',
    selectLinkTypeToSearch: 'Select a link type to start searching.',
    childWorkItems: 'Child Work Items',
    loadingChildItems: 'Loading child work items...',

    // Item Detail Sidebar
    setStatus: 'Set status',
    unassigned: 'Unassigned',
    milestone: 'Milestone',
    iteration: 'Iteration',
    project: 'Project',
    clickToViewDetails: 'Click to view item details',

    // Clipboard
    itemLinkCopied: 'Item link copied to clipboard',
    failedToCopyToClipboard: 'Failed to copy to clipboard',
    copyError: 'Copy Error'
  },

  comments: {
    failedToLoad: 'Failed to load comments',
    failedToCreate: 'Failed to post comment',
    confirmDelete: 'Are you sure you want to delete this comment?',
    failedToDelete: 'Failed to delete comment',
    failedToUpdate: 'Failed to update comment',
    edited: 'edited',
    editComment: 'Edit comment',
    deleteComment: 'Delete comment',
    editPlaceholder: 'Edit your comment...',
    writePlaceholder: 'Write a comment...',
    markdownSupported: 'Markdown supported',
    posting: 'Posting...',
    comment: 'Comment',
    noComments: 'No comments yet',
    beFirstToComment: 'Be the first to comment on this item.'
  },

  todo: {
    failedToCreate: 'Failed to create task',
    confirmDelete: 'Are you sure you want to delete this task?',
    deleteTask: 'Delete Task',
    failedToDelete: 'Failed to delete task',
    loadingTasks: 'Loading tasks...',
    myPersonalTasks: 'My Personal Tasks',
    whatNeedsToBeDone: 'What needs to be done?',
    addPersonalTask: 'Add personal task',
    noPersonalTasks: 'No personal tasks',
    addFirstTask: 'Add your first task to keep track of what you need to do.',
    ofPersonalTasksRemaining: '{count} of {total} personal tasks remaining',
    assignedToMe: 'Assigned to Me',
    noAssignedWork: 'No assigned work',
    assignedItemsWillAppear: 'Work items assigned to you will appear here.',
    ofAssignedItemsRemaining: '{count} of {total} assigned items remaining',
    task: 'Task',
    dueDate: 'Due Date',
    progress: 'Progress'
  },

  collectionTree: {
    loading: 'Loading...',
    tree: 'Tree',
    noWorkItemsYet: 'No work items yet',
    createFirstWorkItem: 'Create your first work item to see the hierarchy tree.',
    expandAll: 'Expand All',
    collapseAll: 'Collapse All',
    showTests: 'Show Tests',
    hideTests: 'Hide Tests',
    showingRootItems: 'Showing {start}-{end} of {total} root items',
    page: 'Page',
    pageOfTotal: 'Page {current} of {total}',
    issue: 'Issue',
    noStatus: 'No status',
    workspaceNotFound: 'Workspace not found.'
  },

  collections: {
    // Page titles and headers
    title: 'Collections',
    subtitle: 'Saved queries and filters',
    allGlobal: 'All Global',
    globalCollection: 'Global Collection',
    workspaceCollections: 'Workspace Collections',
    workspaceCollectionsTitle: 'Workspace Collections',
    allGlobalCollections: 'All Global Collections',
    categoryCollections: '{category} Collections',

    // Collection management
    newCollection: 'New Collection',
    createCollection: 'Create Collection',
    editCollection: 'Edit Collection',
    deleteCollection: 'Delete Collection',
    viewCollection: 'View Collection',
    saveCollection: 'Save Collection',
    updateCollection: 'Update Collection',

    // Collection properties
    collectionName: 'Collection Name',
    collectionDescription: 'Description',
    noQuery: 'No query',
    noFiltersApplied: 'No filters applied',

    // Workspace association
    associateWorkspace: 'Associate Workspace',
    changeWorkspace: 'Change Workspace',
    changeWorkspaceAssociation: 'Change Workspace Association',
    associateWithWorkspace: 'Associate with a Workspace',
    workspaceAssociationDesc: 'Selecting a workspace will scope this collection to that workspace. Leave it unassigned to keep it global.',
    saveAssociation: 'Save Association',
    workspaceAssociationNote: 'Only one workspace can be associated at a time. Removing the selection converts the collection back to a global view.',
    searchWorkspace: 'Search for a workspace...',

    // Categories
    manageCategories: 'Manage Categories',
    noCategory: 'No Category',

    // Filters and query
    filters: 'Filters',
    expandSidebar: 'Expand sidebar',
    collapseSidebar: 'Collapse sidebar',
    workspaces: 'Workspaces',
    selectWorkspaces: 'Select workspaces...',
    status: 'Status',
    selectStatuses: 'Select statuses...',
    priority: 'Priority',
    selectPriorities: 'Select priorities...',
    searchItems: 'Search items...',
    addFieldFilter: 'Add Field Filter',
    clearSearch: 'Clear search',

    // Query editor
    query: 'Query',
    queryLanguage: 'Query Language',
    queryPlaceholder: 'Example: workspace = "My Project" AND status = "open"',
    edit: 'Edit',
    hide: 'Hide',
    clear: 'Clear',
    execute: 'Execute',
    executeShortcut: '{shortcut} to execute',
    error: 'Error',

    // Search modal
    searchItemsTitle: 'Search Items',
    enterSearchText: 'Enter search text...',
    apply: 'Apply',

    // Views
    board: 'Board',
    backlog: 'Backlog',
    configure: 'Configure',
    map: 'Map',

    // Backlog view
    noItemsInBacklog: 'No Items in Backlog',
    noItemsInBacklogDesc: 'All work items are either completed or no items exist yet.',
    showingItemsFromBacklog: 'Showing {count} items from backlog',

    // Map view
    loadingStoryMap: 'Loading story map...',
    rootLevel: 'Root Level',
    currentLevel: 'Current Level',
    showingItems: 'Showing {count} {type}{plural}',
    childItems: '{count} child item{plural}',
    childWorkItems: 'Child Work Items ({count})',
    noChildItems: 'No child items yet',
    noChildItemsLowest: 'No child items (lowest hierarchy level)',
    addCard: 'Add card',
    create: 'Create',
    enterSummary: 'Enter a summary...',
    selectWorkspace: 'Select workspace',
    selectItemType: 'Select item type',
    noTypesAvailable: 'No types available',
    drillDown: 'Drill down to show children as backbone',
    noTopLevelItems: 'No top-level items found',
    noTopLevelItemsDesc: 'Create some work items to see your story map',
    workspaceNotFound: 'Workspace not found.',

    // Collections list
    collection: 'Collection',
    queryColumn: 'Query',
    created: 'Created',
    actions: 'Actions',
    public: 'Public',
    workspaceFilter: 'Workspace Filter',
    allWorkspaces: 'All workspaces',
    noCollectionsFound: 'No collections found. Create your first collection to save and reuse work item queries.',
    collectionCount: '{count} collection',
    collectionCountPlural: '{count} collections',

    // Results
    addFiltersToStart: 'Add filters to get started',
    addFiltersDesc: 'Use the sidebar filters or write a query to search for work items.',
    loadingWorkspaces: 'Loading workspaces...',
    loadingWorkItems: 'Loading work items...',
    noWorkItemsFound: 'No work items found',
    tryAdjustingFilters: 'Try adjusting your filters or search terms.',
    showingWorkItems: 'Showing {count} work items',

    // Confirmations
    confirmDeleteCollection: 'Are you sure you want to delete the collection "{name}"? This action cannot be undone.',
    confirmDeleteItem: 'Are you sure you want to delete "{title}"? This action cannot be undone.',
    noQueryToSave: 'No query to save. Please set up some filters or enter a QL query first.',

    // Board view
    boardSummary: 'Total: {itemCount} work items across {columnCount} columns'
  },

  links: {
    title: 'Links',
    subtitle: 'Manage item links',
    addLink: 'Add Link',
    removeLink: 'Remove link',
    linkText: 'Link text',
    linkUrl: 'URL'
  },

  workspaceSettings: {
    // Tab navigation
    tabs: {
      mode: 'Mode',
      general: 'General',
      appearance: 'Appearance',
      categories: 'Categories',
      members: 'Members',
      configurationSets: 'Configuration Sets',
      sourceControl: 'Source Control',
      removeWorkspace: 'Remove Workspace'
    },

    // Page header
    title: 'Settings',
    subtitle: 'Configure settings for {name}',
    breadcrumbs: {
      workspaces: 'Workspaces',
      settings: 'Settings'
    },

    // Access denied
    accessDenied: 'Access Denied',
    accessDeniedDescription: 'You need workspace administrator permissions to access settings.',
    backToWorkspace: 'Back to Workspace',

    // Mode tab
    displayMode: 'Display Mode',
    displayModeDescription: 'Choose how this workspace is displayed. This affects the navigation layout and default behavior.',
    modeDefault: 'Default',
    modeDefaultDescription: 'Full navigation sidebar with all workspace views, collections, and tools.',
    modeBoard: 'Board',
    modeBoardDescription: 'Simplified layout focused on the board view. Navigation is available through a compact toolbar.',
    modeItsm: 'ITSM',
    modeItsmDescription: 'Service management layout optimized for ticket handling and SLA tracking.',
    modeComingSoon: 'Coming Soon',

    // General tab
    basicInformation: 'Basic Information',
    workspaceName: 'Workspace Name',
    workspaceNamePlaceholder: 'Enter workspace name',
    workspaceKey: 'Workspace Key',
    workspaceKeyPlaceholder: 'e.g., DEV, TEST, PROD',
    workspaceKeyHelp: 'Used for item prefixes (e.g., DEV-123). Uppercase letters and numbers only.',
    description: 'Description',
    descriptionPlaceholder: 'Optional description for this workspace',
    defaultTimeProject: 'Default Time Tracking Project',
    noDefaultProject: 'No default project',
    defaultTimeProjectHelp: 'Default project used when logging time from work items in this workspace. Can be overridden per work item.',
    defaultView: 'Default Workspace View',
    defaultViewHelp: 'Default view displayed when entering this workspace.',
    activeWorkspace: 'Active Workspace',
    activeWorkspaceHelp: 'When inactive, only system admins and workspace admins can access this workspace. All data is preserved.',

    // View options
    views: {
      board: 'Board',
      backlog: 'Backlog',
      list: 'List',
      tree: 'Tree',
      map: 'Map',
      overview: 'Overview'
    },

    // Appearance tab
    visualIdentity: 'Visual Identity',
    visualIdentityDescription: 'Customize the visual appearance of your workspace with icons, colors, and avatars.',
    workspaceIconColor: 'Workspace Icon & Color',
    workspaceAvatar: 'Workspace Avatar',
    customAvatar: 'Custom Avatar',
    imageUploadedSuccessfully: 'Image uploaded successfully',
    defaultIcon: 'Default Icon',
    usingSelectedIconColor: 'Using selected icon and color',
    changeAvatar: 'Change Avatar',
    uploadAvatar: 'Upload Avatar',
    attachmentsRequired: 'Attachments must be enabled to upload workspace icons',
    uploadRecommendation: 'Recommended: Square images, at least 256x256 pixels for best quality',
    avatarOrIconNote: 'You can either use a custom avatar image or the icon & color combination above.',
    uploading: 'Uploading...',
    avatarUploadedSuccess: 'Avatar uploaded successfully',

    // Categories tab
    projectCategoryRestrictions: 'Project Category Restrictions',
    selectProjectCategories: 'Select project categories...',
    categoryRestrictionsHelp: 'Optionally restrict project selection to specific categories for this workspace. When set, users can only select projects from the chosen categories.',
    leaveEmptyNote: 'Note: Leave empty to allow selection from all project categories.',

    // Configuration tab
    activeConfiguration: 'Active Configuration',

    // Danger zone
    permanentRemoval: 'Permanent Removal',
    removeWarningIntro: 'Removing this workspace will permanently delete:',
    removeWarningItems: 'All work items and projects in this workspace',
    removeWarningFields: 'All custom field configurations',
    removeWarningScreens: 'All screen configurations',
    removeWarningFiles: 'All uploaded files associated with work items',
    removeWarningFinal: 'This action cannot be undone.',
    removeWorkspaceButton: 'Remove Workspace',
    typeToConfirm: 'Type {name} to confirm removal:',
    typeNameHere: "Type '{name}' here",
    yesRemoveWorkspace: 'Yes, Remove Workspace',

    // Actions and messages
    saveChanges: 'Save Changes',
    saving: 'Saving...',
    reset: 'Reset',
    remove: 'Remove',
    cancel: 'Cancel',
    workspaceNotFound: 'Workspace not found.',
    workspaceNameRequired: 'Workspace name is required',
    workspaceKeyRequired: 'Workspace key is required',
    savedSuccessfully: 'Workspace settings saved successfully',
    failedToSave: 'Failed to save workspace settings: {error}',
    deletedSuccessfully: 'Workspace "{name}" deleted successfully',
    failedToDelete: 'Failed to delete workspace: {error}',
    pleaseConfirmDeletion: 'Please enter the workspace name exactly as shown to confirm deletion',
    pleaseSelectImage: 'Please select an image file',
    failedToUploadAvatar: 'Failed to upload avatar: {error}'
  },

  lookAndFeel: {
    title: 'Look and Feel',
    subtitle: 'Customize the appearance and layout of your workspace',
    displayModeTitle: 'Display Mode',
    displayModeDescription: 'Choose how this workspace is displayed. This affects the navigation layout and default behavior.',
    gradientTitle: 'Background & Gradient',
    gradientDescription: 'Choose a color scheme for your workspace',
    gradients: 'Gradients',
    backgroundImages: 'Background Images',
    currentBackground: 'Current Background',
    uploadCustomImage: 'Upload Custom Image',
    backgroundUploadRecommendation: 'Recommended: High-resolution images (1920x1080 or larger) for best quality',
    backgroundUploadedSuccess: 'Background image uploaded successfully',
    failedToUploadBackground: 'Failed to upload background: {error}',
    identityTitle: 'Workspace Identity',
    identityDescription: 'Customize the icon, color, and avatar for your workspace',
    savedSuccessfully: 'Look and feel settings saved successfully',
    failedToSave: 'Failed to save look and feel settings: {error}'
  }
};
