package handlers

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/utils"

	securejoin "github.com/cyphar/filepath-securejoin"
	"github.com/google/uuid"
)

// --- Request/Response types ---

// CSVUploadResponse is returned after uploading a CSV file for preview.
type CSVUploadResponse struct {
	UploadID      string     `json:"upload_id"`
	Headers       []string   `json:"headers"`
	PreviewRows   [][]string `json:"preview_rows"`
	TotalRows     int        `json:"total_rows"`
	Delimiter     string     `json:"delimiter"`
	HeaderWarning string     `json:"header_warning,omitempty"`
}

// StartAssetImportRequest is the request body for starting a CSV import job.
type StartAssetImportRequest struct {
	UploadID    string              `json:"upload_id"`
	AssetTypeID int                 `json:"asset_type_id"`
	Mappings    AssetImportMappings `json:"mappings"`
	CategoryMap map[string]int      `json:"category_map,omitempty"`
	StatusMap   map[string]int      `json:"status_map,omitempty"`
	HasHeader   bool                `json:"has_header"`
	Delimiter   string              `json:"delimiter,omitempty"`
}

// AssetImportMappings maps CSV columns to asset fields.
type AssetImportMappings struct {
	Title        int            `json:"title"`
	Description  int            `json:"description"`
	AssetTag     int            `json:"asset_tag"`
	CategoryID   int            `json:"category_id"`
	StatusID     int            `json:"status_id"`
	CustomFields map[string]int `json:"custom_fields,omitempty"`
}

// AssetImportProgress tracks import job progress.
type AssetImportProgress struct {
	Phase         string   `json:"phase"`
	TotalRows     int      `json:"total_rows"`
	ImportedCount int      `json:"imported_count"`
	FailedCount   int      `json:"failed_count"`
	Errors        []string `json:"errors,omitempty"`
}

// AssetImportJobResponse is the API response for job status.
type AssetImportJobResponse struct {
	JobID        string               `json:"job_id"`
	Status       string               `json:"status"`
	Phase        string               `json:"phase,omitempty"`
	Progress     *AssetImportProgress `json:"progress,omitempty"`
	ErrorMessage string               `json:"error_message,omitempty"`
	CreatedAt    *time.Time           `json:"created_at,omitempty"`
	StartedAt    *time.Time           `json:"started_at,omitempty"`
	CompletedAt  *time.Time           `json:"completed_at,omitempty"`
}

// --- Upload Handler ---

// UploadCSV handles POST /asset-sets/{setId}/import/upload
func (h *AssetHandler) UploadCSV(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}

	setID, err := strconv.Atoi(r.PathValue("setId"))
	if err != nil {
		respondInvalidID(w, r, "setId")
		return
	}

	canAdmin, err := h.canAdminSet(currentUser.ID, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canAdmin {
		respondForbidden(w, r)
		return
	}

	if h.attachmentPath == "" {
		respondBadRequest(w, r, "File storage is not configured")
		return
	}

	// Parse multipart form (max 50MB)
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		respondBadRequest(w, r, "Failed to parse form data")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		respondBadRequest(w, r, "No file provided")
		return
	}
	defer file.Close()

	hasHeader := r.FormValue("has_header") != "false"
	delimiterStr := r.FormValue("delimiter")

	// Sanitize filename - strip directory components
	safeFilename := filepath.Base(header.Filename)

	// Generate unique upload ID and storage path
	uploadID := uuid.New().String()
	importsBase := filepath.Join(h.attachmentPath, "imports")
	importDir, err := securejoin.SecureJoin(importsBase, uploadID)
	if err != nil {
		respondBadRequest(w, r, "Invalid file path")
		return
	}

	if err := os.MkdirAll(importDir, 0o750); err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to create import directory: %w", err))
		return
	}

	destPath, err := securejoin.SecureJoin(importDir, safeFilename)
	if err != nil {
		respondBadRequest(w, r, "Invalid file path")
		return
	}
	destFile, err := os.Create(destPath)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to create temp file: %w", err))
		return
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, file); err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to save file: %w", err))
		return
	}

	// Auto-detect delimiter if not provided
	delimiter := ','
	if delimiterStr != "" {
		switch delimiterStr {
		case "tab", "\t":
			delimiter = '\t'
		case "semicolon", ";":
			delimiter = ';'
		case "pipe", "|":
			delimiter = '|'
		default:
			if len(delimiterStr) == 1 {
				delimiter = rune(delimiterStr[0])
			}
		}
	} else {
		delimiter = h.detectDelimiter(destPath)
	}

	// Parse preview rows
	headers, previewRows, totalRows, err := h.parseCSVPreview(destPath, delimiter, hasHeader, 5)
	if err != nil {
		// Clean up on error
		_ = os.RemoveAll(importDir)
		respondBadRequest(w, r, fmt.Sprintf("Failed to parse CSV: %v", err))
		return
	}

	headerWarning := detectHeaderMismatch(headers, previewRows, hasHeader)

	delimiterDisplay := string(delimiter)
	if delimiter == '\t' {
		delimiterDisplay = "tab"
	}

	respondJSONOK(w, CSVUploadResponse{
		UploadID:      uploadID,
		Headers:       headers,
		PreviewRows:   previewRows,
		TotalRows:     totalRows,
		Delimiter:     delimiterDisplay,
		HeaderWarning: headerWarning,
	})
}

