package models

// CustomerAccessPolicy controls admission independently of resource content visibility.
type CustomerAccessPolicy struct {
	Mode            string `json:"mode"`
	OrganisationIDs []int  `json:"organisation_ids"`
}

type CustomerSurface struct {
	Kind string `json:"kind"`
	ID   int    `json:"id"`
	Slug string `json:"slug"`
}

type CustomerLoginProvider struct {
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	ProviderType string `json:"provider_type"`
}

type CustomerAccessStatus struct {
	Restricted    bool                    `json:"restricted"`
	Authenticated bool                    `json:"authenticated"`
	Allowed       bool                    `json:"allowed"`
	Providers     []CustomerLoginProvider `json:"providers"`
	Passkeys      bool                    `json:"passkeys"`
}

type CustomerLoginRequest struct {
	Email     string `json:"email,omitempty"`
	Token     string `json:"token,omitempty"`
	SessionID string `json:"sessionId,omitempty"`
	Response  any    `json:"response,omitempty"`
}

type CustomerLoginResponse struct {
	Success   bool           `json:"success"`
	Message   string         `json:"message,omitempty"`
	PublicKey any            `json:"publicKey,omitempty"`
	SessionID string         `json:"sessionId,omitempty"`
	Customer  map[string]any `json:"customer,omitempty"`
}
