package middleware

import (
	"fmt"
	"log/slog"
	"strings"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/services"

	"github.com/charmbracelet/ssh"
	gossh "golang.org/x/crypto/ssh"
)

// SSHAuthMiddleware provides SSH public key authentication
type SSHAuthMiddleware struct {
	db             database.Database
	sshAuthService *services.SSHAuthService
}

// NewSSHAuthMiddleware creates a new SSH authentication middleware
func NewSSHAuthMiddleware(db database.Database) *SSHAuthMiddleware {
	return &SSHAuthMiddleware{
		db:             db,
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

		remoteAddr := ctx.RemoteAddr().String()

		if userCredential == nil {
			// Log the failed attempt with key fingerprint for security monitoring
			fingerprint := gossh.FingerprintSHA256(key)
			slog.Warn("unauthorized key attempt", slog.String("component", "ssh_auth"), slog.String("remote_addr", remoteAddr), slog.String("fingerprint", fingerprint))
			_ = logger.LogAudit(m.db, logger.AuditEvent{
				IPAddress:    remoteAddr,
				ActionType:   logger.ActionLoginFailure,
				ResourceType: logger.ResourceUser,
				Details: map[string]interface{}{
					"auth_method": "ssh",
					"fingerprint": fingerprint,
				},
				Success:      false,
				ErrorMessage: "unauthorized ssh key",
			})
			return false
		}

		// Log successful authentication
		slog.Debug("successful authentication", slog.String("component", "ssh_auth"), slog.Int("user_id", userCredential.UserID), slog.String("credential_name", userCredential.CredentialName), slog.String("remote_addr", remoteAddr))
		_ = logger.LogAudit(m.db, logger.AuditEvent{
			UserID:       userCredential.UserID,
			Username:     userCredential.Username,
			IPAddress:    remoteAddr,
			ActionType:   logger.ActionLoginSuccess,
			ResourceType: logger.ResourceUser,
			Details: map[string]interface{}{
				"auth_method":     "ssh",
				"credential_name": userCredential.CredentialName,
			},
			Success: true,
		})

		// Store user information in SSH context for use by handlers
		ctx.SetValue("authenticated", true)
		ctx.SetValue("user_id", userCredential.UserID)
		ctx.SetValue("credential_id", userCredential.ID)
		ctx.SetValue("credential_name", userCredential.CredentialName)
		ctx.SetValue("user_email", userCredential.Email)
		ctx.SetValue("user_username", userCredential.Username)
		ctx.SetValue("user_first_name", userCredential.FirstName)
		ctx.SetValue("user_last_name", userCredential.LastName)

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

// GetCredentialInfo extracts credential information from SSH context.
// credential_id is stored as a string to accommodate both legacy integer IDs
// and string-based IDs (e.g. WebAuthn); see models.UserCredential.
func GetCredentialInfo(ctx ssh.Context) (credentialID, credentialName string, ok bool) {
	credID, hasCredID := ctx.Value("credential_id").(string)
	credName, hasCredName := ctx.Value("credential_name").(string)

	if hasCredID && hasCredName {
		return credID, credName, true
	}
	return "", "", false
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
