package factory

import (
	"fmt"
	"sync/atomic"

	"windshift/internal/database"
	"windshift/internal/services"
)

// counter provides unique values for default test data
var counter int64

func nextID() int64 {
	return atomic.AddInt64(&counter, 1)
}

// TestFactory provides black-box test data creation using service methods
type TestFactory struct {
	db database.Database
}

// NewTestFactory creates a new TestFactory
func NewTestFactory(db database.Database) *TestFactory {
	return &TestFactory{db: db}
}

// CreateUserOpts contains options for creating a test user
type CreateUserOpts struct {
	Email     string // default: "testuser{n}@example.com"
	Username  string // default: "testuser{n}"
	FirstName string // default: "Test"
	LastName  string // default: "User"
	Password  string // default: "password123"
	IsActive  bool   // default: true
}

// CreateUser creates a test user using direct SQL (no UserService.Create exists)
func (f *TestFactory) CreateUser(opts *CreateUserOpts) (userID int, err error) {
	n := nextID()

	// Apply defaults
	email := fmt.Sprintf("testuser%d@example.com", n)
	username := fmt.Sprintf("testuser%d", n)
	firstName := "Test"
	lastName := "User"
	password := "password123"
	isActive := true

	if opts != nil {
		if opts.Email != "" {
			email = opts.Email
		}
		if opts.Username != "" {
			username = opts.Username
		}
		if opts.FirstName != "" {
			firstName = opts.FirstName
		}
		if opts.LastName != "" {
			lastName = opts.LastName
		}
		if opts.Password != "" {
			password = opts.Password
		}
		// IsActive defaults to true, but opts can override it
		isActive = opts.IsActive
		// Handle case where opts is provided but IsActive not explicitly set
		if opts.Email == "" && opts.Username == "" && opts.FirstName == "" &&
			opts.LastName == "" && opts.Password == "" && !opts.IsActive {
			isActive = true // Default when opts provided with all zero values
		}
	}

	result, err := f.db.Exec(`
		INSERT INTO users (username, email, first_name, last_name, password_hash, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, username, email, firstName, lastName, password, isActive)
	if err != nil {
		return 0, fmt.Errorf("failed to create user: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get user id: %w", err)
	}

	return int(id), nil
}

// CreateWorkspaceOpts contains options for creating a test workspace
type CreateWorkspaceOpts struct {
	Name        string // default: "Test Workspace {n}"
	Key         string // default: "TST{n}"
	Description string // default: "Test workspace"
	CreatorID   int    // REQUIRED
}

// CreateWorkspace creates a test workspace using WorkspaceService.Create
func (f *TestFactory) CreateWorkspace(opts CreateWorkspaceOpts) (workspaceID int, err error) {
	if opts.CreatorID == 0 {
		return 0, fmt.Errorf("CreatorID is required")
	}

	n := nextID()

	// Apply defaults
	name := fmt.Sprintf("Test Workspace %d", n)
	key := fmt.Sprintf("TST%d", n)
	description := "Test workspace"

	if opts.Name != "" {
		name = opts.Name
	}
	if opts.Key != "" {
		key = opts.Key
	}
	if opts.Description != "" {
		description = opts.Description
	}

	service := services.NewWorkspaceService(f.db)
	result, err := service.Create(services.CreateWorkspaceParams{
		Name:        name,
		Key:         key,
		Description: description,
		CreatorID:   opts.CreatorID,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to create workspace: %w", err)
	}

	return result.Workspace.ID, nil
}

// CreateItemOpts contains options for creating a test item
type CreateItemOpts struct {
	WorkspaceID int    // REQUIRED
	Title       string // default: "Test Item {n}"
	Description string // default: ""
	StatusID    *int   // default: nil (uses workflow initial)
	PriorityID  *int   // default: nil (uses DB default)
	ItemTypeID  *int   // default: nil
	ParentID    *int   // default: nil
	AssigneeID  *int   // default: nil
	CreatorID   *int   // default: nil
}

// CreateItem creates a test item using services.CreateItem
func (f *TestFactory) CreateItem(opts CreateItemOpts) (itemID int, err error) {
	if opts.WorkspaceID == 0 {
		return 0, fmt.Errorf("WorkspaceID is required")
	}

	n := nextID()

	// Apply defaults
	title := fmt.Sprintf("Test Item %d", n)
	if opts.Title != "" {
		title = opts.Title
	}

	id, err := services.CreateItem(f.db, services.ItemCreationParams{
		WorkspaceID: opts.WorkspaceID,
		Title:       title,
		Description: opts.Description,
		StatusID:    opts.StatusID,
		PriorityID:  opts.PriorityID,
		ItemTypeID:  opts.ItemTypeID,
		ParentID:    opts.ParentID,
		AssigneeID:  opts.AssigneeID,
		CreatorID:   opts.CreatorID,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to create item: %w", err)
	}

	return int(id), nil
}

// CreateCommentOpts contains options for creating a test comment
type CreateCommentOpts struct {
	ItemID    int    // REQUIRED
	AuthorID  int    // REQUIRED
	Content   string // default: "Test comment"
	IsPrivate bool   // default: false
}

// CreateComment creates a test comment using CommentService.Create
func (f *TestFactory) CreateComment(opts CreateCommentOpts) (commentID int, err error) {
	if opts.ItemID == 0 {
		return 0, fmt.Errorf("ItemID is required")
	}
	if opts.AuthorID == 0 {
		return 0, fmt.Errorf("AuthorID is required")
	}

	// Apply defaults
	content := "Test comment"
	if opts.Content != "" {
		content = opts.Content
	}

	service := services.NewCommentService(f.db)
	result, err := service.Create(services.CreateCommentParams{
		ItemID:      opts.ItemID,
		AuthorID:    opts.AuthorID,
		Content:     content,
		IsPrivate:   opts.IsPrivate,
		ActorUserID: opts.AuthorID,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to create comment: %w", err)
	}

	return int(result.CommentID), nil
}

// CreateUserAndWorkspace composes CreateUser + CreateWorkspace
func (f *TestFactory) CreateUserAndWorkspace() (userID, workspaceID int, err error) {
	userID, err = f.CreateUser(nil) // uses defaults
	if err != nil {
		return 0, 0, err
	}
	workspaceID, err = f.CreateWorkspace(CreateWorkspaceOpts{CreatorID: userID})
	return userID, workspaceID, err
}

// TestEnv contains common test environment data
type TestEnv struct {
	UserID      int
	WorkspaceID int
	ItemID      int
}

// CreateFullTestEnv composes CreateUser + CreateWorkspace + CreateItem
func (f *TestFactory) CreateFullTestEnv() (*TestEnv, error) {
	userID, workspaceID, err := f.CreateUserAndWorkspace()
	if err != nil {
		return nil, err
	}
	itemID, err := f.CreateItem(CreateItemOpts{
		WorkspaceID: workspaceID,
		CreatorID:   &userID,
	})
	if err != nil {
		return nil, err
	}
	return &TestEnv{UserID: userID, WorkspaceID: workspaceID, ItemID: itemID}, nil
}
