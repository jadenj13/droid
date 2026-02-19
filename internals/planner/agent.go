package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	slackhandler "github.com/jadenj13/droid/internals/slack"
)

type LLM interface {
	CompleteWithTools(ctx context.Context, system string, messages []Message, tools []anthropic.ToolParam) (*anthropic.Message, error)
}

type Agent struct {
	sessions *SessionStore
	llm      LLM
	issues   IssueCreator
	log      *slog.Logger
}

func NewAgent(sessions *SessionStore, llm LLM, issues IssueCreator, log *slog.Logger) *Agent {
	return &Agent{sessions: sessions, llm: llm, issues: issues, log: log}
}

func (a *Agent) Handle(ctx context.Context, msg slackhandler.IncomingMessage) (string, error) {
	sess := a.sessions.GetOrCreate(msg.ThreadTS, msg.ChannelID)

	if err := a.sessions.AppendMessage(sess, "user", msg.Text); err != nil {
		return "", fmt.Errorf("append user message: %w", err)
	}

	reply, err := a.runLoop(ctx, sess)
	if err != nil {
		return "", err
	}

	if err := a.sessions.AppendMessage(sess, "assistant", reply); err != nil {
		return "", fmt.Errorf("append assistant message: %w", err)
	}

	return reply, nil
}

func (a *Agent) runLoop(ctx context.Context, sess *Session) (string, error) {
	msgs := make([]Message, len(sess.Messages))
	copy(msgs, sess.Messages)

	const maxIter = 10 // safety limit
	for i := range maxIter {
		resp, err := a.llm.CompleteWithTools(ctx, systemPrompt(sess), msgs, AllTools)
		if err != nil {
			return "", fmt.Errorf("llm (iter %d): %w", i, err)
		}

		toolCalls := extractToolCalls(resp)

		if len(toolCalls) == 0 {
			return extractText(resp), nil
		}

		a.log.Info("Executing tools", "count", len(toolCalls), "iter", i)

		toolResults := make([]anthropic.ToolResultBlockParam, 0, len(toolCalls))
		for _, tc := range toolCalls {
			result, err := ExecuteTool(ctx, tc.Name, tc.Input, sess, a.issues)
			if err != nil {
				return "", fmt.Errorf("Execute tool %q: %w", tc.Name, err)
			}
			a.log.Info("Tool executed", "tool", tc.Name, "result", result.Content)
			toolResults = append(toolResults, anthropic.ToolResultBlockParam{
				ToolUseID: tc.ID,
				Content: []anthropic.ToolResultBlockParamContentUnion{
					{OfText: &anthropic.TextBlockParam{Text: result.Content}},
				},
			})
		}

		msgs = append(msgs,
			Message{Role: "assistant", Content: marshalBlocks(resp.Content)},
			Message{Role: "tool_result", RawBlocks: toolResults},
		)
	}

	return "", fmt.Errorf("tool loop exceeded %d iterations", maxIter)
}

type toolCall struct {
	ID    string
	Name  string
	Input json.RawMessage
}

func extractToolCalls(resp *anthropic.Message) []toolCall {
	var out []toolCall
	for _, block := range resp.Content {
		if block.Type == "tool_use" {
			out = append(out, toolCall{
				ID:    block.ID,
				Name:  block.Name,
				Input: block.Input,
			})
		}
	}
	return out
}

func extractText(resp *anthropic.Message) string {
	var parts []string
	for _, block := range resp.Content {
		if block.Type == "text" && block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func marshalBlocks(blocks []anthropic.ContentBlockUnion) string {
	b, _ := json.Marshal(blocks)
	return string(b)
}

func systemPrompt(sess *Session) string {
	base := `You are a technical project planning assistant embedded in Slack.
Your job is to help the user plan software projects and features by working through:
1. Understanding the problem and goals (brainstorm)
2. Writing a clear Product Requirements Document (PRD)
3. Defining acceptance criteria
4. Breaking the work into discrete GitHub issues

Guidelines:
- Ask clarifying questions before writing any documents.
- Be concise in Slack — use bullet points, avoid walls of text.
- When writing PRDs or acceptance criteria, be specific and testable.
- Only move to the next stage when the user confirms they're happy.
- When creating issues, make each one small enough for a single engineer to complete in a day or two.
- Always include the 'agent:ready' label when creating issues.
`
	switch sess.Stage {
	case StageBrainstorm:
		base += `
Current stage: BRAINSTORM
Help the user articulate what they're building and why. Ask about:
- The problem being solved
- Who the users are
- What success looks like
- Any known constraints or dependencies
When you have enough context, suggest moving to writing the PRD.`

	case StagePRD:
		base += `
Current stage: PRD
Write a structured PRD with these sections:
- Overview
- Problem Statement
- Goals & Non-goals
- User Stories
- Technical Approach (high level)
- Open Questions
Present it in full, then ask the user for feedback.`

	case StageCriteria:
		base += `
Current stage: ACCEPTANCE CRITERIA
Based on the PRD, write clear, testable acceptance criteria.
Format each as: "Given [context], when [action], then [outcome]".
Group them by feature area if there are many.`

	case StageIssues:
		base += `
Current stage: ISSUE BREAKDOWN
Break the work into GitHub issues. For each issue:
- Present the full list to the user first and ask for approval.
- Only call create_issue AFTER the user says they're happy with the breakdown.
- Call create_issue once per issue, not in bulk.
- Call finish_planning after all issues are created.`

	case StageDone:
		base += `
Current stage: DONE
All issues have been created. Help the user review or answer questions.`
	}

	if sess.PRDDraft != "" {
		base += "\n\nCurrent PRD draft:\n" + sess.PRDDraft
	}

	if len(sess.Issues) > 0 {
		base += "\n\nIssues created so far:"
		for _, iss := range sess.Issues {
			base += fmt.Sprintf("\n- #%d %s (%s)", iss.Number, iss.Title, iss.URL)
		}
	}

	return base
}
