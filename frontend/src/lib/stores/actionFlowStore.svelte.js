/**
 * Store for managing Action Flow Editor state.
 * Uses Svelte 5 class-based reactive state with immutable updates
 * to ensure proper reactivity in SvelteFlow canvas nodes.
 */
class ActionFlowStore {
  // Core state
  nodes = $state([]);
  edges = $state([]);
  selectedNodeId = $state(null);
  triggerType = $state('status_transition');
  statuses = $state([]);
  saving = $state(false);

  // Original action reference for API format conversion
  #action = null;

  /**
   * Get the currently selected node.
   * Returns the full node object or null if none selected.
   */
  get selectedNode() {
    if (!this.selectedNodeId) return null;
    return this.nodes.find(n => n.id === this.selectedNodeId) || null;
  }

  /**
   * Get the trigger node from the nodes array.
   */
  get triggerNode() {
    return this.nodes.find(n => n.type === 'trigger') || null;
  }

  /**
   * Initialize the store with action data and reference statuses.
   * @param {Object} action - The action object from the API
   * @param {Array} statuses - Available statuses for dropdowns
   */
  init(action, statuses = []) {
    this.#action = action;
    this.statuses = statuses;
    this.selectedNodeId = null;
    this.saving = false;

    if (!action) {
      this.nodes = [];
      this.edges = [];
      this.triggerType = 'status_transition';
      return;
    }

    this.triggerType = action.trigger_type || 'status_transition';

    // Convert action data to SvelteFlow format
    if (action.nodes && action.nodes.length > 0) {
      this.nodes = action.nodes.map(node => ({
        id: `node-${node.id}`,
        type: node.node_type,
        position: { x: node.position_x, y: node.position_y },
        data: {
          nodeId: node.id,
          config: this.#parseConfig(node.node_config),
          statuses
        }
      }));
    } else {
      // Create default trigger node
      this.nodes = [{
        id: 'node-trigger',
        type: 'trigger',
        position: { x: 100, y: 200 },
        data: {
          triggerType: action.trigger_type,
          config: this.#parseConfig(action.trigger_config),
          statuses
        }
      }];
    }

    if (action.edges && action.edges.length > 0) {
      this.edges = action.edges.map(edge => ({
        id: `edge-${edge.id}`,
        source: `node-${edge.source_node_id}`,
        target: `node-${edge.target_node_id}`,
        type: 'action',
        sourceHandle: edge.source_handle,
        targetHandle: edge.target_handle,
        data: {
          edgeType: edge.edge_type,
          sourceHandle: edge.source_handle,
          targetHandle: edge.target_handle
        }
      }));
    } else {
      this.edges = [];
    }
  }

  /**
   * Update a node's config with immutable update pattern.
   * Creates new node objects to trigger Svelte reactivity.
   * @param {string} nodeId - The node ID to update
   * @param {Object} configUpdates - Object with config properties to merge
   */
  updateNodeConfig(nodeId, configUpdates) {
    this.nodes = this.nodes.map(node => {
      if (node.id !== nodeId) return node;
      return {
        ...node,
        data: {
          ...node.data,
          config: { ...node.data?.config, ...configUpdates }
        }
      };
    });
  }

  /**
   * Update a node's data (not just config) with immutable pattern.
   * @param {string} nodeId - The node ID to update
   * @param {Object} dataUpdates - Object with data properties to merge
   */
  updateNodeData(nodeId, dataUpdates) {
    this.nodes = this.nodes.map(node => {
      if (node.id !== nodeId) return node;
      return {
        ...node,
        data: {
          ...node.data,
          ...dataUpdates
        }
      };
    });
  }

  /**
   * Update the trigger type.
   * Also updates the trigger node's data.
   * @param {string} type - New trigger type
   */
  updateTriggerType(type) {
    this.triggerType = type;

    // Update the trigger node's data
    const triggerNode = this.triggerNode;
    if (triggerNode) {
      this.updateNodeData(triggerNode.id, { triggerType: type });
    }
  }

  /**
   * Update a node's position with immutable pattern.
   * @param {string} nodeId - The node ID to update
   * @param {Object} position - New position { x, y }
   */
  updateNodePosition(nodeId, position) {
    this.nodes = this.nodes.map(node => {
      if (node.id !== nodeId) return node;
      return {
        ...node,
        position: { ...position }
      };
    });
  }

  /**
   * Add a new node to the flow.
   * @param {string} nodeType - Type of node to add
   * @param {Object} position - Optional position, defaults to random offset
   */
  addNode(nodeType, position = null) {
    const newNode = {
      id: `node-${Date.now()}`,
      type: nodeType,
      position: position || {
        x: 300 + Math.random() * 200,
        y: 100 + Math.random() * 300
      },
      data: {
        config: this.#getDefaultConfig(nodeType),
        statuses: this.statuses
      }
    };

    this.nodes = [...this.nodes, newNode];
    return newNode;
  }

  /**
   * Remove a node from the flow.
   * Also removes any connected edges.
   * @param {string} nodeId - The node ID to remove
   */
  removeNode(nodeId) {
    this.nodes = this.nodes.filter(node => node.id !== nodeId);
    this.edges = this.edges.filter(edge =>
      edge.source !== nodeId && edge.target !== nodeId
    );

    // Clear selection if removed node was selected
    if (this.selectedNodeId === nodeId) {
      this.selectedNodeId = null;
    }
  }

