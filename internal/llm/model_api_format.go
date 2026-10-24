package llm

import "strings"

// EnrichModelsAPIFormat fills in each model's wire protocol when the provider
// routes by model and the catalog didn't carry one. It is idempotent and never
// overwrites an explicit format, so it is safe to call at load time, before
// persisting a refresh, and again on cache read (protecting caches written
// before a model was classified).
func EnrichModelsAPIFormat(providerType ProviderType, models []ModelInfo) {
	for i := range models {
		if models[i].APIFormat != "" {
			continue
		}
		if format := classifyModelAPIFormat(providerType, models[i].ID); format != "" {
			models[i].APIFormat = format
		}
	}
}

// ModelAPIFormat returns the wire protocol a model requires, or "" when the
// provider/connection default applies. It consults the loaded provider seed
// first, then the provider-specific classifier, so dynamically discovered
// models resolve without a refresh-time catalog hint.
func ModelAPIFormat(providerType ProviderType, modelID string) string {
	if p := GetProvider(providerType); p != nil {
		for _, model := range p.Models {
			if model.ID == modelID && model.APIFormat != "" {
				return model.APIFormat
			}
		}
	}
	return classifyModelAPIFormat(providerType, modelID)
}

// classifyModelAPIFormat maps a model id to a protocol for providers that mix
// protocols behind one base URL. Providers with a single protocol return "".
func classifyModelAPIFormat(providerType ProviderType, modelID string) string {
	switch providerType {
	case ProviderType("opencode-go"):
		return classifyOpenCodeGoAPIFormat(modelID)
	default:
		return ""
	}
}

// classifyOpenCodeGoAPIFormat mirrors OpenCode Go's published per-model
// endpoint table. Models whose family is not yet listed fall back to Chat
// Completions, which is the compatibility default for the gateway.
func classifyOpenCodeGoAPIFormat(modelID string) string {
	id := strings.ToLower(strings.TrimSpace(modelID))
	if id == "" {
		return ""
	}
	switch {
	case strings.HasPrefix(id, "grok-"),
		strings.HasPrefix(id, "gpt-"),
		strings.HasPrefix(id, "muse-spark-"):
		return APIContractResponses
	case strings.HasPrefix(id, "claude-"),
		strings.HasPrefix(id, "minimax-"),
		strings.HasPrefix(id, "qwen"):
		return APIContractAnthropic
	default:
		return APIContractChatCompletions
	}
}
