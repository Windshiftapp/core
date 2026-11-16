package models

import "time"

// WorkspaceAgentSkill is one entry in a workspace's agent-skills library
// (WI-258): a markdown "knowledge pack" in the shape of the Anthropic Agent
// Skills standard (the body is a SKILL.md), attachable to workspace agent
// bindings m:n. Delivery is progressive disclosure: a run's initial prompt
// lists the attached skills' names + descriptions, and the agent fetches a
// body with `ws skill get <id>` only when it judges the skill relevant —
// so Description carries the trigger ("when to use this"), not the content.
type WorkspaceAgentSkill struct {
	ID          int    `json:"id"`
	WorkspaceID int    `json:"workspace_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Body        string `json:"body,omitempty"`
	Enabled     bool   `json:"enabled"`
	// Pages are the workspace pages referenced by this skill (WI-517). On the
	// admin surface they round-trip with snapshot review state; the exact saved
	// content is rendered into a run grant. nil/omitted when there are none.
	Pages []SkillPageReference  `json:"pages,omitempty"`
	Usage *SkillActivationUsage `json:"usage,omitempty"`
	// CreatedByUserID is a soft audit ref; nil when the creator was deleted.
	CreatedByUserID *int      `json:"created_by_user_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// SkillPageReference is a lightweight handle on a workspace page referenced by
// a skill (WI-517): enough for the editor to render a chip and for the agent
// surface to show the selected source and whether it changed since review.
type SkillPageReference struct {
	ID              int       `json:"id"`
	Title           string    `json:"title"`
	SnapshotTitle   string    `json:"snapshot_title,omitempty"`
	ContentSnapshot string    `json:"-"`
	SnapshotAt      time.Time `json:"snapshot_at"`
	PageUpdatedAt   time.Time `json:"page_updated_at,omitempty"`
	Stale           bool      `json:"stale"`
	ActivationBytes int       `json:"activation_bytes"`
	ActivationRunes int       `json:"activation_runes"`
}

type SkillActivationUsage struct {
	Bytes           int `json:"bytes"`
	EstimatedTokens int `json:"estimated_tokens"`
	MaxBytes        int `json:"max_bytes"`
	MaxTokens       int `json:"max_tokens"`
	PagePrefixBytes int `json:"page_prefix_bytes"`
	PagePrefixRunes int `json:"page_prefix_runes"`
}
