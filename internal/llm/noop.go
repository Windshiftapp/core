package llm

import "context"

// noopClient is returned when LLM_ENDPOINT is not configured.
// All methods return ErrNotConfigured.
type noopClient struct{}

func (c *noopClient) ChatCompletion(_ context.Context, _ ChatCompletionRequest) (*ChatCompletionResponse, error) {
	return nil, ErrNotConfigured
}

func (c *noopClient) Health(_ context.Context) error {
	return ErrNotConfigured
}

func (c *noopClient) Available() bool {
	return false
}
