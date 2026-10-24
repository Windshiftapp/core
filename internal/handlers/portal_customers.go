package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// PortalCustomersHandler handles portal customer management operations
type PortalCustomersHandler struct {
	db                    database.Database
	permService           *services.PermissionService
	customerOrgPermission *services.CustomerOrganisationPermissionService
	auditor               *logger.Auditor
}

// NewPortalCustomersHandler creates a new portal customers handler
func NewPortalCustomersHandler(db database.Database, permService *services.PermissionService, customerOrgPermission *services.CustomerOrganisationPermissionService) *PortalCustomersHandler {
	return &PortalCustomersHandler{db: db, permService: permService, customerOrgPermission: customerOrgPermission, auditor: logger.NewAuditor(db)}
}

// parseTimestamp parses a timestamp string from the database
func parseTimestamp(s string) (time.Time, error) { //nolint:unparam // error return kept for API consistency
	// Try multiple common timestamp formats
	formats := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05.000Z",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t, nil
		}
	}

	return time.Time{}, nil
}

//nolint:misspell // British spelling used in database (customer_organisations)
const portalCustomerBaseQuery = `
	SELECT
		pc.id, pc.name, pc.email, pc.phone,
		pc.user_id, pc.customer_organisation_id, pc.is_primary,
		pc.custom_field_values,
		pc.created_via,
		pc.deactivated_at,
		pc.created_at, pc.updated_at,
		u.first_name AS user_first_name,
		u.last_name AS user_last_name,
		u.email AS user_email,
		co.name AS customer_organisation_name
	FROM portal_customers pc
	LEFT JOIN users u ON pc.user_id = u.id
	LEFT JOIN customer_organisations co ON pc.customer_organisation_id = co.id`

// customFieldParseError wraps a JSON parse error for custom field values,
// allowing callers to distinguish it from scan errors.
type customFieldParseError struct {
	err error
}

func (e *customFieldParseError) Error() string {
	return fmt.Sprintf("parse custom field values: %s", e.err)
}

func (e *customFieldParseError) Unwrap() error {
	return e.err
}

// isCustomFieldParseError reports whether err is a custom field parse error.
func isCustomFieldParseError(err error) bool {
	var target *customFieldParseError
	return errors.As(err, &target)
}

// scanPortalCustomer scans a single portal customer row from a scanner (works with both *sql.Row and *sql.Rows).
func scanPortalCustomer(scanner interface{ Scan(...any) error }) (models.PortalCustomer, error) {
	var c models.PortalCustomer
	var phone sql.NullString
	var userFirstName, userLastName, userEmail, orgName sql.NullString
	var customFieldValuesStr sql.NullString
	var createdAtStr, updatedAtStr string
	var deactivatedAtStr sql.NullString
	var createdViaStr string

	err := scanner.Scan(
		&c.ID, &c.Name, &c.Email, &phone,
		&c.UserID, &c.CustomerOrganisationID, &c.IsPrimary,
		&customFieldValuesStr,
		&createdViaStr,
		&deactivatedAtStr,
		&createdAtStr, &updatedAtStr,
		&userFirstName, &userLastName, &userEmail, &orgName,
	)
	if err != nil {
		return c, err
	}

	// Parse timestamps
	if createdAt, err := parseTimestamp(createdAtStr); err == nil {
		c.CreatedAt = createdAt
	}
	if updatedAt, err := parseTimestamp(updatedAtStr); err == nil {
		c.UpdatedAt = updatedAt
	}
	if deactivatedAtStr.Valid && deactivatedAtStr.String != "" {
		if deactivatedAt, err := parseTimestamp(deactivatedAtStr.String); err == nil && !deactivatedAt.IsZero() {
			c.DeactivatedAt = &deactivatedAt
		}
	}

	// Populate nullable fields
	c.Phone = phone.String

	// Populate joined fields
	c.UserName = strings.TrimSpace(userFirstName.String + " " + userLastName.String)
	c.UserEmail = userEmail.String
	c.CustomerOrganisationName = orgName.String

	// Parse custom field values
	if customFieldValuesStr.Valid && customFieldValuesStr.String != "" {
		if err = json.Unmarshal([]byte(customFieldValuesStr.String), &c.CustomFieldValues); err != nil {
			return c, &customFieldParseError{err: err}
		}
	}

	c.CreatedVia = createdViaStr

	return c, nil
}

