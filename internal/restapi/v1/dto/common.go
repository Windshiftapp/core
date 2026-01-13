package dto

import "time"

// UserSummary provides a minimal user representation for nested responses
type UserSummary struct {
	ID        int    `json:"id"`
	Email     string `json:"email"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	FullName  string `json:"full_name"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

// StatusSummary provides a minimal status representation
type StatusSummary struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	CategoryID    int    `json:"category_id"`
	CategoryName  string `json:"category_name,omitempty"`
	CategoryColor string `json:"category_color,omitempty"`
	IsCompleted   bool   `json:"is_completed,omitempty"`
}

// PrioritySummary provides a minimal priority representation
type PrioritySummary struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Icon  string `json:"icon,omitempty"`
	Color string `json:"color,omitempty"`
}

// ItemTypeSummary provides a minimal item type representation
type ItemTypeSummary struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	Icon           string `json:"icon,omitempty"`
	Color          string `json:"color,omitempty"`
	HierarchyLevel int    `json:"hierarchy_level"`
}

// WorkspaceSummary provides a minimal workspace representation
type WorkspaceSummary struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Key  string `json:"key"`
}

// MilestoneSummary provides a minimal milestone representation
type MilestoneSummary struct {
	ID         int     `json:"id"`
	Name       string  `json:"name"`
	TargetDate *string `json:"target_date,omitempty"`
	Status     string  `json:"status"`
}

// IterationSummary provides a minimal iteration representation
type IterationSummary struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Status    string `json:"status"`
}

// ProjectSummary provides a minimal project representation
type ProjectSummary struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// LinksResponse provides HATEOAS-style links for resources
type LinksResponse struct {
	Self     string `json:"self"`
	Parent   string `json:"parent,omitempty"`
	Children string `json:"children,omitempty"`
}

// TimestampResponse provides common timestamp fields
type TimestampResponse struct {
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}
