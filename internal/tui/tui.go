// Package tui wires an authenticated SSH session to the Bubble Tea app:
// per-connection token minting/cleanup and construction of the shared
// context + root model. The UI itself lives in the sub-packages (app, core,
// screens, dialog, components, data, styles).
package tui

import (
	"fmt"
	"log/slog"
	"time"

	"windshift/internal/auth"
	"windshift/internal/middleware"
	"windshift/internal/models"
	"windshift/internal/tui/app"
	"windshift/internal/tui/core"
	"windshift/internal/tui/data"
	"windshift/internal/tui/screens/workspaces"
	"windshift/internal/tui/styles"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/ssh"
)

// tuiTokenLifetime caps the SSH-minted API token at 24h, matching the SSH
// session's wish.WithMaxTimeout in main.go. The cleanup goroutine below
// usually deletes the token well before this on connection close.
const tuiTokenLifetime = 24 * time.Hour

// tuiTokenScopes is the minimal v2 permission set the TUI's API client needs:
// workspaces:read covers workspace list/get + workspace-scoped statuses;
// items:read/write covers item CRUD + comments (which live under /items/{id}).
// priorities:read covers the priority list. Time scopes cover project reads and
// worklog creation. No admin or delete scopes.
var tuiTokenScopes = []string{
	"workspaces:read",
	"items:read",
	"items:write",
	"priorities:read",
	"users:read",             // /users/me + /workspaces/{id}/assignable-users (assignee picker)
	"user-preferences:read",  // theme / split / last-workspace persistence
	"user-preferences:write", // (see /users/me/tui-preferences)
	"time:read",
	"time:write",
}

// NewTUIHandler creates a new TUI handler for SSH sessions.
//
// The handler mints a short-lived bearer token for each connection and revokes
// it when the SSH context ends.
func NewTUIHandler(apiURL string, tokenManager *auth.TokenManager) func(s ssh.Session) (tea.Model, []tea.ProgramOption) {
	return func(s ssh.Session) (tea.Model, []tea.ProgramOption) {
		// Extract authenticated user information from SSH context
		var userInfo *data.UserInfo

		var bearerToken string
		var apiTokenIDForCleanup int
		var apiTokenUserIDForCleanup int

		if middleware.IsAuthenticated(s.Context()) {
			userID, _ := middleware.GetAuthenticatedUserID(s.Context())
			credentialID, credentialName, _ := middleware.GetCredentialInfo(s.Context())
			email, username, firstName, lastName, _ := middleware.GetUserInfo(s.Context())

			userInfo = &data.UserInfo{
				UserID:         userID,
				CredentialID:   credentialID,
				CredentialName: credentialName,
				RemoteAddr:     s.RemoteAddr().String(),
				Email:          email,
				Username:       username,
				FirstName:      firstName,
				LastName:       lastName,
			}

			// Mint a short-lived API token for the v2 endpoints. The narrow
			// scope list reflects exactly what the TUI's APIClient calls.
			if tokenManager != nil {
				exp := time.Now().Add(tuiTokenLifetime)
				resp, err := tokenManager.CreateToken(userID, models.APITokenCreate{
					Name:        fmt.Sprintf("ssh-tui:%s", data.SanitizeLine(credentialName)),
					Permissions: tuiTokenScopes,
					ExpiresAt:   &exp,
					IsTemporary: true,
				})
				if err != nil {
					slog.Error("failed to mint api token",
						slog.String("component", "tui"),
						slog.Int("user_id", userID),
						slog.Any("error", err))
				} else {
					bearerToken = resp.Token
					apiTokenIDForCleanup = resp.APIToken.ID
					apiTokenUserIDForCleanup = userID
					slog.Debug("minted bearer token for SSH TUI",
						slog.String("component", "tui"),
						slog.Int("user_id", userID),
						slog.Int("token_id", resp.APIToken.ID),
						slog.String("token_prefix", resp.APIToken.TokenPrefix))
				}
			}

			if apiTokenIDForCleanup != 0 {
				sctx := s.Context()
				go func() {
					<-sctx.Done()
					if err := tokenManager.RevokeToken(apiTokenIDForCleanup, apiTokenUserIDForCleanup); err != nil {
						slog.Warn("failed to revoke SSH api token on disconnect",
							slog.String("component", "tui"),
							slog.Any("error", err))
					}
				}()
			}
		}

		// Create a new app instance for each session: a fresh client, a
		// fresh shared context and a fresh screen stack.
		client := data.NewClient(apiURL)
		if bearerToken != "" {
			client.SetBearerToken(bearerToken)
		}

		defaultTheme := styles.ByName(styles.DefaultTheme)
		ctx := &core.Ctx{
			Styles: styles.New(defaultTheme.Palette),
			Theme:  defaultTheme.Name,
			Client: client,
			User:   userInfo,
			Keys:   core.DefaultKeyMap(),
		}

		model := app.New(ctx, workspaces.New(ctx))

		// Bubble Tea v2 owns alt-screen + mouse mode via the View struct
		// (see app.Model.View). No program options needed for those.
		return model, nil
	}
}
