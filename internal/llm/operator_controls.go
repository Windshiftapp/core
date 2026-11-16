package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"windshift/internal/operatorcontrols"
)

type controlledClient struct {
	Client
	controls *operatorcontrols.Controls
	model    string
}

// WithOperatorControls protects every call in a tool loop, not just its first
// HTTP request. The fallback client and runner broker use this same boundary.
func WithOperatorControls(client Client, controls *operatorcontrols.Controls, model string) Client {
	if controls == nil || client == nil {
		return client
	}
	if existing, ok := client.(*controlledClient); ok && existing.controls == controls {
		return client
	}
	return &controlledClient{Client: client, controls: controls, model: model}
}

func (c *controlledClient) Available() bool {
	return c.controls.Allowed(operatorcontrols.AI) && c.Client.Available()
}

func (c *controlledClient) Health(ctx context.Context) error {
	if err := c.controls.Check(operatorcontrols.AI); err != nil {
		return err
	}
	if !c.controls.Active() {
		return c.Client.Health(ctx)
	}
	request := ConnectionProbeRequest()
	request.Model = c.model
	_, err := c.Complete(ctx, request)
	if err != nil && !isCompletionTokenLimitError(err) {
		return fmt.Errorf("%w: %w", ErrConnectionFailed, err)
	}
	return nil
}

func (c *controlledClient) Complete(ctx context.Context, request CompletionRequest) (*CompletionResponse, error) {
	if err := c.controls.Check(operatorcontrols.AI); err != nil {
		return nil, err
	}
	policy := c.controls.AI()
	model := c.model
	if model == "" {
		model = request.Model
	}
	if policy != nil {
		if len(policy.AllowedModels) > 0 && !slices.Contains(policy.AllowedModels, model) {
			return nil, &operatorcontrols.DeniedError{Capability: operatorcontrols.AI}
		}
		if policy.MaxOutputTokens > 0 && (request.MaxTokens <= 0 || request.MaxTokens > policy.MaxOutputTokens) {
			request.MaxTokens = policy.MaxOutputTokens
		}
		if c.model != "" {
			request.Model = c.model
		}
	}
	release, err := c.controls.Acquire(ctx, "ai", c.controls.Limits().ConcurrentAI)
	if err != nil {
		return nil, err
	}
	defer release()
	finish := func(int64) error { return nil }
	var reservation int64
	if c.controls.Limits().AITokensPerDay > 0 {
		if policy == nil {
			return nil, operatorcontrols.ErrBudgetUnavailable
		}
		reservation = policy.CallTokenReservation
		encoded, err := json.Marshal(request)
		if err != nil {
			return nil, err
		}
		// Leave explicit framing headroom and reject oversized requests before
		// contacting a paid provider. The operator qualifies the total envelope.
		if len(request.Messages) > 100 || int64(len(encoded)) > reservation-int64(request.MaxTokens)-8192 {
			return nil, &operatorcontrols.LimitError{Resource: "ai_request_tokens"}
		}
		finish, err = c.controls.Reserve(ctx, "ai_tokens", reservation, c.controls.Limits().AITokensPerDay, 24*time.Hour)
		if err != nil {
			return nil, err
		}
	}
	response, err := c.Client.Complete(ctx, request)
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, errors.New("AI provider returned no response")
	}
	if reservation > 0 {
		usage := response.Usage
		if usage.PromptTokens < 0 || usage.CompletionTokens < 0 || usage.CacheReadTokens < 0 || usage.CacheWriteTokens < 0 || usage.TotalTokens < 0 {
			return nil, errors.New("AI provider returned invalid usage")
		}
		// Cache categories may overlap provider prompt totals. Over-counting is
		// preferable to granting free credit when their reporting is ambiguous.
		var actual int64
		for _, tokens := range []int{usage.PromptTokens, usage.CompletionTokens, usage.CacheReadTokens, usage.CacheWriteTokens} {
			if int64(tokens) > reservation-actual {
				return nil, errors.New("AI usage exceeded qualified operator call envelope")
			}
			actual += int64(tokens)
		}
		actual = max(actual, int64(usage.TotalTokens))
		if actual > 0 && actual <= reservation {
			if err := finish(actual); err != nil {
				return nil, fmt.Errorf("settle operator AI allowance: %w", err)
			}
		} else if actual > reservation {
			return nil, errors.New("AI usage exceeded qualified operator call envelope")
		}
	}
	return response, nil
}
