package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"windshift/internal/jira"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"

	"uuid"
)

const jiraRequestTypeFieldType = "com.atlassian.servicedesk:vp-origin"

var jiraPortalSlugUnsafe = regexp.MustCompile(`[^a-z0-9]+`)

type jiraServiceManagementImport struct {
	ServiceDeskID         string
	ChannelID             int
	RequestTypes          map[string]int
	OrganizationCustomers []JiraUserSummary
	CustomerOrganizations map[string]int
}

type jiraCustomerOrganizationClient interface {
	ListServiceDeskOrganizations(ctx context.Context, serviceDeskID string) ([]jira.JiraServiceDeskOrganization, error)
	ListServiceDeskOrganizationUsers(ctx context.Context, organizationID string) ([]jira.JiraUser, error)
}

func (h *JiraImportHandler) prepareJiraServiceManagementImport(
	ctx context.Context,
	jobID, projectKey string,
	workspaceID int,
	itemTypeMap map[string]int,
	client jira.Client,
	createdByUserID int,
	importOrganizations bool,
) (*jiraServiceManagementImport, error) {
	project, err := client.GetProject(ctx, projectKey)
	if err != nil {
		return nil, fmt.Errorf("load Jira project: %w", err)
	}
	if project == nil || !strings.EqualFold(project.ProjectType, "service_desk") {
		return nil, nil
	}

	serviceDesks, err := client.ListServiceDesks(ctx)
	if err != nil {
		return nil, fmt.Errorf("list Jira service desks: %w", err)
	}
	var serviceDesk *jira.JiraServiceDesk
	for i := range serviceDesks {
		if serviceDesks[i].ProjectID == project.ID || strings.EqualFold(serviceDesks[i].ProjectKey, project.Key) {
			serviceDesk = &serviceDesks[i]
			break
		}
	}
	if serviceDesk == nil {
		return nil, fmt.Errorf("no Jira service desk found for project %s", projectKey)
	}

	channelID, err := h.ensureJiraPortal(ctx, jobID, *project, *serviceDesk, workspaceID, createdByUserID)
	if err != nil {
		return nil, err
	}
	requestTypes, err := client.ListServiceDeskRequestTypes(ctx, serviceDesk.ID)
	if err != nil {
		return nil, fmt.Errorf("list Jira service desk request types: %w", err)
	}
	requestTypeMap, err := h.ensureJiraRequestTypes(jobID, channelID, workspaceID, requestTypes, itemTypeMap)
	if err != nil {
		return nil, err
	}
	if err := h.ensureJiraPortalRequestTypeSection(channelID, requestTypeMap); err != nil {
		return nil, err
	}

	organizationCustomers, customerOrganizations, err := h.ensureJiraCustomerOrganizations(
		ctx, jobID, project.Key, serviceDesk.ID, client, importOrganizations,
	)
	if err != nil {
		// Organization metadata is optional enrichment. A JSM version that
		// rejects the experimental read must not abort the whole project's
		// core issue import, so degrade to no organization customers.
		slog.Warn("Jira customer organization metadata unavailable; continuing without it",
			slog.String("component", "jira"),
			slog.String("project", projectKey),
			slog.Any("error", err))
		organizationCustomers, customerOrganizations = nil, nil
	}

	return &jiraServiceManagementImport{
		ServiceDeskID:         serviceDesk.ID,
		ChannelID:             channelID,
		RequestTypes:          requestTypeMap,
		OrganizationCustomers: organizationCustomers,
		CustomerOrganizations: customerOrganizations,
	}, nil
}

