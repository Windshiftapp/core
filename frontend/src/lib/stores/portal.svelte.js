/**
 * Portal store for managing portal page state
 * Uses Svelte 5 runes pattern following theme.svelte.js
 */

import { api } from '../api.js';
import { authStore } from '../stores';

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

// Gradient presets
export const gradients = [
  { name: 'Blue to Purple', value: 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)' },
  { name: 'Deep Ocean', value: 'linear-gradient(135deg, #2E3192 0%, #1BFFFF 100%)' },
  { name: 'Sunset Warmth', value: 'linear-gradient(135deg, #FF6B6B 0%, #FFE66D 100%)' },
  { name: 'Forest Green', value: 'linear-gradient(135deg, #11998e 0%, #38ef7d 100%)' },
  { name: 'Deep Night', value: 'linear-gradient(135deg, #1e3c72 0%, #2a5298 100%)' },
  { name: 'Pink Blush', value: 'linear-gradient(135deg, #ec008c 0%, #fc6767 100%)' },
  { name: 'Teal to Cyan', value: 'linear-gradient(135deg, #13547a 0%, #80d0c7 100%)' },
  { name: 'Royal Purple', value: 'linear-gradient(135deg, #6a11cb 0%, #2575fc 100%)' },
  { name: 'Fire Orange', value: 'linear-gradient(135deg, #f83600 0%, #f9d423 100%)' },
  { name: 'Cool Blues', value: 'linear-gradient(135deg, #2b5876 0%, #4e4376 100%)' },
  { name: 'Rose Garden', value: 'linear-gradient(135deg, #ee0979 0%, #ff6a00 100%)' },
  { name: 'Midnight', value: 'linear-gradient(135deg, #000428 0%, #004e92 100%)' },
  { name: 'Emerald Water', value: 'linear-gradient(135deg, #348f50 0%, #56b4d3 100%)' },
  { name: 'Peach', value: 'linear-gradient(135deg, #ed4264 0%, #ffedbc 100%)' },
  { name: 'Purple Dream', value: 'linear-gradient(135deg, #bf5ae0 0%, #a811da 100%)' },
  { name: 'Cosmic', value: 'linear-gradient(135deg, #ff0844 0%, #ffb199 100%)' },
  { name: 'Sea Breeze', value: 'linear-gradient(135deg, #4facfe 0%, #00f2fe 100%)' },
  { name: 'Autumn', value: 'linear-gradient(135deg, #fa8bff 0%, #2bd2ff 90%, #2bff88 100%)' },
];

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

// Portal sections
let portalSections = $state([]);

// Drag-and-drop state
let draggedRequestType = $state(null);

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

    // Load request types for rendering sections
    if (portalData.channel_id) {
      await loadRequestTypes();
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
    requestTypes = await api.requestTypes.getForChannel(portalData.channel_id);
  } catch (err) {
    console.error('Failed to load request types:', err);
  } finally {
    loadingRequestTypes = false;
    isLoadingRequestTypes = false;
  }
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
}

async function toggleMyRequests() {
  showMyRequests = !showMyRequests;
  showProfileMenu = false;

  if (showMyRequests && authStore.isAuthenticated) {
    await loadMyRequests();
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
  showProfileMenu = false;
  showMainMenu = false;
  showLoginDialog = false;
  editableTitle = '';
  editableDescription = '';
  editableSearchPlaceholder = 'Search the knowledge base...';
  editableSearchHint = 'Search for articles, guides, and answers to common questions';
  requestTypes = [];
  loadingRequestTypes = false;
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

  // Getters for sections/footer
  get portalSections() { return portalSections; },
  get footerColumns() { return footerColumns; },
  get draggedRequestType() { return draggedRequestType; },

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

  // Setters for UI state
  set isEditing(value) { isEditing = value; },
  set showCustomizePanel(value) { showCustomizePanel = value; },
  set showMyRequests(value) { showMyRequests = value; },
  set activeSection(value) { activeSection = value; },
  set showProfileMenu(value) { showProfileMenu = value; },
  set showMainMenu(value) { showMainMenu = value; },
  set showLoginDialog(value) { showLoginDialog = value; },
  set draggedRequestType(value) { draggedRequestType = value; },

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

  // Section actions
  addSection,
  deleteSection,
  updateSection,
  moveSectionUp,
  moveSectionDown,
  addRequestTypeToSection,
  removeRequestTypeFromSection,

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

  // Menu actions
  closeAllMenus,

  // Reset
  reset,
};
