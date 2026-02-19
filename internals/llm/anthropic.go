package llm

import (
	"context"
	"encoding/json"
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

func (c *Client) CompleteWithTools(ctx context.Context, system string, messages []planner.Message, tools []anthropic.ToolParam) (*anthropic.Message, error) {
	apiMessages, err := toAPIMessages(messages)
	if err != nil {
		return nil, err
	}

	toolUnions := make([]anthropic.ToolUnionParam, len(tools))
	for i := range tools {
		t := tools[i]
		toolUnions[i] = anthropic.ToolUnionParam{OfTool: &t}
	}

	resp, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     c.model,
		MaxTokens: c.maxTokens,
		System:    []anthropic.TextBlockParam{{Text: system}},
		Messages:  apiMessages,
		Tools:     toolUnions,
	})
	if err != nil {
		return nil, fmt.Errorf("anthropic api: %w", err)
	}

	return resp, nil
}

func toAPIMessages(messages []planner.Message) ([]anthropic.MessageParam, error) {
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
