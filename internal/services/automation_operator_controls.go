package services

import (
	"context"
	"time"

	"windshift/internal/database"
	"windshift/internal/operatorcontrols"
)

func automationStepAdmission(db database.Database) func(context.Context) error {
	controls := operatorcontrols.From(db)
	if controls.Limits().AutomationStepsPerDay == 0 {
		return nil
	}
	return func(ctx context.Context) error {
		return controls.Consume(ctx, "automation_steps", 1, controls.Limits().AutomationStepsPerDay, 24*time.Hour)
	}
}
