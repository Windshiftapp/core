package handlers

import (
	"errors"
	"strconv"
	"strings"

	"windshift/internal/models"
	"windshift/internal/repository"
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

// enrichAssetRefCustomFields resolves asset-type custom fields to the
// referenced asset's summary (id, title, asset_tag). If the referenced asset
// has been deleted, it is tagged {"deleted": true} so the UI can render a
// marker instead of silently dropping the reference.
func (h *AssetHandler) enrichAssetRefCustomFields(asset *models.Asset) error {
	if len(asset.CustomFieldValues) == 0 {
		return nil
	}

	assetFieldIDs, err := h.repo.FindCustomFieldIDsByType(asset.AssetTypeID, "asset")
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

		summary, err := h.repo.GetAssetSummary(refID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
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
			"title":     summary.Title,
			"asset_tag": summary.AssetTag,
		}
	}

	return nil
}

// enrichUserCustomFields resolves user IDs to full user data for user-type custom fields
func (h *AssetHandler) enrichUserCustomFields(asset *models.Asset) error {
	if len(asset.CustomFieldValues) == 0 {
		return nil
	}

	userFieldIDs, err := h.repo.FindCustomFieldIDsByType(asset.AssetTypeID, "user")
	if err != nil {
		return err
	}
	if len(userFieldIDs) == 0 {
		return nil
	}

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

		info, err := h.repo.GetUserBasicInfo(userID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
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

		asset.CustomFieldValues[fieldKey] = map[string]interface{}{
			"id":         userID,
			"name":       strings.TrimSpace(info.FirstName.String + " " + info.LastName.String),
			"email":      info.Email.String,
			"avatar_url": info.AvatarURL.String,
		}
	}

	return nil
}

// normalizeUserFieldValues extracts just the user ID from user-type custom field values before storage
func (h *AssetHandler) normalizeUserFieldValues(customFieldValues map[string]interface{}, assetTypeID int) error {
	if len(customFieldValues) == 0 {
		return nil
	}

	userFieldIDs, err := h.repo.FindCustomFieldIDsByType(assetTypeID, "user")
	if err != nil {
		return err
	}
	if len(userFieldIDs) == 0 {
		return nil
	}

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
			delete(customFieldValues, fieldKey)
		}
	}

	return nil
}