func (h *JiraImportHandler) ensureJiraPortal(
	ctx context.Context,
	jobID string,
	project jira.JiraProject,
	serviceDesk jira.JiraServiceDesk,
	workspaceID, createdByUserID int,
) (int, error) {
	if mappedID, ok := h.imports.MappedEntity(jobID, "portal", serviceDesk.ID); ok {
		return mappedID, nil
	}

	baseSlug := "jira-" + strings.Trim(jiraPortalSlugUnsafe.ReplaceAllString(strings.ToLower(project.Key), "-"), "-")
	if baseSlug == "jira-" {
		baseSlug = "jira-service-desk"
	}
	slug := baseSlug
	for suffix := 2; ; suffix++ {
		existing, err := h.imports.PortalBySlug(ctx, slug)
		if errors.Is(err, repository.ErrNotFound) {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("find Jira portal slug: %w", err)
		}
		var config models.ChannelConfig
		if json.Unmarshal([]byte(existing.Config), &config) == nil && jiraPortalContainsWorkspace(config.PortalWorkspaceIDs, workspaceID) {
			if err := h.recordMapping(jobID, "portal", serviceDesk.ID, project.Key, existing.ID, map[string]any{
				"was_created": false,
				"project_id":  project.ID,
			}); err != nil {
				return 0, fmt.Errorf("record Jira portal mapping: %w", err)
			}
			return existing.ID, nil
		}
		slug = baseSlug + "-" + strconv.Itoa(suffix)
	}

	portalTitle := strings.TrimSpace(project.Name)
	if portalTitle == "" {
		portalTitle = project.Key
	}
	configBytes, err := json.Marshal(models.ChannelConfig{
		PortalSlug:             slug,
		PortalWorkspaceIDs:     []int{workspaceID},
		PortalTitle:            portalTitle,
		PortalDescription:      "Imported from Jira Service Management",
		PortalGradient:         1,
		PortalRegistrationMode: "manual",
	})
	if err != nil {
		return 0, fmt.Errorf("marshal Jira portal config: %w", err)
	}

	channel := &models.Channel{
		Name:        portalTitle,
		Type:        "portal",
		Direction:   "inbound",
		Description: "Imported from Jira Service Management",
		Status:      "enabled",
		Config:      string(configBytes),
	}
	channelID, err := h.imports.CreatePortal(ctx, channel, createdByUserID)
	if err != nil {
		return 0, fmt.Errorf("create Jira portal: %w", err)
	}
	if err := h.recordMapping(jobID, "portal", serviceDesk.ID, project.Key, channelID, map[string]any{
		"was_created": true,
		"project_id":  project.ID,
	}); err != nil {
		return 0, fmt.Errorf("record Jira portal mapping: %w", err)
	}
	return channelID, nil
}

func (h *JiraImportHandler) ensureJiraRequestTypes(
	jobID string,
	channelID, workspaceID int,
	requestTypes []jira.JiraServiceDeskRequestType,
	itemTypeMap map[string]int,
) (map[string]int, error) {
	result := make(map[string]int, len(requestTypes))
	for order, requestType := range requestTypes {
		itemTypeID, ok := itemTypeMap[requestType.IssueTypeID]
		if !ok {
			continue
		}

		restriction := classifyJiraRequestTypeRestriction(requestType.RestrictionStatus)
		description := strings.TrimSpace(requestType.Description)
		if description == "" {
			description = strings.TrimSpace(requestType.HelpText)
		}
		id, created, err := h.imports.EnsureRequestType(&models.RequestType{
			ChannelID:    channelID,
			Name:         sanitize.PlainTextField.Sanitize(requestType.Name),
			Description:  sanitize.PlainTextField.Sanitize(description),
			ItemTypeID:   itemTypeID,
			Icon:         "LifeBuoy",
			Color:        "#0ea5e9",
			DisplayOrder: order + 1,
			IsActive:     !restriction.failClosed(),
			WorkspaceID:  &workspaceID,
		}, models.RequestTypeConfig{
			RequireAuth:      true,
			AllowAttachments: true,
		})
		if err != nil {
			return nil, fmt.Errorf("ensure Jira request type %s: %w", requestType.ID, err)
		}
		// Reused rows keep an administrator-configured audience; otherwise a
		// restricted or unknown source type is disabled again on reimport.
		disposition, err := h.imports.ReconcileImportedRequestTypeRestriction(id, channelID, restriction.failClosed())
		if err != nil {
			return nil, fmt.Errorf("reconcile Jira request type %s restriction: %w", requestType.ID, err)
		}
		result[requestType.ID] = id
		if err := h.recordMapping(jobID, "request_type", requestType.ID, requestType.Name, id, map[string]any{
			"was_created":             created,
			"service_desk_id":         requestType.ServiceDeskID,
			"jira_issue_type_id":      requestType.IssueTypeID,
			"jira_group_ids":          requestType.GroupIDs,
			"restriction_status":      restriction.SourceStatus,
			"restriction_disposition": disposition,
		}); err != nil {
			return nil, fmt.Errorf("record Jira request type mapping: %w", err)
		}
	}
	return result, nil
}

