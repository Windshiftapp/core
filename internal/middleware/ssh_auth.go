package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/services"

	"github.com/charmbracelet/ssh"
	gossh "golang.org/x/crypto/ssh"
)

// backgroundUpdateTimeout is the maximum time allowed for background credential updates
const backgroundUpdateTimeout = 5 * time.Second

// SSHAuthMiddleware provides SSH public key authentication
type SSHAuthMiddleware struct {
	sshAuthService *services.SSHAuthService
}

// NewSSHAuthMiddleware creates a new SSH authentication middleware
func NewSSHAuthMiddleware(db database.Database) *SSHAuthMiddleware {
	return &SSHAuthMiddleware{
		sshAuthService: services.NewSSHAuthService(db),
	}
}

// PublicKeyHandler returns an SSH public key authentication handler
func (m *SSHAuthMiddleware) PublicKeyHandler() ssh.PublicKeyHandler {
	return func(ctx ssh.Context, key ssh.PublicKey) bool {
		// Convert the SSH public key to string format for comparison
		keyStr, err := m.convertPublicKeyToString(key)
		if err != nil {
			slog.Error("failed to convert public key to string", slog.String("component", "ssh_auth"), slog.Any("error", err))
			return false
		}

		// Check if the key is authorized and get user details
		userCredential, err := m.sshAuthService.FindUserBySSHKeyWithDetails(keyStr)
		if err != nil {
			slog.Error("key authorization check failed", slog.String("component", "ssh_auth"), slog.Any("error", err))
			return false
		}

		if userCredential == nil {
			// Log the failed attempt with key fingerprint for security monitoring
			fingerprint := gossh.FingerprintSHA256(key)
			slog.Warn("unauthorized key attempt", slog.String("component", "ssh_auth"), slog.String("remote_addr", ctx.RemoteAddr().String()), slog.String("fingerprint", fingerprint))
			return false
		}

		// Log successful authentication
		slog.Debug("successful authentication", slog.String("component", "ssh_auth"), slog.Int("user_id", userCredential.UserID), slog.String("credential_name", userCredential.CredentialName), slog.String("remote_addr", ctx.RemoteAddr().String()))

		// Store user information in SSH context for use by handlers
		ctx.SetValue("authenticated", true)
		ctx.SetValue("user_id", userCredential.UserID)
		ctx.SetValue("credential_id", userCredential.ID)
		ctx.SetValue("credential_name", userCredential.CredentialName)
		ctx.SetValue("user_email", userCredential.Email)
		ctx.SetValue("user_username", userCredential.Username)
		ctx.SetValue("user_first_name", userCredential.FirstName)
		ctx.SetValue("user_last_name", userCredential.LastName)

		// Update last used timestamp in background with timeout
		// Capture credential info for the goroutine to avoid closure issues
		credentialID := userCredential.ID
		go func() {
			// Create a timeout context to prevent indefinite hanging
			ctx, cancel := context.WithTimeout(context.Background(), backgroundUpdateTimeout)
			defer cancel()

			// Convert string ID to int for legacy SSH credentials
			credID, err := strconv.Atoi(credentialID)
			if err != nil {
				slog.Error("invalid credential ID format", slog.String("component", "ssh_auth"), slog.String("credential_id", credentialID))
				return
			}

			// Run the update with timeout monitoring
			done := make(chan error, 1)
			go func() {
				done <- m.sshAuthService.UpdateLastUsed(credID)
			}()

			select {
			case err := <-done:
				if err != nil {
					slog.Error("failed to update last_used_at for credential", slog.String("component", "ssh_auth"), slog.String("credential_id", credentialID), slog.Any("error", err))
				}
			case <-ctx.Done():
				slog.Warn("timeout updating last_used_at for credential", slog.String("component", "ssh_auth"), slog.String("credential_id", credentialID))
			}
		}()

		return true
	}
}

// convertPublicKeyToString converts an ssh.PublicKey to the standard string format
func (m *SSHAuthMiddleware) convertPublicKeyToString(key ssh.PublicKey) (string, error) {
	// Use the standard SSH marshaling to get the authorized key format
	keyStr := strings.TrimSpace(string(gossh.MarshalAuthorizedKey(key)))

	// Split and rejoin to ensure clean format (removes any comments)
	parts := strings.Fields(keyStr)
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid key format after conversion")
	}

	// Return normalized format: "type data" (without comment)
	return fmt.Sprintf("%s %s", parts[0], parts[1]), nil
}

// GetAuthenticatedUserID extracts the authenticated user ID from SSH context
func GetAuthenticatedUserID(ctx ssh.Context) (int, bool) {
	if userID, ok := ctx.Value("user_id").(int); ok {
		return userID, true
	}
	return 0, false
}

// GetCredentialInfo extracts credential information from SSH context
func GetCredentialInfo(ctx ssh.Context) (credentialID int, credentialName string, ok bool) {
	credID, hasCredID := ctx.Value("credential_id").(int)
	credName, hasCredName := ctx.Value("credential_name").(string)

	if hasCredID && hasCredName {
		return credID, credName, true
	}
	return 0, "", false
}

// GetUserInfo extracts all user information from SSH context
func GetUserInfo(ctx ssh.Context) (email, username, firstName, lastName string, ok bool) {
	email, hasEmail := ctx.Value("user_email").(string)
	username, hasUsername := ctx.Value("user_username").(string)
	firstName, hasFirstName := ctx.Value("user_first_name").(string)
	lastName, hasLastName := ctx.Value("user_last_name").(string)

	if hasEmail && hasUsername && hasFirstName && hasLastName {
		return email, username, firstName, lastName, true
	}
	return "", "", "", "", false
}

// IsAuthenticated checks if the SSH session is authenticated
func IsAuthenticated(ctx ssh.Context) bool {
	if authenticated, ok := ctx.Value("authenticated").(bool); ok {
		return authenticated
	}
	return false
}