// --- Start Import Handler ---

// StartImport handles POST /asset-sets/{setId}/import/start
func (h *AssetHandler) StartImport(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}

	setID, err := strconv.Atoi(r.PathValue("setId"))
	if err != nil {
		respondInvalidID(w, r, "setId")
		return
	}

	canAdmin, err := h.canAdminSet(currentUser.ID, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canAdmin {
		respondForbidden(w, r)
		return
	}

	var req StartAssetImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	if req.UploadID == "" {
		respondValidationError(w, r, "upload_id is required")
		return
	}
	if req.AssetTypeID == 0 {
		respondValidationError(w, r, "asset_type_id is required")
		return
	}

	// Validate asset type belongs to this set
	var typeSetID int
	err = h.db.QueryRow("SELECT set_id FROM asset_types WHERE id = ?", req.AssetTypeID).Scan(&typeSetID)
	if err == sql.ErrNoRows {
		respondValidationError(w, r, "Asset type not found")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if typeSetID != setID {
		respondValidationError(w, r, "Asset type does not belong to this set")
		return
	}

	// Locate the uploaded file
	importsBase := filepath.Join(h.attachmentPath, "imports")
	importDir, err := securejoin.SecureJoin(importsBase, req.UploadID)
	if err != nil {
		respondBadRequest(w, r, "Invalid upload ID")
		return
	}

	// Find the CSV file in the upload directory
	entries, err := os.ReadDir(importDir)
	if err != nil {
		respondBadRequest(w, r, "Upload not found - please re-upload the file")
		return
	}
	if len(entries) == 0 {
		respondBadRequest(w, r, "Upload directory is empty")
		return
	}
	filePath := filepath.Join(importDir, entries[0].Name())

	// Create job
	jobID := uuid.New().String()
	configJSON, err := json.Marshal(req)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	_, err = h.db.ExecWrite(`
		INSERT INTO asset_import_jobs (id, set_id, status, phase, file_path, config_json, created_by, created_at)
		VALUES (?, ?, 'queued', 'initializing', ?, ?, ?, ?)
	`, jobID, setID, filePath, string(configJSON), currentUser.ID, time.Now())
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Audit log
	_ = logger.LogAudit(h.db, logger.AuditEvent{
		UserID:       currentUser.ID,
		Username:     currentUser.Username,
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   "asset_import",
		ResourceType: "asset_import",
		ResourceName: jobID,
		Details: map[string]interface{}{
			"set_id":        setID,
			"asset_type_id": req.AssetTypeID,
		},
		Success: true,
	})

	// Start background import
	go h.executeCSVImport(jobID, setID, req, filePath, currentUser.ID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"job_id":  jobID,
		"message": "Import started successfully",
	})
}

// --- Job Status Handlers ---

