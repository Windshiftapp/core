package tui

import (
	"fmt"
	"log/slog"

	"windshift/internal/auth"
	"windshift/internal/middleware"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/ssh"
)

// NewTUIHandler creates a new TUI handler for SSH sessions
func NewTUIHandler(apiURL string, sessionManager *auth.SessionManager) func(s ssh.Session) (tea.Model, []tea.ProgramOption) {
	return func(s ssh.Session) (tea.Model, []tea.ProgramOption) {
		// Extract authenticated user information from SSH context
		var userInfo *UserInfo

		var sessionToken string

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

			// Create a real session for this SSH connection (same as frontend users)
			if sessionManager != nil {
				remoteAddr := s.RemoteAddr().String()
				userAgent := fmt.Sprintf("SSH TUI (%s via %s)", username, credentialName)
				session, err := sessionManager.CreateSession(userID, remoteAddr, userAgent, false)
				if err != nil {
					slog.Error("failed to create session",
						slog.String("component", "tui"),
						slog.Int("user_id", userID),
						slog.Any("error", err))
				} else {
					sessionToken = session.Token
					slog.Debug("created session for SSH TUI",
						slog.String("component", "tui"),
						slog.Int("user_id", userID),
						slog.String("username", username),
						slog.Int("session_id", session.ID))
				}
			}
		}

		// Create new app instance for each session
		model := NewModelWithUserAndToken(apiURL, userInfo, sessionToken)

		return model, []tea.ProgramOption{
			tea.WithAltScreen(),
			tea.WithMouseCellMotion(),
		}
	}
}
