package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"windshift/internal/models"
	"windshift/internal/repository"
	windshiftsmtp "windshift/internal/smtp"
	"windshift/internal/utils"
)

// ChannelConfigError is a domain error returned by the channel-config
// application operation. Its kind keeps transport-specific status mapping out
// of the policy and persistence code.
type ChannelConfigError struct {
	Kind    ChannelConfigErrorKind
	Message string
}

type ChannelConfigErrorKind uint8

const (
	ChannelConfigInvalid ChannelConfigErrorKind = iota + 1
	ChannelConfigForbidden
	ChannelConfigConflict
	ChannelConfigWorkspaceForbidden
)

func (e *ChannelConfigError) Error() string { return e.Message }

func channelConfigInvalid(message string) error {
	return &ChannelConfigError{Kind: ChannelConfigInvalid, Message: message}
}

func channelConfigForbidden(message string) error {
	return &ChannelConfigError{Kind: ChannelConfigForbidden, Message: message}
}

func channelConfigConflict(message string) error {
	return &ChannelConfigError{Kind: ChannelConfigConflict, Message: message}
}

// ChannelConfigUpdateService owns channel configuration merge, validation,
// authorization, and persistence. The HTTP handler only decodes the request,
// supplies the actor, and maps the returned domain error.
type ChannelConfigUpdateService struct {
	channels      *ChannelService
	permission    *PermissionService
	secret        func(string) (string, error)
	validateEmail func(*models.Channel, *models.ChannelConfig) error
	validateURL   func(string) error
	refresh       func()
	// validatePageSource checks that a knowledge-base root page exists,
	// is live, and belongs to the wired workspace. Nil skips the check.
	validatePageSource func(workspaceID, pageID int) error
}

var channelSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$`)

func NewChannelConfigUpdateService(channels *ChannelService, permission *PermissionService) *ChannelConfigUpdateService {
	return &ChannelConfigUpdateService{channels: channels, permission: permission}
}

func (s *ChannelConfigUpdateService) SetSecretEncryptor(encrypt func(string) (string, error)) {
	s.secret = encrypt
}

func (s *ChannelConfigUpdateService) SetEmailConfigValidator(validate func(*models.Channel, *models.ChannelConfig) error) {
	s.validateEmail = validate
}

func (s *ChannelConfigUpdateService) SetURLValidator(validate func(string) error) {
	s.validateURL = validate
}

func (s *ChannelConfigUpdateService) SetSubscriptionInvalidator(invalidate func()) {
	s.refresh = invalidate
}

// SetKnowledgeBasePageValidator wires the page-existence check used when a
// portal wires a sub-page subtree into its knowledge base.
func (s *ChannelConfigUpdateService) SetKnowledgeBasePageValidator(validate func(workspaceID, pageID int) error) {
	s.validatePageSource = validate
}

// Update applies a partial configuration object and returns true only when
// the compare-and-swap write committed. An unchanged false result is a
// concurrent-edit conflict, not a successful no-op.
func (s *ChannelConfigUpdateService) Update(ctx context.Context, actorUserID, channelID int, incoming map[string]any) (bool, error) {
	canManage, err := s.channels.UserCanManage(ctx, actorUserID, channelID)
	if err != nil {
		return false, err
	}
	if !canManage {
		return false, channelConfigForbidden("channel management permission is required")
	}

	channel, err := s.channels.GetByID(ctx, channelID)
	if errors.Is(err, repository.ErrNotFound) || channel == nil {
		return false, repository.ErrNotFound
	}
	if err != nil {
		return false, err
	}
	if channel.PluginName != nil && *channel.PluginName != "" {
		return false, channelConfigForbidden("plugin-managed channels cannot be modified")
	}

	existingJSON, err := s.channels.GetConfig(ctx, channelID)
	if err != nil {
		return false, err
	}
	incomingStored := make(map[string]any, len(incoming))
	for key, value := range incoming {
		incomingStored[key] = value
	}
	if err := s.encryptSecrets(incomingStored); err != nil {
		return false, err
	}
	merged, stored, err := mergeChannelConfig(existingJSON, incomingStored)
	if err != nil {
		return false, err
	}
	normalizeEmailAuthConfig(merged)

	configJSON, err := json.Marshal(merged)
	if err != nil {
		return false, fmt.Errorf("marshal merged channel configuration: %w", err)
	}
	var final models.ChannelConfig
	if err := json.Unmarshal(configJSON, &final); err != nil {
		return false, channelConfigInvalid("Channel config fields have invalid types")
	}

	if err := s.validate(ctx, actorUserID, channel, incoming, stored, &final); err != nil {
		return false, err
	}
	updated, err := s.channels.UpdateConfigIfUnchanged(ctx, channelID, existingJSON, channel.Status, string(configJSON))
	if err != nil {
		if errors.Is(err, repository.ErrChannelSlugConflict) {
			return false, channelConfigConflict("That public channel slug was claimed by another request; choose a different slug")
		}
		return false, err
	}
	if !updated {
		return false, channelConfigConflict("Channel configuration or status changed while it was being saved; reload and try again")
	}
	if s.refresh != nil {
		s.refresh()
	}
	// Provision the portal's system Email request type once it serves a
	// workspace. Best-effort: email intake heals a missing row lazily.
	if channel.Type == "portal" {
		if _, err := EnsureEmailRequestType(s.channels.db, channelID, 0, nil); err != nil {
			slog.Warn("failed to provision portal email request type",
				"channel_id", channelID, "error", err)
		}
		s.markOrphanedPortalIntakes(ctx, channelID, final.PortalWorkspaceIDs)
	}
	return true, nil
}

// markOrphanedPortalIntakes parks intakes whose linked portal no longer serves
// their routing workspace, so the admin sees a needs_attention warning instead
// of a mailbox that keeps polling a target the customer cannot see.
func (s *ChannelConfigUpdateService) markOrphanedPortalIntakes(ctx context.Context, portalChannelID int, servedWorkspaceIDs []int) {
	repo := repository.NewIntakeRepository(s.channels.db)
	intakes, err := repo.ListByPortal(ctx, portalChannelID)
	if err != nil {
		slog.Warn("failed to list portal intakes for workspace check", "channel_id", portalChannelID, "error", err)
		return
	}
	served := make(map[int]struct{}, len(servedWorkspaceIDs))
	for _, id := range servedWorkspaceIDs {
		served[id] = struct{}{}
	}
	reasons := map[int]string{}
	for _, intake := range intakes {
		if intake.Status != models.IntakeStatusEnabled {
			continue
		}
		if _, ok := served[intake.WorkspaceID]; ok {
			continue
		}
		reasons[intake.ID] = fmt.Sprintf("Portal no longer serves workspace %d; re-point this intake and re-enable it", intake.WorkspaceID)
	}
	if err := repo.MarkNeedsAttention(ctx, reasons); err != nil {
		slog.Warn("failed to mark orphaned portal intakes", "channel_id", portalChannelID, "error", err)
	}
}

// PrepareEnable validates that a channel may transition to "enabled" and
// returns the exact stored configuration JSON when the channel type has
// enable-time requirements. The caller must persist the transition through
// SetStatusIfConfigUnchanged with that JSON so a concurrent configuration
// edit cannot bypass validation; an empty result signals a transition that
// needs no configuration-conditional check (disabling, or a type without
// enable-time requirements).
func (s *ChannelConfigUpdateService) PrepareEnable(ctx context.Context, actorUserID, channelID int) (string, error) {
	channel, err := s.channels.GetByID(ctx, channelID)
	if err != nil {
		return "", err
	}
	if channel == nil {
		return "", repository.ErrNotFound
	}
	// Only enabling requires validation; an already-enabled channel is being
	// disabled and needs no configuration checks.
	if channel.Status == "enabled" {
		return "", nil
	}
	if channel.PluginName != nil && *channel.PluginName != "" {
		return "", channelConfigForbidden("plugin-managed channels cannot be modified")
	}

	needsConfig := (channel.Type == "email" && channel.Direction == "inbound") ||
		channel.Type == "portal" || channel.Type == "form" ||
		(channel.Type == "webhook" && channel.Direction == "outbound") ||
		(channel.Type == "smtp" && channel.Direction == "outbound")
	if !needsConfig {
		return "", nil
	}

	rawConfig, err := s.channels.GetConfig(ctx, channelID)
	if err != nil {
		return "", err
	}
	var config models.ChannelConfig
	if rawConfig != "" {
		if err := json.Unmarshal([]byte(rawConfig), &config); err != nil {
			return "", fmt.Errorf("decode stored channel configuration: %w", err)
		}
	}

	admin, err := s.permission.IsSystemAdmin(actorUserID)
	if err != nil {
		return "", err
	}

	switch channel.Type {
	case "email":
		if err := s.prepareEmailEnable(ctx, actorUserID, channel, &config, admin); err != nil {
			return "", err
		}
	case "portal", "form":
		if err := s.preparePublicChannelEnable(ctx, actorUserID, channel, &config, admin); err != nil {
			return "", err
		}
	case "webhook":
		if err := s.prepareWebhookEnable(&config, admin); err != nil {
			return "", err
		}
	case "smtp":
		if err := s.prepareSMTPEnable(&config); err != nil {
			return "", err
		}
	}
	return rawConfig, nil
}

// prepareEmailEnable validates an inbound email channel before activation: the
// mailbox connection must be complete and the channel must have at least one
// intake (folder + target) or legacy routing config. Per-intake target
// validation happens when the intake is created; the legacy config checks are
// kept so channels not yet migrated still enable.
func (s *ChannelConfigUpdateService) prepareEmailEnable(ctx context.Context, actorUserID int, channel *models.Channel, config *models.ChannelConfig, admin bool) error {
	if s.validateEmail == nil {
		return channelConfigInvalid("Email channel validation is not configured")
	}
	if err := s.validateEmail(channel, config); err != nil {
		return channelConfigInvalid(err.Error())
	}

	intakes, err := repository.NewIntakeRepository(s.channels.db).ListEnabledForMailbox(ctx, channel.ID)
	if err != nil {
		return err
	}
	hasLegacyRouting := config.EmailWorkspaceID > 0 || config.EmailConnectedPortalID != nil
	if len(intakes) == 0 && !hasLegacyRouting {
		return channelConfigInvalid("Email channel has no intake: add at least one intake (folder and target) before enabling")
	}

	// Routing lives on intakes once any exist, so authorize the actor against
	// each intake target instead of the stale legacy email_workspace_id field.
	if len(intakes) > 0 {
		for _, intake := range intakes {
			if err := s.validateEmailIntakeEnable(ctx, actorUserID, intake, admin); err != nil {
				return err
			}
		}
		return nil
	}

	if config.EmailWorkspaceID > 0 {
		bad, err := s.channels.repo.FindBadWorkspaceIDs([]int{config.EmailWorkspaceID})
		if err != nil {
			return err
		}
		if len(bad) > 0 {
			return channelConfigInvalid("Configured email workspace is missing or personal")
		}
	}
	if err := s.validateEmailReferences(ctx, actorUserID, config); err != nil {
		return err
	}
	if !admin {
		canConnect, err := s.permission.HasWorkspacePermission(actorUserID, config.EmailWorkspaceID, models.PermissionWorkspaceAdmin)
		if err != nil {
			return err
		}
		if !canConnect {
			return channelConfigForbidden("workspace administration permission is required to connect the email target workspace")
		}
	}
	return nil
}

// validateEmailIntakeEnable checks one enabled intake before the mailbox is
// activated: its workspace item type must still be valid and, for non-admins,
// the actor must manage the routing workspace and any linked portal.
func (s *ChannelConfigUpdateService) validateEmailIntakeEnable(ctx context.Context, actorUserID int, intake models.Intake, admin bool) error {
	if intake.ItemTypeID == nil || *intake.ItemTypeID <= 0 {
		return channelConfigInvalid(fmt.Sprintf("Email intake %d has no item type", intake.ID))
	}
	allowed, err := s.channels.ItemTypeAllowedInWorkspace(intake.WorkspaceID, *intake.ItemTypeID)
	if err != nil {
		return err
	}
	if !allowed {
		return channelConfigInvalid(fmt.Sprintf("Item type %d is not allowed in workspace %d", *intake.ItemTypeID, intake.WorkspaceID))
	}
	if admin {
		return nil
	}
	canAdmin, err := s.permission.HasWorkspacePermission(actorUserID, intake.WorkspaceID, models.PermissionWorkspaceAdmin)
	if err != nil {
		return err
	}
	if !canAdmin {
		return channelConfigForbidden("workspace administration permission is required to connect the email intake target workspace")
	}
	if intake.PortalChannelID != nil {
		canManage, err := s.channels.UserCanManage(ctx, actorUserID, *intake.PortalChannelID)
		if err != nil {
			return err
		}
		if !canManage {
			return channelConfigForbidden("permission to manage the email intake portal is required")
		}
	}
	return nil
}

// preparePublicChannelEnable validates a portal/form channel before
// activation: a routable public slug, existing target workspaces the actor
// may administer, consistent request-type routes, a valid registration mode,
// and an unused slug.
func (s *ChannelConfigUpdateService) preparePublicChannelEnable(ctx context.Context, actorUserID int, channel *models.Channel, config *models.ChannelConfig, admin bool) error {
	slug := config.PortalSlug
	workspaceIDs := config.PortalWorkspaceIDs
	if channel.Type == "form" {
		slug = config.FormSlug
		workspaceIDs = config.FormWorkspaceIDs
	}
	if slug == "" || !channelSlugPattern.MatchString(slug) {
		return channelConfigInvalid("A valid public slug is required before enabling this channel")
	}
	if len(workspaceIDs) == 0 {
		return channelConfigInvalid("At least one target workspace is required before enabling this channel")
	}
	bad, err := s.channels.repo.FindBadWorkspaceIDs(append([]int(nil), workspaceIDs...))
	if err != nil {
		return err
	}
	if len(bad) > 0 {
		return channelConfigInvalid(fmt.Sprintf("Target workspaces %v are missing or personal", bad))
	}
	if !admin {
		for _, workspaceID := range workspaceIDs {
			canConnect, err := s.permission.HasWorkspacePermission(actorUserID, workspaceID, models.PermissionWorkspaceAdmin)
			if err != nil {
				return err
			}
			if !canConnect {
				return channelConfigForbidden("workspace administration permission is required to connect every target workspace")
			}
		}
	}
	if err := s.validateRequestTypeRoutes(channel.ID, channel.Type, config); err != nil {
		return err
	}
	if channel.Type == "portal" && config.PortalRegistrationMode != "" && config.PortalRegistrationMode != "open" && config.PortalRegistrationMode != "manual" {
		return channelConfigInvalid("Portal registration mode must be open or manual")
	}
	inUse, err := s.channels.repo.SlugInUse(ctx, channel.Type, slug, channel.ID)
	if err != nil {
		return err
	}
	if inUse {
		return channelConfigConflict(fmt.Sprintf("Slug %q is already in use by another %s channel", slug, channel.Type))
	}
	return nil
}

// prepareWebhookEnable validates an automatic outbound webhook before
// activation. Automatic triggers serialize item data without an interactive
// authorization context, so only a system administrator may enable them; a
// manual webhook only needs a public destination URL.
func (s *ChannelConfigUpdateService) prepareWebhookEnable(config *models.ChannelConfig, admin bool) error {
	if config.WebhookAutoTrigger && !admin {
		return channelConfigForbidden("system administrator permission is required to enable automatic webhooks")
	}
	if strings.TrimSpace(config.WebhookURL) == "" {
		return channelConfigInvalid("A webhook URL is required before enabling this channel")
	}
	if s.validateURL == nil {
		return channelConfigInvalid("Webhook URL validation is not configured")
	}
	if err := s.validateURL(config.WebhookURL); err != nil {
		return channelConfigInvalid("Webhook URL must target a public host")
	}
	return nil
}

// prepareSMTPEnable validates an outbound SMTP channel before activation:
// host, port, from address, transport mode, and authentication policy.
func (s *ChannelConfigUpdateService) prepareSMTPEnable(config *models.ChannelConfig) error {
	if strings.TrimSpace(config.SMTPHost) == "" || config.SMTPPort <= 0 || config.SMTPPort > 65535 || strings.TrimSpace(config.SMTPFromEmail) == "" {
		return channelConfigInvalid("SMTP host, port, and from address are required before enabling this channel")
	}
	if !validBareEmail(strings.TrimSpace(config.SMTPFromEmail)) {
		return channelConfigInvalid("SMTP from address must be a valid bare email address")
	}
	if err := windshiftsmtp.ValidateTransport(config); err != nil {
		return channelConfigInvalid(err.Error())
	}
	return nil
}

func mergeChannelConfig(existingJSON string, incoming map[string]any) (map[string]any, models.ChannelConfig, error) {
	merged := make(map[string]any)
	var stored models.ChannelConfig
	if existingJSON != "" {
		if err := json.Unmarshal([]byte(existingJSON), &merged); err != nil {
			return nil, stored, channelConfigConflict("Stored channel configuration is invalid; repair it before applying a partial update")
		}
		if merged == nil {
			return nil, stored, channelConfigConflict("Stored channel configuration is not a JSON object; repair it before applying a partial update")
		}
		if err := json.Unmarshal([]byte(existingJSON), &stored); err != nil {
			return nil, stored, channelConfigConflict("Stored channel configuration has invalid field types; repair it before applying a partial update")
		}
	}
	previousProvider, _ := merged["email_oauth_provider_type"].(string)
	previousClientID, _ := merged["email_oauth_client_id"].(string)
	previousTenantID, _ := merged["email_oauth_tenant_id"].(string)
	for key, value := range incoming {
		merged[key] = value
	}
	currentProvider, _ := merged["email_oauth_provider_type"].(string)
	currentClientID, _ := merged["email_oauth_client_id"].(string)
	currentTenantID, _ := merged["email_oauth_tenant_id"].(string)
	if previousProvider != currentProvider || previousClientID != currentClientID || previousTenantID != currentTenantID {
		for _, key := range []string{"email_oauth_access_token", "email_oauth_refresh_token", "email_oauth_expires_at", "email_oauth_email"} {
			delete(merged, key)
		}
		if previousProvider != currentProvider || previousClientID != currentClientID {
			if _, supplied := incoming["email_oauth_client_secret"]; !supplied {
				delete(merged, "email_oauth_client_secret")
			}
		}
	}
	return merged, stored, nil
}

func (s *ChannelConfigUpdateService) encryptSecrets(config map[string]any) error {
	for _, key := range []string{"smtp_password", "imap_password", "webhook_secret", "email_oauth_client_secret"} {
		value, ok := config[key]
		if !ok {
			continue
		}
		secret, ok := value.(string)
		if !ok || secret == "" {
			continue
		}
		if s.secret == nil {
			return fmt.Errorf("encrypt %s: secret encryption is not configured", key)
		}
		ciphertext, err := s.secret(secret)
		if err != nil {
			return fmt.Errorf("encrypt %s: %w", key, err)
		}
		config[key] = ciphertext
	}
	return nil
}

func (s *ChannelConfigUpdateService) validate(ctx context.Context, actorID int, channel *models.Channel, incoming map[string]any, stored models.ChannelConfig, config *models.ChannelConfig) error {
	if channel.Type == "webhook" && channel.Direction == "outbound" {
		if config.WebhookAutoTrigger {
			admin, err := s.permission.IsSystemAdmin(actorID)
			if err != nil {
				return err
			}
			if !admin {
				return channelConfigForbidden("system administrator permission is required to enable automatic webhooks")
			}
		}
		if err := validateWebhookConfig(config); err != nil {
			return channelConfigInvalid(err.Error())
		}
	}
	if channel.Type == "portal" {
		if err := ValidatePortalConfig(config); err != nil {
			return channelConfigInvalid(err.Error())
		}
		if err := s.validateKnowledgeBasePageSources(actorID, &stored, config); err != nil {
			return err
		}
	} else if _, present := incoming["knowledge_base_page_sources"]; present {
		return channelConfigInvalid(fmt.Sprintf("knowledge_base_page_sources is not valid for a %s channel", channel.Type))
	}
	if err := validateChannelTargetField(channel.Type, incoming); err != nil {
		return err
	}
	if err := s.validateTargetWorkspaces(actorID, channel.Type, stored, config); err != nil {
		return err
	}
	if channel.Type == "email" {
		if err := s.validateEmailReferences(ctx, actorID, config); err != nil {
			return err
		}
	}
	if channel.Type == "portal" || channel.Type == "form" {
		if err := s.validatePublicChannel(ctx, channel, config); err != nil {
			return err
		}
	}
	if err := validateGeneralChannelURLs(config, s.validateURL); err != nil {
		return err
	}
	if err := s.validateRequestTypeRoutes(channel.ID, channel.Type, config); err != nil {
		return err
	}
	if channel.Status == "enabled" {
		if err := validateEnabledChannel(channel, config, s.validateEmail); err != nil {
			return err
		}
	}
	return nil
}

// knowledgeBaseSourceKey identifies a KB page source independent of ordering.
func knowledgeBaseSourceKey(src models.KnowledgeBasePageSource) string {
	root := 0
	if src.RootPageID != nil {
		root = *src.RootPageID
	}
	return fmt.Sprintf("%d:%d", src.WorkspaceID, root)
}

// validateKnowledgeBasePageSources checks a portal's workspace-pages
// knowledge-base wiring: bounded count, target workspaces only, live root
// pages, and workspace-administered disclosure for non-admin actors. Sources
// that already exist unchanged are not re-authorized, so editing unrelated
// portal settings cannot be blocked by a source the actor cannot administer.
func (s *ChannelConfigUpdateService) validateKnowledgeBasePageSources(actorID int, stored, config *models.ChannelConfig) error {
	sources := config.KnowledgeBasePageSources
	if len(sources) == 0 {
		return nil
	}
	if len(sources) > maxKnowledgeBasePageSources {
		return channelConfigInvalid(fmt.Sprintf("knowledge_base_page_sources accepts at most %d entries", maxKnowledgeBasePageSources))
	}
	admin, err := s.permission.IsSystemAdmin(actorID)
	if err != nil {
		return err
	}
	unchanged := make(map[string]bool, len(stored.KnowledgeBasePageSources))
	for _, src := range stored.KnowledgeBasePageSources {
		unchanged[knowledgeBaseSourceKey(src)] = true
	}
	seen := make(map[int]map[int]bool, len(sources))
	for _, src := range sources {
		if src.WorkspaceID <= 0 {
			return channelConfigInvalid("knowledge_base_page_sources entries need a workspace_id")
		}
		if !slices.Contains(config.PortalWorkspaceIDs, src.WorkspaceID) {
			return channelConfigInvalid(fmt.Sprintf("workspace %d must be connected to the portal before its pages can join the knowledge base", src.WorkspaceID))
		}
		if seen[src.WorkspaceID] == nil {
			seen[src.WorkspaceID] = map[int]bool{}
		}
		rootKey := 0
		if src.RootPageID != nil {
			rootKey = *src.RootPageID
			if err := s.validatePageSource(src.WorkspaceID, rootKey); err != nil {
				return channelConfigInvalid(fmt.Sprintf("knowledge base page source is invalid: %s", err.Error()))
			}
		}
		if seen[src.WorkspaceID][rootKey] {
			return channelConfigInvalid(fmt.Sprintf("workspace %d is wired into the knowledge base more than once", src.WorkspaceID))
		}
		seen[src.WorkspaceID][rootKey] = true
		if admin || unchanged[knowledgeBaseSourceKey(src)] {
			continue
		}
		allowed, err := s.permission.HasWorkspacePermission(actorID, src.WorkspaceID, models.PermissionWorkspaceAdmin)
		if err != nil {
			return err
		}
		if !allowed {
			return &ChannelConfigError{
				Kind:    ChannelConfigWorkspaceForbidden,
				Message: fmt.Sprintf("Workspace administration permission is required to publish workspace %d pages through the knowledge base", src.WorkspaceID),
			}
		}
	}
	return nil
}

func validateWebhookConfig(config *models.ChannelConfig) error {
	if config.WebhookScopeType != "" && config.WebhookScopeType != "all" && config.WebhookScopeType != "workspaces" {
		return fmt.Errorf("webhook scope must be all or workspaces")
	}
	for _, event := range config.WebhookSubscribedEvents {
		switch event {
		case "item.created", "item.updated", "item.deleted", "item.assigned", "status.changed":
		default:
			return fmt.Errorf("unsupported automatic webhook event %q", event)
		}
	}
	return nil
}

// ValidatePortalConfig validates public portal customization and registration
// settings independently from the HTTP transport.
func ValidatePortalConfig(config *models.ChannelConfig) error {
	switch config.PortalRegistrationMode {
	case "", "open", "manual":
	default:
		return fmt.Errorf("portal registration mode must be open or manual")
	}
	for field, value := range map[string]string{
		"portal background image URL": config.PortalBackgroundImageURL,
		"portal logo URL":             config.PortalLogoURL,
	} {
		if err := utils.ValidateBrowserAssetURL(value); err != nil {
			return fmt.Errorf("%s is invalid: %w", field, err)
		}
	}
	for _, column := range config.PortalFooterColumns {
		for _, link := range column.Links {
			if err := utils.ValidateBrowserNavigationURL(link.URL); err != nil {
				return fmt.Errorf("portal footer link URL is invalid: %w", err)
			}
		}
	}
	if config.KnowledgeBaseShareLink != "" {
		if err := utils.ValidateClientRedirectURL(config.KnowledgeBaseShareLink); err != nil {
			return fmt.Errorf("knowledge base share link is invalid: %w", err)
		}
		shareURL, err := url.Parse(config.KnowledgeBaseShareLink)
		if err != nil || !strings.EqualFold(shareURL.Scheme, "https") || shareURL.User != nil {
			return fmt.Errorf("knowledge base share link must be an unambiguous HTTPS URL")
		}
	}
	return nil
}

func validateChannelTargetField(channelType string, incoming map[string]any) error {
	allowed := map[string]string{"portal": "portal_workspace_ids", "form": "form_workspace_ids", "email": "email_workspace_id"}[channelType]
	for _, field := range []string{"portal_workspace_ids", "form_workspace_ids", "email_workspace_id"} {
		if _, present := incoming[field]; present && field != allowed {
			return channelConfigInvalid(fmt.Sprintf("%s is not valid for a %s channel", field, channelType))
		}
	}
	return nil
}

func (s *ChannelConfigUpdateService) validateTargetWorkspaces(actorID int, channelType string, stored models.ChannelConfig, config *models.ChannelConfig) error {
	var targets, previous []int
	switch channelType {
	case "portal":
		targets, previous = config.PortalWorkspaceIDs, stored.PortalWorkspaceIDs
	case "form":
		targets, previous = config.FormWorkspaceIDs, stored.FormWorkspaceIDs
	case "email":
		if config.EmailWorkspaceID > 0 {
			targets = []int{config.EmailWorkspaceID}
		}
		if stored.EmailWorkspaceID > 0 {
			previous = []int{stored.EmailWorkspaceID}
		}
	default:
		return nil
	}
	if len(targets) == 0 {
		return nil
	}
	bad, err := s.channels.repo.FindBadWorkspaceIDs(append([]int(nil), targets...))
	if err != nil {
		return err
	}
	if len(bad) > 0 {
		return channelConfigInvalid(fmt.Sprintf("Workspace IDs %v are missing or personal and cannot be used as channel targets", bad))
	}
	admin, err := s.permission.IsSystemAdmin(actorID)
	if err != nil {
		return err
	}
	if admin {
		return nil
	}
	alreadyConnected := make(map[int]struct{}, len(previous))
	for _, id := range previous {
		alreadyConnected[id] = struct{}{}
	}
	seen := make(map[int]struct{}, len(targets))
	for _, id := range targets {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		if _, ok := alreadyConnected[id]; ok {
			continue
		}
		allowed, err := s.permission.HasWorkspacePermission(actorID, id, models.PermissionWorkspaceAdmin)
		if err != nil {
			return err
		}
		if !allowed {
			return &ChannelConfigError{
				Kind:    ChannelConfigWorkspaceForbidden,
				Message: fmt.Sprintf("Workspace administration permission is required to connect workspace %d", id),
			}
		}
	}
	return nil
}

func (s *ChannelConfigUpdateService) validateEmailReferences(ctx context.Context, actorID int, config *models.ChannelConfig) error {
	if config.EmailWorkspaceID > 0 && config.EmailItemTypeID != nil && *config.EmailItemTypeID > 0 {
		allowed, err := s.channels.ItemTypeAllowedInWorkspace(config.EmailWorkspaceID, *config.EmailItemTypeID)
		if err != nil {
			return err
		}
		if !allowed {
			return channelConfigInvalid(fmt.Sprintf("Item type %d is not allowed in workspace %d", *config.EmailItemTypeID, config.EmailWorkspaceID))
		}
	}
	if config.EmailDefaultPriorityID != nil {
		if *config.EmailDefaultPriorityID <= 0 {
			return channelConfigInvalid("Email default priority must be a positive ID")
		}
		if config.EmailWorkspaceID > 0 {
			allowed, err := s.channels.PriorityAllowedInWorkspace(config.EmailWorkspaceID, *config.EmailDefaultPriorityID)
			if err != nil {
				return err
			}
			if !allowed {
				return channelConfigInvalid(fmt.Sprintf("Priority %d is not allowed in workspace %d", *config.EmailDefaultPriorityID, config.EmailWorkspaceID))
			}
		}
	}
	if config.EmailConnectedPortalID != nil {
		portal, err := s.channels.GetByID(ctx, *config.EmailConnectedPortalID)
		if err != nil {
			return err
		}
		if portal == nil || portal.Type != "portal" || portal.Direction != "inbound" {
			return channelConfigInvalid("Connected portal must reference an inbound portal channel")
		}
		canManage, err := s.channels.UserCanManage(ctx, actorID, portal.ID)
		if err != nil {
			return err
		}
		if !canManage {
			return channelConfigForbidden("permission to manage the connected portal is required")
		}
	}
	return nil
}

func (s *ChannelConfigUpdateService) validatePublicChannel(ctx context.Context, channel *models.Channel, config *models.ChannelConfig) error {
	slug := config.PortalSlug
	if channel.Type == "form" {
		slug = config.FormSlug
	}
	if slug == "" {
		return nil
	}
	if !channelSlugPattern.MatchString(slug) {
		return channelConfigInvalid(fmt.Sprintf("%s must be 3-64 chars: lowercase letters, digits, or hyphens (no leading/trailing hyphen)", channel.Type+"_slug"))
	}
	inUse, err := s.channels.repo.SlugInUse(ctx, channel.Type, slug, channel.ID)
	if err != nil {
		return err
	}
	if inUse {
		return channelConfigConflict(fmt.Sprintf("%s_slug %q is already in use by another %s channel", channel.Type, slug, channel.Type))
	}
	return nil
}

func validateGeneralChannelURLs(config *models.ChannelConfig, validateWebhookURL func(string) error) error {
	if config.KnowledgeBaseURL != "" {
		if err := utils.ValidateExternalURL(config.KnowledgeBaseURL); err != nil {
			return channelConfigInvalid("Knowledge base URL must be a valid public HTTPS URL")
		}
	}
	if config.WebhookURL != "" {
		if validateWebhookURL == nil {
			return channelConfigInvalid("Webhook URL validation is not configured")
		}
		if err := validateWebhookURL(config.WebhookURL); err != nil {
			return channelConfigInvalid("Webhook URL must target a public host")
		}
	}
	if config.FormRedirectURL != "" {
		if err := utils.ValidateClientRedirectURL(config.FormRedirectURL); err != nil {
			return channelConfigInvalid("Form redirect URL must be an http(s) URL")
		}
	}
	if config.FormLogoURL != "" {
		if err := utils.ValidateClientRedirectURL(config.FormLogoURL); err != nil {
			return channelConfigInvalid("Form logo URL must be an http(s) URL")
		}
	}
	return nil
}

func (s *ChannelConfigUpdateService) validateRequestTypeRoutes(channelID int, channelType string, config *models.ChannelConfig) error {
	if channelType != "portal" && channelType != "form" {
		return nil
	}
	served := config.PortalWorkspaceIDs
	if channelType == "form" {
		served = config.FormWorkspaceIDs
	}
	routes, err := s.channels.repo.ListRequestTypeRoutes(channelID)
	if err != nil {
		return err
	}
	invalid := make([]string, 0)
	for _, route := range routes {
		if route.WorkspaceID == nil || !containsChannelConfigID(served, *route.WorkspaceID) {
			invalid = append(invalid, route.Name)
			continue
		}
		allowed, err := s.channels.ItemTypeAllowedInWorkspace(*route.WorkspaceID, route.ItemTypeID)
		if err != nil {
			return err
		}
		if !allowed {
			invalid = append(invalid, route.Name)
		}
	}
	if len(invalid) > 0 {
		return channelConfigInvalid(fmt.Sprintf("Request types are pinned to workspaces this channel does not serve: %s. Delete them before removing the workspace.", strings.Join(invalid, ", ")))
	}
	return nil
}

func validateEnabledChannel(channel *models.Channel, config *models.ChannelConfig, validateEmail func(*models.Channel, *models.ChannelConfig) error) error {
	switch channel.Type {
	case "email":
		if validateEmail == nil {
			return channelConfigInvalid("Email channel validation is not configured")
		}
		if err := validateEmail(channel, config); err != nil {
			return channelConfigInvalid(err.Error())
		}
	case "portal", "form":
		slug, workspaces := config.PortalSlug, config.PortalWorkspaceIDs
		if channel.Type == "form" {
			slug, workspaces = config.FormSlug, config.FormWorkspaceIDs
		}
		if !channelSlugPattern.MatchString(slug) || len(workspaces) == 0 {
			return channelConfigInvalid("Enabled public channels require a valid slug and at least one target workspace")
		}
	case "webhook":
		if strings.TrimSpace(config.WebhookURL) == "" {
			return channelConfigInvalid("Enabled webhooks require a destination URL")
		}
	case "smtp":
		from := strings.TrimSpace(config.SMTPFromEmail)
		if strings.TrimSpace(config.SMTPHost) == "" || config.SMTPPort <= 0 || config.SMTPPort > 65535 || from == "" || !validBareEmail(from) {
			return channelConfigInvalid("Enabled SMTP channels require a valid host, port, from address, and transport mode")
		}
		if err := windshiftsmtp.ValidateTransport(config); err != nil {
			return channelConfigInvalid(err.Error())
		}
	}
	return nil
}

func validBareEmail(value string) bool {
	parsed, err := mail.ParseAddress(value)
	return err == nil && parsed.Address == value
}

func containsChannelConfigID(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func normalizeEmailAuthConfig(config map[string]any) {
	method, _ := config["email_auth_method"].(string)
	switch strings.ToLower(method) {
	case "basic":
		for _, key := range []string{"email_oauth_provider_type", "email_oauth_client_id", "email_oauth_client_secret", "email_oauth_tenant_id", "email_oauth_access_token", "email_oauth_refresh_token", "email_oauth_expires_at", "email_oauth_email"} {
			delete(config, key)
		}
	case "oauth":
		for _, key := range []string{"imap_host", "imap_port", "imap_username", "imap_password", "imap_encryption"} {
			delete(config, key)
		}
	}
}
