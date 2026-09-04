package v2

import (
	"errors"
	"net/http"
	"sort"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

func registerAdminRoutes(builder *routeBuilder, deps Deps) {
	builder.SessionPage("/admin/groups", listAdminGroups(deps))
	builder.SessionRead("/admin/groups/{group_id}", getAdminGroup(deps))
	builder.SessionJSON(http.MethodPost, "/admin/groups", http.StatusCreated, false, createAdminGroup(deps))
	builder.SessionJSON(http.MethodPatch, "/admin/groups/{group_id}", http.StatusOK, true, updateAdminGroup(deps))
	builder.SessionCommand(http.MethodDelete, "/admin/groups/{group_id}", deleteAdminGroup(deps))
	builder.SessionPage("/admin/users", listAdminUsers(deps))
	builder.SessionRead("/admin/users/{user_id}", getAdminUser(deps))
	builder.SessionJSON(http.MethodPatch, "/admin/users/{user_id}", http.StatusOK, true, updateAdminUser(deps))
}

type adminGroupCreateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type adminGroupPatchRequest struct {
	Name        Optional[string] `json:"name"`
	Description Optional[string] `json:"description"`
	IsActive    Optional[bool]   `json:"is_active"`
	MemberIDs   Optional[[]int]  `json:"member_ids"`
}

type adminGroupDTO struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsActive    bool      `json:"is_active"`
	MemberCount int       `json:"member_count"`
	MemberIDs   []int     `json:"member_ids"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Warnings    []string  `json:"warnings,omitempty"`
}

type adminUserPatchRequest struct {
	Email     Optional[string] `json:"email"`
	Username  Optional[string] `json:"username"`
	FirstName Optional[string] `json:"first_name"`
	LastName  Optional[string] `json:"last_name"`
	AvatarURL Optional[string] `json:"avatar_url"`
	Timezone  Optional[string] `json:"timezone"`
	Language  Optional[string] `json:"language"`
	IsActive  Optional[bool]   `json:"is_active"`
}

type adminUserDTO struct {
	ID                    int       `json:"id"`
	Email                 string    `json:"email"`
	Username              string    `json:"username"`
	FirstName             string    `json:"first_name"`
	LastName              string    `json:"last_name"`
	FullName              string    `json:"full_name"`
	IsActive              bool      `json:"is_active"`
	RequiresPasswordReset bool      `json:"requires_password_reset"`
	IsAgent               bool      `json:"is_agent"`
	AgentOwnerUserID      *int      `json:"agent_owner_user_id"`
	AgentOwnerName        string    `json:"agent_owner_name"`
	AvatarURL             string    `json:"avatar_url"`
	Timezone              string    `json:"timezone"`
	Language              string    `json:"language"`
	GroupIDs              []int     `json:"group_ids"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

func listAdminGroups(deps Deps) pageOperation[adminGroupDTO] {
	return func(r *http.Request) ([]adminGroupDTO, Pagination, int, error) {
		if _, err := requireSystemAdmin(r, deps); err != nil {
			return nil, Pagination{}, 0, err
		}
		page, err := ParsePage(r)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		groups, total, err := deps.Groups.ListPage(page.PageSize, page.Offset)
		if err != nil {
			return nil, Pagination{}, 0, internalError(err)
		}
		result := make([]adminGroupDTO, len(groups))
		for i := range groups {
			result[i] = adminGroupFromModel(&groups[i], nil)
		}
		return result, page, total, nil
	}
}

func getAdminGroup(deps Deps) readOperation[adminGroupDTO] {
	return func(r *http.Request) (adminGroupDTO, error) {
		if _, err := requireSystemAdmin(r, deps); err != nil {
			return adminGroupDTO{}, err
		}
		groupID, err := pathID(r, "group_id")
		if err != nil {
			return adminGroupDTO{}, err
		}
		group, err := deps.Groups.Get(groupID, true)
		if err != nil {
			return adminGroupDTO{}, adminGroupError(err)
		}
		return adminGroupFromModel(group, nil), nil
	}
}

func createAdminGroup(deps Deps) jsonOperation[adminGroupCreateRequest, adminGroupDTO] {
	return func(r *http.Request, input adminGroupCreateRequest) (adminGroupDTO, error) {
		user, err := requireSystemAdmin(r, deps)
		if err != nil {
			return adminGroupDTO{}, err
		}
		result, err := deps.Groups.Create(input.Name, input.Description, user.ID)
		if err != nil {
			return adminGroupDTO{}, adminGroupError(err)
		}
		return adminGroupFromModel(result.Group, result.Warnings), nil
	}
}

func updateAdminGroup(deps Deps) jsonOperation[adminGroupPatchRequest, adminGroupDTO] {
	return func(r *http.Request, input adminGroupPatchRequest) (adminGroupDTO, error) {
		if input.Name.Null || input.Description.Null || input.IsActive.Null || input.MemberIDs.Null {
			return adminGroupDTO{}, newError(http.StatusBadRequest, "invalid_request", "Group fields cannot be null")
		}
		actor, err := requireSystemAdmin(r, deps)
		if err != nil {
			return adminGroupDTO{}, err
		}
		groupID, err := pathID(r, "group_id")
		if err != nil {
			return adminGroupDTO{}, err
		}
		result, err := deps.Groups.Update(groupID, services.GroupUpdateInput{
			Name: optionalValue(input.Name), Description: optionalValue(input.Description), IsActive: optionalValue(input.IsActive),
			MemberIDs: optionalValue(input.MemberIDs), ActorID: &actor.ID,
		})
		if err != nil {
			return adminGroupDTO{}, adminGroupError(err)
		}
		group, err := deps.Groups.Get(groupID, true)
		if err != nil {
			return adminGroupDTO{}, adminGroupError(err)
		}
		return adminGroupFromModel(group, result.Warnings), nil
	}
}

func deleteAdminGroup(deps Deps) commandOperation {
	return func(r *http.Request) error {
		if _, err := requireSystemAdmin(r, deps); err != nil {
			return err
		}
		groupID, err := pathID(r, "group_id")
		if err != nil {
			return err
		}
		_, err = deps.Groups.Delete(groupID)
		return adminGroupError(err)
	}
}

func listAdminUsers(deps Deps) pageOperation[adminUserDTO] {
	return func(r *http.Request) ([]adminUserDTO, Pagination, int, error) {
		if _, err := requireSystemAdmin(r, deps); err != nil {
			return nil, Pagination{}, 0, err
		}
		page, err := ParsePage(r)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		users, total, err := deps.AdminUsers.ListAdmin(services.PaginationParams{Limit: page.PageSize, Offset: page.Offset})
		if err != nil {
			return nil, Pagination{}, 0, internalError(err)
		}
		result := make([]adminUserDTO, len(users))
		for i := range users {
			result[i], err = adminUserFromModel(deps.AdminUsers, &users[i])
			if err != nil {
				return nil, Pagination{}, 0, err
			}
		}
		return result, page, total, nil
	}
}

func getAdminUser(deps Deps) readOperation[adminUserDTO] {
	return func(r *http.Request) (adminUserDTO, error) {
		if _, err := requireSystemAdmin(r, deps); err != nil {
			return adminUserDTO{}, err
		}
		userID, err := pathID(r, "user_id")
		if err != nil {
			return adminUserDTO{}, err
		}
		user, err := deps.AdminUsers.GetByID(userID)
		if err != nil {
			return adminUserDTO{}, adminUserError(err)
		}
		return adminUserFromModel(deps.AdminUsers, user)
	}
}

func updateAdminUser(deps Deps) jsonOperation[adminUserPatchRequest, adminUserDTO] {
	return func(r *http.Request, input adminUserPatchRequest) (adminUserDTO, error) {
		if input.Email.Null || input.Username.Null || input.FirstName.Null || input.LastName.Null ||
			input.Timezone.Null || input.Language.Null || input.IsActive.Null {
			return adminUserDTO{}, newError(http.StatusBadRequest, "invalid_request", "Only avatar_url may be null")
		}
		actor, err := requireSystemAdmin(r, deps)
		if err != nil {
			return adminUserDTO{}, err
		}
		userID, err := pathID(r, "user_id")
		if err != nil {
			return adminUserDTO{}, err
		}
		if userID == actor.ID && input.IsActive.Set && !input.IsActive.Value {
			return adminUserDTO{}, newError(http.StatusConflict, "conflict", "You cannot deactivate your own account")
		}
		result, err := deps.AdminUsers.UpdateAdmin(userID, services.AdminUserUpdate{
			Email: optionalValue(input.Email), Username: optionalValue(input.Username),
			FirstName: optionalValue(input.FirstName), LastName: optionalValue(input.LastName),
			AvatarURL: optionalNullableString(input.AvatarURL), Timezone: optionalValue(input.Timezone),
			Language: optionalValue(input.Language), IsActive: optionalValue(input.IsActive),
		})
		if err != nil {
			return adminUserDTO{}, adminUserError(err)
		}
		return adminUserFromModel(deps.AdminUsers, result.User)
	}
}

func requireSystemAdmin(r *http.Request, deps Deps) (*models.User, error) {
	user, err := principal(r)
	if err != nil {
		return nil, err
	}
	allowed, err := deps.SystemAdmins.IsSystemAdmin(user.ID)
	if err != nil {
		return nil, internalError(err)
	}
	if !allowed {
		return nil, newError(http.StatusForbidden, "insufficient_permission", "System administrator permission is required")
	}
	return user, nil
}

func optionalNullableString(value Optional[string]) *string {
	if !value.Set {
		return nil
	}
	if value.Null {
		empty := ""
		return &empty
	}
	return &value.Value
}

func adminGroupFromModel(group *models.TeamGroup, warnings []string) adminGroupDTO {
	memberIDs := make([]int, len(group.Members))
	for i, member := range group.Members {
		memberIDs[i] = member.UserID
	}
	sort.Ints(memberIDs)
	return adminGroupDTO{
		ID: group.ID, Name: group.Name, Description: group.Description, IsActive: group.IsActive,
		MemberCount: group.MemberCount, MemberIDs: memberIDs, CreatedAt: group.CreatedAt,
		UpdatedAt: group.UpdatedAt, Warnings: warnings,
	}
}

func adminUserFromModel(application adminUserApplication, user *models.User) (adminUserDTO, error) {
	groupIDs, err := application.GetGroupIDs(user.ID)
	if err != nil {
		return adminUserDTO{}, internalError(err)
	}
	sort.Ints(groupIDs)
	return adminUserDTO{
		ID: user.ID, Email: user.Email, Username: user.Username, FirstName: user.FirstName,
		LastName: user.LastName, FullName: user.FullName, IsActive: user.IsActive,
		RequiresPasswordReset: user.RequiresPasswordReset, IsAgent: user.IsAgent,
		AgentOwnerUserID: user.AgentOwnerUserID, AgentOwnerName: user.AgentOwnerName, AvatarURL: user.AvatarURL,
		Timezone: user.Timezone, Language: user.Language, GroupIDs: groupIDs, CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}, nil
}

func adminGroupError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, repository.ErrNotFound):
		return newError(http.StatusNotFound, "not_found", "Group was not found")
	case errors.Is(err, services.ErrGroupNameRequired), errors.Is(err, services.ErrGroupNoFields):
		return newError(http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, services.ErrGroupDuplicate):
		return newError(http.StatusConflict, "conflict", err.Error())
	case errors.Is(err, services.ErrGroupManaged), errors.Is(err, services.ErrGroupSystem):
		return newError(http.StatusForbidden, "insufficient_permission", err.Error())
	default:
		return internalError(err)
	}
}

func adminUserError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, repository.ErrNotFound), errors.Is(err, services.ErrUserNotFound):
		return newError(http.StatusNotFound, "not_found", "User was not found")
	case errors.Is(err, services.ErrUserManagedExternally):
		return newError(http.StatusForbidden, "insufficient_permission", err.Error())
	case errors.Is(err, services.ErrUserEmailInvalid):
		return newError(http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, services.ErrUserEmailExists), errors.Is(err, services.ErrUserUsernameExists):
		return newError(http.StatusConflict, "conflict", err.Error())
	default:
		return internalError(err)
	}
}
