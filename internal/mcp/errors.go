package mcp

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// toolError returns a CallToolResult representing an error.
// Returns 3 values to match ToolHandlerFor signature so call sites can
// pass through to the SDK's typed-handler return shape uniformly.
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
