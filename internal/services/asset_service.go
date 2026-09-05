package services

import (
	stdctx "context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"windshift/internal/assetevents"
	"windshift/internal/database"
	"windshift/internal/events"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/utils"
	"windshift/internal/validation"
)

// sanitizeAssetText centralizes title, description, and tag policies for both authenticated surfaces.
func sanitizeAssetText(title, description, assetTag *string) {
	sanitize.ApplyAll(
		sanitize.Pair{Target: title, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: description, Policy: sanitize.RichText},
		sanitize.Pair{Target: assetTag, Policy: sanitize.ShortIdentifier},
	)
}

// reencodeCustomFieldValues refreshes the pre-encoded JSON payload from
// the (caller-supplied) values map. Handlers marshal custom_field_values
// before calling the service, but ValidateCustomFieldsSchema sanitizes
// text/textarea values in the map afterwards — without a re-encode the
// sanitized values would never reach persistence. No-op when the caller
// didn't supply a values map (partial update keeping stored values).
func reencodeCustomFieldValues(values map[string]any, target **string) error {
	if values == nil {
		return nil
	}
	b, err := json.Marshal(values)
	if err != nil {
		return fmt.Errorf("encode custom field values: %w", err)
	}
	s := string(b)
	*target = &s
	return nil
}

// AuditActor carries the actor + transport context an audit event needs.
// Both the cookie-auth and bearer-auth surfaces build this from their
// *http.Request before calling into AssetService so the service layer
// stays HTTP-agnostic and the two surfaces produce identical audit rows
// for equivalent operations.
//
// AuthMethod / APITokenID / APITokenPrefix / APITokenName are populated
// when the request was bearer-token authenticated; cookie-auth requests
// leave them zero. Compromised-token investigations switch on these to
// identify the specific token that drove a mutation under a user the
// attacker may share with many tokens.
type AuditActor struct {
	UserID         int
	Username       string
	IPAddress      string
	UserAgent      string
	AuthMethod     string
	Source         string
	APITokenID     int
	APITokenPrefix string
	APITokenName   string
}

// AssetAutomationContext preserves cascade metadata when an automation node
// routes a mutation through the same AssetService boundary as interactive
// writes. Zero values describe an ordinary user-initiated mutation.
type AssetAutomationContext struct {
	TriggeredByAction bool
	ExecutionChainID  string
	CascadeDepth      int
	SourceApplication string
	CausationEventKey string
}

// AssetMutationPatch describes the fields an automation node changes. The
// service reloads all untouched values and validates the complete resulting
// asset through UpdateAsset before persisting it.
type AssetMutationPatch struct {
	Title             *string
	Description       *string
	AssetTag          *string
	StatusID          *int
	CustomFieldValues map[string]any
}

// NewAuditActorFromRequest extracts the audit fields from a request +
// authenticated user. Convenience shared by both surfaces. authMethod
// is "cookie" or "bearer" (handlers know which they are); apiToken is
// non-nil only on the bearer path and gets unpacked into the actor's
// token-attribution fields. Passing both args explicitly keeps the
// services package off the restapi import path (no context-key dep).
func NewAuditActorFromRequest(r *http.Request, user *models.User, apiToken *models.APIToken, authMethod string) AuditActor {
	actor := AuditActor{
		IPAddress:  utils.GetClientIP(r),
		UserAgent:  r.UserAgent(),
		AuthMethod: authMethod,
	}
	if user != nil {
		actor.UserID = user.ID
		actor.Username = user.Username
	}
	if apiToken != nil {
		actor.APITokenID = apiToken.ID
		actor.APITokenPrefix = apiToken.TokenPrefix
		actor.APITokenName = apiToken.Name
		if actor.AuthMethod == "" {
			actor.AuthMethod = "bearer"
		}
	}
	return actor
}

// AssetValidationError signals a user-facing validation failure (400 at
// the HTTP layer) — as opposed to repo / IO errors which the caller
// renders as 500. Handlers use errors.As to switch on it.
type AssetValidationError struct{ Msg string }

func (e *AssetValidationError) Error() string { return e.Msg }

// IsAssetValidationError reports whether err is (or wraps) an
// AssetValidationError. Handlers use this when rendering 400 vs 500.
func IsAssetValidationError(err error) (*AssetValidationError, bool) {
	var ve *AssetValidationError
	if errors.As(err, &ve) {
		return ve, true
	}
	return nil, false
}

// AssetService owns the asset mutation pipeline: repo writes, audit
// emission, automation-event emission, and custom-field schema
// validation. Both /api/assets (cookie auth) and /rest/api/v1/assets
// (bearer auth) flow through here so a single audit row + a single
// automation event is produced per mutation, regardless of which
// surface drove it.
type AssetService struct {
	db            database.Database
	repo          *repository.AssetRepository
	eventRecorder assetEventRecorder
	compatibility *DurableAssetActionIngress
}

// NewAssetService constructs an AssetService backed by the given repository.
func NewAssetService(db database.Database, repo *repository.AssetRepository) *AssetService {
	return &AssetService{
		db: db, repo: repo, eventRecorder: assetevents.NewRecorder(db),
		compatibility: NewDurableAssetActionIngress(db),
	}
}

type assetEventRecorder interface {
	Created(stdctx.Context, database.Tx, assetevents.AssetSnapshot, map[string]any, assetevents.Metadata) (*events.Event, error)
	Updated(stdctx.Context, database.Tx, assetevents.AssetSnapshot, map[string]any, map[string]any, assetevents.Metadata) (*events.Event, error)
	StatusChanged(stdctx.Context, database.Tx, assetevents.AssetSnapshot, *int, *int, map[string]any, map[string]any, assetevents.Metadata) (*events.Event, error)
	Deleted(stdctx.Context, database.Tx, assetevents.AssetSnapshot, map[string]any, assetevents.Metadata) (*events.Event, error)
}

