package aitools

import (
	"context"

	"windshift/internal/auth"
)

type getAgentSkillArgs struct {
	SkillID int `json:"skill_id"`
}

func init() {
	Register(Default, Tool[getAgentSkillArgs]{
		Name: "get_agent_skill", Group: CapabilityReadComment, Access: AccessRead, Risk: RiskLow,
		Scopes:      []string{auth.ScopeAgentSkillsRead},
		Sources:     []string{SourceStandardAgent},
		Description: "Activate a saved skill granted to this Standard run. Read the skill ID from the run's skill index. Returns its immutable body including reviewed page snapshots, not current workspace library content.",
		Run: func(_ context.Context, env *Env, args getAgentSkillArgs) (any, error) {
			if env.RunID <= 0 || env.Source != SourceStandardAgent {
				return map[string]string{"error": "skill not found"}, nil
			}
			if skill := env.Grants.SkillFor(args.SkillID); skill != nil {
				// Older persisted snapshots can contain unavailable grants.
				if skill.Error != "" {
					return map[string]string{"error": "skill activation unavailable"}, nil
				}
				return *skill, nil
			}
			return map[string]string{"error": "skill not found"}, nil
		},
	})
}
