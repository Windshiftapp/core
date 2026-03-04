package sso

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

var (
	ErrExternalAccountNotFound            = errors.New("external account not found")
	ErrUserNotFound                       = errors.New("user not found")
	ErrEmailNotVerified                   = errors.New("email not verified by SSO provider")
	ErrAutoProvisionDisabled              = errors.New("automatic user provisioning is disabled")
	ErrAccountLinkingFailed               = errors.New("failed to link external account")
	ErrAccountLinkingRequiresVerification = errors.New("account linking requires verified email from identity provider")
)

// FindOrCreateResult contains the result of FindOrCreateUser with verification status
type FindOrCreateResult struct {
	User                   *models.User
	NeedsEmailVerification bool // True if we need to verify email ourselves (IdP didn't provide claim)
	IsNewUser              bool // True if user was just created
}

// ExternalAccount represents a user's linked external SSO identity
type ExternalAccount struct {
	ID          int       `json:"id"`
	UserID      int       `json:"user_id"`
	ProviderID  int       `json:"provider_id"`
	ExternalID  string    `json:"external_id"` // 'sub' claim for OIDC
	Email       string    `json:"email"`
	ProfileData string    `json:"profile_data"` // JSON blob of raw claims
	LinkedAt    time.Time `json:"linked_at"`
	LastLoginAt time.Time `json:"last_login_at"`
}

// UserStore handles database operations for SSO user management
type UserStore struct {
	db database.Database
}

// NewUserStore creates a new user store
func NewUserStore(db database.Database) *UserStore {
	return &UserStore{db: db}
}

// FindExternalAccount looks up an external account by provider and external ID
func (s *UserStore) FindExternalAccount(providerID int, externalID string) (*ExternalAccount, error) {
	query := `
		SELECT id, user_id, provider_id, external_id, email, profile_data,
		       linked_at, last_login_at
		FROM user_external_accounts
		WHERE provider_id = ? AND external_id = ?
	`

	var account ExternalAccount
	var email, profileData sql.NullString
	var lastLoginAt sql.NullTime

	err := s.db.QueryRow(query, providerID, externalID).Scan(
		&account.ID,
		&account.UserID,
		&account.ProviderID,
		&account.ExternalID,
		&email,
		&profileData,
		&account.LinkedAt,
		&lastLoginAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrExternalAccountNotFound
	}
	if err != nil {
		return nil, err
	}

	account.Email = email.String
	account.ProfileData = profileData.String
	if lastLoginAt.Valid {
		account.LastLoginAt = lastLoginAt.Time
	}

	return &account, nil
}

// FindUserByEmail finds a user by email address
func (s *UserStore) FindUserByEmail(email string) (*models.User, error) {
	query := `
		SELECT id, email, username, first_name, last_name, is_active, avatar_url,
		       password_hash, requires_password_reset, timezone, language, created_at, updated_at
		FROM users
		WHERE LOWER(email) = LOWER(?)
	`

	var user models.User
	var avatarURL, timezone, language sql.NullString

	err := s.db.QueryRow(query, email).Scan(
		&user.ID,
		&user.Email,
		&user.Username,
		&user.FirstName,
		&user.LastName,
		&user.IsActive,
		&avatarURL,
		&user.PasswordHash,
		&user.RequiresPasswordReset,
		&timezone,
		&language,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}

	user.AvatarURL = avatarURL.String
	user.Timezone = timezone.String
	user.Language = language.String

	return &user, nil
}

// GetUserByID retrieves a user by ID
func (s *UserStore) GetUserByID(userID int) (*models.User, error) {
	query := `
		SELECT id, email, username, first_name, last_name, is_active, avatar_url,
		       password_hash, requires_password_reset, timezone, language, created_at, updated_at
		FROM users
		WHERE id = ?
	`

	var user models.User
	var avatarURL, timezone, language sql.NullString

	err := s.db.QueryRow(query, userID).Scan(
		&user.ID,
		&user.Email,
		&user.Username,
		&user.FirstName,
		&user.LastName,
		&user.IsActive,
		&avatarURL,
		&user.PasswordHash,
		&user.RequiresPasswordReset,
		&timezone,
		&language,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}

	user.AvatarURL = avatarURL.String
	user.Timezone = timezone.String
	user.Language = language.String

	return &user, nil
}

