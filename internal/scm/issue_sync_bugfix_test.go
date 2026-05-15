package scm

import (
	"context"
	"errors"
	"testing"

	"windshift/internal/database"
	"windshift/internal/models"
)

// newSCMSchema sets up an in-memory SQLite DB with the minimum tables this
// package's regression tests need. It mirrors the production schema for
// workspace_scm_connections, workspace_repositories, issue_sync_configs,
// issue_sync_items, items, and item_milestones — keeping the columns/types
// these tests touch and dropping FKs we don't exercise.
func newSCMSchema(t *testing.T) database.Database {
	t.Helper()
	// Use a per-test shared-cache URI so the separate read/write connection
	// pools created by NewSQLiteDB both see the same in-memory database.
	// A bare ":memory:" gives each connection its own DB and write/read drift.
	dsn := "file:scmtest_" + t.Name() + "?mode=memory&cache=shared"
	db, err := database.NewSQLiteDB(dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tables := []string{
		`CREATE TABLE workspace_scm_connections (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workspace_id INTEGER NOT NULL,
			scm_provider_id INTEGER NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1
		)`,
		`CREATE TABLE workspace_repositories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workspace_scm_connection_id INTEGER NOT NULL,
			repository_external_id TEXT NOT NULL,
			repository_name TEXT NOT NULL
		)`,
		`CREATE TABLE issue_sync_configs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workspace_repository_id INTEGER NOT NULL UNIQUE,
			sync_enabled INTEGER NOT NULL DEFAULT 0,
			status_mapping TEXT NOT NULL DEFAULT '{}',
			reverse_status_mapping TEXT NOT NULL DEFAULT '{}',
			label_sync_mode TEXT NOT NULL DEFAULT 'none',
			label_mappings TEXT NOT NULL DEFAULT '[]',
			filter_labels TEXT NOT NULL DEFAULT '[]',
			assignee_mappings TEXT NOT NULL DEFAULT '{}',
			milestone_mappings TEXT NOT NULL DEFAULT '{}',
			default_item_type_id INTEGER,
			default_priority_id INTEGER,
			sync_comments INTEGER NOT NULL DEFAULT 0,
			last_full_sync_at DATETIME,
			last_sync_error TEXT,
			created_by INTEGER,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workspace_id INTEGER NOT NULL,
			title TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE item_milestones (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			item_id INTEGER NOT NULL,
			milestone_id INTEGER NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(item_id, milestone_id)
		)`,
	}
	for _, ddl := range tables {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatalf("create schema: %v", err)
		}
	}
	return db
}

// insertRepo inserts a connection + repo for a workspace and returns the
// workspace_repository_id.
func insertRepo(t *testing.T, db database.Database, workspaceID int, repoName string) int {
	t.Helper()
	res, err := db.Exec(
		"INSERT INTO workspace_scm_connections (workspace_id, scm_provider_id) VALUES (?, 1)",
		workspaceID,
	)
	if err != nil {
		t.Fatalf("insert connection: %v", err)
	}
	connID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("conn id: %v", err)
	}
	res, err = db.Exec(
		"INSERT INTO workspace_repositories (workspace_scm_connection_id, repository_external_id, repository_name) VALUES (?, ?, ?)",
		connID, "ext-"+repoName, repoName,
	)
	if err != nil {
		t.Fatalf("insert repo: %v", err)
	}
	repoID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("repo id: %v", err)
	}
	return int(repoID)
}

