package repository

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
)

// AuditLogRepository serves the admin audit-log read endpoints.
type AuditLogRepository struct {
	db database.Database
}

// NewAuditLogRepository creates an AuditLogRepository.
func NewAuditLogRepository(db database.Database) *AuditLogRepository {
	return &AuditLogRepository{db: db}
}

// AuditLogFilters carries optional filters for AuditLogRepository.List.
// Zero-valued / nil fields are skipped at WHERE-clause build time.
type AuditLogFilters struct {
	ActionType   string     // exact match
	UserID       *int       // exact match (nil = any)
	ResourceType string     // exact match
	Success      *bool      // nil = any
	From         *time.Time // timestamp >= From
	To           *time.Time // timestamp <= To
	Search       string     // substring match against username, resource_name, action_type
}

// AuditLogRow is one audit_logs row; Details is the parsed JSON map (or nil
// if the row's details column is null/empty/invalid).
type AuditLogRow struct {
	ID           int
	Timestamp    time.Time
	UserID       *int
	Username     string
	IPAddress    string
	UserAgent    string
	ActionType   string
	ResourceType string
	ResourceID   *int
	ResourceName string
	Details      map[string]interface{}
	Success      bool
	ErrorMessage string
}

// List returns a page of audit logs matching the filters plus the unfiltered-by-page total.
func (r *AuditLogRepository) List(filters AuditLogFilters, page, perPage int) ([]AuditLogRow, int, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 50
	}

	whereClause, args := buildAuditLogWhere(filters)

	var total int
	if err := r.db.QueryRow("SELECT COUNT(*) FROM audit_logs "+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count audit logs: %w", err)
	}

	offset := (page - 1) * perPage
	query := `SELECT id, timestamp, user_id, username, ip_address, user_agent,
		action_type, resource_type, resource_id, resource_name, details, success, error_message
		FROM audit_logs ` + whereClause + ` ORDER BY timestamp DESC, id DESC LIMIT ? OFFSET ?`

	dataArgs := append(args, perPage, offset) //nolint:gocritic // separate variable to keep filter args reusable above
	rows, err := r.db.Query(query, dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("query audit logs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	entries := make([]AuditLogRow, 0)
	for rows.Next() {
		var e AuditLogRow
		var ipAddress, userAgent, resourceName, detailsJSON, errorMessage *string
		if err := rows.Scan(
			&e.ID, &e.Timestamp, &e.UserID, &e.Username,
			&ipAddress, &userAgent,
			&e.ActionType, &e.ResourceType, &e.ResourceID, &resourceName,
			&detailsJSON, &e.Success, &errorMessage,
		); err != nil {
			return nil, 0, fmt.Errorf("scan audit log: %w", err)
		}
		if ipAddress != nil {
			e.IPAddress = *ipAddress
		}
		if userAgent != nil {
			e.UserAgent = *userAgent
		}
		if resourceName != nil {
			e.ResourceName = *resourceName
		}
		if errorMessage != nil {
			e.ErrorMessage = *errorMessage
		}
		if detailsJSON != nil && *detailsJSON != "" {
			_ = json.Unmarshal([]byte(*detailsJSON), &e.Details)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate audit logs: %w", err)
	}
	return entries, total, nil
}

// ListDistinctActionTypes returns every action_type value in the audit log,
// ordered alphabetically. Used to populate filter dropdowns.
func (r *AuditLogRepository) ListDistinctActionTypes() ([]string, error) {
	return r.queryDistinctStrings("SELECT DISTINCT action_type FROM audit_logs ORDER BY action_type")
}

// ListDistinctResourceTypes returns every resource_type value in the audit log,
// ordered alphabetically. Used to populate filter dropdowns.
func (r *AuditLogRepository) ListDistinctResourceTypes() ([]string, error) {
	return r.queryDistinctStrings("SELECT DISTINCT resource_type FROM audit_logs ORDER BY resource_type")
}

func (r *AuditLogRepository) queryDistinctStrings(query string) ([]string, error) {
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query distinct: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, fmt.Errorf("scan distinct: %w", err)
		}
		result = append(result, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate distinct: %w", err)
	}
	return result, nil
}

func auditLogSearchPattern(s string) (pattern string, escaped bool) {
	escapedPattern := escapeLikePattern(s)
	if escapedPattern == s {
		return "%" + s + "%", false
	}
	return "%" + escapedPattern + "%", true
}

func escapeLikePattern(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

func buildAuditLogWhere(f AuditLogFilters) (whereClause string, args []interface{}) {
	var conditions []string

	if f.ActionType != "" {
		conditions = append(conditions, "action_type = ?")
		args = append(args, f.ActionType)
	}
	if f.UserID != nil {
		conditions = append(conditions, "user_id = ?")
		args = append(args, *f.UserID)
	}
	if f.ResourceType != "" {
		conditions = append(conditions, "resource_type = ?")
		args = append(args, f.ResourceType)
	}
	if f.Success != nil {
		if *f.Success {
			conditions = append(conditions, "success = true")
		} else {
			conditions = append(conditions, "success = false")
		}
	}
	if f.From != nil {
		conditions = append(conditions, "timestamp >= ?")
		args = append(args, *f.From)
	}
	if f.To != nil {
		conditions = append(conditions, "timestamp <= ?")
		args = append(args, *f.To)
	}
	if f.Search != "" {
		search, escaped := auditLogSearchPattern(f.Search)
		if escaped {
			conditions = append(conditions, "(username LIKE ? ESCAPE '\\' OR resource_name LIKE ? ESCAPE '\\' OR action_type LIKE ? ESCAPE '\\')")
		} else {
			conditions = append(conditions, "(username LIKE ? OR resource_name LIKE ? OR action_type LIKE ?)")
		}
		args = append(args, search, search, search)
	}

	if len(conditions) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(conditions, " AND "), args
}
