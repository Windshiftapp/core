//go:build test

package cql

import (
	"fmt"
	"testing"

	"windshift/internal/testutils"
)

type assetTestData struct {
	tdb            *testutils.TestDB
	setID          int
	setMap         map[string]int
	customFieldMap map[string]int
	assetIDs       map[string]int
}

func setupAssetTestDB(t *testing.T) *assetTestData {
	t.Helper()
	tdb := testutils.CreateTestDB(t, true)
	t.Cleanup(func() { tdb.Close() })

	// Create user
	_, err := tdb.Exec(`INSERT INTO users (id, email, username, first_name, last_name, password_hash, is_active)
		VALUES (1, 'test@example.com', 'testuser', 'Test', 'User', '$2a$10$hash', 1)`)
	if err != nil {
		t.Fatalf("Failed to insert user: %v", err)
	}

	// Create asset management set
	_, err = tdb.Exec(`INSERT INTO asset_management_sets (id, name, description, created_by)
		VALUES (1, 'Test Set', 'Test asset set', 1)`)
	if err != nil {
		t.Fatalf("Failed to insert set: %v", err)
	}

	// Create asset type
	_, err = tdb.Exec(`INSERT INTO asset_types (id, set_id, name, icon, color)
		VALUES (1, 1, 'Equipment', 'Box', '#6b7280')`)
	if err != nil {
		t.Fatalf("Failed to insert asset type: %v", err)
	}

	// Create asset status
	_, err = tdb.Exec(`INSERT INTO asset_statuses (id, set_id, name, color, is_default)
		VALUES (1, 1, 'Active', '#22c55e', true)`)
	if err != nil {
		t.Fatalf("Failed to insert status: %v", err)
	}

	// Create asset category
	_, err = tdb.Exec(`INSERT INTO asset_categories (id, set_id, name, path)
		VALUES (1, 1, 'General', '/General')`)
	if err != nil {
		t.Fatalf("Failed to insert category: %v", err)
	}

	assetIDs := make(map[string]int)

	// Asset 1: string value custom fields (keys are numeric field IDs as in production)
	// Field ID 1 = "Time Estimate", Field ID 2 = "Location"
	_, err = tdb.Exec(`INSERT INTO assets (id, set_id, asset_type_id, category_id, status_id, title, custom_field_values, created_by)
		VALUES (1, 1, 1, 1, 1, 'Asset String20', '{"1": "20", "2": "Building A"}', 1)`)
	if err != nil {
		t.Fatalf("Failed to insert asset 1: %v", err)
	}
	assetIDs["string20"] = 1

	// Asset 2: numeric value custom fields (keys are numeric field IDs)
	_, err = tdb.Exec(`INSERT INTO assets (id, set_id, asset_type_id, category_id, status_id, title, custom_field_values, created_by)
		VALUES (2, 1, 1, 1, 1, 'Asset Numeric20', '{"1": 20, "2": "Building B"}', 1)`)
	if err != nil {
		t.Fatalf("Failed to insert asset 2: %v", err)
	}
	assetIDs["numeric20"] = 2

	// Asset 3: missing Time Estimate field (only has Location field ID 2)
	_, err = tdb.Exec(`INSERT INTO assets (id, set_id, asset_type_id, category_id, status_id, title, custom_field_values, created_by)
		VALUES (3, 1, 1, 1, 1, 'Asset NoField', '{"2": "Building C"}', 1)`)
	if err != nil {
		t.Fatalf("Failed to insert asset 3: %v", err)
	}
	assetIDs["nofield"] = 3

	// Asset 4: NULL custom_field_values
	_, err = tdb.Exec(`INSERT INTO assets (id, set_id, asset_type_id, category_id, status_id, title, custom_field_values, created_by)
		VALUES (4, 1, 1, 1, 1, 'Asset Null', NULL, 1)`)
	if err != nil {
		t.Fatalf("Failed to insert asset 4: %v", err)
	}
	assetIDs["null"] = 4

	// Asset 5: empty string custom_field_values
	_, err = tdb.Exec(`INSERT INTO assets (id, set_id, asset_type_id, category_id, status_id, title, custom_field_values, created_by)
		VALUES (5, 1, 1, 1, 1, 'Asset Empty', '', 1)`)
	if err != nil {
		t.Fatalf("Failed to insert asset 5: %v", err)
	}
	assetIDs["empty"] = 5

	// Asset 6: empty object custom_field_values
	_, err = tdb.Exec(`INSERT INTO assets (id, set_id, asset_type_id, category_id, status_id, title, custom_field_values, created_by)
		VALUES (6, 1, 1, 1, 1, 'Asset EmptyObj', '{}', 1)`)
	if err != nil {
		t.Fatalf("Failed to insert asset 6: %v", err)
	}
	assetIDs["emptyobj"] = 6

	return &assetTestData{
		tdb:            tdb,
		setID:          1,
		setMap:         map[string]int{"test set": 1},
		customFieldMap: map[string]int{"time estimate": 1, "location": 2},
		assetIDs:       assetIDs,
	}
}

