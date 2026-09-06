package services

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"windshift/internal/database"
	"windshift/internal/integrations/zammad"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sso"
	"windshift/internal/utils"
)

type fakeZammadTransport struct {
	mu                    sync.Mutex
	requests              int
	posts                 int
	puts                  int
	ticket                map[string]any
	postErrorAfterCreate  error
	postStatusAfterCreate int
	postResponse          map[string]any
	putError              error
	searchError           error
	getStatus             int
	getTicket             map[string]any
	hideSearch            bool
	groups                []map[string]any
	states                []map[string]any
	users                 []map[string]any
	putPayloads           []map[string]any
	createGroupID         int
	putRerouteGroupID     int
}

func zammadTestGroupRefs(ids ...int) []models.ZammadGroupRef {
	groups := make([]models.ZammadGroupRef, 0, len(ids))
	for _, id := range ids {
		name := "Support"
		if id != 7 {
			name = "Escalations"
		}
		groups = append(groups, models.ZammadGroupRef{ID: id, Name: name})
	}
	return groups
}

func (f *fakeZammadTransport) Do(_ context.Context, method, targetURL string, body []byte, headers map[string]string) (*zammad.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests++
	if headers["Authorization"] != "Token token=synthetic-zammad-token" {
		return nil, errors.New("unexpected authorization header")
	}
	parsed, err := url.Parse(targetURL)
	if err != nil {
		return nil, err
	}
	switch {
	case parsed.Path == "/api/v1/tickets/search":
		if f.searchError != nil {
			err := f.searchError
			f.searchError = nil
			return nil, err
		}
		if f.ticket == nil || f.hideSearch {
			return jsonResponse(http.StatusOK, []any{}), nil
		}
		return jsonResponse(http.StatusOK, []any{f.ticket}), nil
	case parsed.Path == "/api/v1/tickets" && method == http.MethodPost:
		f.posts++
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, err
		}
		createGroupID := f.createGroupID
		if createGroupID == 0 {
			createGroupID = 7
		}
		f.ticket = map[string]any{
			"id": 901, "number": "420901", "group_id": createGroupID,
			"state_id": 2, "state": "open",
			"windshift_item_key": payload["windshift_item_key"],
		}
		if f.postErrorAfterCreate != nil {
			err := f.postErrorAfterCreate
			f.postErrorAfterCreate = nil
			return nil, err
		}
		if f.postStatusAfterCreate != 0 {
			status := f.postStatusAfterCreate
			f.postStatusAfterCreate = 0
			return jsonResponse(status, map[string]string{"error": "synthetic create failure"}), nil
		}
		if f.postResponse != nil {
			return jsonResponse(http.StatusCreated, f.postResponse), nil
		}
		return jsonResponse(http.StatusCreated, f.ticket), nil
	case strings.HasPrefix(parsed.Path, "/api/v1/tickets/") && method == http.MethodPut:
		f.puts++
		if f.putError != nil {
			return nil, f.putError
		}
		payload := map[string]any{}
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, err
		}
		f.putPayloads = append(f.putPayloads, payload)
		if f.ticket != nil {
			for key, value := range payload {
				f.ticket[key] = value
			}
			if f.putRerouteGroupID > 0 {
				f.ticket["group_id"] = f.putRerouteGroupID
			}
		}
		return jsonResponse(http.StatusOK, f.ticket), nil
	case strings.HasPrefix(parsed.Path, "/api/v1/tickets/"):
		if f.getStatus != 0 {
			return jsonResponse(f.getStatus, map[string]string{"error": "synthetic remote detail"}), nil
		}
		if f.getTicket != nil {
			return jsonResponse(http.StatusOK, f.getTicket), nil
		}
		return jsonResponse(http.StatusOK, f.ticket), nil
	case parsed.Path == "/api/v1/ticket_states":
		if f.states != nil {
			return jsonResponse(http.StatusOK, f.states), nil
		}
		return jsonResponse(http.StatusOK, []map[string]any{{"id": 2, "name": "open", "active": true}, {"id": 4, "name": "closed", "active": true}}), nil
	case parsed.Path == "/api/v1/users/search":
		return jsonResponse(http.StatusOK, f.users), nil
	default:
		return jsonResponse(http.StatusNotFound, map[string]string{"error": "not found"}), nil
	}
}

func (f *fakeZammadTransport) counts() (requests, posts int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests, f.posts
}

func (f *fakeZammadTransport) putCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.puts
}

func (f *fakeZammadTransport) lastPutPayload() map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.putPayloads) == 0 {
		return nil
	}
	return f.putPayloads[len(f.putPayloads)-1]
}

func (f *fakeZammadTransport) resetRequests() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = 0
}

func jsonResponse(status int, value any) *zammad.Response {
	body, _ := json.Marshal(value)
	return &zammad.Response{StatusCode: status, Body: body}
}

type allowZammadPermission struct{}

func (allowZammadPermission) HasWorkspacePermission(_, _ int, _ string) (bool, error) {
	return true, nil
}

type recordingItemChangePublisher struct {
	itemID int
	kind   ItemChangeKind
}

func (p *recordingItemChangePublisher) PublishItemChange(itemID int, kind ItemChangeKind) {
	p.itemID = itemID
	p.kind = kind
}

type fakeZammadWorkflow struct {
	db            database.Database
	writes        int
	postCommitErr error
}

func (f *fakeZammadWorkflow) PerformTransition(_ context.Context, req PerformTransitionRequest, itemRepo *repository.ItemRepository, _ *ConditionService, _ transitionApprovalService) (*PerformTransitionResult, error) {
	item, err := itemRepo.FindByIDWithDetails(req.ItemID)
	if err != nil {
		return nil, err
	}
	if item.StatusID != nil && *item.StatusID == req.ToStatusID {
		return &PerformTransitionResult{Item: item, OldStatusID: item.StatusID, NewStatusID: item.StatusID, NoOp: true}, nil
	}
	oldStatusID := item.StatusID
	if _, err := f.db.ExecWrite("UPDATE items SET status_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", req.ToStatusID, req.ItemID); err != nil {
		return nil, err
	}
	f.writes++
	item, err = itemRepo.FindByIDWithDetails(req.ItemID)
	if err != nil {
		return nil, err
	}
	newStatusID := req.ToStatusID
	result := &PerformTransitionResult{Item: item, OldStatusID: oldStatusID, NewStatusID: &newStatusID}
	if f.postCommitErr != nil {
		return result, f.postCommitErr
	}
	return result, nil
}

type zammadServiceFixture struct {
	t           *testing.T
	db          database.Database
	service     *ZammadService
	credentials *ActionCredentialService
	transport   *fakeZammadTransport
	connection  *models.ZammadConnection
	workspace1  int
	workspace2  int
	item1       int
	item2       int
	actorID     int
	openStatus  int
	doneStatus  int
}

func newZammadServiceFixture(t *testing.T, workflow zammadWorkflowTransitioner) *zammadServiceFixture {
	t.Helper()
	db, err := database.NewSQLiteDB(t.TempDir() + "/windshift.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Initialize(); err != nil {
		t.Fatal(err)
	}
	f := &zammadServiceFixture{t: t, db: db, transport: &fakeZammadTransport{}}
	f.actorID = mustInsertID(t, db, `INSERT INTO users (email, username, first_name, last_name) VALUES (?, ?, ?, ?)`, "agent@example.test", "zammad-agent", "Zammad", "Agent")
	f.workspace1 = mustInsertID(t, db, `INSERT INTO workspaces (name, key) VALUES (?, ?)`, "Primary", "PRI")
	f.workspace2 = mustInsertID(t, db, `INSERT INTO workspaces (name, key) VALUES (?, ?)`, "Other", "OTH")
	if err := db.QueryRow("SELECT id FROM statuses ORDER BY id LIMIT 1").Scan(&f.openStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT id FROM statuses ORDER BY id DESC LIMIT 1").Scan(&f.doneStatus); err != nil {
		t.Fatal(err)
	}
	f.item1 = mustInsertID(t, db, `INSERT INTO items (workspace_id, workspace_item_number, title, description, frac_index, status_id, creator_id, last_active_at) VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`, f.workspace1, 49, "Synthetic ticket source", "Synthetic description", "a0", f.openStatus, f.actorID)
	f.item2 = mustInsertID(t, db, `INSERT INTO items (workspace_id, workspace_item_number, title, description, frac_index, status_id, creator_id, last_active_at) VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`, f.workspace2, 1, "Out of scope", "Synthetic description", "a1", f.openStatus, f.actorID)
	credentialService := NewActionCredentialService(repository.NewActionCredentialRepository(db), "synthetic-server-secret-for-zammad-tests")
	f.credentials = credentialService
	if workflow == nil {
		workflow = &fakeZammadWorkflow{db: db}
	}
	f.service = NewZammadService(db, repository.NewZammadRepository(db), credentialService, allowZammadPermission{}, workflow, nil, nil)
	f.service.SetTransportForTesting(f.transport)
	f.connection, err = f.service.CreateConnection(models.CreateZammadConnectionRequest{
		Slug: "helpdesk", Name: "Synthetic helpdesk", BaseURL: "https://zammad.example.test",
		APIToken: "synthetic-zammad-token", DefaultGroupID: 7, DefaultGroupName: "Support",
		DefaultCustomer: "robot@example.test", CorrelationField: "windshift_item_key",
		WorkspaceIDs: []int{f.workspace1},
	}, f.actorID)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func mustInsertID(t *testing.T, db database.Database, query string, args ...any) int {
	t.Helper()
	result, err := db.ExecWrite(query, args...)
	if err != nil {
		t.Fatal(err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return int(id)
}

func TestZammadCreateTicketIsIdempotentAndPersistsGenericLink(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	first, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	_, posts := f.transport.counts()
	if posts != 1 || first.TicketID != 901 || second.ID != first.ID {
		t.Fatalf("unexpected idempotency result: first=%#v second=%#v posts=%d", first, second, posts)
	}
	var genericID string
	if err := f.db.QueryRow("SELECT item_integration_link_id FROM zammad_ticket_links WHERE id = ?", first.ID).Scan(&genericID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := f.db.QueryRow("SELECT COUNT(*) FROM item_integration_links WHERE id = ? AND external_id = ?", genericID, strconv.Itoa(first.TicketID)).Scan(&count); err != nil || count != 1 {
		t.Fatalf("generic link was not persisted: count=%d err=%v", count, err)
	}
}

func TestZammadLinkExistingTicketPinsExactTicketAndIsIdempotent(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	f.transport.ticket = map[string]any{
		"id": 711, "number": "420711", "group_id": 7, "group": "Support",
		"state_id": 2, "state": "open", "owner_id": 99,
	}
	f.transport.users = []map[string]any{{"id": 99, "active": true, "firstname": "Grace", "lastname": "Hopper", "group_ids": map[string][]string{"7": {"full"}}}}

	first, err := f.service.LinkExistingTicket(context.Background(), f.item1, f.actorID, models.LinkZammadTicketRequest{
		ConnectionID: f.connection.ProviderID, TicketNumber: "420711",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.service.LinkExistingTicket(context.Background(), f.item1, f.actorID, models.LinkZammadTicketRequest{
		ConnectionID: f.connection.ProviderID, TicketNumber: "420711",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || first.TicketID != 711 || first.SyncState != models.ZammadSyncLinked || f.transport.putCount() != 1 {
		t.Fatalf("existing-ticket link was not idempotent: first=%#v second=%#v puts=%d", first, second, f.transport.putCount())
	}
	if first.OwnerID != 99 || first.OwnerName != "Grace Hopper" {
		t.Fatalf("existing-ticket owner was not resolved: %#v", first)
	}
	if got := f.transport.lastPutPayload()["windshift_item_key"]; got != first.CorrelationKey {
		t.Fatalf("remote correlation was not pinned to the local link: %#v", f.transport.lastPutPayload())
	}
}

func TestZammadLinkExistingTicketSetupFailureCannotBePromotedBySync(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	f.transport.ticket = map[string]any{
		"id": 715, "number": "420715", "group_id": 7, "group": "Support",
		"state_id": 2, "state": "open", "owner_id": 1,
	}
	f.transport.putError = context.DeadlineExceeded

	_, err := f.service.LinkExistingTicket(context.Background(), f.item1, f.actorID, models.LinkZammadTicketRequest{
		ConnectionID: f.connection.ProviderID, TicketNumber: "420715",
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected correlation update failure, got %v", err)
	}
	reserved, err := f.service.TicketLinksForItem(f.item1)
	if err != nil || len(reserved) != 1 {
		t.Fatalf("failed setup did not retain exactly one reservation: links=%#v err=%v", reserved, err)
	}
	link := reserved[0]
	if link.SyncState != models.ZammadSyncCreating || link.ItemIntegrationLinkID != "" || link.LastError == "" {
		t.Fatalf("failed setup looked complete or lost its safe error: %#v", link)
	}
	var genericLinks int
	if err := f.db.QueryRow("SELECT COUNT(*) FROM item_integration_links WHERE integration_provider_id = ?", f.connection.ProviderID).Scan(&genericLinks); err != nil || genericLinks != 0 {
		t.Fatalf("failed setup created a generic integration link: count=%d err=%v", genericLinks, err)
	}

	f.transport.resetRequests()
	if err := f.service.SyncDue(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	requests, _ := f.transport.counts()
	if requests != 0 {
		t.Fatalf("scheduler attempted to sync incomplete reservation: requests=%d", requests)
	}
	if _, err := f.service.SyncTicketLink(context.Background(), link.ID); err == nil {
		t.Fatal("manual refresh promoted an incomplete reservation")
	}
	closedStateID := 4
	if _, err := f.service.UpdateTicketLink(context.Background(), link.ID, models.UpdateZammadTicketLinkRequest{StateID: &closedStateID}); err == nil {
		t.Fatal("ticket edit was allowed before link setup completed")
	}
	stored, err := f.service.GetTicketLink(link.ID)
	if err != nil || stored.SyncState != models.ZammadSyncCreating || stored.ItemIntegrationLinkID != "" {
		t.Fatalf("sync attempts changed incomplete reservation: link=%#v err=%v", stored, err)
	}

	f.transport.putError = nil
	completed, err := f.service.LinkExistingTicket(context.Background(), f.item1, f.actorID, models.LinkZammadTicketRequest{
		ConnectionID: f.connection.ProviderID, TicketNumber: "420715",
	})
	if err != nil {
		t.Fatal(err)
	}
	if completed.ID != link.ID || completed.SyncState != models.ZammadSyncLinked || completed.ItemIntegrationLinkID == "" || completed.LastError != "" {
		t.Fatalf("retry did not finish the retained reservation: %#v", completed)
	}
}

func TestZammadLinkExistingTicketRejectsDisallowedGroupAndConflictingCorrelation(t *testing.T) {
	t.Run("disallowed group", func(t *testing.T) {
		f := newZammadServiceFixture(t, nil)
		f.transport.ticket = map[string]any{"id": 712, "number": "420712", "group_id": 99, "state_id": 2, "state": "open"}
		_, err := f.service.LinkExistingTicket(context.Background(), f.item1, f.actorID, models.LinkZammadTicketRequest{ConnectionID: f.connection.ProviderID, TicketNumber: "420712"})
		var validationErr *ZammadValidationError
		if !errors.As(err, &validationErr) || f.transport.putCount() != 0 {
			t.Fatalf("disallowed ticket group was accepted: err=%v puts=%d", err, f.transport.putCount())
		}
	})
	t.Run("conflicting correlation", func(t *testing.T) {
		f := newZammadServiceFixture(t, nil)
		f.transport.ticket = map[string]any{
			"id": 713, "number": "420713", "group_id": 7, "state_id": 2, "state": "open",
			"windshift_item_key": "windshift:other-connection:PRI-49",
		}
		_, err := f.service.LinkExistingTicket(context.Background(), f.item1, f.actorID, models.LinkZammadTicketRequest{ConnectionID: f.connection.ProviderID, TicketNumber: "420713"})
		var validationErr *ZammadValidationError
		if !errors.As(err, &validationErr) || f.transport.putCount() != 0 {
			t.Fatalf("conflicting remote correlation was accepted: err=%v puts=%d", err, f.transport.putCount())
		}
		var links int
		if err := f.db.QueryRow("SELECT COUNT(*) FROM zammad_ticket_links").Scan(&links); err != nil || links != 0 {
			t.Fatalf("conflicting ticket left a local link behind: links=%d err=%v", links, err)
		}
	})
}

func TestZammadLinkExistingTicketDoesNotStealAlreadyReservedRemoteTicket(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	f.transport.ticket = map[string]any{"id": 714, "number": "420714", "group_id": 7, "state_id": 2, "state": "open"}
	first, err := f.service.LinkExistingTicket(context.Background(), f.item1, f.actorID, models.LinkZammadTicketRequest{ConnectionID: f.connection.ProviderID, TicketNumber: "420714"})
	if err != nil {
		t.Fatal(err)
	}
	secondItem := mustInsertID(t, f.db, `INSERT INTO items (workspace_id, workspace_item_number, title, description, frac_index, status_id, creator_id, last_active_at) VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`, f.workspace1, 50, "Second ticket source", "Synthetic description", "a2", f.openStatus, f.actorID)
	// Simulate a remote-side correlation edit between attempts. The local unique
	// provider/ticket reservation must still prevent the second item from taking
	// over this ticket.
	f.transport.ticket["windshift_item_key"] = ""
	puts := f.transport.putCount()
	_, err = f.service.LinkExistingTicket(context.Background(), secondItem, f.actorID, models.LinkZammadTicketRequest{ConnectionID: f.connection.ProviderID, TicketNumber: "420714"})
	if !errors.Is(err, repository.ErrDuplicateEntry) || f.transport.putCount() != puts {
		t.Fatalf("second item stole an already reserved ticket: err=%v puts=%d", err, f.transport.putCount())
	}
	stored, err := f.service.GetTicketLink(first.ID)
	if err != nil || stored.ItemID != f.item1 {
		t.Fatalf("original local reservation changed: link=%#v err=%v", stored, err)
	}
}

func TestZammadUpdateTicketLinkValidatesOwnerForEffectiveGroupAndPersistsSnapshot(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	f.transport.ticket = map[string]any{"id": 901, "number": "420901", "group_id": 7, "state_id": 2, "owner_id": 1}
	f.transport.groups = []map[string]any{{"id": 7, "name": "Support", "active": true}, {"id": 8, "name": "Escalations", "active": true}}
	f.transport.states = []map[string]any{{"id": 2, "name": "open", "active": true}, {"id": 4, "name": "closed", "active": true}}
	f.transport.users = []map[string]any{{"id": 99, "active": true, "firstname": "Grace", "lastname": "Hopper", "group_ids": map[string][]string{"8": {"full"}}}}
	allowedGroups := zammadTestGroupRefs(7, 8)
	if _, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{AllowedGroups: &allowedGroups}); err != nil {
		t.Fatal(err)
	}
	stateID, groupID, ownerID := 4, 8, 99
	updated, err := f.service.UpdateTicketLink(context.Background(), link.ID, models.UpdateZammadTicketLinkRequest{StateID: &stateID, GroupID: &groupID, OwnerID: &ownerID})
	if err != nil {
		t.Fatal(err)
	}
	if updated.LastStatusID != 4 || updated.LastStatusName != "closed" || updated.GroupID != 8 || updated.GroupName != "Escalations" || updated.OwnerID != 99 || updated.OwnerName != "Grace Hopper" {
		t.Fatalf("remote update snapshot was not persisted: %#v", updated)
	}
	puts := f.transport.putCount()
	invalidOwner := 100
	_, err = f.service.UpdateTicketLink(context.Background(), link.ID, models.UpdateZammadTicketLinkRequest{OwnerID: &invalidOwner})
	var validationErr *ZammadValidationError
	if !errors.As(err, &validationErr) || f.transport.putCount() != puts {
		t.Fatalf("owner without group access reached remote update: err=%v puts=%d", err, f.transport.putCount())
	}
}

func TestZammadTicketSnapshotBlocksConcurrentGroupPolicyNarrowing(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	f.transport.groups = []map[string]any{{"id": 7, "name": "Support", "active": true}, {"id": 8, "name": "Escalations", "active": true}}
	initialAllowedGroups := zammadTestGroupRefs(7, 8)
	if _, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{AllowedGroups: &initialAllowedGroups}); err != nil {
		t.Fatal(err)
	}

	arrived := make(chan struct{}, 1)
	release := make(chan struct{})
	f.service.SetPersistBeforeConnectionLockForTesting(func() {
		arrived <- struct{}{}
		<-release
	})
	defer f.service.SetPersistBeforeConnectionLockForTesting(nil)

	newGroupID := 8
	updateErr := make(chan error, 1)
	go func() {
		_, err := f.service.UpdateTicketLink(context.Background(), link.ID, models.UpdateZammadTicketLinkRequest{GroupID: &newGroupID})
		updateErr <- err
	}()
	<-arrived
	narrowedAllowedGroups := zammadTestGroupRefs(7)
	if _, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{AllowedGroups: &narrowedAllowedGroups}); !errors.Is(err, ErrZammadConnectionBusy) {
		t.Fatalf("group policy changed while remote update was pending: %v", err)
	}
	close(release)
	if err := <-updateErr; err != nil {
		t.Fatalf("remote group move failed after conflicting policy update was rejected: %v", err)
	}

	stored, err := f.service.GetTicketLink(link.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.GroupID != 8 || stored.SyncState != models.ZammadSyncLinked || stored.LastError != "" {
		t.Fatalf("rejected policy race prevented the valid snapshot: %#v", stored)
	}
	if got := int(f.transport.ticket["group_id"].(float64)); got != 8 {
		t.Fatalf("test did not move the remote ticket before narrowing: group=%d", got)
	}
}

func TestZammadExpiredUpdateLeaseReloadsGroupPolicyBeforeRemoteWrite(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	f.transport.groups = []map[string]any{{"id": 7, "name": "Support", "active": true}, {"id": 8, "name": "Escalations", "active": true}}
	initialAllowed := zammadTestGroupRefs(7, 8)
	if _, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{AllowedGroups: &initialAllowed}); err != nil {
		t.Fatal(err)
	}
	arrived := make(chan struct{}, 1)
	release := make(chan struct{})
	f.service.SetUpdateBeforeRemoteWriteForTesting(func() {
		arrived <- struct{}{}
		<-release
	})
	defer f.service.SetUpdateBeforeRemoteWriteForTesting(nil)

	groupID := 8
	updateErr := make(chan error, 1)
	go func() {
		_, err := f.service.UpdateTicketLink(context.Background(), link.ID, models.UpdateZammadTicketLinkRequest{GroupID: &groupID})
		updateErr <- err
	}()
	<-arrived
	if _, err := f.db.ExecWrite("UPDATE zammad_ticket_links SET sync_lock_until = ? WHERE id = ?", time.Now().Add(-time.Minute), link.ID); err != nil {
		t.Fatal(err)
	}
	narrowed := zammadTestGroupRefs(7)
	if _, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{AllowedGroups: &narrowed}); err != nil {
		t.Fatalf("group policy could not change after synthetic lease expiry: %v", err)
	}
	puts := f.transport.putCount()
	close(release)
	if err := <-updateErr; !errors.Is(err, ErrZammadTicketGroupPolicyChanged) {
		t.Fatalf("expired update worker did not honor current group policy: %v", err)
	}
	if f.transport.putCount() != puts {
		t.Fatalf("expired update worker wrote a group removed by current policy")
	}
}

func TestZammadTicketCreationBlocksGroupNarrowingAcrossRemoteReroute(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	f.transport.groups = []map[string]any{{"id": 7, "name": "Support", "active": true}, {"id": 8, "name": "Escalations", "active": true}}
	f.transport.createGroupID = 8
	initialAllowedGroups := zammadTestGroupRefs(7, 8)
	if _, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{AllowedGroups: &initialAllowedGroups}); err != nil {
		t.Fatal(err)
	}
	arrived := make(chan struct{}, 1)
	release := make(chan struct{})
	f.service.SetPersistBeforeConnectionLockForTesting(func() {
		arrived <- struct{}{}
		<-release
	})
	defer f.service.SetPersistBeforeConnectionLockForTesting(nil)

	type creationResult struct {
		link *models.ZammadTicketLink
		err  error
	}
	result := make(chan creationResult, 1)
	go func() {
		link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID, GroupID: 7})
		result <- creationResult{link: link, err: err}
	}()
	<-arrived
	narrowedAllowedGroups := zammadTestGroupRefs(7)
	if _, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{AllowedGroups: &narrowedAllowedGroups}); !errors.Is(err, ErrZammadConnectionBusy) {
		t.Fatalf("group policy changed while rerouted ticket creation was incomplete: %v", err)
	}
	close(release)
	created := <-result
	if created.err != nil || created.link == nil || created.link.GroupID != 8 {
		t.Fatalf("ticket creation did not finish under the preserved policy: link=%#v err=%v", created.link, created.err)
	}
}

func TestZammadIncompleteReservationAllowsPolicyExpansionAndCorrelationRecovery(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	f.transport.groups = []map[string]any{{"id": 7, "name": "Support", "active": true}, {"id": 8, "name": "Escalations", "active": true}}
	f.transport.createGroupID = 8
	initialAllowedGroups := zammadTestGroupRefs(7)
	if _, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{AllowedGroups: &initialAllowedGroups}); err != nil {
		t.Fatal(err)
	}

	_, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	var validationErr *ZammadValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("rerouted ticket unexpectedly completed under the old group policy: %v", err)
	}
	links, err := f.service.TicketLinksForItem(f.item1)
	if err != nil || len(links) != 1 || links[0].SyncState != models.ZammadSyncUncertain {
		t.Fatalf("rerouted ticket was not retained for correlation recovery: links=%#v err=%v", links, err)
	}

	expandedAllowedGroups := zammadTestGroupRefs(7, 8)
	if _, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{AllowedGroups: &expandedAllowedGroups}); err != nil {
		t.Fatalf("safe group policy expansion was blocked by incomplete reservation: %v", err)
	}
	narrowedAllowedGroups := zammadTestGroupRefs(7)
	if _, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{AllowedGroups: &narrowedAllowedGroups}); !errors.Is(err, ErrZammadConnectionBusy) {
		t.Fatalf("group policy narrowing was allowed while reservation was incomplete: %v", err)
	}

	recovered, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatalf("correlation retry did not recover after policy expansion: %v", err)
	}
	if recovered.TicketID != 901 || recovered.GroupID != 8 || recovered.SyncState != models.ZammadSyncLinked {
		t.Fatalf("correlation retry did not complete the rerouted ticket: %#v", recovered)
	}
	_, posts := f.transport.counts()
	if posts != 1 {
		t.Fatalf("correlation retry created a duplicate ticket: posts=%d", posts)
	}
}

