package v2

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/auth"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
)

type auditLogReader interface {
	List(repository.AuditLogFilters, int, int) ([]repository.AuditLogRow, int, error)
	ListSince(int, int) ([]repository.AuditLogRow, error)
}

type adminTokenManager interface {
	ListAllTokens(*int, int, int) ([]models.APIToken, int, error)
	AdminRevokeToken(int) error
}

func registerAdminIntegrationRoutes(b *routeBuilder, deps Deps) {
	b.Page("/admin/audit-logs", AuthAuthenticated, []string{"admin:audit-logs:read"}, listAdminAuditLogs(deps))
	b.Read("/admin/audit-logs/since", AuthAuthenticated, []string{"admin:audit-logs:read"}, streamAdminAuditLogs(deps))
	b.Page("/admin/api-tokens", AuthAuthenticated, []string{"admin:api-tokens:read"}, listAdminTokens(deps))
	b.Command(http.MethodDelete, "/admin/api-tokens/{token_id}", AuthAuthenticated, []string{"admin:api-tokens:write"}, revokeAdminToken(deps))
	registerAdminTranslationRoutes(b, deps)
}

type auditLogDTO struct {
	ID           int            `json:"id"`
	Timestamp    string         `json:"timestamp"`
	UserID       *int           `json:"user_id"`
	Username     string         `json:"username"`
	IPAddress    string         `json:"ip_address,omitempty"`
	UserAgent    string         `json:"user_agent,omitempty"`
	ActionType   string         `json:"action_type"`
	ResourceType string         `json:"resource_type"`
	ResourceID   *int           `json:"resource_id,omitempty"`
	ResourceName string         `json:"resource_name,omitempty"`
	Details      map[string]any `json:"details,omitempty"`
	Success      bool           `json:"success"`
	ErrorMessage string         `json:"error_message,omitempty"`
}

type auditLogStreamDTO struct {
	Entries     []auditLogDTO `json:"entries"`
	NextAfterID int           `json:"next_after_id"`
	HasMore     bool          `json:"has_more"`
}

func adminUserFilter(r *http.Request) (*int, error) {
	if value := r.URL.Query().Get("user_id"); value != "" {
		id, err := strconv.Atoi(value)
		if err != nil || id <= 0 {
			return nil, newError(http.StatusBadRequest, "invalid_request", "user_id must be a positive integer")
		}
		return &id, nil
	}
	return nil, nil
}

func listAdminAuditLogs(deps Deps) pageOperation[auditLogDTO] {
	return func(r *http.Request) ([]auditLogDTO, Pagination, int, error) {
		if _, err := requireSystemAdmin(r, deps); err != nil {
			return nil, Pagination{}, 0, err
		}
		page, err := ParsePage(r)
		if err != nil {
			return nil, page, 0, err
		}
		filters := repository.AuditLogFilters{ActionType: r.URL.Query().Get("action_type"), ResourceType: r.URL.Query().Get("resource_type")}
		filters.UserID, err = adminUserFilter(r)
		if err != nil {
			return nil, page, 0, err
		}
		for _, bound := range []struct {
			name   string
			target **time.Time
		}{{"from", &filters.From}, {"to", &filters.To}} {
			if value := r.URL.Query().Get(bound.name); value != "" {
				parsed, parseErr := time.Parse(time.RFC3339, value)
				if parseErr != nil {
					return nil, page, 0, newError(http.StatusBadRequest, "invalid_request", bound.name+" must be RFC3339")
				}
				*bound.target = &parsed
			}
		}
		rows, total, err := deps.AuditLogs.List(filters, page.Page, page.PageSize)
		if err != nil {
			return nil, page, 0, internalError(err)
		}
		return auditLogDTOs(rows), page, total, nil
	}
}

func streamAdminAuditLogs(deps Deps) readOperation[auditLogStreamDTO] {
	return func(r *http.Request) (auditLogStreamDTO, error) {
		if _, err := requireSystemAdmin(r, deps); err != nil {
			return auditLogStreamDTO{}, err
		}
		afterID := 0
		if value := r.URL.Query().Get("after_id"); value != "" {
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed < 0 {
				return auditLogStreamDTO{}, newError(http.StatusBadRequest, "invalid_request", "after_id must be a non-negative integer")
			}
			afterID = parsed
		}
		limit := 500
		if value := r.URL.Query().Get("limit"); value != "" {
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed < 1 {
				return auditLogStreamDTO{}, newError(http.StatusBadRequest, "invalid_request", "limit must be a positive integer")
			}
			limit = min(parsed, 1000)
		}
		rows, err := deps.AuditLogs.ListSince(afterID, limit)
		if err != nil {
			return auditLogStreamDTO{}, internalError(err)
		}
		next := afterID
		if len(rows) > 0 {
			next = rows[len(rows)-1].ID
		}
		return auditLogStreamDTO{Entries: auditLogDTOs(rows), NextAfterID: next, HasMore: len(rows) == limit}, nil
	}
}

func auditLogDTOs(rows []repository.AuditLogRow) []auditLogDTO {
	result := make([]auditLogDTO, len(rows))
	for i, row := range rows {
		result[i] = auditLogDTO{ID: row.ID, Timestamp: row.Timestamp.Format(time.RFC3339), UserID: row.UserID, Username: row.Username, IPAddress: row.IPAddress, UserAgent: row.UserAgent, ActionType: row.ActionType, ResourceType: row.ResourceType, ResourceID: row.ResourceID, ResourceName: row.ResourceName, Details: row.Details, Success: row.Success, ErrorMessage: row.ErrorMessage}
	}
	return result
}

func listAdminTokens(deps Deps) pageOperation[models.APIToken] {
	return func(r *http.Request) ([]models.APIToken, Pagination, int, error) {
		if _, err := requireSystemAdmin(r, deps); err != nil {
			return nil, Pagination{}, 0, err
		}
		page, err := ParsePage(r)
		if err != nil {
			return nil, page, 0, err
		}
		userID, err := adminUserFilter(r)
		if err != nil {
			return nil, page, 0, err
		}
		tokens, total, err := deps.AdminTokens.ListAllTokens(userID, page.PageSize, page.Offset)
		if err != nil {
			return nil, page, 0, internalError(err)
		}
		if tokens == nil {
			tokens = []models.APIToken{}
		}
		return tokens, page, total, nil
	}
}

func revokeAdminToken(deps Deps) commandOperation {
	return func(r *http.Request) error {
		actor, err := requireSystemAdmin(r, deps)
		if err != nil {
			return err
		}
		id, err := pathID(r, "token_id")
		if err != nil {
			return err
		}
		if err := deps.AdminTokens.AdminRevokeToken(id); err != nil {
			if errors.Is(err, auth.ErrTokenNotFound) {
				return newError(http.StatusNotFound, "not_found", "Token was not found")
			}
			return internalError(err)
		}
		deps.AdminAuditor.Log(r, actor, logger.ActionAPITokenAdminRevoke, logger.ResourceAPIToken, &id, "")
		return nil
	}
}
