package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"windshift/internal/csvimport"
	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
)

const (
	assetImportErrorCap = 100
	assetImportMaxBytes = 50 << 20
)

var (
	ErrAssetImportStorageDisabled = errors.New("asset import storage is not configured")
	ErrAssetImportUploadNotFound  = errors.New("asset import upload was not found")
)

type AssetCSVUpload struct {
	UploadID      string     `json:"upload_id"`
	Headers       []string   `json:"headers"`
	PreviewRows   [][]string `json:"preview_rows"`
	TotalRows     int        `json:"total_rows"`
	Delimiter     string     `json:"delimiter"`
	HeaderWarning string     `json:"header_warning,omitempty"`
}

type AssetImportMappings struct {
	Title        int            `json:"title"`
	Description  int            `json:"description"`
	AssetTag     int            `json:"asset_tag"`
	CategoryID   int            `json:"category_id"`
	StatusID     int            `json:"status_id"`
	CustomFields map[string]int `json:"custom_fields,omitempty"`
}

type StartAssetImport struct {
	UploadID          string              `json:"upload_id"`
	AssetTypeID       int                 `json:"asset_type_id"`
	DefaultCategoryID *int                `json:"default_category_id,omitempty"`
	DefaultStatusID   *int                `json:"default_status_id,omitempty"`
	Mappings          AssetImportMappings `json:"mappings"`
	CategoryMap       map[string]int      `json:"category_map,omitempty"`
	StatusMap         map[string]int      `json:"status_map,omitempty"`
	HasHeader         bool                `json:"has_header"`
	Delimiter         string              `json:"delimiter,omitempty"`
}

type AssetImportProgress struct {
	Phase         string   `json:"phase"`
	TotalRows     int      `json:"total_rows"`
	ImportedCount int      `json:"imported_count"`
	FailedCount   int      `json:"failed_count"`
	Errors        []string `json:"errors,omitempty"`
}

type AssetImportJob struct {
	JobID        string               `json:"job_id"`
	Status       string               `json:"status"`
	Phase        string               `json:"phase,omitempty"`
	Progress     *AssetImportProgress `json:"progress,omitempty"`
	ErrorMessage string               `json:"error_message,omitempty"`
	CreatedAt    *time.Time           `json:"created_at,omitempty"`
	StartedAt    *time.Time           `json:"started_at,omitempty"`
	CompletedAt  *time.Time           `json:"completed_at,omitempty"`
}

type AssetImportFieldSuggestion struct {
	ColumnIndex   int      `json:"column_index"`
	HeaderName    string   `json:"header_name"`
	SuggestedName string   `json:"suggested_name"`
	SuggestedType string   `json:"suggested_type"`
	Options       []string `json:"options,omitempty"`
	SampleValues  []string `json:"sample_values"`
	IsStandard    bool     `json:"is_standard"`
}

type AssetImportFieldSuggestions struct {
	SuggestedFields []AssetImportFieldSuggestion `json:"suggested_fields"`
}

type AssetImportTypeField struct {
	Name         string   `json:"name"`
	FieldType    string   `json:"field_type"`
	Options      []string `json:"options,omitempty"`
	IsRequired   bool     `json:"is_required"`
	DisplayOrder int      `json:"display_order"`
}

type AssetImportTypeInput struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Icon        string                 `json:"icon"`
	Color       string                 `json:"color"`
	Fields      []AssetImportTypeField `json:"fields"`
}

type AssetImportTypeResult struct {
	AssetType models.AssetType        `json:"asset_type"`
	Fields    []models.AssetTypeField `json:"fields"`
}

func (s *AssetApplicationService) WithImportStorage(path string) *AssetApplicationService {
	s.attachmentPath = path
	return s
}