func (s *AssetService) events() assetEventRecorder {
	if s.eventRecorder == nil {
		return assetevents.NewRecorder(s.db)
	}
	return s.eventRecorder
}

func (s *AssetService) compatibilityEvents() *DurableAssetActionIngress {
	if s.compatibility == nil {
		return NewDurableAssetActionIngress(s.db)
	}
	return s.compatibility
}

// ValidateActionTaxonomyReferences rejects asset action nodes whose taxonomy
// IDs do not belong to the configured asset set.
func (s *AssetService) ValidateActionTaxonomyReferences(nodes []models.ActionNode) error {
	for i, node := range nodes {
		switch node.NodeType {
		case models.ActionNodeCreateAsset:
			var config models.CreateAssetNodeConfig
			if err := json.Unmarshal([]byte(node.NodeConfig), &config); err != nil {
				return fmt.Errorf("nodes[%d].node_config: parse create_asset config: %w", i, err)
			}
			if err := s.validateActionTaxonomy(config.AssetSetID, config.AssetTypeID, config.CategoryID, config.StatusID); err != nil {
				return fmt.Errorf("nodes[%d].node_config: %w", i, err)
			}
		case models.ActionNodeUpdateAsset:
			var config models.UpdateAssetNodeConfig
			if err := json.Unmarshal([]byte(node.NodeConfig), &config); err != nil {
				return fmt.Errorf("nodes[%d].node_config: parse update_asset config: %w", i, err)
			}
			if err := s.validateActionTaxonomy(config.AssetSetID, config.AssetTypeID, nil, nil); err != nil {
				return fmt.Errorf("nodes[%d].node_config: %w", i, err)
			}
		}
	}
	return nil
}

func (s *AssetService) validateActionTaxonomy(setID, typeID int, categoryID, statusID *int) error {
	if setID <= 0 {
		return fmt.Errorf("asset_set_id must be positive")
	}
	return s.validateAssetTaxonomy(setID, typeID, categoryID, statusID)
}

// FindAsset returns the complete asset model used by mutation adapters.
func (s *AssetService) FindAsset(assetID int) (*models.Asset, error) {
	row, err := s.repo.FindAssetFullByID(assetID)
	if err != nil {
		return nil, err
	}
	asset := repository.AssetRowToModel(*row)
	return &asset, nil
}

func assetCreatedValues(assetTypeID int, statusID, categoryID *int, title, description, assetTag string, customFieldValues map[string]any) map[string]any {
	newValues := map[string]any{
		"title":         title,
		"description":   description,
		"asset_tag":     assetTag,
		"asset_type_id": assetTypeID,
	}
	if statusID != nil {
		newValues["status_id"] = *statusID
	}
	if categoryID != nil {
		newValues["category_id"] = *categoryID
	}
	for key, value := range customFieldValues {
		newValues[key] = value
	}
	return newValues
}

func assetEventMetadata(actorUserID int, sourceKind string, automation AssetAutomationContext) assetevents.Metadata {
	metadata := assetevents.System(sourceKind)
	if actorUserID > 0 {
		metadata = assetevents.User(actorUserID, sourceKind)
	}
	metadata.CorrelationID = automation.ExecutionChainID
	metadata.CausationEventKey = automation.CausationEventKey
	if automation.TriggeredByAction || automation.ExecutionChainID != "" || automation.CascadeDepth > 0 || automation.SourceApplication != "" {
		metadata.SourceKind = "automation"
		metadata.SourceRef = automation.SourceApplication
		metadata.Automation = &assetevents.AutomationContext{
			TriggeredByAction: automation.TriggeredByAction,
			ExecutionChainID:  automation.ExecutionChainID,
			CascadeDepth:      automation.CascadeDepth,
			SourceApplication: automation.SourceApplication,
		}
	}
	return metadata
}

func assetEventSnapshot(assetID, setID, assetTypeID int, categoryID, statusID *int, title, description, assetTag string) assetevents.AssetSnapshot {
	return assetevents.AssetSnapshot{
		ID: assetID, SetID: setID, AssetTypeID: assetTypeID,
		CategoryID: categoryID, StatusID: statusID, Title: title,
		Description: description, AssetTag: assetTag,
	}
}

func assetCompatibilityEvent(eventType models.AssetActionTriggerType, snapshot assetevents.AssetSnapshot, actorUserID int, oldValues, newValues map[string]any, automation AssetAutomationContext) *models.AssetActionEvent {
	return &models.AssetActionEvent{
		EventType: eventType, SetID: snapshot.SetID, AssetID: snapshot.ID, ActorUserID: actorUserID,
		OldValues: oldValues, NewValues: newValues,
		TriggeredByAction: automation.TriggeredByAction, ExecutionChainID: automation.ExecutionChainID,
		CascadeDepth: automation.CascadeDepth, SourceApplication: automation.SourceApplication,
		CausationEventKey: automation.CausationEventKey,
	}
}

func sameOptionalAssetID(left, right *int) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func automationAuditActor(db database.Database, userID int, source string) AuditActor {
	actor := AuditActor{UserID: userID, Source: source}
	if userID > 0 {
		_ = db.QueryRow(`SELECT username FROM users WHERE id = ?`, userID).Scan(&actor.Username)
	}
	return actor
}

// CustomFieldsValidationOpts controls required-field validation for creates and
// full replacements; partial updates leave it disabled.
type CustomFieldsValidationOpts struct {
	EnforceRequired bool
}

// ValidateCustomFieldsSchema rejects unknown or invalid values, sanitizes text
// in place, and optionally requires all mandatory fields. It accepts legacy ID
// keys and case-insensitive field names.
func (s *AssetService) ValidateCustomFieldsSchema(assetTypeID int, values map[string]any, opts CustomFieldsValidationOpts) error {
	if len(values) == 0 && !opts.EnforceRequired {
		return nil
	}
	fields, err := s.repo.FindAssetTypeFields(assetTypeID)
	if err != nil {
		return fmt.Errorf("load asset type fields: %w", err)
	}
	return validateCustomFieldsSchemaCore(fields, values, opts)
}

