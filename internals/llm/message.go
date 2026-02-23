package llm

import "github.com/anthropics/anthropic-sdk-go"

type Message struct {
	Role      string // "user", "assistant", or "tool_result"
	Content   string // plain text, or JSON-serialised content blocks for assistant tool calls
	RawBlocks []anthropic.ToolResultBlockParam // populated for tool_result role only
}