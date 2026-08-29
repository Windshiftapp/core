package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"windshift/internal/itemevents"
	"windshift/internal/jira"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/services"
	"windshift/internal/validation"

	"uuid"
)

// ensureUsers matches or creates users for import.
// Returns:
//   - userMap: Jira accountID → Windshift user ID
//   - usernameMap: Jira accountID → Windshift username (for ADF mention resolution)
//
// Fetches missing emails via the Jira API when needed.
func (h *JiraImportHandler) ensureUsers(ctx context.Context, jobID string, users []JiraUserSummary, client jira.Client) (map[string]int, map[string]string, error) { //nolint:unparam,gocritic // error return kept for API consistency; named returns aren't worth the noise here
	result := make(map[string]int)
	usernames := make(map[string]string)

	for i := range users {
		if users[i].AccountID == "" {
			continue
		}
		if users[i].Email != "" {
			continue // Already have email
		}

		email, err := client.GetUserEmail(ctx, users[i].AccountID)
		if err != nil {
			slog.Debug("Failed to fetch email for user", slog.String("component", "jira"),
				slog.String("accountID", users[i].AccountID), slog.Any("error", err))
		} else if email != "" {
			users[i].Email = email
			slog.Debug("Fetched email for user", slog.String("component", "jira"),
				slog.String("accountID", users[i].AccountID), slog.String("email", email))
		}
	}

	for _, u := range users {
		if u.AccountID == "" {
			continue
		}

		// Synthesize an email for accounts where Jira Cloud's GDPR rules hide
		// the real one. Without this every emailless account would be skipped
		// and downstream fields (reporter, comment author, mentions, custom
		// user-pickers) would silently lose their user reference. The synthetic
		// address is deterministic per accountID so re-imports map to the same
		// inactive user instead of creating a new ghost each run.
		if u.Email == "" {
			u.Email = syntheticEmailForAccount(u.AccountID)
		}

		// Check if we already have a mapping for this user in this job.
		if existing, ok := h.imports.UserMapping(jobID, u.AccountID); ok {
			result[u.AccountID] = existing.UserID
			usernames[u.AccountID] = existing.Username
			continue
		}

		// Try to find existing Windshift user by email
		if u.Email != "" {
			existing, err := h.imports.FindUserByEmail(u.Email)
			if err == nil {
				// Found existing user
				result[u.AccountID] = existing.UserID
				usernames[u.AccountID] = existing.Username
				h.recordUserMapping(jobID, u, existing.UserID, false)
				continue
			}
			if !errors.Is(err, repository.ErrNotFound) {
				slog.Error("Failed to find existing user by email", slog.String("component", "jira"), slog.String("displayName", u.DisplayName), slog.Any("error", err))
				continue
			}
		}

		firstName, lastName := parseDisplayName(u.DisplayName)
		username, err := h.imports.UniqueImportedUsername(generateUsername(u.Email, u.DisplayName))
		if err != nil {
			slog.Error("Failed to allocate username for imported user", slog.String("component", "jira"), slog.String("displayName", u.DisplayName), slog.Any("error", err))
			continue
		}

		newUserID, err := h.imports.CreateImportedUser(
			u.Email,
			username,
			firstName,
			lastName,
			u.AvatarURL,
		)
		if err != nil {
			slog.Error("Failed to create user", slog.String("component", "jira"), slog.String("displayName", u.DisplayName), slog.String("email", u.Email), slog.Any("error", err))
			continue
		}

		result[u.AccountID] = newUserID
		usernames[u.AccountID] = username
		h.recordUserMapping(jobID, u, newUserID, true)

		slog.Debug("Created user", slog.String("component", "jira"), slog.String("displayName", u.DisplayName), slog.String("email", u.Email), slog.Int("userID", newUserID))
	}

	return result, usernames, nil
}

// recordUserMapping stores a Jira user to Windshift user mapping
func (h *JiraImportHandler) recordUserMapping(jobID string, user JiraUserSummary, windshiftUserID int, wasCreated bool) {
	err := h.imports.RecordUserMapping(jobID, user.AccountID, user.Email, user.DisplayName, windshiftUserID, wasCreated)
	if err != nil {
		slog.Error("Failed to record user mapping", slog.String("component", "jira"), slog.Any("error", err))
	}
}

// ensureLabel returns the global label ID for name, creating it when needed.
func (h *JiraImportHandler) ensureLabel(_ int, name string) (int, error) {
	return h.imports.EnsureLabel(name)
}

// importLabels ensures each Jira label exists and links it to the imported
// item. Duplicate input names are collapsed.
func (h *JiraImportHandler) importLabels(workspaceID, itemID int, labels []string) {
	if len(labels) == 0 {
		return
	}
	seen := make(map[string]struct{}, len(labels))
	for _, raw := range labels {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}

		labelID, err := h.ensureLabel(workspaceID, name)
		if err != nil {
			slog.Error("Failed to ensure label",
				slog.String("component", "jira"),
				slog.String("label", name),
				slog.Int("workspaceID", workspaceID),
				slog.Any("error", err))
			continue
		}
		if err := h.imports.AddItemLabel(itemID, labelID); err != nil {
			slog.Error("Failed to link label to item",
				slog.String("component", "jira"),
				slog.String("label", name),
				slog.Int("itemID", itemID),
				slog.Int("labelID", labelID),
				slog.Any("error", err))
		}
	}
}

// importedDummyUserEmail is the well-known address used for the shared
// fallback user that owns comments whose Jira author can't be resolved (e.g.
// deleted Jira accounts, comments on Service Desk requests from removed
// portal customers). One row across all imports — re-imports don't pile up
// dummy rows.
const importedDummyUserEmail = "imported-user@jira-import.invalid"

// ensureImportedDummyUser returns the ID of the shared fallback user, creating
// it on first use. The user is inactive and password-locked so the row never
// becomes a real account. Concurrent imports that race on creation are handled
// by re-SELECTing after a UNIQUE-violating INSERT.
func (h *JiraImportHandler) ensureImportedDummyUser() (int, error) {
	return h.imports.EnsureImportedDummyUser(importedDummyUserEmail)
}

// syntheticEmailForAccount produces a deterministic, RFC-safe email for a Jira
// account whose real email is hidden (GDPR-restricted). Colons that appear in
// Cloud accountIDs aren't legal in the local-part of an address, so we map
// them to hyphens. The `.invalid` TLD is reserved by RFC 2606, guaranteeing
// no collision with real domains.
func syntheticEmailForAccount(accountID string) string {
	safe := strings.ReplaceAll(accountID, ":", "-")
	return safe + "@imported.invalid"
}

// parseDisplayName splits a display name into first and last name
func parseDisplayName(displayName string) (firstName, lastName string) {
	parts := strings.SplitN(strings.TrimSpace(displayName), " ", 2)
	if len(parts) >= 1 {
		firstName = parts[0]
	}
	if len(parts) >= 2 {
		lastName = parts[1]
	}
	if firstName == "" {
		firstName = "Imported"
	}
	if lastName == "" {
		lastName = "User"
	}
	return
}

// generateUsername creates a unique username from email or display name
func generateUsername(email, displayName string) string {
	if email != "" {
		parts := strings.Split(email, "@")
		if len(parts) > 0 && parts[0] != "" {
			return strings.ToLower(parts[0])
		}
	}
	if displayName != "" {
		return strings.ToLower(strings.ReplaceAll(displayName, " ", "."))
	}
	return fmt.Sprintf("user_%d", time.Now().UnixNano())
}

// uniqueImportUsername returns an available, case-insensitively unique username.
// Jira accounts commonly share the same email local-part across domains, so the
// plain generated name is not guaranteed to satisfy users.username's uniqueness
// constraint.
// collectUsersFromCustomField extracts users from a custom field value
func collectUsersFromCustomField(value any, fieldType string,
	existingMap map[string]int, usersToProcess *[]JiraUserSummary, seen map[string]bool) {

	switch fieldType {
	case "user":
		if userObj, ok := value.(map[string]any); ok {
			addUserFromObject(userObj, existingMap, usersToProcess, seen)
		}
	case "multi_user":
		if users, ok := value.([]any); ok {
			for _, u := range users {
				if userObj, ok := u.(map[string]any); ok {
					addUserFromObject(userObj, existingMap, usersToProcess, seen)
				}
			}
		}
	}
}

// collectUsersFromADF walks Jira ADF-like structures and queues mention-only
// users. Jira comments/descriptions can mention accounts that do not appear in
// assignee/reporter/comment-author fields; pre-collecting them lets the ADF
// converter resolve to Windshift @username syntax instead of falling back to
// inert display text.
func collectUsersFromADF(value any, existingMap map[string]int, usersToProcess *[]JiraUserSummary, seen map[string]bool) {
	switch v := value.(type) {
	case map[string]any:
		if nodeType, _ := v["type"].(string); nodeType == "mention" {
			if attrs, ok := v["attrs"].(map[string]any); ok {
				accountID, _ := attrs["id"].(string)
				if accountID == "" {
					accountID, _ = attrs["accountId"].(string)
				}
				if accountID != "" {
					displayName, _ := attrs["text"].(string)
					displayName = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(displayName), "@"))
					addMentionUserSummary(accountID, displayName, existingMap, usersToProcess, seen)
				}
			}
		}
		for _, child := range v {
			collectUsersFromADF(child, existingMap, usersToProcess, seen)
		}
	case []any:
		for _, child := range v {
			collectUsersFromADF(child, existingMap, usersToProcess, seen)
		}
	}
}

