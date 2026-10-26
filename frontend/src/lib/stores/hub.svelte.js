/**
 * Hub store for managing Portal Hub page state
 * Uses Svelte 5 runes pattern following portal.svelte.js
 */

import { hub } from '../api/hub.js';
import { attachments } from '../api/misc.js';
import { uploadBrandImage } from '../utils/brandLogoUpload.js';
import { createId } from '../utils/uuid.js';
import { authStore } from './auth.svelte.js';
import { createFooterLinkHelpers } from './footerLinks.js';
import {
  addSectionMember,
  addSection as addSectionToList,
  moveSection,
  removeSection as removeSectionFromList,
  removeSectionMember,
  updateSectionField,
} from './sectionManagement.js';

export { WINDSHIFT_GRADIENT } from '../utils/gradients.js';
export { gradients, iconMap } from './portalPresentation.js';

// Core state
let hubConfig = $state(null);
let portals = $state([]);
let loading = $state(true);
let error = $state(null);
let openRequestCount = $state(0);

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
let inboxStatusFacets = $state([]);

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

    const data = await hub.get();

    hubConfig = data.config;
    portals = data.portals || [];
    openRequestCount = data.open_request_count ?? 0;

    // Initialize editable state with hub config
    editableTitle = data.config.title || 'Portal Hub';
    editableDescription = data.config.description || '';
    selectedGradient = data.config.gradient || 0;
    isDarkMode = data.config.theme === 'dark';
    editableSearchPlaceholder = data.config.search_placeholder || 'Search portals...';
    editableSearchHint = data.config.search_hint || '';
    logoUrl = data.config.logo_url || null;

    // Load hub sections. Normalize the shape the editor persists so a
    // section saved before a field existed (or with a null assignment list)
    // still renders instead of crashing on `portal_ids.map`.
    hubSections = (data.config.sections || []).map((section, index) => ({
      ...section,
      subtitle: section.subtitle || '',
      display_order: section.display_order ?? index,
      portal_ids: Array.isArray(section.portal_ids) ? section.portal_ids : [],
    }));

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
  uploadingLogo = true;
  try {
    const url = await uploadBrandImage(files, {
      category: 'hub_logo',
      upload: attachments.upload,
    });
    if (url) {
      logoUrl = url;
      saveCustomizations();
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

      await hub.updateConfig(config);
    } catch (err) {
      console.error('Failed to save hub customizations:', err);
    }
  }, 1000);
}

// Hub Sections Management
function addSection() {
  const id = createId();
  hubSections = addSectionToList(hubSections, 'portal_ids', id);
  saveCustomizations();
  return id;
}

function deleteSection(sectionId) {
  hubSections = removeSectionFromList(hubSections, sectionId);
  saveCustomizations();
}

function updateSection(sectionId, field, value) {
  hubSections = updateSectionField(hubSections, sectionId, field, value);
  saveCustomizations();
}

function moveSectionUp(index) {
  hubSections = moveSection(hubSections, index, -1);
  saveCustomizations();
}

function moveSectionDown(index) {
  hubSections = moveSection(hubSections, index, 1);
  saveCustomizations();
}

function addPortalToSection(sectionId, portalId) {
  hubSections = addSectionMember(hubSections, sectionId, 'portal_ids', portalId);
  saveCustomizations();
}

function removePortalFromSection(sectionId, portalId) {
  hubSections = removeSectionMember(hubSections, sectionId, 'portal_ids', portalId);
  saveCustomizations();
}

/**
 * Get portals for a section
 */
function getSectionPortals(section) {
  return (section.portal_ids || [])
    .map((id) => portals.find((p) => p.id === id))
    .filter((p) => p !== undefined);
}

/**
 * Get portals not assigned to any section
 */
function getUnassignedPortals() {
  const assignedIds = new Set(hubSections.flatMap((s) => s.portal_ids || []));
  return portals.filter((p) => !assignedIds.has(p.id));
}

// Footer management — see ./footerLinks.js for the shared implementation.
const { addFooterLink, removeFooterLink, updateColumnTitle, updateFooterLink } =
  createFooterLinkHelpers({
    setColumns: (updater) => {
      footerColumns = updater(footerColumns);
    },
    saveCustomizations,
  });

// Inbox functions
async function loadInbox() {
  try {
    inboxLoading = true;
    const data = await hub.getInbox({
      page: inboxPage,
      per_page: inboxPerPage,
      portal_id: inboxPortalFilter || undefined,
      status: inboxStatusFilter || undefined,
    });

    inboxItems = data.items || [];
    inboxTotal = data.total;
    inboxTotalPages = data.total_pages;
    inboxStatusFacets = data.status_facets || [];
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
  // Cancel a pending debounced save so it cannot fire after navigation and
  // overwrite the hub with default/empty state.
  if (saveTimeout) {
    clearTimeout(saveTimeout);
    saveTimeout = null;
  }
  hubConfig = null;
  portals = [];
  loading = true;
  error = null;
  openRequestCount = 0;
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
  inboxStatusFacets = [];
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
  get openRequestCount() {
    return openRequestCount;
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
  get inboxStatusFacets() {
    return inboxStatusFacets;
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
