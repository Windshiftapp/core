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
