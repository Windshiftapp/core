//go:build test

package services

import (
	"testing"
	"time"

	"windshift/internal/testutils"
)

// CommentServiceTestData contains test data for comment service tests
type CommentServiceTestData struct {
	WorkspaceID int
	WorkspaceKey string
	ItemID      int
	UserID      int
	CommentID   int64
}

// setupCommentServiceTestData creates test data for comment service tests
func setupCommentServiceTestData(t *testing.T, tdb *testutils.TestDB) *CommentServiceTestData {
	now := time.Now()

	// Create workspace
	result, err := tdb.DB.Exec(`
		INSERT INTO workspaces (name, key, description, created_at, updated_at)
		VALUES ('Test Workspace', 'CMNT', 'Test workspace for comments', ?, ?)
	`, now, now)
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	workspaceID64, _ := result.LastInsertId()
	workspaceID := int(workspaceID64)

	// Create user
	userResult, err := tdb.DB.Exec(`
		INSERT INTO users (username, email, first_name, last_name, password_hash, is_active, created_at, updated_at)
		VALUES ('testuser', 'test@example.com', 'Test', 'User', 'hash', 1, ?, ?)
	`, now, now)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	userID64, _ := userResult.LastInsertId()
	userID := int(userID64)

	// Create test item
	itemResult, err := tdb.DB.Exec(`
		INSERT INTO items (workspace_id, workspace_item_number, title, description, status, is_task,
		                   frac_index, path, created_at, updated_at)
		VALUES (?, 1, 'Test Item for Comments', 'Test Description', 'open', 0, 'a0', '/1/', ?, ?)
	`, workspaceID, now, now)
	if err != nil {
		t.Fatalf("Failed to create test item: %v", err)
	}
	itemID64, _ := itemResult.LastInsertId()
	itemID := int(itemID64)

	return &CommentServiceTestData{
		WorkspaceID:  workspaceID,
		WorkspaceKey: "CMNT",
		ItemID:       itemID,
		UserID:       userID,
	}
}

func TestCommentService_Create(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	service := NewCommentService(tdb.GetDatabase())
	testData := setupCommentServiceTestData(t, tdb)

	t.Run("Success", func(t *testing.T) {
		params := CreateCommentParams{
			ItemID:      testData.ItemID,
			AuthorID:    testData.UserID,
			Content:     "This is a test comment",
			IsPrivate:   false,
			ActorUserID: testData.UserID,
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
		err = tdb.DB.QueryRow("SELECT COUNT(*) FROM comments WHERE id = ?", result.CommentID).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to verify comment creation: %v", err)
		}
		if count != 1 {
			t.Errorf("Expected 1 comment, got %d", count)
		}
	})

	t.Run("SanitizesContent", func(t *testing.T) {
		params := CreateCommentParams{
			ItemID:      testData.ItemID,
			AuthorID:    testData.UserID,
			Content:     "<script>alert('xss')</script>Safe content",
			IsPrivate:   false,
			ActorUserID: testData.UserID,
		}

		result, err := service.Create(params)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Verify content was sanitized
		var content string
		err = tdb.DB.QueryRow("SELECT content FROM comments WHERE id = ?", result.CommentID).Scan(&content)
		if err != nil {
			t.Fatalf("Failed to fetch comment: %v", err)
		}

		if content == params.Content {
			t.Error("Expected content to be sanitized, but it was not")
		}
		if content != "Safe content" {
			t.Errorf("Expected sanitized content 'Safe content', got '%s'", content)
		}
	})

	t.Run("PrivateComment", func(t *testing.T) {
		params := CreateCommentParams{
			ItemID:      testData.ItemID,
			AuthorID:    testData.UserID,
			Content:     "This is a private note",
			IsPrivate:   true,
			ActorUserID: testData.UserID,
		}

		result, err := service.Create(params)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Verify is_private flag
		var isPrivate bool
		err = tdb.DB.QueryRow("SELECT is_private FROM comments WHERE id = ?", result.CommentID).Scan(&isPrivate)
		if err != nil {
			t.Fatalf("Failed to fetch comment: %v", err)
		}
		if !isPrivate {
			t.Error("Expected comment to be private")
		}
	})

	t.Run("ItemNotFound", func(t *testing.T) {
		params := CreateCommentParams{
			ItemID:      99999,
			AuthorID:    testData.UserID,
			Content:     "Comment on non-existent item",
			IsPrivate:   false,
			ActorUserID: testData.UserID,
		}

		_, err := service.Create(params)
		if err == nil {
			t.Error("Expected error for non-existent item")
		}
	})
}

