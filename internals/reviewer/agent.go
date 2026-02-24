package reviewer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/jadenj13/droid/internals/git"
	"github.com/jadenj13/droid/internals/llm"
)

type LLM interface {
	CompleteWithTools(ctx context.Context, system string, messages []llm.Message, tools []anthropic.ToolParam) (*anthropic.Message, error)
}

type Agent struct {
	llm LLM
	log *slog.Logger
}

func NewAgent(llm LLM, log *slog.Logger) *Agent {
	return &Agent{llm: llm, log: log}
}

func (a *Agent) Review(ctx context.Context, pr git.PR, originalIssue git.Issue) (git.Review, error) {
	msgs := []llm.Message{{
		Role:    "user",
		Content: buildReviewPrompt(pr, originalIssue),
	}}

	resp, err := a.llm.CompleteWithTools(ctx, systemPrompt(), msgs, []anthropic.ToolParam{toolSubmitReview})
	if err != nil {
		return git.Review{}, fmt.Errorf("llm review: %w", err)
	}

	for _, block := range resp.Content {
		if block.Type == "tool_use" && block.Name == "submit_review" {
			return parseReviewResult(block.Input)
		}
	}

	text := extractText(resp)
	a.log.Warn("reviewer responded with text instead of tool call — using as comment")
	return git.Review{
		Verdict: "comment",
		Summary: text,
	}, nil
}

var toolSubmitReview = anthropic.ToolParam{
	Name:        "submit_review",
	Description: anthropic.String("Submit the completed code review. Always call this — never respond with plain text."),
	InputSchema: anthropic.ToolInputSchemaParam{
		Properties: map[string]interface{}{
			"verdict": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"approve", "request_changes", "comment"},
				"description": "approve if all acceptance criteria are met and the code is correct. request_changes if there are issues that must be fixed. comment for minor observations that don't block merging.",
			},
			"summary": map[string]interface{}{
				"type":        "string",
				"description": "Overall review summary. Be specific about what's good and what needs fixing.",
			},
			"comments": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "File path relative to repo root.",
						},
						"line": map[string]interface{}{
							"type":        "integer",
							"description": "Line number in the diff to attach this comment to.",
						},
						"body": map[string]interface{}{
							"type":        "string",
							"description": "Comment text. Be specific and actionable.",
						},
					},
					"required": []string{"path", "line", "body"},
				},
				"description": "Inline comments on specific lines. Only include comments for genuine issues, not style nits.",
			},
		},
		Required: []string{"verdict", "summary", "comments"},
	},
}

type submitReviewInput struct {
	Verdict  string `json:"verdict"`
	Summary  string `json:"summary"`
	Comments []struct {
		Path string `json:"path"`
		Line int    `json:"line"`
		Body string `json:"body"`
	} `json:"comments"`
}

func parseReviewResult(raw json.RawMessage) (git.Review, error) {
	var input submitReviewInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return git.Review{}, fmt.Errorf("unmarshal review: %w", err)
	}

	comments := make([]git.PRComment, 0, len(input.Comments))
	for _, c := range input.Comments {
		comments = append(comments, git.PRComment{
			Path: c.Path,
			Line: c.Line,
			Body: c.Body,
			Side: "RIGHT",
		})
	}

	return git.Review{
		Verdict:  input.Verdict,
		Summary:  input.Summary,
		Comments: comments,
	}, nil
}

func systemPrompt() string {
	return `You are an expert code reviewer. You will be given a pull request diff and the
original issue it addresses. Your job is to review the changes and decide whether they
should be approved, require changes, or need a comment.

Review criteria — check all of these:
- Does the implementation satisfy every acceptance criterion in the issue?
- Are there any bugs, logic errors, or edge cases not handled?
- Does the code follow the patterns and conventions visible in the surrounding codebase?
- Are there missing tests or inadequate test coverage for the changes?
- Is error handling present and appropriate?
- Are there any security concerns (injection, auth bypass, data exposure)?

Be direct and specific. When requesting changes, tell the executor exactly what to fix.
Do not request stylistic changes that don't affect correctness or maintainability.
Always respond by calling submit_review — never with plain text.`
}

func buildReviewPrompt(pr git.PR, issue git.Issue) string {
	return fmt.Sprintf(`Please review the following pull request.

## Original Issue

Title: %s
URL: %s

## Pull Request

Title: %s
Branch: %s → %s

%s

## Diff

%s`,
		issue.Title,
		issue.URL,
		pr.Title,
		pr.Branch, pr.BaseBranch,
		truncate(pr.Description, 1000),
		truncate(pr.Diff, 20000),
	)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + fmt.Sprintf("\n... (truncated, %d chars total)", len(s))
}

func extractText(resp *anthropic.Message) string {
	for _, b := range resp.Content {
		if b.Type == "text" {
			return b.Text
		}
	}
	return ""
}