// GetImportJob handles GET /asset-sets/{setId}/import/jobs/{jobId}
func (h *AssetHandler) GetImportJob(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}

	setID, err := strconv.Atoi(r.PathValue("setId"))
	if err != nil {
		respondInvalidID(w, r, "setId")
		return
	}

	canAdmin, err := h.canAdminSet(currentUser.ID, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canAdmin {
		respondForbidden(w, r)
		return
	}

	jobID := r.PathValue("jobId")

	var status, phase, progressJSON, errorMessage sql.NullString
	var createdAt sql.NullTime
	var startedAt, completedAt sql.NullTime

	err = h.db.QueryRow(`
		SELECT status, phase, progress_json, error_message, created_at, started_at, completed_at
		FROM asset_import_jobs WHERE id = ? AND set_id = ?
	`, jobID, setID).Scan(&status, &phase, &progressJSON, &errorMessage, &createdAt, &startedAt, &completedAt)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "import job")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	response := AssetImportJobResponse{
		JobID:  jobID,
		Status: status.String,
		Phase:  phase.String,
	}

	if progressJSON.Valid && progressJSON.String != "" {
		var progress AssetImportProgress
		if err := json.Unmarshal([]byte(progressJSON.String), &progress); err == nil {
			response.Progress = &progress
		}
	}

	if errorMessage.Valid {
		response.ErrorMessage = errorMessage.String
	}
	if createdAt.Valid {
		response.CreatedAt = &createdAt.Time
	}
	if startedAt.Valid {
		response.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		response.CompletedAt = &completedAt.Time
	}

	respondJSONOK(w, response)
}

// GetImportJobs handles GET /asset-sets/{setId}/import/jobs
func (h *AssetHandler) GetImportJobs(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}

	setID, err := strconv.Atoi(r.PathValue("setId"))
	if err != nil {
		respondInvalidID(w, r, "setId")
		return
	}

	canAdmin, err := h.canAdminSet(currentUser.ID, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canAdmin {
		respondForbidden(w, r)
		return
	}

	rows, err := h.db.Query(`
		SELECT id, status, phase, progress_json, error_message, created_at, started_at, completed_at
		FROM asset_import_jobs WHERE set_id = ? ORDER BY created_at DESC LIMIT 20
	`, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer rows.Close()

	var jobs []AssetImportJobResponse
	for rows.Next() {
		var status, phase, progressJSON, errorMessage sql.NullString
		var createdAt sql.NullTime
		var startedAt, completedAt sql.NullTime
		var jobID string

		if err := rows.Scan(&jobID, &status, &phase, &progressJSON, &errorMessage, &createdAt, &startedAt, &completedAt); err != nil {
			continue
		}

		job := AssetImportJobResponse{
			JobID:  jobID,
			Status: status.String,
			Phase:  phase.String,
		}

		if progressJSON.Valid && progressJSON.String != "" {
			var progress AssetImportProgress
			if err := json.Unmarshal([]byte(progressJSON.String), &progress); err == nil {
				job.Progress = &progress
			}
		}
		if errorMessage.Valid {
			job.ErrorMessage = errorMessage.String
		}
		if createdAt.Valid {
			job.CreatedAt = &createdAt.Time
		}
		if startedAt.Valid {
			job.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			job.CompletedAt = &completedAt.Time
		}

		jobs = append(jobs, job)
	}

	if jobs == nil {
		jobs = []AssetImportJobResponse{}
	}

	respondJSONOK(w, jobs)
}

// --- Background Import Execution ---

func (h *AssetHandler) executeCSVImport(jobID string, setID int, req StartAssetImportRequest, filePath string, userID int) {
	h.updateImportJobStatus(jobID, "running", "initializing", nil, "")

	// Open CSV file
	f, err := os.Open(filePath) //nolint:gosec // filePath from trusted internal import job state
	if err != nil {
		h.updateImportJobStatus(jobID, "failed", "", nil, fmt.Sprintf("Failed to open CSV file: %v", err))
		return
	}
	defer f.Close()

	delimiter := ','
	if req.Delimiter != "" {
		switch req.Delimiter {
		case "tab", "\t":
			delimiter = '\t'
		case "semicolon", ";":
			delimiter = ';'
		case "pipe", "|":
			delimiter = '|'
		default:
			if len(req.Delimiter) == 1 {
				delimiter = rune(req.Delimiter[0])
			}
		}
	}

	reader := csv.NewReader(f)
	reader.Comma = delimiter
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true

	// Skip header row if present
	if req.HasHeader {
		if _, err := reader.Read(); err != nil {
			h.updateImportJobStatus(jobID, "failed", "", nil, "Failed to read CSV header")
			return
		}
	}

	// Get default status for this set
	var defaultStatusID *int
	var defStatusID int
	err = h.db.QueryRow("SELECT id FROM asset_statuses WHERE set_id = ? AND is_default = true LIMIT 1", setID).Scan(&defStatusID)
	if err == nil {
		defaultStatusID = &defStatusID
	}

	progress := &AssetImportProgress{
		Phase: "importing",
	}

	// Count total rows first by reading through the file
	totalRows := 0
	for {
		_, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Skip malformed rows in count
			continue
		}
		totalRows++
	}
	progress.TotalRows = totalRows

	// Reset file position
	_, _ = f.Seek(0, 0)
	reader = csv.NewReader(f)
	reader.Comma = delimiter
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true
	if req.HasHeader {
		_, _ = reader.Read() // skip header again
	}

	h.updateImportJobProgress(jobID, progress)

	// Process rows in batches
	batchSize := 100
	rowNum := 0

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			rowNum++
			progress.FailedCount++
			if len(progress.Errors) < 100 {
				progress.Errors = append(progress.Errors, fmt.Sprintf("Row %d: %v", rowNum, err))
			}
			continue
		}
		rowNum++

		importErr := h.importCSVRow(record, setID, req, userID, defaultStatusID)
		if importErr != nil {
			progress.FailedCount++
			if len(progress.Errors) < 100 {
				progress.Errors = append(progress.Errors, fmt.Sprintf("Row %d: %v", rowNum, importErr))
			}
		} else {
			progress.ImportedCount++
		}

		// Update progress every batch
		if rowNum%batchSize == 0 {
			h.updateImportJobProgress(jobID, progress)
		}
	}

	// Final progress update
	progress.Phase = "completed"
	h.updateImportJobStatus(jobID, "completed", "completed", progress, "")

	// Clean up temp file
	importDir := filepath.Dir(filePath) //nolint:gosec // filePath from trusted internal import job state
	if err := os.RemoveAll(importDir); err != nil {
		slog.Error("Failed to clean up import temp files", "dir", importDir, "error", err)
	}
}

