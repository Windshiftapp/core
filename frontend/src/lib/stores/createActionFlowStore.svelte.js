export function getActionNotificationRecipients(config = {}) {
  if (config.recipients?.length) return config.recipients;
  if (Number(config.user_id) > 0) return [String(config.user_id)];
  if (config.recipient_type)
    return config.recipient_type === 'specific' ? [] : [config.recipient_type];
  return [];
}

/**
 * Factory for creating Action Flow stores.
 * Eliminates duplication between domain-specific flow stores by sharing
 * all common SvelteFlow canvas logic.
 *
 * @param {Object} options
 * @param {string} options.defaultTrigger - Default trigger type for this domain
 * @param {Record<string, Object>} options.nodeConfigDefaults - Map of nodeType → default config object
 */
export function createActionFlowStore({ defaultTrigger, nodeConfigDefaults }) {
  // Core state
  let nodes = $state([]);
  let edges = $state([]);
  let selectedNodeId = $state(null);
  let triggerType = $state(defaultTrigger);
  let saving = $state(false);
  let direction = $state('horizontal');

  let clientNodeSeq = 0;
  let clientEdgeSeq = 0;

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

  function normalizeNodeConfig(nodeType, config) {
    const normalized = { ...(config || {}) };

    if (
      nodeType === 'notify_user' &&
      !normalized.recipients?.length &&
      Number(normalized.user_id) > 0
    ) {
      normalized.recipient_type = 'specific';
      normalized.recipients = getActionNotificationRecipients(normalized);
    }

    if (nodeType === 'notify_user' && normalized.recipient_type && !normalized.recipients?.length) {
      normalized.recipients = getActionNotificationRecipients(normalized);
    }

    if (nodeType === 'notify_user') {
      delete normalized.user_id;
    }

    // Output field names are validated against a trimmed value, so trim before
    // serializing too — otherwise " response " passes validation but persists
    // with the surrounding spaces.
    if (typeof normalized.output_field === 'string') {
      normalized.output_field = normalized.output_field.trim();
    }

    // Optional capability on http_request should be omitted when cleared;
    // JSON null fails the server schema because the field is int-or-absent.
    if (nodeType === 'http_request' && normalized.capability_id == null) {
      delete normalized.capability_id;
    }

    // Drop an empty headers map so the payload stays clean (server treats it
    // as omitempty anyway).
    if (
      nodeType === 'http_request' &&
      normalized.headers &&
      typeof normalized.headers === 'object' &&
      Object.keys(normalized.headers).length === 0
    ) {
      delete normalized.headers;
    }

    return normalized;
  }

  // hydrateNodeConfig massages a freshly-loaded node config so legacy payloads
  // render in the friendly editor controls. Runs once on load, not on save.
  function hydrateNodeConfig(nodeType, config) {
    const hydrated = { ...(config || {}) };

    if (
      nodeType === 'notify_user' &&
      !hydrated.recipients?.length &&
      Number(hydrated.user_id) > 0
    ) {
      hydrated.recipient_type = 'specific';
      hydrated.recipients = getActionNotificationRecipients(hydrated);
    }

    // Legacy notify_user configs stored specific user IDs in `recipients`
    // without a `recipient_type`. Without this, the recipient select falls back
    // to recipients[0] (a numeric ID) and the specific-recipient section, which
    // is gated on recipient_type === 'specific', never renders.
    if (
      nodeType === 'notify_user' &&
      !hydrated.recipient_type &&
      Array.isArray(hydrated.recipients) &&
      hydrated.recipients.length > 0 &&
      hydrated.recipients.every((r) => /^\d+$/.test(String(r)))
    ) {
      hydrated.recipient_type = 'specific';
    }

    return hydrated;
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

    init(action) {
      _action = action;
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
        nodes = action.nodes.map((node) => {
          const isTrigger = node.node_type === 'trigger';
          return {
            id: `node-${node.id}`,
            type: node.node_type,
            position: { x: node.position_x, y: node.position_y },
            deletable: !isTrigger,
            data: {
              nodeType: node.node_type,
              nodeId: node.id,
              ...(isTrigger ? { triggerType: action.trigger_type } : {}),
              config: isTrigger
                ? parseConfig(action.trigger_config)
                : hydrateNodeConfig(node.node_type, parseConfig(node.node_config)),
            },
          };
        });
      } else {
        nodes = [
          {
            id: 'node-trigger',
            type: 'trigger',
            position: { x: 100, y: 200 },
            deletable: false,
            data: {
              nodeType: 'trigger',
              triggerType: action.trigger_type,
              config: parseConfig(action.trigger_config),
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
        id: `node-new-${Date.now()}-${++clientNodeSeq}`,
        type: nodeType,
        position: position || {
          x: isVertical ? 100 + Math.random() * 300 : 300 + Math.random() * 200,
          y: isVertical ? 300 + Math.random() * 200 : 100 + Math.random() * 300,
        },
        data: {
          nodeType,
          config: getDefaultConfig(nodeType),
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
        id: `edge-new-${Date.now()}-${++clientEdgeSeq}`,
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
      const trigger = store.triggerNode;
      const triggerConfig = trigger?.data?.config
        ? JSON.stringify(normalizeNodeConfig('trigger', trigger.data.config))
        : baseAction?.trigger_config;

      const nodeIdMap = {};
      const usedNodeIds = new Set(
        nodes.map((node) => node.data?.nodeId).filter((id) => Number.isInteger(id) && id > 0)
      );
      let nextNodeId = 1;
      function allocateNodeId() {
        while (usedNodeIds.has(nextNodeId)) nextNodeId += 1;
        const id = nextNodeId;
        usedNodeIds.add(id);
        nextNodeId += 1;
        return id;
      }
      const actionNodes = nodes.map((node) => {
        const nodeId = node.data?.nodeId || allocateNodeId();
        nodeIdMap[node.id] = nodeId;
        const config =
          node.type === 'trigger'
            ? normalizeNodeConfig('trigger', parseConfig(triggerConfig))
            : normalizeNodeConfig(node.type, node.data?.config || {});
        return {
          id: nodeId,
          action_id: baseAction?.id,
          node_type: node.type,
          node_config: JSON.stringify(config),
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
      saving = false;
      direction = 'horizontal';
      _action = null;
    },
  };

  return store;
}
