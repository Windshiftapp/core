package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var milestoneCmd = &cobra.Command{
	Use:   "milestone",
	Short: "Manage milestones",
	Long:  `Commands for viewing, creating, and managing milestones.`,
}

var milestoneListCmd = &cobra.Command{
	Use:   "ls",
	Short: "List milestones",
	Long: `List milestones with optional filtering.

Examples:
  ws milestone ls                         # All milestones
  ws milestone ls -w PROJ                 # Workspace milestones
  ws milestone ls --status in-progress    # Filter by status
  ws milestone ls --global                # Global milestones only`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}

		filters := make(map[string]string)

		// Add workspace filter if configured or passed as flag
		if wsKey := cfg.GetEffectiveWorkspace(); wsKey != "" {
			wsID, err := client.ResolveWorkspaceID(wsKey)
			if err != nil {
				return fmt.Errorf("failed to resolve workspace: %w", err)
			}
			filters["workspace_id"] = fmt.Sprintf("%d", wsID)
		}

		// Add status filter
		if milestoneStatusFilter != "" {
			filters["status"] = milestoneStatusFilter
		}

		// Add global filter
		if milestoneGlobalOnly {
			filters["is_global"] = "true"
		}

		milestones, err := client.ListMilestones(filters)
		if err != nil {
			return fmt.Errorf("failed to list milestones: %w", err)
		}

		output := NewOutput()
		output.Print(milestones)
		return nil
	},
}

var milestoneGetCmd = &cobra.Command{
	Use:   "get <id|name>",
	Short: "Get milestone details",
	Long: `Get detailed information about a milestone.

Examples:
  ws milestone get 5                      # By ID
  ws milestone get "v1.0 Release"         # By name (fuzzy match)
  ws milestone get 5 --progress           # Include progress report`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}

		// Resolve workspace if set (for name lookup)
		var wsID *int
		if wsKey := cfg.GetEffectiveWorkspace(); wsKey != "" {
			id, err := client.ResolveWorkspaceID(wsKey)
			if err != nil {
				return fmt.Errorf("failed to resolve workspace: %w", err)
			}
			wsID = &id
		}

		milestoneID, err := client.ResolveMilestoneID(args[0], wsID)
		if err != nil {
			return fmt.Errorf("failed to resolve milestone: %w", err)
		}

		// Get with progress if requested
		if milestoneShowProgress {
			progress, err := client.GetMilestoneProgress(milestoneID)
			if err != nil {
				return fmt.Errorf("failed to get milestone progress: %w", err)
			}

			output := NewOutput()
			output.Print(progress)
			return nil
		}

		milestone, err := client.GetMilestone(milestoneID)
		if err != nil {
			return fmt.Errorf("failed to get milestone: %w", err)
		}

		output := NewOutput()
		output.Print(milestone)
		return nil
	},
}

var milestoneCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new milestone",
	Long: `Create a new milestone.

Examples:
  ws milestone create -n "v2.0 Release" -d "Major release" --target 2024-06-01
  ws milestone create -n "Sprint 5" -w PROJ --status planning`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if milestoneCreateName == "" {
			return fmt.Errorf("name is required: use -n or --name")
		}

		client, err := NewClient()
		if err != nil {
			return err
		}

		req := MilestoneCreateRequest{
			Name:        milestoneCreateName,
			Description: milestoneCreateDesc,
			TargetDate:  milestoneCreateTarget,
			Status:      milestoneCreateStatus,
		}

		// Set workspace ID if provided
		if wsKey := cfg.GetEffectiveWorkspace(); wsKey != "" {
			wsID, err := client.ResolveWorkspaceID(wsKey)
			if err != nil {
				return fmt.Errorf("failed to resolve workspace: %w", err)
			}
			req.WorkspaceID = &wsID
		}

		milestone, err := client.CreateMilestone(req)
		if err != nil {
			return fmt.Errorf("failed to create milestone: %w", err)
		}

		if outputFormat == "table" {
			fmt.Printf("Created milestone \"%s\" (ID: %d)\n", milestone.Name, milestone.ID)
		}

		output := NewOutput()
		output.Print(milestone)
		return nil
	},
}

var milestoneUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update a milestone",
	Long: `Update an existing milestone.

Examples:
  ws milestone update 5 --status completed
  ws milestone update 5 -n "v2.1 Release" --target 2024-07-01`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}

		// Resolve workspace if set (for name lookup)
		var wsID *int
		if wsKey := cfg.GetEffectiveWorkspace(); wsKey != "" {
			id, err := client.ResolveWorkspaceID(wsKey)
			if err != nil {
				return fmt.Errorf("failed to resolve workspace: %w", err)
			}
			wsID = &id
		}

		milestoneID, err := client.ResolveMilestoneID(args[0], wsID)
		if err != nil {
			return fmt.Errorf("failed to resolve milestone: %w", err)
		}

		req := MilestoneUpdateRequest{}
		hasUpdate := false

		if cmd.Flags().Changed("name") {
			req.Name = &milestoneUpdateName
			hasUpdate = true
		}
		if cmd.Flags().Changed("description") {
			req.Description = &milestoneUpdateDesc
			hasUpdate = true
		}
		if cmd.Flags().Changed("target") {
			req.TargetDate = &milestoneUpdateTarget
			hasUpdate = true
		}
		if cmd.Flags().Changed("status") {
			req.Status = &milestoneUpdateStatus
			hasUpdate = true
		}

		if !hasUpdate {
			return fmt.Errorf("no updates specified. Use --name, --description, --target, or --status")
		}

		milestone, err := client.UpdateMilestone(milestoneID, req)
		if err != nil {
			return fmt.Errorf("failed to update milestone: %w", err)
		}

		if outputFormat == "table" {
			fmt.Printf("Updated milestone \"%s\" (ID: %d)\n", milestone.Name, milestone.ID)
		}

		output := NewOutput()
		output.Print(milestone)
		return nil
	},
}

var milestoneDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a milestone",
	Long: `Delete a milestone.

Examples:
  ws milestone delete 5`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}

		// Resolve workspace if set (for name lookup)
		var wsID *int
		if wsKey := cfg.GetEffectiveWorkspace(); wsKey != "" {
			id, err := client.ResolveWorkspaceID(wsKey)
			if err != nil {
				return fmt.Errorf("failed to resolve workspace: %w", err)
			}
			wsID = &id
		}

		milestoneID, err := client.ResolveMilestoneID(args[0], wsID)
		if err != nil {
			return fmt.Errorf("failed to resolve milestone: %w", err)
		}

		if err := client.DeleteMilestone(milestoneID); err != nil {
			return fmt.Errorf("failed to delete milestone: %w", err)
		}

		switch outputFormat {
		case "table":
			fmt.Printf("Deleted milestone %d\n", milestoneID)
		case "json":
			output := NewOutput()
			output.Print(map[string]interface{}{
				"deleted":      true,
				"milestone_id": milestoneID,
			})
		}

		return nil
	},
}

// Flags for milestone commands
var (
	milestoneStatusFilter string
	milestoneGlobalOnly   bool
	milestoneShowProgress bool

	milestoneCreateName   string
	milestoneCreateDesc   string
	milestoneCreateTarget string
	milestoneCreateStatus string

	milestoneUpdateName   string
	milestoneUpdateDesc   string
	milestoneUpdateTarget string
	milestoneUpdateStatus string
)

func init() {
	rootCmd.AddCommand(milestoneCmd)
	milestoneCmd.AddCommand(milestoneListCmd)
	milestoneCmd.AddCommand(milestoneGetCmd)
	milestoneCmd.AddCommand(milestoneCreateCmd)
	milestoneCmd.AddCommand(milestoneUpdateCmd)
	milestoneCmd.AddCommand(milestoneDeleteCmd)

	// List filters
	milestoneListCmd.Flags().StringVarP(&milestoneStatusFilter, "status", "s", "", "filter by status (planning, in-progress, completed, canceled)")
	milestoneListCmd.Flags().BoolVar(&milestoneGlobalOnly, "global", false, "show only global milestones")

	// Get flags
	milestoneGetCmd.Flags().BoolVar(&milestoneShowProgress, "progress", false, "include progress report")

	// Create flags
	milestoneCreateCmd.Flags().StringVarP(&milestoneCreateName, "name", "n", "", "milestone name (required)")
	milestoneCreateCmd.Flags().StringVarP(&milestoneCreateDesc, "description", "d", "", "milestone description")
	milestoneCreateCmd.Flags().StringVar(&milestoneCreateTarget, "target", "", "target date (YYYY-MM-DD)")
	milestoneCreateCmd.Flags().StringVar(&milestoneCreateStatus, "status", "", "initial status (default: planning)")

	// Update flags
	milestoneUpdateCmd.Flags().StringVarP(&milestoneUpdateName, "name", "n", "", "new milestone name")
	milestoneUpdateCmd.Flags().StringVarP(&milestoneUpdateDesc, "description", "d", "", "new description")
	milestoneUpdateCmd.Flags().StringVar(&milestoneUpdateTarget, "target", "", "new target date (YYYY-MM-DD)")
	milestoneUpdateCmd.Flags().StringVar(&milestoneUpdateStatus, "status", "", "new status")
}
