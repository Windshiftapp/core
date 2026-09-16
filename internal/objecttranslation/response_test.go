package objecttranslation

import (
	"context"
	"path/filepath"
	"testing"

	"windshift/internal/database"
	"windshift/internal/models"
)

func TestLinkTypeDirectionalDisplayValues(t *testing.T) {
	db, err := database.NewSQLiteDB(filepath.Join(t.TempDir(), "translations.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Initialize(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	service := NewService(db)
	if err := service.SyncSystem(ctx, ShippedSystemTranslations()); err != nil {
		t.Fatal(err)
	}
	var builtin models.LinkType
	if err := db.QueryRow(`SELECT id, name, forward_label, reverse_label
		FROM link_types WHERE builtin_key = 'implements'`).Scan(
		&builtin.ID, &builtin.Name, &builtin.ForwardLabel, &builtin.ReverseLabel,
	); err != nil {
		t.Fatal(err)
	}
	custom := models.LinkType{Name: "Reviews", ForwardLabel: "reviews", ReverseLabel: "reviewed by"}
	if err := db.QueryRow(`INSERT INTO link_types (name, forward_label, reverse_label)
		VALUES (?, ?, ?) RETURNING id`, custom.Name, custom.ForwardLabel, custom.ReverseLabel).Scan(&custom.ID); err != nil {
		t.Fatal(err)
	}

	t.Run("system locale fallback and custom canonical labels", func(t *testing.T) {
		items := []models.LinkType{builtin, custom}
		if err := service.LocalizeResponse(ctx, "ru-RU", "link_type", &items); err != nil {
			t.Fatal(err)
		}
		if items[0].DisplayForwardLabel != "Реализует" || items[0].DisplayReverseLabel != "Реализуется через" {
			t.Fatalf("unexpected Russian direction labels: %+v", items[0])
		}
		if items[0].ForwardLabel != "implements" || items[0].ReverseLabel != "implemented by" {
			t.Fatalf("canonical labels were changed: %+v", items[0])
		}
		if items[1].DisplayForwardLabel != custom.ForwardLabel || items[1].DisplayReverseLabel != custom.ReverseLabel {
			t.Fatalf("custom canonical labels were lost: %+v", items[1])
		}
	})

	t.Run("instance overrides take precedence for both directions", func(t *testing.T) {
		for field, value := range map[string]string{
			FieldForwardLabel: "Выполняет требование", FieldReverseLabel: "Выполняется задачей",
		} {
			if _, err := service.UpsertInstance(ctx, "link_type", builtin.ID, field, "ru", value); err != nil {
				t.Fatal(err)
			}
		}
		item := builtin
		if err := service.LocalizeResponse(ctx, "ru-RU", "link_type", &item); err != nil {
			t.Fatal(err)
		}
		if item.DisplayForwardLabel != "Выполняет требование" || item.DisplayReverseLabel != "Выполняется задачей" {
			t.Fatalf("instance translations were not preferred: %+v", item)
		}
	})

	t.Run("untranslated locale preserves canonical labels", func(t *testing.T) {
		item := builtin
		if err := service.LocalizeResponse(ctx, "ja", "link_type", &item); err != nil {
			t.Fatal(err)
		}
		if item.DisplayForwardLabel != builtin.ForwardLabel || item.DisplayReverseLabel != builtin.ReverseLabel {
			t.Fatalf("unexpected untranslated labels: %+v", item)
		}
	})

	t.Run("existing catalogs without direction fields still localize", func(t *testing.T) {
		var priority models.Priority
		if err := db.QueryRow(`SELECT id, name FROM priorities ORDER BY id LIMIT 1`).Scan(&priority.ID, &priority.Name); err != nil {
			t.Fatal(err)
		}
		if err := service.LocalizeResponse(ctx, "ja", "priority", &priority); err != nil {
			t.Fatal(err)
		}
		if priority.DisplayName != priority.Name {
			t.Fatalf("priority canonical fallback was lost: %+v", priority)
		}
	})
}