  /**
   * Select a node by ID.
   * @param {string} nodeId - The node ID to select
   */
  selectNode(nodeId) {
    this.selectedNodeId = nodeId;
  }

  /**
   * Clear the current node selection.
   */
  clearSelection() {
    this.selectedNodeId = null;
  }

  /**
   * Add an edge from a connection event.
   * @param {Object} connection - Connection params from SvelteFlow
   */
  addEdge(connection) {
    const { source, target, sourceHandle, targetHandle } = connection;

    // Determine edge type based on connection
    let edgeType = 'default';
    if (sourceHandle === 'true' || sourceHandle === 'false') {
      edgeType = sourceHandle;
    }

    const newEdge = {
      id: `edge-${Date.now()}`,
      source,
      target,
      type: 'action',
      sourceHandle,
      targetHandle,
      data: { edgeType }
    };

    this.edges = [...this.edges, newEdge];
    return newEdge;
  }

  /**
   * Remove edges by their IDs.
   * @param {Array<string>} edgeIds - Array of edge IDs to remove
   */
  removeEdges(edgeIds) {
    this.edges = this.edges.filter(edge => !edgeIds.includes(edge.id));
  }

  /**
   * Update edges array (for SvelteFlow compatibility).
   * @param {Array} newEdges - New edges array
   */
  setEdges(newEdges) {
    this.edges = newEdges;
  }

  /**
   * Update an existing edge with new connection info.
   * Used for edge reconnection.
   * @param {string} edgeId - The edge ID to update
   * @param {Object} updates - Object with edge properties to merge (source, target, sourceHandle, targetHandle)
   */
  updateEdge(edgeId, updates) {
    this.edges = this.edges.map(edge => {
      if (edge.id !== edgeId) return edge;

      // Determine edgeType based on new sourceHandle
      const sourceHandle = updates.sourceHandle ?? edge.sourceHandle;
      const edgeType = sourceHandle === 'true' || sourceHandle === 'false'
        ? sourceHandle
        : 'default';

      return {
        ...edge,
        ...updates,
        data: { ...edge.data, edgeType }
      };
    });
  }

  /**
   * Update nodes array (for SvelteFlow compatibility).
   * @param {Array} newNodes - New nodes array
   */
  setNodes(newNodes) {
    this.nodes = newNodes;
  }

  /**
   * Set the saving state.
   * @param {boolean} isSaving
   */
  setSaving(isSaving) {
    this.saving = isSaving;
  }

  /**
   * Convert current store state to API format.
   * @param {Object} baseAction - Base action object to merge with
   * @returns {Object} Action data in API format
   */
  toApiFormat(baseAction = this.#action) {
    // Build node ID mapping
    const nodeIdMap = {};
    const actionNodes = this.nodes.map((node, index) => {
      const nodeId = node.data?.nodeId || index + 1;
      nodeIdMap[node.id] = nodeId;
      return {
        id: nodeId,
        action_id: baseAction?.id,
        node_type: node.type,
        node_config: JSON.stringify(node.data?.config || {}),
        position_x: node.position.x,
        position_y: node.position.y
      };
    });

    const actionEdges = this.edges.map((edge, index) => ({
      id: index + 1,
      action_id: baseAction?.id,
      source_node_id: nodeIdMap[edge.source] || parseInt(edge.source.replace('node-', '')),
      target_node_id: nodeIdMap[edge.target] || parseInt(edge.target.replace('node-', '')),
      edge_type: edge.data?.edgeType || 'default',
      source_handle: edge.sourceHandle,
      target_handle: edge.targetHandle
    }));

    // Get trigger config from trigger node
    const triggerNode = this.triggerNode;
    const triggerConfig = triggerNode?.data?.config
      ? JSON.stringify(triggerNode.data.config)
      : baseAction?.trigger_config;

    return {
      ...baseAction,
      trigger_type: this.triggerType,
      trigger_config: triggerConfig,
      nodes: actionNodes,
      edges: actionEdges
    };
  }

  /**
   * Reset the store to initial state.
   */
  reset() {
    this.nodes = [];
    this.edges = [];
    this.selectedNodeId = null;
    this.triggerType = 'status_transition';
    this.statuses = [];
    this.saving = false;
    this.#action = null;
  }

  // Private helper methods

  #parseConfig(config) {
    if (!config) return {};
    try {
      return typeof config === 'string' ? JSON.parse(config) : config;
    } catch {
      return {};
    }
  }

  #getDefaultConfig(nodeType) {
    switch (nodeType) {
      case 'set_field':
        return { field_name: '', value: '' };
      case 'set_status':
        return { status_id: null };
      case 'add_comment':
        return { content: '', is_private: false };
      case 'notify_user':
        return { recipient_type: 'assignee', recipients: [], message: '', include_link: true };
      case 'condition':
        return { field_name: '', operator: 'eq', value: '' };
      case 'update_asset':
        return { source_field_id: '', asset_set_id: 0, asset_type_id: 0, field_mappings: [] };
      case 'create_asset':
        return {
          asset_set_id: 0,
          asset_type_id: 0,
          title: '',
          description: '',
          asset_tag: '',
          category_id: null,
          status_id: null,
          field_mappings: []
        };
      default:
        return {};
    }
  }
}

export const actionFlowStore = new ActionFlowStore();
