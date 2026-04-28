package services

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"windshift/internal/database"
	"windshift/internal/emailutil"
	"windshift/internal/models"
)

var (
	ErrTokenExpired          = errors.New("verification token has expired")
	ErrTokenInvalid          = errors.New("verification token is invalid")
	ErrUserNotFound          = errors.New("user not found")
	ErrAlreadyVerified       = errors.New("email is already verified")
	ErrTokenGenerationFailed = errors.New("failed to generate verification token")
)

const (
	// TokenExpiry is the duration for which a verification token is valid
	TokenExpiry = 24 * time.Hour
)

// EmailVerificationService handles email verification for SSO users
type EmailVerificationService struct {
	db         database.Database
	smtpSender TransactionalEmailSender
	baseURL    string
}

// NewEmailVerificationService creates a new email verification service
func NewEmailVerificationService(db database.Database, smtpSender TransactionalEmailSender, baseURL string) *EmailVerificationService {
	return &EmailVerificationService{
		db:         db,
		smtpSender: smtpSender,
		baseURL:    baseURL,
	}
}

// GenerateVerificationToken generates a secure token and stores it for the user
func (s *EmailVerificationService) GenerateVerificationToken(userID int) (string, error) {
	// Generate a cryptographically secure random token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("%w: %w", ErrTokenGenerationFailed, err)
	}
	token := base64.URLEncoding.EncodeToString(tokenBytes)

	// Set expiry time
	expiresAt := time.Now().Add(TokenExpiry)

	// Store token in database
	query := `
		UPDATE users
		SET email_verification_token = ?, email_verification_expires = ?, updated_at = ?
		WHERE id = ?
	`
	result, err := s.db.Exec(query, token, expiresAt, time.Now(), userID)
	if err != nil {
		return "", fmt.Errorf("failed to store verification token: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return "", ErrUserNotFound
	}

	return token, nil
}

// SendVerificationEmail sends a verification email to the user.
func (s *EmailVerificationService) SendVerificationEmail(user *models.User, token string) error {
	firstName := user.FirstName
	if firstName == "" {
		firstName = "there"
	}
	url := fmt.Sprintf("%s/verify-email?token=%s", s.baseURL, token)
	return s.smtpSender.SendTransactional(user.Email, emailutil.TemplateEmailVerification, struct {
		FirstName       string
		VerificationURL string
	}{firstName, url})
}

// VerifyEmail validates the token and marks the user's email as verified
func (s *EmailVerificationService) VerifyEmail(token string) (*models.User, error) {
	// Find user by token
	query := `
		SELECT id, email, username, first_name, last_name, is_active, avatar_url,
		       email_verified, email_verification_expires
		FROM users
		WHERE email_verification_token = ?
	`

	var user models.User
	var expiresAt *time.Time
	err := s.db.QueryRow(query, token).Scan(
		&user.ID, &user.Email, &user.Username, &user.FirstName, &user.LastName,
		&user.IsActive, &user.AvatarURL, &user.EmailVerified, &expiresAt,
	)
	if err != nil {
		return nil, ErrTokenInvalid
	}

	// Check if already verified
	if user.EmailVerified {
		return &user, ErrAlreadyVerified
	}

	// Check if token has expired
	if expiresAt == nil || time.Now().After(*expiresAt) {
		return nil, ErrTokenExpired
	}

	// Mark email as verified and clear the token
	updateQuery := `
		UPDATE users
		SET email_verified = ?, email_verification_token = NULL, email_verification_expires = NULL, updated_at = ?
		WHERE id = ?
	`
	_, err = s.db.Exec(updateQuery, true, time.Now(), user.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to update user verification status: %w", err)
	}

	user.EmailVerified = true
	slog.Info("email verified", slog.String("component", "email"), slog.Int("user_id", user.ID), slog.String("email", user.Email))

	return &user, nil
}

// ResendVerification generates a new token and resends the verification email
func (s *EmailVerificationService) ResendVerification(userID int) error {
	// Get user details
	query := `
		SELECT id, email, username, first_name, last_name, is_active, avatar_url, email_verified
		FROM users
		WHERE id = ?
	`

	var user models.User
	err := s.db.QueryRow(query, userID).Scan(
		&user.ID, &user.Email, &user.Username, &user.FirstName, &user.LastName,
		&user.IsActive, &user.AvatarURL, &user.EmailVerified,
	)
	if err != nil {
		return ErrUserNotFound
	}

	// Check if already verified
	if user.EmailVerified {
		return ErrAlreadyVerified
	}

	// Generate new token
	token, err := s.GenerateVerificationToken(userID)
	if err != nil {
		return err
	}

	// Send verification email
	return s.SendVerificationEmail(&user, token)
}

// IsEmailVerified checks if a user's email is verified
func (s *EmailVerificationService) IsEmailVerified(userID int) (bool, error) {
	query := `SELECT email_verified FROM users WHERE id = ?`

	var verified bool
	err := s.db.QueryRow(query, userID).Scan(&verified)
	if err != nil {
		return false, ErrUserNotFound
	}

	return verified, nil
}

// SetEmailVerified directly sets the email_verified status for a user
// Used when IdP provides verified email
func (s *EmailVerificationService) SetEmailVerified(userID int, verified bool) error {
	query := `
		UPDATE users
		SET email_verified = ?, email_verification_token = NULL, email_verification_expires = NULL, updated_at = ?
		WHERE id = ?
	`
	result, err := s.db.Exec(query, verified, time.Now(), userID)
	if err != nil {
		return fmt.Errorf("failed to update email verified status: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrUserNotFound
	}

	return nil
}
