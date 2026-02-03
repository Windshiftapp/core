/**
 * Hub store for managing Portal Hub page state
 * Uses Svelte 5 runes pattern following portal.svelte.js
 */

import { api } from '../api.js';
import { authStore } from '../stores';

// Import gradients from portal store for sharing
export { gradients, iconMap } from './portal.svelte.js';

// Core state
let hubConfig = $state(null);
let portals = $state([]);
let loading = $state(true);
let error = $state(null);

// UI state
let isEditing = $state(false);
let isDarkMode = $state(false);
let showCustomizePanel = $state(false);
let showInbox = $state(false);
let selectedGradient = $state(0);
let activeSection = $state('hero-gradient');
let searchQuery = $state('');

// Logo state
let logoUrl = $state(null);
let uploadingLogo = $state(false);

// Editable content
let editableTitle = $state('Portal Hub');
let editableDescription = $state('');
let editableSearchPlaceholder = $state('Search portals...');
let editableSearchHint = $state('');

// Hub sections
let hubSections = $state([]);

// Footer columns
let footerColumns = $state([
  { title: '', links: [] },
  { title: '', links: [] },
  { title: '', links: [] },
]);

// Inbox state
let inboxItems = $state([]);
let inboxLoading = $state(false);
let inboxTotal = $state(0);
let inboxPage = $state(1);
let inboxPerPage = $state(20);
let inboxTotalPages = $state(0);
let inboxPortalFilter = $state('');
let inboxStatusFilter = $state('');

// Drag-and-drop state
let draggedPortal = $state(null);

// Internal state
let isInitialLoad = true;
let saveTimeout = null;

/**
 * Load hub data
 */
async function loadHub() {
  try {
    loading = true;
    error = null;

    const data = await api.hub.get();

    hubConfig = data.config;
    portals = data.portals || [];

    // Initialize editable state with hub config
    editableTitle = data.config.title || 'Portal Hub';
    editableDescription = data.config.description || '';
    selectedGradient = data.config.gradient || 0;
    isDarkMode = data.config.theme === 'dark';
    editableSearchPlaceholder = data.config.search_placeholder || 'Search portals...';
    editableSearchHint = data.config.search_hint || '';
    logoUrl = data.config.logo_url || null;

    // Load hub sections
    hubSections = data.config.sections || [];

    // Load footer columns
    footerColumns = data.config.footer_columns || [
      { title: '', links: [] },
      { title: '', links: [] },
      { title: '', links: [] },
    ];

    // Allow saves from user changes after initial load
    setTimeout(() => {
      isInitialLoad = false;
    }, 100);
  } catch (err) {
    console.error('Failed to load hub:', err);
    error = err.message || 'Failed to load hub';
  } finally {
    loading = false;
  }
}

/**
 * Toggle editing mode
 */
function toggleEditing() {
  const wasEditing = isEditing;
  isEditing = !isEditing;

  // Save changes when exiting edit mode
  if (wasEditing && !isEditing) {
    saveCustomizations();
  }
}

/**
 * Toggle dark/light theme
 */
function toggleTheme() {
  isDarkMode = !isDarkMode;
  saveCustomizations();
}

/**
 * Select a gradient
 */
function selectGradient(index) {
  selectedGradient = index;
  saveCustomizations();
}

/**
 * Handle logo upload
 */
async function handleLogoUpload(files) {
  if (!files || files.length === 0) return;

  const file = files[0];
  if (!file.type.startsWith('image/')) {
    console.error('Please select an image file');
    return;
  }

  uploadingLogo = true;
  try {
    const uploadFormData = new FormData();
    uploadFormData.append('file', file);
    uploadFormData.append('category', 'hub_logo');

    const response = await fetch('/api/attachments/upload', {
      method: 'POST',
      body: uploadFormData,
    });

    if (!response.ok) {
      throw new Error(`Upload failed: ${response.statusText}`);
    }

    const uploadResult = await response.json();

    if (uploadResult?.success && uploadResult.logo_url) {
      logoUrl = uploadResult.logo_url;
      saveCustomizations();
      console.log('Hub logo uploaded successfully');
    }
  } catch (err) {
    console.error('Failed to upload hub logo:', err);
  } finally {
    uploadingLogo = false;
  }
}

/**
 * Remove logo
 */
function removeLogo() {
  logoUrl = null;
  saveCustomizations();
}

/**
 * Save customizations (debounced)
 */
