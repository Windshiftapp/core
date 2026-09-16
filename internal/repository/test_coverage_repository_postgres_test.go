package repository

import (
	"fmt"
	"testing"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/testdb"
)

func TestCoverageConfigUpdateSwitchesToRequirementTypesPostgres(t *testing.T) {
	testdb.RunPostgres(t, func(db database.Database) {
		suffix := fmt.Sprintf("%d", time.Now().UnixNano())
		var wsID int
		if err := db.QueryRow(`
			INSERT INTO workspaces (name, key) VALUES (?, ?) RETURNING id
		`, "CRM "+suffix, "CRM"+suffix).Scan(&wsID); err != nil {
			t.Fatal(err)
		}

		repo := NewTestCoverageRepository(db)
		config, err := repo.CreateConfigForWorkspace(wsID, CoverageConfigWrite{
			RequirementItemTypeIDs: []int{1, 2},
		})
		if err != nil {
			t.Fatal(err)
		}
		if config.UsesPageBackedRequirements() {
			t.Fatalf("expected legacy config, got %+v", config)
		}

		updated, err := repo.UpdateConfig(config.ID, CoverageConfigWrite{
			RequirementTypes: []string{models.RequirementTypeBusinessRule},
		})
		if err != nil {
			t.Fatal(err)
		}
		if !updated.UsesPageBackedRequirements() {
			t.Fatalf("expected page-backed config after update, got %+v", updated)
		}
		if len(updated.RequirementItemTypeIDs) != 0 {
			t.Fatalf("legacy item type ids should be cleared, got %+v", updated.RequirementItemTypeIDs)
		}
	})
}