func (s *AssetApplicationService) UploadCSV(userID, setID int, filename string, hasHeader bool, delimiterName string, source io.Reader) (AssetCSVUpload, error) {
	if err := s.require(userID, setID, AssetPermissionKeyAdmin); err != nil {
		return AssetCSVUpload{}, err
	}
	if s.attachmentPath == "" {
		return AssetCSVUpload{}, ErrAssetImportStorageDisabled
	}
	staged, _, err := csvimport.StageUpload(s.imports, s.attachmentPath, csvimport.KindAsset, setID, userID, filename, hasHeader, delimiterName, source, assetImportMaxBytes)
	if err != nil {
		return AssetCSVUpload{}, &AssetValidationError{Msg: err.Error()}
	}
	return AssetCSVUpload{
		UploadID: staged.UploadID, Headers: staged.Headers, PreviewRows: staged.PreviewRows,
		TotalRows: staged.TotalRows, Delimiter: staged.Delimiter, HeaderWarning: staged.HeaderWarning,
	}, nil
}

func (s *AssetApplicationService) StartImport(userID, setID int, actor AuditActor, input StartAssetImport) (AssetImportJob, error) {
	if err := s.require(userID, setID, AssetPermissionKeyAdmin); err != nil {
		return AssetImportJob{}, err
	}
	if input.UploadID == "" || input.AssetTypeID == 0 {
		return AssetImportJob{}, &AssetValidationError{Msg: "upload_id and asset_type_id are required"}
	}
	if _, err := uuid.Parse(input.UploadID); err != nil {
		return AssetImportJob{}, &AssetValidationError{Msg: "upload_id is invalid"}
	}
	if err := s.requireImportUpload(userID, setID, input.UploadID); err != nil {
		return AssetImportJob{}, err
	}
	belongs, err := s.repo.AssetTypeBelongsToSet(input.AssetTypeID, setID)
	if err != nil {
		return AssetImportJob{}, err
	}
	if !belongs {
		return AssetImportJob{}, &AssetValidationError{Msg: "asset type does not belong to this set"}
	}
	if input.DefaultCategoryID != nil {
		belongs, err = s.repo.CategoryBelongsToSet(*input.DefaultCategoryID, setID)
		if err != nil || !belongs {
			return AssetImportJob{}, &AssetValidationError{Msg: "default category does not belong to this set"}
		}
	}
	if input.DefaultStatusID != nil {
		belongs, err = s.repo.StatusBelongsToSet(*input.DefaultStatusID, setID)
		if err != nil || !belongs {
			return AssetImportJob{}, &AssetValidationError{Msg: "default status does not belong to this set"}
		}
	}
	for name, id := range input.CategoryMap {
		belongs, err = s.repo.CategoryBelongsToSet(id, setID)
		if err != nil || !belongs {
			return AssetImportJob{}, &AssetValidationError{Msg: "category " + name + " does not belong to this set"}
		}
	}
	for name, id := range input.StatusMap {
		belongs, err = s.repo.StatusBelongsToSet(id, setID)
		if err != nil || !belongs {
			return AssetImportJob{}, &AssetValidationError{Msg: "status " + name + " does not belong to this set"}
		}
	}
	path := s.assetImportUploadPath(input.UploadID)
	config, err := json.Marshal(input)
	if err != nil {
		return AssetImportJob{}, err
	}
	jobID := input.UploadID
	claimed, err := s.imports.ClaimUpload(csvimport.KindAsset, setID, userID, jobID, path, string(config), time.Now().UTC())
	if err != nil {
		return AssetImportJob{}, err
	}
	if !claimed {
		return s.GetImportJob(userID, setID, jobID)
	}
	emitServiceAudit(s.db, actor, "asset_import", "asset_import", nil, jobID, map[string]any{"set_id": setID, "asset_type_id": input.AssetTypeID})
	job, err := s.GetImportJob(userID, setID, jobID)
	if err != nil {
		return AssetImportJob{}, err
	}
	go s.executeAssetCSVImport(jobID, setID, input, path, userID)
	return job, nil
}

