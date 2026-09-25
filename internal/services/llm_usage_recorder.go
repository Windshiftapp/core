package services

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"windshift/internal/llm"
	"windshift/internal/repository"
)

// MeteredCall is one metered unit of provider work: the normalized usage a
// provider reported plus the two dimensions that usage alone cannot express.
//
// A single API call needs neither — but an agentic turn is several calls, and
// both the flat per-request rate and the per-image rate scale with them, so
// pricing a multi-call turn from Usage alone would under-report the spend.
type MeteredCall struct {
	Usage llm.Usage
	// Images is the total number of image parts sent across Calls.
	Images int
	// Calls is the number of provider round-trips. The flat per-request rate
	// applies once per call.
	Calls int
}

// usageInsertTimeout bounds the metering write. Metering is bookkeeping that
// must never hold up the request it is recording for, and must not inherit a
// caller's canceled context.
const usageInsertTimeout = 5 * time.Second

// RecordLLMCall resolves the cost of one metered unit of provider work and
// persists it to llm_usage under runID, returning the run's totals.
//
// Cost resolution lives here, as the single implementation, because getting it
// wrong is a billing error rather than a cosmetic one:
//
//   - A provider-reported figure wins outright. It already accounts for
//     discounts and routing we cannot see.
//   - Otherwise catalog rates apply, but only when they cover every class the
//     call actually used. Pricing a cache write at the base input rate, or an
//     image at zero, would silently under-bill, so those rows are recorded
//     unpriced instead.
//   - "unpriced" is recorded explicitly so a row is never indistinguishable
//     from a model the catalog knows nothing about.
//
// The returned totals are the authoritative numbers for the run; callers should
// prefer them over re-deriving a sum.
func RecordLLMCall(
	ctx context.Context,
	repo *repository.LLMUsageRepository,
	runID int,
	descriptor llm.ConnectionDescriptor,
	metered MeteredCall,
) (repository.RunUsageTotals, error) {
	if repo == nil {
		return repository.RunUsageTotals{}, nil
	}
	record := repository.LLMUsageRecord{
		RunID: runID, Model: descriptor.Model,
		PromptTokens: metered.Usage.PromptTokens, CompletionTokens: metered.Usage.CompletionTokens,
		TotalTokens: metered.Usage.TotalTokens, CacheReadTokens: metered.Usage.CacheReadTokens,
		CacheWriteTokens: metered.Usage.CacheWriteTokens, ReasoningTokens: metered.Usage.ReasoningTokens,
	}
	pricing := descriptor.Pricing
	switch {
	case metered.Usage.ProviderCostUSD != nil:
		record.CostUSD = metered.Usage.ProviderCostUSD
		record.CostSource = "provider"
	case canPriceCall(pricing, metered):
		cost := pricing.CostUSDForCalls(metered.Usage, metered.Images, metered.Calls)
		record.CostUSD = &cost
		record.CostSource = "computed"
	case pricing != nil:
		record.CostSource = "unpriced"
		slog.WarnContext(ctx, "llm usage not priced: model pricing is missing a rate for a class this call used",
			slog.Int("run_id", runID),
			slog.String("model", descriptor.Model),
			slog.Int("cache_read_tokens", metered.Usage.CacheReadTokens),
			slog.Int("cache_write_tokens", metered.Usage.CacheWriteTokens),
			slog.Int("images", metered.Images),
		)
	}

	insertCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), usageInsertTimeout)
	defer cancel()
	if err := repo.Insert(insertCtx, record); err != nil {
		return repository.RunUsageTotals{}, fmt.Errorf("insert llm_usage for run %d: %w", runID, err)
	}
	return repo.TotalsForRun(insertCtx, runID)
}

// canPriceCall reports whether catalog rates cover every class the call
// actually used. Token classes are CanPriceUsage's job; the image rate is
// checked here because a call that sent images with no image rate would
// otherwise price those images at zero and report a confidently wrong total.
func canPriceCall(pricing *llm.Pricing, metered MeteredCall) bool {
	if pricing == nil {
		return false
	}
	if !pricing.CanPriceUsage(metered.Usage) {
		return false
	}
	return metered.Images == 0 || pricing.ImageUSD > 0
}
