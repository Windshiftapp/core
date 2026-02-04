package services

import (
	"path/filepath"
	"testing"

	"windshift/internal/database"
)

// mentionTestEnv contains test data for mention service tests
type mentionTestEnv struct {
	WorkspaceID int
	ItemID      int
	UserID1     int
	UserID2     int
	UserID3     int
	CommentID   int
}

// createMentionTestDB creates a test database for mention service tests
func createMentionTestDB(t *testing.T) database.Database {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "mention_test.db")
	db, err := database.NewSQLiteDB(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	if err := db.Initialize(); err != nil {
		t.Fatalf("failed to initialize test database: %v", err)
	}
	return db
}

// setupMentionTestEnv creates test data for mention service tests
func setupMentionTestEnv(t *testing.T, db database.Database) mentionTestEnv {
	t.Helper()

	// Create workspace
	workspaceResult, err := db.Exec(`
		INSERT INTO workspaces (name, key, description, active, is_personal, created_at, updated_at)
		VALUES ('Mention Test Workspace', 'MNT', 'Test workspace', 1, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	workspaceID64, _ := workspaceResult.LastInsertId()
	workspaceID := int(workspaceID64)

	// Create test users
	userResult1, err := db.Exec(`
		INSERT INTO users (username, first_name, last_name, email, is_active, created_at, updated_at)
		VALUES ('johndoe', 'John', 'Doe', 'john@example.com', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	if err != nil {
		t.Fatalf("Failed to create user 1: %v", err)
	}
	userID1, _ := userResult1.LastInsertId()

	userResult2, err := db.Exec(`
		INSERT INTO users (username, first_name, last_name, email, is_active, created_at, updated_at)
		VALUES ('janedoe', 'Jane', 'Doe', 'jane@example.com', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	if err != nil {
		t.Fatalf("Failed to create user 2: %v", err)
	}
	userID2, _ := userResult2.LastInsertId()

	userResult3, err := db.Exec(`
		INSERT INTO users (username, first_name, last_name, email, is_active, created_at, updated_at)
		VALUES ('inactive_user', 'Inactive', 'User', 'inactive@example.com', 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	if err != nil {
		t.Fatalf("Failed to create user 3: %v", err)
	}
	userID3, _ := userResult3.LastInsertId()

	// Get a status ID for the item
	var statusID int
	err = db.QueryRow("SELECT id FROM statuses LIMIT 1").Scan(&statusID)
	if err != nil {
		t.Fatalf("Failed to get status ID: %v", err)
	}

	// Create test item
	itemResult, err := db.Exec(`
		INSERT INTO items (workspace_id, title, description, is_task, status_id, workspace_item_number, created_at, updated_at)
		VALUES (?, 'Test Item', 'Item description', 1, ?, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, workspaceID, statusID)
	if err != nil {
		t.Fatalf("Failed to create item: %v", err)
	}
	itemID, _ := itemResult.LastInsertId()

	// Create test comment
	commentResult, err := db.Exec(`
		INSERT INTO comments (item_id, author_id, content, created_at, updated_at)
		VALUES (?, ?, 'Test comment', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, itemID, userID1)
	if err != nil {
		t.Fatalf("Failed to create comment: %v", err)
	}
	commentID, _ := commentResult.LastInsertId()

	return mentionTestEnv{
		WorkspaceID: workspaceID,
		ItemID:      int(itemID),
		UserID1:     int(userID1),
		UserID2:     int(userID2),
		UserID3:     int(userID3),
		CommentID:   int(commentID),
	}
}

func TestMentionService_ExtractMentionIdentifiers(t *testing.T) {
	db := createMentionTestDB(t)
	defer db.Close()

	service := NewMentionService(db, nil)

	t.Run("ExtractUsername", func(t *testing.T) {
		content := "Hello @johndoe, how are you?"
		identifiers := service.ExtractMentionIdentifiers(content)

		if len(identifiers) != 1 {
			t.Fatalf("Expected 1 identifier, got %d", len(identifiers))
		}
		if identifiers[0] != "johndoe" {
			t.Errorf("Expected 'johndoe', got '%s'", identifiers[0])
		}
	})

	t.Run("ExtractQuotedDisplayName", func(t *testing.T) {
		content := "Hello @\"John Doe\", how are you?"
		identifiers := service.ExtractMentionIdentifiers(content)

		if len(identifiers) != 1 {
			t.Fatalf("Expected 1 identifier, got %d", len(identifiers))
		}
		if identifiers[0] != "John Doe" {
			t.Errorf("Expected 'John Doe', got '%s'", identifiers[0])
		}
	})

	t.Run("ExtractMultipleMentions", func(t *testing.T) {
		content := "Hello @johndoe and @janedoe!"
		identifiers := service.ExtractMentionIdentifiers(content)

		if len(identifiers) != 2 {
			t.Fatalf("Expected 2 identifiers, got %d", len(identifiers))
		}
	})

	t.Run("DeduplicateMentions", func(t *testing.T) {
		content := "Hello @johndoe, @johndoe again!"
		identifiers := service.ExtractMentionIdentifiers(content)

		if len(identifiers) != 1 {
			t.Fatalf("Expected 1 identifier (deduplicated), got %d", len(identifiers))
		}
	})

	t.Run("MixedFormats", func(t *testing.T) {
		content := "Hello @johndoe and @\"Jane Doe\"!"
		identifiers := service.ExtractMentionIdentifiers(content)

		if len(identifiers) != 2 {
			t.Fatalf("Expected 2 identifiers, got %d", len(identifiers))
		}
	})

	t.Run("NoMentions", func(t *testing.T) {
		content := "Hello, how are you?"
		identifiers := service.ExtractMentionIdentifiers(content)

		if len(identifiers) != 0 {
			t.Errorf("Expected 0 identifiers, got %d", len(identifiers))
		}
	})

	t.Run("EmptyContent", func(t *testing.T) {
		content := ""
		identifiers := service.ExtractMentionIdentifiers(content)

		if len(identifiers) != 0 {
			t.Errorf("Expected 0 identifiers, got %d", len(identifiers))
		}
	})

	t.Run("UsernameWithDots", func(t *testing.T) {
		content := "Hello @john.doe.jr!"
		identifiers := service.ExtractMentionIdentifiers(content)

		if len(identifiers) != 1 {
			t.Fatalf("Expected 1 identifier, got %d", len(identifiers))
		}
		if identifiers[0] != "john.doe.jr" {
			t.Errorf("Expected 'john.doe.jr', got '%s'", identifiers[0])
		}
	})

	t.Run("UsernameWithUnderscores", func(t *testing.T) {
		content := "Hello @john_doe!"
		identifiers := service.ExtractMentionIdentifiers(content)

		if len(identifiers) != 1 {
			t.Fatalf("Expected 1 identifier, got %d", len(identifiers))
		}
		if identifiers[0] != "john_doe" {
			t.Errorf("Expected 'john_doe', got '%s'", identifiers[0])
		}
	})

	t.Run("UsernameWithDashes", func(t *testing.T) {
		content := "Hello @john-doe!"
		identifiers := service.ExtractMentionIdentifiers(content)

		if len(identifiers) != 1 {
			t.Fatalf("Expected 1 identifier, got %d", len(identifiers))
		}
		if identifiers[0] != "john-doe" {
			t.Errorf("Expected 'john-doe', got '%s'", identifiers[0])
		}
	})
}

func TestMentionService_ProcessMentions(t *testing.T) {
	db := createMentionTestDB(t)
	defer db.Close()

	service := NewMentionService(db, nil)
	env := setupMentionTestEnv(t, db)

	t.Run("CreatesMention", func(t *testing.T) {
		params := ProcessMentionsParams{
			SourceType:  "comment",
			SourceID:    env.CommentID,
			Content:     "Hello @johndoe!",
			ItemID:      env.ItemID,
			WorkspaceID: env.WorkspaceID,
			ActorUserID: env.UserID2, // Jane mentions John
		}

		err := service.ProcessMentions(params)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Verify mention was created
		var count int
		err = db.QueryRow(`
			SELECT COUNT(*) FROM mentions
			WHERE source_type = ? AND source_id = ? AND mentioned_user_id = ?
		`, "comment", env.CommentID, env.UserID1).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to query mentions: %v", err)
		}
		if count != 1 {
			t.Errorf("Expected 1 mention, got %d", count)
		}
	})

	t.Run("SkipsSelfMention", func(t *testing.T) {
		// Clean up previous test
		db.Exec("DELETE FROM mentions")

		params := ProcessMentionsParams{
			SourceType:  "comment",
			SourceID:    env.CommentID + 100, // Different source
			Content:     "Hello @johndoe!",
			ItemID:      env.ItemID,
			WorkspaceID: env.WorkspaceID,
			ActorUserID: env.UserID1, // John mentions himself
		}

		err := service.ProcessMentions(params)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Verify self-mention was not created
		var count int
		err = db.QueryRow(`
			SELECT COUNT(*) FROM mentions
			WHERE source_type = ? AND source_id = ? AND mentioned_user_id = ?
		`, "comment", env.CommentID+100, env.UserID1).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to query mentions: %v", err)
		}
		if count != 0 {
			t.Errorf("Expected 0 mentions (self-mention skipped), got %d", count)
		}
	})

	t.Run("SkipsInactiveUser", func(t *testing.T) {
		// Clean up previous test
		db.Exec("DELETE FROM mentions")

		params := ProcessMentionsParams{
			SourceType:  "comment",
			SourceID:    env.CommentID + 200, // Different source
			Content:     "Hello @inactive_user!",
			ItemID:      env.ItemID,
			WorkspaceID: env.WorkspaceID,
			ActorUserID: env.UserID1,
		}

		err := service.ProcessMentions(params)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Verify inactive user mention was not created
		var count int
		err = db.QueryRow(`
			SELECT COUNT(*) FROM mentions
			WHERE source_type = ? AND source_id = ? AND mentioned_user_id = ?
		`, "comment", env.CommentID+200, env.UserID3).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to query mentions: %v", err)
		}
		if count != 0 {
			t.Errorf("Expected 0 mentions (inactive user skipped), got %d", count)
		}
	})

	t.Run("SkipsUnknownUser", func(t *testing.T) {
		// Clean up previous test
		db.Exec("DELETE FROM mentions")

		params := ProcessMentionsParams{
			SourceType:  "comment",
			SourceID:    env.CommentID + 300, // Different source
			Content:     "Hello @nonexistentuser!",
			ItemID:      env.ItemID,
			WorkspaceID: env.WorkspaceID,
			ActorUserID: env.UserID1,
		}

		err := service.ProcessMentions(params)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Verify no mentions were created
		var count int
		err = db.QueryRow(`
			SELECT COUNT(*) FROM mentions WHERE source_type = ? AND source_id = ?
		`, "comment", env.CommentID+300).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to query mentions: %v", err)
		}
		if count != 0 {
			t.Errorf("Expected 0 mentions, got %d", count)
		}
	})

	t.Run("RemovesMentionOnUpdate", func(t *testing.T) {
		// Clean up
		db.Exec("DELETE FROM mentions")

		sourceID := env.CommentID + 400

		// First, create a mention
		params := ProcessMentionsParams{
			SourceType:  "comment",
			SourceID:    sourceID,
			Content:     "Hello @johndoe!",
			ItemID:      env.ItemID,
			WorkspaceID: env.WorkspaceID,
			ActorUserID: env.UserID2,
		}
		err := service.ProcessMentions(params)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Verify mention exists
		var count int
		err = db.QueryRow(`SELECT COUNT(*) FROM mentions WHERE source_id = ?`, sourceID).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to query mentions: %v", err)
		}
		if count != 1 {
			t.Fatalf("Expected 1 mention, got %d", count)
		}

		// Now update with content that has no mentions
		params.Content = "Hello everyone!"
		err = service.ProcessMentions(params)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Verify mention was removed
		err = db.QueryRow(`SELECT COUNT(*) FROM mentions WHERE source_id = ?`, sourceID).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to query mentions: %v", err)
		}
		if count != 0 {
			t.Errorf("Expected 0 mentions after update, got %d", count)
		}
	})

	t.Run("HandlesDisplayNameMention", func(t *testing.T) {
		// Clean up
		db.Exec("DELETE FROM mentions")

		sourceID := env.CommentID + 500

		params := ProcessMentionsParams{
			SourceType:  "comment",
			SourceID:    sourceID,
			Content:     "Hello @\"John Doe\"!",
			ItemID:      env.ItemID,
			WorkspaceID: env.WorkspaceID,
			ActorUserID: env.UserID2,
		}

		err := service.ProcessMentions(params)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Verify mention was created (matching by display name)
		var count int
		err = db.QueryRow(`
			SELECT COUNT(*) FROM mentions
			WHERE source_type = ? AND source_id = ? AND mentioned_user_id = ?
		`, "comment", sourceID, env.UserID1).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to query mentions: %v", err)
		}
		if count != 1 {
			t.Errorf("Expected 1 mention from display name, got %d", count)
		}
	})
}

