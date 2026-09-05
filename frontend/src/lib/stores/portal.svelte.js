/**
 * Portal store for managing portal page state
 * Uses Svelte 5 runes pattern following theme.svelte.js
 */

import { api } from '../api.js';
import { authStore } from '../stores';
import { safeCssUrl } from '../utils/sanitize';
import { createFooterLinkHelpers } from './footerLinks.js';
import {
  configurePortalActivityStore,
  portalActivityStore,
  portalApprovalsStore,
  portalDraftsStore,
  portalRequestsStore,
} from './portalActivity.svelte.js';
import { portalAuthStore } from './portalAuth.svelte.js';
import { gradients } from './portalPresentation.js';
import { configurePortalSearchStore, portalSearchStore } from './portalSearch.svelte.js';
import { errorToast } from './toasts.svelte.js';

// Transitional exports for consumers that have not moved to portalPresentation yet.
export { gradients, iconMap } from './portalPresentation.js';

// Core state
let portalData = $state(null);
let loading = $state(true);
let error = $state(null);
let currentSlug = $state(null);

// UI state
let isEditing = $state(false);
let isDarkMode = $state(false);
let showCustomizePanel = $state(false);
let selectedGradient = $state(0);
let activeSection = $state('hero-gradient');

// Background image state
let backgroundImageUrl = $state(null);
let uploadingBackground = $state(false);
let selectedBackgroundCategory = $state('abstract');

// Logo state
let logoUrl = $state(null);
let hubLogoUrl = $state(null);
let uploadingLogo = $state(false);

// Menu states
let showProfileMenu = $state(false);
let showMainMenu = $state(false);
let showLoginDialog = $state(false);

// Editable content
let editableTitle = $state('');
let editableDescription = $state('');
let editableSearchPlaceholder = $state('Search the knowledge base...');
let editableSearchHint = $state('Search for articles, guides, and answers to common questions');

// Request types state
let requestTypes = $state([]);
let loadingRequestTypes = $state(false);
let requestTypesLoadId = 0;

// Asset reports state
let assetReports = $state([]);
let loadingAssetReports = $state(false);
let hasAssetSets = $state(false);
let assetReportsLoadId = 0;

// Portal sections
let portalSections = $state([]);

// Drag-and-drop state
let draggedRequestType = $state(null);
let draggedAssetReport = $state(null);

// Footer columns
let footerColumns = $state([
  { title: '', links: [] },
  { title: '', links: [] },
  { title: '', links: [] },
]);

// Knowledge base
let knowledgeBaseShareLink = $state('');

// Pending request type (for opening form after login)
let pendingRequestType = $state(null);

// Internal state
let isInitialLoad = true;
let saveTimeout = null;

configurePortalSearchStore({
  getKnowledgeBaseShareLink: () => knowledgeBaseShareLink,
  getSlug: () => portalData?.slug || currentSlug,
});
configurePortalActivityStore({
  closeProfileMenu: () => {
    showProfileMenu = false;
  },
  getSlug: () => currentSlug,
});

/**
 * Load portal data by slug
 */
async function loadPortal(slug) {
  try {
    loading = true;
    error = null;
    const switchedPortal = currentSlug !== slug;
    currentSlug = slug;

    if (switchedPortal) {
      portalSearchStore.reset();
      portalActivityStore.reset();
    }

    if (!slug) {
      error = 'Portal not specified';
      return;
    }

    const bootstrap = await api.portal.getBootstrap(slug);
    portalData = bootstrap.portal;

    // Initialize editable state with portal data
    editableTitle = portalData.title || 'Support Portal';
    editableDescription = portalData.description || '';

    // Load customization data
    selectedGradient = portalData.gradient || 0;
    isDarkMode = portalData.theme === 'dark';
    editableSearchPlaceholder = portalData.search_placeholder || 'Search the knowledge base...';
    editableSearchHint =
      portalData.search_hint || 'Search for articles, guides, and answers to common questions';
    backgroundImageUrl = portalData.background_image_url || null;
    logoUrl = portalData.logo_url || null;
    hubLogoUrl = portalData.hub_logo_url || null;

    // Load footer columns
    footerColumns = portalData.footer_columns || [
      { title: '', links: [] },
      { title: '', links: [] },
      { title: '', links: [] },
    ];

    // Load portal sections
    portalSections = portalData.sections || [];

    // Load knowledge base configuration
    knowledgeBaseShareLink = portalData.knowledge_base_share_link || '';

    // Ensure workspace_ids is always an array
    portalData.workspace_ids = portalData.workspace_ids || [];

    requestTypes = normalizeRequestTypes(bootstrap.request_types || []);
    assetReports = bootstrap.asset_reports || [];
    hasAssetSets = assetReports.length > 0;

    // Allow saves from user changes after initial load
    setTimeout(() => {
      isInitialLoad = false;
    }, 100);
  } catch (err) {
    console.error('Failed to load portal:', err);
    error = err.message || 'Failed to load portal';
  } finally {
    loading = false;
  }
}

