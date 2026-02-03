/**
 * Composable for managing item link state and operations.
 * Handles loading, adding, and removing links between items.
 */

import { api } from '../api.js';

/**
 * Creates an item links state manager.
 *
 * @param {Function} getItemId - Function that returns the current item ID
 * @param {Function} onError - Callback when an error occurs
 * @returns {Object} Links state and functions
 */
export function useItemLinks(getItemId, onError) {
  // Links state
  let itemLinks = $state([]);
  let linkTypes = $state([]);
  let loadingLinks = $state(false);
  let showAddLinkForm = $state(false);

  // Add link form data
  let addLinkData = $state({
    link_type_id: null,
    target_id: null,
    target_title: '',
    target_type: 'item',
  });

  // Search state for link targets
  let searchResults = $state([]);
  let searchQuery = $state('');
  let searching = $state(false);

  /**
   * Loads link types from the API.
   */
  async function loadLinkTypes() {
    try {
      const result = await api.linkTypes.getAll();
      linkTypes = result || [];
    } catch (error) {
      console.error('Failed to load link types:', error);
      linkTypes = [];
    }
  }

  /**
   * Loads links for the current item.
   */
  async function loadItemLinks() {
    const itemId = getItemId();
    if (!itemId) return;

    try {
      loadingLinks = true;
      const result = await api.items.getLinks(itemId);
      itemLinks = result || [];
    } catch (error) {
      console.error('Failed to load item links:', error);
      itemLinks = [];
      if (onError) {
        onError('Failed to load item links', error.message || String(error));
      }
    } finally {
      loadingLinks = false;
    }
  }

  /**
   * Searches for potential link targets.
   */
  async function searchLinkTargets(query) {
    if (!query || query.length < 2) {
      searchResults = [];
      return;
    }

    const itemId = getItemId();
    if (!itemId) return;

    try {
      searching = true;
      searchQuery = query;

      if (addLinkData.target_type === 'item') {
        const result = await api.items.search({ q: query, limit: 10 });
        // Filter out the current item
        searchResults = (result || []).filter((item) => item.id !== itemId);
      } else if (addLinkData.target_type === 'test_case') {
        const result = await api.tests.testCases.search(query);
        searchResults = result || [];
      }
    } catch (error) {
      console.error('Failed to search link targets:', error);
      searchResults = [];
    } finally {
      searching = false;
    }
  }

  /**
   * Adds a new link.
   */
  async function addLink() {
    const itemId = getItemId();
    if (!itemId || !addLinkData.target_id || !addLinkData.link_type_id) {
      return false;
    }

    try {
      await api.items.addLink(itemId, {
        link_type_id: addLinkData.link_type_id,
        target_id: addLinkData.target_id,
        target_type: addLinkData.target_type,
      });

      // Reload links
      await loadItemLinks();

      // Reset form
      resetAddLinkForm();

      return true;
    } catch (error) {
      console.error('Failed to add link:', error);
      if (onError) {
        onError('Failed to add link', error.message || String(error));
      }
      return false;
    }
  }

  /**
   * Removes a link.
   */
  async function removeLink(linkId) {
    const itemId = getItemId();
    if (!itemId || !linkId) return false;

    try {
      await api.items.removeLink(itemId, linkId);
      await loadItemLinks();
      return true;
    } catch (error) {
      console.error('Failed to remove link:', error);
      if (onError) {
        onError('Failed to remove link', error.message || String(error));
      }
      return false;
    }
  }

  /**
   * Resets the add link form.
   */
  function resetAddLinkForm() {
    addLinkData = {
      link_type_id: null,
      target_id: null,
      target_title: '',
      target_type: 'item',
    };
    searchResults = [];
    searchQuery = '';
    showAddLinkForm = false;
  }

  /**
   * Shows the add link form.
   */
  function openAddLinkForm() {
    showAddLinkForm = true;
  }

  /**
   * Hides the add link form.
   */
  function closeAddLinkForm() {
    resetAddLinkForm();
  }

  /**
   * Updates a field in the add link form.
   */
  function updateAddLinkField(field, value) {
    addLinkData = { ...addLinkData, [field]: value };
  }

  /**
   * Gets links filtered by type.
   */
  function getLinksByType(typeId) {
    return itemLinks.filter((link) => link.link_type_id === typeId);
  }

  /**
   * Resets all link state.
   */
  function resetAll() {
    itemLinks = [];
    linkTypes = [];
    loadingLinks = false;
    resetAddLinkForm();
  }

  return {
    // State getters
    get itemLinks() {
      return itemLinks;
    },
    get linkTypes() {
      return linkTypes;
    },
    get loadingLinks() {
      return loadingLinks;
    },
    get showAddLinkForm() {
      return showAddLinkForm;
    },
    get addLinkData() {
      return addLinkData;
    },
    get searchResults() {
      return searchResults;
    },
    get searchQuery() {
      return searchQuery;
    },
    get searching() {
      return searching;
    },

    // State setters
    set showAddLinkForm(value) {
      showAddLinkForm = value;
    },
    set addLinkData(value) {
      addLinkData = value;
    },
    set searchQuery(value) {
      searchQuery = value;
    },

    // Methods
    loadLinkTypes,
    loadItemLinks,
    searchLinkTargets,
    addLink,
    removeLink,
    resetAddLinkForm,
    openAddLinkForm,
    closeAddLinkForm,
    updateAddLinkField,
    getLinksByType,
    resetAll,
  };
}
