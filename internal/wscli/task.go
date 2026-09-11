package wscli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "Manage tasks and items",
	Long:  `Commands for viewing, creating, and managing work items.`,
}

var (
	taskSearchLimit int
	taskSearchQL    bool
)

var taskSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search work items by text or CQL filter",
	Long: `Search items the caller can view via the v2 API.
Multiple arguments are joined into a single query string. This is the same
search as the top-level "ws search", scoped under "task" for discoverability;
when a workspace is configured the results are filtered to it client-side.

The query may be free text or a structured CQL filter. A query that parses as
a CQL filter (e.g. "milestone = '0.8.2'") is evaluated as such; otherwise it is
matched as free text. Pass --ql to force CQL evaluation.

Examples:
  ws task search "login bug"
  ws task search login bug --limit 5
  ws task search "rate limit" -w PROJ
  ws task search "milestone = '0.8.2' AND status != Done"
  ws task search --ql "assignee = currentUser() AND status != Done"`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		return runItemSearch(strings.Join(args, " "), taskSearchLimit, taskSearchQL)
	},
}

var taskMineCmd = &cobra.Command{
	Use:   "mine",
	Short: "List tasks assigned to me",
	Long: `List tasks assigned to the current user.

Examples:
  ws task mine                            # All my tasks
  ws task mine -s ~done                   # My tasks excluding done
  ws task mine --created today            # My tasks created today
  ws task mine --updated -7d              # My tasks updated in last 7 days`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}

		// Get current user
		user, err := client.GetCurrentUser()
		if err != nil {
			return fmt.Errorf("failed to get current user: %w", err)
		}

		filters, err := newFiltersWithWorkspace(client, map[string]string{
			"assignee_id": fmt.Sprintf("%d", user.ID),
		})
		if err != nil {
			return err
		}

		if err := applyStatusFilter(filters, statusFilter, client); err != nil {
			return err
		}

		// Add date filters
		if err := applyDateFilters(filters, createdFilter, updatedFilter); err != nil {
			return err
		}

		items, err := client.ListItems(filters)
		if err != nil {
			return fmt.Errorf("failed to list items: %w", err)
		}

		output := NewOutput()
		output.Print(items)
		return nil
	},
}

var taskCreatedCmd = &cobra.Command{
	Use:   "created",
	Short: "List tasks created by me",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}

		// Get current user
		user, err := client.GetCurrentUser()
		if err != nil {
			return fmt.Errorf("failed to get current user: %w", err)
		}

		filters, err := newFiltersWithWorkspace(client, map[string]string{
			"creator_id": fmt.Sprintf("%d", user.ID),
		})
		if err != nil {
			return err
		}

		items, err := client.ListItems(filters)
		if err != nil {
			return fmt.Errorf("failed to list items: %w", err)
		}

		output := NewOutput()
		output.Print(items)
		return nil
	},
}

var taskListCmd = &cobra.Command{
	Use:   "ls",
	Short: "List and filter tasks",
	Long: `List tasks with optional filtering.

Examples:
  ws task ls                             # First page of accessible tasks
  ws task ls --all                        # Load all matching pages
  ws task ls --page 2 --limit 25           # Select a page
  ws task ls -w PROJ -s "In Progress"      # Workspace status name
  ws task ls -w PROJ --milestone "Q4"      # Exact milestone name or ID
  ws task ls -s 1                         # Filter by status ID
  ws task ls -s ~done                     # Exclude done status (negation)
  ws task ls --assignee 5                 # Filter by assignee ID
  ws task ls -w PROJ                      # Filter by workspace
  ws task ls --created today              # Tasks created today
  ws task ls --updated -7d                # Tasks updated in last 7 days`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}

		filters, err := newFiltersWithWorkspace(client, nil)
		if err != nil {
			return err
		}

		if err := applyStatusFilter(filters, statusFilter, client); err != nil {
			return err
		}
		if taskListMilestone != "" {
			workspaceID, err := resolveOptionalWorkspace(client)
			if err != nil {
				return err
			}
			id, err := resolveTaskMilestone(client, taskListMilestone, workspaceID)
			if err != nil {
				return err
			}
			if id <= 0 {
				return fmt.Errorf("milestone ID must be positive")
			}
			filters["milestone_id"] = strconv.Itoa(id)
		}

		if assigneeFilter != "" {
			filters["assignee_id"] = assigneeFilter
		}
		if itemTypeFilter != "" {
			filters["item_type_id"] = itemTypeFilter
		}
		if priorityFilter != "" {
			filters["priority_id"] = priorityFilter
		}

		// Add date filters
		if err := applyDateFilters(filters, createdFilter, updatedFilter); err != nil {
			return err
		}

		items, err := listTaskPage(client, filters, taskListPage, taskListLimit, taskListAll)
		if err != nil {
			return fmt.Errorf("failed to list items: %w", err)
		}

		output := NewOutput()
		output.Print(items)
		return nil
	},
}

