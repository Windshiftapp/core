package services

import (
	"context"
	"fmt"
	"sort"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/validation"
)

// MaxBulkLabelItems bounds one bulk label request, matching the other bulk
// item endpoints.
const MaxBulkLabelItems = 500

// BulkLabelOperation describes one label change applied to a set of items.
// When Set is non-nil it replaces each item's full label set; otherwise Add
// and Remove apply a per-item delta.
type BulkLabelOperation struct {
	ItemIDs []int
	Add     []int
	Remove  []int
	Set     *[]int
}

// BulkLabelFailure records one selected item a bulk label change could not
// update.
type BulkLabelFailure struct {
	ItemID  int    `json:"item_id"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// BulkLabelResult reports the per-item outcome of a bulk label change.
type BulkLabelResult struct {
	RequestedCount int                `json:"requested_count"`
	UpdatedCount   int                `json:"updated_count"`
	UnchangedCount int                `json:"unchanged_count"`
	FailedCount    int                `json:"failed_count"`
	Failures       []BulkLabelFailure `json:"failures"`
}

// BulkApply applies one label operation to many items. Each item commits in
// its own transaction, so items that cannot be updated are reported in
// Failures while the rest succeed. Items whose label set does not change
// produce no write, history, or event.
func (s *LabelApplicationService) BulkApply(actor AuditActor, op BulkLabelOperation, authorize func(workspaceID int) (bool, error)) (BulkLabelResult, error) {
	ids := uniquePositiveSortedIDs(op.ItemIDs)
	if len(ids) == 0 {
		return BulkLabelResult{}, &validation.ValidationError{Field: "item_ids", Message: "at least one item id is required"}
	}
	if len(ids) > MaxBulkLabelItems {
		return BulkLabelResult{}, ErrBulkItemLimit
	}
	if op.Set != nil && (len(op.Add) > 0 || len(op.Remove) > 0) {
		return BulkLabelResult{}, &validation.ValidationError{Field: "set", Message: "set cannot be combined with add or remove"}
	}
	if op.Set == nil && len(op.Add) == 0 && len(op.Remove) == 0 {
		return BulkLabelResult{}, &validation.ValidationError{Field: "add", Message: "at least one label to add, remove, or set is required"}
	}

	catalog, err := s.labelCatalog()
	if err != nil {
		return BulkLabelResult{}, err
	}
	if err := requireCatalogLabels(catalog, op); err != nil {
		return BulkLabelResult{}, err
	}

	items, err := repository.NewItemRepository(s.db).FindByIDsWithDetails(ids)
	if err != nil {
		return BulkLabelResult{}, fmt.Errorf("load bulk label items: %w", err)
	}
	byID := make(map[int]*models.Item, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}

	result := BulkLabelResult{RequestedCount: len(ids), Failures: []BulkLabelFailure{}}
	authorized := map[int]bool{}
	for _, id := range ids {
		item := byID[id]
		if item == nil {
			result.Failures = append(result.Failures, BulkLabelFailure{ItemID: id, Code: "not_found", Message: "item not found"})
			continue
		}
		allowed, known := authorized[item.WorkspaceID]
		if !known {
			allowed, err = authorizeWorkspace(authorize, item.WorkspaceID)
			if err != nil {
				return BulkLabelResult{}, err
			}
			authorized[item.WorkspaceID] = allowed
		}
		if !allowed {
			result.Failures = append(result.Failures, BulkLabelFailure{ItemID: id, Code: "forbidden", Message: "item access forbidden"})
			continue
		}
		changed, err := s.applyItemLabels(actor, item, op, catalog)
		if err != nil {
			return BulkLabelResult{}, fmt.Errorf("update labels for item %d: %w", id, err)
		}
		if changed {
			result.UpdatedCount++
		} else {
			result.UnchangedCount++
		}
	}
	result.FailedCount = len(result.Failures)
	emitServiceAudit(s.db, actor, logger.ActionItemBulkLabels, logger.ResourceItem, nil, "", map[string]any{
		"requested": result.RequestedCount,
		"updated":   result.UpdatedCount,
		"unchanged": result.UnchangedCount,
		"failed":    result.FailedCount,
	})
	return result, nil
}

func (s *LabelApplicationService) labelCatalog() (map[int]models.Label, error) {
	labels, err := s.labels.ListAll()
	if err != nil {
		return nil, err
	}
	catalog := make(map[int]models.Label, len(labels))
	for _, label := range labels {
		catalog[label.ID] = label
	}
	return catalog, nil
}

func requireCatalogLabels(catalog map[int]models.Label, op BulkLabelOperation) error {
	check := func(labelIDs []int) error {
		for _, labelID := range labelIDs {
			if _, ok := catalog[labelID]; !ok {
				return repository.ErrNotFound
			}
		}
		return nil
	}
	if err := check(op.Add); err != nil {
		return err
	}
	if err := check(op.Remove); err != nil {
		return err
	}
	if op.Set != nil {
		return check(*op.Set)
	}
	return nil
}

func authorizeWorkspace(authorize func(int) (bool, error), workspaceID int) (bool, error) {
	if authorize == nil {
		return false, nil
	}
	return authorize(workspaceID)
}

// applyItemLabels swaps one item's label set inside its own transaction and
// reports whether anything changed.
func (s *LabelApplicationService) applyItemLabels(actor AuditActor, item *models.Item, op BulkLabelOperation, catalog map[int]models.Label) (bool, error) {
	changed := false
	err := database.WithTx(s.db, func(tx database.Tx) error {
		before, err := s.labels.ListForItemTx(tx, item.ID)
		if err != nil {
			return err
		}
		after := targetLabelSet(before, op, catalog)
		if !labelSetsDiffer(before, after) {
			return nil
		}
		afterIDs := make([]int, len(after))
		for i := range after {
			afterIDs[i] = after[i].ID
		}
		if err := s.labels.ReplaceItemLabelsTx(context.Background(), tx, item.ID, afterIDs); err != nil {
			return err
		}
		if err := recordLabelChangeFact(s.db, tx, item, before, after, actor); err != nil {
			return err
		}
		changed = true
		return nil
	})
	return changed, err
}

// targetLabelSet computes the resulting label set for one item, ordered by
// name to match the single-item read path.
func targetLabelSet(before []models.Label, op BulkLabelOperation, catalog map[int]models.Label) []models.Label {
	present := make(map[int]bool, len(before))
	for _, label := range before {
		present[label.ID] = true
	}
	if op.Set != nil {
		present = make(map[int]bool, len(*op.Set))
		for _, labelID := range *op.Set {
			present[labelID] = true
		}
	} else {
		for _, labelID := range op.Add {
			present[labelID] = true
		}
		for _, labelID := range op.Remove {
			delete(present, labelID)
		}
	}
	after := make([]models.Label, 0, len(present))
	for labelID := range present {
		if label, ok := catalog[labelID]; ok {
			after = append(after, label)
		}
	}
	sort.Slice(after, func(i, j int) bool { return after[i].Name < after[j].Name })
	return after
}
