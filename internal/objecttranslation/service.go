package objecttranslation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/text/language"

	"windshift/internal/database"
)

// Translation is one sparse localized field value.
type Translation struct {
	ObjectType string    `json:"object_type"`
	ObjectID   int       `json:"object_id"`
	Field      string    `json:"field"`
	Locale     string    `json:"locale"`
	Source     string    `json:"source"`
	Value      string    `json:"value"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Target describes a canonical value that may have a localized display value.
type Target struct {
	ObjectType string `json:"object_type"`
	ObjectID   int    `json:"object_id"`
	Field      string `json:"field"`
	Fallback   string `json:"fallback"`
}

// ResolvedValue retains the canonical fallback and reports the selected display value.
type ResolvedValue struct {
	ObjectType string `json:"object_type"`
	ObjectID   int    `json:"object_id"`
	Field      string `json:"field"`
	Value      string `json:"value"`
	Locale     string `json:"locale,omitempty"`
	Source     string `json:"source"`
}

// SystemTranslation binds shipped copy to an object through its immutable built-in key.
type SystemTranslation struct {
	ObjectType string
	BuiltinKey string
	Field      string
	Locale     string
	Value      string
}

// CanonicalDifference identifies built-in copy that cannot be assigned a source locale safely.
type CanonicalDifference struct {
	ObjectType string `json:"object_type"`
	ObjectID   int    `json:"object_id"`
	BuiltinKey string `json:"builtin_key"`
	Field      string `json:"field"`
	Canonical  string `json:"canonical"`
	System     string `json:"system"`
}

type cacheKey struct {
	objectType string
	objectID   int
	field      string
	locale     string
	source     string
}

type cacheValue struct {
	value string
	found bool
}

// Service validates, stores, and resolves instance-wide object translations.
type Service struct {
	db    database.Database
	mu    sync.RWMutex
	cache map[cacheKey]cacheValue
}

// NewService creates an object translation service.
func NewService(db database.Database) *Service {
	return &Service{db: db, cache: make(map[cacheKey]cacheValue)}
}

// NormalizeLocale parses and canonicalizes a BCP 47 locale.
func NormalizeLocale(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.Contains(raw, "_") {
		return "", fmt.Errorf("%w: %q", ErrInvalidLocale, raw)
	}
	tag, err := language.Parse(raw)
	if err != nil || tag == language.Und {
		return "", fmt.Errorf("%w: %q", ErrInvalidLocale, raw)
	}
	return tag.String(), nil
}

func localeCandidates(locale string) []string {
	tag := language.Make(locale)
	candidates := []string{tag.String()}
	if parent := tag.Parent(); parent != language.Und && parent != tag {
		candidates = append(candidates, parent.String())
	}
	return candidates
}

// List returns every translation row for one object.
func (s *Service) List(ctx context.Context, objectType string, objectID int) ([]Translation, error) {
	if objectID <= 0 {
		return nil, fmt.Errorf("%w: %s %d", ErrObjectNotFound, objectType, objectID)
	}
	spec, err := lookupObjectSpec(objectType)
	if err != nil {
		return nil, err
	}
	if err := s.requireOwner(ctx, s.db, spec, objectID, false); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT object_type, object_id, field, locale, source, value, created_at, updated_at
		FROM object_translations
		WHERE object_type = ? AND object_id = ?
		ORDER BY field, locale, source
	`, objectType, objectID)
	if err != nil {
		return nil, fmt.Errorf("list object translations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	translations := make([]Translation, 0)
	for rows.Next() {
		translation, err := scanTranslation(rows)
		if err != nil {
			return nil, err
		}
		translations = append(translations, translation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate object translations: %w", err)
	}
	return translations, nil
}

// UpsertInstance creates or replaces one administrator-managed translation.
func (s *Service) UpsertInstance(ctx context.Context, objectType string, objectID int, field, locale, value string) (Translation, error) {
	spec, normalizedLocale, value, err := validateWrite(objectType, objectID, field, locale, value)
	if err != nil {
		return Translation{}, err
	}

	err = database.WithTx(s.db, func(tx database.Tx) error {
		if err := s.requireOwner(ctx, tx, spec, objectID, true); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO object_translations (object_type, object_id, field, locale, source, value)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT (object_type, object_id, field, locale, source)
			DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP
		`, objectType, objectID, field, normalizedLocale, SourceInstance, value)
		if err != nil {
			return fmt.Errorf("upsert instance translation: %w", err)
		}
		return nil
	})
	if err != nil {
		return Translation{}, err
	}

	s.invalidate(objectType, objectID, field, normalizedLocale)
	return s.get(ctx, objectType, objectID, field, normalizedLocale, SourceInstance)
}

// DeleteInstance removes one administrator-managed translation.
func (s *Service) DeleteInstance(ctx context.Context, objectType string, objectID int, field, locale string) error {
	if _, err := lookupSpec(objectType, field); err != nil {
		return err
	}
	normalizedLocale, err := NormalizeLocale(locale)
	if err != nil {
		return err
	}
	result, err := s.db.ExecWriteContext(ctx, `
		DELETE FROM object_translations
		WHERE object_type = ? AND object_id = ? AND field = ? AND locale = ? AND source = ?
	`, objectType, objectID, field, normalizedLocale, SourceInstance)
	if err != nil {
		return fmt.Errorf("delete instance translation: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted translation count: %w", err)
	}
	if affected == 0 {
		return ErrTranslationNotFound
	}
	s.invalidate(objectType, objectID, field, normalizedLocale)
	return nil
}

// Resolve selects display values in bounded set queries grouped by object type and field.
func (s *Service) Resolve(ctx context.Context, locale string, targets []Target) ([]ResolvedValue, error) {
	normalizedLocale, err := NormalizeLocale(locale)
	if err != nil {
		return nil, err
	}
	type groupKey struct {
		objectType string
		field      string
	}
	groups := make(map[groupKey][]int)
	for _, target := range targets {
		if target.ObjectID <= 0 {
			return nil, fmt.Errorf("%w: %s %d", ErrObjectNotFound, target.ObjectType, target.ObjectID)
		}
		if _, err := lookupSpec(target.ObjectType, target.Field); err != nil {
			return nil, err
		}
		key := groupKey{objectType: target.ObjectType, field: target.Field}
		if !slices.Contains(groups[key], target.ObjectID) {
			groups[key] = append(groups[key], target.ObjectID)
		}
	}

	candidates := localeCandidates(normalizedLocale)
	for key, ids := range groups {
		if err := s.loadCandidates(ctx, key.objectType, key.field, candidates, ids); err != nil {
			return nil, err
		}
	}

	resolved := make([]ResolvedValue, 0, len(targets))
	for _, target := range targets {
		value := ResolvedValue{
			ObjectType: target.ObjectType,
			ObjectID:   target.ObjectID,
			Field:      target.Field,
			Value:      target.Fallback,
			Source:     "canonical",
		}
		for _, source := range []string{SourceInstance, SourceSystem} {
			for _, candidate := range candidates {
				entry := s.cached(cacheKey{target.ObjectType, target.ObjectID, target.Field, candidate, source})
				if !entry.found {
					continue
				}
				value.Value = entry.value
				value.Locale = candidate
				value.Source = source
				break
			}
			if value.Source != "canonical" {
				break
			}
		}
		resolved = append(resolved, value)
	}
	return resolved, nil
}

// SyncSystem updates shipped rows by immutable built-in key without touching instance rows.
func (s *Service) SyncSystem(ctx context.Context, translations []SystemTranslation) error {
	touched := make([]cacheKey, 0, len(translations))
	err := database.WithTx(s.db, func(tx database.Tx) error {
		for _, translation := range translations {
			spec, normalizedLocale, value, err := validateWrite(
				translation.ObjectType, 1, translation.Field, translation.Locale, translation.Value,
			)
			if err != nil {
				return err
			}
			if strings.TrimSpace(translation.BuiltinKey) == "" {
				return errors.New("system translation built-in key is required")
			}

			var objectID int
			err = tx.QueryRowContext(ctx,
				fmt.Sprintf("SELECT id FROM %s WHERE builtin_key = ?", spec.table),
				translation.BuiltinKey,
			).Scan(&objectID)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				return fmt.Errorf("resolve system translation owner: %w", err)
			}
			_, err = tx.ExecContext(ctx, `
				INSERT INTO object_translations (object_type, object_id, field, locale, source, value)
				VALUES (?, ?, ?, ?, ?, ?)
				ON CONFLICT (object_type, object_id, field, locale, source)
				DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP
				WHERE object_translations.value <> excluded.value
			`, translation.ObjectType, objectID, translation.Field, normalizedLocale, SourceSystem, value)
			if err != nil {
				return fmt.Errorf("sync system translation: %w", err)
			}
			touched = append(touched, cacheKey{translation.ObjectType, objectID, translation.Field, normalizedLocale, SourceSystem})
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, key := range touched {
		s.invalidate(key.objectType, key.objectID, key.field, key.locale)
	}
	return nil
}

// FindOrphans reports rows whose allowlisted owner no longer exists.
func (s *Service) FindOrphans(ctx context.Context) ([]Translation, error) {
	orphans := make([]Translation, 0)
	for _, spec := range registry {
		query := fmt.Sprintf(`
			SELECT ot.object_type, ot.object_id, ot.field, ot.locale, ot.source, ot.value, ot.created_at, ot.updated_at
			FROM object_translations ot
			LEFT JOIN %s owner ON owner.id = ot.object_id
			WHERE ot.object_type = ? AND owner.id IS NULL
			ORDER BY ot.object_id, ot.field, ot.locale, ot.source
		`, spec.table)
		rows, err := s.db.QueryContext(ctx, query, spec.objectType)
		if err != nil {
			return nil, fmt.Errorf("find %s translation orphans: %w", spec.objectType, err)
		}
		for rows.Next() {
			translation, scanErr := scanTranslation(rows)
			if scanErr != nil {
				_ = rows.Close()
				return nil, scanErr
			}
			orphans = append(orphans, translation)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("iterate %s translation orphans: %w", spec.objectType, err)
		}
		_ = rows.Close()
	}
	return orphans, nil
}

// FindCanonicalDifferences reports built-in values whose source locale cannot be inferred safely.
func (s *Service) FindCanonicalDifferences(ctx context.Context, translations []SystemTranslation) ([]CanonicalDifference, error) {
	differences := make([]CanonicalDifference, 0)
	seen := make(map[string]struct{})
	for _, translation := range translations {
		if translation.Locale != "en" {
			continue
		}
		spec, err := lookupSpec(translation.ObjectType, translation.Field)
		if err != nil {
			return nil, err
		}
		key := strings.Join([]string{translation.ObjectType, translation.BuiltinKey, translation.Field}, "\x00")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		var objectID int
		var canonical sql.NullString
		err = s.db.QueryRowContext(ctx,
			fmt.Sprintf("SELECT id, %s FROM %s WHERE builtin_key = ?", translation.Field, spec.table),
			translation.BuiltinKey,
		).Scan(&objectID, &canonical)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("inspect canonical system value: %w", err)
		}
		if canonical.String == translation.Value {
			continue
		}
		differences = append(differences, CanonicalDifference{
			ObjectType: translation.ObjectType,
			ObjectID:   objectID,
			BuiltinKey: translation.BuiltinKey,
			Field:      translation.Field,
			Canonical:  canonical.String,
			System:     translation.Value,
		})
	}
	return differences, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanTranslation(row rowScanner) (Translation, error) {
	var translation Translation
	if err := row.Scan(
		&translation.ObjectType, &translation.ObjectID, &translation.Field, &translation.Locale,
		&translation.Source, &translation.Value, &translation.CreatedAt, &translation.UpdatedAt,
	); err != nil {
		return Translation{}, fmt.Errorf("scan object translation: %w", err)
	}
	return translation, nil
}

func validateWrite(objectType string, objectID int, field, locale, value string) (
	spec objectSpec,
	normalizedLocale string,
	normalizedValue string,
	err error,
) {
	spec, err = lookupSpec(objectType, field)
	if err != nil {
		return objectSpec{}, "", "", err
	}
	if objectID <= 0 {
		return objectSpec{}, "", "", fmt.Errorf("%w: %s %d", ErrObjectNotFound, objectType, objectID)
	}
	normalizedLocale, err = NormalizeLocale(locale)
	if err != nil {
		return objectSpec{}, "", "", err
	}
	normalizedValue = strings.TrimSpace(value)
	if normalizedValue == "" {
		return objectSpec{}, "", "", fmt.Errorf("%w: value is required", ErrInvalidValue)
	}
	if len(normalizedValue) > 10000 {
		return objectSpec{}, "", "", fmt.Errorf("%w: value exceeds 10000 bytes", ErrInvalidValue)
	}
	return spec, normalizedLocale, normalizedValue, nil
}

func (s *Service) requireOwner(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, spec objectSpec, objectID int, lock bool) error {
	query := fmt.Sprintf("SELECT id FROM %s WHERE id = ?", spec.table)
	if lock && database.IsPostgresDriver(s.db.GetDriverName()) {
		query += " FOR KEY SHARE"
	}
	var id int
	err := queryer.QueryRowContext(ctx, query, objectID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: %s %d", ErrObjectNotFound, spec.objectType, objectID)
	}
	if err != nil {
		return fmt.Errorf("check translated object: %w", err)
	}
	return nil
}

func (s *Service) get(ctx context.Context, objectType string, objectID int, field, locale, source string) (Translation, error) {
	translation, err := scanTranslation(s.db.QueryRowContext(ctx, `
		SELECT object_type, object_id, field, locale, source, value, created_at, updated_at
		FROM object_translations
		WHERE object_type = ? AND object_id = ? AND field = ? AND locale = ? AND source = ?
	`, objectType, objectID, field, locale, source))
	if errors.Is(err, sql.ErrNoRows) {
		return Translation{}, ErrTranslationNotFound
	}
	return translation, err
}

func (s *Service) loadCandidates(ctx context.Context, objectType, field string, locales []string, ids []int) error {
	unknown := false
	s.mu.RLock()
	for _, id := range ids {
		for _, locale := range locales {
			for _, source := range []string{SourceInstance, SourceSystem} {
				if _, ok := s.cache[cacheKey{objectType, id, field, locale, source}]; !ok {
					unknown = true
					break
				}
			}
		}
	}
	s.mu.RUnlock()
	if !unknown {
		return nil
	}

	localePlaceholders := strings.TrimSuffix(strings.Repeat("?,", len(locales)), ",")
	idPlaceholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, 2+len(locales)+len(ids))
	args = append(args, objectType, field)
	for _, locale := range locales {
		args = append(args, locale)
	}
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT object_type, object_id, field, locale, source, value, created_at, updated_at
		FROM object_translations
		WHERE object_type = ? AND field = ?
		  AND locale IN (%s) AND object_id IN (%s)
	`, localePlaceholders, idPlaceholders), args...)
	if err != nil {
		return fmt.Errorf("load translation candidates: %w", err)
	}
	defer func() { _ = rows.Close() }()

	loaded := make(map[cacheKey]cacheValue)
	for rows.Next() {
		translation, err := scanTranslation(rows)
		if err != nil {
			return err
		}
		loaded[cacheKey{
			translation.ObjectType, translation.ObjectID, translation.Field, translation.Locale, translation.Source,
		}] = cacheValue{value: translation.Value, found: true}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate translation candidates: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range ids {
		for _, locale := range locales {
			for _, source := range []string{SourceInstance, SourceSystem} {
				key := cacheKey{objectType, id, field, locale, source}
				if value, ok := loaded[key]; ok {
					s.cache[key] = value
				} else {
					s.cache[key] = cacheValue{}
				}
			}
		}
	}
	return nil
}

func (s *Service) cached(key cacheKey) cacheValue {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cache[key]
}

func (s *Service) invalidate(objectType string, objectID int, field, locale string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cache, cacheKey{objectType, objectID, field, locale, SourceInstance})
	delete(s.cache, cacheKey{objectType, objectID, field, locale, SourceSystem})
}