func TestZammadGroupPolicyNarrows(t *testing.T) {
	currentNumeric := &repository.ZammadConnectionMutationSnapshot{DefaultGroupID: 7, AllowedGroups: zammadTestGroupRefs(7, 8)}
	currentNameOnly := &repository.ZammadConnectionMutationSnapshot{DefaultGroupName: "Support"}
	tests := []struct {
		name     string
		current  *repository.ZammadConnectionMutationSnapshot
		proposed *models.ZammadConnection
		narrows  bool
	}{
		{
			name: "numeric expansion", current: currentNumeric,
			proposed: &models.ZammadConnection{DefaultGroupID: 7, AllowedGroups: zammadTestGroupRefs(7, 8, 9)},
		},
		{
			name: "numeric expansion preserves name-only default through allowlist",
			current: &repository.ZammadConnectionMutationSnapshot{
				DefaultGroupName: "Support", AllowedGroups: zammadTestGroupRefs(7, 8),
			},
			proposed: &models.ZammadConnection{DefaultGroupName: "Support", AllowedGroups: zammadTestGroupRefs(7, 8, 9)},
		},
		{
			name: "numeric narrowing", current: currentNumeric,
			proposed: &models.ZammadConnection{DefaultGroupID: 7, AllowedGroups: zammadTestGroupRefs(7)}, narrows: true,
		},
		{
			name: "old default remains allowed", current: &repository.ZammadConnectionMutationSnapshot{DefaultGroupID: 7},
			proposed: &models.ZammadConnection{DefaultGroupID: 8, AllowedGroups: zammadTestGroupRefs(7, 8)},
		},
		{
			name: "old default excluded", current: &repository.ZammadConnectionMutationSnapshot{DefaultGroupID: 7},
			proposed: &models.ZammadConnection{DefaultGroupID: 8, AllowedGroups: zammadTestGroupRefs(8)}, narrows: true,
		},
		{
			name: "same name-only policy", current: currentNameOnly,
			proposed: &models.ZammadConnection{DefaultGroupName: "Support"},
		},
		{
			name: "name-only changed to IDs", current: currentNameOnly,
			proposed: &models.ZammadConnection{DefaultGroupID: 7, AllowedGroups: zammadTestGroupRefs(7)}, narrows: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := zammadGroupPolicyNarrows(tt.current, tt.proposed); got != tt.narrows {
				t.Fatalf("zammadGroupPolicyNarrows() = %v, want %v", got, tt.narrows)
			}
		})
	}
}

func TestZammadExistingTicketLinkBlocksGroupNarrowingAcrossRemoteReroute(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	f.transport.groups = []map[string]any{{"id": 7, "name": "Support", "active": true}, {"id": 8, "name": "Escalations", "active": true}}
	f.transport.ticket = map[string]any{"id": 714, "number": "420714", "group_id": 7, "state_id": 2, "state": "open"}
	f.transport.putRerouteGroupID = 8
	initialAllowedGroups := zammadTestGroupRefs(7, 8)
	if _, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{AllowedGroups: &initialAllowedGroups}); err != nil {
		t.Fatal(err)
	}
	arrived := make(chan struct{}, 1)
	release := make(chan struct{})
	f.service.SetPersistBeforeConnectionLockForTesting(func() {
		arrived <- struct{}{}
		<-release
	})
	defer f.service.SetPersistBeforeConnectionLockForTesting(nil)

	type linkResult struct {
		link *models.ZammadTicketLink
		err  error
	}
	result := make(chan linkResult, 1)
	go func() {
		link, err := f.service.LinkExistingTicket(context.Background(), f.item1, f.actorID, models.LinkZammadTicketRequest{ConnectionID: f.connection.ProviderID, TicketNumber: "420714"})
		result <- linkResult{link: link, err: err}
	}()
	<-arrived
	narrowedAllowedGroups := zammadTestGroupRefs(7)
	if _, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{AllowedGroups: &narrowedAllowedGroups}); !errors.Is(err, ErrZammadConnectionBusy) {
		t.Fatalf("group policy changed while rerouted existing-ticket link was incomplete: %v", err)
	}
	close(release)
	linked := <-result
	if linked.err != nil || linked.link == nil || linked.link.GroupID != 8 {
		t.Fatalf("existing ticket link did not finish under the preserved policy: link=%#v err=%v", linked.link, linked.err)
	}
}

func TestZammadUnlinkClearsOnlyExactRemoteCorrelationWithoutDeletingTicket(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.UnlinkTicket(context.Background(), link.ID); err != nil {
		t.Fatal(err)
	}
	if f.transport.ticket == nil || f.transport.ticket["id"] != 901 || f.transport.ticket["windshift_item_key"] != "" {
		t.Fatalf("unlink changed or deleted the remote ticket: %#v", f.transport.ticket)
	}
	if _, err := f.service.GetTicketLink(link.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("unlink retained local association: %v", err)
	}
}

func TestZammadLocalDetachRemovesLinksWithoutContactingUnavailableUpstream(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	f.transport.resetRequests()
	f.transport.getStatus = http.StatusServiceUnavailable
	f.transport.putError = errors.New("synthetic upstream outage")

	detached, err := f.service.DetachTicketLinkLocally(link.ID)
	if err != nil {
		t.Fatalf("local detach failed while upstream was unavailable: %v", err)
	}
	if detached.ID != link.ID {
		t.Fatalf("local detach returned the wrong link: got %q want %q", detached.ID, link.ID)
	}
	requests, _ := f.transport.counts()
	if requests != 0 {
		t.Fatalf("local detach contacted Zammad %d times", requests)
	}
	if f.transport.ticket["windshift_item_key"] != link.CorrelationKey {
		t.Fatalf("local detach unexpectedly changed remote correlation: %#v", f.transport.ticket)
	}
	if _, err := f.service.GetTicketLink(link.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("local detach retained typed link: %v", err)
	}
	var genericLinkCount int
	if err := f.db.QueryRow("SELECT COUNT(*) FROM item_integration_links WHERE id = ?", link.ItemIntegrationLinkID).Scan(&genericLinkCount); err != nil || genericLinkCount != 0 {
		t.Fatalf("local detach retained generic link: count=%d err=%v", genericLinkCount, err)
	}
	if _, err := NewItemCRUDService(f.db).Delete(f.item1); err != nil {
		t.Fatalf("item remained protected after local detach: %v", err)
	}
}

