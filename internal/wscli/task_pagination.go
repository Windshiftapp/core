package wscli

import (
	"fmt"
	"strconv"
	"strings"
)

var (
	taskListPage      int
	taskListLimit     int
	taskListAll       bool
	taskListMilestone string
)

func warnItemPagination(items *PaginatedResponse[Item]) {
	if items == nil {
		return
	}
	total := items.Pagination.TotalItems
	if total == 0 {
		total = items.Pagination.Total
	}
	if total == 0 {
		total = items.Total
	}
	if total > len(items.Data) {
		_, _ = fmt.Fprintf(stderr, "Showing %d of %d items (page %d/%d); task ls supports --page/--limit and --all (reapply the relevant filters).\n", len(items.Data), total, items.Pagination.Page, items.Pagination.TotalPages)
	}
}

func listTaskPage(client *Client, filters map[string]string, page, limit int, all bool) (*PaginatedResponse[Item], error) {
	if page < 1 || limit < 1 || limit > 100 {
		return nil, fmt.Errorf("--page must be positive and --limit must be between 1 and 100")
	}
	params := make(map[string]string, len(filters)+2)
	for key, value := range filters {
		params[key] = value
	}
	params["page_size"] = strconv.Itoa(limit)
	var combined []Item
	seen := make(map[int]bool)
	for {
		params["page"] = strconv.Itoa(page)
		response, err := client.ListItems(params)
		if err != nil {
			return nil, err
		}
		if response.Pagination.Page != page || response.Pagination.TotalPages < 0 ||
			(response.Pagination.TotalPages == 0 && (len(response.Data) > 0 || response.Pagination.TotalItems > 0)) {
			return nil, fmt.Errorf("invalid pagination returned for page %d", page)
		}
		if !all {
			return response, nil
		}
		before := len(combined)
		for _, item := range response.Data {
			if !seen[item.ID] {
				seen[item.ID] = true
				combined = append(combined, item)
			}
		}
		if page >= response.Pagination.TotalPages {
			if combined == nil {
				combined = []Item{}
			}
			return &PaginatedResponse[Item]{Data: combined, Pagination: PaginationMeta{Page: 1, PageSize: len(combined), TotalItems: len(combined), TotalPages: 1}}, nil
		}
		if len(combined) == before {
			return nil, fmt.Errorf("pagination made no progress on page %d; retry the query", page)
		}
		page++
	}
}

// A name must identify exactly one milestone in the selected workspace.
// Never turn a misspelled filter into an unfiltered or fuzzy-matched list.
func resolveTaskMilestone(client *Client, value string, workspaceID *int) (int, error) {
	if id, err := strconv.Atoi(value); err == nil {
		return id, nil
	}
	if workspaceID == nil {
		return 0, fmt.Errorf("milestone names require a workspace: use -w, or supply a numeric milestone ID")
	}
	match := 0
	for page := 1; ; page++ {
		response, err := client.ListMilestonesInWorkspace(*workspaceID, map[string]string{"page": strconv.Itoa(page), "page_size": "100"})
		if err != nil {
			return 0, fmt.Errorf("resolve milestone: %w", err)
		}
		if response.Pagination.Page != page || response.Pagination.TotalPages < 0 ||
			(response.Pagination.TotalPages == 0 && (len(response.Data) > 0 || response.Pagination.TotalItems > 0)) {
			return 0, fmt.Errorf("invalid milestone pagination returned for page %d", page)
		}
		for _, milestone := range response.Data {
			if strings.EqualFold(milestone.Name, value) {
				if match != 0 && match != milestone.ID {
					return 0, fmt.Errorf("milestone name %q is ambiguous; use its numeric ID", value)
				}
				match = milestone.ID
			}
		}
		if page >= response.Pagination.TotalPages {
			break
		}
		if len(response.Data) == 0 {
			return 0, fmt.Errorf("milestone pagination made no progress on page %d", page)
		}
	}
	if match == 0 {
		return 0, fmt.Errorf("milestone not found: %s", value)
	}
	return match, nil
}
