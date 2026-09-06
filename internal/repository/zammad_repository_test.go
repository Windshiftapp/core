package repository

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

type zammadLeaseFixture struct {
	db     database.Database
	repo   *ZammadRepository
	linkID string
}

func newZammadLeaseFixture(t *testing.T) *zammadLeaseFixture {
	t.Helper()
	db, err := database.NewSQLiteDB(filepath.Join(t.TempDir(), "zammad-repository.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Initialize(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecWrite(`INSERT INTO users (id, email, username, first_name, last_name)
		VALUES (1, 'zammad-repository@example.test', 'zammad-repository', 'Zammad', 'Repository')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecWrite(`INSERT INTO workspaces (id, name, key) VALUES (1, 'Zammad tests', 'ZRT')`); err != nil {
		t.Fatal(err)
	}
	var statusID int
	if err := db.QueryRow(`SELECT id FROM statuses ORDER BY id LIMIT 1`).Scan(&statusID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecWrite(`INSERT INTO items
		(id, workspace_id, workspace_item_number, title, description, frac_index, status_id, creator_id, last_active_at)
		VALUES (1, 1, 1, 'Zammad repository lease test', '', 'a0', ?, 1, CURRENT_TIMESTAMP)`, statusID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecWrite(`INSERT INTO action_credentials
		(id, name, credential_type, applies_to_all_workspaces, encrypted_secret, is_enabled)
		VALUES (1, 'Zammad test credential', 'custom_header', true, 'test-ciphertext', true)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecWrite(`INSERT INTO integration_providers
		(id, slug, name, provider_type, enabled) VALUES ('zammad-test', 'zammad-test', 'Zammad test', 'zammad', true)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecWrite(`INSERT INTO zammad_connections
		(provider_id, credential_id, base_url, default_customer) VALUES ('zammad-test', 1, 'https://zammad.example.test', 'robot@example.test')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecWrite(`INSERT INTO zammad_ticket_links
		(id, item_id, provider_id, correlation_key, sync_state, created_by)
		VALUES ('zammad-link', 1, 'zammad-test', 'ZRT-1', ?, 1)`, models.ZammadSyncPending); err != nil {
		t.Fatal(err)
	}
	return &zammadLeaseFixture{db: db, repo: NewZammadRepository(db), linkID: "zammad-link"}
}

func (f *zammadLeaseFixture) makeComplete(t *testing.T) {
	t.Helper()
	if _, err := f.db.ExecWrite(`INSERT INTO item_integration_links
		(id, item_id, integration_provider_id, external_id, external_url, title, link_type, linked_by)
		VALUES ('zammad-link-external', '1', 'zammad-test', '901', 'https://zammad.example.test/#ticket/zoom/901', 'Zammad #901', 'ticket', '1')`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecWrite(`UPDATE zammad_ticket_links
		SET item_integration_link_id = 'zammad-link-external', ticket_id = 901, ticket_number = '901',
			sync_state = ? WHERE id = ?`, models.ZammadSyncLinked, f.linkID); err != nil {
		t.Fatal(err)
	}
}

func TestZammadCorrelationKeyResolvesCurrentItem(t *testing.T) {
	f := newZammadLeaseFixture(t)

	itemID, workspaceID, err := f.repo.GetItemDestinationByCorrelationKey("zammad-test", "ZRT-1")
	if err != nil || itemID != 1 || workspaceID != 1 {
		t.Fatalf("resolve correlation key: item=%d workspace=%d err=%v", itemID, workspaceID, err)
	}
	if _, _, err := f.repo.GetItemDestinationByCorrelationKey("zammad-test", "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing correlation key must fail closed: %v", err)
	}
}

func TestZammadSyncLeaseOwnerTakeoverProtectsSnapshotReleaseAndDelete(t *testing.T) {
	f := newZammadLeaseFixture(t)
	f.makeComplete(t)
	now := time.Now().UTC()

	claimed, err := f.repo.ClaimSync(f.linkID, "owner-a", now.Add(-time.Minute))
	if err != nil || !claimed {
		t.Fatalf("claim owner A: claimed=%v err=%v", claimed, err)
	}
	claimed, err = f.repo.ClaimSync(f.linkID, "owner-b", now.Add(time.Minute))
	if err != nil || !claimed {
		t.Fatalf("take over expired lease as owner B: claimed=%v err=%v", claimed, err)
	}

	if err := f.repo.ReleaseSyncClaim(f.linkID, "owner-a"); !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("owner A must not release owner B lease: %v", err)
	}
	if err := f.repo.UpdateTicketLinkSync(f.linkID, "owner-a", 4, "closed", 7, "Support", 0, "", "", now, true, true); !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("owner A must not persist a snapshot after takeover: %v", err)
	}
	if err := f.repo.DeleteTicketLinkClaimed(f.linkID, "owner-a"); !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("owner A must not delete owner B link: %v", err)
	}
	renewed, err := f.repo.RenewSyncClaim(f.linkID, "owner-a", now.Add(2*time.Minute))
	if err != nil || renewed {
		t.Fatalf("owner A must not renew owner B lease: renewed=%v err=%v", renewed, err)
	}

	var lockOwner string
	var ticketID, statusID int
	if err := f.db.QueryRow(`SELECT sync_lock_owner, ticket_id, COALESCE(last_status_id, 0)
		FROM zammad_ticket_links WHERE id = ?`, f.linkID).Scan(&lockOwner, &ticketID, &statusID); err != nil {
		t.Fatal(err)
	}
	if lockOwner != "owner-b" || ticketID != 901 || statusID != 0 {
		t.Fatalf("stale owner changed stored link: owner=%q ticket=%d status=%d", lockOwner, ticketID, statusID)
	}
	var genericCount int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM item_integration_links WHERE id = 'zammad-link-external'`).Scan(&genericCount); err != nil {
		t.Fatal(err)
	}
	if genericCount != 1 {
		t.Fatalf("stale owner deleted generic link: count=%d", genericCount)
	}
	if err := f.repo.ReleaseSyncClaim(f.linkID, "owner-b"); err != nil {
		t.Fatalf("release current lease owner: %v", err)
	}
}

func TestZammadSQLiteTimePredicatesNormalizeNonUTCTimestamps(t *testing.T) {
	f := newZammadLeaseFixture(t)
	f.makeComplete(t)
	localNow := time.Now().In(time.FixedZone("UTC+14", 14*60*60))

	if _, err := f.db.ExecWrite(`UPDATE zammad_ticket_links SET next_attempt_at = ? WHERE id = ?`, localNow.Add(-time.Minute), f.linkID); err != nil {
		t.Fatal(err)
	}
	due, err := f.repo.ListDueTicketLinks(localNow.Add(time.Minute), 10)
	if err != nil || len(due) != 1 || due[0].ID != f.linkID {
		t.Fatalf("non-UTC retry timestamp was not compared in UTC: links=%#v err=%v", due, err)
	}

	claimed, err := f.repo.ClaimSync(f.linkID, "expired-owner", localNow.Add(-time.Minute))
	if err != nil || !claimed {
		t.Fatalf("claim expired non-UTC lease: claimed=%v err=%v", claimed, err)
	}
	claimed, err = f.repo.ClaimSync(f.linkID, "replacement-owner", localNow.Add(time.Minute))
	if err != nil || !claimed {
		t.Fatalf("take over expired non-UTC lease: claimed=%v err=%v", claimed, err)
	}

	if err := f.repo.CreateOAuthState("non-utc-state", "zammad-test", 1, 1, localNow.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	state, err := f.repo.ConsumeOAuthState("non-utc-state")
	if err != nil || state.ProviderID != "zammad-test" || state.InitiatedBy != 1 {
		t.Fatalf("non-UTC OAuth expiry was not compared in UTC: state=%#v err=%v", state, err)
	}

	if err := f.repo.UpsertOAuthToken(ZammadOAuthToken{
		ProviderID: "zammad-test", OAuthGeneration: 1, ExpiresAt: localNow.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecWrite(`UPDATE zammad_oauth_tokens
		SET refresh_lock_until = ?, refresh_claim_owner = ? WHERE provider_id = ?`,
		localNow.Add(-time.Minute), "expired-oauth-owner", "zammad-test"); err != nil {
		t.Fatal(err)
	}
	claimed, err = f.repo.ClaimOAuthRefresh("zammad-test", 1, "replacement-oauth-owner", localNow.Add(time.Minute))
	if err != nil || !claimed {
		t.Fatalf("take over expired non-UTC OAuth lease: claimed=%v err=%v", claimed, err)
	}
	if _, err := f.repo.GetOAuthTokenForRefreshClaim("zammad-test", 1, "replacement-oauth-owner"); err != nil {
		t.Fatalf("read active non-UTC OAuth lease: %v", err)
	}
	if err := database.WithTx(f.db, func(tx database.Tx) error {
		guarded, err := f.repo.GuardOAuthRefreshClaimTx(tx, "zammad-test", 1, "replacement-oauth-owner")
		if err != nil {
			return err
		}
		if !guarded {
			t.Fatal("active non-UTC OAuth lease was not guarded")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestZammadTicketCompletionRequiresCurrentLeaseOwner(t *testing.T) {
	f := newZammadLeaseFixture(t)
	now := time.Now().UTC()

	claimed, wasUncertain, err := f.repo.ClaimTicketCreation(f.linkID, "owner-a", now, now.Add(-time.Minute))
	if err != nil || !claimed {
		t.Fatalf("claim ticket creation as owner A: claimed=%v err=%v", claimed, err)
	}
	if wasUncertain {
		t.Fatal("pending ticket creation must not report uncertain state")
	}
	claimed, err = f.repo.ClaimSync(f.linkID, "owner-b", now.Add(time.Minute))
	if err != nil || !claimed {
		t.Fatalf("take over ticket creation lease as owner B: claimed=%v err=%v", claimed, err)
	}

	if err := f.repo.CompleteTicketCreation(f.linkID, "owner-a", 902, "902", "https://zammad.example.test/#ticket/zoom/902", 2, "open", 7, "Support", 0, "", 1); !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("stale owner must not complete ticket creation: %v", err)
	}
	var genericCount int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM item_integration_links WHERE id = 'zammad-link-external'`).Scan(&genericCount); err != nil {
		t.Fatal(err)
	}
	if genericCount != 0 {
		t.Fatalf("stale owner created generic item link: count=%d", genericCount)
	}

	if err := f.repo.CompleteTicketCreation(f.linkID, "owner-b", 902, "902", "https://zammad.example.test/#ticket/zoom/902", 2, "open", 7, "Support", 0, "", 1); err != nil {
		t.Fatalf("current owner completes ticket creation: %v", err)
	}
	var ticketID int
	var lockOwner any
	if err := f.db.QueryRow(`SELECT ticket_id, sync_lock_owner FROM zammad_ticket_links WHERE id = ?`, f.linkID).Scan(&ticketID, &lockOwner); err != nil {
		t.Fatal(err)
	}
	if ticketID != 902 || lockOwner != nil {
		t.Fatalf("completion did not atomically store ticket and clear lease: ticket=%d owner=%v", ticketID, lockOwner)
	}
}

func TestZammadExistingTicketCompletionRequiresCurrentLeaseOwner(t *testing.T) {
	f := newZammadLeaseFixture(t)
	now := time.Now().UTC()
	claimed, _, err := f.repo.ClaimTicketCreation(f.linkID, "owner-a", now, now.Add(-time.Minute))
	if err != nil || !claimed {
		t.Fatalf("claim existing-ticket reservation as owner A: claimed=%v err=%v", claimed, err)
	}
	claimed, err = f.repo.ClaimSync(f.linkID, "owner-b", now.Add(time.Minute))
	if err != nil || !claimed {
		t.Fatalf("take over existing-ticket reservation as owner B: claimed=%v err=%v", claimed, err)
	}
	ticket := &models.ZammadTicketLink{
		TicketID: 903, TicketNumber: "903", TicketURL: "https://zammad.example.test/#ticket/zoom/903",
		GroupID: 7, GroupName: "Support", LastStatusID: 2, LastStatusName: "open",
	}
	if err := f.repo.CompleteExistingTicketLink(f.linkID, "owner-a", f.linkID+"-external", ticket, 1); !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("stale owner must not complete existing ticket link: %v", err)
	}
	if err := f.repo.CompleteExistingTicketLink(f.linkID, "owner-b", f.linkID+"-external", ticket, 1); err != nil {
		t.Fatalf("current owner completes existing ticket link: %v", err)
	}
}

func TestZammadExistingTicketCompletionReusesPreexistingGenericLinkID(t *testing.T) {
	f := newZammadLeaseFixture(t)
	if _, err := f.db.ExecWrite(`INSERT INTO item_integration_links
		(id, item_id, integration_provider_id, external_id, external_url, title, link_type, linked_by)
		VALUES ('preexisting-generic', '1', 'zammad-test', '903', 'https://old.example.test', 'Old title', 'ticket', '1')`); err != nil {
		t.Fatal(err)
	}
	claimed, err := f.repo.ClaimSync(f.linkID, "owner", time.Now().Add(time.Minute))
	if err != nil || !claimed {
		t.Fatalf("claim existing-ticket reservation: claimed=%v err=%v", claimed, err)
	}
	ticket := &models.ZammadTicketLink{
		TicketID: 903, TicketNumber: "903", TicketURL: "https://zammad.example.test/#ticket/zoom/903",
		GroupID: 7, GroupName: "Support", LastStatusID: 2, LastStatusName: "open",
	}
	if err := f.repo.CompleteExistingTicketLink(f.linkID, "owner", f.linkID+"-external", ticket, 1); err != nil {
		t.Fatalf("complete against preexisting generic link: %v", err)
	}
	var genericID string
	if err := f.db.QueryRow(`SELECT item_integration_link_id FROM zammad_ticket_links WHERE id = ?`, f.linkID).Scan(&genericID); err != nil {
		t.Fatal(err)
	}
	if genericID != "preexisting-generic" {
		t.Fatalf("completion stored generated generic ID %q instead of existing row", genericID)
	}
}

func TestZammadTicketCreationClaimReturnsAtomicUncertainState(t *testing.T) {
	for _, tt := range []struct {
		name      string
		state     models.ZammadSyncState
		startedAt any
	}{
		{name: "explicit uncertain", state: models.ZammadSyncUncertain},
		{name: "stale in-flight create", state: models.ZammadSyncCreating, startedAt: time.Now().UTC().Add(-3 * time.Minute)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newZammadLeaseFixture(t)
			if _, err := f.db.ExecWrite(`UPDATE zammad_ticket_links SET sync_state = ?, creating_started_at = ? WHERE id = ?`, tt.state, tt.startedAt, f.linkID); err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			claimed, wasUncertain, err := f.repo.ClaimTicketCreation(f.linkID, "owner-a", now, now.Add(time.Minute))
			if err != nil || !claimed || !wasUncertain {
				t.Fatalf("unsafe retry claim must report uncertain state: claimed=%v uncertain=%v err=%v", claimed, wasUncertain, err)
			}
		})
	}
}
