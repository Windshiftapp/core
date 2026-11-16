package services

import (
	"database/sql"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
)

const MaxTestCaseSteps = 100
const MaxTestCaseLabels = 100

func (s *TestCaseService) validateAuthoredContent(workspaceID int, format string, steps *[]TestStepCreateRequest, labels *[]int) error {
	if steps != nil {
		if len(*steps) > MaxTestCaseSteps {
			return &TestManagementValidationError{Msg: "at most 100 steps are allowed"}
		}
		if format == TestFormatBDD && len(*steps) > 0 {
			return &TestManagementValidationError{Msg: "BDD cases use gherkin, not steps"}
		}
		for _, step := range *steps {
			if strings.TrimSpace(sanitize.Comment.Sanitize(step.Action)) == "" {
				return &TestManagementValidationError{Msg: "step action is required"}
			}
		}
	}
	if labels != nil {
		if len(*labels) > MaxTestCaseLabels {
			return &TestManagementValidationError{Msg: "at most 100 labels are allowed"}
		}
		seen := map[int]bool{}
		for _, id := range *labels {
			if seen[id] {
				return &TestManagementValidationError{Msg: "label IDs must be unique"}
			}
			seen[id] = true
		}
		return validateAuthoredLabels(s.db, workspaceID, *labels, false)
	}
	return nil
}

type authoredLabelReader interface {
	Query(string, ...any) (*sql.Rows, error)
}

func validateAuthoredLabels(reader authoredLabelReader, workspaceID int, ids []int, lock bool) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids)+1)
	args = append(args, workspaceID)
	for _, id := range ids {
		args = append(args, id)
	}
	query := "SELECT id FROM test_labels WHERE workspace_id = ? AND id IN (" + placeholders + ") ORDER BY id"
	if lock {
		query += " FOR KEY SHARE"
	}
	rows, err := reader.Query(query, args...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	count := 0
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return err
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count != len(ids) {
		return repository.ErrNotFound
	}
	return nil
}

// Explicit arrays replace authored steps and labels; omitted arrays preserve them.
func (s *TestCaseService) writeAuthoredContent(tx database.Tx, workspaceID, caseID int, steps *[]TestStepCreateRequest, labels *[]int) error {
	if labels != nil {
		// SQLite serializes writers; PostgreSQL needs row locks through label insertion.
		if err := validateAuthoredLabels(tx, workspaceID, *labels, s.db.GetDriverName() == "postgres"); err != nil {
			return err
		}
	}
	if steps != nil {
		rows, err := tx.Query("SELECT id FROM test_steps WHERE test_case_id = ? ORDER BY step_number, id", caseID)
		if err != nil {
			return err
		}
		ids := []int{}
		for rows.Next() {
			var id int
			if err := rows.Scan(&id); err != nil {
				_ = rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return err
		}
		// Retain step identities and never cascade-delete recorded execution evidence.
		for _, id := range ids[min(len(*steps), len(ids)):] {
			var used bool
			if err := tx.QueryRow("SELECT EXISTS(SELECT 1 FROM test_step_results WHERE test_step_id = ?)", id).Scan(&used); err != nil {
				return err
			}
			if used {
				return &TestManagementValidationError{Msg: "cannot remove steps with execution results; archive this case and create a replacement"}
			}
			if err := s.repo.DeleteStep(tx, id, caseID); err != nil {
				return err
			}
		}
		now := time.Now()
		for i, input := range *steps {
			step := &models.TestStep{TestCaseID: caseID, StepNumber: i + 1,
				Action: sanitize.Comment.Sanitize(input.Action), Data: sanitize.Comment.Sanitize(input.Data), Expected: sanitize.Comment.Sanitize(input.Expected), CreatedAt: now, UpdatedAt: now}
			var err error
			if i < len(ids) {
				step.ID = ids[i]
				err = s.repo.UpdateStep(tx, step)
			} else {
				_, err = s.repo.CreateStep(tx, step)
			}
			if err != nil {
				return err
			}
		}
	}
	if labels != nil {
		if _, err := tx.Exec("DELETE FROM test_case_labels WHERE test_case_id = ?", caseID); err != nil {
			return err
		}
		for _, id := range *labels {
			if err := s.repo.AddLabelToTestCase(tx, caseID, id); err != nil {
				return err
			}
		}
	}
	return nil
}
