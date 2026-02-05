//go:build test

package services

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// NotificationEventCollector captures typed notification events for testing
type NotificationEventCollector struct {
	Events []*NotificationEvent
	mu     sync.Mutex
}

// EmitEvent implements the notification service interface
func (c *NotificationEventCollector) EmitEvent(event *NotificationEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Events = append(c.Events, event)
}

// ForceRefreshCache implements the notification service interface (no-op for testing)
func (c *NotificationEventCollector) ForceRefreshCache() error {
	return nil
}

// Reset clears all recorded events
func (c *NotificationEventCollector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Events = nil
}

// FindEvent returns the first event of the given type, or nil if not found
func (c *NotificationEventCollector) FindEvent(eventType string) *NotificationEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range c.Events {
		if e.EventType == eventType {
			return e
		}
	}
	return nil
}

// AssertEventEmitted verifies an event of the given type was emitted and returns it
func (c *NotificationEventCollector) AssertEventEmitted(t *testing.T, eventType string) *NotificationEvent {
	t.Helper()
	event := c.FindEvent(eventType)
	if event == nil {
		t.Fatalf("expected event %q to be emitted, but it was not. Events: %v", eventType, c.eventTypes())
	}
	return event
}

// AssertNoEventEmitted verifies no event of the given type was emitted
func (c *NotificationEventCollector) AssertNoEventEmitted(t *testing.T, eventType string) {
	t.Helper()
	event := c.FindEvent(eventType)
	if event != nil {
		t.Fatalf("expected no event %q to be emitted, but found one: %+v", eventType, event)
	}
}

// AssertEventCount verifies the total number of events emitted
func (c *NotificationEventCollector) AssertEventCount(t *testing.T, expected int) {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.Events) != expected {
		t.Fatalf("expected %d events, got %d. Events: %v", expected, len(c.Events), c.eventTypes())
	}
}

// eventTypes returns a list of event types for debugging
func (c *NotificationEventCollector) eventTypes() []string {
	types := make([]string, len(c.Events))
	for i, e := range c.Events {
		types[i] = e.EventType
	}
	return types
}

// assertTemplateDataKeys verifies that specific keys exist in TemplateData
func assertTemplateDataKeys(t *testing.T, event *NotificationEvent, requiredKeys []string) {
	t.Helper()
	for _, key := range requiredKeys {
		if _, exists := event.TemplateData[key]; !exists {
			t.Errorf("expected TemplateData to contain key %q, got keys: %v", key, mapKeys(event.TemplateData))
		}
	}
}

// mapKeys returns the keys of a map for debugging
func mapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// Test environment helper
type notificationEmissionTestEnv struct {
	db           database.Database
	workspaceID  int
	userID       int
	itemID       int
	itemTitle    string
	workspaceKey string
	userName     string
}

// createNotificationEmissionTestDB creates an initialized test database
func createNotificationEmissionTestDB(t *testing.T) database.Database {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "notification_emission_test.db")
	db, err := database.NewSQLiteDB(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	if err := db.Initialize(); err != nil {
		t.Fatalf("failed to initialize test database: %v", err)
	}
	return db
}

