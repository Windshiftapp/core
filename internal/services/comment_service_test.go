package services_test

import (
	"testing"
	"time"

	"windshift/internal/database"
	"windshift/internal/services"
	"windshift/internal/testutils"
	"windshift/internal/testutils/factory"
)

// commentTestEnv contains test data for comment service tests
type commentTestEnv struct {
	WorkspaceID int
	ItemID      int
	UserID      int
}

// createCommentTestDB creates a test database for comment service tests
func createCommentTestDB(t *testing.T) database.Database {
	t.Helper()
	tdb := testutils.CreateTestDB(t, true)
	t.Cleanup(func() { tdb.Close() })
	return tdb.GetDatabase()
}

// setupCommentTestEnv creates test data for comment service tests using the factory
func setupCommentTestEnv(t *testing.T, db database.Database) commentTestEnv {
	t.Helper()
	f := factory.NewTestFactory(db)
	env, err := f.CreateFullTestEnv()
	if err != nil {
		t.Fatalf("Failed to create test env: %v", err)
	}
	return commentTestEnv{
		WorkspaceID: env.WorkspaceID,
		ItemID:      env.ItemID,
		UserID:      env.UserID,
	}
}

func TestCommentService_Create(t *testing.T) {
	db := createCommentTestDB(t)

	service := services.NewCommentService(db)
	env := setupCommentTestEnv(t, db)

	t.Run("Success", func(t *testing.T) {
		params := services.CreateCommentParams{
			ItemID:      env.ItemID,
			AuthorID:    env.UserID,
			Content:     "This is a test comment",
			IsPrivate:   false,
			ActorUserID: env.UserID,
		}

		result, err := service.Create(params)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result.CommentID == 0 {
			t.Error("Expected non-zero comment ID")
		}

		// Verify comment was created
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM comments WHERE id = ?", result.CommentID).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to verify comment creation: %v", err)
		}
		if count != 1 {
			t.Errorf("Expected 1 comment, got %d", count)
		}
	})

	t.Run("SanitizesContent", func(t *testing.T) {
		params := services.CreateCommentParams{
			ItemID:      env.ItemID,
			AuthorID:    env.UserID,
			Content:     "<script>alert('xss')</script>Safe content",
			IsPrivate:   false,
			ActorUserID: env.UserID,
		}

		result, err := service.Create(params)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Verify content was sanitized (HTML tags stripped but inner text preserved)
		var content string
		err = db.QueryRow("SELECT content FROM comments WHERE id = ?", result.CommentID).Scan(&content)
		if err != nil {
			t.Fatalf("Failed to fetch comment: %v", err)
		}

		if content == params.Content {
			t.Error("Expected content to be sanitized, but it was not")
		}
		// The sanitizer strips HTML tags but keeps inner text
		if content != "alert('xss')Safe content" {
			t.Errorf("Expected sanitized content without HTML tags, got '%s'", content)
		}
	})

	t.Run("PrivateComment", func(t *testing.T) {
		params := services.CreateCommentParams{
			ItemID:      env.ItemID,
			AuthorID:    env.UserID,
			Content:     "This is a private note",
			IsPrivate:   true,
			ActorUserID: env.UserID,
		}

		result, err := service.Create(params)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Verify is_private flag
		var isPrivate bool
		err = db.QueryRow("SELECT is_private FROM comments WHERE id = ?", result.CommentID).Scan(&isPrivate)
		if err != nil {
			t.Fatalf("Failed to fetch comment: %v", err)
		}
		if !isPrivate {
			t.Error("Expected comment to be private")
		}
	})

	t.Run("ItemNotFound", func(t *testing.T) {
		params := services.CreateCommentParams{
			ItemID:      99999,
			AuthorID:    env.UserID,
			Content:     "Comment on non-existent item",
			IsPrivate:   false,
			ActorUserID: env.UserID,
		}

		_, err := service.Create(params)
		if err == nil {
			t.Error("Expected error for non-existent item")
		}
	})
}

