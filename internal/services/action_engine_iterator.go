package services

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
)

// defaultIteratorMaxItems caps the per-iterator emission count when the node
// config doesn't override it. Prevents a misconfigured `descendants` iterator
// on a pathological tree from running thousands of transitions.
const defaultIteratorMaxItems = 1000

// maxStepsPerFlow caps the total node executions a single action invocation
// may perform, including iterator bodies summed across iterations. Per-iterator
// caps (defaultIteratorMaxItems) only protect a single level — a 1000-item
// outer iterator wrapping a 1000-item inner iterator multiplies to 1M body
// executions, which the cascade-depth guard does not catch (those are within
// one action, not across actions). Bail at this budget instead.
const maxStepsPerFlow = 10_000

// errStepBudgetExceeded is returned when an action invocation tries to execute
// more nodes than maxStepsPerFlow allows. The caller marks the action
// execution failed and records this error on the trace.
var errStepBudgetExceeded = fmt.Errorf("action step budget exceeded (%d): nested iterator likely fanned out beyond safe bounds", maxStepsPerFlow)

// iteratorBodyNodes computes the set of nodes that form an iterator's body —
// all nodes reachable from the iterator's outgoing edges. The engine runs
// this subgraph once per emitted item with ctx.Item swapped, then marks the
// body as executed in the outer loop.
//
// v1 constraint: a body node's incoming edges must all originate from
// iterator-body nodes (no joining back from outside). The runtime trusts the
// validator at action-creation time and doesn't re-check here.
func iteratorBodyNodes(iteratorNodeID int, edges []models.ActionEdge) map[int]bool {
	body := map[int]bool{}
	queue := []int{}

	for _, e := range edges {
		if e.SourceNodeID == iteratorNodeID && !body[e.TargetNodeID] {
			body[e.TargetNodeID] = true
			queue = append(queue, e.TargetNodeID)
		}
	}

	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]
		for _, e := range edges {
			if e.SourceNodeID == next && !body[e.TargetNodeID] {
				body[e.TargetNodeID] = true
				queue = append(queue, e.TargetNodeID)
			}
		}
	}

	return body
}

// runIterator handles a single iterator node: it executes the iterator's
// node executor to produce the items list, then runs the body subgraph once
// per item with ctx.Item swapped. Per-item step results are nested under
// stepResult.Iterations so the execution trace stays comprehensible.
func (as *ActionService) runIterator(
	node *models.ActionNode,
	ctx *models.ExecutionContext,
	stepResult *models.StepResult,
	allNodes []models.ActionNode,
	allEdges []models.ActionEdge,
	executedNodes map[int]bool,
) error {
	// Run the iterator-specific executor, which populates stepResult.Output.
	// Currently only related_items is an iterator; future iterators get added
	// to this switch.
	switch node.NodeType {
	case models.ActionNodeRelatedItems:
		if err := as.executeRelatedItems(node, ctx, stepResult); err != nil {
			return err
		}
	default:
		return fmt.Errorf("runIterator called for non-iterator node type: %s", node.NodeType)
	}

	itemsRaw, ok := stepResult.Output["items"]
	if !ok {
		return nil
	}
	items, ok := itemsRaw.([]*models.Item)
	if !ok {
		return fmt.Errorf("iterator %d: stepResult.Output[\"items\"] has wrong type %T", node.ID, itemsRaw)
	}

	body := iteratorBodyNodes(node.ID, allEdges)
	if len(body) == 0 {
		// Nothing downstream — emission is a no-op for this iterator. Strip
		// the items slice from the step output (it's noisy and not JSON-safe
		// for trace serialization).
		delete(stepResult.Output, "items")
		stepResult.Output["item_count"] = len(items)
		return nil
	}

	// Execute body once per item.
	previousItem := ctx.Item
	stepResult.Iterations = make([]models.IterationResult, 0, len(items))

	for _, item := range items {
		ctx.Item = item

		bodySteps, err := as.runBodyOnce(allNodes, allEdges, body, ctx)
		stepResult.Iterations = append(stepResult.Iterations, models.IterationResult{
			ItemID:      item.ID,
			WorkspaceID: item.WorkspaceID,
			Steps:       bodySteps,
		})

		if err != nil {
			ctx.Item = previousItem
			return err
		}
	}

	ctx.Item = previousItem

	// Mark every body node as executed so the outer loop doesn't re-run them.
	for nodeID := range body {
		executedNodes[nodeID] = true
	}

	// Replace the items slice with a JSON-friendly summary in the step output
	// (the slice itself isn't valuable in the trace and wouldn't serialize
	// cleanly).
	delete(stepResult.Output, "items")
	stepResult.Output["item_count"] = len(items)

	return nil
}