// validateCustomFieldsSchemaCore validates an already-loaded field list in place.
func validateCustomFieldsSchemaCore(fields []models.AssetTypeField, values map[string]any, opts CustomFieldsValidationOpts) error {
	if len(values) == 0 && !opts.EnforceRequired {
		return nil
	}
	byKey := make(map[string]models.AssetTypeField, len(fields)*2)
	for _, f := range fields {
		byKey[fmt.Sprintf("%d", f.CustomFieldID)] = f
		byKey[strings.ToLower(f.FieldName)] = f
	}

	var unknown []string
	for k := range values {
		if _, ok := byKey[k]; ok {
			continue
		}
		if _, ok := byKey[strings.ToLower(k)]; ok {
			continue
		}
		unknown = append(unknown, k)
	}
	if len(unknown) > 0 {
		return &AssetValidationError{
			Msg: "custom_field_values contains key(s) not declared on the asset type: " + strings.Join(unknown, ", "),
		}
	}

	for k, v := range values {
		f, ok := byKey[k]
		if !ok {
			f = byKey[strings.ToLower(k)]
		}
		if err := validateAssetFieldValue(f, v); err != nil {
			return &AssetValidationError{Msg: fmt.Sprintf("custom_field_values[%q]: %s", k, err.Error())}
		}
		switch f.FieldType {
		case "text", "textarea":
			if s, ok := v.(string); ok {
				if f.FieldType == "textarea" {
					values[k] = sanitize.RichText.Sanitize(s)
				} else {
					values[k] = sanitize.PlainTextField.Sanitize(s)
				}
			}
		}
	}

	if opts.EnforceRequired {
		for _, f := range fields {
			if !f.IsRequired || models.IsBooleanCustomFieldType(f.FieldType) {
				continue
			}
			if !customFieldValuePresent(values, f) {
				return &AssetValidationError{Msg: fmt.Sprintf("custom field %q is required", f.FieldName)}
			}
		}
	}
	return nil
}

// CoerceAndValidateCustomFieldValues normalizes caller-provided custom-field
// values (converting CSV strings to numbers, booleans, arrays, etc.) and runs
// them through the asset type's schema validation. It returns the coerced
// and sanitized map that should be persisted.
func (s *AssetService) CoerceAndValidateCustomFieldValues(assetTypeID int, values map[string]any) (map[string]any, error) {
	fields, err := s.repo.FindAssetTypeFields(assetTypeID)
	if err != nil {
		return nil, fmt.Errorf("load asset type fields: %w", err)
	}
	coerced := coerceCustomFieldValues(fields, values)
	if err := validateCustomFieldsSchemaCore(fields, coerced, CustomFieldsValidationOpts{EnforceRequired: true}); err != nil {
		return nil, err
	}
	return coerced, nil
}

// coerceCustomFieldValues converts string-ish CSV/import values into the
// types expected by the field definition. Non-string values are left
// mostly untouched so values already resolved by callers (e.g. select
// option IDs) are preserved.
func coerceCustomFieldValues(fields []models.AssetTypeField, values map[string]any) map[string]any {
	if len(values) == 0 {
		return values
	}
	byKey := make(map[string]models.AssetTypeField, len(fields)*2)
	for _, f := range fields {
		byKey[fmt.Sprintf("%d", f.CustomFieldID)] = f
		byKey[strings.ToLower(f.FieldName)] = f
		byKey[f.FieldName] = f
	}
	coerced := make(map[string]any, len(values))
	for k, v := range values {
		f, ok := byKey[k]
		if !ok {
			if f, ok = byKey[strings.ToLower(k)]; !ok {
				coerced[k] = v
				continue
			}
		}
		coerced[k] = coerceAssetFieldValue(f, v)
	}
	return coerced
}

// coerceAssetFieldValue converts a single raw value toward the type expected
// by the asset field. The result is still validated by validateAssetFieldValue.
func coerceAssetFieldValue(f models.AssetTypeField, v any) any {
	if v == nil {
		return nil
	}
	switch f.FieldType {
	case "number":
		switch v.(type) {
		case float64, int, int64:
			return v
		}
		if s, ok := v.(string); ok {
			if n, err := strconv.ParseFloat(s, 64); err == nil {
				return n
			}
		}
		return v
	case models.CustomFieldTypeBoolean, models.CustomFieldTypeCheckbox:
		if b, ok := v.(bool); ok {
			return b
		}
		if s, ok := v.(string); ok {
			if b, err := strconv.ParseBool(strings.ToLower(s)); err == nil {
				return b
			}
		}
		return v
	case "user":
		switch v.(type) {
		case float64, int, int64:
			return v
		}
		if s, ok := v.(string); ok {
			if id, err := strconv.Atoi(s); err == nil {
				return id
			}
		}
		return v
	case "multiselect":
		if arr, ok := toInterfaceSlice(v); ok {
			return arr
		}
		if s, ok := v.(string); ok && s != "" {
			parts := strings.Split(s, ",")
			arr := make([]any, 0, len(parts))
			for _, part := range parts {
				if t := strings.TrimSpace(part); t != "" {
					arr = append(arr, t)
				}
			}
			return arr
		}
		return v
	case "select", "date", "text", "textarea":
		return v
	default:
		return v
	}
}

// toInterfaceSlice converts any slice or array value into []any.
func toInterfaceSlice(v any) ([]any, bool) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil, false
	}
	out := make([]any, rv.Len())
	for i := range out {
		out[i] = rv.Index(i).Interface()
	}
	return out, true
}

