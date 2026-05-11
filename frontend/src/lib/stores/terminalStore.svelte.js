import { derived, writable } from 'svelte/store';

const TERMINAL_VISIBLE_KEY = 'windshift-terminal-visible';
const TERMINAL_SPLIT_KEY = 'windshift-terminal-split';
const TERMINAL_SPLIT_DEFAULT = 50; // percentage

function getInitialVisible() {
  if (typeof window === 'undefined') return false;
  try {
    return localStorage.getItem(TERMINAL_VISIBLE_KEY) === 'true';
  } catch {
    return false;
  }
}

function getInitialSplit() {
  if (typeof window === 'undefined') return TERMINAL_SPLIT_DEFAULT;
  try {
    const stored = localStorage.getItem(TERMINAL_SPLIT_KEY);
    if (stored) {
      const val = parseFloat(stored);
      if (!Number.isNaN(val) && val >= 20 && val <= 80) return val;
    }
  } catch {
    // ignore
  }
  return TERMINAL_SPLIT_DEFAULT;
}

function createTerminalStore() {
  const visible = writable(getInitialVisible());
  const splitPercent = writable(getInitialSplit());
  const activeTabId = writable(0);
  const tabCounter = writable(1);
  const tabs = writable([{ id: 0, title: 'Terminal 1' }]);

  // Persist
  visible.subscribe((value) => {
    if (typeof window !== 'undefined') {
      try {
        localStorage.setItem(TERMINAL_VISIBLE_KEY, String(value));
      } catch {
        /* ignore */
      }
    }
  });

  splitPercent.subscribe((value) => {
    if (typeof window !== 'undefined') {
      try {
        localStorage.setItem(TERMINAL_SPLIT_KEY, String(value));
      } catch {
        /* ignore */
      }
    }
  });

  const combined = derived(
    [visible, splitPercent, activeTabId, tabs],
    ([$visible, $splitPercent, $activeTabId, $tabs]) => ({
      visible: $visible,
      splitPercent: $splitPercent,
      activeTabId: $activeTabId,
      tabs: $tabs,
    })
  );

  return {
    subscribe: combined.subscribe,

    toggle() {
      visible.update((v) => !v);
    },

    show() {
      visible.set(true);
    },

    hide() {
      visible.set(false);
    },

    setSplitPercent(value) {
      const clamped = Math.min(80, Math.max(20, value));
      splitPercent.set(clamped);
    },

    addTab() {
      let newId;
      tabCounter.update((c) => {
        newId = c;
        return c + 1;
      });
      tabs.update((t) => [...t, { id: newId, title: `Terminal ${newId + 1}` }]);
      activeTabId.set(newId);
      return newId;
    },

    removeTab(id) {
      tabs.update((t) => {
        const filtered = t.filter((tab) => tab.id !== id);
        if (filtered.length === 0) {
          visible.set(false);
          return [{ id: 0, title: 'Terminal 1' }];
        }
        return filtered;
      });
      activeTabId.update((current) => {
        if (current === id) {
          /** @type {any[]} */
          let currentTabs = [];
          tabs.subscribe((t) => (currentTabs = t))();
          return currentTabs[0]?.id ?? 0;
        }
        return current;
      });
    },

    setActiveTab(id) {
      activeTabId.set(id);
    },

    /** Write text into the active terminal PTY (dispatched via event) */
    writeToTerminal(text) {
      window.dispatchEvent(new CustomEvent('terminal-write', { detail: { text } }));
    },
  };
}

export const terminalStore = createTerminalStore();