func addMentionUserSummary(accountID, displayName string, existingMap map[string]int, usersToProcess *[]JiraUserSummary, seen map[string]bool) {
	if accountID == "" {
		return
	}
	if _, exists := existingMap[accountID]; exists || seen[accountID] {
		return
	}
	*usersToProcess = append(*usersToProcess, JiraUserSummary{
		AccountID:   accountID,
		DisplayName: displayName,
	})
	seen[accountID] = true
}

// addJiraUserSummaryFromUser adds a typed Jira user reference from standard
// issue fields/comments/attachments/worklogs to the import user queue.
func addJiraUserSummaryFromUser(user *jira.JiraUser, existingMap map[string]int, usersToProcess *[]JiraUserSummary, seen map[string]bool) {
	if user == nil || user.GetIdentifier() == "" {
		return
	}
	accountID := user.GetIdentifier()
	if _, exists := existingMap[accountID]; exists || seen[accountID] {
		return
	}
	avatarURL := ""
	if user.AvatarURLs != nil {
		avatarURL = user.AvatarURLs["48x48"]
	}
	*usersToProcess = append(*usersToProcess, JiraUserSummary{
		AccountID:   accountID,
		AccountType: user.AccountType,
		Email:       user.EmailAddress,
		DisplayName: user.DisplayName,
		AvatarURL:   avatarURL,
	})
	seen[accountID] = true
}

// addUserFromObject extracts user data from a Jira user object and adds it to the processing list
func addUserFromObject(userObj map[string]any, existingMap map[string]int,
	usersToProcess *[]JiraUserSummary, seen map[string]bool) {

	accountID, _ := userObj["accountId"].(string)
	if accountID == "" {
		accountID, _ = userObj["name"].(string)
	}
	if accountID == "" {
		accountID, _ = userObj["key"].(string)
	}
	if accountID == "" {
		return
	}
	if _, exists := existingMap[accountID]; exists {
		return
	}
	if seen[accountID] {
		return
	}

	email, _ := userObj["emailAddress"].(string)
	accountType, _ := userObj["accountType"].(string)
	displayName, _ := userObj["displayName"].(string)
	avatarURL := ""
	if avatars, ok := userObj["avatarUrls"].(map[string]any); ok {
		avatarURL, _ = avatars["48x48"].(string)
	}

	*usersToProcess = append(*usersToProcess, JiraUserSummary{
		AccountID:   accountID,
		AccountType: accountType,
		Email:       email,
		DisplayName: displayName,
		AvatarURL:   avatarURL,
	})
	seen[accountID] = true
}

// isJiraStoryPointsField identifies Jira Software's story-points fields, which
// Windshift stores in items.story_points rather than as a generic custom field.
func isJiraStoryPointsField(mapping CustomFieldMapping) bool {
	return mapping.JiraType == "com.pyxis.greenhopper.jira:jsw-story-points" || strings.EqualFold(strings.TrimSpace(mapping.JiraName), "Story Points")
}

func shouldImportJiraStoryPoints(mapping CustomFieldMapping) bool {
	return mapping.Action != "skip" && isJiraStoryPointsField(mapping)
}

// isJiraSprintField identifies Jira Software's sprint field. Windshift has a
// first-class iteration_id on items, so sprint data should resolve there rather
// than into a generic custom field bag.
func isJiraSprintField(mapping CustomFieldMapping) bool {
	return mapping.JiraType == "com.pyxis.greenhopper.jira:gh-sprint" || strings.EqualFold(strings.TrimSpace(mapping.JiraName), "Sprint")
}

// extractCustomFieldValue resolves a single Jira custom field into the value
// that belongs in the item's custom_field_values JSON bag. Returns (nil, false)
// for skip/unmapped paths.
func (h *JiraImportHandler) customFieldType(fieldID int) string {
	return h.imports.CustomFieldType(fieldID)
}

func (h *JiraImportHandler) customFieldAssetAllowsMultiple(fieldID int) bool {
	options := h.imports.CustomFieldOptions(fieldID)
	if options == "" {
		return false
	}
	var config struct {
		Multi bool `json:"multi"`
	}
	return json.Unmarshal([]byte(options), &config) == nil && config.Multi
}

func (h *JiraImportHandler) customFieldAssetSetID(fieldID int) int {
	options := h.imports.CustomFieldOptions(fieldID)
	if options == "" {
		return 0
	}
	var config struct {
		AssetSetID int `json:"asset_set_id"`
	}
	if json.Unmarshal([]byte(options), &config) != nil {
		return 0
	}
	return config.AssetSetID
}

func assetCustomFieldValue(ref jiraIssueAssetReference) map[string]any {
	value := map[string]any{"id": ref.AssetID}
	if ref.Title != "" {
		value["title"] = ref.Title
	}
	if ref.AssetTag != "" {
		value["asset_tag"] = ref.AssetTag
	}
	return value
}

func jiraIssueAssetReferencesForSet(refs []jiraIssueAssetReference, setID int) []jiraIssueAssetReference {
	if setID <= 0 {
		return refs
	}
	matching := make([]jiraIssueAssetReference, 0, len(refs))
	for _, ref := range refs {
		if ref.SetID == setID {
			matching = append(matching, ref)
		}
	}
	return matching
}

func extractCustomFieldValueWithOptions(
	mapping CustomFieldMapping,
	fields *jira.JiraIssueFields,
	userMap, versionMap map[string]int,
	choiceOptionIDs map[string]int,
) (any, bool) {
	if mapping.Action == "skip" || isJiraStoryPointsField(mapping) || isJiraSprintField(mapping) {
		return nil, false
	}
	if fields == nil || fields.CustomFields == nil {
		return nil, false
	}
	value, exists := fields.CustomFields[mapping.JiraID]
	if !exists || value == nil {
		return nil, false
	}
	if mapping.PreserveRaw {
		data, err := json.Marshal(value)
		if err != nil {
			return nil, false
		}
		return string(data), true
	}

	switch mapping.WindshiftType {
	case "user":
		userObj, ok := value.(map[string]any)
		if !ok {
			return nil, false
		}
		accountID := userIdentifierFromObject(userObj)
		if accountID == "" {
			return nil, false
		}
		uid, ok := userMap[accountID]
		if !ok {
			return nil, false
		}
		return uid, true
	case "multi_user":
		users, ok := value.([]any)
		if !ok {
			return nil, false
		}
		var userIDs []int
		for _, u := range users {
			userObj, ok := u.(map[string]any)
			if !ok {
				continue
			}
			accountID := userIdentifierFromObject(userObj)
			if accountID == "" {
				continue
			}
			if uid, ok := userMap[accountID]; ok {
				userIDs = append(userIDs, uid)
			}
		}
		if len(userIDs) == 0 {
			return nil, false
		}
		return userIDs, true
	case "number":
		if n, ok := numericCustomFieldValue(value); ok {
			return n, true
		}
	case "date":
		if s := customFieldDisplayValue(value); s != "" {
			if _, err := time.Parse(time.DateOnly, s); err == nil {
				return s, true
			}
			if t := jira.ParseJiraTimestamp(s); t != nil {
				return t.Format(time.DateOnly), true
			}
		}
	case "select":
		if s := customFieldDisplayValue(value); s != "" {
			optionID := choiceOptionIDs[strings.ToLower(strings.TrimSpace(s))]
			if optionID > 0 {
				return optionID, true
			}
		}
	case "multiselect":
		values := customFieldDisplayValues(value)
		if len(values) > 0 {
			optionIDs := make([]int, 0, len(values))
			seen := make(map[int]bool)
			for _, label := range values {
				optionID := choiceOptionIDs[strings.ToLower(strings.TrimSpace(label))]
				if optionID <= 0 || seen[optionID] {
					continue
				}
				seen[optionID] = true
				optionIDs = append(optionIDs, optionID)
			}
			if len(optionIDs) > 0 {
				return optionIDs, true
			}
		}
	case "milestone":
		if id := customFieldIDValue(value); id != "" {
			if mid, ok := versionMap[id]; ok {
				return mid, true
			}
		}
	case "asset":
		values := customFieldDisplayValues(value)
		if len(values) > 0 {
			return strings.Join(values, "\n"), true
		}
	case "text", "textarea":
		if s := customFieldDisplayValue(value); s != "" {
			return s, true
		}
	case models.CustomFieldTypeBoolean, models.CustomFieldTypeCheckbox:
		if b, ok := jiraCheckboxValue(value); ok {
			return b, true
		}
	}
	return nil, false
}

func unresolvedJiraCustomFieldRawValue(
	mapping CustomFieldMapping,
	fields *jira.JiraIssueFields,
	typedValueResolved bool,
	choiceOptionIDs map[string]int,
) (any, bool) {
	if typedValueResolved && mapping.WindshiftType != "multiselect" {
		return nil, false
	}
	if mapping.Action == "skip" || mapping.PreserveRaw || isJiraStoryPointsField(mapping) || isJiraSprintField(mapping) {
		return nil, false
	}
	switch mapping.WindshiftType {
	case "date", "select", "multiselect", "milestone":
	default:
		return nil, false
	}
	if fields == nil || fields.CustomFields == nil {
		return nil, false
	}
	raw, exists := fields.CustomFields[mapping.JiraID]
	if !exists || raw == nil {
		return nil, false
	}
	if !typedValueResolved {
		return raw, true
	}

	for _, label := range customFieldDisplayValues(raw) {
		if choiceOptionIDs[strings.ToLower(strings.TrimSpace(label))] <= 0 {
			return raw, true
		}
	}
	return nil, false
}