// SanitizeCustomFieldTextValues runs only the text/textarea sanitize
// pass of ValidateCustomFieldsSchema over a values map, mutating it in
// place. For write paths (automation actions) that merge values into
// stored custom_field_values without the full schema validation —
// unknown keys and non-string values are left untouched so existing
// behavior for those callers is preserved; only string values on
// text/textarea fields get the rendering-matched policies applied.
func (s *AssetService) SanitizeCustomFieldTextValues(assetTypeID int, values map[string]any) error {
	if len(values) == 0 {
		return nil
	}
	fields, err := s.repo.FindAssetTypeFields(assetTypeID)
	if err != nil {
		return fmt.Errorf("load asset type fields: %w", err)
	}
	byKey := make(map[string]models.AssetTypeField, len(fields)*2)
	for _, f := range fields {
		byKey[fmt.Sprintf("%d", f.CustomFieldID)] = f
		byKey[strings.ToLower(f.FieldName)] = f
	}
	for k, v := range values {
		f, ok := byKey[k]
		if !ok {
			if f, ok = byKey[strings.ToLower(k)]; !ok {
				continue
			}
		}
		switch f.FieldType {
		case "text", "textarea":
			if s, ok := v.(string); ok {
				if f.FieldType == "textarea" {
					values[k] = sanitize.RichText.Sanitize(s)
				} else {
					values[k] = sanitize.PlainTextField.Sanitize(s)
				}
			}
		}
	}
	return nil
}

// validateAssetFieldValue type-checks a single field value. Returns
// nil for empty / null values — required-field presence is enforced
// separately by ValidateCustomFieldsSchema when opts.EnforceRequired
// is set, so a value of explicit-null here just means "not set this
// time", not "schema violation".
func validateAssetFieldValue(f models.AssetTypeField, v any) error {
	if v == nil {
		return nil
	}
	switch f.FieldType {
	case "text", "textarea", "":
		// Empty FieldType (unknown legacy types) accept anything stringy.
		if _, ok := v.(string); !ok {
			return fmt.Errorf("expected string for %s field", f.FieldType)
		}
	case "number":
		switch x := v.(type) {
		case float64, int, int64:
			return nil
		case string:
			if _, err := strconv.ParseFloat(x, 64); err != nil {
				return fmt.Errorf("expected numeric value, got %q", x)
			}
		default:
			return fmt.Errorf("expected number")
		}
	case models.CustomFieldTypeBoolean, models.CustomFieldTypeCheckbox:
		_, err := validation.ValidateCheckboxValue(f.FieldName, v)
		return err
	case "date":
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("expected date string")
		}
		if _, err := time.Parse("2006-01-02", s); err == nil {
			return nil
		}
		if _, err := time.Parse(time.RFC3339, s); err == nil {
			return nil
		}
		return fmt.Errorf("expected YYYY-MM-DD or RFC3339 date, got %q", s)
	case "select":
		if !assetFieldOptionAllowed(f, v) {
			return fmt.Errorf("value %v is not an allowed option for this field", v)
		}
	case "multiselect":
		arr, ok := v.([]any)
		if !ok {
			return fmt.Errorf("expected array for multiselect field")
		}
		for _, elem := range arr {
			if !assetFieldOptionAllowed(f, elem) {
				return fmt.Errorf("value %v is not an allowed option for this field", elem)
			}
		}
	case "user":
		switch x := v.(type) {
		case float64, int, int64:
			return nil
		case map[string]any:
			if _, ok := x["id"]; ok {
				return nil
			}
			return fmt.Errorf("user object missing 'id' key")
		default:
			return fmt.Errorf("expected user id (int) or {id: int}")
		}
	}
	return nil
}

// assetFieldOptionAllowed reports whether the given value matches any
// option in the field's declared option whitelist. It accepts either
// an option ID (as a JSON number or numeric string) or an option label
// (legacy string values). Returns true when no options are declared
// (the field accepts any value) or when the stored options JSON is
// malformed (fail-open — better to accept than to block legitimate
// writes against a misconfigured field).
func assetFieldOptionAllowed(f models.AssetTypeField, value any) bool {
	if value == nil || f.Options == "" {
		return true
	}
	opts, err := models.ParseSelectOptions(f.Options)
	if err != nil {
		return true
	}
	allowedIDs := make(map[int]struct{}, len(opts.Items))
	allowedLabels := make(map[string]struct{}, len(opts.Items))
	for _, item := range opts.Items {
		allowedIDs[item.ID] = struct{}{}
		allowedLabels[item.Label] = struct{}{}
	}
	switch v := value.(type) {
	case int:
		_, ok := allowedIDs[v]
		return ok
	case int64:
		_, ok := allowedIDs[int(v)]
		return ok
	case float64:
		_, ok := allowedIDs[int(v)]
		return ok
	case string:
		if _, ok := allowedLabels[v]; ok {
			return true
		}
		// Also accept numeric strings that match an option ID.
		if id, err := strconv.Atoi(v); err == nil {
			_, ok := allowedIDs[id]
			return ok
		}
		return false
	default:
		return false
	}
}

// customFieldValuePresent reports whether the values map carries a
// non-empty value for the given field. Accepts the field-id-string
// key, the lowercased field-name key, and the raw field-name key
// (so an editor that sends mixed-case names is satisfied).
func customFieldValuePresent(values map[string]any, f models.AssetTypeField) bool {
	keys := []string{
		fmt.Sprintf("%d", f.CustomFieldID),
		strings.ToLower(f.FieldName),
		f.FieldName,
	}
	for _, k := range keys {
		if v, ok := values[k]; ok && !isEmptyCustomFieldValue(v) {
			return true
		}
	}
	return false
}

func isEmptyCustomFieldValue(v any) bool {
	if v == nil {
		return true
	}
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x) == ""
	case []any:
		return len(x) == 0
	}
	return false
}