// loadPortalCustomerWithRoles fetches a single portal customer by ID and loads its roles.
func (h *PortalCustomersHandler) loadPortalCustomerWithRoles(id int64) (models.PortalCustomer, error) {
	row := h.db.QueryRow(portalCustomerBaseQuery+" WHERE pc.id = ?", id)
	c, err := scanPortalCustomer(row)
	if err != nil {
		return c, err
	}

	roles, err := h.loadPortalCustomerRoles(c.ID)
	if err != nil {
		slog.Warn("failed to load roles for customer", slog.String("component", "portal"), slog.Int("customer_id", c.ID), slog.Any("error", err))
		c.Roles = []models.ContactRole{}
	} else {
		c.Roles = roles
	}

	return c, nil
}

// GetPortalCustomers returns a list of all portal customers
func (h *PortalCustomersHandler) GetPortalCustomers(w http.ResponseWriter, r *http.Request) {
	slog.Debug("GetPortalCustomers called", slog.String("component", "portal"))

	rows, err := h.db.Query(portalCustomerBaseQuery + " ORDER BY pc.created_at DESC LIMIT 500")
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var customers []models.PortalCustomer
	var customerIDs []int
	for rows.Next() {
		c, err := scanPortalCustomer(rows)
		if err != nil {
			if isCustomFieldParseError(err) {
				slog.Warn("failed to parse custom field values for customer", slog.String("component", "portal"), slog.Int("customer_id", c.ID), slog.Any("error", err))
				continue
			}
			respondInternalError(w, r, err)
			return
		}
		customers = append(customers, c)
		customerIDs = append(customerIDs, c.ID)
	}
	if err := rows.Err(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Batched role lookup avoids the previous N+1 per customer.
	rolesByCustomer, err := h.loadRolesForCustomers(customerIDs)
	if err != nil {
		slog.Warn("failed to batch-load roles for portal customers", slog.String("component", "portal"), slog.Any("error", err))
		rolesByCustomer = map[int][]models.ContactRole{}
	}
	for i := range customers {
		if r, ok := rolesByCustomer[customers[i].ID]; ok {
			customers[i].Roles = r
		} else {
			customers[i].Roles = []models.ContactRole{}
		}
	}

	if customers == nil {
		customers = []models.PortalCustomer{}
	}

	respondJSONOK(w, customers)
}

// GetPortalCustomer returns a single portal customer by ID
func (h *PortalCustomersHandler) GetPortalCustomer(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	c, err := h.loadPortalCustomerWithRoles(int64(id))
	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "customer")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Customer-data access is audited (WI-1142): every detail view of a
	// customer record leaves a trail, matching the export/erase events.
	if user, ok := RequireAuth(w, r); ok {
		h.auditor.LogWithDetails(r, user, logger.ActionCustomerView,
			logger.ResourcePortalCustomer, &id, c.Email, nil)
	}

	respondJSONOK(w, c)
}

