/**
 * Context-sensitive command store for the Command Palette
 * Allows components to dynamically register/unregister commands based on their context
 */
import { derived, writable } from 'svelte/store';

// Store for all registered context commands
const registeredCommands = writable(new Map());

// Derived store that provides a flat array of all context commands
export const contextCommands = derived(registeredCommands, ($registeredCommands) => {
  const allCommands = [];

  // Flatten all commands from all registered components
  for (const [componentId, commands] of $registeredCommands.entries()) {
    for (const command of commands) {
      allCommands.push({
        ...command,
        // Add metadata about which component registered this command
        _registeredBy: componentId,
        _isContextCommand: true,
      });
    }
  }

  // Sort by priority: higher priority first
  return allCommands.sort((a, b) => {
    const aPriority = a.priority || 0;
    const bPriority = b.priority || 0;
    return bPriority - aPriority;
  });
});

/**
 * Register context commands for a component
 * @param {string} componentId - Unique identifier for the component
 * @param {Array} commands - Array of command objects
 */
export function registerContextCommands(componentId, commands) {
  if (!componentId) {
    console.error('registerContextCommands: componentId is required');
    return;
  }

  if (!Array.isArray(commands)) {
    console.error('registerContextCommands: commands must be an array');
    return;
  }

  // Validate command structure
  const validCommands = commands.filter((command) => {
    if (!command.id || !command.label) {
      console.error('registerContextCommands: command must have id and label', command);
      return false;
    }
    return true;
  });

  registeredCommands.update((map) => {
    map.set(componentId, validCommands);
    return map;
  });
}

/**
 * Unregister all commands for a component
 * @param {string} componentId - Unique identifier for the component
 */
export function unregisterContextCommands(componentId) {
  if (!componentId) return;

  registeredCommands.update((map) => {
    map.delete(componentId);
    return map;
  });
}

/**
 * Update commands for a component (replaces existing commands)
 * @param {string} componentId - Unique identifier for the component
 * @param {Array} commands - Array of command objects
 */
export function updateContextCommands(componentId, commands) {
  // This is essentially the same as register, but semantically different
  registerContextCommands(componentId, commands);
}

/**
 * Get all registered commands (for debugging)
 */
export function getAllRegisteredCommands() {
  let allCommands = {};
  registeredCommands.subscribe((map) => {
    allCommands = Object.fromEntries(map);
  })();
  return allCommands;
}

/**
 * Helper function to create a context command object
 * @param {Object} config - Command configuration
 * @returns {Object} Formatted command object
 */
export function createContextCommand(config) {
  const {
    id,
    label,
    description,
    keywords = [],
    action,
    icon,
    priority = 10, // Context commands get higher priority than base commands
    enabled = true,
    visible = true,
    shortcut,
    category = 'context',
  } = config;

  return {
    id,
    label,
    description,
    keywords: Array.isArray(keywords) ? keywords : [keywords],
    action,
    icon,
    priority,
    enabled,
    visible,
    shortcut,
    category,
    type: 'context-action',
  };
}

/**
 * Pre-defined command categories for consistent grouping
 */
export const COMMAND_CATEGORIES = {
  CONTEXT: 'context',
  NAVIGATION: 'navigation',
  CREATE: 'create',
  ACTION: 'action',
  TIME: 'time',
  TEST: 'test',
  ADMIN: 'admin',
};

/**
 * Priority levels for consistent command ordering
 */
export const COMMAND_PRIORITIES = {
  URGENT: 100, // Critical context-specific actions
  HIGH: 50, // Important context actions
  NORMAL: 10, // Default context commands
  LOW: 5, // Less important context commands
  WORKSPACE: 3, // Workspace-level commands
  GLOBAL: 1, // Global application commands
};

// Export the store subscription function for external use
export const subscribe = contextCommands.subscribe;