func preserveUnresolvedJiraCustomField(
	values map[string]any,
	mapping CustomFieldMapping,
	raw any,
) {
	values["_jira_custom_field_"+mapping.JiraID] = map[string]any{
		"jira_field_id":   mapping.JiraID,
		"jira_field_name": mapping.JiraName,
		"jira_field_type": mapping.JiraType,
		"value":           raw,
	}
}

func userIdentifierFromObject(userObj map[string]any) string {
	for _, key := range []string{"accountId", "name", "key"} {
		if v, _ := userObj[key].(string); v != "" {
			return v
		}
	}
	return ""
}

func numericCustomFieldValue(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

// jiraCheckboxValue converts a Jira boolean representation to a canonical Go
// boolean. It accepts a raw JSON/Go bool, exact true/false strings from export
// payloads, and known wrapper shapes containing one of those values. Unknown
// or ambiguous shapes are not coerced — the caller reports them as
// unmapped/skipped import data rather than persisting a guessed value.
func jiraCheckboxValue(value any) (result, ok bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true":
			return true, true
		case "false":
			return false, true
		}
	case map[string]any:
		for _, k := range []string{"value", "checked", "enabled", "name"} {
			if b, ok := jiraCheckboxValue(v[k]); ok {
				return b, true
			}
		}
	case []any:
		if len(v) == 1 {
			return jiraCheckboxValue(v[0])
		}
	}
	return false, false
}

func customFieldIDValue(value any) string {
	if m, ok := value.(map[string]any); ok {
		for _, key := range []string{"id", "key", "accountId", "name"} {
			if s, _ := m[key].(string); s != "" {
				return s
			}
		}
	}
	return ""
}

func customFieldDisplayValue(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	case map[string]any:
		// Jira option/version/user-like objects usually carry a human label in one
		// of these fields. Prefer labels over IDs so imports preserve admin-facing
		// display values instead of opaque Jira identifiers.
		if parent := firstStringKey(v, "value", "name", "displayName", "label", "key"); parent != "" {
			if child, ok := v["child"].(map[string]any); ok {
				if childLabel := customFieldDisplayValue(child); childLabel != "" {
					return parent + " / " + childLabel
				}
			}
			return parent
		}
		if id, _ := v["id"].(string); id != "" {
			return id
		}
		b, err := json.Marshal(v)
		if err == nil {
			return string(b)
		}
	case []any:
		values := customFieldDisplayValues(v)
		if len(values) > 0 {
			return strings.Join(values, ", ")
		}
	}
	return ""
}

func customFieldDisplayValues(value any) []string {
	arr, ok := value.([]any)
	if !ok {
		if s := customFieldDisplayValue(value); s != "" {
			return []string{s}
		}
		return nil
	}
	values := make([]string, 0, len(arr))
	for _, entry := range arr {
		if s := customFieldDisplayValue(entry); s != "" {
			values = append(values, s)
		}
	}
	return values
}

// sanitizeJiraImportStrings recursively plain-text-sanitizes external display
// strings in importer map/slice shapes. It mutates maps, preserves scalars, and
// is idempotent for clean values.
func sanitizeJiraImportStrings(value any) any {
	switch v := value.(type) {
	case string:
		return sanitize.PlainTextField.Sanitize(v)
	case []string:
		for i := range v {
			v[i] = sanitize.PlainTextField.Sanitize(v[i])
		}
		return v
	case map[string]string:
		for k := range v {
			v[k] = sanitize.PlainTextField.Sanitize(v[k])
		}
		return v
	case map[string]any:
		for k := range v {
			v[k] = sanitizeJiraImportStrings(v[k])
		}
		return v
	case []any:
		for i := range v {
			v[i] = sanitizeJiraImportStrings(v[i])
		}
		return v
	case []map[string]string:
		for i := range v {
			sanitizeJiraImportStrings(v[i])
		}
		return v
	case []map[string]any:
		for i := range v {
			sanitizeJiraImportStrings(v[i])
		}
		return v
	default:
		return v
	}
}

func firstStringKey(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if s, _ := m[key].(string); strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

var legacySprintIDPattern = regexp.MustCompile(`\bid=(\d+)\b`)

func extractSprintIterationID(fields *jira.JiraIssueFields, mappings []CustomFieldMapping, iterationMap map[string]int) *int {
	if fields == nil || len(iterationMap) == 0 {
		return nil
	}
	ids := sprintIDsFromValue(fields.Sprint)
	for _, mapping := range mappings {
		if !isJiraSprintField(mapping) || mapping.Action == "skip" {
			continue
		}
		if fields.CustomFields != nil {
			ids = append(ids, sprintIDsFromValue(fields.CustomFields[mapping.JiraID])...)
		}
	}
	for i := len(ids) - 1; i >= 0; i-- {
		if iterationID, ok := iterationMap[ids[i]]; ok {
			return &iterationID
		}
	}
	return nil
}

func sprintIDsFromValue(value any) []string {
	switch v := value.(type) {
	case nil:
		return nil
	case map[string]any:
		if id := sprintIDFromMap(v); id != "" {
			return []string{id}
		}
	case []any:
		ids := make([]string, 0, len(v))
		for _, entry := range v {
			ids = append(ids, sprintIDsFromValue(entry)...)
		}
		return ids
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		if _, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return []string{strings.TrimSpace(v)}
		}
		matches := legacySprintIDPattern.FindAllStringSubmatch(v, -1)
		ids := make([]string, 0, len(matches))
		for _, match := range matches {
			if len(match) > 1 {
				ids = append(ids, match[1])
			}
		}
		return ids
	}
	return nil
}

func sprintIDFromMap(m map[string]any) string {
	switch raw := m["id"].(type) {
	case string:
		return strings.TrimSpace(raw)
	case float64:
		return strconv.FormatInt(int64(raw), 10)
	case int:
		return strconv.Itoa(raw)
	case int64:
		return strconv.FormatInt(raw, 10)
	}
	return ""
}

func jiraVersionMetadata(version jira.JiraVersion) map[string]any {
	return map[string]any{
		"id":           version.ID,
		"name":         version.Name,
		"description":  version.Description,
		"archived":     version.Archived,
		"released":     version.Released,
		"release_date": version.ReleaseDate,
		"start_date":   version.StartDate,
	}
}

func jiraPreservationLabels(issue *jira.JiraIssue) []string {
	if issue == nil {
		return nil
	}
	labels := make([]string, 0, len(issue.Fields.Components)+len(issue.Fields.Versions))
	for _, component := range issue.Fields.Components {
		if name := strings.TrimSpace(component.Name); name != "" {
			labels = append(labels, "component:"+name)
		}
	}
	for _, version := range issue.Fields.Versions {
		if name := strings.TrimSpace(version.Name); name != "" {
			labels = append(labels, "affects:"+name)
		}
	}
	return labels
}

func affectedVersionOptionValues(issue *jira.JiraIssue, field *jiraAffectsVersionCustomField) []int {
	if issue == nil || field == nil || len(issue.Fields.Versions) == 0 {
		return nil
	}
	values := make([]int, 0, len(issue.Fields.Versions))
	seen := make(map[int]struct{}, len(issue.Fields.Versions))
	for _, version := range issue.Fields.Versions {
		optionID, ok := field.OptionIDsByJiraID[version.ID]
		if !ok || optionID == 0 {
			continue
		}
		if _, exists := seen[optionID]; exists {
			continue
		}
		seen[optionID] = struct{}{}
		values = append(values, optionID)
	}
	return values
}

