/**
 * Portal store for managing portal page state
 * Uses Svelte 5 runes pattern following theme.svelte.js
 */

import { api } from '../api.js';
import { authStore, attachmentStatus } from '../stores';
import { navigate } from '../router.js';
import { portalAuthStore } from './portalAuth.svelte.js';
import { gradients } from '../utils/gradients.js';

// Re-export gradients for backward compatibility
export { gradients };

// Icon mapping for request types
import {
  Target, Zap, BookOpen, CheckSquare, Bug, Minus, Star, Flag, Lightbulb,
  Settings, User, Users, Calendar, Clock, MapPin, Search, Filter, Tag,
  Bookmark, Heart, Shield, Key, Lock, Globe, Wifi, Database, Server,
  Code, Terminal, FileText, Folder, Image, Video, Music, Download,
  Upload, Send, Mail, Phone, MessageSquare, AlertCircle, Info,
  CheckCircle, XCircle, HelpCircle, Archive, Trash, Edit, Copy,
  Scissors, Paperclip, Link, ExternalLink, Package, Building,
  Rocket, Award, Bell, Camera, Coffee, Compass, Feather, Gift, Home,
  Layers, Map as MapIcon, Megaphone, Monitor, Pen, Printer, RefreshCw, Save, Smile,
  Wrench, Truck, Volume2, Watch, Briefcase, Cloud, BarChart
} from 'lucide-svelte';

// Icon map export for components
export const iconMap = {
  Target, Zap, BookOpen, CheckSquare, Bug, Minus, Star, Flag, Lightbulb,
  Settings, User, Users, Calendar, Clock, MapPin, Search, Filter, Tag,
  Bookmark, Heart, Shield, Key, Lock, Globe, Wifi, Database, Server,
  Code, Terminal, FileText, Folder, Image, Video, Music, Download,
  Upload, Send, Mail, Phone, MessageSquare, AlertCircle, Info,
  CheckCircle, XCircle, HelpCircle, Archive, Trash, Edit, Copy,
  Scissors, Paperclip, Link, ExternalLink, Package, Building,
  Rocket, Award, Bell, Camera, Coffee, Compass, Feather, Gift, Home,
  Layers, Map: MapIcon, Megaphone, Monitor, Pen, Printer, RefreshCw, Save, Smile,
  Wrench, Truck, Volume2, Watch, Briefcase, Cloud, BarChart
};

// Core state
let portalData = $state(null);
let loading = $state(true);
let error = $state(null);
let currentSlug = $state(null);

// UI state
let isEditing = $state(false);
let isDarkMode = $state(false);
let showCustomizePanel = $state(false);
let showMyRequests = $state(false);
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

// Asset reports state
let assetReports = $state([]);
let loadingAssetReports = $state(false);
let hasAssetSets = $state(false);

// Portal sections
let portalSections = $state([]);

// Drag-and-drop state
let draggedRequestType = $state(null);
let draggedAssetReport = $state(null);

// Footer columns
let footerColumns = $state([
  { title: '', links: [] },
  { title: '', links: [] },
  { title: '', links: [] }
]);

// Knowledge base
let knowledgeBaseShareLink = $state('');

// Search state
let searchQuery = $state('');
let showSearchResults = $state(false);
let searchResults = $state([]);
let searchLoading = $state(false);
let searchError = $state(null);

// My Requests state
let myRequests = $state([]);
let loadingRequests = $state(false);
let selectedRequest = $state(null);
let requestComments = $state([]);
let loadingComments = $state(false);
let newCommentContent = $state('');
let addingComment = $state(false);

// Pending request type (for opening form after login)
let pendingRequestType = $state(null);

// Internal state
let isInitialLoad = true;
let saveTimeout = null;
let searchTimeout = null;
let isLoadingRequestTypes = false;

/**
 * Load portal data by slug
 */