func (h *AssetHandler) importCSVRow(record []string, setID int, req StartAssetImportRequest, userID int, defaultStatusID *int) error {
	getCol := func(idx int) string {
		if idx < 0 || idx >= len(record) {
			return ""
		}
		return strings.TrimSpace(record[idx])
	}

	title := utils.StripHTMLTags(getCol(req.Mappings.Title))
	if title == "" {
		return fmt.Errorf("title is empty")
	}

	description := ""
	if req.Mappings.Description >= 0 {
		description = utils.SanitizeDescription(getCol(req.Mappings.Description))
	}

	assetTag := ""
	if req.Mappings.AssetTag >= 0 {
		assetTag = utils.StripHTMLTags(getCol(req.Mappings.AssetTag))
	}

	// Resolve category
	var categoryID *int
	if req.Mappings.CategoryID >= 0 {
		catName := getCol(req.Mappings.CategoryID)
		if catName != "" && req.CategoryMap != nil {
			if id, ok := req.CategoryMap[catName]; ok {
				categoryID = &id
			}
		}
	}

	// Resolve status
	var statusID *int
	if req.Mappings.StatusID >= 0 {
		statusName := getCol(req.Mappings.StatusID)
		if statusName != "" && req.StatusMap != nil {
			if id, ok := req.StatusMap[statusName]; ok {
				statusID = &id
			}
		}
	}
	if statusID == nil {
		statusID = defaultStatusID
	}

	// Build custom field values
	var customFieldValuesJSON *string
	if len(req.Mappings.CustomFields) > 0 {
		cfValues := make(map[string]interface{})
		for fieldKey, colIdx := range req.Mappings.CustomFields {
			val := getCol(colIdx)
			if val != "" {
				cfValues[fieldKey] = utils.StripHTMLTags(val)
			}
		}
		if len(cfValues) > 0 {
			b, err := json.Marshal(cfValues)
			if err == nil {
				s := string(b)
				customFieldValuesJSON = &s
			}
		}
	}

	now := time.Now()
	_, err := h.db.ExecWrite(`
		INSERT INTO assets (set_id, asset_type_id, category_id, status_id, title, description, asset_tag, custom_field_values, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, setID, req.AssetTypeID, categoryID, statusID, title, description, assetTag, customFieldValuesJSON, userID, now, now)

	return err
}

// --- Job Status Update Helpers ---

func (h *AssetHandler) updateImportJobStatus(jobID, status, phase string, progress *AssetImportProgress, errorMessage string) {
	progressJSON := "{}"
	if progress != nil {
		if data, err := json.Marshal(progress); err == nil {
			progressJSON = string(data)
		}
	}

	var err error
	switch status {
	case "running":
		_, err = h.db.ExecWrite(`UPDATE asset_import_jobs SET status = ?, phase = ?, progress_json = ?, started_at = CURRENT_TIMESTAMP WHERE id = ?`,
			status, phase, progressJSON, jobID)
	case "completed", "failed":
		_, err = h.db.ExecWrite(`UPDATE asset_import_jobs SET status = ?, phase = ?, progress_json = ?, error_message = ?, completed_at = CURRENT_TIMESTAMP WHERE id = ?`,
			status, phase, progressJSON, errorMessage, jobID)
	default:
		_, err = h.db.ExecWrite(`UPDATE asset_import_jobs SET status = ?, phase = ?, progress_json = ? WHERE id = ?`,
			status, phase, progressJSON, jobID)
	}
	if err != nil {
		slog.Error("Failed to update import job status", "jobID", jobID, "error", err)
	}
}

func (h *AssetHandler) updateImportJobProgress(jobID string, progress *AssetImportProgress) {
	progressJSON := "{}"
	if progress != nil {
		if data, err := json.Marshal(progress); err == nil {
			progressJSON = string(data)
		}
	}
	if _, err := h.db.ExecWrite(`UPDATE asset_import_jobs SET phase = ?, progress_json = ? WHERE id = ?`,
		progress.Phase, progressJSON, jobID); err != nil {
		slog.Error("Failed to update import job progress", "jobID", jobID, "error", err)
	}
}

// --- Suggest Fields & Create Type ---

// SuggestFieldsRequest is the request body for suggesting fields from CSV columns.
type SuggestFieldsRequest struct {
	UploadID  string `json:"upload_id"`
	HasHeader bool   `json:"has_header"`
	Delimiter string `json:"delimiter,omitempty"`
}

// SuggestedField represents a single suggested field from CSV analysis.
type SuggestedField struct {
	ColumnIndex   int      `json:"column_index"`
	HeaderName    string   `json:"header_name"`
	SuggestedName string   `json:"suggested_name"`
	SuggestedType string   `json:"suggested_type"`
	Options       []string `json:"options,omitempty"`
	SampleValues  []string `json:"sample_values"`
	IsStandard    bool     `json:"is_standard"`
}

// CreateTypeFromImportRequest is the request body for creating a type with fields during import.
type CreateTypeFromImportRequest struct {
	Name        string                      `json:"name"`
	Description string                      `json:"description"`
	Icon        string                      `json:"icon"`
	Color       string                      `json:"color"`
	Fields      []CreateTypeFromImportField `json:"fields"`
}

// CreateTypeFromImportField represents a field to create/associate with the new type.
type CreateTypeFromImportField struct {
	Name         string   `json:"name"`
	FieldType    string   `json:"field_type"`
	Options      []string `json:"options,omitempty"`
	IsRequired   bool     `json:"is_required"`
	DisplayOrder int      `json:"display_order"`
}

// SuggestFieldsFromCSV handles POST /asset-sets/{setId}/import/suggest-fields
func (h *AssetHandler) SuggestFieldsFromCSV(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}

	setID, err := strconv.Atoi(r.PathValue("setId"))
	if err != nil {
		respondInvalidID(w, r, "setId")
		return
	}

	canAdmin, err := h.canAdminSet(currentUser.ID, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canAdmin {
		respondForbidden(w, r)
		return
	}

	var req SuggestFieldsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	if req.UploadID == "" {
		respondValidationError(w, r, "upload_id is required")
		return
	}

	// Locate the uploaded file
	importsBase := filepath.Join(h.attachmentPath, "imports")
	importDir, err := securejoin.SecureJoin(importsBase, req.UploadID)
	if err != nil {
		respondBadRequest(w, r, "Invalid upload ID")
		return
	}

	entries, err := os.ReadDir(importDir)
	if err != nil || len(entries) == 0 {
		respondBadRequest(w, r, "Upload not found - please re-upload the file")
		return
	}
	filePath := filepath.Join(importDir, entries[0].Name())

	// Parse delimiter
	delimiter := ','
	if req.Delimiter != "" {
		switch req.Delimiter {
		case "tab", "\t":
			delimiter = '\t'
		case "semicolon", ";":
			delimiter = ';'
		case "pipe", "|":
			delimiter = '|'
		default:
			if len(req.Delimiter) == 1 {
				delimiter = rune(req.Delimiter[0])
			}
		}
	}

	// Parse CSV with more rows for better sampling
	headers, previewRows, _, err := h.parseCSVPreview(filePath, delimiter, req.HasHeader, 20)
	if err != nil {
		respondBadRequest(w, r, fmt.Sprintf("Failed to parse CSV: %v", err))
		return
	}

	var suggestions []SuggestedField
	for colIdx, header := range headers {
		// Collect sample values for this column
		var samples []string
		seen := make(map[string]bool)
		for _, row := range previewRows {
			if colIdx < len(row) {
				val := strings.TrimSpace(row[colIdx])
				if val != "" && !seen[val] {
					seen[val] = true
					samples = append(samples, val)
				}
			}
		}

		isStd := isStandardField(header)
		suggestedType, options := inferFieldType(samples)
		suggestedName := cleanHeaderName(header)

		// Cap sample values for response
		displaySamples := samples
		if len(displaySamples) > 5 {
			displaySamples = displaySamples[:5]
		}

		suggestions = append(suggestions, SuggestedField{
			ColumnIndex:   colIdx,
			HeaderName:    header,
			SuggestedName: suggestedName,
			SuggestedType: suggestedType,
			Options:       options,
			SampleValues:  displaySamples,
			IsStandard:    isStd,
		})
	}

	respondJSONOK(w, map[string]interface{}{
		"suggested_fields": suggestions,
	})
}

// CreateTypeFromImport handles POST /asset-sets/{setId}/import/create-type
func (h *AssetHandler) CreateTypeFromImport(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}

	setID, err := strconv.Atoi(r.PathValue("setId"))
	if err != nil {
		respondInvalidID(w, r, "setId")
		return
	}

	canAdmin, err := h.canAdminSet(currentUser.ID, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canAdmin {
		respondForbidden(w, r)
		return
	}

	var req CreateTypeFromImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	if req.Name == "" {
		respondValidationError(w, r, "Name is required")
		return
	}

	if req.Icon == "" {
		req.Icon = "Box"
	}
	if req.Color == "" {
		req.Color = "#6b7280"
	}

	// Validate field types
	allowedTypes := map[string]bool{
		"text": true, "textarea": true, "number": true, "date": true, "select": true,
	}
	for _, f := range req.Fields {
		if f.Name == "" {
			respondValidationError(w, r, "All fields must have a name")
			return
		}
		if !allowedTypes[f.FieldType] {
			respondValidationError(w, r, fmt.Sprintf("Invalid field type: %s", f.FieldType))
			return
		}
	}

	tx, err := h.db.Begin()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now()

	// Insert the asset type
	var typeID int64
	err = tx.QueryRow(`
		INSERT INTO asset_types (set_id, name, description, icon, color, display_order, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 0, true, ?, ?) RETURNING id
	`, setID, req.Name, req.Description, req.Icon, req.Color, now, now).Scan(&typeID)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "unique") {
			respondValidationError(w, r, "An asset type with this name already exists")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Create or reuse custom field definitions and link them to the type
	var fields []models.AssetTypeField
	for _, f := range req.Fields {
		// Marshal options for select fields
		var optionsJSON *string
		if f.FieldType == "select" && len(f.Options) > 0 {
			b, err := json.Marshal(f.Options)
			if err == nil {
				s := string(b)
				optionsJSON = &s
			}
		}

		// Try to find existing custom field definition by name + type
		var cfID int
		err = tx.QueryRow(`
			SELECT id FROM custom_field_definitions
			WHERE LOWER(name) = LOWER(?) AND field_type = ?
		`, f.Name, f.FieldType).Scan(&cfID)

		if err == sql.ErrNoRows {
			// Create new custom field definition
			err = tx.QueryRow(`
				INSERT INTO custom_field_definitions (name, field_type, options, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?) RETURNING id
			`, f.Name, f.FieldType, optionsJSON, now, now).Scan(&cfID)
			if err != nil {
				respondInternalError(w, r, err)
				return
			}
		} else if err != nil {
			respondInternalError(w, r, err)
			return
		}

		// Link field to type
		var atfID int64
		err = tx.QueryRow(`
			INSERT INTO asset_type_fields (asset_type_id, custom_field_id, is_required, display_order, created_at)
			VALUES (?, ?, ?, ?, ?) RETURNING id
		`, typeID, cfID, f.IsRequired, f.DisplayOrder, now).Scan(&atfID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		fields = append(fields, models.AssetTypeField{
			ID:            int(atfID),
			AssetTypeID:   int(typeID),
			CustomFieldID: cfID,
			IsRequired:    f.IsRequired,
			DisplayOrder:  f.DisplayOrder,
			CreatedAt:     now,
			FieldName:     f.Name,
			FieldType:     f.FieldType,
		})

		if optionsJSON != nil {
			fields[len(fields)-1].Options = *optionsJSON
		}
	}

	if err = tx.Commit(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	id := int(typeID)
	_ = logger.LogAudit(h.db, logger.AuditEvent{
		UserID:       currentUser.ID,
		Username:     currentUser.Username,
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   logger.ActionAssetTypeCreate,
		ResourceType: logger.ResourceAssetType,
		ResourceID:   &id,
		ResourceName: req.Name,
		Details: map[string]interface{}{
			"source":      "import_wizard",
			"field_count": len(req.Fields),
		},
		Success: true,
	})

	assetType := models.AssetType{
		ID:          int(typeID),
		SetID:       setID,
		Name:        req.Name,
		Description: req.Description,
		Icon:        req.Icon,
		Color:       req.Color,
		IsActive:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
		Fields:      fields,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"asset_type": assetType,
		"fields":     fields,
	})
}

// inferFieldType analyzes sample values and returns a suggested field type and options.
func inferFieldType(values []string) (string, []string) {
	if len(values) == 0 {
		return "text", nil
	}

	allNumeric := true
	allDate := true
	hasLong := false
	uniqueValues := make(map[string]bool)

	dateRegex := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$|^\d{1,2}/\d{1,2}/\d{2,4}$|^\d{1,2}\.\d{1,2}\.\d{2,4}$`)
	numRegex := regexp.MustCompile(`^-?\d+([.,]\d+)?$`)

	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		uniqueValues[v] = true
		if !numRegex.MatchString(v) {
			allNumeric = false
		}
		if !dateRegex.MatchString(v) {
			allDate = false
		}
		if len(v) > 200 {
			hasLong = true
		}
	}

	nonEmpty := len(uniqueValues)
	if nonEmpty == 0 {
		return "text", nil
	}

	if allNumeric {
		return "number", nil
	}
	if allDate {
		return "date", nil
	}
	if nonEmpty <= 10 && len(values) >= 2 {
		opts := make([]string, 0, len(uniqueValues))
		for v := range uniqueValues {
			opts = append(opts, v)
		}
		return "select", opts
	}
	if hasLong {
		return "textarea", nil
	}
	return "text", nil
}

