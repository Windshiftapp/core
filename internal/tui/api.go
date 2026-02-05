// Package tui provides a terminal user interface for Windshift.
package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// APIClient handles communication with the Windshift API
type APIClient struct {
	baseURL      string
	httpClient   *http.Client
	sessionToken string
}

// NewAPIClient creates a new API client
func NewAPIClient(baseURL string) *APIClient {
	return &APIClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SetSessionToken sets the session token for authentication
func (c *APIClient) SetSessionToken(token string) {
	c.sessionToken = token
}

// Workspace represents a workspace from the Windshift API.
type Workspace struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Key           string `json:"key"`
	Description   string `json:"description"`
	Active        bool   `json:"active"`
	TimeProjectID *int   `json:"time_project_id"`
}

// Status represents a workflow status
type Status struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	CategoryID    int    `json:"category_id"`
	CategoryName  string `json:"category_name"`
	CategoryColor string `json:"category_color"`
}

// Priority represents a priority level
type Priority struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Icon      string `json:"icon"`
	Color     string `json:"color"`
	SortOrder int    `json:"sort_order"`
}

type WorkItem struct {
	ID                int                    `json:"id"`
	WorkspaceID       int                    `json:"workspace_id"`
	ItemTypeID        *int                   `json:"item_type_id"`
	Title             string                 `json:"title"`
	Description       string                 `json:"description"`
	Status            string                 `json:"status"`                // Legacy text field
	Priority          string                 `json:"priority"`              // Legacy text field
	StatusID          *int                   `json:"status_id,omitempty"`   // ID-based status
	PriorityID        *int                   `json:"priority_id,omitempty"` // ID-based priority
	MilestoneID       *int                   `json:"milestone_id"`
	TimeProjectID     *int                   `json:"time_project_id"`
	AssigneeID        *int                   `json:"assignee_id"`
	CreatorID         *int                   `json:"creator_id"`
	CustomFieldValues map[string]interface{} `json:"custom_field_values"`
	ParentID          *int                   `json:"parent_id"`
	Path              string                 `json:"path"`
	Rank              *string                `json:"rank"`
	CreatedAt         string                 `json:"created_at"`
	UpdatedAt         string                 `json:"updated_at"`
	// Joined fields for display
	WorkspaceName   string `json:"workspace_name"`
	WorkspaceKey    string `json:"workspace_key"`
	ItemTypeName    string `json:"item_type_name"`
	ParentTitle     string `json:"parent_title"`
	MilestoneName   string `json:"milestone_name"`
	TimeProjectName string `json:"time_project_name"`
	AssigneeName    string `json:"assignee_name"`
	AssigneeEmail   string `json:"assignee_email"`
	CreatorName     string `json:"creator_name"`
	CreatorEmail    string `json:"creator_email"`
	// ID-based status/priority display fields
	StatusName          string `json:"status_name,omitempty"`
	StatusCategoryColor string `json:"category_color,omitempty"`
	PriorityName        string `json:"priority_name,omitempty"`
	PriorityIcon        string `json:"priority_icon,omitempty"`
	PriorityColor       string `json:"priority_color,omitempty"`
}

// GetLevel calculates hierarchy level from path
func (wi *WorkItem) GetLevel() int {
	if wi.Path == "" {
		return 0
	}
	// Path format is like "/1/5/12/" - count slashes minus 1
	level := 0
	for _, char := range wi.Path {
		if char == '/' {
			level++
		}
	}
	// Subtract 1 because path starts and ends with /
	return level - 1
}

