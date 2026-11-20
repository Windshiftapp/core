package database

import "sync"

// TxEffects accumulates best-effort side effects staged during one
// transaction. The transaction publishes them after a successful commit and
// discards them on rollback or a failed commit.
//
// The interface is deliberately generic: the database package does not know
// what a side effect is. A process installs one factory at startup; without a
// factory no effects are staged and transaction overhead is a single nil
// check.
type TxEffects interface {
	// Publish transfers every staged effect to its bounded lane. It runs once,
	// after the outer commit succeeds.
	Publish()
	// Discard releases every staged effect without publishing. It runs when the
	// transaction rolls back or the commit fails.
	Discard()
}

// TxEffectsStaging is implemented by accumulators that can scope staged
// candidates to a savepoint. A savepoint owner records a mark before the
// SAVEPOINT and calls RollbackTo after ROLLBACK TO SAVEPOINT so candidates
// staged in the rolled-back suffix are released.
type TxEffectsStaging interface {
	Mark() int
	RollbackTo(mark int) int
}

// txEffectState owns one transaction's accumulator and guarantees exactly one
// of Publish or Discard runs, even when a rollback follows a commit or a
// callback panics during unwinding.
type txEffectState struct {
	effects TxEffects
	done    bool
}

func (s *txEffectState) TxEffects() TxEffects { return s.effects }

func (s *txEffectState) setTxEffects(effects TxEffects) { s.effects = effects }

func (s *txEffectState) publish() {
	if s.effects == nil || s.done {
		return
	}
	s.done = true
	s.effects.Publish()
}

func (s *txEffectState) discard() {
	if s.effects == nil || s.done {
		return
	}
	s.done = true
	s.effects.Discard()
}

var (
	txEffectsFactoryMu sync.RWMutex
	txEffectsFactory   func() TxEffects
)

// SetTxEffectsFactory installs the per-transaction effects factory. Passing nil
// disables effect staging; tests use that to isolate themselves.
func SetTxEffectsFactory(factory func() TxEffects) {
	txEffectsFactoryMu.Lock()
	txEffectsFactory = factory
	txEffectsFactoryMu.Unlock()
}

// NewTxEffects builds one transaction's effects accumulator, or nil when no
// factory is installed.
func NewTxEffects() TxEffects {
	txEffectsFactoryMu.RLock()
	factory := txEffectsFactory
	txEffectsFactoryMu.RUnlock()
	if factory == nil {
		return nil
	}
	return factory()
}

// AttachTxEffects binds an accumulator to a transaction. Transactions that do
// not support effects ignore the call.
func AttachTxEffects(tx Tx, effects TxEffects) {
	if holder, ok := tx.(txEffectsHolder); ok {
		holder.setTxEffects(effects)
	}
}

// TxEffectsOf returns the accumulator attached to tx, or nil. Producers use it
// to stage candidates without importing the database implementation.
func TxEffectsOf(tx Tx) TxEffects {
	if holder, ok := tx.(txEffectsHolder); ok {
		return holder.TxEffects()
	}
	return nil
}

// txEffectsHolder is implemented by concrete transaction wrappers so effects
// can travel with the transaction without widening the Tx interface.
type txEffectsHolder interface {
	TxEffects() TxEffects
	setTxEffects(TxEffects)
}
