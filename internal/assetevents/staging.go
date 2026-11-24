package assetevents

import (
	"encoding/json"

	"windshift/internal/database"
	"windshift/internal/events"
	"windshift/internal/sideeffects"
)

// stageFact offers an appended asset fact to the transaction's side-effect
// accumulator. Imported facts are historical and never drive live automation.
func stageFact(tx database.Tx, event *events.Event, metadata Metadata) {
	if event == nil {
		return
	}
	stager, ok := database.TxEffectsOf(tx).(sideeffects.FactStager)
	if !ok {
		return
	}
	if metadata.ActorKind == "import" || metadata.SourceKind == "import" {
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