// importIssue imports a single Jira issue as a Windshift work item
func (h *JiraImportHandler) importIssue(ctx context.Context, jobID string, workspaceID int, issue *jira.JiraIssue, statusMap, itemTypeMap, userMap map[string]int, usernameMap map[string]string, portalCustomerMap, versionMap, iterationMap, customFieldIDMap map[string]int, choiceOptionIDs map[string]map[string]int, timeProjectID *int, affectsVersionField *jiraAffectsVersionCustomField, customFieldMappings []CustomFieldMapping, jsmImport *jiraServiceManagementImport, client jira.Client, progress *ImportProgress, reimport ...bool) error {
	mentionResolver := jira.MentionResolver(func(accountID string) string {
		return usernameMap[accountID]
	})
	var statusID *int
	if issue.Fields.Status != nil {
		if sid, ok := statusMap[issue.Fields.Status.ID]; ok {
			statusID = &sid
		}
	}

	var itemTypeID *int
	if issue.Fields.IssueType != nil {
		if tid, ok := itemTypeMap[issue.Fields.IssueType.ID]; ok {
			itemTypeID = &tid
		}
	}

	var assigneeID *int
	if issue.Fields.Assignee != nil && issue.Fields.Assignee.GetIdentifier() != "" {
		if uid, ok := userMap[issue.Fields.Assignee.GetIdentifier()]; ok {
			assigneeID = &uid
		}
	}

	var reporterID *int
	if issue.Fields.Reporter != nil && issue.Fields.Reporter.GetIdentifier() != "" {
		if uid, ok := userMap[issue.Fields.Reporter.GetIdentifier()]; ok {
			reporterID = &uid
		}
	}

	// Creator (immutable in Jira) is distinct from Reporter (mutable). Preserve
	// it on items.creator_id so audit views in Windshift reflect who originated
	// the issue, not who happened to run the import.
	var creatorID *int
	if issue.Fields.Creator != nil && issue.Fields.Creator.GetIdentifier() != "" {
		if uid, ok := userMap[issue.Fields.Creator.GetIdentifier()]; ok {
			creatorID = &uid
		}
	}

	var creatorPortalCustomerID *int
	for _, candidate := range []*jira.JiraUser{issue.Fields.Reporter, issue.Fields.Creator} {
		if candidate == nil || candidate.GetIdentifier() == "" {
			continue
		}
		if customerID, ok := portalCustomerMap[candidate.GetIdentifier()]; ok {
			creatorPortalCustomerID = &customerID
			break
		}
	}

	var channelID, requestTypeID *int
	jiraRequestType := jiraRequestTypeID(issue, customFieldMappings)
	if jsmImport != nil {
		channelID = &jsmImport.ChannelID
		if mappedID, ok := jsmImport.RequestTypes[jiraRequestType]; ok {
			requestTypeID = &mappedID
		}
	}

	// Map all Jira fixVersions to Windshift milestones. Older importer builds
	// only attached the first version, losing multi-release semantics even though
	// Windshift supports multiple item_milestones rows.
	milestoneIDs := make([]int, 0, len(issue.Fields.FixVersions))
	seenMilestones := make(map[int]struct{}, len(issue.Fields.FixVersions))
	for _, version := range issue.Fields.FixVersions {
		if mid, ok := versionMap[version.ID]; ok {
			if _, exists := seenMilestones[mid]; !exists {
				milestoneIDs = append(milestoneIDs, mid)
				seenMilestones[mid] = struct{}{}
			}
		}
	}

	// Map priority through the synonym table so Jira-only names (Highest, Lowest,
	// Blocker, Major, Minor, Trivial) land on canonical Windshift priorities
	// instead of falling back to the workspace default.
	var priorityName string
	if issue.Fields.Priority != nil && issue.Fields.Priority.Name != "" {
		priorityName = jira.SuggestPriorityMapping(issue.Fields.Priority.Name)
	}

	var dueDate *time.Time
	if issue.Fields.DueDate != "" {
		if parsed, err := time.Parse("2006-01-02", issue.Fields.DueDate); err == nil {
			dueDate = &parsed
		}
	}

	// Preserve Jira's original timestamps so chronology survives the import.
	// Without this every imported item appears created at import time, which
	// breaks reports, "recent" filters, and the timeline view.
	createdAt := jira.ParseJiraTimestamp(issue.Fields.Created)
	updatedAt := jira.ParseJiraTimestamp(issue.Fields.Updated)

	// Convert description from ADF to markdown, resolving Jira accountIDs to
	// Windshift usernames so MentionService picks up @mentions on import. Media
	// nodes are converted without a resolver here (placeholders); once
	// attachments are imported below we re-render with a media resolver that
	// links them to the imported attachments, and persist the updated text.
	rawDescription := issue.Fields.Description
	description := ""
	if rawDescription != nil {
		description = jira.ConvertADFToMarkdown(rawDescription, mentionResolver, nil)
	}

	// Process custom fields. Standard top-level fields that have no dedicated
	// Windshift column ride along inside the same JSON bag so reports and exports
	// can still surface them. Underscore-prefixed keys are importer metadata;
	// user-selected Jira custom fields are keyed by Windshift custom field ID.
	customFieldValues := make(map[string]any)
	customFieldValues["_jira_issue_id"] = issue.ID
	customFieldValues["_jira_issue_key"] = issue.Key
	if jiraKeyFieldID := customFieldIDMap[jiraIssueKeyFieldSourceID]; jiraKeyFieldID > 0 && issue.Key != "" {
		customFieldValues[strconv.Itoa(jiraKeyFieldID)] = issue.Key
	}
	if issue.Self != "" {
		customFieldValues["_jira_self"] = issue.Self
	}
	if issue.Fields.Project != nil {
		customFieldValues["_jira_project_key"] = issue.Fields.Project.Key
	}
	if resolved := jira.ParseJiraTimestamp(issue.Fields.Resolved); resolved != nil {
		customFieldValues["_jira_resolved_at"] = resolved.UTC().Format(time.RFC3339)
	}
	if issue.Fields.TimeTracking != nil {
		if issue.Fields.TimeTracking.RemainingEstimateSeconds > 0 {
			customFieldValues["_jira_remaining_estimate_seconds"] = issue.Fields.TimeTracking.RemainingEstimateSeconds
		}
		if issue.Fields.TimeTracking.TimeSpentSeconds > 0 {
			customFieldValues["_jira_time_spent_seconds"] = issue.Fields.TimeTracking.TimeSpentSeconds
		}
	}
	if len(issue.Fields.Components) > 0 {
		components := make([]map[string]string, 0, len(issue.Fields.Components))
		for _, component := range issue.Fields.Components {
			components = append(components, map[string]string{
				"id":          component.ID,
				"name":        component.Name,
				"description": component.Description,
			})
		}
		customFieldValues["_jira_components"] = components
	}
	if reporter := jiraUserIdentityMetadata(issue.Fields.Reporter); len(reporter) > 0 {
		customFieldValues["_jira_reporter"] = reporter
	}
	if creator := jiraUserIdentityMetadata(issue.Fields.Creator); len(creator) > 0 {
		customFieldValues["_jira_creator"] = creator
	}
	if assignee := jiraUserIdentityMetadata(issue.Fields.Assignee); len(assignee) > 0 {
		customFieldValues["_jira_assignee"] = assignee
	}
	if issue.Fields.Watches != nil {
		customFieldValues["_jira_watcher_count"] = issue.Fields.Watches.WatchCount
	}
	customFieldValues["_jira_watcher_identities_available"] = issue.Fields.WatcherIdentitiesAvailable
	if issue.Fields.WatcherFetchError != "" {
		customFieldValues["_jira_watcher_fetch_error"] = issue.Fields.WatcherFetchError
	}
	if len(issue.Fields.Watchers) > 0 {
		watchers := make([]map[string]any, 0, len(issue.Fields.Watchers))
		for idx := range issue.Fields.Watchers {
			if identity := jiraUserIdentityMetadata(&issue.Fields.Watchers[idx]); len(identity) > 0 {
				if _, mapped := userMap[issue.Fields.Watchers[idx].GetIdentifier()]; mapped {
					identity["mapped"] = true
				} else {
					identity["mapped"] = false
				}
				watchers = append(watchers, identity)
			}
		}
		if len(watchers) > 0 {
			customFieldValues["_jira_watchers"] = watchers
		}
	}
	if issue.Fields.Votes != nil {
		votes := map[string]any{
			"count":     issue.Fields.Votes.Votes,
			"has_voted": issue.Fields.Votes.HasVoted,
		}
		if len(issue.Fields.Votes.Voters) > 0 {
			voters := make([]map[string]any, 0, len(issue.Fields.Votes.Voters))
			for idx := range issue.Fields.Votes.Voters {
				if identity := jiraUserIdentityMetadata(&issue.Fields.Votes.Voters[idx]); len(identity) > 0 {
					voters = append(voters, identity)
				}
			}
			if len(voters) > 0 {
				votes["voters"] = voters
			}
		}
		customFieldValues["_jira_votes"] = votes
	}
	if issue.Fields.Security != nil {
		customFieldValues["_jira_security_level"] = map[string]string{
			"id":          issue.Fields.Security.ID,
			"name":        issue.Fields.Security.Name,
			"description": issue.Fields.Security.Description,
			"self":        issue.Fields.Security.Self,
		}
	}
	if jiraRequestType != "" {
		customFieldValues["_jira_request_type_id"] = jiraRequestType
	}
	if len(issue.Fields.Versions) > 0 {
		affectsVersions := make([]map[string]any, 0, len(issue.Fields.Versions))
		for _, version := range issue.Fields.Versions {
			affectsVersions = append(affectsVersions, jiraVersionMetadata(version))
		}
		customFieldValues["_jira_affects_versions"] = affectsVersions
		if affectsVersionField != nil {
			if values := affectedVersionOptionValues(issue, affectsVersionField); len(values) > 0 {
				customFieldValues[strconv.Itoa(affectsVersionField.FieldID)] = values
			}
		}
	}

	iterationID := extractSprintIterationID(&issue.Fields, customFieldMappings, iterationMap)

	var storyPoints *float64
	for _, mapping := range customFieldMappings {
		if mapping.JiraType == jiraRequestTypeFieldType || strings.EqualFold(strings.TrimSpace(mapping.JiraName), "Request Type") {
			continue
		}
		if isJiraStoryPointsField(mapping) {
			if shouldImportJiraStoryPoints(mapping) {
				if raw, ok := issue.Fields.CustomFields[mapping.JiraID]; ok {
					if sp, ok := numericCustomFieldValue(raw); ok {
						storyPoints = &sp
					}
				}
			}
			continue
		}
		fieldID, mapped := customFieldIDMap[mapping.JiraID]
		if !mapped {
			continue
		}
		if mapping.WindshiftType == string(jira.FieldTypeAsset) && h.customFieldType(fieldID) == string(jira.FieldTypeAsset) {
			refs := h.resolveJiraIssueAssetReferences(jobID, issue.Fields.CustomFields[mapping.JiraID])
			refs = jiraIssueAssetReferencesForSet(refs, h.customFieldAssetSetID(fieldID))
			if h.customFieldAssetAllowsMultiple(fieldID) && len(refs) > 0 {
				values := make([]map[string]any, 0, len(refs))
				for _, ref := range refs {
					values = append(values, assetCustomFieldValue(ref))
				}
				customFieldValues[strconv.Itoa(fieldID)] = values
				continue
			}
			if len(refs) == 1 {
				customFieldValues[strconv.Itoa(fieldID)] = assetCustomFieldValue(refs[0])
				continue
			}
			if labels := customFieldDisplayValues(issue.Fields.CustomFields[mapping.JiraID]); len(labels) > 0 {
				customFieldValues["_jira_asset_field_"+mapping.JiraID] = labels
			}
			continue
		}
		v, resolved := extractCustomFieldValueWithOptions(
			mapping,
			&issue.Fields,
			userMap,
			versionMap,
			choiceOptionIDs[mapping.JiraID],
		)
		if resolved {
			customFieldValues[strconv.Itoa(fieldID)] = v
		}
		if raw, preserve := unresolvedJiraCustomFieldRawValue(
			mapping,
			&issue.Fields,
			resolved,
			choiceOptionIDs[mapping.JiraID],
		); preserve {
			preserveUnresolvedJiraCustomField(customFieldValues, mapping, raw)
		}
	}

	var estimateMinutes *int
	if issue.Fields.TimeTracking != nil && issue.Fields.TimeTracking.OriginalEstimateSeconds > 0 {
		minutes := issue.Fields.TimeTracking.OriginalEstimateSeconds / 60
		if minutes == 0 {
			minutes = 1
		}
		estimateMinutes = &minutes
	}

	// Plain-text-sanitize untrusted imported values except text/textarea fields,
	// which receive their type-correct policy below.
	fieldTypes, err := validation.CustomFieldTypes(h.db, customFieldValues)
	if err != nil {
		return fmt.Errorf("failed to resolve custom field types: %w", err)
	}
	for key, v := range customFieldValues {
		if ft := fieldTypes[key]; ft == "text" || ft == "textarea" {
			continue
		}
		customFieldValues[key] = sanitizeJiraImportStrings(v)
	}
	if err := validation.ValidateAndNormalizeCustomFieldValues(h.db, customFieldValues); err != nil {
		return fmt.Errorf("failed to validate custom field values: %w", err)
	}

	customFieldValuesJSON := ""
	if len(customFieldValues) > 0 {
		if jsonBytes, err := json.Marshal(customFieldValues); err == nil {
			customFieldValuesJSON = string(jsonBytes)
		}
	}

	// The summary gets the same title sanitize the normal item-create path applies.
	itemParams := services.ItemCreationParams{
		WorkspaceID:             workspaceID,
		Title:                   sanitize.PlainTextField.Sanitize(issue.Fields.Summary),
		Description:             description,
		StatusID:                statusID,
		ItemTypeID:              itemTypeID,
		Priority:                priorityName,
		DueDate:                 dueDate,
		AssigneeID:              assigneeID,
		ReporterID:              reporterID,
		CreatorID:               creatorID,
		CreatorPortalCustomerID: creatorPortalCustomerID,
		ChannelID:               channelID,
		RequestTypeID:           requestTypeID,
		MilestoneIDs:            milestoneIDs,
		IterationID:             iterationID,
		TimeProjectID:           timeProjectID,
		StoryPoints:             storyPoints,
		EstimateMinutes:         estimateMinutes,
		CustomFieldValuesJSON:   customFieldValuesJSON,
		CreatedAt:               createdAt,
		UpdatedAt:               updatedAt,
		EventMetadata:           itemevents.Import(jobID),
		// A bulk import of issues pre-assigned to an agent user must not
		// start one agent run per imported item.
		SkipAssigneeTrigger:           true,
		AllowUnparentedGenericSubtask: true,
	}
	var itemID int64
	var previousItemMapping *previousJiraImportMapping
	upsertExisting := len(reimport) > 0 && reimport[0]
	if upsertExisting && issue.ID != "" {
		previousItemMapping, err = h.findPreviousJiraImportMapping(jobID, "item", issue.ID)
		if err != nil {
			return fmt.Errorf("find previous Jira item mapping: %w", err)
		}
		if previousItemMapping != nil {
			previousWorkspaceID, lookupErr := h.imports.ItemWorkspaceID(previousItemMapping.WindshiftID)
			if lookupErr != nil {
				if !errors.Is(lookupErr, sql.ErrNoRows) {
					return fmt.Errorf("load previous Jira item: %w", lookupErr)
				}
				previousItemMapping = nil
			} else if previousWorkspaceID != workspaceID {
				// A changed project→workspace mapping is a deliberate fork, not
				// an update of the old workspace's item.
				previousItemMapping = nil
			}
		}
	}
	if previousItemMapping != nil {
		itemID, err = h.updateImportedJiraItem(previousItemMapping.WindshiftID, itemParams)
	} else {
		itemID, err = services.CreateItem(h.db, itemParams)
	}
	if err != nil {
		return fmt.Errorf("failed to create or update item: %w", err)
	}

	meta := map[string]any{
		"summary": issue.Fields.Summary,
	}
	if jiraRequestType != "" {
		meta["jira_request_type_id"] = jiraRequestType
	}
	// Resolve the parent issue key across the three places Jira encodes it:
	//   1. Fields.Parent — team-managed projects (always populated when present).
	//   2. Fields.Epic   — some Jira Cloud responses surface the epic this way.
	//   3. customfield_* of type gh-epic-link — company-managed projects.
	// Without (3) the importer would lose epic→story relationships on the most
	// common deployment shape; without (2) we'd miss them on Cloud responses.
	parentKey := ""
	switch {
	case issue.Fields.Parent != nil && issue.Fields.Parent.Key != "":
		parentKey = issue.Fields.Parent.Key
	case issue.Fields.Epic != nil && issue.Fields.Epic.Key != "":
		parentKey = issue.Fields.Epic.Key
	default:
		for _, mapping := range customFieldMappings {
			if mapping.JiraType == "com.pyxis.greenhopper.jira:gh-epic-link" {
				if v, ok := issue.Fields.CustomFields[mapping.JiraID].(string); ok && v != "" {
					parentKey = v
				}
				break
			}
		}
	}
	if parentKey != "" {
		meta["parent_key"] = parentKey
	}
	if len(issue.Fields.IssueLinks) > 0 {
		var links []map[string]any
		for _, link := range issue.Fields.IssueLinks {
			entry := map[string]any{}
			if link.Type != nil {
				entry["type_name"] = link.Type.Name
				entry["inward"] = link.Type.Inward
				entry["outward"] = link.Type.Outward
			}
			if link.InwardIssue != nil {
				entry["inward_key"] = link.InwardIssue.Key
			}
			if link.OutwardIssue != nil {
				entry["outward_key"] = link.OutwardIssue.Key
			}
			links = append(links, entry)
		}
		meta["issue_links"] = links
	}

	if previousItemMapping != nil {
		meta["action"] = "update_existing"
		meta["was_created"] = jiraImportMappingWasCreated(previousItemMapping.Metadata)
		meta["reimported_from_job_id"] = previousItemMapping.JobID
	}
	if err := h.recordMappingAndTransferOwnership(jobID, "item", issue.ID, issue.Key, int(itemID), meta, previousItemMapping); err != nil {
		return fmt.Errorf("record Jira item mapping: %w", err)
	}

	if err := h.importIssueWatchers(jobID, int(itemID), issue, userMap); err != nil {
		return fmt.Errorf("import Jira issue watchers: %w", err)
	}

	// Attach Jira labels (top-level Fields.Labels, distinct from labels-typed
	// custom fields). Workspace-scoped — same name in different workspaces is
	// independent. Components and Affects Versions have no first-class Windshift
	// schema yet, so preserve them as prefixed labels in addition to Jira metadata.
	importLabels := append([]string{}, issue.Fields.Labels...)
	importLabels = append(importLabels, jiraPreservationLabels(issue)...)
	h.importLabels(workspaceID, int(itemID), importLabels)

	// Import attachments for this issue before comments/description media
	// linking so the Jira attachment ids are mapped to Windshift attachments,
	// letting ADF media nodes reference the imported files.
	mediaRefs, err := h.importAttachments(ctx, jobID, int(itemID), issue, userMap, client, progress)
	if err != nil {
		return fmt.Errorf("import Jira attachments: %w", err)
	}
	var mediaResolver jira.MediaResolver
	if len(mediaRefs) > 0 {
		mediaResolver = jira.NewMediaResolver(mediaRefs)
	}

	// Re-render the description now that attachments are available so media
	// nodes link to the imported attachments instead of placeholders.
	if mediaResolver != nil && rawDescription != nil {
		linked := jira.ConvertADFToMarkdown(rawDescription, mentionResolver, mediaResolver)
		if linked != "" && linked != description {
			if err := h.imports.UpdateItemDescription(int(itemID), linked, itemevents.Import(jobID)); err != nil {
				slog.Warn("Failed to update item description with linked media",
					slog.String("component", "jira"),
					slog.String("issue", issue.Key),
					slog.Any("error", err))
			}
		}
	}

	if err := h.importComments(jobID, int(itemID), issue, userMap, portalCustomerMap, mentionResolver, mediaResolver, progress); err != nil {
		return fmt.Errorf("import Jira comments: %w", err)
	}

	// Import Jira worklogs into Windshift time tracking when the project has a
	// time-project target. Jira exposes only the first page in the issue payload;
	// import what we have and log if pagination would be needed.
	if err := h.importWorklogs(jobID, int(itemID), issue, userMap, mentionResolver, timeProjectID, progress); err != nil {
		return fmt.Errorf("import Jira worklogs: %w", err)
	}

	return nil
}

