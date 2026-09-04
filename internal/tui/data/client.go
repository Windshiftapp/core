// Package data is the TUI's only gateway to the Windshift API: the HTTP
// client, the wire/domain types, the tea.Cmd loaders and the message types
// they emit.
//
// Sanitization rule: no string from the API reaches a renderer except
// through a converter in types.go (or an explicit Sanitize* call at the
// ingestion point). See sanitize.go.
package data

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client handles communication with the Windshift API.
type Client struct {
	baseURL     string
	httpClient  *http.Client
	bearerToken string
}

// NewClient creates a new API client.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SetBearerToken sets the API token used by the REST API.
func (c *Client) SetBearerToken(token string) {
	c.bearerToken = token
}

func (c *Client) setAuth(req *http.Request) {
	if c.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	}
}

// doGet performs a GET request to the given path and decodes the JSON response into result.
func (c *Client) doGet(path string, result any) error {
	req, err := http.NewRequest("GET", c.baseURL+path, http.NoBody)
	if err != nil {
		return err
	}
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("API error: %s; failed to read response body: %w", resp.Status, readErr)
		}
		return fmt.Errorf("API error: %s - %s", resp.Status, SanitizeText(string(body)))
	}

	if result == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(result)
}

// doMutate performs a mutating HTTP request (POST, PUT, etc.) with a JSON body.
// If result is non-nil, the response body is decoded into it.
func (c *Client) doMutate(method, path string, body, result any) error { //nolint:unparam // result is wired for callers that will decode bodies; all current call sites pass nil
	return c.doMutateWithContentType(method, path, body, result, "application/json")
}

func (c *Client) doMutateWithContentType(method, path string, body, result any, contentType string) error {
	jsonData, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(method, c.baseURL+path, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("API error: %s; failed to read response body: %w", resp.Status, readErr)
		}
		return fmt.Errorf("API error: %s - %s", resp.Status, SanitizeText(string(body)))
	}

	if result == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(result)
}

// ─── HTTP API methods ─────────────────────────────────────────────────

func (c *Client) getWorkspaces() ([]Workspace, error) {
	out := make([]Workspace, 0, 100)
	for page := 1; ; page++ {
		var resp workspacePageDocument
		path := fmt.Sprintf("/rest/api/v2/workspaces?page=%d&page_size=100", page)
		if err := c.doGet(path, &resp); err != nil {
			return nil, err
		}
		for _, w := range resp.Data {
			out = append(out, workspaceFromDTO(w))
		}
		if len(resp.Data) == 0 || page >= resp.Pagination.TotalPages {
			return out, nil
		}
	}
}

// maxWorkItems caps how many items getWorkItems accumulates across pages —
// beyond this the board truncates (and says so) rather than hammering the
// API with dozens of page fetches.
const maxWorkItems = 500

// getWorkItems fetches all pages of a workspace's items up to maxWorkItems.
// The bool result reports truncation.
func (c *Client) getWorkItems(workspaceID int) ([]WorkItem, bool, error) {
	out := make([]WorkItem, 0, 64)
	for page := 1; ; page++ {
		var resp itemPageDocument
		path := fmt.Sprintf("/rest/api/v2/items?workspace_id=%d&page=%d&page_size=100", workspaceID, page)
		if err := c.doGet(path, &resp); err != nil {
			return nil, false, err
		}
		for _, it := range resp.Data {
			out = append(out, workItemFromDTO(it))
		}
		if len(resp.Data) == 0 || page >= resp.Pagination.TotalPages {
			return out, false, nil
		}
		if len(out) >= maxWorkItems {
			return out, true, nil
		}
	}
}