// CreateAsset writes the asset, validates custom field schema, emits the
// audit event, and emits an asset_created automation event when an
// action service is wired. Returns the freshly-loaded row.
//
// All required fields declared on the asset type must be present in
// customFieldValues (EnforceRequired is on for creates).
func (s *AssetService) CreateAsset(actor AuditActor, in repository.CreateAssetInput, customFieldValues map[string]any) (*models.Asset, error) {
	return s.CreateAssetWithContext(actor, in, customFieldValues, AssetAutomationContext{})
}

// CreateAssetWithContext runs the canonical create pipeline while preserving
// automation cascade metadata on the emitted asset-created event.
func (s *AssetService) CreateAssetWithContext(actor AuditActor, in repository.CreateAssetInput, customFieldValues map[string]any, context AssetAutomationContext) (*models.Asset, error) {
	if err := s.validateAssetTaxonomy(in.SetID, in.AssetTypeID, in.CategoryID, in.StatusID); err != nil {
		return nil, err
	}
	if err := s.ValidateCustomFieldsSchema(in.AssetTypeID, customFieldValues, CustomFieldsValidationOpts{EnforceRequired: true}); err != nil {
		return nil, err
	}
	if err := reencodeCustomFieldValues(customFieldValues, &in.CustomFieldValuesJSON); err != nil {
		return nil, err
	}
	if in.StatusID == nil {
		if defaultStatusID, err := s.repo.GetDefaultStatus(in.SetID); err != nil {
			slog.Warn("failed to load default asset status", slog.String("component", "assets"), slog.Int("set_id", in.SetID), slog.Any("error", err))
		} else if defaultStatusID != nil {
			in.StatusID = defaultStatusID
		}
	}
	sanitizeAssetText(&in.Title, &in.Description, &in.AssetTag)
	if strings.TrimSpace(in.Title) == "" {
		return nil, &AssetValidationError{Msg: "title is required"}
	}
	var assetID int
	err := database.WithTx(s.db, func(tx database.Tx) error {
		var err error
		assetID, err = s.repo.CreateAssetInTx(tx, in)
		if err != nil {
			return err
		}
		snapshot := assetEventSnapshot(assetID, in.SetID, in.AssetTypeID, in.CategoryID, in.StatusID, in.Title, in.Description, in.AssetTag)
		newValues := assetCreatedValues(in.AssetTypeID, in.StatusID, in.CategoryID, in.Title, in.Description, in.AssetTag, customFieldValues)
		_, err = s.events().Created(
			stdctx.Background(), tx, snapshot,
			newValues,
			assetEventMetadata(actor.UserID, "application", context),
		)
		if err != nil {
			return err
		}
		return s.compatibilityEvents().EmitInTx(stdctx.Background(), tx, assetCompatibilityEvent(
			models.AssetTriggerAssetCreated, snapshot, actor.UserID, nil, newValues, context,
		))
	})
	if err != nil {
		return nil, fmt.Errorf("create asset: %w", err)
	}
	s.emitAudit(actor, logger.ActionAssetCreate, &assetID, in.Title, nil)
	row, err := s.repo.FindAssetFullByID(assetID)
	if err != nil {
		return nil, fmt.Errorf("reload after create: %w", err)
	}
	m := repository.AssetRowToModel(*row)
	return &m, nil
}

// InsertImportedAsset persists one async-import row and its canonical created
// fact in the same transaction.
func (s *AssetService) InsertImportedAsset(in repository.ImportAssetRowInput) (int, error) {
	customFieldValues := loadStoredCustomFieldValues(in.CustomFieldValuesJSON)
	var assetID int
	err := database.WithTx(s.db, func(tx database.Tx) error {
		if err := s.repo.RenewImportJobLeaseInTx(tx, in.ImportJobID, in.SetID); err != nil {
			return err
		}
		var err error
		assetID, err = s.repo.InsertImportedAssetInTx(tx, in)
		if err != nil {
			return err
		}
		snapshot := assetEventSnapshot(assetID, in.SetID, in.AssetTypeID, in.CategoryID, in.StatusID, in.Title, in.Description, in.AssetTag)
		newValues := assetCreatedValues(in.AssetTypeID, in.StatusID, in.CategoryID, in.Title, in.Description, in.AssetTag, customFieldValues)
		_, err = s.events().Created(
			stdctx.Background(), tx, snapshot,
			newValues,
			assetEventMetadata(in.CreatedBy, "asset_import", AssetAutomationContext{}),
		)
		if err != nil {
			return err
		}
		return s.compatibilityEvents().EmitInTx(stdctx.Background(), tx, assetCompatibilityEvent(
			models.AssetTriggerAssetCreated, snapshot, in.CreatedBy, nil, newValues, AssetAutomationContext{},
		))
	})
	return assetID, err
}

// UpdateAsset writes the (partial) update, validates the custom-field
// schema, emits the audit event, and emits asset_updated +
// asset_status_changed automation events when applicable. oldSnap (read
// from repo.GetAssetUpdateSnapshot before the call) is used to detect
// the status transition.
func (s *AssetService) UpdateAsset(actor AuditActor, assetID int, oldSnap repository.AssetUpdateSnapshot, in repository.UpdateAssetInput, customFieldValues map[string]any) (*models.Asset, error) {
	return s.UpdateAssetWithContext(actor, assetID, oldSnap, in, customFieldValues, AssetAutomationContext{})
}

