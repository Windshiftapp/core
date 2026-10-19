package services

import (
	"errors"
	"testing"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/validation"
)

// fakeMilestoneAccess lets readers view only the listed milestones. Milestones
// in local are local to commentWorkspace; the rest are global.
type fakeMilestoneAccess struct {
	readable map[int]bool
	local    map[int]bool
}

func (f fakeMilestoneAccess) AuthorizeMilestoneRead(_, milestoneID int) (*MilestoneResult, error) {
	if !f.readable[milestoneID] {
		return nil, ErrPlanningForbidden
	}
	if f.local[milestoneID] {
		workspaceID := commentWorkspace
		return &MilestoneResult{ID: milestoneID, WorkspaceID: &workspaceID}, nil
	}
	return &MilestoneResult{ID: milestoneID, IsGlobal: true}, nil
}

// fakeCommentPermissions grants workspace permissions per user.
type fakeCommentPermissions map[int]map[string]bool

func (f fakeCommentPermissions) HasWorkspacePermission(userID, workspaceID int, permission string) (bool, error) {
	return workspaceID == commentWorkspace && f[userID][permission], nil
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
	localMilestone      = 13
	commentWorkspace    = 5
	authorUserID        = 1
	otherUserID         = 2
	moderatorUserID     = 3
)

// newTestMilestoneCommentService serves global readableMilestone and
// otherMilestone and localMilestone in commentWorkspace. The author may
// comment there; the moderator holds comment.edit_others; otherUserID holds
// neither.
func newTestMilestoneCommentService() (*MilestoneCommentService, *fakeMilestoneCommentStore) {
	store := newFakeMilestoneCommentStore()
	access := fakeMilestoneAccess{
		readable: map[int]bool{readableMilestone: true, otherMilestone: true, localMilestone: true},
		local:    map[int]bool{localMilestone: true},
	}
	permissions := fakeCommentPermissions{
		authorUserID:    {models.PermissionItemComment: true},
		moderatorUserID: {models.PermissionCommentEditOthers: true},
	}
	return NewMilestoneCommentService(store, access, permissions), store
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

func TestMilestoneCommentStoresMarkdownAsWritten(t *testing.T) {
	service, _ := newTestMilestoneCommentService()
	// Like item comments, the source is stored as written and made safe
	// when rendered.
	content := "line one<br />line two **now**"
	created, err := service.Create(authorUserID, readableMilestone, content)
	if err != nil {
		t.Fatal(err)
	}
	if created.AuthorID != authorUserID || created.MilestoneID != readableMilestone || created.Content != content {
		t.Fatalf("unexpected comment: %+v", created)
	}
	var invalid *validation.ValidationError
	if _, err := service.Create(authorUserID, readableMilestone, " \n\t"); !errors.As(err, &invalid) {
		t.Fatalf("expected a validation error for blank content, got %v", err)
	}
	if _, err := service.Update(authorUserID, readableMilestone, created.ID, "  "); !errors.As(err, &invalid) {
		t.Fatalf("expected a validation error for a blank edit, got %v", err)
	}
}

// TestMilestoneCommentLocalCreateNeedsItemComment checks that a local
// milestone applies its workspace's item.comment permission, and a global one
// lets every viewer comment.
func TestMilestoneCommentLocalCreateNeedsItemComment(t *testing.T) {
	service, store := newTestMilestoneCommentService()
	if _, err := service.Create(otherUserID, localMilestone, "hello"); !errors.Is(err, ErrPlanningForbidden) {
		t.Fatalf("expected ErrPlanningForbidden without item.comment, got %v", err)
	}
	if _, err := service.Create(authorUserID, localMilestone, "hello"); err != nil {
		t.Fatalf("expected item.comment holder to comment, got %v", err)
	}
	if _, err := service.Create(otherUserID, readableMilestone, "hello"); err != nil {
		t.Fatalf("expected any viewer to comment on a global milestone, got %v", err)
	}
	if len(store.rows) != 2 {
		t.Fatalf("expected 2 stored comments, got %d", len(store.rows))
	}
}

func TestMilestoneCommentEditRules(t *testing.T) {
	service, store := newTestMilestoneCommentService()
	global, err := service.Create(authorUserID, readableMilestone, "Global")
	if err != nil {
		t.Fatal(err)
	}
	local, err := service.Create(authorUserID, localMilestone, "Local")
	if err != nil {
		t.Fatal(err)
	}

	// Global milestones have no workspace to grant comment.edit_others in.
	for _, userID := range []int{otherUserID, moderatorUserID} {
		if _, err := service.Update(userID, readableMilestone, global.ID, "Hijacked"); !errors.Is(err, ErrPlanningForbidden) {
			t.Fatalf("user %d: expected ErrPlanningForbidden on a foreign global edit, got %v", userID, err)
		}
	}
	if _, err := service.Update(otherUserID, localMilestone, local.ID, "Hijacked"); !errors.Is(err, ErrPlanningForbidden) {
		t.Fatalf("expected ErrPlanningForbidden without comment.edit_others, got %v", err)
	}
	if err := service.Delete(otherUserID, localMilestone, local.ID); !errors.Is(err, ErrPlanningForbidden) {
		t.Fatalf("expected ErrPlanningForbidden on a foreign delete, got %v", err)
	}
	if _, err := service.Update(authorUserID, otherMilestone, global.ID, "Wrong milestone"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for a comment on another milestone, got %v", err)
	}

	updated, err := service.Update(authorUserID, readableMilestone, global.ID, "Edited")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Content != "Edited" {
		t.Fatalf("expected edited content, got %q", updated.Content)
	}
	moderated, err := service.Update(moderatorUserID, localMilestone, local.ID, "Moderated")
	if err != nil {
		t.Fatalf("expected comment.edit_others holder to edit, got %v", err)
	}
	if moderated.Content != "Moderated" {
		t.Fatalf("expected moderated content, got %q", moderated.Content)
	}
	if err := service.Delete(moderatorUserID, localMilestone, local.ID); err != nil {
		t.Fatal(err)
	}
	if err := service.Delete(authorUserID, readableMilestone, global.ID); err != nil {
		t.Fatal(err)
	}
	if len(store.rows) != 0 {
		t.Fatalf("expected the comments to be deleted, got %d rows", len(store.rows))
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