async function loadPortal(slug) {
  try {
    loading = true;
    error = null;
    currentSlug = slug;

    if (!slug) {
      error = 'Portal not specified';
      return;
    }

    portalData = await api.portal.get(slug);

    // Initialize editable state with portal data
    editableTitle = portalData.title || 'Support Portal';
    editableDescription = portalData.description || '';

    // Load customization data
    selectedGradient = portalData.gradient || 0;
    isDarkMode = portalData.theme === 'dark';
    editableSearchPlaceholder = portalData.search_placeholder || 'Search the knowledge base...';
    editableSearchHint = portalData.search_hint || 'Search for articles, guides, and answers to common questions';
    backgroundImageUrl = portalData.background_image_url || null;
    logoUrl = portalData.logo_url || null;
    hubLogoUrl = portalData.hub_logo_url || null;

    // Load footer columns
    footerColumns = portalData.footer_columns || [
      { title: '', links: [] },
      { title: '', links: [] },
      { title: '', links: [] }
    ];

    // Load portal sections
    portalSections = portalData.sections || [];

    // Load knowledge base configuration
    knowledgeBaseShareLink = portalData.knowledge_base_share_link || '';

    // Ensure workspace_ids is always an array
    portalData.workspace_ids = portalData.workspace_ids || [];

    // Load request types and asset reports for rendering sections
    if (portalData.channel_id) {
      await Promise.all([
        loadRequestTypes(),
        loadAssetReports()
      ]);
    }

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

    const response = await fetch('/api/attachments/upload', {
      method: 'POST',
      body: uploadFormData,
    });

    if (!response.ok) {
      throw new Error(`Upload failed: ${response.statusText}`);
    }

    const uploadResult = await response.json();

    if (uploadResult && uploadResult.success && uploadResult.background_url) {
      selectBackgroundImage(uploadResult.background_url);
      console.log('Portal background uploaded successfully');
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

    const response = await fetch('/api/attachments/upload', {
      method: 'POST',
      body: uploadFormData,
    });

    if (!response.ok) {
      throw new Error(`Upload failed: ${response.statusText}`);
    }

    const uploadResult = await response.json();

    if (uploadResult && uploadResult.success && uploadResult.logo_url) {
      logoUrl = uploadResult.logo_url;
      saveCustomizations();
      console.log('Portal logo uploaded successfully');
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
  if (!link || !link.trim()) {
    return { baseURL: '', shareID: '' };
  }

  try {
    const url = new URL(link.trim());
    const pathParts = url.pathname.split('/').filter(p => p);

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
 * Save customizations (debounced)
 */
async function saveCustomizations() {
  if (!portalData || !portalData.channel_id || !authStore.isAuthenticated) return;
  if (isInitialLoad) return;

  if (saveTimeout) clearTimeout(saveTimeout);

  saveTimeout = setTimeout(async () => {
    try {
      let workspaceIds = portalData.workspace_ids || [];
      if (workspaceIds.length === 0 && portalData.workspace_id && portalData.workspace_id > 0) {
        workspaceIds = [portalData.workspace_id];
      }

      const { baseURL, shareID } = parseDocmostShareLink(knowledgeBaseShareLink);

      const config = {
        portal_slug: portalData.slug,
        portal_workspace_ids: workspaceIds,
        portal_enabled: true,
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

      await api.channels.updateConfig(portalData.channel_id, config);
      console.log('Portal customizations saved');
    } catch (err) {
      console.error('Failed to save customizations:', err);
    }
  }, 1000);
}

/**
 * Save knowledge base configuration
 */
async function saveKnowledgeBaseConfig() {
  if (!portalData || !portalData.channel_id || !authStore.isAuthenticated) {
    console.log('Cannot save: missing portal data, channel ID, or not authenticated');
    return;
  }

  const { baseURL, shareID } = parseDocmostShareLink(knowledgeBaseShareLink);

  try {
    let workspaceIds = portalData.workspace_ids || [];
    if (workspaceIds.length === 0 && portalData.workspace_id && portalData.workspace_id > 0) {
      workspaceIds = [portalData.workspace_id];
    }

    const config = {
      portal_slug: portalData.slug,
      portal_workspace_ids: workspaceIds,
      portal_enabled: true,
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

    await api.channels.updateConfig(portalData.channel_id, config);
    console.log('Knowledge base configuration saved successfully');
  } catch (err) {
    console.error('Failed to save knowledge base configuration:', err);
    alert('Failed to save knowledge base configuration: ' + (err.message || err));
  }
}

/**
 * Load request types
 */
async function loadRequestTypes() {
  if (!portalData || !portalData.channel_id) return;
  if (isLoadingRequestTypes) return;

  try {
    isLoadingRequestTypes = true;
    loadingRequestTypes = true;
    const types = await api.requestTypes.getForChannel(portalData.channel_id);

    // Fetch field counts in parallel
    const typesWithFields = await Promise.all(
      types.map(async (rt) => {
        try {
          const fields = await api.requestTypes.getFields(rt.id);
          return { ...rt, field_count: fields.length };
        } catch (err) {
          console.error(`Failed to load fields for request type ${rt.id}:`, err);
          return { ...rt, field_count: 0 };
        }
      })
    );

    requestTypes = typesWithFields;
  } catch (err) {
    console.error('Failed to load request types:', err);
  } finally {
    loadingRequestTypes = false;
    isLoadingRequestTypes = false;
  }
}

/**
 * Load asset reports
 */
async function loadAssetReports() {
  if (!portalData || !portalData.channel_id) return;

  try {
    loadingAssetReports = true;
    const reports = await api.assetReports.getForChannel(portalData.channel_id);
    assetReports = reports;
    hasAssetSets = reports.length > 0 || await checkAssetSetsExist();
  } catch (err) {
    console.error('Failed to load asset reports:', err);
  } finally {
    loadingAssetReports = false;
  }
}

/**
 * Check if asset sets exist (to show/hide the asset reports section)
 */
async function checkAssetSetsExist() {
  try {
    const sets = await api.assetSets.getAll();
    return sets && sets.length > 0;
  } catch (err) {
    return false;
  }
}

/**
 * Get asset reports for a section
 */
function getSectionAssetReports(section, inCustomizeMode = false) {
  const reportIds = section.asset_report_ids || [];
  return reportIds
    .map(id => assetReports.find(ar => ar.id === id))
    .filter(ar => ar !== undefined)
    .filter(ar => inCustomizeMode || ar.is_active);
}

/**
 * Get request types for a section
 */
function getSectionRequestTypes(section, inCustomizeMode = false) {
  return section.request_type_ids
    .map(id => requestTypes.find(rt => rt.id === id))
    .filter(rt => rt !== undefined)
    .filter(rt => inCustomizeMode || rt.is_active);
}

// Portal Sections Management
function addSection() {
  const newSection = {
    id: crypto.randomUUID(),
    title: '',
    subtitle: '',
    display_order: portalSections.length,
    request_type_ids: []
  };
  portalSections = [...portalSections, newSection];
  saveCustomizations();
  return newSection.id;
}

function deleteSection(sectionId) {
  portalSections = portalSections
    .filter(s => s.id !== sectionId)
    .map((s, i) => ({ ...s, display_order: i }));
  saveCustomizations();
}

function updateSection(sectionId, field, value) {
  portalSections = portalSections.map(s => {
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
  portalSections = portalSections.map(s => {
    if (s.id === sectionId) {
      if (!s.request_type_ids.includes(requestTypeId)) {
        return {
          ...s,
          request_type_ids: [...s.request_type_ids, requestTypeId]
        };
      }
    }
    return s;
  });
  saveCustomizations();
}

function removeRequestTypeFromSection(sectionId, requestTypeId) {
  portalSections = portalSections.map(s => {
    if (s.id === sectionId) {
      return {
        ...s,
        request_type_ids: s.request_type_ids.filter(id => id !== requestTypeId)
      };
    }
    return s;
  });
  saveCustomizations();
}

function addAssetReportToSection(sectionId, reportId) {
  portalSections = portalSections.map(s => {
    if (s.id === sectionId) {
      const currentIds = s.asset_report_ids || [];
      if (!currentIds.includes(reportId)) {
        return {
          ...s,
          asset_report_ids: [...currentIds, reportId]
        };
      }
    }
    return s;
  });
  saveCustomizations();
}

function removeAssetReportFromSection(sectionId, reportId) {
  portalSections = portalSections.map(s => {
    if (s.id === sectionId) {
      return {
        ...s,
        asset_report_ids: (s.asset_report_ids || []).filter(id => id !== reportId)
      };
    }
    return s;
  });
  saveCustomizations();
}

// Footer management
function addFooterLink(columnIndex) {
  footerColumns = footerColumns.map((col, idx) => {
    if (idx === columnIndex) {
      return {
        ...col,
        links: [...col.links, { text: '', url: '' }]
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
        links: col.links.filter((_, i) => i !== linkIndex)
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
        })
      };
    }
    return col;
  });
  saveCustomizations();
}

// Search functions
async function performSearch() {
  if (!searchQuery.trim()) return;

  if (!knowledgeBaseShareLink) {
    searchError = 'Knowledge base not configured';
    showSearchResults = true;
    return;
  }

  try {
    searchLoading = true;
    searchError = null;
    searchResults = [];
    showSearchResults = true;

    const results = await api.portal.searchKnowledgeBase(portalData.slug, searchQuery);
    searchResults = results;
  } catch (err) {
    console.error('Failed to search knowledge base:', err);
    searchError = err.message || 'Failed to search knowledge base';
  } finally {
    searchLoading = false;
  }
}

function debouncedSearch() {
  if (searchTimeout) clearTimeout(searchTimeout);

  if (searchQuery.length < 3) {
    showSearchResults = false;
    return;
  }

  searchTimeout = setTimeout(() => {
    performSearch();
  }, 400);
}

function closeSearchResults() {
  showSearchResults = false;
}

// My Requests functions
async function loadMyRequests() {
  if (!authStore.isAuthenticated || !currentSlug) return;

  try {
    loadingRequests = true;
    myRequests = await api.portal.getMyRequests(currentSlug);
  } catch (err) {
    console.error('Failed to load requests:', err);
  } finally {
    loadingRequests = false;
  }
}

async function viewRequest(request) {
  selectedRequest = request;
  // Update URL with request ID
  navigate(`/portal/${currentSlug}?view=requests&id=${request.id}`);
  await loadRequestComments(request.id);
}

async function loadRequestComments(itemId) {
  if (!currentSlug) return;

  try {
    loadingComments = true;
    requestComments = await api.portal.getRequestComments(currentSlug, itemId);
  } catch (err) {
    console.error('Failed to load comments:', err);
  } finally {
    loadingComments = false;
  }
}

async function addComment() {
  if (!newCommentContent.trim() || !selectedRequest || !currentSlug) return;

  try {
    addingComment = true;
    const comment = await api.portal.addRequestComment(currentSlug, selectedRequest.id, newCommentContent);
    requestComments = [...requestComments, comment];
    newCommentContent = '';
  } catch (err) {
    console.error('Failed to add comment:', err);
    alert('Failed to add comment. Please try again.');
  } finally {
    addingComment = false;
  }
}

function closeRequestDetail() {
  selectedRequest = null;
  requestComments = [];
  newCommentContent = '';
  // Return to requests list URL
  navigate(`/portal/${currentSlug}?view=requests`);
}

/**
 * Set showMyRequests directly without navigation (for URL sync)
 */
function setShowMyRequests(value) {
  showMyRequests = value;
  if (value && (authStore.isAuthenticated || portalAuthStore.isAuthenticated)) {
    loadMyRequests();
  }
  if (!value) {
    selectedRequest = null;
  }
}

/**
 * Load a specific request by ID and view it (for URL sync)
 */
async function loadAndViewRequest(requestId) {
  if (!currentSlug) return;
  try {
    const request = await api.portal.getRequestDetail(currentSlug, requestId);
    selectedRequest = request;
    await loadRequestComments(request.id);
  } catch (err) {
    console.error('Failed to load request:', err);
  }
}

async function toggleMyRequests() {
  showMyRequests = !showMyRequests;
  showProfileMenu = false;

  // Update URL
  if (showMyRequests) {
    navigate(`/portal/${currentSlug}?view=requests`);
    if (authStore.isAuthenticated || portalAuthStore.isAuthenticated) {
      await loadMyRequests();
    }
  } else {
    navigate(`/portal/${currentSlug}`);
  }

  if (!showMyRequests) {
    selectedRequest = null;
    requestComments = [];
  }
}

// Menu functions
function closeAllMenus() {
  showProfileMenu = false;
  showMainMenu = false;
}

// Reset store (for cleanup)
function reset() {
  portalData = null;
  loading = true;
  error = null;
  currentSlug = null;
  isEditing = false;
  isDarkMode = false;
  showCustomizePanel = false;
  showMyRequests = false;
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
    { title: '', links: [] }
  ];
  knowledgeBaseShareLink = '';
  searchQuery = '';
  showSearchResults = false;
  searchResults = [];
  searchLoading = false;
  searchError = null;
  myRequests = [];
  loadingRequests = false;
  selectedRequest = null;
  requestComments = [];
  loadingComments = false;
  newCommentContent = '';
  addingComment = false;
  pendingRequestType = null;
  isInitialLoad = true;
}

// Export the store with getters and actions
export const portalStore = {
  // Getters for core state
  get portalData() { return portalData; },
  get loading() { return loading; },
  get error() { return error; },
  get currentSlug() { return currentSlug; },

  // Getters for UI state
  get isEditing() { return isEditing; },
  get isDarkMode() { return isDarkMode; },
  get showCustomizePanel() { return showCustomizePanel; },
  get showMyRequests() { return showMyRequests; },
  get selectedGradient() { return selectedGradient; },
  get activeSection() { return activeSection; },

  // Getters for background image state
  get backgroundImageUrl() { return backgroundImageUrl; },
  get uploadingBackground() { return uploadingBackground; },
  get selectedBackgroundCategory() { return selectedBackgroundCategory; },
  get hasBackgroundImage() { return backgroundImageUrl !== null && backgroundImageUrl !== ''; },
  get hasGradient() { return !backgroundImageUrl && selectedGradient > 0 && gradients[selectedGradient]?.value; },

  // Getters for logo state
  get logoUrl() { return logoUrl; },
  get hubLogoUrl() { return hubLogoUrl; },
  get uploadingLogo() { return uploadingLogo; },
  get effectiveLogoUrl() { return logoUrl || hubLogoUrl; }, // Portal logo with hub fallback

  // Getters for menu states
  get showProfileMenu() { return showProfileMenu; },
  get showMainMenu() { return showMainMenu; },
  get showLoginDialog() { return showLoginDialog; },

  // Getters for editable content
  get editableTitle() { return editableTitle; },
  get editableDescription() { return editableDescription; },
  get editableSearchPlaceholder() { return editableSearchPlaceholder; },
  get editableSearchHint() { return editableSearchHint; },

  // Getters for request types
  get requestTypes() { return requestTypes; },
  get loadingRequestTypes() { return loadingRequestTypes; },

  // Getters for asset reports
  get assetReports() { return assetReports; },
  get loadingAssetReports() { return loadingAssetReports; },
  get hasAssetSets() { return hasAssetSets; },

  // Getters for sections/footer
  get portalSections() { return portalSections; },
  get footerColumns() { return footerColumns; },
  get draggedRequestType() { return draggedRequestType; },
  get draggedAssetReport() { return draggedAssetReport; },

  // Getters for knowledge base
  get knowledgeBaseShareLink() { return knowledgeBaseShareLink; },

  // Getters for search
  get searchQuery() { return searchQuery; },
  get showSearchResults() { return showSearchResults; },
  get searchResults() { return searchResults; },
  get searchLoading() { return searchLoading; },
  get searchError() { return searchError; },

  // Getters for my requests
  get myRequests() { return myRequests; },
  get loadingRequests() { return loadingRequests; },
  get selectedRequest() { return selectedRequest; },
  get requestComments() { return requestComments; },
  get loadingComments() { return loadingComments; },
  get newCommentContent() { return newCommentContent; },
  get addingComment() { return addingComment; },
  get pendingRequestType() { return pendingRequestType; },

  // Setters for UI state
  set isEditing(value) { isEditing = value; },
  set showCustomizePanel(value) { showCustomizePanel = value; },
  set showMyRequests(value) { showMyRequests = value; },
  set activeSection(value) { activeSection = value; },
  set showProfileMenu(value) { showProfileMenu = value; },
  set showMainMenu(value) { showMainMenu = value; },
  set showLoginDialog(value) { showLoginDialog = value; },
  set draggedRequestType(value) { draggedRequestType = value; },
  set draggedAssetReport(value) { draggedAssetReport = value; },
  set selectedBackgroundCategory(value) { selectedBackgroundCategory = value; },

  // Setters for editable content
  set editableTitle(value) { editableTitle = value; },
  set editableDescription(value) { editableDescription = value; },
  set editableSearchPlaceholder(value) { editableSearchPlaceholder = value; },
  set editableSearchHint(value) { editableSearchHint = value; },

  // Setters for knowledge base
  set knowledgeBaseShareLink(value) { knowledgeBaseShareLink = value; },

  // Setters for search
  set searchQuery(value) { searchQuery = value; },

  // Setters for my requests
  set newCommentContent(value) { newCommentContent = value; },
  set pendingRequestType(value) { pendingRequestType = value; },

  // Actions
  loadPortal,
  toggleEditing,
  toggleTheme,
  selectGradient,
  saveCustomizations,
  saveKnowledgeBaseConfig,
  loadRequestTypes,
  getSectionRequestTypes,
  parseDocmostShareLink,

  // Asset report actions
  loadAssetReports,
  getSectionAssetReports,

  // Background image actions
  selectBackgroundImage,
  removeBackgroundImage,
  handleBackgroundUpload,

  // Logo actions
  handleLogoUpload,
  removeLogo,

  // Section actions
  addSection,
  deleteSection,
  updateSection,
  moveSectionUp,
  moveSectionDown,
  addRequestTypeToSection,
  removeRequestTypeFromSection,
  addAssetReportToSection,
  removeAssetReportFromSection,

  // Footer actions
  addFooterLink,
  removeFooterLink,
  updateColumnTitle,
  updateFooterLink,

  // Search actions
  performSearch,
  debouncedSearch,
  closeSearchResults,

  // My Requests actions
  loadMyRequests,
  viewRequest,
  loadRequestComments,
  addComment,
  closeRequestDetail,
  toggleMyRequests,
  setShowMyRequests,
  loadAndViewRequest,

  // Menu actions
  closeAllMenus,

  // Reset
  reset,
};
