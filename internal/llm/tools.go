package llm

// BuiltinTools used to host hand-written ToolDefinition entries for every
// agent tool. They've all migrated to internal/aitools/ — the handler
// layer's BuildLLMTools() now sources the full list from that registry
// directly. This stub remains so existing call sites (and external tests)
// keep compiling; new tools should not be added here.
func BuiltinTools() []ToolDefinition {
	return []ToolDefinition{}
}
