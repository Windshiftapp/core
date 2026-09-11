package services

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"windshift/internal/database"
	"windshift/internal/gherkin"
	"windshift/internal/models"
)

// Test formats. Format is fixed at creation: this release has no
// conversion flow between the step-based and BDD representations.
const (
	TestFormatSteps = "steps"
	TestFormatBDD   = "bdd"
)

// maxGherkinRunes bounds authored Gherkin like other long-form text. The
// source is stored verbatim without HTML stripping: <param> placeholders are
// Gherkin syntax, not markup, and rendering escapes content downstream.
const maxGherkinRunes = 256 * 1024

// TestBDDValidationError carries structured Gherkin parse errors with their
// authored positions.
type TestBDDValidationError struct {
	Errors []gherkin.ParseError
}

func (e *TestBDDValidationError) Error() string {
	lines := make([]string, len(e.Errors))
	for i, err := range e.Errors {
		lines[i] = err.Error()
	}
	return strings.Join(lines, "\n")
}

// BuildScenarioSpec parses authored Gherkin and returns the storable spec
// for the single scenario a BDD test case represents.
func BuildScenarioSpec(source string) (*gherkin.ScenarioSpec, *gherkin.ErrorList) {
	doc, errs := gherkin.Parse(source)
	if !errs.Empty() {
		return nil, errs
	}
	if len(doc.Scenarios) != 1 {
		return nil, &gherkin.ErrorList{Errors: []gherkin.ParseError{{
			Line: 1, Column: 1,
			Message: fmt.Sprintf("a BDD test case must contain exactly one Scenario or Scenario Outline, found %d; use feature-file import for multi-scenario documents", len(doc.Scenarios)),
		}}}
	}
	scenario := doc.Scenarios[0]
	if scenario.Name == "" {
		return nil, &gherkin.ErrorList{Errors: []gherkin.ParseError{{
			Line: scenario.Line, Column: 1,
			Message: "the scenario needs a name: Windshift uses it as the test case title",
		}}}
	}
	spec := doc.ScenarioSpecFrom(&scenario)
	return &spec, nil
}

// capGherkin enforces the length bound without altering content.
func capGherkin(source string) (string, error) {
	if utf8.RuneCountInString(source) > maxGherkinRunes {
		return "", &TestManagementValidationError{Msg: "Gherkin source is too long"}
	}
	return source, nil
}

// GetBDDContent returns the authored Gherkin of a BDD-format case.
func (s *TestCaseService) GetBDDContent(testCaseID int) (*models.TestCaseBDD, error) {
	return s.bddRepo.FindBDD(testCaseID)
}

// UpdateBDDContent validates and stores new Gherkin for a BDD case inside
// its workspace, syncing the case title with the scenario name — the
// authored source stays authoritative.
func (s *TestCaseService) UpdateBDDContent(id, workspaceID int, source string) (*models.TestCase, error) {
	existing, err := s.repo.FindByID(id, workspaceID)
	if err != nil {
		return nil, err
	}
	if existing.Format != TestFormatBDD {
		return nil, &TestManagementValidationError{Msg: "gherkin can only be updated on BDD-format test cases"}
	}
	source, err = capGherkin(source)
	if err != nil {
		return nil, err
	}
	spec, errs := BuildScenarioSpec(source)
	if !errs.Empty() {
		return nil, &TestBDDValidationError{Errors: errs.Errors}
	}

	encoded, err := encodeScenarioSpec(spec)
	if err != nil {
		return nil, err
	}
	keyword := spec.ScenarioKeyword
	if keyword == "" {
		keyword = "Scenario"
	}

	return database.WithTxResult(s.db, func(tx database.Tx) (*models.TestCase, error) {
		bdd := &models.TestCaseBDD{
			TestCaseID:      id,
			Gherkin:         source,
			FeatureName:     spec.FeatureName,
			ScenarioKeyword: keyword,
			Spec:            encoded,
		}
		if err := s.bddRepo.Upsert(tx, bdd); err != nil {
			return nil, err
		}
		existing.Title = spec.ScenarioName
		existing.UpdatedAt = time.Now()
		if err := s.repo.Update(tx, existing); err != nil {
			return nil, err
		}
		return existing, nil
	})
}

