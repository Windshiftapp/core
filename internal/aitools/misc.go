package aitools

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"windshift/internal/auth"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/services"
)

// ----------------------------------------------------------------------------
// list_milestones
// ----------------------------------------------------------------------------

type createMilestoneArgs struct {
	WorkspaceID int    `json:"workspace_id" jsonschema:"Workspace to create the milestone in"`
	Name        string `json:"name" jsonschema:"Milestone name"`
	Description string `json:"description,omitempty" jsonschema:"Milestone description (TipTap JSON or plain text)"`
	TargetDate  string `json:"target_date,omitempty" jsonschema:"Target date in YYYY-MM-DD format"`
	Status      string `json:"status,omitempty" jsonschema:"Initial status: planning, in-progress, completed, or cancelled (default planning)"` //nolint:misspell // British spelling matches the persisted planning status
	CategoryID  *int   `json:"category_id,omitempty" jsonschema:"Milestone category ID"`
}

type listMilestonesArgs struct {
	WorkspaceID   int    `json:"workspace_id,omitempty" jsonschema:"Filter to a specific workspace"`
	Status        string `json:"status,omitempty" jsonschema:"Filter by status: planning, in-progress, completed, cancelled"` //nolint:misspell // British spelling matches the persisted planning status
	IncludeGlobal *bool  `json:"include_global,omitempty" jsonschema:"Include cross-workspace milestones (default true)"`
}

type milestoneDTO struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	Status        string `json:"status"`
	TargetDate    string `json:"target_date,omitempty"`
	CategoryName  string `json:"category_name,omitempty"`
	WorkspaceID   int    `json:"workspace_id,omitempty"`
	WorkspaceName string `json:"workspace_name,omitempty"`
}

type listMilestonesOut struct {
	Milestones []milestoneDTO `json:"milestones"`
}

func milestoneToDTO(m *services.MilestoneResult) milestoneDTO {
	result := milestoneDTO{
		ID:            m.ID,
		Name:          m.Name,
		Description:   m.Description,
		Status:        m.Status,
		TargetDate:    m.TargetDate,
		CategoryName:  m.CategoryName,
		WorkspaceName: m.WorkspaceName,
	}
	if m.WorkspaceID != nil {
		result.WorkspaceID = *m.WorkspaceID
	}
	return result
}

// ----------------------------------------------------------------------------
// list_iterations
// ----------------------------------------------------------------------------

type createIterationArgs struct {
	WorkspaceID int    `json:"workspace_id" jsonschema:"Workspace to create the iteration in"`
	Name        string `json:"name" jsonschema:"Iteration name"`
	Description string `json:"description,omitempty" jsonschema:"Iteration description (TipTap JSON or plain text)"`
	StartDate   string `json:"start_date" jsonschema:"Start date in YYYY-MM-DD format"`
	EndDate     string `json:"end_date" jsonschema:"End date in YYYY-MM-DD format"`
	Status      string `json:"status,omitempty" jsonschema:"Initial status: planned, active, completed, or cancelled (default planned)"` //nolint:misspell // British spelling matches the persisted planning status
	TypeID      *int   `json:"type_id,omitempty" jsonschema:"Iteration type ID"`
}

type listIterationsArgs struct {
	WorkspaceID   int    `json:"workspace_id,omitempty" jsonschema:"Filter to a specific workspace"`
	Status        string `json:"status,omitempty" jsonschema:"Filter by status: planned, active, completed, cancelled"` //nolint:misspell // British spelling matches the persisted planning status
	IncludeGlobal *bool  `json:"include_global,omitempty" jsonschema:"Include cross-workspace iterations (default true)"`
}

type iterationDTO struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	Status        string `json:"status"`
	StartDate     string `json:"start_date"`
	EndDate       string `json:"end_date"`
	TypeName      string `json:"type_name,omitempty"`
	WorkspaceID   int    `json:"workspace_id,omitempty"`
	WorkspaceName string `json:"workspace_name,omitempty"`
}

type listIterationsOut struct {
	Iterations []iterationDTO `json:"iterations"`
}

