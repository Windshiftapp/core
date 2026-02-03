import { derived, writable } from 'svelte/store';

// Storage key for nav expanded state
const NAV_EXPANDED_STORAGE_KEY = 'windshift-nav-expanded';

// Helper to get initial navExpanded value from localStorage
function getInitialNavExpanded() {
  if (typeof window === 'undefined') return false;
  try {
    const stored = localStorage.getItem(NAV_EXPANDED_STORAGE_KEY);
    return stored === 'true';
  } catch {
    return false;
  }
}

// UI store - manages UI-specific state
function createUIStore() {
  const reviewFullscreen = writable(false);
  const navExpanded = writable(getInitialNavExpanded());

  // Persist navExpanded to localStorage on changes
  navExpanded.subscribe((value) => {
    if (typeof window !== 'undefined') {
      try {
        localStorage.setItem(NAV_EXPANDED_STORAGE_KEY, String(value));
      } catch {
        // Ignore localStorage errors
      }
    }
  });

  // Create a combined derived store for easy subscription
  const combined = derived(
    [reviewFullscreen, navExpanded],
    ([$reviewFullscreen, $navExpanded]) => ({
      reviewFullscreen: $reviewFullscreen,
      navExpanded: $navExpanded,
    })
  );

  return {
    // Subscribe to combined state
    subscribe: combined.subscribe,

    // Convenience getter for backward compatibility
    get reviewFullscreen() {
      let value;
      reviewFullscreen.subscribe((v) => (value = v))();
      return value;
    },

    // Setter for reviewFullscreen
    set reviewFullscreen(value) {
      reviewFullscreen.set(value);
    },

    // Toggle reviewFullscreen
    toggleReviewFullscreen() {
      reviewFullscreen.update((v) => !v);
    },

    // Convenience getter for navExpanded
    get navExpanded() {
      let value;
      navExpanded.subscribe((v) => (value = v))();
      return value;
    },

    // Setter for navExpanded
    set navExpanded(value) {
      navExpanded.set(value);
    },

    // Toggle navExpanded
    toggleNavExpanded() {
      navExpanded.update((v) => !v);
    },
  };
}

export const uiStore = createUIStore();