var taskGetCmd = &cobra.Command{
	Use:   "get <id|KEY-123>",
	Short: "Get task details",
	Long: `Get detailed information about a task, including available status transitions
and the latest 10 comments (newest first).

Examples:
  ws task get 123                         # Get by ID
  ws task get PROJ-45                     # Get by workspace key and item number
  ws task get PROJ-45 --web               # Open in browser`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}

		itemID, err := client.ResolveItemID(args[0])
		if err != nil {
			return fmt.Errorf("failed to resolve item: %w", err)
		}

		// Get item with transitions, comments, and attachments expanded. Server
		// returns comments newest-first; we keep at most the latest 10 so the
		// output stays scannable on busy items. Attachments drive the
		// image-attachment hint that points a coding agent at view_image.
		item, err := client.GetItem(itemID, "transitions,comments,attachments")
		if err != nil {
			return fmt.Errorf("failed to get item: %w", err)
		}
		if len(item.Comments) > taskGetCommentLimit {
			item.Comments = item.Comments[:taskGetCommentLimit]
		}

		// Open in browser if requested
		if openInBrowser {
			wsKey := item.WorkspaceKey
			if wsKey == "" {
				wsKey = cfg.GetEffectiveWorkspace()
			}
			url := buildItemURL(wsKey, item.WorkspaceItemNumber)
			if err := openBrowser(url); err != nil {
				return fmt.Errorf("failed to open browser: %w", err)
			}
			_, _ = fmt.Fprintf(stdout, "Opened %s in browser\n", url)
			return nil
		}

		// Surface the item's children so both directions of the hierarchy are
		// visible from a single `get` (the parent key/title come straight from
		// the server). A children-fetch failure is non-fatal — still print the
		// item we already have.
		if children, cerr := client.GetItemChildren(itemID); cerr == nil {
			item.Children = children
		}

		output := NewOutput()
		output.Print(item)
		return nil
	},
}

var taskCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new task",
	Long: `Create a new task/item.

Examples:
  ws task create -t "Fix login bug"
  ws task create -t "Add feature" -d "Detailed description"
  ws task create -t "Bug" --type Bug --priority 2
  ws task create -t "Ship it" --due-date 2026-07-20
  ws task create -t "Spike" --custom-field "Risk=High" --custom-field 7=42
  ws task create -t "Sprint work" --iteration "Sprint 12" --project 3
  ws task create -t "New feature" --web    # Create and open in browser`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if createTitle == "" {
			return fmt.Errorf("title is required: use -t or --title")
		}

		client, err := NewClient()
		if err != nil {
			return err
		}

		// Resolve workspace
		wsKey := cfg.GetEffectiveWorkspace()
		if wsKey == "" {
			return fmt.Errorf("workspace is required: use -w flag or set defaults.workspace_key in config")
		}

		wsID, err := client.ResolveWorkspaceID(wsKey)
		if err != nil {
			return fmt.Errorf("failed to resolve workspace: %w", err)
		}

		req := ItemCreateRequest{
			WorkspaceID: wsID,
			Title:       createTitle,
			Description: ParseCLIEscapes(createDescription),
		}

		// Set optional fields
		if createType != "" {
			typeID, err := resolveItemTypeID(client, createType, &wsID)
			if err != nil {
				return err
			}
			req.ItemTypeID = &typeID
		}
		// --template seeds the description with a template's scaffold. It is a
		// convenience for the get/fill flow; it is mutually exclusive with -d so
		// the seed is never silently dropped.
		if createTemplate != "" {
			if req.Description != "" {
				return fmt.Errorf("--template and -d/--description are mutually exclusive (use one)")
			}
			tmpl, err := resolveItemTemplate(client, wsID, createTemplate)
			if err != nil {
				return err
			}
			req.Description = tmpl.DescriptionBody
		}
		if createPriorityID > 0 {
			req.PriorityID = &createPriorityID
		}
		if createStatusID > 0 {
			req.StatusID = &createStatusID
		}
		if createAssigneeID > 0 {
			req.AssigneeID = &createAssigneeID
		}
		if cmd.Flags().Changed("parent") {
			parentID, err := resolveParentID(client, createParent)
			if err != nil {
				return err
			}
			if parentID > 0 {
				req.ParentID = &parentID
			}
		}
		if createDueDate != "" {
			d, err := parseDateFlag("due-date", createDueDate)
			if err != nil {
				return err
			}
			req.DueDate = d
		}
		if createStartDate != "" {
			d, err := parseDateFlag("start-date", createStartDate)
			if err != nil {
				return err
			}
			req.StartDate = d
		}
		if createEndDate != "" {
			d, err := parseDateFlag("end-date", createEndDate)
			if err != nil {
				return err
			}
			req.EndDate = d
		}
		if len(createCustomFields) > 0 {
			cf, err := parseCustomFieldFlags(client, createCustomFields)
			if err != nil {
				return err
			}
			req.CustomFields = cf
		}
		if createIteration != "" {
			id, err := client.ResolveIterationID(createIteration, &wsID)
			if err != nil {
				return fmt.Errorf("failed to resolve iteration: %w", err)
			}
			req.IterationID = &id
		}
		if createProject != "" {
			id, err := parseProjectFlag(createProject)
			if err != nil {
				return err
			}
			req.ProjectID = &id
		}

		item, err := client.CreateItem(req)
		if err != nil {
			return fmt.Errorf("failed to create item: %w", err)
		}

		// Open in browser if requested
		if openInBrowser {
			url := buildItemURL(wsKey, item.WorkspaceItemNumber)
			if err := openBrowser(url); err != nil {
				return fmt.Errorf("failed to open browser: %w", err)
			}
			_, _ = fmt.Fprintf(stdout, "Created %s-%d and opened in browser\n", wsKey, item.WorkspaceItemNumber)
		}

		// Echo any mandatory template the item's type enforces (WI-438) so the
		// caller learns the expected structure even when it passed its own
		// description. Printed to stderr to keep stdout clean for -o json/table.
		if item.EnforcedTemplate != nil {
			if item.EnforcedTemplate.Applied {
				_, _ = fmt.Fprintf(stderr, "Type enforces template %q; applied to the (empty) description.\n", item.EnforcedTemplate.Name)
			} else {
				_, _ = fmt.Fprintf(stderr, "Type enforces template %q; not applied (description provided). Fetch it with: ws task template get %d\n", item.EnforcedTemplate.Name, item.EnforcedTemplate.TemplateID)
			}
		}

		output := NewOutput()
		output.Print(item)
		return nil
	},
}