func TestMentionService_DeleteMentionsForSource(t *testing.T) {
	db := createMentionTestDB(t)
	defer db.Close()

	service := NewMentionService(db, nil)
	env := setupMentionTestEnv(t, db)

	t.Run("DeletesAllMentionsForSource", func(t *testing.T) {
		sourceID := env.CommentID + 600

		// Create some mentions
		params := ProcessMentionsParams{
			SourceType:  "comment",
			SourceID:    sourceID,
			Content:     "Hello @johndoe and @janedoe!",
			ItemID:      env.ItemID,
			WorkspaceID: env.WorkspaceID,
			ActorUserID: env.UserID3, // Third user mentioning others (UserID3 is inactive so won't be mentioned back)
		}

		// Note: UserID3 is inactive, but they can still mention others
		// We need to make this work differently - use a fourth user or change the test
		// Actually, let's use the janedoe user to mention johndoe
		params.ActorUserID = env.UserID2 // Jane mentioning John only
		params.Content = "Hello @johndoe!"

		err := service.ProcessMentions(params)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Verify mentions exist
		var count int
		err = db.QueryRow(`SELECT COUNT(*) FROM mentions WHERE source_type = ? AND source_id = ?`, "comment", sourceID).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to query mentions: %v", err)
		}
		if count == 0 {
			t.Fatal("Expected at least 1 mention")
		}

		// Delete all mentions for this source
		err = service.DeleteMentionsForSource("comment", sourceID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Verify mentions were deleted
		err = db.QueryRow(`SELECT COUNT(*) FROM mentions WHERE source_type = ? AND source_id = ?`, "comment", sourceID).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to query mentions: %v", err)
		}
		if count != 0 {
			t.Errorf("Expected 0 mentions after delete, got %d", count)
		}
	})

	t.Run("DoesNotDeleteOtherSources", func(t *testing.T) {
		// Clean up
		db.Exec("DELETE FROM mentions")

		sourceID1 := env.CommentID + 700
		sourceID2 := env.CommentID + 701

		// Create mentions for two different sources
		params1 := ProcessMentionsParams{
			SourceType:  "comment",
			SourceID:    sourceID1,
			Content:     "Hello @johndoe!",
			ItemID:      env.ItemID,
			WorkspaceID: env.WorkspaceID,
			ActorUserID: env.UserID2,
		}
		err := service.ProcessMentions(params1)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		params2 := ProcessMentionsParams{
			SourceType:  "comment",
			SourceID:    sourceID2,
			Content:     "Hello @johndoe!",
			ItemID:      env.ItemID,
			WorkspaceID: env.WorkspaceID,
			ActorUserID: env.UserID2,
		}
		err = service.ProcessMentions(params2)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Delete mentions for first source only
		err = service.DeleteMentionsForSource("comment", sourceID1)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Verify second source still has mentions
		var count int
		err = db.QueryRow(`SELECT COUNT(*) FROM mentions WHERE source_type = ? AND source_id = ?`, "comment", sourceID2).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to query mentions: %v", err)
		}
		if count != 1 {
			t.Errorf("Expected 1 mention for other source, got %d", count)
		}
	})

	t.Run("HandlesNonExistentSource", func(t *testing.T) {
		err := service.DeleteMentionsForSource("comment", 99999)
		if err != nil {
			t.Errorf("Expected no error for non-existent source, got: %v", err)
		}
	})
}

