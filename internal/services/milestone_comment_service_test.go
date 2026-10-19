package services

import (
	"errors"
	"strings"
	"testing"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
)

// fakeMilestoneAccess lets readers view only the listed milestones.
type fakeMilestoneAccess struct {
	readable map[int]bool
}

func (f fakeMilestoneAccess) AuthorizeMilestoneRead(_, milestoneID int) (*MilestoneResult, error) {
	if !f.readable[milestoneID] {
		return nil, ErrPlanningForbidden
	}
	return &MilestoneResult{ID: milestoneID}, nil
}

type fakeMilestoneCommentStore struct {
	nextID   int
	rows     map[int]models.MilestoneComment
	lastFeed repository.MilestoneCommentFeedQuery
}

func newFakeMilestoneCommentStore() *fakeMilestoneCommentStore {
	return &fakeMilestoneCommentStore{nextID: 1, rows: map[int]models.MilestoneComment{}}
}

// ListFeed records the query and returns the milestone's comments newest
// first, up to the limit. Cursor filtering is the repository's job.
func (f *fakeMilestoneCommentStore) ListFeed(milestoneID int, feed repository.MilestoneCommentFeedQuery) ([]models.MilestoneComment, error) {
	f.lastFeed = feed
	result := []models.MilestoneComment{}
	for id := f.nextID - 1; id >= 1 && len(result) < feed.Limit; id-- {
		if row, ok := f.rows[id]; ok && row.MilestoneID == milestoneID {
			result = append(result, row)
		}
	}
	return result, nil
}

func (f *fakeMilestoneCommentStore) CountByMilestone(milestoneID int) (int, error) {
	count := 0
	for _, row := range f.rows {
		if row.MilestoneID == milestoneID {
			count++
		}
	}
	return count, nil
}