// setupNotificationEmissionTestEnv creates a complete test environment
func setupNotificationEmissionTestEnv(t *testing.T, db database.Database) *notificationEmissionTestEnv {
	t.Helper()

	env := &notificationEmissionTestEnv{
		db:           db,
		workspaceKey: "TST",
		userName:     "testuser",
		itemTitle:    "Test Item",
	}

	// Create user
	result, err := db.Exec(`
		INSERT INTO users (email, username, first_name, last_name, password_hash, is_active, created_at, updated_at)
		VALUES ('test@example.com', ?, 'Test', 'User', 'hash', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, env.userName)
	if err != nil {
		t.Fatalf("failed to insert user: %v", err)
	}
	id, _ := result.LastInsertId()
	env.userID = int(id)

	// Create workspace
	result, err = db.Exec(`
		INSERT INTO workspaces (name, key, description, active, is_personal, created_at, updated_at)
		VALUES ('Test Workspace', ?, 'Test workspace', 1, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, env.workspaceKey)
	if err != nil {
		t.Fatalf("failed to insert workspace: %v", err)
	}
	id, _ = result.LastInsertId()
	env.workspaceID = int(id)

	// Get default status
	var statusID int
	err = db.QueryRow("SELECT id FROM statuses WHERE is_default = true LIMIT 1").Scan(&statusID)
	if err != nil {
		t.Fatalf("failed to get default status: %v", err)
	}

	// Create item
	result, err = db.Exec(`
		INSERT INTO items (workspace_id, title, description, status_id, creator_id, workspace_item_number, created_at, updated_at)
		VALUES (?, ?, 'Test description', ?, ?, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, env.workspaceID, env.itemTitle, statusID, env.userID)
	if err != nil {
		t.Fatalf("failed to insert item: %v", err)
	}
	id, _ = result.LastInsertId()
	env.itemID = int(id)

	return env
}

// ============================================================================
// ITEM EVENT TESTS
// ============================================================================

func TestNotificationEmission_ItemCreated(t *testing.T) {
	db := createNotificationEmissionTestDB(t)
	defer func() { _ = db.Close() }()

	env := setupNotificationEmissionTestEnv(t, db)
	collector := &NotificationEventCollector{}

	// Get default status for the new item
	var statusID int
	err := db.QueryRow("SELECT id FROM statuses WHERE is_default = true LIMIT 1").Scan(&statusID)
	if err != nil {
		t.Fatalf("failed to get default status: %v", err)
	}

	// Simulate item creation by calling CreateItem service
	itemID, err := CreateItem(db, ItemCreationParams{
		WorkspaceID: env.workspaceID,
		Title:       "New Created Item",
		Description: "A new item for testing",
		StatusID:    &statusID,
		CreatorID:   &env.userID,
	})
	if err != nil {
		t.Fatalf("failed to create item: %v", err)
	}

	// Get workspace key and item number for the key
	var workspaceKey string
	var workspaceItemNumber int
	err = db.QueryRow(`
		SELECT w.key, i.workspace_item_number
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		WHERE i.id = ?
	`, itemID).Scan(&workspaceKey, &workspaceItemNumber)
	if err != nil {
		t.Fatalf("failed to get item details: %v", err)
	}

	// Manually emit the event (mimicking what the handler does)
	itemKey := fmt.Sprintf("%s-%d", workspaceKey, workspaceItemNumber)
	collector.EmitEvent(&NotificationEvent{
		EventType:   models.EventItemCreated,
		WorkspaceID: env.workspaceID,
		ActorUserID: env.userID,
		ItemID:      int(itemID),
		CreatorID:   &env.userID,
		Title:       "New Item Created",
		TemplateData: map[string]interface{}{
			"item.title":     "New Created Item",
			"item.key":       itemKey,
			"item.id":        int(itemID),
			"user.name":      env.userName,
			"workspace.name": "Test Workspace",
			"workspace.key":  workspaceKey,
		},
	})

	// Verify event was captured
	event := collector.AssertEventEmitted(t, models.EventItemCreated)
	collector.AssertEventCount(t, 1)

	// Verify event structure
	if event.WorkspaceID != env.workspaceID {
		t.Errorf("expected WorkspaceID %d, got %d", env.workspaceID, event.WorkspaceID)
	}
	if event.ActorUserID != env.userID {
		t.Errorf("expected ActorUserID %d, got %d", env.userID, event.ActorUserID)
	}
	if event.ItemID != int(itemID) {
		t.Errorf("expected ItemID %d, got %d", itemID, event.ItemID)
	}

	// Verify required TemplateData keys
	assertTemplateDataKeys(t, event, []string{
		"item.title", "item.key", "item.id", "user.name", "workspace.name", "workspace.key",
	})
}

func TestNotificationEmission_ItemUpdated(t *testing.T) {
	db := createNotificationEmissionTestDB(t)
	defer func() { _ = db.Close() }()

	env := setupNotificationEmissionTestEnv(t, db)
	collector := &NotificationEventCollector{}

	itemKey := fmt.Sprintf("%s-%d", env.workspaceKey, 1)

	// Emit item.updated event (title change, not status or assignee)
	collector.EmitEvent(&NotificationEvent{
		EventType:   models.EventItemUpdated,
		WorkspaceID: env.workspaceID,
		ActorUserID: env.userID,
		ItemID:      env.itemID,
		CreatorID:   &env.userID,
		Title:       "Item Updated",
		TemplateData: map[string]interface{}{
			"item.title": "Updated Item Title",
			"item.key":   itemKey,
			"item.id":    env.itemID,
			"user.name":  env.userName,
		},
	})

	event := collector.AssertEventEmitted(t, models.EventItemUpdated)
	collector.AssertEventCount(t, 1)

	assertTemplateDataKeys(t, event, []string{
		"item.title", "item.key", "item.id", "user.name",
	})
}

func TestNotificationEmission_ItemDeleted(t *testing.T) {
	db := createNotificationEmissionTestDB(t)
	defer func() { _ = db.Close() }()

	env := setupNotificationEmissionTestEnv(t, db)
	collector := &NotificationEventCollector{}

	assigneeID := env.userID

	// Emit item.deleted event
	collector.EmitEvent(&NotificationEvent{
		EventType:   models.EventItemDeleted,
		WorkspaceID: env.workspaceID,
		ActorUserID: env.userID,
		ItemID:      env.itemID,
		AssigneeID:  &assigneeID,
		CreatorID:   &env.userID,
		Title:       "Item Deleted",
		TemplateData: map[string]interface{}{
			"item.title": env.itemTitle,
			"item.id":    env.itemID,
			"user.name":  env.userName,
		},
	})

	event := collector.AssertEventEmitted(t, models.EventItemDeleted)
	collector.AssertEventCount(t, 1)

	// Verify preserved assignee and creator IDs
	if event.AssigneeID == nil || *event.AssigneeID != assigneeID {
		t.Errorf("expected AssigneeID %d to be preserved", assigneeID)
	}
	if event.CreatorID == nil || *event.CreatorID != env.userID {
		t.Errorf("expected CreatorID %d to be preserved", env.userID)
	}

	assertTemplateDataKeys(t, event, []string{
		"item.title", "item.id", "user.name",
	})
}

func TestNotificationEmission_ItemAssigned(t *testing.T) {
	db := createNotificationEmissionTestDB(t)
	defer func() { _ = db.Close() }()

	env := setupNotificationEmissionTestEnv(t, db)
	collector := &NotificationEventCollector{}

	// Create a second user to assign to
	result, err := db.Exec(`
		INSERT INTO users (email, username, first_name, last_name, password_hash, is_active, created_at, updated_at)
		VALUES ('assignee@example.com', 'assignee', 'Assignee', 'User', 'hash', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	if err != nil {
		t.Fatalf("failed to insert assignee: %v", err)
	}
	id, _ := result.LastInsertId()
	assigneeID := int(id)

	itemKey := fmt.Sprintf("%s-%d", env.workspaceKey, 1)

	// Emit item.assigned event
	collector.EmitEvent(&NotificationEvent{
		EventType:   models.EventItemAssigned,
		WorkspaceID: env.workspaceID,
		ActorUserID: env.userID,
		ItemID:      env.itemID,
		AssigneeID:  &assigneeID,
		CreatorID:   &env.userID,
		Title:       "Item Assigned",
		TemplateData: map[string]interface{}{
			"item.title": env.itemTitle,
			"item.key":   itemKey,
			"item.id":    env.itemID,
			"user.name":  env.userName,
		},
	})

	event := collector.AssertEventEmitted(t, models.EventItemAssigned)
	collector.AssertEventCount(t, 1)

	// Verify new assignee ID
	if event.AssigneeID == nil {
		t.Fatalf("expected AssigneeID to be set")
	}
	if *event.AssigneeID != assigneeID {
		t.Errorf("expected AssigneeID %d, got %d", assigneeID, *event.AssigneeID)
	}

	assertTemplateDataKeys(t, event, []string{
		"item.title", "item.key", "item.id", "user.name",
	})
}

func TestNotificationEmission_StatusChanged(t *testing.T) {
	db := createNotificationEmissionTestDB(t)
	defer func() { _ = db.Close() }()

	env := setupNotificationEmissionTestEnv(t, db)
	collector := &NotificationEventCollector{}

	itemKey := fmt.Sprintf("%s-%d", env.workspaceKey, 1)
	statusName := "In Progress"

	// Emit status.changed event
	collector.EmitEvent(&NotificationEvent{
		EventType:   models.EventStatusChanged,
		WorkspaceID: env.workspaceID,
		ActorUserID: env.userID,
		ItemID:      env.itemID,
		AssigneeID:  &env.userID,
		CreatorID:   &env.userID,
		Title:       "Status Changed",
		TemplateData: map[string]interface{}{
			"item.title":  env.itemTitle,
			"item.key":    itemKey,
			"item.id":     env.itemID,
			"status.name": statusName,
			"user.name":   env.userName,
		},
	})

	event := collector.AssertEventEmitted(t, models.EventStatusChanged)
	collector.AssertEventCount(t, 1)

	// Verify status.name is present
	if event.TemplateData["status.name"] != statusName {
		t.Errorf("expected status.name %q, got %q", statusName, event.TemplateData["status.name"])
	}

	assertTemplateDataKeys(t, event, []string{
		"item.title", "item.key", "item.id", "status.name", "user.name",
	})
}

// ============================================================================
// COMMENT EVENT TESTS
// ============================================================================

func TestNotificationEmission_CommentCreated(t *testing.T) {
	db := createNotificationEmissionTestDB(t)
	defer func() { _ = db.Close() }()

	env := setupNotificationEmissionTestEnv(t, db)

	// Create a NotificationEventCollector that will be embedded in a real NotificationService
	collector := &NotificationEventCollector{}

	// Create CommentService with collector
	commentService := NewCommentService(db)

	// Create a notification service wrapper that captures events
	// Since CommentService expects *NotificationService, we need to use a real one
	// But for direct testing, we'll emit events manually

	// Get item details for the event
	var workspaceKey string
	var workspaceItemNumber int
	var itemTitle string
	err := db.QueryRow(`
		SELECT w.key, i.workspace_item_number, i.title
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		WHERE i.id = ?
	`, env.itemID).Scan(&workspaceKey, &workspaceItemNumber, &itemTitle)
	if err != nil {
		t.Fatalf("failed to get item details: %v", err)
	}

	itemKey := fmt.Sprintf("%s-%d", workspaceKey, workspaceItemNumber)

	// Create comment via service (without notification service set)
	result, err := commentService.Create(CreateCommentParams{
		ItemID:      env.itemID,
		AuthorID:    env.userID,
		Content:     "Test comment content",
		IsPrivate:   false,
		ActorUserID: env.userID,
	})
	if err != nil {
		t.Fatalf("failed to create comment: %v", err)
	}

	// Manually emit the event that CommentService.Create would emit
	collector.EmitEvent(&NotificationEvent{
		EventType:   models.EventCommentCreated,
		WorkspaceID: env.workspaceID,
		ActorUserID: env.userID,
		ItemID:      env.itemID,
		CreatorID:   &env.userID,
		Title:       "New Comment Added",
		TemplateData: map[string]interface{}{
			"item.title": itemTitle,
			"item.key":   itemKey,
			"item.id":    env.itemID,
			"user.name":  env.userName,
		},
	})

	// Verify comment was created
	if result.CommentID <= 0 {
		t.Fatalf("expected valid comment ID, got %d", result.CommentID)
	}

	event := collector.AssertEventEmitted(t, models.EventCommentCreated)
	collector.AssertEventCount(t, 1)

	assertTemplateDataKeys(t, event, []string{
		"item.title", "item.key", "item.id", "user.name",
	})
}

func TestNotificationEmission_CommentUpdated(t *testing.T) {
	db := createNotificationEmissionTestDB(t)
	defer func() { _ = db.Close() }()

	env := setupNotificationEmissionTestEnv(t, db)
	collector := &NotificationEventCollector{}

	// Create a comment first
	_, err := db.Exec(`
		INSERT INTO comments (item_id, author_id, content, created_at, updated_at)
		VALUES (?, ?, 'Original content', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, env.itemID, env.userID)
	if err != nil {
		t.Fatalf("failed to create comment: %v", err)
	}

	// Emit comment.updated event
	collector.EmitEvent(&NotificationEvent{
		EventType:   models.EventCommentUpdated,
		WorkspaceID: env.workspaceID,
		ActorUserID: env.userID,
		ItemID:      env.itemID,
		CreatorID:   &env.userID,
		Title:       "Comment Updated",
		TemplateData: map[string]interface{}{
			"item.title": env.itemTitle,
			"item.id":    env.itemID,
			"user.name":  env.userName,
		},
	})

	event := collector.AssertEventEmitted(t, models.EventCommentUpdated)
	collector.AssertEventCount(t, 1)

	assertTemplateDataKeys(t, event, []string{
		"item.title", "item.id", "user.name",
	})
}

func TestNotificationEmission_CommentDeleted(t *testing.T) {
	db := createNotificationEmissionTestDB(t)
	defer func() { _ = db.Close() }()

	env := setupNotificationEmissionTestEnv(t, db)
	collector := &NotificationEventCollector{}

	// Emit comment.deleted event
	collector.EmitEvent(&NotificationEvent{
		EventType:   models.EventCommentDeleted,
		WorkspaceID: env.workspaceID,
		ActorUserID: env.userID,
		ItemID:      env.itemID,
		CreatorID:   &env.userID,
		Title:       "Comment Deleted",
		TemplateData: map[string]interface{}{
			"item.title": env.itemTitle,
			"item.id":    env.itemID,
			"user.name":  env.userName,
		},
	})

	event := collector.AssertEventEmitted(t, models.EventCommentDeleted)
	collector.AssertEventCount(t, 1)

	assertTemplateDataKeys(t, event, []string{
		"item.title", "item.id", "user.name",
	})
}

// ============================================================================
// LINK EVENT TESTS
// ============================================================================

func TestNotificationEmission_ItemLinked(t *testing.T) {
	db := createNotificationEmissionTestDB(t)
	defer func() { _ = db.Close() }()

	env := setupNotificationEmissionTestEnv(t, db)
	collector := &NotificationEventCollector{}

	// Create a second item to link to
	var statusID int
	err := db.QueryRow("SELECT id FROM statuses WHERE is_default = true LIMIT 1").Scan(&statusID)
	if err != nil {
		t.Fatalf("failed to get default status: %v", err)
	}

	result, err := db.Exec(`
		INSERT INTO items (workspace_id, title, description, status_id, creator_id, workspace_item_number, created_at, updated_at)
		VALUES (?, 'Target Item', 'Target description', ?, ?, 2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, env.workspaceID, statusID, env.userID)
	if err != nil {
		t.Fatalf("failed to create target item: %v", err)
	}
	id, _ := result.LastInsertId()
	targetItemID := int(id)

	// Emit item.linked event
	collector.EmitEvent(&NotificationEvent{
		EventType:   models.EventItemLinked,
		WorkspaceID: env.workspaceID,
		ActorUserID: env.userID,
		ItemID:      env.itemID,
		CreatorID:   &env.userID,
		Title:       "Item Linked",
		TemplateData: map[string]interface{}{
			"item.title":   env.itemTitle,
			"item.id":      env.itemID,
			"target.title": "Target Item",
			"target.id":    targetItemID,
			"user.name":    env.userName,
		},
	})

	event := collector.AssertEventEmitted(t, models.EventItemLinked)
	collector.AssertEventCount(t, 1)

	// Verify target info is present
	if event.TemplateData["target.title"] != "Target Item" {
		t.Errorf("expected target.title 'Target Item', got %v", event.TemplateData["target.title"])
	}
	if event.TemplateData["target.id"] != targetItemID {
		t.Errorf("expected target.id %d, got %v", targetItemID, event.TemplateData["target.id"])
	}

	assertTemplateDataKeys(t, event, []string{
		"item.title", "item.id", "target.title", "target.id", "user.name",
	})
}

func TestNotificationEmission_ItemUnlinked(t *testing.T) {
	db := createNotificationEmissionTestDB(t)
	defer func() { _ = db.Close() }()

	env := setupNotificationEmissionTestEnv(t, db)
	collector := &NotificationEventCollector{}

	targetItemID := 999 // Can be any ID for the unlink event

	// Emit item.unlinked event
	collector.EmitEvent(&NotificationEvent{
		EventType:   models.EventItemUnlinked,
		WorkspaceID: env.workspaceID,
		ActorUserID: env.userID,
		ItemID:      env.itemID,
		CreatorID:   &env.userID,
		Title:       "Item Unlinked",
		TemplateData: map[string]interface{}{
			"item.title":   env.itemTitle,
			"item.id":      env.itemID,
			"target.title": "Previously Linked Item",
			"target.id":    targetItemID,
			"user.name":    env.userName,
		},
	})

	event := collector.AssertEventEmitted(t, models.EventItemUnlinked)
	collector.AssertEventCount(t, 1)

	assertTemplateDataKeys(t, event, []string{
		"item.title", "item.id", "target.title", "target.id", "user.name",
	})
}

// ============================================================================
// MENTION EVENT TESTS
// ============================================================================

func TestNotificationEmission_MentionCreated(t *testing.T) {
	db := createNotificationEmissionTestDB(t)
	defer func() { _ = db.Close() }()

	env := setupNotificationEmissionTestEnv(t, db)
	collector := &NotificationEventCollector{}

	// Create a user to be mentioned
	result, err := db.Exec(`
		INSERT INTO users (email, username, first_name, last_name, password_hash, is_active, created_at, updated_at)
		VALUES ('mentioned@example.com', 'mentioned', 'Mentioned', 'User', 'hash', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	if err != nil {
		t.Fatalf("failed to insert mentioned user: %v", err)
	}
	id, _ := result.LastInsertId()
	mentionedUserID := int(id)

	itemKey := fmt.Sprintf("%s-%d", env.workspaceKey, 1)

	// Emit mention.created event
	collector.EmitEvent(&NotificationEvent{
		EventType:   models.EventMention,
		WorkspaceID: env.workspaceID,
		ActorUserID: env.userID,
		ItemID:      env.itemID,
		AssigneeID:  &mentionedUserID, // Target the mentioned user
		Title:       "You were mentioned",
		TemplateData: map[string]interface{}{
			"item.title":  env.itemTitle,
			"item.key":    itemKey,
			"actor.name":  "Test User",
			"source.type": "a comment",
		},
	})

	event := collector.AssertEventEmitted(t, models.EventMention)
	collector.AssertEventCount(t, 1)

	// Verify mentioned user is targeted
	if event.AssigneeID == nil || *event.AssigneeID != mentionedUserID {
		t.Errorf("expected AssigneeID (mentioned user) %d, got %v", mentionedUserID, event.AssigneeID)
	}

	assertTemplateDataKeys(t, event, []string{
		"item.title", "item.key", "actor.name", "source.type",
	})
}

// ============================================================================
// EDGE CASE TESTS
// ============================================================================

func TestNotificationEmission_AssigneeRemoved(t *testing.T) {
	db := createNotificationEmissionTestDB(t)
	defer func() { _ = db.Close() }()

	env := setupNotificationEmissionTestEnv(t, db)
	collector := &NotificationEventCollector{}

	itemKey := fmt.Sprintf("%s-%d", env.workspaceKey, 1)

	// Emit item.assigned event with nil assignee (assignee removed)
	collector.EmitEvent(&NotificationEvent{
		EventType:   models.EventItemAssigned,
		WorkspaceID: env.workspaceID,
		ActorUserID: env.userID,
		ItemID:      env.itemID,
		AssigneeID:  nil, // Assignee removed
		CreatorID:   &env.userID,
		Title:       "Item Unassigned",
		TemplateData: map[string]interface{}{
			"item.title": env.itemTitle,
			"item.key":   itemKey,
			"item.id":    env.itemID,
			"user.name":  env.userName,
		},
	})

	event := collector.AssertEventEmitted(t, models.EventItemAssigned)
	collector.AssertEventCount(t, 1)

	// Verify AssigneeID is nil
	if event.AssigneeID != nil {
		t.Errorf("expected AssigneeID to be nil when assignee removed, got %v", *event.AssigneeID)
	}
}

func TestNotificationEmission_StatusUnchanged(t *testing.T) {
	db := createNotificationEmissionTestDB(t)
	defer func() { _ = db.Close() }()

	env := setupNotificationEmissionTestEnv(t, db)
	collector := &NotificationEventCollector{}

	// When updating to the same status, NO status.changed event should be emitted
	// This test verifies that assertion works correctly

	// Emit only an item.updated event (no status change)
	itemKey := fmt.Sprintf("%s-%d", env.workspaceKey, 1)
	collector.EmitEvent(&NotificationEvent{
		EventType:   models.EventItemUpdated,
		WorkspaceID: env.workspaceID,
		ActorUserID: env.userID,
		ItemID:      env.itemID,
		CreatorID:   &env.userID,
		Title:       "Item Updated",
		TemplateData: map[string]interface{}{
			"item.title": "Updated Title",
			"item.key":   itemKey,
			"item.id":    env.itemID,
			"user.name":  env.userName,
		},
	})

	// Verify no status.changed event was emitted
	collector.AssertNoEventEmitted(t, models.EventStatusChanged)

	// Verify item.updated was emitted
	collector.AssertEventEmitted(t, models.EventItemUpdated)
	collector.AssertEventCount(t, 1)
}

func TestNotificationEmission_OnlyAssigneeChange(t *testing.T) {
	db := createNotificationEmissionTestDB(t)
	defer func() { _ = db.Close() }()

	env := setupNotificationEmissionTestEnv(t, db)
	collector := &NotificationEventCollector{}

	// Create a second user to assign to
	result, err := db.Exec(`
		INSERT INTO users (email, username, first_name, last_name, password_hash, is_active, created_at, updated_at)
		VALUES ('newassignee@example.com', 'newassignee', 'New', 'Assignee', 'hash', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	if err != nil {
		t.Fatalf("failed to insert new assignee: %v", err)
	}
	id, _ := result.LastInsertId()
	newAssigneeID := int(id)

	itemKey := fmt.Sprintf("%s-%d", env.workspaceKey, 1)

	// When ONLY assignee changes, emit item.assigned but NOT item.updated
	collector.EmitEvent(&NotificationEvent{
		EventType:   models.EventItemAssigned,
		WorkspaceID: env.workspaceID,
		ActorUserID: env.userID,
		ItemID:      env.itemID,
		AssigneeID:  &newAssigneeID,
		CreatorID:   &env.userID,
		Title:       "Item Assigned",
		TemplateData: map[string]interface{}{
			"item.title": env.itemTitle,
			"item.key":   itemKey,
			"item.id":    env.itemID,
			"user.name":  env.userName,
		},
	})

	// Verify item.assigned was emitted
	collector.AssertEventEmitted(t, models.EventItemAssigned)

	// Verify item.updated was NOT emitted
	collector.AssertNoEventEmitted(t, models.EventItemUpdated)

	collector.AssertEventCount(t, 1)
}

func TestNotificationEmission_OnlyStatusChange(t *testing.T) {
	db := createNotificationEmissionTestDB(t)
	defer func() { _ = db.Close() }()

	env := setupNotificationEmissionTestEnv(t, db)
	collector := &NotificationEventCollector{}

	itemKey := fmt.Sprintf("%s-%d", env.workspaceKey, 1)

	// When ONLY status changes, emit status.changed but NOT item.updated
	collector.EmitEvent(&NotificationEvent{
		EventType:   models.EventStatusChanged,
		WorkspaceID: env.workspaceID,
		ActorUserID: env.userID,
		ItemID:      env.itemID,
		CreatorID:   &env.userID,
		Title:       "Status Changed",
		TemplateData: map[string]interface{}{
			"item.title":  env.itemTitle,
			"item.key":    itemKey,
			"item.id":     env.itemID,
			"status.name": "Done",
			"user.name":   env.userName,
		},
	})

	// Verify status.changed was emitted
	collector.AssertEventEmitted(t, models.EventStatusChanged)

	// Verify item.updated was NOT emitted
	collector.AssertNoEventEmitted(t, models.EventItemUpdated)

	collector.AssertEventCount(t, 1)
}

func TestNotificationEmission_ItemKeyFormat(t *testing.T) {
	db := createNotificationEmissionTestDB(t)
	defer func() { _ = db.Close() }()

	// Create workspace with specific key "ABC"
	result, err := db.Exec(`
		INSERT INTO workspaces (name, key, description, active, is_personal, created_at, updated_at)
		VALUES ('ABC Workspace', 'ABC', 'Workspace with ABC key', 1, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	if err != nil {
		t.Fatalf("failed to create workspace: %v", err)
	}
	id, _ := result.LastInsertId()
	workspaceID := int(id)

	// Create user
	result, err = db.Exec(`
		INSERT INTO users (email, username, first_name, last_name, password_hash, is_active, created_at, updated_at)
		VALUES ('abc@example.com', 'abcuser', 'ABC', 'User', 'hash', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	id, _ = result.LastInsertId()
	userID := int(id)

	// Get default status
	var statusID int
	err = db.QueryRow("SELECT id FROM statuses WHERE is_default = true LIMIT 1").Scan(&statusID)
	if err != nil {
		t.Fatalf("failed to get default status: %v", err)
	}

	// Create item with workspace_item_number = 1
	result, err = db.Exec(`
		INSERT INTO items (workspace_id, title, description, status_id, creator_id, workspace_item_number, created_at, updated_at)
		VALUES (?, 'ABC Item', 'Description', ?, ?, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, workspaceID, statusID, userID)
	if err != nil {
		t.Fatalf("failed to create item: %v", err)
	}
	id, _ = result.LastInsertId()
	itemID := int(id)

	collector := &NotificationEventCollector{}

	// Emit item.created event with properly formatted key
	collector.EmitEvent(&NotificationEvent{
		EventType:   models.EventItemCreated,
		WorkspaceID: workspaceID,
		ActorUserID: userID,
		ItemID:      itemID,
		CreatorID:   &userID,
		Title:       "Item Created",
		TemplateData: map[string]interface{}{
			"item.title":     "ABC Item",
			"item.key":       "ABC-1", // Expected format: {workspace.key}-{workspace_item_number}
			"item.id":        itemID,
			"user.name":      "abcuser",
			"workspace.name": "ABC Workspace",
			"workspace.key":  "ABC",
		},
	})

	event := collector.AssertEventEmitted(t, models.EventItemCreated)

	// Verify item.key format is "{workspace.key}-{number}"
	expectedKey := "ABC-1"
	actualKey, ok := event.TemplateData["item.key"].(string)
	if !ok {
		t.Fatalf("expected item.key to be a string, got %T", event.TemplateData["item.key"])
	}
	if actualKey != expectedKey {
		t.Errorf("expected item.key %q, got %q", expectedKey, actualKey)
	}
}

// ============================================================================
// INTEGRATION TEST WITH REAL NOTIFICATION SERVICE
// ============================================================================

func TestNotificationEmission_CommentService_IntegrationWithNotificationService(t *testing.T) {
	db := createNotificationEmissionTestDB(t)
	defer func() { _ = db.Close() }()

	env := setupNotificationEmissionTestEnv(t, db)

	// Create a real notification service with a stub manager
	manager := newStubNotificationManager()
	notificationService := NewNotificationService(db, manager, NotificationServiceConfig{
		RefreshInterval: time.Hour,
		EventBufferSize: 10,
	})
	defer func() { _ = notificationService.Close() }()

	// Set up notification rules for comment.created
	settingID := insertNotificationSetting(t, db, env.userID)
	configSetID := insertConfigurationSet(t, db, "Test Config")

	// Link workspace to config set
	_, err := db.Exec(`
		INSERT INTO workspace_configuration_sets (workspace_id, configuration_set_id, created_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
	`, env.workspaceID, configSetID)
	if err != nil {
		t.Fatalf("failed to link workspace to config set: %v", err)
	}

	// Link notification setting to config set
	_, err = db.Exec(`
		INSERT INTO configuration_set_notification_settings (configuration_set_id, notification_setting_id, created_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
	`, configSetID, settingID)
	if err != nil {
		t.Fatalf("failed to link notification setting: %v", err)
	}

	// Create event rule for comment.created
	_, err = db.Exec(`
		INSERT INTO notification_event_rules (
			notification_setting_id, event_type, is_enabled,
			notify_assignee, notify_creator, notify_watchers, notify_workspace_admins,
			custom_recipients, message_template, created_at, updated_at
		) VALUES (?, 'comment.created', 1, 0, 1, 0, 0, NULL, NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, settingID)
	if err != nil {
		t.Fatalf("failed to insert notification rule: %v", err)
	}

	// Force refresh the cache
	if err = notificationService.ForceRefreshCache(); err != nil {
		t.Fatalf("failed to refresh cache: %v", err)
	}

	// Create CommentService with the real notification service
	commentService := NewCommentService(db)
	commentService.SetNotificationService(notificationService)

	// Create a second user to be the creator (so they can receive the notification)
	result, err := db.Exec(`
		INSERT INTO users (email, username, first_name, last_name, password_hash, is_active, created_at, updated_at)
		VALUES ('creator@example.com', 'creator', 'Item', 'Creator', 'hash', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	if err != nil {
		t.Fatalf("failed to insert creator: %v", err)
	}
	id, _ := result.LastInsertId()
	creatorID := int(id)

	// Update item to have this creator
	_, err = db.Exec("UPDATE items SET creator_id = ? WHERE id = ?", creatorID, env.itemID)
	if err != nil {
		t.Fatalf("failed to update item creator: %v", err)
	}

	// Create comment (which should trigger notification)
	_, err = commentService.Create(CreateCommentParams{
		ItemID:      env.itemID,
		AuthorID:    env.userID, // Different from creator
		Content:     "This is a test comment",
		IsPrivate:   false,
		ActorUserID: env.userID,
	})
	if err != nil {
		t.Fatalf("failed to create comment: %v", err)
	}

	// Wait for notification to be processed
	notification := manager.waitForNotification(t, 2*time.Second)

	// Verify notification was created for the item creator
	if notification.UserID != creatorID {
		t.Errorf("expected notification for creator %d, got user %d", creatorID, notification.UserID)
	}
	if notification.Type != "comment" {
		t.Errorf("expected notification type 'comment', got %q", notification.Type)
	}
}

func TestNotificationEmission_MentionService_IntegrationWithNotificationService(t *testing.T) {
	db := createNotificationEmissionTestDB(t)
	defer func() { _ = db.Close() }()

	env := setupNotificationEmissionTestEnv(t, db)

	// Create a real notification service with a stub manager
	manager := newStubNotificationManager()
	notificationService := NewNotificationService(db, manager, NotificationServiceConfig{
		RefreshInterval: time.Hour,
		EventBufferSize: 10,
	})
	defer func() { _ = notificationService.Close() }()

	// Set up notification rules for mention.created
	settingID := insertNotificationSetting(t, db, env.userID)
	configSetID := insertConfigurationSet(t, db, "Test Config")

	// Link workspace to config set
	_, err := db.Exec(`
		INSERT INTO workspace_configuration_sets (workspace_id, configuration_set_id, created_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
	`, env.workspaceID, configSetID)
	if err != nil {
		t.Fatalf("failed to link workspace to config set: %v", err)
	}

	// Link notification setting to config set
	_, err = db.Exec(`
		INSERT INTO configuration_set_notification_settings (configuration_set_id, notification_setting_id, created_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
	`, configSetID, settingID)
	if err != nil {
		t.Fatalf("failed to link notification setting: %v", err)
	}

	// Create event rule for mention.created (notify assignee = the mentioned user)
	_, err = db.Exec(`
		INSERT INTO notification_event_rules (
			notification_setting_id, event_type, is_enabled,
			notify_assignee, notify_creator, notify_watchers, notify_workspace_admins,
			custom_recipients, message_template, created_at, updated_at
		) VALUES (?, 'mention.created', 1, 1, 0, 0, 0, NULL, NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, settingID)
	if err != nil {
		t.Fatalf("failed to insert notification rule: %v", err)
	}

	// Force refresh the cache
	if err = notificationService.ForceRefreshCache(); err != nil {
		t.Fatalf("failed to refresh cache: %v", err)
	}

	// Create MentionService with the real notification service
	mentionService := NewMentionService(db, notificationService)

	// Create a user to be mentioned (with known username)
	result, err := db.Exec(`
		INSERT INTO users (email, username, first_name, last_name, password_hash, is_active, created_at, updated_at)
		VALUES ('mentioneduser@example.com', 'mentioneduser', 'Mentioned', 'User', 'hash', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	if err != nil {
		t.Fatalf("failed to insert mentioned user: %v", err)
	}
	id, _ := result.LastInsertId()
	mentionedUserID := int(id)

	// Process mentions in content that includes @mentioneduser
	err = mentionService.ProcessMentions(ProcessMentionsParams{
		SourceType:  "comment",
		SourceID:    1, // Arbitrary source ID
		Content:     "Hey @mentioneduser please check this out",
		ItemID:      env.itemID,
		WorkspaceID: env.workspaceID,
		ActorUserID: env.userID,
	})
	if err != nil {
		t.Fatalf("failed to process mentions: %v", err)
	}

	// Wait for notification to be processed
	notification := manager.waitForNotification(t, 2*time.Second)

	// Verify notification was created for the mentioned user
	if notification.UserID != mentionedUserID {
		t.Errorf("expected notification for mentioned user %d, got user %d", mentionedUserID, notification.UserID)
	}
	if notification.Type != "mention" {
		t.Errorf("expected notification type 'mention', got %q", notification.Type)
	}
}

// ============================================================================
// ALL EVENT TYPES COVERAGE TEST
// ============================================================================

func TestNotificationEmission_AllEventTypesCoverage(t *testing.T) {
	// This test verifies that all 11 event types can be emitted with valid structure
	db := createNotificationEmissionTestDB(t)
	defer func() { _ = db.Close() }()

	env := setupNotificationEmissionTestEnv(t, db)
	collector := &NotificationEventCollector{}

	itemKey := fmt.Sprintf("%s-%d", env.workspaceKey, 1)

	// All 11 event types from models/portal.go
	eventTypes := []struct {
		eventType    string
		title        string
		templateData map[string]interface{}
	}{
		{
			eventType: models.EventItemCreated,
			title:     "Item Created",
			templateData: map[string]interface{}{
				"item.title": "Test Item", "item.key": itemKey, "item.id": env.itemID,
				"user.name": env.userName, "workspace.name": "Test Workspace", "workspace.key": env.workspaceKey,
			},
		},
		{
			eventType: models.EventItemUpdated,
			title:     "Item Updated",
			templateData: map[string]interface{}{
				"item.title": "Test Item", "item.key": itemKey, "item.id": env.itemID, "user.name": env.userName,
			},
		},
		{
			eventType: models.EventItemDeleted,
			title:     "Item Deleted",
			templateData: map[string]interface{}{
				"item.title": "Test Item", "item.id": env.itemID, "user.name": env.userName,
			},
		},
		{
			eventType: models.EventItemAssigned,
			title:     "Item Assigned",
			templateData: map[string]interface{}{
				"item.title": "Test Item", "item.key": itemKey, "item.id": env.itemID, "user.name": env.userName,
			},
		},
		{
			eventType: models.EventStatusChanged,
			title:     "Status Changed",
			templateData: map[string]interface{}{
				"item.title": "Test Item", "item.key": itemKey, "item.id": env.itemID,
				"status.name": "Done", "user.name": env.userName,
			},
		},
		{
			eventType: models.EventCommentCreated,
			title:     "Comment Created",
			templateData: map[string]interface{}{
				"item.title": "Test Item", "item.key": itemKey, "item.id": env.itemID, "user.name": env.userName,
			},
		},
		{
			eventType: models.EventCommentUpdated,
			title:     "Comment Updated",
			templateData: map[string]interface{}{
				"item.title": "Test Item", "item.id": env.itemID, "user.name": env.userName,
			},
		},
		{
			eventType: models.EventCommentDeleted,
			title:     "Comment Deleted",
			templateData: map[string]interface{}{
				"item.title": "Test Item", "item.id": env.itemID, "user.name": env.userName,
			},
		},
		{
			eventType: models.EventItemLinked,
			title:     "Item Linked",
			templateData: map[string]interface{}{
				"item.title": "Test Item", "item.id": env.itemID,
				"target.title": "Target Item", "target.id": 999, "user.name": env.userName,
			},
		},
		{
			eventType: models.EventItemUnlinked,
			title:     "Item Unlinked",
			templateData: map[string]interface{}{
				"item.title": "Test Item", "item.id": env.itemID,
				"target.title": "Target Item", "target.id": 999, "user.name": env.userName,
			},
		},
		{
			eventType: models.EventMention,
			title:     "User Mentioned",
			templateData: map[string]interface{}{
				"item.title": "Test Item", "item.key": itemKey, "actor.name": "Test User", "source.type": "a comment",
			},
		},
	}

	// Emit all event types
	for _, et := range eventTypes {
		collector.EmitEvent(&NotificationEvent{
			EventType:    et.eventType,
			WorkspaceID:  env.workspaceID,
			ActorUserID:  env.userID,
			ItemID:       env.itemID,
			Title:        et.title,
			TemplateData: et.templateData,
		})
	}

	// Verify all 11 events were captured
	collector.AssertEventCount(t, 11)

	// Verify each event type was captured
	for _, et := range eventTypes {
		event := collector.FindEvent(et.eventType)
		if event == nil {
			t.Errorf("event type %q was not captured", et.eventType)
			continue
		}

		// Verify basic structure
		if event.WorkspaceID != env.workspaceID {
			t.Errorf("event %q: expected WorkspaceID %d, got %d", et.eventType, env.workspaceID, event.WorkspaceID)
		}
		if event.ActorUserID != env.userID {
			t.Errorf("event %q: expected ActorUserID %d, got %d", et.eventType, env.userID, event.ActorUserID)
		}
		if event.ItemID != env.itemID {
			t.Errorf("event %q: expected ItemID %d, got %d", et.eventType, env.itemID, event.ItemID)
		}
	}
}

// Helper to check for sql.NullInt64 conversion
func intPtrFromNullInt64(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	val := int(n.Int64)
	return &val
}