func TestMentionPattern(t *testing.T) {
	t.Run("MatchesSimpleUsername", func(t *testing.T) {
		matches := MentionPattern.FindAllStringSubmatch("@johndoe", -1)
		if len(matches) != 1 {
			t.Fatalf("Expected 1 match, got %d", len(matches))
		}
		if matches[0][1] != "johndoe" {
			t.Errorf("Expected 'johndoe', got '%s'", matches[0][1])
		}
	})

	t.Run("MatchesQuotedName", func(t *testing.T) {
		matches := MentionPattern.FindAllStringSubmatch("@\"John Doe\"", -1)
		if len(matches) != 1 {
			t.Fatalf("Expected 1 match, got %d", len(matches))
		}
		if matches[0][2] != "John Doe" {
			t.Errorf("Expected 'John Doe', got '%s'", matches[0][2])
		}
	})

	t.Run("MatchesAtStartOfString", func(t *testing.T) {
		matches := MentionPattern.FindAllStringSubmatch("@johndoe hello", -1)
		if len(matches) != 1 {
			t.Fatalf("Expected 1 match, got %d", len(matches))
		}
	})

	t.Run("MatchesAtEndOfString", func(t *testing.T) {
		matches := MentionPattern.FindAllStringSubmatch("hello @johndoe", -1)
		if len(matches) != 1 {
			t.Fatalf("Expected 1 match, got %d", len(matches))
		}
	})

	t.Run("MatchesInMiddleOfString", func(t *testing.T) {
		matches := MentionPattern.FindAllStringSubmatch("hello @johndoe world", -1)
		if len(matches) != 1 {
			t.Fatalf("Expected 1 match, got %d", len(matches))
		}
	})

	t.Run("DoesNotMatchEmail", func(t *testing.T) {
		content := "contact me at email@example.com"
		matches := MentionPattern.FindAllStringSubmatch(content, -1)
		if len(matches) != 0 {
			t.Fatalf("Expected 0 matches for email address, got %d", len(matches))
		}
	})

	t.Run("DoesNotMatchEmailWithDottedLocalPart", func(t *testing.T) {
		content := "contact user.name@example.com"
		matches := MentionPattern.FindAllStringSubmatch(content, -1)
		if len(matches) != 0 {
			t.Fatalf("Expected 0 matches for email address, got %d", len(matches))
		}
	})

	t.Run("DoesNotMatchEmailWithPlusTag", func(t *testing.T) {
		// user+tag@example.com — the + is not in [a-zA-Z0-9.] so @example.com
		// would start after a non-alnum char, but the local part "user+tag" means
		// the char before @ is 'g' which IS alphanumeric → should not match.
		// Actually '+' is not in [a-zA-Z0-9.], so the char before @ is 'g'
		// Wait: "user+tag@example.com" → char before @ is 'g' → alphanumeric → no match
		content := "contact user+tag@example.com"
		matches := MentionPattern.FindAllStringSubmatch(content, -1)
		if len(matches) != 0 {
			t.Fatalf("Expected 0 matches for email with plus tag, got %d", len(matches))
		}
	})

	t.Run("MatchesMentionInParentheses", func(t *testing.T) {
		content := "(@johndoe)"
		matches := MentionPattern.FindAllStringSubmatch(content, -1)
		if len(matches) != 1 {
			t.Fatalf("Expected 1 match, got %d", len(matches))
		}
		if matches[0][1] != "johndoe" {
			t.Errorf("Expected 'johndoe', got '%s'", matches[0][1])
		}
	})

	t.Run("MatchesMentionAfterNewline", func(t *testing.T) {
		content := "hello\n@johndoe"
		matches := MentionPattern.FindAllStringSubmatch(content, -1)
		if len(matches) != 1 {
			t.Fatalf("Expected 1 match, got %d", len(matches))
		}
		if matches[0][1] != "johndoe" {
			t.Errorf("Expected 'johndoe', got '%s'", matches[0][1])
		}
	})
}
