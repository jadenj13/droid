package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/jadenj13/droid/internals/git"
	"github.com/jadenj13/droid/internals/llm"
)

const (
	maxIterations = 50 // hard ceiling on tool call loop
	maxTokens     = int64(16000)
	model         = anthropic.ModelClaude4Sonnet20250514
)

type LLM interface {
	CompleteWithTools(ctx context.Context, system string, messages []llm.Message, tools []anthropic.ToolParam) (*anthropic.Message, error)
}

type PRResult struct {
	Branch   string
	Title    string
	Summary  string
	IssueURL string
}

type Agent struct {
	llm LLM
	log *slog.Logger
}

func NewAgent(llm LLM, log *slog.Logger) *Agent {
	return &Agent{llm: llm, log: log}
}

func (a *Agent) Run(ctx context.Context, issue git.Issue, provider git.GitProvider, token string) (PRResult, error) {
	repo, err := git.Clone(ctx, provider.RepoURL(), token)
	if err != nil {
		return PRResult{}, fmt.Errorf("clone: %w", err)
	}
	defer repo.Cleanup()

	branch := git.BranchName(issue.Number, issue.Title)
	if err := repo.CreateBranch(ctx, branch); err != nil {
		return PRResult{}, fmt.Errorf("create branch: %w", err)
	}

	a.log.Info("executor started", "issue", issue.Number, "branch", branch)

	result, err := a.runLoop(ctx, repo, issue)
	if err != nil {
		return PRResult{}, err
	}

	if err := repo.Push(ctx); err != nil {
		return PRResult{}, fmt.Errorf("push: %w", err)
	}

	return PRResult{
		Branch:   branch,
		Title:    result.PRTitle,
		Summary:  result.PRSummary,
		IssueURL: issue.URL,
	}, nil
}

func (a *Agent) runLoop(ctx context.Context, repo *git.Repo, issue git.Issue) (ToolResult, error) {
	msgs := []llm.Message{{Role: "user", Content: initialPrompt(issue)}}
	system := systemPrompt()

	for i := range maxIterations {
		resp, err := a.llm.CompleteWithTools(ctx, system, msgs, AllTools)
		if err != nil {
			return ToolResult{}, fmt.Errorf("llm iter %d: %w", i, err)
		}

		toolCalls := extractToolCalls(resp)

		if len(toolCalls) == 0 {
			text := extractText(resp)
			return ToolResult{}, fmt.Errorf("executor stopped without submit_work: %s", text)
		}

		toolResults := make([]anthropic.ToolResultBlockParam, 0, len(toolCalls))
		var finalResult ToolResult

		for _, tc := range toolCalls {
			result, err := ExecuteTool(ctx, tc.Name, tc.Input, repo)
			if err != nil {
				return ToolResult{}, fmt.Errorf("tool %q: %w", tc.Name, err)
			}

			a.log.Info("tool executed", "tool", tc.Name, "iter", i,
				"preview", preview(result.Content, 120))

			toolResults = append(toolResults, anthropic.ToolResultBlockParam{
				ToolUseID: tc.ID,
				Content: []anthropic.ToolResultBlockParamContentUnion{
					{OfText: &anthropic.TextBlockParam{Text: result.Content}},
				},
			})

			if result.Done {
				finalResult = result
			}
		}

		msgs = append(msgs,
			llm.Message{Role: "assistant", Content: marshalBlocks(resp.Content)},
			llm.Message{Role: "tool_result", RawBlocks: toolResults},
		)

		if finalResult.Done {
			a.log.Info("executor completed", "issue", issue.Number, "iters", i+1)
			return finalResult, nil
		}
	}

	return ToolResult{}, fmt.Errorf("executor exceeded %d iterations without completing", maxIterations)
}

func initialPrompt(issue git.Issue) string {
	return fmt.Sprintf(`Please complete the following GitHub issue.

Issue #%d: %s
URL: %s

Issue body:
---
%s
---

Start by listing the repository structure so you understand the codebase, then plan your approach before making any changes.
When you are done and all tests pass, call submit_work.`,
		issue.Number, issue.Title, issue.URL, "{{ISSUE_BODY}}")
	// Note: issue body will be fetched separately and substituted in the worker.
}

func systemPrompt() string {
	return `You are an expert software engineer working autonomously on a code repository.
You have been assigned a GitHub issue to complete.

Your workflow:
1. Use list_files to understand the project structure
2. Use read_file to read relevant existing code
3. Plan your changes before writing anything
4. Use write_file to implement changes
5. Use run_command to run tests, linters, and build checks
6. Fix any issues found by tests or linters
7. Use commit_changes to commit logical groups of changes
8. Once all tests pass and the work is complete, call submit_work

Rules:
- Never commit broken or untested code
- Make the smallest change that satisfies the acceptance criteria
- Follow existing code style and conventions — read existing files first
- If you encounter something ambiguous in the requirements, make a reasonable decision and note it in the PR summary
- Do not modify files unrelated to the issue
- Always run tests before submitting`
}

type toolCall struct {
	ID    string
	Name  string
	Input json.RawMessage
}

func extractToolCalls(resp *anthropic.Message) []toolCall {
	var out []toolCall
	for _, b := range resp.Content {
		if b.Type == "tool_use" {
			out = append(out, toolCall{ID: b.ID, Name: b.Name, Input: b.Input})
		}
	}
	return out
}

func extractText(resp *anthropic.Message) string {
	var parts []string
	for _, b := range resp.Content {
		if b.Type == "text" && b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func marshalBlocks(blocks []anthropic.ContentBlockUnion) string {
	b, _ := json.Marshal(blocks)
	return string(b)
}

func preview(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
