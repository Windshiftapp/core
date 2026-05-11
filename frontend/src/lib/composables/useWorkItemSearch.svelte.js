/**
 * Build the filter-update / QL-mode handler set used by Collections and
 * SearchPage against the shared work-item search store.
 *
 * Both pages bind a UI to the same `store` and forward the same set of nine
 * handlers; only the local `rawMode` flag, the page-size constant, and the
 * current-page reset callback differ.
 *
 * @param store         Search store with the standard mutators.
 * @param getRawMode    () => boolean — if true, value-mutating handlers no-op.
 *                      Defaults to always-false (callers without a raw mode).
 * @param getItemsPerPage  () => number — page size for executeSearch calls.
 * @param onPageReset   () => void — called after handleExecuteQL /
 *                      handleResetToBuilder so the caller can reset its own
 *                      currentPage state.
 */
/**
 * @param {any} store
 * @param {{
 *   getRawMode?: () => boolean,
 *   getItemsPerPage?: () => number,
 *   onPageReset?: () => void,
 * }} [opts]
 */
export function createWorkItemSearchHandlers(
  store,
  { getRawMode = () => false, getItemsPerPage = () => 50, onPageReset = () => {} } = {}
) {
  function toNumberArray(value) {
    return (value || []).map((v) => Number(v)).filter((id) => !Number.isNaN(id));
  }

  return {
    handleUpdateWorkspaces(value) {
      if (getRawMode()) return;
      store.setSelectedWorkspaces(value);
    },
    handleUpdateStatuses(value) {
      if (getRawMode()) return;
      store.setSelectedStatuses(toNumberArray(value));
    },
    handleUpdatePriorities(value) {
      if (getRawMode()) return;
      store.setSelectedPriorities(toNumberArray(value));
    },
    handleUpdateSearch(value) {
      if (getRawMode()) return;
      store.setSearchQuery(value);
    },
    handleUpdateDynamicFilters(value) {
      if (getRawMode()) return;
      store.setDynamicFilters(value);
    },
    async handleExecuteQL() {
      store.syncToURL();
      await store.executeSearch({ page: 1, limit: getItemsPerPage() });
      onPageReset();
    },
    async handleEnterRawMode() {
      await store.enterRawMode();
      store.syncToURL();
    },
    async handleResetToBuilder() {
      await store.resetToBuilder();
      store.syncToURL();
      await store.executeSearch({ page: 1, limit: getItemsPerPage() });
      onPageReset();
    },
    handleQueryChange(value) {
      store.setRawQlQuery(value);
    },
  };
}
