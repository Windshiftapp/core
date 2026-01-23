package main

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

var testCmd = &cobra.Command{
	Use:   "test",
	Short: "Test management commands",
	Long:  `Commands for managing test cases, runs, and results.`,
}

// ============================================
// Test Case Commands
// ============================================

var testCaseCmd = &cobra.Command{
	Use:   "case",
	Short: "Manage test cases",
}

var testCaseListCmd = &cobra.Command{
	Use:   "ls",
	Short: "List test cases",
	Long: `List test cases in the workspace.

Examples:
  ws test case ls                         # List all test cases
  ws test case ls --folder 5              # List cases in folder ID 5`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}

		wsID, err := getWorkspaceID(client)
		if err != nil {
			return err
		}

		cases, err := client.ListTestCases(wsID, testCaseFolderFilter)
		if err != nil {
			return fmt.Errorf("failed to list test cases: %w", err)
		}

		output := NewOutput()
		output.Print(cases)
		return nil
	},
}

var testCaseGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get test case details with steps",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}

		wsID, err := getWorkspaceID(client)
		if err != nil {
			return err
		}

		caseID, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid test case ID: %s", args[0])
		}

		testCase, err := client.GetTestCase(wsID, caseID)
		if err != nil {
			return fmt.Errorf("failed to get test case: %w", err)
		}

		// Get steps for the test case
		steps, err := client.GetTestSteps(wsID, caseID)
		if err != nil {
			return fmt.Errorf("failed to get test steps: %w", err)
		}

		// For JSON output, include steps in the response
		if outputFormat == "json" {
			result := struct {
				*TestCase
				Steps []TestStep `json:"steps"`
			}{
				TestCase: testCase,
				Steps:    steps,
			}
			output := NewOutput()
			output.Print(result)
		} else {
			output := NewOutput()
			output.Print(testCase)
			if len(steps) > 0 {
				fmt.Println("\nSteps:")
				for _, step := range steps {
					fmt.Printf("  %d. %s\n", step.StepNumber, step.Action)
					if step.Data != "" {
						fmt.Printf("     Data: %s\n", step.Data)
					}
					if step.Expected != "" {
						fmt.Printf("     Expected: %s\n", step.Expected)
					}
				}
			}
		}
		return nil
	},
}

// ============================================
// Test Run Commands
// ============================================

var testRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Manage test runs",
}

var testRunMineCmd = &cobra.Command{
	Use:   "mine",
	Short: "List test runs assigned to me",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}

		wsID, err := getWorkspaceID(client)
		if err != nil {
			return err
		}

		// Get current user
		user, err := client.GetCurrentUser()
		if err != nil {
			return fmt.Errorf("failed to get current user: %w", err)
		}

		runs, err := client.ListTestRuns(wsID, fmt.Sprintf("%d", user.ID))
		if err != nil {
			return fmt.Errorf("failed to list test runs: %w", err)
		}

		output := NewOutput()
		output.Print(runs)
		return nil
	},
}

var testRunListCmd = &cobra.Command{
	Use:   "ls",
	Short: "List test runs",
	Long: `List test runs with optional filtering.

Examples:
  ws test run ls                          # List all test runs
  ws test run ls --set 3                  # List runs for test set 3`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}

		wsID, err := getWorkspaceID(client)
		if err != nil {
			return err
		}

		runs, err := client.ListTestRuns(wsID, testRunAssigneeFilter)
		if err != nil {
			return fmt.Errorf("failed to list test runs: %w", err)
		}

		// Filter by set if specified
		if testRunSetFilter != "" {
			setID, err := strconv.Atoi(testRunSetFilter)
			if err == nil {
				var filtered []TestRun
				for _, run := range runs {
					if run.SetID == setID {
						filtered = append(filtered, run)
					}
				}
				runs = filtered
			}
		}

		output := NewOutput()
		output.Print(runs)
		return nil
	},
}

var testRunGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get test run details with results",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}

		wsID, err := getWorkspaceID(client)
		if err != nil {
			return err
		}

		runID, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid run ID: %s", args[0])
		}

		run, err := client.GetTestRun(wsID, runID)
		if err != nil {
			return fmt.Errorf("failed to get test run: %w", err)
		}

		results, err := client.GetTestRunResults(wsID, runID)
		if err != nil {
			return fmt.Errorf("failed to get results: %w", err)
		}

		// Calculate summary
		summary := struct {
			Total   int `json:"total"`
			Passed  int `json:"passed"`
			Failed  int `json:"failed"`
			Blocked int `json:"blocked"`
			Skipped int `json:"skipped"`
			NotRun  int `json:"not_run"`
		}{}

		for _, r := range results {
			summary.Total++
			switch r.Status {
			case "passed":
				summary.Passed++
			case "failed":
				summary.Failed++
			case "blocked":
				summary.Blocked++
			case "skipped":
				summary.Skipped++
			case "not_run":
				summary.NotRun++
			}
		}

		// For JSON output, include everything
		if outputFormat == "json" {
			result := struct {
				*TestRun
				Results []TestResult `json:"results"`
				Summary struct {
					Total   int `json:"total"`
					Passed  int `json:"passed"`
					Failed  int `json:"failed"`
					Blocked int `json:"blocked"`
					Skipped int `json:"skipped"`
					NotRun  int `json:"not_run"`
				} `json:"summary"`
			}{
				TestRun: run,
				Results: results,
				Summary: summary,
			}
			output := NewOutput()
			output.Print(result)
		} else {
			output := NewOutput()
			output.Print(run)
			fmt.Printf("\nSummary: %d total | %d passed | %d failed | %d blocked | %d skipped | %d not run\n",
				summary.Total, summary.Passed, summary.Failed, summary.Blocked, summary.Skipped, summary.NotRun)
			if len(results) > 0 {
				fmt.Println("\nResults:")
				output.Print(results)
			}
		}
		return nil
	},
}