func (s *AssetApplicationService) GetImportJob(userID, setID int, jobID string) (AssetImportJob, error) {
	if err := s.require(userID, setID, AssetPermissionKeyAdmin); err != nil {
		return AssetImportJob{}, err
	}
	return s.getImportJob(setID, jobID)
}

func (s *AssetApplicationService) getImportJob(setID int, jobID string) (AssetImportJob, error) {
	row, err := s.imports.GetJob(csvimport.KindAsset, setID, jobID)
	if errors.Is(err, csvimport.ErrUploadNotFound) {
		row, err = nil, repository.ErrNotFound
	}
	if err != nil {
		return AssetImportJob{}, err
	}
	if (row.Status.String == "queued" || row.Status.String == "running") &&
		(!row.LeaseExpiresAt.Valid || row.LeaseExpiresAt.Int64 <= time.Now().UTC().Unix()) {
		if _, err := s.ReconcileInterruptedImports(); err != nil {
			return AssetImportJob{}, err
		}
		row, err = s.imports.GetJob(csvimport.KindAsset, setID, jobID)
		if errors.Is(err, csvimport.ErrUploadNotFound) {
			row, err = nil, repository.ErrNotFound
		}
		if err != nil {
			return AssetImportJob{}, err
		}
	}
	return assetImportJobFromRow(jobID, row), nil
}

func (s *AssetApplicationService) ListImportJobs(userID, setID int) ([]AssetImportJob, error) {
	if err := s.require(userID, setID, AssetPermissionKeyAdmin); err != nil {
		return nil, err
	}
	rows, err := s.imports.ListJobs(csvimport.KindAsset, setID, 20)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Unix()
	for i := range rows {
		if (rows[i].Status.String == "queued" || rows[i].Status.String == "running") &&
			(!rows[i].LeaseExpiresAt.Valid || rows[i].LeaseExpiresAt.Int64 <= now) {
			if _, err := s.ReconcileInterruptedImports(); err != nil {
				return nil, err
			}
			rows, err = s.imports.ListJobs(csvimport.KindAsset, setID, 20)
			if err != nil {
				return nil, err
			}
			break
		}
	}
	jobs := make([]AssetImportJob, len(rows))
	for i := range rows {
		jobs[i] = assetImportJobFromRow(rows[i].JobID, &rows[i])
	}
	return jobs, nil
}

func (s *AssetApplicationService) SuggestImportFields(userID, setID int, uploadID string, hasHeader bool, delimiterName string) (AssetImportFieldSuggestions, error) {
	if err := s.require(userID, setID, AssetPermissionKeyAdmin); err != nil {
		return AssetImportFieldSuggestions{}, err
	}
	if _, err := uuid.Parse(uploadID); err != nil {
		return AssetImportFieldSuggestions{}, &AssetValidationError{Msg: "upload_id is invalid"}
	}
	if err := s.requireImportUpload(userID, setID, uploadID); err != nil {
		return AssetImportFieldSuggestions{}, err
	}
	headers, rows, _, err := csvimport.ParsePreview(csvimport.UploadPath(s.attachmentPath, uploadID), csvimport.ParseDelimiter(delimiterName), hasHeader, 20)
	if os.IsNotExist(err) {
		return AssetImportFieldSuggestions{}, ErrAssetImportUploadNotFound
	}
	if err != nil {
		return AssetImportFieldSuggestions{}, &AssetValidationError{Msg: fmt.Sprintf("parse CSV: %v", err)}
	}
	suggestions := make([]AssetImportFieldSuggestion, 0, len(headers))
	for column, header := range headers {
		samples, seen := make([]string, 0), make(map[string]bool)
		for _, row := range rows {
			if column < len(row) {
				value := strings.TrimSpace(row[column])
				if value != "" && !seen[value] {
					seen[value] = true
					samples = append(samples, value)
				}
			}
		}
		fieldType, options := InferAssetImportFieldType(samples)
		display := samples
		if len(display) > 5 {
			display = display[:5]
		}
		suggestions = append(suggestions, AssetImportFieldSuggestion{ColumnIndex: column, HeaderName: header, SuggestedName: cleanAssetImportHeader(header), SuggestedType: fieldType, Options: options, SampleValues: display, IsStandard: isStandardAssetImportField(header)})
	}
	return AssetImportFieldSuggestions{SuggestedFields: suggestions}, nil
}

