/**
 * Utility functions for building and managing folder tree structures.
 * Used for test case folders and other hierarchical folder systems.
 */

/**
 * Builds a hierarchical tree from a flat array of folders.
 * Computes total counts including nested children.
 *
 * @param {Array} folders - Flat array of folder objects with id, parent_id, name
 * @param {string} countField - Field name containing the direct count (default: 'test_case_count')
 * @returns {Array} Array of root-level folder nodes with children arrays
 */
export function buildFolderTree(folders = [], countField = 'test_case_count') {
  const folderMap = new Map();
  (folders || []).forEach((folder) => {
    folderMap.set(folder.id, {
      ...folder,
      children: [],
      total_count: folder[countField] || 0,
    });
  });

  folderMap.forEach((folder) => {
    if (folder.parent_id && folderMap.has(folder.parent_id)) {
      folderMap.get(folder.parent_id).children.push(folder);
    }
  });

  const roots = [];
  folderMap.forEach((folder) => {
    if (!folder.parent_id || !folderMap.has(folder.parent_id)) {
      roots.push(folder);
    }
  });

  const sortNodes = (nodes) => {
    nodes.sort((a, b) => {
      const orderDiff = (a.sort_order || 0) - (b.sort_order || 0);
      if (orderDiff !== 0) return orderDiff;
      return a.name.localeCompare(b.name);
    });
    nodes.forEach((child) => sortNodes(child.children));
  };

  const computeTotals = (node) => {
    const childTotal = node.children.reduce((sum, child) => sum + computeTotals(child), 0);
    node.total_count = (node[countField] || 0) + childTotal;
    return node.total_count;
  };

  sortNodes(roots);
  roots.forEach((node) => computeTotals(node));
  return roots;
}

/**
 * Flattens a hierarchical tree into an array with depth information.
 * Respects collapsed state to hide children of collapsed folders.
 *
 * @param {Array} tree - Hierarchical tree from buildFolderTree
 * @param {Set} collapsed - Set of collapsed folder IDs
 * @returns {Array} Array of { node, depth } objects
 */
export function flattenFolderTree(tree = [], collapsed = new Set()) {
  const result = [];
  const traverse = (nodes, depth = 0) => {
    nodes.forEach((node) => {
      result.push({ node, depth });
      if (node.children && node.children.length > 0 && !collapsed.has(node.id)) {
        traverse(node.children, depth + 1);
      }
    });
  };
  traverse(tree, 0);
  return result;
}

/**
 * Gets the display path for a folder (parent/child format).
 *
 * @param {number|null} folderId - The folder ID to get path for
 * @param {Array} folders - Flat array of all folders
 * @returns {string|null} Path string or null if folder not found
 */
export function getFolderPath(folderId, folders = []) {
  if (folderId === null || folderId === undefined) {
    return null;
  }
  const folder = folders.find((f) => f.id === folderId);
  if (!folder) return null;
  if (folder.parent_id) {
    const parent = folders.find((f) => f.id === folder.parent_id);
    return parent ? `${parent.name} / ${folder.name}` : folder.name;
  }
  return folder.name;
}

/**
 * Gets the display count for a folder based on depth.
 * Root folders show total count, nested folders show direct count.
 *
 * @param {Object} folder - Folder node from tree
 * @param {number} depth - Depth level (0 = root)
 * @param {string} countField - Field name for direct count (default: 'test_case_count')
 * @returns {number} Count to display
 */
export function getFolderDisplayCount(folder, depth, countField = 'test_case_count') {
  if (depth === 0) {
    return folder.total_count ?? folder[countField] ?? 0;
  }
  return folder[countField] ?? 0;
}

/**
 * Calculates indentation for nested folder display.
 *
 * @param {number} depth - Depth level
 * @param {number} baseIndent - Base indentation in pixels (default: 12)
 * @param {number} stepIndent - Additional indentation per level (default: 16)
 * @returns {string} CSS padding-left value
 */
export function getFolderIndent(depth = 0, baseIndent = 12, stepIndent = 16) {
  return `${baseIndent + depth * stepIndent}px`;
}

/**
 * Creates a collapsed state manager with reactive updates.
 *
 * @param {Set} initial - Initial collapsed folder IDs
 * @returns {Object} Manager with toggle, isCollapsed, and getSet methods
 */
export function createCollapsedState(initial = new Set()) {
  let collapsed = new Set(initial);

  return {
    toggle(folderId) {
      if (collapsed.has(folderId)) {
        collapsed.delete(folderId);
      } else {
        collapsed.add(folderId);
      }
      collapsed = new Set(collapsed); // Trigger reactivity
      return collapsed;
    },

    isCollapsed(folderId) {
      return collapsed.has(folderId);
    },

    getSet() {
      return collapsed;
    },
  };
}

/**
 * Filters root-level folders and sorts them.
 *
 * @param {Array} folders - Flat array of all folders
 * @returns {Array} Sorted array of root-level folders
 */
export function getRootFolderOptions(folders = []) {
  return (folders || [])
    .filter((folder) => folder.parent_id === null || folder.parent_id === undefined)
    .sort((a, b) => {
      const orderDiff = (a.sort_order || 0) - (b.sort_order || 0);
      if (orderDiff !== 0) return orderDiff;
      return a.name.localeCompare(b.name);
    });
}
