package zammad

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

type httpTestTransport struct{ client *http.Client }

func (t httpTestTransport) Do(ctx context.Context, method, targetURL string, body []byte, headers map[string]string) (*Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return &Response{StatusCode: resp.StatusCode, Body: responseBody}, nil
}

func TestClientStatesCorrelationSearchAndTicketCreation(t *testing.T) {
	t.Parallel()
	const token = "synthetic-secret-token"
	postCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Token token="+token {
			t.Fatalf("unexpected authorization header: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/ticket_states":
			_, _ = w.Write([]byte(`[{"id":4,"name":"closed","state_type_id":5,"active":true},{"id":5,"name":"inactive","active":false}]`))
		case r.URL.Path == "/api/v1/tickets/search":
			_, _ = w.Write([]byte(`[{"id":9,"number":"42009","group_id":1,"state_id":2,"state":"open","windshift_item_key":"windshift:abc:ITEM-49"}]`))
		case r.URL.Path == "/api/v1/tickets" && r.Method == http.MethodPost:
			postCount++
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"windshift_item_key":"windshift:abc:ITEM-50"`) ||
				!strings.Contains(string(body), `"group_id":7`) || strings.Contains(string(body), `"group":`) {
				t.Fatalf("ticket payload lacks correlation field: %s", body)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":10,"number":"42010","group_id":1,"state_id":2,"state":"open"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, token, httpTestTransport{client: server.Client()})
	states, err := client.States(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 || states[0].Name != "closed" {
		t.Fatalf("unexpected states: %#v", states)
	}
	found, err := client.FindByCorrelation(context.Background(), "windshift_item_key", "windshift:abc:ITEM-49")
	if err != nil || found == nil || found.ID != 9 {
		t.Fatalf("unexpected search result: ticket=%#v err=%v", found, err)
	}
	created, err := client.CreateTicket(context.Background(), "ITEM-50", "Synthetic body", "robot@example.test", 7, "windshift_item_key", "windshift:abc:ITEM-50")
	if err != nil || created.ID != 10 || postCount != 1 {
		t.Fatalf("unexpected create result: ticket=%#v posts=%d err=%v", created, postCount, err)
	}
}

func TestClientRedactsAPIErrorBodyAndToken(t *testing.T) {
	t.Parallel()
	const token = "must-not-appear"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"token must-not-appear is invalid"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, token, httpTestTransport{client: server.Client()})
	_, err := client.States(context.Background())
	if err == nil {
		t.Fatal("expected authentication error")
	}
	if strings.Contains(err.Error(), token) || strings.Contains(err.Error(), "invalid") {
		t.Fatalf("API error disclosed response content: %v", err)
	}
}

func TestClientHonorsContextTimeout(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	client := NewClient(server.URL, "token", httpTestTransport{client: server.Client()})
	_, err := client.States(ctx)
	if err == nil {
		t.Fatal("expected timeout")
	}
}

func TestClientFindByNumberRequiresExactMatch(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/tickets/search" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("query"); got != `number:"42009"` {
			t.Fatalf("unexpected ticket-number query: %q", got)
		}
		_, _ = w.Write([]byte(`[{"id":9,"number":"420090"},{"id":10,"number":"42008"}]`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", httpTestTransport{client: server.Client()})
	found, err := client.FindByNumber(context.Background(), "42009")
	if err != nil {
		t.Fatal(err)
	}
	if found != nil {
		t.Fatalf("non-exact search result was accepted: %#v", found)
	}
}

func TestClientOwnersUsesGroupPermissionQueryAndFiltersIneligibleUsers(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/users/search" {
			http.NotFound(w, r)
			return
		}
		query := r.URL.Query()
		if query.Get("query") != "*" || query.Get("expand") != "true" || query.Get("permissions[]") != "ticket.agent" || query.Get("group_ids[7]") != "full" || query.Get("page") != "1" || query.Get("per_page") != "100" {
			t.Fatalf("unexpected owner query: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`[
			{"id":2,"active":true,"firstname":"Ada","lastname":"Lovelace","group_ids":{"7":["full"]}},
			{"id":6,"active":true,"login":"change-only","group_ids":{"7":["change"]}},
			{"id":3,"active":true,"login":"read-only","group_ids":{"7":["read"]}},
			{"id":4,"active":false,"login":"inactive","group_ids":{"7":["full"]}},
			{"id":5,"active":true,"login":"fallback","group_ids":{"8":["full"]}}
		]`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", httpTestTransport{client: server.Client()})
	owners, err := client.Owners(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(owners) != 1 || owners[0] != (Owner{ID: 2, Name: "Ada Lovelace"}) {
		t.Fatalf("unexpected eligible owners: %#v", owners)
	}
}

func TestClientOwnersDecodesZammad711ExpandedSearchFixture(t *testing.T) {
	t.Parallel()
	fixture, err := os.ReadFile("testdata/users_search_expand_zammad_7_1_1.json")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/users/search" || r.URL.Query().Get("expand") != "true" {
			t.Fatalf("unexpected owner request: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", httpTestTransport{client: server.Client()})
	owners, err := client.Owners(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(owners) != 1 || owners[0] != (Owner{ID: 42, Name: "Ada Lovelace"}) {
		t.Fatalf("unexpected owners from Zammad 7.1.1 fixture: %#v", owners)
	}
}

func TestClientOwnersPagesUntilShortPage(t *testing.T) {
	t.Parallel()
	pageOne := make([]string, 100)
	for i := range pageOne {
		pageOne[i] = fmt.Sprintf(`{"id":%d,"active":true,"login":"user-%d","group_ids":{"7":["full"]}}`, i+1, i+1)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/users/search" {
			http.NotFound(w, r)
			return
		}
		query := r.URL.Query()
		if query.Get("query") != "*" || query.Get("expand") != "true" || query.Get("permissions[]") != "ticket.agent" || query.Get("group_ids[7]") != "full" || query.Get("per_page") != "100" {
			t.Fatalf("unexpected owner query: %s", r.URL.RawQuery)
		}
		switch query.Get("page") {
		case "1":
			_, _ = fmt.Fprintf(w, "[%s]", strings.Join(pageOne, ","))
		case "2":
			_, _ = w.Write([]byte(`[{"id":101,"active":true,"firstname":"Grace","lastname":"Hopper","group_ids":{"7":["full"]}}]`))
		default:
			t.Fatalf("unexpected page request: %s", query.Get("page"))
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", httpTestTransport{client: server.Client()})
	owners, err := client.Owners(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(owners) != 101 || owners[0] != (Owner{ID: 1, Name: "user-1"}) || owners[100] != (Owner{ID: 101, Name: "Grace Hopper"}) {
		t.Fatalf("unexpected paged owners: len=%d first=%#v last=%#v", len(owners), owners[0], owners[len(owners)-1])
	}
}

func TestClientOwnersPagingHonorsContext(t *testing.T) {
	t.Parallel()
	pageOne := make([]string, 100)
	for i := range pageOne {
		pageOne[i] = fmt.Sprintf(`{"id":%d,"active":true,"login":"user-%d","group_ids":{"7":["full"]}}`, i+1, i+1)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	requests := 0
	transport := TransportFunc(func(requestCtx context.Context, _ string, _ string, _ []byte, _ map[string]string) (*Response, error) {
		requests++
		if requests == 1 {
			cancel()
			body := fmt.Sprintf("[%s]", strings.Join(pageOne, ","))
			return &Response{StatusCode: http.StatusOK, Body: []byte(body)}, nil
		}
		if err := requestCtx.Err(); err != nil {
			return nil, err
		}
		return &Response{StatusCode: http.StatusOK, Body: []byte(`[]`)}, nil
	})

	client := NewClient("https://zammad.example.test", "token", transport)
	_, err := client.Owners(ctx, 7)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	if requests != 2 {
		t.Fatalf("expected second page request to receive canceled context, got %d requests", requests)
	}
}
