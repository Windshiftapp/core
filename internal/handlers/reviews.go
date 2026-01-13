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
	user, ok := r.Context().Value("user").(*models.User)
	if !ok {
		http.Error(w, "User not authenticated", http.StatusUnauthorized)
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
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var reviews []models.Review
	for rows.Next() {
		var review models.Review
		var userName, userEmail sql.NullString

		err := rows.Scan(&review.ID, &review.UserID, &review.ReviewDate, &review.ReviewType,
			&review.ReviewData, &review.CreatedAt, &review.UpdatedAt, &userName, &userEmail)
		if err != nil {
			http.Error(w, fmt.Sprintf("Row scan error: %v", err), http.StatusInternalServerError)
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
		http.Error(w, fmt.Sprintf("Rows iteration error: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reviews)
}

// GetReview retrieves a specific review by ID
func (h *ReviewHandler) GetReview(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value("user").(*models.User)
	if !ok {
		http.Error(w, "User not authenticated", http.StatusUnauthorized)
		return
	}
	userID := user.ID

	reviewID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid review ID", http.StatusBadRequest)
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
		http.Error(w, "Review not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	if userName.Valid {
		review.UserName = userName.String
	}
	if userEmail.Valid {
		review.UserEmail = userEmail.String
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(review)
}

// CreateReview creates a new review
func (h *ReviewHandler) CreateReview(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value("user").(*models.User)
	if !ok {
		http.Error(w, "User not authenticated", http.StatusUnauthorized)
		return
	}
	userID := user.ID

	var req models.ReviewCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.ReviewDate == "" || req.ReviewType == "" || req.ReviewData == "" {
		http.Error(w, "Missing required fields: review_date, review_type, review_data", http.StatusBadRequest)
		return
	}

	// Validate review type
	if req.ReviewType != "daily" && req.ReviewType != "weekly" {
		http.Error(w, "Review type must be 'daily' or 'weekly'", http.StatusBadRequest)
		return
	}

	// Validate date format
	if _, err := time.Parse("2006-01-02", req.ReviewDate); err != nil {
		http.Error(w, "Invalid date format. Use YYYY-MM-DD", http.StatusBadRequest)
		return
	}

	// Insert review
	var reviewID int64
	err := h.db.QueryRow(`
		INSERT INTO reviews (user_id, review_date, review_type, review_data, created_at, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) RETURNING id
	`, userID, req.ReviewDate, req.ReviewType, req.ReviewData).Scan(&reviewID)

	if err != nil {
		if err.Error() == "UNIQUE constraint failed: reviews.user_id, reviews.review_date, reviews.review_type" {
			http.Error(w, "Review already exists for this date and type", http.StatusConflict)
		} else {
			http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
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
		http.Error(w, fmt.Sprintf("Failed to retrieve created review: %v", err), http.StatusInternalServerError)
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
	json.NewEncoder(w).Encode(review)
}

// UpdateReview updates an existing review
func (h *ReviewHandler) UpdateReview(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value("user").(*models.User)
	if !ok {
		http.Error(w, "User not authenticated", http.StatusUnauthorized)
		return
	}
	userID := user.ID

	reviewID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid review ID", http.StatusBadRequest)
		return
	}

	var req models.ReviewUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.ReviewData == "" {
		http.Error(w, "Missing required field: review_data", http.StatusBadRequest)
		return
	}

	// Update review
	result, err := h.db.ExecWrite(`
		UPDATE reviews 
		SET review_data = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND user_id = ?
	`, req.ReviewData, reviewID, userID)

	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to check rows affected: %v", err), http.StatusInternalServerError)
		return
	}

	if rowsAffected == 0 {
		http.Error(w, "Review not found or access denied", http.StatusNotFound)
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
		http.Error(w, fmt.Sprintf("Failed to retrieve updated review: %v", err), http.StatusInternalServerError)
		return
	}

	if userName.Valid {
		review.UserName = userName.String
	}
	if userEmail.Valid {
		review.UserEmail = userEmail.String
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(review)
}

// DeleteReview deletes a review
func (h *ReviewHandler) DeleteReview(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value("user").(*models.User)
	if !ok {
		http.Error(w, "User not authenticated", http.StatusUnauthorized)
		return
	}
	userID := user.ID

	reviewID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid review ID", http.StatusBadRequest)
		return
	}

	result, err := h.db.ExecWrite("DELETE FROM reviews WHERE id = ? AND user_id = ?", reviewID, userID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to check rows affected: %v", err), http.StatusInternalServerError)
		return
	}

	if rowsAffected == 0 {
		http.Error(w, "Review not found or access denied", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetCompletedItems gets completed items for a user within a date range
func (h *ReviewHandler) GetCompletedItems(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value("user").(*models.User)
	if !ok {
		http.Error(w, "User not authenticated", http.StatusUnauthorized)
		return
	}
	userID := user.ID

	// Parse query parameters
	startDate := r.URL.Query().Get("start_date") // YYYY-MM-DD
	endDate := r.URL.Query().Get("end_date")     // YYYY-MM-DD

	if startDate == "" || endDate == "" {
		http.Error(w, "Missing required parameters: start_date, end_date", http.StatusBadRequest)
		return
	}

	// Validate date formats
	if _, err := time.Parse("2006-01-02", startDate); err != nil {
		http.Error(w, "Invalid start_date format. Use YYYY-MM-DD", http.StatusBadRequest)
		return
	}
	if _, err := time.Parse("2006-01-02", endDate); err != nil {
		http.Error(w, "Invalid end_date format. Use YYYY-MM-DD", http.StatusBadRequest)
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
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var items []models.Item
	for rows.Next() {
		var item models.Item
		var completedAtStr sql.NullString
		err := rows.Scan(&item.ID, &item.WorkspaceID, &item.Title, &item.Description,
			&item.CreatedAt, &item.UpdatedAt,
			&item.WorkspaceName, &item.WorkspaceKey, &completedAtStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("Row scan error: %v", err), http.StatusInternalServerError)
			return
		}

		if completedAtStr.Valid && completedAtStr.String != "" {
			if t, err := time.Parse("2006-01-02 15:04:05", completedAtStr.String); err == nil {
				item.CompletedAt = &t
			} else if t, err := time.Parse(time.RFC3339, completedAtStr.String); err == nil {
				item.CompletedAt = &t
			}
		}

		items = append(items, item)
	}

	if err = rows.Err(); err != nil {
		http.Error(w, fmt.Sprintf("Rows iteration error: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}