/**
 * Toggle editing mode
 */
function toggleEditing() {
  const wasUsingManagementData = isEditing || showCustomizePanel;
  const wasEditing = isEditing;
  isEditing = !isEditing;

  // Close customize panel when entering edit mode
  if (!wasEditing && isEditing) {
    showCustomizePanel = false;
  }

  // Save changes when exiting edit mode
  if (wasEditing && !isEditing) {
    saveCustomizations();
  }

  const usesManagementData = isEditing || showCustomizePanel;
  if (portalData?.channel_id && usesManagementData !== wasUsingManagementData) {
    void loadAssetReports({ forCustomization: usesManagementData });
    void loadRequestTypes({ forCustomization: usesManagementData });
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
  // Clear background image when selecting a gradient (if selecting non-None)
  if (index > 0) {
    backgroundImageUrl = null;
  }
  saveCustomizations();
}

/**
 * Select a background image
 */
function selectBackgroundImage(url) {
  backgroundImageUrl = url;
  // Clear gradient when selecting a background image
  selectedGradient = 0;
  saveCustomizations();
}

/**
 * Remove background image
 */
function removeBackgroundImage() {
  backgroundImageUrl = null;
  saveCustomizations();
}

/**
 * Handle background image upload
 */
async function handleBackgroundUpload(files) {
  if (!files || files.length === 0) return;

  const file = files[0];
  if (!file.type.startsWith('image/')) {
    console.error('Please select an image file');
    return;
  }

  uploadingBackground = true;
  try {
    const uploadFormData = new FormData();
    uploadFormData.append('file', file);
    uploadFormData.append('category', 'portal_background');
    if (portalData?.channel_id) {
      uploadFormData.append('entity_id', String(portalData.channel_id));
    }

    const uploadResult = await api.attachments.upload(uploadFormData);

    if (uploadResult?.success && uploadResult.background_url) {
      selectBackgroundImage(uploadResult.background_url);
    }
  } catch (err) {
    console.error('Failed to upload portal background:', err);
  } finally {
    uploadingBackground = false;
  }
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
    uploadFormData.append('category', 'portal_logo');
    if (portalData?.channel_id) {
      uploadFormData.append('entity_id', String(portalData.channel_id));
    }

    const uploadResult = await api.attachments.upload(uploadFormData);

    if (uploadResult?.success && uploadResult.logo_url) {
      logoUrl = uploadResult.logo_url;
      saveCustomizations();
    }
  } catch (err) {
    console.error('Failed to upload portal logo:', err);
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
 * Parse Docmost share link to extract baseURL and shareID
 */
function parseDocmostShareLink(link) {
  if (!link?.trim()) {
    return { baseURL: '', shareID: '' };
  }

  try {
    const url = new URL(link.trim());
    const pathParts = url.pathname.split('/').filter((p) => p);

    if (pathParts.length >= 2 && pathParts[0] === 'share') {
      const shareID = pathParts[1];
      const baseURL = `${url.protocol}//${url.host}`;
      return { baseURL, shareID };
    }

    return { baseURL: '', shareID: '' };
  } catch (err) {
    console.error('Failed to parse Docmost share link:', err);
    return { baseURL: '', shareID: '' };
  }
}

/**
 * Build the portal channel config from the current customization state.
 * Shared by the debounced save and the explicit knowledge-base save so both
 * persistence paths produce identical configuration.
 */
function buildPortalConfig() {
  let workspaceIds = portalData.workspace_ids || [];
  if (workspaceIds.length === 0 && portalData.workspace_id && portalData.workspace_id > 0) {
    workspaceIds = [portalData.workspace_id];
  }

  const { baseURL, shareID } = parseDocmostShareLink(knowledgeBaseShareLink);

  return {
    portal_slug: portalData.slug,
    portal_workspace_ids: workspaceIds,
    portal_title: editableTitle,
    portal_description: editableDescription,
    portal_gradient: selectedGradient,
    portal_theme: isDarkMode ? 'dark' : 'light',
    portal_search_placeholder: editableSearchPlaceholder,
    portal_search_hint: editableSearchHint,
    portal_sections: portalSections,
    portal_footer_columns: footerColumns,
    knowledge_base_share_link: knowledgeBaseShareLink,
    knowledge_base_url: baseURL,
    knowledge_base_share_id: shareID,
    portal_background_image_url: backgroundImageUrl || '',
    portal_logo_url: logoUrl || '',
  };
}

/**
 * Save customizations (debounced)
 */
async function saveCustomizations() {
  // Allow internal main-app and portal sessions, never portal customers.
  const canCustomize =
    authStore.isAuthenticated || (portalAuthStore.isAuthenticated && portalAuthStore.isInternal);
  if (!portalData?.channel_id || !canCustomize) return;
  if (isInitialLoad) return;

  if (saveTimeout) clearTimeout(saveTimeout);

  saveTimeout = setTimeout(async () => {
    try {
      await api.channels.updateConfig(portalData.channel_id, buildPortalConfig());
    } catch (err) {
      console.error('Failed to save customizations:', err);
    }
  }, 1000);
}

/**
 * Save knowledge base configuration
 */
async function saveKnowledgeBaseConfig() {
  const canCustomize =
    authStore.isAuthenticated || (portalAuthStore.isAuthenticated && portalAuthStore.isInternal);
  if (!portalData?.channel_id || !canCustomize) {
    return;
  }

  try {
    await api.channels.updateConfig(portalData.channel_id, buildPortalConfig());
  } catch (err) {
    console.error('Failed to save knowledge base configuration:', err);
    errorToast(`Failed to save knowledge base configuration: ${err.message || err}`);
  }
}

/**
 * Load request types
 * Uses portal endpoint to properly handle portal customer authentication
 */
async function loadRequestTypes({ forCustomization = isEditing || showCustomizePanel } = {}) {
  if (!portalData || !currentSlug) return;

  const loadId = ++requestTypesLoadId;
  try {
    loadingRequestTypes = true;
    // Clear stale data while switching response shape.
    requestTypes = [];
    const types = forCustomization
      ? await api.requestTypes.getForChannel(portalData.channel_id)
      : await api.requestTypes.getForPortal(currentSlug);

    if (loadId !== requestTypesLoadId) return;
    // Field counts are internal-only.
    requestTypes = normalizeRequestTypes(types);
  } catch (err) {
    if (loadId !== requestTypesLoadId) return;
    requestTypes = [];
    console.error('Failed to load request types:', err);
  } finally {
    if (loadId === requestTypesLoadId) {
      loadingRequestTypes = false;
    }
  }
}

function normalizeRequestTypes(types, isInternal = null) {
  const showFieldCounts =
    isInternal ??
    (authStore.isAuthenticated || (portalAuthStore.isAuthenticated && portalAuthStore.isInternal));
  return types.map((requestType) => {
    const rawFieldCount = requestType._field_count ?? requestType.field_count ?? 0;
    return {
      ...requestType,
      _field_count: rawFieldCount,
      field_count: showFieldCounts ? rawFieldCount : 0,
    };
  });
}

function hydrateUserBootstrap(bootstrap) {
  portalActivityStore.hydrate(bootstrap);
  if (bootstrap?.authenticated) {
    requestTypes = normalizeRequestTypes(requestTypes, bootstrap.is_internal === true);
  }
}

/**
 * Load asset reports
 */
async function loadAssetReports({ forCustomization = isEditing || showCustomizePanel } = {}) {
  if (!portalData?.channel_id) return;

  const loadId = ++assetReportsLoadId;
  try {
    loadingAssetReports = true;
    // Clear stale data while switching response shape.
    assetReports = [];
    hasAssetSets = false;

    const response = forCustomization
      ? await api.assetReports.getForChannel(portalData.channel_id)
      : await api.assetReports.getForPortal(currentSlug);
    const reports = Array.isArray(response) ? response : [];
    const assetSetsExist =
      reports.length > 0 || (forCustomization && (await checkAssetSetsExist()));

    if (loadId !== assetReportsLoadId) return;
    assetReports = reports;
    hasAssetSets = assetSetsExist;
  } catch (err) {
    if (loadId !== assetReportsLoadId) return;
    assetReports = [];
    hasAssetSets = false;
    console.error('Failed to load asset reports:', err);
  } finally {
    if (loadId === assetReportsLoadId) {
      loadingAssetReports = false;
    }
  }
}

/**
 * Check if asset sets exist (to show/hide the asset reports section)
 */
async function checkAssetSetsExist() {
  try {
    const sets = await api.assetSets.getAll();
    return sets && sets.length > 0;
  } catch (_err) {
    return false;
  }
}

/**
 * Get asset reports for a section
 */
function getSectionAssetReports(section, inCustomizeMode = false) {
  const reportIds = section.asset_report_ids || [];
  return reportIds
    .map((id) => assetReports.find((ar) => ar.id === id))
    .filter((ar) => ar !== undefined)
    .filter((ar) => inCustomizeMode || ar.is_active !== false);
}

/**
 * Get request types for a section
 */
function getSectionRequestTypes(section, inCustomizeMode = false) {
  return section.request_type_ids
    .map((id) => requestTypes.find((rt) => rt.id === id))
    .filter((rt) => rt !== undefined)
    .filter((rt) => inCustomizeMode || rt.is_active);
}

// Portal Sections Management
function addSection() {
  const newSection = {
    id: crypto.randomUUID(),
    title: '',
    subtitle: '',
    display_order: portalSections.length,
    request_type_ids: [],
  };
  portalSections = [...portalSections, newSection];
  saveCustomizations();
  return newSection.id;
}

function deleteSection(sectionId) {
  portalSections = portalSections
    .filter((s) => s.id !== sectionId)
    .map((s, i) => ({ ...s, display_order: i }));
  saveCustomizations();
}

function updateSection(sectionId, field, value) {
  portalSections = portalSections.map((s) => {
    if (s.id === sectionId) {
      return { ...s, [field]: value };
    }
    return s;
  });
  saveCustomizations();
}

function moveSectionUp(index) {
  if (index === 0) return;
  const newSections = [...portalSections];
  [newSections[index - 1], newSections[index]] = [newSections[index], newSections[index - 1]];
  portalSections = newSections.map((s, i) => ({ ...s, display_order: i }));
  saveCustomizations();
}

function moveSectionDown(index) {
  if (index === portalSections.length - 1) return;
  const newSections = [...portalSections];
  [newSections[index], newSections[index + 1]] = [newSections[index + 1], newSections[index]];
  portalSections = newSections.map((s, i) => ({ ...s, display_order: i }));
  saveCustomizations();
}

function addRequestTypeToSection(sectionId, requestTypeId) {
  portalSections = portalSections.map((s) => {
    if (s.id === sectionId) {
      if (!s.request_type_ids.includes(requestTypeId)) {
        return {
          ...s,
          request_type_ids: [...s.request_type_ids, requestTypeId],
        };
      }
    }
    return s;
  });
  saveCustomizations();
}

function removeRequestTypeFromSection(sectionId, requestTypeId) {
  portalSections = portalSections.map((s) => {
    if (s.id === sectionId) {
      return {
        ...s,
        request_type_ids: s.request_type_ids.filter((id) => id !== requestTypeId),
      };
    }
    return s;
  });
  saveCustomizations();
}

function addAssetReportToSection(sectionId, reportId) {
  portalSections = portalSections.map((s) => {
    if (s.id === sectionId) {
      const currentIds = s.asset_report_ids || [];
      if (!currentIds.includes(reportId)) {
        return {
          ...s,
          asset_report_ids: [...currentIds, reportId],
        };
      }
    }
    return s;
  });
  saveCustomizations();
}

function removeAssetReportFromSection(sectionId, reportId) {
  portalSections = portalSections.map((s) => {
    if (s.id === sectionId) {
      return {
        ...s,
        asset_report_ids: (s.asset_report_ids || []).filter((id) => id !== reportId),
      };
    }
    return s;
  });
  saveCustomizations();
}

// Footer management — see ./footerLinks.js for the shared implementation.
const { addFooterLink, removeFooterLink, updateColumnTitle, updateFooterLink } =
  createFooterLinkHelpers({
    setColumns: (updater) => {
      footerColumns = updater(footerColumns);
    },
    saveCustomizations,
  });

// Menu functions
function closeAllMenus() {
  showProfileMenu = false;
  showMainMenu = false;
}

// Reset store (for cleanup)
function reset() {
  assetReportsLoadId++;
  requestTypesLoadId++;
  portalData = null;
  loading = true;
  error = null;
  currentSlug = null;
  isEditing = false;
  isDarkMode = false;
  showCustomizePanel = false;
  selectedGradient = 0;
  activeSection = 'hero-gradient';
  backgroundImageUrl = null;
  uploadingBackground = false;
  selectedBackgroundCategory = 'abstract';
  logoUrl = null;
  hubLogoUrl = null;
  uploadingLogo = false;
  showProfileMenu = false;
  showMainMenu = false;
  showLoginDialog = false;
  editableTitle = '';
  editableDescription = '';
  editableSearchPlaceholder = 'Search the knowledge base...';
  editableSearchHint = 'Search for articles, guides, and answers to common questions';
  requestTypes = [];
  loadingRequestTypes = false;
  assetReports = [];
  loadingAssetReports = false;
  hasAssetSets = false;
  portalSections = [];
  footerColumns = [
    { title: '', links: [] },
    { title: '', links: [] },
    { title: '', links: [] },
  ];
  knowledgeBaseShareLink = '';
  portalSearchStore.reset();
  portalActivityStore.reset();
  pendingRequestType = null;
  isInitialLoad = true;
}

// Focused shell and customization state.
export const portalCustomizationStore = {
  get portalData() {
    return portalData;
  },
  get loading() {
    return loading;
  },
  get error() {
    return error;
  },
  get currentSlug() {
    return currentSlug;
  },
  get isEditing() {
    return isEditing;
  },
  set isEditing(value) {
    isEditing = value;
  },
  get isDarkMode() {
    return isDarkMode;
  },
  get showCustomizePanel() {
    return showCustomizePanel;
  },
  set showCustomizePanel(value) {
    const wasUsingManagementData = isEditing || showCustomizePanel;
    const shouldShowCustomizePanel = Boolean(value);

    if (shouldShowCustomizePanel) {
      showCustomizePanel = true;
      isEditing = true;
    } else {
      if (isEditing) saveCustomizations();
      isEditing = false;
      showCustomizePanel = false;
    }

    const usesManagementData = isEditing || showCustomizePanel;
    if (portalData?.channel_id && usesManagementData !== wasUsingManagementData) {
      void loadAssetReports({ forCustomization: usesManagementData });
      void loadRequestTypes({ forCustomization: usesManagementData });
    }
  },
  get selectedGradient() {
    return selectedGradient;
  },
  get activeSection() {
    return activeSection;
  },
  set activeSection(value) {
    activeSection = value;
  },
  get backgroundImageUrl() {
    return backgroundImageUrl;
  },
  get uploadingBackground() {
    return uploadingBackground;
  },
  get selectedBackgroundCategory() {
    return selectedBackgroundCategory;
  },
  set selectedBackgroundCategory(value) {
    selectedBackgroundCategory = value;
  },
  get hasBackgroundImage() {
    return backgroundImageUrl !== null && backgroundImageUrl !== '';
  },
  get hasGradient() {
    return !backgroundImageUrl && selectedGradient > 0 && gradients[selectedGradient]?.value;
  },
  get headerBackgroundStyle() {
    const safeUrl = safeCssUrl(backgroundImageUrl);
    if (safeUrl) {
      return `background: linear-gradient(rgba(0,0,0,0.4), rgba(0,0,0,0.4)), url("${safeUrl}") center/cover no-repeat;`;
    }
    return `background: ${gradients[selectedGradient]?.value || gradients[1].value};`;
  },
  get logoUrl() {
    return logoUrl;
  },
  get hubLogoUrl() {
    return hubLogoUrl;
  },
  get uploadingLogo() {
    return uploadingLogo;
  },
  get effectiveLogoUrl() {
    return logoUrl || hubLogoUrl;
  },
  get showProfileMenu() {
    return showProfileMenu;
  },
  set showProfileMenu(value) {
    showProfileMenu = value;
  },
  get showMainMenu() {
    return showMainMenu;
  },
  set showMainMenu(value) {
    showMainMenu = value;
  },
  get showLoginDialog() {
    return showLoginDialog;
  },
  set showLoginDialog(value) {
    showLoginDialog = value;
  },
  get editableTitle() {
    return editableTitle;
  },
  set editableTitle(value) {
    editableTitle = value;
  },
  get editableDescription() {
    return editableDescription;
  },
  set editableDescription(value) {
    editableDescription = value;
  },
  get editableSearchPlaceholder() {
    return editableSearchPlaceholder;
  },
  set editableSearchPlaceholder(value) {
    editableSearchPlaceholder = value;
  },
  get editableSearchHint() {
    return editableSearchHint;
  },
  set editableSearchHint(value) {
    editableSearchHint = value;
  },
  get footerColumns() {
    return footerColumns;
  },
  get knowledgeBaseShareLink() {
    return knowledgeBaseShareLink;
  },
  set knowledgeBaseShareLink(value) {
    knowledgeBaseShareLink = value;
  },
  get pendingRequestType() {
    return pendingRequestType;
  },
  set pendingRequestType(value) {
    pendingRequestType = value;
  },
  loadPortal,
  hydrateUserBootstrap,
  toggleEditing,
  toggleTheme,
  selectGradient,
  saveCustomizations,
  saveKnowledgeBaseConfig,
  parseDocmostShareLink,
  selectBackgroundImage,
  removeBackgroundImage,
  handleBackgroundUpload,
  handleLogoUpload,
  removeLogo,
  addFooterLink,
  removeFooterLink,
  updateColumnTitle,
  updateFooterLink,
  closeAllMenus,
  reset,
};

// Focused catalog and section state.
export const portalCatalogStore = {
  get requestTypes() {
    return requestTypes;
  },
  get loadingRequestTypes() {
    return loadingRequestTypes;
  },
  get assetReports() {
    return assetReports;
  },
  get loadingAssetReports() {
    return loadingAssetReports;
  },
  get hasAssetSets() {
    return hasAssetSets;
  },
  get portalSections() {
    return portalSections;
  },
  get draggedRequestType() {
    return draggedRequestType;
  },
  set draggedRequestType(value) {
    draggedRequestType = value;
  },
  get draggedAssetReport() {
    return draggedAssetReport;
  },
  set draggedAssetReport(value) {
    draggedAssetReport = value;
  },
  loadRequestTypes,
  getSectionRequestTypes,
  loadAssetReports,
  getSectionAssetReports,
  addSection,
  deleteSection,
  updateSection,
  moveSectionUp,
  moveSectionDown,
  addRequestTypeToSection,
  removeRequestTypeFromSection,
  addAssetReportToSection,
  removeAssetReportFromSection,
};

const compatibilityAliases = {
  searchQuery: [portalSearchStore, 'query'],
  showSearchResults: [portalSearchStore, 'visible'],
  searchResults: [portalSearchStore, 'results'],
  searchLoading: [portalSearchStore, 'loading'],
  searchError: [portalSearchStore, 'error'],
  performSearch: [portalSearchStore, 'search'],
  debouncedSearch: [portalSearchStore, 'searchDebounced'],
  closeSearchResults: [portalSearchStore, 'close'],
  showMyRequests: [portalRequestsStore, 'visible'],
  myRequests: [portalRequestsStore, 'requests'],
  openRequestCount: [portalRequestsStore, 'openCount'],
  loadingRequests: [portalRequestsStore, 'loading'],
  selectedRequest: [portalRequestsStore, 'selected'],
  requestComments: [portalRequestsStore, 'comments'],
  loadingComments: [portalRequestsStore, 'loadingComments'],
  newCommentContent: [portalRequestsStore, 'newComment'],
  addingComment: [portalRequestsStore, 'addingComment'],
  loadMyRequests: [portalRequestsStore, 'load'],
  viewRequest: [portalRequestsStore, 'view'],
  loadRequestComments: [portalRequestsStore, 'loadComments'],
  addComment: [portalRequestsStore, 'addComment'],
  closeRequestDetail: [portalRequestsStore, 'closeDetail'],
  toggleMyRequests: [portalRequestsStore, 'toggle'],
  setShowMyRequests: [portalRequestsStore, 'setVisible'],
  loadAndViewRequest: [portalRequestsStore, 'loadAndView'],
  showMyDrafts: [portalDraftsStore, 'visible'],
  myDrafts: [portalDraftsStore, 'drafts'],
  loadingDrafts: [portalDraftsStore, 'loading'],
  draftCount: [portalDraftsStore, 'count'],
  loadMyDrafts: [portalDraftsStore, 'load'],
  deleteDraft: [portalDraftsStore, 'delete'],
  toggleMyDrafts: [portalDraftsStore, 'toggle'],
  setShowMyDrafts: [portalDraftsStore, 'setVisible'],
  showMyApprovals: [portalApprovalsStore, 'visible'],
  myApprovals: [portalApprovalsStore, 'approvals'],
  pendingApprovalCount: [portalApprovalsStore, 'pendingCount'],
  loadingApprovals: [portalApprovalsStore, 'loading'],
  selectedApproval: [portalApprovalsStore, 'selected'],
  selectedApprovalRequest: [portalApprovalsStore, 'selectedRequest'],
  loadingApprovalDetail: [portalApprovalsStore, 'loadingDetail'],
  approvalComment: [portalApprovalsStore, 'comment'],
  decidingApproval: [portalApprovalsStore, 'deciding'],
  loadMyApprovals: [portalApprovalsStore, 'load'],
  viewApproval: [portalApprovalsStore, 'view'],
  loadAndViewApproval: [portalApprovalsStore, 'loadAndView'],
  decideApproval: [portalApprovalsStore, 'decide'],
  closeApprovalDetail: [portalApprovalsStore, 'closeDetail'],
  toggleMyApprovals: [portalApprovalsStore, 'toggle'],
  setShowMyApprovals: [portalApprovalsStore, 'setVisible'],
};

const compatibilityDomains = [portalCustomizationStore, portalCatalogStore];

// Transitional compatibility facade. New consumers should import a focused store.
export const portalStore = new Proxy(
  {},
  {
    get(_target, property) {
      const alias = compatibilityAliases[property];
      if (alias) return alias[0][alias[1]];
      const domain = compatibilityDomains.find((store) => property in store);
      return domain?.[property];
    },
    set(_target, property, value) {
      const alias = compatibilityAliases[property];
      if (alias) {
        alias[0][alias[1]] = value;
        return true;
      }
      const domain = compatibilityDomains.find((store) => property in store);
      if (!domain) return false;
      domain[property] = value;
      return true;
    },
  }
);
