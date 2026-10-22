package llm

import (
	"encoding/json"
	"fmt"
	"strings"
)

// providerConfigVisionModeKey is the reserved provider_config key holding the
// per-connection vision override. It is windshift-private — see
// reservedProviderConfigKeys.
const (
	providerConfigVisionModeKey      = "vision_mode"
	providerConfigAPIContractKey     = "api_contract"
	providerConfigReasoningKey       = "reasoning"
	providerConfigMaxOutputTokensKey = "max_output_tokens"

	APIContractAuto            = "auto"
	APIContractResponses       = "responses"
	APIContractChatCompletions = "chat_completions"
	APIContractAnthropic       = "anthropic"
)

// reservedProviderConfigKeys are provider_config keys that windshift interprets
// itself and must NOT be merged into the outbound provider request body. The
// blob is otherwise pass-through (MergeProviderConfig*), so without this guard a
// key like vision_mode would be sent to OpenAI/OpenRouter as an unknown request
// field and could be rejected.
var reservedProviderConfigKeys = map[string]bool{
	providerConfigVisionModeKey:      true,
	providerConfigAPIContractKey:     true,
	providerConfigReasoningKey:       true,
	providerConfigMaxOutputTokensKey: true,
}

// ResponsesReasoningConfig contains the Responses reasoning controls that
// Windshift accepts from a connection's provider_config. It is deliberately
// limited to the request fields used by the Responses API so the private
// connection setting cannot leak to another provider protocol.
type ResponsesReasoningConfig struct {
	Effort       string `json:"effort,omitempty"`
	Summary      string `json:"summary,omitempty"`
	BudgetTokens int64  `json:"budget_tokens,omitempty"`
	// Enabled is supported by OpenAI-compatible Chat Completions providers,
	// but not by the Responses or Anthropic contracts.
	Enabled *bool `json:"enabled,omitempty"`
}

// ValidateProviderConfig verifies the per-connection provider_config blob.
// It is intentionally generic: any provider can use it, but it must be a JSON
// object because it is merged into the provider request body. Reserved
// windshift keys (e.g. vision_mode) are additionally value-validated.
func ValidateProviderConfig(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var cfg map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return fmt.Errorf("provider_config must be valid JSON: %w", err)
	}
	if cfg == nil {
		return fmt.Errorf("provider_config must be a JSON object")
	}
	if rawMode, ok := cfg[providerConfigVisionModeKey]; ok {
		var mode string
		if err := json.Unmarshal(rawMode, &mode); err != nil {
			return fmt.Errorf("provider_config.vision_mode must be a string")
		}
		if !isValidVisionMode(strings.ToLower(strings.TrimSpace(mode))) {
			return fmt.Errorf("provider_config.vision_mode must be one of auto, on, off")
		}
	}
	if rawContract, ok := cfg[providerConfigAPIContractKey]; ok {
		var contract string
		if err := json.Unmarshal(rawContract, &contract); err != nil {
			return fmt.Errorf("provider_config.api_contract must be a string")
		}
		if !isValidAPIContract(strings.ToLower(strings.TrimSpace(contract))) {
			return fmt.Errorf("provider_config.api_contract must be one of auto, responses, chat_completions")
		}
	}
	if rawReasoning, ok := cfg[providerConfigReasoningKey]; ok {
		var reasoningObject map[string]json.RawMessage
		if err := json.Unmarshal(rawReasoning, &reasoningObject); err != nil || reasoningObject == nil {
			return fmt.Errorf("provider_config.reasoning must be an object with string effort and summary fields")
		}
		var reasoning ResponsesReasoningConfig
		if err := json.Unmarshal(rawReasoning, &reasoning); err != nil {
			return fmt.Errorf("provider_config.reasoning must contain valid effort, summary, budget_tokens, and enabled fields")
		}
		if reasoning.BudgetTokens < 0 {
			return fmt.Errorf("provider_config.reasoning.budget_tokens must be non-negative")
		}
		for key := range reasoningObject {
			switch key {
			case "effort", "summary", "budget_tokens", "enabled":
			default:
				return fmt.Errorf("provider_config.reasoning.%s is unsupported", key)
			}
		}
	}
	if rawMax, ok := cfg[providerConfigMaxOutputTokensKey]; ok {
		var value int
		if err := json.Unmarshal(rawMax, &value); err != nil {
			return fmt.Errorf("provider_config.max_output_tokens must be an integer")
		}
		if value < 0 {
			return fmt.Errorf("provider_config.max_output_tokens must be non-negative")
		}
	}
	return nil
}

// ProviderConfigMaxOutputTokens returns the per-connection output-token
// override, or 0 when absent or invalid. A positive value replaces the model
// catalog's output limit so an operator can force a larger (or smaller) cap for
// a model the catalog describes incorrectly or does not describe at all.
func ProviderConfigMaxOutputTokens(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	var cfg map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return 0
	}
	rawMax, ok := cfg[providerConfigMaxOutputTokensKey]
	if !ok {
		return 0
	}
	var value int
	if err := json.Unmarshal(rawMax, &value); err != nil || value < 0 {
		return 0
	}
	return value
}

// ProviderConfigAPIContract returns the requested generation wire contract.
// Auto lets the provider catalog choose while keeping custom OpenAI-compatible
// gateways on Chat Completions for backward compatibility.
func ProviderConfigAPIContract(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return APIContractAuto
	}
	var cfg map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return APIContractAuto
	}
	rawContract, ok := cfg[providerConfigAPIContractKey]
	if !ok {
		return APIContractAuto
	}
	var contract string
	if err := json.Unmarshal(rawContract, &contract); err != nil {
		return APIContractAuto
	}
	contract = strings.ToLower(strings.TrimSpace(contract))
	if !isValidAPIContract(contract) {
		return APIContractAuto
	}
	return contract
}

func isValidAPIContract(contract string) bool {
	switch contract {
	case APIContractAuto, APIContractResponses, APIContractChatCompletions:
		return true
	default:
		return false
	}
}

// ProviderConfigVisionMode extracts the vision override from a provider_config
// blob, returning the normalized mode (auto/on/off). It defaults to "auto" for
// an absent key, empty/invalid blob, or unparseable value — auto is the safe
// default (defer to the model's curated capability).
func ProviderConfigVisionMode(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return VisionModeAuto
	}
	var cfg map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return VisionModeAuto
	}
	rawMode, ok := cfg[providerConfigVisionModeKey]
	if !ok {
		return VisionModeAuto
	}
	var mode string
	if err := json.Unmarshal(rawMode, &mode); err != nil {
		return VisionModeAuto
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	if !isValidVisionMode(mode) {
		return VisionModeAuto
	}
	return mode
}

// ProviderConfigResponsesReasoning extracts Responses reasoning controls from
// provider_config. Invalid or incomplete input deliberately falls back to nil,
// matching the permissive read behavior of the other provider config accessors.
func ProviderConfigResponsesReasoning(raw string) *ResponsesReasoningConfig {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var cfg map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil
	}
	rawReasoning, ok := cfg[providerConfigReasoningKey]
	if !ok {
		return nil
	}
	var reasoning ResponsesReasoningConfig
	if err := json.Unmarshal(rawReasoning, &reasoning); err != nil {
		return nil
	}
	reasoning.Effort = strings.TrimSpace(reasoning.Effort)
	reasoning.Summary = strings.TrimSpace(reasoning.Summary)
	if reasoning.Effort == "" && reasoning.Summary == "" && reasoning.BudgetTokens == 0 && reasoning.Enabled == nil {
		return nil
	}
	return &reasoning
}
