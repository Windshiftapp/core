package zammad

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"windshift/internal/models"
)

type Response struct {
	StatusCode int
	Body       []byte
}

type Transport interface {
	Do(ctx context.Context, method, targetURL string, body []byte, headers map[string]string) (*Response, error)
}

type TransportFunc func(context.Context, string, string, []byte, map[string]string) (*Response, error)

func (f TransportFunc) Do(ctx context.Context, method, targetURL string, body []byte, headers map[string]string) (*Response, error) {
	return f(ctx, method, targetURL, body, headers)
}

type APIError struct {
	StatusCode int
}

func (e *APIError) Error() string {
	return fmt.Sprintf("Zammad API returned HTTP %d", e.StatusCode)
}

// UpstreamError represents a network or response-shape failure without
// exposing the remote response or request credentials through Error().
type UpstreamError struct{ Cause error }

func (e *UpstreamError) Error() string { return "Zammad request failed" }
func (e *UpstreamError) Unwrap() error { return e.Cause }

type Client struct {
	baseURL   string
	authValue string
	transport Transport
}

func NewClient(baseURL, apiToken string, transport Transport) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), authValue: "Token token=" + apiToken, transport: transport}
}

// NewOAuthClient uses OAuth bearer authentication. Keeping NewClient unchanged
// preserves the API-token wire format used by existing Zammad connections.
func NewOAuthClient(baseURL, accessToken string, transport Transport) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), authValue: "Bearer " + accessToken, transport: transport}
}

type Ticket struct {
	ID         int
	Number     string
	GroupID    int
	GroupName  string
	StateID    int
	StateName  string
	OwnerID    int
	OwnerName  string
	Attributes map[string]json.RawMessage
}

type Owner struct {
	ID   int
	Name string
}

// States returns the active ticket states. Zammad permits this endpoint for
// ordinary ticket agents, so it is safe for runtime use.
func (c *Client) States(ctx context.Context) ([]models.ZammadState, error) {
	states := []models.ZammadState{}
	if err := c.getJSON(ctx, "/api/v1/ticket_states", &states); err != nil {
		return nil, err
	}
	activeStates := states[:0]
	for _, state := range states {
		if state.Active {
			activeStates = append(activeStates, state)
		}
	}
	return activeStates, nil
}

func (c *Client) FindByCorrelation(ctx context.Context, field, value string) (*Ticket, error) {
	query := url.Values{}
	query.Set("query", field+":"+strconv.Quote(value))
	query.Set("per_page", "10")
	var rawTickets []map[string]json.RawMessage
	if err := c.getJSON(ctx, "/api/v1/tickets/search?"+query.Encode(), &rawTickets); err != nil {
		return nil, err
	}
	for _, raw := range rawTickets {
		var correlation string
		if fieldValue, ok := raw[field]; ok {
			_ = json.Unmarshal(fieldValue, &correlation)
		}
		if correlation != value {
			continue
		}
		ticket, err := decodeTicket(raw)
		if err != nil {
			return nil, &UpstreamError{Cause: err}
		}
		return ticket, nil
	}
	return nil, nil
}

// FindByNumber performs an exact match after asking Zammad to narrow the
// result set. The local check is essential because the search endpoint is not
// an exact-match API.
func (c *Client) FindByNumber(ctx context.Context, number string) (*Ticket, error) {
	query := url.Values{}
	query.Set("query", "number:"+strconv.Quote(number))
	query.Set("per_page", "10")
	var rawTickets []map[string]json.RawMessage
	if err := c.getJSON(ctx, "/api/v1/tickets/search?"+query.Encode(), &rawTickets); err != nil {
		return nil, err
	}
	for _, raw := range rawTickets {
		ticket, err := decodeTicket(raw)
		if err != nil {
			return nil, &UpstreamError{Cause: err}
		}
		if ticket.Number == number {
			return ticket, nil
		}
	}
	return nil, nil
}

