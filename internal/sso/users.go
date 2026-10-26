package sso

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

var (
	ErrExternalAccountNotFound            = errors.New("external account not found")
	ErrUserNotFound                       = errors.New("user not found")
	ErrAutoProvisionDisabled              = errors.New("automatic user provisioning is disabled")
	ErrAccountLinkingFailed               = errors.New("failed to link external account")
	ErrAccountLinkingRequiresVerification = errors.New("account linking requires verified email from identity provider")
)

// FindOrCreateResult contains the result of FindOrCreateUser
type FindOrCreateResult struct {
	User      *models.User
	IsNewUser bool // True if user was just created
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

	if errors.Is(err, sql.ErrNoRows) {
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

// getUserByQuery executes a user query with the given WHERE clause and argument,
// scanning the result into a models.User. The query must select columns in the
// standard order: id, email, username, first_name, last_name, is_active,
// avatar_url, password_hash, requires_password_reset, timezone, language,
// email_verified, created_at, updated_at.
func (s *UserStore) getUserByQuery(query string, arg any) (*models.User, error) {
	var user models.User
	var avatarURL, timezone, language sql.NullString

	err := s.db.QueryRow(query, arg).Scan(
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
		&user.EmailVerified,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
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

// FindUserByEmail finds a user by email address
func (s *UserStore) FindUserByEmail(email string) (*models.User, error) {
	query := `
		SELECT id, email, username, first_name, last_name, is_active, avatar_url,
		       password_hash, requires_password_reset, timezone, language, email_verified, created_at, updated_at
		FROM users
		WHERE LOWER(email) = LOWER(?)
	`
	return s.getUserByQuery(query, email)
}

// GetUserByID retrieves a user by ID
func (s *UserStore) GetUserByID(userID int) (*models.User, error) {
	query := `
		SELECT id, email, username, first_name, last_name, is_active, avatar_url,
		       password_hash, requires_password_reset, timezone, language, email_verified, created_at, updated_at
		FROM users
		WHERE id = ?
	`
	return s.getUserByQuery(query, userID)
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

	_, err := s.db.ExecWrite(query, userID, providerID, externalID, nullStringFromString(email), nullStringFromString(profileData))
	return err
}

// UpdateLastLogin updates the last login timestamp for an external account
func (s *UserStore) UpdateLastLogin(accountID int) error {
	query := `UPDATE user_external_accounts SET last_login_at = CURRENT_TIMESTAMP WHERE id = ?`
	_, err := s.db.ExecWrite(query, accountID)
	return err
}

// CreateUser creates a new user from SSO claims
// emailVerified indicates whether the IdP has verified the email (used for setting email_verified field)
func (s *UserStore) CreateUser(claims *OIDCClaims, emailVerified bool) (*models.User, error) {
	// Generate username from claims
	username := generateUsername(claims)

	// Resolve first/last name from the explicit claims, falling back to the
	// display name (which some IdPs format as "Last, First").
	firstName, lastName := resolveNames(claims)

	// Avatar URL from claims
	avatarURL := claims.Picture

	query := `
		INSERT INTO users (
			email, username, first_name, last_name, is_active, avatar_url,
			password_hash, requires_password_reset, timezone, language,
			email_verified,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, true, ?, '', false, 'UTC', 'en', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
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
// Email and linking policy:
//   - SSO authentication is itself evidence that the identity controls the
//     account: every user returned from a successful login is treated as
//     email-verified. Nothing in the app gates on the verification column.
//   - require_verified_email only governs auto-linking to pre-existing
//     accounts: when disabled, linking requires a verified email claim from
//     the provider.
func (s *UserStore) FindOrCreateUser(provider *SSOProvider, claims *OIDCClaims) (*FindOrCreateResult, error) {
	result := &FindOrCreateResult{}

	claimVerifiesEmail := claims.Email != "" && claims.EmailVerifiedProvided && claims.EmailVerified

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
		s.syncNamesFromProvider(user, claims, provider)
		result.User = user
		return result, nil
	}

	// 2. Link by email only when the claim verifies it or the provider is trusted.
	if claims.Email != "" {
		var existingUser *models.User
		existingUser, err = s.FindUserByEmail(claims.Email)
		if err == nil {
			// Never grant an unverified SSO identity access to an existing account.
			if !claimVerifiesEmail && !provider.RequireVerifiedEmail {
				return nil, fmt.Errorf("%w: cannot automatically link to existing account '%s' without verified email from identity provider", ErrAccountLinkingRequiresVerification, claims.Email)
			}
			// Link the external account to existing user
			if err = s.LinkExternalAccount(existingUser.ID, provider.ID, claims.Subject, claims.Email, claims); err != nil {
				return nil, fmt.Errorf("%w: %w", ErrAccountLinkingFailed, err)
			}
			s.syncNamesFromProvider(existingUser, claims, provider)
			result.User = existingUser
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

	// Record explicit negative verification before creating the user; the row
	// is still created verified because SSO authentication just happened.
	if claims.EmailVerifiedProvided && !claims.EmailVerified {
		slog.Warn("SSO auto-provisioning a new user from an IdP-unverified email claim",
			slog.String("component", "sso"),
			slog.String("provider", provider.Slug),
			slog.Int("provider_id", provider.ID),
			slog.String("email", claims.Email),
			slog.String("subject", claims.Subject))
	}

	// Create the user; SSO authentication verifies account ownership.
	newUser, err := s.CreateUser(claims, true)
	if err != nil {
		return nil, err
	}

	// Link the external account
	if err := s.LinkExternalAccount(newUser.ID, provider.ID, claims.Subject, claims.Email, claims); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrAccountLinkingFailed, err)
	}

	result.User = newUser
	result.IsNewUser = true
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
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return accounts, nil
}

// UnlinkExternalAccount removes an external account link
func (s *UserStore) UnlinkExternalAccount(accountID, userID int) error {
	query := `DELETE FROM user_external_accounts WHERE id = ? AND user_id = ?`
	result, err := s.db.ExecWrite(query, accountID, userID)
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

// resolveNames derives the first and last name to persist from IdP claims.
// Explicit given_name / family_name win. A missing half is only filled from a
// comma-separated display name ("Last, First"), where the family/given
// boundary is unambiguous. A space-separated display name is left unparsed
// because multi-part names cannot be split reliably.
func resolveNames(claims *OIDCClaims) (firstName, lastName string) {
	firstName = strings.TrimSpace(claims.GivenName)
	lastName = strings.TrimSpace(claims.FamilyName)
	if firstName != "" && lastName != "" {
		return firstName, lastName
	}

	fallbackFirst, fallbackLast, ok := splitCommaDisplayName(claims.Name)
	if !ok {
		return firstName, lastName
	}
	if firstName == "" {
		firstName = fallbackFirst
	}
	if lastName == "" {
		lastName = fallbackLast
	}
	return firstName, lastName
}

// splitCommaDisplayName splits the "Last, First" display name some IdPs (e.g.
// Entra) use. It reports ok=false when the name has no comma, since a
// space-separated name does not reliably mark the family/given boundary for
// multi-part names.
func splitCommaDisplayName(name string) (firstName, lastName string, ok bool) {
	name = strings.TrimSpace(name)
	comma := strings.Index(name, ",")
	if comma < 0 {
		return "", "", false
	}
	family := strings.TrimSpace(name[:comma])
	given := strings.TrimSpace(name[comma+1:])
	if family == "" && given == "" {
		return "", "", false
	}
	return given, family, true
}

// syncNamesFromProvider updates a user's name from the IdP claims, best-effort:
// a name-sync failure must not block authentication.
func (s *UserStore) syncNamesFromProvider(user *models.User, claims *OIDCClaims, provider *SSOProvider) {
	if err := s.syncNames(user, claims); err != nil {
		slog.Warn("failed to sync SSO user name",
			slog.String("component", "sso"),
			slog.String("provider", provider.Slug),
			slog.Int("user_id", user.ID),
			slog.Any("error", err))
	}
}

// syncNames updates a user's name from the identity provider. It only runs
// when the provider sends an explicit given_name or family_name, so a
// display-name guess cannot overwrite a locally corrected name. A half that
// the provider omits keeps the stored value.
func (s *UserStore) syncNames(user *models.User, claims *OIDCClaims) error {
	if strings.TrimSpace(claims.GivenName) == "" && strings.TrimSpace(claims.FamilyName) == "" {
		return nil
	}

	firstName, lastName := resolveNames(claims)
	if firstName == "" {
		firstName = user.FirstName
	}
	if lastName == "" {
		lastName = user.LastName
	}
	if firstName == user.FirstName && lastName == user.LastName {
		return nil
	}

	if _, err := s.db.ExecWrite(
		`UPDATE users SET first_name = ?, last_name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		firstName, lastName, user.ID,
	); err != nil {
		return err
	}
	user.FirstName = firstName
	user.LastName = lastName
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