func TestCommentService_Get(t *testing.T) {
	db := createCommentTestDB(t)

	service := services.NewCommentService(db)
	env := setupCommentTestEnv(t, db)

	// Create a comment to retrieve
	params := services.CreateCommentParams{
		ItemID:      env.ItemID,
		AuthorID:    env.UserID,
		Content:     "Comment to retrieve",
		IsPrivate:   false,
		ActorUserID: env.UserID,
	}
	created, err := service.Create(params)
	if err != nil {
		t.Fatalf("Failed to create test comment: %v", err)
	}

	t.Run("Success", func(t *testing.T) {
		comment, err := service.Get(int(created.CommentID))
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if comment.ID != int(created.CommentID) {
			t.Errorf("Expected comment ID %d, got %d", created.CommentID, comment.ID)
		}
		if comment.Content != "Comment to retrieve" {
			t.Errorf("Expected content 'Comment to retrieve', got '%s'", comment.Content)
		}
		if comment.AuthorName == "" {
			t.Error("Expected author name to be populated")
		}
		if comment.WorkspaceID != env.WorkspaceID {
			t.Errorf("Expected workspace ID %d, got %d", env.WorkspaceID, comment.WorkspaceID)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		_, err := service.Get(99999)
		if err == nil {
			t.Error("Expected error for non-existent comment")
		}
	})
}

func TestCommentService_Update(t *testing.T) {
	db := createCommentTestDB(t)

	service := services.NewCommentService(db)
	env := setupCommentTestEnv(t, db)

	// Create a comment to update
	params := services.CreateCommentParams{
		ItemID:      env.ItemID,
		AuthorID:    env.UserID,
		Content:     "Original content",
		IsPrivate:   false,
		ActorUserID: env.UserID,
	}
	created, err := service.Create(params)
	if err != nil {
		t.Fatalf("Failed to create test comment: %v", err)
	}

	t.Run("Success", func(t *testing.T) {
		comment, err := service.Update(int(created.CommentID), "Updated content", env.UserID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if comment.Content != "Updated content" {
			t.Errorf("Expected content 'Updated content', got '%s'", comment.Content)
		}
	})

	t.Run("SanitizesContent", func(t *testing.T) {
		comment, err := service.Update(int(created.CommentID), "<b>Bold</b> text", env.UserID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if comment.Content != "Bold text" {
			t.Errorf("Expected sanitized content 'Bold text', got '%s'", comment.Content)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		_, err := service.Update(99999, "New content", env.UserID)
		if err == nil {
			t.Error("Expected error for non-existent comment")
		}
	})
}

func TestCommentService_Delete(t *testing.T) {
	db := createCommentTestDB(t)

	service := services.NewCommentService(db)
	env := setupCommentTestEnv(t, db)

	t.Run("Success", func(t *testing.T) {
		// Create a comment to delete
		params := services.CreateCommentParams{
			ItemID:      env.ItemID,
			AuthorID:    env.UserID,
			Content:     "Comment to delete",
			IsPrivate:   false,
			ActorUserID: env.UserID,
		}
		created, err := service.Create(params)
		if err != nil {
			t.Fatalf("Failed to create test comment: %v", err)
		}

		err = service.Delete(int(created.CommentID))
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Verify comment was deleted
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM comments WHERE id = ?", created.CommentID).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to verify deletion: %v", err)
		}
		if count != 0 {
			t.Errorf("Expected 0 comments, got %d", count)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		err := service.Delete(99999)
		if err == nil {
			t.Error("Expected error for non-existent comment")
		}
	})
}

func TestCommentService_GetByItemID(t *testing.T) {
	db := createCommentTestDB(t)

	service := services.NewCommentService(db)
	env := setupCommentTestEnv(t, db)

	// Create multiple comments
	for i := 1; i <= 3; i++ {
		params := services.CreateCommentParams{
			ItemID:      env.ItemID,
			AuthorID:    env.UserID,
			Content:     "Comment " + string(rune('0'+i)),
			IsPrivate:   false,
			ActorUserID: env.UserID,
		}
		_, err := service.Create(params)
		if err != nil {
			t.Fatalf("Failed to create test comment: %v", err)
		}
	}

	t.Run("ReturnsAllComments", func(t *testing.T) {
		comments, err := service.GetByItemID(env.ItemID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(comments) != 3 {
			t.Errorf("Expected 3 comments, got %d", len(comments))
		}
	})

	t.Run("EmptyForNoComments", func(t *testing.T) {
		// Create a new item without comments
		result, _ := db.Exec(`
			INSERT INTO items (workspace_id, workspace_item_number, title, is_task, frac_index, path, created_at, updated_at)
			VALUES (?, 2, 'Item without comments', 0, 'a1', '/2/', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, env.WorkspaceID)
		newItemID, _ := result.LastInsertId()

		comments, err := service.GetByItemID(int(newItemID))
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(comments) != 0 {
			t.Errorf("Expected 0 comments, got %d", len(comments))
		}
	})
}

func TestCommentService_GetWorkspaceIDForComment(t *testing.T) {
	db := createCommentTestDB(t)

	service := services.NewCommentService(db)
	env := setupCommentTestEnv(t, db)

	// Create a comment
	params := services.CreateCommentParams{
		ItemID:      env.ItemID,
		AuthorID:    env.UserID,
		Content:     "Test comment",
		IsPrivate:   false,
		ActorUserID: env.UserID,
	}
	created, err := service.Create(params)
	if err != nil {
		t.Fatalf("Failed to create test comment: %v", err)
	}

	t.Run("Success", func(t *testing.T) {
		workspaceID, err := service.GetWorkspaceIDForComment(int(created.CommentID))
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if workspaceID != env.WorkspaceID {
			t.Errorf("Expected workspace ID %d, got %d", env.WorkspaceID, workspaceID)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		_, err := service.GetWorkspaceIDForComment(99999)
		if err == nil {
			t.Error("Expected error for non-existent comment")
		}
	})
}

func TestCommentService_GetAuthorID(t *testing.T) {
	db := createCommentTestDB(t)

	service := services.NewCommentService(db)
	env := setupCommentTestEnv(t, db)

	// Create a comment
	params := services.CreateCommentParams{
		ItemID:      env.ItemID,
		AuthorID:    env.UserID,
		Content:     "Test comment",
		IsPrivate:   false,
		ActorUserID: env.UserID,
	}
	created, err := service.Create(params)
	if err != nil {
		t.Fatalf("Failed to create test comment: %v", err)
	}

	t.Run("Success", func(t *testing.T) {
		authorID, err := service.GetAuthorID(int(created.CommentID))
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if authorID != env.UserID {
			t.Errorf("Expected author ID %d, got %d", env.UserID, authorID)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		_, err := service.GetAuthorID(99999)
		if err == nil {
			t.Error("Expected error for non-existent comment")
		}
	})
}

// mockEmailReplyHandler implements services.EmailReplyHandler for testing.
type mockEmailReplyHandler struct {
	calls []services.HandleCommentParams
	err   error
}

func (m *mockEmailReplyHandler) HandleCommentCreated(p services.HandleCommentParams) error {
	m.calls = append(m.calls, p)
	return m.err
}

func TestCommentService_CreateWithPortalCustomerID(t *testing.T) {
	db := createCommentTestDB(t)
	service := services.NewCommentService(db)
	env := setupCommentTestEnv(t, db)

	// Create a portal customer
	res, err := db.Exec(`
		INSERT INTO portal_customers (name, email) VALUES ('Portal User', 'portal@example.com')
	`)
	if err != nil {
		t.Fatalf("Failed to create portal customer: %v", err)
	}
	pcID64, _ := res.LastInsertId()
	pcID := int(pcID64)

	t.Run("PortalCustomerWithoutLinkedUser", func(t *testing.T) {
		params := services.CreateCommentParams{
			ItemID:           env.ItemID,
			AuthorID:         0,
			PortalCustomerID: &pcID,
			Content:          "Comment from portal customer",
			ActorUserID:      0,
		}

		result, err := service.Create(params)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if result.CommentID == 0 {
			t.Error("Expected non-zero comment ID")
		}

		// Verify portal_customer_id is set
		var portalCustomerID int
		err = db.QueryRow("SELECT portal_customer_id FROM comments WHERE id = ?", result.CommentID).Scan(&portalCustomerID)
		if err != nil {
			t.Fatalf("Failed to query comment: %v", err)
		}
		if portalCustomerID != pcID {
			t.Errorf("Expected portal_customer_id %d, got %d", pcID, portalCustomerID)
		}
	})

	t.Run("PortalCustomerWithLinkedUser", func(t *testing.T) {
		params := services.CreateCommentParams{
			ItemID:           env.ItemID,
			AuthorID:         env.UserID,
			PortalCustomerID: &pcID,
			Content:          "Comment from linked portal customer",
			ActorUserID:      env.UserID,
		}

		result, err := service.Create(params)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// When AuthorID is set, the internal user path is used (author_id column)
		var authorID int
		err = db.QueryRow("SELECT author_id FROM comments WHERE id = ?", result.CommentID).Scan(&authorID)
		if err != nil {
			t.Fatalf("Failed to query comment: %v", err)
		}
		if authorID != env.UserID {
			t.Errorf("Expected author_id %d, got %d", env.UserID, authorID)
		}
	})
}

func TestCommentService_CreateCallsEmailReplyService(t *testing.T) {
	db := createCommentTestDB(t)
	service := services.NewCommentService(db)
	env := setupCommentTestEnv(t, db)

	mock := &mockEmailReplyHandler{}
	service.SetEmailReplyService(mock)

	t.Run("InternalUserComment", func(t *testing.T) {
		mock.calls = nil
		params := services.CreateCommentParams{
			ItemID:      env.ItemID,
			AuthorID:    env.UserID,
			Content:     "Internal reply",
			ActorUserID: env.UserID,
		}

		_, err := service.Create(params)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(mock.calls) != 1 {
			t.Fatalf("Expected 1 call to HandleCommentCreated, got %d", len(mock.calls))
		}
		call := mock.calls[0]
		if call.AuthorID != env.UserID {
			t.Errorf("Expected AuthorID %d, got %d", env.UserID, call.AuthorID)
		}
		if call.ItemID != env.ItemID {
			t.Errorf("Expected ItemID %d, got %d", env.ItemID, call.ItemID)
		}
		if call.PortalCustomerID != nil {
			t.Error("Expected PortalCustomerID to be nil for internal user")
		}
	})

	t.Run("PortalCustomerComment", func(t *testing.T) {
		mock.calls = nil

		// Create portal customer for this test
		res, err := db.Exec(`
			INSERT INTO portal_customers (name, email) VALUES ('Another Customer', 'another@example.com')
		`)
		if err != nil {
			t.Fatalf("Failed to create portal customer: %v", err)
		}
		pcID64, _ := res.LastInsertId()
		pcID := int(pcID64)

		params := services.CreateCommentParams{
			ItemID:           env.ItemID,
			AuthorID:         0,
			PortalCustomerID: &pcID,
			Content:          "Customer comment",
			ActorUserID:      0,
		}

		_, err = service.Create(params)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(mock.calls) != 1 {
			t.Fatalf("Expected 1 call to HandleCommentCreated, got %d", len(mock.calls))
		}
		call := mock.calls[0]
		if call.PortalCustomerID == nil {
			t.Fatal("Expected PortalCustomerID to be set")
		}
		if *call.PortalCustomerID != pcID {
			t.Errorf("Expected PortalCustomerID %d, got %d", pcID, *call.PortalCustomerID)
		}
	})
}

// Remove unused import warning
var _ = time.Now