func (c *Client) getComments(itemID int) ([]Comment, error) {
	out := make([]Comment, 0, 64)
	path := fmt.Sprintf("/rest/api/v2/items/%d/comments?page_size=100", itemID)
	for {
		var document dataDocument[struct {
			Comments   []commentDTO `json:"comments"`
			NextCursor string       `json:"next_cursor"`
			HasMore    bool         `json:"has_more"`
		}]
		if err := c.doGet(path, &document); err != nil {
			return nil, err
		}
		for _, c2 := range document.Data.Comments {
			out = append(out, commentFromDTO(c2))
		}
		if !document.Data.HasMore || document.Data.NextCursor == "" || len(document.Data.Comments) == 0 {
			return out, nil
		}
		path = fmt.Sprintf("/rest/api/v2/items/%d/comments?page_size=100&cursor=%s", itemID, document.Data.NextCursor)
	}
}

func (c *Client) getStatuses(workspaceID int) ([]Status, error) {
	// Workspace-scoped: /workspaces/{id}/statuses requires workspaces:read
	// (the global /statuses route would need a separate statuses:read scope).
	var doc dataDocument[[]statusDTO]
	if err := c.doGet(fmt.Sprintf("/rest/api/v2/workspaces/%d/statuses", workspaceID), &doc); err != nil {
		return nil, err
	}
	out := make([]Status, 0, len(doc.Data))
	for _, s := range doc.Data {
		out = append(out, Status{
			ID:            s.ID,
			Name:          SanitizeLine(s.Name),
			CategoryID:    s.Category.ID,
			CategoryName:  SanitizeLine(s.Category.Name),
			CategoryColor: SanitizeLine(s.Category.Color),
		})
	}
	return out, nil
}

func (c *Client) getPriorities() ([]Priority, error) {
	var doc dataDocument[[]priorityDTO]
	if err := c.doGet("/rest/api/v2/priorities", &doc); err != nil {
		return nil, err
	}
	out := make([]Priority, 0, len(doc.Data))
	for _, p := range doc.Data {
		out = append(out, Priority{
			ID:    p.ID,
			Name:  SanitizeLine(p.Name),
			Icon:  SanitizeLine(p.Icon),
			Color: SanitizeLine(p.Color),
		})
	}
	return out, nil
}

func (c *Client) getAssignableUsers(workspaceID int) ([]User, error) {
	var doc dataDocument[[]assignableUserDTO]
	if err := c.doGet(fmt.Sprintf("/rest/api/v2/workspaces/%d/assignable-users", workspaceID), &doc); err != nil {
		return nil, err
	}
	out := make([]User, 0, len(doc.Data))
	for _, u := range doc.Data {
		out = append(out, User{
			ID:       u.ID,
			Username: SanitizeLine(u.Username),
			FullName: SanitizeLine(u.FullName),
			IsAgent:  u.IsAgent,
		})
	}
	return out, nil
}

// getWorkItem fetches a single item for targeted refresh after a mutation.
func (c *Client) getWorkItem(itemID int) (WorkItem, error) {
	var document dataDocument[itemDTO]
	if err := c.doGet(fmt.Sprintf("/rest/api/v2/items/%d", itemID), &document); err != nil {
		return WorkItem{}, err
	}
	return workItemFromDTO(document.Data), nil
}

// setItemStatus drives the workflow transition endpoint.
func (c *Client) setItemStatus(itemID, statusID int) error {
	body := map[string]any{"to_status_id": statusID}
	return c.doMutate("POST", fmt.Sprintf("/rest/api/v2/items/%d/transition", itemID), body, nil)
}

// setItemField merge-patches a single field.
func (c *Client) setItemField(itemID int, field string, value any) error {
	body := map[string]any{field: value}
	return c.doMutateWithContentType("PATCH", fmt.Sprintf("/rest/api/v2/items/%d", itemID), body, nil, "application/merge-patch+json")
}

func (c *Client) getAgentRuns(itemID int) ([]AgentRun, error) {
	var document dataDocument[[]agentRunDTO]
	if err := c.doGet(fmt.Sprintf("/rest/api/v2/items/%d/agent-runs?page_size=10", itemID), &document); err != nil {
		return nil, err
	}
	fmtTime := func(t *time.Time) string {
		if t == nil {
			return ""
		}
		return t.Format(time.RFC3339)
	}
	out := make([]AgentRun, 0, len(document.Data))
	for _, r := range document.Data {
		out = append(out, AgentRun{
			ID:        r.ID,
			Status:    SanitizeLine(r.Status),
			JobKind:   SanitizeLine(r.JobKind),
			QueuedAt:  r.QueuedAt.Format(time.RFC3339),
			StartedAt: fmtTime(r.StartedAt),
			EndedAt:   fmtTime(r.EndedAt),
			Error:     SanitizeLine(r.Error),
		})
	}
	return out, nil
}