func (s *AssetApplicationService) CreateTypeFromImport(userID, setID int, actor AuditActor, input AssetImportTypeInput) (AssetImportTypeResult, error) {
	if err := s.require(userID, setID, AssetPermissionKeyAdmin); err != nil {
		return AssetImportTypeResult{}, err
	}
	input.Name = sanitize.PlainTextField.Sanitize(input.Name)
	input.Description = sanitize.RichText.Sanitize(input.Description)
	if input.Name == "" {
		return AssetImportTypeResult{}, &AssetValidationError{Msg: "name is required"}
	}
	if input.Icon == "" {
		input.Icon = "Box"
	}
	if input.Color == "" {
		input.Color = "#6b7280"
	}
	allowed := map[string]bool{"text": true, "textarea": true, "number": true, "date": true, "select": true, models.CustomFieldTypeBoolean: true, models.CustomFieldTypeCheckbox: true}
	fields := make([]repository.ImportTypeFieldInput, len(input.Fields))
	for i := range input.Fields {
		field := &input.Fields[i]
		field.Name = sanitize.PlainTextField.Sanitize(field.Name)
		field.FieldType = models.CanonicalCustomFieldType(field.FieldType)
		if field.Name == "" || !allowed[field.FieldType] {
			return AssetImportTypeResult{}, &AssetValidationError{Msg: "every field needs a name and supported field_type"}
		}
		var options *string
		if field.FieldType == "select" && len(field.Options) > 0 {
			encoded, err := json.Marshal(field.Options)
			if err != nil {
				return AssetImportTypeResult{}, err
			}
			value := string(encoded)
			options = &value
		}
		fields[i] = repository.ImportTypeFieldInput{Name: field.Name, FieldType: field.FieldType, OptionsJSON: options, IsRequired: field.IsRequired, DisplayOrder: field.DisplayOrder}
	}
	typeID, createdAt, results, err := s.repo.CreateAssetTypeWithFields(setID, models.AssetType{Name: input.Name, Description: input.Description, Icon: input.Icon, Color: input.Color}, fields)
	if err != nil {
		return AssetImportTypeResult{}, err
	}
	createdFields := make([]models.AssetTypeField, len(results))
	for i, result := range results {
		field := input.Fields[i]
		createdFields[i] = models.AssetTypeField{ID: result.AssetTypeFieldID, AssetTypeID: typeID, CustomFieldID: result.CustomFieldID, IsRequired: field.IsRequired, DisplayOrder: field.DisplayOrder, CreatedAt: createdAt, FieldName: field.Name, FieldType: field.FieldType}
		if fields[i].OptionsJSON != nil {
			createdFields[i].Options = *fields[i].OptionsJSON
		}
	}
	assetType := models.AssetType{ID: typeID, SetID: setID, Name: input.Name, Description: input.Description, Icon: input.Icon, Color: input.Color, IsActive: true, CreatedAt: createdAt, UpdatedAt: createdAt, Fields: createdFields}
	emitServiceAudit(s.db, actor, logger.ActionAssetTypeCreate, logger.ResourceAssetType, &typeID, input.Name, map[string]any{"source": "import_wizard", "field_count": len(createdFields)})
	return AssetImportTypeResult{AssetType: assetType, Fields: createdFields}, nil
}

func (s *AssetApplicationService) ReconcileInterruptedImports() (int, error) {
	return s.imports.ReconcileExpired(csvimport.KindAsset, time.Now().UTC(), func(tx database.Tx, jobID string) error {
		return s.repo.DeleteAssetsFromImportJobInTx(tx, jobID)
	})
}

