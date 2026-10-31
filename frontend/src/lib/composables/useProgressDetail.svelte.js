import { t } from '../stores/i18n.svelte.js';

/**
 * Shared progress-detail state for the iteration and milestone pages.
 *
 * Both load a progress document plus the full record, expand every status
 * category by default, and derive a status lozenge from a per-entity option
 * list. The record loader is injected because each page fetches a different API
 * (the iteration also pulls burndown data).
 *
 * @param {{
 *   load: () => Promise<{ progress: object, entity?: object }>,
 *   errorLabel: string,
 *   getStatusOptions: () => Array<{ value: string, label: string, lozengeColor: string }>,
 * }} config
 */
export function useProgressDetail({ load, errorLabel, getStatusOptions }) {
  let loading = $state(true);
  let error = $state(null);
  let progress = $state(null);
  let entity = $state(null);
  let expandedCategories = $state({});

  async function loadProgress() {
    loading = true;
    error = null;
    try {
      const result = await load();
      progress = result.progress;
      entity = result.entity ?? null;
      // Expand every category by default; collapsing is per-session state.
      if (progress?.status_breakdown) {
        progress.status_breakdown.forEach((category) => {
          expandedCategories[category.category_name] = true;
        });
      }
    } catch (err) {
      console.error(`Failed to load ${errorLabel}:`, err);
      error = err.message || t('dialogs.alerts.failedToLoad', { error: errorLabel });
    } finally {
      loading = false;
    }
  }

  function toggleCategory(categoryName) {
    expandedCategories[categoryName] = !expandedCategories[categoryName];
  }

  function getStatusInfo(status) {
    const options = getStatusOptions();
    return options.find((option) => option.value === status) || options[0];
  }

  return {
    get loading() {
      return loading;
    },
    get error() {
      return error;
    },
    get progress() {
      return progress;
    },
    get entity() {
      return entity;
    },
    get expandedCategories() {
      return expandedCategories;
    },
    loadProgress,
    toggleCategory,
    getStatusInfo,
  };
}
