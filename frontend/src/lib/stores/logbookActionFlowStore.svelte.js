/**
 * Store for managing Logbook Action Flow Editor state.
 * Uses Svelte 5 class-based reactive state with immutable updates.
 * Pattern follows actionFlowStore.svelte.js but with logbook-specific node types.
 */
class LogbookActionFlowStore {
  // Core state
  nodes = $state([]);
  edges = $state([]);
  selectedNodeId = $state(null);
  triggerType = $state('document_classified');
  saving = $state(false);
  direction = $state('horizontal');

  // Original action reference for API format conversion
  #action = null;

  get selectedNode() {
    if (!this.selectedNodeId) return null;
    return this.nodes.find((n) => n.id === this.selectedNodeId) || null;
  }

  get triggerNode() {
    return this.nodes.find((n) => n.type === 'trigger') || null;
  }

  /**
   * Initialize the store with logbook action data.
   * @param {Object} action - The logbook action object from the API
   */
  init(action) {
    this.#action = action;
    this.selectedNodeId = null;
    this.saving = false;

    if (!action) {
      this.nodes = [];
      this.edges = [];
      this.triggerType = 'document_classified';
      return;
    }

    this.triggerType = action.trigger_type || 'document_classified';

    if (action.nodes && action.nodes.length > 0) {
      this.nodes = action.nodes.map((node) => ({
        id: `node-${node.id}`,
        type: node.node_type,
        position: { x: node.position_x, y: node.position_y },
        data: {
          nodeId: node.id,
          config: this.#parseConfig(node.node_config),
        },
      }));
    } else {
      this.nodes = [
        {
          id: 'node-trigger',
          type: 'trigger',
          position: { x: 100, y: 200 },
          data: {
            triggerType: action.trigger_type,
            config: this.#parseConfig(action.trigger_config),
          },
        },
      ];
    }

    if (action.edges && action.edges.length > 0) {
      this.edges = action.edges.map((edge) => ({
        id: `edge-${edge.id}`,
        source: `node-${edge.source_node_id}`,
        target: `node-${edge.target_node_id}`,
        type: 'action',
        sourceHandle: edge.source_handle,
        targetHandle: edge.target_handle,
        data: {
          edgeType: edge.edge_type,
          sourceHandle: edge.source_handle,
          targetHandle: edge.target_handle,
        },
      }));
    } else {
      this.edges = [];
    }
  }

  updateNodeConfig(nodeId, configUpdates) {
    this.nodes = this.nodes.map((node) => {
      if (node.id !== nodeId) return node;
      return {
        ...node,
        data: {
          ...node.data,
          config: { ...node.data?.config, ...configUpdates },
        },
      };
    });
  }

  updateNodeData(nodeId, dataUpdates) {
    this.nodes = this.nodes.map((node) => {
      if (node.id !== nodeId) return node;
      return {
        ...node,
        data: {
          ...node.data,
          ...dataUpdates,
        },
      };
    });
  }

  updateTriggerType(type) {
    this.triggerType = type;
    const triggerNode = this.triggerNode;
    if (triggerNode) {
      this.updateNodeData(triggerNode.id, { triggerType: type });
    }
  }

  updateNodePosition(nodeId, position) {
    this.nodes = this.nodes.map((node) => {
      if (node.id !== nodeId) return node;
      return {
        ...node,
        position: { ...position },
      };
    });
  }

  toggleDirection() {
    this.direction = this.direction === 'horizontal' ? 'vertical' : 'horizontal';
  }

  addNode(nodeType, position = null) {
    const isVertical = this.direction === 'vertical';
    const newNode = {
      id: `node-${Date.now()}`,
      type: nodeType,
      position: position || {
        x: isVertical ? 100 + Math.random() * 300 : 300 + Math.random() * 200,
        y: isVertical ? 300 + Math.random() * 200 : 100 + Math.random() * 300,
      },
      data: {
        config: this.#getDefaultConfig(nodeType),
      },
    };

    this.nodes = [...this.nodes, newNode];
    return newNode;
  }

  removeNode(nodeId) {
    this.nodes = this.nodes.filter((node) => node.id !== nodeId);
    this.edges = this.edges.filter((edge) => edge.source !== nodeId && edge.target !== nodeId);
    if (this.selectedNodeId === nodeId) {
      this.selectedNodeId = null;
    }
  }

  selectNode(nodeId) {
    this.selectedNodeId = nodeId;
  }

  clearSelection() {
    this.selectedNodeId = null;
  }

  addEdge(connection) {
    const { source, target, sourceHandle, targetHandle } = connection;

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
      data: { edgeType },
    };

    this.edges = [...this.edges, newEdge];
    return newEdge;
  }

  removeEdges(edgeIds) {
    this.edges = this.edges.filter((edge) => !edgeIds.includes(edge.id));
  }

  setEdges(newEdges) {
    this.edges = newEdges;
  }

  updateEdge(edgeId, updates) {
    this.edges = this.edges.map((edge) => {
      if (edge.id !== edgeId) return edge;
      const sourceHandle = updates.sourceHandle ?? edge.sourceHandle;
      const edgeType =
        sourceHandle === 'true' || sourceHandle === 'false' ? sourceHandle : 'default';
      return {
        ...edge,
        ...updates,
        data: { ...edge.data, edgeType },
      };
    });
  }

  setNodes(newNodes) {
    this.nodes = newNodes;
  }

  setSaving(isSaving) {
    this.saving = isSaving;
  }

  /**
   * Convert current store state to API format.
   * @param {Object} baseAction - Base action object to merge with
   * @returns {Object} Action data in API format
   */
  toApiFormat(baseAction = this.#action) {
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
        position_y: node.position.y,
      };
    });

    const actionEdges = this.edges.map((edge, index) => ({
      id: index + 1,
      action_id: baseAction?.id,
      source_node_id: nodeIdMap[edge.source] || parseInt(edge.source.replace('node-', ''), 10),
      target_node_id: nodeIdMap[edge.target] || parseInt(edge.target.replace('node-', ''), 10),
      edge_type: edge.data?.edgeType || 'default',
      source_handle: edge.sourceHandle,
      target_handle: edge.targetHandle,
    }));

    const triggerNode = this.triggerNode;
    const triggerConfig = triggerNode?.data?.config
      ? JSON.stringify(triggerNode.data.config)
      : baseAction?.trigger_config;

    return {
      ...baseAction,
      trigger_type: this.triggerType,
      trigger_config: triggerConfig,
      nodes: actionNodes,
      edges: actionEdges,
    };
  }

  reset() {
    this.nodes = [];
    this.edges = [];
    this.selectedNodeId = null;
    this.triggerType = 'document_classified';
    this.saving = false;
    this.direction = 'horizontal';
    this.#action = null;
  }

  // Private helpers

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
      case 'create_item':
        return {
          workspace_id: 0,
          item_type_id: 0,
          title: '{{doc.title}}',
          description: 'Source: {{doc.link}}',
        };
      case 'create_asset':
        return {
          asset_set_id: 0,
          asset_type_id: 0,
          title: '{{doc.title}}',
          description: '',
          asset_tag: '',
          category_id: null,
          status_id: null,
          field_mappings: [],
        };
      case 'associate_customer':
        return { customer_organisation_id: null, portal_customer_id: null };
      case 'condition':
        return { field_name: 'content_type', operator: 'eq', value: '' };
      default:
        return {};
    }
  }
}

export const logbookActionFlowStore = new LogbookActionFlowStore();