var testRunStartCmd = &cobra.Command{
	Use:   "start <set-id>",
	Short: "Start a new test run from a test set",
	Long: `Start a new test run from a test set or template.

Examples:
  ws test run start 3                     # Start run from test set 3
  ws test run start --template 5          # Start run from template 5`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}

		wsID, err := getWorkspaceID(client)
		if err != nil {
			return err
		}

		var run *TestRun

		if testRunTemplateID > 0 {
			// Start from template
			run, err = client.ExecuteRunTemplate(wsID, testRunTemplateID)
			if err != nil {
				return fmt.Errorf("failed to start run from template: %w", err)
			}
		} else if len(args) > 0 {
			// Start from test set
			setID, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid set ID: %s", args[0])
			}

			// Get test set to use its name
			set, err := client.GetTestSet(wsID, setID)
			if err != nil {
				return fmt.Errorf("failed to get test set: %w", err)
			}

			name := testRunName
			if name == "" {
				name = fmt.Sprintf("%s - Run", set.Name)
			}

			req := TestRunCreateRequest{
				SetID: setID,
				Name:  name,
			}

			run, err = client.CreateTestRun(wsID, req)
			if err != nil {
				return fmt.Errorf("failed to create test run: %w", err)
			}
		} else {
			return fmt.Errorf("either a set ID argument or --template flag is required")
		}

		output := NewOutput()
		output.Print(run)
		return nil
	},
}

var testRunEndCmd = &cobra.Command{
	Use:   "end <id>",
	Short: "End/complete a test run",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}

		wsID, err := getWorkspaceID(client)
		if err != nil {
			return err
		}

		runID, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid run ID: %s", args[0])
		}

		if err := client.EndTestRun(wsID, runID); err != nil {
			return fmt.Errorf("failed to end test run: %w", err)
		}

		// Get updated run with results for summary
		run, err := client.GetTestRun(wsID, runID)
		if err != nil {
			return fmt.Errorf("failed to get test run: %w", err)
		}

		results, err := client.GetTestRunResults(wsID, runID)
		if err != nil {
			return fmt.Errorf("failed to get results: %w", err)
		}

		// Calculate summary
		summary := struct {
			Total   int `json:"total"`
			Passed  int `json:"passed"`
			Failed  int `json:"failed"`
			Blocked int `json:"blocked"`
			Skipped int `json:"skipped"`
		}{}

		for _, r := range results {
			summary.Total++
			switch r.Status {
			case "passed":
				summary.Passed++
			case "failed":
				summary.Failed++
			case "blocked":
				summary.Blocked++
			case "skipped":
				summary.Skipped++
			}
		}

		if outputFormat == "json" {
			result := struct {
				*TestRun
				Summary struct {
					Total   int `json:"total"`
					Passed  int `json:"passed"`
					Failed  int `json:"failed"`
					Blocked int `json:"blocked"`
					Skipped int `json:"skipped"`
				} `json:"summary"`
			}{
				TestRun: run,
				Summary: summary,
			}
			output := NewOutput()
			output.Print(result)
		} else {
			fmt.Println("Test run ended.")
			fmt.Printf("Summary: %d total | %d passed | %d failed | %d blocked | %d skipped\n",
				summary.Total, summary.Passed, summary.Failed, summary.Blocked, summary.Skipped)
		}
		return nil
	},
}

// ============================================
// Test Result Commands
// ============================================