// GetCustomerChannels returns the channels a portal customer has access to
func (h *PortalCustomersHandler) GetCustomerChannels(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	customerID, err := strconv.Atoi(idStr)
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	query := `
		SELECT
			pcc.id, pcc.portal_customer_id, pcc.channel_id, pcc.created_at,
			c.name AS channel_name,
			c.type AS channel_type
		FROM portal_customer_channels pcc
		JOIN channels c ON pcc.channel_id = c.id
		WHERE pcc.portal_customer_id = ?
		ORDER BY pcc.created_at DESC
	`

	rows, err := h.db.Query(query, customerID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	type CustomerChannelAccess struct {
		ID               int    `json:"id"`
		PortalCustomerID int    `json:"portal_customer_id"`
		ChannelID        int    `json:"channel_id"`
		ChannelName      string `json:"channel_name"`
		ChannelType      string `json:"channel_type"`
		CreatedAt        string `json:"created_at"`
	}

	var channels []CustomerChannelAccess
	for rows.Next() {
		var ca CustomerChannelAccess
		err := rows.Scan(
			&ca.ID, &ca.PortalCustomerID, &ca.ChannelID, &ca.CreatedAt,
			&ca.ChannelName, &ca.ChannelType,
		)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		channels = append(channels, ca)
	}
	if err := rows.Err(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	if channels == nil {
		channels = []CustomerChannelAccess{}
	}

	respondJSONOK(w, channels)
}

// GetCustomerSubmissions returns the portal submissions by this customer that
// the caller can view. Submissions filed in workspaces the caller cannot view
// are omitted entirely rather than returned with redacted metadata.
func (h *PortalCustomersHandler) GetCustomerSubmissions(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	idStr := r.PathValue("id")
	customerID, err := strconv.Atoi(idStr)
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	accessibleWorkspaceIDs, err := GetAccessibleWorkspaceIDs(user, h.db, h.permService)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	type CustomerSubmission struct {
		ID                  int    `json:"id"`
		WorkspaceID         int    `json:"workspace_id"`
		WorkspaceItemNumber int    `json:"workspace_item_number"`
		WorkspaceName       string `json:"workspace_name"`
		WorkspaceKey        string `json:"workspace_key"`
		Title               string `json:"title"`
		Description         string `json:"description"`
		StatusName          string `json:"status_name"`
		StatusColor         string `json:"status_color"`
		CreatedAt           string `json:"created_at"`
	}

	rows, err := repository.NewItemRepository(h.db).ListPortalCustomerSubmissions(customerID, accessibleWorkspaceIDs)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	submissions := make([]CustomerSubmission, len(rows))
	for i, s := range rows {
		submissions[i] = CustomerSubmission{
			ID:                  s.ID,
			WorkspaceID:         s.WorkspaceID,
			WorkspaceItemNumber: s.WorkspaceItemNumber,
			WorkspaceName:       s.WorkspaceName,
			WorkspaceKey:        s.WorkspaceKey,
			Title:               s.Title,
			Description:         s.Description,
			StatusName:          s.StatusName,
			StatusColor:         s.StatusColor,
			CreatedAt:           s.CreatedAt,
		}
	}

	respondJSONOK(w, submissions)
}

// portalCustomerInput holds the decoded and validated input for create/update portal customer.
//
//nolint:misspell // API uses British spelling (customer_organisation_id)
type portalCustomerInput struct {
	Name                   string
	Email                  string
	Phone                  string
	CustomerOrganisationID *int
	IsPrimary              bool
	RoleIDs                []int
	CustomFieldValuesJSON  *string
}

// decodePortalCustomerInput decodes, validates, and serializes custom fields for portal customer create/update.
func decodePortalCustomerInput(w http.ResponseWriter, r *http.Request) (portalCustomerInput, bool) {
	//nolint:misspell // API uses British spelling (customer_organisation_id)
	var requestData struct {
		Name                   string         `json:"name"`
		Email                  string         `json:"email"`
		Phone                  string         `json:"phone"`
		CustomerOrganisationID *int           `json:"customer_organisation_id"`
		IsPrimary              bool           `json:"is_primary"`
		RoleIDs                []int          `json:"role_ids"`
		CustomFieldValues      map[string]any `json:"custom_field_values"`
	}

	if err := newJSONDecoder(w, r).Decode(&requestData); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return portalCustomerInput{}, false
	}

	requestData.Name = sanitize.ShortIdentifier.Sanitize(requestData.Name)
	requestData.Phone = sanitize.ShortIdentifier.Sanitize(requestData.Phone)

	if requestData.Name == "" {
		respondValidationError(w, r, "Name is required")
		return portalCustomerInput{}, false
	}
	if requestData.Email == "" {
		respondValidationError(w, r, "Email is required")
		return portalCustomerInput{}, false
	}

	var customFieldValuesJSON *string
	if len(requestData.CustomFieldValues) > 0 {
		b, err := json.Marshal(requestData.CustomFieldValues)
		if err != nil {
			respondBadRequest(w, r, "Invalid custom field values")
			return portalCustomerInput{}, false
		}
		s := string(b)
		customFieldValuesJSON = &s
	}

	return portalCustomerInput{
		Name:                   requestData.Name,
		Email:                  requestData.Email,
		Phone:                  requestData.Phone,
		CustomerOrganisationID: requestData.CustomerOrganisationID,
		IsPrimary:              requestData.IsPrimary,
		RoleIDs:                requestData.RoleIDs,
		CustomFieldValuesJSON:  customFieldValuesJSON,
	}, true
}

// CreatePortalCustomer creates a new portal customer
func (h *PortalCustomersHandler) CreatePortalCustomer(w http.ResponseWriter, r *http.Request) {
	input, ok := decodePortalCustomerInput(w, r)
	if !ok {
		return
	}

	// Wrap the insert + role assignment in a single transaction so a partial
	// failure (e.g. role assignment) does not leave a row without roles.
	var customerID int64
	txErr := database.WithTx(h.db, func(tx database.Tx) error {
		//nolint:misspell // database column uses British spelling
		err := tx.QueryRow(`
			INSERT INTO portal_customers (name, email, phone, customer_organisation_id, is_primary, custom_field_values, created_via, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) RETURNING id
		`, input.Name, input.Email, input.Phone, input.CustomerOrganisationID, input.IsPrimary, input.CustomFieldValuesJSON, models.CustomerCreatedViaAgent).Scan(&customerID)
		if err != nil {
			return err
		}

		// Resolve roles to assign. When the caller did not specify any, fall
		// back to the seeded "Portal Customer" default role. A missing seed
		// row is a deployment misconfiguration — fail the create rather than
		// silently creating a roleless customer.
		roleIDsToAssign := input.RoleIDs
		if len(roleIDsToAssign) == 0 {
			var defaultRoleID int
			if err := tx.QueryRow("SELECT id FROM contact_roles WHERE name = 'Portal Customer'").Scan(&defaultRoleID); err != nil {
				return fmt.Errorf("resolve default 'Portal Customer' contact_role: %w", err)
			}
			roleIDsToAssign = []int{defaultRoleID}
		}

		return h.assignRolesToPortalCustomerTx(tx, int(customerID), roleIDsToAssign)
	})
	if txErr != nil {
		if database.IsUniqueConstraintError(txErr) {
			respondConflict(w, r, "A portal customer with this email address already exists")
			return
		}
		slog.Error("failed to create portal customer", slog.String("component", "portal"), slog.Any("error", txErr))
		respondInternalError(w, r, txErr)
		return
	}

	// Fetch the created customer with joined data
	c, err := h.loadPortalCustomerWithRoles(customerID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if user := utils.GetCurrentUser(r); user != nil {
		id := c.ID
		logAudit(h.db, r, user, logger.ActionPortalCustomerCreate, logger.ResourcePortalCustomer, &id, c.Name)
	}

	respondJSONCreated(w, c)
}

// UpdatePortalCustomerOrganisation updates the customer organisation assignment for a portal customer
//
//nolint:misspell // British spelling used in API (Organisation)
func (h *PortalCustomersHandler) UpdatePortalCustomerOrganisation(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	customerID, err := strconv.Atoi(idStr)
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	//nolint:misspell // British spelling used in API (customer_organisation_id)
	var requestData struct {
		CustomerOrganisationID *int `json:"customer_organisation_id"`
	}

	if err = newJSONDecoder(w, r).Decode(&requestData); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	//nolint:misspell // British spelling used in database (customer_organisation_id)
	// Update the customer organisation assignment. Erased tombstones are
	// immutable: a re-identification write must not resurrect them.
	query := `UPDATE portal_customers SET customer_organisation_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND erased_at IS NULL`
	if !h.execCustomerWrite(w, r, query, requestData.CustomerOrganisationID, customerID) {
		return
	}

	if user := utils.GetCurrentUser(r); user != nil {
		logAudit(h.db, r, user, logger.ActionPortalCustomerUpdateOrg, logger.ResourcePortalCustomer, &customerID, "")
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// UpdatePortalCustomer updates all fields of a portal customer
func (h *PortalCustomersHandler) UpdatePortalCustomer(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	customerID, err := strconv.Atoi(idStr)
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	input, ok := decodePortalCustomerInput(w, r)
	if !ok {
		return
	}

	// Wrap update + role assignment in a transaction so partial failures
	// don't leave the customer in a half-updated state.
	txErr := database.WithTx(h.db, func(tx database.Tx) error {
		//nolint:misspell // customer_organisation_id is a database column name
		query := `
			UPDATE portal_customers
			SET name = ?, email = ?, phone = ?, customer_organisation_id = ?, is_primary = ?, custom_field_values = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ? AND erased_at IS NULL
		`
		result, err := tx.Exec(query, input.Name, input.Email, input.Phone, input.CustomerOrganisationID, input.IsPrimary, input.CustomFieldValuesJSON, customerID)
		if err != nil {
			return err
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rowsAffected == 0 {
			return repository.ErrNotFound
		}

		if input.RoleIDs != nil {
			return h.assignRolesToPortalCustomerTx(tx, customerID, input.RoleIDs)
		}
		return nil
	})
	if errors.Is(txErr, repository.ErrNotFound) {
		respondNotFound(w, r, "customer")
		return
	}
	if txErr != nil {
		if database.IsUniqueConstraintError(txErr) {
			respondConflict(w, r, "A portal customer with this email address already exists")
			return
		}
		slog.Error("failed to update portal customer", slog.String("component", "portal"), slog.Any("error", txErr))
		respondInternalError(w, r, txErr)
		return
	}

	// Fetch and return the updated customer
	c, err := h.loadPortalCustomerWithRoles(int64(customerID))
	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "customer")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if user := utils.GetCurrentUser(r); user != nil {
		id := c.ID
		logAudit(h.db, r, user, logger.ActionPortalCustomerUpdate, logger.ResourcePortalCustomer, &id, c.Name)
	}

	respondJSONOK(w, c)
}

// execCustomerWrite runs a single-row portal customer write and writes the
// error responses itself, including a 404 when no row matched.
func (h *PortalCustomersHandler) execCustomerWrite(w http.ResponseWriter, r *http.Request, query string, args ...any) bool {
	result, err := h.db.ExecWrite(query, args...)
	if err != nil {
		respondInternalError(w, r, err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		respondInternalError(w, r, err)
		return false
	}
	if rowsAffected == 0 {
		respondNotFound(w, r, "customer")
		return false
	}
	return true
}

// PortalCustomerErasureRequest is the DSAR intake payload for
// POST /portal-customers/{id}/erase.
type PortalCustomerErasureRequest struct {
	// RequestedBy records where the erasure request came from — the data
	// subject's email or an intake-channel reference. Required.
	RequestedBy string `json:"requested_by"`
	// RequestedAt is when the controller received the request; defaults to now.
	RequestedAt *time.Time `json:"requested_at,omitempty"`
	// Notes optionally records the controller's decision context.
	Notes string `json:"notes,omitempty"`
}

// ErasePortalCustomer executes an irreversible Article 17 erasure (DSAR).
// The customer row is pseudonymized, never deleted, so customer-authored
// comments, items, item history, attachments, and approval decisions stay
// interpretable. Distinct from the historical hard delete: erasure is a
// documented data-subject right execution, not an administrative cleanup.
func (h *PortalCustomersHandler) ErasePortalCustomer(w http.ResponseWriter, r *http.Request) {
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	body, ok := decodeJSON[PortalCustomerErasureRequest](w, r)
	if !ok {
		return
	}

	input := services.CustomerErasureInput{RequestedBy: body.RequestedBy, Notes: body.Notes}
	if body.RequestedAt != nil {
		input.RequestedAt = *body.RequestedAt
	}
	h.eraseCustomer(w, r, currentUser, id, input)
}

// DeletePortalCustomer is retained as a DELETE alias for erasure so existing
// admin UI flows keep working. The controller decision is the authenticated
// customer manager's action; the intake reference is derived from the actor
// because the legacy endpoint carries no DSAR body.
func (h *PortalCustomersHandler) DeletePortalCustomer(w http.ResponseWriter, r *http.Request) {
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	intake := fmt.Sprintf("admin:%s via customer management", currentUser.Username)
	h.eraseCustomer(w, r, currentUser, id, services.CustomerErasureInput{RequestedBy: intake})
}

// ExportPortalCustomer serves the Article 15/20 data export for one customer
// as a JSON download. The payload structure is pinned by the service's schema
// version so repeated DSAR exports stay comparable.
func (h *PortalCustomersHandler) ExportPortalCustomer(w http.ResponseWriter, r *http.Request) {
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	accessibleWorkspaceIDs, err := GetAccessibleWorkspaceIDs(currentUser, h.db, h.permService)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	actor := services.NewAuditActorFromRequest(r, currentUser, nil, "")
	export, err := services.ExportCustomerData(h.db, id, actor, accessibleWorkspaceIDs)
	if err != nil {
		if se, ok := err.(*services.ServiceError); ok {
			handleServiceError(w, r, se)
			return
		}
		slog.Error("failed to export portal customer data", slog.String("component", "portal"), slog.Int("customer_id", id), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	payload, err := json.Marshal(export)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	filename := fmt.Sprintf("customer-data-export-%d.json", id)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

// DeactivatePortalCustomer cuts portal access for a customer without
// exercising the erasure right: it stamps deactivated_at, invalidates every
// live session, and records the decision with the acting admin. All portal
// auth paths (session validation, magic-link issuance/redemption, passkey
// login) refuse deactivated customers.
func (h *PortalCustomersHandler) DeactivatePortalCustomer(w http.ResponseWriter, r *http.Request) {
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var name, email string
	var deactivatedAt, erasedAt sql.NullString
	if err := h.db.QueryRow(`SELECT name, email, deactivated_at, erased_at FROM portal_customers WHERE id = ?`, id).
		Scan(&name, &email, &deactivatedAt, &erasedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondNotFound(w, r, "customer")
			return
		}
		respondInternalError(w, r, err)
		return
	}
	if deactivatedAt.Valid && deactivatedAt.String != "" {
		respondConflict(w, r, "Portal customer is already deactivated")
		return
	}
	if erasedAt.Valid && erasedAt.String != "" {
		respondConflict(w, r, "Portal customer has been erased and cannot be deactivated")
		return
	}

	deactivated := time.Now()
	if _, err := h.db.ExecWrite(`UPDATE portal_customers SET deactivated_at = ?, updated_at = ? WHERE id = ?`, deactivated, deactivated, id); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Invalidate every live session immediately; validation would refuse
	// them anyway, but deletion removes the stored ip/user_agent access
	// surface right away.
	if _, err := h.db.ExecWrite(`DELETE FROM portal_customer_sessions WHERE portal_customer_id = ?`, id); err != nil {
		slog.Error("failed to invalidate portal sessions on deactivation", slog.String("component", "portal"), slog.Int("customer_id", id), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	if currentUser != nil {
		logAuditWithDetails(h.db, r, currentUser, logger.ActionPortalCustomerDeactivate, logger.ResourcePortalCustomer, &id, name, map[string]any{
			"email": email,
		})
	}

	c, err := h.loadPortalCustomerWithRoles(int64(id))
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, c)
}

// ActivatePortalCustomer re-grants portal access to a deactivated customer.
// Reactivation is an explicit admin decision, distinct from erasure — erased
// customers are never reactivatable.
func (h *PortalCustomersHandler) ActivatePortalCustomer(w http.ResponseWriter, r *http.Request) {
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var name, email string
	var deactivatedAt, erasedAt sql.NullString
	if err := h.db.QueryRow(`SELECT name, email, deactivated_at, erased_at FROM portal_customers WHERE id = ?`, id).
		Scan(&name, &email, &deactivatedAt, &erasedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondNotFound(w, r, "customer")
			return
		}
		respondInternalError(w, r, err)
		return
	}
	if !deactivatedAt.Valid || deactivatedAt.String == "" {
		respondValidationError(w, r, "Portal customer is not deactivated")
		return
	}
	if erasedAt.Valid && erasedAt.String != "" {
		respondConflict(w, r, "Portal customer has been erased and cannot be reactivated")
		return
	}

	res, err := h.db.ExecWrite(`UPDATE portal_customers SET deactivated_at = NULL, updated_at = ? WHERE id = ? AND erased_at IS NULL`, time.Now(), id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		respondConflict(w, r, "Portal customer has been erased and cannot be reactivated")
		return
	}

	if currentUser != nil {
		logAuditWithDetails(h.db, r, currentUser, logger.ActionPortalCustomerActivate, logger.ResourcePortalCustomer, &id, name, map[string]any{
			"email": email,
		})
	}

	c, err := h.loadPortalCustomerWithRoles(int64(id))
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, c)
}

// PortalCustomerBulkCleanupRequest selects the stale auto-created customers
// a cleanup run targets.
type PortalCustomerBulkCleanupRequest struct {
	// CreatedVia optionally filters by provenance (agent, email-intake,
	// magic-link, ticket-import, unknown). Empty = any.
	CreatedVia string `json:"created_via,omitempty"`
	// OlderThanDays optionally restricts to customers created more than this
	// many days ago. 0 = no age filter.
	OlderThanDays int `json:"older_than_days,omitempty"`
}

// BulkCleanupPortalCustomers erases stale auto-created customers (spam and
// one-off email senders that never became tickets) through the standard
// erasure flow, one DSAR evidence row each, plus a single batch audit event.
func (h *PortalCustomersHandler) BulkCleanupPortalCustomers(w http.ResponseWriter, r *http.Request) {
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	body, ok := decodeJSON[PortalCustomerBulkCleanupRequest](w, r)
	if !ok {
		return
	}

	actor := services.NewAuditActorFromRequest(r, currentUser, nil, "")
	result, err := services.CleanupAutoCreatedCustomers(h.db, actor, services.CustomerBulkCleanupInput{
		CreatedVia:    body.CreatedVia,
		OlderThanDays: body.OlderThanDays,
	})
	if err != nil {
		if se, ok := err.(*services.ServiceError); ok {
			handleServiceError(w, r, se)
			return
		}
		slog.Error("failed to run portal customer bulk cleanup", slog.String("component", "portal"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, result)
}

// eraseCustomer runs the shared erasure execution and writes the response:
// 201 with the DSAR evidence on success, 409 on repeat erasure, 404 for an
// unknown customer, and mapped service errors otherwise.
func (h *PortalCustomersHandler) eraseCustomer(w http.ResponseWriter, r *http.Request, currentUser *models.User, customerID int, input services.CustomerErasureInput) {
	actor := services.NewAuditActorFromRequest(r, currentUser, nil, "")
	evidence, err := services.EraseCustomer(h.db, customerID, actor, input)
	if err != nil {
		if errors.Is(err, services.ErrCustomerAlreadyErased) {
			respondConflict(w, r, "Portal customer has already been erased")
			return
		}
		if se, ok := err.(*services.ServiceError); ok {
			handleServiceError(w, r, se)
			return
		}
		slog.Error("failed to erase portal customer", slog.String("component", "portal"), slog.Int("customer_id", customerID), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	respondJSONCreated(w, evidence)
}

// GetOrganisationContacts returns all portal customers (contacts) for a given customer organisation
//
//nolint:misspell // "organisation" is intentional British spelling used throughout codebase
func (h *PortalCustomersHandler) GetOrganisationContacts(w http.ResponseWriter, r *http.Request) {
	_, orgID, ok := h.requireOrgViewAccess(w, r)
	if !ok {
		return
	}

	//nolint:misspell // "organisation" is intentional British spelling used throughout codebase
	rows, err := h.db.Query(portalCustomerBaseQuery+" WHERE pc.customer_organisation_id = ? ORDER BY pc.is_primary DESC, pc.created_at DESC LIMIT 500", orgID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var contacts []models.PortalCustomer
	var contactIDs []int
	for rows.Next() {
		c, err := scanPortalCustomer(rows)
		if err != nil {
			if isCustomFieldParseError(err) {
				// Log error but continue with other contacts
				slog.Warn("failed to parse custom field values for contact", slog.String("component", "portal"), slog.Int("contact_id", c.ID), slog.Any("error", err))
				continue
			}
			respondInternalError(w, r, err)
			return
		}
		contacts = append(contacts, c)
		contactIDs = append(contactIDs, c.ID)
	}
	if err := rows.Err(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	rolesByContact, err := h.loadRolesForCustomers(contactIDs)
	if err != nil {
		slog.Warn("failed to batch-load roles for org contacts", slog.String("component", "portal"), slog.Any("error", err))
		rolesByContact = map[int][]models.ContactRole{}
	}
	for i := range contacts {
		if r, ok := rolesByContact[contacts[i].ID]; ok {
			contacts[i].Roles = r
		} else {
			contacts[i].Roles = []models.ContactRole{}
		}
	}

	if contacts == nil {
		contacts = []models.PortalCustomer{}
	}

	respondJSONOK(w, contacts)
}

// GetOrganisationTickets returns all work items created by contacts belonging to a customer organisation,
// filtered by the requesting user's workspace permissions.
//
// requireOrgViewAccess resolves the organisation ID from the path and checks
// the caller's organisation view permission. Writes the auth/ACL responses
// itself and returns the authenticated user for follow-up queries.
//
//nolint:misspell // "organisation" is intentional British spelling used throughout codebase
func (h *PortalCustomersHandler) requireOrgViewAccess(w http.ResponseWriter, r *http.Request) (user *models.User, orgID int, ok bool) {
	user, ok = RequireAuth(w, r)
	if !ok {
		return nil, 0, false
	}

	orgID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return nil, 0, false
	}

	if h.customerOrgPermission != nil {
		canView, err := h.customerOrgPermission.CanView(user.ID, orgID)
		if err != nil {
			respondInternalError(w, r, err)
			return nil, 0, false
		}
		if !canView {
			respondForbidden(w, r)
			return nil, 0, false
		}
	}
	return user, orgID, true
}

// GetOrganisationTickets returns all work items created by contacts belonging to a customer organisation,
// filtered by the requesting user's workspace permissions.
//
//nolint:misspell // "organisation" is intentional British spelling used throughout codebase
func (h *PortalCustomersHandler) GetOrganisationTickets(w http.ResponseWriter, r *http.Request) {
	user, orgID, ok := h.requireOrgViewAccess(w, r)
	if !ok {
		return
	}

	// Org ACL authorises reading the org's tickets; workspace visibility then
	// limits which of those tickets are returned, so an org member never sees a
	// ticket filed in a workspace they cannot view.
	accessibleWorkspaceIDs, err := GetAccessibleWorkspaceIDs(user, h.db, h.permService)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	type OrgTicket struct {
		ID                  int    `json:"id"`
		WorkspaceID         int    `json:"workspace_id"`
		WorkspaceItemNumber int    `json:"workspace_item_number"`
		Title               string `json:"title"`
		CreatedAt           string `json:"created_at"`
		WorkspaceName       string `json:"workspace_name"`
		WorkspaceKey        string `json:"workspace_key"`
		StatusName          string `json:"status_name"`
		StatusColor         string `json:"status_color"`
		CreatorContactName  string `json:"creator_contact_name"`
		CreatorContactEmail string `json:"creator_contact_email"`
	}

	rows, err := repository.NewItemRepository(h.db).ListOrganisationTickets(orgID, accessibleWorkspaceIDs)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	tickets := make([]OrgTicket, len(rows))
	for i, t := range rows {
		tickets[i] = OrgTicket{
			ID:                  t.ID,
			WorkspaceID:         t.WorkspaceID,
			WorkspaceItemNumber: t.WorkspaceItemNumber,
			Title:               t.Title,
			CreatedAt:           t.CreatedAt,
			WorkspaceName:       t.WorkspaceName,
			WorkspaceKey:        t.WorkspaceKey,
			StatusName:          t.StatusName,
			StatusColor:         t.StatusColor,
			CreatorContactName:  t.CreatorContactName,
			CreatorContactEmail: t.CreatorContactEmail,
		}
	}

	respondJSONOK(w, tickets)
}

// loadPortalCustomerRoles loads the contact roles for a given portal customer
func (h *PortalCustomersHandler) loadPortalCustomerRoles(customerID int) ([]models.ContactRole, error) {
	query := `
		SELECT cr.id, cr.name, cr.description, cr.is_system, cr.sort_order, cr.created_at
		FROM contact_roles cr
		JOIN portal_customer_roles pcr ON cr.id = pcr.contact_role_id
		WHERE pcr.portal_customer_id = ?
		ORDER BY cr.is_system DESC, cr.sort_order ASC, cr.name ASC
	`

	rows, err := h.db.Query(query, customerID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var roles []models.ContactRole
	for rows.Next() {
		var role models.ContactRole
		var createdAtStr string

		err := rows.Scan(&role.ID, &role.Name, &role.Description, &role.IsSystem, &role.SortOrder, &createdAtStr)
		if err != nil {
			return nil, err
		}

		if t, err := parseTimestamp(createdAtStr); err == nil {
			role.CreatedAt = t
		}

		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if roles == nil {
		roles = []models.ContactRole{}
	}

	return roles, nil
}

// loadRolesForCustomers batches role-lookups for many customers into a single
// query. Replaces the previous per-customer N+1 in list endpoints. Returns a
// map keyed by customer id; callers should default to []ContactRole{} for any
// id absent from the map.
func (h *PortalCustomersHandler) loadRolesForCustomers(customerIDs []int) (map[int][]models.ContactRole, error) {
	out := make(map[int][]models.ContactRole, len(customerIDs))
	if len(customerIDs) == 0 {
		return out, nil
	}

	placeholders := make([]string, len(customerIDs))
	args := make([]any, len(customerIDs))
	for i, id := range customerIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := `
		SELECT pcr.portal_customer_id, cr.id, cr.name, cr.description, cr.is_system, cr.sort_order, cr.created_at
		FROM contact_roles cr
		JOIN portal_customer_roles pcr ON cr.id = pcr.contact_role_id
		WHERE pcr.portal_customer_id IN (` + strings.Join(placeholders, ",") + `)
		ORDER BY pcr.portal_customer_id, cr.is_system DESC, cr.sort_order ASC, cr.name ASC
	`

	rows, err := h.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var customerID int
		var role models.ContactRole
		var createdAtStr string
		if err := rows.Scan(&customerID, &role.ID, &role.Name, &role.Description, &role.IsSystem, &role.SortOrder, &createdAtStr); err != nil {
			return nil, err
		}
		if t, err := parseTimestamp(createdAtStr); err == nil {
			role.CreatedAt = t
		}
		out[customerID] = append(out[customerID], role)
	}
	return out, rows.Err()
}

// assignRolesToPortalCustomerTx replaces the role assignments for a portal
// customer within an open transaction. Callers wrap this together with the
// customer write so a role-assignment failure rolls back the whole change.
func (h *PortalCustomersHandler) assignRolesToPortalCustomerTx(tx database.Tx, customerID int, roleIDs []int) error {
	if _, err := tx.Exec(`DELETE FROM portal_customer_roles WHERE portal_customer_id = ?`, customerID); err != nil {
		return fmt.Errorf("clear roles for portal customer %d: %w", customerID, err)
	}
	if len(roleIDs) == 0 {
		return nil
	}
	insertQuery := `INSERT INTO portal_customer_roles (portal_customer_id, contact_role_id) VALUES (?, ?)`
	for _, roleID := range roleIDs {
		if _, err := tx.Exec(insertQuery, customerID, roleID); err != nil {
			return fmt.Errorf("assign role %d to portal customer %d: %w", roleID, customerID, err)
		}
	}
	return nil
}
