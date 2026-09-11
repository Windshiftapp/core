package v2

import (
	"math"
	"net/http"
	"strconv"
)

const (
	defaultPage     = 1
	defaultPageSize = 50
	maxPageSize     = 1000
)

// Pagination is the canonical v2 page request.
type Pagination struct {
	Page     int
	PageSize int
	Offset   int
	Sort     string
	Desc     bool
}

// ParsePagination validates the canonical v2 page and sort query.
func ParsePagination(r *http.Request, allowedSort map[string]bool, defaultSort string) (Pagination, error) {
	page, err := ParsePage(r)
	if err != nil {
		return Pagination{}, err
	}

	sort := r.URL.Query().Get("sort")
	if sort == "" {
		sort = defaultSort
	}
	desc := sort != "" && sort[0] == '-'
	if desc {
		sort = sort[1:]
	}
	if sort == "" || !allowedSort[sort] {
		apiErr := newError(http.StatusBadRequest, "invalid_request", "sort is not supported")
		apiErr.Details = map[string]any{"field": "sort"}
		return Pagination{}, apiErr
	}

	page.Sort = sort
	page.Desc = desc
	return page, nil
}

// ParsePage validates canonical pagination for collections with fixed ordering.
func ParsePage(r *http.Request) (Pagination, error) {
	page, err := parsePositiveInt(r, "page", defaultPage, math.MaxInt)
	if err != nil {
		return Pagination{}, err
	}
	pageSize, err := parsePositiveInt(r, "page_size", defaultPageSize, maxPageSize)
	if err != nil {
		return Pagination{}, err
	}
	if page-1 > math.MaxInt/pageSize {
		apiErr := newError(http.StatusBadRequest, "invalid_request", "page offset is too large")
		apiErr.Details = map[string]any{"field": "page"}
		return Pagination{}, apiErr
	}
	return Pagination{Page: page, PageSize: pageSize, Offset: (page - 1) * pageSize}, nil
}

func parsePositiveInt(r *http.Request, name string, fallback, maximum int) (int, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > maximum {
		apiErr := newError(http.StatusBadRequest, "invalid_request", name+" is out of range")
		apiErr.Details = map[string]any{"field": name}
		return 0, apiErr
	}
	return value, nil
}

func parseNonNegativeQueryInt(r *http.Request, name string, fallback int) (int, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		apiErr := newError(http.StatusBadRequest, "invalid_request", name+" must be non-negative")
		apiErr.Details = map[string]any{"field": name}
		return 0, apiErr
	}
	return value, nil
}
