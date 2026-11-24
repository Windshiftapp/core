package itemevents

import (
	"encoding/json"

	"windshift/internal/database"
	"windshift/internal/events"
	"windshift/internal/sideeffects"
)

// stageFact offers an appended fact to the transaction's side-effect
// accumulator. Historical imports and explicitly suppressed facts are filtered
// here, before any lane budget is reserved. Neither the store append nor the
// in-transaction observer is a publication point; the transaction owner
// publishes after the outer commit succeeds.
func stageFact(tx database.Tx, event *events.Event) {
	if event == nil {
		return
	}
	stager, ok := database.TxEffectsOf(tx).(sideeffects.FactStager)
	if !ok {
		return
	}
	if IsImportOrigin(event.ActorKind, event.SourceKind) {
		return
	}
	if SuppressesSideEffects(event.Type, event.Payload) {
		return
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	stager.StageFact(sideeffects.Candidate{
		EventType: event.Type,
		Key:       event.Key,
		Payload:   payload,
	})
}
