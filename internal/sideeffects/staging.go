package sideeffects

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// StagingConfig configures a transaction-owned staging buffer. Every field is
// required: a zero or negative limit is rejected rather than treated as
// unlimited.
type StagingConfig struct {
	// MaxEvents bounds how many candidates one transaction may stage.
	MaxEvents int
	// MaxBytes bounds the retained payload bytes one transaction may stage.
	MaxBytes int64
	// MaxPayloadBytes bounds a single staged payload.
	MaxPayloadBytes int
	// MaxEventAge bounds how old a candidate may be when it is staged.
	MaxEventAge time.Duration
	// Clock supplies the current time. It defaults to time.Now and exists for
	// deterministic tests.
	Clock func() time.Time
}

// Staging is a bounded, transaction-owned buffer of immutable candidates. The
// owner stages candidates while the transaction is open, publishes them once
// after the outer commit succeeds, and discards them on failure. Nested
// helpers contribute to the owner's buffer rather than publishing themselves.
//
// A Staging is not safe for concurrent use by multiple transactions. It may be
// reused for one transaction at a time: Publish and Discard both empty it.
type Staging struct {
	cfg   StagingConfig
	clock func() time.Time

	mu        sync.Mutex
	entries   []*stagedEntry
	countUsed int
	byteUsed  int64
	staged    int64
	published int64
	discarded int64
	dropped   map[DropReason]int64
}

type stagedEntry struct {
	lane        *Lane
	candidate   Candidate
	reservation *reservation
}

// NewStaging validates cfg and returns an empty staging buffer.
func NewStaging(cfg StagingConfig) (*Staging, error) {
	if cfg.MaxEvents <= 0 {
		return nil, errors.New("sideeffects: staging max events must be positive")
	}
	if cfg.MaxBytes <= 0 {
		return nil, errors.New("sideeffects: staging max bytes must be positive")
	}
	if cfg.MaxPayloadBytes <= 0 {
		return nil, errors.New("sideeffects: staging max payload bytes must be positive")
	}
	if cfg.MaxEventAge <= 0 {
		return nil, errors.New("sideeffects: staging max event age must be positive")
	}
	clock := cfg.Clock
	if clock == nil {
		clock = time.Now
	}
	return &Staging{cfg: cfg, clock: clock}, nil
}

// Stage reserves lane budget and retains an immutable snapshot of candidate.
// It reports whether the candidate was staged and, when it was not, why. The
// per-transaction and per-lane limits are both enforced here so a candidate is
// never retained without budget.
func (s *Staging) Stage(lane *Lane, candidate Candidate) (bool, DropReason) {
	if lane == nil {
		s.recordDrop(DropNoLane)
		return false, DropNoLane
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	size := candidate.size()
	if s.countUsed >= s.cfg.MaxEvents || s.byteUsed+size > s.cfg.MaxBytes {
		s.recordDrop(DropStagingLimit)
		return false, DropStagingLimit
	}
	if len(candidate.Payload) > s.cfg.MaxPayloadBytes {
		s.recordDrop(DropOversized)
		return false, DropOversized
	}
	now := s.clock()
	if !candidate.OccurredAt.IsZero() && now.Sub(candidate.OccurredAt) > s.cfg.MaxEventAge {
		s.recordDrop(DropExpired)
		return false, DropExpired
	}
	reservation, reason := lane.reserveForStaging(candidate, now)
	if reason != "" {
		s.recordDrop(reason)
		return false, reason
	}
	s.entries = append(s.entries, &stagedEntry{
		lane:        lane,
		candidate:   cloneCandidate(candidate, now),
		reservation: reservation,
	})
	s.countUsed++
	s.byteUsed += size
	s.staged++
	return true, ""
}

// Mark returns an opaque position for the current staging suffix. Pass it to
// RollbackTo when a savepoint rolls back.
func (s *Staging) Mark() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries)
}

// RollbackTo discards every candidate staged after mark and releases its lane
// budget. It reports how many candidates were discarded. Marks at or beyond
// the current length discard nothing.
func (s *Staging) RollbackTo(mark int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if mark < 0 {
		mark = 0
	}
	if mark >= len(s.entries) {
		return 0
	}
	discarded := s.releaseFrom(mark)
	s.entries = s.entries[:mark]
	s.discarded += int64(discarded)
	return discarded
}

// Discard releases every staged candidate without publishing. The owner calls
// it when the transaction fails or is abandoned.
func (s *Staging) Discard() int {
	return s.RollbackTo(0)
}

// Publish transfers every staged candidate to its lane without waiting and
// empties the buffer. A closed lane, a full lane, or an expired candidate
// drops only that candidate; the rest are still published. It reports how many
// candidates were published. The owner calls Publish once, after the outer
// commit succeeds.
func (s *Staging) Publish() int {
	s.mu.Lock()
	entries := s.entries
	s.entries = nil
	s.countUsed = 0
	s.byteUsed = 0
	s.mu.Unlock()

	published := 0
	for _, entry := range entries {
		if ok, _ := entry.lane.publishOwned(entry.candidate, entry.reservation); ok {
			published++
		}
	}

	s.mu.Lock()
	s.published += int64(published)
	s.discarded += int64(len(entries) - published)
	s.mu.Unlock()
	return published
}

// Pending reports the count and retained bytes currently staged.
func (s *Staging) Pending() (count int, bytes int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.countUsed, s.byteUsed
}

// releaseFrom releases the reservations of entries[from:] and returns their
// count. The caller holds s.mu.
func (s *Staging) releaseFrom(from int) int {
	for _, entry := range s.entries[from:] {
		entry.reservation.release()
		s.countUsed--
		s.byteUsed -= entry.candidate.size()
	}
	return len(s.entries) - from
}

// Stats returns a snapshot of the buffer's lifetime counters.
func (s *Staging) Stats() StagingStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	dropped := make(map[DropReason]int64, len(s.dropped))
	for reason, count := range s.dropped {
		dropped[reason] = count
	}
	return StagingStats{
		Pending:      s.countUsed,
		PendingBytes: s.byteUsed,
		Staged:       s.staged,
		Published:    s.published,
		Discarded:    s.discarded,
		Dropped:      dropped,
	}
}

// StagingStats is a point-in-time view of a staging buffer's counters.
type StagingStats struct {
	Pending      int
	PendingBytes int64
	Staged       int64
	Published    int64
	Discarded    int64
	Dropped      map[DropReason]int64
}

func (s *Staging) recordDrop(reason DropReason) {
	if s.dropped == nil {
		s.dropped = make(map[DropReason]int64)
	}
	s.dropped[reason]++
}

// String renders the drop reason for diagnostics and tests.
func (r DropReason) String() string { return string(r) }

// Validate ensures a DropReason is one of the defined values. It exists so
// diagnostics can reject unknown reasons instead of silently grouping them.
func (r DropReason) Validate() error {
	switch r {
	case DropFull, DropClosed, DropOversized, DropExpired, DropStagingLimit, DropNoLane:
		return nil
	default:
		return fmt.Errorf("sideeffects: unknown drop reason %q", string(r))
	}
}
