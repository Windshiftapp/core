package portalwebauthn

import (
	"encoding/base64"
	"fmt"
	"strconv"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"windshift/internal/auth"
)

// Subject adapts auth.PortalCustomer to the webauthn.User interface.
type Subject struct {
	*auth.PortalCustomer
	credentials []webauthn.Credential
}

// NewSubject creates a Subject for the given portal customer with no
// credentials loaded; call SetCredentials before BeginRegistration/BeginLogin.
func NewSubject(c *auth.PortalCustomer) *Subject {
	return &Subject{PortalCustomer: c, credentials: []webauthn.Credential{}}
}

// WebAuthnID returns the user-handle bytes the authenticator stores. We use
// the portal customer ID as ASCII digits — at most 19 bytes for INT64, well
// under the WebAuthn 64-byte limit.
func (s *Subject) WebAuthnID() []byte { return []byte(strconv.Itoa(s.ID)) }

// WebAuthnName is the user-visible identifier (email).
func (s *Subject) WebAuthnName() string { return s.Email }

// WebAuthnDisplayName is the customer's full name; falls back to email.
func (s *Subject) WebAuthnDisplayName() string {
	if s.Name != "" {
		return s.Name
	}
	return s.Email
}

// WebAuthnCredentials returns currently-loaded credentials.
func (s *Subject) WebAuthnCredentials() []webauthn.Credential { return s.credentials }

// WebAuthnIcon is unused.
func (s *Subject) WebAuthnIcon() string { return "" }

// SetCredentials loads the customer's stored credentials onto the subject.
func (s *Subject) SetCredentials(creds []webauthn.Credential) { s.credentials = creds }

// CredentialExcludeList builds the exclusion list used during registration so
// the same authenticator cannot be enrolled twice.
func (s *Subject) CredentialExcludeList() []protocol.CredentialDescriptor {
	excludeList := make([]protocol.CredentialDescriptor, 0, len(s.credentials))
	for _, cred := range s.credentials {
		excludeList = append(excludeList, protocol.CredentialDescriptor{
			Type:         protocol.PublicKeyCredentialType,
			CredentialID: cred.ID,
			Transport:    cred.Transport,
		})
	}
	return excludeList
}

// Credential is the on-the-wire / database shape for a stored portal
// credential. Mirrors webauthn.WebAuthnCredential but keyed on
// portal_customer_id.
type Credential struct {
	ID                  string   `json:"id"`
	PortalCustomerID    int      `json:"portal_customer_id"`
	CredentialName      string   `json:"credential_name"`
	PublicKey           []byte   `json:"-"`
	AttestationType     string   `json:"attestation_type"`
	AAGUID              []byte   `json:"-"`
	SignCount           uint32   `json:"sign_count"`
	CloneWarning        bool     `json:"clone_warning"`
	Transport           []string `json:"transport"`
	FlagsUserPresent    bool     `json:"flags_user_present"`
	FlagsUserVerified   bool     `json:"flags_user_verified"`
	FlagsBackupEligible bool     `json:"flags_backup_eligible"`
	FlagsBackupState    bool     `json:"flags_backup_state"`
	CreatedAt           string   `json:"created_at"`
	UpdatedAt           string   `json:"updated_at"`
	LastUsedAt          *string  `json:"last_used_at,omitempty"`
}

// ToWebAuthnCredential converts the database row to a go-webauthn credential.
func (wc *Credential) ToWebAuthnCredential() (webauthn.Credential, error) {
	credID, err := base64.RawURLEncoding.DecodeString(wc.ID)
	if err != nil {
		return webauthn.Credential{}, fmt.Errorf("failed to decode credential ID: %w", err)
	}
	transports := make([]protocol.AuthenticatorTransport, 0, len(wc.Transport))
	for _, t := range wc.Transport {
		transports = append(transports, protocol.AuthenticatorTransport(t))
	}
	return webauthn.Credential{
		ID:              credID,
		PublicKey:       wc.PublicKey,
		AttestationType: wc.AttestationType,
		Transport:       transports,
		Flags: webauthn.CredentialFlags{
			UserPresent:    wc.FlagsUserPresent,
			UserVerified:   wc.FlagsUserVerified,
			BackupEligible: wc.FlagsBackupEligible,
			BackupState:    wc.FlagsBackupState,
		},
		Authenticator: webauthn.Authenticator{
			AAGUID:       wc.AAGUID,
			SignCount:    wc.SignCount,
			CloneWarning: wc.CloneWarning,
		},
	}, nil
}

// FromWebAuthnCredential builds a database row from a go-webauthn credential.
func FromWebAuthnCredential(portalCustomerID int, name string, cred *webauthn.Credential) *Credential {
	credID := base64.RawURLEncoding.EncodeToString(cred.ID)
	transports := make([]string, 0, len(cred.Transport))
	for _, t := range cred.Transport {
		transports = append(transports, string(t))
	}
	return &Credential{
		ID:                  credID,
		PortalCustomerID:    portalCustomerID,
		CredentialName:      name,
		PublicKey:           cred.PublicKey,
		AttestationType:     cred.AttestationType,
		AAGUID:              cred.Authenticator.AAGUID,
		SignCount:           cred.Authenticator.SignCount,
		CloneWarning:        cred.Authenticator.CloneWarning,
		Transport:           transports,
		FlagsUserPresent:    cred.Flags.UserPresent,
		FlagsUserVerified:   cred.Flags.UserVerified,
		FlagsBackupEligible: cred.Flags.BackupEligible,
		FlagsBackupState:    cred.Flags.BackupState,
	}
}