func (c *Client) getPrefs() (Prefs, error) {
	var document struct {
		Data struct {
			TUI Prefs `json:"tui"`
		} `json:"data"`
	}
	if err := c.doGet("/rest/api/v2/users/me/preferences", &document); err != nil {
		return Prefs{}, err
	}
	p := document.Data.TUI
	p.Theme = SanitizeLine(p.Theme)
	return p, nil
}

func (c *Client) getCurrentUserTimezone() (string, error) {
	var doc dataDocument[currentUserDTO]
	if err := c.doGet("/rest/api/v2/users/me", &doc); err != nil {
		return "", err
	}
	return SanitizeLine(doc.Data.Timezone), nil
}

func (c *Client) putPrefs(p Prefs) error {
	return c.doMutateWithContentType("PATCH", "/rest/api/v2/users/me/preferences", map[string]any{"tui": p}, nil, "application/merge-patch+json")
}

func (c *Client) getTimeProjects() ([]TimeProject, error) {
	var document dataDocument[[]TimeProject]
	if err := c.doGet("/rest/api/v2/time/projects?status=Active", &document); err != nil {
		return nil, err
	}
	projects := document.Data
	for i := range projects {
		projects[i].Name = SanitizeLine(projects[i].Name)
		projects[i].Description = SanitizeStringPtr(projects[i].Description, false)
		projects[i].CustomerName = SanitizeStringPtr(projects[i].CustomerName, true)
		projects[i].Status = SanitizeLine(projects[i].Status)
	}
	return projects, nil
}

// updateWorkItem updates title/description/priority via PATCH, then if statusID
// changed, drives the workflow transition through POST /items/{id}/transition.
func (c *Client) updateWorkItem(itemID int, title, description string, statusID, priorityID *int, assigneeSet bool, assigneeID int) error {
	body := map[string]any{
		"title":       title,
		"description": description,
	}
	if priorityID != nil {
		body["priority_id"] = *priorityID
	}
	if assigneeSet {
		if assigneeID > 0 {
			body["assignee_id"] = assigneeID
		} else {
			body["assignee_id"] = nil
		}
	}
	if err := c.doMutateWithContentType("PATCH", fmt.Sprintf("/rest/api/v2/items/%d", itemID), body, nil, "application/merge-patch+json"); err != nil {
		return err
	}
	if statusID != nil {
		transition := map[string]any{"to_status_id": *statusID}
		if err := c.doMutate("POST", fmt.Sprintf("/rest/api/v2/items/%d/transition", itemID), transition, nil); err != nil {
			return fmt.Errorf("status transition: %w", err)
		}
	}
	return nil
}

func (c *Client) createWorkItem(workspaceID int, title, description string, priorityID *int) error {
	body := map[string]any{
		"workspace_id": workspaceID,
		"title":        title,
		"description":  description,
	}
	if priorityID != nil {
		body["priority_id"] = *priorityID
	}
	return c.doMutate("POST", "/rest/api/v2/items", body, nil)
}

func (c *Client) createComment(itemID int, content string) error {
	body := map[string]any{"content": content}
	return c.doMutate("POST", fmt.Sprintf("/rest/api/v2/items/%d/comments", itemID), body, nil)
}

func (c *Client) createTimeLog(itemID, projectID int, description, duration, date, startTime, timezone string) error {
	data := map[string]any{
		"project_id":  projectID,
		"item_id":     itemID,
		"description": description,
		"date":        date,
		"start_time":  startTime,
		"duration":    duration,
		"timezone":    timezone,
	}
	return c.doMutate("POST", "/rest/api/v2/time/worklogs", data, nil)
}
