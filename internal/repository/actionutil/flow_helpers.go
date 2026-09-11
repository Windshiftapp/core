package actionutil

import "fmt"

// Descendants returns every node reachable from root through directed edges.
// Root is excluded unless an edge cycle leads back to it.
func Descendants[ID comparable, E any](root ID, edges []E, endpoints func(E) (ID, ID)) map[ID]bool {
	descendants := make(map[ID]bool)
	queue := make([]ID, 0)

	for _, edge := range edges {
		source, target := endpoints(edge)
		if source == root && !descendants[target] {
			descendants[target] = true
			queue = append(queue, target)
		}
	}

	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]
		for _, edge := range edges {
			source, target := endpoints(edge)
			if source == next && !descendants[target] {
				descendants[target] = true
				queue = append(queue, target)
			}
		}
	}

	return descendants
}

// ValidateFlowAcyclic returns an error if the directed graph formed by the
// given nodes and edges (using their client-side IDs) contains a cycle. It is
// intended to run before persisting a flow so users get immediate validation
// feedback instead of hitting a runtime chain-depth guard.
func ValidateFlowAcyclic[
	N any, NP interface {
		*N
		FlowNodeItem
	},
	E any, EP interface {
		*E
		FlowEdgeItem
	},
](nodes []N, edges []E) error {
	if len(nodes) == 0 {
		if len(edges) > 0 {
			return fmt.Errorf("flow has %d edges but no nodes", len(edges))
		}
		return nil
	}
	adj := make(map[int][]int, len(nodes))
	known := make(map[int]struct{}, len(nodes))
	for i := range nodes {
		id := NP(&nodes[i]).FlowNodeID()
		if _, exists := known[id]; exists {
			return fmt.Errorf("flow contains duplicate node ID %d", id)
		}
		known[id] = struct{}{}
	}
	if len(edges) == 0 {
		return nil
	}
	for i := range edges {
		ep := EP(&edges[i])
		src, dst := ep.FlowSourceNodeID(), ep.FlowTargetNodeID()
		if _, ok := known[src]; !ok {
			return fmt.Errorf("edge %d references unknown source node %d", i, src)
		}
		if _, ok := known[dst]; !ok {
			return fmt.Errorf("edge %d references unknown target node %d", i, dst)
		}
		adj[src] = append(adj[src], dst)
	}

	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make(map[int]int, len(nodes))

	var visit func(node int) (int, int, bool)
	visit = func(node int) (int, int, bool) {
		color[node] = gray
		for _, next := range adj[node] {
			switch color[next] {
			case gray:
				return node, next, true
			case white:
				if a, b, cyc := visit(next); cyc {
					return a, b, true
				}
			}
		}
		color[node] = black
		return 0, 0, false
	}

	for id := range known {
		if color[id] == white {
			if a, b, cyc := visit(id); cyc {
				return fmt.Errorf("flow contains a cycle at node %d → %d", a, b)
			}
		}
	}
	return nil
}

// --- Validation helpers ---

// ValidateActionFields checks that the common required fields for an action
// creation request are non-empty. Returns a human-readable error message
// or "" if valid.
func ValidateActionFields(name, triggerType string) string {
	if name == "" {
		return "Name is required"
	}
	if triggerType == "" {
		return "Trigger type is required"
	}
	return ""
}

// --- Generic node+edge creation with rollback ---

// FlowNodeSource is implemented by domain node types whose fields can be
// copied losslessly into the storage-level FlowNode.
type FlowNodeSource interface {
	FlowNodeItem
	FlowNodeData() (actionID int, nodeType, config string, x, y float64)
}

// FlowEdgeSource is implemented by domain edge types whose fields can be
// copied losslessly into the storage-level FlowEdge.
type FlowEdgeSource interface {
	FlowEdgeItem
	FlowEdgeID() int
	FlowEdgeData() (actionID int, edgeType, sourceHandle, targetHandle string)
}

// ToFlowNodes converts domain flow nodes to the storage-level representation.
func ToFlowNodes[
	N any, NP interface {
		*N
		FlowNodeSource
	},
](nodes []N) []FlowNode {
	out := make([]FlowNode, len(nodes))
	for i := range nodes {
		np := NP(&nodes[i])
		actionID, nodeType, config, x, y := np.FlowNodeData()
		out[i] = FlowNode{
			ID: np.FlowNodeID(), ActionID: actionID, NodeType: nodeType,
			NodeConfig: config, PositionX: x, PositionY: y,
		}
	}
	return out
}

