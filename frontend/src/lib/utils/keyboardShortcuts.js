/**
 * Centralized keyboard shortcuts configuration
 * Allows for easy platform-specific customization and management
 */

// Detect the current platform
function getPlatform() {
  const platform = navigator.platform.toLowerCase();
  const userAgent = navigator.userAgent.toLowerCase();

  if (platform.includes('mac') || userAgent.includes('mac')) {
    return 'mac';
  } else if (platform.includes('win') || userAgent.includes('win')) {
    return 'windows';
  } else if (platform.includes('linux') || userAgent.includes('linux')) {
    return 'linux';
  }
  return 'other';
}

const currentPlatform = getPlatform();

// Keyboard shortcuts configuration by context
const shortcuts = {
  global: {
    commandPalette: { key: 'k', modifierKey: true }
  },
  modal: {
    submit: { key: 'Enter', modifierKey: true },
    cancel: { key: 'Escape' }
  },
  ql: {
    execute: { key: 'Enter', modifierKey: true }
  },
  description: {
    save: { key: 'Enter', modifierKey: true },
    cancel: { key: 'Escape' }
  },
  workflow: {
    save: { key: 'Enter', modifierKey: true },
    new: { key: 'n' }
  },
  workspaces: {
    addWorkspace: { key: 'a' },
    submitForm: { key: 'Enter' },
    cancelForm: { key: 'Escape' }
  },
  testCases: {
    addTestCase: { key: 'a' },
    addFolder: { key: 'a', modifierKey: true },
    submitForm: { key: 'Enter' },
    cancelForm: { key: 'Escape' }
  },
  timeProjects: {
    addProject: { key: 'a' },
    addCategory: { key: 'a' },
    submitForm: { key: 'Enter' },
    cancelForm: { key: 'Escape' }
  },
  timeCustomers: {
    addCustomer: { key: 'a' },
    submitForm: { key: 'Enter' },
    cancelForm: { key: 'Escape' }
  },
  statusCategories: {
    addCategory: { key: 'a' },
    submitForm: { key: 'Enter' },
    cancelForm: { key: 'Escape' }
  },
  workspaceMembers: {
    addMember: { key: 'a' },
    submitForm: { key: 'Enter' },
    cancelForm: { key: 'Escape' }
  },
  sso: {
    addProvider: { key: 'a' },
    submitForm: { key: 'Enter' },
    cancelForm: { key: 'Escape' }
  },
  scmProviders: {
    addProvider: { key: 'a' },
    submitForm: { key: 'Enter' },
    cancelForm: { key: 'Escape' }
  },
  channels: {
    addChannel: { key: 'a' },
    submitForm: { key: 'Enter' },
    cancelForm: { key: 'Escape' }
  },
  testSteps: {
    addStep: { key: 'a' }
  },
  timeEntry: {
    openLog: { key: 'a' },
    toggleSelector: { key: 'l', modifierKey: true }
  },
  // Settings
  statuses: {
    add: { key: 'a' }
  },
  customFields: {
    add: { key: 'a' }
  },
  themes: {
    add: { key: 'a' }
  },
  itemTypes: {
    add: { key: 'a' }
  },
  priorities: {
    add: { key: 'a' }
  },
  permissionSets: {
    add: { key: 'a' }
  },
  hierarchyLevels: {
    add: { key: 'a' }
  },
  configurationSets: {
    add: { key: 'a' }
  },
  // Features
  milestones: {
    add: { key: 'a' }
  },
  iterations: {
    add: { key: 'a' }
  },
  assets: {
    upload: { key: 'a' }
  },
  customers: {
    add: { key: 'a' }
  },
  systemImport: {
    add: { key: 'a' }
  },
  // Pages
  screens: {
    add: { key: 'a' }
  },
  // Actions automation
  actions: {
    add: { key: 'a' },
    save: { key: 'Enter', modifierKey: true },
    cancel: { key: 'Escape' }
  },
  // Item detail time tracking
  itemDetail: {
    startTimer: { key: 'a', modifierKey: true },
    logTime: { key: 'a' }
  }
};

/**
 * Get keyboard shortcut configuration for a specific action
 * @param {string} context - The context (e.g., 'workspaces', 'testCases')
 * @param {string} action - The action (e.g., 'addWorkspace', 'addFolder')
 * @returns {Object} Shortcut configuration for current platform
 */
export function getShortcut(context, action) {
  const contextShortcuts = shortcuts[context];
  if (!contextShortcuts) {
    console.warn(`Unknown shortcut context: ${context}`);
    return null;
  }
  
  const actionShortcuts = contextShortcuts[action];
  if (!actionShortcuts) {
    console.warn(`Unknown shortcut action: ${action} in context ${context}`);
    return null;
  }
  
  if (actionShortcuts.key) {
    return actionShortcuts;
  }

  return actionShortcuts[currentPlatform] || actionShortcuts.other;
}

/**
 * Get the platform-specific modifier key property name
 * @returns {string} 'metaKey' for Mac, 'ctrlKey' for others
 */
