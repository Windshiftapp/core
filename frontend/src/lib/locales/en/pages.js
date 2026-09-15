/**
 * English (en) - Knowledge pages (wiki) translations
 *
 * Surfaces:
 *   - PagesView (left tree + editor pane)
 *   - PageMoveDialog
 *   - PagePermissionsDialog
 */
export default {
  pages: {
    // Sidebar / drilldown nav
    backWorkspace: 'Workspace',
    treeHeading: 'Pages',
    addPageAria: 'Add page',
    untitled: 'Untitled',
    treeLoading: 'Loading…',
    treeEmptyTitle: 'No pages yet',
    treeEmptyDescription: 'Use the + button above to create your first page.',

    // Per-item kebab menu
    menuAddChild: 'Add child page',
    menuRename: 'Rename',
    menuMove: 'Move',
    menuPermissions: 'Permissions',
    menuHistory: 'History',
    menuPrint: 'Print',
    menuArchive: 'Archive',
    menuPromoteToRequirement: 'Promote to requirement',

    // Chrome-free print / save-to-PDF view (opened in a new tab)
    print: {
      button: 'Print',
      back: 'Back to page',
      loading: 'Preparing page for print…',
      error: 'Failed to load page.',
    },

    // Revision history drawer
    history: {
      title: 'Revision history',
      empty: 'No revisions yet.',
      loadError: 'Failed to load revision history.',
      restoreTitle: 'Restore revision #{rev}?',
      restoreMessage:
        'This will replace the current page body with the content of the selected revision and create a new revision recording the restore.',
      restoreConfirm: 'Restore',
      restoreAction: 'Restore #{rev}',
      restoring: 'Restoring…',
      restoredOK: 'Restored revision #{rev}.',
      restoreError: 'Failed to restore revision.',
    },

    // Page pane
    pageLoading: 'Loading page…',
    emptyPaneTitle: 'Knowledge Pages',
    emptyPaneDescription: 'Select a page from the tree, or create one to get started.',
    titlePlaceholder: 'Untitled',
    editorPlaceholder: 'Start writing…',
    tocHeading: 'On this page',
    tocAriaLabel: 'Table of contents',

    // Action buttons on the open page
    save: 'Save',
    move: 'Move',
    permissions: 'Permissions',
    archive: 'Archive',

    // Auto-save status indicator next to the toolbar kebab
    statusSaving: 'Saving…',
    statusSaved: 'Saved',
    statusUnsaved: 'Unsaved',
    statusError: 'Save failed',

    // Public knowledge-base publication banner (green bar above the page)
    publiclyViewableBanner:
      'Publicly viewable — this page is published through a portal knowledge base and can be read by portal users.',
    publiclyViewableBannerWithPortals:
      'Publicly viewable — this page is published through the {portals} knowledge base and can be read by portal users.',

    // Segmented Edit / Read mode toggle
    modeEdit: 'Edit',
    modeRead: 'Read',
    modeAria: 'View mode',
    canvasWide: 'Use full-width canvas',
    canvasComfortable: 'Use readable-width canvas',

    // Error fallbacks
    errorLoadTree: 'Failed to load pages',
    errorLoadPage: 'Failed to load page',
    errorSave: 'Failed to save',
    errorCreate: 'Failed to create page',
    errorArchive: 'Failed to archive',

    // Discard / archive confirms
    discardTitle: 'Discard unsaved changes?',
    discardMessage:
      'You have unsaved changes on the current page. They will be lost if you switch.',
    discardConfirm: 'Discard',
    discardCancel: 'Keep editing',
    archiveTitle: 'Archive "{title}"?',
    archiveMessage:
      'This archives the page and every child page. This action cannot be undone.',
    archiveConfirm: 'Archive',

    // Archived pages list (admin-only full-page view opened from the sidebar header)
    archivedOpenAria: 'View archived pages',
    archivedHeading: 'Archived pages',
    archivedSubtitle: 'Review and restore pages that were previously archived.',
    archivedBack: 'Back to pages',
    archivedEmpty: 'No archived pages',
    archivedColTitle: 'Title',
    archivedColArchivedAt: 'Archived',
    archivedColArchivedBy: 'Archived by',
    archivedUnarchive: 'Unarchive',
    archivedUnarchiveTitle: 'Unarchive "{title}"?',
    archivedUnarchiveMessage:
      'The page reappears in the tree. If its parent is still archived, the page stays hidden until you unarchive the parent too.',
    archivedUnarchiveConfirm: 'Unarchive',
    archivedUnarchiveOK: 'Restored "{title}"',
    archivedLoadError: 'Failed to load archived pages',
    archivedUnarchiveError: 'Failed to unarchive page',

    // Move dialog
    moveTitle: 'Move "{title}"',
    moveSubtitle:
      'Pick a new parent. Pages under the current page are hidden because they would create a cycle.',
    moveWorkspaceLabel: 'Destination workspace',
    moveWorkspacePlaceholder: 'Select a workspace…',
    moveParentLabel: 'Parent page',
    moveSearchPlaceholder: 'Search pages…',
    moveRoot: 'Workspace root',
    moveCrossWorkspaceSummary: 'Pages in this subtree: {count}.',
    moveCrossWorkspacePolicy:
      'Matching labels are kept. Explicit permissions, work-item links, and agent-skill references are removed.',
    moveButton: 'Move',
    moveCancel: 'Cancel',
    errorLoadWorkspaces: 'Failed to load workspaces',
    errorMove: 'Move failed',

    // Permissions dialog
    permsTitle: 'Page permissions',
    permsEffectiveAccess: 'Your effective access: {level}',
    permsEffectiveAccessNone: 'none',
    permsLoading: 'Loading…',
    permsInheritLabel: 'Inherit permissions from ancestors',
    permsInheritHint:
      'When inheritance is on and no explicit grants exist, workspace role permissions decide. Breaking inheritance with no grants restricts the page to admins.',
    permsExplicitGrants: 'Explicit grants',
    permsEmptyGrantsTitle: 'No explicit grants on this page.',
    permsEmptyGrantsDescription: 'Inheritance and workspace roles still apply.',
    permsColumnPrincipal: 'Principal',
    permsColumnLevel: 'Level',
    permsRemove: 'Remove',
    permsRemoveTitle: 'Remove permission?',
    permsRemoveMessage: 'This grant will be removed from the page. You can re-add it later.',
    permsRemoveConfirm: 'Remove',
    permsRemoveCancel: 'Cancel',
    permsClose: 'Close',
    permsAdd: 'Add',
    permsPrincipalUser: 'User',
    permsPrincipalGroup: 'Group',
    permsPrincipalRole: 'Role',
    permsLevelView: 'View',
    permsLevelEdit: 'Edit',
    permsLevelAdmin: 'Admin',
    permsPickUser: 'Pick a user',
    permsPickGroup: 'Pick a group',
    permsPickRole: 'Pick a role',
    permsErrorNoPrincipal: 'Pick a principal before adding the grant',
    permsErrorLoad: 'Failed to load permissions',
    permsErrorInherit: 'Failed to update inheritance',
    permsErrorGrant: 'Failed to add permission',
    permsErrorRevoke: 'Failed to revoke',

    // Page labels (workspace-scoped, attach to pages only)
    labelsTitle: 'Labels',
    labelsAdd: 'Add label',
    labelsCreate: 'Create label',
    labelsCreateNamed: 'Create "{name}"',
    labelsSearchPlaceholder: 'Search or create…',
    labelsFilterTitle: 'Filter by label',
    labelsFilterPlaceholder: 'Filter pages by label',
    labelsFilterClear: 'Clear filter',
    labelsEmpty: 'No labels yet',
    labelsNameRequired: 'Label name is required',
    labelsDuplicate: 'A label with that name already exists',
    labelsDeleteConfirm: 'Delete this label? It will be removed from every page it is attached to.',
    labelsDelete: 'Delete label',
    labelsRemoveAria: 'Remove label {name}',
    labelsErrorLoad: 'Failed to load labels',
    labelsErrorSave: 'Failed to save label',
    labelsErrorAttach: 'Failed to attach label',
    labelsErrorDetach: 'Failed to detach label',

    // Sidebar title search
    searchAria: 'Search pages',
    searchPlaceholder: 'Search pages…',
    searchClear: 'Clear search',

    // Sidebar tree expand/collapse
    toggleSubtreeAria: 'Toggle children of {title}',
    expandAllAria: 'Expand all',
    collapseAllAria: 'Collapse all',

    // Linked work items button + popover (top-right on page detail)
    workItemsButton: 'Work items',
    workItemsAria: 'Show linked work items',
    workItemsEmpty: 'No work items link here yet',
    workItemsLoading: 'Loading work items…',
    workItemsTitle: 'Work items',
    addWorkItem: 'Add work item',
    addWorkItemCancel: 'Cancel',
    addWorkItemSearchPlaceholder: 'Search work items…',
    removeWorkItemLink: 'Unlink work item',
    workItemsErrorLoad: 'Failed to load linked work items',
    workItemsErrorLink: 'Failed to link work item',
    workItemsErrorUnlink: 'Failed to unlink work item',
  },
};