var taskMoveCmd = &cobra.Command{
	Use:   "move <id|KEY-123> <status>",
	Short: "Change task status",
	Long: `Move a task to a different status. Validates workflow transitions.

The status can be:
  - A status alias from your config (e.g., "done", "progress", "blocked")
  - An exact status name (case-insensitive)
  - A partial match (e.g., "prog" matches "In Progress")
  - A status ID

Examples:
  ws task move PROJ-45 done               # Use status alias
  ws task move PROJ-45 "In Progress"      # Use exact name
  ws task move PROJ-45 3                  # Use status ID`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}

		itemID, err := client.ResolveItemID(args[0])
		if err != nil {
			return fmt.Errorf("failed to resolve item: %w", err)
		}

		statusInput := args[1]

		// Resolve status alias
		resolvedStatus := cfg.ResolveStatus(statusInput)

		// Get available transitions
		transitions, err := client.GetItemTransitions(itemID)
		if err != nil {
			return fmt.Errorf("failed to get transitions: %w", err)
		}

		// Find matching transition
		var targetStatusID int
		var matchedStatus string

		// First, try exact match by ID
		var statusID int
		if _, err = fmt.Sscanf(resolvedStatus, "%d", &statusID); err == nil {
			for _, t := range transitions {
				if t.ToStatusID == statusID {
					targetStatusID = statusID
					if t.ToStatus != nil {
						matchedStatus = t.ToStatus.Name
					}
					break
				}
			}
		}

		// If not found by ID, try name matching
		if targetStatusID == 0 {
			resolvedLower := strings.ToLower(resolvedStatus)
			for _, t := range transitions {
				if t.ToStatus == nil {
					continue
				}
				statusName := t.ToStatus.Name
				statusLower := strings.ToLower(statusName)

				// Exact match (case-insensitive)
				if statusLower == resolvedLower {
					targetStatusID = t.ToStatusID
					matchedStatus = statusName
					break
				}
				// Partial match
				if strings.Contains(statusLower, resolvedLower) {
					targetStatusID = t.ToStatusID
					matchedStatus = statusName
					// Don't break - continue looking for exact match
				}
			}
		}

		if targetStatusID == 0 {
			// Build error message with available options
			var available []string
			for _, t := range transitions {
				if t.ToStatus != nil {
					available = append(available, fmt.Sprintf("%s (ID: %d)", t.ToStatus.Name, t.ToStatusID))
				}
			}

			// Check if input was an alias
			aliasNote := ""
			if statusInput != resolvedStatus {
				aliasNote = fmt.Sprintf(" (alias for %q)", resolvedStatus)
			}

			return fmt.Errorf("cannot move to \"%s\"%s. Valid transitions:\n  - %s",
				statusInput, aliasNote, strings.Join(available, "\n  - "))
		}

		// Perform workflow transition
		result, err := client.TransitionItem(itemID, targetStatusID)
		if err != nil {
			return fmt.Errorf("failed to transition item: %w", err)
		}
		item := result.Item

		// Show success message for table output
		if outputFormat == "table" {
			_, _ = fmt.Fprintf(stdout, "Moved to \"%s\"\n", matchedStatus)
		}

		output := NewOutput()
		output.Print(item)
		return nil
	},
}

