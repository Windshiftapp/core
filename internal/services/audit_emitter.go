package services

import (
	"windshift/internal/database"
	"windshift/internal/logger"
)

// emitServiceAudit records one successful, committed mutation from a shared
// application service. Transport adapters supply the actor metadata, while
// the service owns the decision about which domain operation is audited.
func emitServiceAudit(db database.Database, actor AuditActor, actionType, resourceType string, resourceID *int, resourceName string, details map[string]any) {
	_ = logger.LogAudit(db, logger.AuditEvent{
		UserID:       actor.UserID,
		Username:     actor.Username,
		IPAddress:    actor.IPAddress,
		UserAgent:    actor.UserAgent,
		Source:       actor.Source,
		ActionType:   actionType,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		ResourceName: resourceName,
		Details:      mergeAuditDetails(details, actor),
		Success:      true,
	})
}

func optionalAuditActor(actors []AuditActor) *AuditActor {
	if len(actors) == 0 {
		return nil
	}
	return &actors[0]
}
