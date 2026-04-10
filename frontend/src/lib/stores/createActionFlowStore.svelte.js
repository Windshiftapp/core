/**
 * Factory for creating Action Flow stores.
 * Eliminates duplication between domain-specific flow stores by sharing
 * all common SvelteFlow canvas logic.
 *
 * @param {Object} options
 * @param {string} options.defaultTrigger - Default trigger type for this domain
 * @param {Record<string, Object>} options.nodeConfigDefaults - Map of nodeType → default config object
 * @param {boolean} [options.includeStatuses=false] - Whether to track statuses state and pass to nodes
 */
export function createActionFlowStore({
  defaultTrigger,
  nodeConfigDefaults,
  includeStatuses = false,
}) {
  // Core state
  let nodes = $state([]);
  let edges = $state([]);
  let selectedNodeId = $state(null);
  let triggerType = $state(defaultTrigger);
  let saving = $state(false);
  let direction = $state('horizontal');

  let statuses = $state(includeStatuses ? [] : null);

  // Original action reference for API format conversion
  let _action = null;

  function parseConfig(config) {
    if (!config) return {};
    try {
      return typeof config === 'string' ? JSON.parse(config) : config;
    } catch {
      return {};
    }
  }

  function getDefaultConfig(nodeType) {
    return nodeConfigDefaults[nodeType] ? { ...nodeConfigDefaults[nodeType] } : {};
  }

  const store = {
    get nodes() {
      return nodes;
    },
    set nodes(value) {
      nodes = value;
    },
    get edges() {
      return edges;
    },
    set edges(value) {
      edges = value;
    },
    get selectedNodeId() {
      return selectedNodeId;
    },
    set selectedNodeId(value) {
      selectedNodeId = value;
    },
    get triggerType() {
      return triggerType;
    },
    set triggerType(value) {
      triggerType = value;
    },
    get saving() {
      return saving;
    },
    set saving(value) {
      saving = value;
    },
    get direction() {
      return direction;
    },
    set direction(value) {
      direction = value;
    },

    get selectedNode() {
      if (!selectedNodeId) return null;
      return nodes.find((n) => n.id === selectedNodeId) || null;
    },

    get triggerNode() {
      return nodes.find((n) => n.type === 'trigger') || null;
    },

    get statuses() {
      return includeStatuses ? statuses : undefined;
    },

    init(action, initStatuses = []) {
      _action = action;
      if (includeStatuses) statuses = initStatuses;
      selectedNodeId = null;
      saving = false;

      if (!action) {
        nodes = [];
        edges = [];
        triggerType = defaultTrigger;
        return;
      }

      triggerType = action.trigger_type || defaultTrigger;

      if (action.nodes && action.nodes.length > 0) {
        nodes = action.nodes.map((node) => ({
          id: `node-${node.id}`,
          type: node.node_type,
          position: { x: node.position_x, y: node.position_y },
          data: {
            nodeId: node.id,
            config: parseConfig(node.node_config),
            ...(includeStatuses ? { statuses: initStatuses } : {}),
          },
        }));
      } else {
        nodes = [
          {
            id: 'node-trigger',
            type: 'trigger',
            position: { x: 100, y: 200 },
            data: {
              triggerType: action.trigger_type,
              config: parseConfig(action.trigger_config),
              ...(includeStatuses ? { statuses: initStatuses } : {}),
            },
          },
        ];
      }

      if (action.edges && action.edges.length > 0) {
        edges = action.edges.map((edge) => ({
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
        edges = [];
      }
    },

    updateNodeConfig(nodeId, configUpdates) {
      nodes = nodes.map((node) => {
        if (node.id !== nodeId) return node;
        return {
          ...node,
          data: {
            ...node.data,
            config: { ...node.data?.config, ...configUpdates },
          },
        };
      });
    },

    updateNodeData(nodeId, dataUpdates) {
      nodes = nodes.map((node) => {
        if (node.id !== nodeId) return node;
        return {
          ...node,
          data: {
            ...node.data,
            ...dataUpdates,
          },
        };
      });
    },

    updateTriggerType(type) {
      triggerType = type;
      const trigger = store.triggerNode;
      if (trigger) {
        store.updateNodeData(trigger.id, { triggerType: type });
      }
    },

    updateNodePosition(nodeId, position) {
      nodes = nodes.map((node) => {
        if (node.id !== nodeId) return node;
        return {
          ...node,
          position: { ...position },
        };
      });
    },

    toggleDirection() {
      direction = direction === 'horizontal' ? 'vertical' : 'horizontal';
    },

    addNode(nodeType, position = null) {
      const isVertical = direction === 'vertical';
      const newNode = {
        id: `node-${Date.now()}`,
        type: nodeType,
        position: position || {
          x: isVertical ? 100 + Math.random() * 300 : 300 + Math.random() * 200,
          y: isVertical ? 300 + Math.random() * 200 : 100 + Math.random() * 300,
        },
        data: {
          config: getDefaultConfig(nodeType),
          ...(includeStatuses ? { statuses } : {}),
        },
      };

      nodes = [...nodes, newNode];
      return newNode;
    },

    removeNode(nodeId) {
      nodes = nodes.filter((node) => node.id !== nodeId);
      edges = edges.filter((edge) => edge.source !== nodeId && edge.target !== nodeId);
      if (selectedNodeId === nodeId) {
        selectedNodeId = null;
      }
    },

    selectNode(nodeId) {
      selectedNodeId = nodeId;
    },

    clearSelection() {
      selectedNodeId = null;
    },

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

      edges = [...edges, newEdge];
      return newEdge;
    },

    removeEdges(edgeIds) {
      edges = edges.filter((edge) => !edgeIds.includes(edge.id));
    },

    setEdges(newEdges) {
      edges = newEdges;
    },

    updateEdge(edgeId, updates) {
      edges = edges.map((edge) => {
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
    },

    setNodes(newNodes) {
      nodes = newNodes;
    },

    setSaving(isSaving) {
      saving = isSaving;
    },

    toApiFormat(baseAction = _action) {
      const nodeIdMap = {};
      const actionNodes = nodes.map((node, index) => {
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

      const actionEdges = edges.map((edge, index) => ({
        id: index + 1,
        action_id: baseAction?.id,
        source_node_id: nodeIdMap[edge.source] || parseInt(edge.source.replace('node-', ''), 10),
        target_node_id: nodeIdMap[edge.target] || parseInt(edge.target.replace('node-', ''), 10),
        edge_type: edge.data?.edgeType || 'default',
        source_handle: edge.sourceHandle,
        target_handle: edge.targetHandle,
      }));

      const trigger = store.triggerNode;
      const triggerConfig = trigger?.data?.config
        ? JSON.stringify(trigger.data.config)
        : baseAction?.trigger_config;

      return {
        ...baseAction,
        trigger_type: triggerType,
        trigger_config: triggerConfig,
        nodes: actionNodes,
        edges: actionEdges,
      };
    },

    reset() {
      nodes = [];
      edges = [];
      selectedNodeId = null;
      triggerType = defaultTrigger;
      if (includeStatuses) statuses = [];
      saving = false;
      direction = 'horizontal';
      _action = null;
    },
  };

  return store;
}
