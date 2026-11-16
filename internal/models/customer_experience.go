package models

import "slices"

var PortalModuleKinds = []string{"boards", "requests", "forms", "knowledge-base", "drafts", "approvals", "profile", "asset-reports"}

func (config ChannelConfig) PortalModuleEnabled(kind string) bool {
	return config.PortalModules == nil || slices.Contains(config.PortalModules, kind)
}

type CustomerExperienceBoard struct {
	ID                  string `json:"id"`
	Label               string `json:"label"`
	DisplayOrder        int    `json:"display_order"`
	PublicationRevision int    `json:"publication_revision"`
	PlacementRevision   int    `json:"placement_revision"`
}

type CustomerExperience struct {
	Presentation  string                    `json:"presentation"`
	Title         string                    `json:"title"`
	LogoURL       string                    `json:"logo_url"`
	Slug          string                    `json:"slug"`
	Modules       []string                  `json:"modules"`
	Navigation    []string                  `json:"navigation"`
	DefaultModule string                    `json:"default_module"`
	Boards        []CustomerExperienceBoard `json:"boards"`
	Authenticated bool                      `json:"authenticated"`
	IsInternal    bool                      `json:"is_internal"`
	IdentityKey   int                       `json:"identity_key"`
	Revision      string                    `json:"revision"`
}
