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
  // State
  let attachments = $state([]);
  let pagination = $state(null);
  let settings = $state(null);
  let loading = $state(false);
  let currentPage = $state(1);
  let pageSize = $state(50);

  /**
   * Load attachment settings from the server (for file size limits, allowed types, etc.)
   */
  async function loadSettings() {
    try {
      settings = await api.attachmentSettings.get();
    } catch (err) {
      // If attachment settings endpoint doesn't exist (404), attachments are not configured
      if (err.message.includes('Not Found')) {
        settings = {
          enabled: false,
          attachment_path: null,
          max_file_size: 52428800, // 50MB default
          allowed_mime_types: '[]'
        };
      } else {
        console.error('Failed to load attachment settings:', err);
        // Set defaults for other errors
        settings = {
          enabled: false,
          attachment_path: null,
          max_file_size: 52428800, // 50MB
          allowed_mime_types: '[]'
        };
      }
    }
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

      if (response && response.attachments) {
        // Handle paginated response
        attachments = response.attachments;
        pagination = response.pagination;
        currentPage = page;
        pageSize = limit;
      } else {
        // Handle legacy response (backward compatibility)
        attachments = response || [];
        pagination = null;
      }
    } catch (err) {
      console.error('Failed to load attachments:', err);
      attachments = [];
      pagination = null;
    } finally {
      loading = false;
    }
  }

  /**
   * Handle attachment upload event from AttachmentList component
   * @param {CustomEvent} event - Upload event with detail { attachment, message }
   */
  async function handleUpload(event) {
    // Reload attachments to get updated pagination info
    if (isEnabled()) {
      await load(1, pageSize); // Go to first page to see new upload
    }
  }

  /**
   * Handle attachment delete event from AttachmentList component
   * @param {CustomEvent} event - Delete event with attachment detail
   */
  async function handleDelete(event) {
    const attachment = event.detail;

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
   * Handle page change event
   * @param {CustomEvent} event - Event with detail { page, itemsPerPage }
   */
  async function handlePageChange(event) {
    if (isEnabled()) {
      await load(event.detail.page, event.detail.itemsPerPage);
    }
  }

  /**
   * Handle page size change event
   * @param {CustomEvent} event - Event with detail { page, itemsPerPage }
   */
  async function handlePageSizeChange(event) {
    if (isEnabled()) {
      await load(event.detail.page, event.detail.itemsPerPage);
    }
  }

  /**
   * Upload files directly (from Attach button)
   * @param {CustomEvent} event - Event with detail { files }
   */
  async function uploadFiles(event) {
    const { files } = event.detail;
    if (!files || files.length === 0) return;

    const itemId = getItemId();
    if (!itemId) return;

    for (const file of files) {
      try {
        const formData = new FormData();
        formData.append('file', file);
        formData.append('item_id', itemId.toString());

        const response = await fetch('/api/attachments/upload', {
          method: 'POST',
          body: formData,
        });

        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(errorText || 'Upload failed');
        }

        const result = await response.json();
        if (!result.success) {
          throw new Error(result.message || 'Upload failed');
        }
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
    get attachments() { return attachments; },
    get pagination() { return pagination; },
    get settings() { return settings; },
    get loading() { return loading; },
    get currentPage() { return currentPage; },
    get pageSize() { return pageSize; },

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
    setPageSize
  };
}
