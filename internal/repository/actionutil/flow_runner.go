package actionutil

import (
	"context"
	"errors"
	"time"

	"windshift/internal/models"
)

// ErrCycleDetected is returned by RunFlow when the node/edge graph is not a
// DAG. The message matches the per-engine sort failures this runner replaces.
var ErrCycleDetected = errors.New("cycle detected in action flow")

// RunnerNode combines node identity with a plain-string node type so the
// runner can special-case trigger nodes without knowing domain node enums.
type RunnerNode interface {
	GetID() int
	GetNodeType() string
}

// FlowOptions tunes the shared DAG execution loop.
type FlowOptions struct {
	// TriggerNodeType is pre-marked as executed and skipped ("" = none).
	TriggerNodeType string
	// AllowRoots executes nodes with no incoming edges even when the flow has
	// edges. Iterator bodies need this: stripping the iterator->body edge
	// leaves body entry nodes without incoming edges.
	AllowRoots bool
	// MaxSteps caps recorded steps; 0 disables the cap. When the cap is hit,
	// the offending node is recorded as a failed step and the flow stops.
	MaxSteps int
	// BudgetExceededErr is the error message written into the failed step
	// when the step budget runs out. Unused when MaxSteps is 0.
	BudgetExceededErr error
	// TotalSteps is a shared counter across nested flows — an iterator body
	// shares the outer flow's budget. Nil means the runner keeps its own.
	TotalSteps *int
	// OnStep is invoked for every recorded step, in order. Engines use it to
	// mirror steps into their execution context while the flow runs, since
	// condition edges and iterators read earlier results mid-flow.
	OnStep func(step *models.StepResult)
}

// FlowDispatch executes one node and fills in its step output. markExecuted
// lets iterators record body nodes they already ran so the loop skips them.
type FlowDispatch[N RunnerNode] func(node *N, step *models.StepResult, markExecuted func(int)) error

// FlowRun reports the recorded steps and whether the step budget stopped the
// flow early. Step failures are recorded in Steps, not returned as errors —
// a failed step does not abort the flow.
type FlowRun struct {
	Steps          []models.StepResult
	BudgetExceeded bool
}

// RunFlow executes a DAG in topological order: trigger nodes are skipped,
// nodes with unsatisfied incoming edges are skipped, and execution continues
// after a failed step. These are the loop semantics of the three action
// engines this runner replaces.
//
// The returned error is non-nil only for a cyclic graph (errors.Is with
// ErrCycleDetected) or a canceled execCtx; a canceled context is detected
// after each step and the in-flight step is recorded as failed.
func RunFlow[N RunnerNode, E ActionEdge](execCtx context.Context, nodes []N, edges []E, opts FlowOptions, dispatch FlowDispatch[N]) (FlowRun, error) {
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
		return FlowRun{}, ErrCycleDetected
	}

	total := opts.TotalSteps
	if total == nil {
		total = new(int)
	}

	run := FlowRun{Steps: []models.StepResult{}}
	record := func(step models.StepResult) {
		run.Steps = append(run.Steps, step)
		if opts.OnStep != nil {
			opts.OnStep(&run.Steps[len(run.Steps)-1])
		}
	}

	executed := make(map[int]bool)
	for _, nodeID := range sortedIDs {
		node := nodeMap[nodeID]
		if node.GetNodeType() == opts.TriggerNodeType {
			executed[node.GetID()] = true
			continue
		}
		// Iterators mark their body as executed while running, so the outer
		// loop must not re-run those nodes when it reaches them.
		if executed[node.GetID()] {
			continue
		}
		allowRoots := opts.AllowRoots || len(flowEdges) == 0
		if !canExecuteWithRoots(node.GetID(), flowEdges, executed, run.Steps, allowRoots) {
			continue
		}

		*total++
		if opts.MaxSteps > 0 && *total > opts.MaxSteps {
			now := time.Now()
			message := "step budget exceeded"
			if opts.BudgetExceededErr != nil {
				message = opts.BudgetExceededErr.Error()
			}
			record(models.StepResult{
				NodeID:       node.GetID(),
				NodeType:     models.ActionNodeType(node.GetNodeType()),
				Status:       models.ActionStatusFailed,
				StartedAt:    now,
				CompletedAt:  &now,
				ErrorMessage: message,
			})
			run.BudgetExceeded = true
			return run, nil
		}

		step := models.StepResult{
			NodeID:    node.GetID(),
			NodeType:  models.ActionNodeType(node.GetNodeType()),
			Status:    models.ActionStatusRunning,
			StartedAt: time.Now(),
		}
		nodeCopy := node
		err := dispatch(&nodeCopy, &step, func(id int) { executed[id] = true })
		completedAt := time.Now()
		step.CompletedAt = &completedAt

		if ctxErr := execCtx.Err(); ctxErr != nil {
			step.Status = models.ActionStatusFailed
			step.ErrorMessage = ctxErr.Error()
			record(step)
			return run, ctxErr
		}
		if err != nil {
			step.Status = models.ActionStatusFailed
			step.ErrorMessage = err.Error()
		} else {
			step.Status = models.ActionStatusCompleted
			executed[node.GetID()] = true
		}
		record(step)
	}

	return run, nil
}

// canExecuteWithRoots determines whether a node can run given the executed
// set and recorded step results. Incoming edges must have executed sources,
// and "true"/"false" edges must match the source's recorded condition result.
// A missing condition result fails closed: an executed condition node always
// records a step, so absence means the result is unknowable.
func canExecuteWithRoots(nodeID int, flowEdges []FlowEdge, executed map[int]bool, steps []models.StepResult, allowRoots bool) bool {
	hasIncomingEdge := false
	for _, edge := range flowEdges {
		if edge.TargetNodeID != nodeID {
			continue
		}
		hasIncomingEdge = true

		if !executed[edge.SourceNodeID] {
			return false
		}

		if edge.EdgeType == "true" || edge.EdgeType == "false" {
			foundConditionResult := false
			for i := range steps {
				if steps[i].NodeID != edge.SourceNodeID {
					continue
				}
				foundConditionResult = true
				condResult, ok := steps[i].Output["condition_result"].(bool)
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
			if !foundConditionResult {
				return false
			}
		}
	}

	return hasIncomingEdge || allowRoots
}
