package wscli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Client calls the Windshift API.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func NewClient() (*Client, error) {
	if cfg.Server.URL == "" {
		return nil, fmt.Errorf("server URL not configured. Set WS_URL, use --url, or run 'ws config init'")
	}
	if cfg.Server.Token == "" {
		return nil, fmt.Errorf("API token not configured. Set WS_TOKEN, use --token, or run 'ws config init'")
	}

	return &Client{
		baseURL: strings.TrimSuffix(cfg.Server.URL, "/"),
		token:   cfg.Server.Token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// APIError represents an error response from the API. Status carries
// the HTTP status code so callers can branch on 404/403/etc. without
// pattern-matching the message string. Zero means "unknown" (e.g.
// transport failure before a response arrived).
type APIError struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}

// doRequest executes an HTTP request with authentication. WS_DEBUG_HTTP=1
// in the env enables one-line request/response logging on stderr — useful
// when triaging server-side errors from the CLI.
func (c *Client) doRequest(method, path string, body, result any) error {
	return c.doRequestWithContentType(method, path, body, result, "application/json")
}

func (c *Client) doRequestWithContentType(method, path string, body, result any, contentType string) error {
	var bodyReader io.Reader
	var jsonBody []byte
	if body != nil {
		var err error
		jsonBody, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}
	if debugHTTP {
		_, _ = fmt.Fprintf(stderr, "[ws-debug] %s %s body=%s\n", method, path, string(jsonBody))
	}

	reqURL := c.baseURL + path
	req, err := http.NewRequest(method, reqURL, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req) //nolint:gosec // G704: URL from server config, not user input
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if debugHTTP {
		// #nosec G705 -- writing to a CLI terminal, not HTML; G705 is checking for an XSS sink that doesn't exist here
		_, _ = fmt.Fprintf(stderr, "[ws-debug] -> status=%d body=%s\n", resp.StatusCode, string(respBody))
	}
	if resp.StatusCode >= 400 {
		return decodeAPIError(resp.StatusCode, respBody)
	}

	if result != nil && len(respBody) > 0 {
		if err := decodeResponse(respBody, result); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}
	}

	return nil
}

func decodeAPIError(status int, body []byte) error {
	var document struct {
		Error APIError `json:"error"`
	}
	if err := json.Unmarshal(body, &document); err == nil && (document.Error.Code != "" || document.Error.Message != "") {
		document.Error.Status = status
		return &document.Error
	}
	return fmt.Errorf("API error (status %d): %s", status, string(body))
}

func decodeResponse(body []byte, result any) error {
	if responseOwnsEnvelope(result) {
		return json.Unmarshal(body, result)
	}
	var document struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &document); err != nil {
		return err
	}
	if len(document.Data) == 0 {
		return errors.New("response is missing data")
	}
	return json.Unmarshal(document.Data, result)
}

func responseOwnsEnvelope(result any) bool {
	typeOf := reflect.TypeOf(result)
	for typeOf != nil && typeOf.Kind() == reflect.Pointer {
		typeOf = typeOf.Elem()
	}
	if typeOf == nil || typeOf.Kind() != reflect.Struct {
		return false
	}
	for i := 0; i < typeOf.NumField(); i++ {
		if strings.Split(typeOf.Field(i).Tag.Get("json"), ",")[0] == "data" {
			return true
		}
	}
	return false
}

func v2QueryName(name string) string {
	switch name {
	case "limit":
		return "page_size"
	case "q":
		return "search"
	default:
		return name
	}
}

func (c *Client) GET(path string, result any) error {
	return c.doRequest("GET", path, nil, result)
}

func (c *Client) POST(path string, body, result any) error {
	return c.doRequest("POST", path, body, result)
}

func (c *Client) PUT(path string, body, result any) error {
	return c.doRequest("PUT", path, body, result)
}

func (c *Client) PATCH(path string, body, result any) error {
	return c.doRequest("PATCH", path, body, result)
}

func (c *Client) MERGEPATCH(path string, body, result any) error {
	return c.doRequestWithContentType("PATCH", path, body, result, "application/merge-patch+json")
}

func (c *Client) DELETE(path string) error {
	return c.doRequest("DELETE", path, nil, nil)
}

// REST API methods.

func (c *Client) GetCurrentUser() (*User, error) {
	var doc DataDocument[User]
	if err := c.GET("/rest/api/v2/users/me", &doc); err != nil {
		return nil, err
	}
	return &doc.Data, nil
}