var testResultCmd = &cobra.Command{
	Use:   "result <run-id> <case-id> <status>",
	Short: "Record a test case result",
	Long: `Record the result of a test case in a test run.

Status must be one of: passed, failed, blocked, skipped

Examples:
  ws test result 7 1 passed               # Mark case 1 as passed in run 7
  ws test result 7 2 failed --notes "Button not visible on mobile"`,
	Args: cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}

		wsID, err := getWorkspaceID(client)
		if err != nil {
			return err
		}

		runID, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid run ID: %s", args[0])
		}

		caseID, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("invalid case ID: %s", args[1])
		}

		status := args[2]
		validStatuses := map[string]bool{
			"passed":  true,
			"failed":  true,
			"blocked": true,
			"skipped": true,
		}
		if !validStatuses[status] {
			return fmt.Errorf("invalid status: %s. Must be one of: passed, failed, blocked, skipped", status)
		}

		// Get results to find the result ID for this test case
		results, err := client.GetTestRunResults(wsID, runID)
		if err != nil {
			return fmt.Errorf("failed to get results: %w", err)
		}

		var resultID int
		for _, r := range results {
			if r.TestCaseID == caseID {
				resultID = r.ID
				break
			}
		}

		if resultID == 0 {
			return fmt.Errorf("test case %d not found in run %d", caseID, runID)
		}

		req := TestResultUpdateRequest{
			Status:       status,
			ActualResult: testResultActual,
			Notes:        testResultNotes,
		}

		if err := client.UpdateTestResult(wsID, runID, resultID, req); err != nil {
			return fmt.Errorf("failed to update result: %w", err)
		}

		// Return updated result
		results, err = client.GetTestRunResults(wsID, runID)
		if err != nil {
			return fmt.Errorf("failed to get results: %w", err)
		}

		for _, r := range results {
			if r.TestCaseID == caseID {
				output := NewOutput()
				output.Print(r)
				return nil
			}
		}

		fmt.Println("Result recorded successfully")
		return nil
	},
}

// ============================================
// Test Set Commands
// ============================================

var testSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Manage test sets",
}

var testSetListCmd = &cobra.Command{
	Use:   "ls",
	Short: "List test sets",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}

		wsID, err := getWorkspaceID(client)
		if err != nil {
			return err
		}

		sets, err := client.ListTestSets(wsID)
		if err != nil {
			return fmt.Errorf("failed to list test sets: %w", err)
		}

		output := NewOutput()
		output.Print(sets)
		return nil
	},
}

var testSetGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get test set details with test cases",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := NewClient()
		if err != nil {
			return err
		}

		wsID, err := getWorkspaceID(client)
		if err != nil {
			return err
		}

		setID, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid set ID: %s", args[0])
		}

		set, err := client.GetTestSet(wsID, setID)
		if err != nil {
			return fmt.Errorf("failed to get test set: %w", err)
		}

		cases, err := client.GetTestSetTestCases(wsID, setID)
		if err != nil {
			return fmt.Errorf("failed to get test cases: %w", err)
		}

		if outputFormat == "json" {
			result := struct {
				*TestSet
				TestCases []TestCase `json:"test_cases"`
			}{
				TestSet:   set,
				TestCases: cases,
			}
			output := NewOutput()
			output.Print(result)
		} else {
			output := NewOutput()
			output.Print(set)
			if len(cases) > 0 {
				fmt.Println("\nTest Cases:")
				output.Print(cases)
			}
		}
		return nil
	},
}

// ============================================
// Helper Functions
// ============================================

func getWorkspaceID(client *Client) (int, error) {
	wsKey := cfg.GetEffectiveWorkspace()
	if wsKey == "" {
		return 0, fmt.Errorf("workspace is required: use -w flag or set defaults.workspace_key in config")
	}
	return client.ResolveWorkspaceID(wsKey)
}

// Flags for test commands
var (
	testCaseFolderFilter  string
	testRunSetFilter      string
	testRunAssigneeFilter string
	testRunTemplateID     int
	testRunName           string
	testResultNotes       string
	testResultActual      string
)

func init() {
	rootCmd.AddCommand(testCmd)

	// Test case commands
	testCmd.AddCommand(testCaseCmd)
	testCaseCmd.AddCommand(testCaseListCmd)
	testCaseCmd.AddCommand(testCaseGetCmd)

	testCaseListCmd.Flags().StringVar(&testCaseFolderFilter, "folder", "", "filter by folder ID")

	// Test run commands
	testCmd.AddCommand(testRunCmd)
	testRunCmd.AddCommand(testRunMineCmd)
	testRunCmd.AddCommand(testRunListCmd)
	testRunCmd.AddCommand(testRunGetCmd)
	testRunCmd.AddCommand(testRunStartCmd)
	testRunCmd.AddCommand(testRunEndCmd)

	testRunListCmd.Flags().StringVar(&testRunSetFilter, "set", "", "filter by test set ID")
	testRunListCmd.Flags().StringVar(&testRunAssigneeFilter, "assignee", "", "filter by assignee ID")
	testRunStartCmd.Flags().IntVar(&testRunTemplateID, "template", 0, "start from run template ID")
	testRunStartCmd.Flags().StringVar(&testRunName, "name", "", "custom name for the run")

	// Test result command
	testCmd.AddCommand(testResultCmd)
	testResultCmd.Flags().StringVar(&testResultNotes, "notes", "", "notes about the result")
	testResultCmd.Flags().StringVar(&testResultActual, "actual", "", "actual result description")

	// Test set commands
	testCmd.AddCommand(testSetCmd)
	testSetCmd.AddCommand(testSetListCmd)
	testSetCmd.AddCommand(testSetGetCmd)
}
