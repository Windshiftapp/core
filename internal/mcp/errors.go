package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// toolError returns a CallToolResult representing an error.
// Returns 3 values to match ToolHandlerFor signature.
func toolError(msg string) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
		IsError: true,
	}, nil, nil
}

// toolErrorf returns a formatted tool error.
func toolErrorf(format string, args ...any) (*mcp.CallToolResult, any, error) {
	return toolError(fmt.Sprintf(format, args...))
}

// toolJSON returns a CallToolResult with JSON-encoded data.
func toolJSON(v any) (*mcp.CallToolResult, any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return toolErrorf("failed to marshal response: %v", err)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
	}, nil, nil
}