func (c *Client) ListItems(filters map[string]string) (*PaginatedResponse[Item], error) {
	path := "/rest/api/v2/items"
	if len(filters) > 0 {
		params := url.Values{}
		for k, v := range filters {
			params.Set(v2QueryName(k), v)
		}
		path += "?" + params.Encode()
	}

	var resp PaginatedResponse[Item]
	if err := c.GET(path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetItem(id int, _ string) (*Item, error) {
	path := fmt.Sprintf("/rest/api/v2/items/%d", id)

	var item Item
	if err := c.GET(path, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

// GetItemByKeyAndNumber gets an item by its (workspace_key, workspace_item_number) pair
// via the direct-lookup endpoint, avoiding paginating over /rest/api/v2/items when
// resolving a KEY-NUMBER identifier.
func (c *Client) GetItemByKeyAndNumber(wsKey string, number int) (*Item, error) {
	path := fmt.Sprintf("/rest/api/v2/workspaces/%s/items/%d", url.PathEscape(wsKey), number)
	var item Item
	if err := c.GET(path, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

func (c *Client) GetItemChildren(id int) ([]Item, error) {
	var items []Item
	if err := c.GET(fmt.Sprintf("/rest/api/v2/items/%d/children", id), &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (c *Client) CreateItem(req ItemCreateRequest) (*Item, error) {
	var item Item
	if err := c.POST("/rest/api/v2/items", req, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

func (c *Client) UpdateItem(id int, req ItemUpdateRequest) (*Item, error) {
	var item Item
	if err := c.MERGEPATCH(fmt.Sprintf("/rest/api/v2/items/%d", id), req, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

func (c *Client) GetItemTransitions(id int) ([]Transition, error) {
	var response struct {
		Available []struct {
			ID            int    `json:"id"`
			Name          string `json:"name"`
			CategoryColor string `json:"category_color"`
		} `json:"available_transitions"`
	}
	if err := c.GET(fmt.Sprintf("/rest/api/v2/items/%d/available-transitions", id), &response); err != nil {
		return nil, err
	}
	transitions := make([]Transition, len(response.Available))
	for i, option := range response.Available {
		transitions[i] = Transition{ToStatusID: option.ID, ToStatus: &StatusSummary{ID: option.ID, Name: option.Name, CategoryColor: option.CategoryColor}}
	}
	return transitions, nil
}

// TransitionItem performs a workflow status transition. Use this instead of
// setting status_id via UpdateItem — the update endpoint rejects status_id
// because transitions must run through the workflow + condition pipeline.
func (c *Client) TransitionItem(id, toStatusID int) (*TransitionResult, error) {
	var result TransitionResult
	req := TransitionRequest{ToStatusID: toStatusID}
	if err := c.POST(fmt.Sprintf("/rest/api/v2/items/%d/transition", id), req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ChangeItemType changes an item's item type through the dedicated endpoint.
func (c *Client) ChangeItemType(id, targetItemTypeID int, targetStatusID *int) (*Item, error) {
	var item Item
	req := ItemTypeChangeRequest{TargetItemTypeID: targetItemTypeID, TargetStatusID: targetStatusID}
	if err := c.POST(fmt.Sprintf("/rest/api/v2/items/%d/change-type", id), req, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

func (c *Client) ListWorkspaces() (*PaginatedResponse[Workspace], error) {
	var resp PaginatedResponse[Workspace]
	if err := c.GET("/rest/api/v2/workspaces?page_size=100", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetWorkspace(id int) (*Workspace, error) {
	var doc DataDocument[Workspace]
	if err := c.GET(fmt.Sprintf("/rest/api/v2/workspaces/%d", id), &doc); err != nil {
		return nil, err
	}
	return &doc.Data, nil
}

func (c *Client) GetWorkspaceStatuses(workspaceID int) ([]Status, error) {
	var doc DataDocument[[]v2Status]
	if err := c.GET(fmt.Sprintf("/rest/api/v2/workspaces/%d/statuses", workspaceID), &doc); err != nil {
		return nil, err
	}
	return statusesFromV2(doc.Data), nil
}

func (c *Client) GetCompletedStatuses(workspaceID int) ([]Status, error) {
	statuses, err := c.GetWorkspaceStatuses(workspaceID)
	if err != nil {
		return nil, err
	}
	completed := make([]Status, 0, len(statuses))
	for _, status := range statuses {
		if status.IsCompleted {
			completed = append(completed, status)
		}
	}
	return completed, nil
}

func (c *Client) ListStatuses() ([]Status, error) {
	var doc DataDocument[[]v2Status]
	if err := c.GET("/rest/api/v2/statuses", &doc); err != nil {
		return nil, err
	}
	return statusesFromV2(doc.Data), nil
}

func (c *Client) ListItemTypes() ([]ItemType, error) {
	var doc DataDocument[[]ItemType]
	if err := c.GET("/rest/api/v2/item-types", &doc); err != nil {
		return nil, err
	}
	return doc.Data, nil
}

func (c *Client) GetWorkspaceItemTypes(workspaceID int) ([]ItemType, error) {
	var doc DataDocument[[]ItemType]
	if err := c.GET(fmt.Sprintf("/rest/api/v2/workspaces/%d/item-types", workspaceID), &doc); err != nil {
		return nil, err
	}
	return doc.Data, nil
}

func (c *Client) ListPriorities() ([]Priority, error) {
	var doc DataDocument[[]Priority]
	if err := c.GET("/rest/api/v2/priorities", &doc); err != nil {
		return nil, err
	}
	return doc.Data, nil
}

func (c *Client) GetWorkspacePriorities(workspaceID int) ([]Priority, error) {
	var doc DataDocument[[]Priority]
	if err := c.GET(fmt.Sprintf("/rest/api/v2/workspaces/%d/priorities", workspaceID), &doc); err != nil {
		return nil, err
	}
	return doc.Data, nil
}

func (c *Client) ListWorkflows() ([]Workflow, error) {
	var doc DataDocument[[]Workflow]
	if err := c.GET("/rest/api/v2/workflows", &doc); err != nil {
		return nil, err
	}
	return doc.Data, nil
}

func (c *Client) GetWorkspaceWorkflows(workspaceID int) ([]Workflow, error) {
	var doc DataDocument[[]Workflow]
	if err := c.GET(fmt.Sprintf("/rest/api/v2/workspaces/%d/workflows", workspaceID), &doc); err != nil {
		return nil, err
	}
	return doc.Data, nil
}

func (c *Client) GetWorkflowTransitions(workflowID int) ([]Transition, error) {
	var doc DataDocument[[]v2Transition]
	if err := c.GET(fmt.Sprintf("/rest/api/v2/workflows/%d/transitions", workflowID), &doc); err != nil {
		return nil, err
	}
	return transitionsFromV2(doc.Data), nil
}

type v2Status struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsDefault   bool   `json:"is_default"`
	Category    struct {
		ID          int    `json:"id"`
		Name        string `json:"name"`
		Color       string `json:"color"`
		IsCompleted bool   `json:"is_completed"`
	} `json:"category"`
}

func statusesFromV2(items []v2Status) []Status {
	result := make([]Status, len(items))
	for i, item := range items {
		result[i] = Status{
			ID: item.ID, Name: item.Name, Description: item.Description,
			CategoryID: item.Category.ID, CategoryName: item.Category.Name,
			CategoryColor: item.Category.Color, IsDefault: item.IsDefault,
			IsCompleted: item.Category.IsCompleted,
		}
	}
	return result
}

type v2TransitionStatus struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	CategoryName  string `json:"category_name"`
	CategoryColor string `json:"category_color"`
}

type v2Transition struct {
	ID   int                 `json:"id"`
	From *v2TransitionStatus `json:"from"`
	To   v2TransitionStatus  `json:"to"`
}

func transitionsFromV2(items []v2Transition) []Transition {
	result := make([]Transition, len(items))
	for i, item := range items {
		to := StatusSummary{
			ID: item.To.ID, Name: item.To.Name,
			CategoryName: item.To.CategoryName, CategoryColor: item.To.CategoryColor,
		}
		result[i] = Transition{ID: item.ID, ToStatusID: item.To.ID, ToStatus: &to}
		if item.From != nil {
			fromID := item.From.ID
			from := StatusSummary{
				ID: item.From.ID, Name: item.From.Name,
				CategoryName: item.From.CategoryName, CategoryColor: item.From.CategoryColor,
			}
			result[i].FromStatusID = &fromID
			result[i].FromStatus = &from
		}
	}
	return result
}

// Test-run reads and writes use the canonical v2 contract.

func (c *Client) ListTestCases(workspaceID int, folderID string) ([]TestCase, error) {
	path := fmt.Sprintf("/rest/api/v2/workspaces/%d/test-cases", workspaceID)
	if folderID != "" {
		path += "?folder_id=" + url.QueryEscape(folderID)
	} else {
		path += "?all=true"
	}

	var cases []TestCase
	if err := c.GET(path, &cases); err != nil {
		return nil, err
	}
	return cases, nil
}

func (c *Client) GetTestCase(workspaceID, id int) (*TestCase, error) {
	var tc TestCase
	if err := c.GET(fmt.Sprintf("/rest/api/v2/workspaces/%d/test-cases/%d", workspaceID, id), &tc); err != nil {
		return nil, err
	}
	return &tc, nil
}

func (c *Client) GetTestSteps(workspaceID, testCaseID int) ([]TestStep, error) {
	var steps []TestStep
	if err := c.GET(fmt.Sprintf("/rest/api/v2/workspaces/%d/test-cases/%d/steps", workspaceID, testCaseID), &steps); err != nil {
		return nil, err
	}
	return steps, nil
}

func (c *Client) ListTestRuns(workspaceID int, assigneeID string) ([]TestRun, error) {
	path := fmt.Sprintf("/rest/api/v2/workspaces/%d/test-runs", workspaceID)
	if assigneeID != "" {
		path += "?assignee_id=" + url.QueryEscape(assigneeID)
	}

	var runs []TestRun
	if err := c.GET(path, &runs); err != nil {
		return nil, err
	}
	return runs, nil
}

func (c *Client) GetTestRun(workspaceID, id int) (*TestRun, error) {
	var run TestRun
	if err := c.GET(fmt.Sprintf("/rest/api/v2/workspaces/%d/test-runs/%d", workspaceID, id), &run); err != nil {
		return nil, err
	}
	return &run, nil
}

func (c *Client) CreateTestRun(workspaceID int, req TestRunCreateRequest) (*TestRun, error) {
	var run TestRun
	if err := c.POST(fmt.Sprintf("/rest/api/v2/workspaces/%d/test-runs", workspaceID), req, &run); err != nil {
		return nil, err
	}
	return &run, nil
}

func (c *Client) EndTestRun(workspaceID, id int) error {
	return c.POST(fmt.Sprintf("/rest/api/v2/workspaces/%d/test-runs/%d/end", workspaceID, id), nil, nil)
}

func (c *Client) GetTestRunResults(workspaceID, runID int) ([]TestResult, error) {
	var results []TestResult
	if err := c.GET(fmt.Sprintf("/rest/api/v2/workspaces/%d/test-runs/%d/results", workspaceID, runID), &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (c *Client) UpdateTestResult(workspaceID, runID, resultID int, req TestResultUpdateRequest) error {
	return c.MERGEPATCH(fmt.Sprintf("/rest/api/v2/workspaces/%d/test-runs/%d/results/%d", workspaceID, runID, resultID), req, nil)
}

func (c *Client) ListTestSets(workspaceID int) ([]TestSet, error) {
	var sets []TestSet
	if err := c.GET(fmt.Sprintf("/rest/api/v2/workspaces/%d/test-plans", workspaceID), &sets); err != nil {
		return nil, err
	}
	return sets, nil
}

func (c *Client) GetTestSet(workspaceID, id int) (*TestSet, error) {
	var set TestSet
	if err := c.GET(fmt.Sprintf("/rest/api/v2/workspaces/%d/test-plans/%d", workspaceID, id), &set); err != nil {
		return nil, err
	}
	return &set, nil
}

func (c *Client) GetTestSetTestCases(workspaceID, setID int) ([]TestCase, error) {
	var cases []TestCase
	if err := c.GET(fmt.Sprintf("/rest/api/v2/workspaces/%d/test-plans/%d/test-cases", workspaceID, setID), &cases); err != nil {
		return nil, err
	}
	return cases, nil
}

func (c *Client) ExecuteRunTemplate(workspaceID, templateID int) (*TestRun, error) {
	var run TestRun
	if err := c.POST(fmt.Sprintf("/rest/api/v2/workspaces/%d/test-run-templates/%d/execute", workspaceID, templateID), nil, &run); err != nil {
		return nil, err
	}
	return &run, nil
}

// Comment methods.

func (c *Client) GetComments(itemID int) ([]Comment, error) {
	const pageSize = 100
	comments := make([]Comment, 0)
	path := fmt.Sprintf("/rest/api/v2/items/%d/comments?page_size=%d", itemID, pageSize)
	for {
		var response struct {
			Comments []Comment `json:"comments"`
			HasMore  bool      `json:"has_more"`
		}
		if err := c.GET(path, &response); err != nil {
			return nil, err
		}
		comments = append(comments, response.Comments...)
		if !response.HasMore || len(response.Comments) == 0 {
			break
		}
		last := response.Comments[len(response.Comments)-1]
		path = fmt.Sprintf("/rest/api/v2/items/%d/comments?page_size=%d&before=%s&before_id=%d", itemID, pageSize, url.QueryEscape(last.CreatedAt.Format(time.RFC3339Nano)), last.ID)
	}
	return comments, nil
}

func (c *Client) CreateComment(itemID int, content string) (*Comment, error) {
	req := map[string]string{"content": content}
	var comment Comment
	if err := c.POST(fmt.Sprintf("/rest/api/v2/items/%d/comments", itemID), req, &comment); err != nil {
		return nil, err
	}
	return &comment, nil
}

func (c *Client) UpdateComment(commentID int, content string) (*Comment, error) {
	req := map[string]string{"content": content}
	var comment Comment
	if err := c.MERGEPATCH(fmt.Sprintf("/rest/api/v2/comments/%d", commentID), req, &comment); err != nil {
		return nil, err
	}
	return &comment, nil
}

func (c *Client) DeleteComment(commentID int) error {
	return c.DELETE(fmt.Sprintf("/rest/api/v2/comments/%d", commentID))
}

// Diagram routes accept opaque Excalidraw or Mermaid seed data.

func (c *Client) ListDiagrams(itemID int) ([]Diagram, error) {
	var doc DataDocument[[]Diagram]
	if err := c.GET(fmt.Sprintf("/rest/api/v2/items/%d/diagrams", itemID), &doc); err != nil {
		return nil, err
	}
	return doc.Data, nil
}

func (c *Client) GetDiagram(id int) (*Diagram, error) {
	var doc DataDocument[Diagram]
	if err := c.GET(fmt.Sprintf("/rest/api/v2/item-diagrams/%d", id), &doc); err != nil {
		return nil, err
	}
	return &doc.Data, nil
}

// CreateDiagram stores the raw diagram payload; Mermaid callers must wrap it
// as JSON with type "mermaid" and a source field.
func (c *Client) CreateDiagram(itemID int, name, diagramData string) (*Diagram, error) {
	req := map[string]string{"name": name, "diagram_data": diagramData}
	var doc DataDocument[Diagram]
	if err := c.POST(fmt.Sprintf("/rest/api/v2/items/%d/diagrams", itemID), req, &doc); err != nil {
		return nil, err
	}
	return &doc.Data, nil
}

func (c *Client) UpdateDiagram(id int, name, diagramData string) (*Diagram, error) {
	req := map[string]string{"name": name, "diagram_data": diagramData}
	var doc DataDocument[Diagram]
	if err := c.MERGEPATCH(fmt.Sprintf("/rest/api/v2/item-diagrams/%d", id), req, &doc); err != nil {
		return nil, err
	}
	return &doc.Data, nil
}

func (c *Client) DeleteDiagram(id int) error {
	return c.DELETE(fmt.Sprintf("/rest/api/v2/item-diagrams/%d", id))
}

// UploadPageAttachment uploads through the page v2 endpoint. Its 404
// deliberately hides whether a page is missing or inaccessible.
func (c *Client) UploadPageAttachment(workspaceID, pageID int, originalFilename string, body io.Reader) (*Attachment, error) {
	path := fmt.Sprintf("/rest/api/v2/workspaces/%d/pages/%d/attachments", workspaceID, pageID)
	return c.uploadAttachment(path, originalFilename, body)
}

// UploadItemAttachment shares the page upload envelope. Its 404 hides item
// existence; callers pre-check the route's 32 MB limit.
func (c *Client) UploadItemAttachment(itemID int, originalFilename string, body io.Reader) (*Attachment, error) {
	path := fmt.Sprintf("/rest/api/v2/items/%d/attachments", itemID)
	return c.uploadAttachment(path, originalFilename, body)
}

// uploadAttachment builds a multipart request and decodes its response envelope.
func (c *Client) uploadAttachment(path, originalFilename string, body io.Reader) (*Attachment, error) {
	var buf bytes.Buffer
	mp := multipart.NewWriter(&buf)
	part, err := mp.CreateFormFile("file", originalFilename)
	if err != nil {
		return nil, fmt.Errorf("multipart create part: %w", err)
	}
	if _, err := io.Copy(part, body); err != nil {
		return nil, fmt.Errorf("multipart copy body: %w", err)
	}
	if err := mp.Close(); err != nil {
		return nil, fmt.Errorf("multipart close: %w", err)
	}

	reqURL := c.baseURL + path
	if debugHTTP {
		_, _ = fmt.Fprintf(stderr, "[ws-debug] POST %s upload=%s bytes=%d\n", reqURL, originalFilename, buf.Len())
	}

	req, err := http.NewRequest("POST", reqURL, &buf)
	if err != nil {
		return nil, fmt.Errorf("create upload request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", mp.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req) //nolint:gosec // G704: URL from server config
	if err != nil {
		return nil, fmt.Errorf("upload request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read upload response: %w", err)
	}
	if debugHTTP {
		// #nosec G705 -- writing to a CLI terminal, not HTML
		_, _ = fmt.Fprintf(stderr, "[ws-debug] -> status=%d body=%s\n", resp.StatusCode, string(respBody))
	}

	if resp.StatusCode >= 400 {
		return nil, decodeAPIError(resp.StatusCode, respBody)
	}

	var document DataDocument[Attachment]
	if err := json.Unmarshal(respBody, &document); err != nil {
		return nil, fmt.Errorf("parse upload response: %w", err)
	}
	if document.Data.ID == 0 {
		return nil, fmt.Errorf("upload response missing attachment id: %s", string(respBody))
	}
	return &document.Data, nil
}

// ListAttachments returns all attachments on an item.
func (c *Client) ListAttachments(itemID int) ([]Attachment, error) {
	var atts []Attachment
	if err := c.GET(fmt.Sprintf("/rest/api/v2/items/%d/attachments", itemID), &atts); err != nil {
		return nil, err
	}
	return atts, nil
}

// DownloadAttachment streams the attachment bytes for the given id into w
// and returns the filename suggested by the server's Content-Disposition
// header. Falls back to "attachment-<id>" if no filename is advertised.
func (c *Client) DownloadAttachment(id int, w io.Writer) (string, error) {
	reqURL := c.baseURL + fmt.Sprintf("/rest/api/v2/attachments/%d/content", id)
	if debugHTTP {
		_, _ = fmt.Fprintf(stderr, "[ws-debug] GET %s\n", reqURL)
	}

	req, err := http.NewRequest("GET", reqURL, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req) //nolint:gosec // G704: URL from server config
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if debugHTTP {
		_, _ = fmt.Fprintf(stderr, "[ws-debug] -> status=%d content-type=%s\n", resp.StatusCode, resp.Header.Get("Content-Type"))
	}

	if resp.StatusCode >= 400 {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return "", fmt.Errorf("API error (status %d); failed to read response body: %w", resp.StatusCode, readErr)
		}
		return "", decodeAPIError(resp.StatusCode, body)
	}

	filename := parseContentDispositionFilename(resp.Header.Get("Content-Disposition"))
	if filename == "" {
		filename = fmt.Sprintf("attachment-%d", id)
	}

	if _, err := io.Copy(w, resp.Body); err != nil {
		return filename, fmt.Errorf("failed to stream file: %w", err)
	}
	return filename, nil
}

// parseContentDispositionFilename extracts the filename from a
// Content-Disposition header. Returns "" if the header is missing,
// malformed, or has no filename parameter.
func parseContentDispositionFilename(header string) string {
	if header == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(header)
	if err != nil {
		return ""
	}
	return params["filename"]
}

// Milestone API methods.

func (c *Client) ListMilestones(filters map[string]string) (*PaginatedResponse[Milestone], error) {
	path := "/rest/api/v2/milestones?scope=global"
	if len(filters) > 0 {
		params := url.Values{"scope": {"global"}}
		for k, v := range filters {
			params.Set(v2QueryName(k), v)
		}
		path = "/rest/api/v2/milestones?" + params.Encode()
	}

	var resp PaginatedResponse[Milestone]
	if err := c.GET(path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetMilestone(id int) (*Milestone, error) {
	var milestone Milestone
	if err := c.GET(fmt.Sprintf("/rest/api/v2/milestones/%d", id), &milestone); err != nil {
		return nil, err
	}
	return &milestone, nil
}

func (c *Client) GetMilestoneProgress(id int) (*MilestoneProgress, error) {
	var progress MilestoneProgress
	if err := c.GET(fmt.Sprintf("/rest/api/v2/milestones/%d/progress", id), &progress); err != nil {
		return nil, err
	}
	return &progress, nil
}

func (c *Client) CreateMilestone(req MilestoneCreateRequest) (*Milestone, error) {
	req.Scope = "global"
	var milestone Milestone
	if err := c.POST("/rest/api/v2/milestones", req, &milestone); err != nil {
		return nil, err
	}
	return &milestone, nil
}

func (c *Client) UpdateMilestone(id int, req MilestoneUpdateRequest) (*Milestone, error) {
	var milestone Milestone
	if err := c.MERGEPATCH(fmt.Sprintf("/rest/api/v2/milestones/%d", id), req, &milestone); err != nil {
		return nil, err
	}
	return &milestone, nil
}

func (c *Client) DeleteMilestone(id int) error {
	return c.DELETE(fmt.Sprintf("/rest/api/v2/milestones/%d", id))
}

// Workspace milestone helpers use the canonical workspace_id scope.

func (c *Client) ListMilestonesInWorkspace(workspaceID int, filters map[string]string) (*PaginatedResponse[Milestone], error) {
	params := url.Values{"workspace_id": {strconv.Itoa(workspaceID)}}
	if len(filters) > 0 {
		for k, v := range filters {
			params.Set(v2QueryName(k), v)
		}
	}
	path := "/rest/api/v2/milestones?" + params.Encode()

	var resp PaginatedResponse[Milestone]
	if err := c.GET(path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetMilestoneInWorkspace(_, milestoneID int) (*Milestone, error) {
	var milestone Milestone
	if err := c.GET(fmt.Sprintf("/rest/api/v2/milestones/%d", milestoneID), &milestone); err != nil {
		return nil, err
	}
	return &milestone, nil
}

func (c *Client) GetMilestoneProgressInWorkspace(_, milestoneID int) (*MilestoneProgress, error) {
	var progress MilestoneProgress
	if err := c.GET(fmt.Sprintf("/rest/api/v2/milestones/%d/progress", milestoneID), &progress); err != nil {
		return nil, err
	}
	return &progress, nil
}

func (c *Client) CreateMilestoneInWorkspace(workspaceID int, req MilestoneCreateRequest) (*Milestone, error) {
	req.WorkspaceID = &workspaceID
	req.Scope = ""
	var milestone Milestone
	if err := c.POST("/rest/api/v2/milestones", req, &milestone); err != nil {
		return nil, err
	}
	return &milestone, nil
}

func (c *Client) UpdateMilestoneInWorkspace(_, milestoneID int, req MilestoneUpdateRequest) (*Milestone, error) {
	var milestone Milestone
	if err := c.MERGEPATCH(fmt.Sprintf("/rest/api/v2/milestones/%d", milestoneID), req, &milestone); err != nil {
		return nil, err
	}
	return &milestone, nil
}

func (c *Client) DeleteMilestoneInWorkspace(_, milestoneID int) error {
	return c.DELETE(fmt.Sprintf("/rest/api/v2/milestones/%d", milestoneID))
}

// ResolveMilestoneID resolves a milestone name or ID to an ID. When workspaceID
// is non-nil the lookup uses the workspace-scoped list endpoint; otherwise it
// falls back to the global list (which only callers with global access can use).
func (c *Client) ResolveMilestoneID(nameOrID string, workspaceID *int) (int, error) {
	// Parse strict integer IDs before falling back to name lookup.
	if id, err := strconv.Atoi(nameOrID); err == nil {
		return id, nil
	}

	var resp *PaginatedResponse[Milestone]
	var err error
	if workspaceID != nil {
		resp, err = c.ListMilestonesInWorkspace(*workspaceID, nil)
	} else {
		resp, err = c.ListMilestones(nil)
	}
	if err != nil {
		return 0, err
	}

	nameLower := strings.ToLower(nameOrID)
	var bestMatch *Milestone

	for i := range resp.Data {
		m := &resp.Data[i]
		mNameLower := strings.ToLower(m.Name)

		if mNameLower == nameLower {
			return m.ID, nil
		}
		if bestMatch == nil && strings.Contains(mNameLower, nameLower) {
			bestMatch = m
		}
	}

	if bestMatch != nil {
		return bestMatch.ID, nil
	}

	return 0, fmt.Errorf("milestone not found: %s", nameOrID)
}

// SearchItems sends explicit CQL when asCQL is true; non-positive limits use
// the server default.
func (c *Client) SearchItems(query string, limit int, asCQL bool, workspaceID *int) (*PaginatedResponse[Item], error) {
	params := url.Values{}
	if asCQL {
		params.Set("ql", query)
	} else {
		params.Set("search", query)
	}
	if limit > 0 {
		params.Set("page_size", strconv.Itoa(limit))
	}
	if workspaceID != nil {
		params.Set("workspace_id", strconv.Itoa(*workspaceID))
	}

	var resp PaginatedResponse[Item]
	if err := c.GET("/rest/api/v2/items?"+params.Encode(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetItemHistory returns the full change history as a plain array.
func (c *Client) GetItemHistory(itemID int) ([]History, error) {
	var history []History
	if err := c.GET(fmt.Sprintf("/rest/api/v2/items/%d/history", itemID), &history); err != nil {
		return nil, err
	}
	return history, nil
}

// Work-item labels are separate from page labels.

func (c *Client) ListLabels(workspaceID int) ([]Label, error) {
	var doc DataDocument[[]Label]
	if err := c.GET(fmt.Sprintf("/rest/api/v2/workspaces/%d/labels", workspaceID), &doc); err != nil {
		return nil, err
	}
	return doc.Data, nil
}

func (c *Client) ListItemLabels(itemID int) ([]Label, error) {
	var doc DataDocument[[]Label]
	if err := c.GET(fmt.Sprintf("/rest/api/v2/items/%d/labels", itemID), &doc); err != nil {
		return nil, err
	}
	return doc.Data, nil
}

func (c *Client) SetItemLabels(itemID int, labelIDs []int) ([]Label, error) {
	var doc DataDocument[[]Label]
	if err := c.PUT(
		fmt.Sprintf("/rest/api/v2/items/%d/labels", itemID),
		ItemLabelSetRequest{LabelIDs: labelIDs},
		&doc,
	); err != nil {
		return nil, err
	}
	return doc.Data, nil
}

func (c *Client) AddItemLabel(itemID, labelID int) ([]Label, error) {
	var doc DataDocument[[]Label]
	if err := c.POST(
		fmt.Sprintf("/rest/api/v2/items/%d/labels", itemID),
		ItemLabelAddRequest{LabelID: labelID},
		&doc,
	); err != nil {
		return nil, err
	}
	return doc.Data, nil
}

func (c *Client) RemoveItemLabel(itemID, labelID int) error {
	return c.DELETE(fmt.Sprintf("/rest/api/v2/items/%d/labels/%d", itemID, labelID))
}

// Work-item templates are workspace-scoped and require item-templates:read.

// ListItemTemplates optionally filters templates by item type.
func (c *Client) ListItemTemplates(workspaceID, itemTypeID int) (ItemTemplateListResponse, error) {
	var resp ItemTemplateListResponse
	path := fmt.Sprintf("/rest/api/v2/workspaces/%d/item-templates", workspaceID)
	if itemTypeID > 0 {
		path += fmt.Sprintf("?item_type_id=%d", itemTypeID)
	}
	if err := c.GET(path, &resp); err != nil {
		return ItemTemplateListResponse{}, err
	}
	return resp, nil
}

func (c *Client) GetItemTemplate(workspaceID, templateID int) (*ItemTemplate, error) {
	var doc DataDocument[ItemTemplate]
	if err := c.GET(fmt.Sprintf("/rest/api/v2/workspaces/%d/item-templates/%d", workspaceID, templateID), &doc); err != nil {
		return nil, err
	}
	return &doc.Data, nil
}

// Custom-field methods.

// ListCustomFields requires the custom-fields:read scope.
func (c *Client) ListCustomFields() ([]CustomField, error) {
	var doc DataDocument[[]CustomField]
	if err := c.GET("/rest/api/v2/custom-fields", &doc); err != nil {
		return nil, err
	}
	return doc.Data, nil
}

// Iteration API methods.

// ListIterations requires the iterations:read scope.
func (c *Client) ListIterations(filters map[string]string) (*PaginatedResponse[Iteration], error) {
	path := "/rest/api/v2/iterations?scope=global"
	if len(filters) > 0 {
		params := url.Values{"scope": {"global"}}
		for k, v := range filters {
			params.Set(v2QueryName(k), v)
		}
		path = "/rest/api/v2/iterations?" + params.Encode()
	}

	var resp PaginatedResponse[Iteration]
	if err := c.GET(path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) ListIterationsInWorkspace(workspaceID int, filters map[string]string) (*PaginatedResponse[Iteration], error) {
	params := url.Values{"workspace_id": {strconv.Itoa(workspaceID)}}
	if len(filters) > 0 {
		for k, v := range filters {
			params.Set(v2QueryName(k), v)
		}
	}
	path := "/rest/api/v2/iterations?" + params.Encode()

	var resp PaginatedResponse[Iteration]
	if err := c.GET(path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ResolveIterationID resolves an iteration name or ID to an ID. Mirrors
// ResolveMilestoneID: numeric input passes through; otherwise the lookup is
// fuzzy (exact case-insensitive first, then first substring match) against
// the workspace-scoped list when workspaceID is non-nil, else the global list.
func (c *Client) ResolveIterationID(nameOrID string, workspaceID *int) (int, error) {
	// Use Atoi so malformed inputs like "123abc" do not resolve as ID 123.
	if id, err := strconv.Atoi(nameOrID); err == nil {
		return id, nil
	}

	var resp *PaginatedResponse[Iteration]
	var err error
	if workspaceID != nil {
		resp, err = c.ListIterationsInWorkspace(*workspaceID, nil)
	} else {
		resp, err = c.ListIterations(nil)
	}
	if err != nil {
		return 0, err
	}

	nameLower := strings.ToLower(nameOrID)
	var bestMatch *Iteration

	for i := range resp.Data {
		it := &resp.Data[i]
		itNameLower := strings.ToLower(it.Name)

		// Exact match (case-insensitive)
		if itNameLower == nameLower {
			return it.ID, nil
		}
		// Partial match - prefer first match
		if bestMatch == nil && strings.Contains(itNameLower, nameLower) {
			bestMatch = it
		}
	}

	if bestMatch != nil {
		return bestMatch.ID, nil
	}

	return 0, fmt.Errorf("iteration not found: %s", nameOrID)
}

// Lookup helpers.

func (c *Client) ResolveWorkspaceID(keyOrID string) (int, error) {
	// Parse a strict integer ID before falling back to name lookup.
	if id, err := strconv.Atoi(keyOrID); err == nil {
		return id, nil
	}

	// Look up by key from workspace list
	workspaces, err := c.ListWorkspaces()
	if err != nil {
		return 0, fmt.Errorf("failed to list workspaces: %w", err)
	}

	for _, ws := range workspaces.Data {
		if strings.EqualFold(ws.Key, keyOrID) {
			return ws.ID, nil
		}
	}

	return 0, fmt.Errorf("workspace not found: %s", keyOrID)
}

func (c *Client) ResolveItemID(keyOrID string) (int, error) {
	// Try parsing as integer first. Use Atoi so malformed inputs like
	// "123abc" do not accidentally resolve as ID 123.
	if id, err := strconv.Atoi(keyOrID); err == nil {
		return id, nil
	}

	// Parse as workspace key + item number (e.g., PROJ-123). Split on
	// the last dash so workspace keys that themselves contain dashes (notably
	// personal workspace keys) still resolve correctly.
	dash := strings.LastIndex(keyOrID, "-")
	if dash <= 0 || dash == len(keyOrID)-1 {
		return 0, fmt.Errorf("invalid item identifier: %s (expected ID or KEY-NUMBER format)", keyOrID)
	}

	wsKey := keyOrID[:dash]
	itemNum, err := strconv.Atoi(keyOrID[dash+1:])
	if err != nil {
		return 0, fmt.Errorf("invalid item number in: %s", keyOrID)
	}

	item, err := c.GetItemByKeyAndNumber(wsKey, itemNum)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
			return 0, fmt.Errorf("item not found: %s", keyOrID)
		}
		return 0, err
	}
	return item.ID, nil
}

// Pages API methods.

// AgentSkill is the agent-skills payload.
type AgentSkill struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Body        string `json:"body,omitempty"`
	Enabled     bool   `json:"enabled"`
}

func (c *Client) ListAgentSkills(workspaceID int) ([]AgentSkill, error) {
	var skills []AgentSkill
	if err := c.GET(fmt.Sprintf("/rest/api/v2/workspaces/%d/agent-skills", workspaceID), &skills); err != nil {
		return nil, err
	}
	return skills, nil
}

func (c *Client) GetAgentSkill(workspaceID, skillID int) (*AgentSkill, error) {
	var skill AgentSkill
	if err := c.GET(fmt.Sprintf("/rest/api/v2/workspaces/%d/agent-skills/%d", workspaceID, skillID), &skill); err != nil {
		return nil, err
	}
	return &skill, nil
}

// ListPages returns every visible page in server sort order.
func (c *Client) ListPages(workspaceID int) ([]Page, error) {
	var pages []Page
	if err := c.GET(fmt.Sprintf("/rest/api/v2/workspaces/%d/pages", workspaceID), &pages); err != nil {
		return nil, err
	}
	return pages, nil
}

// SearchPages searches visible page titles and content. Results omit bodies.
func (c *Client) SearchPages(workspaceID int, query string, limit int) ([]Page, error) {
	params := url.Values{}
	params.Set("q", query)
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	var pages []Page
	if err := c.GET(fmt.Sprintf("/rest/api/v2/workspaces/%d/pages/search?%s", workspaceID, params.Encode()), &pages); err != nil {
		return nil, err
	}
	return pages, nil
}

func (c *Client) GetPage(workspaceID, pageID int) (*Page, error) {
	var page Page
	if err := c.GET(fmt.Sprintf("/rest/api/v2/workspaces/%d/pages/%d", workspaceID, pageID), &page); err != nil {
		return nil, err
	}
	return &page, nil
}

func (c *Client) CreatePage(workspaceID int, req PageCreateRequest) (*Page, error) {
	var page Page
	if err := c.POST(fmt.Sprintf("/rest/api/v2/workspaces/%d/pages", workspaceID), req, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

// UpdatePage applies a partial page update; nil fields remain unchanged.
func (c *Client) UpdatePage(workspaceID, pageID int, req PageUpdateRequest) (*Page, error) {
	var page Page
	if err := c.MERGEPATCH(fmt.Sprintf("/rest/api/v2/workspaces/%d/pages/%d", workspaceID, pageID), req, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

func (c *Client) ListPageDiagrams(workspaceID, pageID int) ([]PageDiagram, error) {
	var doc DataDocument[[]PageDiagram]
	path := fmt.Sprintf("/rest/api/v2/workspaces/%d/pages/%d/diagrams", workspaceID, pageID)
	if err := c.GET(path, &doc); err != nil {
		return nil, err
	}
	return doc.Data, nil
}

func (c *Client) GetPageDiagram(workspaceID, pageID, attachmentID int) (*PageDiagram, error) {
	var doc DataDocument[PageDiagram]
	path := fmt.Sprintf("/rest/api/v2/workspaces/%d/pages/%d/diagrams/%d", workspaceID, pageID, attachmentID)
	if err := c.GET(path, &doc); err != nil {
		return nil, err
	}
	return &doc.Data, nil
}

// CreatePageDiagram uploads an immutable diagram attachment and inserts its
// Markdown fence into the Page.
func (c *Client) CreatePageDiagram(workspaceID, pageID int, req PageDiagramCreateRequest) (*PageDiagram, error) {
	var doc DataDocument[PageDiagram]
	path := fmt.Sprintf("/rest/api/v2/workspaces/%d/pages/%d/diagrams", workspaceID, pageID)
	if err := c.POST(path, req, &doc); err != nil {
		return nil, err
	}
	return &doc.Data, nil
}

// UpdatePageDiagram creates a replacement attachment and atomically replaces
// the matching fence in the Page.
func (c *Client) UpdatePageDiagram(workspaceID, pageID, attachmentID int, req PageDiagramUpdateRequest) (*PageDiagram, error) {
	var doc DataDocument[PageDiagram]
	path := fmt.Sprintf("/rest/api/v2/workspaces/%d/pages/%d/diagrams/%d", workspaceID, pageID, attachmentID)
	if err := c.MERGEPATCH(path, req, &doc); err != nil {
		return nil, err
	}
	return &doc.Data, nil
}

// MovePage reparents a page. Pass parentID=nil to move to the workspace
// root. An optional destination workspace ID moves the whole subtree across
// workspaces; prevSiblingID / nextSiblingID place it among destination
// siblings. Pass nil for optional values to let the server pick defaults.
func (c *Client) MovePage(workspaceID, pageID int, parentID, prevSiblingID, nextSiblingID *int, destinationWorkspaceID ...*int) (*Page, error) {
	var page Page
	req := PageMoveRequest{
		ParentID:      parentID,
		PrevSiblingID: prevSiblingID,
		NextSiblingID: nextSiblingID,
	}
	if len(destinationWorkspaceID) > 0 {
		req.DestinationWorkspaceID = destinationWorkspaceID[0]
	}
	if err := c.POST(fmt.Sprintf("/rest/api/v2/workspaces/%d/pages/%d/move", workspaceID, pageID), req, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

// ArchivePage soft-deletes a page and its entire subtree.
func (c *Client) ArchivePage(workspaceID, pageID int) error {
	return c.DELETE(fmt.Sprintf("/rest/api/v2/workspaces/%d/pages/%d", workspaceID, pageID))
}

// GetPageHistory returns revisions for a page newest-first. Optional
// pagination arguments are limit, offset (kept variadic for compatibility with
// existing call sites/tests that used the original two-argument form).
func (c *Client) GetPageHistory(workspaceID, pageID int, pagination ...int) ([]PageRevision, error) {
	var resp PaginatedResponse[PageRevision]
	endpoint := fmt.Sprintf("/rest/api/v2/workspaces/%d/pages/%d/history", workspaceID, pageID)
	limit, offset := 0, 0
	if len(pagination) > 0 {
		limit = pagination[0]
	}
	if len(pagination) > 1 {
		offset = pagination[1]
	}
	q := url.Values{}
	if limit > 0 {
		q.Set("page_size", fmt.Sprintf("%d", limit))
	}
	if offset > 0 {
		pageSize := limit
		if pageSize == 0 {
			pageSize = 50
		}
		q.Set("page", fmt.Sprintf("%d", offset/pageSize+1))
	}
	if encoded := q.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	if err := c.GET(endpoint, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

func (c *Client) GetPageRevision(workspaceID, pageID, revisionID int) (*PageRevision, error) {
	var rev PageRevision
	if err := c.GET(fmt.Sprintf("/rest/api/v2/workspaces/%d/pages/%d/history/%d", workspaceID, pageID, revisionID), &rev); err != nil {
		return nil, err
	}
	return &rev, nil
}

func (c *Client) RestorePageRevision(workspaceID, pageID, revisionID int) (*Page, error) {
	var page Page
	if err := c.POST(fmt.Sprintf("/rest/api/v2/workspaces/%d/pages/%d/history/%d/restore", workspaceID, pageID, revisionID), map[string]any{}, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

func (c *Client) GetPagePermissions(workspaceID, pageID int) (*PagePermissions, error) {
	var perms PagePermissions
	if err := c.GET(fmt.Sprintf("/rest/api/v2/workspaces/%d/pages/%d/permissions", workspaceID, pageID), &perms); err != nil {
		return nil, err
	}
	return &perms, nil
}

func (c *Client) GrantPagePermission(workspaceID, pageID int, req PageGrantPermissionRequest) (*PagePermission, error) {
	var perm PagePermission
	if err := c.POST(fmt.Sprintf("/rest/api/v2/workspaces/%d/pages/%d/permissions", workspaceID, pageID), req, &perm); err != nil {
		return nil, err
	}
	return &perm, nil
}

func (c *Client) RevokePagePermission(workspaceID, pageID, permissionID int) error {
	return c.DELETE(fmt.Sprintf("/rest/api/v2/workspaces/%d/pages/%d/permissions/%d", workspaceID, pageID, permissionID))
}

func (c *Client) SetPageInheritance(workspaceID, pageID int, inherit bool) (*Page, error) {
	var page Page
	if err := c.PATCH(fmt.Sprintf("/rest/api/v2/workspaces/%d/pages/%d/inheritance", workspaceID, pageID), PageSetInheritanceRequest{InheritPermissions: inherit}, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

// Page labels are separate from work-item labels.

func (c *Client) ListPageLabels(workspaceID int) ([]PageLabel, error) {
	var doc DataDocument[[]PageLabel]
	if err := c.GET(fmt.Sprintf("/rest/api/v2/workspaces/%d/page-labels", workspaceID), &doc); err != nil {
		return nil, err
	}
	return doc.Data, nil
}

func (c *Client) GetPageLabel(workspaceID, labelID int) (*PageLabel, error) {
	var doc DataDocument[PageLabel]
	if err := c.GET(fmt.Sprintf("/rest/api/v2/workspaces/%d/page-labels/%d", workspaceID, labelID), &doc); err != nil {
		return nil, err
	}
	return &doc.Data, nil
}

func (c *Client) CreatePageLabel(workspaceID int, req PageLabelCreateRequest) (*PageLabel, error) {
	var doc DataDocument[PageLabel]
	if err := c.POST(fmt.Sprintf("/rest/api/v2/workspaces/%d/page-labels", workspaceID), req, &doc); err != nil {
		return nil, err
	}
	return &doc.Data, nil
}

func (c *Client) UpdatePageLabel(workspaceID, labelID int, req PageLabelUpdateRequest) (*PageLabel, error) {
	var doc DataDocument[PageLabel]
	if err := c.MERGEPATCH(fmt.Sprintf("/rest/api/v2/workspaces/%d/page-labels/%d", workspaceID, labelID), req, &doc); err != nil {
		return nil, err
	}
	return &doc.Data, nil
}

func (c *Client) DeletePageLabel(workspaceID, labelID int) error {
	return c.DELETE(fmt.Sprintf("/rest/api/v2/workspaces/%d/page-labels/%d", workspaceID, labelID))
}

func (c *Client) ListPageLabelsForPage(workspaceID, pageID int) ([]PageLabel, error) {
	var doc DataDocument[[]PageLabel]
	if err := c.GET(fmt.Sprintf("/rest/api/v2/workspaces/%d/pages/%d/labels", workspaceID, pageID), &doc); err != nil {
		return nil, err
	}
	return doc.Data, nil
}

func (c *Client) SetPageLabelsForPage(workspaceID, pageID int, labelIDs []int) ([]PageLabel, error) {
	var doc DataDocument[[]PageLabel]
	if err := c.PUT(
		fmt.Sprintf("/rest/api/v2/workspaces/%d/pages/%d/labels", workspaceID, pageID),
		PageLabelSetRequest{LabelIDs: labelIDs},
		&doc,
	); err != nil {
		return nil, err
	}
	return doc.Data, nil
}

func (c *Client) AddPageLabelToPage(workspaceID, pageID, labelID int) ([]PageLabel, error) {
	var doc DataDocument[[]PageLabel]
	if err := c.POST(
		fmt.Sprintf("/rest/api/v2/workspaces/%d/pages/%d/labels", workspaceID, pageID),
		PageLabelAddRequest{LabelID: labelID},
		&doc,
	); err != nil {
		return nil, err
	}
	return doc.Data, nil
}

func (c *Client) RemovePageLabelFromPage(workspaceID, pageID, labelID int) error {
	return c.DELETE(fmt.Sprintf("/rest/api/v2/workspaces/%d/pages/%d/labels/%d", workspaceID, pageID, labelID))
}

// Links API methods cover item, page, and test-case relationships.

// ListLinkTypes returns active link types and the system catalog.
func (c *Client) ListLinkTypes() ([]LinkType, error) {
	var types []LinkType
	if err := c.GET("/rest/api/v2/link-types", &types); err != nil {
		return nil, err
	}
	return types, nil
}

// CreateLink creates a cross-entity link. The server enforces the
// link-type / entity-type compatibility check; the CLI front-loads an
// obvious-mismatch check for a friendlier error.
func (c *Client) CreateLink(req LinkCreateRequest) (*ItemLink, error) {
	var link ItemLink
	if err := c.POST("/rest/api/v2/links", req, &link); err != nil {
		return nil, err
	}
	return &link, nil
}

// ListLinksForEntity returns outgoing and incoming links for a single
// entity. The route prefix depends on entityType — items, pages, and
// test cases each get their own list endpoint that funnels into the
// same handler.
func (c *Client) ListLinksForEntity(entityType string, id int) (*LinkListResponse, error) {
	var route string
	switch entityType {
	case "item":
		route = fmt.Sprintf("/rest/api/v2/items/%d/links", id)
	case "page":
		route = fmt.Sprintf("/rest/api/v2/pages/%d/links", id)
	case "test_case":
		route = fmt.Sprintf("/rest/api/v2/test-cases/%d/links", id)
	default:
		return nil, fmt.Errorf("unsupported entity type %q (want item, page, or test_case)", entityType)
	}
	var resp LinkListResponse
	if err := c.GET(route, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// DeleteLink removes a link by ID; the server enforces source edit permission.
func (c *Client) DeleteLink(id int) error {
	return c.DELETE(fmt.Sprintf("/rest/api/v2/links/%d", id))
}

// Asset API methods.

// ListAssets returns a paginated, filtered asset list.
func (c *Client) ListAssets(setID int, filters map[string]string) (*PaginatedResponse[Asset], error) {
	path := fmt.Sprintf("/rest/api/v2/asset-sets/%d/assets", setID)
	if len(filters) > 0 {
		params := url.Values{}
		for k, v := range filters {
			if v != "" {
				params.Set(k, v)
			}
		}
		if encoded := params.Encode(); encoded != "" {
			path += "?" + encoded
		}
	}
	var resp PaginatedResponse[Asset]
	if err := c.GET(path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetAsset(id int) (*Asset, error) {
	var a Asset
	if err := c.GET(fmt.Sprintf("/rest/api/v2/assets/%d", id), &a); err != nil {
		return nil, err
	}
	return &a, nil
}

func (c *Client) CreateAsset(setID int, req AssetCreateRequest) (*Asset, error) {
	var a Asset
	if err := c.POST(fmt.Sprintf("/rest/api/v2/asset-sets/%d/assets", setID), req, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

// UpdateAsset applies only the non-nil fields in req.
func (c *Client) UpdateAsset(id int, req AssetUpdateRequest) (*Asset, error) {
	var a Asset
	if err := c.MERGEPATCH(fmt.Sprintf("/rest/api/v2/assets/%d", id), req, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

// DeleteAsset removes an asset and its item links; it requires assets:delete.
func (c *Client) DeleteAsset(id int) error {
	return c.DELETE(fmt.Sprintf("/rest/api/v2/assets/%d", id))
}

func (c *Client) ListAssetSets() ([]AssetSet, error) {
	var sets []AssetSet
	if err := c.GET("/rest/api/v2/asset-sets", &sets); err != nil {
		return nil, err
	}
	return sets, nil
}

func (c *Client) GetAssetSet(id int) (*AssetSet, error) {
	var s AssetSet
	if err := c.GET(fmt.Sprintf("/rest/api/v2/asset-sets/%d", id), &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (c *Client) ListAssetTypes(setID int) ([]AssetType, error) {
	var types []AssetType
	if err := c.GET(fmt.Sprintf("/rest/api/v2/asset-sets/%d/types", setID), &types); err != nil {
		return nil, err
	}
	return types, nil
}

func (c *Client) GetAssetType(id int) (*AssetType, error) {
	var t AssetType
	if err := c.GET(fmt.Sprintf("/rest/api/v2/asset-types/%d", id), &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (c *Client) ListAssetCategories(setID int) ([]AssetCategory, error) {
	var cats []AssetCategory
	if err := c.GET(fmt.Sprintf("/rest/api/v2/asset-sets/%d/categories", setID), &cats); err != nil {
		return nil, err
	}
	return cats, nil
}

func (c *Client) ListAssetStatuses(setID int) ([]AssetStatus, error) {
	var statuses []AssetStatus
	if err := c.GET(fmt.Sprintf("/rest/api/v2/asset-sets/%d/statuses", setID), &statuses); err != nil {
		return nil, err
	}
	return statuses, nil
}

// ImportAssetsCSV uploads a CSV and queues a v2 import job.
func (c *Client) ImportAssetsCSV(setID, assetTypeID int, statusID, categoryID *int, filename string, body io.Reader) (*AssetImportJob, error) {
	var buf bytes.Buffer
	mp := multipart.NewWriter(&buf)
	if err := mp.WriteField("has_header", "true"); err != nil {
		return nil, fmt.Errorf("multipart write has_header: %w", err)
	}
	part, err := mp.CreateFormFile("file", filename)
	if err != nil {
		return nil, fmt.Errorf("multipart create file: %w", err)
	}
	if _, err := io.Copy(part, body); err != nil {
		return nil, fmt.Errorf("multipart copy file: %w", err)
	}
	if err := mp.Close(); err != nil {
		return nil, fmt.Errorf("multipart close: %w", err)
	}

	path := fmt.Sprintf("/rest/api/v2/asset-sets/%d/import/upload", setID)
	endpoint := c.baseURL + path
	req, err := http.NewRequest(http.MethodPost, endpoint, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", mp.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, decodeAPIError(resp.StatusCode, respBody)
	}
	var upload DataDocument[AssetCSVUpload]
	if err := json.Unmarshal(respBody, &upload); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if upload.Data.UploadID == "" {
		return nil, errors.New("import upload response is missing upload_id")
	}
	titleColumn := findAssetImportColumn(upload.Data.Headers, "title", "name", "asset name", "asset_name")
	if titleColumn < 0 {
		return nil, errors.New("import CSV must contain a title or name column")
	}
	mappings := map[string]any{
		"title":       titleColumn,
		"description": findAssetImportColumn(upload.Data.Headers, "description", "desc", "details", "notes"),
		"asset_tag":   findAssetImportColumn(upload.Data.Headers, "asset tag", "asset_tag", "tag", "serial", "serial number", "serial_number", "id"),
		"category_id": -1,
		"status_id":   -1,
	}
	request := map[string]any{
		"upload_id": upload.Data.UploadID, "asset_type_id": assetTypeID,
		"default_status_id": statusID, "default_category_id": categoryID,
		"mappings": mappings, "has_header": true, "delimiter": upload.Data.Delimiter,
	}
	var job AssetImportJob
	if err := c.POST(fmt.Sprintf("/rest/api/v2/asset-sets/%d/import/start", setID), request, &job); err != nil {
		return nil, err
	}
	return &job, nil
}

func findAssetImportColumn(headers []string, names ...string) int {
	for i, header := range headers {
		header = strings.ToLower(strings.TrimSpace(header))
		if header == "" {
			continue
		}
		for _, name := range names {
			if header == name || strings.Contains(header, name) || strings.Contains(name, header) {
				return i
			}
		}
	}
	return -1
}

// Time-tracking methods.

func (c *Client) ListTimeProjects() ([]TimeProject, error) {
	var projects []TimeProject
	if err := c.GET("/rest/api/v2/time/projects", &projects); err != nil {
		return nil, err
	}
	return projects, nil
}

func (c *Client) ListTimeWorklogs(filters map[string]string) (*PaginatedResponse[TimeWorklog], error) {
	path := "/rest/api/v2/time/worklogs"
	if len(filters) > 0 {
		params := url.Values{}
		for k, v := range filters {
			switch k {
			case "date_from":
				k = "from"
			case "date_to":
				k = "to"
			}
			params.Set(k, v)
		}
		path += "?" + params.Encode()
	}
	var resp PaginatedResponse[TimeWorklog]
	if err := c.GET(path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) CreateTimeWorklog(req TimeWorklogCreateRequest) (map[string]any, error) {
	var doc DataDocument[map[string]any]
	if err := c.POST("/rest/api/v2/time/worklogs", req, &doc); err != nil {
		return nil, err
	}
	return doc.Data, nil
}

func (c *Client) UpdateTimeWorklog(id int, description string) error {
	body := map[string]string{"description": description}
	var doc DataDocument[map[string]any]
	return c.MERGEPATCH(fmt.Sprintf("/rest/api/v2/time/worklogs/%d", id), body, &doc)
}

func (c *Client) DeleteTimeWorklog(id int) error {
	return c.DELETE(fmt.Sprintf("/rest/api/v2/time/worklogs/%d", id))
}

func (c *Client) StartTimer(req TimerStartRequest) (map[string]any, error) {
	var out map[string]any
	if err := c.POST("/rest/api/v2/timer/start", req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetActiveTimer() (map[string]any, error) {
	var out map[string]any
	if err := c.GET("/rest/api/v2/timer/active", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) StopTimer() (map[string]any, error) {
	var out map[string]any
	// DELETE on /timer/stop returns a JSON body; use a custom request so we
	// can pass a result target (the convenience Delete method discards the body).
	if err := c.doRequest("DELETE", "/rest/api/v2/timer/stop", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}
