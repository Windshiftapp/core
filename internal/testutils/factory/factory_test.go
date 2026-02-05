//go:build test

package factory

import (
	"testing"

	"windshift/internal/testutils"
)

func TestFactory_CreateUser(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	factory := NewTestFactory(tdb.GetDatabase())

	t.Run("WithDefaults", func(t *testing.T) {
		userID, err := factory.CreateUser(nil)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if userID == 0 {
			t.Error("Expected non-zero user ID")
		}

		// Verify user was created
		var count int
		err = tdb.QueryRow("SELECT COUNT(*) FROM users WHERE id = ?", userID).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to verify user: %v", err)
		}
		if count != 1 {
			t.Errorf("Expected 1 user, got %d", count)
		}
	})

	t.Run("WithCustomOpts", func(t *testing.T) {
		userID, err := factory.CreateUser(&CreateUserOpts{
			Email:     "custom@example.com",
			Username:  "customuser",
			FirstName: "Custom",
			LastName:  "User",
			IsActive:  true,
		})
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		var email, username, firstName, lastName string
		err = tdb.QueryRow(
			"SELECT email, username, first_name, last_name FROM users WHERE id = ?",
			userID,
		).Scan(&email, &username, &firstName, &lastName)
		if err != nil {
			t.Fatalf("Failed to fetch user: %v", err)
		}

		if email != "custom@example.com" {
			t.Errorf("Expected email 'custom@example.com', got '%s'", email)
		}
		if username != "customuser" {
			t.Errorf("Expected username 'customuser', got '%s'", username)
		}
	})
}

func TestFactory_CreateWorkspace(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	factory := NewTestFactory(tdb.GetDatabase())

	// Create a user first (required for workspace)
	userID, err := factory.CreateUser(nil)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	t.Run("WithDefaults", func(t *testing.T) {
		workspaceID, err := factory.CreateWorkspace(CreateWorkspaceOpts{
			CreatorID: userID,
		})
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if workspaceID == 0 {
			t.Error("Expected non-zero workspace ID")
		}

		// Verify workspace was created
		var name string
		err = tdb.QueryRow("SELECT name FROM workspaces WHERE id = ?", workspaceID).Scan(&name)
		if err != nil {
			t.Fatalf("Failed to verify workspace: %v", err)
		}
	})

	t.Run("RequiresCreatorID", func(t *testing.T) {
		_, err := factory.CreateWorkspace(CreateWorkspaceOpts{})
		if err == nil {
			t.Error("Expected error when CreatorID not provided")
		}
	})

	t.Run("WithCustomOpts", func(t *testing.T) {
		workspaceID, err := factory.CreateWorkspace(CreateWorkspaceOpts{
			Name:        "Custom Workspace",
			Key:         "CUST",
			Description: "Custom description",
			CreatorID:   userID,
		})
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		var name, key, description string
		err = tdb.QueryRow(
			"SELECT name, key, description FROM workspaces WHERE id = ?",
			workspaceID,
		).Scan(&name, &key, &description)
		if err != nil {
			t.Fatalf("Failed to fetch workspace: %v", err)
		}

		if name != "Custom Workspace" {
			t.Errorf("Expected name 'Custom Workspace', got '%s'", name)
		}
		if key != "CUST" {
			t.Errorf("Expected key 'CUST', got '%s'", key)
		}
	})
}

func TestFactory_CreateItem(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	factory := NewTestFactory(tdb.GetDatabase())

	// Create user and workspace first
	userID, workspaceID, err := factory.CreateUserAndWorkspace()
	if err != nil {
		t.Fatalf("Failed to create user and workspace: %v", err)
	}

	t.Run("WithDefaults", func(t *testing.T) {
		itemID, err := factory.CreateItem(CreateItemOpts{
			WorkspaceID: workspaceID,
		})
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if itemID == 0 {
			t.Error("Expected non-zero item ID")
		}

		// Verify item was created
		var title string
		err = tdb.QueryRow("SELECT title FROM items WHERE id = ?", itemID).Scan(&title)
		if err != nil {
			t.Fatalf("Failed to verify item: %v", err)
		}
	})

	t.Run("RequiresWorkspaceID", func(t *testing.T) {
		_, err := factory.CreateItem(CreateItemOpts{})
		if err == nil {
			t.Error("Expected error when WorkspaceID not provided")
		}
	})

	t.Run("WithCustomOpts", func(t *testing.T) {
		itemID, err := factory.CreateItem(CreateItemOpts{
			WorkspaceID: workspaceID,
			Title:       "Custom Item",
			Description: "Custom description",
			CreatorID:   &userID,
		})
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		var title, description string
		err = tdb.QueryRow(
			"SELECT title, description FROM items WHERE id = ?",
			itemID,
		).Scan(&title, &description)
		if err != nil {
			t.Fatalf("Failed to fetch item: %v", err)
		}

		if title != "Custom Item" {
			t.Errorf("Expected title 'Custom Item', got '%s'", title)
		}
	})
}

