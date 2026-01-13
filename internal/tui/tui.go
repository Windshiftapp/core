package tui

import (
	"fmt"
	"log/slog"

	"windshift/internal/auth"
	"windshift/internal/middleware"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/ssh"
)

// NewTUIHandler creates a new TUI handler for SSH sessions
func NewTUIHandler(apiURL string, tokenManager *auth.TokenManager) func(s ssh.Session) (tea.Model, []tea.ProgramOption) {
	return func(s ssh.Session) (tea.Model, []tea.ProgramOption) {
		// Extract authenticated user information from SSH context
		var userInfo *UserInfo
		
		var bearerToken string

		if middleware.IsAuthenticated(s.Context()) {
			userID, _ := middleware.GetAuthenticatedUserID(s.Context())
			credentialID, credentialName, _ := middleware.GetCredentialInfo(s.Context())
			email, username, firstName, lastName, _ := middleware.GetUserInfo(s.Context())

			userInfo = &UserInfo{
				UserID:         userID,
				CredentialID:   credentialID,
				CredentialName: credentialName,
				RemoteAddr:     s.RemoteAddr().String(),
				Email:          email,
				Username:       username,
				FirstName:      firstName,
				LastName:       lastName,
			}

			// Create session token for this SSH session
			if tokenManager != nil {
				sessionName := fmt.Sprintf("SSH Session (%s via %s)", username, credentialName)
				token, err := tokenManager.CreateSessionToken(userID, sessionName)
				if err != nil {
					slog.Error("failed to create session token",
						slog.String("component", "tui"),
						slog.Int("user_id", int(userID)),
						slog.Any("error", err))
				} else {
					bearerToken = token
					slog.Debug("created session token",
						slog.String("component", "tui"),
						slog.Int("user_id", int(userID)),
						slog.String("username", username))
				}
			}
		}

		// Create new app instance for each session with token
		model := NewModelWithUserAndToken(apiURL, userInfo, bearerToken)
		
		return model, []tea.ProgramOption{
			tea.WithAltScreen(),
			tea.WithMouseCellMotion(),
		}
	}
}