type Comment struct {
	ID          int     `json:"id"`
	ItemID      int     `json:"item_id"`
	AuthorID    int     `json:"author_id"`
	Content     string  `json:"content"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
	AuthorName  *string `json:"author_name"`
	AuthorEmail *string `json:"author_email"`
}

type TimeProject struct {
	ID           int32   `json:"id"`
	CustomerID   int32   `json:"customer_id"`
	Name         string  `json:"name"`
	Description  *string `json:"description"`
	HourlyRate   float64 `json:"hourly_rate"`
	Active       bool    `json:"active"`
	CustomerName *string `json:"customer_name"`
}

type CreateWorkItemRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Priority    string `json:"priority"`
}

type CreateCommentRequest struct {
	Content  string `json:"content"`
	AuthorID int    `json:"author_id"`
}

type CreateTimeLogRequest struct {
	ProjectID   int     `json:"project_id"`
	ItemID      *int    `json:"item_id"`
	Description string  `json:"description"`
	Date        string  `json:"date"`
	StartTime   string  `json:"start_time"`
	Duration    string  `json:"duration"`
	EndTime     *string `json:"end_time"`
}

// Message types for tea.Cmd
type workspacesLoadedMsg struct {
	workspaces []Workspace
}

type workItemsLoadedMsg struct {
	items []WorkItem
}

type commentsLoadedMsg struct {
	comments []Comment
}

type workItemUpdatedMsg struct{}

type workItemCreatedMsg struct{}

type commentCreatedMsg struct{}

type timeLogCreatedMsg struct{}

type timeProjectsLoadedMsg struct {
	projects []TimeProject
}

type statusesLoadedMsg struct {
	statuses []Status
}

type prioritiesLoadedMsg struct {
	priorities []Priority
}

type errorMsg struct {
	error string
}

// API methods that return tea.Cmd
func (m Model) loadWorkspaces() tea.Cmd {
	return tea.Cmd(func() tea.Msg {
		workspaces, err := m.apiClient.getWorkspaces()
		if err != nil {
			return errorMsg{error: err.Error()}
		}
		return workspacesLoadedMsg{workspaces: workspaces}
	})
}

func (m Model) loadWorkItems(workspaceID int) tea.Cmd {
	return tea.Cmd(func() tea.Msg {
		items, err := m.apiClient.getWorkItems(workspaceID)
		if err != nil {
			return errorMsg{error: err.Error()}
		}
		return workItemsLoadedMsg{items: items}
	})
}

func (m Model) loadComments(itemID int) tea.Cmd {
	return tea.Cmd(func() tea.Msg {
		comments, err := m.apiClient.getComments(itemID)
		if err != nil {
			return errorMsg{error: err.Error()}
		}
		return commentsLoadedMsg{comments: comments}
	})
}

func (m Model) loadStatuses() tea.Cmd {
	return tea.Cmd(func() tea.Msg {
		statuses, err := m.apiClient.getStatuses()
		if err != nil {
			return errorMsg{error: err.Error()}
		}
		return statusesLoadedMsg{statuses: statuses}
	})
}

func (m Model) loadPriorities() tea.Cmd {
	return tea.Cmd(func() tea.Msg {
		priorities, err := m.apiClient.getPriorities()
		if err != nil {
			return errorMsg{error: err.Error()}
		}
		return prioritiesLoadedMsg{priorities: priorities}
	})
}

func (m Model) loadTimeProjects() tea.Cmd {
	return tea.Cmd(func() tea.Msg {
		projects, err := m.apiClient.getTimeProjects()
		if err != nil {
			return errorMsg{error: err.Error()}
		}
		return timeProjectsLoadedMsg{projects: projects}
	})
}

func (m Model) updateWorkItem(itemID int, title, description string, statusID, priorityID *int) tea.Cmd {
	return tea.Cmd(func() tea.Msg {
		err := m.apiClient.updateWorkItem(itemID, title, description, statusID, priorityID)
		if err != nil {
			return errorMsg{error: err.Error()}
		}
		return workItemUpdatedMsg{}
	})
}

func (m Model) createWorkItem(workspaceID int, title, description string, priorityID *int) tea.Cmd {
	return tea.Cmd(func() tea.Msg {
		err := m.apiClient.createWorkItem(workspaceID, title, description, priorityID)
		if err != nil {
			return errorMsg{error: err.Error()}
		}
		return workItemCreatedMsg{}
	})
}

func (m Model) createComment(itemID int, content string) tea.Cmd {
	return tea.Cmd(func() tea.Msg {
		err := m.apiClient.createComment(itemID, content)
		if err != nil {
			return errorMsg{error: err.Error()}
		}
		return commentCreatedMsg{}
	})
}

func (m Model) createTimeLog(itemID, projectID int, description, duration, date, startTime string) tea.Cmd {
	return tea.Cmd(func() tea.Msg {
		err := m.apiClient.createTimeLog(itemID, projectID, description, duration, date, startTime)
		if err != nil {
			return errorMsg{error: err.Error()}
		}
		return timeLogCreatedMsg{}
	})
}

// HTTP API methods
func (c *APIClient) getWorkspaces() ([]Workspace, error) {
	req, err := http.NewRequest("GET", c.baseURL+"/api/workspaces", http.NoBody)
	if err != nil {
		return nil, err
	}

	// Add bearer token if available
	if c.sessionToken != "" {
		req.Header.Set("X-Session-Token", c.sessionToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error: %s - %s", resp.Status, string(body))
	}

	var workspaces []Workspace
	if err := json.NewDecoder(resp.Body).Decode(&workspaces); err != nil {
		return nil, err
	}

	return workspaces, nil
}

func (c *APIClient) getWorkItems(workspaceID int) ([]WorkItem, error) {
	url := fmt.Sprintf("%s/api/items?workspace_id=%d", c.baseURL, workspaceID)
	req, err := http.NewRequest("GET", url, http.NoBody)
	if err != nil {
		return nil, err
	}

	// Add bearer token if available
	if c.sessionToken != "" {
		req.Header.Set("X-Session-Token", c.sessionToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error: %s - %s", resp.Status, string(body))
	}

	// Handle paginated response
	var paginatedResponse struct {
		Items      []WorkItem `json:"items"`
		Pagination struct {
			Page       int `json:"page"`
			Limit      int `json:"limit"`
			Total      int `json:"total"`
			TotalPages int `json:"total_pages"`
		} `json:"pagination"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&paginatedResponse); err != nil {
		return nil, err
	}

	return paginatedResponse.Items, nil
}

