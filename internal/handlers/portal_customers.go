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
	"windshift/internal/services"
	"windshift/internal/utils"
)

// PortalCustomersHandler handles portal customer management operations
type PortalCustomersHandler struct {
	db                    database.Database
	permService           *services.PermissionService
	customerOrgPermission *services.CustomerOrganisationPermissionService
}

// NewPortalCustomersHandler creates a new portal customers handler
func NewPortalCustomersHandler(db database.Database, permService *services.PermissionService, customerOrgPermission *services.CustomerOrganisationPermissionService) *PortalCustomersHandler {
	return &PortalCustomersHandler{db: db, permService: permService, customerOrgPermission: customerOrgPermission}
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
func scanPortalCustomer(scanner interface{ Scan(...interface{}) error }) (models.PortalCustomer, error) {
	var c models.PortalCustomer
	var phone sql.NullString
	var userFirstName, userLastName, userEmail, orgName sql.NullString
	var customFieldValuesStr sql.NullString
	var createdAtStr, updatedAtStr string

	err := scanner.Scan(
		&c.ID, &c.Name, &c.Email, &phone,
		&c.UserID, &c.CustomerOrganisationID, &c.IsPrimary,
		&customFieldValuesStr,
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
				// Log error but continue with other customers
				continue
			}
			respondInternalError(w, r, err)
			return
		}
		customers = append(customers, c)
		customerIDs = append(customerIDs, c.ID)
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

	if channels == nil {
		channels = []CustomerChannelAccess{}
	}

	respondJSONOK(w, channels)
}

// GetCustomerSubmissions returns all portal submissions by this customer
func (h *PortalCustomersHandler) GetCustomerSubmissions(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	customerID, err := strconv.Atoi(idStr)
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	type CustomerSubmission struct {
		ID            int    `json:"id"`
		WorkspaceID   int    `json:"workspace_id"`
		WorkspaceName string `json:"workspace_name"`
		WorkspaceKey  string `json:"workspace_key"`
		Title         string `json:"title"`
		Description   string `json:"description"`
		StatusName    string `json:"status_name"`
		StatusColor   string `json:"status_color"`
		CreatedAt     string `json:"created_at"`
	}

	rows, err := repository.NewItemRepository(h.db).ListPortalCustomerSubmissions(customerID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	submissions := make([]CustomerSubmission, len(rows))
	for i, s := range rows {
		submissions[i] = CustomerSubmission{
			ID:            s.ID,
			WorkspaceID:   s.WorkspaceID,
			WorkspaceName: s.WorkspaceName,
			WorkspaceKey:  s.WorkspaceKey,
			Title:         s.Title,
			Description:   s.Description,
			StatusName:    s.StatusName,
			StatusColor:   s.StatusColor,
			CreatedAt:     s.CreatedAt,
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
		Name                   string                 `json:"name"`
		Email                  string                 `json:"email"`
		Phone                  string                 `json:"phone"`
		CustomerOrganisationID *int                   `json:"customer_organisation_id"`
		IsPrimary              bool                   `json:"is_primary"`
		RoleIDs                []int                  `json:"role_ids"`
		CustomFieldValues      map[string]interface{} `json:"custom_field_values"`
	}

	if err := json.NewDecoder(r.Body).Decode(&requestData); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return portalCustomerInput{}, false
	}

	requestData.Name = utils.SanitizeName(requestData.Name)
	requestData.Phone = utils.SanitizeName(requestData.Phone)

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
			INSERT INTO portal_customers (name, email, phone, customer_organisation_id, is_primary, custom_field_values, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) RETURNING id
		`, input.Name, input.Email, input.Phone, input.CustomerOrganisationID, input.IsPrimary, input.CustomFieldValuesJSON).Scan(&customerID)
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

	if err = json.NewDecoder(r.Body).Decode(&requestData); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	//nolint:misspell // British spelling used in database (customer_organisation_id)
	// Update the customer organisation assignment
	query := `UPDATE portal_customers SET customer_organisation_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	result, err := h.db.ExecWrite(query, requestData.CustomerOrganisationID, customerID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if rowsAffected == 0 {
		respondNotFound(w, r, "customer")
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
			WHERE id = ?
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

// DeletePortalCustomer deletes a portal customer
func (h *PortalCustomersHandler) DeletePortalCustomer(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	// Delete the portal customer
	query := `DELETE FROM portal_customers WHERE id = ?`
	result, err := h.db.ExecWrite(query, id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if rowsAffected == 0 {
		respondNotFound(w, r, "customer")
		return
	}

	if user := utils.GetCurrentUser(r); user != nil {
		logAudit(h.db, r, user, logger.ActionPortalCustomerDelete, logger.ResourcePortalCustomer, &id, "")
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetOrganisationContacts returns all portal customers (contacts) for a given customer organisation
//
//nolint:misspell // "organisation" is intentional British spelling used throughout codebase
func (h *PortalCustomersHandler) GetOrganisationContacts(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	idStr := r.PathValue("id")
	orgID, err := strconv.Atoi(idStr)
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	if h.customerOrgPermission != nil {
		canView, err := h.customerOrgPermission.CanView(user.ID, orgID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !canView {
			respondForbidden(w, r)
			return
		}
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
//nolint:misspell // "organisation" is intentional British spelling used throughout codebase
func (h *PortalCustomersHandler) GetOrganisationTickets(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	idStr := r.PathValue("id")
	orgID, err := strconv.Atoi(idStr)
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	if h.customerOrgPermission != nil {
		canView, err := h.customerOrgPermission.CanView(user.ID, orgID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !canView {
			respondForbidden(w, r)
			return
		}
	}

	wsIDs, err := GetAccessibleWorkspaceIDs(user, h.db, h.permService)
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

	if len(wsIDs) == 0 {
		respondJSONOK(w, []OrgTicket{})
		return
	}

	rows, err := repository.NewItemRepository(h.db).ListOrganisationTickets(orgID, wsIDs)
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
		SELECT cr.id, cr.name, cr.description, cr.is_system, cr.created_at
		FROM contact_roles cr
		JOIN portal_customer_roles pcr ON cr.id = pcr.contact_role_id
		WHERE pcr.portal_customer_id = ?
		ORDER BY cr.is_system DESC, cr.name ASC
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

		err := rows.Scan(&role.ID, &role.Name, &role.Description, &role.IsSystem, &createdAtStr)
		if err != nil {
			return nil, err
		}

		if t, err := parseTimestamp(createdAtStr); err == nil {
			role.CreatedAt = t
		}

		roles = append(roles, role)
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
	args := make([]interface{}, len(customerIDs))
	for i, id := range customerIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := `
		SELECT pcr.portal_customer_id, cr.id, cr.name, cr.description, cr.is_system, cr.created_at
		FROM contact_roles cr
		JOIN portal_customer_roles pcr ON cr.id = pcr.contact_role_id
		WHERE pcr.portal_customer_id IN (` + strings.Join(placeholders, ",") + `)
		ORDER BY pcr.portal_customer_id, cr.is_system DESC, cr.name ASC
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
		if err := rows.Scan(&customerID, &role.ID, &role.Name, &role.Description, &role.IsSystem, &createdAtStr); err != nil {
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