func encodeScenarioSpec(spec *gherkin.ScenarioSpec) (string, error) {
	encoded, err := json.Marshal(spec)
	if err != nil {
		return "", fmt.Errorf("failed to encode BDD spec: %w", err)
	}
	return string(encoded), nil
}

// ImportFeatureFile validates a complete .feature document and creates one
// test case per Scenario or Scenario Outline in a single transaction. Any
// parse error creates nothing.
func (s *TestCaseService) ImportFeatureFile(workspaceID int, folderID *int, source string) ([]models.TestCase, error) {
	source, err := capGherkin(source)
	if err != nil {
		return nil, err
	}
	doc, errs := gherkin.Parse(source)
	if !errs.Empty() {
		return nil, &TestBDDValidationError{Errors: errs.Errors}
	}
	for i := range doc.Scenarios {
		if doc.Scenarios[i].Name == "" {
			return nil, &TestBDDValidationError{Errors: []gherkin.ParseError{{
				Line: doc.Scenarios[i].Line, Column: 1,
				Message: "every scenario needs a name: Windshift uses it as the test case title",
			}}}
		}
	}
	if err := s.validateFolderInWorkspace(workspaceID, folderID); err != nil {
		return nil, err
	}

	return database.WithTxResult(s.db, func(tx database.Tx) ([]models.TestCase, error) {
		maxSortOrder, err := s.repo.GetMaxSortOrderTx(tx, workspaceID, folderID)
		if err != nil {
			return nil, fmt.Errorf("failed to get sort order: %w", err)
		}
		created := make([]models.TestCase, 0, len(doc.Scenarios))
		for i := range doc.Scenarios {
			scenario := doc.Scenarios[i]
			spec := doc.ScenarioSpecFrom(&scenario)
			encoded, err := encodeScenarioSpec(&spec)
			if err != nil {
				return nil, err
			}
			keyword := spec.ScenarioKeyword
			if keyword == "" {
				keyword = "Scenario"
			}
			now := time.Now()
			tc := &models.TestCase{
				WorkspaceID:   workspaceID,
				FolderID:      folderID,
				Title:         scenario.Name,
				Format:        TestFormatBDD,
				Preconditions: featurePreconditions(spec.FeatureName),
				Priority:      "medium",
				Status:        "active",
				SortOrder:     maxSortOrder + (i+1)*1000,
				CreatedAt:     now,
				UpdatedAt:     now,
			}
			id, err := s.repo.Create(tx, tc)
			if err != nil {
				return nil, err
			}
			tc.ID = id
			bdd := &models.TestCaseBDD{
				TestCaseID:      id,
				Gherkin:         source,
				FeatureName:     spec.FeatureName,
				ScenarioKeyword: keyword,
				Spec:            encoded,
			}
			if err := s.bddRepo.Upsert(tx, bdd); err != nil {
				return nil, err
			}
			created = append(created, *tc)
		}
		return created, nil
	})
}

// featurePreconditions records the originating feature for imported cases.
func featurePreconditions(featureName string) string {
	if featureName == "" {
		return ""
	}
	return "Imported from feature: " + featureName
}

// ExportFeatureFile composes a complete .feature document for one BDD case.
func (s *TestCaseService) ExportFeatureFile(id, workspaceID int) (string, error) {
	tc, err := s.repo.FindByID(id, workspaceID)
	if err != nil {
		return "", err
	}
	if tc.Format != TestFormatBDD {
		return "", &TestManagementValidationError{Msg: "only BDD-format test cases can be exported as feature files"}
	}
	bdd, err := s.bddRepo.FindBDD(id)
	if err != nil {
		return "", err
	}
	var spec gherkin.ScenarioSpec
	if err := json.Unmarshal([]byte(bdd.Spec), &spec); err != nil {
		return "", fmt.Errorf("stored BDD spec is unreadable: %w", err)
	}
	if spec.ScenarioKeyword == "" {
		spec.ScenarioKeyword = "Scenario"
	}
	return gherkin.RenderSpec(spec), nil
}
