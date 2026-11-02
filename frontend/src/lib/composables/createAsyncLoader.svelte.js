/**
 * Creates reactive async data loader with loading and error states
 * @param {Function} fetchFn - Async function that fetches data
 * @returns {Object} Reactive loader with data, loading, error states and load method
 */
export function createAsyncLoader(fetchFn) {
  let data = $state([]);
  let loading = $state(false);
  let error = $state(null);
  let disposed = false;
  // Each run owns a generation so a superseded load can never overwrite the
  // data or clear the loading flag of a newer one.
  let generation = 0;

  async function run() {
    if (disposed) return;

    const current = ++generation;
    loading = true;
    error = null;

    try {
      const result = await fetchFn();
      if (disposed || current !== generation) return;
      data = result || [];
    } catch (e) {
      // Navigation can destroy the owner or abort its in-flight request.
      if (disposed || current !== generation || e?.name === 'AbortError') return;
      console.error('Failed to load data:', e);
      error = e.message || 'Failed to load data';
      data = [];
    } finally {
      if (current === generation) loading = false;
    }
  }

  async function load() {
    if (loading || disposed) return;
    await run();
  }

  // Reload even when a load is already in flight. Callers use this when the
  // loader's inputs (for example a workspace scope) change.
  async function reload() {
    if (disposed) return;
    await run();
  }

  async function refetch() {
    data = [];
    await reload();
  }

  function dispose() {
    disposed = true;
  }

  return {
    get data() {
      return data;
    },
    get loading() {
      return loading;
    },
    get error() {
      return error;
    },
    load,
    reload,
    refetch,
    dispose,
  };
}