func (h *JiraImportHandler) importIssueWatchers(
	jobID string,
	itemID int,
	issue *jira.JiraIssue,
	userMap map[string]int,
) error {
	if issue == nil || len(issue.Fields.Watchers) == 0 {
		return nil
	}
	itemRepo := repository.NewItemRepository(h.db)
	stableIssueID := issue.ID
	if stableIssueID == "" {
		stableIssueID = issue.Key
	}
	for idx := range issue.Fields.Watchers {
		accountID := issue.Fields.Watchers[idx].GetIdentifier()
		userID, mapped := userMap[accountID]
		if !mapped || accountID == "" {
			continue
		}
		externalID := stableIssueID + ":" + accountID
		previousMapping, err := h.findPreviousJiraImportMapping(jobID, "watch", externalID)
		if err != nil {
			return fmt.Errorf("find previous watch mapping for %s: %w", accountID, err)
		}
		wasWatching, err := itemRepo.IsWatching(userID, itemID)
		if err != nil {
			return err
		}
		if err := itemRepo.Watch(userID, itemID, ""); err != nil {
			return err
		}
		wasCreated := !wasWatching
		metadata := map[string]any{
			"user_id":     userID,
			"account_id":  accountID,
			"was_created": wasCreated,
		}
		ownershipMapping := previousMapping
		if previousMapping != nil && previousMapping.WindshiftID == itemID {
			metadata["was_created"] = jiraImportMappingWasCreated(previousMapping.Metadata)
			metadata["reimported_from_job_id"] = previousMapping.JobID
		} else {
			ownershipMapping = nil
		}
		if err := h.recordMappingAndTransferOwnership(jobID, "watch", externalID, issue.Key, itemID, metadata, ownershipMapping); err != nil {
			return fmt.Errorf("record Jira watch mapping: %w", err)
		}
	}
	return nil
}

