/**
 * Time tracking related translations for English locale
 * Includes: time, timeProject, timeProjectCategory sections
 */

export default {
  time: {
    title: 'Time Tracking',
    subtitle: 'Track time spent on work items',
    logTime: 'Log Time',
    editTimeEntry: 'Edit Time Entry',
    updateEntry: 'Update Entry',
    timeSpent: 'Time Spent',
    remaining: 'Remaining',
    estimate: 'Estimate',
    originalEstimate: 'Original Estimate',
    hours: 'hours',
    minutes: 'minutes',
    days: 'days',
    weeks: 'weeks',
    startTimer: 'Start Timer',
    stopTimer: 'Stop Timer',
    pauseTimer: 'Pause Timer',
    resumeTimer: 'Resume Timer',
    timeLogged: 'Time logged successfully',
    timeTrackingProject: 'Time Tracking Project',
    workItemOptional: 'Work Item (Optional)',
    whatDidYouWorkOn: 'What did you work on?',
    start: 'Start',
    end: 'End',
    duration: 'Duration',
    durationHelperText: 'Enter start time + duration (2h) to auto-calculate end time, or enter start + end times to auto-calculate duration. Time formats: 1h, 30m, 1h30m, 2h15m, 1d (=8h)',

    // Onboarding
    onboarding: {
      title: 'Set Up Time Tracking',
      subtitle: "Let's create your first customer organization and project to get started",
      setupProgress: 'Setup Progress',
      stepOf: 'Step {current} of {total}',
      createCustomerTitle: 'Create Your First Customer Organization',
      createCustomerDescription: 'A customer organization represents the company or entity you\'re doing work for. This could be a client organization, your employer, or your own company.',
      createProjectTitle: 'Create Your First Project',
      createProjectDescription: 'Projects help organize your work within a customer. You can track time against specific projects.',
      organizationNameRequired: 'Organization name is required',
      projectNameRequired: 'Project name is required',
      failedToCreateCustomer: 'Failed to create customer organization. Please try again.',
      failedToCreateProject: 'Failed to create project. Please try again.',
      customerCreatedSuccess: 'Customer organization "{name}" created successfully!',
      organizationNamePlaceholder: 'e.g., Acme Corporation, TechStart Inc, Freelance Work',
      emailPlaceholder: 'billing@customer.com',
      contactPersonPlaceholder: 'John Smith',
      projectNamePlaceholder: 'e.g., Website Development, Consulting, General Work',
      projectDescriptionPlaceholder: 'Brief description of the project...',
      hourlyRateHint: 'You can set this later if you\'re not sure',
      skipForNow: 'Skip for now',
      completeSetup: 'Complete Setup'
    },

    // Categories
    categories: {
      title: 'Project Categories',
      subtitle: 'Organize projects into categories for better management',
      newCategory: 'New Category',
      noCategories: 'No categories yet',
      createFirstHint: 'Create your first category to organize projects',
      failedToSave: 'Failed to save category',
      failedToDelete: 'Failed to delete category',
      confirmDelete: 'Are you sure you want to delete "{name}"?'
    },

    // Reports
    reports: {
      title: 'Reports',
      subtitle: 'Analyze your time tracking data and export reports',
      exportCSV: 'Export CSV',
      exportPDF: 'Export PDF',
      filters: 'Filters',
      customer: 'Customer',
      project: 'Project',
      allCustomers: 'All customers',
      allProjects: 'All projects',
      descriptionFilter: 'Description Filter',
      searchDescriptions: 'Search descriptions...',
      fromDate: 'From Date',
      toDate: 'To Date',
      applyFilters: 'Apply Filters',
      totalHours: 'Total Hours',
      totalEntries: 'Total Entries',
      averagePerDay: 'Average per Day',
      topProject: 'Top Project',
      loadingReports: 'Loading reports...',
      noEntriesFound: 'No time entries found for the selected filters.',
      totalTime: 'Total Time',
      entriesShown: '{count} entries shown'
    },

    // Timer
    timer: {
      goToWorkItem: 'Go to work item: {title}',
      expandTimer: 'Expand timer',
      collapseTimer: 'Collapse timer',
      project: 'Project',
      workspace: 'Workspace'
    },

    // Projects
    projects: {
      title: 'Projects',
      subtitle: 'Manage global projects for time tracking across workspaces',
      addProject: 'Add Project',
      projectsTab: 'Projects',
      categoriesTab: 'Categories',
      searchProjects: 'Search projects...',
      allCategories: 'All Categories',
      allStatuses: 'All Statuses',
      statusCount: '{count} statuses',
      noProjects: 'No projects found. Create your first project to start tracking time.',
      noProjectsInCategory: 'No projects in this category.',
      failedToSave: 'Failed to save project',
      deleteProject: 'Delete Project',
      confirmDelete: 'Are you sure you want to delete "{name}"?',
      unknownCustomer: 'Unknown Customer',
      project: 'Project',
      customer: 'Customer',
      rate: 'Rate',
      projectName: 'Project Name',
      descriptionOptional: 'Description (Optional)',
      hourlyRateOptional: 'Hourly Rate (Optional)'
    },

    // Calendar
    calendar: {
      title: 'Weekly Calendar',
      itemCount: '{count} items',
      exportWeekToICS: 'Export Week to ICS',
      myWorkItems: 'My Work Items',
      dragToSchedule: 'Drag items to schedule them',
      noWorkItems: 'No work items assigned',
      workItemsWillAppear: 'Work items will appear here when assigned to you',
      itemsCompleted: '{completed} of {total} items completed',
      previousWeek: 'Previous week',
      thisWeek: 'this week',
      nextWeek: 'Next week',
      newTaskPlaceholder: 'New task title...',
      failedToCreateTask: 'Failed to create task'
    },

    // Time Entry
    entry: {
      title: 'Time Entry',
      subtitle: 'Log your work hours and manage time entries',
      addTimeEntry: 'Add a new time entry',
      failedToSave: 'Failed to save time entry. Please check your input.',
      confirmDelete: 'Are you sure you want to delete this time entry?',
      needProjects: 'You need to create active projects before logging time.',
      goToProjects: 'Go to Projects',
      startSetupWizard: 'start the setup wizard',
      applyFiltersTitle: 'Apply the selected filters to the time entries list',
      clearFiltersTitle: 'Clear all filters and show all time entries',
      noEntries: 'No time entries found. Log your first time entry to get started.',
      clickToView: 'Click to view {key}-{number}',
      budgetExceeded: '- budget exceeded'
    },

    // Organizations (formerly Customers)
    organizations: {
      title: 'Organizations',
      subtitle: 'Manage your client organizations',
      addOrganization: 'Add Organization',
      noOrganizations: 'No organizations found. Create your first organization to get started.',
      name: 'Organization Name',
      emailOptional: 'Email (Optional)',
      contactPersonOptional: 'Contact Person (Optional)',
      failedToSave: 'Failed to save organization',
      deleteOrganization: 'Delete Organization',
      confirmDelete: 'Are you sure you want to delete "{name}"?'
    }
  },

  timeProject: {
    editProject: 'Edit Project',
    newProject: 'New Project',
    projectName: 'Project Name',
    status: 'Status',
    customerOptional: 'Customer (Optional)',
    none: 'None',
    categoryOptional: 'Category (Optional)',
    hourlyRate: 'Hourly Rate ($)',
    maxHours: 'Max Hours',
    maxHoursPlaceholder: 'No limit',
    maxHoursHint: 'Optional budget for reporting',
    projectColor: 'Project Color',
    updateProject: 'Update Project',
    createProject: 'Create Project'
  },

  timeProjectCategory: {
    editCategory: 'Edit Category',
    newCategory: 'New Category',
    categoryName: 'Category Name',
    categoryNamePlaceholder: 'Development, Marketing, Operations...',
    optionalDescription: 'Optional description...',
    updateCategory: 'Update Category',
    createCategory: 'Create Category'
  }
};
