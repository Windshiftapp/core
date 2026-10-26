package services

import (
	"strconv"
	"testing"
	"time"

	"windshift/internal/repository"
)

// TestMilestoneLastActivityFollowsActivityWrites checks that each writer of a
// milestone Activity source moves the milestone's last_activity_at, and that
// milestones without activity keep it unset.
func TestMilestoneLastActivityFollowsActivityWrites(t *testing.T) {
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
	workspace := activityInsert(t, db, `INSERT INTO workspaces (name, key) VALUES ('Hub', 'HUB')`)
	var statusID int
	if err := db.QueryRow(`SELECT id FROM statuses ORDER BY id LIMIT 1`).Scan(&statusID); err != nil {
		t.Fatal(err)
	}
	number := 0
	item := func(milestoneIDs ...int) int {
		number++
		id := activityInsert(t, db, `INSERT INTO items (workspace_id, workspace_item_number, title, description, frac_index, status_id, creator_id, created_at, updated_at)
			VALUES (?, ?, ?, '', ?, ?, 1, ?, ?)`, workspace, number, "Item "+strconv.Itoa(number), "a"+strconv.Itoa(number), statusID, base, base)
		for _, milestoneID := range milestoneIDs {
			exec(`INSERT INTO item_milestones (item_id, milestone_id, created_at) VALUES (?, ?, ?)`, id, milestoneID, base)
		}
		return id
	}
	userID := 1
	items := repository.NewItemRepository(db)
	record := func(entry repository.HistoryEntry) {
		t.Helper()
		entry.UserID = userID
		if err := items.RecordHistory(db, entry); err != nil {
			t.Fatal(err)
		}
	}

	untouched := insertGlobalMilestone(t, db, "Untouched")
	history := insertGlobalMilestone(t, db, "History")
	if err := repository.RecordMilestoneHistory(db, repository.MilestoneHistoryEntry{
		MilestoneID: history, UserID: &userID, FieldName: repository.MilestoneHistoryStatus,
		OldValue: "planning", NewValue: "in-progress", ChangedAt: at(1),
	}); err != nil {
		t.Fatal(err)
	}
	comment := insertGlobalMilestone(t, db, "Comment")
	before := time.Now().UTC().Add(-time.Second)
	if _, err := repository.NewMilestoneCommentRepository(db).Create(comment, userID, "hi"); err != nil {
		t.Fatal(err)
	}
	// Moving an item from one milestone to another counts for both.
	left := insertGlobalMilestone(t, db, "Left")
	joined := insertGlobalMilestone(t, db, "Joined")
	record(repository.HistoryEntry{ItemID: item(joined), FieldName: "milestones",
		OldValue: strconv.Itoa(left), NewValue: strconv.Itoa(joined), ChangedAt: at(2)})
	status := insertGlobalMilestone(t, db, "Status")
	record(repository.HistoryEntry{ItemID: item(status), FieldName: "status_id",
		OldValue: "1", NewValue: "2", ChangedAt: at(3)})
	// A creation status row has no old status and is not an Activity entry.
	created := insertGlobalMilestone(t, db, "Created")
	record(repository.HistoryEntry{ItemID: item(created), FieldName: "status_id",
		OldValueNull: true, NewValue: "1", ChangedAt: at(4)})
	itemComment := insertGlobalMilestone(t, db, "Item comment")
	commentAt := at(5)
	if _, err := NewCommentService(db).Create(CreateCommentParams{
		ItemID: item(itemComment), AuthorID: userID, Content: "x", CreatedAt: &commentAt, SuppressNotifications: true,
	}); err != nil {
		t.Fatal(err)
	}

	rows, _, err := planning.ListMilestones(MilestoneListParams{Limit: 100, IncludeGlobal: true})
	if err != nil {
		t.Fatal(err)
	}
	got := map[int]*time.Time{}
	for _, row := range rows {
		got[row.ID] = row.LastActivityAt
	}
	for id, want := range map[int]time.Time{history: at(1), left: at(2), joined: at(2), status: at(3), itemComment: at(5)} {
		if got[id] == nil || !got[id].Equal(want) {
			t.Errorf("milestone %d: LastActivityAt = %v, want %v", id, got[id], want)
		}
	}
	if got[comment] == nil || got[comment].Before(before) {
		t.Errorf("comment milestone: LastActivityAt = %v, want after %v", got[comment], before)
	}
	for _, id := range []int{untouched, created} {
		if got[id] != nil {
			t.Errorf("milestone %d: LastActivityAt = %v, want nil", id, *got[id])
		}
	}
}
