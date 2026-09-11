package services

import (
	"errors"
	"fmt"
	"strings"

	"windshift/internal/gherkin"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// BDD test case orchestration. Authorization follows the same require()
// contract as the rest of the test-management surfaces: tests:read for
// consumption, tests:write for authoring and execution.

// GherkinDocumentResponse is the validate-feature payload: structured
// errors with authored positions plus the parsed document for previews.
type GherkinDocumentResponse struct {
	Valid    bool                 `json:"valid"`
	Errors   []gherkin.ParseError `json:"errors"`
	Document *gherkin.Document    `json:"document,omitempty"`
}

func newGherkinDocumentResponse(doc *gherkin.Document, errs *gherkin.ErrorList) *GherkinDocumentResponse {
	response := &GherkinDocumentResponse{Valid: errs.Empty(), Errors: []gherkin.ParseError{}, Document: doc}
	if !errs.Empty() {
		response.Errors = errs.Errors
	}
	return response
}

// GetCaseDetail returns a case with its BDD content attached when the case
// is BDD-format. The plain GetCase stays unchanged for step-based cases.
func (s *TestManagementApplicationService) GetCaseDetail(userID, workspaceID, id int) (*models.TestCase, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestView); err != nil {
		return nil, err
	}
	testCase, err := s.cases.GetByID(id, workspaceID)
	if err != nil {
		return nil, err
	}
	if testCase.Format == TestFormatBDD {
		bdd, err := s.cases.GetBDDContent(id)
		if err != nil && !errors.Is(err, repository.ErrNotFound) {
			return nil, err
		}
		testCase.BDD = bdd
	}
	return testCase, nil
}

// UpdateCaseGherkin validates and stores new Gherkin for a BDD case.
func (s *TestManagementApplicationService) UpdateCaseGherkin(userID, workspaceID, id int, actor AuditActor, source string) (*models.TestCase, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestManage); err != nil {
		return nil, err
	}
	updated, err := s.cases.UpdateBDDContent(id, workspaceID, source)
	if err != nil {
		return nil, err
	}
	emitServiceAudit(s.db, actor, logger.ActionTestCaseUpdate, logger.ResourceTestCase, &updated.ID, updated.Title, nil)
	return updated, nil
}

// ImportFeatureResult reports the outcome of a feature-file import.
type ImportFeatureResult struct {
	Cases []models.TestCase `json:"cases"`
}

// ImportFeature validates a complete .feature document and creates one case
// per scenario. Nothing is created when any part of the file is invalid.
func (s *TestManagementApplicationService) ImportFeature(userID, workspaceID int, actor AuditActor, folderID *int, content string) ([]models.TestCase, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestManage); err != nil {
		return nil, err
	}
	created, err := s.cases.ImportFeatureFile(workspaceID, folderID, content)
	if err != nil {
		return nil, err
	}
	for i := range created {
		emitServiceAudit(s.db, actor, logger.ActionTestCaseCreate, logger.ResourceTestCase, &created[i].ID, created[i].Title, nil)
	}
	return created, nil
}

// ExportFeature composes a .feature document for one BDD case and suggests
// a download filename derived from the case title.
func (s *TestManagementApplicationService) ExportFeature(userID, workspaceID, id int) (content, filename string, err error) {
	if err := s.require(userID, workspaceID, models.PermissionTestView); err != nil {
		return "", "", err
	}
	var exportErr error
	content, exportErr = s.cases.ExportFeatureFile(id, workspaceID)
	if exportErr != nil {
		return "", "", exportErr
	}
	testCase, getErr := s.cases.GetByID(id, workspaceID)
	if getErr != nil {
		return "", "", getErr
	}
	return content, featureFilename(testCase.Title, id), nil
}

// featureFilename builds a filesystem-safe .feature name from a case title.
func featureFilename(title string, id int) string {
	var builder strings.Builder
	for _, r := range strings.ToLower(title) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			builder.WriteRune(r)
		default:
			builder.WriteRune('-')
		}
	}
	name := strings.Trim(builder.String(), "-")
	if name == "" {
		name = "scenario"
	}
	return fmt.Sprintf("%s-%d.feature", name, id)
}

// ValidateFeature parses authored Gherkin and returns structured errors plus
// the parsed document for preview purposes. It persists nothing.
func (s *TestManagementApplicationService) ValidateFeature(userID, workspaceID int, content string) (*GherkinDocumentResponse, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestManage); err != nil {
		return nil, err
	}
	doc, errs := gherkin.Parse(content)
	return newGherkinDocumentResponse(doc, errs), nil
}

// ListRunExamples returns the per-example execution results of a run.
func (s *TestManagementApplicationService) ListRunExamples(userID, workspaceID, runID int) ([]models.TestExampleResult, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestView); err != nil {
		return nil, err
	}
	return s.runs.ListExampleResults(runID, workspaceID)
}

// UpdateRunExample records the result of one Examples row.
func (s *TestManagementApplicationService) UpdateRunExample(userID, workspaceID, runID, testCaseID, exampleIndex int, req TestExampleResultUpdateRequest) (*models.TestExampleResult, error) {
	if err := s.require(userID, workspaceID, models.PermissionTestManage); err != nil {
		return nil, err
	}
	return s.runs.UpdateExampleResult(workspaceID, runID, testCaseID, exampleIndex, req)
}

// UpdateRunExampleStep records one step result within an example execution.
func (s *TestManagementApplicationService) UpdateRunExampleStep(userID, workspaceID, runID, testCaseID, exampleIndex, stepNumber int, req TestExampleStepResultUpdateRequest) error {
	if err := s.require(userID, workspaceID, models.PermissionTestManage); err != nil {
		return err
	}
	return s.runs.UpdateExampleStepResult(workspaceID, runID, testCaseID, exampleIndex, stepNumber, req)
}