// runBodyOnce executes the iterator body subgraph in topological order with
// the current ctx.Item. Returns per-node step results so the iterator can
// nest them in its own step result.
func (as *ActionService) runBodyOnce(
	allNodes []models.ActionNode,
	allEdges []models.ActionEdge,
	body map[int]bool,
	ctx *models.ExecutionContext,
) ([]models.StepResult, error) {
	// Filter nodes/edges to the body subgraph.
	bodyNodes := make([]models.ActionNode, 0, len(body))
	for _, n := range allNodes {
		if body[n.ID] {
			bodyNodes = append(bodyNodes, n)
		}
	}
	bodyEdges := make([]models.ActionEdge, 0)
	for _, e := range allEdges {
		if body[e.SourceNodeID] && body[e.TargetNodeID] {
			bodyEdges = append(bodyEdges, e)
		}
	}

	sorted, err := as.topologicalSort(bodyNodes, bodyEdges)
	if err != nil {
		return nil, fmt.Errorf("iterator body subgraph: %w", err)
	}

	executed := map[int]bool{}
	var results []models.StepResult

	for _, n := range sorted {
		// Reuse the outer canExecuteNode logic so condition-edge gating still
		// works inside iterator bodies.
		if !as.canExecuteNode(n.ID, bodyEdges, executed, ctx) {
			continue
		}

		ctx.TotalSteps++
		if ctx.TotalSteps > maxStepsPerFlow {
			return results, errStepBudgetExceeded
		}

		nodeCopy := n
		step := models.StepResult{
			NodeID:    n.ID,
			NodeType:  n.NodeType,
			Status:    models.ActionStatusRunning,
			StartedAt: time.Now(),
		}

		// Iterators inside iterator bodies are valid — recurse.
		if n.NodeType.IsIterator() {
			err := as.runIterator(&nodeCopy, ctx, &step, allNodes, allEdges, executed)
			completedAt := time.Now()
			step.CompletedAt = &completedAt
			if err != nil {
				step.Status = models.ActionStatusFailed
				step.ErrorMessage = err.Error()
				slog.Warn("nested iterator failed",
					slog.String("component", "actions"),
					slog.Int("node_id", n.ID),
					slog.Any("error", err),
				)
			} else {
				step.Status = models.ActionStatusCompleted
				executed[n.ID] = true
			}
			results = append(results, step)
			continue
		}

		err := as.executeNode(&nodeCopy, ctx, &step)
		completedAt := time.Now()
		step.CompletedAt = &completedAt
		if err != nil {
			step.Status = models.ActionStatusFailed
			step.ErrorMessage = err.Error()
			slog.Warn("iterator body node failed",
				slog.String("component", "actions"),
				slog.Int("node_id", n.ID),
				slog.String("node_type", string(n.NodeType)),
				slog.Any("error", err),
			)
		} else {
			step.Status = models.ActionStatusCompleted
			executed[n.ID] = true
		}
		results = append(results, step)
	}

	return results, nil
}

