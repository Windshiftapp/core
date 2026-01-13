package handlers

import (
	"database/sql"
	"net/http"
	"strconv"


	"windshift/internal/database"
	"windshift/internal/restapi"
	"windshift/internal/restapi/v1/middleware"
)

// UserHandler handles public API requests for users
type UserHandler struct {
	db database.Database
}

// NewUserHandler creates a new user handler
func NewUserHandler(db database.Database) *UserHandler {
	return &UserHandler{db: db}
}

// UserResponse is the public API representation of a User
type UserResponse struct {
	ID        int    `json:"id"`
	Email     string `json:"email"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	FullName  string `json:"full_name"`
	IsActive  bool   `json:"is_active"`
	AvatarURL string `json:"avatar_url,omitempty"`
	Timezone  string `json:"timezone,omitempty"`
	Language  string `json:"language,omitempty"`
	CreatedAt string `json:"created_at"`
}

// List handles GET /rest/api/v1/users
func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	pagination := restapi.ParsePaginationParams(r)

	rows, err := h.db.Query(`
		SELECT id, email, username, first_name, last_name, is_active, avatar_url, timezone, language, created_at
		FROM users
		WHERE is_active = 1
		ORDER BY first_name, last_name
		LIMIT ? OFFSET ?
	`, pagination.Limit, pagination.Offset)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}
	defer rows.Close()

	var users []UserResponse
	for rows.Next() {
		var u UserResponse
		var avatarURL, timezone, language sql.NullString
		rows.Scan(&u.ID, &u.Email, &u.Username, &u.FirstName, &u.LastName, &u.IsActive,
			&avatarURL, &timezone, &language, &u.CreatedAt)
		u.FullName = u.FirstName + " " + u.LastName
		u.AvatarURL = nullStringValue(avatarURL)
		u.Timezone = nullStringValue(timezone)
		u.Language = nullStringValue(language)
		users = append(users, u)
	}

	var total int
	h.db.QueryRow("SELECT COUNT(*) FROM users WHERE is_active = 1").Scan(&total)

	restapi.RespondPaginated(w, users, restapi.NewPaginationMeta(pagination, total))
}

// Get handles GET /rest/api/v1/users/{id}
func (h *UserHandler) Get(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid user ID"))
		return
	}

	var u UserResponse
	var avatarURL, timezone, language sql.NullString
	err = h.db.QueryRow(`
		SELECT id, email, username, first_name, last_name, is_active, avatar_url, timezone, language, created_at
		FROM users WHERE id = ?
	`, id).Scan(&u.ID, &u.Email, &u.Username, &u.FirstName, &u.LastName, &u.IsActive,
		&avatarURL, &timezone, &language, &u.CreatedAt)
	if err == sql.ErrNoRows {
		restapi.RespondError(w, r, restapi.ErrUserNotFound)
		return
	}
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	u.FullName = u.FirstName + " " + u.LastName
	u.AvatarURL = nullStringValue(avatarURL)
	u.Timezone = nullStringValue(timezone)
	u.Language = nullStringValue(language)

	restapi.RespondOK(w, u)
}

// GetCurrent handles GET /rest/api/v1/users/me
func (h *UserHandler) GetCurrent(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	var u UserResponse
	var avatarURL, timezone, language sql.NullString
	err := h.db.QueryRow(`
		SELECT id, email, username, first_name, last_name, is_active, avatar_url, timezone, language, created_at
		FROM users WHERE id = ?
	`, user.ID).Scan(&u.ID, &u.Email, &u.Username, &u.FirstName, &u.LastName, &u.IsActive,
		&avatarURL, &timezone, &language, &u.CreatedAt)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	u.FullName = u.FirstName + " " + u.LastName
	u.AvatarURL = nullStringValue(avatarURL)
	u.Timezone = nullStringValue(timezone)
	u.Language = nullStringValue(language)

	restapi.RespondOK(w, u)
}
