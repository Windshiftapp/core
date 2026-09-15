package repository

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/testdb"
)

func TestCoverageLegacySunsetMigrationPostgres(t *testing.T) {
	testdb.RunPostgres(t, func(db database.Database) {
		suffix := fmt.Sprintf("%d", time.Now().UnixNano())
		var wsID int
		if err := db.QueryRow(`
			INSERT INTO workspaces (name, key) VALUES (?, ?) RETURNING id
		`, "CRM "+suffix, "CRM"+suffix).Scan(&wsID); err != nil {
			t.Fatal(err)
		}

		legacyIDs, err := json.Marshal([]int{1, 2})
		if err != nil {
			t.Fatal(err)
		}
		var configID int
		if err := db.QueryRow(`
			INSERT INTO test_coverage_configurations (workspace_id, requirement_item_type_ids, requirement_types)
			VALUES (?, ?, NULL)
			RETURNING id
		`, wsID, string(legacyIDs)).Scan(&configID); err != nil {
			t.Fatal(err)
		}

		if _, err := db.ExecWrite(`
			UPDATE test_coverage_configurations
			SET requirement_types = '["business_requirement","functional_requirement","non_functional_requirement","business_rule","use_case","business_process","system_specification","api_specification","data_model","architecture_decision","glossary_entry"]',
				requirement_item_type_ids = NULL,
				updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, configID); err != nil {
			t.Fatal(err)
		}

		var legacyJSON *string
		var typesJSON *string
		if err := db.QueryRow(`
			SELECT requirement_item_type_ids, requirement_types
			FROM test_coverage_configurations WHERE id = ?
		`, configID).Scan(&legacyJSON, &typesJSON); err != nil {
			t.Fatal(err)
		}
		if legacyJSON != nil && *legacyJSON != "" && *legacyJSON != "[]" {
			t.Fatalf("expected legacy item type ids cleared, got %v", legacyJSON)
		}
		if typesJSON == nil || *typesJSON == "" || *typesJSON == "[]" {
			t.Fatalf("expected requirement types populated, got %v", typesJSON)
		}

		var types []string
		if err := json.Unmarshal([]byte(*typesJSON), &types); err != nil {
			t.Fatal(err)
		}
		if len(types) != len(models.RequirementTypes) {
			t.Fatalf("expected %d requirement types, got %d", len(models.RequirementTypes), len(types))
		}
	})
}