// UpdateAssetWithContext runs the canonical update pipeline while preserving
// automation cascade metadata on emitted update and status-change events.
func (s *AssetService) UpdateAssetWithContext(actor AuditActor, assetID int, oldSnap repository.AssetUpdateSnapshot, in repository.UpdateAssetInput, customFieldValues map[string]any, context AssetAutomationContext) (*models.Asset, error) {
	if err := s.validateAssetTaxonomy(oldSnap.SetID, in.AssetTypeID, in.CategoryID, in.StatusID); err != nil {
		return nil, err
	}
	// Type change: any persisted custom field that's incompatible with
	// the new type would slip through if we only validated the supplied
	// values map (which the caller may have omitted on a partial-update
	// PUT). Force a re-validation pass — caller-supplied values win,
	// otherwise we run the persisted values through the new type's
	// schema so stale or required-but-missing fields surface as 400.
	typeChanged := oldSnap.AssetTypeID != 0 && oldSnap.AssetTypeID != in.AssetTypeID
	toValidate := customFieldValues
	enforceRequired := customFieldValues != nil
	if typeChanged {
		if toValidate == nil {
			stored := in.CustomFieldValuesJSON
			if stored == nil && oldSnap.CustomFieldValuesJSON.Valid {
				stored = &oldSnap.CustomFieldValuesJSON.String
			}
			toValidate = loadStoredCustomFieldValues(stored)
		}
		if err := s.retainCustomFieldsForType(in.AssetTypeID, toValidate); err != nil {
			return nil, err
		}
		customFieldValues = toValidate
		enforceRequired = true
	}
	if err := s.ValidateCustomFieldsSchema(in.AssetTypeID, toValidate, CustomFieldsValidationOpts{EnforceRequired: enforceRequired}); err != nil {
		return nil, err
	}
	if err := reencodeCustomFieldValues(customFieldValues, &in.CustomFieldValuesJSON); err != nil {
		return nil, err
	}
	sanitizeAssetText(&in.Title, &in.Description, &in.AssetTag)
	if strings.TrimSpace(in.Title) == "" {
		return nil, &AssetValidationError{Msg: "title is required"}
	}
	oldSID := 0
	var oldStatusID *int
	if oldSnap.StatusID.Valid {
		oldSID = int(oldSnap.StatusID.Int64)
		oldStatusID = &oldSID
	}
	newStatusID := in.StatusID
	snapshot := assetEventSnapshot(assetID, oldSnap.SetID, in.AssetTypeID, in.CategoryID, in.StatusID, in.Title, in.Description, in.AssetTag)
	oldValues := map[string]any{"status_id": oldStatusID}
	newValues := map[string]any{"title": in.Title, "asset_type_id": in.AssetTypeID, "status_id": newStatusID}
	err := database.WithTx(s.db, func(tx database.Tx) error {
		version, err := s.repo.LockAssetVersion(tx, assetID)
		if err != nil {
			return err
		}
		if !version.Equal(oldSnap.UpdatedAt) {
			return ErrAssetVersionConflict
		}
		if err := s.repo.UpdateAssetInTx(tx, assetID, in); err != nil {
			return err
		}
		recorder := s.events()
		metadata := assetEventMetadata(actor.UserID, "application", context)
		if !sameOptionalAssetID(oldStatusID, newStatusID) {
			if _, err := recorder.StatusChanged(stdctx.Background(), tx, snapshot, oldStatusID, newStatusID, oldValues, newValues, metadata); err != nil {
				return err
			}
			if err := s.compatibilityEvents().EmitInTx(stdctx.Background(), tx, assetCompatibilityEvent(
				models.AssetTriggerAssetStatusChanged, snapshot, actor.UserID, oldValues, newValues, context,
			)); err != nil {
				return err
			}
		}
		if _, err := recorder.Updated(stdctx.Background(), tx, snapshot, nil, newValues, metadata); err != nil {
			return err
		}
		return s.compatibilityEvents().EmitInTx(stdctx.Background(), tx, assetCompatibilityEvent(
			models.AssetTriggerAssetUpdated, snapshot, actor.UserID, nil, newValues, context,
		))
	})
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("update asset: %w", err)
	}
	s.emitAudit(actor, logger.ActionAssetUpdate, &assetID, in.Title, nil)
	row, err := s.repo.FindAssetFullByID(assetID)
	if err != nil {
		return nil, fmt.Errorf("reload after update: %w", err)
	}
	m := repository.AssetRowToModel(*row)
	return &m, nil
}

// MutateAsset applies an automation patch to a freshly loaded asset and then
// routes the complete result through UpdateAssetWithContext. This keeps
// set_field and set_status aligned with interactive validation, auditing, and
// event behavior.
func (s *AssetService) MutateAsset(actor AuditActor, assetID int, patch AssetMutationPatch, context AssetAutomationContext) (*models.Asset, error) {
	row, err := s.repo.FindAssetFullByID(assetID)
	if err != nil {
		return nil, err
	}
	current := repository.AssetRowToModel(*row)
	title := current.Title
	description := current.Description
	assetTag := current.AssetTag
	statusID := current.StatusID
	customFields := current.CustomFieldValues
	if customFields == nil {
		customFields = make(map[string]any)
	}
	if patch.Title != nil {
		title = *patch.Title
	}
	if patch.Description != nil {
		description = *patch.Description
	}
	if patch.AssetTag != nil {
		assetTag = *patch.AssetTag
	}
	if patch.StatusID != nil {
		statusID = patch.StatusID
	}
	if patch.CustomFieldValues != nil {
		customFields = patch.CustomFieldValues
	}
	customFields, err = s.CoerceAndValidateCustomFieldValues(current.AssetTypeID, customFields)
	if err != nil {
		return nil, err
	}
	return s.UpdateAssetWithContext(
		actor,
		assetID,
		row.UpdateSnapshot(),
		repository.UpdateAssetInput{
			AssetTypeID: current.AssetTypeID,
			CategoryID:  current.CategoryID,
			StatusID:    statusID,
			Title:       title,
			Description: description,
			AssetTag:    assetTag,
		},
		customFields,
		context,
	)
}

