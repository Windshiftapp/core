package services

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

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
	ext := strings.ToLower(filepath.Ext(filename))
	if ext != ".csv" && ext != ".tsv" {
		return AssetCSVUpload{}, &AssetValidationError{Msg: "only CSV and TSV files are accepted"}
	}

	uploadID := uuid.NewString()
	dir := filepath.Join(s.attachmentPath, "imports", uploadID)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return AssetCSVUpload{}, fmt.Errorf("create import directory: %w", err)
	}
	path := filepath.Join(dir, "upload.csv")
	// path is derived from the configured storage root and a server-generated UUID.
	//nolint:gosec // G304 cannot infer that the path has no user-controlled segment.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		_ = os.RemoveAll(dir)
		return AssetCSVUpload{}, fmt.Errorf("create import upload: %w", err)
	}
	written, copyErr := io.Copy(file, io.LimitReader(source, assetImportMaxBytes+1))
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil || written > assetImportMaxBytes {
		_ = os.RemoveAll(dir)
		if written > assetImportMaxBytes {
			return AssetCSVUpload{}, &AssetValidationError{Msg: "CSV upload exceeds 50 MiB"}
		}
		return AssetCSVUpload{}, errors.Join(copyErr, closeErr)
	}

	delimiter := parseAssetImportDelimiter(delimiterName)
	if delimiterName == "" {
		delimiter = detectAssetImportDelimiter(path)
	}
	headers, rows, total, err := parseAssetCSVPreview(path, delimiter, hasHeader, 5)
	if err != nil {
		_ = os.RemoveAll(dir)
		return AssetCSVUpload{}, &AssetValidationError{Msg: fmt.Sprintf("parse CSV: %v", err)}
	}
	if err := s.repo.CreateImportUpload(uploadID, setID, userID, time.Now().UTC()); err != nil {
		_ = os.RemoveAll(dir)
		return AssetCSVUpload{}, err
	}
	return AssetCSVUpload{
		UploadID: uploadID, Headers: headers, PreviewRows: rows, TotalRows: total,
		Delimiter: assetImportDelimiterName(delimiter), HeaderWarning: detectAssetHeaderMismatch(headers, rows, hasHeader),
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
	claimed, err := s.repo.ClaimImportUpload(jobID, setID, userID, path, string(config), time.Now().UTC())
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
	row, err := s.repo.GetImportJob(jobID, setID)
	if err != nil {
		return AssetImportJob{}, err
	}
	return assetImportJobFromRow(jobID, row), nil
}

