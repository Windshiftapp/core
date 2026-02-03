/**
 * Data transformation utilities for converting between API format and Svelte Flow format
 */

/**
 * Default positions for the standard workflow (workflow ID 1)
 * Creates a clean hierarchical layout: To Do → In Progress → Done
 */
export const DEFAULT_WORKFLOW_POSITIONS = {
  Open: { x: 80, y: 200 },
  'To Do': { x: 80, y: 350 },
  'In Progress': { x: 350, y: 200 },
  'Under Review': { x: 350, y: 350 },
  Done: { x: 600, y: 200 },
  Closed: { x: 600, y: 350 },
};

/**
 * Convert workflow statuses with positions to Svelte Flow nodes
 * @param {Array} workflowStatuses - Array of statuses with x,y positions
 * @returns {Array} Svelte Flow nodes array
 */
export function statusesToNodes(workflowStatuses, initialStatusId = null) {
  return workflowStatuses.map((status) => ({
    id: `status-${status.id}`,
    type: 'status',
    position: { x: status.x, y: status.y },
    data: {
      statusId: status.id,
      name: status.name,
      category_color: status.category_color,
      category_name: status.category_name,
      description: status.description,
      initial: initialStatusId !== null && status.id === initialStatusId,
    },
  }));
}

/**
 * Convert workflow transitions to Svelte Flow edges
 * @param {Array} transitions - Array of workflow transitions
 * @returns {Array} Svelte Flow edges array
 */
export function transitionsToEdges(transitions) {
  const handleOptions = ['top', 'right', 'bottom', 'left'];

  // Normalize source handle - remove any prefix
  const normalizeSourceHandle = (handle) => {
    if (!handle) return null;
    return handle.replace('target-', '');
  };

  return transitions
    .filter(
      (transition) =>
        transition.from_status_id !== null && transition.from_status_id !== transition.to_status_id
    )
    .map((transition, index) => {
      // Use saved handles if available, otherwise distribute intelligently
      const sourceHandle =
        normalizeSourceHandle(transition.source_handle) || handleOptions[index % 4];
      const targetHandleBase =
        normalizeSourceHandle(transition.target_handle) || handleOptions[(index + 2) % 4];
      const targetHandle = `target-${targetHandleBase}`;

      return {
        id: `edge-${transition.from_status_id}-${transition.to_status_id}`,
        type: 'reconnectable',
        source: `status-${transition.from_status_id}`,
        target: `status-${transition.to_status_id}`,
        sourceHandle,
        targetHandle,
        sourcePosition: sourceHandle,
        targetPosition: targetHandleBase,
        data: {
          transitionId: transition.id,
          workflow_id: transition.workflow_id,
          from_status_id: transition.from_status_id,
          to_status_id: transition.to_status_id,
          display_order: transition.display_order,
        },
      };
    });
}

/**
 * Convert Svelte Flow nodes back to workflow statuses format
 * @param {Array} nodes - Svelte Flow nodes array
 * @returns {Array} Workflow statuses with positions
 */
export function nodesToStatuses(nodes) {
  return nodes
    .filter((node) => node.type === 'status')
    .map((node) => ({
      id: node.data.statusId,
      name: node.data.name,
      category_color: node.data.category_color,
      category_name: node.data.category_name,
      description: node.data.description,
      x: node.position.x,
      y: node.position.y,
    }));
}

/**
 * Convert Svelte Flow edges back to workflow transitions format
 * @param {Array} edges - Svelte Flow edges array
 * @param {number} workflowId - Current workflow ID
 * @returns {Array} Workflow transitions
 */
export function edgesToTransitions(edges, workflowId) {
  return edges.map((edge, index) => ({
    id: edge.data?.transitionId || null,
    workflow_id: workflowId,
    from_status_id: edge.data?.from_status_id || parseInt(edge.source.replace('status-', ''), 10),
    to_status_id: edge.data?.to_status_id || parseInt(edge.target.replace('status-', ''), 10),
    display_order: edge.data?.display_order || index,
    source_handle: edge.sourceHandle || 'right',
    target_handle: edge.targetHandle || 'left',
  }));
}

/**
 * Create a new edge for Svelte Flow from status IDs
 * @param {number} fromStatusId - Source status ID
 * @param {number} toStatusId - Target status ID
 * @param {number} workflowId - Workflow ID
 * @returns {Object} Svelte Flow edge object
 */
export function createEdge(fromStatusId, toStatusId, workflowId) {
  return {
    id: `edge-${fromStatusId}-${toStatusId}`,
    type: 'reconnectable',
    source: `status-${fromStatusId}`,
    target: `status-${toStatusId}`,
    sourceHandle: 'right',
    targetHandle: 'target-left',
    sourcePosition: 'right',
    targetPosition: 'left',
    data: {
      workflow_id: workflowId,
      from_status_id: fromStatusId,
      to_status_id: toStatusId,
      display_order: 0,
    },
  };
}

/**
 * Add preservation transitions for disconnected statuses
 * @param {Array} statuses - Workflow statuses
 * @param {Array} transitions - Current transitions
 * @param {number} workflowId - Workflow ID
 * @returns {Array} Combined transitions including preservation ones
 */
export function addPreservationTransitions(statuses, transitions, workflowId) {
  const statusesInTransitions = new Set();
  transitions.forEach((t) => {
    if (t.from_status_id) statusesInTransitions.add(t.from_status_id);
    statusesInTransitions.add(t.to_status_id);
  });

  const preservationTransitions = [];
  statuses.forEach((status, index) => {
    if (!statusesInTransitions.has(status.id)) {
      preservationTransitions.push({
        workflow_id: workflowId,
        from_status_id: status.id,
        to_status_id: status.id,
        display_order: 1000 + index,
      });
    }
  });

  return [...transitions, ...preservationTransitions];
}

/**
 * Save/load node positions to/from localStorage
 */
export const positionPersistence = {
  save(workflowId, nodes) {
    const positions = {};
    nodes
      .filter((node) => node.type === 'status')
      .forEach((node) => {
        positions[node.data.statusId] = {
          x: node.position.x,
          y: node.position.y,
        };
      });

    const storageKey = `workflow_${workflowId}_positions`;
    localStorage.setItem(storageKey, JSON.stringify(positions));
  },

  load(workflowId) {
    const storageKey = `workflow_${workflowId}_positions`;
    const stored = localStorage.getItem(storageKey);

    if (stored) {
      try {
        return JSON.parse(stored);
      } catch (error) {
        console.error('Failed to parse stored positions:', error);
      }
    }

    return {};
  },
};