var taskSetMilestoneCmd = &cobra.Command{
	Use:   "set-milestone <item> [milestone]",
	Short: "Assign item to milestone",
	Long: `Assign an item to a milestone or remove it from its current milestone.

Examples:
  ws task set-milestone PROJ-123 5           # By milestone ID
  ws task set-milestone PROJ-123 "v1.0"      # By milestone name
  ws task set-milestone PROJ-123 --clear     # Remove from milestone`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}

		itemID, err := client.ResolveItemID(args[0])
		if err != nil {
			return fmt.Errorf("failed to resolve item: %w", err)
		}
		var itemForScope *Item
		if !clearMilestone {
			itemForScope, err = client.GetItem(itemID, "")
			if err != nil {
				return fmt.Errorf("failed to get item: %w", err)
			}
		}

		// With multi-milestone the typed request carries milestone_ids as a
		// pointer-to-slice: nil = "leave alone", non-nil empty slice =
		// "clear all", non-nil populated slice = "replace with these".
		var item *Item
		if clearMilestone {
			empty := []int{}
			req := ItemUpdateRequest{MilestoneIDs: &empty}
			var err error
			item, err = client.UpdateItem(itemID, req)
			if err != nil {
				return fmt.Errorf("failed to update item: %w", err)
			}
		} else {
			if len(args) < 2 {
				return fmt.Errorf("milestone argument required (or use --clear to remove)")
			}
			id, err := client.ResolveMilestoneID(args[1], &itemForScope.WorkspaceID)
			if err != nil {
				return fmt.Errorf("failed to resolve milestone: %w", err)
			}
			ids := []int{id}
			req := ItemUpdateRequest{MilestoneIDs: &ids}
			item, err = client.UpdateItem(itemID, req)
			if err != nil {
				return fmt.Errorf("failed to update item: %w", err)
			}
		}

		// Show success message for table output
		if outputFormat == "table" {
			if clearMilestone {
				_, _ = fmt.Fprintf(stdout, "Removed %s from milestone\n", args[0])
			} else if len(item.Milestones) > 0 {
				names := make([]string, len(item.Milestones))
				for i, m := range item.Milestones {
					names[i] = m.Name
				}
				_, _ = fmt.Fprintf(stdout, "Assigned %s to milestone(s) %q\n", args[0], strings.Join(names, ", "))
			} else {
				_, _ = fmt.Fprintf(stdout, "Updated %s milestone assignment\n", args[0])
			}
		}

		output := NewOutput()
		output.Print(item)
		return nil
	},
}

var taskHistoryCmd = &cobra.Command{
	Use:   "history <id|KEY-123>",
	Short: "Show the change history of a task",
	Long: `Show the field-level change history of a work item (field, old
value, new value, actor, time). The server returns the full history; --limit
keeps only the first N entries of that response.

Examples:
  ws task history PROJ-45
  ws task history PROJ-45 --limit 20
  ws task history 123 -o json`,
	Args: cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}

		itemID, err := client.ResolveItemID(args[0])
		if err != nil {
			return fmt.Errorf("failed to resolve item: %w", err)
		}

		history, err := client.GetItemHistory(itemID)
		if err != nil {
			return fmt.Errorf("failed to get item history: %w", err)
		}
		if historyLimit > 0 && len(history) > historyLimit {
			history = history[:historyLimit]
		}

		output := NewOutput()
		output.Print(history)
		return nil
	},
}

var taskChildrenCmd = &cobra.Command{
	Use:   "children <id|KEY-123>",
	Short: "List children of a task or epic",
	Long: `List all child items of a given item (e.g., stories under an epic).

Examples:
  ws task children CP-11                  # List stories under epic CP-11
  ws task children CP-11 -s ~done         # Exclude done items
  ws task children CP-11 --type 3         # Only stories (item type 3)
  ws task children 24                     # By numeric ID`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}

		itemID, err := client.ResolveItemID(args[0])
		if err != nil {
			return fmt.Errorf("failed to resolve item: %w", err)
		}

		filters, err := newFiltersWithWorkspace(client, map[string]string{
			"parent_id": fmt.Sprintf("%d", itemID),
		})
		if err != nil {
			return err
		}

		if err := applyStatusFilter(filters, childStatusFilter, client); err != nil {
			return err
		}
		if childTypeFilter != "" {
			filters["item_type_id"] = childTypeFilter
		}

		items, err := client.ListItems(filters)
		if err != nil {
			return fmt.Errorf("failed to list children: %w", err)
		}

		output := NewOutput()
		output.Print(items)
		return nil
	},
}

