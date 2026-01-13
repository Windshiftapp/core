package services

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/smtp"
)

var (
	ErrTokenExpired       = errors.New("verification token has expired")
	ErrTokenInvalid       = errors.New("verification token is invalid")
	ErrUserNotFound       = errors.New("user not found")
	ErrAlreadyVerified    = errors.New("email is already verified")
	ErrSMTPNotConfigured  = errors.New("SMTP is not configured")
	ErrTokenGenerationFailed = errors.New("failed to generate verification token")
)

const (
	// TokenExpiry is the duration for which a verification token is valid
	TokenExpiry = 24 * time.Hour
)

// EmailVerificationService handles email verification for SSO users
type EmailVerificationService struct {
	db          database.Database
	smtpSender  *smtp.NotificationSMTPSender
	baseURL     string
}

// NewEmailVerificationService creates a new email verification service
func NewEmailVerificationService(db database.Database, smtpSender *smtp.NotificationSMTPSender, baseURL string) *EmailVerificationService {
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
		return "", fmt.Errorf("%w: %v", ErrTokenGenerationFailed, err)
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

// SendVerificationEmail sends a verification email to the user
func (s *EmailVerificationService) SendVerificationEmail(user *models.User, token string) error {
	if s.smtpSender == nil || !s.smtpSender.IsSMTPConfigured() {
		return ErrSMTPNotConfigured
	}

	// Generate verification URL
	verificationURL := fmt.Sprintf("%s/verify-email?token=%s", s.baseURL, token)

	// Generate email content
	subject := "Verify your email address"
	htmlBody, textBody, err := s.generateEmailBody(user.FirstName, verificationURL)
	if err != nil {
		return fmt.Errorf("failed to generate email body: %w", err)
	}

	// Send email using the SMTP sender
	return s.smtpSender.SendCustomEmail(user.Email, subject, htmlBody, textBody)
}

// generateEmailBody generates the HTML and text email body for verification
func (s *EmailVerificationService) generateEmailBody(firstName, verificationURL string) (string, string, error) {
	if firstName == "" {
		firstName = "there"
	}

	// HTML template
	htmlTemplate := `
<!DOCTYPE html>
<html>
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>Verify your email address</title>
	<style>
		body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; margin: 0; padding: 0; background-color: #f5f5f5; }
		.container { max-width: 600px; margin: 0 auto; background-color: white; }
		.header { background-color: #2563eb; color: white; padding: 24px; text-align: center; }
		.header h1 { margin: 0; font-size: 24px; font-weight: 600; }
		.content { padding: 32px 24px; }
		.greeting { font-size: 16px; color: #374151; margin-bottom: 24px; }
		.message { font-size: 14px; color: #4b5563; line-height: 1.6; margin-bottom: 24px; }
		.button-container { text-align: center; margin: 32px 0; }
		.button { display: inline-block; background-color: #2563eb; color: white; text-decoration: none; padding: 14px 32px; border-radius: 6px; font-weight: 600; font-size: 14px; }
		.button:hover { background-color: #1d4ed8; }
		.link-fallback { font-size: 12px; color: #9ca3af; margin-top: 24px; word-break: break-all; }
		.expiry-notice { font-size: 14px; color: #6b7280; margin-top: 24px; padding: 16px; background-color: #f9fafb; border-radius: 6px; }
		.footer { background-color: #f9fafb; padding: 24px; text-align: center; font-size: 14px; color: #6b7280; border-top: 1px solid #e5e7eb; }
	</style>
</head>
<body>
	<div class="container">
		<div class="header">
			<h1>Verify Your Email</h1>
		</div>
		<div class="content">
			<div class="greeting">
				Hi {{.FirstName}},
			</div>
			<div class="message">
				Please verify your email address to complete your account setup. Click the button below to verify:
			</div>
			<div class="button-container">
				<a href="{{.VerificationURL}}" class="button">Verify Email Address</a>
			</div>
			<div class="expiry-notice">
				This link expires in 24 hours. If you didn't create an account, you can safely ignore this email.
			</div>
			<div class="link-fallback">
				If the button doesn't work, copy and paste this link into your browser:<br>
				{{.VerificationURL}}
			</div>
		</div>
		<div class="footer">
			This is an automated email. Please do not reply.
		</div>
	</div>
</body>
</html>`

	// Text template
	textTemplate := `Hi {{.FirstName}},

Please verify your email address to complete your account setup.

Click the link below to verify:
{{.VerificationURL}}

This link expires in 24 hours.

If you didn't create an account, you can ignore this email.

---
This is an automated email. Please do not reply.`

	// Prepare template data
	templateData := struct {
		FirstName       string
		VerificationURL string
	}{
		FirstName:       firstName,
		VerificationURL: verificationURL,
	}

	// Parse and execute HTML template
	htmlTmpl, err := template.New("html").Parse(htmlTemplate)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse HTML template: %w", err)
	}

	var htmlBuffer bytes.Buffer
	if err := htmlTmpl.Execute(&htmlBuffer, templateData); err != nil {
		return "", "", fmt.Errorf("failed to execute HTML template: %w", err)
	}

	// Parse and execute text template
	textTmpl, err := template.New("text").Parse(textTemplate)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse text template: %w", err)
	}

	var textBuffer bytes.Buffer
	if err := textTmpl.Execute(&textBuffer, templateData); err != nil {
		return "", "", fmt.Errorf("failed to execute text template: %w", err)
	}

	return htmlBuffer.String(), textBuffer.String(), nil
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
