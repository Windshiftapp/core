import { onDestroy } from 'svelte';
import { api } from '../api.js';
import { attachmentStatus } from '../stores';

/**
 * Composable for managing item attachments
 * Handles loading, uploading, deleting, and pagination of attachments
 *
 * @param {Function} getItemId - Function that returns the current item ID
 * @param {Function} showError - Error display callback (message, details)
 * @returns {Object} Attachment state and methods
 */
export function useItemAttachments(getItemId, showError = console.error) {
  let destroyed = false;
  const markDestroyed = () => {
    destroyed = true;
  };
  globalThis.window?.addEventListener('pagehide', markDestroyed);
  onDestroy(() => {
    markDestroyed();
    globalThis.window?.removeEventListener('pagehide', markDestroyed);
  });

  // State
  let attachments = $state([]);
  let pagination = $state(null);
  let settings = $state(null);
  let loading = $state(false);
  let currentPage = $state(1);
  let pageSize = $state(50);

  /**
   * Load attachment settings from the shared store
   * The attachmentStatus store is loaded centrally and handles API calls
   */
  async function loadSettings() {
    // Ensure the shared store is loaded
    await attachmentStatus.load();

    // Use defaults - actual enabled state comes from the store via isEnabled()
    settings = {
      enabled: attachmentStatus.enabled,
      attachment_path: null,
      max_file_size: 52428800, // 50MB default
      allowed_mime_types: '[]',
    };
  }

  /**
   * Check if attachments are enabled (uses shared store)
   * @returns {boolean}
   */
  function isEnabled() {
    return attachmentStatus.enabled;
  }

  /**
   * Load attachments for the current item
   * @param {number} page - Page number (default: 1)
   * @param {number} limit - Items per page (default: current pageSize)
   */
  async function load(page = 1, limit = pageSize) {
    const itemId = getItemId();
    if (!itemId) return;

    try {
      loading = true;
      const response = await api.attachments.getByItem(itemId, { page, limit });

      attachments = response?.data || [];
      pagination = response?.pagination || null;
      currentPage = page;
      pageSize = limit;
    } catch (err) {
      if (destroyed || err?.name === 'AbortError') return;
      console.error('Failed to load attachments:', err);
      attachments = [];
      pagination = null;
    } finally {
      loading = false;
    }
  }

  /**
   * Handle attachment upload event
   * Called when an attachment is uploaded
   */
  async function handleUpload() {
    // Reload attachments to get updated pagination info
    if (isEnabled()) {
      await load(1, pageSize); // Go to first page to see new upload
    }
  }

  /**
   * Handle attachment delete
   * @param {Object} attachment - The attachment to delete
   */
  async function handleDelete(attachment) {
    try {
      await api.attachments.delete(attachment.id);

      // Reload current page to update pagination
      if (isEnabled()) {
        await load(currentPage, pageSize);
      }
    } catch (err) {
      console.error('Failed to delete attachment:', err);
      showError('Failed to delete attachment', err.message || String(err));
    }
  }

  /**
   * Handle page change
   * @param {Object} detail - { page, itemsPerPage }
   */
  async function handlePageChange({ page, itemsPerPage }) {
    if (isEnabled()) {
      await load(page, itemsPerPage);
    }
  }

  /**
   * Handle page size change
   * @param {Object} detail - { page, itemsPerPage }
   */
  async function handlePageSizeChange({ page, itemsPerPage }) {
    if (isEnabled()) {
      await load(page, itemsPerPage);
    }
  }

  /**
   * Upload files directly (from Attach button)
   * @param {Object} detail - { files }
   */
  async function uploadFiles({ files }) {
    if (!files || files.length === 0) return;

    const itemId = getItemId();
    if (!itemId) return;

    for (const file of files) {
      try {
        await api.attachments.uploadToItem(itemId, file);
      } catch (err) {
        console.error('Upload error:', err);
        showError('Failed to upload attachment', err.message || String(err));
      }
    }

    // Reload attachments after all uploads complete
    if (isEnabled()) {
      await load(1, pageSize);
    }
  }

  /**
   * Set the current page
   * @param {number} page
   */
  function setPage(page) {
    currentPage = page;
  }

  /**
   * Set the page size
   * @param {number} size
   */
  function setPageSize(size) {
    pageSize = size;
  }

  // Public API
  return {
    // State (reactive getters)
    get attachments() {
      return attachments;
    },
    get pagination() {
      return pagination;
    },
    get settings() {
      return settings;
    },
    get loading() {
      return loading;
    },
    get currentPage() {
      return currentPage;
    },
    get pageSize() {
      return pageSize;
    },

    // Methods
    loadSettings,
    load,
    isEnabled,
    handleUpload,
    handleDelete,
    handlePageChange,
    handlePageSizeChange,
    uploadFiles,
    setPage,
    setPageSize,
  };
}
