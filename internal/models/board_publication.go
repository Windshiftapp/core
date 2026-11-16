package models

type BoardPlacement struct {
	ID                string                `json:"id"`
	CollectionID      int                   `json:"collection_id"`
	ChannelID         int                   `json:"channel_id"`
	Label             string                `json:"label"`
	DisplayOrder      int                   `json:"display_order"`
	Enabled           bool                  `json:"enabled"`
	Policy            CustomerAccessPolicy  `json:"policy"`
	Revision          int                   `json:"revision"`
	SourceName        string                `json:"source_name,omitempty"`
	BaselinePolicy    *CustomerAccessPolicy `json:"baseline_policy,omitempty"`
	EffectivePolicy   *CustomerAccessPolicy `json:"effective_policy,omitempty"`
	StandaloneEnabled bool                  `json:"standalone_enabled"`
}
