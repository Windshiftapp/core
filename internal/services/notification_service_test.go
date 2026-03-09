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

type fullNotificationTestEnv struct {
	workspaceID int
	configSetID int
	actorUserID int
	assigneeID  int
	creatorID   int // item creator, different from actor
	watcherID   int // watches the item
	itemID      int // item for watches/creator context
}

func seedFullNotificationEnv(t *testing.T, db database.Database) fullNotificationTestEnv {
	t.Helper()

	actorID := insertUser(t, db, "actor@example.com", "actor")
	assigneeID := insertUser(t, db, "assignee@example.com", "assignee")
	creatorID := insertUser(t, db, "creator@example.com", "creator")
	watcherID := insertUser(t, db, "watcher@example.com", "watcher")

	workspaceID := insertWorkspace(t, db, "Full Test Workspace", "FTW")
	configSetID := insertConfigurationSet(t, db, "Full Test Config")

	_, err := db.Exec(`
		INSERT INTO workspace_configuration_sets (workspace_id, configuration_set_id, created_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
	`, workspaceID, configSetID)
	if err != nil {
		t.Fatalf("failed to link workspace and configuration set: %v", err)
	}

	// Insert an item so we can create watches against it
	result, err := db.Exec(`
		INSERT INTO items (workspace_id, title, creator_id, assignee_id, created_at, updated_at)
		VALUES (?, 'Test Item', ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, workspaceID, creatorID, assigneeID)
	if err != nil {
		t.Fatalf("failed to insert item: %v", err)
	}
	itemID64, _ := result.LastInsertId()
	itemID := int(itemID64)

	// Add watcher on the item
	_, err = db.Exec(`
		INSERT INTO item_watches (item_id, user_id, is_active, watch_reason, created_at, updated_at)
		VALUES (?, ?, 1, 'test', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, itemID, watcherID)
	if err != nil {
		t.Fatalf("failed to insert item watch: %v", err)
	}

	// Make assignee an Administrator (for notify_workspace_admins tests)
	var roleID int
	err = db.QueryRow(`SELECT id FROM workspace_roles WHERE name = 'Administrator'`).Scan(&roleID)
	if err != nil {
		// Insert the Administrator role if it doesn't exist
		res, insertErr := db.Exec(`
			INSERT INTO workspace_roles (name, description, is_system, created_at, updated_at)
			VALUES ('Administrator', 'Workspace admin', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`)
		if insertErr != nil {
			t.Fatalf("failed to insert Administrator role: %v", insertErr)
		}
		id64, _ := res.LastInsertId()
		roleID = int(id64)
	}

	_, err = db.Exec(`
		INSERT INTO user_workspace_roles (user_id, workspace_id, role_id, granted_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
	`, assigneeID, workspaceID, roleID)
	if err != nil {
		t.Fatalf("failed to insert user workspace role: %v", err)
	}

	return fullNotificationTestEnv{
		workspaceID: workspaceID,
		configSetID: configSetID,
		actorUserID: actorID,
		assigneeID:  assigneeID,
		creatorID:   creatorID,
		watcherID:   watcherID,
		itemID:      itemID,
	}
}

func attachAllEventRulesWithMixedFlags(t *testing.T, db database.Database, env fullNotificationTestEnv) {
	t.Helper()

	settingID := insertNotificationSetting(t, db, env.actorUserID)
	_, err := db.Exec(`
		INSERT INTO configuration_set_notification_settings (configuration_set_id, notification_setting_id, created_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
	`, env.configSetID, settingID)
	if err != nil {
		t.Fatalf("failed to link notification setting: %v", err)
	}

	type ruleSpec struct {
		eventType           string
		notifyAssignee      int
		notifyCreator       int
		notifyWatchers      int
		notifyWorkspaceAdmins int
	}

	rules := []ruleSpec{
		{models.EventItemCreated, 1, 0, 0, 0},
		{models.EventItemUpdated, 1, 0, 0, 0},
		{models.EventItemDeleted, 1, 0, 0, 0},
		{models.EventItemAssigned, 1, 0, 0, 0},
		{models.EventCommentCreated, 0, 1, 0, 0},
		{models.EventCommentUpdated, 0, 1, 0, 0},
		{models.EventCommentDeleted, 0, 1, 0, 0},
		{models.EventItemLinked, 0, 0, 1, 0},
		{models.EventItemUnlinked, 0, 0, 1, 0},
		{models.EventStatusChanged, 0, 0, 0, 1},
		{models.EventMention, 1, 0, 0, 0},
	}

	for _, r := range rules {
		_, err := db.Exec(`
			INSERT INTO notification_event_rules (
				notification_setting_id, event_type, is_enabled,
				notify_assignee, notify_creator, notify_watchers, notify_workspace_admins,
				custom_recipients, message_template, created_at, updated_at
			) VALUES (?, ?, 1, ?, ?, ?, ?, NULL, NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, settingID, r.eventType, r.notifyAssignee, r.notifyCreator, r.notifyWatchers, r.notifyWorkspaceAdmins)
		if err != nil {
			t.Fatalf("failed to insert rule for %s: %v", r.eventType, err)
		}
	}
}

func TestNotificationService_AllEventTypes(t *testing.T) {
	db := createTestDB(t)
	defer func() { _ = db.Close() }()

	env := seedFullNotificationEnv(t, db)
	attachAllEventRulesWithMixedFlags(t, db, env)

	manager := newStubNotificationManager()
	service := NewNotificationService(db, manager, NotificationServiceConfig{
		RefreshInterval: time.Hour,
		EventBufferSize: 100,
	})
	defer func() { _ = service.Close() }()

	type testCase struct {
		name            string
		eventType       string
		title           string
		templateData    map[string]interface{}
		assigneeID      *int
		creatorID       *int
		expectedUserID  int
		expectedType    string
		expectedContains string
	}

	cases := []testCase{
		{
			name:            "item.created notifies assignee",
			eventType:       models.EventItemCreated,
			title:           "Item Created",
			templateData:    map[string]interface{}{"item.key": "FTW-1", "item.title": "Test Item"},
			assigneeID:      &env.assigneeID,
			creatorID:       &env.creatorID,
			expectedUserID:  env.assigneeID,
			expectedType:    "info",
			expectedContains: "New work item created",
		},
		{
			name:            "item.updated notifies assignee",
			eventType:       models.EventItemUpdated,
			title:           "Item Updated",
			templateData:    map[string]interface{}{"item.key": "FTW-1", "item.title": "Test Item"},
			assigneeID:      &env.assigneeID,
			creatorID:       &env.creatorID,
			expectedUserID:  env.assigneeID,
			expectedType:    "info",
			expectedContains: "Work item updated",
		},
		{
			name:            "item.deleted notifies assignee with warning",
			eventType:       models.EventItemDeleted,
			title:           "Item Deleted",
			templateData:    map[string]interface{}{"item.key": "FTW-1", "item.title": "Test Item"},
			assigneeID:      &env.assigneeID,
			creatorID:       &env.creatorID,
			expectedUserID:  env.assigneeID,
			expectedType:    "warning",
			expectedContains: "Work item deleted",
		},
		{
			name:            "item.assigned notifies assignee",
			eventType:       models.EventItemAssigned,
			title:           "Item Assigned",
			templateData:    map[string]interface{}{"item.key": "FTW-1", "item.title": "Test Item"},
			assigneeID:      &env.assigneeID,
			creatorID:       &env.creatorID,
			expectedUserID:  env.assigneeID,
			expectedType:    "assignment",
			expectedContains: "You have been assigned to",
		},
		{
			name:            "comment.created notifies creator",
			eventType:       models.EventCommentCreated,
			title:           "Comment Created",
			templateData:    map[string]interface{}{"item.key": "FTW-1", "item.title": "Test Item", "user.name": "Actor"},
			assigneeID:      &env.assigneeID,
			creatorID:       &env.creatorID,
			expectedUserID:  env.creatorID,
			expectedType:    "comment",
			expectedContains: "New comment added by",
		},
		{
			name:            "comment.updated notifies creator",
			eventType:       models.EventCommentUpdated,
			title:           "Comment Updated",
			templateData:    map[string]interface{}{"item.key": "FTW-1", "item.title": "Test Item", "user.name": "Actor"},
			assigneeID:      &env.assigneeID,
			creatorID:       &env.creatorID,
			expectedUserID:  env.creatorID,
			expectedType:    "comment",
			expectedContains: "Comment updated by",
		},
		{
			name:            "comment.deleted notifies creator",
			eventType:       models.EventCommentDeleted,
			title:           "Comment Deleted",
			templateData:    map[string]interface{}{"item.key": "FTW-1", "item.title": "Test Item", "user.name": "Actor"},
			assigneeID:      &env.assigneeID,
			creatorID:       &env.creatorID,
			expectedUserID:  env.creatorID,
			expectedType:    "comment",
			expectedContains: "Comment deleted by",
		},
		{
			name:            "item.linked notifies watcher",
			eventType:       models.EventItemLinked,
			title:           "Item Linked",
			templateData:    map[string]interface{}{"item.key": "FTW-1", "item.title": "Test Item"},
			assigneeID:      &env.assigneeID,
			creatorID:       &env.creatorID,
			expectedUserID:  env.watcherID,
			expectedType:    "info",
			expectedContains: "Work items linked",
		},
		{
			name:            "item.unlinked notifies watcher",
			eventType:       models.EventItemUnlinked,
			title:           "Item Unlinked",
			templateData:    map[string]interface{}{"item.key": "FTW-1", "item.title": "Test Item"},
			assigneeID:      &env.assigneeID,
			creatorID:       &env.creatorID,
			expectedUserID:  env.watcherID,
			expectedType:    "info",
			expectedContains: "Work item link removed",
		},
		{
			name:            "status.changed notifies workspace admins",
			eventType:       models.EventStatusChanged,
			title:           "Status Changed",
			templateData:    map[string]interface{}{"item.key": "FTW-1", "item.title": "Test Item", "status.name": "In Progress"},
			assigneeID:      &env.assigneeID,
			creatorID:       &env.creatorID,
			expectedUserID:  env.assigneeID, // assignee is the admin
			expectedType:    "status_change",
			expectedContains: "Status changed to",
		},
		{
			name:            "mention.created notifies assignee",
			eventType:       models.EventMention,
			title:           "Mention",
			templateData:    map[string]interface{}{"item.key": "FTW-1", "item.title": "Test Item", "actor.name": "Actor", "source.type": "comment"},
			assigneeID:      &env.assigneeID,
			creatorID:       &env.creatorID,
			expectedUserID:  env.assigneeID,
			expectedType:    "mention",
			expectedContains: "mentioned you",
		},
	}

	expectedURL := fmt.Sprintf("/workspaces/%d/items/%d", env.workspaceID, env.itemID)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service.EmitEvent(&NotificationEvent{
				EventType:    tc.eventType,
				WorkspaceID:  env.workspaceID,
				ActorUserID:  env.actorUserID,
				ItemID:       env.itemID,
				AssigneeID:   tc.assigneeID,
				CreatorID:    tc.creatorID,
				Title:        tc.title,
				TemplateData: tc.templateData,
			})

			notification := manager.waitForNotification(t, 2*time.Second)

			if notification.UserID != tc.expectedUserID {
				t.Errorf("expected UserID %d, got %d", tc.expectedUserID, notification.UserID)
			}
			if notification.Type != tc.expectedType {
				t.Errorf("expected Type %q, got %q", tc.expectedType, notification.Type)
			}
			if !strings.Contains(notification.Message, tc.expectedContains) {
				t.Errorf("expected Message to contain %q, got %q", tc.expectedContains, notification.Message)
			}
			if notification.ActionURL != expectedURL {
				t.Errorf("expected ActionURL %q, got %q", expectedURL, notification.ActionURL)
			}
		})
	}
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
