/**
 * Centralized error handling utilities for API errors
 *
 * Provides a unified way to handle and display errors across the application
 * with support for i18n translations.
 */

import { translateError } from '../stores/i18n.svelte.js';
import { errorToast } from '../stores/toasts.svelte.js';

/**
 * Handle an API error by showing a translated toast notification
 * @param {Error|object} error - Error object with optional code and details
 * @param {object} options - Additional options
 * @param {string} options.fallbackMessage - Fallback message if translation fails
 * @param {boolean} options.silent - If true, don't show toast (just return message)
 */
export function handleApiError(error, options = {}) {
  const { fallbackMessage, silent = false } = options;

  let message;

  try {
    message = translateError(error);
  } catch (err) {
    console.error('Error translating error message:', err);
    message = fallbackMessage || error?.message || 'An error occurred';
  }

  if (!silent) {
    errorToast(message);
  }

  return message;
}

/**
 * Parse an error response to extract structured error information
 * @param {Error} error - Error object to parse
 * @returns {object} Parsed error with code, message, and details
 */
export function parseApiError(error) {
  // If error already has code property, return as-is
  if (error?.code) {
    return error;
  }

  // Try to extract error code from message
  // Backend errors are typically in format: "ERROR_CODE: Human readable message"
  // or JSON: {"error": "message", "code": "ERROR_CODE", "details": {...}}
  if (error?.message) {
    const message = error.message;

    // Try to parse as JSON first
    try {
      const parsed = JSON.parse(message);
      return {
        code: parsed.code || 'UNKNOWN',
        message: parsed.error || parsed.message || message,
        details: parsed.details || {},
        requestId: parsed.request_id,
      };
    } catch {
      // Not JSON, continue with other parsing
    }

    // Check for "CODE: message" format
    const colonIndex = message.indexOf(':');
    if (colonIndex > 0 && colonIndex < 30) {
      const potentialCode = message.substring(0, colonIndex).trim();
      // Check if it looks like an error code (all uppercase, underscores)
      if (/^[A-Z_]+$/.test(potentialCode)) {
        return {
          code: potentialCode,
          message: message.substring(colonIndex + 1).trim(),
          details: {},
        };
      }
    }
  }

  // Return generic error structure
  return {
    code: 'UNKNOWN',
    message: error?.message || String(error) || 'Unknown error',
    details: {},
  };
}

/**
 * Create an enhanced error object from an API response
 * @param {Response} response - Fetch Response object
 * @param {string} responseText - Response body text
 * @returns {Error} Enhanced error object
 */
export function createApiError(response, responseText) {
  const error = new Error(responseText || `Request failed: ${response.statusText}`);

  // Try to parse structured error from response
  try {
    const parsed = JSON.parse(responseText);
    error.code = parsed.code;
    error.errorCode = parsed.code; // Alias for compatibility
    error.details = parsed.details || {};
    error.requestId = parsed.request_id;
    error.message = parsed.error || parsed.message || error.message;
  } catch {
    // Response is not JSON, keep original message
    // Try to extract code from plain text response
    const parsed = parseApiError({ message: responseText });
    error.code = parsed.code;
    error.details = parsed.details;
  }

  // Add HTTP status info
  error.status = response.status;
  error.statusText = response.statusText;

  return error;
}

/**
 * Check if an error is a specific error code
 * @param {Error|object} error - Error to check
 * @param {string} code - Error code to match
 * @returns {boolean}
 */
export function isErrorCode(error, code) {
  if (!error) return false;
  return error.code === code || error.errorCode === code;
}

/**
 * Check if an error is an authentication error
 * @param {Error|object} error - Error to check
 * @returns {boolean}
 */
export function isAuthError(error) {
  return (
    isErrorCode(error, 'UNAUTHORIZED') ||
    isErrorCode(error, 'AUTHENTICATION_REQUIRED') ||
    isErrorCode(error, 'INVALID_TOKEN') ||
    isErrorCode(error, 'TOKEN_EXPIRED')
  );
}

/**
 * Check if an error is a permission error
 * @param {Error|object} error - Error to check
 * @returns {boolean}
 */
export function isPermissionError(error) {
  return isErrorCode(error, 'INSUFFICIENT_PERMISSION');
}

/**
 * Check if an error is a not found error
 * @param {Error|object} error - Error to check
 * @returns {boolean}
 */
export function isNotFoundError(error) {
  return (
    isErrorCode(error, 'NOT_FOUND') ||
    isErrorCode(error, 'ITEM_NOT_FOUND') ||
    isErrorCode(error, 'WORKSPACE_NOT_FOUND') ||
    isErrorCode(error, 'USER_NOT_FOUND')
  );
}

/**
 * Check if an error is a validation error
 * @param {Error|object} error - Error to check
 * @returns {boolean}
 */
export function isValidationError(error) {
  return (
    isErrorCode(error, 'VALIDATION_FAILED') ||
    isErrorCode(error, 'INVALID_INPUT') ||
    isErrorCode(error, 'MISSING_FIELD')
  );
}

/**
 * Check if an error is a rate limit error
 * @param {Error|object} error - Error to check
 * @returns {boolean}
 */
export function isRateLimitError(error) {
  return isErrorCode(error, 'RATE_LIMITED');
}
