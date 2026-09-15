package models

import (
	"fmt"
	"slices"
	"time"
)

// Requirement is a workspace-scoped managed document backed by a Knowledge
// Page. Ordinary wiki pages have no row here; promotion allocates an immutable
// number once.
type Requirement struct {
	ID                int       `json:"id" db:"id"`
	PageID            int       `json:"page_id" db:"page_id"`
	WorkspaceID       int       `json:"workspace_id" db:"workspace_id"`
	RequirementNumber int       `json:"requirement_number" db:"requirement_number"`
	RequirementType   string    `json:"requirement_type" db:"requirement_type"`
	Status            string    `json:"status" db:"status"`
	OwnerID           *int      `json:"owner_id" db:"owner_id"`
	CreatedBy         int       `json:"created_by" db:"created_by"`
	CreatedAt         time.Time `json:"created_at" db:"created_at"`
	UpdatedBy         *int      `json:"updated_by" db:"updated_by"`
	UpdatedAt         time.Time `json:"updated_at" db:"updated_at"`
}

// RequirementKeySegment is the type-independent middle part of a display
// key. Requirement type is stored separately so renaming a document from a
// functional requirement to a business rule does not break references.
const RequirementKeySegment = "DOC"

const (
	RequirementTypeBusinessRequirement      = "business_requirement"
	RequirementTypeFunctionalRequirement    = "functional_requirement"
	RequirementTypeNonFunctionalRequirement = "non_functional_requirement"
	RequirementTypeBusinessRule             = "business_rule"
	RequirementTypeUseCase                  = "use_case"
	RequirementTypeBusinessProcess          = "business_process"
	RequirementTypeSystemSpecification      = "system_specification"
	RequirementTypeAPISpecification         = "api_specification"
	RequirementTypeDataModel                = "data_model"
	RequirementTypeArchitectureDecision     = "architecture_decision"
	RequirementTypeGlossaryEntry            = "glossary_entry"
)

const (
	RequirementStatusDraft      = "draft"
	RequirementStatusInReview   = "in_review"
	RequirementStatusApproved   = "approved"
	RequirementStatusDeprecated = "deprecated"
)

// RequirementHistoryFieldPromoted is the history field written when a page
// first becomes a requirement.
const RequirementHistoryFieldPromoted = "promoted"

const (
	RequirementHistoryFieldType   = "requirement_type"
	RequirementHistoryFieldStatus = "status"
	RequirementHistoryFieldOwner  = "owner_id"
)

// RequirementHistoryEntry is an immutable audit row for requirement metadata
// changes. Markdown content revisions stay on page_revisions.
type RequirementHistoryEntry struct {
	ID            int       `json:"id" db:"id"`
	RequirementID int       `json:"requirement_id" db:"requirement_id"`
	UserID        int       `json:"user_id" db:"user_id"`
	FieldName     string    `json:"field_name" db:"field_name"`
	OldValue      *string   `json:"old_value" db:"old_value"`
	NewValue      *string   `json:"new_value" db:"new_value"`
	ChangedAt     time.Time `json:"changed_at" db:"changed_at"`
}

// RequirementTypes is the closed set stored by CHECK constraints.
var RequirementTypes = []string{
	RequirementTypeBusinessRequirement,
	RequirementTypeFunctionalRequirement,
	RequirementTypeNonFunctionalRequirement,
	RequirementTypeBusinessRule,
	RequirementTypeUseCase,
	RequirementTypeBusinessProcess,
	RequirementTypeSystemSpecification,
	RequirementTypeAPISpecification,
	RequirementTypeDataModel,
	RequirementTypeArchitectureDecision,
	RequirementTypeGlossaryEntry,
}

// RequirementStatuses is the closed lifecycle set stored by CHECK constraints.
var RequirementStatuses = []string{
	RequirementStatusDraft,
	RequirementStatusInReview,
	RequirementStatusApproved,
	RequirementStatusDeprecated,
}

// IsValidRequirementType reports whether typ is one of the system types.
func IsValidRequirementType(typ string) bool {
	return slices.Contains(RequirementTypes, typ)
}

// IsValidRequirementStatus reports whether status is one of the system statuses.
func IsValidRequirementStatus(status string) bool {
	return slices.Contains(RequirementStatuses, status)
}

// FormatRequirementKey builds the live display key. The prefix follows
// workspaces.key at read time (same as work-item keys): renaming a
// workspace changes CRM-DOC-42 to SALES-DOC-42 without rewriting
// requirement_number.
func FormatRequirementKey(workspaceKey string, number int) string {
	return fmt.Sprintf("%s-%s-%d", workspaceKey, RequirementKeySegment, number)
}