func (c *APIClient) getComments(itemID int) ([]Comment, error) {
	url := fmt.Sprintf("%s/api/items/%d/comments", c.baseURL, itemID)
	req, err := http.NewRequest("GET", url, http.NoBody)
	if err != nil {
		return nil, err
	}

	// Add bearer token if available
	if c.sessionToken != "" {
		req.Header.Set("X-Session-Token", c.sessionToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error: %s - %s", resp.Status, string(body))
	}

	var comments []Comment
	if err := json.NewDecoder(resp.Body).Decode(&comments); err != nil {
		return nil, err
	}

	return comments, nil
}

func (c *APIClient) getStatuses() ([]Status, error) {
	req, err := http.NewRequest("GET", c.baseURL+"/api/statuses", http.NoBody)
	if err != nil {
		return nil, err
	}

	if c.sessionToken != "" {
		req.Header.Set("X-Session-Token", c.sessionToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error: %s - %s", resp.Status, string(body))
	}

	var statuses []Status
	if err := json.NewDecoder(resp.Body).Decode(&statuses); err != nil {
		return nil, err
	}

	return statuses, nil
}

func (c *APIClient) getPriorities() ([]Priority, error) {
	req, err := http.NewRequest("GET", c.baseURL+"/api/priorities", http.NoBody)
	if err != nil {
		return nil, err
	}

	if c.sessionToken != "" {
		req.Header.Set("X-Session-Token", c.sessionToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error: %s - %s", resp.Status, string(body))
	}

	var priorities []Priority
	if err := json.NewDecoder(resp.Body).Decode(&priorities); err != nil {
		return nil, err
	}

	return priorities, nil
}

func (c *APIClient) getTimeProjects() ([]TimeProject, error) {
	req, err := http.NewRequest("GET", c.baseURL+"/api/time/projects", http.NoBody)
	if err != nil {
		return nil, err
	}

	if c.sessionToken != "" {
		req.Header.Set("X-Session-Token", c.sessionToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error: %s - %s", resp.Status, string(body))
	}

	var projects []TimeProject
	if err := json.NewDecoder(resp.Body).Decode(&projects); err != nil {
		return nil, err
	}

	return projects, nil
}

func (c *APIClient) updateWorkItem(itemID int, title, description string, statusID, priorityID *int) error {
	data := map[string]interface{}{
		"title":       title,
		"description": description,
	}
	if statusID != nil {
		data["status_id"] = *statusID
	}
	if priorityID != nil {
		data["priority_id"] = *priorityID
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/items/%d", c.baseURL, itemID)
	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	if c.sessionToken != "" {
		req.Header.Set("X-Session-Token", c.sessionToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s - %s", resp.Status, string(body))
	}

	return nil
}

func (c *APIClient) createWorkItem(workspaceID int, title, description string, priorityID *int) error {
	data := map[string]interface{}{
		"workspace_id": workspaceID,
		"title":        title,
		"description":  description,
	}
	if priorityID != nil {
		data["priority_id"] = *priorityID
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", c.baseURL+"/api/items", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	if c.sessionToken != "" {
		req.Header.Set("X-Session-Token", c.sessionToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s - %s", resp.Status, string(body))
	}

	return nil
}

func (c *APIClient) createComment(itemID int, content string) error {
	data := CreateCommentRequest{
		Content:  content,
		AuthorID: 1, // Default author ID - in a real app, this would be the current user
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/items/%d/comments", c.baseURL, itemID)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	// Add bearer token if available
	if c.sessionToken != "" {
		req.Header.Set("X-Session-Token", c.sessionToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s - %s", resp.Status, string(body))
	}

	return nil
}

func (c *APIClient) createTimeLog(itemID, projectID int, description, duration, date, startTime string) error {
	data := CreateTimeLogRequest{
		ProjectID:   projectID,
		ItemID:      &itemID,
		Description: description,
		Date:        date,
		StartTime:   startTime,
		Duration:    duration,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", c.baseURL+"/api/time/worklogs", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	if c.sessionToken != "" {
		req.Header.Set("X-Session-Token", c.sessionToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s - %s", resp.Status, string(body))
	}

	return nil
}