// LinkExternalAccount creates a new external account link
func (s *UserStore) LinkExternalAccount(userID, providerID int, externalID, email string, claims *OIDCClaims) error {
	// Serialize claims for profile_data
	profileData := ""
	if claims != nil && claims.Raw != nil {
		data, err := json.Marshal(claims.Raw)
		if err == nil {
			profileData = string(data)
		}
	}

	query := `
		INSERT INTO user_external_accounts (
			user_id, provider_id, external_id, email, profile_data,
			linked_at, last_login_at
		) VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`

	_, err := s.db.Exec(query, userID, providerID, externalID, nullStringFromString(email), nullStringFromString(profileData))
	return err
}

// UpdateLastLogin updates the last login timestamp for an external account
func (s *UserStore) UpdateLastLogin(accountID int) error {
	query := `UPDATE user_external_accounts SET last_login_at = CURRENT_TIMESTAMP WHERE id = ?`
	_, err := s.db.Exec(query, accountID)
	return err
}

// CreateUser creates a new user from SSO claims
// emailVerified indicates whether the IdP has verified the email (used for setting email_verified field)
func (s *UserStore) CreateUser(claims *OIDCClaims, emailVerified bool) (*models.User, error) {
	// Generate username from claims
	username := generateUsername(claims)

	// Use first/last name from claims
	firstName := claims.GivenName
	lastName := claims.FamilyName

	// If no first/last name, try to parse from full name
	if firstName == "" && lastName == "" && claims.Name != "" {
		parts := strings.SplitN(claims.Name, " ", 2)
		firstName = parts[0]
		if len(parts) > 1 {
			lastName = parts[1]
		}
	}

	// Avatar URL from claims
	avatarURL := claims.Picture

	query := `
		INSERT INTO users (
			email, username, first_name, last_name, is_active, avatar_url,
			password_hash, requires_password_reset, timezone, language,
			email_verified,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, 1, ?, '', 0, 'UTC', 'en', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		RETURNING id
	`

	var id int64
	err := s.db.QueryRow(query,
		claims.Email,
		username,
		firstName,
		lastName,
		nullStringFromString(avatarURL),
		emailVerified,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return s.GetUserByID(int(id))
}

// FindOrCreateUser handles the complete SSO user flow:
// 1. Check if external account already linked -> return user
// 2. Check if user exists with same email -> link account -> return user
// 3. If auto-provision enabled -> create user -> link account -> return user
// 4. If auto-provision disabled -> return error
//
// Email verification logic:
// - If IdP says email_verified: true -> user is verified
// - If IdP says email_verified: false AND RequireVerifiedEmail is true -> block login
// - If IdP doesn't provide email_verified claim -> we need to verify ourselves
func (s *UserStore) FindOrCreateUser(provider *SSOProvider, claims *OIDCClaims) (*FindOrCreateResult, error) {
	result := &FindOrCreateResult{}

	// Determine email verification status from IdP
	// Three cases:
	// 1. IdP provided email_verified: true -> trust it, user is verified
	// 2. IdP provided email_verified: false -> if RequireVerifiedEmail, block; otherwise continue
	// 3. IdP didn't provide email_verified -> we need to verify ourselves
	needsOurVerification := false
	emailIsVerified := false

	if claims.EmailVerifiedProvided {
		// IdP provided the claim
		if claims.EmailVerified {
			// Case 1: IdP confirms email is verified
			emailIsVerified = true
		} else {
			// Case 2: IdP explicitly says email is NOT verified
			if provider.RequireVerifiedEmail {
				return nil, fmt.Errorf("%w: the identity provider reports your email address is not verified", ErrEmailNotVerified)
			}
			// If provider doesn't require verification, allow login but mark as unverified
			emailIsVerified = false
		}
	} else {
		// Case 3: IdP didn't provide the claim - we need to verify ourselves
		needsOurVerification = true
		emailIsVerified = false
	}

	// 1. Check if external account already linked
	extAccount, err := s.FindExternalAccount(provider.ID, claims.Subject)
	if err == nil {
		// Update last login time
		_ = s.UpdateLastLogin(extAccount.ID)
		var user *models.User
		user, err = s.GetUserByID(extAccount.UserID)
		if err != nil {
			return nil, err
		}
		result.User = user
		// For existing users, only need verification if they're not already verified
		// AND the IdP doesn't provide verification status
		result.NeedsEmailVerification = needsOurVerification && !user.EmailVerified
		return result, nil
	}

	// 2. Try to link by email - ONLY if IdP has verified the email
	// Security: This prevents account takeover via malicious IdP that claims
	// unverified emails matching existing users
	if claims.Email != "" {
		var existingUser *models.User
		existingUser, err = s.FindUserByEmail(claims.Email)
		if err == nil {
			// Security check: Only auto-link if IdP has explicitly verified the email
			// This prevents account takeover where an attacker controls an IdP and
			// creates a user with a victim's email address
			if !claims.EmailVerified || !claims.EmailVerifiedProvided {
				return nil, fmt.Errorf("%w: cannot automatically link to existing account '%s' without verified email from identity provider", ErrAccountLinkingRequiresVerification, claims.Email)
			}
			// Link the external account to existing user
			if err = s.LinkExternalAccount(existingUser.ID, provider.ID, claims.Subject, claims.Email, claims); err != nil {
				return nil, fmt.Errorf("%w: %w", ErrAccountLinkingFailed, err)
			}
			result.User = existingUser
			// For existing users, only need verification if they're not already verified
			// AND the IdP doesn't provide verification status
			result.NeedsEmailVerification = needsOurVerification && !existingUser.EmailVerified
			return result, nil
		}
	}

	// 3. Check if auto-provisioning is enabled
	if !provider.AutoProvisionUsers {
		return nil, ErrAutoProvisionDisabled
	}

	// 4. Create new user
	if claims.Email == "" {
		return nil, fmt.Errorf("%w: email is required for user provisioning", ErrOIDCMissingClaims)
	}

	// Create user with email_verified set based on IdP claim
	newUser, err := s.CreateUser(claims, emailIsVerified)
	if err != nil {
		return nil, err
	}

	// Link the external account
	if err := s.LinkExternalAccount(newUser.ID, provider.ID, claims.Subject, claims.Email, claims); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrAccountLinkingFailed, err)
	}

	result.User = newUser
	result.IsNewUser = true
	result.NeedsEmailVerification = needsOurVerification
	return result, nil
}