func (f *fakeMilestoneCommentStore) GetByID(id int) (*models.MilestoneComment, error) {
	row, ok := f.rows[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return &row, nil
}

func (f *fakeMilestoneCommentStore) Create(milestoneID, authorID int, content string) (int, error) {
	id := f.nextID
	f.nextID++
	f.rows[id] = models.MilestoneComment{ID: id, MilestoneID: milestoneID, AuthorID: authorID, Content: content}
	return id, nil
}

func (f *fakeMilestoneCommentStore) UpdateContent(id int, content string) error {
	row, ok := f.rows[id]
	if !ok {
		return repository.ErrNotFound
	}
	row.Content = content
	f.rows[id] = row
	return nil
}

func (f *fakeMilestoneCommentStore) Delete(id int) error {
	if _, ok := f.rows[id]; !ok {
		return repository.ErrNotFound
	}
	delete(f.rows, id)
	return nil
}

const (
	readableMilestone   = 10
	otherMilestone      = 11
	unreadableMilestone = 12
	authorUserID        = 1
	otherUserID         = 2
)

func newTestMilestoneCommentService() (*MilestoneCommentService, *fakeMilestoneCommentStore) {
	store := newFakeMilestoneCommentStore()
	access := fakeMilestoneAccess{readable: map[int]bool{readableMilestone: true, otherMilestone: true}}
	return NewMilestoneCommentService(store, access), store
}

func TestMilestoneCommentCreateRequiresMilestoneRead(t *testing.T) {
	service, store := newTestMilestoneCommentService()
	if _, err := service.Create(authorUserID, unreadableMilestone, "hello"); !errors.Is(err, ErrPlanningForbidden) {
		t.Fatalf("expected ErrPlanningForbidden, got %v", err)
	}
	if _, err := service.GetFeed(authorUserID, unreadableMilestone, CommentFeedOptions{Limit: 10}); !errors.Is(err, ErrPlanningForbidden) {
		t.Fatalf("expected ErrPlanningForbidden listing, got %v", err)
	}
	if len(store.rows) != 0 {
		t.Fatalf("expected no stored comments, got %d", len(store.rows))
	}
}

func TestMilestoneCommentCreateSanitizesAndRejectsEmptyContent(t *testing.T) {
	service, _ := newTestMilestoneCommentService()
	created, err := service.Create(authorUserID, readableMilestone, "Plan <script>alert(1)</script>**now**")
	if err != nil {
		t.Fatal(err)
	}
	if created.AuthorID != authorUserID || created.MilestoneID != readableMilestone {
		t.Fatalf("unexpected comment: %+v", created)
	}
	if created.Content == "" || strings.Contains(created.Content, "<script>") {
		t.Fatalf("expected sanitized Markdown, got %q", created.Content)
	}
	for _, empty := range []string{"  <b></b> ", "<br />\n<br>"} {
		if _, err := service.Create(authorUserID, readableMilestone, empty); err == nil {
			t.Fatalf("expected empty content %q to be rejected", empty)
		} else if _, ok := AsPlanningValidationError(err); !ok {
			t.Fatalf("expected a validation error for %q, got %v", empty, err)
		}
	}
}

func TestMilestoneCommentKeepsEditorHardBreaks(t *testing.T) {
	service, _ := newTestMilestoneCommentService()
	created, err := service.Create(authorUserID, readableMilestone, "line one<br />line two")
	if err != nil {
		t.Fatal(err)
	}
	if created.Content != "line one<br />line two" {
		t.Fatalf("expected the editor's hard break to survive, got %q", created.Content)
	}
}

func TestMilestoneCommentOnlyAuthorMayEditOrDelete(t *testing.T) {
	service, store := newTestMilestoneCommentService()
	created, err := service.Create(authorUserID, readableMilestone, "Original")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.Update(otherUserID, readableMilestone, created.ID, "Hijacked"); !errors.Is(err, ErrMilestoneCommentNotAuthor) {
		t.Fatalf("expected ErrMilestoneCommentNotAuthor on foreign edit, got %v", err)
	}
	if err := service.Delete(otherUserID, readableMilestone, created.ID); !errors.Is(err, ErrMilestoneCommentNotAuthor) {
		t.Fatalf("expected ErrMilestoneCommentNotAuthor on foreign delete, got %v", err)
	}
	if _, err := service.Update(authorUserID, otherMilestone, created.ID, "Wrong milestone"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for a comment on another milestone, got %v", err)
	}

	updated, err := service.Update(authorUserID, readableMilestone, created.ID, "Edited")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Content != "Edited" {
		t.Fatalf("expected edited content, got %q", updated.Content)
	}
	if err := service.Delete(authorUserID, readableMilestone, created.ID); err != nil {
		t.Fatal(err)
	}
	if len(store.rows) != 0 {
		t.Fatalf("expected the comment to be deleted, got %d rows", len(store.rows))
	}
}

// TestMilestoneCommentFeedPages checks that the feed reads one row past the
// page to set HasMore, applies the item feed's default and maximum page size,
// and passes a cursor through as a repository position.
func TestMilestoneCommentFeedPages(t *testing.T) {
	service, store := newTestMilestoneCommentService()
	for range 3 {
		if _, err := service.Create(authorUserID, readableMilestone, "note"); err != nil {
			t.Fatal(err)
		}
	}

	page, err := service.GetFeed(authorUserID, readableMilestone, CommentFeedOptions{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Comments) != 2 || !page.HasMore || page.Comments[0].ID != 3 {
		t.Fatalf("first page = %+v, want comments 3 and 2 with more", page)
	}
	if store.lastFeed.Limit != 3 {
		t.Errorf("repository limit = %d, want page size + 1", store.lastFeed.Limit)
	}

	page, err = service.GetFeed(authorUserID, readableMilestone, CommentFeedOptions{Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Comments) != 3 || page.HasMore {
		t.Fatalf("whole thread = %+v, want 3 comments and no more", page)
	}

	if _, err := service.GetFeed(authorUserID, readableMilestone, CommentFeedOptions{}); err != nil {
		t.Fatal(err)
	}
	if store.lastFeed.Limit != DefaultCommentFeedLimit+1 {
		t.Errorf("default repository limit = %d, want %d", store.lastFeed.Limit, DefaultCommentFeedLimit+1)
	}
	if _, err := service.GetFeed(authorUserID, readableMilestone, CommentFeedOptions{Limit: 10_000}); err != nil {
		t.Fatal(err)
	}
	if store.lastFeed.Limit != MaxCommentFeedLimit+1 {
		t.Errorf("capped repository limit = %d, want %d", store.lastFeed.Limit, MaxCommentFeedLimit+1)
	}

	cursor := CommentFeedCursor{CreatedAt: time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC), ID: 2}
	if _, err := service.GetFeed(authorUserID, readableMilestone, CommentFeedOptions{Limit: 2, Before: &cursor}); err != nil {
		t.Fatal(err)
	}
	if got := store.lastFeed.Before; got == nil || got.ID != 2 || !got.CreatedAt.Equal(cursor.CreatedAt) || store.lastFeed.Since != nil {
		t.Errorf("before cursor passed as %+v / since %+v", got, store.lastFeed.Since)
	}
	if _, err := service.GetFeed(authorUserID, readableMilestone, CommentFeedOptions{Limit: 2, Since: &cursor}); err != nil {
		t.Fatal(err)
	}
	if got := store.lastFeed.Since; got == nil || got.ID != 2 || store.lastFeed.Before != nil {
		t.Errorf("since cursor passed as %+v / before %+v", got, store.lastFeed.Before)
	}

	if total, err := service.CountFeed(readableMilestone); err != nil || total != 3 {
		t.Errorf("CountFeed = %d, %v; want 3", total, err)
	}
}