func (s *AssetApplicationService) ListImportJobs(userID, setID int) ([]AssetImportJob, error) {
	if err := s.require(userID, setID, AssetPermissionKeyAdmin); err != nil {
		return nil, err
	}
	rows, err := s.repo.ListImportJobs(setID, 20)
	if err != nil {
		return nil, err
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
	headers, rows, _, err := parseAssetCSVPreview(s.assetImportUploadPath(uploadID), parseAssetImportDelimiter(delimiterName), hasHeader, 20)
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
	return s.repo.ReconcileExpiredAssetImports(time.Now().UTC())
}

func (s *AssetApplicationService) assetImportUploadPath(uploadID string) string {
	return filepath.Join(s.attachmentPath, "imports", uploadID, "upload.csv")
}

func (s *AssetApplicationService) executeAssetCSVImport(jobID string, setID int, input StartAssetImport, path string, userID int) {
	defer func() {
		if recovered := recover(); recovered != nil {
			_ = s.updateAssetImportStatus(jobID, "failed", "", nil, fmt.Sprintf("Import crashed: %v", recovered))
		}
	}()
	if err := s.updateAssetImportStatus(jobID, "running", "initializing", nil, ""); err != nil {
		return
	}
	// path was built from the configured storage root and a validated UUID.
	//nolint:gosec // G304 cannot follow validation across the asynchronous boundary.
	file, err := os.Open(path)
	if err != nil {
		_ = s.updateAssetImportStatus(jobID, "failed", "", nil, "Failed to open CSV file")
		return
	}
	defer func() { _ = file.Close() }()
	reader := newAssetCSVReader(file, parseAssetImportDelimiter(input.Delimiter))
	if input.HasHeader {
		if _, err := reader.Read(); err != nil {
			_ = s.updateAssetImportStatus(jobID, "failed", "", nil, "Failed to read CSV header")
			return
		}
	}
	defaultStatusID := input.DefaultStatusID
	if defaultStatusID == nil {
		defaultStatusID, _ = s.repo.GetDefaultStatus(setID)
	}
	progress := &AssetImportProgress{Phase: "importing"}
	errorsTruncated := false
	appendError := func(message string) {
		if len(progress.Errors) < assetImportErrorCap {
			progress.Errors = append(progress.Errors, message)
		} else {
			errorsTruncated = true
		}
	}
	for row := 1; ; row++ {
		record, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		progress.TotalRows = row
		if readErr != nil {
			progress.FailedCount++
			appendError(fmt.Sprintf("Row %d: %v", row, readErr))
		} else if err := s.importAssetCSVRow(record, setID, input, userID, defaultStatusID, jobID); err != nil {
			if errors.Is(err, repository.ErrAssetImportLeaseLost) {
				return
			}
			progress.FailedCount++
			appendError(fmt.Sprintf("Row %d: %v", row, err))
		} else {
			progress.ImportedCount++
		}
		if row%100 == 0 {
			if err := s.updateAssetImportProgress(jobID, progress); err != nil {
				return
			}
		}
	}
	if errorsTruncated {
		progress.Errors = append(progress.Errors, fmt.Sprintf("additional errors omitted; only the first %d are shown", assetImportErrorCap))
	}
	progress.Phase = "completed"
	if err := s.updateAssetImportStatus(jobID, "completed", "completed", progress, ""); err != nil {
		return
	}
	if err := os.RemoveAll(filepath.Dir(path)); err != nil {
		slog.Warn("failed to clean asset import upload", "path", path, "error", err)
	}
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

func (s *AssetApplicationService) updateAssetImportStatus(jobID, status, phase string, progress *AssetImportProgress, message string) error {
	encoded := "{}"
	if progress != nil {
		if data, err := json.Marshal(progress); err == nil {
			encoded = string(data)
		}
	}
	var err error
	switch status {
	case "running":
		err = s.repo.StartImportJobRunning(jobID, phase, encoded)
	case "completed", "failed":
		err = s.repo.FinishImportJob(jobID, status, phase, encoded, message)
	default:
		err = s.repo.UpdateImportJobStatus(jobID, status, phase, encoded)
	}
	if err != nil {
		slog.Error("failed to update asset import job", "job_id", jobID, "error", err)
	}
	return err
}

func (s *AssetApplicationService) updateAssetImportProgress(jobID string, progress *AssetImportProgress) error {
	data, err := json.Marshal(progress)
	if err == nil {
		err = s.repo.UpdateImportJobProgress(jobID, progress.Phase, string(data))
	}
	if err != nil {
		slog.Error("failed to update asset import progress", "job_id", jobID, "error", err)
	}
	return err
}

func assetImportJobFromRow(jobID string, row *repository.ImportJobRow) AssetImportJob {
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

func newAssetCSVReader(source io.Reader, delimiter rune) *csv.Reader {
	buffer := bufio.NewReader(source)
	if bytes, err := buffer.Peek(3); err == nil && len(bytes) == 3 && bytes[0] == 0xef && bytes[1] == 0xbb && bytes[2] == 0xbf {
		_, _ = buffer.Discard(3)
	}
	reader := csv.NewReader(buffer)
	reader.Comma = delimiter
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true
	return reader
}

func parseAssetImportDelimiter(value string) rune {
	switch value {
	case "tab", "\\t":
		return '\t'
	case "semicolon", ";":
		return ';'
	case "pipe", "|":
		return '|'
	default:
		if len(value) == 1 {
			return rune(value[0])
		}
		return ','
	}
}

func assetImportDelimiterName(delimiter rune) string {
	if delimiter == '\t' {
		return "tab"
	}
	return string(delimiter)
}

func detectAssetImportDelimiter(path string) rune {
	// Callers pass the server-owned upload path built from a validated UUID.
	//nolint:gosec // G304 cannot infer the caller's path validation.
	file, err := os.Open(path)
	if err != nil {
		return ','
	}
	defer func() { _ = file.Close() }()
	data := make([]byte, 8192)
	read, _ := file.Read(data)
	lines := strings.SplitN(string(data[:read]), "\n", 5)
	best, bestScore := ',', 0
	for _, delimiter := range []rune{',', '\t', ';', '|'} {
		counts := make([]int, 0, len(lines))
		for _, line := range lines {
			if strings.TrimSpace(line) != "" {
				counts = append(counts, strings.Count(line, string(delimiter)))
			}
		}
		if len(counts) < 2 || counts[0] == 0 {
			continue
		}
		score := counts[0]
		consistent := true
		for _, count := range counts[1:] {
			consistent = consistent && count == counts[0]
		}
		if consistent {
			score *= 2
		}
		if score > bestScore {
			best, bestScore = delimiter, score
		}
	}
	return best
}

func parseAssetCSVPreview(path string, delimiter rune, hasHeader bool, limit int) (headers []string, rows [][]string, total int, err error) {
	// Callers pass the server-owned upload path built from a validated UUID.
	//nolint:gosec // G304 cannot infer the caller's path validation.
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, 0, err
	}
	defer func() { _ = file.Close() }()
	reader := newAssetCSVReader(file, delimiter)
	if hasHeader {
		headers, err = reader.Read()
		if err != nil {
			return nil, nil, 0, fmt.Errorf("read header: %w", err)
		}
	}
	rows = make([][]string, 0, limit)
	for {
		record, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		total++
		if readErr != nil {
			continue
		}
		if len(rows) < limit {
			rows = append(rows, record)
		}
		if !hasHeader && headers == nil {
			headers = make([]string, len(record))
			for i := range headers {
				headers[i] = fmt.Sprintf("Column %d", i+1)
			}
		}
	}
	return headers, rows, total, nil
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

func detectAssetHeaderMismatch(headers []string, rows [][]string, hasHeader bool) string {
	if len(headers) == 0 || len(rows) == 0 {
		return ""
	}
	looksLikeData := func(values []string) bool {
		matches := 0
		for _, value := range values {
			value = strings.TrimSpace(value)
			if _, err := strconv.ParseFloat(value, 64); err == nil || strings.Contains(value, "@") {
				matches++
			}
		}
		return matches > len(values)/2
	}
	if hasHeader && looksLikeData(headers) {
		return "The first row looks like data rather than column headers."
	}
	if !hasHeader && !looksLikeData(rows[0]) {
		keywords := 0
		for _, value := range rows[0] {
			if isStandardAssetImportField(value) {
				keywords++
			}
		}
		if keywords > 0 {
			return "The first row looks like column headers."
		}
	}
	return ""
}

func (s *AssetApplicationService) requireImportUpload(userID, setID int, uploadID string) error {
	if s.attachmentPath == "" {
		return ErrAssetImportStorageDisabled
	}
	owned, err := s.repo.ImportUploadOwnedBy(uploadID, setID, userID)
	if err != nil {
		return err
	}
	if !owned {
		return ErrAssetImportUploadNotFound
	}
	return nil
}
