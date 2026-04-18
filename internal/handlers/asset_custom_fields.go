package handlers

import (
	"database/sql"
	"strconv"
	"strings"

	"windshift/internal/models"
)

// extractUserID extracts user ID from various value formats (int, float64, or map with "id")
func extractUserID(val interface{}) int {
	switch v := val.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case map[string]interface{}:
		if id, ok := v["id"]; ok {
			return extractUserID(id)
		}
	}
	return 0
}

// extractRefID extracts an integer ID from a raw JSON value shape used by
// reference-typed custom fields (asset, user, etc.). Shares the same semantics
// as extractUserID but is named to make intent clear at call sites.
func extractRefID(val interface{}) int { return extractUserID(val) }

// getAssetFieldIDsForAssetType returns a set of custom field IDs that are
// asset-type for a given asset type.
func (h *AssetHandler) getAssetFieldIDsForAssetType(assetTypeID int) (map[int]bool, error) {
	rows, err := h.db.Query(`
		SELECT cfd.id
		FROM custom_field_definitions cfd
		JOIN asset_type_fields atf ON atf.custom_field_id = cfd.id
		WHERE atf.asset_type_id = ? AND cfd.field_type = 'asset'
	`, assetTypeID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	fieldIDs := make(map[int]bool)
	for rows.Next() {
		var fieldID int
		if err := rows.Scan(&fieldID); err != nil {
			return nil, err
		}
		fieldIDs[fieldID] = true
	}
	return fieldIDs, nil
}

// enrichAssetRefCustomFields resolves asset-type custom fields to the
// referenced asset's summary (id, title, asset_tag). If the referenced asset
// has been deleted, it is tagged {"deleted": true} so the UI can render a
// marker instead of silently dropping the reference.
func (h *AssetHandler) enrichAssetRefCustomFields(asset *models.Asset) error {
	if len(asset.CustomFieldValues) == 0 {
		return nil
	}

	assetFieldIDs, err := h.getAssetFieldIDsForAssetType(asset.AssetTypeID)
	if err != nil {
		return err
	}
	if len(assetFieldIDs) == 0 {
		return nil
	}

	for fieldID := range assetFieldIDs {
		fieldKey := strconv.Itoa(fieldID)
		val, ok := asset.CustomFieldValues[fieldKey]
		if !ok || val == nil {
			continue
		}

		refID := extractRefID(val)
		if refID <= 0 {
			continue
		}

		var title, assetTag sql.NullString
		err := h.db.QueryRow(`SELECT title, asset_tag FROM assets WHERE id = ?`, refID).Scan(&title, &assetTag)
		if err != nil {
			if err == sql.ErrNoRows {
				asset.CustomFieldValues[fieldKey] = map[string]interface{}{
					"id":      refID,
					"title":   "(deleted asset)",
					"deleted": true,
				}
				continue
			}
			return err
		}

		asset.CustomFieldValues[fieldKey] = map[string]interface{}{
			"id":        refID,
			"title":     title.String,
			"asset_tag": assetTag.String,
		}
	}

	return nil
}

// getUserFieldIDsForAssetType returns a set of custom field IDs that are user-type for a given asset type
func (h *AssetHandler) getUserFieldIDsForAssetType(assetTypeID int) (map[int]bool, error) {
	rows, err := h.db.Query(`
		SELECT cfd.id
		FROM custom_field_definitions cfd
		JOIN asset_type_fields atf ON atf.custom_field_id = cfd.id
		WHERE atf.asset_type_id = ? AND cfd.field_type = 'user'
	`, assetTypeID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	fieldIDs := make(map[int]bool)
	for rows.Next() {
		var fieldID int
		if err := rows.Scan(&fieldID); err != nil {
			return nil, err
		}
		fieldIDs[fieldID] = true
	}
	return fieldIDs, nil
}

// enrichUserCustomFields resolves user IDs to full user data for user-type custom fields
func (h *AssetHandler) enrichUserCustomFields(asset *models.Asset) error {
	if len(asset.CustomFieldValues) == 0 {
		return nil
	}

	// Get user-type field IDs for this asset's type
	userFieldIDs, err := h.getUserFieldIDsForAssetType(asset.AssetTypeID)
	if err != nil {
		return err
	}

	if len(userFieldIDs) == 0 {
		return nil
	}

	// Resolve each user field
	for fieldID := range userFieldIDs {
		fieldKey := strconv.Itoa(fieldID)
		val, ok := asset.CustomFieldValues[fieldKey]
		if !ok || val == nil {
			continue
		}

		userID := extractUserID(val)
		if userID <= 0 {
			continue
		}

		// Query user data
		var firstName, lastName, email, avatarURL sql.NullString
		err := h.db.QueryRow(`
			SELECT first_name, last_name, email, avatar_url
			FROM users WHERE id = ?
		`, userID).Scan(&firstName, &lastName, &email, &avatarURL)
		if err != nil {
			if err == sql.ErrNoRows {
				// User was deleted — keep the ID so the UI can render a
				// "(deleted user)" marker instead of silently losing the
				// assignment history.
				asset.CustomFieldValues[fieldKey] = map[string]interface{}{
					"id":      userID,
					"name":    "(deleted user)",
					"deleted": true,
				}
				continue
			}
			return err
		}

		// Replace with enriched data
		asset.CustomFieldValues[fieldKey] = map[string]interface{}{
			"id":         userID,
			"name":       strings.TrimSpace(firstName.String + " " + lastName.String),
			"email":      email.String,
			"avatar_url": avatarURL.String,
		}
	}

	return nil
}

// normalizeUserFieldValues extracts just the user ID from user-type custom field values before storage
func (h *AssetHandler) normalizeUserFieldValues(customFieldValues map[string]interface{}, assetTypeID int) error {
	if len(customFieldValues) == 0 {
		return nil
	}

	// Get user-type field IDs for this asset's type
	userFieldIDs, err := h.getUserFieldIDsForAssetType(assetTypeID)
	if err != nil {
		return err
	}

	if len(userFieldIDs) == 0 {
		return nil
	}

	// Normalize each user field to just the ID
	for fieldID := range userFieldIDs {
		fieldKey := strconv.Itoa(fieldID)
		val, ok := customFieldValues[fieldKey]
		if !ok || val == nil {
			continue
		}

		userID := extractUserID(val)
		if userID > 0 {
			customFieldValues[fieldKey] = userID
		} else {
			// Invalid value, remove it
			delete(customFieldValues, fieldKey)
		}
	}

	return nil
}