var taskParentCmd = &cobra.Command{
	Use:   "parent <id|KEY-123>",
	Short: "Show the parent of a task",
	Long: `Show the parent item of a given item — the inverse of "task children".

Resolves the item, then fetches and prints its parent. The parent is looked up
by its database id directly, so there is no need to reconstruct a key from the
raw parent_id field (which is a DB id, not a workspace key).

Examples:
  ws task parent WI-385                   # Show WI-385's parent
  ws task parent 552                      # By numeric ID`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}

		itemID, err := client.ResolveItemID(args[0])
		if err != nil {
			return fmt.Errorf("failed to resolve item: %w", err)
		}

		item, err := client.GetItem(itemID, "")
		if err != nil {
			return fmt.Errorf("failed to get item: %w", err)
		}

		if item.ParentID == nil {
			_, _ = fmt.Fprintf(stdout, "%s has no parent\n", args[0])
			return nil
		}

		// Fetch the parent by its DB id — GET /items/{id} takes the numeric id
		// directly, so this is correct by construction (no key arithmetic).
		parent, err := client.GetItem(*item.ParentID, "transitions")
		if err != nil {
			return fmt.Errorf("failed to get parent: %w", err)
		}

		output := NewOutput()
		output.Print(parent)
		return nil
	},
}

var taskEditCmd = &cobra.Command{
	Use:   "edit <id|KEY-123>",
	Short: "Edit a task",
	Long: `Edit an existing task's title, description, priority, assignee, or other fields.

Examples:
  ws task edit CP-30 -t "New title"
  ws task edit CP-30 -d "Updated description"
  ws task edit CP-30 --priority 2 --assignee 3
  ws task edit CP-30 --type Bug                 # Change item type by name
  ws task edit CP-30 --due-date 2026-07-20      # Set due date
  ws task edit CP-30 --start-date 2026-07-01 --end-date 2026-07-15
  ws task edit CP-30 --custom-field "Risk=High" # Set custom field by name
  ws task edit CP-30 --iteration "Sprint 12"    # Assign to iteration by name
  ws task edit CP-30 --project 3                # Assign to project by ID`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}

		itemID, err := client.ResolveItemID(args[0])
		if err != nil {
			return fmt.Errorf("failed to resolve item: %w", err)
		}
		needsWorkspaceContext := cmd.Flags().Changed("type") || cmd.Flags().Changed("iteration")
		var itemForScope *Item
		if needsWorkspaceContext {
			itemForScope, err = client.GetItem(itemID, "")
			if err != nil {
				return fmt.Errorf("failed to get item: %w", err)
			}
		}

		req := ItemUpdateRequest{}
		hasChanges := false

		if cmd.Flags().Changed("title") {
			req.Title = &editTitle
			hasChanges = true
		}
		if cmd.Flags().Changed("description") {
			desc := ParseCLIEscapes(editDescription)
			req.Description = &desc
			hasChanges = true
		}
		typeChanged := cmd.Flags().Changed("type")
		if cmd.Flags().Changed("priority") {
			req.PriorityID = &editPriorityID
			hasChanges = true
		}
		if cmd.Flags().Changed("assignee") {
			req.AssigneeID = &editAssigneeID
			hasChanges = true
		}
		if cmd.Flags().Changed("parent") {
			parentID, err := resolveParentID(client, editParent)
			if err != nil {
				return err
			}
			var parent *int
			if parentID > 0 {
				parent = &parentID
			}
			req.ParentID = &parent
			hasChanges = true
		}
		if cmd.Flags().Changed("due-date") {
			d, err := parseDateFlag("due-date", editDueDate)
			if err != nil {
				return err
			}
			req.DueDate = d
			hasChanges = true
		}
		if cmd.Flags().Changed("start-date") {
			d, err := parseDateFlag("start-date", editStartDate)
			if err != nil {
				return err
			}
			req.StartDate = d
			hasChanges = true
		}
		if cmd.Flags().Changed("end-date") {
			d, err := parseDateFlag("end-date", editEndDate)
			if err != nil {
				return err
			}
			req.EndDate = d
			hasChanges = true
		}
		if len(editCustomFields) > 0 {
			cf, err := parseCustomFieldFlags(client, editCustomFields)
			if err != nil {
				return err
			}
			req.CustomFields = cf
			hasChanges = true
		}
		if cmd.Flags().Changed("iteration") {
			id, err := client.ResolveIterationID(editIteration, &itemForScope.WorkspaceID)
			if err != nil {
				return fmt.Errorf("failed to resolve iteration: %w", err)
			}
			req.IterationID = &id
			hasChanges = true
		}
		if cmd.Flags().Changed("project") {
			id, err := parseProjectFlag(editProject)
			if err != nil {
				return err
			}
			req.ProjectID = &id
			hasChanges = true
		}

		if !hasChanges && !typeChanged {
			return fmt.Errorf("no changes specified. Use flags like -t, -d, --type, --priority, --assignee, --due-date, --custom-field, --iteration, --project")
		}

		var item *Item
		if hasChanges {
			item, err = client.UpdateItem(itemID, req)
			if err != nil {
				return fmt.Errorf("failed to update item: %w", err)
			}
		}
		if typeChanged {
			typeID, err := resolveItemTypeID(client, editType, &itemForScope.WorkspaceID)
			if err != nil {
				return err
			}
			var targetStatusID *int
			if cmd.Flags().Changed("type-status") {
				if editTypeStatusID <= 0 {
					return fmt.Errorf("--type-status must be a positive status ID")
				}
				targetStatusID = &editTypeStatusID
			}
			item, err = client.ChangeItemType(itemID, typeID, targetStatusID)
			if err != nil {
				return fmt.Errorf("failed to change item type: %w", err)
			}
		}

		if outputFormat == "table" {
			_, _ = fmt.Fprintf(stdout, "Updated %s\n", args[0])
		}

		output := NewOutput()
		output.Print(item)
		return nil
	},
}

