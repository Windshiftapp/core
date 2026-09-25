package services

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"windshift/internal/csvimport"
	"windshift/internal/database"
	"windshift/internal/itemevents"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/validation"
)

// ErrTicketImportStorageDisabled marks a server without import storage.
var ErrTicketImportStorageDisabled = errors.New("ticket import storage is not configured")

// ticketImportColumns is the stable exchange schema. Order is contractual:
// exports emit exactly this header, imports resolve columns by name.
var ticketImportColumns = []string{
	"external_ref", "title", "description", "requester_email",
	"status", "priority", "assignee_email", "created_at",
}

// TicketImportService implements the support-ticket CSV exchange on the
// shared csvimport pipeline: a stable-column export and a retry-idempotent
// import keyed by the source system's external reference.
type TicketImportService struct {
	db             database.Database
	perm           *PermissionService
	statuses       *repository.StatusRepository
	imports        *csvimport.Store
	attachmentPath string
}

func NewTicketImportService(db database.Database, perm *PermissionService, attachmentPath string) *TicketImportService {
	return &TicketImportService{
		db:             db,
		perm:           perm,
		statuses:       repository.NewStatusRepository(db),
		imports:        csvimport.NewStore(db),
		attachmentPath: attachmentPath,
	}
}

// TicketImportStaged is the client-visible upload result.
type TicketImportStaged = csvimport.StagedUpload

// Upload stages a CSV file for a workspace import.
func (s *TicketImportService) Upload(ctx context.Context, userID, workspaceID int, filename string, hasHeader bool, delimiterName string, source io.Reader) (*TicketImportStaged, error) {
	if err := s.require(userID, workspaceID, models.PermissionItemCreate); err != nil {
		return nil, err
	}
	if s.attachmentPath == "" {
		return nil, ErrTicketImportStorageDisabled
	}
	staged, _, err := csvimport.StageUpload(s.imports, s.attachmentPath, csvimport.KindTicket, workspaceID, userID, filename, hasHeader, delimiterName, source, 50<<20)
	if err != nil {
		return nil, &validation.ValidationError{Field: "file", Message: err.Error()}
	}
	return staged, nil
}

// TicketImportStart kicks off a staged import.
type TicketImportStart struct {
	UploadID  string `json:"upload_id"`
	HasHeader *bool  `json:"has_header,omitempty"`
	Delimiter string `json:"delimiter,omitempty"`
}