func TestZammadDisabledConnectionCanBeUnlinkedAndDeleted(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	disabled := false
	if _, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{Enabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.UnlinkTicket(context.Background(), link.ID); err != nil {
		t.Fatalf("unlink through disabled connection failed: %v", err)
	}
	if f.transport.ticket["windshift_item_key"] != "" {
		t.Fatalf("disabled-connection unlink left remote correlation: %#v", f.transport.ticket)
	}
	if _, err := f.service.GetTicketLink(link.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("disabled-connection unlink retained local link: %v", err)
	}
	if err := f.service.DeleteConnection(f.connection.ProviderID); err != nil {
		t.Fatalf("connection remained undeletable after unlink: %v", err)
	}
}

func TestZammadUnlinkStillRequiresWorkspaceScope(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecWrite("DELETE FROM action_credential_workspaces WHERE credential_id = ?", f.connection.CredentialID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.UnlinkTicket(context.Background(), link.ID); !errors.Is(err, ErrCredentialScopeMismatch) {
		t.Fatalf("out-of-scope unlink returned %v", err)
	}
	if _, err := f.service.GetTicketLink(link.ID); err != nil {
		t.Fatalf("out-of-scope unlink removed local link: %v", err)
	}
	if f.transport.ticket["windshift_item_key"] != link.CorrelationKey {
		t.Fatalf("out-of-scope unlink changed remote correlation: %#v", f.transport.ticket)
	}
}

func TestZammadUnlinkStillRequiresEnabledCredential(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecWrite("UPDATE action_credentials SET is_enabled = false WHERE id = ?", f.connection.CredentialID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.UnlinkTicket(context.Background(), link.ID); !errors.Is(err, ErrCredentialDisabled) {
		t.Fatalf("disabled-credential unlink returned %v", err)
	}
	if _, err := f.service.GetTicketLink(link.ID); err != nil {
		t.Fatalf("disabled-credential unlink removed local link: %v", err)
	}
	if f.transport.ticket["windshift_item_key"] != link.CorrelationKey {
		t.Fatalf("disabled-credential unlink changed remote correlation: %#v", f.transport.ticket)
	}
}

func TestZammadLinkedItemDeletionRequiresExplicitUnlink(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := f.db.ExecWrite("DELETE FROM items WHERE id = ?", f.item1); err == nil {
		t.Fatal("database allowed direct deletion of a Zammad-linked item")
	}
	crud := NewItemCRUDService(f.db)
	if _, err := crud.Delete(f.item1); !errors.Is(err, ErrItemHasProtectedIntegrationLinks) {
		t.Fatalf("linked item deletion returned %v", err)
	}
	if _, err := repository.NewItemRepository(f.db).FindByID(f.item1); err != nil {
		t.Fatalf("rejected deletion removed item: %v", err)
	}
	if _, err := f.service.GetTicketLink(link.ID); err != nil {
		t.Fatalf("rejected deletion removed Zammad link: %v", err)
	}
	var genericLinkCount int
	if err := f.db.QueryRow("SELECT COUNT(*) FROM item_integration_links WHERE id = ?", link.ItemIntegrationLinkID).Scan(&genericLinkCount); err != nil || genericLinkCount != 1 {
		t.Fatalf("rejected deletion removed generic item link: count=%d err=%v", genericLinkCount, err)
	}

	if _, err := f.service.UnlinkTicket(context.Background(), link.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := crud.Delete(f.item1); err != nil {
		t.Fatalf("item remained undeletable after explicit unlink: %v", err)
	}
	if _, err := repository.NewItemRepository(f.db).FindByID(f.item1); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("item remained after unlink and delete: %v", err)
	}
}

func TestZammadCascadeDeletionIsBlockedByLinkedDescendant(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	childID := mustInsertID(t, f.db, `INSERT INTO items
		(workspace_id, workspace_item_number, parent_id, title, description, frac_index, status_id, creator_id, last_active_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		f.workspace1, 50, f.item1, "Linked child", "Synthetic child", "a0V", f.openStatus, f.actorID)
	if _, err := f.service.CreateTicket(context.Background(), childID, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID}); err != nil {
		t.Fatal(err)
	}

	if _, err := NewItemCRUDService(f.db).Delete(f.item1); !errors.Is(err, ErrItemHasProtectedIntegrationLinks) {
		t.Fatalf("cascade with linked descendant returned %v", err)
	}
	itemRepo := repository.NewItemRepository(f.db)
	if _, err := itemRepo.FindByID(f.item1); err != nil {
		t.Fatalf("rejected cascade removed root item: %v", err)
	}
	if _, err := itemRepo.FindByID(childID); err != nil {
		t.Fatalf("rejected cascade removed linked descendant: %v", err)
	}
}

func TestZammadLinkedItemsBlockWorkspaceDeletion(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}

	workspaceService := NewWorkspaceService(f.db)
	if err := workspaceService.Delete(f.workspace1); !errors.Is(err, ErrWorkspaceHasProtectedIntegrationLinks) {
		t.Fatalf("workspace deletion with linked item returned %v", err)
	}
	if exists, err := repository.NewWorkspaceRepository(f.db).Exists(f.workspace1); err != nil || !exists {
		t.Fatalf("rejected workspace deletion removed workspace: exists=%t err=%v", exists, err)
	}
	if _, err := repository.NewItemRepository(f.db).FindByID(f.item1); err != nil {
		t.Fatalf("rejected workspace deletion removed item: %v", err)
	}
	if _, err := f.service.GetTicketLink(link.ID); err != nil {
		t.Fatalf("rejected workspace deletion removed Zammad link: %v", err)
	}

	if _, err := f.service.UnlinkTicket(context.Background(), link.ID); err != nil {
		t.Fatal(err)
	}
	if err := workspaceService.Delete(f.workspace1); err != nil {
		t.Fatalf("workspace remained undeletable after explicit unlink: %v", err)
	}
	if exists, err := repository.NewWorkspaceRepository(f.db).Exists(f.workspace1); err != nil || exists {
		t.Fatalf("workspace remained after unlink and delete: exists=%t err=%v", exists, err)
	}
}

func TestZammadLinkedPersonalWorkspaceBlocksUserOffboardingBeforeMutation(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	if _, err := f.db.ExecWrite("UPDATE workspaces SET is_personal = true, owner_id = ? WHERE id = ?", f.actorID, f.workspace1); err != nil {
		t.Fatal(err)
	}
	link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	var beforeEmail, beforeUsername string
	var beforeActive bool
	if err := f.db.QueryRow("SELECT email, username, is_active FROM users WHERE id = ?", f.actorID).Scan(&beforeEmail, &beforeUsername, &beforeActive); err != nil {
		t.Fatal(err)
	}

	if _, err := OffboardUser(f.db, f.actorID, nil); !errors.Is(err, ErrUserOffboardingHasProtectedIntegrationLinks) {
		t.Fatalf("offboarding with linked personal item returned %v", err)
	}
	var afterEmail, afterUsername string
	var afterActive bool
	if err := f.db.QueryRow("SELECT email, username, is_active FROM users WHERE id = ?", f.actorID).Scan(&afterEmail, &afterUsername, &afterActive); err != nil {
		t.Fatal(err)
	}
	if beforeEmail != afterEmail || beforeUsername != afterUsername || beforeActive != afterActive {
		t.Fatalf("blocked offboarding mutated user: before=(%q,%q,%t) after=(%q,%q,%t)", beforeEmail, beforeUsername, beforeActive, afterEmail, afterUsername, afterActive)
	}
	if exists, err := repository.NewWorkspaceRepository(f.db).Exists(f.workspace1); err != nil || !exists {
		t.Fatalf("blocked offboarding removed personal workspace: exists=%t err=%v", exists, err)
	}
	if _, err := repository.NewItemRepository(f.db).FindByID(f.item1); err != nil {
		t.Fatalf("blocked offboarding removed item: %v", err)
	}
	if _, err := f.service.GetTicketLink(link.ID); err != nil {
		t.Fatalf("blocked offboarding removed Zammad link: %v", err)
	}
}

func TestZammadUnlinkUpstreamFailurePreservesLocalLink(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	f.transport.putError = context.DeadlineExceeded
	if _, err := f.service.UnlinkTicket(context.Background(), link.ID); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected remote unlink failure, got %v", err)
	}
	if stored, err := f.service.GetTicketLink(link.ID); err != nil || stored.CorrelationKey != link.CorrelationKey {
		t.Fatalf("unlink failure removed or corrupted the local link: link=%#v err=%v", stored, err)
	}
}

func TestZammadUnlinkDoesNotClearAnotherCorrelation(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	f.transport.ticket["windshift_item_key"] = "windshift:another-link:OTHER-8"
	puts := f.transport.putCount()
	if _, err := f.service.UnlinkTicket(context.Background(), link.ID); err != nil {
		t.Fatal(err)
	}
	if f.transport.putCount() != puts || f.transport.ticket["windshift_item_key"] != "windshift:another-link:OTHER-8" {
		t.Fatalf("unlink cleared a foreign correlation: %#v", f.transport.ticket)
	}
	if _, err := f.service.GetTicketLink(link.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("stale local link was not removed: %v", err)
	}
}

func TestZammadUnlinkUncertainCreationResolvesExactCorrelationBeforeDelete(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	f.transport.postErrorAfterCreate = context.DeadlineExceeded
	_, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected ambiguous create timeout, got %v", err)
	}
	links, err := f.service.TicketLinksForItem(f.item1)
	if err != nil || len(links) != 1 || links[0].TicketID != 0 || links[0].SyncState != models.ZammadSyncUncertain {
		t.Fatalf("ambiguous create reservation missing: links=%#v err=%v", links, err)
	}
	if _, err := f.service.UnlinkTicket(context.Background(), links[0].ID); err != nil {
		t.Fatalf("uncertain unlink did not resolve the exact remote correlation: %v", err)
	}
	if f.transport.ticket["windshift_item_key"] != "" {
		t.Fatalf("uncertain unlink left the resolved remote correlation: ticket=%#v", f.transport.ticket)
	}
	if _, err := f.service.GetTicketLink(links[0].ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("resolved uncertain reservation remained local: %v", err)
	}
}

func TestZammadUnlinkUncertainCreationPreservesReservationWhenSearchIsEmpty(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	f.transport.postErrorAfterCreate = context.DeadlineExceeded
	_, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected ambiguous create timeout, got %v", err)
	}
	links, err := f.service.TicketLinksForItem(f.item1)
	if err != nil || len(links) != 1 {
		t.Fatalf("ambiguous create reservation missing: links=%#v err=%v", links, err)
	}
	correlation := links[0].CorrelationKey
	f.transport.hideSearch = true
	if _, err := f.service.UnlinkTicket(context.Background(), links[0].ID); err == nil {
		t.Fatal("uncertain unlink discarded an unresolved correlation")
	}
	stored, err := f.service.GetTicketLink(links[0].ID)
	if err != nil || stored.SyncState != models.ZammadSyncUncertain {
		t.Fatalf("empty correlation search removed uncertain reservation: link=%#v err=%v", stored, err)
	}
	if f.transport.ticket["windshift_item_key"] != correlation {
		t.Fatalf("empty search changed remote correlation: ticket=%#v", f.transport.ticket)
	}
}

func TestZammadCreateTicketMarksLocalCompletionFailureUncertainAndKeepsCorrelationPinned(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	if _, err := f.db.ExecWrite(`CREATE TRIGGER fail_zammad_ticket_completion
		BEFORE UPDATE OF ticket_id ON zammad_ticket_links
		WHEN NEW.ticket_id IS NOT NULL
		BEGIN SELECT RAISE(ABORT, 'synthetic completion failure'); END`); err != nil {
		t.Fatal(err)
	}
	_, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err == nil {
		t.Fatal("expected local completion failure")
	}
	link, err := f.service.TicketLinksForItem(f.item1)
	if err != nil || len(link) != 1 || link[0].SyncState != models.ZammadSyncUncertain || f.transport.ticket["windshift_item_key"] != link[0].CorrelationKey {
		t.Fatalf("known remote ticket was not preserved as uncertain: links=%#v ticket=%#v err=%v", link, f.transport.ticket, err)
	}
	if _, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID}); err == nil {
		t.Fatal("expected retry to surface the retained local completion failure")
	}
	_, posts := f.transport.counts()
	if posts != 1 {
		t.Fatalf("retry created a second ticket after local completion failure: posts=%d", posts)
	}
}

func TestZammadCreateTicketMarksAmbiguousHTTPFailuresUncertainWithoutRetryingPOST(t *testing.T) {
	for _, status := range []int{http.StatusInternalServerError, http.StatusRequestTimeout, http.StatusTooManyRequests} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			f := newZammadServiceFixture(t, nil)
			f.transport.hideSearch = true
			f.transport.postStatusAfterCreate = status

			_, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
			var apiErr *zammad.APIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != status {
				t.Fatalf("expected HTTP %d create failure, got %v", status, err)
			}
			links, err := f.service.TicketLinksForItem(f.item1)
			if err != nil || len(links) != 1 || links[0].SyncState != models.ZammadSyncUncertain {
				t.Fatalf("ambiguous HTTP %d result was not retained as uncertain: links=%#v err=%v", status, links, err)
			}

			link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
			if err != nil {
				t.Fatalf("normal retry after ambiguous HTTP %d failed: %v", status, err)
			}
			_, posts := f.transport.counts()
			if posts != 1 || link.SyncState != models.ZammadSyncUncertain {
				t.Fatalf("normal retry after ambiguous HTTP %d sent another POST: link=%#v posts=%d", status, link, posts)
			}
		})
	}
}

func TestZammadCreateTicketMarksRejectingHTTP400Failed(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	f.transport.hideSearch = true
	f.transport.postStatusAfterCreate = http.StatusBadRequest

	_, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	var apiErr *zammad.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected HTTP 400 create failure, got %v", err)
	}
	links, err := f.service.TicketLinksForItem(f.item1)
	if err != nil || len(links) != 1 || links[0].SyncState != models.ZammadSyncFailed {
		t.Fatalf("rejecting HTTP 400 result was not retained as failed: links=%#v err=%v", links, err)
	}
}

func TestZammadCreateTicketRejectsCorrelationMatchInDisallowedGroup(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	f.transport.ticket = map[string]any{
		"id": 715, "number": "420715", "group_id": 99, "state_id": 2, "state": "open",
		"windshift_item_key": "windshift:" + f.connection.ProviderID + ":PRI-49",
	}

	_, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	var validationErr *ZammadValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("correlation match in a disallowed group was accepted: %v", err)
	}
	links, err := f.service.TicketLinksForItem(f.item1)
	if err != nil || len(links) != 1 || links[0].TicketID != 0 || links[0].SyncState == models.ZammadSyncLinked {
		t.Fatalf("disallowed correlation match completed a local link: links=%#v err=%v", links, err)
	}
	var genericLinks int
	if err := f.db.QueryRow("SELECT COUNT(*) FROM item_integration_links").Scan(&genericLinks); err != nil || genericLinks != 0 {
		t.Fatalf("disallowed correlation match created a generic link: count=%d err=%v", genericLinks, err)
	}
}

func TestZammadSyncPersistsRemoteGroupAndOwner(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	f.transport.getTicket = map[string]any{"id": 901, "number": "420901", "group_id": 8, "state_id": 3, "state": "pending", "owner_id": 99}
	f.transport.groups = []map[string]any{{"id": 7, "name": "Support", "active": true}, {"id": 8, "name": "Escalations", "active": true}}
	f.transport.users = []map[string]any{{"id": 99, "active": true, "firstname": "Grace", "lastname": "Hopper", "group_ids": map[string][]string{"8": {"full"}}}}
	allowedGroups := zammadTestGroupRefs(7, 8)
	if _, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{AllowedGroups: &allowedGroups}); err != nil {
		t.Fatal(err)
	}
	synced, err := f.service.SyncTicketLink(context.Background(), link.ID)
	if err != nil {
		t.Fatal(err)
	}
	if synced.GroupID != 8 || synced.GroupName != "Escalations" || synced.OwnerID != 99 || synced.OwnerName != "Grace Hopper" || synced.LastSyncedAt == nil {
		t.Fatalf("sync did not retain remote group/owner state: %#v", synced)
	}
}

func TestZammadSyncRejectsTicketMovedToDisallowedGroupBeforeSnapshotOrCompletion(t *testing.T) {
	workflow := &fakeZammadWorkflow{}
	f := newZammadServiceFixture(t, workflow)
	workflow.db = f.db
	link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	allowedGroups := zammadTestGroupRefs(7)
	closedStates := []int{4}
	if _, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{
		AllowedGroups:      &allowedGroups,
		ClosedStateIDs:     &closedStates,
		CompletionStatusID: &f.doneStatus,
	}); err != nil {
		t.Fatal(err)
	}
	f.transport.getTicket = map[string]any{"id": 901, "number": "420901", "group_id": 99, "state_id": 4, "state": "closed"}
	f.transport.groups = []map[string]any{{"id": 7, "name": "Support", "active": true}, {"id": 99, "name": "Restricted", "active": true}}

	if _, err := f.service.SyncTicketLink(context.Background(), link.ID); err == nil {
		t.Fatal("sync accepted a ticket moved to a disallowed group")
	}
	stored, err := f.service.GetTicketLink(link.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.LastStatusID != link.LastStatusID || stored.GroupID != link.GroupID || stored.CompletionApplied || workflow.writes != 0 {
		t.Fatalf("disallowed remote group changed local snapshot or completed item: link=%#v writes=%d", stored, workflow.writes)
	}
	var statusID int
	if err := f.db.QueryRow("SELECT status_id FROM items WHERE id = ?", f.item1).Scan(&statusID); err != nil {
		t.Fatal(err)
	}
	if statusID != f.openStatus {
		t.Fatalf("disallowed remote group completed the item: status=%d", statusID)
	}
}

func TestZammadDueLinksRespectRetryDelayAndOAuthReauthorization(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	oldSync := time.Now().Add(-10 * time.Minute)
	if _, err := f.db.ExecWrite("UPDATE zammad_ticket_links SET last_synced_at = ? WHERE id = ?", oldSync, link.ID); err != nil {
		t.Fatal(err)
	}
	attemptedAt := time.Now()
	syncOwner := "retry-delay-test"
	claimed, err := f.service.repo.ClaimSync(link.ID, syncOwner, time.Now().Add(time.Minute))
	if err != nil || !claimed {
		t.Fatalf("could not claim retry-delay test link: claimed=%v err=%v", claimed, err)
	}
	if err := f.service.repo.UpdateTicketLinkSync(link.ID, syncOwner, link.LastStatusID, link.LastStatusName,
		link.GroupID, link.GroupName, link.OwnerID, link.OwnerName,
		"synthetic safe failure", attemptedAt, false, false); err != nil {
		t.Fatal(err)
	}
	due, err := f.service.repo.ListDueTicketLinks(time.Now().Add(-2*time.Minute), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 0 {
		t.Fatalf("failed link ignored retry delay: %#v", due)
	}
	if _, err := f.db.ExecWrite("UPDATE zammad_ticket_links SET next_attempt_at = ? WHERE id = ?", time.Now().Add(-time.Minute), link.ID); err != nil {
		t.Fatal(err)
	}
	due, err = f.service.repo.ListDueTicketLinks(time.Now().Add(-2*time.Minute), 10)
	if err != nil || len(due) != 1 || due[0].ID != link.ID {
		t.Fatalf("eligible retry was not scheduled: due=%#v err=%v", due, err)
	}
	if _, err := f.db.ExecWrite("UPDATE zammad_connections SET auth_method = 'oauth' WHERE provider_id = ?", f.connection.ProviderID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecWrite(`INSERT INTO zammad_oauth_tokens(provider_id, oauth_generation, expires_at, reauthorization_required)
		VALUES (?, 1, ?, true)`, f.connection.ProviderID, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	due, err = f.service.repo.ListDueTicketLinks(time.Now().Add(-2*time.Minute), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 0 {
		t.Fatalf("reauthorization-required OAuth connection remained due: %#v", due)
	}
}

func TestZammadRetryAfterAmbiguousTimeoutFindsExistingTicket(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	f.transport.postErrorAfterCreate = context.DeadlineExceeded
	_, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected timeout, got %v", err)
	}
	link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	_, posts := f.transport.counts()
	if posts != 1 || link.TicketID != 901 || link.SyncState != models.ZammadSyncLinked {
		t.Fatalf("retry created a duplicate or failed to recover: link=%#v posts=%d", link, posts)
	}
}

func TestZammadAmbiguousCreateNeverPostsAgainWhileSearchIsEmpty(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	f.transport.hideSearch = true
	f.transport.postErrorAfterCreate = context.DeadlineExceeded
	_, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected timeout, got %v", err)
	}
	link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	_, posts := f.transport.counts()
	if posts != 1 || link.SyncState != models.ZammadSyncUncertain {
		t.Fatalf("uncertain retry sent another POST: state=%s posts=%d", link.SyncState, posts)
	}
	f.transport.postErrorAfterCreate = nil
	link, err = f.service.RetryUncertainTicketCreation(context.Background(), link.ID, f.actorID)
	if err != nil {
		t.Fatal(err)
	}
	_, posts = f.transport.counts()
	if posts != 2 || link.TicketID != 901 {
		t.Fatalf("explicit administrator override did not retry creation: ticket=%d posts=%d", link.TicketID, posts)
	}
}

func TestZammadUncertainCreateSurvivesTransientCorrelationSearchFailure(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	f.transport.postErrorAfterCreate = context.DeadlineExceeded
	_, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected ambiguous initial timeout, got %v", err)
	}
	f.transport.searchError = context.DeadlineExceeded
	if _, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected transient correlation-search failure, got %v", err)
	}
	links, err := f.service.TicketLinksForItem(f.item1)
	if err != nil || len(links) != 1 || links[0].SyncState != models.ZammadSyncUncertain {
		t.Fatalf("search failure downgraded uncertain creation: links=%#v err=%v", links, err)
	}
	f.transport.hideSearch = true
	retried, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	_, posts := f.transport.counts()
	if posts != 1 || retried.SyncState != models.ZammadSyncUncertain {
		t.Fatalf("search failure enabled a duplicate POST: posts=%d link=%#v", posts, retried)
	}
}

func TestZammadStaleCreatingLeaseRetriesSearchOnly(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	item, err := repository.NewItemRepository(f.db).FindByIDWithDetails(f.item1)
	if err != nil {
		t.Fatal(err)
	}
	link := &models.ZammadTicketLink{
		ID: "stale-create-link", ItemID: item.ID, ProviderID: f.connection.ProviderID,
		GroupID: 7, GroupName: "Support", CorrelationKey: "windshift:stale-create:PRI-49",
		SyncState: models.ZammadSyncPending, CreatedBy: &f.actorID,
	}
	if err := f.service.reserveZammadTicketLink(f.connection, item, link, false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecWrite(`UPDATE zammad_ticket_links
		SET sync_state = ?, creating_started_at = ?, sync_lock_until = ?, sync_lock_owner = ?
		WHERE id = ?`, models.ZammadSyncCreating, time.Now().Add(-3*time.Minute), time.Now().Add(-time.Minute), "crashed-worker", link.ID); err != nil {
		t.Fatal(err)
	}
	f.transport.hideSearch = true
	retried, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	_, posts := f.transport.counts()
	if posts != 0 || retried.SyncState != models.ZammadSyncUncertain {
		t.Fatalf("stale in-flight creation retried POST: posts=%d link=%#v", posts, retried)
	}
}

func TestZammadCreateVerifiesGroupFromTicketDetailAfterPartialResponse(t *testing.T) {
	for _, tt := range []struct {
		name      string
		getTicket map[string]any
		wantGroup int
	}{
		{
			name:      "remote reroute is rejected",
			getTicket: map[string]any{"id": 901, "number": "420901", "group_id": 8, "state_id": 2, "state": "open"},
			wantGroup: 8,
		},
		{
			name:      "missing detail group is rejected",
			getTicket: map[string]any{"id": 901, "number": "420901", "state_id": 2, "state": "open"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newZammadServiceFixture(t, nil)
			f.transport.groups = []map[string]any{{"id": 7, "name": "Support", "active": true}, {"id": 8, "name": "Escalations", "active": true}}
			f.transport.createGroupID = tt.wantGroup
			f.transport.postResponse = map[string]any{"id": 901, "number": "420901", "state_id": 2, "state": "open"}
			f.transport.getTicket = tt.getTicket

			_, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
			var validationErr *ZammadValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("partial create response completed without a verified allowed group: %v", err)
			}
			links, err := f.service.TicketLinksForItem(f.item1)
			if err != nil || len(links) != 1 || links[0].SyncState != models.ZammadSyncUncertain || links[0].ItemIntegrationLinkID != "" {
				t.Fatalf("unverified create was not retained as uncertain: links=%#v err=%v", links, err)
			}
			if got, ok := f.transport.ticket["group_id"].(int); tt.wantGroup > 0 && (!ok || got != tt.wantGroup) {
				t.Fatalf("test did not model remote reroute: ticket=%#v", f.transport.ticket)
			}
		})
	}
}

func TestZammadRejectsUnconfiguredGroupBeforeCreate(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	_, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID, GroupID: 99})
	var validationErr *ZammadValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected group validation error, got %v", err)
	}
	_, posts := f.transport.counts()
	if posts != 0 {
		t.Fatalf("unconfigured group reached ticket create endpoint %d times", posts)
	}
}

func TestZammadWorkspaceScopeBlocksHTTPAndSecretIsEncrypted(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	_, err := f.service.CreateTicket(context.Background(), f.item2, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if !errors.Is(err, ErrCredentialScopeMismatch) {
		t.Fatalf("expected scope mismatch, got %v", err)
	}
	requests, _ := f.transport.counts()
	if requests != 0 {
		t.Fatalf("out-of-scope request reached transport %d times", requests)
	}
	encoded, err := json.Marshal(f.connection)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "synthetic-zammad-token") || strings.Contains(string(encoded), "credential_id") {
		t.Fatalf("connection API model disclosed credential material: %s", encoded)
	}
	var encrypted string
	if err := f.db.QueryRow("SELECT encrypted_secret FROM action_credentials WHERE id = ?", f.connection.CredentialID).Scan(&encrypted); err != nil {
		t.Fatal(err)
	}
	if encrypted == "synthetic-zammad-token" || strings.Contains(encrypted, "synthetic-zammad-token") {
		t.Fatal("Zammad token was stored in plaintext")
	}
	listed, err := f.credentials.ListForWorkspace(f.workspace1)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("managed Zammad credential leaked into generic list: %#v", listed)
	}
	if _, err := f.credentials.Get(f.connection.CredentialID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("managed Zammad credential was addressable through generic CRUD: %v", err)
	}
	if _, _, err := f.credentials.Resolve(context.Background(), f.connection.CredentialID, f.workspace1); !errors.Is(err, ErrCredentialPurposeMismatch) {
		t.Fatalf("managed Zammad credential resolved through generic action path: %v", err)
	}
}

func TestZammadConnectionScopeCannotStrandExistingTicketLinks(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}

	excludedScope := []int{f.workspace2}
	if _, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{WorkspaceIDs: &excludedScope}); err == nil {
		t.Fatal("connection scope excluded a workspace containing a linked ticket")
	}
	stored, err := f.service.GetTicketLink(link.ID)
	if err != nil || stored.ItemIntegrationLinkID == "" {
		t.Fatalf("rejected scope update changed the link: link=%#v err=%v", stored, err)
	}
	if _, _, err := f.credentials.ResolveManaged(context.Background(), f.connection.CredentialID, f.workspace1, string(models.IntegrationProviderZammad), f.connection.ProviderID); err != nil {
		t.Fatalf("rejected scope update changed credential scope: %v", err)
	}

	expandedScope := []int{f.workspace1, f.workspace2}
	updated, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{WorkspaceIDs: &expandedScope})
	if err != nil || !slices.Contains(updated.WorkspaceIDs, f.workspace2) {
		t.Fatalf("scope expansion with existing link failed: connection=%#v err=%v", updated, err)
	}
}

func TestZammadConnectionRoutingAndGroupsCannotStrandExistingTicketLinks(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	if _, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID}); err != nil {
		t.Fatal(err)
	}
	newBaseURL := "https://replacement.example.test"
	if _, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{BaseURL: &newBaseURL}); err == nil {
		t.Fatal("base URL changed with an existing ticket link")
	}
	newCorrelationField := "replacement_item_key"
	if _, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{CorrelationField: &newCorrelationField}); err == nil {
		t.Fatal("correlation field changed with an existing ticket link")
	}
	newDefaultGroupID := 8
	newDefaultGroupName := "Escalations"
	newAllowedGroups := zammadTestGroupRefs(8)
	if _, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{
		DefaultGroupID: &newDefaultGroupID, DefaultGroupName: &newDefaultGroupName, AllowedGroups: &newAllowedGroups,
	}); err == nil {
		t.Fatal("group scope excluded an existing ticket link")
	}
	stored, err := f.service.GetConnection(f.connection.ProviderID)
	if err != nil || stored.BaseURL != f.connection.BaseURL || stored.CorrelationField != f.connection.CorrelationField || stored.DefaultGroupID != 7 {
		t.Fatalf("rejected routing/group updates changed connection: connection=%#v err=%v", stored, err)
	}
}

func TestZammadReservationRejectsStaleConnectionAndItemSnapshots(t *testing.T) {
	t.Run("connection routing", func(t *testing.T) {
		f := newZammadServiceFixture(t, nil)
		item, err := repository.NewItemRepository(f.db).FindByIDWithDetails(f.item1)
		if err != nil {
			t.Fatal(err)
		}
		staleConnection := *f.connection
		newBaseURL := "https://new-routing.example.test"
		if _, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{BaseURL: &newBaseURL}); err != nil {
			t.Fatal(err)
		}
		link := &models.ZammadTicketLink{ID: "stale-routing", ItemID: item.ID, ProviderID: f.connection.ProviderID, GroupID: 7, GroupName: "Support", CorrelationKey: "stale", SyncState: models.ZammadSyncPending, CreatedBy: &f.actorID}
		if err := f.service.reserveZammadTicketLink(&staleConnection, item, link, false); !errors.Is(err, ErrZammadLinkReservationConflict) {
			t.Fatalf("stale connection snapshot reserved a link: %v", err)
		}
	})

	t.Run("connection group policy", func(t *testing.T) {
		f := newZammadServiceFixture(t, nil)
		item, err := repository.NewItemRepository(f.db).FindByIDWithDetails(f.item1)
		if err != nil {
			t.Fatal(err)
		}
		staleConnection := *f.connection
		newDefaultGroupID := 8
		newDefaultGroupName := "Escalations"
		newAllowedGroups := zammadTestGroupRefs(8)
		if _, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{
			DefaultGroupID: &newDefaultGroupID, DefaultGroupName: &newDefaultGroupName, AllowedGroups: &newAllowedGroups,
		}); err != nil {
			t.Fatal(err)
		}
		link := &models.ZammadTicketLink{ID: "stale-group", ItemID: item.ID, ProviderID: f.connection.ProviderID, GroupID: 7, GroupName: "Support", CorrelationKey: "stale", SyncState: models.ZammadSyncPending, CreatedBy: &f.actorID}
		if err := f.service.reserveZammadTicketLink(&staleConnection, item, link, false); !errors.Is(err, ErrZammadLinkReservationConflict) {
			t.Fatalf("stale group policy reserved a link: %v", err)
		}
	})

	t.Run("connection default customer", func(t *testing.T) {
		f := newZammadServiceFixture(t, nil)
		item, err := repository.NewItemRepository(f.db).FindByIDWithDetails(f.item1)
		if err != nil {
			t.Fatal(err)
		}
		staleConnection := *f.connection
		newDefaultCustomer := "new-customer@example.test"
		if _, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{DefaultCustomer: &newDefaultCustomer}); err != nil {
			t.Fatal(err)
		}
		link := &models.ZammadTicketLink{ID: "stale-customer", ItemID: item.ID, ProviderID: f.connection.ProviderID, GroupID: 7, GroupName: "Support", CorrelationKey: "stale", SyncState: models.ZammadSyncPending, CreatedBy: &f.actorID}
		if err := f.service.reserveZammadTicketLink(&staleConnection, item, link, false); !errors.Is(err, ErrZammadLinkReservationConflict) {
			t.Fatalf("stale default customer reserved a link: %v", err)
		}
	})

	t.Run("item workspace and key", func(t *testing.T) {
		f := newZammadServiceFixture(t, nil)
		item, err := repository.NewItemRepository(f.db).FindByIDWithDetails(f.item1)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.db.ExecWrite("UPDATE items SET workspace_id = ?, workspace_item_number = 2 WHERE id = ?", f.workspace2, f.item1); err != nil {
			t.Fatal(err)
		}
		link := &models.ZammadTicketLink{ID: "stale-item", ItemID: item.ID, ProviderID: f.connection.ProviderID, GroupID: 7, GroupName: "Support", CorrelationKey: "stale", SyncState: models.ZammadSyncPending, CreatedBy: &f.actorID}
		if err := f.service.reserveZammadTicketLink(f.connection, item, link, false); !errors.Is(err, ErrZammadLinkReservationConflict) {
			t.Fatalf("stale item snapshot reserved a link: %v", err)
		}
	})
}

func TestZammadOutOfScopeLegacyLinkIsNotExposed(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	if _, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecWrite("UPDATE items SET workspace_id = ?, workspace_item_number = 2 WHERE id = ?", f.workspace2, f.item1); err != nil {
		t.Fatal(err)
	}
	links, err := f.service.TicketLinksForItem(f.item1)
	if err != nil || len(links) != 0 {
		t.Fatalf("out-of-scope legacy link was exposed: links=%#v err=%v", links, err)
	}
}

func TestZammadLinkedItemMoveRequiresDestinationConnectionScope(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	var itemTypeID int
	if err := f.db.QueryRow("SELECT id FROM item_types WHERE hierarchy_level >= 0 ORDER BY id LIMIT 1").Scan(&itemTypeID); err != nil {
		t.Fatal(err)
	}
	input := ItemWorkspaceMoveInput{DestinationWorkspaceID: f.workspace2, TargetItemTypeID: itemTypeID, TargetStatusID: f.openStatus}
	moveService := NewItemWorkspaceMoveService(f.db)
	if _, err := moveService.Move(f.item1, f.actorID, input); err == nil {
		t.Fatal("item moved outside its linked Zammad connection scope")
	}

	expandedScope := []int{f.workspace1, f.workspace2}
	if _, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{WorkspaceIDs: &expandedScope}); err != nil {
		t.Fatal(err)
	}
	result, err := moveService.Move(f.item1, f.actorID, input)
	if err != nil {
		t.Fatalf("item move inside linked Zammad connection scope failed: %v", err)
	}
	if result.Item.WorkspaceID != f.workspace2 {
		t.Fatalf("item remained in workspace %d", result.Item.WorkspaceID)
	}
	stored, err := f.service.GetTicketLink(link.ID)
	if err != nil || stored.CorrelationKey != link.CorrelationKey {
		t.Fatalf("allowed item move changed linked ticket identity: link=%#v err=%v", stored, err)
	}
}

func TestZammadDeleteConnectionRejectsExistingTicketLinks(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.DeleteConnection(f.connection.ProviderID); err == nil {
		t.Fatal("connection deletion succeeded despite existing ticket link")
	}
	var providerCount, credentialCount, linkCount int
	if err := f.db.QueryRow("SELECT COUNT(*) FROM integration_providers WHERE id = ?", f.connection.ProviderID).Scan(&providerCount); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow("SELECT COUNT(*) FROM action_credentials WHERE id = ?", f.connection.CredentialID).Scan(&credentialCount); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow("SELECT COUNT(*) FROM zammad_ticket_links WHERE id = ?", link.ID).Scan(&linkCount); err != nil {
		t.Fatal(err)
	}
	if providerCount != 1 || credentialCount != 1 || linkCount != 1 {
		t.Fatalf("rejected connection delete changed persisted data: providers=%d credentials=%d links=%d", providerCount, credentialCount, linkCount)
	}
}

func TestZammadDeleteConnectionWithoutTicketLinksRemovesOwnedCredentialAtomically(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	credentialID := f.connection.CredentialID
	if err := f.service.DeleteConnection(f.connection.ProviderID); err != nil {
		t.Fatal(err)
	}
	var providerCount, credentialCount int
	if err := f.db.QueryRow("SELECT COUNT(*) FROM integration_providers WHERE id = ?", f.connection.ProviderID).Scan(&providerCount); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow("SELECT COUNT(*) FROM action_credentials WHERE id = ?", credentialID).Scan(&credentialCount); err != nil {
		t.Fatal(err)
	}
	if providerCount != 0 || credentialCount != 0 {
		t.Fatalf("connection delete left data behind: providers=%d credentials=%d", providerCount, credentialCount)
	}
}

func TestZammadConnectionUpdateRollsBackManagedCredentialOnProviderConflict(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	other, err := f.service.CreateConnection(models.CreateZammadConnectionRequest{
		Slug: "other-helpdesk", Name: "Other helpdesk", BaseURL: "https://other.example.test",
		APIToken: "other-token", DefaultGroupID: 7, DefaultGroupName: "Support",
		DefaultCustomer: "robot@example.test", WorkspaceIDs: []int{f.workspace1},
	}, f.actorID)
	if err != nil {
		t.Fatal(err)
	}
	newToken := "must-be-rolled-back"
	newScope := []int{f.workspace2}
	if _, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{Slug: &other.Slug, APIToken: &newToken, WorkspaceIDs: &newScope}); !errors.Is(err, repository.ErrDuplicateEntry) {
		t.Fatalf("expected duplicate provider slug, got %v", err)
	}
	secret, _, err := f.credentials.ResolveManaged(context.Background(), f.connection.CredentialID, f.workspace1, string(models.IntegrationProviderZammad), f.connection.ProviderID)
	if err != nil || secret != "synthetic-zammad-token" {
		t.Fatalf("managed credential was not restored: secret_matches=%v err=%v", secret == "synthetic-zammad-token", err)
	}
	if _, _, err := f.credentials.ResolveManaged(context.Background(), f.connection.CredentialID, f.workspace2, string(models.IntegrationProviderZammad), f.connection.ProviderID); !errors.Is(err, ErrCredentialScopeMismatch) {
		t.Fatalf("managed credential scope was not restored: %v", err)
	}
}

func TestZammadConcurrentPartialConnectionUpdatesDoNotOverwriteEachOther(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	f.service.SetUpdateBeforeConnectionLockForTesting(func() {
		arrived <- struct{}{}
		<-release
	})
	defer f.service.SetUpdateBeforeConnectionLockForTesting(nil)

	updatedName := "Concurrent name"
	expandedScope := []int{f.workspace1, f.workspace2}
	errorsByRequest := make(chan error, 2)
	go func() {
		_, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{Name: &updatedName})
		errorsByRequest <- err
	}()
	go func() {
		_, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{WorkspaceIDs: &expandedScope})
		errorsByRequest <- err
	}()

	<-arrived
	<-arrived
	close(release)
	firstErr, secondErr := <-errorsByRequest, <-errorsByRequest
	conflicts := 0
	for _, err := range []error{firstErr, secondErr} {
		if errors.Is(err, repository.ErrConcurrentUpdate) {
			conflicts++
		} else if err != nil {
			t.Fatalf("unexpected concurrent update error: %v", err)
		}
	}
	if conflicts != 1 {
		t.Fatalf("expected exactly one optimistic conflict, got %d: first=%v second=%v", conflicts, firstErr, secondErr)
	}

	stored, err := f.service.GetConnection(f.connection.ProviderID)
	if err != nil {
		t.Fatal(err)
	}
	nameWon := stored.Name == updatedName && slices.Equal(stored.WorkspaceIDs, []int{f.workspace1})
	scopeWon := stored.Name == f.connection.Name && slices.Equal(stored.WorkspaceIDs, expandedScope)
	if !nameWon && !scopeWon {
		t.Fatalf("concurrent partial updates produced a mixed or stale result: name=%q workspaces=%v", stored.Name, stored.WorkspaceIDs)
	}
}

func TestZammadSyncUpdatesStatusAndDisabledConnectionsAreSkipped(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	f.transport.getTicket = map[string]any{"id": 901, "number": "420901", "group_id": 7, "state_id": 3, "state": "pending"}
	link, err = f.service.SyncTicketLink(context.Background(), link.ID)
	if err != nil || link.LastStatusID != 3 || link.LastStatusName != "pending" {
		t.Fatalf("unexpected synced link: link=%#v err=%v", link, err)
	}
	disabled := false
	if _, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{Enabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	f.transport.resetRequests()
	if err := f.service.SyncDue(context.Background(), 50); err != nil {
		t.Fatal(err)
	}
	requests, _ := f.transport.counts()
	if requests != 0 {
		t.Fatalf("disabled connection was polled %d times", requests)
	}
}

func TestZammadDisabledConnectionCanStillBeTestedByAdmin(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	disabled := false
	if _, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{Enabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	metadata, err := f.service.TestConnection(context.Background(), f.connection.ProviderID)
	if err != nil {
		t.Fatal(err)
	}
	if len(metadata.Groups) != 1 || metadata.Groups[0].Name != "Support" {
		t.Fatalf("unexpected connection test metadata: %#v", metadata)
	}
	if metadata.GroupCatalogVerified || metadata.CorrelationFieldVerified {
		t.Fatalf("least-privilege checks must report admin-only metadata as unverified: %#v", metadata)
	}
}

func TestZammadConnectionValidationRequiresUsableDefaultGroup(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	_, err := f.service.CreateConnection(models.CreateZammadConnectionRequest{
		Slug: "missing-default", Name: "Missing default", BaseURL: "https://missing-default.example.test",
		APIToken: "token", DefaultCustomer: "robot@example.test", WorkspaceIDs: []int{f.workspace1},
	}, f.actorID)
	var validationErr *ZammadValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("connection without a default group was accepted: %v", err)
	}

	_, err = f.service.CreateConnection(models.CreateZammadConnectionRequest{
		Slug: "default-outside-allowset", Name: "Default outside allowset", BaseURL: "https://default-outside.example.test",
		APIToken: "token", DefaultGroupID: 7, AllowedGroups: zammadTestGroupRefs(8),
		DefaultCustomer: "robot@example.test", WorkspaceIDs: []int{f.workspace1},
	}, f.actorID)
	if !errors.As(err, &validationErr) {
		t.Fatalf("default group outside allowset was accepted: %v", err)
	}
}

func TestZammadLegacyGroupCatalogCanBeSavedUnchanged(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	connection, err := f.service.CreateConnection(models.CreateZammadConnectionRequest{
		Slug: "legacy-groups", Name: "Legacy groups", BaseURL: "https://legacy-groups.example.test",
		APIToken: "token", DefaultGroupID: 7, DefaultGroupName: "Support", AllowedGroupIDs: []int{7, 8},
		DefaultCustomer: "robot@example.test", WorkspaceIDs: []int{f.workspace1},
	}, f.actorID)
	if err != nil {
		t.Fatal(err)
	}
	if len(connection.AllowedGroups) != 2 || connection.AllowedGroups[1].ID != 8 || connection.AllowedGroups[1].Name != "" {
		t.Fatalf("unexpected legacy catalog: %#v", connection.AllowedGroups)
	}
	name := "Legacy groups renamed"
	updated, err := f.service.UpdateConnection(connection.ProviderID, models.UpdateZammadConnectionRequest{
		Name: &name, AllowedGroups: &connection.AllowedGroups,
	})
	if err != nil {
		t.Fatalf("unchanged legacy group catalog blocked an unrelated update: %v", err)
	}
	if updated.Name != name || len(updated.AllowedGroups) != 2 || updated.AllowedGroups[1].Name != "" {
		t.Fatalf("legacy group catalog was not preserved: %#v", updated)
	}
}

func TestZammadMetadataValidatesOnlyRuntimeTicketStates(t *testing.T) {
	connection := &models.ZammadConnection{DefaultGroupName: "Support", AllowedGroups: zammadTestGroupRefs(8), ClosedStateIDs: []int{4}}
	metadata := &models.ZammadConnectionMetadata{
		Groups: []models.ZammadGroup{{ID: 99, Name: "Different", Active: true}},
		States: []models.ZammadState{{ID: 2, Name: "open", Active: true}},
	}
	var validationErr *ZammadValidationError
	if err := validateZammadStates(connection, metadata); !errors.As(err, &validationErr) {
		t.Fatalf("missing configured closed state was accepted: %v", err)
	}
}

func TestZammadUnknownTicketDoesNotChangeItem(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	f.transport.getStatus = http.StatusNotFound
	if _, err := f.service.SyncTicketLink(context.Background(), link.ID); err == nil {
		t.Fatal("expected remote not-found error")
	}
	var statusID int
	if err := f.db.QueryRow("SELECT status_id FROM items WHERE id = ?", f.item1).Scan(&statusID); err != nil {
		t.Fatal(err)
	}
	if statusID != f.openStatus {
		t.Fatalf("unknown remote ticket changed item status to %d", statusID)
	}
	stored, err := f.service.GetTicketLink(link.ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored.LastError, "synthetic remote detail") {
		t.Fatalf("remote response body leaked into stored error: %q", stored.LastError)
	}
}

func TestZammadClosedStateCompletesItemOnce(t *testing.T) {
	workflow := &fakeZammadWorkflow{}
	f := newZammadServiceFixture(t, workflow)
	workflow.db = f.db
	link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	closed := []int{4}
	if _, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{ClosedStateIDs: &closed, CompletionStatusID: &f.doneStatus}); err != nil {
		t.Fatal(err)
	}
	f.transport.getTicket = map[string]any{"id": 901, "number": "420901", "group_id": 7, "state_id": 4, "state": "closed"}
	if _, err := f.service.SyncTicketLink(context.Background(), link.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.SyncTicketLink(context.Background(), link.ID); err != nil {
		t.Fatal(err)
	}
	var statusID int
	if err := f.db.QueryRow("SELECT status_id FROM items WHERE id = ?", f.item1).Scan(&statusID); err != nil {
		t.Fatal(err)
	}
	if statusID != f.doneStatus || workflow.writes != 1 {
		t.Fatalf("closed-state mapping was not idempotent: status=%d writes=%d", statusID, workflow.writes)
	}
	if _, err := f.db.ExecWrite("UPDATE items SET status_id = ? WHERE id = ?", f.openStatus, f.item1); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.SyncTicketLink(context.Background(), link.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow("SELECT status_id FROM items WHERE id = ?", f.item1).Scan(&statusID); err != nil {
		t.Fatal(err)
	}
	if statusID != f.openStatus || workflow.writes != 1 {
		t.Fatalf("same remote closed episode re-completed a reopened item: status=%d writes=%d", statusID, workflow.writes)
	}
	f.transport.getTicket = map[string]any{"id": 901, "number": "420901", "group_id": 7, "state_id": 2, "state": "open"}
	if _, err := f.service.SyncTicketLink(context.Background(), link.ID); err != nil {
		t.Fatal(err)
	}
	f.transport.getTicket = map[string]any{"id": 901, "number": "420901", "group_id": 7, "state_id": 4, "state": "closed"}
	if _, err := f.service.SyncTicketLink(context.Background(), link.ID); err != nil {
		t.Fatal(err)
	}
	if workflow.writes != 2 {
		t.Fatalf("new remote closed episode did not complete the item: writes=%d", workflow.writes)
	}
}

func TestZammadCompletionPolicyCannotChangeDuringClaimedTransition(t *testing.T) {
	workflow := &fakeZammadWorkflow{}
	f := newZammadServiceFixture(t, workflow)
	workflow.db = f.db
	link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	closed := []int{4}
	if _, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{ClosedStateIDs: &closed, CompletionStatusID: &f.doneStatus}); err != nil {
		t.Fatal(err)
	}
	f.transport.getTicket = map[string]any{"id": 901, "number": "420901", "group_id": 7, "state_id": 4, "state": "closed"}
	arrived := make(chan struct{}, 1)
	release := make(chan struct{})
	f.service.SetCompletionBeforeTransitionForTesting(func() {
		arrived <- struct{}{}
		<-release
	})
	defer f.service.SetCompletionBeforeTransitionForTesting(nil)

	syncErr := make(chan error, 1)
	go func() {
		_, err := f.service.SyncTicketLink(context.Background(), link.ID)
		syncErr <- err
	}()
	<-arrived
	noClosedStates := []int{}
	if _, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{
		ClosedStateIDs: &noClosedStates, ClearCompletionStatus: true,
	}); !errors.Is(err, ErrZammadConnectionBusy) {
		t.Fatalf("completion policy changed during a claimed transition: %v", err)
	}
	close(release)
	if err := <-syncErr; err != nil {
		t.Fatalf("claimed transition failed after conflicting config update was rejected: %v", err)
	}
	stored, err := f.service.GetTicketLink(link.ID)
	if err != nil {
		t.Fatal(err)
	}
	if workflow.writes != 1 || !stored.CompletionApplied {
		t.Fatalf("completion did not commit exactly once: writes=%d link=%#v", workflow.writes, stored)
	}
}

func TestZammadPostCommitTransitionErrorStillFencesClosedEpisode(t *testing.T) {
	workflow := &fakeZammadWorkflow{postCommitErr: errors.New("synthetic post-commit hook failure")}
	f := newZammadServiceFixture(t, workflow)
	workflow.db = f.db
	link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	closed := []int{4}
	if _, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{ClosedStateIDs: &closed, CompletionStatusID: &f.doneStatus}); err != nil {
		t.Fatal(err)
	}
	f.transport.getTicket = map[string]any{"id": link.TicketID, "number": link.TicketNumber, "group_id": 7, "state_id": 4, "state": "closed"}
	if _, err := f.service.SyncTicketLink(context.Background(), link.ID); err == nil {
		t.Fatal("post-commit hook error was not reported")
	}
	stored, err := f.service.GetTicketLink(link.ID)
	if err != nil {
		t.Fatal(err)
	}
	if workflow.writes != 1 || !stored.CompletionApplied {
		t.Fatalf("committed transition was not fenced after hook error: writes=%d link=%#v", workflow.writes, stored)
	}
	if _, err := f.db.ExecWrite("UPDATE items SET status_id = ? WHERE id = ?", f.openStatus, f.item1); err != nil {
		t.Fatal(err)
	}
	workflow.postCommitErr = nil
	if _, err := f.service.SyncTicketLink(context.Background(), link.ID); err != nil {
		t.Fatal(err)
	}
	var statusID int
	if err := f.db.QueryRow("SELECT status_id FROM items WHERE id = ?", f.item1).Scan(&statusID); err != nil {
		t.Fatal(err)
	}
	if workflow.writes != 1 || statusID != f.openStatus {
		t.Fatalf("same remote closed episode replayed after post-commit error: writes=%d status=%d", workflow.writes, statusID)
	}
}

func TestZammadClaimedSyncBlocksConcurrentUnlinkBeforeCompletion(t *testing.T) {
	workflow := &fakeZammadWorkflow{}
	f := newZammadServiceFixture(t, workflow)
	workflow.db = f.db
	link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	closed := []int{4}
	if _, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{ClosedStateIDs: &closed, CompletionStatusID: &f.doneStatus}); err != nil {
		t.Fatal(err)
	}
	f.transport.getTicket = map[string]any{"id": link.TicketID, "number": link.TicketNumber, "group_id": 7, "state_id": 4, "state": "closed"}
	arrived := make(chan struct{}, 1)
	release := make(chan struct{})
	f.service.SetCompletionBeforeTransitionForTesting(func() {
		arrived <- struct{}{}
		<-release
	})
	defer f.service.SetCompletionBeforeTransitionForTesting(nil)

	syncErr := make(chan error, 1)
	go func() {
		_, err := f.service.SyncTicketLink(context.Background(), link.ID)
		syncErr <- err
	}()
	<-arrived
	if _, err := f.service.UnlinkTicket(context.Background(), link.ID); !errors.Is(err, ErrZammadConnectionBusy) {
		t.Fatalf("unlink entered a claimed sync before completion: %v", err)
	}
	close(release)
	if err := <-syncErr; err != nil {
		t.Fatalf("claimed sync failed after concurrent unlink was rejected: %v", err)
	}
	stored, err := f.service.GetTicketLink(link.ID)
	if err != nil {
		t.Fatalf("rejected unlink removed the local link: %v", err)
	}
	if workflow.writes != 1 || !stored.CompletionApplied {
		t.Fatalf("claimed sync did not complete exactly once: writes=%d link=%#v", workflow.writes, stored)
	}
}

func TestZammadExpiredSyncLeaseReloadsChangedCompletionPolicyBeforeTransition(t *testing.T) {
	workflow := &fakeZammadWorkflow{}
	f := newZammadServiceFixture(t, workflow)
	workflow.db = f.db
	link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	closed := []int{4}
	if _, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{ClosedStateIDs: &closed, CompletionStatusID: &f.doneStatus}); err != nil {
		t.Fatal(err)
	}
	f.transport.getTicket = map[string]any{"id": link.TicketID, "number": link.TicketNumber, "group_id": 7, "state_id": 4, "state": "closed"}
	arrived := make(chan struct{}, 1)
	release := make(chan struct{})
	f.service.SetCompletionBeforeTransitionForTesting(func() {
		arrived <- struct{}{}
		<-release
	})
	defer f.service.SetCompletionBeforeTransitionForTesting(nil)

	syncErr := make(chan error, 1)
	go func() {
		_, err := f.service.SyncTicketLink(context.Background(), link.ID)
		syncErr <- err
	}()
	<-arrived
	if _, err := f.db.ExecWrite("UPDATE zammad_ticket_links SET sync_lock_until = ? WHERE id = ?", time.Now().Add(-time.Minute), link.ID); err != nil {
		t.Fatal(err)
	}
	noClosedStates := []int{}
	if _, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{
		ClosedStateIDs: &noClosedStates, ClearCompletionStatus: true,
	}); err != nil {
		t.Fatalf("completion policy could not change after the synthetic lease expiry: %v", err)
	}
	close(release)
	if err := <-syncErr; err != nil {
		t.Fatalf("sync did not recover its untaken lease under the current policy: %v", err)
	}
	var statusID int
	if err := f.db.QueryRow("SELECT status_id FROM items WHERE id = ?", f.item1).Scan(&statusID); err != nil {
		t.Fatal(err)
	}
	if workflow.writes != 0 || statusID != f.openStatus {
		t.Fatalf("expired worker applied the superseded completion policy: writes=%d status=%d", workflow.writes, statusID)
	}
}

func TestZammadTicketCreationClaimBlocksConcurrentUnlinkBeforeCompletion(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	arrived := make(chan struct{}, 1)
	release := make(chan struct{})
	f.service.SetPersistBeforeConnectionLockForTesting(func() {
		arrived <- struct{}{}
		<-release
	})
	defer f.service.SetPersistBeforeConnectionLockForTesting(nil)

	type createResult struct {
		link *models.ZammadTicketLink
		err  error
	}
	result := make(chan createResult, 1)
	go func() {
		link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
		result <- createResult{link: link, err: err}
	}()
	<-arrived
	links, err := f.service.TicketLinksForItem(f.item1)
	if err != nil || len(links) != 1 {
		t.Fatalf("creation reservation was not visible: links=%#v err=%v", links, err)
	}
	if _, err := f.service.UnlinkTicket(context.Background(), links[0].ID); !errors.Is(err, ErrZammadConnectionBusy) {
		t.Fatalf("unlink deleted a claimed creation before local completion: %v", err)
	}
	close(release)
	created := <-result
	if created.err != nil || created.link == nil || created.link.ItemIntegrationLinkID == "" {
		t.Fatalf("ticket creation did not complete after unlink was rejected: link=%#v err=%v", created.link, created.err)
	}
	var genericLinks int
	if err := f.db.QueryRow("SELECT COUNT(*) FROM item_integration_links WHERE id = ?", created.link.ItemIntegrationLinkID).Scan(&genericLinks); err != nil {
		t.Fatal(err)
	}
	if genericLinks != 1 {
		t.Fatalf("ticket creation committed %d generic links, want 1", genericLinks)
	}
}

func TestZammadExistingTicketClaimBlocksConcurrentUnlinkBeforeCompletion(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	f.transport.ticket = map[string]any{"id": 714, "number": "420714", "group_id": 7, "state_id": 2, "state": "open"}
	arrived := make(chan struct{}, 1)
	release := make(chan struct{})
	f.service.SetPersistBeforeConnectionLockForTesting(func() {
		arrived <- struct{}{}
		<-release
	})
	defer f.service.SetPersistBeforeConnectionLockForTesting(nil)

	type linkResult struct {
		link *models.ZammadTicketLink
		err  error
	}
	result := make(chan linkResult, 1)
	go func() {
		link, err := f.service.LinkExistingTicket(context.Background(), f.item1, f.actorID, models.LinkZammadTicketRequest{
			ConnectionID: f.connection.ProviderID, TicketNumber: "420714",
		})
		result <- linkResult{link: link, err: err}
	}()
	<-arrived
	links, err := f.service.TicketLinksForItem(f.item1)
	if err != nil || len(links) != 1 {
		t.Fatalf("existing-ticket reservation was not visible: links=%#v err=%v", links, err)
	}
	if _, err := f.service.UnlinkTicket(context.Background(), links[0].ID); !errors.Is(err, ErrZammadConnectionBusy) {
		t.Fatalf("unlink deleted a claimed existing-ticket link before completion: %v", err)
	}
	close(release)
	linked := <-result
	if linked.err != nil || linked.link == nil || linked.link.ItemIntegrationLinkID == "" {
		t.Fatalf("existing-ticket link did not complete after unlink was rejected: link=%#v err=%v", linked.link, linked.err)
	}
	var genericLinks int
	if err := f.db.QueryRow("SELECT COUNT(*) FROM item_integration_links WHERE id = ?", linked.link.ItemIntegrationLinkID).Scan(&genericLinks); err != nil {
		t.Fatal(err)
	}
	if genericLinks != 1 {
		t.Fatalf("existing-ticket completion committed %d generic links, want 1", genericLinks)
	}
}

func TestZammadSyncReloadsCompletionStateAfterClaim(t *testing.T) {
	workflow := &fakeZammadWorkflow{}
	f := newZammadServiceFixture(t, workflow)
	workflow.db = f.db
	link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	closedStates := []int{4}
	if _, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{ClosedStateIDs: &closedStates, CompletionStatusID: &f.doneStatus}); err != nil {
		t.Fatal(err)
	}
	stale, err := f.service.GetTicketLink(link.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecWrite("UPDATE zammad_ticket_links SET completion_applied = true WHERE id = ?", link.ID); err != nil {
		t.Fatal(err)
	}
	f.transport.getTicket = map[string]any{"id": link.TicketID, "number": link.TicketNumber, "group_id": 7, "state_id": 4, "state": "closed"}
	syncOwner := "stale-completion-test"
	claimed, err := f.service.repo.ClaimSync(link.ID, syncOwner, time.Now().Add(time.Minute))
	if err != nil || !claimed {
		t.Fatalf("could not claim stale-link regression sync: claimed=%v err=%v", claimed, err)
	}
	updated, err := f.service.syncClaimedTicketLink(context.Background(), stale, syncOwner)
	if err != nil {
		t.Fatal(err)
	}
	if workflow.writes != 0 || !updated.CompletionApplied {
		t.Fatalf("stale completion state was replayed: writes=%d link=%#v", workflow.writes, updated)
	}
	var statusID int
	if err := f.db.QueryRow("SELECT status_id FROM items WHERE id = ?", f.item1).Scan(&statusID); err != nil || statusID != f.openStatus {
		t.Fatalf("stale sync changed item status: status=%d err=%v", statusID, err)
	}
}

func TestZammadManualRefreshReportsBusyWhenAnotherWorkerOwnsLease(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := f.service.repo.ClaimSync(link.ID, "scheduler-owner", time.Now().Add(time.Minute))
	if err != nil || !claimed {
		t.Fatalf("scheduler could not claim ticket: claimed=%v err=%v", claimed, err)
	}
	if _, err := f.service.SyncTicketLink(context.Background(), link.ID); !errors.Is(err, ErrZammadConnectionBusy) {
		t.Fatalf("manual refresh reported success while scheduler owned the lease: %v", err)
	}
	if err := f.service.repo.ReleaseSyncClaim(link.ID, "scheduler-owner"); err != nil {
		t.Fatal(err)
	}
}

func TestZammadSyncUsesPersistedGroupPolicyWithoutAdminLookup(t *testing.T) {
	workflow := &fakeZammadWorkflow{}
	f := newZammadServiceFixture(t, workflow)
	workflow.db = f.db
	link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	closedStates := []int{4}
	if _, err := f.service.UpdateConnection(f.connection.ProviderID, models.UpdateZammadConnectionRequest{ClosedStateIDs: &closedStates, CompletionStatusID: &f.doneStatus}); err != nil {
		t.Fatal(err)
	}
	f.transport.groups = []map[string]any{{"id": 7, "name": "Support", "active": false}}
	f.transport.getTicket = map[string]any{"id": link.TicketID, "number": link.TicketNumber, "group_id": 7, "state_id": 4, "state": "closed"}
	_, err = f.service.SyncTicketLink(context.Background(), link.ID)
	if err != nil {
		t.Fatalf("persisted group policy required an admin-only lookup: %v", err)
	}
	stored, getErr := f.service.GetTicketLink(link.ID)
	if getErr != nil || workflow.writes != 1 || !stored.CompletionApplied {
		t.Fatalf("persisted group policy did not complete normal sync: link=%#v writes=%d err=%v", stored, workflow.writes, getErr)
	}
}

func TestNormalizeZammadBaseURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
		err   bool
	}{
		{input: "https://support.example.test/desk/", want: "https://support.example.test/desk"},
		{input: "http://support.example.test", err: true},
		{input: "https://user:secret@support.example.test", err: true},
		{input: "https://support.example.test?token=secret", err: true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := NormalizeZammadBaseURL(tt.input)
			if (err != nil) != tt.err || got != tt.want {
				t.Fatalf("NormalizeZammadBaseURL(%q) = %q, %v", tt.input, got, err)
			}
		})
	}
}

func TestZammadDueSyncUsesAgeThreshold(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecWrite("UPDATE zammad_ticket_links SET last_synced_at = ? WHERE id = ?", time.Now().Add(-10*time.Minute), link.ID); err != nil {
		t.Fatal(err)
	}
	f.transport.resetRequests()
	if err := f.service.SyncDue(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	requests, _ := f.transport.counts()
	if requests != 1 {
		t.Fatalf("expected only the ticket refresh, got %d transport requests", requests)
	}
}

func TestZammadDueSyncPollsFreshLinkOnNextSchedulerTick(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	if _, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID}); err != nil {
		t.Fatal(err)
	}
	f.transport.resetRequests()
	if err := f.service.SyncDue(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	requests, _ := f.transport.counts()
	if requests != 1 {
		t.Fatalf("fresh link missed the next scheduler tick: requests=%d", requests)
	}
}

func TestZammadSyncAllTicketLinksOverridesRetryDelay(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecWrite("UPDATE zammad_ticket_links SET next_attempt_at = ? WHERE id = ?", time.Now().Add(time.Hour), link.ID); err != nil {
		t.Fatal(err)
	}
	f.transport.resetRequests()
	summary, err := f.service.SyncAllTicketLinks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if summary.Selected != 1 || summary.Succeeded != 1 || summary.Failed != 0 || summary.Skipped != 0 {
		t.Fatalf("unexpected system refresh summary: %#v", summary)
	}
	requests, _ := f.transport.counts()
	if requests != 1 {
		t.Fatalf("system refresh did not override the retry delay: requests=%d", requests)
	}
}

func TestZammadSyncAllTicketLinksReportsAlreadyClaimedLinks(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := f.service.repo.ClaimSync(link.ID, "other-worker", time.Now().Add(time.Minute))
	if err != nil || !claimed {
		t.Fatalf("failed to create competing claim: claimed=%v err=%v", claimed, err)
	}
	f.transport.resetRequests()
	summary, err := f.service.SyncAllTicketLinks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if summary.Selected != 1 || summary.Succeeded != 0 || summary.Failed != 0 || summary.Skipped != 1 {
		t.Fatalf("claimed link disappeared from system refresh summary: %#v", summary)
	}
	requests, _ := f.transport.counts()
	if requests != 0 {
		t.Fatalf("claimed link was refreshed concurrently: requests=%d", requests)
	}
}

func TestZammadTicketSyncPublishesLivePanelUpdate(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	publisher := &recordingItemChangePublisher{}
	SetItemChangePublisher(publisher)
	t.Cleanup(func() { SetItemChangePublisher(nil) })

	if _, err := f.service.SyncTicketLink(context.Background(), link.ID); err != nil {
		t.Fatal(err)
	}
	if publisher.itemID != f.item1 || publisher.kind != ItemChangeZammad {
		t.Fatalf("unexpected live-update event: item=%d kind=%q", publisher.itemID, publisher.kind)
	}
}

func TestZammadTicketSyncFailurePublishesLivePanelUpdate(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	link, err := f.service.CreateTicket(context.Background(), f.item1, f.actorID, models.CreateZammadTicketRequest{ConnectionID: f.connection.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	publisher := &recordingItemChangePublisher{}
	SetItemChangePublisher(publisher)
	t.Cleanup(func() { SetItemChangePublisher(nil) })
	f.transport.getStatus = http.StatusBadGateway

	if _, err := f.service.SyncTicketLink(context.Background(), link.ID); err == nil {
		t.Fatal("expected upstream refresh failure")
	}
	if publisher.itemID != f.item1 || publisher.kind != ItemChangeZammad {
		t.Fatalf("failure did not publish a live-update event: item=%d kind=%q", publisher.itemID, publisher.kind)
	}
	stored, err := f.service.GetTicketLink(link.ID)
	if err != nil || stored.LastError == "" || stored.SyncState != models.ZammadSyncFailed {
		t.Fatalf("failure state was not persisted: link=%#v err=%v", stored, err)
	}
}

func TestZammadOAuthConnectionStoresConnectionTokensAndConsumesStateOnce(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	f.service.SetOAuthEncryption(sso.NewSecretEncryption("synthetic-server-secret-for-zammad-tests"))
	f.service.SetOAuthTransportForTesting(zammad.TransportFunc(func(_ context.Context, method, targetURL string, body []byte, headers map[string]string) (*zammad.Response, error) {
		if method != http.MethodPost || targetURL != "https://oauth.example.test/oauth/token" || headers["Content-Type"] != "application/x-www-form-urlencoded" || strings.Contains(string(body), "synthetic-client-secret") == false {
			t.Fatalf("unexpected OAuth token request: %s %s %q %#v", method, targetURL, body, headers)
		}
		return jsonResponse(http.StatusOK, map[string]any{"access_token": "oauth-access-token", "refresh_token": "oauth-refresh-token", "expires_in": 7200}), nil
	}))
	connection, err := f.service.CreateConnection(models.CreateZammadConnectionRequest{
		Slug: "oauth-helpdesk", Name: "OAuth helpdesk", BaseURL: "https://oauth.example.test",
		AuthMethod: models.ZammadAuthMethodOAuth, OAuthClientID: "synthetic-client", OAuthClientSecret: "synthetic-client-secret",
		DefaultGroupName: "Support", DefaultCustomer: "robot@example.test", WorkspaceIDs: []int{f.workspace1},
	}, f.actorID)
	if err != nil {
		t.Fatal(err)
	}
	if connection.CredentialID <= 0 || connection.OAuthConnected || connection.HasAPIToken {
		t.Fatalf("OAuth connection should have a non-token pending credential before callback: %#v", connection)
	}
	pending, _, err := f.credentials.ResolveManaged(context.Background(), connection.CredentialID, f.workspace1, string(models.IntegrationProviderZammad), connection.ProviderID)
	if err != nil || strings.Contains(pending, "token") || !strings.Contains(pending, "pending") {
		t.Fatalf("OAuth pending credential is unsafe: %q, %v", pending, err)
	}
	var credentialsBefore int
	if err := f.db.QueryRow("SELECT COUNT(*) FROM action_credentials").Scan(&credentialsBefore); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.CreateConnection(models.CreateZammadConnectionRequest{
		Slug: "oauth-helpdesk", Name: "Duplicate OAuth helpdesk", BaseURL: "https://duplicate.example.test",
		AuthMethod: models.ZammadAuthMethodOAuth, OAuthClientID: "client", OAuthClientSecret: "secret", DefaultGroupName: "Support", DefaultCustomer: "robot@example.test", WorkspaceIDs: []int{f.workspace1},
	}, f.actorID); !errors.Is(err, repository.ErrDuplicateEntry) {
		t.Fatalf("expected duplicate OAuth connection error, got %v", err)
	}
	var credentialsAfter int
	if err := f.db.QueryRow("SELECT COUNT(*) FROM action_credentials").Scan(&credentialsAfter); err != nil || credentialsAfter != credentialsBefore {
		t.Fatalf("failed OAuth creation left a managed credential behind: before=%d after=%d err=%v", credentialsBefore, credentialsAfter, err)
	}
	authURL, err := f.service.StartOAuth(context.Background(), connection.ProviderID, f.actorID, "https://windshift.example.test")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Path != "/oauth/authorize" || parsed.Query().Get("scope") != "full" || parsed.Query().Get("redirect_uri") != "https://windshift.example.test/api/integrations/oauth/system/zammad/callback" {
		t.Fatalf("unexpected authorization URL: %s", authURL)
	}
	if _, err := f.service.StartOAuth(context.Background(), connection.ProviderID, f.actorID, "http://windshift.example.test"); err == nil {
		t.Fatal("OAuth start accepted a non-HTTPS public base URL")
	}
	abortedURL, err := f.service.StartOAuth(context.Background(), connection.ProviderID, f.actorID, "https://windshift.example.test")
	if err != nil {
		t.Fatal(err)
	}
	abortedState, err := url.Parse(abortedURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.InvalidateOAuthState(abortedState.Query().Get("state")); err != nil {
		t.Fatal(err)
	}
	invalidated, err := f.service.GetConnection(connection.ProviderID)
	if err != nil {
		t.Fatal(err)
	}
	if invalidated.OAuthAttemptID != "" {
		t.Fatalf("invalidated OAuth state retained connection attempt %q", invalidated.OAuthAttemptID)
	}
	if _, err := f.service.CompleteOAuth(context.Background(), abortedState.Query().Get("state"), "aborted-code", "https://windshift.example.test"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("aborted OAuth state remained usable: %v", err)
	}
	usableURL, err := f.service.StartOAuth(context.Background(), connection.ProviderID, f.actorID, "https://windshift.example.test")
	if err != nil {
		t.Fatal(err)
	}
	usableState, err := url.Parse(usableURL)
	if err != nil {
		t.Fatal(err)
	}
	state := usableState.Query().Get("state")
	if _, err := f.service.CompleteOAuth(context.Background(), state, "authorization-code", "https://windshift.example.test"); err != nil {
		t.Fatal(err)
	}
	completed, err := f.service.GetConnection(connection.ProviderID)
	if err != nil || !completed.OAuthConnected || completed.HasAPIToken {
		t.Fatalf("unexpected OAuth connection status after callback: %#v, %v", completed, err)
	}
	if _, err := f.service.StartOAuth(context.Background(), connection.ProviderID, f.actorID, "https://windshift.example.test"); err != nil {
		t.Fatal(err)
	}
	newClientID := "rotated-client"
	reset, err := f.service.UpdateConnection(connection.ProviderID, models.UpdateZammadConnectionRequest{OAuthClientID: &newClientID})
	if err != nil || reset.OAuthConnected || reset.ReauthorizationRequired || reset.HasAPIToken {
		t.Fatalf("OAuth credential change did not reset authorization: %#v, %v", reset, err)
	}
	var stateCount, tokenCount int
	if err := f.db.QueryRow("SELECT COUNT(*) FROM zammad_oauth_state WHERE provider_id = ?", connection.ProviderID).Scan(&stateCount); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow("SELECT COUNT(*) FROM zammad_oauth_tokens WHERE provider_id = ?", connection.ProviderID).Scan(&tokenCount); err != nil {
		t.Fatal(err)
	}
	if stateCount != 0 || tokenCount != 0 {
		t.Fatalf("OAuth reset retained state=%d token=%d", stateCount, tokenCount)
	}
	bundle, err := activeZammadOAuthCredential("access", "refresh")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.credentials.RotateManaged(connection.CredentialID, models.RotateActionCredentialRequest{Secret: bundle}, string(models.IntegrationProviderZammad), connection.ProviderID); err != nil {
		t.Fatal(err)
	}
	if err := repository.NewZammadRepository(f.db).UpsertOAuthToken(repository.ZammadOAuthToken{ProviderID: connection.ProviderID, ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.StartOAuth(context.Background(), connection.ProviderID, f.actorID, "https://windshift.example.test"); err != nil {
		t.Fatal(err)
	}
	newBaseURL := "https://other-zammad.example.test"
	reset, err = f.service.UpdateConnection(connection.ProviderID, models.UpdateZammadConnectionRequest{BaseURL: &newBaseURL})
	if err != nil || reset.OAuthConnected || reset.ReauthorizationRequired {
		t.Fatalf("OAuth base URL change did not reset authorization: %#v, %v", reset, err)
	}
	if err := f.db.QueryRow("SELECT COUNT(*) FROM zammad_oauth_state WHERE provider_id = ?", connection.ProviderID).Scan(&stateCount); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow("SELECT COUNT(*) FROM zammad_oauth_tokens WHERE provider_id = ?", connection.ProviderID).Scan(&tokenCount); err != nil {
		t.Fatal(err)
	}
	if stateCount != 0 || tokenCount != 0 {
		t.Fatalf("OAuth base URL reset retained state=%d token=%d", stateCount, tokenCount)
	}
	if _, err := f.service.CompleteOAuth(context.Background(), state, "authorization-code", "https://windshift.example.test"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("OAuth state was not consumed exactly once: %v", err)
	}
	var encryptedBundle string
	if err := f.db.QueryRow("SELECT encrypted_secret FROM action_credentials WHERE id = ?", connection.CredentialID).Scan(&encryptedBundle); err != nil {
		t.Fatal(err)
	}
	if encryptedBundle == "oauth-access-token" || strings.Contains(encryptedBundle, "oauth-access-token") || strings.Contains(encryptedBundle, "oauth-refresh-token") {
		t.Fatal("OAuth tokens were not encrypted at rest")
	}
}

func createActiveZammadOAuthConnection(t *testing.T, f *zammadServiceFixture, slug string, expiresAt time.Time) *models.ZammadConnection {
	t.Helper()
	f.service.SetOAuthEncryption(sso.NewSecretEncryption("synthetic-server-secret-for-zammad-tests"))
	connection, err := f.service.CreateConnection(models.CreateZammadConnectionRequest{
		Slug: slug, Name: "OAuth " + slug, BaseURL: "https://" + slug + ".example.test",
		AuthMethod: models.ZammadAuthMethodOAuth, OAuthClientID: "client", OAuthClientSecret: "secret",
		DefaultGroupName: "Support", DefaultCustomer: "robot@example.test", WorkspaceIDs: []int{f.workspace1},
	}, f.actorID)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := activeZammadOAuthCredential("old-access", "old-refresh")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.credentials.RotateManaged(connection.CredentialID, models.RotateActionCredentialRequest{Secret: bundle}, string(models.IntegrationProviderZammad), connection.ProviderID); err != nil {
		t.Fatal(err)
	}
	if err := repository.NewZammadRepository(f.db).UpsertOAuthToken(repository.ZammadOAuthToken{
		ProviderID: connection.ProviderID, OAuthGeneration: connection.OAuthGeneration, ExpiresAt: expiresAt,
	}); err != nil {
		t.Fatal(err)
	}
	connection, err = f.service.GetConnection(connection.ProviderID)
	if err != nil {
		t.Fatal(err)
	}
	return connection
}

func assertZammadManagedCredentialMetadata(t *testing.T, f *zammadServiceFixture, connection *models.ZammadConnection, wantName string, wantWorkspaceID int) {
	t.Helper()
	credential, err := repository.NewActionCredentialRepository(f.db).GetActionCredentialByID(connection.CredentialID)
	if err != nil {
		t.Fatal(err)
	}
	if credential.Name != wantName || credential.AppliesToAllWorkspaces || len(credential.WorkspaceIDs) != 1 || credential.WorkspaceIDs[0] != wantWorkspaceID {
		t.Fatalf("OAuth secret write overwrote concurrent credential metadata: %#v", credential)
	}
}

func TestZammadOAuthCallbackCannotCommitAfterConfigurationReset(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	f.service.SetOAuthEncryption(sso.NewSecretEncryption("synthetic-server-secret-for-zammad-tests"))
	connection, err := f.service.CreateConnection(models.CreateZammadConnectionRequest{
		Slug: "callback-race", Name: "OAuth callback race", BaseURL: "https://callback-race.example.test",
		AuthMethod: models.ZammadAuthMethodOAuth, OAuthClientID: "client", OAuthClientSecret: "secret",
		DefaultGroupName: "Support", DefaultCustomer: "robot@example.test", WorkspaceIDs: []int{f.workspace1},
	}, f.actorID)
	if err != nil {
		t.Fatal(err)
	}
	authURL, err := f.service.StartOAuth(context.Background(), connection.ProviderID, f.actorID, "https://windshift.example.test")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	f.service.SetOAuthTransportForTesting(zammad.TransportFunc(func(_ context.Context, _ string, _ string, _ []byte, _ map[string]string) (*zammad.Response, error) {
		close(entered)
		<-release
		return jsonResponse(http.StatusOK, map[string]any{"access_token": "stale-access", "refresh_token": "stale-refresh", "expires_in": 3600}), nil
	}))
	type callbackResult struct {
		err error
	}
	done := make(chan callbackResult, 1)
	go func() {
		_, err := f.service.CompleteOAuth(context.Background(), parsed.Query().Get("state"), "code", "https://windshift.example.test")
		done <- callbackResult{err: err}
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("OAuth callback did not reach token exchange")
	}
	newClientID := "replacement-client"
	reset, err := f.service.UpdateConnection(connection.ProviderID, models.UpdateZammadConnectionRequest{OAuthClientID: &newClientID})
	if err != nil {
		t.Fatal(err)
	}
	close(release)
	result := <-done
	if !errors.Is(result.err, ErrZammadOAuthSuperseded) {
		t.Fatalf("stale callback commit error = %v", result.err)
	}
	if reset.OAuthGeneration <= connection.OAuthGeneration || reset.OAuthConnected {
		t.Fatalf("reset did not advance generation and clear authorization: %#v", reset)
	}
	raw, _, err := f.credentials.ResolveManaged(context.Background(), connection.CredentialID, f.workspace1, string(models.IntegrationProviderZammad), connection.ProviderID)
	if err != nil || !strings.Contains(raw, `"status":"pending"`) || strings.Contains(raw, "stale-access") {
		t.Fatalf("stale callback reactivated credential: raw=%q err=%v", raw, err)
	}
}

func TestZammadOAuthCallbackCannotCommitAfterConnectionIsDisabled(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	f.service.SetOAuthEncryption(sso.NewSecretEncryption("synthetic-server-secret-for-zammad-tests"))
	connection, err := f.service.CreateConnection(models.CreateZammadConnectionRequest{
		Slug: "callback-disable", Name: "OAuth callback disable", BaseURL: "https://callback-disable.example.test",
		AuthMethod: models.ZammadAuthMethodOAuth, OAuthClientID: "client", OAuthClientSecret: "secret",
		DefaultGroupName: "Support", DefaultCustomer: "robot@example.test", WorkspaceIDs: []int{f.workspace1},
	}, f.actorID)
	if err != nil {
		t.Fatal(err)
	}
	authURL, err := f.service.StartOAuth(context.Background(), connection.ProviderID, f.actorID, "https://windshift.example.test")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	f.service.SetOAuthTransportForTesting(zammad.TransportFunc(func(_ context.Context, _ string, _ string, _ []byte, _ map[string]string) (*zammad.Response, error) {
		close(entered)
		<-release
		return jsonResponse(http.StatusOK, map[string]any{"access_token": "stale-access", "refresh_token": "stale-refresh", "expires_in": 3600}), nil
	}))
	done := make(chan error, 1)
	go func() {
		_, callbackErr := f.service.CompleteOAuth(context.Background(), parsed.Query().Get("state"), "code", "https://windshift.example.test")
		done <- callbackErr
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("OAuth callback did not reach token exchange")
	}
	disabled := false
	if _, err := f.service.UpdateConnection(connection.ProviderID, models.UpdateZammadConnectionRequest{Enabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; !errors.Is(err, ErrZammadOAuthSuperseded) {
		t.Fatalf("disabled connection accepted OAuth credentials: %v", err)
	}
	if _, err := f.service.StartOAuth(context.Background(), connection.ProviderID, f.actorID, "https://windshift.example.test"); err == nil {
		t.Fatal("disabled connection allowed a new OAuth attempt")
	}
}

func TestZammadOAuthCallbackPreservesConcurrentNameAndScopeUpdate(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	f.service.SetOAuthEncryption(sso.NewSecretEncryption("synthetic-server-secret-for-zammad-tests"))
	connection, err := f.service.CreateConnection(models.CreateZammadConnectionRequest{
		Slug: "callback-metadata-race", Name: "Original callback name", BaseURL: "https://callback-metadata-race.example.test",
		AuthMethod: models.ZammadAuthMethodOAuth, OAuthClientID: "client", OAuthClientSecret: "secret",
		DefaultGroupName: "Support", DefaultCustomer: "robot@example.test", WorkspaceIDs: []int{f.workspace1},
	}, f.actorID)
	if err != nil {
		t.Fatal(err)
	}
	authURL, err := f.service.StartOAuth(context.Background(), connection.ProviderID, f.actorID, "https://windshift.example.test")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	f.service.SetOAuthTransportForTesting(zammad.TransportFunc(func(_ context.Context, _ string, _ string, _ []byte, _ map[string]string) (*zammad.Response, error) {
		close(entered)
		<-release
		return jsonResponse(http.StatusOK, map[string]any{"access_token": "callback-access", "refresh_token": "callback-refresh", "expires_in": 3600}), nil
	}))
	done := make(chan error, 1)
	go func() {
		_, err := f.service.CompleteOAuth(context.Background(), parsed.Query().Get("state"), "code", "https://windshift.example.test")
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("OAuth callback did not reach token exchange")
	}
	updatedName := "Renamed during callback"
	updatedScope := []int{f.workspace2}
	if _, err := f.service.UpdateConnection(connection.ProviderID, models.UpdateZammadConnectionRequest{Name: &updatedName, WorkspaceIDs: &updatedScope}); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	assertZammadManagedCredentialMetadata(t, f, connection, updatedName+" Zammad OAuth credentials", f.workspace2)
	raw, _, err := f.credentials.ResolveManaged(context.Background(), connection.CredentialID, f.workspace2, string(models.IntegrationProviderZammad), connection.ProviderID)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := parseZammadOAuthCredential(raw)
	if err != nil || bundle.AccessToken != "callback-access" {
		t.Fatalf("callback secret was not committed: bundle=%#v err=%v", bundle, err)
	}
}

func TestZammadOAuthParallelStartsLeaveOnlyNewestUsableState(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	f.service.SetOAuthEncryption(sso.NewSecretEncryption("synthetic-server-secret-for-zammad-tests"))
	connection, err := f.service.CreateConnection(models.CreateZammadConnectionRequest{
		Slug: "parallel-state", Name: "Parallel OAuth state", BaseURL: "https://parallel-state.example.test",
		AuthMethod: models.ZammadAuthMethodOAuth, OAuthClientID: "client", OAuthClientSecret: "secret",
		DefaultGroupName: "Support", DefaultCustomer: "robot@example.test", WorkspaceIDs: []int{f.workspace1},
	}, f.actorID)
	if err != nil {
		t.Fatal(err)
	}
	f.service.SetOAuthTransportForTesting(zammad.TransportFunc(func(_ context.Context, _ string, _ string, _ []byte, _ map[string]string) (*zammad.Response, error) {
		return jsonResponse(http.StatusOK, map[string]any{"access_token": "parallel-access", "refresh_token": "parallel-refresh", "expires_in": 3600}), nil
	}))
	type startResult struct {
		state string
		err   error
	}
	results := make(chan startResult, 2)
	start := func() {
		authURL, err := f.service.StartOAuth(context.Background(), connection.ProviderID, f.actorID, "https://windshift.example.test")
		if err != nil {
			results <- startResult{err: err}
			return
		}
		parsed, parseErr := url.Parse(authURL)
		if parseErr != nil {
			results <- startResult{err: parseErr}
			return
		}
		results <- startResult{state: parsed.Query().Get("state")}
	}
	go start()
	go start()
	first, second := <-results, <-results
	if first.err != nil || second.err != nil || first.state == "" || second.state == "" || first.state == second.state {
		t.Fatalf("parallel OAuth starts failed: first=%#v second=%#v", first, second)
	}
	var persistedState string
	var stateCount int
	if err := f.db.QueryRow("SELECT state FROM zammad_oauth_state WHERE provider_id = ?", connection.ProviderID).Scan(&persistedState); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow("SELECT COUNT(*) FROM zammad_oauth_state WHERE provider_id = ?", connection.ProviderID).Scan(&stateCount); err != nil || stateCount != 1 {
		t.Fatalf("parallel starts retained %d states: %v", stateCount, err)
	}
	staleState := first.state
	if persistedState == first.state {
		staleState = second.state
	}
	if _, err := f.service.CompleteOAuth(context.Background(), staleState, "stale-code", "https://windshift.example.test"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("superseded parallel state remained usable: %v", err)
	}
	if _, err := f.service.CompleteOAuth(context.Background(), persistedState, "winning-code", "https://windshift.example.test"); err != nil {
		t.Fatalf("winning parallel state failed: %v", err)
	}
}

func TestZammadOAuthNewStartSupersedesAlreadyRunningCallback(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	f.service.SetOAuthEncryption(sso.NewSecretEncryption("synthetic-server-secret-for-zammad-tests"))
	connection, err := f.service.CreateConnection(models.CreateZammadConnectionRequest{
		Slug: "running-callback", Name: "Running OAuth callback", BaseURL: "https://running-callback.example.test",
		AuthMethod: models.ZammadAuthMethodOAuth, OAuthClientID: "client", OAuthClientSecret: "secret",
		DefaultGroupName: "Support", DefaultCustomer: "robot@example.test", WorkspaceIDs: []int{f.workspace1},
	}, f.actorID)
	if err != nil {
		t.Fatal(err)
	}
	oldURL, err := f.service.StartOAuth(context.Background(), connection.ProviderID, f.actorID, "https://windshift.example.test")
	if err != nil {
		t.Fatal(err)
	}
	oldParsed, err := url.Parse(oldURL)
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var transportMu sync.Mutex
	transportCalls := 0
	f.service.SetOAuthTransportForTesting(zammad.TransportFunc(func(_ context.Context, _ string, _ string, _ []byte, _ map[string]string) (*zammad.Response, error) {
		transportMu.Lock()
		transportCalls++
		call := transportCalls
		transportMu.Unlock()
		if call == 1 {
			close(entered)
			<-release
			return jsonResponse(http.StatusOK, map[string]any{"access_token": "superseded-access", "refresh_token": "superseded-refresh", "expires_in": 3600}), nil
		}
		return jsonResponse(http.StatusOK, map[string]any{"access_token": "winning-access", "refresh_token": "winning-refresh", "expires_in": 3600}), nil
	}))
	oldDone := make(chan error, 1)
	go func() {
		_, err := f.service.CompleteOAuth(context.Background(), oldParsed.Query().Get("state"), "old-code", "https://windshift.example.test")
		oldDone <- err
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("old callback did not reach token exchange")
	}
	newURL, err := f.service.StartOAuth(context.Background(), connection.ProviderID, f.actorID, "https://windshift.example.test")
	if err != nil {
		t.Fatal(err)
	}
	newParsed, err := url.Parse(newURL)
	if err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-oldDone; !errors.Is(err, ErrZammadOAuthSuperseded) {
		t.Fatalf("already-running old callback commit error = %v", err)
	}
	if _, err := f.service.CompleteOAuth(context.Background(), newParsed.Query().Get("state"), "new-code", "https://windshift.example.test"); err != nil {
		t.Fatalf("new callback failed after superseding running callback: %v", err)
	}
	raw, _, err := f.credentials.ResolveManaged(context.Background(), connection.CredentialID, f.workspace1, string(models.IntegrationProviderZammad), connection.ProviderID)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := parseZammadOAuthCredential(raw)
	if err != nil || bundle.AccessToken != "winning-access" || bundle.RefreshToken != "winning-refresh" {
		t.Fatalf("superseded callback overwrote winning tokens: bundle=%#v err=%v", bundle, err)
	}
}

func TestZammadOAuthRefreshCannotCommitAfterConfigurationReset(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	connection := createActiveZammadOAuthConnection(t, f, "refresh-race", time.Now().Add(-time.Minute))
	entered, release := make(chan struct{}), make(chan struct{})
	f.service.SetOAuthTransportForTesting(zammad.TransportFunc(func(_ context.Context, _ string, _ string, _ []byte, _ map[string]string) (*zammad.Response, error) {
		close(entered)
		<-release
		return jsonResponse(http.StatusOK, map[string]any{"access_token": "stale-refreshed-access", "refresh_token": "stale-refreshed-refresh", "expires_in": 3600}), nil
	}))
	done := make(chan error, 1)
	go func() {
		_, err := f.service.oauthAccessToken(context.Background(), connection, f.workspace1)
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("OAuth refresh did not reach token exchange")
	}
	newClientID := "replacement-client"
	reset, err := f.service.UpdateConnection(connection.ProviderID, models.UpdateZammadConnectionRequest{OAuthClientID: &newClientID})
	if err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; !errors.Is(err, ErrZammadOAuthSuperseded) {
		t.Fatalf("stale refresh commit error = %v", err)
	}
	if reset.OAuthGeneration <= connection.OAuthGeneration || reset.OAuthConnected {
		t.Fatalf("reset did not advance generation and clear authorization: %#v", reset)
	}
	raw, _, err := f.credentials.ResolveManaged(context.Background(), connection.CredentialID, f.workspace1, string(models.IntegrationProviderZammad), connection.ProviderID)
	if err != nil || !strings.Contains(raw, `"status":"pending"`) || strings.Contains(raw, "stale-refreshed-access") {
		t.Fatalf("stale refresh reactivated credential: raw=%q err=%v", raw, err)
	}
}

func TestZammadOAuthRefreshPreservesConcurrentNameAndScopeUpdate(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	connection := createActiveZammadOAuthConnection(t, f, "refresh-metadata-race", time.Now().Add(-time.Minute))
	entered, release := make(chan struct{}), make(chan struct{})
	f.service.SetOAuthTransportForTesting(zammad.TransportFunc(func(_ context.Context, _ string, _ string, _ []byte, _ map[string]string) (*zammad.Response, error) {
		close(entered)
		<-release
		return jsonResponse(http.StatusOK, map[string]any{"access_token": "refreshed-access", "refresh_token": "refreshed-refresh", "expires_in": 3600}), nil
	}))
	done := make(chan struct {
		token string
		err   error
	}, 1)
	go func() {
		token, err := f.service.oauthAccessToken(context.Background(), connection, f.workspace1)
		done <- struct {
			token string
			err   error
		}{token: token, err: err}
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("OAuth refresh did not reach token exchange")
	}
	updatedName := "Renamed during refresh"
	updatedScope := []int{f.workspace2}
	if _, err := f.service.UpdateConnection(connection.ProviderID, models.UpdateZammadConnectionRequest{Name: &updatedName, WorkspaceIDs: &updatedScope}); err != nil {
		t.Fatal(err)
	}
	close(release)
	result := <-done
	if result.err != nil || result.token != "refreshed-access" {
		t.Fatalf("refresh result: token=%q err=%v", result.token, result.err)
	}
	assertZammadManagedCredentialMetadata(t, f, connection, updatedName+" Zammad OAuth credentials", f.workspace2)
}

func TestZammadOAuthConcurrentRefreshWaitsForRotatedCredential(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	connection := createActiveZammadOAuthConnection(t, f, "refresh-contention", time.Now().Add(-time.Minute))
	entered, release := make(chan struct{}), make(chan struct{})
	var transportMu sync.Mutex
	transportCalls := 0
	f.service.SetOAuthTransportForTesting(zammad.TransportFunc(func(_ context.Context, _ string, _ string, _ []byte, _ map[string]string) (*zammad.Response, error) {
		transportMu.Lock()
		transportCalls++
		call := transportCalls
		transportMu.Unlock()
		if call == 1 {
			close(entered)
			<-release
		}
		return jsonResponse(http.StatusOK, map[string]any{"access_token": "shared-refreshed-access", "refresh_token": "shared-refreshed-refresh", "expires_in": 3600}), nil
	}))
	type tokenResult struct {
		token string
		err   error
	}
	results := make(chan tokenResult, 2)
	refresh := func() {
		token, err := f.service.oauthAccessToken(context.Background(), connection, f.workspace1)
		results <- tokenResult{token: token, err: err}
	}
	go refresh()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first OAuth refresh did not reach token exchange")
	}
	go refresh()
	time.Sleep(2 * zammadOAuthRefreshPollInterval)
	transportMu.Lock()
	callsBeforeRelease := transportCalls
	transportMu.Unlock()
	if callsBeforeRelease != 1 {
		t.Fatalf("contending request issued a second refresh: calls=%d", callsBeforeRelease)
	}
	close(release)
	first, second := <-results, <-results
	for i, result := range []tokenResult{first, second} {
		if result.err != nil || result.token != "shared-refreshed-access" {
			t.Fatalf("refresh result %d did not reuse rotated access token: token=%q err=%v", i+1, result.token, result.err)
		}
	}
	transportMu.Lock()
	finalCalls := transportCalls
	transportMu.Unlock()
	if finalCalls != 1 {
		t.Fatalf("parallel refresh used the refresh token %d times", finalCalls)
	}
}

func TestZammadOAuthRefreshCommitRequiresCurrentClaimOwner(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	connection := createActiveZammadOAuthConnection(t, f, "claim-loss", time.Now().Add(-time.Minute))
	entered, release := make(chan struct{}), make(chan struct{})
	f.service.SetOAuthTransportForTesting(zammad.TransportFunc(func(_ context.Context, _ string, _ string, _ []byte, _ map[string]string) (*zammad.Response, error) {
		close(entered)
		<-release
		return jsonResponse(http.StatusOK, map[string]any{"access_token": "claim-lost-access", "refresh_token": "claim-lost-refresh", "expires_in": 3600}), nil
	}))
	done := make(chan error, 1)
	go func() {
		_, err := f.service.oauthAccessToken(context.Background(), connection, f.workspace1)
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("OAuth refresh did not reach token exchange")
	}
	var claimOwner string
	var leaseUntil time.Time
	if err := f.db.QueryRow("SELECT refresh_claim_owner, refresh_lock_until FROM zammad_oauth_tokens WHERE provider_id = ?", connection.ProviderID).Scan(&claimOwner, &leaseUntil); err != nil {
		t.Fatal(err)
	}
	if claimOwner == "" || time.Until(leaseUntil) <= zammadHTTPTimeout {
		t.Fatalf("refresh lease is not safely longer than HTTP timeout: owner=%q remaining=%s", claimOwner, time.Until(leaseUntil))
	}
	if _, err := f.db.ExecWrite("UPDATE zammad_oauth_tokens SET refresh_claim_owner = ? WHERE provider_id = ?", "replacement-owner", connection.ProviderID); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; !errors.Is(err, ErrZammadOAuthSuperseded) {
		t.Fatalf("claim-lost refresh commit error = %v", err)
	}
	raw, _, err := f.credentials.ResolveManaged(context.Background(), connection.CredentialID, f.workspace1, string(models.IntegrationProviderZammad), connection.ProviderID)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := parseZammadOAuthCredential(raw)
	if err != nil || bundle.AccessToken != "old-access" || bundle.RefreshToken != "old-refresh" {
		t.Fatalf("claim-lost refresh overwrote credential: bundle=%#v err=%v", bundle, err)
	}
}

func TestZammadOAuthRefreshRereadsRotatedCredentialAfterClaim(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	connection := createActiveZammadOAuthConnection(t, f, "refresh-reread", time.Now().Add(-time.Minute))
	beforeClaim, continueClaim := make(chan struct{}), make(chan struct{})
	f.service.SetOAuthBeforeRefreshClaimForTesting(func() {
		close(beforeClaim)
		<-continueClaim
	})
	var transportMu sync.Mutex
	transportCalls := 0
	f.service.SetOAuthTransportForTesting(zammad.TransportFunc(func(_ context.Context, _ string, _ string, _ []byte, _ map[string]string) (*zammad.Response, error) {
		transportMu.Lock()
		transportCalls++
		transportMu.Unlock()
		return jsonResponse(http.StatusOK, map[string]any{"access_token": "unexpected-access", "refresh_token": "unexpected-refresh", "expires_in": 3600}), nil
	}))
	done := make(chan struct {
		token string
		err   error
	}, 1)
	go func() {
		token, err := f.service.oauthAccessToken(context.Background(), connection, f.workspace1)
		done <- struct {
			token string
			err   error
		}{token: token, err: err}
	}()
	select {
	case <-beforeClaim:
	case <-time.After(5 * time.Second):
		t.Fatal("second refresh request did not pause before its claim")
	}
	rotatedBundle, err := activeZammadOAuthCredential("already-refreshed-access", "already-rotated-refresh")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.credentials.RotateManaged(connection.CredentialID, models.RotateActionCredentialRequest{Secret: rotatedBundle}, string(models.IntegrationProviderZammad), connection.ProviderID); err != nil {
		t.Fatal(err)
	}
	if err := repository.NewZammadRepository(f.db).UpsertOAuthToken(repository.ZammadOAuthToken{
		ProviderID: connection.ProviderID, OAuthGeneration: connection.OAuthGeneration, ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	close(continueClaim)
	result := <-done
	if result.err != nil || result.token != "already-refreshed-access" {
		t.Fatalf("second request did not reuse freshly rotated credential: token=%q err=%v", result.token, result.err)
	}
	transportMu.Lock()
	calls := transportCalls
	transportMu.Unlock()
	if calls != 0 {
		t.Fatalf("second request reused a stale refresh token in %d upstream calls", calls)
	}
	var owner *string
	if err := f.db.QueryRow("SELECT refresh_claim_owner FROM zammad_oauth_tokens WHERE provider_id = ?", connection.ProviderID).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if owner != nil {
		t.Fatalf("fresh-token fast path retained refresh claim %q", *owner)
	}
}

func TestZammadOAuthInvalidGrantReturnsPersistenceFailure(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	connection := createActiveZammadOAuthConnection(t, f, "invalid-grant-persist", time.Now().Add(-time.Minute))
	f.service.SetOAuthTransportForTesting(zammad.TransportFunc(func(_ context.Context, _ string, _ string, _ []byte, _ map[string]string) (*zammad.Response, error) {
		return jsonResponse(http.StatusBadRequest, map[string]string{"error": "invalid_grant"}), nil
	}))
	if _, err := f.db.ExecWrite(`CREATE TRIGGER fail_zammad_reauthorization
		BEFORE UPDATE ON action_credentials
		BEGIN SELECT RAISE(ABORT, 'synthetic credential persistence failure'); END`); err != nil {
		t.Fatal(err)
	}
	_, err := f.service.oauthAccessToken(context.Background(), connection, f.workspace1)
	if err == nil || errors.Is(err, ErrZammadReauthorizationRequired) {
		t.Fatalf("invalid_grant persistence failure was hidden: %v", err)
	}
	token, tokenErr := repository.NewZammadRepository(f.db).GetOAuthToken(connection.ProviderID)
	if tokenErr != nil || token.ReauthorizationRequired {
		t.Fatalf("failed transaction partially marked reauthorization: token=%#v err=%v", token, tokenErr)
	}
	raw, _, resolveErr := f.credentials.ResolveManaged(context.Background(), connection.CredentialID, f.workspace1, string(models.IntegrationProviderZammad), connection.ProviderID)
	if resolveErr != nil {
		t.Fatal(resolveErr)
	}
	bundle, parseErr := parseZammadOAuthCredential(raw)
	if parseErr != nil || bundle.AccessToken != "old-access" {
		t.Fatalf("failed transaction partially replaced credential: bundle=%#v err=%v", bundle, parseErr)
	}
}

func TestZammadOAuthInvalidGrantPreservesConcurrentNameAndScopeUpdate(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	connection := createActiveZammadOAuthConnection(t, f, "invalid-grant-metadata-race", time.Now().Add(-time.Minute))
	entered, release := make(chan struct{}), make(chan struct{})
	f.service.SetOAuthTransportForTesting(zammad.TransportFunc(func(_ context.Context, _ string, _ string, _ []byte, _ map[string]string) (*zammad.Response, error) {
		close(entered)
		<-release
		return jsonResponse(http.StatusBadRequest, map[string]string{"error": "invalid_grant"}), nil
	}))
	done := make(chan error, 1)
	go func() {
		_, err := f.service.oauthAccessToken(context.Background(), connection, f.workspace1)
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("OAuth invalid_grant test did not reach token exchange")
	}
	updatedName := "Renamed during invalid grant"
	updatedScope := []int{f.workspace2}
	if _, err := f.service.UpdateConnection(connection.ProviderID, models.UpdateZammadConnectionRequest{Name: &updatedName, WorkspaceIDs: &updatedScope}); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; !errors.Is(err, ErrZammadReauthorizationRequired) {
		t.Fatalf("invalid_grant result = %v", err)
	}
	assertZammadManagedCredentialMetadata(t, f, connection, updatedName+" Zammad OAuth credentials", f.workspace2)
	raw, _, err := f.credentials.ResolveManaged(context.Background(), connection.CredentialID, f.workspace2, string(models.IntegrationProviderZammad), connection.ProviderID)
	if err != nil || !strings.Contains(raw, `"status":"reauthorization_required"`) {
		t.Fatalf("invalid_grant did not persist reauthorization secret: raw=%q err=%v", raw, err)
	}
}

func TestZammadOAuthTestSkipsAdminOnlyCorrelationFieldCheck(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	f.service.SetOAuthEncryption(sso.NewSecretEncryption("synthetic-server-secret-for-zammad-tests"))
	connection, err := f.service.CreateConnection(models.CreateZammadConnectionRequest{
		Slug: "oauth-scope", Name: "Scoped OAuth", BaseURL: "https://scope.example.test", AuthMethod: models.ZammadAuthMethodOAuth,
		OAuthClientID: "client", OAuthClientSecret: "secret", DefaultGroupID: 7, DefaultGroupName: "Support",
		AllowedGroups: zammadTestGroupRefs(7), DefaultCustomer: "robot@example.test", WorkspaceIDs: []int{f.workspace1},
	}, f.actorID)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := activeZammadOAuthCredential("access", "refresh")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.credentials.RotateManaged(connection.CredentialID, models.RotateActionCredentialRequest{Secret: bundle}, string(models.IntegrationProviderZammad), connection.ProviderID); err != nil {
		t.Fatal(err)
	}
	if err := repository.NewZammadRepository(f.db).UpsertOAuthToken(repository.ZammadOAuthToken{ProviderID: connection.ProviderID, ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	calledObjectManager := false
	f.service.SetTransportForTesting(zammad.TransportFunc(func(_ context.Context, _ string, targetURL string, _ []byte, _ map[string]string) (*zammad.Response, error) {
		if strings.Contains(targetURL, "object_manager_attributes") {
			calledObjectManager = true
			return jsonResponse(http.StatusForbidden, map[string]string{}), nil
		}
		return jsonResponse(http.StatusOK, []map[string]any{{"id": 2, "name": "open", "active": true}}), nil
	}))
	metadata, err := f.service.TestConnection(context.Background(), connection.ProviderID)
	if err != nil || metadata.CorrelationFieldVerified || calledObjectManager {
		t.Fatalf("OAuth test must not require admin.object: metadata=%#v err=%v called=%v", metadata, err, calledObjectManager)
	}
}

func TestZammadAPITokenTestSkipsAdminOnlyCorrelationFieldCheck(t *testing.T) {
	f := newZammadServiceFixture(t, nil)
	calledObjectManager := false
	f.service.SetTransportForTesting(zammad.TransportFunc(func(_ context.Context, _ string, targetURL string, _ []byte, _ map[string]string) (*zammad.Response, error) {
		if strings.Contains(targetURL, "object_manager_attributes") {
			calledObjectManager = true
			return jsonResponse(http.StatusForbidden, map[string]string{}), nil
		}
		return jsonResponse(http.StatusOK, []map[string]any{{"id": 2, "name": "open", "active": true}}), nil
	}))
	metadata, err := f.service.TestConnection(context.Background(), f.connection.ProviderID)
	if err != nil || metadata.CorrelationFieldVerified || calledObjectManager {
		t.Fatalf("API-token test must not require admin.object: metadata=%#v err=%v called=%v", metadata, err, calledObjectManager)
	}
}

func TestZammadSafeTransportHonorsAllowLocalConnections(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/ticket_states" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()
	previous := utils.AllowLocalConnections()
	defer utils.SetAllowLocalConnections(previous)
	transport := newZammadSafeTransport(server.URL, "/api/v1/")
	utils.SetAllowLocalConnections(true)
	if _, err := transport.Do(context.Background(), http.MethodGet, server.URL+"/api/v1/ticket_states", nil, nil); err != nil {
		t.Fatalf("ALLOW_LOCAL_CONNECTIONS=true blocked local Zammad target: %v", err)
	}
	utils.SetAllowLocalConnections(false)
	if _, err := transport.Do(context.Background(), http.MethodGet, server.URL+"/api/v1/ticket_states", nil, nil); !errors.Is(err, utils.ErrBlockedSSRFAddr) {
		t.Fatalf("ALLOW_LOCAL_CONNECTIONS=false did not block local Zammad target: %v", err)
	}
}

func TestZammadClientsUseExactlyOneExpectedAuthorizationScheme(t *testing.T) {
	for _, testCase := range []struct {
		name, want string
		client     *zammad.Client
	}{
		{name: "legacy", want: "Token token=legacy-token", client: zammad.NewClient("https://zammad.example.test", "legacy-token", zammad.TransportFunc(func(_ context.Context, _ string, _ string, _ []byte, headers map[string]string) (*zammad.Response, error) {
			if headers["Authorization"] != "Token token=legacy-token" {
				t.Fatalf("legacy authorization = %q", headers["Authorization"])
			}
			return jsonResponse(http.StatusOK, []map[string]any{}), nil
		}))},
		{name: "oauth", want: "Bearer oauth-token", client: zammad.NewOAuthClient("https://zammad.example.test", "oauth-token", zammad.TransportFunc(func(_ context.Context, _ string, _ string, _ []byte, headers map[string]string) (*zammad.Response, error) {
			if headers["Authorization"] != "Bearer oauth-token" {
				t.Fatalf("OAuth authorization = %q", headers["Authorization"])
			}
			return jsonResponse(http.StatusOK, []map[string]any{}), nil
		}))},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := testCase.client.States(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}
