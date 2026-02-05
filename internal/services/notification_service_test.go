package services

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

type stubNotificationManager struct {
	mu             sync.Mutex
	notifications  []models.Notification
	notificationCh chan models.Notification
}

func newStubNotificationManager() *stubNotificationManager {
	return &stubNotificationManager{
		notificationCh: make(chan models.Notification, 10),
	}
}

func (s *stubNotificationManager) AddNotification(notification models.Notification) error {
	s.mu.Lock()
	s.notifications = append(s.notifications, notification)
	s.mu.Unlock()
	s.notificationCh <- notification
	return nil
}

func (s *stubNotificationManager) waitForNotification(t *testing.T, timeout time.Duration) models.Notification {
	t.Helper()

	select {
	case n := <-s.notificationCh:
		return n
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for notification")
		return models.Notification{}
	}
}

func (s *stubNotificationManager) expectNoNotification(t *testing.T, timeout time.Duration) {
	t.Helper()

	select {
	case n := <-s.notificationCh:
		t.Fatalf("expected no notification, but received %+v", n)
	case <-time.After(timeout):
		// expected outcome
	}
}

func TestNotificationService_AssignmentIncludesItemKey(t *testing.T) {
	db := createTestDB(t)
	defer func() { _ = db.Close() }()

	env := seedBaseNotificationEnv(t, db)
	attachNotificationSettingAndRule(t, db, env)

	manager := newStubNotificationManager()
	service := NewNotificationService(db, manager, NotificationServiceConfig{
		RefreshInterval: time.Hour,
		EventBufferSize: 10,
	})
	defer func() { _ = service.Close() }()

	itemID := 42
	itemKey := "TST-42"
	service.EmitEvent(&NotificationEvent{
		EventType:   models.EventItemAssigned,
		WorkspaceID: env.workspaceID,
		ActorUserID: env.actorUserID,
		ItemID:      itemID,
		AssigneeID:  &env.assigneeID,
		CreatorID:   &env.actorUserID,
		Title:       "Item Assigned",
		TemplateData: map[string]interface{}{
			"item.key":   itemKey,
			"item.title": "Important Item",
		},
	})

	notification := manager.waitForNotification(t, 2*time.Second)
	if notification.UserID != env.assigneeID {
		t.Fatalf("expected notification for user %d, got %d", env.assigneeID, notification.UserID)
	}
	if notification.Type != "assignment" {
		t.Fatalf("expected type 'assignment', got %s", notification.Type)
	}
	if !strings.Contains(notification.Message, itemKey) {
		t.Fatalf("expected message to contain %q, got %q", itemKey, notification.Message)
	}

	expectedURL := fmt.Sprintf("/workspaces/%d/items/%d", env.workspaceID, itemID)
	if notification.ActionURL != expectedURL {
		t.Fatalf("expected action URL %q, got %q", expectedURL, notification.ActionURL)
	}
}

func TestNotificationService_ForceRefreshLoadsNewAssignments(t *testing.T) {
	db := createTestDB(t)
	defer func() { _ = db.Close() }()

	env := seedBaseNotificationEnv(t, db)

	manager := newStubNotificationManager()
	service := NewNotificationService(db, manager, NotificationServiceConfig{
		RefreshInterval: time.Hour,
		EventBufferSize: 10,
	})
	defer func() { _ = service.Close() }()

	itemID := 55

	// Emit event before notification settings are linked - no notification expected
	service.EmitEvent(&NotificationEvent{
		EventType:   models.EventItemAssigned,
		WorkspaceID: env.workspaceID,
		ActorUserID: env.actorUserID,
		ItemID:      itemID,
		AssigneeID:  &env.assigneeID,
		Title:       "Assignment",
		TemplateData: map[string]interface{}{
			"item.key": "TST-55",
		},
	})
	manager.expectNoNotification(t, 200*time.Millisecond)

	attachNotificationSettingAndRule(t, db, env)
	if err := service.ForceRefreshCache(); err != nil {
		t.Fatalf("ForceRefreshCache failed: %v", err)
	}

	service.EmitEvent(&NotificationEvent{
		EventType:   models.EventItemAssigned,
		WorkspaceID: env.workspaceID,
		ActorUserID: env.actorUserID,
		ItemID:      itemID,
		AssigneeID:  &env.assigneeID,
		Title:       "Assignment",
		TemplateData: map[string]interface{}{
			"item.key": "TST-55",
		},
	})

	notification := manager.waitForNotification(t, 2*time.Second)
	if notification.UserID != env.assigneeID {
		t.Fatalf("expected notification for user %d, got %d", env.assigneeID, notification.UserID)
	}
}

