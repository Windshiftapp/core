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
	providerConfigReasoningLevelKey  = "reasoning_level"
	providerConfigMaxOutputTokensKey = "max_output_tokens"

	APIContractAuto            = "auto"
	APIContractResponses       = "responses"
	APIContractChatCompletions = "chat_completions"
	APIContractAnthropic       = "anthropic"
)

// Normalized reasoning levels accepted in provider_config.reasoning_level.
// ReasoningLevelDefault leaves the provider's own reasoning behavior in place.
const (
	ReasoningLevelDefault = "default"
	ReasoningLevelLow     = "low"
	ReasoningLevelMedium  = "medium"
	ReasoningLevelHigh    = "high"
)

// reservedProviderConfigKeys are provider_config keys that windshift interprets
// itself and must NOT be merged into the outbound provider request body. The
// blob is otherwise pass-through (MergeProviderConfig*), so without this guard a
// key like vision_mode would be sent to OpenAI/OpenRouter as an unknown request
// field and could be rejected.
var reservedProviderConfigKeys = map[string]bool{
	providerConfigVisionModeKey:      true,
	providerConfigAPIContractKey:     true,
	providerConfigReasoningLevelKey:  true,
	providerConfigMaxOutputTokensKey: true,
}

// reasoningOverrideKeys are provider-native reasoning fields. When an operator
// sets any of them the value is forwarded verbatim and takes precedence over the
// normalized reasoning_level setting.
var reasoningOverrideKeys = []string{"reasoning", "reasoning_effort", "thinking"}

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
	if rawLevel, ok := cfg[providerConfigReasoningLevelKey]; ok {
		var level string
		if err := json.Unmarshal(rawLevel, &level); err != nil {
			return fmt.Errorf("provider_config.reasoning_level must be a string")
		}
		if !isValidReasoningLevel(strings.ToLower(strings.TrimSpace(level))) {
			return fmt.Errorf("provider_config.reasoning_level must be one of default, low, medium, high")
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

func isValidReasoningLevel(level string) bool {
	switch level {
	case ReasoningLevelDefault, ReasoningLevelLow, ReasoningLevelMedium, ReasoningLevelHigh:
		return true
	default:
		return false
	}
}

// ProviderConfigReasoningLevel returns the normalized reasoning level
// (low/medium/high), or "" when the operator left it at the provider default.
// Invalid or unparseable input deliberately falls back to "", matching the
// permissive read behavior of the other provider config accessors.
func ProviderConfigReasoningLevel(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var cfg map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return ""
	}
	rawLevel, ok := cfg[providerConfigReasoningLevelKey]
	if !ok {
		return ""
	}
	var level string
	if err := json.Unmarshal(rawLevel, &level); err != nil {
		return ""
	}
	level = strings.ToLower(strings.TrimSpace(level))
	if !isValidReasoningLevel(level) || level == ReasoningLevelDefault {
		return ""
	}
	return level
}

// ProviderConfigHasReasoningOverride reports whether the operator supplied a
// provider-native reasoning field. Such a field is forwarded verbatim and takes
// precedence over the normalized reasoning_level.
func ProviderConfigHasReasoningOverride(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	var cfg map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return false
	}
	for _, key := range reasoningOverrideKeys {
		if _, ok := cfg[key]; ok {
			return true
		}
	}
	return false
}

// providerConfigReasoningOverrideBody returns the operator-supplied
// provider-native reasoning fields. Protocols that take generic provider_config
// passthrough (Responses, Chat Completions) forward them through the request
// body already; this accessor lets the Anthropic path forward them verbatim too.
func providerConfigReasoningOverrideBody(raw string) map[string]any {
	var cfg map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &cfg) != nil {
		return nil
	}
	body := map[string]any{}
	for _, key := range reasoningOverrideKeys {
		rawValue, ok := cfg[key]
		if !ok {
			continue
		}
		var value any
		if json.Unmarshal(rawValue, &value) == nil {
			body[key] = value
		}
	}
	if len(body) == 0 {
		return nil
	}
	return body
}