// parseDateFlag parses a YYYY-MM-DD CLI flag value into a UTC time.Time.
func parseDateFlag(flagName, value string) (*time.Time, error) {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil, fmt.Errorf("invalid --%s %q: expected YYYY-MM-DD", flagName, value)
	}
	return &parsed, nil
}

// resolveItemTypeID resolves an item type given as a numeric ID or a name.
// Names match case-insensitively, exact first, then unique substring.
func resolveItemTypeID(client *Client, input string, workspaceID *int) (int, error) {
	if id, err := strconv.Atoi(input); err == nil {
		if id <= 0 {
			return 0, fmt.Errorf("item type ID must be positive")
		}
		return id, nil
	}

	var types []ItemType
	var err error
	if workspaceID != nil {
		types, err = client.GetWorkspaceItemTypes(*workspaceID)
	} else {
		types, err = client.ListItemTypes()
	}
	if err != nil {
		return 0, fmt.Errorf("failed to list item types: %w", err)
	}

	inputLower := strings.ToLower(input)
	var partial []ItemType
	for _, t := range types {
		nameLower := strings.ToLower(t.Name)
		if nameLower == inputLower {
			return t.ID, nil
		}
		if strings.Contains(nameLower, inputLower) {
			partial = append(partial, t)
		}
	}
	if len(partial) == 1 {
		return partial[0].ID, nil
	}

	var available []string
	for _, t := range types {
		available = append(available, fmt.Sprintf("%s (ID: %d)", t.Name, t.ID))
	}
	if len(partial) > 1 {
		var matches []string
		for _, t := range partial {
			matches = append(matches, t.Name)
		}
		return 0, fmt.Errorf("item type %q is ambiguous (matches %s)", input, strings.Join(matches, ", "))
	}
	return 0, fmt.Errorf("unknown item type %q. Available types:\n  - %s", input, strings.Join(available, "\n  - "))
}

// parseCustomFieldFlags turns repeated --custom-field <field>=<value> flags
// into the custom_fields wire map. Keys are resolved to custom-field IDs
// (numeric input passes through; names resolve via the v2 custom-fields
// read endpoint). Values pass through as strings — the server validates
// them against the field type.
func parseCustomFieldFlags(client *Client, pairs []string) (map[string]any, error) {
	fields := make(map[string]any, len(pairs))
	var defs []CustomField // lazily loaded, only when a non-numeric key shows up
	for _, pair := range pairs {
		key, value, found := strings.Cut(pair, "=")
		key = strings.TrimSpace(key)
		if !found || key == "" {
			return nil, fmt.Errorf("invalid --custom-field %q: expected <field>=<value>", pair)
		}
		if id, err := strconv.Atoi(key); err == nil {
			if id <= 0 {
				return nil, fmt.Errorf("invalid --custom-field %q: field ID must be positive", pair)
			}
			fields[strconv.Itoa(id)] = value
			continue
		}
		if defs == nil {
			var err error
			defs, err = client.ListCustomFields()
			if err != nil {
				return nil, fmt.Errorf("failed to list custom fields: %w", err)
			}
		}
		id, err := resolveCustomFieldID(key, defs)
		if err != nil {
			return nil, err
		}
		fields[strconv.Itoa(id)] = value
	}
	return fields, nil
}