func TestCommentService_Get(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	service := NewCommentService(tdb.GetDatabase())
	testData := setupCommentServiceTestData(t, tdb)

	// Create a comment to retrieve
	params := CreateCommentParams{
		ItemID:      testData.ItemID,
		AuthorID:    testData.UserID,
		Content:     "Comment to retrieve",
		IsPrivate:   false,
		ActorUserID: testData.UserID,
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
		if comment.WorkspaceID != testData.WorkspaceID {
			t.Errorf("Expected workspace ID %d, got %d", testData.WorkspaceID, comment.WorkspaceID)
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
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	service := NewCommentService(tdb.GetDatabase())
	testData := setupCommentServiceTestData(t, tdb)

	// Create a comment to update
	params := CreateCommentParams{
		ItemID:      testData.ItemID,
		AuthorID:    testData.UserID,
		Content:     "Original content",
		IsPrivate:   false,
		ActorUserID: testData.UserID,
	}
	created, err := service.Create(params)
	if err != nil {
		t.Fatalf("Failed to create test comment: %v", err)
	}

	t.Run("Success", func(t *testing.T) {
		comment, err := service.Update(int(created.CommentID), "Updated content", testData.UserID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if comment.Content != "Updated content" {
			t.Errorf("Expected content 'Updated content', got '%s'", comment.Content)
		}
	})

	t.Run("SanitizesContent", func(t *testing.T) {
		comment, err := service.Update(int(created.CommentID), "<b>Bold</b> text", testData.UserID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if comment.Content != "Bold text" {
			t.Errorf("Expected sanitized content 'Bold text', got '%s'", comment.Content)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		_, err := service.Update(99999, "New content", testData.UserID)
		if err == nil {
			t.Error("Expected error for non-existent comment")
		}
	})
}

func TestCommentService_Delete(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	service := NewCommentService(tdb.GetDatabase())
	testData := setupCommentServiceTestData(t, tdb)

	t.Run("Success", func(t *testing.T) {
		// Create a comment to delete
		params := CreateCommentParams{
			ItemID:      testData.ItemID,
			AuthorID:    testData.UserID,
			Content:     "Comment to delete",
			IsPrivate:   false,
			ActorUserID: testData.UserID,
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
		err = tdb.DB.QueryRow("SELECT COUNT(*) FROM comments WHERE id = ?", created.CommentID).Scan(&count)
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
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	service := NewCommentService(tdb.GetDatabase())
	testData := setupCommentServiceTestData(t, tdb)

	// Create multiple comments
	for i := 1; i <= 3; i++ {
		params := CreateCommentParams{
			ItemID:      testData.ItemID,
			AuthorID:    testData.UserID,
			Content:     "Comment " + string(rune('0'+i)),
			IsPrivate:   false,
			ActorUserID: testData.UserID,
		}
		_, err := service.Create(params)
		if err != nil {
			t.Fatalf("Failed to create test comment: %v", err)
		}
	}

	t.Run("ReturnsAllComments", func(t *testing.T) {
		comments, err := service.GetByItemID(testData.ItemID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(comments) != 3 {
			t.Errorf("Expected 3 comments, got %d", len(comments))
		}
	})

	t.Run("EmptyForNoComments", func(t *testing.T) {
		// Create a new item without comments
		now := time.Now()
		result, _ := tdb.DB.Exec(`
			INSERT INTO items (workspace_id, workspace_item_number, title, status, is_task, frac_index, path, created_at, updated_at)
			VALUES (?, 2, 'Item without comments', 'open', 0, 'a1', '/2/', ?, ?)
		`, testData.WorkspaceID, now, now)
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
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	service := NewCommentService(tdb.GetDatabase())
	testData := setupCommentServiceTestData(t, tdb)

	// Create a comment
	params := CreateCommentParams{
		ItemID:      testData.ItemID,
		AuthorID:    testData.UserID,
		Content:     "Test comment",
		IsPrivate:   false,
		ActorUserID: testData.UserID,
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

		if workspaceID != testData.WorkspaceID {
			t.Errorf("Expected workspace ID %d, got %d", testData.WorkspaceID, workspaceID)
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
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	service := NewCommentService(tdb.GetDatabase())
	testData := setupCommentServiceTestData(t, tdb)

	// Create a comment
	params := CreateCommentParams{
		ItemID:      testData.ItemID,
		AuthorID:    testData.UserID,
		Content:     "Test comment",
		IsPrivate:   false,
		ActorUserID: testData.UserID,
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

		if authorID != testData.UserID {
			t.Errorf("Expected author ID %d, got %d", testData.UserID, authorID)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		_, err := service.GetAuthorID(99999)
		if err == nil {
			t.Error("Expected error for non-existent comment")
		}
	})
}