func (c *Client) CreateTicket(ctx context.Context, title, body, customer string, groupID int, correlationField, correlationValue string) (*Ticket, error) {
	payload := map[string]any{
		"title":    title,
		"group_id": groupID,
		"customer": customer,
		"article": map[string]any{
			"subject":  title,
			"body":     body,
			"type":     "note",
			"sender":   "Agent",
			"internal": true,
		},
		correlationField: correlationValue,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := c.requestJSON(ctx, http.MethodPost, "/api/v1/tickets", encoded, &raw); err != nil {
		return nil, err
	}
	ticket, err := decodeTicket(raw)
	if err != nil {
		return nil, &UpstreamError{Cause: err}
	}
	return ticket, nil
}

func (c *Client) GetTicket(ctx context.Context, ticketID int) (*Ticket, error) {
	var raw map[string]json.RawMessage
	path := "/api/v1/tickets/" + strconv.Itoa(ticketID) + "?expand=true"
	if err := c.getJSON(ctx, path, &raw); err != nil {
		return nil, err
	}
	ticket, err := decodeTicket(raw)
	if err != nil {
		return nil, &UpstreamError{Cause: err}
	}
	return ticket, nil
}

func (c *Client) UpdateTicket(ctx context.Context, ticketID int, stateID, groupID, ownerID *int, correlationField, correlationValue string) (*Ticket, error) {
	payload := map[string]any{}
	if stateID != nil {
		payload["state_id"] = *stateID
	}
	if groupID != nil {
		payload["group_id"] = *groupID
	}
	if ownerID != nil {
		payload["owner_id"] = *ownerID
	}
	if correlationField != "" {
		payload[correlationField] = correlationValue
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := c.requestJSON(ctx, http.MethodPut, "/api/v1/tickets/"+strconv.Itoa(ticketID), encoded, &raw); err != nil {
		return nil, err
	}
	ticket, err := decodeTicket(raw)
	if err != nil {
		return nil, &UpstreamError{Cause: err}
	}
	return ticket, nil
}

// Owners searches on the effective target group. Zammad only keeps an
// assignment when the selected agent has full access to the ticket group.
func (c *Client) Owners(ctx context.Context, groupID int) ([]Owner, error) {
	const pageSize = 100
	owners := make([]Owner, 0)
	for page := 1; ; page++ {
		query := url.Values{}
		query.Set("query", "*")
		// Zammad's label/term search form is intentionally compact. Request the
		// documented expanded record form explicitly so owner validation always
		// receives active, group_ids, and display-name fields.
		query.Set("expand", "true")
		query.Add("permissions[]", "ticket.agent")
		query.Set(fmt.Sprintf("group_ids[%d]", groupID), "full")
		query.Set("page", strconv.Itoa(page))
		query.Set("per_page", strconv.Itoa(pageSize))
		var rawUsers []map[string]json.RawMessage
		if err := c.getJSON(ctx, "/api/v1/users/search?"+query.Encode(), &rawUsers); err != nil {
			return nil, err
		}
		for _, raw := range rawUsers {
			var id int
			var active bool
			var first, last, login string
			if json.Unmarshal(raw["id"], &id) != nil || id <= 0 {
				continue
			}
			if value, ok := raw["active"]; ok {
				if err := json.Unmarshal(value, &active); err != nil || !active {
					continue
				}
			}
			var groups map[string][]string
			groupValue, ok := raw["group_ids"]
			if !ok || json.Unmarshal(groupValue, &groups) != nil || !containsAccess(groups[strconv.Itoa(groupID)], "full") {
				continue
			}
			_ = json.Unmarshal(raw["firstname"], &first)
			_ = json.Unmarshal(raw["lastname"], &last)
			_ = json.Unmarshal(raw["login"], &login)
			name := strings.TrimSpace(first + " " + last)
			if name == "" {
				name = login
			}
			owners = append(owners, Owner{ID: id, Name: name})
		}
		if len(rawUsers) < pageSize {
			break
		}
	}
	return owners, nil
}

func containsAccess(values []string, accepted ...string) bool {
	for _, value := range values {
		for _, want := range accepted {
			if value == want {
				return true
			}
		}
	}
	return false
}

func (c *Client) getJSON(ctx context.Context, path string, target any) error {
	return c.requestJSON(ctx, http.MethodGet, path, nil, target)
}

func (c *Client) requestJSON(ctx context.Context, method, path string, body []byte, target any) error {
	if c.transport == nil {
		return errors.New("zammad transport is not configured")
	}
	response, err := c.transport.Do(ctx, method, c.baseURL+path, body, map[string]string{
		"Accept":        "application/json",
		"Content-Type":  "application/json",
		"Authorization": c.authValue,
	})
	if err != nil {
		return &UpstreamError{Cause: err}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return &APIError{StatusCode: response.StatusCode}
	}
	if err := json.Unmarshal(response.Body, target); err != nil {
		return &UpstreamError{Cause: errors.New("zammad API returned invalid JSON")}
	}
	return nil
}

func decodeTicket(raw map[string]json.RawMessage) (*Ticket, error) {
	ticket := &Ticket{Attributes: raw}
	if err := decodeRequired(raw, "id", &ticket.ID); err != nil {
		return nil, err
	}
	if err := decodeRequired(raw, "number", &ticket.Number); err != nil {
		return nil, err
	}
	_ = json.Unmarshal(raw["group_id"], &ticket.GroupID)
	_ = json.Unmarshal(raw["group"], &ticket.GroupName)
	_ = json.Unmarshal(raw["state_id"], &ticket.StateID)
	_ = json.Unmarshal(raw["state"], &ticket.StateName)
	_ = json.Unmarshal(raw["owner_id"], &ticket.OwnerID)
	_ = json.Unmarshal(raw["owner"], &ticket.OwnerName)
	return ticket, nil
}

func decodeRequired(raw map[string]json.RawMessage, field string, target any) error {
	value, ok := raw[field]
	if !ok {
		return fmt.Errorf("zammad ticket response is missing %s", field)
	}
	if err := json.Unmarshal(value, target); err != nil {
		return fmt.Errorf("zammad ticket response has invalid %s", field)
	}
	return nil
}
