package repository

import (
	"context"
	"database/sql"
	"fmt"

	"windshift/internal/database"
	"windshift/internal/models"
)

// LLMUsageRepository persists per-call LLM token usage + cost, metered at the
// broker. One row per chat-completion; per-run totals are aggregated on read.
type LLMUsageRepository struct {
	db database.Database
}

// NewLLMUsageRepository constructs a new repository.
func NewLLMUsageRepository(db database.Database) *LLMUsageRepository {
	return &LLMUsageRepository{db: db}
}

// LLMUsageRecord is one metered chat-completion call. CostUSD is nil when the
// provider catalog carries no pricing (tokens metered, cost unknown).
type LLMUsageRecord struct {
	RunID            int
	Model            string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	CacheReadTokens  int
	CacheWriteTokens int
	ReasoningTokens  int
	CostUSD          *float64
	// CostSource is "provider" (the provider billed this number), "computed"
	// (priced from catalog rates), "unpriced" (rates exist but not for every
	// class the call used, so no cost is claimed), or "" (no rates at all).
	CostSource string
}

// Insert records one metered call.
func (r *LLMUsageRepository) Insert(ctx context.Context, rec LLMUsageRecord) error {
	_, err := r.db.ExecWriteContext(ctx, `
		INSERT INTO llm_usage
			(run_id, model, prompt_tokens, completion_tokens, total_tokens,
			 cache_read_tokens, cache_write_tokens, reasoning_tokens, cost_usd, cost_source)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		rec.RunID, rec.Model, rec.PromptTokens, rec.CompletionTokens, rec.TotalTokens,
		rec.CacheReadTokens, rec.CacheWriteTokens, rec.ReasoningTokens,
		nullFloatArg(rec.CostUSD), rec.CostSource,
	)
	if err != nil {
		return fmt.Errorf("insert llm_usage: %w", err)
	}
	return nil
}

// RunUsageTotals is the aggregated token + cost spend for a single run. It
// aliases the model because the same numbers are read here, written by the
// metering services, and served on agent-message and agent-run responses — one
// definition, so a field added for one surface cannot be missing on another.
type RunUsageTotals = models.RunUsageTotals

// TotalsForRun aggregates all metered calls for a run. CostUSD is the sum of
// the calls whose cost was known; it is nil when none were.
func (r *LLMUsageRepository) TotalsForRun(ctx context.Context, runID int) (RunUsageTotals, error) {
	var t RunUsageTotals
	var prompt, completion, total, cacheRead, cacheWrite, reasoning, calls sql.NullInt64
	var cost sql.NullFloat64
	err := r.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(prompt_tokens), 0),
			COALESCE(SUM(completion_tokens), 0),
			COALESCE(SUM(total_tokens), 0),
			COALESCE(SUM(cache_read_tokens), 0),
			COALESCE(SUM(cache_write_tokens), 0),
			COALESCE(SUM(reasoning_tokens), 0),
			SUM(cost_usd),
			COUNT(*)
		FROM llm_usage WHERE run_id = ?
	`, runID).Scan(&prompt, &completion, &total, &cacheRead, &cacheWrite, &reasoning, &cost, &calls)
	if err != nil {
		return t, fmt.Errorf("aggregate llm_usage for run %d: %w", runID, err)
	}
	t.PromptTokens = int(prompt.Int64)
	t.CompletionTokens = int(completion.Int64)
	t.TotalTokens = int(total.Int64)
	t.CacheReadTokens = int(cacheRead.Int64)
	t.CacheWriteTokens = int(cacheWrite.Int64)
	t.ReasoningTokens = int(reasoning.Int64)
	t.Calls = int(calls.Int64)
	if cost.Valid {
		c := cost.Float64
		t.CostUSD = &c
	}
	return t, nil
}

func nullFloatArg(f *float64) any {
	if f == nil {
		return nil
	}
	return *f
}