func queryAssetIDs(t *testing.T, data *assetTestData, cqlQuery string) []int {
	t.Helper()

	evaluator := NewAssetEvaluator(data.setMap, nil, data.customFieldMap, "sqlite")
	cqlSQL, cqlArgs, err := evaluator.EvaluateToSQL(cqlQuery)
	if err != nil {
		t.Fatalf("EvaluateToSQL failed for %q: %v", cqlQuery, err)
	}

	query := fmt.Sprintf(`SELECT a.id FROM assets a
		LEFT JOIN asset_management_sets ams ON a.set_id = ams.id
		LEFT JOIN asset_types at ON a.asset_type_id = at.id
		LEFT JOIN asset_categories ac ON a.category_id = ac.id
		LEFT JOIN asset_statuses ast ON a.status_id = ast.id
		LEFT JOIN users u ON a.created_by = u.id
		WHERE a.set_id = ? AND (%s)
		ORDER BY a.id`, cqlSQL)

	args := []interface{}{data.setID}
	args = append(args, cqlArgs...)

	rows, err := data.tdb.Query(query, args...)
	if err != nil {
		t.Fatalf("Query failed for CQL %q:\n  SQL: %s\n  Args: %v\n  Error: %v", cqlQuery, query, args, err)
	}
	defer rows.Close()

	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("Scan failed: %v", err)
		}
		ids = append(ids, id)
	}
	return ids
}

func assertIDs(t *testing.T, label string, got []int, want ...int) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: got %v (len %d), want %v (len %d)", label, got, len(got), want, len(want))
		return
	}
	wantSet := make(map[int]bool)
	for _, w := range want {
		wantSet[w] = true
	}
	for _, g := range got {
		if !wantSet[g] {
			t.Errorf("%s: unexpected ID %d in result %v, want %v", label, g, got, want)
		}
	}
}

func TestAssetCQLCustomFields(t *testing.T) {
	data := setupAssetTestDB(t)

	t.Run("string equality cf_Location", func(t *testing.T) {
		ids := queryAssetIDs(t, data, `cf_Location = "Building A"`)
		assertIDs(t, "cf_Location = Building A", ids, 1)
	})

	t.Run("string match cf_Time Estimate", func(t *testing.T) {
		// CQL string "20" should match both JSON string "20" and JSON number 20
		ids := queryAssetIDs(t, data, "`cf_Time Estimate` = \"20\"")
		assertIDs(t, "cf_Time Estimate = \"20\"", ids, 1, 2)
	})

	t.Run("number match cf_Time Estimate", func(t *testing.T) {
		// CQL number 20 should match both JSON string "20" and JSON number 20
		ids := queryAssetIDs(t, data, "`cf_Time Estimate` = 20")
		assertIDs(t, "cf_Time Estimate = 20", ids, 1, 2)
	})

	t.Run("NULL custom_field_values no error", func(t *testing.T) {
		ids := queryAssetIDs(t, data, `cf_Location = "Building A"`)
		for _, id := range ids {
			if id == data.assetIDs["null"] {
				t.Error("NULL asset should not match")
			}
		}
	})

	t.Run("empty string custom_field_values no error", func(t *testing.T) {
		ids := queryAssetIDs(t, data, `cf_Location = "Building A"`)
		for _, id := range ids {
			if id == data.assetIDs["empty"] {
				t.Error("empty string asset should not match")
			}
		}
	})

	t.Run("empty object no match", func(t *testing.T) {
		ids := queryAssetIDs(t, data, `cf_Location = "Building A"`)
		for _, id := range ids {
			if id == data.assetIDs["emptyobj"] {
				t.Error("empty object asset should not match")
			}
		}
	})

	t.Run("missing field no match", func(t *testing.T) {
		ids := queryAssetIDs(t, data, "`cf_Time Estimate` = \"20\"")
		for _, id := range ids {
			if id == data.assetIDs["nofield"] {
				t.Error("asset without Time Estimate field should not match")
			}
		}
	})

	t.Run("standard field status", func(t *testing.T) {
		ids := queryAssetIDs(t, data, `status = "Active"`)
		assertIDs(t, "status = Active", ids, 1, 2, 3, 4, 5, 6)
	})

	t.Run("combined filter", func(t *testing.T) {
		ids := queryAssetIDs(t, data, `status = "Active" AND cf_Location = "Building A"`)
		assertIDs(t, "status + location", ids, 1)
	})

	t.Run("not equal cf_Location", func(t *testing.T) {
		ids := queryAssetIDs(t, data, `cf_Location != "Building A"`)
		// Assets 2 and 3 have Location != "Building A"; 4,5,6 have NULL/empty/missing → no match
		assertIDs(t, "cf_Location != Building A", ids, 2, 3)
	})

	t.Run("number range greater than", func(t *testing.T) {
		// Both "20" (string) and 20 (number) should be > 10 when cast to NUMERIC
		ids := queryAssetIDs(t, data, "`cf_Time Estimate` > 10")
		assertIDs(t, "cf_Time Estimate > 10", ids, 1, 2)
	})

	t.Run("number range less than", func(t *testing.T) {
		// No assets have Time Estimate < 10
		ids := queryAssetIDs(t, data, "`cf_Time Estimate` < 10")
		assertIDs(t, "cf_Time Estimate < 10", ids)
	})
}