async function saveCustomizations() {
  if (!authStore.isAuthenticated) return;
  if (isInitialLoad) return;

  if (saveTimeout) clearTimeout(saveTimeout);

  saveTimeout = setTimeout(async () => {
    try {
      const config = {
        title: editableTitle,
        description: editableDescription,
        gradient: selectedGradient,
        theme: isDarkMode ? 'dark' : 'light',
        search_placeholder: editableSearchPlaceholder,
        search_hint: editableSearchHint,
        logo_url: logoUrl || '',
        sections: hubSections,
        footer_columns: footerColumns,
      };

      await api.hub.updateConfig(config);
      console.log('Hub customizations saved');
    } catch (err) {
      console.error('Failed to save hub customizations:', err);
    }
  }, 1000);
}

// Hub Sections Management
function addSection() {
  const newSection = {
    id: crypto.randomUUID(),
    title: '',
    subtitle: '',
    display_order: hubSections.length,
    portal_ids: [],
  };
  hubSections = [...hubSections, newSection];
  saveCustomizations();
  return newSection.id;
}

function deleteSection(sectionId) {
  hubSections = hubSections
    .filter((s) => s.id !== sectionId)
    .map((s, i) => ({ ...s, display_order: i }));
  saveCustomizations();
}

function updateSection(sectionId, field, value) {
  hubSections = hubSections.map((s) => {
    if (s.id === sectionId) {
      return { ...s, [field]: value };
    }
    return s;
  });
  saveCustomizations();
}

function moveSectionUp(index) {
  if (index === 0) return;
  const newSections = [...hubSections];
  [newSections[index - 1], newSections[index]] = [newSections[index], newSections[index - 1]];
  hubSections = newSections.map((s, i) => ({ ...s, display_order: i }));
  saveCustomizations();
}

function moveSectionDown(index) {
  if (index === hubSections.length - 1) return;
  const newSections = [...hubSections];
  [newSections[index], newSections[index + 1]] = [newSections[index + 1], newSections[index]];
  hubSections = newSections.map((s, i) => ({ ...s, display_order: i }));
  saveCustomizations();
}

function addPortalToSection(sectionId, portalId) {
  hubSections = hubSections.map((s) => {
    if (s.id === sectionId) {
      if (!s.portal_ids.includes(portalId)) {
        return {
          ...s,
          portal_ids: [...s.portal_ids, portalId],
        };
      }
    }
    return s;
  });
  saveCustomizations();
}

function removePortalFromSection(sectionId, portalId) {
  hubSections = hubSections.map((s) => {
    if (s.id === sectionId) {
      return {
        ...s,
        portal_ids: s.portal_ids.filter((id) => id !== portalId),
      };
    }
    return s;
  });
  saveCustomizations();
}

/**
 * Get portals for a section
 */
function getSectionPortals(section) {
  return section.portal_ids
    .map((id) => portals.find((p) => p.id === id))
    .filter((p) => p !== undefined);
}

/**
 * Get portals not assigned to any section
 */
function getUnassignedPortals() {
  const assignedIds = new Set(hubSections.flatMap((s) => s.portal_ids));
  return portals.filter((p) => !assignedIds.has(p.id));
}

// Footer management
function addFooterLink(columnIndex) {
  footerColumns = footerColumns.map((col, idx) => {
    if (idx === columnIndex) {
      return {
        ...col,
        links: [...col.links, { text: '', url: '' }],
      };
    }
    return col;
  });
  saveCustomizations();
}

function removeFooterLink(columnIndex, linkIndex) {
  footerColumns = footerColumns.map((col, idx) => {
    if (idx === columnIndex) {
      return {
        ...col,
        links: col.links.filter((_, i) => i !== linkIndex),
      };
    }
    return col;
  });
  saveCustomizations();
}

function updateColumnTitle(columnIndex, title) {
  footerColumns = footerColumns.map((col, idx) => {
    if (idx === columnIndex) {
      return { ...col, title };
    }
    return col;
  });
  saveCustomizations();
}

function updateFooterLink(columnIndex, linkIndex, field, value) {
  footerColumns = footerColumns.map((col, idx) => {
    if (idx === columnIndex) {
      return {
        ...col,
        links: col.links.map((link, i) => {
          if (i === linkIndex) {
            return { ...link, [field]: value };
          }
          return link;
        }),
      };
    }
    return col;
  });
  saveCustomizations();
}

// Inbox functions
async function loadInbox() {
  try {
    inboxLoading = true;
    const data = await api.hub.getInbox({
      page: inboxPage,
      per_page: inboxPerPage,
      portal_id: inboxPortalFilter || undefined,
      status: inboxStatusFilter || undefined,
    });

    inboxItems = data.items || [];
    inboxTotal = data.total;
    inboxTotalPages = data.total_pages;
  } catch (err) {
    console.error('Failed to load inbox:', err);
  } finally {
    inboxLoading = false;
  }
}