func (h *JiraImportHandler) updateImportedJiraItem(
	itemID int,
	params services.ItemCreationParams,
) (int64, error) {
	return h.imports.UpdateImportedItem(itemID, params)
}

// ================================================================
// Phase 3: Parent/Hierarchy Linking
// ================================================================

// linkParents sets parent_id on imported items whose Jira issue had a parent field.
// Must be called after all issues for a project are imported so that both
// parent and child exist in jira_import_id_mappings.
func (h *JiraImportHandler) linkParents(jobID string) {
	links, err := h.imports.ParentLinks(jobID)
	if err != nil {
		slog.Error("Failed to query item mappings for parent linking", slog.String("component", "jira"), slog.Any("error", err))
		return
	}

	for _, link := range links {
		// Look up parent's Windshift ID from mappings
		parentID, err := h.imports.LookupMappedEntityByKey(jobID, "item", link.ParentKey)
		if err != nil {
			slog.Debug("Parent not found in import mappings",
				slog.String("component", "jira"),
				slog.String("parentKey", link.ParentKey),
				slog.Int("childID", link.ChildID))
			continue
		}

		if err := h.imports.ValidateParentLink(link.ChildID, parentID); err != nil {
			slog.Error("Rejected invalid Jira parent link",
				slog.String("component", "jira"),
				slog.Int("childID", link.ChildID),
				slog.Int("parentID", parentID),
				slog.Any("error", err))
			continue
		}

		// Update the child item's parent_id directly.
		// We cannot use ItemUpdateService here because it requires a valid user ID
		// for history tracking, and the import runs without a user context.
		err = repository.NewItemRepository(h.db).SetParentDirect(link.ChildID, parentID)
		if err != nil {
			slog.Error("Failed to set parent_id",
				slog.String("component", "jira"),
				slog.Int("childID", link.ChildID),
				slog.Int("parentID", parentID),
				slog.Any("error", err))
		}
	}
}

// ================================================================
// Phase 4: Comment Import
// ================================================================

// importComments imports comments from a Jira issue into Windshift
func (h *JiraImportHandler) importComments(jobID string, itemID int, issue *jira.JiraIssue, userMap, portalCustomerMap map[string]int, mentionResolver jira.MentionResolver, mediaResolver jira.MediaResolver, progress *ImportProgress) error {
	if issue.Fields.Comment == nil || len(issue.Fields.Comment.Comments) == 0 {
		return nil
	}

	// Create a CommentService without notification/webhook/mention services
	// so bulk import doesn't generate notifications
	commentSvc := services.NewCommentService(h.db)

	// dummyID is fetched lazily — most issues have only resolvable authors,
	// and we don't want to create the row unless we actually need it.
	dummyID := 0
	resolveAuthor := func(c *jira.JiraComment) (int, *int) {
		if c.Author != nil && c.Author.GetIdentifier() != "" {
			if uid, ok := userMap[c.Author.GetIdentifier()]; ok {
				return uid, nil
			}
			if customerID, ok := portalCustomerMap[c.Author.GetIdentifier()]; ok {
				return 0, &customerID
			}
		}
		if dummyID == 0 {
			id, err := h.ensureImportedDummyUser()
			if err != nil {
				slog.Error("Failed to ensure imported dummy user",
					slog.String("component", "jira"),
					slog.String("issue", issue.Key),
					slog.Any("error", err))
				return 0, nil
			}
			dummyID = id
		}
		return dummyID, nil
	}

	for _, comment := range issue.Fields.Comment.Comments {
		if progress != nil {
			progress.TotalComments++
		}
		content := jira.ConvertADFToMarkdown(comment.Body, mentionResolver, mediaResolver)
		if content == "" {
			continue
		}

		authorID, portalCustomerID := resolveAuthor(&comment)
		if authorID == 0 && portalCustomerID == nil {
			// Dummy-user creation failed; skip rather than violate the FK.
			continue
		}

		createdAt := jira.ParseJiraTimestamp(comment.Created)
		updatedAt := jira.ParseJiraTimestamp(comment.Updated)

		// Jira distinguishes "internal" (restricted to agents) from public
		// comments via its visibility field. Windshift models only a
		// private/internal toggle, so any restricted comment is imported as
		// private and the original role/group scope is preserved in metadata.
		isPrivate := (comment.ServiceDeskPublic != nil && !*comment.ServiceDeskPublic) ||
			(comment.Visibility != nil && (comment.Visibility.Type != "" || comment.Visibility.Value != ""))

		var commentID int
		var importErr error
		var previousMapping *previousJiraImportMapping
		if comment.ID != "" {
			previousMapping, _ = h.findPreviousJiraImportMapping(jobID, "comment", comment.ID)
		}
		if previousMapping != nil {
			if !h.imports.CommentExists(previousMapping.WindshiftID) {
				previousMapping = nil
			}
		}
		if previousMapping != nil {
			importErr = commentSvc.UpdateImported(services.UpdateImportedCommentParams{
				CommentID:        previousMapping.WindshiftID,
				ItemID:           itemID,
				AuthorID:         authorID,
				PortalCustomerID: portalCustomerID,
				Content:          content,
				IsPrivate:        isPrivate,
				CreatedAt:        createdAt,
				UpdatedAt:        updatedAt,
			})
			commentID = previousMapping.WindshiftID
		} else {
			var result *services.CreateCommentResult
			result, importErr = commentSvc.CreateImported(services.CreateCommentParams{
				ItemID:           itemID,
				AuthorID:         authorID,
				PortalCustomerID: portalCustomerID,
				Content:          content,
				ActorUserID:      authorID,
				IsPrivate:        isPrivate,
				CreatedAt:        createdAt,
				UpdatedAt:        updatedAt, // preserve Jira's original updated timestamp
				EventMetadata:    itemevents.Import(jobID),
			})
			if importErr == nil {
				commentID = int(result.CommentID)
			}
		}
		if importErr != nil {
			slog.Error("Failed to import comment",
				slog.String("component", "jira"),
				slog.String("issue", issue.Key),
				slog.String("commentID", comment.ID),
				slog.Any("error", importErr))
			continue
		}

		commentMeta := map[string]any{}
		if updatedAt != nil {
			commentMeta["updated"] = updatedAt.UTC().Format(time.RFC3339)
		}
		if createdAt != nil {
			commentMeta["created"] = createdAt.UTC().Format(time.RFC3339)
		}
		if author := jiraUserIdentityMetadata(comment.Author); len(author) > 0 {
			commentMeta["author"] = author
			if accountID := comment.Author.GetIdentifier(); accountID != "" {
				commentMeta["author_account_id"] = accountID
			}
		} else {
			commentMeta["author_resolution"] = "anonymous_import_fallback"
		}
		if updateAuthor := jiraUserIdentityMetadata(comment.UpdateAuthor); len(updateAuthor) > 0 {
			commentMeta["update_author"] = updateAuthor
			if accountID := comment.UpdateAuthor.GetIdentifier(); accountID != "" {
				commentMeta["update_author_account_id"] = accountID
			}
		}
		if isPrivate {
			commentMeta["is_private"] = true
			if comment.Visibility != nil {
				commentMeta["visibility_type"] = comment.Visibility.Type
				commentMeta["visibility_value"] = comment.Visibility.Value
			}
		}
		if comment.ServiceDeskPublic != nil {
			commentMeta["jira_service_desk_public"] = *comment.ServiceDeskPublic
		}

		if previousMapping != nil {
			commentMeta["action"] = "update_existing"
			commentMeta["was_created"] = jiraImportMappingWasCreated(previousMapping.Metadata)
			commentMeta["reimported_from_job_id"] = previousMapping.JobID
		}
		if err := h.recordMappingAndTransferOwnership(jobID, "comment", comment.ID, issue.Key, commentID, commentMeta, previousMapping); err != nil {
			return fmt.Errorf("record Jira comment mapping: %w", err)
		}
		if progress != nil {
			progress.ImportedComments++
		}
	}
	return nil
}

