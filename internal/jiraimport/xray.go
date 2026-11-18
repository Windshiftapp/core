package jiraimport

import (
	"encoding/json"
	"errors"
	"fmt"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

type XrayTestCaseInput struct {
	JobID        string
	JiraID       string
	JiraKey      string
	TestCase     models.TestCase
	Steps        []models.TestStep
	Labels       []models.TestLabel
	XrayTestType string
}

func (s *Service) CreateXrayTestCase(input XrayTestCaseInput) (int, error) {
	return database.WithTxResult(s.db, func(tx database.Tx) (int, error) {
		maxSortOrder, err := s.testCases.GetMaxSortOrderTx(
			tx,
			input.TestCase.WorkspaceID,
			input.TestCase.FolderID,
		)
		if err != nil {
			return 0, err
		}
		input.TestCase.SortOrder = maxSortOrder + 1000
		testCaseID, err := s.testCases.Create(tx, &input.TestCase)
		if err != nil {
			return 0, err
		}
		for index := range input.Steps {
			input.Steps[index].TestCaseID = testCaseID
			if _, err := s.testCases.CreateStep(tx, &input.Steps[index]); err != nil {
				return 0, fmt.Errorf("create Xray Test step %d: %w", index+1, err)
			}
		}
		for index := range input.Labels {
			label := &input.Labels[index]
			existing, err := s.testCases.FindLabelByNameTx(
				tx,
				input.TestCase.WorkspaceID,
				label.Name,
			)
			switch {
			case err == nil:
				label.ID = existing.ID
			case errors.Is(err, repository.ErrNotFound):
				if _, err := s.testCases.CreateLabel(tx, label); err != nil {
					return 0, err
				}
			default:
				return 0, err
			}
			if err := s.testCases.AddLabelToTestCase(tx, testCaseID, label.ID); err != nil {
				return 0, err
			}
		}
		metadata, err := json.Marshal(map[string]any{
			"was_created":    true,
			"xray_test_type": input.XrayTestType,
		})
		if err != nil {
			return 0, err
		}
		if _, err := tx.Exec(`
			INSERT INTO jira_import_id_mappings
				(job_id, entity_type, jira_id, jira_key, windshift_id, metadata_json)
			VALUES (?, 'test_case', ?, ?, ?, ?)
			ON CONFLICT (job_id, entity_type, jira_id) DO UPDATE SET
				windshift_id = excluded.windshift_id,
				jira_key = excluded.jira_key,
				metadata_json = excluded.metadata_json
		`, input.JobID, input.JiraID, input.JiraKey, testCaseID, string(metadata)); err != nil {
			return 0, fmt.Errorf("record Xray Test mapping: %w", err)
		}
		return testCaseID, nil
	})
}

// TestCaseExists reports whether a previously imported Xray Test still exists in
// the target workspace.
func (s *Service) TestCaseExists(testCaseID, workspaceID int) (bool, error) {
	return s.testCases.Exists(testCaseID, workspaceID)
}

// UpdateXrayTestCase refreshes a test case previously imported from the same
// Jira Test so a re-import updates it in place instead of orphaning it.
func (s *Service) UpdateXrayTestCase(testCaseID int, input XrayTestCaseInput) (int, error) {
	return database.WithTxResult(s.db, func(tx database.Tx) (int, error) {
		existing, err := s.testCases.FindByID(testCaseID, input.TestCase.WorkspaceID)
		if err != nil {
			return 0, err
		}
		existing.Title = input.TestCase.Title
		existing.Priority = input.TestCase.Priority
		existing.Status = input.TestCase.Status
		existing.UpdatedAt = input.TestCase.UpdatedAt
		if err := s.testCases.Update(tx, existing); err != nil {
			return 0, err
		}
		currentSteps, err := s.testCases.FindSteps(testCaseID)
		if err != nil {
			return 0, err
		}
		for _, step := range currentSteps {
			if err := s.testCases.DeleteStep(tx, step.ID, testCaseID); err != nil {
				return 0, err
			}
		}
		for index := range input.Steps {
			input.Steps[index].TestCaseID = testCaseID
			if _, err := s.testCases.CreateStep(tx, &input.Steps[index]); err != nil {
				return 0, fmt.Errorf("update Xray Test step %d: %w", index+1, err)
			}
		}
		currentLabels, err := s.testCases.FindLabelsByTestCaseID(testCaseID)
		if err != nil {
			return 0, err
		}
		for _, label := range currentLabels {
			if err := s.testCases.RemoveLabelFromTestCase(tx, testCaseID, label.ID); err != nil {
				return 0, err
			}
		}
		for index := range input.Labels {
			label := &input.Labels[index]
			existingLabel, err := s.testCases.FindLabelByNameTx(tx, input.TestCase.WorkspaceID, label.Name)
			switch {
			case err == nil:
				label.ID = existingLabel.ID
			case errors.Is(err, repository.ErrNotFound):
				if _, err := s.testCases.CreateLabel(tx, label); err != nil {
					return 0, err
				}
			default:
				return 0, err
			}
			if err := s.testCases.AddLabelToTestCase(tx, testCaseID, label.ID); err != nil {
				return 0, err
			}
		}
		metadata, err := json.Marshal(map[string]any{
			"was_created":    false,
			"xray_test_type": input.XrayTestType,
		})
		if err != nil {
			return 0, err
		}
		if _, err := tx.Exec(`
			INSERT INTO jira_import_id_mappings
				(job_id, entity_type, jira_id, jira_key, windshift_id, metadata_json)
			VALUES (?, 'test_case', ?, ?, ?, ?)
			ON CONFLICT (job_id, entity_type, jira_id) DO UPDATE SET
				windshift_id = excluded.windshift_id,
				jira_key = excluded.jira_key,
				metadata_json = excluded.metadata_json
		`, input.JobID, input.JiraID, input.JiraKey, testCaseID, string(metadata)); err != nil {
			return 0, fmt.Errorf("record Xray Test mapping: %w", err)
		}
		return testCaseID, nil
	})
}