// TicketImportJob is the client-visible job state.
type TicketImportJob struct {
	JobID         string     `json:"job_id"`
	Status        string     `json:"status"`
	Phase         string     `json:"phase"`
	TotalRows     int        `json:"total_rows"`
	ImportedCount int        `json:"imported_count"`
	FailedCount   int        `json:"failed_count"`
	SkippedCount  int        `json:"skipped_count"`
	Errors        []string   `json:"errors,omitempty"`
	CreatedAt     *time.Time `json:"created_at,omitempty"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
}

// Start validates and launches the background import for a staged upload.
// The upload ID doubles as the job ID, so re-issuing the same start request
// returns the existing job instead of running the file twice.
func (s *TicketImportService) Start(ctx context.Context, actor AuditActor, workspaceID int, input TicketImportStart) (*TicketImportJob, error) {
	if err := s.require(actor.UserID, workspaceID, models.PermissionItemCreate); err != nil {
		return nil, err
	}
	if input.UploadID == "" {
		return nil, &validation.ValidationError{Field: "upload_id", Message: "upload_id is required"}
	}
	owned, err := s.imports.UploadOwnedBy(csvimport.KindTicket, workspaceID, actor.UserID, input.UploadID)
	if err != nil {
		return nil, err
	}
	if !owned {
		return nil, repository.ErrNotFound
	}
	hasHeader := input.HasHeader == nil || *input.HasHeader
	configJSON := fmt.Sprintf(`{"has_header":%t,"delimiter":%q}`, hasHeader, input.Delimiter)

	path := csvimport.UploadPath(s.attachmentPath, input.UploadID)
	claimed, err := s.imports.ClaimUpload(csvimport.KindTicket, workspaceID, actor.UserID, input.UploadID, path, configJSON, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	if claimed {
		emitServiceAudit(s.db, actor, "ticket_import", "workspace", &workspaceID, "", map[string]any{"upload_id": input.UploadID})
		go s.run(workspaceID, input.UploadID, path, hasHeader, csvimport.ParseDelimiter(input.Delimiter), actor.UserID)
	}
	return s.getJob(workspaceID, input.UploadID)
}

// GetJob returns one import job's state.
func (s *TicketImportService) GetJob(ctx context.Context, userID, workspaceID int, jobID string) (*TicketImportJob, error) {
	if err := s.require(userID, workspaceID, models.PermissionItemCreate); err != nil {
		return nil, err
	}
	return s.getJob(workspaceID, jobID)
}

func (s *TicketImportService) getJob(workspaceID int, jobID string) (*TicketImportJob, error) {
	row, err := s.imports.GetJob(csvimport.KindTicket, workspaceID, jobID)
	if err != nil {
		return nil, err
	}
	if (row.Status.String == "queued" || row.Status.String == "running") &&
		(!row.LeaseExpiresAt.Valid || row.LeaseExpiresAt.Int64 <= time.Now().UTC().Unix()) {
		if _, err := s.ReconcileInterrupted(); err != nil {
			return nil, err
		}
		row, err = s.imports.GetJob(csvimport.KindTicket, workspaceID, jobID)
		if err != nil {
			return nil, err
		}
	}
	job := &TicketImportJob{
		JobID: jobID, Status: row.Status.String, Phase: row.Phase.String,
		CreatedAt: nullableTime(row.CreatedAt), CompletedAt: nullableTime(row.CompletedAt),
	}
	if row.ProgressJSON.Valid && row.ProgressJSON.String != "" {
		var progress csvimport.Progress
		if err := json.Unmarshal([]byte(row.ProgressJSON.String), &progress); err == nil {
			job.TotalRows = progress.TotalRows
			job.ImportedCount = progress.ImportedCount
			job.FailedCount = progress.FailedCount
			job.SkippedCount = progress.SkippedCount
			job.Errors = progress.Errors
		}
	}
	return job, nil
}

// ReconcileInterrupted rolls back abandoned ticket imports: the job's
// item_import_rows mapping drives the delete, and the FK cascade removes the
// partially imported tickets with it.
func (s *TicketImportService) ReconcileInterrupted() (int, error) {
	return s.imports.ReconcileExpired(csvimport.KindTicket, time.Now().UTC(), s.rollbackExpiredImport)
}

// rollbackExpiredImport deletes the tickets an interrupted import created so
// a retried upload starts from a clean slate.
func (s *TicketImportService) rollbackExpiredImport(tx database.Tx, jobID string) error {
	_, err := tx.ExecWrite(`DELETE FROM items WHERE id IN (
		SELECT item_id FROM item_import_rows WHERE job_id = ?
	)`, jobID)
	return err
}

// run executes the import. Every created ticket is tracked in
// item_import_rows; reconciliation deletes that mapping (and, by cascade,
// the tickets) when a worker loses its lease, so retries start clean.
func (s *TicketImportService) run(workspaceID int, jobID, path string, hasHeader bool, delimiter rune, actorUserID int) {
	columns, err := s.resolveColumns(path, delimiter, hasHeader)
	if err != nil {
		_ = s.imports.Finish(jobID, "failed", "", "{}", err.Error())
		return
	}

	csvimport.RunRows(s.imports, jobID, path, delimiter, hasHeader, func(rowNumber int, record []string) error {
		return s.importRow(context.Background(), workspaceID, jobID, columns, record, actorUserID)
	})
}

// resolveColumns maps the exchange column names to file column indexes.
// external_ref and title are required; the rest are optional.
func (s *TicketImportService) resolveColumns(path string, delimiter rune, hasHeader bool) (map[string]int, error) {
	headers, _, _, err := csvimport.ParsePreview(path, delimiter, hasHeader, 0)
	if err != nil {
		return nil, fmt.Errorf("read CSV header: %v", err)
	}
	columns := make(map[string]int, len(ticketImportColumns))
	for index, header := range headers {
		name := strings.ToLower(strings.TrimSpace(header))
		for _, known := range ticketImportColumns {
			if name == known {
				columns[known] = index
				break
			}
		}
	}
	for _, required := range []string{"external_ref", "title"} {
		if _, ok := columns[required]; !ok {
			return nil, fmt.Errorf("required column %q is missing from the header", required)
		}
	}
	return columns, nil
}

// importRow maps one CSV row onto a ticket with customer-safe attribution.
func (s *TicketImportService) importRow(ctx context.Context, workspaceID int, jobID string, columns map[string]int, record []string, actorUserID int) error {
	column := func(name string) string {
		index, ok := columns[name]
		if !ok || index < 0 || index >= len(record) {
			return ""
		}
		return strings.TrimSpace(record[index])
	}
	externalRef := column("external_ref")
	title := sanitize.PlainTextField.Sanitize(column("title"))
	if externalRef == "" || title == "" {
		return errors.New("external_ref and title are required")
	}
	description := sanitize.RichText.Sanitize(column("description"))
	requesterEmail := strings.ToLower(column("requester_email"))
	assigneeEmail := strings.ToLower(column("assignee_email"))
	createdAt := time.Now().UTC()
	if raw := column("created_at"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return fmt.Errorf("created_at must be RFC 3339: %v", err)
		}
		createdAt = parsed.UTC()
	}

	// Customer-safe attribution: the requester is a portal customer, never an
	// internal user by import.
	params := ItemCreationParams{
		WorkspaceID: workspaceID, Title: title, Description: description,
		CreatorID: &actorUserID, EventMetadata: itemevents.Import(jobID),
		CreatedAt: &createdAt,
	}
	if requesterEmail != "" {
		customerID, _, err := repository.NewPortalCustomerRepository(s.db).
			FindOrCreateByEmail(ctx, strings.SplitN(requesterEmail, "@", 2)[0], requesterEmail)
		if err != nil {
			return fmt.Errorf("resolve requester: %w", err)
		}
		params.CreatorPortalCustomerID = &customerID
	}
	if name := column("status"); name != "" {
		statusID, err := s.resolveStatusName(workspaceID, name)
		if err != nil {
			return err
		}
		params.StatusID = statusID
	}
	if name := column("priority"); name != "" {
		priorityID, err := s.resolvePriorityName(name)
		if err != nil {
			return err
		}
		params.PriorityID = priorityID
	}
	if assigneeEmail != "" {
		assigneeID, err := s.resolveAssigneeEmail(assigneeEmail)
		if err != nil {
			return err
		}
		params.AssigneeID = assigneeID
	}

	itemID, err := CreateItem(s.db, params)
	if err != nil {
		return fmt.Errorf("create ticket: %w", err)
	}

	// Track the mapping for retry idempotency and rollback; renew the lease in
	// the same transaction so long imports stay alive. A conflict means the
	// same external_ref landed concurrently: remove the just-created duplicate
	// and count the row as skipped.
	var skipped bool
	if err := database.WithTx(s.db, func(tx database.Tx) error {
		if err := s.imports.RenewLeaseInTx(tx, jobID); err != nil {
			return err
		}
		res, err := tx.ExecWrite(`
			INSERT INTO item_import_rows (workspace_id, external_ref, item_id, job_id)
			VALUES (?, ?, ?, ?)
			ON CONFLICT (workspace_id, external_ref) DO NOTHING
		`, workspaceID, externalRef, itemID, jobID)
		if err != nil {
			return fmt.Errorf("track imported row: %w", err)
		}
		if rows, _ := res.RowsAffected(); rows == 0 {
			skipped = true
		}
		return nil
	}); err != nil {
		if errors.Is(err, csvimport.ErrLeaseLost) {
			return err
		}
		return fmt.Errorf("track imported row: %w", err)
	}
	if skipped {
		if _, err := s.db.ExecWrite(`DELETE FROM items WHERE id = ?`, itemID); err != nil {
			return fmt.Errorf("remove concurrently imported duplicate %d: %w", itemID, err)
		}
		return fmt.Errorf("%w: %s", csvimport.ErrSkipRow, externalRef)
	}
	return nil
}

// Export streams the workspace's tickets in the exchange schema. external_ref
// is the item key, so re-importing an export into the same workspace is a
// no-op.
func (s *TicketImportService) Export(ctx context.Context, w io.Writer, userID, workspaceID int) error {
	if err := s.require(userID, workspaceID, models.PermissionItemView); err != nil {
		return err
	}
	writer := csv.NewWriter(w)
	if err := writer.Write(ticketImportColumns); err != nil {
		return err
	}

	// Keyed pagination keeps memory flat and gives a stable stream.
	lastID := 0
	for {
		rows, err := s.db.Query(`
			SELECT i.id, w.key || '-' || i.workspace_item_number, i.title,
			       COALESCE(i.description, ''), i.created_at,
			       COALESCE(s.name, ''), COALESCE(p.name, ''),
			       COALESCE(u.email, ''), COALESCE(pc.email, '')
			FROM items i
			JOIN workspaces w ON i.workspace_id = w.id
			LEFT JOIN statuses s ON i.status_id = s.id
			LEFT JOIN priorities p ON i.priority_id = p.id
			LEFT JOIN users u ON i.assignee_id = u.id
			LEFT JOIN portal_customers pc ON i.creator_portal_customer_id = pc.id
			WHERE i.workspace_id = ? AND i.id > ?
			ORDER BY i.id LIMIT 500
		`, workspaceID, lastID)
		if err != nil {
			return fmt.Errorf("query export page: %w", err)
		}
		written := 0
		for rows.Next() {
			var id int
			var key, title, description, status, priority, assigneeEmail, requesterEmail string
			var createdAt time.Time
			if err := rows.Scan(&id, &key, &title, &description, &createdAt, &status, &priority, &assigneeEmail, &requesterEmail); err != nil {
				_ = rows.Close()
				return fmt.Errorf("scan export row: %w", err)
			}
			lastID = id
			written++
			if err := writer.Write([]string{
				key, title, description, requesterEmail, status, priority, assigneeEmail,
				createdAt.Format(time.RFC3339),
			}); err != nil {
				_ = rows.Close()
				return err
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		rows.Close()
		if written == 0 {
			break
		}
	}
	writer.Flush()
	return writer.Error()
}

// Authorize is the pre-parse permission check for raw handlers: it maps
// authorization failures to the masked not-found contract before any request
// body is read.
func (s *TicketImportService) Authorize(ctx context.Context, userID, workspaceID int, permission string) error {
	return s.require(userID, workspaceID, permission)
}

func (s *TicketImportService) require(userID, workspaceID int, permission string) error {
	allowed, err := s.perm.HasWorkspacePermission(userID, workspaceID, permission)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrItemForbidden
	}
	return nil
}

// resolveStatusName maps a status display name onto the workspace's valid
// statuses. Comparison is case-insensitive.
func (s *TicketImportService) resolveStatusName(workspaceID int, name string) (*int, error) {
	statuses, err := s.statuses.ListForWorkspaces([]int{workspaceID})
	if err != nil {
		return nil, fmt.Errorf("list workspace statuses: %w", err)
	}
	for _, status := range statuses {
		if strings.EqualFold(status.Name, name) {
			id := status.ID
			return &id, nil
		}
	}
	return nil, fmt.Errorf("status %q does not exist in this workspace", name)
}

func (s *TicketImportService) resolvePriorityName(name string) (*int, error) {
	var id int
	err := s.db.QueryRow(`SELECT id FROM priorities WHERE LOWER(name) = LOWER(?)`, name).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("priority %q does not exist", name)
	}
	if err != nil {
		return nil, fmt.Errorf("resolve priority: %w", err)
	}
	return &id, nil
}

func (s *TicketImportService) resolveAssigneeEmail(email string) (*int, error) {
	var id int
	err := s.db.QueryRow(`SELECT id FROM users WHERE LOWER(email) = ? AND is_active = true`, email).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("assignee %q is not an active user", email)
	}
	if err != nil {
		return nil, fmt.Errorf("resolve assignee: %w", err)
	}
	return &id, nil
}

func nullableTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time
	return &t
}
