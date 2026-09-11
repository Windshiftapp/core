package v2

import (
	"net/http"
	"time"

	"windshift/internal/contextkeys"
	"windshift/internal/models"
)

type userReader interface {
	GetByID(int) (*models.User, error)
}

type userDTO struct {
	ID            int     `json:"id"`
	Email         string  `json:"email"`
	Username      string  `json:"username"`
	FirstName     string  `json:"first_name"`
	LastName      string  `json:"last_name"`
	FullName      string  `json:"full_name"`
	IsActive      bool    `json:"is_active"`
	IsAgent       bool    `json:"is_agent"`
	AgentPresence string  `json:"agent_presence"`
	AvatarURL     *string `json:"avatar_url"`
	Timezone      string  `json:"timezone"`
	Language      string  `json:"language"`
	CreatedAt     string  `json:"created_at"`
}

func getCurrentUser(users userReader) readOperation[userDTO] {
	return func(r *http.Request) (userDTO, error) {
		principal, ok := r.Context().Value(contextkeys.User).(*models.User)
		if !ok || principal == nil {
			return userDTO{}, newError(http.StatusUnauthorized, "authentication_required", "Authentication is required")
		}
		user, err := users.GetByID(principal.ID)
		if err != nil {
			return userDTO{}, internalError(err)
		}
		return mapUser(user), nil
	}
}

func mapUser(user *models.User) userDTO {
	var avatarURL *string
	if user.AvatarURL != "" {
		value := user.AvatarURL
		avatarURL = &value
	}
	return userDTO{
		ID:            user.ID,
		Email:         user.Email,
		Username:      user.Username,
		FirstName:     user.FirstName,
		LastName:      user.LastName,
		FullName:      user.FullName,
		IsActive:      user.IsActive,
		IsAgent:       user.IsAgent,
		AgentPresence: user.AgentPresence,
		AvatarURL:     avatarURL,
		Timezone:      user.Timezone,
		Language:      user.Language,
		CreatedAt:     user.CreatedAt.UTC().Format(time.RFC3339),
	}
}
