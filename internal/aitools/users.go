package aitools

import (
	"context"
	"sort"
	"strings"

	"windshift/internal/auth"
	"windshift/internal/repository"
	"windshift/internal/services"
)

// findUsersMaxResults caps find_users output so a broad query can't flood
// the model's context with the whole user directory.
const findUsersMaxResults = 20

type findUsersArgs struct {
	Query          string `json:"query" jsonschema:"Case-insensitive substring matched against user names and emails"`
	WorkspaceID    *int   `json:"workspace_id,omitempty" jsonschema:"Optional workspace ID. If set, only that workspace's members are searched and their workspace roles are included."`
	AssignableOnly bool   `json:"assignable_only,omitempty" jsonschema:"Search the workspace's assignee-picker roster; requires workspace_id"`
}

type foundUserDTO struct {
	ID       int      `json:"id"`
	Name     string   `json:"name"`
	Username string   `json:"username,omitempty"`
	Email    string   `json:"email,omitempty"`
	Roles    []string `json:"workspace_roles,omitempty"`
}

type findUsersOut struct {
	Users []foundUserDTO `json:"users"`
}

func init() {
	Register(Default, Tool[findUsersArgs]{
		Name:        "find_users",
		Group:       CapabilityUsersApprovals,
		Access:      AccessRead,
		Risk:        RiskLow,
		Scopes:      []string{auth.ScopeUsersRead},
		Description: "Find users by name (case-insensitive substring match; system admins can also match on email). Pass workspace_id to restrict the search to that workspace's members. Returns at most 20 matches — narrow the query if the person you want is missing.",
		Run: func(ctx context.Context, env *Env, args findUsersArgs) (any, error) {
			query := strings.ToLower(strings.TrimSpace(args.Query))
			if query == "" {
				return map[string]string{"error": "query is required"}, nil
			}

			// Email addresses and workspace role assignments are admin-only
			// on the HTTP surfaces (GetAll strips email for non-admins; role
			// assignments are an admin-gated route) — mirror that here, and
			// keep email out of the *match* too so a non-admin can't confirm
			// addresses by substring probing.
			isAdmin, _ := env.PermService.IsSystemAdmin(env.UserID) //nolint:errcheck // fail closed: lookup error means non-admin masking

			var candidates []foundUserDTO
			switch {
			case args.AssignableOnly:
				if args.WorkspaceID == nil || !env.HasWorkspaceAccess(*args.WorkspaceID) {
					return map[string]string{"error": "assignable_only requires an available workspace"}, nil
				}
				roster, err := services.NewWorkspaceUserResolver(env.DB, env.PermService).List(ctx, *args.WorkspaceID)
				if err != nil {
					return nil, err
				}
				for _, user := range roster {
					candidates = append(candidates, foundUserDTO{ID: user.ID, Name: displayName(user.FirstName, user.LastName, user.Username), Username: user.Username})
				}
			case args.WorkspaceID != nil:
				if !env.HasWorkspaceAccess(*args.WorkspaceID) {
					return map[string]string{"error": "workspace not found"}, nil
				}
				assignments, err := repository.NewWorkspaceRoleRepository(env.DB).ListUserAssignments(*args.WorkspaceID)
				if err != nil {
					return nil, err
				}
				// One row per user↔role pair; fold roles into a single entry per user.
				byID := map[int]*foundUserDTO{}
				var order []int
				for _, a := range assignments {
					u, exists := byID[a.UserID]
					if !exists {
						first, last := "", ""
						if a.FirstName != nil {
							first = *a.FirstName
						}
						if a.LastName != nil {
							last = *a.LastName
						}
						u = &foundUserDTO{
							ID:       a.UserID,
							Name:     displayName(first, last, a.Username),
							Username: a.Username,
							Email:    a.Email,
						}
						byID[a.UserID] = u
						order = append(order, a.UserID)
					}
					u.Roles = append(u.Roles, a.RoleName)
				}
				for _, id := range order {
					candidates = append(candidates, *byID[id])
				}
			default:
				users, err := services.NewUserReadService(env.DB).ListAll()
				if err != nil {
					return nil, err
				}
				for _, u := range users {
					candidates = append(candidates, foundUserDTO{
						ID:       u.ID,
						Name:     displayName(u.FirstName, u.LastName, u.Username),
						Username: u.Username,
						Email:    u.Email,
					})
				}
			}

			out := findUsersOut{Users: []foundUserDTO{}}
			for _, c := range candidates {
				matched := strings.Contains(strings.ToLower(c.Name), query) ||
					strings.Contains(strings.ToLower(c.Username), query) ||
					(isAdmin && strings.Contains(strings.ToLower(c.Email), query))
				if !matched {
					continue
				}
				if !isAdmin {
					c.Email = ""
					c.Roles = nil
				}
				out.Users = append(out.Users, c)
			}
			sort.Slice(out.Users, func(i, j int) bool {
				if out.Users[i].Name != out.Users[j].Name {
					return out.Users[i].Name < out.Users[j].Name
				}
				return out.Users[i].ID < out.Users[j].ID
			})
			if len(out.Users) > findUsersMaxResults {
				out.Users = out.Users[:findUsersMaxResults]
			}
			return out, nil
		},
	})
}

// displayName builds a user's full name from first/last, falling back to the
// username when both are empty.
func displayName(first, last, username string) string {
	name := strings.TrimSpace(strings.TrimSpace(first) + " " + strings.TrimSpace(last))
	if name == "" {
		return username
	}
	return name
}
