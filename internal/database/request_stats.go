package database

import (
	"context"
	"errors"
	"sync/atomic"
	"time"
)

var requestQueryOutcomes struct {
	canceled  atomic.Uint64
	deadlines atomic.Uint64
	errors    atomic.Uint64
	successes atomic.Uint64
}

var requestTransactions struct {
	count      atomic.Uint64
	totalNanos atomic.Int64
	maxNanos   atomic.Int64
}

// RequestQueryOutcomeStats separates client cancellation and local deadline
// expiry from genuine database/server failures on request-owned SQL paths.
type RequestQueryOutcomeStats struct {
	Canceled  uint64 `json:"canceled"`
	Deadlines uint64 `json:"deadlines"`
	Errors    uint64 `json:"errors"`
	Successes uint64 `json:"successes"`
}

// ObserveRequestQueryError records the terminal outcome of request-owned
// database work. A nil error is deliberately ignored.
func ObserveRequestQueryError(err error) {
	switch {
	case errors.Is(err, context.Canceled):
		requestQueryOutcomes.canceled.Add(1)
	case errors.Is(err, context.DeadlineExceeded):
		requestQueryOutcomes.deadlines.Add(1)
	case err != nil:
		requestQueryOutcomes.errors.Add(1)
	}
}

// ObserveRequestQuerySuccess records a query that returned without error. It is
// a bounded-cardinality volume counter, not a per-route label set.
func ObserveRequestQuerySuccess() {
	requestQueryOutcomes.successes.Add(1)
}

// ObserveRequestTransaction records a committed transaction's wall duration.
func ObserveRequestTransaction(duration time.Duration) {
	nanos := duration.Nanoseconds()
	requestTransactions.count.Add(1)
	requestTransactions.totalNanos.Add(nanos)
	for {
		current := requestTransactions.maxNanos.Load()
		if nanos <= current || requestTransactions.maxNanos.CompareAndSwap(current, nanos) {
			return
		}
	}
}

// RequestTransactionStats is a process-local summary of committed transactions.
type RequestTransactionStats struct {
	Count           uint64 `json:"count"`
	TotalDurationMs int64  `json:"total_duration_ms"`
	MaxDurationMs   int64  `json:"max_duration_ms"`
}

// RequestQueryStats returns a process-local lifetime snapshot.
func RequestQueryStats() RequestQueryOutcomeStats {
	return RequestQueryOutcomeStats{
		Canceled:  requestQueryOutcomes.canceled.Load(),
		Deadlines: requestQueryOutcomes.deadlines.Load(),
		Errors:    requestQueryOutcomes.errors.Load(),
		Successes: requestQueryOutcomes.successes.Load(),
	}
}

// RequestTransactionStatsSnapshot returns a process-local lifetime snapshot.
func RequestTransactionStatsSnapshot() RequestTransactionStats {
	millis := time.Millisecond.Nanoseconds()
	return RequestTransactionStats{
		Count:           requestTransactions.count.Load(),
		TotalDurationMs: requestTransactions.totalNanos.Load() / millis,
		MaxDurationMs:   requestTransactions.maxNanos.Load() / millis,
	}
}
