package models

import "time"

type AgentSessionType string

const (
	AgentSessionGeneral  AgentSessionType = "general"
	AgentSessionStandard AgentSessionType = "standard"
)

type AgentSession struct {
	ID             int                       `json:"id"`
	SessionType    AgentSessionType          `json:"session_type"`
	OwnerUserID    int                       `json:"owner_user_id"`
	WorkspaceID    *int                      `json:"workspace_id,omitempty"`
	AgentProfileID *int                      `json:"agent_profile_id,omitempty"`
	Title          string                    `json:"title"`
	ArchivedAt     *time.Time                `json:"archived_at,omitempty"`
	CreatedAt      time.Time                 `json:"created_at"`
	UpdatedAt      time.Time                 `json:"updated_at"`
	Participants   []AgentSessionParticipant `json:"participants,omitempty"`
}

type AgentSessionParticipant struct {
	UserID          int       `json:"user_id"`
	ParticipantRole string    `json:"participant_role"`
	DisplayName     string    `json:"display_name,omitempty"`
	Username        string    `json:"username,omitempty"`
	IsAgent         bool      `json:"is_agent"`
	JoinedAt        time.Time `json:"joined_at"`
}

type AgentMessage struct {
	ID           int    `json:"id"`
	SessionID    int    `json:"session_id"`
	Role         string `json:"role"`
	AuthorUserID *int   `json:"author_user_id,omitempty"`
	AgentRunID   *int   `json:"agent_run_id,omitempty"`
	Content      string `json:"content"`
	ContextJSON  string `json:"-"`
	MetadataJSON string `json:"-"`
	// Model is the model that produced an assistant turn, recovered from
	// MetadataJSON. Empty for user turns and for turns the env-var fallback
	// client served, which has no configured model to name.
	Model string `json:"model,omitempty"`
	// Usage is the metered token/cost total for the turn. Nil when the turn was
	// not metered, so a client can omit the reading rather than render a
	// confident zero.
	Usage     *RunUsageTotals `json:"usage,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

// RunUsageTotals is the token + cost spend for one metered turn. CostUSD is nil
// when no catalog rate or provider figure covered the call; tokens are always
// reported, because tokens are metered even when their price is unknown.
//
// Every field is always present on the wire — no omitempty — so a client can
// rely on required-ness instead of guarding each read, and so this stays
// shape-compatible with the agent-run usage response serving the same numbers.
type RunUsageTotals struct {
	PromptTokens     int      `json:"prompt_tokens"`
	CompletionTokens int      `json:"completion_tokens"`
	TotalTokens      int      `json:"total_tokens"`
	CacheReadTokens  int      `json:"cache_read_tokens"`
	CacheWriteTokens int      `json:"cache_write_tokens"`
	ReasoningTokens  int      `json:"reasoning_tokens"`
	CostUSD          *float64 `json:"cost_usd"` // nil when no call carried a known cost
	Calls            int      `json:"calls"`
}