// jiraRequestTypeRestriction is the fail-closed reading of a Jira
// RequestTypeDTO.restrictionStatus. Jira documents OPEN and RESTRICTED; every
// other value (including empty) is treated as unknown so a missing status
// cannot silently grant access.
type jiraRequestTypeRestriction struct {
	SourceStatus string
	Disposition  string
}

const (
	jiraRequestTypeStatusOpen       = "OPEN"
	jiraRequestTypeStatusRestricted = "RESTRICTED"
	jiraRequestTypeDispositionOpen  = "open"
)

func classifyJiraRequestTypeRestriction(status string) jiraRequestTypeRestriction {
	normalized := strings.ToUpper(strings.TrimSpace(status))
	switch normalized {
	case jiraRequestTypeStatusOpen:
		return jiraRequestTypeRestriction{SourceStatus: normalized, Disposition: jiraRequestTypeDispositionOpen}
	case jiraRequestTypeStatusRestricted:
		return jiraRequestTypeRestriction{SourceStatus: normalized, Disposition: "restricted"}
	default:
		return jiraRequestTypeRestriction{SourceStatus: normalized, Disposition: "unknown"}
	}
}

func (r jiraRequestTypeRestriction) failClosed() bool {
	return r.Disposition != jiraRequestTypeDispositionOpen
}

func (h *JiraImportHandler) ensureJiraPortalRequestTypeSection(channelID int, requestTypes map[string]int) error {
	requestTypeIDs := make([]int, 0, len(requestTypes))
	for _, id := range requestTypes {
		requestTypeIDs = append(requestTypeIDs, id)
	}
	sort.Ints(requestTypeIDs)
	if err := h.imports.AddPortalRequestTypeSection(context.Background(), channelID, requestTypeIDs, uuid.New().String()); err != nil {
		return fmt.Errorf("update Jira portal request type section: %w", err)
	}
	return nil
}

func jiraIsPortalCustomer(accountID, accountType string) bool {
	return strings.EqualFold(strings.TrimSpace(accountType), "customer") ||
		strings.HasPrefix(strings.ToLower(strings.TrimSpace(accountID)), "qm:")
}

func jiraUserIdentityMetadata(user *jira.JiraUser) map[string]any {
	if user == nil {
		return nil
	}
	identity := map[string]any{}
	if accountID := strings.TrimSpace(user.GetIdentifier()); accountID != "" {
		identity["account_id"] = accountID
	}
	if accountType := strings.TrimSpace(user.AccountType); accountType != "" {
		identity["account_type"] = accountType
	}
	if displayName := strings.TrimSpace(user.DisplayName); displayName != "" {
		identity["display_name"] = displayName
	}
	if email := strings.TrimSpace(user.EmailAddress); email != "" {
		identity["email"] = email
	}
	if len(identity) == 0 {
		return nil
	}
	return identity
}