func (s *AssetService) validateAssetTaxonomy(setID, assetTypeID int, categoryID, statusID *int) error {
	if assetTypeID <= 0 {
		return &AssetValidationError{Msg: "asset_type_id is required"}
	}
	belongs, err := s.repo.AssetTypeBelongsToSet(assetTypeID, setID)
	if err != nil {
		return fmt.Errorf("validate asset type: %w", err)
	}
	if !belongs {
		return &AssetValidationError{Msg: fmt.Sprintf("asset_type_id %d does not belong to asset set %d", assetTypeID, setID)}
	}
	if categoryID != nil {
		belongs, err = s.repo.CategoryBelongsToSet(*categoryID, setID)
		if err != nil {
			return fmt.Errorf("validate asset category: %w", err)
		}
		if !belongs {
			return &AssetValidationError{Msg: fmt.Sprintf("category_id %d does not belong to asset set %d", *categoryID, setID)}
		}
	}
	if statusID != nil {
		belongs, err = s.repo.StatusBelongsToSet(*statusID, setID)
		if err != nil {
			return fmt.Errorf("validate asset status: %w", err)
		}
		if !belongs {
			return &AssetValidationError{Msg: fmt.Sprintf("status_id %d does not belong to asset set %d", *statusID, setID)}
		}
	}
	return nil
}

// retainCustomFieldsForType removes values that are not declared on the target
// type. It is used only during an explicit type change: compatible values are
// retained, incompatible values are pruned, and the subsequent required-field
// validation tells the caller which new values must be supplied.
func (s *AssetService) retainCustomFieldsForType(assetTypeID int, values map[string]any) error {
	if len(values) == 0 {
		return nil
	}
	fields, err := s.repo.FindAssetTypeFields(assetTypeID)
	if err != nil {
		return fmt.Errorf("load asset type fields: %w", err)
	}
	allowed := make(map[string]struct{}, len(fields)*3)
	for _, field := range fields {
		allowed[fmt.Sprintf("%d", field.CustomFieldID)] = struct{}{}
		allowed[field.FieldName] = struct{}{}
		allowed[strings.ToLower(field.FieldName)] = struct{}{}
	}
	for key := range values {
		if _, ok := allowed[key]; ok {
			continue
		}
		if _, ok := allowed[strings.ToLower(key)]; !ok {
			delete(values, key)
		}
	}
	return nil
}

// DeleteAsset resolves the title via GetAssetSetAndTitle (so the audit
// row carries human-readable context post-delete), removes the asset +
// its item_links rows, and emits the audit event + an
// asset_deleted automation event.
func (s *AssetService) DeleteAsset(actor AuditActor, assetID int) error {
	row, err := s.repo.FindAssetFullByID(assetID)
	if errors.Is(err, repository.ErrNotFound) {
		return err
	}
	if err != nil {
		return fmt.Errorf("load asset for delete: %w", err)
	}
	asset := repository.AssetRowToModel(*row)
	snapshot := assetEventSnapshot(asset.ID, asset.SetID, asset.AssetTypeID, asset.CategoryID, asset.StatusID, asset.Title, asset.Description, asset.AssetTag)
	metadata := assetEventMetadata(actor.UserID, "application", AssetAutomationContext{})
	err = database.WithTx(s.db, func(tx database.Tx) error {
		if err := s.repo.DeleteAssetWithLinksInTx(stdctx.Background(), tx, assetID, itemEventMetadata(actor.UserID, "application", nil)); err != nil {
			return err
		}
		oldValues := map[string]any{
			"title": asset.Title, "asset_type_id": asset.AssetTypeID, "status_id": asset.StatusID,
		}
		if _, err := s.events().Deleted(stdctx.Background(), tx, snapshot, oldValues, metadata); err != nil {
			return err
		}
		return s.compatibilityEvents().EmitInTx(stdctx.Background(), tx, assetCompatibilityEvent(
			models.AssetTriggerAssetDeleted, snapshot, actor.UserID, oldValues, nil, AssetAutomationContext{},
		))
	})
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return err
		}
		return fmt.Errorf("delete asset: %w", err)
	}
	s.emitAudit(actor, logger.ActionAssetDelete, &assetID, asset.Title, nil)
	return nil
}

// ImportCSVDefaults carries the optional column defaults that apply to
// every row of a sync CSV import.
type ImportCSVDefaults struct {
	StatusID   *int
	CategoryID *int
}

// ImportCSVSummary is the aggregate result of a sync CSV import.
type ImportCSVSummary struct {
	SetID         int
	AssetTypeID   int
	TotalRows     int
	ProcessedRows int
	CreatedRows   int
	ErrorRows     int
	Status        string
	ErrorMessage  string
	StartedAt     time.Time
	CompletedAt   time.Time
}

