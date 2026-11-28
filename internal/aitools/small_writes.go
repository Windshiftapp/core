package aitools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"windshift/internal/auth"
	"windshift/internal/models"
	"windshift/internal/services"
)

type editMyWorklogArgs struct {
	worklogRef
	ExpectedDescription *string `json:"expected_description,omitempty"`
	ExpectedDuration    *int    `json:"expected_duration_minutes,omitempty"`
	ExpectedItemKey     *string `json:"expected_item_key,omitempty" jsonschema:"Current task key; use an empty string for unlinked"`
	Description         *string `json:"description,omitempty"`
	Duration            *int    `json:"duration_minutes,omitempty"`
	ItemKey             *string `json:"item_key,omitempty" jsonschema:"Replacement task key; use an empty string to unlink"`
}

type bookDraftArgs struct {
	DraftID         int     `json:"draft_id"`
	Version         int     `json:"version"`
	IdempotencyKey  string  `json:"idempotency_key"`
	ProjectID       int     `json:"project_id,omitempty"`
	ItemKey         *string `json:"item_key,omitempty" jsonschema:"Leave omitted to keep the draft's task, or pass an empty string to unlink"`
	Description     *string `json:"description,omitempty"`
	Date            string  `json:"date,omitempty" jsonschema:"Local date YYYY-MM-DD"`
	StartTime       string  `json:"start_time,omitempty"`
	EndTime         string  `json:"end_time,omitempty"`
	DurationMinutes int     `json:"duration_minutes,omitempty"`
}

type watchArgs struct {
	itemRef
	Watching bool `json:"watching"`
}

func registerWrite[A any](name, description string, group CapabilityGroup, scope string, run func(context.Context, *Env, A) (any, error)) {
	Register(Default, Tool[A]{Name: name, Description: description, Group: group, Access: AccessWrite, Risk: RiskMedium,
		Scopes: []string{scope}, Sources: []string{SourceAIChat, SourceMCP}, Run: run})
}

func itemKeyFromWorklog(log *models.Worklog) string {
	if log.ItemID == nil || log.WorkspaceKey == "" || log.WorkspaceItemNumber <= 0 {
		return ""
	}
	return fmt.Sprintf("%s-%d", log.WorkspaceKey, log.WorkspaceItemNumber)
}

func editMyWorklog(ctx context.Context, env *Env, args editMyWorklogArgs) (any, error) {
	current, err := args.read(env)
	if err != nil {
		return nil, err
	}
	if current.UserID == nil || *current.UserID != env.UserID {
		return nil, errors.New("worklog not found")
	}
	if args.Description == nil && args.Duration == nil && args.ItemKey == nil {
		return nil, errors.New("specify a worklog field to correct")
	}
	bookable, err := env.timePermission().CanBookTimeOnProject(env.UserID, current.ProjectID)
	if err != nil {
		return nil, err
	}
	if !bookable {
		return nil, errors.New("there is no permission to book in this time project")
	}
	visible := worklogView(env, *current)
	input := services.WorklogMutationInput{Expected: current, ProjectID: current.ProjectID, ItemID: current.ItemID,
		Description: current.Description, Date: time.Unix(current.Date, 0).UTC().Format(time.DateOnly), UserTimezone: env.Timezone,
		DurationMinutes: current.DurationMins, StoredStartUnix: current.StartTime, StoredEndUnix: current.EndTime, PreserveStored: true}
	if args.Description != nil {
		if args.ExpectedDescription == nil || *args.ExpectedDescription != current.Description {
			return nil, errors.New("description changed or expected_description is missing")
		}
		input.Description = *args.Description
	}
	if args.Duration != nil {
		if args.ExpectedDuration == nil || *args.ExpectedDuration != current.DurationMins {
			return nil, errors.New("duration changed or expected_duration_minutes is missing")
		}
		if *args.Duration <= 0 || *args.Duration > 24*60 {
			return nil, errors.New("duration_minutes must be between 1 and 1440")
		}
		input.DurationMinutes = *args.Duration
		input.StoredEndUnix = input.StoredStartUnix + int64(*args.Duration)*60
	}
	if args.ItemKey != nil {
		if current.ItemID != nil && visible.ItemID == nil {
			return nil, errors.New("the current linked task is unavailable")
		}
		if args.ExpectedItemKey == nil || !strings.EqualFold(*args.ExpectedItemKey, itemKeyFromWorklog(&visible)) {
			return nil, errors.New("linked item changed or expected_item_key is missing")
		}
		input.ItemID = nil
		if *args.ItemKey != "" {
			target, err := (itemRef{ItemKey: *args.ItemKey}).read(ctx, env)
			if err != nil {
				return nil, err
			}
			input.ItemID = &target.ID
		}
	}
	result, err := env.timeWorklogService().Update(env.UserID, current.ID, input)
	if err != nil {
		return nil, err
	}
	env.AuditWrite(resourceTimeWorklog, current.ID, "edit_my_worklog", "corrected worklog")
	return map[string]any{"worklog": worklogView(env, *result.Worklog), "warnings": result.Warnings, "url": "/time/worklogs"}, nil
}

