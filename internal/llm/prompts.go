package llm

import (
	"embed"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

//go:embed prompts/*.txt
var defaultPromptsFS embed.FS

const (
	PromptPlanMyDay          = "plan_my_day"
	PromptCatchMeUp          = "catch_me_up"
	PromptFindSimilar        = "find_similar"
	PromptDecompose          = "decompose"
	PromptReleaseNotes       = "release_notes"
	PromptDependencyAnalysis = "dependency_analysis"
	PromptAIChat             = "ai_chat"
	PromptDailyBriefing      = "daily_briefing"
	PromptSummarizeTestPlan  = "summarize_test_plan"
)

var allPromptNames = []string{
	PromptPlanMyDay,
	PromptCatchMeUp,
	PromptFindSimilar,
	PromptDecompose,
	PromptReleaseNotes,
	PromptDependencyAnalysis,
	PromptAIChat,
	PromptDailyBriefing,
	PromptSummarizeTestPlan,
}

// PromptStore holds AI system prompts, loaded from embedded defaults with
// optional runtime overrides from a directory on disk.
type PromptStore struct {
	prompts map[string]string
}

// NewPromptStore loads embedded defaults, optionally overridden from dir.
func NewPromptStore(overrideDir string) *PromptStore {
	ps := &PromptStore{prompts: make(map[string]string)}

	// Load embedded defaults
	for _, name := range allPromptNames {
		data, err := defaultPromptsFS.ReadFile("prompts/" + name + ".txt")
		if err != nil {
			slog.Error("missing embedded prompt", "feature", name, "error", err)
			continue
		}
		ps.prompts[name] = strings.TrimSpace(string(data))
	}

	// Override from directory if provided
	if overrideDir != "" {
		ps.loadOverrides(overrideDir)
	}

	return ps
}

// Get returns the prompt for the given feature name.
func (ps *PromptStore) Get(feature string) string {
	return ps.prompts[feature]
}

func (ps *PromptStore) loadOverrides(dir string) {
	for _, name := range allPromptNames {
		path := filepath.Join(dir, name+".txt")
		// #nosec G304 -- dir is operator-configured prompt override path; filename is from the closed allPromptNames set
		data, err := os.ReadFile(path)
		if err != nil {
			continue // file doesn't exist — use embedded default
		}
		content := strings.TrimSpace(string(data))
		if content == "" {
			continue
		}
		ps.prompts[name] = content
		slog.Info("AI prompt overridden from file", "feature", name, "path", path)
	}
}
