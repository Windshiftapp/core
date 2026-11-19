// Package sideeffects provides bounded, process-local admission and
// transaction-owned staging for best-effort side effects.
//
// A Lane is a bounded queue plus a fixed worker pool. Admission never waits
// for a slot or a handler: when the lane is full, closed, or handed an
// oversized or expired candidate, the candidate is dropped and counted.
//
// A Staging buffer belongs to one transaction owner. It reserves lane budget
// without waiting before retaining an immutable candidate, transfers that
// reservation to the lane when the owner publishes after a successful commit,
// and releases it on discard. Savepoint rollback discards only the candidates
// staged after the savepoint mark. Neither a store append nor an
// in-transaction observer is a publication point; the owner publishes once.
//
// Zero-valued limits are invalid. Construct lanes and staging buffers through
// NewLane and NewStaging, which reject configurations that would leave an
// effect unbounded.
package sideeffects
