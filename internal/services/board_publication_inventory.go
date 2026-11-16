package services

import (
	"encoding/json"
	"net/url"
	"slices"

	"windshift/internal/models"
)

type BoardAudienceOrganisation struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type BoardPublicationLocation struct {
	Kind            string                      `json:"kind"`
	Label           string                      `json:"label"`
	URL             string                      `json:"url"`
	ChannelID       *int                        `json:"channel_id"`
	Enabled         bool                        `json:"enabled"`
	EffectivePolicy models.CustomerAccessPolicy `json:"effective_policy"`
}

// Intersect policies across locations; IDs are any-of within each policy.
func intersectBoardPolicies(policies ...models.CustomerAccessPolicy) models.CustomerAccessPolicy {
	result := models.CustomerAccessPolicy{Mode: "public", OrganisationIDs: []int{}}
	for _, policy := range policies {
		if policy.Mode == "public" {
			continue
		}
		if result.Mode == "public" {
			result.Mode = "organisations"
			result.OrganisationIDs = slices.Clone(policy.OrganisationIDs)
			continue
		}
		result.OrganisationIDs = slices.DeleteFunc(result.OrganisationIDs, func(id int) bool { return !slices.Contains(policy.OrganisationIDs, id) })
	}
	return result
}

// Load page metadata in batches, never fetching board content or performing per-row reads.
func (s *CollectionApplicationService) boardSharingRows(ids []int, admin, instanceEnabled bool) ([]BoardSharing, error) {
	items := make([]BoardSharing, 0, len(ids))
	if len(ids) == 0 {
		return items, nil
	}
	placeholders, args := sqlPlaceholders(ids)
	rows, err := s.db.Query(`SELECT c.id, c.name, c.created_by, COALESCE(NULLIF(TRIM(u.first_name || ' ' || u.last_name), ''), u.username, ''), u.is_active,
	 c.workspace_id, COALESCE(w.name, ''), COALESCE(c.ql_query, ''), c.board_enabled, c.board_standalone_enabled,
	 c.public_slug, c.public_milestone_swimlanes, COALESCE(a.mode, 'public'), COALESCE(a.organisation_ids, '[]'),
	 c.sharing_revision, c.public_revoked_at, c.public_revoke_reason, c.sharing_changed_at, c.sharing_changed_by,
	 COALESCE(NULLIF(TRIM(actor.first_name || ' ' || actor.last_name), ''), actor.username, '')
	 FROM collections c LEFT JOIN users u ON u.id = c.created_by LEFT JOIN workspaces w ON w.id = c.workspace_id
	 LEFT JOIN users actor ON actor.id = c.sharing_changed_by
	 LEFT JOIN customer_resource_access a ON a.resource_kind = 'board' AND a.resource_id = c.id
	 WHERE c.id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	byID := map[int]int{}
	for rows.Next() {
		var item BoardSharing
		var raw string
		err := rows.Scan(&item.CollectionID, &item.Name, &item.OwnerID, &item.OwnerName, &item.OwnerActive,
			&item.WorkspaceID, &item.WorkspaceName, &item.Scope, &item.Enabled, &item.StandaloneEnabled, &item.Slug,
			&item.MilestoneSwimlanes, &item.Policy.Mode, &raw, &item.Revision, &item.RevokedAt, &item.RevokeReason,
			&item.ChangedAt, &item.ChangedBy, &item.ChangedByName)
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &item.Policy.OrganisationIDs); err != nil {
			_ = rows.Close()
			return nil, err
		}
		item.CanAdminister, item.InstanceEnabled = admin, instanceEnabled
		item.Organizations, item.Locations = []BoardAudienceOrganisation{}, []BoardPublicationLocation{}
		if item.StandaloneEnabled && item.Slug != nil {
			item.Locations = append(item.Locations, BoardPublicationLocation{Kind: "standalone", Label: "Standalone", URL: "/board/" + url.PathEscape(*item.Slug),
				Enabled: instanceEnabled && item.Enabled && item.RevokedAt == nil, EffectivePolicy: item.Policy})
		}
		byID[item.CollectionID] = len(items)
		items = append(items, item)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	placements, err := s.db.Query(`SELECT p.collection_id, p.id, p.channel_id, ch.name, ch.status, COALESCE(ch.config, '{}'),
	 p.label, p.enabled, p.access_mode, p.organisation_ids, COALESCE(a.mode, 'public'), COALESCE(a.organisation_ids, '[]')
	 FROM board_placements p JOIN channels ch ON ch.id = p.channel_id
	 LEFT JOIN customer_resource_access a ON a.resource_kind = 'portal' AND a.resource_id = p.channel_id
	 WHERE p.collection_id IN (`+placeholders+`) ORDER BY p.display_order, p.id`, args...)
	if err != nil {
		return nil, err
	}
	for placements.Next() {
		var collectionID, channelID int
		var id, name, status, rawConfig, label, placementIDs, portalIDs string
		var enabled bool
		var placementPolicy, portalPolicy models.CustomerAccessPolicy
		err := placements.Scan(&collectionID, &id, &channelID, &name, &status, &rawConfig, &label, &enabled, &placementPolicy.Mode, &placementIDs, &portalPolicy.Mode, &portalIDs)
		if err != nil {
			_ = placements.Close()
			return nil, err
		}
		var config models.ChannelConfig
		for _, value := range []struct {
			raw    string
			target any
		}{{rawConfig, &config}, {placementIDs, &placementPolicy.OrganisationIDs}, {portalIDs, &portalPolicy.OrganisationIDs}} {
			if err := json.Unmarshal([]byte(value.raw), value.target); err != nil {
				_ = placements.Close()
				return nil, err
			}
		}
		index, found := byID[collectionID]
		if !found {
			continue
		}
		item := &items[index]
		item.Locations = append(item.Locations, BoardPublicationLocation{Kind: "portal", Label: name + " / " + label, ChannelID: &channelID,
			URL:             "/portal/" + url.PathEscape(config.PortalSlug) + "/boards/" + url.PathEscape(id),
			Enabled:         instanceEnabled && item.Enabled && item.RevokedAt == nil && enabled && status == "enabled" && config.PortalModuleEnabled("boards"),
			EffectivePolicy: intersectBoardPolicies(item.Policy, portalPolicy, placementPolicy)})
	}
	err = placements.Err()
	_ = placements.Close()
	if err != nil {
		return nil, err
	}
	orgSet := map[int]bool{}
	for _, item := range items {
		for _, id := range item.Policy.OrganisationIDs {
			orgSet[id] = true
		}
		for _, location := range item.Locations {
			for _, id := range location.EffectivePolicy.OrganisationIDs {
				orgSet[id] = true
			}
		}
	}
	orgs := map[int]string{}
	orgIDs := make([]int, 0, len(orgSet))
	for id := range orgSet {
		orgIDs = append(orgIDs, id)
	}
	if len(orgIDs) > 0 {
		p, a := sqlPlaceholders(orgIDs)
		rows, err := s.db.Query(`SELECT id, name FROM customer_organisations WHERE id IN (`+p+`)`, a...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id int
			var name string
			if err := rows.Scan(&id, &name); err != nil {
				_ = rows.Close()
				return nil, err
			}
			orgs[id] = name
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return nil, err
		}
	}
	for i := range items {
		relevant := slices.Clone(items[i].Policy.OrganisationIDs)
		for _, location := range items[i].Locations {
			relevant = append(relevant, location.EffectivePolicy.OrganisationIDs...)
		}
		slices.Sort(relevant)
		for _, id := range slices.Compact(relevant) {
			if name, found := orgs[id]; found {
				items[i].Organizations = append(items[i].Organizations, BoardAudienceOrganisation{ID: id, Name: name})
			}
		}
	}
	// Preserve the page's requested ordering, not the database's IN ordering.
	ordered := make([]BoardSharing, 0, len(items))
	for _, id := range ids {
		if index, found := byID[id]; found {
			ordered = append(ordered, items[index])
		}
	}
	return ordered, nil
}

func sqlPlaceholders(ids []int) (placeholders string, args []any) {
	args = make([]any, len(ids))
	for i, id := range ids {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args[i] = id
	}
	return placeholders, args
}
