package restapi

import (
	"net/http"
	"strconv"
)

const (
	DefaultPage     = 1
	DefaultLimit    = 50
	MaxLimit        = 100
	DefaultSortBy   = "created_at"
	DefaultSortDesc = true
)

// PaginationMeta provides pagination information in responses
type PaginationMeta struct {
	Page       int  `json:"page"`
	Limit      int  `json:"limit"`
	Total      int  `json:"total"`
	TotalPages int  `json:"total_pages"`
	HasMore    bool `json:"has_more"`
}

// PaginationParams holds parsed pagination parameters from request
type PaginationParams struct {
	Page    int
	Limit   int
	Offset  int
	SortBy  string
	SortAsc bool
}

// ParsePaginationParams extracts pagination parameters from request
func ParsePaginationParams(r *http.Request) PaginationParams {
	params := PaginationParams{
		Page:    DefaultPage,
		Limit:   DefaultLimit,
		SortBy:  DefaultSortBy,
		SortAsc: !DefaultSortDesc,
	}

	// Parse page
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if page, err := strconv.Atoi(pageStr); err == nil && page > 0 {
			params.Page = page
		}
	}

	// Parse limit
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 {
			params.Limit = limit
			if params.Limit > MaxLimit {
				params.Limit = MaxLimit
			}
		}
	}

	// Parse sort field
	if sortBy := r.URL.Query().Get("sort"); sortBy != "" {
		params.SortBy = sortBy
	}

	// Parse sort order
	if order := r.URL.Query().Get("order"); order != "" {
		params.SortAsc = order == "asc"
	}

	// Calculate offset
	params.Offset = (params.Page - 1) * params.Limit

	return params
}

// NewPaginationMeta creates pagination metadata from params and total count
func NewPaginationMeta(params PaginationParams, total int) PaginationMeta {
	totalPages := total / params.Limit
	if total%params.Limit > 0 {
		totalPages++
	}

	return PaginationMeta{
		Page:       params.Page,
		Limit:      params.Limit,
		Total:      total,
		TotalPages: totalPages,
		HasMore:    params.Page < totalPages,
	}
}

// PaginatedResponse wraps any data with pagination metadata
type PaginatedResponse struct {
	Data       interface{}    `json:"data"`
	Pagination PaginationMeta `json:"pagination"`
}

// NewPaginatedResponse creates a paginated response
func NewPaginatedResponse(data interface{}, pagination PaginationMeta) PaginatedResponse {
	return PaginatedResponse{
		Data:       data,
		Pagination: pagination,
	}
}