// isStandardField checks if a header name matches a standard asset field.
func isStandardField(header string) bool {
	h := strings.ToLower(strings.TrimSpace(header))
	standardFields := map[string]bool{
		"title": true, "name": true, "asset name": true, "asset_name": true,
		"description": true, "desc": true, "details": true,
		"tag": true, "asset tag": true, "asset_tag": true,
		"serial": true, "serial number": true, "serial_number": true,
		"category": true, "status": true, "state": true,
		"id": true, "asset id": true, "asset_id": true,
	}
	return standardFields[h]
}

// cleanHeaderName converts a raw CSV header into a display name.
func cleanHeaderName(header string) string {
	s := strings.TrimSpace(header)
	if s == "" {
		return s
	}
	// Replace underscores and hyphens with spaces
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, "-", " ")
	// Title-case each word
	words := strings.Fields(s)
	for i, w := range words {
		if w != "" {
			runes := []rune(w)
			runes[0] = unicode.ToUpper(runes[0])
			words[i] = string(runes)
		}
	}
	return strings.Join(words, " ")
}

// --- CSV Helpers ---

func (h *AssetHandler) detectDelimiter(filePath string) rune {
	f, err := os.Open(filePath)
	if err != nil {
		return ','
	}
	defer f.Close()

	buf := make([]byte, 8192)
	n, err := f.Read(buf)
	if err != nil || n == 0 {
		return ','
	}
	sample := string(buf[:n])

	// Count occurrences of common delimiters in first few lines
	lines := strings.SplitN(sample, "\n", 5)
	if len(lines) == 0 {
		return ','
	}

	delimiters := []rune{',', '\t', ';', '|'}
	bestDelim := ','
	bestScore := 0

	for _, d := range delimiters {
		// Check if delimiter appears consistently across lines
		counts := make([]int, 0, len(lines))
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			counts = append(counts, strings.Count(line, string(d)))
		}

		if len(counts) < 2 {
			continue
		}

		// Good delimiter: appears multiple times and consistently
		if counts[0] > 0 {
			consistent := true
			for i := 1; i < len(counts); i++ {
				if counts[i] != counts[0] {
					consistent = false
					break
				}
			}
			score := counts[0]
			if consistent {
				score *= 2
			}
			if score > bestScore {
				bestScore = score
				bestDelim = d
			}
		}
	}

	return bestDelim
}