// GetExternalAccountsForUser returns all external accounts linked to a user
func (s *UserStore) GetExternalAccountsForUser(userID int) ([]*ExternalAccount, error) {
	query := `
		SELECT ea.id, ea.user_id, ea.provider_id, ea.external_id, ea.email,
		       ea.profile_data, ea.linked_at, ea.last_login_at
		FROM user_external_accounts ea
		WHERE ea.user_id = ?
		ORDER BY ea.linked_at DESC
	`

	rows, err := s.db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var accounts []*ExternalAccount
	for rows.Next() {
		var account ExternalAccount
		var email, profileData sql.NullString
		var lastLoginAt sql.NullTime

		err := rows.Scan(
			&account.ID,
			&account.UserID,
			&account.ProviderID,
			&account.ExternalID,
			&email,
			&profileData,
			&account.LinkedAt,
			&lastLoginAt,
		)
		if err != nil {
			return nil, err
		}

		account.Email = email.String
		account.ProfileData = profileData.String
		if lastLoginAt.Valid {
			account.LastLoginAt = lastLoginAt.Time
		}

		accounts = append(accounts, &account)
	}

	return accounts, nil
}

// UnlinkExternalAccount removes an external account link
func (s *UserStore) UnlinkExternalAccount(accountID, userID int) error {
	query := `DELETE FROM user_external_accounts WHERE id = ? AND user_id = ?`
	result, err := s.db.Exec(query, accountID, userID)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrExternalAccountNotFound
	}

	return nil
}

// generateUsername creates a username from SSO claims
func generateUsername(claims *OIDCClaims) string {
	// Try preferred_username first
	if claims.Username != "" {
		return sanitizeUsername(claims.Username)
	}

	// Try email prefix
	if claims.Email != "" {
		parts := strings.Split(claims.Email, "@")
		return sanitizeUsername(parts[0])
	}

	// Try name
	if claims.Name != "" {
		return sanitizeUsername(strings.ReplaceAll(claims.Name, " ", "."))
	}

	// Fallback to subject (truncated)
	if len(claims.Subject) > 20 {
		return claims.Subject[:20]
	}
	return claims.Subject
}

// sanitizeUsername makes a string safe for use as a username
func sanitizeUsername(s string) string {
	s = strings.ToLower(s)
	// Replace common separators with dots
	s = strings.ReplaceAll(s, " ", ".")
	s = strings.ReplaceAll(s, "_", ".")
	// Remove any characters that aren't alphanumeric or dots
	var result strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// nullStringFromString converts a string to sql.NullString
func nullStringFromString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