// resolveCustomFieldID resolves a custom-field name against the workspace
// catalog. Names match case-insensitively, exact first, then unique
// substring — same convention as resolveItemTypeID.
func resolveCustomFieldID(name string, defs []CustomField) (int, error) {
	nameLower := strings.ToLower(name)
	var partial []CustomField
	for _, f := range defs {
		fLower := strings.ToLower(f.Name)
		if fLower == nameLower {
			return f.ID, nil
		}
		if strings.Contains(fLower, nameLower) {
			partial = append(partial, f)
		}
	}
	if len(partial) == 1 {
		return partial[0].ID, nil
	}
	if len(partial) > 1 {
		var matches []string
		for _, f := range partial {
			matches = append(matches, f.Name)
		}
		return 0, fmt.Errorf("custom field %q is ambiguous (matches %s)", name, strings.Join(matches, ", "))
	}
	var available []string
	for _, f := range defs {
		available = append(available, fmt.Sprintf("%s (ID: %d, %s)", f.Name, f.ID, f.FieldType))
	}
	if len(available) == 0 {
		return 0, fmt.Errorf("unknown custom field %q (no custom fields are defined)", name)
	}
	return 0, fmt.Errorf("unknown custom field %q. Available fields:\n  - %s", name, strings.Join(available, "\n  - "))
}

// parseProjectFlag parses the --project flag. The v2 item API has no project
// listing endpoint, so only numeric project IDs are accepted — no
// name-based resolution.
func parseProjectFlag(value string) (int, error) {
	id, err := strconv.Atoi(value)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid --project %q: expected a numeric project ID", value)
	}
	return id, nil
}

// applyDateFilters parses created/updated relative date filters and adds them to the filters map.
func applyDateFilters(filters map[string]string, createdFilter, updatedFilter string) error {
	if createdFilter != "" {
		from, to, err := parseRelativeDate(createdFilter)
		if err != nil {
			return err
		}
		if from != "" {
			filters["created_after"] = from
		}
		if to != "" {
			filters["created_before"] = to
		}
	}
	if updatedFilter != "" {
		from, to, err := parseRelativeDate(updatedFilter)
		if err != nil {
			return err
		}
		if from != "" {
			filters["updated_after"] = from
		}
		if to != "" {
			filters["updated_before"] = to
		}
	}
	return nil
}

// taskGetCommentLimit is the maximum number of comments embedded in `ws task get`
// output. Server returns comments newest-first, so this keeps the most recent ones.
const taskGetCommentLimit = 10

// Flags for task commands
var (
	statusFilter   string
	assigneeFilter string
	itemTypeFilter string
	priorityFilter string
	createdFilter  string
	updatedFilter  string
	openInBrowser  bool
	clearMilestone bool
	historyLimit   int

	childStatusFilter string
	childTypeFilter   string

	createTitle        string
	createDescription  string
	createType         string
	createTemplate     string
	createPriorityID   int
	createStatusID     int
	createAssigneeID   int
	createParent       string
	createDueDate      string
	createStartDate    string
	createEndDate      string
	createCustomFields []string
	createIteration    string
	createProject      string

	editTitle        string
	editDescription  string
	editType         string
	editTypeStatusID int
	editPriorityID   int
	editAssigneeID   int
	editParent       string
	editDueDate      string
	editStartDate    string
	editEndDate      string
	editCustomFields []string
	editIteration    string
	editProject      string
)