// ================================================================
// Phase 5: Issue Link Import
// ================================================================

// importIssueLinks creates item_links from Jira issue links stored in mapping metadata.
// Must be called after all issues for a project are imported.
func (h *JiraImportHandler) importIssueLinks(jobID string) error {
	allLinks, err := h.imports.IssueLinks(jobID)
	if err != nil {
		slog.Error("Failed to query item mappings for link import", slog.String("component", "jira"), slog.Any("error", err))
		return fmt.Errorf("query Jira item mappings for links: %w", err)
	}

	linkTypeCache := make(map[string]int) // link type name -> ID
	linkSvc := services.NewItemLinkService(h.db)

	for _, info := range allLinks {
		for _, link := range info.Links {
			typeName, _ := link["type_name"].(string)
			if typeName == "" {
				continue
			}

			// Build each relationship from its outward side to avoid duplicates.
			outwardKey, _ := link["outward_key"].(string)
			if outwardKey == "" {
				// Inward-only entry. If the source is in this import we'll catch
				// the same relationship from the source's outward_key in another
				// iteration. If it isn't, we cannot represent the link in
				// Windshift today (no external-reference support yet) — log it
				// so the loss isn't silent.
				inwardKey, _ := link["inward_key"].(string)
				if inwardKey == "" {
					continue
				}
				_, err := h.imports.LookupMappedEntityByKey(jobID, "item", inwardKey)
				if errors.Is(err, sql.ErrNoRows) {
					if externalErr := h.importExternalJiraIssueLink(jobID, info.SourceID, info.SourceKey, inwardKey, typeName, "inward", link); externalErr != nil {
						if mappingErr := h.mappingFailure(jobID); mappingErr != nil {
							return mappingErr
						}
						slog.Warn("Failed to preserve inward link from non-imported issue",
							slog.String("component", "jira"), slog.String("source", inwardKey),
							slog.String("target", info.SourceKey), slog.String("typeName", typeName), slog.Any("error", externalErr))
					}
				} else if err != nil {
					slog.Warn("Failed to look up inward link source",
						slog.String("component", "jira"),
						slog.String("source", inwardKey),
						slog.Any("error", err))
				}
				continue
			}

			// Look up target Windshift ID
			targetID, err := h.imports.LookupMappedEntityByKey(jobID, "item", outwardKey)
			if errors.Is(err, sql.ErrNoRows) {
				if externalErr := h.importExternalJiraIssueLink(jobID, info.SourceID, info.SourceKey, outwardKey, typeName, "outward", link); externalErr != nil {
					if mappingErr := h.mappingFailure(jobID); mappingErr != nil {
						return mappingErr
					}
					slog.Warn("Failed to preserve outward link to non-imported issue",
						slog.String("component", "jira"), slog.String("source", info.SourceKey),
						slog.String("target", outwardKey), slog.String("typeName", typeName), slog.Any("error", externalErr))
				}
				continue
			} else if err != nil {
				continue
			}

			linkTypeID, ok := linkTypeCache[typeName]
			if !ok {
				linkTypeID, err = h.ensureLinkType(typeName, link)
				if err != nil {
					slog.Error("Failed to ensure link type",
						slog.String("component", "jira"),
						slog.String("typeName", typeName),
						slog.Any("error", err))
					continue
				}
				linkTypeCache[typeName] = linkTypeID
			}

			mappingID := fmt.Sprintf("%s-%s-%s", info.SourceKey, typeName, outwardKey)
			previousMapping, previousErr := h.findPreviousJiraImportMapping(jobID, "link", mappingID)
			if previousErr != nil {
				slog.Warn("Failed to find prior Jira item-link mapping",
					slog.String("component", "jira"),
					slog.String("mappingID", mappingID),
					slog.Any("error", previousErr))
				previousMapping = nil
			}
			linkID, err := linkSvc.CreateLinkWithMetadata(services.CreateItemLinkParams{
				LinkTypeID: linkTypeID,
				SourceType: "item",
				SourceID:   info.SourceID,
				TargetType: "item",
				TargetID:   targetID,
			}, itemevents.Import(jobID))
			if err != nil {
				slog.Error("Failed to create item link",
					slog.String("component", "jira"),
					slog.String("source", info.SourceKey),
					slog.String("target", outwardKey),
					slog.Any("error", err))
				continue
			}

			if linkID > 0 {
				if err := h.recordMapping(jobID, "link", mappingID, "", int(linkID), map[string]any{"action": "create"}); err != nil {
					return fmt.Errorf("record Jira item link mapping: %w", err)
				}
				continue
			}
			if previousMapping != nil && h.imports.ItemLinkExists(previousMapping.WindshiftID) {
				if err := h.recordMappingAndTransferOwnership(jobID, "link", mappingID, "", previousMapping.WindshiftID, map[string]any{
					"action":                 "reuse_existing_mapping",
					"was_created":            jiraImportMappingWasCreated(previousMapping.Metadata),
					"reimported_from_job_id": previousMapping.JobID,
				}, previousMapping); err != nil {
					return fmt.Errorf("record Jira item link mapping: %w", err)
				}
			}
		}
	}
	return nil
}

func (h *JiraImportHandler) importExternalJiraIssueLink(
	jobID string,
	itemID int,
	itemKey, externalKey, typeName, direction string,
	sourceMetadata map[string]any,
) error {
	return h.imports.UpsertExternalIssueLink(
		jobID, itemID, itemKey, externalKey, typeName, direction, sourceMetadata,
	)
}

// ensureLinkType finds or creates a link type matching the Jira link type
func (h *JiraImportHandler) ensureLinkType(typeName string, linkData map[string]any) (int, error) {
	forwardLabel, _ := linkData["outward"].(string)
	reverseLabel, _ := linkData["inward"].(string)
	if forwardLabel == "" {
		forwardLabel = typeName
	}
	if reverseLabel == "" {
		reverseLabel = typeName
	}

	id, err := h.imports.EnsureLinkType(typeName, forwardLabel, reverseLabel)
	if err != nil {
		return 0, fmt.Errorf("failed to create link type: %w", err)
	}
	return id, nil
}

// ================================================================
// Phase 6: Worklog Import
// ================================================================

func (h *JiraImportHandler) importWorklogs(jobID string, itemID int, issue *jira.JiraIssue, userMap map[string]int, mentionResolver jira.MentionResolver, timeProjectID *int, progress *ImportProgress) error {
	if issue.Fields.Worklog == nil || len(issue.Fields.Worklog.Worklogs) == 0 || timeProjectID == nil {
		return nil
	}
	if issue.Fields.Worklog.Total > len(issue.Fields.Worklog.Worklogs) {
		slog.Warn("Jira issue worklog payload is paginated; importing visible worklogs only",
			slog.String("component", "jira"),
			slog.String("issue", issue.Key),
			slog.Int("visible", len(issue.Fields.Worklog.Worklogs)),
			slog.Int("total", issue.Fields.Worklog.Total))
	}

	customerID, err := h.imports.TimeProjectCustomerID(*timeProjectID)
	if err != nil {
		slog.Error("Failed to resolve time project customer for Jira worklogs",
			slog.String("component", "jira"),
			slog.String("issue", issue.Key),
			slog.Int("timeProjectID", *timeProjectID),
			slog.Any("error", err))
		return nil
	}

	for _, worklog := range issue.Fields.Worklog.Worklogs {
		if progress != nil {
			progress.TotalWorklogs++
		}
		if worklog.ID == "" || worklog.TimeSpentSeconds <= 0 {
			continue
		}
		started := jira.ParseJiraTimestamp(worklog.Started)
		if started == nil {
			slog.Warn("Skipping Jira worklog without parseable started timestamp",
				slog.String("component", "jira"),
				slog.String("issue", issue.Key),
				slog.String("worklogID", worklog.ID),
				slog.String("started", worklog.Started))
			continue
		}

		durationMinutes := (worklog.TimeSpentSeconds + 59) / 60
		if durationMinutes <= 0 {
			durationMinutes = 1
		}
		end := started.Add(time.Duration(worklog.TimeSpentSeconds) * time.Second)
		date := time.Date(started.Year(), started.Month(), started.Day(), 0, 0, 0, 0, started.Location())

		description := strings.TrimSpace(jira.ConvertADFToMarkdownWithUsers(worklog.Comment, mentionResolver))
		if description == "" {
			description = strings.TrimSpace(worklog.TimeSpent)
		}
		if description == "" {
			description = "Imported Jira worklog"
		}

		var userID *int
		if worklog.Author != nil && worklog.Author.GetIdentifier() != "" {
			if uid, ok := userMap[worklog.Author.GetIdentifier()]; ok {
				userID = &uid
			}
		}

		createdAt := time.Now().Unix()
		if created := jira.ParseJiraTimestamp(worklog.Created); created != nil {
			createdAt = created.Unix()
		}
		updatedAt := createdAt
		if updated := jira.ParseJiraTimestamp(worklog.Updated); updated != nil {
			updatedAt = updated.Unix()
		}

		previousMapping, lookupErr := h.findPreviousJiraImportMapping(jobID, "worklog", worklog.ID)
		if lookupErr != nil {
			slog.Warn("Failed to find prior Jira worklog mapping",
				slog.String("component", "jira"),
				slog.String("worklogID", worklog.ID),
				slog.Any("error", lookupErr))
			previousMapping = nil
		}
		worklogID, err := h.imports.UpsertImportedWorklog(repository.ImportedWorklog{
			ProjectID:       *timeProjectID,
			CustomerID:      customerID,
			UserID:          userID,
			ItemID:          itemID,
			Description:     description,
			DateUnix:        date.Unix(),
			StartTimeUnix:   started.Unix(),
			EndTimeUnix:     end.Unix(),
			DurationMinutes: durationMinutes,
			CreatedAtUnix:   createdAt,
			UpdatedAtUnix:   updatedAt,
		}, previousMapping)
		if err != nil {
			slog.Error("Failed to import Jira worklog",
				slog.String("component", "jira"),
				slog.String("issue", issue.Key),
				slog.String("worklogID", worklog.ID),
				slog.Any("error", err))
			continue
		}

		worklogMeta := map[string]any{
			"time_spent_seconds": worklog.TimeSpentSeconds,
			"started":            started.UTC().Format(time.RFC3339),
		}
		if author := jiraUserIdentityMetadata(worklog.Author); len(author) > 0 {
			worklogMeta["author"] = author
		} else {
			worklogMeta["author_resolution"] = "anonymous"
		}
		if previousMapping != nil {
			worklogMeta["action"] = "update_existing"
			worklogMeta["was_created"] = jiraImportMappingWasCreated(previousMapping.Metadata)
			worklogMeta["reimported_from_job_id"] = previousMapping.JobID
		}
		if err := h.recordMappingAndTransferOwnership(jobID, "worklog", worklog.ID, issue.Key, int(worklogID), worklogMeta, previousMapping); err != nil {
			return fmt.Errorf("record Jira worklog mapping: %w", err)
		}
		if progress != nil {
			progress.ImportedWorklogs++
		}
	}
	return nil
}

