// Package actionutil provides shared helpers for action flow execution.
package actionutil

import (
	"encoding/json"
	"fmt"
	"time"

	"windshift/internal/models"
)

// ActionNode is any type that has an int ID field.
type ActionNode interface {
	GetID() int
}

// ActionEdge is any type that exposes edge routing fields.
type ActionEdge interface {
	GetSourceNodeID() int
	GetTargetNodeID() int
	GetEdgeType() string
}

// TopologicalSort performs a topological sort on concrete node/edge slices,
// returning sorted nodes and an error if a cycle is detected.
func TopologicalSort[N ActionNode, E ActionEdge](nodes []N, edges []E) ([]N, error) {
	if len(nodes) == 0 {
		return nil, nil
	}

	nodeIDs := make([]int, len(nodes))
	nodeMap := make(map[int]N, len(nodes))
	for i, n := range nodes {
		nodeIDs[i] = n.GetID()
		nodeMap[n.GetID()] = n
	}

	flowEdges := make([]FlowEdge, len(edges))
	for i, e := range edges {
		flowEdges[i] = FlowEdge{
			SourceNodeID: e.GetSourceNodeID(),
			TargetNodeID: e.GetTargetNodeID(),
			EdgeType:     e.GetEdgeType(),
		}
	}

	sortedIDs, err := TopologicalSortByID(nodeIDs, flowEdges)
	if err != nil {
		return nil, err
	}

	sorted := make([]N, len(sortedIDs))
	for i, id := range sortedIDs {
		sorted[i] = nodeMap[id]
	}
	return sorted, nil
}

// CanExecuteNodeTyped converts typed edges to FlowEdge and delegates to CanExecuteNode.
func CanExecuteNodeTyped[E ActionEdge](nodeID int, edges []E, executedNodes map[int]bool, stepResults []models.StepResult) bool {
	flowEdges := make([]FlowEdge, len(edges))
	for i, e := range edges {
		flowEdges[i] = FlowEdge{
			SourceNodeID: e.GetSourceNodeID(),
			TargetNodeID: e.GetTargetNodeID(),
			EdgeType:     e.GetEdgeType(),
		}
	}
	return CanExecuteNode(nodeID, flowEdges, executedNodes, stepResults)
}

// TopologicalSortByID performs a topological sort on a set of node IDs connected
// by edges. It returns the IDs in execution order and an error if a cycle is
// detected.
func TopologicalSortByID(nodeIDs []int, edges []FlowEdge) ([]int, error) {
	if len(nodeIDs) == 0 {
		return nil, nil
	}

	inDegree := make(map[int]int)
	adjacency := make(map[int][]int)

	for _, id := range nodeIDs {
		inDegree[id] = 0
		adjacency[id] = []int{}
	}

	for _, edge := range edges {
		adjacency[edge.SourceNodeID] = append(adjacency[edge.SourceNodeID], edge.TargetNodeID)
		inDegree[edge.TargetNodeID]++
	}

	var queue []int
	for nodeID, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, nodeID)
		}
	}

	var sorted []int
	for len(queue) > 0 {
		nodeID := queue[0]
		queue = queue[1:]
		sorted = append(sorted, nodeID)

		for _, targetID := range adjacency[nodeID] {
			inDegree[targetID]--
			if inDegree[targetID] == 0 {
				queue = append(queue, targetID)
			}
		}
	}

	if len(sorted) != len(nodeIDs) {
		return nil, fmt.Errorf("cycle detected in action flow")
	}

	return sorted, nil
}

// CanExecuteNode determines whether a node can be executed based on the
// current set of already-executed nodes and edge conditions. It checks that
// all incoming edges have their source nodes executed and that conditional
// edge types ("true"/"false") match the condition results from step results.
//
// This deduplicates the identical logic in AssetActionService.canExecuteNode
// and LogbookActionService.canExecuteNode.
func CanExecuteNode(nodeID int, edges []FlowEdge, executedNodes map[int]bool, stepResults []models.StepResult) bool {
	hasIncomingEdge := false
	for _, edge := range edges {
		if edge.TargetNodeID == nodeID {
			hasIncomingEdge = true

			if !executedNodes[edge.SourceNodeID] {
				return false
			}

			if edge.EdgeType == "true" || edge.EdgeType == "false" {
				for _, result := range stepResults {
					if result.NodeID == edge.SourceNodeID {
						condResult, ok := result.Output["condition_result"].(bool)
						if !ok {
							return false
						}
						if edge.EdgeType == "true" && !condResult {
							return false
						}
						if edge.EdgeType == "false" && condResult {
							return false
						}
					}
				}
			}
		}
	}

	return hasIncomingEdge || len(edges) == 0
}

// FinalizeExecutionLog updates an execution log's completion status and trace
// from step results. It sets CompletedAt, determines the final status (failed
// if any step failed), and serializes the step results as JSON trace.
//
// This deduplicates the identical finalization pattern in both action services.
func FinalizeExecutionLog(stepResults []models.StepResult) (completedAt *time.Time, status models.ActionExecutionStatus, errorMessage, executionTrace string) {
	now := time.Now()
	completedAt = &now
	status = models.ActionStatusCompleted

	for _, result := range stepResults {
		if result.Status == models.ActionStatusFailed {
			status = models.ActionStatusFailed
			if errorMessage == "" {
				errorMessage = result.ErrorMessage
			}
			break
		}
	}

	if trace, err := json.Marshal(stepResults); err == nil {
		executionTrace = string(trace)
	}

	return completedAt, status, errorMessage, executionTrace
}
