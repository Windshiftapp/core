package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
)

type PortalExperienceSettings struct {
	Revision      string   `json:"revision"`
	Presentation  string   `json:"presentation"`
	Modules       []string `json:"modules"`
	Navigation    []string `json:"navigation"`
	DefaultModule string   `json:"default_module"`
}

var portalNavigationTarget = regexp.MustCompile(`^(board:[a-zA-Z0-9-]{1,64}|(form|article):[1-9]\d*)$`)

func validatePortalExperienceSettings(input PortalExperienceSettings) error {
	if input.Presentation != "" && input.Presentation != "classic" && input.Presentation != "compact" {
		return fmt.Errorf("presentation must be classic or compact")
	}
	if len(input.Modules) > len(models.PortalModuleKinds) || len(input.Navigation) > 100 || len(input.DefaultModule) > 200 {
		return fmt.Errorf("portal navigation configuration is too large")
	}
	for _, module := range input.Modules {
		if !slices.Contains(models.PortalModuleKinds, module) {
			return fmt.Errorf("unknown portal module %q", module)
		}
	}
	seen := map[string]bool{}
	for _, key := range input.Navigation {
		if seen[key] || (!slices.Contains(models.PortalModuleKinds, key) && !portalNavigationTarget.MatchString(key)) {
			return fmt.Errorf("invalid or duplicate navigation key %q", key)
		}
		seen[key] = true
	}
	if key := input.DefaultModule; key != "" && !slices.Contains(models.PortalModuleKinds, key) && !portalNavigationTarget.MatchString(key) {
		return fmt.Errorf("invalid default module %q", key)
	}
	return nil
}

func experienceSettings(config string) (PortalExperienceSettings, error) {
	var value models.ChannelConfig
	if err := json.Unmarshal([]byte(config), &value); err != nil {
		return PortalExperienceSettings{}, err
	}
	digest := sha256.Sum256([]byte(config))
	settings := PortalExperienceSettings{Revision: hex.EncodeToString(digest[:]), Presentation: value.PortalPresentation, Modules: value.PortalModules, Navigation: value.PortalNavigation, DefaultModule: value.PortalDefaultModule}
	if settings.Presentation == "" {
		settings.Presentation = "classic"
	}
	return settings, nil
}

func (s *CollectionApplicationService) GetPortalExperienceSettings(ctx context.Context, userID, channelID int) (*PortalExperienceSettings, error) {
	if err := s.requirePortalManager(ctx, userID, channelID); err != nil {
		return nil, err
	}
	raw, err := repository.NewChannelRepository(s.db).GetConfig(ctx, channelID)
	if err != nil {
		return nil, err
	}
	settings, err := experienceSettings(raw)
	return &settings, err
}

func (s *CollectionApplicationService) SavePortalExperienceSettings(ctx context.Context, actor AuditActor, channelID int, input PortalExperienceSettings) (*PortalExperienceSettings, error) {
	if err := s.requirePortalManager(ctx, actor.UserID, channelID); err != nil {
		return nil, err
	}
	if err := validatePortalExperienceSettings(input); err != nil {
		return nil, collectionValidation(err.Error())
	}
	channel, err := repository.NewChannelRepository(s.db).FindByID(ctx, channelID)
	if err != nil {
		return nil, err
	}
	storedConfig, err := repository.NewChannelRepository(s.db).GetConfig(ctx, channelID)
	if err != nil {
		return nil, err
	}
	before, err := experienceSettings(storedConfig)
	if err != nil {
		return nil, err
	}
	if input.Revision != before.Revision {
		return nil, ErrCollectionConflict
	}
	var config map[string]json.RawMessage
	if err := json.Unmarshal([]byte(storedConfig), &config); err != nil {
		return nil, err
	}
	if config == nil {
		config = make(map[string]json.RawMessage)
	}
	var channelConfig models.ChannelConfig
	if err := json.Unmarshal([]byte(storedConfig), &channelConfig); err != nil {
		return nil, err
	}
	if (input.Modules == nil || slices.Contains(input.Modules, "forms")) && len(channelConfig.PortalWorkspaceIDs) == 0 {
		return nil, collectionValidation("configure a target workspace before enabling request forms")
	}
	for key, value := range map[string]any{"portal_presentation": input.Presentation, "portal_modules": input.Modules, "portal_navigation": input.Navigation, "portal_default_module": input.DefaultModule} {
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		config[key] = raw
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	after, err := experienceSettings(string(raw))
	if err != nil {
		return nil, err
	}
	err = database.WithTx(s.db, func(tx database.Tx) error {
		result, err := tx.Exec(`UPDATE channels SET config = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND config = ?`, string(raw), channelID, storedConfig)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return ErrCollectionConflict
		}
		return logger.LogAuditTx(ctx, tx, logger.AuditEvent{UserID: actor.UserID, Username: actor.Username, IPAddress: actor.IPAddress, UserAgent: actor.UserAgent, Source: actor.Source,
			ActionType: logger.ActionChannelExperienceUpdate, ResourceType: logger.ResourceChannel, ResourceID: &channelID, ResourceName: channel.Name, Success: true,
			Details: map[string]any{"before": before, "after": after}})
	})
	if err != nil {
		return nil, err
	}
	return &after, nil
}
