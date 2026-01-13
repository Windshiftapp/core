import { writable, derived } from 'svelte/store';

// UI store - manages UI-specific state
function createUIStore() {
  const reviewFullscreen = writable(false);

  // Create a combined derived store for easy subscription
  const combined = derived(reviewFullscreen, ($reviewFullscreen) => ({
    reviewFullscreen: $reviewFullscreen
  }));

  return {
    // Subscribe to combined state
    subscribe: combined.subscribe,

    // Convenience getter for backward compatibility
    get reviewFullscreen() {
      let value;
      reviewFullscreen.subscribe(v => value = v)();
      return value;
    },

    // Setter for reviewFullscreen
    set reviewFullscreen(value) {
      reviewFullscreen.set(value);
    },

    // Toggle reviewFullscreen
    toggleReviewFullscreen() {
      reviewFullscreen.update(v => !v);
    }
  };
}

export const uiStore = createUIStore();
