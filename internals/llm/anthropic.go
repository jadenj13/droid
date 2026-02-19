package llm

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/jadenj13/droid/internals/planner"
)

const (
	DefaultModel     = anthropic.ModelClaude4Sonnet20250514
	DefaultMaxTokens = 8096
)

type Client struct {
	client    anthropic.Client
	model     anthropic.Model
	maxTokens int64
}

type Option func(*Client)

func WithModel(model anthropic.Model) Option {
	return func(c *Client) { c.model = model }
}

func WithMaxTokens(n int64) Option {
	return func(c *Client) { c.maxTokens = n }
}

func NewClient(apiKey string, opts ...Option) *Client {
	c := &Client{
		client:    anthropic.NewClient(option.WithAPIKey(apiKey)),
		model:     DefaultModel,
		maxTokens: DefaultMaxTokens,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

func (c *Client) Complete(ctx context.Context, system string, messages []planner.Message) (string, error) {
	apiMessages, err := toAPIMessages(messages)
	if err != nil {
		return "", err
	}

	resp, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     c.model,
		MaxTokens: c.maxTokens,
		System: []anthropic.TextBlockParam{
			{Text: system},
		},
		Messages: apiMessages,
	})
	if err != nil {
		return "", fmt.Errorf("anthropic api: %w", err)
	}

	if len(resp.Content) == 0 {
		return "", fmt.Errorf("anthropic returned empty content")
	}

	switch block := resp.Content[0]; block.Type {
	case "text":
		return block.Text, nil
	default:
		return "", fmt.Errorf("unexpected content block type: %s", block.Type)
	}
}

func toAPIMessages(messages []planner.Message) ([]anthropic.MessageParam, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("messages cannot be empty")
	}

	out := make([]anthropic.MessageParam, 0, len(messages))
	for i, m := range messages {
		switch m.Role {
		case "user":
			out = append(out, anthropic.NewUserMessage(anthropic.NewTextBlock(m.Content)))
		case "assistant":
			out = append(out, anthropic.NewAssistantMessage(anthropic.NewTextBlock(m.Content)))
		default:
			return nil, fmt.Errorf("message[%d]: unknown role %q", i, m.Role)
		}
	}

	if last := out[len(out)-1]; last.Role != anthropic.MessageParamRoleUser {
		return nil, fmt.Errorf("last message must be from user, got %q", last.Role)
	}

	return out, nil
}