package constants

// Status IDs for system-critical statuses that cannot be deleted.
// These IDs correspond to the default statuses created in database initialization
// (see internal/database/database.go).
//
// The order of insertion determines the IDs:
// 1. Open (default initial status)
// 2. In Progress
// 3. Done
//
// Note: These statuses can be renamed by users, but cannot be deleted as they are
// required for core system functionality (particularly personal tasks).
const (
	// StatusIDOpen is the default status for new work items and personal tasks
	StatusIDOpen = 1

	// StatusIDDone is used to mark personal tasks as completed
	StatusIDDone = 3
)
