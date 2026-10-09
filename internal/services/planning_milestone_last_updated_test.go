package services

import (
	"strconv"
	"testing"
	"time"
)

// TestListMilestonesLastUpdated checks that each listed milestone's
// LastUpdatedAt is the latest of its own updated_at, its history, comments,
// page links, member items, comments on them, and items added to it. Item
// sources count only for items in the viewer's workspaces.
func TestListMilestonesLastUpdated(t *testing.T) {
	planning, db := newMilestoneReleaseTestService(t)
	t.Cleanup(func() { _ = db.Close() })
	base := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	at := func(hours int) time.Time { return base.Add(time.Duration(hours) * time.Hour) }
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecWrite(query, args...); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	exec(`INSERT INTO users (id, email, username, first_name, last_name) VALUES (1, 'ada@example.test', 'ada', 'Ada', 'Lovelace')`)
	hub := activityInsert(t, db, `INSERT INTO workspaces (name, key) VALUES ('Hub', 'HUB')`)
	sec := activityInsert(t, db, `INSERT INTO workspaces (name, key) VALUES ('Secret', 'SEC')`)
	var statusID int
	if err := db.QueryRow(`SELECT id FROM statuses ORDER BY id LIMIT 1`).Scan(&statusID); err != nil {
		t.Fatal(err)
	}
	number := 0
	item := func(workspaceID int, updated time.Time) int {
		number++
		return activityInsert(t, db, `INSERT INTO items (workspace_id, workspace_item_number, title, description, frac_index, status_id, creator_id, created_at, updated_at)
			VALUES (?, ?, ?, '', ?, ?, 1, ?, ?)`, workspaceID, number, "Item "+strconv.Itoa(number), "a"+strconv.Itoa(number), statusID, base, updated)
	}
	memberSince := func(itemID, milestoneID int, added time.Time) {
		exec(`INSERT INTO item_milestones (item_id, milestone_id, created_at) VALUES (?, ?, ?)`, itemID, milestoneID, added)
	}
	member := func(itemID, milestoneID int) { memberSince(itemID, milestoneID, base) }
	milestone := func(name string) int {
		id := insertGlobalMilestone(t, db, name)
		exec(`UPDATE milestones SET created_at = ?, updated_at = ? WHERE id = ?`, base, base, id)
		return id
	}

	untouched := milestone("Untouched")
	history := milestone("History")
	exec(`INSERT INTO milestone_history (milestone_id, user_id, field_name, old_value, new_value, changed_at) VALUES (?, 1, 'status', 'planning', 'in-progress', ?)`, history, at(1))
	comment := milestone("Comment")
	exec(`INSERT INTO milestone_comments (milestone_id, author_id, content, created_at, updated_at) VALUES (?, 1, 'hi', ?, ?)`, comment, at(1), at(2))
	page := milestone("Page link")
	var linkTypeID int
	if err := db.QueryRow(`SELECT id FROM link_types WHERE builtin_key = 'page'`).Scan(&linkTypeID); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO item_links (link_type_id, source_type, source_id, target_type, target_id, created_by, created_at) VALUES (?, 'milestone', ?, 'page', 99, 1, ?)`, linkTypeID, page, at(3))
	itemChange := milestone("Item change")
	member(item(hub, at(4)), itemChange)
	itemComment := milestone("Item comment")
	commented := item(hub, base)
	member(commented, itemComment)
	exec(`INSERT INTO comments (item_id, author_id, content, created_at, updated_at) VALUES (?, 1, 'x', ?, ?)`, commented, at(5), at(5))
	// An item added later than its last edit moves the milestone to when it
	// was added.
	membership := milestone("Membership")
	memberSince(item(hub, base), membership, at(6))
	// An item that left the milestone does not count: only its membership
	// history remains, and the list does not scan item history.
	removed := milestone("Removed")
	exec(`INSERT INTO item_history (item_id, user_id, field_name, old_value, new_value, changed_at) VALUES (?, 1, 'milestones', ?, '', ?)`,
		item(hub, base), strconv.Itoa(removed), at(20))
	// SEC items count only for viewers who can access SEC.
	hidden := milestone("Hidden")
	member(item(sec, at(7)), hidden)
	// SQLite CURRENT_TIMESTAMP text compares correctly with Go-written times.
	textStamp := milestone("Text stamp")
	exec(`INSERT INTO milestone_history (milestone_id, user_id, field_name, old_value, new_value, changed_at) VALUES (?, 1, 'status', 'planning', 'completed', '2026-03-01 17:30:00')`, textStamp)

	list := func(viewerWorkspaces []int) map[int]time.Time {
		t.Helper()
		rows, _, err := planning.ListMilestones(MilestoneListParams{
			Limit: 100, IncludeGlobal: true, IncludeLastUpdated: true, ViewerWorkspaceIDs: viewerWorkspaces,
		})
		if err != nil {
			t.Fatal(err)
		}
		out := map[int]time.Time{}
		for _, row := range rows {
			if row.LastUpdatedAt == nil {
				t.Fatalf("milestone %d has no LastUpdatedAt", row.ID)
			}
			out[row.ID] = *row.LastUpdatedAt
		}
		return out
	}

	got := list([]int{hub})
	want := map[int]time.Time{
		untouched: base, history: at(1), comment: at(2), page: at(3), itemChange: at(4),
		itemComment: at(5), membership: at(6), removed: base, hidden: base, textStamp: base.Add(8*time.Hour + 30*time.Minute),
	}
	for id, expected := range want {
		if !got[id].Equal(expected) {
			t.Errorf("milestone %d: LastUpdatedAt = %v, want %v", id, got[id], expected)
		}
	}

	if got := list([]int{hub, sec}); !got[hidden].Equal(at(7)) {
		t.Errorf("viewer with SEC access: hidden LastUpdatedAt = %v, want %v", got[hidden], at(7))
	}
	if got := list(nil); !got[itemChange].Equal(base) || !got[page].Equal(at(3)) {
		t.Errorf("viewer with no workspaces: item change %v (want %v), page link %v (want %v)", got[itemChange], base, got[page], at(3))
	}

	// Lists without IncludeLastUpdated leave it unset.
	rows, _, err := planning.ListMilestones(MilestoneListParams{Limit: 100, IncludeGlobal: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(want) || rows[0].LastUpdatedAt != nil {
		t.Errorf("plain list: %d rows, first LastUpdatedAt %v; want %d rows and nil", len(rows), rows[0].LastUpdatedAt, len(want))
	}
}

// TestReorderMilestonesKeepsUpdatedAt checks that reordering changes only
// positions, so the renumbered milestones keep their updated_at and
// last_updated_at, while a real edit still moves both.
func TestReorderMilestonesKeepsUpdatedAt(t *testing.T) {
	planning, db := newMilestoneReleaseTestService(t)
	t.Cleanup(func() { _ = db.Close() })
	base := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)

	ids := make([]int, 3)
	for i, name := range []string{"First", "Second", "Third"} {
		ids[i] = insertGlobalMilestone(t, db, name)
		stamp := base.Add(time.Duration(i) * time.Hour)
		if _, err := db.ExecWrite(`UPDATE milestones SET position = ?, created_at = ?, updated_at = ? WHERE id = ?`,
			(i+1)*milestonePositionStep, stamp, stamp, ids[i]); err != nil {
			t.Fatal(err)
		}
	}

	list := func() map[int]MilestoneResult {
		t.Helper()
		rows, _, err := planning.ListMilestones(MilestoneListParams{Limit: 100, IncludeGlobal: true, IncludeLastUpdated: true})
		if err != nil {
			t.Fatal(err)
		}
		out := map[int]MilestoneResult{}
		for _, row := range rows {
			out[row.ID] = row
		}
		return out
	}
	before := list()

	reordered := []int{ids[2], ids[0], ids[1]}
	if err := planning.ReorderMilestones(MilestoneScope{IsGlobal: true}, reordered); err != nil {
		t.Fatal(err)
	}
	after := list()
	for i, id := range reordered {
		if want := (i + 1) * milestonePositionStep; after[id].Position != want {
			t.Errorf("milestone %d: position = %d, want %d", id, after[id].Position, want)
		}
		if !after[id].UpdatedAt.Equal(before[id].UpdatedAt) {
			t.Errorf("milestone %d: updated_at moved on reorder: %v -> %v", id, before[id].UpdatedAt, after[id].UpdatedAt)
		}
		if after[id].LastUpdatedAt == nil || !after[id].LastUpdatedAt.Equal(*before[id].LastUpdatedAt) {
			t.Errorf("milestone %d: last_updated_at moved on reorder: %v -> %v", id, before[id].LastUpdatedAt, after[id].LastUpdatedAt)
		}
	}

	// A real edit still bumps updated_at (and so last_updated_at).
	edited := before[ids[0]]
	if _, err := planning.UpdateMilestone(UpdateMilestoneParams{
		ID: edited.ID, Name: "First, renamed", Description: edited.Description, Status: edited.Status,
	}); err != nil {
		t.Fatal(err)
	}
	if got := list()[ids[0]]; !got.UpdatedAt.After(edited.UpdatedAt) || !got.LastUpdatedAt.After(*edited.LastUpdatedAt) {
		t.Errorf("edit did not bump updated_at/last_updated_at: %v / %v, before %v", got.UpdatedAt, got.LastUpdatedAt, edited.UpdatedAt)
	}
}
