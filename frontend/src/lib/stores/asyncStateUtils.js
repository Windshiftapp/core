import { writable } from 'svelte/store';

/**
 * Create async state management utilities
 * Reduces boilerplate for loading/error state handling in stores
 *
 * @example Basic usage
 * const { loading, error, run } = createAsyncState();
 *
 * async function loadData() {
 *   return await run(async () => {
 *     return await api.getData();
 *   });
 * }
 *
 * @example With onError callback
 * const { loading, error, run } = createAsyncState({
 *   onError: (err) => console.error('API failed:', err)
 * });
 *
 * @example With default error message
 * const { loading, error, run } = createAsyncState({
 *   defaultErrorMessage: 'Failed to load data'
 * });
 *
 * @param {Object} options Configuration options
 * @param {Function} options.onError Optional callback called when an error occurs
 * @param {string} options.defaultErrorMessage Default error message if none provided
 * @returns {{ loading: Writable<boolean>, error: Writable<string|null>, run: Function, reset: Function }}
 */
export function createAsyncState(options = {}) {
  const { onError, defaultErrorMessage = 'An error occurred' } = options;

  const loading = writable(false);
  const error = writable(null);

  /**
   * Run an async function with automatic loading/error state management
   * @param {Function} asyncFn Async function to execute
   * @returns {Promise<any>} Result of the async function
   * @throws Re-throws the error after setting error state
   */
  async function run(asyncFn) {
    loading.set(true);
    error.set(null);

    try {
      const result = await asyncFn();
      return result;
    } catch (err) {
      const errorMessage = err?.message || defaultErrorMessage;
      error.set(errorMessage);

      if (onError) {
        onError(err);
      }

      throw err;
    } finally {
      loading.set(false);
    }
  }

  /**
   * Run an async function without re-throwing errors (for cases where you want to handle errors silently)
   * @param {Function} asyncFn Async function to execute
   * @returns {Promise<{ success: boolean, data?: any, error?: Error }>}
   */
  async function runSafe(asyncFn) {
    loading.set(true);
    error.set(null);

    try {
      const result = await asyncFn();
      return { success: true, data: result };
    } catch (err) {
      const errorMessage = err?.message || defaultErrorMessage;
      error.set(errorMessage);

      if (onError) {
        onError(err);
      }

      return { success: false, error: err };
    } finally {
      loading.set(false);
    }
  }

  /**
   * Reset the async state
   */
  function reset() {
    loading.set(false);
    error.set(null);
  }

  /**
   * Set loading state manually
   * @param {boolean} isLoading
   */
  function setLoading(isLoading) {
    loading.set(isLoading);
  }

  /**
   * Set error state manually
   * @param {string|null} errorMessage
   */
  function setError(errorMessage) {
    error.set(errorMessage);
  }

  return {
    loading,
    error,
    run,
    runSafe,
    reset,
    setLoading,
    setError,
  };
}

/**
 * Create multiple named async states for a store with multiple async operations
 *
 * @example
 * const asyncStates = createMultipleAsyncStates(['fetch', 'save', 'delete']);
 * // Use: asyncStates.fetch.loading, asyncStates.fetch.run(...)
 * // Use: asyncStates.save.loading, asyncStates.save.run(...)
 *
 * @param {string[]} names Names of the async states
 * @param {Object} options Shared options for all states
 * @returns {Record<string, ReturnType<typeof createAsyncState>>}
 */
export function createMultipleAsyncStates(names, options = {}) {
  return names.reduce((acc, name) => {
    acc[name] = createAsyncState(options);
    return acc;
  }, {});
}
