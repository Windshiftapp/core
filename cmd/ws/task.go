package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "Manage tasks and items",
	Long:  `Commands for viewing, creating, and managing work items.`,
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

		// Build filters
		filters := map[string]string{
			"assignee_id": fmt.Sprintf("%d", user.ID),
		}

		// Add workspace filter if configured
		if wsKey := cfg.GetEffectiveWorkspace(); wsKey != "" {
			var wsID int
			wsID, err = client.ResolveWorkspaceID(wsKey)
			if err != nil {
				return fmt.Errorf("failed to resolve workspace: %w", err)
			}
			filters["workspace_id"] = fmt.Sprintf("%d", wsID)
		}

		// Add optional filters from flags
		if statusFilter != "" {
			if isNegatedFilter(statusFilter) {
				resolved := cfg.ResolveStatus(stripNegation(statusFilter))
				filters["status_id_not"] = resolved
			} else {
				resolved := cfg.ResolveStatus(statusFilter)
				filters["status_id"] = resolved
			}
		}

		// Add date filters
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

		// Build filters
		filters := map[string]string{
			"creator_id": fmt.Sprintf("%d", user.ID),
		}

		// Add workspace filter if configured
		if wsKey := cfg.GetEffectiveWorkspace(); wsKey != "" {
			var wsID int
			wsID, err = client.ResolveWorkspaceID(wsKey)
			if err != nil {
				return fmt.Errorf("failed to resolve workspace: %w", err)
			}
			filters["workspace_id"] = fmt.Sprintf("%d", wsID)
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
  ws task ls                              # List all accessible tasks
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

		filters := make(map[string]string)

		// Add workspace filter if configured or passed as flag
		if wsKey := cfg.GetEffectiveWorkspace(); wsKey != "" {
			var wsID int
			wsID, err = client.ResolveWorkspaceID(wsKey)
			if err != nil {
				return fmt.Errorf("failed to resolve workspace: %w", err)
			}
			filters["workspace_id"] = fmt.Sprintf("%d", wsID)
		}

		// Add optional filters from flags
		if statusFilter != "" {
			if isNegatedFilter(statusFilter) {
				// Negation: exclude this status
				resolved := cfg.ResolveStatus(stripNegation(statusFilter))
				filters["status_id_not"] = resolved
			} else {
				resolved := cfg.ResolveStatus(statusFilter)
				filters["status_id"] = resolved
			}
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

		items, err := client.ListItems(filters)
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
	Long: `Get detailed information about a task, including available status transitions.

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

		// Get item with transitions expanded
		item, err := client.GetItem(itemID, "transitions")
		if err != nil {
			return fmt.Errorf("failed to get item: %w", err)
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
			fmt.Printf("Opened %s in browser\n", url)
			return nil
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
  ws task create -t "Bug" --type 1 --priority 2
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
			Description: createDescription,
		}

		// Set optional fields
		if createTypeID > 0 {
			req.ItemTypeID = &createTypeID
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
		if createParentID > 0 {
			req.ParentID = &createParentID
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
			fmt.Printf("Created %s-%d and opened in browser\n", wsKey, item.WorkspaceItemNumber)
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

		// Update item status
		req := ItemUpdateRequest{
			StatusID: &targetStatusID,
		}

		item, err := client.UpdateItem(itemID, req)
		if err != nil {
			return fmt.Errorf("failed to update item: %w", err)
		}

		// Show success message for table output
		if outputFormat == "table" {
			fmt.Printf("Moved to \"%s\"\n", matchedStatus)
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

		var milestoneID *int

		if clearMilestone {
			// Clear milestone - set to nil (use zero value)
			zero := 0
			milestoneID = &zero
		} else if len(args) < 2 {
			return fmt.Errorf("milestone argument required (or use --clear to remove)")
		} else {
			// Resolve workspace for milestone lookup
			var wsID *int
			if wsKey := cfg.GetEffectiveWorkspace(); wsKey != "" {
				id, err := client.ResolveWorkspaceID(wsKey)
				if err != nil {
					return fmt.Errorf("failed to resolve workspace: %w", err)
				}
				wsID = &id
			}

			id, err := client.ResolveMilestoneID(args[1], wsID)
			if err != nil {
				return fmt.Errorf("failed to resolve milestone: %w", err)
			}
			milestoneID = &id
		}

		req := ItemUpdateRequest{
			MilestoneID: milestoneID,
		}

		item, err := client.UpdateItem(itemID, req)
		if err != nil {
			return fmt.Errorf("failed to update item: %w", err)
		}

		// Show success message for table output
		if outputFormat == "table" {
			if clearMilestone {
				fmt.Printf("Removed %s from milestone\n", args[0])
			} else if item.Milestone != nil {
				fmt.Printf("Assigned %s to milestone \"%s\"\n", args[0], item.Milestone.Name)
			} else {
				fmt.Printf("Updated %s milestone assignment\n", args[0])
			}
		}

		output := NewOutput()
		output.Print(item)
		return nil
	},
}

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

	createTitle       string
	createDescription string
	createTypeID      int
	createPriorityID  int
	createStatusID    int
	createAssigneeID  int
	createParentID    int
)

func init() {
	rootCmd.AddCommand(taskCmd)
	taskCmd.AddCommand(taskMineCmd)
	taskCmd.AddCommand(taskCreatedCmd)
	taskCmd.AddCommand(taskListCmd)
	taskCmd.AddCommand(taskGetCmd)
	taskCmd.AddCommand(taskCreateCmd)
	taskCmd.AddCommand(taskMoveCmd)
	taskCmd.AddCommand(taskSetMilestoneCmd)

	// List filters
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

	// Create flags
	taskCreateCmd.Flags().StringVarP(&createTitle, "title", "t", "", "task title (required)")
	taskCreateCmd.Flags().StringVarP(&createDescription, "description", "d", "", "task description")
	taskCreateCmd.Flags().IntVar(&createTypeID, "type", 0, "item type ID")
	taskCreateCmd.Flags().IntVar(&createPriorityID, "priority", 0, "priority ID")
	taskCreateCmd.Flags().IntVar(&createStatusID, "status", 0, "status ID")
	taskCreateCmd.Flags().IntVar(&createAssigneeID, "assignee", 0, "assignee user ID")
	taskCreateCmd.Flags().IntVar(&createParentID, "parent", 0, "parent item ID")
}
