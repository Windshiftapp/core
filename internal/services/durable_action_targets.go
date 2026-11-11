package services

import (
	"context"
	"sync/atomic"

	"windshift/internal/actionevents"
	"windshift/internal/database"
	"windshift/internal/events"
	"windshift/internal/itemevents"
)

func appendStandaloneDurableActionEvent(
	ctx context.Context,
	db database.Database,
	store *events.Store,
	input events.NewEvent,
	label string,
) error {
	return actionevents.AppendStandalone(ctx, db, store, input, label)
}

func handleDurableActionEvent[T any](
	ctx context.Context,
	event events.Event,
	targets *actionevents.TargetStore,
	decode func(events.Event) (T, string, error),
	materialize func(context.Context, events.Event, string, T) error,
	callbacks func(T) actionevents.Callbacks,
	actionsExecuted *int64,
	eventsProcessed *int64,
) error {
	// Imported facts are historical: never let them trigger automation, which
	// can itself send email or call external endpoints.
	if itemevents.IsImportOrigin(event.ActorKind, event.SourceKind) {
		return nil
	}
	// Machine-driven writes can opt out of live side effects explicitly.
	if itemevents.SuppressesSideEffects(event.Type, event.Payload) {
		return nil
	}
	actionEvent, consumerKey, err := decode(event)
	if err != nil {
		return events.Permanent(err)
	}
	if err := materialize(ctx, event, consumerKey, actionEvent); err != nil {
		return err
	}
	executed, err := actionevents.RunTargets(ctx, targets, event.Key, callbacks(actionEvent))
	atomic.AddInt64(actionsExecuted, executed)
	if err != nil {
		return err
	}
	atomic.AddInt64(eventsProcessed, 1)
	return nil
}

func selectDurableActionTargets[T any](
	actions []T,
	cascadeDepth int,
	chain *ExecutionChain,
	id func(T) int,
	chainKey func(T) string,
	matches func(T) bool,
) []int {
	matching := make([]int, 0, len(actions))
	for _, action := range actions {
		if cascadeDepth >= MaxCascadeDepth {
			break
		}
		if chain != nil && chain.HasExecuted(chainKey(action)) {
			continue
		}
		if matches(action) {
			matching = append(matching, id(action))
		}
	}
	return matching
}