func (s *AssetApplicationService) assetImportUploadPath(uploadID string) string {
	return filepath.Join(s.attachmentPath, "imports", uploadID, "upload.csv")
}

func (s *AssetApplicationService) executeAssetCSVImport(jobID string, setID int, input StartAssetImport, path string, userID int) {
	// Resolve the set's default status up front so rows without a mapping land
	// somewhere sensible; the old per-run resolution moved into this wrapper.
	if input.DefaultStatusID == nil {
		if defaultStatusID, err := s.repo.GetDefaultStatus(setID); err == nil {
			input.DefaultStatusID = defaultStatusID
		}
	}
	csvimport.RunRows(s.imports, jobID, path, csvimport.ParseDelimiter(input.Delimiter), input.HasHeader, func(rowNumber int, record []string) error {
		err := s.importAssetCSVRow(record, setID, input, userID, input.DefaultStatusID, jobID)
		if err == nil {
			return nil
		}
		// A lost lease must stop the runner silently; reconciliation owns the
		// job. Everything else is a per-row failure.
		if errors.Is(err, repository.ErrAssetImportLeaseLost) {
			return csvimport.ErrLeaseLost
		}
		return err
	})
}

func (s *AssetApplicationService) importAssetCSVRow(record []string, setID int, input StartAssetImport, userID int, defaultStatusID *int, jobID string) error {
	column := func(index int) string {
		if index < 0 || index >= len(record) {
			return ""
		}
		return strings.TrimSpace(record[index])
	}
	title := sanitize.PlainTextField.Sanitize(column(input.Mappings.Title))
	if title == "" {
		return errors.New("title is empty")
	}
	description, tag := "", ""
	if input.Mappings.Description >= 0 {
		description = sanitize.RichText.Sanitize(column(input.Mappings.Description))
	}
	if input.Mappings.AssetTag >= 0 {
		tag = sanitize.PlainTextField.Sanitize(column(input.Mappings.AssetTag))
	}
	categoryID, statusID := input.DefaultCategoryID, input.DefaultStatusID
	if value := column(input.Mappings.CategoryID); value != "" {
		if id, ok := input.CategoryMap[value]; ok {
			categoryID = &id
		}
	}
	if value := column(input.Mappings.StatusID); value != "" {
		if id, ok := input.StatusMap[value]; ok {
			statusID = &id
		}
	}
	if statusID == nil {
		statusID = defaultStatusID
	}
	values := make(map[string]any)
	for field, index := range input.Mappings.CustomFields {
		if value := column(index); value != "" {
			values[field] = s.resolveAssetImportFieldValue(field, sanitize.PlainTextField.Sanitize(value))
		}
	}
	coerced, err := s.assets.CoerceAndValidateCustomFieldValues(input.AssetTypeID, values)
	if err != nil {
		return err
	}
	var encoded *string
	if len(coerced) > 0 {
		data, err := json.Marshal(coerced)
		if err != nil {
			return err
		}
		value := string(data)
		encoded = &value
	}
	_, err = s.assets.InsertImportedAsset(repository.ImportAssetRowInput{SetID: setID, AssetTypeID: input.AssetTypeID, CategoryID: categoryID, StatusID: statusID, Title: title, Description: description, AssetTag: tag, CustomFieldValuesJSON: encoded, ImportJobID: jobID, CreatedBy: userID, CreatedAt: time.Now()})
	return err
}

func (s *AssetApplicationService) resolveAssetImportFieldValue(fieldKey, text string) any {
	fieldID, err := strconv.Atoi(fieldKey)
	if err != nil {
		return text
	}
	fieldType, optionsJSON, err := s.repo.GetCustomFieldTypeAndOptions(fieldID)
	if err != nil || !optionsJSON.Valid || fieldType != "select" && fieldType != "multiselect" {
		return text
	}
	options, err := models.ParseSelectOptions(optionsJSON.String)
	if err != nil {
		return text
	}
	ids := make(map[string]int, len(options.Items))
	for _, option := range options.Items {
		ids[option.Label] = option.ID
	}
	if fieldType == "select" {
		if id, ok := ids[text]; ok {
			return id
		}
		return text
	}
	result := make([]int, 0)
	for _, value := range strings.Split(text, ",") {
		if id, ok := ids[strings.TrimSpace(value)]; ok {
			result = append(result, id)
		}
	}
	if len(result) > 0 {
		return result
	}
	return text
}

