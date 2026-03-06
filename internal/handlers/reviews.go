package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

type ReviewHandler struct {
	db database.Database
}

func NewReviewHandler(db database.Database) *ReviewHandler {
	return &ReviewHandler{
		db: db,
	}
}

// GetReviews retrieves reviews for a user with optional filtering
func (h *ReviewHandler) GetReviews(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	userID := user.ID

	// Parse query parameters
	reviewType := r.URL.Query().Get("type")      // daily or weekly
	startDate := r.URL.Query().Get("start_date") // YYYY-MM-DD
	endDate := r.URL.Query().Get("end_date")     // YYYY-MM-DD
	limitStr := r.URL.Query().Get("limit")

	// Build query
	query := `
		SELECT r.id, r.user_id, r.review_date, r.review_type, r.review_data, 
		       r.created_at, r.updated_at,
		       u.first_name || ' ' || u.last_name as user_name, u.email as user_email
		FROM reviews r
		LEFT JOIN users u ON r.user_id = u.id
		WHERE r.user_id = ?`

	args := []interface{}{userID}

	if reviewType != "" {
		query += " AND r.review_type = ?"
		args = append(args, reviewType)
	}

	if startDate != "" {
		query += " AND r.review_date >= ?"
		args = append(args, startDate)
	}

	if endDate != "" {
		query += " AND r.review_date <= ?"
		args = append(args, endDate)
	}

	query += " ORDER BY r.review_date DESC"

	if limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 {
			query += " LIMIT ?"
			args = append(args, limit)
		}
	}

	rows, err := h.db.Query(query, args...)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var reviews []models.Review
	for rows.Next() {
		var review models.Review
		var userName, userEmail sql.NullString

		err = rows.Scan(&review.ID, &review.UserID, &review.ReviewDate, &review.ReviewType,
			&review.ReviewData, &review.CreatedAt, &review.UpdatedAt, &userName, &userEmail)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		if userName.Valid {
			review.UserName = userName.String
		}
		if userEmail.Valid {
			review.UserEmail = userEmail.String
		}

		reviews = append(reviews, review)
	}

	if err = rows.Err(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(reviews)
}

// GetReview retrieves a specific review by ID
func (h *ReviewHandler) GetReview(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	userID := user.ID

	reviewID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	var review models.Review
	var userName, userEmail sql.NullString

	err = h.db.QueryRow(`
		SELECT r.id, r.user_id, r.review_date, r.review_type, r.review_data,
		       r.created_at, r.updated_at,
		       u.first_name || ' ' || u.last_name as user_name, u.email as user_email
		FROM reviews r
		LEFT JOIN users u ON r.user_id = u.id
		WHERE r.id = ? AND r.user_id = ?
	`, reviewID, userID).Scan(&review.ID, &review.UserID, &review.ReviewDate, &review.ReviewType,
		&review.ReviewData, &review.CreatedAt, &review.UpdatedAt, &userName, &userEmail)

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "review")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if userName.Valid {
		review.UserName = userName.String
	}
	if userEmail.Valid {
		review.UserEmail = userEmail.String
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(review)
}

// CreateReview creates a new review
func (h *ReviewHandler) CreateReview(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	userID := user.ID

	var req models.ReviewCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid JSON")
		return
	}

	// Validate required fields
	if req.ReviewDate == "" || req.ReviewType == "" || req.ReviewData == "" {
		respondValidationError(w, r, "Missing required fields: review_date, review_type, review_data")
		return
	}

	// Validate review type
	if req.ReviewType != "daily" && req.ReviewType != "weekly" {
		respondValidationError(w, r, "Review type must be 'daily' or 'weekly'")
		return
	}

	// Validate date format
	if _, err := time.Parse("2006-01-02", req.ReviewDate); err != nil {
		respondValidationError(w, r, "Invalid date format. Use YYYY-MM-DD")
		return
	}

	// Check uniqueness before insert
	var reviewExists bool
	_ = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM reviews WHERE user_id = ? AND review_date = ? AND review_type = ?)", userID, req.ReviewDate, req.ReviewType).Scan(&reviewExists)
	if reviewExists {
		respondConflict(w, r, "Review already exists for this date and type")
		return
	}

	// Insert review
	var reviewID int64
	err := h.db.QueryRow(`
		INSERT INTO reviews (user_id, review_date, review_type, review_data, created_at, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) RETURNING id
	`, userID, req.ReviewDate, req.ReviewType, req.ReviewData).Scan(&reviewID)

	if err != nil {
		if database.IsUniqueConstraintError(err) {
			respondConflict(w, r, "Review already exists for this date and type")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	// Return the created review
	var review models.Review
	var userName, userEmail sql.NullString

	err = h.db.QueryRow(`
		SELECT r.id, r.user_id, r.review_date, r.review_type, r.review_data, 
		       r.created_at, r.updated_at,
		       u.first_name || ' ' || u.last_name as user_name, u.email as user_email
		FROM reviews r
		LEFT JOIN users u ON r.user_id = u.id
		WHERE r.id = ?
	`, reviewID).Scan(&review.ID, &review.UserID, &review.ReviewDate, &review.ReviewType,
		&review.ReviewData, &review.CreatedAt, &review.UpdatedAt, &userName, &userEmail)

	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to retrieve created review: %w", err))
		return
	}

	if userName.Valid {
		review.UserName = userName.String
	}
	if userEmail.Valid {
		review.UserEmail = userEmail.String
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(review)
}

// UpdateReview updates an existing review
func (h *ReviewHandler) UpdateReview(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	userID := user.ID

	reviewID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	var req models.ReviewUpdateRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid JSON")
		return
	}

	// Validate required fields
	if req.ReviewData == "" {
		respondValidationError(w, r, "Missing required field: review_data")
		return
	}

	// Update review
	result, err := h.db.ExecWrite(`
		UPDATE reviews
		SET review_data = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND user_id = ?
	`, req.ReviewData, reviewID, userID)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to check rows affected: %w", err))
		return
	}

	if rowsAffected == 0 {
		respondNotFound(w, r, "review")
		return
	}

	// Return the updated review
	var review models.Review
	var userName, userEmail sql.NullString

	err = h.db.QueryRow(`
		SELECT r.id, r.user_id, r.review_date, r.review_type, r.review_data, 
		       r.created_at, r.updated_at,
		       u.first_name || ' ' || u.last_name as user_name, u.email as user_email
		FROM reviews r
		LEFT JOIN users u ON r.user_id = u.id
		WHERE r.id = ?
	`, reviewID).Scan(&review.ID, &review.UserID, &review.ReviewDate, &review.ReviewType,
		&review.ReviewData, &review.CreatedAt, &review.UpdatedAt, &userName, &userEmail)

	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to retrieve updated review: %w", err))
		return
	}

	if userName.Valid {
		review.UserName = userName.String
	}
	if userEmail.Valid {
		review.UserEmail = userEmail.String
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(review)
}