func (h *AssetHandler) parseCSVPreview(filePath string, delimiter rune, hasHeader bool, maxPreviewRows int) ([]string, [][]string, int, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, nil, 0, err
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.Comma = delimiter
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true

	var headers []string
	var previewRows [][]string

	if hasHeader {
		headers, err = reader.Read()
		if err != nil {
			return nil, nil, 0, fmt.Errorf("failed to read header row: %w", err)
		}
	}

	// Read preview rows
	totalRows := 0
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			totalRows++
			continue
		}
		totalRows++

		if len(previewRows) < maxPreviewRows {
			previewRows = append(previewRows, record)
		}

		// If no header, generate column names from first row
		if !hasHeader && headers == nil {
			headers = make([]string, len(record))
			for i := range record {
				headers[i] = fmt.Sprintf("Column %d", i+1)
			}
		}
	}

	return headers, previewRows, totalRows, nil
}

var (
	numericPattern = regexp.MustCompile(`^\d+(\.\d+)?$`)
	datePattern    = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$|^\d{1,2}/\d{1,2}/\d{2,4}$`)
	headerKeywords = map[string]bool{
		"name": true, "title": true, "description": true, "status": true,
		"category": true, "type": true, "tag": true, "serial": true,
		"id": true, "date": true, "notes": true, "location": true,
		"model": true, "brand": true, "manufacturer": true,
	}
)

// detectHeaderMismatch checks if the user's hasHeader setting likely doesn't match the CSV content.
func detectHeaderMismatch(headers []string, previewRows [][]string, hasHeader bool) string {
	if hasHeader {
		// Check if the "headers" look like data (numeric or date values)
		if len(headers) == 0 {
			return ""
		}
		dataLikeCount := 0
		for _, h := range headers {
			v := strings.TrimSpace(h)
			if numericPattern.MatchString(v) || datePattern.MatchString(v) {
				dataLikeCount++
			}
		}
		if float64(dataLikeCount)/float64(len(headers)) > 0.5 {
			return "The first row looks like it contains data, not column headers. You may want to uncheck 'First row contains column headers' and re-upload."
		}
		return ""
	}

	// hasHeader=false: check if the first preview row looks like headers
	if len(previewRows) == 0 {
		return ""
	}
	firstRow := previewRows[0]
	if len(firstRow) == 0 {
		return ""
	}

	keywordMatches := 0
	shortNonNumeric := 0
	for _, val := range firstRow {
		v := strings.TrimSpace(val)
		if headerKeywords[strings.ToLower(v)] {
			keywordMatches++
		}
		if len(v) <= 30 && !numericPattern.MatchString(v) && v != "" {
			shortNonNumeric++
		}
	}

	if keywordMatches >= 2 {
		return "The first row looks like it contains column headers. You may want to check 'First row contains column headers' and re-upload."
	}

	// Check if first row is mostly short non-numeric while subsequent rows have more numeric/longer values
	if len(previewRows) >= 2 && float64(shortNonNumeric)/float64(len(firstRow)) > 0.5 {
		dataRowNumeric := 0
		dataRowTotal := 0
		for _, row := range previewRows[1:] {
			for _, val := range row {
				v := strings.TrimSpace(val)
				dataRowTotal++
				if numericPattern.MatchString(v) || datePattern.MatchString(v) || len(v) > 30 {
					dataRowNumeric++
				}
			}
		}
		if dataRowTotal > 0 && float64(dataRowNumeric)/float64(dataRowTotal) > 0.5 {
			return "The first row looks like it contains column headers. You may want to check 'First row contains column headers' and re-upload."
		}
	}

	return ""
}