// executeRelatedItems is the iterator's per-execution producer: it fetches
// the related items (descendants, children, ancestors) for the current
// ctx.Item (or trigger item if none) and stores them on
// stepResult.Output["items"] for runIterator to consume.
func (as *ActionService) executeRelatedItems(node *models.ActionNode, ctx *models.ExecutionContext, stepResult *models.StepResult) error {
	var config models.RelatedItemsNodeConfig
	if err := json.Unmarshal([]byte(node.NodeConfig), &config); err != nil {
		return fmt.Errorf("failed to parse related_items config: %w", err)
	}

	// Source item: ctx.Item if an outer iterator put one there, else the
	// trigger item.
	sourceItemID := 0
	if ctx.Item != nil {
		sourceItemID = ctx.Item.ID
	} else if ctx.Event != nil {
		sourceItemID = ctx.Event.ItemID
	}
	if sourceItemID == 0 {
		return fmt.Errorf("related_items: no source item in execution context")
	}

	// Source workspace ID for the cross-workspace filter.
	sourceWorkspaceID := 0
	if ctx.Item != nil {
		sourceWorkspaceID = ctx.Item.WorkspaceID
	} else if ctx.Event != nil {
		sourceWorkspaceID = ctx.Event.WorkspaceID
	}

	// Gate enumeration on the effective actor's read permission for the source
	// workspace. Without this, an action's actor could enumerate descendants
	// of a workspace they have no role in via the iterator's per-iteration
	// trace (which records ItemID + WorkspaceID). Per-item write nodes already
	// re-check edit permission, so this only closes the read-side disclosure.
	if as.permissionService != nil && ctx.EffectiveActorID > 0 && sourceWorkspaceID != 0 {
		ok, err := as.permissionService.HasWorkspacePermission(ctx.EffectiveActorID, sourceWorkspaceID, models.PermissionItemView)
		if err != nil {
			return fmt.Errorf("related_items: source-workspace permission check: %w", err)
		}
		if !ok {
			stepResult.Output = map[string]interface{}{
				"relation":   config.Relation,
				"item_count": 0,
				"items":      []*models.Item{},
				"skipped":    true,
				"reason":     "permission_denied",
			}
			return nil
		}
	}

	var items []*models.Item
	var err error
	switch config.Relation {
	case models.RelatedItemsDescendants, "":
		items, err = as.itemRepo.GetDescendants(sourceItemID)
	case models.RelatedItemsDirectChildren:
		items, err = as.itemRepo.GetChildren(sourceItemID)
	case models.RelatedItemsAncestors:
		items, err = as.itemRepo.GetAncestors(sourceItemID)
	case models.RelatedItemsLinked:
		linkRepo := repository.NewItemLinkRepository(as.db)
		items, err = linkRepo.FindLinkedItems(sourceItemID, config.LinkTypeID, config.LinkDirection)
	default:
		return fmt.Errorf("related_items: unsupported relation %q", config.Relation)
	}
	if err != nil {
		return fmt.Errorf("related_items: fetch %s: %w", config.Relation, err)
	}

	// CrossWorkspace=false (zero value) restricts to the source's workspace.
	// Templates that need cross-workspace recursion set this to true.
	if !config.CrossWorkspace && sourceWorkspaceID != 0 {
		filtered := make([]*models.Item, 0, len(items))
		for _, it := range items {
			if it.WorkspaceID == sourceWorkspaceID {
				filtered = append(filtered, it)
			}
		}
		items = filtered
	} else if config.CrossWorkspace && as.permissionService != nil && ctx.EffectiveActorID > 0 {
		// Cross-workspace enumeration: drop items in workspaces the actor cannot
		// read. Cache the per-workspace decision so a wide fan-out doesn't
		// re-query the permission store for every item.
		readable := map[int]bool{}
		filtered := make([]*models.Item, 0, len(items))
		for _, it := range items {
			ok, cached := readable[it.WorkspaceID]
			if !cached {
				allow, permErr := as.permissionService.HasWorkspacePermission(ctx.EffectiveActorID, it.WorkspaceID, models.PermissionItemView)
				if permErr != nil {
					return fmt.Errorf("related_items: cross-workspace permission check (ws %d): %w", it.WorkspaceID, permErr)
				}
				ok = allow
				readable[it.WorkspaceID] = ok
			}
			if ok {
				filtered = append(filtered, it)
			}
		}
		items = filtered
	}

	maxItems := config.MaxItems
	if maxItems <= 0 {
		maxItems = defaultIteratorMaxItems
	}
	truncated := false
	if len(items) > maxItems {
		items = items[:maxItems]
		truncated = true
	}

	stepResult.Output = map[string]interface{}{
		"relation":   config.Relation,
		"item_count": len(items),
		"items":      items, // consumed by runIterator, then deleted before trace serialization
	}
	if truncated {
		stepResult.Output["truncated_at"] = maxItems
	}
	return nil
}