func splitJiraImportUsers(users []JiraUserSummary) (internalUsers, portalCustomers []JiraUserSummary) {
	for _, user := range users {
		if jiraIsPortalCustomer(user.AccountID, user.AccountType) {
			portalCustomers = append(portalCustomers, user)
		} else {
			internalUsers = append(internalUsers, user)
		}
	}
	return internalUsers, portalCustomers
}

func (h *JiraImportHandler) ensurePortalCustomers(
	jobID string,
	channelID int,
	customers []JiraUserSummary,
	customerOrganizations map[string]int,
) (map[string]int, error) {
	result := make(map[string]int, len(customers))
	for _, customer := range customers {
		if customer.AccountID == "" {
			continue
		}
		customerID, mappingExists := h.imports.MappedEntity(jobID, "portal_customer", customer.AccountID)

		wasCreated := false
		organizationID := customerOrganizations[customer.AccountID]
		var previousOrganizationID *int
		if !mappingExists {
			email := strings.TrimSpace(customer.Email)
			if email == "" {
				email = jiraPortalCustomerSyntheticEmail(customer.AccountID)
			}
			name := sanitize.PlainTextField.Sanitize(strings.TrimSpace(customer.DisplayName))
			if name == "" {
				name = "Imported Jira Customer"
			}
			imported, err := h.imports.EnsurePortalCustomer(name, email, organizationID)
			if err != nil {
				return nil, fmt.Errorf("ensure Jira portal customer: %w", err)
			}
			customerID = imported.ID
			previousOrganizationID = imported.OrganisationID
			wasCreated = imported.Created
		}

		if err := h.ensureJiraPortalCustomerAccess(jobID, customer.AccountID, customerID, channelID); err != nil {
			return nil, err
		}

		result[customer.AccountID] = customerID
		if !mappingExists {
			previousID := 0
			if previousOrganizationID != nil {
				previousID = *previousOrganizationID
			}
			if err := h.recordMapping(jobID, "portal_customer", customer.AccountID, "", customerID, map[string]any{
				"was_created":                       wasCreated,
				"customer_organisation_id":          organizationID,
				"organization_was_assigned":         organizationID > 0 && previousOrganizationID == nil,
				"previous_customer_organisation_id": previousID,
			}); err != nil {
				return nil, fmt.Errorf("record Jira portal customer mapping: %w", err)
			}
		}
	}
	return result, nil
}

func (h *JiraImportHandler) ensureJiraPortalCustomerAccess(
	jobID, accountID string,
	customerID, channelID int,
) error {
	channelAccessCreated, err := h.imports.EnsurePortalCustomerChannel(customerID, channelID)
	if err != nil {
		return fmt.Errorf("grant Jira portal customer channel access: %w", err)
	}
	if err := h.recordMapping(jobID, "portal_customer_channel", accountID+":"+strconv.Itoa(channelID), "", customerID, map[string]any{
		"was_created": channelAccessCreated,
		"channel_id":  channelID,
	}); err != nil {
		return fmt.Errorf("record Jira portal customer channel mapping: %w", err)
	}

	roleID, roleCreated, err := h.imports.EnsurePortalCustomerRole(customerID)
	if err != nil {
		return fmt.Errorf("assign Jira portal customer role: %w", err)
	}
	if err := h.recordMapping(jobID, "portal_customer_role", accountID+":"+strconv.Itoa(roleID), "", customerID, map[string]any{
		"was_created":     roleCreated,
		"contact_role_id": roleID,
	}); err != nil {
		return fmt.Errorf("record Jira portal customer role mapping: %w", err)
	}
	return nil
}