func iterationToDTO(iter *services.IterationResult) iterationDTO {
	result := iterationDTO{
		ID:            iter.ID,
		Name:          iter.Name,
		Description:   iter.Description,
		Status:        iter.Status,
		StartDate:     iter.StartDate,
		EndDate:       iter.EndDate,
		TypeName:      iter.TypeName,
		WorkspaceName: iter.WorkspaceName,
	}
	if iter.WorkspaceID != nil {
		result.WorkspaceID = *iter.WorkspaceID
	}
	return result
}

// ----------------------------------------------------------------------------
// list_custom_fields
// ----------------------------------------------------------------------------

type listCustomFieldsArgs struct{}

type customFieldDTO struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	FieldType   string `json:"field_type"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required"`
	Options     string `json:"options,omitempty"`
}

type listCustomFieldsOut struct {
	CustomFields []customFieldDTO `json:"custom_fields"`
}

// ----------------------------------------------------------------------------
// list_recent_activity
// ----------------------------------------------------------------------------

type listRecentActivityArgs struct {
	SinceDate   string `json:"since_date,omitempty" jsonschema:"Start date (YYYY-MM-DD), defaults to yesterday"`
	WorkspaceID int    `json:"workspace_id,omitempty" jsonschema:"Filter to a specific workspace"`
	Limit       int    `json:"limit,omitempty" jsonschema:"Max items (default 50, max 100)"`
}

type recentChangeDTO struct {
	FieldName string `json:"field"`
	OldValue  string `json:"old_value,omitempty"`
	NewValue  string `json:"new_value,omitempty"`
	ChangedAt string `json:"changed_at"`
	ItemKey   string `json:"item_key"`
	ItemTitle string `json:"item_title"`
	ChangedBy string `json:"changed_by"`
}

type recentCommentDTO struct {
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
	ItemKey   string `json:"item_key"`
	ItemTitle string `json:"item_title"`
	Author    string `json:"author"`
}

type listRecentActivityOut struct {
	Changes  []recentChangeDTO  `json:"changes"`
	Comments []recentCommentDTO `json:"comments"`
}