// ToFlowEdges converts domain flow edges to the storage-level representation.
func ToFlowEdges[
	E any, EP interface {
		*E
		FlowEdgeSource
	},
](edges []E) []FlowEdge {
	out := make([]FlowEdge, len(edges))
	for i := range edges {
		ep := EP(&edges[i])
		actionID, edgeType, sourceHandle, targetHandle := ep.FlowEdgeData()
		out[i] = FlowEdge{
			ID: ep.FlowEdgeID(), ActionID: actionID,
			SourceNodeID: ep.FlowSourceNodeID(), TargetNodeID: ep.FlowTargetNodeID(),
			EdgeType: edgeType, SourceHandle: sourceHandle, TargetHandle: targetHandle,
		}
	}
	return out
}

// FlowNodeItem is the interface that pointer-to-node types must implement
// so they can be used with CreateFlowNodesAndEdges.
type FlowNodeItem interface {
	FlowNodeID() int
	SetFlowActionID(int)
}

// FlowEdgeItem is the interface that pointer-to-edge types must implement
// so they can be used with CreateFlowNodesAndEdges.
type FlowEdgeItem interface {
	SetFlowActionID(int)
	FlowSourceNodeID() int
	FlowTargetNodeID() int
	SetFlowSourceNodeID(int)
	SetFlowTargetNodeID(int)
}

// CreateFlowNodesAndEdges creates action-flow nodes and edges for a newly
// created action, remapping client-side node IDs to server-assigned IDs in
// the edges. On any error it calls rollback (typically deleting the action)
// and returns the error.
//
// Type parameters N and E are the value types (e.g. models.ActionNode).
// NP and EP are the corresponding pointer types that satisfy the
// FlowNodeItem / FlowEdgeItem interfaces.
//
// createNode receives a pointer to the node (with ActionID already set) and
// must return the new server-assigned ID.
// createEdge receives a pointer to the edge (with ActionID and remapped
// source/target IDs already set) and must return the new server-assigned ID.
func CreateFlowNodesAndEdges[
	N any, NP interface {
		*N
		FlowNodeItem
	},
	E any, EP interface {
		*E
		FlowEdgeItem
	},
](
	actionID int,
	nodes []N,
	edges []E,
	createNode func(NP) (int, error),
	createEdge func(EP) (int, error),
	rollback func(),
) error {
	if len(nodes) == 0 {
		if len(edges) > 0 {
			rollback()
			return fmt.Errorf("flow has %d edges but no nodes", len(edges))
		}
		return nil
	}

	known := make(map[int]struct{}, len(nodes))
	for i := range nodes {
		np := NP(&nodes[i])
		id := np.FlowNodeID()
		if _, exists := known[id]; exists {
			rollback()
			return fmt.Errorf("flow contains duplicate node ID %d", id)
		}
		known[id] = struct{}{}
	}
	for i := range edges {
		ep := EP(&edges[i])
		if _, ok := known[ep.FlowSourceNodeID()]; !ok {
			rollback()
			return fmt.Errorf("edge %d references unknown source node %d", i, ep.FlowSourceNodeID())
		}
		if _, ok := known[ep.FlowTargetNodeID()]; !ok {
			rollback()
			return fmt.Errorf("edge %d references unknown target node %d", i, ep.FlowTargetNodeID())
		}
	}

	nodeIDMap := make(map[int]int) // old client ID -> new server ID
	for i := range nodes {
		np := NP(&nodes[i])
		oldID := np.FlowNodeID()
		np.SetFlowActionID(actionID)
		newID, err := createNode(np)
		if err != nil {
			rollback()
			return fmt.Errorf("failed to create nodes: %w", err)
		}
		nodeIDMap[oldID] = newID
	}

	for i := range edges {
		ep := EP(&edges[i])
		ep.SetFlowActionID(actionID)
		if newSourceID, ok := nodeIDMap[ep.FlowSourceNodeID()]; ok {
			ep.SetFlowSourceNodeID(newSourceID)
		}
		if newTargetID, ok := nodeIDMap[ep.FlowTargetNodeID()]; ok {
			ep.SetFlowTargetNodeID(newTargetID)
		}
		if _, err := createEdge(ep); err != nil {
			rollback()
			return fmt.Errorf("failed to create edges: %w", err)
		}
	}

	return nil
}