// ImportAssetsCSV parses csvBody as a CSV with a header row, then creates
// one asset per data row. Header columns "title" / "description" /
// "asset_tag"|"tag" map to built-in fields; every other header is
// matched case-insensitively against the asset type's declared custom
// field names. Rows missing a non-empty title are counted as errors but
// don't abort the import.
//
// Emits one aggregate audit row at the end (mirroring the cookie-auth
// async-import pattern, which audits the job, not each inserted row).
// Per-row audit would balloon the trail without changing what an
// investigator can reconstruct.
func (s *AssetService) ImportAssetsCSV(actor AuditActor, setID, assetTypeID int, defaults ImportCSVDefaults, csvBody io.Reader, filename string) (*ImportCSVSummary, error) {
	fields, err := s.repo.FindAssetTypeFields(assetTypeID)
	if err != nil {
		return nil, fmt.Errorf("load asset type fields: %w", err)
	}
	fieldByName := make(map[string]string, len(fields))
	for _, f := range fields {
		fieldByName[strings.ToLower(f.FieldName)] = f.FieldName
	}

	reader := csv.NewReader(csvBody)
	reader.FieldsPerRecord = -1
	headers, err := reader.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, &AssetValidationError{Msg: "CSV is empty"}
		}
		return nil, &AssetValidationError{Msg: fmt.Sprintf("CSV parse error: %v", err)}
	}

	summary := &ImportCSVSummary{
		SetID:       setID,
		AssetTypeID: assetTypeID,
		Status:      "running",
		StartedAt:   time.Now().UTC(),
	}

	rowStatusID := defaults.StatusID
	if rowStatusID == nil {
		if defaultStatusID, err := s.repo.GetDefaultStatus(setID); err != nil {
			slog.Warn("failed to load default asset status for CSV import", slog.String("component", "assets"), slog.Int("set_id", setID), slog.Any("error", err))
		} else {
			rowStatusID = defaultStatusID
		}
	}

	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			summary.TotalRows++
			summary.ProcessedRows++
			summary.ErrorRows++
			continue
		}
		summary.TotalRows++
		summary.ProcessedRows++

		row := buildCSVRow(headers, record, fieldByName)
		title := strings.TrimSpace(row.title)
		if title == "" {
			summary.ErrorRows++
			continue
		}

		coerced := coerceCustomFieldValues(fields, row.customFields)
		if err := validateCustomFieldsSchemaCore(fields, coerced, CustomFieldsValidationOpts{EnforceRequired: true}); err != nil {
			summary.ErrorRows++
			continue
		}
		cfJSON, _ := encodeCustomFieldValuesJSON(coerced)
		description := row.description
		assetTag := row.assetTag
		sanitizeAssetText(&title, &description, &assetTag)
		_, err = s.InsertImportedAsset(repository.ImportAssetRowInput{
			SetID:                 setID,
			AssetTypeID:           assetTypeID,
			CategoryID:            defaults.CategoryID,
			StatusID:              rowStatusID,
			Title:                 title,
			Description:           description,
			AssetTag:              assetTag,
			CustomFieldValuesJSON: cfJSON,
			CreatedBy:             actor.UserID,
			CreatedAt:             time.Now().UTC(),
		})
		if err != nil {
			summary.ErrorRows++
			continue
		}
		summary.CreatedRows++
	}
	summary.CompletedAt = time.Now().UTC()
	switch {
	case summary.TotalRows == 0:
		summary.Status = "empty"
		summary.ErrorMessage = "no data rows in CSV"
	case summary.ErrorRows == 0:
		summary.Status = "succeeded"
	case summary.CreatedRows == 0:
		summary.Status = "failed"
	default:
		summary.Status = "partial"
	}

	s.emitAudit(actor, logger.ActionAssetCreate, nil, "csv_import:"+filename, map[string]any{
		"source":        "csv_import_sync",
		"set_id":        setID,
		"asset_type_id": assetTypeID,
		"total":         summary.TotalRows,
		"created":       summary.CreatedRows,
		"errors":        summary.ErrorRows,
		"status":        summary.Status,
	})
	return summary, nil
}

// emitAudit best-effort records successful mutations. Details include token
// attribution so each token's footprint remains queryable.
func (s *AssetService) emitAudit(actor AuditActor, action string, resourceID *int, resourceName string, extra map[string]any) {
	details := mergeAuditDetails(extra, actor)
	_ = logger.LogAudit(s.db, logger.AuditEvent{
		UserID:       actor.UserID,
		Username:     actor.Username,
		IPAddress:    actor.IPAddress,
		UserAgent:    actor.UserAgent,
		ActionType:   action,
		ResourceType: logger.ResourceAsset,
		ResourceID:   resourceID,
		ResourceName: resourceName,
		Details:      details,
		Success:      true,
	})
}

// mergeAuditDetails composes the caller's extra map with the auth/token
// attribution stamped onto every row. Caller-supplied keys win on a
// collision so route-specific context (e.g. csv_import totals) isn't
// clobbered by the actor stamp.
func mergeAuditDetails(extra map[string]any, actor AuditActor) map[string]any {
	if actor.AuthMethod == "" && actor.Source == "" && actor.APITokenID == 0 && len(extra) == 0 {
		return nil
	}
	merged := make(map[string]any, len(extra)+4)
	if actor.AuthMethod != "" {
		merged["auth_method"] = actor.AuthMethod
	}
	if actor.Source != "" {
		merged["source"] = actor.Source
	}
	if actor.APITokenID != 0 {
		merged["api_token_id"] = actor.APITokenID
	}
	if actor.APITokenPrefix != "" {
		merged["api_token_prefix"] = actor.APITokenPrefix
	}
	if actor.APITokenName != "" {
		merged["api_token_name"] = actor.APITokenName
	}
	for k, v := range extra {
		merged[k] = v
	}
	return merged
}

// csvRow holds the field values for a single CSV row, split by where
// they route on the asset model.
type csvRow struct {
	title        string
	description  string
	assetTag     string
	customFields map[string]any
}

// buildCSVRow walks the CSV record against its header row and routes
// each cell to either a built-in column or a custom field on the type,
// matched case-insensitively by header name.
func buildCSVRow(headers, record []string, customFieldByName map[string]string) csvRow {
	row := csvRow{customFields: map[string]any{}}
	for i, h := range headers {
		if i >= len(record) {
			break
		}
		key := strings.ToLower(strings.TrimSpace(h))
		val := strings.TrimSpace(record[i])
		switch key {
		case "title":
			row.title = val
		case "description":
			row.description = val
		case "asset_tag", "tag":
			row.assetTag = val
		default:
			if canonical, ok := customFieldByName[key]; ok && val != "" {
				row.customFields[canonical] = val
			}
		}
	}
	return row
}

// loadStoredCustomFieldValues unmarshals a persisted or request-supplied CFV
// column. Returns nil for nil / empty / unparseable JSON — callers fall back
// to "empty map" semantics, which the validator then runs against the new
// type's required-field set.
func loadStoredCustomFieldValues(stored *string) map[string]any {
	if stored == nil || *stored == "" {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(*stored), &m); err != nil {
		return nil
	}
	return m
}

// encodeCustomFieldValuesJSON marshals the values map for storage.
// Returns nil for nil / empty maps so the column stores NULL rather
// than "null" or "{}".
func encodeCustomFieldValuesJSON(m map[string]any) (*string, error) {
	if len(m) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	s := string(b)
	return &s, nil
}