// TestUpdateItemFromIssue_MilestoneReplacementSQL validates the SQL contract
// introduced by Fix #1 of the issue-sync bug hunt: on update, the milestone
// must be replaced via item_milestones (DELETE then conditional INSERT) instead
// of being passed through ItemRepository.UpdateFields — milestone_id is not a
// column on items and previously caused every update to fail with
// "unknown item column: milestone_id".
//
// The body intentionally exercises raw SQL rather than the full
// updateItemFromIssue path because that path needs a provider plus many tables;
// the goal here is to lock in the new SQL pattern.
func TestUpdateItemFromIssue_MilestoneReplacementSQL(t *testing.T) {
	db := newSCMSchema(t)
	ctx := context.Background()

	// Seed an item with an existing milestone attachment.
	if _, err := db.Exec("INSERT INTO items (id, workspace_id, title) VALUES (1, 1, 'orig')"); err != nil {
		t.Fatalf("seed item: %v", err)
	}
	if _, err := db.Exec("INSERT INTO item_milestones (item_id, milestone_id) VALUES (1, 10)"); err != nil {
		t.Fatalf("seed milestone: %v", err)
	}

	// Mirror updateItemFromIssue's milestone replacement: DELETE then optional INSERT.
	apply := func(itemID int, milestoneID *int) {
		t.Helper()
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer func() { _ = tx.Rollback() }()
		if _, err := tx.Exec("DELETE FROM item_milestones WHERE item_id = ?", itemID); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if milestoneID != nil {
			if _, err := tx.Exec(
				"INSERT INTO item_milestones (item_id, milestone_id) VALUES (?, ?)",
				itemID, *milestoneID,
			); err != nil {
				t.Fatalf("insert: %v", err)
			}
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit: %v", err)
		}
	}

	count := func(itemID int) int {
		t.Helper()
		var n int
		if err := db.QueryRow("SELECT COUNT(*) FROM item_milestones WHERE item_id = ?", itemID).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		return n
	}
	currentMilestone := func(itemID int) int {
		t.Helper()
		var n int
		if err := db.QueryRow("SELECT milestone_id FROM item_milestones WHERE item_id = ?", itemID).Scan(&n); err != nil {
			t.Fatalf("scan milestone: %v", err)
		}
		return n
	}

	// 1. Replace existing milestone with a new one.
	newMilestone := 20
	apply(1, &newMilestone)
	if got := count(1); got != 1 {
		t.Fatalf("after replace: count = %d, want 1", got)
	}
	if got := currentMilestone(1); got != 20 {
		t.Fatalf("after replace: milestone_id = %d, want 20", got)
	}

	// 2. Clear milestone (GitHub removed it).
	apply(1, nil)
	if got := count(1); got != 0 {
		t.Fatalf("after clear: count = %d, want 0", got)
	}

	// 3. Re-add from cleared state.
	again := 30
	apply(1, &again)
	if got := count(1); got != 1 {
		t.Fatalf("after re-add: count = %d, want 1", got)
	}
}

// TestGetGitHubLabels_RejectsCrossWorkspaceRepo validates Fix #3 of the bug
// hunt: GetGitHubLabels must refuse a workspace_repository_id that does not
// belong to the caller's workspace, returning ErrRepositoryNotInWorkspace. The
// workspace gate runs before any provider lookup so the test doesn't need an
// SCM provider configured.
func TestGetGitHubLabels_RejectsCrossWorkspaceRepo(t *testing.T) {
	db := newSCMSchema(t)
	repoID := insertRepo(t, db, 1, "owner/repo")
	svc := &IssueSyncService{db: db}

	_, err := svc.GetGitHubLabels(context.Background(), 2 /* wrong workspace */, repoID)
	if !errors.Is(err, ErrRepositoryNotInWorkspace) {
		t.Fatalf("err = %v, want ErrRepositoryNotInWorkspace", err)
	}
}

// TestGetGitHubMilestones_RejectsCrossWorkspaceRepo — symmetric to the labels
// case for Fix #3.
func TestGetGitHubMilestones_RejectsCrossWorkspaceRepo(t *testing.T) {
	db := newSCMSchema(t)
	repoID := insertRepo(t, db, 1, "owner/repo")
	svc := &IssueSyncService{db: db}

	_, err := svc.GetGitHubMilestones(context.Background(), 2 /* wrong workspace */, repoID)
	if !errors.Is(err, ErrRepositoryNotInWorkspace) {
		t.Fatalf("err = %v, want ErrRepositoryNotInWorkspace", err)
	}
}

// TestCreateSyncConfig_RejectsSecondConfigInWorkspace validates Fix #5: even
// though the schema only enforces uniqueness on workspace_repository_id, the
// workspace-scoped API assumes a single config per workspace. A second create
// for a different repo in the same workspace must be rejected.
func TestCreateSyncConfig_RejectsSecondConfigInWorkspace(t *testing.T) {
	db := newSCMSchema(t)
	repoA := insertRepo(t, db, 1, "owner/repo-a")
	repoB := insertRepo(t, db, 1, "owner/repo-b")
	svc := &IssueSyncService{db: db}

	ctx := context.Background()
	if _, err := svc.CreateSyncConfig(ctx, 1, models.IssueSyncConfigRequest{
		WorkspaceRepositoryID: repoA,
	}); err != nil {
		t.Fatalf("first create: %v", err)
	}

	_, err := svc.CreateSyncConfig(ctx, 1, models.IssueSyncConfigRequest{
		WorkspaceRepositoryID: repoB,
	})
	if !errors.Is(err, ErrSyncConfigExists) {
		t.Fatalf("second create: err = %v, want ErrSyncConfigExists", err)
	}

	// Sanity: a config in a *different* workspace is still allowed.
	repoOther := insertRepo(t, db, 2, "owner/repo-c")
	if _, err := svc.CreateSyncConfig(ctx, 1, models.IssueSyncConfigRequest{
		WorkspaceRepositoryID: repoOther,
	}); err != nil {
		t.Fatalf("cross-workspace create: %v", err)
	}
}