func TestFactory_CreateComment(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	factory := NewTestFactory(tdb.GetDatabase())

	// Create full test env first
	env, err := factory.CreateFullTestEnv()
	if err != nil {
		t.Fatalf("Failed to create test env: %v", err)
	}

	t.Run("WithDefaults", func(t *testing.T) {
		commentID, err := factory.CreateComment(CreateCommentOpts{
			ItemID:   env.ItemID,
			AuthorID: env.UserID,
		})
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if commentID == 0 {
			t.Error("Expected non-zero comment ID")
		}

		// Verify comment was created
		var content string
		err = tdb.QueryRow("SELECT content FROM comments WHERE id = ?", commentID).Scan(&content)
		if err != nil {
			t.Fatalf("Failed to verify comment: %v", err)
		}
		if content != "Test comment" {
			t.Errorf("Expected default content 'Test comment', got '%s'", content)
		}
	})

	t.Run("RequiresItemID", func(t *testing.T) {
		_, err := factory.CreateComment(CreateCommentOpts{
			AuthorID: env.UserID,
		})
		if err == nil {
			t.Error("Expected error when ItemID not provided")
		}
	})

	t.Run("RequiresAuthorID", func(t *testing.T) {
		_, err := factory.CreateComment(CreateCommentOpts{
			ItemID: env.ItemID,
		})
		if err == nil {
			t.Error("Expected error when AuthorID not provided")
		}
	})

	t.Run("WithCustomContent", func(t *testing.T) {
		commentID, err := factory.CreateComment(CreateCommentOpts{
			ItemID:   env.ItemID,
			AuthorID: env.UserID,
			Content:  "Custom comment content",
		})
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		var content string
		err = tdb.QueryRow("SELECT content FROM comments WHERE id = ?", commentID).Scan(&content)
		if err != nil {
			t.Fatalf("Failed to fetch comment: %v", err)
		}
		if content != "Custom comment content" {
			t.Errorf("Expected custom content, got '%s'", content)
		}
	})
}

func TestFactory_CreateUserAndWorkspace(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	factory := NewTestFactory(tdb.GetDatabase())

	userID, workspaceID, err := factory.CreateUserAndWorkspace()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if userID == 0 {
		t.Error("Expected non-zero user ID")
	}
	if workspaceID == 0 {
		t.Error("Expected non-zero workspace ID")
	}

	// Verify both exist
	var userCount, workspaceCount int
	tdb.QueryRow("SELECT COUNT(*) FROM users WHERE id = ?", userID).Scan(&userCount)
	tdb.QueryRow("SELECT COUNT(*) FROM workspaces WHERE id = ?", workspaceID).Scan(&workspaceCount)

	if userCount != 1 {
		t.Errorf("Expected 1 user, got %d", userCount)
	}
	if workspaceCount != 1 {
		t.Errorf("Expected 1 workspace, got %d", workspaceCount)
	}
}

func TestFactory_CreateFullTestEnv(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	factory := NewTestFactory(tdb.GetDatabase())

	env, err := factory.CreateFullTestEnv()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if env.UserID == 0 {
		t.Error("Expected non-zero user ID")
	}
	if env.WorkspaceID == 0 {
		t.Error("Expected non-zero workspace ID")
	}
	if env.ItemID == 0 {
		t.Error("Expected non-zero item ID")
	}

	// Verify all exist
	var userCount, workspaceCount, itemCount int
	tdb.QueryRow("SELECT COUNT(*) FROM users WHERE id = ?", env.UserID).Scan(&userCount)
	tdb.QueryRow("SELECT COUNT(*) FROM workspaces WHERE id = ?", env.WorkspaceID).Scan(&workspaceCount)
	tdb.QueryRow("SELECT COUNT(*) FROM items WHERE id = ?", env.ItemID).Scan(&itemCount)

	if userCount != 1 {
		t.Errorf("Expected 1 user, got %d", userCount)
	}
	if workspaceCount != 1 {
		t.Errorf("Expected 1 workspace, got %d", workspaceCount)
	}
	if itemCount != 1 {
		t.Errorf("Expected 1 item, got %d", itemCount)
	}
}