// ================================================================
// Phase 7: Attachment Import
// ================================================================

// importAttachments downloads and stores attachments from a Jira issue. It
// returns a map from Jira attachment id → the Windshift attachment reference
// (id, mime type, original filename) so ADF media nodes can be linked to the
// imported attachment instead of left as a placeholder.
func (h *JiraImportHandler) importAttachments(ctx context.Context, jobID string, itemID int, issue *jira.JiraIssue, userMap map[string]int, client jira.Client, progress *ImportProgress) (map[string]jira.MediaAttachment, error) {
	if len(issue.Fields.Attachment) == 0 {
		return nil, nil
	}

	// Get attachment storage path from settings.
	attachmentPath, ok := h.imports.AttachmentPath()
	if !ok {
		slog.Warn("Attachment settings not configured, skipping attachment import",
			slog.String("component", "jira"), slog.String("issue", issue.Key))
		return jiraNoAttachmentMediaRefs()
	}
	if err := ensureJiraAttachmentStorage(attachmentPath); err != nil {
		slog.Error("Failed to prepare attachment storage",
			slog.String("component", "jira"), slog.String("issue", issue.Key), slog.Any("error", err))
		return map[string]jira.MediaAttachment{}, nil
	}

	mediaRefs := make(map[string]jira.MediaAttachment)
	for _, attachment := range issue.Fields.Attachment {
		if attachment.Content == "" {
			continue
		}

		progress.TotalAttachments++

		previousMapping, lookupErr := h.findPreviousJiraImportMapping(jobID, "attachment", attachment.ID)
		if lookupErr != nil {
			slog.Warn("Failed to find prior Jira attachment mapping",
				slog.String("component", "jira"),
				slog.String("attachmentID", attachment.ID),
				slog.Any("error", lookupErr))
			previousMapping = nil
		}
		if previousMapping != nil {
			mimeType, originalFilename, exists := h.imports.ReassignAttachment(previousMapping.WindshiftID, itemID)
			if !exists {
				previousMapping = nil
			} else {
				metadata := jiraImportMappingMetadata(previousMapping.Metadata)
				if metadata == nil {
					metadata = make(map[string]any)
				}
				metadata["action"] = "reuse_existing_mapping"
				metadata["was_created"] = jiraImportMappingWasCreated(previousMapping.Metadata)
				metadata["reimported_from_job_id"] = previousMapping.JobID
				if mappingErr := h.recordMappingAndTransferOwnership(jobID, "attachment", attachment.ID, issue.Key, previousMapping.WindshiftID, metadata, previousMapping); mappingErr == nil {
					mediaRefs[attachment.ID] = jira.MediaAttachment{
						ID:               previousMapping.WindshiftID,
						MimeType:         mimeType,
						OriginalFilename: originalFilename,
					}
					progress.ImportedAttachments++
					continue
				}
				return nil, fmt.Errorf("record Jira attachment mapping: %w", h.mappingFailure(jobID))
			}
		}

		reader, _, err := client.DownloadAttachment(ctx, attachment.Content)
		if err != nil {
			slog.Error("Failed to download attachment",
				slog.String("component", "jira"),
				slog.String("issue", issue.Key),
				slog.String("filename", attachment.Filename),
				slog.Any("error", err))
			continue
		}

		storedFilename := fmt.Sprintf("%s_%s", uuid.New().String(), filepath.Base(attachment.Filename))
		filePath := filepath.Join(attachmentPath, storedFilename)

		file, err := os.Create(filePath) //nolint:gosec // G304 — filePath from attachmentPath + UUID + filename
		if err != nil {
			_ = reader.Close()
			slog.Error("Failed to create attachment file",
				slog.String("component", "jira"),
				slog.String("path", filePath),
				slog.Any("error", err))
			continue
		}

		written, err := io.Copy(file, reader)
		_ = file.Close()
		_ = reader.Close()
		if err != nil {
			_ = os.Remove(filePath) //nolint:gosec // G703 — filePath from attachmentPath + UUID + filepath.Base(filename)
			slog.Error("Failed to write attachment file",
				slog.String("component", "jira"),
				slog.String("path", filePath),
				slog.Any("error", err))
			continue
		}

		// Use actual written size if Jira didn't report one
		fileSize := attachment.Size
		if fileSize == 0 {
			fileSize = written
		}

		var uploadedBy *int
		if attachment.Author != nil && attachment.Author.GetIdentifier() != "" {
			if uid, ok := userMap[attachment.Author.GetIdentifier()]; ok {
				uploadedBy = &uid
			}
		}

		// SECURITY: do not trust the remote-declared Content-Type. A malicious
		// or compromised Jira source controls attachment.MimeType, and the
		// download handler uses the stored MIME to decide inline rendering.
		// Detect the type from the actual bytes so a remote source cannot get a
		// script-capable document served inline in the app origin (stored XSS).
		mimeType := detectStoredMimeType(filePath)
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}

		attachmentSvc := services.NewAttachmentService(h.db)
		attachmentID, err := attachmentSvc.CreateRecord(services.CreateAttachmentParams{
			ItemID:           itemID,
			EntityType:       "item",
			Filename:         storedFilename,
			OriginalFilename: attachment.Filename,
			FilePath:         filePath,
			MimeType:         mimeType,
			FileSize:         fileSize,
			UploadedBy:       uploadedBy,
		})
		if err != nil {
			slog.Error("Failed to insert attachment record",
				slog.String("component", "jira"),
				slog.String("issue", issue.Key),
				slog.String("filename", attachment.Filename),
				slog.Any("error", err))
			continue
		}

		attachmentMeta := map[string]any{
			"filename":  attachment.Filename,
			"content":   attachment.Content,
			"mime_type": mimeType,
			"size":      fileSize,
		}
		if attachment.Thumbnail != "" {
			attachmentMeta["thumbnail"] = attachment.Thumbnail
		}
		if author := jiraUserIdentityMetadata(attachment.Author); len(author) > 0 {
			attachmentMeta["author"] = author
		}
		if createdAt := jira.ParseJiraTimestamp(attachment.Created); createdAt != nil {
			if err := h.imports.PreserveAttachmentCreatedAt(attachmentID, *createdAt); err != nil {
				slog.Warn("Failed to preserve Jira attachment created timestamp",
					slog.String("component", "jira"),
					slog.String("issue", issue.Key),
					slog.String("attachmentID", attachment.ID),
					slog.Any("error", err))
			} else {
				attachmentMeta["created"] = createdAt.UTC().Format(time.RFC3339)
			}
		}

		if err := h.recordMapping(jobID, "attachment", attachment.ID, issue.Key, int(attachmentID), attachmentMeta); err != nil {
			return nil, fmt.Errorf("record Jira attachment mapping: %w", err)
		}
		progress.ImportedAttachments++

		// Record the Jira→Windshift attachment reference so ADF media nodes
		// in the description/comments can link to the imported attachment.
		if attachment.ID != "" {
			mediaRefs[attachment.ID] = jira.MediaAttachment{
				ID:               int(attachmentID),
				MimeType:         mimeType,
				OriginalFilename: attachment.Filename,
			}
		}
	}
	return mediaRefs, nil
}

func jiraNoAttachmentMediaRefs() (map[string]jira.MediaAttachment, error) {
	return nil, nil
}

func ensureJiraAttachmentStorage(attachmentPath string) error {
	if err := os.MkdirAll(attachmentPath, 0o750); err != nil {
		return fmt.Errorf("create attachment directory: %w", err)
	}
	return nil
}

// detectStoredMimeType sniffs the Content-Type of an already-written file from
// its leading bytes via http.DetectContentType, so the stored MIME reflects the
// actual content rather than a remote-declared value. Returns "" if the file
// cannot be read (caller falls back to application/octet-stream).
func detectStoredMimeType(path string) string {
	f, err := os.Open(path) //nolint:gosec // G304 — path is attachmentPath + UUID + filepath.Base(filename)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return ""
	}
	return http.DetectContentType(buf[:n])
}