func bookTimeDraft(ctx context.Context, env *Env, args bookDraftArgs) (any, error) {
	if env.TimerService == nil {
		return nil, errors.New("timer service is unavailable")
	}
	if args.DraftID <= 0 || args.Version <= 0 || args.IdempotencyKey == "" || len(args.IdempotencyKey) > 200 {
		return nil, errors.New("draft_id, version, and a stable idempotency_key of at most 200 bytes are required")
	}
	if args.DurationMinutes != 0 {
		if _, err := services.ValidateWorklogDurationMinutes(args.DurationMinutes); err != nil {
			return nil, err
		}
	}
	input := services.WorklogMutationInput{
		ProjectID: args.ProjectID, Date: args.Date, UserTimezone: env.Timezone,
		StartTime: args.StartTime, EndTime: args.EndTime, DurationMinutes: args.DurationMinutes,
	}
	draft, err := env.TimerService.GetDraft(env.UserID, args.DraftID)
	if err != nil && !errors.Is(err, services.ErrTimerDraftNotFound) {
		return nil, err
	}
	// A missing draft may be a successful booking whose response was lost.
	// BookDraft checks the caller's retry key before requiring an existing draft.
	if draft != nil {
		if input.ProjectID == 0 && draft.ProjectID != nil {
			input.ProjectID = *draft.ProjectID
		}
		if args.Description == nil {
			input.Description = draft.Description
		} else {
			input.Description = *args.Description
		}
		if input.Date == "" && draft.Date != nil {
			input.Date = time.Unix(*draft.Date, 0).UTC().Format(time.DateOnly)
		}
		if draft.StartTime != nil {
			input.StoredStartUnix = *draft.StartTime
		}
		if draft.EndTime != nil {
			input.StoredEndUnix = *draft.EndTime
		}
		switch {
		case args.Date == "" && args.StartTime == "" && args.EndTime == "" && input.StoredStartUnix > 0 && input.StoredEndUnix > 0:
			input.PreserveStored = true
			if input.DurationMinutes > 0 {
				input.StoredEndUnix = input.StoredStartUnix + int64(input.DurationMinutes)*60
			} else if draft.DurationMins != nil {
				input.DurationMinutes = *draft.DurationMins
			}
		case input.DurationMinutes > 0 && args.EndTime == "":
			input.StoredEndUnix = 0
		case args.EndTime == "" && input.StoredEndUnix == 0 && draft.DurationMins != nil:
			input.DurationMinutes = *draft.DurationMins
		}
		ref := itemRef{}
		if args.ItemKey != nil {
			ref.ItemKey = *args.ItemKey
		} else if draft.ItemID != nil {
			ref.ItemID = *draft.ItemID
		}
		if ref.ItemKey != "" || ref.ItemID > 0 {
			item, err := ref.read(ctx, env)
			if err != nil {
				return nil, err
			}
			input.ItemID = &item.ID
		}
	}
	log, err := env.TimerService.BookDraft(env.UserID, args.DraftID, args.Version, args.IdempotencyKey, input)
	if err != nil {
		return nil, err
	}
	env.AuditWrite(resourceTimeWorklog, log.ID, "book_time_draft", "booked time draft")
	return map[string]any{"worklog": worklogView(env, *log), "url": worklogURL(log.ID)}, nil
}

func init() {
	registerWrite("edit_my_worklog", "Correct one of the caller's booked time entries. Read get_worklog first and send expected current values for the changed fields.", CapabilityTime, auth.ScopeTimeWrite, editMyWorklog)
	registerWrite("book_time_draft", "Book the caller's existing time draft using the version from list_time_drafts and an idempotency key. Supply missing details and requested changes. Omitted details keep their saved values.", CapabilityTime, auth.ScopeTimeWrite, bookTimeDraft)
	registerWrite("set_my_item_watch", "Set whether the caller follows this one work item.", CapabilityUsersApprovals, auth.ScopeItemsWrite,
		func(ctx context.Context, env *Env, args watchArgs) (any, error) {
			item, err := args.read(ctx, env)
			if err != nil {
				return nil, err
			}
			var result services.ItemWatchStatus
			if args.Watching {
				result, err = env.itemApplication().Watch(ctx, env.UserID, item.ID, "manual")
			} else {
				result, err = env.itemApplication().Unwatch(ctx, env.UserID, item.ID)
			}
			if err != nil {
				return nil, err
			}
			env.AuditWrite("item", item.ID, "set_my_item_watch", item.Title)
			return map[string]any{"status": result, "url": itemURL(item)}, nil
		})
}