func assetImportJobFromRow(jobID string, row *csvimport.JobRow) AssetImportJob {
	job := AssetImportJob{JobID: jobID, Status: row.Status.String, Phase: row.Phase.String}
	if row.ProgressJSON.Valid && row.ProgressJSON.String != "" {
		var progress AssetImportProgress
		if json.Unmarshal([]byte(row.ProgressJSON.String), &progress) == nil {
			job.Progress = &progress
		}
	}
	if row.ErrorMessage.Valid {
		job.ErrorMessage = row.ErrorMessage.String
	}
	if row.CreatedAt.Valid {
		value := row.CreatedAt.Time
		job.CreatedAt = &value
	}
	if row.StartedAt.Valid {
		value := row.StartedAt.Time
		job.StartedAt = &value
	}
	if row.CompletedAt.Valid {
		value := row.CompletedAt.Time
		job.CompletedAt = &value
	}
	return job
}

func InferAssetImportFieldType(values []string) (fieldType string, options []string) {
	if len(values) == 0 {
		return "text", nil
	}
	numberPattern := regexp.MustCompile(`^-?\d+([.,]\d+)?$`)
	datePattern := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$|^\d{1,2}/\d{1,2}/\d{2,4}$|^\d{1,2}\.\d{1,2}\.\d{2,4}$`)
	allNumbers, allDates, allBooleans, long := true, true, true, false
	unique := make(map[string]bool)
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		unique[value] = true
		allNumbers = allNumbers && numberPattern.MatchString(value)
		allDates = allDates && datePattern.MatchString(value)
		normalized := strings.ToLower(value)
		allBooleans = allBooleans && (normalized == "true" || normalized == "false")
		long = long || len(value) > 200
	}
	if len(unique) == 0 {
		return "text", nil
	}
	switch {
	case allBooleans:
		return models.CustomFieldTypeBoolean, nil
	case allNumbers:
		return "number", nil
	case allDates:
		return "date", nil
	case len(unique) <= 10 && len(values) >= 2:
		options := make([]string, 0, len(unique))
		for value := range unique {
			options = append(options, value)
		}
		sort.Strings(options)
		return "select", options
	case long:
		return "textarea", nil
	default:
		return "text", nil
	}
}

func isStandardAssetImportField(header string) bool {
	value := strings.ToLower(strings.TrimSpace(header))
	return map[string]bool{"title": true, "name": true, "asset name": true, "asset_name": true, "description": true, "desc": true, "details": true, "tag": true, "asset tag": true, "asset_tag": true, "serial": true, "serial number": true, "serial_number": true, "category": true, "status": true, "state": true, "id": true, "asset id": true, "asset_id": true}[value]
}

func cleanAssetImportHeader(header string) string {
	words := strings.Fields(strings.NewReplacer("_", " ", "-", " ").Replace(strings.TrimSpace(header)))
	for i, word := range words {
		runes := []rune(word)
		if len(runes) > 0 {
			runes[0] = unicode.ToUpper(runes[0])
			words[i] = string(runes)
		}
	}
	return strings.Join(words, " ")
}

func (s *AssetApplicationService) requireImportUpload(userID, setID int, uploadID string) error {
	if s.attachmentPath == "" {
		return ErrAssetImportStorageDisabled
	}
	owned, err := s.imports.UploadOwnedBy(csvimport.KindAsset, setID, userID, uploadID)
	if err != nil {
		return err
	}
	if !owned {
		return ErrAssetImportUploadNotFound
	}
	return nil
}