func init() {
	rootCmd.AddCommand(taskCmd)
	taskCmd.AddCommand(taskSearchCmd)
	taskCmd.AddCommand(taskMineCmd)
	taskCmd.AddCommand(taskCreatedCmd)
	taskCmd.AddCommand(taskListCmd)
	taskCmd.AddCommand(taskGetCmd)
	taskCmd.AddCommand(taskCreateCmd)
	taskCmd.AddCommand(taskEditCmd)
	taskCmd.AddCommand(taskChildrenCmd)
	taskCmd.AddCommand(taskParentCmd)
	taskCmd.AddCommand(taskMoveCmd)
	taskCmd.AddCommand(taskSetMilestoneCmd)
	taskCmd.AddCommand(taskHistoryCmd)

	// List filters
	taskListCmd.Flags().IntVar(&taskListPage, "page", 1, "result page (starts at 1)")
	taskListCmd.Flags().IntVar(&taskListLimit, "limit", 50, "results per page (1-100)")
	taskListCmd.Flags().BoolVar(&taskListAll, "all", false, "fetch every result page")
	taskListCmd.Flags().StringVar(&taskListMilestone, "milestone", "", "filter by milestone name or ID")
	taskListCmd.MarkFlagsMutuallyExclusive("all", "page")
	taskMineCmd.Flags().StringVarP(&statusFilter, "status", "s", "", "filter by status (use ~status to exclude)")
	taskMineCmd.Flags().StringVar(&createdFilter, "created", "", "filter by creation date (today, week, month, year, or -Nd)")
	taskMineCmd.Flags().StringVar(&updatedFilter, "updated", "", "filter by update date (today, week, month, year, or -Nd)")
	taskListCmd.Flags().StringVarP(&statusFilter, "status", "s", "", "filter by status (use ~status to exclude)")
	taskListCmd.Flags().StringVar(&assigneeFilter, "assignee", "", "filter by assignee ID")
	taskListCmd.Flags().StringVar(&itemTypeFilter, "type", "", "filter by item type ID")
	taskListCmd.Flags().StringVar(&priorityFilter, "priority", "", "filter by priority ID")
	taskListCmd.Flags().StringVar(&createdFilter, "created", "", "filter by creation date (today, week, month, year, or -Nd)")
	taskListCmd.Flags().StringVar(&updatedFilter, "updated", "", "filter by update date (today, week, month, year, or -Nd)")

	// Browser flags
	taskGetCmd.Flags().BoolVar(&openInBrowser, "web", false, "open task in browser")
	taskCreateCmd.Flags().BoolVar(&openInBrowser, "web", false, "open task in browser after creation")

	// Set-milestone flags
	taskSetMilestoneCmd.Flags().BoolVar(&clearMilestone, "clear", false, "remove item from milestone")

	// Search flags
	taskSearchCmd.Flags().IntVar(&taskSearchLimit, "limit", 0, "maximum results per page (server default if omitted, max 100)")
	taskSearchCmd.Flags().BoolVar(&taskSearchQL, "ql", false, "treat the query as a CQL filter (surfaces parse errors instead of full-text fallback)")

	// History flags
	taskHistoryCmd.Flags().IntVar(&historyLimit, "limit", 0, "show at most N history entries (0 = all)")

	// Children filters
	taskChildrenCmd.Flags().StringVarP(&childStatusFilter, "status", "s", "", "filter by status (use ~status to exclude)")
	taskChildrenCmd.Flags().StringVar(&childTypeFilter, "type", "", "filter by item type ID")

	// Edit flags
	taskEditCmd.Flags().StringVarP(&editTitle, "title", "t", "", "new title")
	taskEditCmd.Flags().StringVarP(&editDescription, "description", "d", "", "new description (supports \\n / \\t / \\\\)")
	taskEditCmd.Flags().StringVar(&editType, "type", "", "item type (name or ID); changes type via the change-type endpoint")
	taskEditCmd.Flags().IntVar(&editTypeStatusID, "type-status", 0, "target status ID when changing to a type with a different workflow")
	taskEditCmd.Flags().IntVar(&editPriorityID, "priority", 0, "priority ID")
	taskEditCmd.Flags().IntVar(&editAssigneeID, "assignee", 0, "assignee user ID")
	taskEditCmd.Flags().StringVar(&editParent, "parent", "", "parent item key or ID (0 clears parent)")
	taskEditCmd.Flags().StringVar(&editDueDate, "due-date", "", "due date (YYYY-MM-DD)")
	taskEditCmd.Flags().StringVar(&editStartDate, "start-date", "", "start date (YYYY-MM-DD)")
	taskEditCmd.Flags().StringVar(&editEndDate, "end-date", "", "end date (YYYY-MM-DD)")
	taskEditCmd.Flags().StringArrayVar(&editCustomFields, "custom-field", nil, "custom field value as <field>=<value> (repeatable; field is a name or numeric ID)")
	taskEditCmd.Flags().StringVar(&editIteration, "iteration", "", "iteration (name or numeric ID)")
	taskEditCmd.Flags().StringVar(&editProject, "project", "", "project ID (numeric only)")

	// Create flags
	taskCreateCmd.Flags().StringVarP(&createTitle, "title", "t", "", "task title (required)")
	taskCreateCmd.Flags().StringVarP(&createDescription, "description", "d", "", "task description (supports \\n / \\t / \\\\)")
	taskCreateCmd.Flags().StringVar(&createType, "type", "", "item type (name or ID)")
	taskCreateCmd.Flags().StringVar(&createTemplate, "template", "", "seed the description from a work item template (name or ID); exclusive with -d")
	taskCreateCmd.Flags().IntVar(&createPriorityID, "priority", 0, "priority ID")
	taskCreateCmd.Flags().IntVar(&createStatusID, "status", 0, "status ID")
	taskCreateCmd.Flags().IntVar(&createAssigneeID, "assignee", 0, "assignee user ID")
	taskCreateCmd.Flags().StringVar(&createParent, "parent", "", "parent item key or ID")
	taskCreateCmd.Flags().StringVar(&createDueDate, "due-date", "", "due date (YYYY-MM-DD)")
	taskCreateCmd.Flags().StringVar(&createStartDate, "start-date", "", "start date (YYYY-MM-DD)")
	taskCreateCmd.Flags().StringVar(&createEndDate, "end-date", "", "end date (YYYY-MM-DD)")
	taskCreateCmd.Flags().StringArrayVar(&createCustomFields, "custom-field", nil, "custom field value as <field>=<value> (repeatable; field is a name or numeric ID)")
	taskCreateCmd.Flags().StringVar(&createIteration, "iteration", "", "iteration (name or numeric ID)")
	taskCreateCmd.Flags().StringVar(&createProject, "project", "", "project ID (numeric only)")
}