export function getPlatformModifierKey() {
  return currentPlatform === 'mac' ? 'metaKey' : 'ctrlKey';
}

/**
 * Get the platform-specific modifier key symbol for display
 * @returns {string} '⌘' for Mac, 'Ctrl' for others
 */
export function getPlatformModifierSymbol() {
  return currentPlatform === 'mac' ? '⌘' : 'Ctrl';
}

/**
 * Check if a keyboard event matches a shortcut configuration
 * @param {KeyboardEvent} event - The keyboard event
 * @param {Object} shortcut - The shortcut configuration
 * @returns {boolean} True if event matches shortcut
 */
export function matchesShortcut(event, shortcut) {
  if (!shortcut) return false;

  // Check the key
  if (event.key.toLowerCase() !== shortcut.key.toLowerCase()) {
    return false;
  }

  // Handle the modifierKey property (accepts both Ctrl and Cmd on all platforms)
  if (shortcut.modifierKey) {
    if (!event.ctrlKey && !event.metaKey) {
      return false;
    }
  } else {
    // Check specific modifiers if modifierKey is not used
    if (!!event.ctrlKey !== !!shortcut.ctrlKey) return false;
    if (!!event.metaKey !== !!shortcut.metaKey) return false;
  }

  if (!!event.altKey !== !!shortcut.altKey) return false;
  if (!!event.shiftKey !== !!shortcut.shiftKey) return false;

  return true;
}

/**
 * Get a human-readable display string for a shortcut object
 * @param {Object} shortcut - The shortcut configuration object
 * @returns {string} Human-readable shortcut string
 */
export function getDisplayString(shortcut) {
  if (!shortcut) return '';

  const parts = [];

  // Add modifiers first
  if (shortcut.modifierKey) {
    parts.push(getPlatformModifierSymbol());
  } else {
    if (shortcut.ctrlKey) {
      parts.push('Ctrl');
    }
    if (shortcut.metaKey) {
      parts.push(currentPlatform === 'mac' ? '⌘' : 'Meta');
    }
  }
  if (shortcut.altKey) {
    parts.push(currentPlatform === 'mac' ? '⌥' : 'Alt');
  }
  if (shortcut.shiftKey) {
    parts.push(currentPlatform === 'mac' ? '⇧' : 'Shift');
  }

  // Add the key
  let keyDisplay = shortcut.key;
  if (keyDisplay === 'Enter') {
    keyDisplay = '↵';
  } else if (keyDisplay === 'Escape') {
    keyDisplay = 'Esc';
  }
  parts.push(keyDisplay.toUpperCase());

  // Use simple space as separator for clean, readable display
  return parts.join(' ');
}

/**
 * Get a human-readable display string for a shortcut by context and action
 * @param {string} context - The context
 * @param {string} action - The action
 * @returns {string} Human-readable shortcut string
 */
export function getShortcutDisplay(context, action) {
  const shortcut = getShortcut(context, action);
  return getDisplayString(shortcut);
}

/**
 * Check if a keyboard event originated from a text input field
 * (INPUT, TEXTAREA, SELECT, or contenteditable element)
 * @param {KeyboardEvent} event - The keyboard event
 * @returns {boolean} True if user is typing in an input field
 */
export function isTypingInField(event) {
  const target = event.target;
  const active = document.activeElement;
  return (
    target.tagName === 'INPUT' ||
    target.tagName === 'TEXTAREA' ||
    target.tagName === 'SELECT' ||
    target.isContentEditable ||
    active?.tagName === 'INPUT' ||
    active?.tagName === 'TEXTAREA' ||
    active?.tagName === 'SELECT' ||
    active?.isContentEditable
  );
}

/**
 * Create a keyboard event handler that matches shortcuts and calls actions
 * @param {Object} shortcuts - Map of shortcut names to handler functions
 * @param {string} context - The context for shortcuts
 * @param {Object} options - Optional settings
 * @param {Function} options.guard - Guard function, shortcuts disabled if returns false
 * @returns {Function} Event handler function
 */
export function createShortcutHandler(shortcuts, context, options = {}) {
  return (event) => {
    // Check guard first - skip if guard returns false
    if (options.guard && !options.guard()) {
      return;
    }

    // Don't handle shortcuts when typing in inputs, except for special keys
    const isSpecialKey = event.key === 'Enter' || event.key === 'Escape';

    // Allow Enter in INPUT fields (for form submission), but not in TEXTAREA (for new lines)
    // Allow Escape in all input fields (to cancel/close)
    if (isTypingInField(event) && !isSpecialKey) {
      return;
    }
    if (event.target.tagName === 'TEXTAREA' && event.key === 'Enter') {
      return;
    }

    for (const [actionName, handler] of Object.entries(shortcuts)) {
      const shortcut = getShortcut(context, actionName);
      if (matchesShortcut(event, shortcut)) {
        event.preventDefault();
        handler();
        break;
      }
    }
  };
}

export { currentPlatform };