function setInboxPage(page) {
  inboxPage = page;
  loadInbox();
}

function setInboxFilters(portalId, status) {
  inboxPortalFilter = portalId || '';
  inboxStatusFilter = status || '';
  inboxPage = 1;
  loadInbox();
}

async function toggleInbox() {
  showInbox = !showInbox;

  if (showInbox) {
    await loadInbox();
  }
}

// Reset store (for cleanup)
function reset() {
  hubConfig = null;
  portals = [];
  loading = true;
  error = null;
  isEditing = false;
  isDarkMode = false;
  showCustomizePanel = false;
  showInbox = false;
  selectedGradient = 0;
  activeSection = 'hero-gradient';
  searchQuery = '';
  logoUrl = null;
  uploadingLogo = false;
  editableTitle = 'Portal Hub';
  editableDescription = '';
  editableSearchPlaceholder = 'Search portals...';
  editableSearchHint = '';
  hubSections = [];
  footerColumns = [
    { title: '', links: [] },
    { title: '', links: [] },
    { title: '', links: [] },
  ];
  inboxItems = [];
  inboxLoading = false;
  inboxTotal = 0;
  inboxPage = 1;
  inboxPerPage = 20;
  inboxTotalPages = 0;
  inboxPortalFilter = '';
  inboxStatusFilter = '';
  draggedPortal = null;
  isInitialLoad = true;
}

// Export the store with getters and actions
export const hubStore = {
  // Getters for core state
  get hubConfig() {
    return hubConfig;
  },
  get portals() {
    return portals;
  },
  get loading() {
    return loading;
  },
  get error() {
    return error;
  },

  // Getters for UI state
  get isEditing() {
    return isEditing;
  },
  get isDarkMode() {
    return isDarkMode;
  },
  get showCustomizePanel() {
    return showCustomizePanel;
  },
  get showInbox() {
    return showInbox;
  },
  get selectedGradient() {
    return selectedGradient;
  },
  get activeSection() {
    return activeSection;
  },
  get searchQuery() {
    return searchQuery;
  },

  // Getters for logo state
  get logoUrl() {
    return logoUrl;
  },
  get uploadingLogo() {
    return uploadingLogo;
  },

  // Getters for editable content
  get editableTitle() {
    return editableTitle;
  },
  get editableDescription() {
    return editableDescription;
  },
  get editableSearchPlaceholder() {
    return editableSearchPlaceholder;
  },
  get editableSearchHint() {
    return editableSearchHint;
  },

  // Getters for sections/footer
  get hubSections() {
    return hubSections;
  },
  get footerColumns() {
    return footerColumns;
  },
  get draggedPortal() {
    return draggedPortal;
  },

  // Getters for inbox
  get inboxItems() {
    return inboxItems;
  },
  get inboxLoading() {
    return inboxLoading;
  },
  get inboxTotal() {
    return inboxTotal;
  },
  get inboxPage() {
    return inboxPage;
  },
  get inboxPerPage() {
    return inboxPerPage;
  },
  get inboxTotalPages() {
    return inboxTotalPages;
  },
  get inboxPortalFilter() {
    return inboxPortalFilter;
  },
  get inboxStatusFilter() {
    return inboxStatusFilter;
  },

  // Setters for UI state
  set isEditing(value) {
    isEditing = value;
  },
  set showCustomizePanel(value) {
    showCustomizePanel = value;
  },
  set showInbox(value) {
    showInbox = value;
  },
  set activeSection(value) {
    activeSection = value;
  },
  set searchQuery(value) {
    searchQuery = value;
  },
  set draggedPortal(value) {
    draggedPortal = value;
  },

  // Setters for editable content
  set editableTitle(value) {
    editableTitle = value;
  },
  set editableDescription(value) {
    editableDescription = value;
  },
  set editableSearchPlaceholder(value) {
    editableSearchPlaceholder = value;
  },
  set editableSearchHint(value) {
    editableSearchHint = value;
  },

  // Actions
  loadHub,
  toggleEditing,
  toggleTheme,
  selectGradient,
  saveCustomizations,

  // Logo actions
  handleLogoUpload,
  removeLogo,

  // Section actions
  addSection,
  deleteSection,
  updateSection,
  moveSectionUp,
  moveSectionDown,
  addPortalToSection,
  removePortalFromSection,
  getSectionPortals,
  getUnassignedPortals,

  // Footer actions
  addFooterLink,
  removeFooterLink,
  updateColumnTitle,
  updateFooterLink,

  // Inbox actions
  loadInbox,
  setInboxPage,
  setInboxFilters,
  toggleInbox,

  // Reset
  reset,
};