// DeleteReview deletes a review
func (h *ReviewHandler) DeleteReview(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	userID := user.ID

	reviewID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	result, err := h.db.ExecWrite("DELETE FROM reviews WHERE id = ? AND user_id = ?", reviewID, userID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to check rows affected: %w", err))
		return
	}

	if rowsAffected == 0 {
		respondNotFound(w, r, "review")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetCompletedItems gets completed items for a user within a date range
func (h *ReviewHandler) GetCompletedItems(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	userID := user.ID

	// Parse query parameters
	startDate := r.URL.Query().Get("start_date") // YYYY-MM-DD
	endDate := r.URL.Query().Get("end_date")     // YYYY-MM-DD

	if startDate == "" || endDate == "" {
		respondValidationError(w, r, "Missing required parameters: start_date, end_date")
		return
	}

	// Validate date formats
	if _, err := time.Parse("2006-01-02", startDate); err != nil {
		respondValidationError(w, r, "Invalid start_date format. Use YYYY-MM-DD")
		return
	}
	if _, err := time.Parse("2006-01-02", endDate); err != nil {
		respondValidationError(w, r, "Invalid end_date format. Use YYYY-MM-DD")
		return
	}

	query := `
		WITH completed_statuses AS (
			SELECT s.id
			FROM statuses s
			JOIN status_categories sc ON sc.id = s.category_id
			WHERE COALESCE(sc.is_completed, FALSE) = TRUE
		),
		completion_events AS (
			SELECT ih.item_id,
			       MAX(ih.changed_at) AS completed_at
			FROM item_history ih
			JOIN completed_statuses cs ON cs.id = CAST(NULLIF(ih.new_value, '') AS INTEGER)
			LEFT JOIN completed_statuses prev_cs ON prev_cs.id = CAST(NULLIF(ih.old_value, '') AS INTEGER)
			WHERE ih.field_name = 'status_id'
			  AND prev_cs.id IS NULL
			GROUP BY ih.item_id
		)
		SELECT i.id, i.workspace_id, i.title, i.description,
		       i.created_at, i.updated_at,
		       w.name as workspace_name, w.key as workspace_key,
		       i.workspace_item_number, i.item_type_id,
		       ce.completed_at
		FROM completion_events ce
		JOIN items i ON i.id = ce.item_id
		JOIN workspaces w ON i.workspace_id = w.id
		WHERE i.assignee_id = ?
		  AND DATE(ce.completed_at) >= ?
		  AND DATE(ce.completed_at) <= ?
		ORDER BY ce.completed_at DESC`

	rows, err := h.db.Query(query, userID, startDate, endDate)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var items []models.Item
	for rows.Next() {
		var item models.Item
		var completedAtStr sql.NullString
		err = rows.Scan(&item.ID, &item.WorkspaceID, &item.Title, &item.Description,
			&item.CreatedAt, &item.UpdatedAt,
			&item.WorkspaceName, &item.WorkspaceKey,
			&item.WorkspaceItemNumber, &item.ItemTypeID,
			&completedAtStr)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		if completedAtStr.Valid && completedAtStr.String != "" {
			var t time.Time
			if t, err = time.Parse("2006-01-02 15:04:05", completedAtStr.String); err == nil {
				item.CompletedAt = &t
			} else if t, err = time.Parse(time.RFC3339, completedAtStr.String); err == nil {
				item.CompletedAt = &t
			}
		}

		items = append(items, item)
	}

	if err = rows.Err(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(items)
}
