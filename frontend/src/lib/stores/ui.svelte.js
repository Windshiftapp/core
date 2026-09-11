import { derived, writable } from 'svelte/store';

// Storage key for nav expanded state
const NAV_EXPANDED_STORAGE_KEY = 'windshift-nav-expanded';
const WS_SIDEBAR_WIDTH_STORAGE_KEY = 'windshift-ws-sidebar-width';
const WS_SIDEBAR_COLLAPSED_STORAGE_KEY = 'windshift-ws-sidebar-collapsed';
export const WS_SIDEBAR_DEFAULT_WIDTH = 256;
const WS_SIDEBAR_LEGACY_DEFAULT_WIDTH = 192;
export const WS_SIDEBAR_MIN_WIDTH = 180;
export const WS_SIDEBAR_MAX_WIDTH = 520;

// Helper to get initial navExpanded value from localStorage
function getInitialNavExpanded() {
  if (typeof window === 'undefined') return false;
  try {
    const stored = window.localStorage.getItem(NAV_EXPANDED_STORAGE_KEY);
    return stored === 'true';
  } catch {
    return false;
  }
}

function getInitialWsSidebarCollapsed() {
  if (typeof window === 'undefined') return false;
  try {
    return window.localStorage.getItem(WS_SIDEBAR_COLLAPSED_STORAGE_KEY) === 'true';
  } catch {
    return false;
  }
}

function getInitialWsSidebarWidth() {
  if (typeof window === 'undefined') return WS_SIDEBAR_DEFAULT_WIDTH;
  try {
    const stored = window.localStorage.getItem(WS_SIDEBAR_WIDTH_STORAGE_KEY);
    if (stored) {
      const val = parseInt(stored, 10);
      // The store persisted the old default for every visitor, even when they
      // never resized the sidebar. Migrate only that exact value so custom
      // widths continue to be respected.
      if (val === WS_SIDEBAR_LEGACY_DEFAULT_WIDTH) return WS_SIDEBAR_DEFAULT_WIDTH;
      if (!Number.isNaN(val) && val >= WS_SIDEBAR_MIN_WIDTH && val <= WS_SIDEBAR_MAX_WIDTH) {
        return val;
      }
    }
  } catch {
    // Ignore
  }
  return WS_SIDEBAR_DEFAULT_WIDTH;
}

// UI store - manages UI-specific state
function createUIStore() {
  const reviewFullscreen = writable(false);
  const navExpanded = writable(getInitialNavExpanded());
  const wsSidebarWidth = writable(getInitialWsSidebarWidth());
  const wsSidebarCollapsed = writable(getInitialWsSidebarCollapsed());

  // Persist navExpanded to localStorage on changes
  navExpanded.subscribe((value) => {
    if (typeof window !== 'undefined') {
      try {
        window.localStorage.setItem(NAV_EXPANDED_STORAGE_KEY, String(value));
      } catch {
        // Ignore localStorage errors
      }
    }
  });

  // Persist wsSidebarWidth to localStorage on changes
  wsSidebarWidth.subscribe((value) => {
    if (typeof window !== 'undefined') {
      try {
        window.localStorage.setItem(WS_SIDEBAR_WIDTH_STORAGE_KEY, String(value));
      } catch {
        // Ignore localStorage errors
      }
    }
  });

  // Persist wsSidebarCollapsed to localStorage on changes
  wsSidebarCollapsed.subscribe((value) => {
    if (typeof window !== 'undefined') {
      try {
        window.localStorage.setItem(WS_SIDEBAR_COLLAPSED_STORAGE_KEY, String(value));
      } catch {
        // Ignore localStorage errors
      }
    }
  });

  // Create a combined derived store for easy subscription
  const combined = derived(
    [reviewFullscreen, navExpanded, wsSidebarWidth, wsSidebarCollapsed],
    ([$reviewFullscreen, $navExpanded, $wsSidebarWidth, $wsSidebarCollapsed]) => ({
      reviewFullscreen: $reviewFullscreen,
      navExpanded: $navExpanded,
      wsSidebarWidth: $wsSidebarWidth,
      wsSidebarCollapsed: $wsSidebarCollapsed,
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

    // Convenience getter for wsSidebarWidth
    get wsSidebarWidth() {
      /** @type {number | undefined} */
      let value;
      wsSidebarWidth.subscribe((v) => (value = v))();
      return value;
    },

    // Setter for wsSidebarWidth
    /** @param {number} value */
    set wsSidebarWidth(value) {
      wsSidebarWidth.set(value);
    },

    // Reset wsSidebarWidth to default
    resetWsSidebarWidth() {
      wsSidebarWidth.set(WS_SIDEBAR_DEFAULT_WIDTH);
    },

    // Convenience getter for wsSidebarCollapsed
    get wsSidebarCollapsed() {
      /** @type {boolean | undefined} */
      let value;
      wsSidebarCollapsed.subscribe((v) => (value = v))();
      return value;
    },

    // Setter for wsSidebarCollapsed
    /** @param {boolean} value */
    set wsSidebarCollapsed(value) {
      wsSidebarCollapsed.set(value);
    },

    // Toggle wsSidebarCollapsed
    toggleWsSidebarCollapsed() {
      wsSidebarCollapsed.update((v) => !v);
    },
  };
}

export const uiStore = createUIStore();