func (h *JiraImportHandler) ensureJiraCustomerOrganizations(
	ctx context.Context,
	jobID, projectKey, serviceDeskID string,
	client jiraCustomerOrganizationClient,
	importOrganizations bool,
) ([]JiraUserSummary, map[string]int, error) {
	organizations, err := client.ListServiceDeskOrganizations(ctx, serviceDeskID)
	if err != nil {
		return nil, nil, fmt.Errorf("list Jira customer organizations: %w", err)
	}
	sort.SliceStable(organizations, func(i, j int) bool {
		if strings.EqualFold(organizations[i].Name, organizations[j].Name) {
			return organizations[i].ID < organizations[j].ID
		}
		return strings.ToLower(organizations[i].Name) < strings.ToLower(organizations[j].Name)
	})

	customersByAccountID := make(map[string]JiraUserSummary)
	customerOrganizations := make(map[string]int)
	for _, organization := range organizations {
		var organizationID int
		if importOrganizations {
			mappedID, mapped := h.imports.MappedEntity(jobID, "customer_organisation", organization.ID)
			if mapped {
				organizationID = mappedID
			} else {
				var mappingErr error
				var wasCreated bool
				organizationID, wasCreated, mappingErr = h.imports.EnsureCustomerOrganisation(
					sanitize.PlainTextField.Sanitize(organization.Name),
					"Imported from Jira Service Management",
				)
				if mappingErr == nil {
					mappingErr = h.recordMapping(jobID, "customer_organisation", organization.ID, projectKey, organizationID, map[string]any{
						"was_created":     wasCreated,
						"service_desk_id": serviceDeskID,
						"jira_uuid":       organization.UUID,
						"scim_managed":    organization.SCIMManaged,
					})
				}
				if mappingErr != nil {
					return nil, nil, fmt.Errorf("ensure Jira customer organization %s: %w", organization.ID, mappingErr)
				}
			}
		}

		users, usersErr := client.ListServiceDeskOrganizationUsers(ctx, organization.ID)
		if usersErr != nil {
			return nil, nil, fmt.Errorf("list customers for Jira organization %s: %w", organization.ID, usersErr)
		}
		for _, user := range users {
			accountID := user.GetIdentifier()
			if accountID == "" {
				continue
			}
			if organizationID > 0 {
				if _, assigned := customerOrganizations[accountID]; !assigned {
					customerOrganizations[accountID] = organizationID
				}
			}
			if _, seen := customersByAccountID[accountID]; !seen {
				avatarURL := ""
				if user.AvatarURLs != nil {
					avatarURL = user.AvatarURLs["48x48"]
				}
				customersByAccountID[accountID] = JiraUserSummary{
					AccountID:   accountID,
					AccountType: user.AccountType,
					Email:       user.EmailAddress,
					DisplayName: user.DisplayName,
					AvatarURL:   avatarURL,
				}
			}
		}
	}

	customers := make([]JiraUserSummary, 0, len(customersByAccountID))
	for _, customer := range customersByAccountID {
		customers = append(customers, customer)
	}
	sort.SliceStable(customers, func(i, j int) bool {
		return customers[i].AccountID < customers[j].AccountID
	})
	return customers, customerOrganizations, nil
}

func jiraPortalCustomerSyntheticEmail(accountID string) string {
	safe := strings.ReplaceAll(strings.TrimSpace(accountID), ":", "-")
	if safe == "" {
		safe = strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return safe + "@jira-customer.invalid"
}

func jiraRequestTypeID(issue *jira.JiraIssue, mappings []CustomFieldMapping) string {
	if issue == nil {
		return ""
	}
	for _, mapping := range mappings {
		if mapping.JiraType != jiraRequestTypeFieldType && !strings.EqualFold(strings.TrimSpace(mapping.JiraName), "Request Type") {
			continue
		}
		value, ok := issue.Fields.CustomFields[mapping.JiraID].(map[string]any)
		if !ok {
			continue
		}
		requestType, _ := value["requestType"].(map[string]any)
		if id := firstStringKey(requestType, "id"); id != "" {
			return id
		}
		if id := firstStringKey(value, "requestTypeId", "id"); id != "" {
			return id
		}
	}
	return ""
}

func jiraPortalContainsWorkspace(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