func init() {
	Register(Default, Tool[createMilestoneArgs]{
		Name:        "create_milestone",
		Group:       CapabilityPlanningActivity,
		Access:      AccessWrite,
		Risk:        RiskMedium,
		Description: "Create a milestone in an accessible workspace.",
		Scopes:      []string{auth.ScopeItemsWrite},
		Run: func(_ context.Context, env *Env, args createMilestoneArgs) (any, error) {
			if strings.TrimSpace(args.Name) == "" {
				return map[string]string{"error": "name is required"}, nil
			}
			if !env.HasWorkspaceAccess(args.WorkspaceID) {
				return map[string]string{"error": "workspace not found"}, nil
			}
			canEdit, err := env.PermService.HasWorkspacePermission(env.UserID, args.WorkspaceID, models.PermissionItemEdit)
			if err != nil {
				return nil, err
			}
			if !canEdit {
				return map[string]string{"error": "workspace not found"}, nil
			}
			if args.TargetDate != "" {
				if _, err := time.Parse("2006-01-02", args.TargetDate); err != nil {
					return map[string]string{"error": "invalid target_date format, use YYYY-MM-DD"}, nil //nolint:nilerr // surface as a tool error in JSON, not as a protocol error
				}
			}

			name := args.Name
			description := args.Description
			sanitize.ApplyAllWithWarnings(
				sanitize.Pair{Target: &name, Policy: sanitize.PlainTextField, Label: "Name"},
				sanitize.Pair{Target: &description, Policy: sanitize.RichText, Label: "Description"},
			)
			if strings.TrimSpace(name) == "" {
				return map[string]string{"error": "name is required"}, nil
			}

			var targetDate *string
			if args.TargetDate != "" {
				targetDate = &args.TargetDate
			}
			workspaceID := args.WorkspaceID
			milestone, err := services.NewPlanningService(env.DB).CreateMilestone(services.CreateMilestoneParams{
				Name:        name,
				Description: description,
				TargetDate:  targetDate,
				Status:      args.Status,
				CategoryID:  args.CategoryID,
				IsGlobal:    false,
				WorkspaceID: &workspaceID,
			})
			if err != nil {
				return map[string]string{"error": fmt.Sprintf("create failed: %s", err.Error())}, nil //nolint:nilerr // surface as a tool error in JSON, not as a protocol error
			}
			env.AuditWrite(logger.ResourceMilestone, milestone.ID, "create_milestone", milestone.Name)
			return milestoneToDTO(milestone), nil
		},
	})

	Register(Default, Tool[listMilestonesArgs]{
		Name:        "list_milestones",
		Group:       CapabilityPlanningActivity,
		Access:      AccessRead,
		Risk:        RiskLow,
		Description: "List milestones the user can see, with optional workspace, status and global-include filters.",
		Scopes:      []string{auth.ScopeMilestonesRead}, // cross-workspace list — matches v1 GET /milestones
		Run: func(_ context.Context, env *Env, args listMilestonesArgs) (any, error) {
			includeGlobal := true
			if args.IncludeGlobal != nil {
				includeGlobal = *args.IncludeGlobal
			}
			params := services.MilestoneListParams{Limit: 1<<31 - 1, Status: args.Status, IncludeGlobal: includeGlobal}
			if args.WorkspaceID > 0 {
				if !env.HasWorkspaceAccess(args.WorkspaceID) {
					return map[string]string{"error": "workspace not found"}, nil
				}
				params.WorkspaceID = &args.WorkspaceID
			} else {
				params.WorkspaceIDs = env.AccessibleWorkspaceIDs
				if len(params.WorkspaceIDs) == 0 {
					if !includeGlobal {
						return listMilestonesOut{Milestones: []milestoneDTO{}}, nil
					}
					params.WorkspaceIDs = []int{-1}
				}
			}
			milestones, _, err := services.NewPlanningService(env.DB).ListMilestones(params)
			if err != nil {
				return nil, err
			}
			out := listMilestonesOut{Milestones: []milestoneDTO{}}
			cutoff := time.Now().AddDate(-1, 0, 0)
			for i := range milestones {
				milestone := &milestones[i]
				if (milestone.Status == "completed" || milestone.Status == "cancelled") && milestone.UpdatedAt.Before(cutoff) { //nolint:misspell // Status values use British spelling.
					continue
				}
				out.Milestones = append(out.Milestones, milestoneToDTO(milestone))
			}
			sort.SliceStable(out.Milestones, func(i, j int) bool {
				left, right := out.Milestones[i], out.Milestones[j]
				if left.Status != right.Status {
					return left.Status < right.Status
				}
				if left.TargetDate != right.TargetDate {
					if left.TargetDate == "" {
						return false
					}
					if right.TargetDate == "" {
						return true
					}
					return left.TargetDate < right.TargetDate
				}
				return left.Name < right.Name
			})
			return out, nil
		},
	})

	Register(Default, Tool[createIterationArgs]{
		Name:        "create_iteration",
		Group:       CapabilityPlanningActivity,
		Access:      AccessWrite,
		Risk:        RiskMedium,
		Description: "Create an iteration in an accessible workspace.",
		Scopes:      []string{auth.ScopeItemsWrite},
		Run: func(_ context.Context, env *Env, args createIterationArgs) (any, error) {
			if strings.TrimSpace(args.Name) == "" {
				return map[string]string{"error": "name is required"}, nil
			}
			if strings.TrimSpace(args.StartDate) == "" {
				return map[string]string{"error": "start_date is required"}, nil
			}
			if strings.TrimSpace(args.EndDate) == "" {
				return map[string]string{"error": "end_date is required"}, nil
			}
			if !env.HasWorkspaceAccess(args.WorkspaceID) {
				return map[string]string{"error": "workspace not found"}, nil
			}
			canEdit, err := env.PermService.HasWorkspacePermission(env.UserID, args.WorkspaceID, models.PermissionItemEdit)
			if err != nil {
				return nil, err
			}
			if !canEdit {
				return map[string]string{"error": "workspace not found"}, nil
			}
			startDate, err := time.Parse("2006-01-02", args.StartDate)
			if err != nil {
				return map[string]string{"error": "invalid start_date format, use YYYY-MM-DD"}, nil //nolint:nilerr // surface as a tool error in JSON, not as a protocol error
			}
			endDate, err := time.Parse("2006-01-02", args.EndDate)
			if err != nil {
				return map[string]string{"error": "invalid end_date format, use YYYY-MM-DD"}, nil //nolint:nilerr // surface as a tool error in JSON, not as a protocol error
			}
			if endDate.Before(startDate) {
				return map[string]string{"error": "end_date must be on or after start_date"}, nil
			}

			name := args.Name
			description := args.Description
			sanitize.ApplyAllWithWarnings(
				sanitize.Pair{Target: &name, Policy: sanitize.PlainTextField, Label: "Name"},
				sanitize.Pair{Target: &description, Policy: sanitize.RichText, Label: "Description"},
			)
			if strings.TrimSpace(name) == "" {
				return map[string]string{"error": "name is required"}, nil
			}

			workspaceID := args.WorkspaceID
			iteration, err := services.NewPlanningService(env.DB).CreateIteration(services.CreateIterationParams{
				Name:        name,
				Description: description,
				StartDate:   args.StartDate,
				EndDate:     args.EndDate,
				Status:      args.Status,
				TypeID:      args.TypeID,
				IsGlobal:    false,
				WorkspaceID: &workspaceID,
			})
			if err != nil {
				return map[string]string{"error": fmt.Sprintf("create failed: %s", err.Error())}, nil //nolint:nilerr // surface as a tool error in JSON, not as a protocol error
			}
			env.AuditWrite(logger.ResourceIteration, iteration.ID, "create_iteration", iteration.Name)
			return iterationToDTO(iteration), nil
		},
	})

	Register(Default, Tool[listIterationsArgs]{
		Name:        "list_iterations",
		Group:       CapabilityPlanningActivity,
		Access:      AccessRead,
		Risk:        RiskLow,
		Description: "List iterations (sprints, PIs, releases) the user can see.",
		Scopes:      []string{auth.ScopeIterationsRead}, // cross-workspace list — matches v1 GET /iterations
		Run: func(_ context.Context, env *Env, args listIterationsArgs) (any, error) {
			includeGlobal := true
			if args.IncludeGlobal != nil {
				includeGlobal = *args.IncludeGlobal
			}
			params := services.IterationListParams{Limit: 1<<31 - 1, Status: args.Status, IncludeGlobal: includeGlobal}
			if args.WorkspaceID > 0 {
				if !env.HasWorkspaceAccess(args.WorkspaceID) {
					return map[string]string{"error": "workspace not found"}, nil
				}
				params.WorkspaceID = &args.WorkspaceID
			} else {
				params.WorkspaceIDs = env.AccessibleWorkspaceIDs
				if len(params.WorkspaceIDs) == 0 {
					if !includeGlobal {
						return listIterationsOut{Iterations: []iterationDTO{}}, nil
					}
					params.WorkspaceIDs = []int{-1}
				}
			}
			iterations, _, err := services.NewPlanningService(env.DB).ListIterations(params)
			if err != nil {
				return nil, err
			}
			out := listIterationsOut{Iterations: []iterationDTO{}}
			cutoff := time.Now().AddDate(-1, 0, 0).Format(time.DateOnly)
			for i := range iterations {
				iteration := &iterations[i]
				if (iteration.Status == "completed" || iteration.Status == "cancelled") && iteration.EndDate != "" && iteration.EndDate < cutoff { //nolint:misspell // Status values use British spelling.
					continue
				}
				out.Iterations = append(out.Iterations, iterationToDTO(iteration))
			}
			sort.SliceStable(out.Iterations, func(i, j int) bool {
				left, right := out.Iterations[i], out.Iterations[j]
				if left.Status != right.Status {
					return left.Status < right.Status
				}
				if left.StartDate != right.StartDate {
					return left.StartDate < right.StartDate
				}
				return left.Name < right.Name
			})
			return out, nil
		},
	})

	Register(Default, Tool[listCustomFieldsArgs]{
		Name:        "list_custom_fields",
		Group:       CapabilityPlanningActivity,
		Access:      AccessRead,
		Risk:        RiskLow,
		Description: "List available custom field definitions. Use this to discover what custom fields exist before filtering items with cf_<name> or the bare field name in the filter parameter of list_items.",
		Scopes:      []string{auth.ScopeCustomFieldsRead},
		Run: func(_ context.Context, env *Env, _ listCustomFieldsArgs) (any, error) {
			fields, err := repository.NewCustomFieldRepository(env.DB).List()
			if err != nil {
				return nil, err
			}
			out := listCustomFieldsOut{CustomFields: []customFieldDTO{}}
			for _, field := range fields {
				out.CustomFields = append(out.CustomFields, customFieldDTO{
					ID: field.ID, Name: field.Name, FieldType: field.FieldType,
					Description: field.Description, Required: field.Required, Options: field.Options,
				})
			}
			return out, nil
		},
	})

	Register(Default, Tool[listRecentActivityArgs]{
		Name:        "list_recent_activity",
		Group:       CapabilityPlanningActivity,
		Access:      AccessRead,
		Risk:        RiskLow,
		Description: "List recent changes and comments across accessible workspaces. Useful for understanding what happened recently.",
		Scopes:      []string{auth.ScopeItemsRead}, // activity is item history — matches v1 GET /items/{id}/history
		Run: func(_ context.Context, env *Env, args listRecentActivityArgs) (any, error) {
			sinceDate := args.SinceDate
			if sinceDate == "" {
				sinceDate = time.Now().AddDate(0, 0, -1).Format("2006-01-02")
			}
			since, err := time.Parse("2006-01-02", sinceDate)
			if err != nil {
				return map[string]string{"error": "invalid since_date format, use YYYY-MM-DD"}, nil //nolint:nilerr // surface as a tool error in JSON, not as a protocol error
			}
			limit := args.Limit
			if limit <= 0 {
				limit = 50
			}
			if limit > 100 {
				limit = 100
			}
			var wsIDs []int
			if args.WorkspaceID > 0 {
				if !env.HasWorkspaceAccess(args.WorkspaceID) {
					return map[string]string{"error": "workspace not found"}, nil
				}
				wsIDs = []int{args.WorkspaceID}
			} else {
				wsIDs = env.AccessibleWorkspaceIDs
			}
			out := listRecentActivityOut{Changes: []recentChangeDTO{}, Comments: []recentCommentDTO{}}
			if len(wsIDs) == 0 {
				return out, nil
			}
			itemRepo := repository.NewItemRepository(env.DB)
			changes, err := itemRepo.RecentItemChanges(wsIDs, since, limit)
			if err != nil {
				return nil, err
			}
			for _, c := range changes {
				out.Changes = append(out.Changes, recentChangeDTO{
					FieldName: c.FieldName,
					OldValue:  c.OldValue,
					NewValue:  c.NewValue,
					ChangedAt: c.ChangedAt.Format(time.RFC3339),
					ItemKey:   c.ItemKey,
					ItemTitle: c.Title,
					ChangedBy: c.ChangedBy,
				})
			}

			comments, err := itemRepo.RecentComments(wsIDs, since, 30)
			if err != nil {
				return nil, err
			}
			for _, c := range comments {
				cm := recentCommentDTO{
					Content:   c.Content,
					CreatedAt: c.CreatedAt.Format(time.RFC3339),
					ItemKey:   c.ItemKey,
					ItemTitle: c.Title,
					Author:    c.Author,
				}
				if len(cm.Content) > 200 {
					cm.Content = cm.Content[:200] + "..."
				}
				out.Comments = append(out.Comments, cm)
			}
			return out, nil
		},
	})
}
