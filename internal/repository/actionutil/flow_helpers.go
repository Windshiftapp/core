package actionutil

import "fmt"

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
	if len(nodes) == 0 || len(edges) == 0 {
		return nil
	}

	adj := make(map[int][]int, len(nodes))
	known := make(map[int]struct{}, len(nodes))
	for i := range nodes {
		id := NP(&nodes[i]).FlowNodeID()
		known[id] = struct{}{}
	}
	for i := range edges {
		ep := EP(&edges[i])
		src, dst := ep.FlowSourceNodeID(), ep.FlowTargetNodeID()
		if _, ok := known[src]; !ok {
			continue
		}
		if _, ok := known[dst]; !ok {
			continue
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
		return nil
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
