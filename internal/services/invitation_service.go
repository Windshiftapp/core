package services

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"golang.org/x/crypto/bcrypt"

	"windshift/internal/database"
	"windshift/internal/emailutil"
	"windshift/internal/models"
)

var (
	ErrInvitationExpired          = errors.New("invitation has expired")
	ErrInvitationInvalid          = errors.New("invitation is invalid")
	ErrInvitationAlreadyUsed      = errors.New("invitation has already been used")
	ErrInvitationGenerationFailed = errors.New("failed to generate invitation token")
)

const (
	// InvitationExpiry is the duration for which an invitation token is valid
	InvitationExpiry = 7 * 24 * time.Hour
	// InvitationTokenLength is the length of the random bytes for the token
	InvitationTokenLength = 32
)

// InvitationService handles user invitations
type InvitationService struct {
	db         database.Database
	smtpSender TransactionalEmailSender
	baseURL    string
}

// NewInvitationService creates a new invitation service
func NewInvitationService(db database.Database, smtpSender TransactionalEmailSender, baseURL string) *InvitationService {
	return &InvitationService{
		db:         db,
		smtpSender: smtpSender,
		baseURL:    baseURL,
	}
}

// GenerateInvitation creates a new invitation token for a user
func (s *InvitationService) GenerateInvitation(userID int) (string, error) {
	// Generate a cryptographically secure random token
	tokenBytes := make([]byte, InvitationTokenLength)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvitationGenerationFailed, err)
	}
	token := base64.URLEncoding.EncodeToString(tokenBytes)

	// Set expiry time
	expiresAt := time.Now().Add(InvitationExpiry)

	// Store token in database
	query := `
		INSERT INTO user_invitations (user_id, token, expires_at, created_at)
		VALUES (?, ?, ?, ?)
	`
	_, err := s.db.ExecWrite(query, userID, token, expiresAt, time.Now())
	if err != nil {
		return "", fmt.Errorf("failed to store invitation token: %w", err)
	}

	slog.Debug("invitation generated", slog.String("component", "invitation"), slog.Int("user_id", userID))
	return token, nil
}

// SendInvitationEmail sends the invitation email to the user.
func (s *InvitationService) SendInvitationEmail(user *models.User, token string) error {
	firstName := user.FirstName
	if firstName == "" {
		firstName = "there"
	}
	url := fmt.Sprintf("%s/set-password/%s", s.baseURL, token)
	return s.smtpSender.SendTransactional(user.Email, emailutil.TemplateInvitation, struct {
		FirstName     string
		InvitationURL string
	}{firstName, url})
}

// VerifyInvitation validates an invitation token and returns the user info
func (s *InvitationService) VerifyInvitation(token string) (*models.User, error) {
	// Find invitation by token
	query := `
		SELECT i.id, i.user_id, i.expires_at, i.used_at,
		       u.email, u.username, u.first_name, u.last_name, u.is_active
		FROM user_invitations i
		JOIN users u ON i.user_id = u.id
		WHERE i.token = ?
	`

	var invitationID int
	var userID int
	var expiresAt time.Time
	var usedAt sql.NullTime
	var user models.User

	err := s.db.QueryRow(query, token).Scan(
		&invitationID, &userID, &expiresAt, &usedAt,
		&user.Email, &user.Username, &user.FirstName, &user.LastName, &user.IsActive,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvitationInvalid
		}
		return nil, fmt.Errorf("failed to validate invitation: %w", err)
	}

	user.ID = userID

	// Check if already used
	if usedAt.Valid {
		return nil, ErrInvitationAlreadyUsed
	}

	// Check if expired
	if time.Now().After(expiresAt) {
		return nil, ErrInvitationExpired
	}

	return &user, nil
}

// AcceptInvitation sets the user's password and marks the invitation as used
func (s *InvitationService) AcceptInvitation(token, password string) error {
	// 1. Verify invitation
	user, err := s.VerifyInvitation(token)
	if err != nil {
		return err
	}

	// 2. Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// 3. Update user and invitation in a transaction
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Update user: set password, activate, mark as verified, and clear requires_password_reset
	userQuery := `
		UPDATE users
		SET password_hash = ?, requires_password_reset = false, email_verified = true, is_active = true, updated_at = ?
		WHERE id = ?
	`
	_, err = tx.Exec(userQuery, string(hashedPassword), time.Now(), user.ID)
	if err != nil {
		return fmt.Errorf("failed to update user password: %w", err)
	}

	// Mark invitation as used
	inviteQuery := `UPDATE user_invitations SET used_at = ? WHERE token = ?`
	_, err = tx.Exec(inviteQuery, time.Now(), token)
	if err != nil {
		return fmt.Errorf("failed to mark invitation as used: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	slog.Info("invitation accepted", slog.String("component", "invitation"), slog.Int("user_id", user.ID), slog.String("email", user.Email))
	return nil
}

// CleanupExpiredInvitations removes expired invitation tokens
// deadcode-keep: called by core-tests/internal/services/invitation_service_test.go
func (s *InvitationService) CleanupExpiredInvitations() error {
	query := `DELETE FROM user_invitations WHERE expires_at < ? OR used_at IS NOT NULL`
	_, err := s.db.ExecWrite(query, time.Now().Add(-24*time.Hour)) // Keep used/expired for 24 hours
	if err != nil {
		return fmt.Errorf("failed to cleanup expired invitations: %w", err)
	}
	return nil
}
