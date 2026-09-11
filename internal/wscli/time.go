package wscli

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

var timeCmd = &cobra.Command{
	Use:   "time",
	Short: "Manage time tracking (projects, worklogs, timers)",
}

var timeProjectCmd = &cobra.Command{
	Use:   "project",
	Short: "List time tracking projects",
}

var timeProjectListCmd = &cobra.Command{
	Use:   "ls",
	Short: "List accessible time tracking projects",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}
		projects, err := client.ListTimeProjects()
		if err != nil {
			return err
		}
		NewOutput().Print(projects)
		return nil
	},
}

var timeWorklogCmd = &cobra.Command{
	Use:   "worklog",
	Short: "Manage time tracking worklogs",
}

var timeWorklogListCmd = &cobra.Command{
	Use:   "ls",
	Short: "List your time tracking worklogs",
	RunE: func(cmd *cobra.Command, _ []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}
		filters := map[string]string{}
		if v, _ := cmd.Flags().GetString("date-from"); v != "" {
			filters["date_from"] = v
		}
		if v, _ := cmd.Flags().GetString("date-to"); v != "" {
			filters["date_to"] = v
		}
		if v, _ := cmd.Flags().GetString("project-id"); v != "" {
			filters["project_id"] = v
		}
		resp, err := client.ListTimeWorklogs(filters)
		if err != nil {
			return err
		}
		NewOutput().Print(resp.Data)
		return nil
	},
}

var timeWorklogAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Log a new time entry",
	RunE: func(cmd *cobra.Command, _ []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}
		req := TimeWorklogCreateRequest{}
		req.ProjectID, _ = cmd.Flags().GetInt("project-id")
		req.Description, _ = cmd.Flags().GetString("description")
		req.Date, _ = cmd.Flags().GetString("date")
		req.Duration, _ = cmd.Flags().GetString("duration")
		req.DurationMinutes, _ = cmd.Flags().GetInt("duration-minutes")
		req.StartTime, _ = cmd.Flags().GetString("start-time")
		req.EndTime, _ = cmd.Flags().GetString("end-time")
		if itemIDStr, _ := cmd.Flags().GetString("item-id"); itemIDStr != "" {
			id, err := strconv.Atoi(itemIDStr)
			if err != nil {
				return fmt.Errorf("invalid item-id: %w", err)
			}
			req.ItemID = &id
		}
		if itemKey, _ := cmd.Flags().GetString("item-key"); itemKey != "" {
			id, err := client.ResolveItemID(itemKey)
			if err != nil {
				return fmt.Errorf("failed to resolve item key %q: %w", itemKey, err)
			}
			req.ItemID = &id
		}

		out, err := client.CreateTimeWorklog(req)
		if err != nil {
			return err
		}
		fmt.Printf("Worklog created: id=%v, project=%v, date=%v, duration=%v minutes\n",
			out["id"], out["project_name"], out["date"], out["duration_minutes"])
		return nil
	},
}

var timeWorklogEditCmd = &cobra.Command{
	Use:   "edit <id>",
	Short: "Update a worklog description",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}
		id, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid worklog id: %w", err)
		}
		desc, _ := cmd.Flags().GetString("description")
		if err := client.UpdateTimeWorklog(id, desc); err != nil {
			return err
		}
		fmt.Printf("Worklog %d updated.\n", id)
		return nil
	},
}

var timeWorklogRmCmd = &cobra.Command{
	Use:   "rm <id>",
	Short: "Delete a worklog",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}
		id, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid worklog id: %w", err)
		}
		if err := client.DeleteTimeWorklog(id); err != nil {
			return err
		}
		fmt.Printf("Worklog %d deleted.\n", id)
		return nil
	},
}

var timeTimerCmd = &cobra.Command{
	Use:   "timer",
	Short: "Manage active timers",
}

var timeTimerStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start a new timer",
	RunE: func(cmd *cobra.Command, _ []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}
		req := TimerStartRequest{}
		req.WorkspaceID, _ = cmd.Flags().GetInt("workspace-id")
		req.ProjectID, _ = cmd.Flags().GetInt("project-id")
		req.Description, _ = cmd.Flags().GetString("description")
		if itemIDStr, _ := cmd.Flags().GetString("item-id"); itemIDStr != "" {
			id, err := strconv.Atoi(itemIDStr)
			if err != nil {
				return fmt.Errorf("invalid item-id: %w", err)
			}
			req.ItemID = &id
		}
		out, err := client.StartTimer(req)
		if err != nil {
			return err
		}
		fmt.Printf("Timer started: id=%v\n", out["id"])
		return nil
	},
}

var timeTimerStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the currently running timer",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}
		out, err := client.StopTimer()
		if err != nil {
			return err
		}
		var apiErr *APIError
		if out != nil && (!errors.As(err, &apiErr) || apiErr == nil) {
			fmt.Printf("Timer stopped: duration=%v minutes, worklog_created=%v\n",
				out["duration_minutes"], out["worklog_created"])
		}
		return nil
	},
}

var timeTimerStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the currently running timer",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}
		timer, err := client.GetActiveTimer()
		if err != nil {
			return err
		}
		if timer == nil {
			fmt.Println("No active timer.")
			return nil
		}
		NewOutput().Print(timer)
		return nil
	},
}

func init() {
	// Override the default empty-flag-error to include --description
	timeWorklogEditCmd.Flags().String("description", "", "New description")

	timeWorklogListCmd.Flags().String("date-from", "", "Start date (YYYY-MM-DD)")
	timeWorklogListCmd.Flags().String("date-to", "", "End date (YYYY-MM-DD)")
	timeWorklogListCmd.Flags().String("project-id", "", "Filter by project ID")

	timeWorklogAddCmd.Flags().Int("project-id", 0, "Time project ID (required)")
	timeWorklogAddCmd.Flags().String("description", "", "Description of work (required)")
	timeWorklogAddCmd.Flags().String("date", "", "Date in YYYY-MM-DD (required)")
	timeWorklogAddCmd.Flags().String("duration", "", "Duration like '2h', '30m', '1h30m'")
	timeWorklogAddCmd.Flags().Int("duration-minutes", 0, "Duration in minutes")
	timeWorklogAddCmd.Flags().String("start-time", "", "Start time HH:MM (pair with end-time)")
	timeWorklogAddCmd.Flags().String("end-time", "", "End time HH:MM (pair with start-time)")
	timeWorklogAddCmd.Flags().String("item-id", "", "Optional linked work item ID")
	timeWorklogAddCmd.Flags().String("item-key", "", "Optional linked work item key (PROJ-42)")
	timeWorklogAddCmd.MarkFlagsMutuallyExclusive("item-id", "item-key")

	timeTimerStartCmd.Flags().Int("workspace-id", 0, "Workspace ID (required)")
	timeTimerStartCmd.Flags().Int("project-id", 0, "Time project ID (required)")
	timeTimerStartCmd.Flags().String("description", "", "Timer description (required)")
	timeTimerStartCmd.Flags().String("item-id", "", "Optional linked work item ID")

	timeProjectCmd.AddCommand(timeProjectListCmd)
	timeWorklogCmd.AddCommand(timeWorklogListCmd, timeWorklogAddCmd, timeWorklogEditCmd, timeWorklogRmCmd)
	timeTimerCmd.AddCommand(timeTimerStartCmd, timeTimerStopCmd, timeTimerStatusCmd)
	timeCmd.AddCommand(timeProjectCmd, timeWorklogCmd, timeTimerCmd)
	rootCmd.AddCommand(timeCmd)
}