type notificationTestEnv struct {
	workspaceID int
	configSetID int
	actorUserID int
	assigneeID  int
}

func createTestDB(t *testing.T) database.Database {
	t.Helper()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "notifications.db")
	db, err := database.NewSQLiteDB(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	if err := db.Initialize(); err != nil {
		t.Fatalf("failed to initialize test database: %v", err)
	}

	return db
}

func seedBaseNotificationEnv(t *testing.T, db database.Database) notificationTestEnv {
	t.Helper()

	adminID := insertUser(t, db, "admin@example.com", "admin")
	assigneeID := insertUser(t, db, "user@example.com", "assignee")

	workspaceID := insertWorkspace(t, db, "Test Workspace", "TST")
	configSetID := insertConfigurationSet(t, db, "Test Config")

	_, err := db.Exec(`
		INSERT INTO workspace_configuration_sets (workspace_id, configuration_set_id, created_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
	`, workspaceID, configSetID)
	if err != nil {
		t.Fatalf("failed to link workspace and configuration set: %v", err)
	}

	return notificationTestEnv{
		workspaceID: workspaceID,
		configSetID: configSetID,
		actorUserID: adminID,
		assigneeID:  assigneeID,
	}
}

func attachNotificationSettingAndRule(t *testing.T, db database.Database, env notificationTestEnv) {
	t.Helper()

	settingID := insertNotificationSetting(t, db, env.actorUserID)
	_, err := db.Exec(`
		INSERT INTO configuration_set_notification_settings (configuration_set_id, notification_setting_id, created_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
	`, env.configSetID, settingID)
	if err != nil {
		t.Fatalf("failed to link notification setting: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO notification_event_rules (
			notification_setting_id, event_type, is_enabled,
			notify_assignee, notify_creator, notify_watchers, notify_workspace_admins,
			custom_recipients, message_template, created_at, updated_at
		) VALUES (?, 'item.assigned', 1, 1, 0, 0, 0, NULL, NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, settingID)
	if err != nil {
		t.Fatalf("failed to insert notification rule: %v", err)
	}
}

func insertUser(t *testing.T, db database.Database, email, username string) int {
	t.Helper()
	result, err := db.Exec(`
		INSERT INTO users (email, username, first_name, last_name, password_hash, is_active, created_at, updated_at)
		VALUES (?, ?, 'Test', 'User', 'hash', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, email, username)
	if err != nil {
		t.Fatalf("failed to insert user: %v", err)
	}
	id, _ := result.LastInsertId()
	return int(id)
}

func insertWorkspace(t *testing.T, db database.Database, name, key string) int {
	t.Helper()
	result, err := db.Exec(`
		INSERT INTO workspaces (name, key, description, active, is_personal, created_at, updated_at)
		VALUES (?, ?, 'Test workspace', 1, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, name, key)
	if err != nil {
		t.Fatalf("failed to insert workspace: %v", err)
	}
	id, _ := result.LastInsertId()
	return int(id)
}

func insertConfigurationSet(t *testing.T, db database.Database, name string) int {
	t.Helper()
	result, err := db.Exec(`
		INSERT INTO configuration_sets (name, description, is_default, created_at, updated_at)
		VALUES (?, 'Test config set', 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, name)
	if err != nil {
		t.Fatalf("failed to insert configuration set: %v", err)
	}
	id, _ := result.LastInsertId()
	return int(id)
}

func insertNotificationSetting(t *testing.T, db database.Database, creatorID int) int {
	t.Helper()
	result, err := db.Exec(`
		INSERT INTO notification_settings (name, description, is_active, created_by, created_at, updated_at)
		VALUES (?, 'Test notifications', ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, "Test Settings", 1, creatorID)
	if err != nil {
		t.Fatalf("failed to insert notification setting: %v", err)
	}
	id, _ := result.LastInsertId()
	return int(id)
}
