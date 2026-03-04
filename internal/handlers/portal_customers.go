package handlers

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// PortalCustomersHandler handles portal customer management operations
type PortalCustomersHandler struct {
	db database.Database
}

// NewPortalCustomersHandler creates a new portal customers handler
func NewPortalCustomersHandler(db database.Database) *PortalCustomersHandler {
	return &PortalCustomersHandler{db: db}
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

// GetPortalCustomers returns a list of all portal customers
func (h *PortalCustomersHandler) GetPortalCustomers(w http.ResponseWriter, r *http.Request) {
	slog.Debug("GetPortalCustomers called", slog.String("component", "portal"))

	//nolint:misspell // British spelling used in database
	query := `
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
		LEFT JOIN customer_organisations co ON pc.customer_organisation_id = co.id
		ORDER BY pc.created_at DESC
	`

	rows, err := h.db.Query(query)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var customers []models.PortalCustomer
	for rows.Next() {
		var c models.PortalCustomer
		var phone sql.NullString
		var userFirstName, userLastName, userEmail, orgName sql.NullString
		var customFieldValuesStr sql.NullString
		var createdAtStr, updatedAtStr string

		err := rows.Scan(
			&c.ID, &c.Name, &c.Email, &phone,
			&c.UserID, &c.CustomerOrganisationID, &c.IsPrimary,
			&customFieldValuesStr,
			&createdAtStr, &updatedAtStr,
			&userFirstName, &userLastName, &userEmail, &orgName,
		)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		// Parse timestamps
		var createdAt time.Time
		if createdAt, err = parseTimestamp(createdAtStr); err == nil {
			c.CreatedAt = createdAt
		}
		var updatedAt time.Time
		if updatedAt, err = parseTimestamp(updatedAtStr); err == nil {
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
				// Log error but continue with other customers
				continue
			}
		}

		// Load roles for this customer
		roles, err := h.loadPortalCustomerRoles(c.ID)
		if err != nil {
			slog.Warn("failed to load roles for customer", slog.String("component", "portal"), slog.Int("customer_id", c.ID), slog.Any("error", err))
			// Continue without roles rather than failing entirely
			c.Roles = []models.ContactRole{}
		} else {
			c.Roles = roles
		}

		customers = append(customers, c)
	}

	if customers == nil {
		customers = []models.PortalCustomer{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(customers)
}

// GetPortalCustomer returns a single portal customer by ID
func (h *PortalCustomersHandler) GetPortalCustomer(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	//nolint:misspell // database uses British spelling (customer_organisation)
	query := `
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
		LEFT JOIN customer_organisations co ON pc.customer_organisation_id = co.id
		WHERE pc.id = ?
	`

	var c models.PortalCustomer
	var phone sql.NullString
	var userFirstName, userLastName, userEmail, orgName sql.NullString
	var customFieldValuesStr sql.NullString
	var createdAtStr, updatedAtStr string

	err = h.db.QueryRow(query, id).Scan(
		&c.ID, &c.Name, &c.Email, &phone,
		&c.UserID, &c.CustomerOrganisationID, &c.IsPrimary,
		&customFieldValuesStr,
		&createdAtStr, &updatedAtStr,
		&userFirstName, &userLastName, &userEmail, &orgName,
	)

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "customer")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Parse timestamps
	var createdAt time.Time
	if createdAt, err = parseTimestamp(createdAtStr); err == nil {
		c.CreatedAt = createdAt
	}
	var updatedAt time.Time
	if updatedAt, err = parseTimestamp(updatedAtStr); err == nil {
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
			respondInternalError(w, r, err)
			return
		}
	}

	// Load roles for this customer
	roles, err := h.loadPortalCustomerRoles(c.ID)
	if err != nil {
		slog.Warn("failed to load roles for customer", slog.String("component", "portal"), slog.Int("customer_id", c.ID), slog.Any("error", err))
		c.Roles = []models.ContactRole{}
	} else {
		c.Roles = roles
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(c)
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

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(channels)
}

// GetCustomerSubmissions returns all portal submissions by this customer
func (h *PortalCustomersHandler) GetCustomerSubmissions(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	customerID, err := strconv.Atoi(idStr)
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	query := `
		SELECT
			i.id, i.workspace_id, i.title, i.description,
			i.status_id, i.created_at,
			w.name AS workspace_name,
			w.key AS workspace_key
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		WHERE i.creator_portal_customer_id = ?
		ORDER BY i.created_at DESC
	`

	rows, err := h.db.Query(query, customerID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	type CustomerSubmission struct {
		ID            int    `json:"id"`
		WorkspaceID   int    `json:"workspace_id"`
		WorkspaceName string `json:"workspace_name"`
		WorkspaceKey  string `json:"workspace_key"`
		Title         string `json:"title"`
		Description   string `json:"description"`
		Status        string `json:"status"`
		CreatedAt     string `json:"created_at"`
	}

	var submissions []CustomerSubmission
	for rows.Next() {
		var s CustomerSubmission
		err := rows.Scan(
			&s.ID, &s.WorkspaceID, &s.Title, &s.Description,
			&s.Status, &s.CreatedAt,
			&s.WorkspaceName, &s.WorkspaceKey,
		)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		submissions = append(submissions, s)
	}

	if submissions == nil {
		submissions = []CustomerSubmission{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(submissions)
}

// CreatePortalCustomer creates a new portal customer
func (h *PortalCustomersHandler) CreatePortalCustomer(w http.ResponseWriter, r *http.Request) {
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
		return
	}

	// Validate required fields
	if requestData.Name == "" {
		respondValidationError(w, r, "Name is required")
		return
	}
	if requestData.Email == "" {
		respondValidationError(w, r, "Email is required")
		return
	}

	// Serialize custom field values to JSON
	var customFieldValuesJSON []byte
	if len(requestData.CustomFieldValues) > 0 {
		var err error
		customFieldValuesJSON, err = json.Marshal(requestData.CustomFieldValues)
		if err != nil {
			respondBadRequest(w, r, "Invalid custom field values")
			return
		}
	}

	// Insert the new portal customer
	var customerID int64
	//nolint:misspell // database column uses British spelling
	err := h.db.QueryRow(`
		INSERT INTO portal_customers (name, email, phone, customer_organisation_id, is_primary, custom_field_values, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) RETURNING id
	`, requestData.Name, requestData.Email, requestData.Phone, requestData.CustomerOrganisationID, requestData.IsPrimary, customFieldValuesJSON).Scan(&customerID)
	if err != nil {
		// Check for unique constraint violation on email
		if strings.Contains(err.Error(), "UNIQUE constraint failed: portal_customers.email") || strings.Contains(err.Error(), "duplicate key") {
			respondConflict(w, r, "A portal customer with this email address already exists")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Assign roles to the new customer (if no roles provided, assign default "Portal Customer" role)
	roleIDsToAssign := requestData.RoleIDs
	if len(roleIDsToAssign) == 0 {
		// Get the default "Portal Customer" role ID
		var defaultRoleID int
		err = h.db.QueryRow("SELECT id FROM contact_roles WHERE name = 'Portal Customer'").Scan(&defaultRoleID)
		if err == nil {
			roleIDsToAssign = []int{defaultRoleID}
		}
	}

	if len(roleIDsToAssign) > 0 {
		err = h.assignRolesToPortalCustomer(int(customerID), roleIDsToAssign)
		if err != nil {
			slog.Warn("failed to assign roles to portal customer", slog.String("component", "portal"), slog.Any("error", err))
			// Continue even if role assignment fails
		}
	}

	// Fetch the created customer with joined data
	//nolint:misspell // database uses British spelling
	fetchQuery := `
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
		LEFT JOIN customer_organisations co ON pc.customer_organisation_id = co.id
		WHERE pc.id = ?
	`

	var c models.PortalCustomer
	var phone sql.NullString
	var userFirstName, userLastName, userEmail, orgName sql.NullString
	var customFieldValuesStr sql.NullString
	var createdAtStr, updatedAtStr string

	err = h.db.QueryRow(fetchQuery, customerID).Scan(
		&c.ID, &c.Name, &c.Email, &phone,
		&c.UserID, &c.CustomerOrganisationID, &c.IsPrimary,
		&customFieldValuesStr,
		&createdAtStr, &updatedAtStr,
		&userFirstName, &userLastName, &userEmail, &orgName,
	)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Parse timestamps
	var createdAt time.Time
	if createdAt, err = parseTimestamp(createdAtStr); err == nil {
		c.CreatedAt = createdAt
	}
	var updatedAt time.Time
	if updatedAt, err = parseTimestamp(updatedAtStr); err == nil {
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
			respondInternalError(w, r, err)
			return
		}
	}

	// Load roles for the created customer
	roles, err := h.loadPortalCustomerRoles(c.ID)
	if err != nil {
		slog.Warn("failed to load roles for created customer", slog.String("component", "portal"), slog.Int("customer_id", c.ID), slog.Any("error", err))
		c.Roles = []models.ContactRole{}
	} else {
		c.Roles = roles
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(c)
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
	_, err = h.db.ExecWrite(query, requestData.CustomerOrganisationID, customerID)
	if err != nil {
		respondInternalError(w, r, err)
		return
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

	//nolint:misspell // customer_organisation is a database column name
	var requestData struct {
		Name                   string                 `json:"name"`
		Email                  string                 `json:"email"`
		Phone                  string                 `json:"phone"`
		CustomerOrganisationID *int                   `json:"customer_organisation_id"`
		IsPrimary              bool                   `json:"is_primary"`
		RoleIDs                []int                  `json:"role_ids"`
		CustomFieldValues      map[string]interface{} `json:"custom_field_values"`
	}

	if err = json.NewDecoder(r.Body).Decode(&requestData); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Validate required fields
	if requestData.Name == "" {
		respondValidationError(w, r, "Name is required")
		return
	}
	if requestData.Email == "" {
		respondValidationError(w, r, "Email is required")
		return
	}

	// Serialize custom field values to JSON
	var customFieldValuesJSON []byte
	if len(requestData.CustomFieldValues) > 0 {
		customFieldValuesJSON, err = json.Marshal(requestData.CustomFieldValues)
		if err != nil {
			respondBadRequest(w, r, "Invalid custom field values")
			return
		}
	}

	// Update the portal customer
	//nolint:misspell // customer_organisation_id is a database column name
	query := `
		UPDATE portal_customers
		SET name = ?, email = ?, phone = ?, customer_organisation_id = ?, is_primary = ?, custom_field_values = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`
	_, err = h.db.ExecWrite(query, requestData.Name, requestData.Email, requestData.Phone, requestData.CustomerOrganisationID, requestData.IsPrimary, customFieldValuesJSON, customerID)
	if err != nil {
		// Check for unique constraint violation on email
		if strings.Contains(err.Error(), "UNIQUE constraint failed: portal_customers.email") || strings.Contains(err.Error(), "duplicate key") {
			respondConflict(w, r, "A portal customer with this email address already exists")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Update roles if provided
	if requestData.RoleIDs != nil {
		err = h.assignRolesToPortalCustomer(customerID, requestData.RoleIDs)
		if err != nil {
			slog.Error("failed to update roles for portal customer", slog.String("component", "portal"), slog.Any("error", err))
			respondInternalError(w, r, err)
			return
		}
	}

	// Fetch and return the updated customer
	//nolint:misspell // customer_organisation is a database column/table name
	fetchQuery := `
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
		LEFT JOIN customer_organisations co ON pc.customer_organisation_id = co.id
		WHERE pc.id = ?
	`

	var c models.PortalCustomer
	var phone sql.NullString
	var userFirstName, userLastName, userEmail, orgName sql.NullString
	var customFieldValuesStr sql.NullString
	var createdAtStr, updatedAtStr string

	err = h.db.QueryRow(fetchQuery, customerID).Scan(
		&c.ID, &c.Name, &c.Email, &phone,
		&c.UserID, &c.CustomerOrganisationID, &c.IsPrimary,
		&customFieldValuesStr,
		&createdAtStr, &updatedAtStr,
		&userFirstName, &userLastName, &userEmail, &orgName,
	)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Parse timestamps
	var createdAt time.Time
	if createdAt, err = parseTimestamp(createdAtStr); err == nil {
		c.CreatedAt = createdAt
	}
	var updatedAt time.Time
	if updatedAt, err = parseTimestamp(updatedAtStr); err == nil {
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
			respondInternalError(w, r, err)
			return
		}
	}

	// Load roles for the updated customer
	roles, err := h.loadPortalCustomerRoles(c.ID)
	if err != nil {
		slog.Warn("failed to load roles for updated customer", slog.String("component", "portal"), slog.Int("customer_id", c.ID), slog.Any("error", err))
		c.Roles = []models.ContactRole{}
	} else {
		c.Roles = roles
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(c)
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
	_, err = h.db.ExecWrite(query, id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetOrganisationContacts returns all portal customers (contacts) for a given customer organisation
//
//nolint:misspell // "organisation" is intentional British spelling used throughout codebase
func (h *PortalCustomersHandler) GetOrganisationContacts(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	orgID, err := strconv.Atoi(idStr)
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	//nolint:misspell // "organisation" is intentional British spelling used throughout codebase
	query := `
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
		LEFT JOIN customer_organisations co ON pc.customer_organisation_id = co.id
		WHERE pc.customer_organisation_id = ?
		ORDER BY pc.is_primary DESC, pc.created_at DESC
	`

	rows, err := h.db.Query(query, orgID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var contacts []models.PortalCustomer
	for rows.Next() {
		var c models.PortalCustomer
		var phone sql.NullString
		var userFirstName, userLastName, userEmail, orgName sql.NullString
		var customFieldValuesStr sql.NullString
		var createdAtStr, updatedAtStr string

		err := rows.Scan(
			&c.ID, &c.Name, &c.Email, &phone,
			&c.UserID, &c.CustomerOrganisationID, &c.IsPrimary,
			&customFieldValuesStr,
			&createdAtStr, &updatedAtStr,
			&userFirstName, &userLastName, &userEmail, &orgName,
		)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		// Parse timestamps
		var createdAt time.Time
		if createdAt, err = parseTimestamp(createdAtStr); err == nil {
			c.CreatedAt = createdAt
		}
		var updatedAt time.Time
		if updatedAt, err = parseTimestamp(updatedAtStr); err == nil {
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
				// Log error but continue with other contacts
				slog.Warn("failed to parse custom field values for contact", slog.String("component", "portal"), slog.Int("contact_id", c.ID), slog.Any("error", err))
				continue
			}
		}

		// Load roles for this contact
		roles, err := h.loadPortalCustomerRoles(c.ID)
		if err != nil {
			slog.Warn("failed to load roles for contact", slog.String("component", "portal"), slog.Int("contact_id", c.ID), slog.Any("error", err))
			c.Roles = []models.ContactRole{}
		} else {
			c.Roles = roles
		}

		contacts = append(contacts, c)
	}

	if contacts == nil {
		contacts = []models.PortalCustomer{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(contacts)
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

// assignRolesToPortalCustomer assigns roles to a portal customer
func (h *PortalCustomersHandler) assignRolesToPortalCustomer(customerID int, roleIDs []int) error {
	// First, delete existing role assignments
	deleteQuery := `DELETE FROM portal_customer_roles WHERE portal_customer_id = ?`
	_, err := h.db.ExecWrite(deleteQuery, customerID)
	if err != nil {
		return err
	}

	// Then insert new role assignments
	if len(roleIDs) > 0 {
		insertQuery := `INSERT INTO portal_customer_roles (portal_customer_id, contact_role_id) VALUES (?, ?)`
		for _, roleID := range roleIDs {
			_, err := h.db.ExecWrite(insertQuery, customerID, roleID)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
