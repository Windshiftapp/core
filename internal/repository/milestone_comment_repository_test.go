package repository

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"windshift/internal/database"
)

func newMilestoneCommentFixture(t *testing.T) (*MilestoneCommentRepository, database.Database, int) {
	t.Helper()
	db, err := database.NewSQLiteDB(filepath.Join(t.TempDir(), "milestone-comments.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Initialize(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecWrite(`INSERT INTO users (id, email, username, first_name, last_name, avatar_url)
		VALUES (1, 'ada@example.test', 'ada', 'Ada', 'Lovelace', '/avatars/ada.png'),
		       (2, 'bob@example.test', 'bob', '', '', NULL)`); err != nil {
		t.Fatal(err)
	}
	var milestoneID int
	if err := db.QueryRow(`INSERT INTO milestones (name, description, status, is_global)
		VALUES ('Global hub', '', 'planning', true) RETURNING id`).Scan(&milestoneID); err != nil {
		t.Fatal(err)
	}
	return NewMilestoneCommentRepository(db), db, milestoneID
}

func TestMilestoneCommentRepositoryLifecycle(t *testing.T) {
	repo, _, milestoneID := newMilestoneCommentFixture(t)

	firstID, err := repo.Create(milestoneID, 1, "First **note**")
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := repo.Create(milestoneID, 2, "Second note")
	if err != nil {
		t.Fatal(err)
	}

	first, err := repo.GetByID(firstID)
	if err != nil {
		t.Fatal(err)
	}
	if first.MilestoneID != milestoneID || first.AuthorID != 1 || first.Content != "First **note**" {
		t.Fatalf("unexpected comment: %+v", first)
	}
	if first.AuthorName != "Ada Lovelace" || first.AuthorAvatar != "/avatars/ada.png" || first.CreatedAt.IsZero() {
		t.Fatalf("expected joined author fields and timestamps, got %+v", first)
	}

	rows, err := repo.ListFeed(milestoneID, MilestoneCommentFeedQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].ID != secondID || rows[1].ID != firstID {
		t.Fatalf("expected newest-first order, got %+v", rows)
	}
	if rows[0].AuthorName != "bob" {
		t.Fatalf("expected username fallback for author without a name, got %q", rows[0].AuthorName)
	}
	count, err := repo.CountByMilestone(milestoneID)
	if err != nil || count != 2 {
		t.Fatalf("expected 2 comments, got %d (%v)", count, err)
	}

	if err := repo.UpdateContent(firstID, "Edited"); err != nil {
		t.Fatal(err)
	}
	edited, err := repo.GetByID(firstID)
	if err != nil {
		t.Fatal(err)
	}
	if edited.Content != "Edited" || edited.UpdatedAt.Before(edited.CreatedAt) {
		t.Fatalf("unexpected edited comment: %+v", edited)
	}

	if err := repo.Delete(firstID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetByID(firstID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
	if err := repo.Delete(firstID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound deleting a missing comment, got %v", err)
	}
	if err := repo.UpdateContent(firstID, "gone"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound updating a missing comment, got %v", err)
	}
}

func TestMilestoneCommentsCascadeWithMilestone(t *testing.T) {
	repo, db, milestoneID := newMilestoneCommentFixture(t)
	if _, err := repo.Create(milestoneID, 1, "Will cascade"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecWrite(`DELETE FROM milestones WHERE id = ?`, milestoneID); err != nil {
		t.Fatal(err)
	}
	count, err := repo.CountByMilestone(milestoneID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected comments to cascade with their milestone, got %d", count)
	}
}

// TestMilestoneCommentRepositoryFeed checks the keyset reads behind the
// comment feed: newest first by default, older rows before a position, and
// newer rows after one, oldest first. Comments with the same created_at are
// ordered by ID.
func TestMilestoneCommentRepositoryFeed(t *testing.T) {
	repo, db, milestoneID := newMilestoneCommentFixture(t)
	base := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	ids := make([]int, 4)
	// The middle two share a timestamp.
	for i, at := range []time.Time{base, base.Add(time.Hour), base.Add(time.Hour), base.Add(2 * time.Hour)} {
		if err := db.QueryRow(`INSERT INTO milestone_comments (milestone_id, author_id, content, created_at, updated_at)
			VALUES (?, 1, 'c', ?, ?) RETURNING id`, milestoneID, at, at).Scan(&ids[i]); err != nil {
			t.Fatal(err)
		}
	}
	feedIDs := func(query MilestoneCommentFeedQuery) []int {
		t.Helper()
		rows, err := repo.ListFeed(milestoneID, query)
		if err != nil {
			t.Fatal(err)
		}
		out := make([]int, len(rows))
		for i, row := range rows {
			out[i] = row.ID
		}
		return out
	}
	equal := func(got, want []int) bool {
		if len(got) != len(want) {
			return false
		}
		for i := range got {
			if got[i] != want[i] {
				return false
			}
		}
		return true
	}
	at := func(i int, created time.Time) *MilestoneCommentPosition {
		return &MilestoneCommentPosition{CreatedAt: created, ID: ids[i]}
	}

	if got, want := feedIDs(MilestoneCommentFeedQuery{Limit: 2}), []int{ids[3], ids[2]}; !equal(got, want) {
		t.Errorf("newest page = %v, want %v", got, want)
	}
	if got, want := feedIDs(MilestoneCommentFeedQuery{Limit: 10, Before: at(2, base.Add(time.Hour))}), []int{ids[1], ids[0]}; !equal(got, want) {
		t.Errorf("before the later tied comment = %v, want %v", got, want)
	}
	if got, want := feedIDs(MilestoneCommentFeedQuery{Limit: 10, Since: at(1, base.Add(time.Hour))}), []int{ids[2], ids[3]}; !equal(got, want) {
		t.Errorf("since the earlier tied comment = %v, want %v", got, want)
	}
	if got := feedIDs(MilestoneCommentFeedQuery{Limit: 10, Since: at(3, base.Add(2*time.Hour))}); len(got) != 0 {
		t.Errorf("since the newest comment = %v, want none", got)
	}
}
