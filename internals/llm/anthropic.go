package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

const (
	maxRetries    = 4
	baseDelay     = time.Second
	maxDelay      = 30 * time.Second
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

func (c *Client) CompleteWithTools(ctx context.Context, system string, messages []Message, tools []anthropic.ToolParam) (*anthropic.Message, error) {
	apiMessages, err := toAPIMessages(messages)
	if err != nil {
		return nil, err
	}

	toolUnions := make([]anthropic.ToolUnionParam, len(tools))
	for i := range tools {
		t := tools[i]
		toolUnions[i] = anthropic.ToolUnionParam{OfTool: &t}
	}

	params := anthropic.MessageNewParams{
		Model:     c.model,
		MaxTokens: c.maxTokens,
		System:    []anthropic.TextBlockParam{{Text: system}},
		Messages:  apiMessages,
		Tools:     toolUnions,
	}

	var resp *anthropic.Message
	for attempt := range maxRetries {
		resp, err = c.client.Messages.New(ctx, params)
		if err == nil {
			return resp, nil
		}

		if !isRetryable(err) || attempt == maxRetries-1 {
			return nil, fmt.Errorf("anthropic api: %w", err)
		}

		delay := retryDelay(attempt)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}

	return nil, fmt.Errorf("anthropic api: %w", err)
}

// isRetryable returns true for transient errors worth retrying: rate limits,
// overloaded, and 5xx server errors. Authentication and client errors are not retried.
func isRetryable(err error) bool {
	var apiErr *anthropic.Error
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.StatusCode {
	case 429, 500, 502, 503, 504, 529:
		return true
	}
	return false
}

// retryDelay returns an exponential backoff duration with full jitter.
func retryDelay(attempt int) time.Duration {
	exp := baseDelay * (1 << attempt) // 1s, 2s, 4s, 8s, ...
	if exp > maxDelay {
		exp = maxDelay
	}
	// Full jitter: random value in [0, exp)
	jitter := time.Duration(rand.Int64N(int64(exp)))
	return jitter
}

func toAPIMessages(messages []Message) ([]anthropic.MessageParam, error) {
	out := make([]anthropic.MessageParam, 0, len(messages))

	for i, m := range messages {
		switch m.Role {
		case "user":
			out = append(out, anthropic.NewUserMessage(anthropic.NewTextBlock(m.Content)))

		case "assistant":
			if isJSON(m.Content) {
				var blocks []anthropic.ContentBlockUnion
				if err := json.Unmarshal([]byte(m.Content), &blocks); err != nil {
					return nil, fmt.Errorf("message[%d]: unmarshal assistant blocks: %w", i, err)
				}
				out = append(out, anthropic.NewAssistantMessage(blocksToUnion(blocks)...))
			} else {
				out = append(out, anthropic.NewAssistantMessage(anthropic.NewTextBlock(m.Content)))
			}

		case "tool_result":
			if len(m.RawBlocks) == 0 {
				return nil, fmt.Errorf("message[%d]: tool_result has no blocks", i)
			}
			unions := make([]anthropic.ContentBlockParamUnion, 0, len(m.RawBlocks))
			for _, rb := range m.RawBlocks {
				unions = append(unions, anthropic.ContentBlockParamUnion{
					OfToolResult: &rb,
				})
			}
			out = append(out, anthropic.MessageParam{
				Role:    anthropic.MessageParamRoleUser,
				Content: unions,
			})

		default:
			return nil, fmt.Errorf("message[%d]: unknown role %q", i, m.Role)
		}
	}

	return out, nil
}

func isJSON(s string) bool {
	return len(s) > 0 && s[0] == '['
}

func blocksToUnion(blocks []anthropic.ContentBlockUnion) []anthropic.ContentBlockParamUnion {
	out := make([]anthropic.ContentBlockParamUnion, 0, len(blocks))
	for _, b := range blocks {
		switch b.Type {
		case "text":
			out = append(out, anthropic.NewTextBlock(b.Text))
		case "tool_use":
			out = append(out, anthropic.ContentBlockParamUnion{
				OfToolUse: &anthropic.ToolUseBlockParam{
					ID:    b.ID,
					Name:  b.Name,
					Input: b.Input,
				},
			})
		}
	}
	return out
}
