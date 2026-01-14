//go:build test

package sso

import (
	"strings"
	"testing"

	"windshift/internal/handlers/testutils"
)

func TestFindOrCreateUser_AccountLinking_RequiresVerifiedEmail(t *testing.T) {
	tests := []struct {
		name                  string
		existingUserEmail     string
		claimsEmail           string
		claimsEmailVerified   bool
		claimsEmailProvided   bool
		requireVerifiedEmail  bool
		expectError           bool
		expectedErrorContains string
	}{
		{
			name:                 "linking allowed - IdP verified email",
			existingUserEmail:    "verified@example.com",
			claimsEmail:          "verified@example.com",
			claimsEmailVerified:  true,
			claimsEmailProvided:  true,
			requireVerifiedEmail: false,
			expectError:          false,
		},
		{
			name:                  "linking blocked - IdP did not verify email",
			existingUserEmail:     "unverified@example.com",
			claimsEmail:           "unverified@example.com",
			claimsEmailVerified:   false,
			claimsEmailProvided:   true,
			requireVerifiedEmail:  false, // Even without require, the linking check blocks unverified
			expectError:           true,
			expectedErrorContains: "cannot automatically link",
		},
		{
			name:                  "linking blocked - IdP did not provide verification claim",
			existingUserEmail:     "noprovided@example.com",
			claimsEmail:           "noprovided@example.com",
			claimsEmailVerified:   false,
			claimsEmailProvided:   false,
			requireVerifiedEmail:  false,
			expectError:           true,
			expectedErrorContains: "cannot automatically link",
		},
		{
			name:                 "new user - auto-provision allowed without verification",
			existingUserEmail:    "", // No existing user
			claimsEmail:          "newuser@example.com",
			claimsEmailVerified:  false,
			claimsEmailProvided:  false,
			requireVerifiedEmail: false,
			expectError:          false,
		},
		{
			name:                  "new user blocked - require verified email enabled",
			existingUserEmail:     "", // No existing user
			claimsEmail:           "newblocked@example.com",
			claimsEmailVerified:   false,
			claimsEmailProvided:   true, // IdP says NOT verified
			requireVerifiedEmail:  true,
			expectError:           true,
			expectedErrorContains: "not verified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tdb := testutils.CreateTestDB(t, true)
			defer tdb.Close()

			// Create existing user if specified (with unique username based on email)
			if tt.existingUserEmail != "" {
				username := strings.Split(tt.existingUserEmail, "@")[0]
				_, err := tdb.Exec(`
					INSERT INTO users (email, username, first_name, last_name, is_active, email_verified, password_hash)
					VALUES (?, ?, 'Victim', 'User', 1, 0, '')
				`, tt.existingUserEmail, username)
				if err != nil {
					t.Fatalf("Failed to create existing user: %v", err)
				}
			}

			// Create SSO provider
			_, err := tdb.Exec(`
				INSERT INTO sso_providers (slug, name, provider_type, enabled, is_default, auto_provision_users, require_verified_email)
				VALUES ('test', 'Test', 'oidc', 1, 1, 1, ?)
			`, tt.requireVerifiedEmail)
			if err != nil {
				t.Fatalf("Failed to create SSO provider: %v", err)
			}

			userStore := NewUserStore(tdb.GetDatabase())
			provider := &SSOProvider{ID: 1, AutoProvisionUsers: true, RequireVerifiedEmail: tt.requireVerifiedEmail}

			// Use unique username in claims based on email
			claimsUsername := strings.Split(tt.claimsEmail, "@")[0] + "_sso"
			claims := &OIDCClaims{
				Subject:               "external-" + tt.name,
				Email:                 tt.claimsEmail,
				EmailVerified:         tt.claimsEmailVerified,
				EmailVerifiedProvided: tt.claimsEmailProvided,
				Name:                  "Test User",
				Username:              claimsUsername,
			}

			result, err := userStore.FindOrCreateUser(provider, claims)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error containing '%s', got nil", tt.expectedErrorContains)
				} else if !strings.Contains(err.Error(), tt.expectedErrorContains) {
					t.Errorf("Expected error containing '%s', got: %v", tt.expectedErrorContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
				if result == nil || result.User == nil {
					t.Error("Expected user result, got nil")
				}
			}
		})
	}
}

func TestFindOrCreateUser_ExternalAccountAlreadyLinked(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Create existing user
	_, err := tdb.Exec(`
		INSERT INTO users (id, email, username, first_name, last_name, is_active, email_verified, password_hash)
		VALUES (1, 'user@example.com', 'testuser', 'Test', 'User', 1, 1, '')
	`)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Create SSO provider
	_, err = tdb.Exec(`
		INSERT INTO sso_providers (id, slug, name, provider_type, enabled, is_default, auto_provision_users)
		VALUES (1, 'test', 'Test', 'oidc', 1, 1, 1)
	`)
	if err != nil {
		t.Fatalf("Failed to create SSO provider: %v", err)
	}

	// Create existing external account link
	_, err = tdb.Exec(`
		INSERT INTO user_external_accounts (user_id, provider_id, external_id, email, linked_at)
		VALUES (1, 1, 'external-456', 'user@example.com', CURRENT_TIMESTAMP)
	`)
	if err != nil {
		t.Fatalf("Failed to create external account: %v", err)
	}

	userStore := NewUserStore(tdb.GetDatabase())
	provider := &SSOProvider{ID: 1, AutoProvisionUsers: true}

	claims := &OIDCClaims{
		Subject:               "external-456", // Same external ID
		Email:                 "user@example.com",
		EmailVerified:         true,
		EmailVerifiedProvided: true,
	}

	result, err := userStore.FindOrCreateUser(provider, claims)
	if err != nil {
		t.Fatalf("Expected no error for already-linked account, got: %v", err)
	}

	if result.User.ID != 1 {
		t.Errorf("Expected user ID 1, got %d", result.User.ID)
	}

	if result.IsNewUser {
		t.Error("Expected IsNewUser to be false for existing linked account")
	}
}

func TestFindOrCreateUser_AutoProvisionDisabled(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Create SSO provider with auto-provision disabled
	_, err := tdb.Exec(`
		INSERT INTO sso_providers (id, slug, name, provider_type, enabled, is_default, auto_provision_users)
		VALUES (1, 'test', 'Test', 'oidc', 1, 1, 0)
	`)
	if err != nil {
		t.Fatalf("Failed to create SSO provider: %v", err)
	}

	userStore := NewUserStore(tdb.GetDatabase())
	provider := &SSOProvider{ID: 1, AutoProvisionUsers: false}

	claims := &OIDCClaims{
		Subject:               "external-789",
		Email:                 "newuser@example.com",
		EmailVerified:         true,
		EmailVerifiedProvided: true,
	}

	_, err = userStore.FindOrCreateUser(provider, claims)
	if err == nil {
		t.Error("Expected error when auto-provision is disabled")
	}

	if err != ErrAutoProvisionDisabled {
		t.Errorf("Expected ErrAutoProvisionDisabled, got: %v", err)
	}
}
