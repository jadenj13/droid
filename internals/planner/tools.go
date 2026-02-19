package planner

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
)

type IssueCreator interface {
	CreateIssue(ctx context.Context, input IssueInput) (CreatedIssue, error)
}

type IssueInput struct {
	Title  string
	Body   string
	Labels []string
}

type CreatedIssue struct {
	Number int
	Title  string
	URL    string
}

var toolCreateIssue = anthropic.ToolParam{
	Name:        "create_issue",
	Description: anthropic.String("Creates a GitHub issue for a discrete unit of work. Call this once per issue when the user has approved the issue breakdown."),
	InputSchema: anthropic.ToolInputSchemaParam{
		Properties: map[string]interface{}{
			"title": map[string]interface{}{
				"type":        "string",
				"description": "Short, action-oriented issue title. E.g. 'Add user authentication endpoint'",
			},
			"description": map[string]interface{}{
				"type":        "string",
				"description": "2-3 sentence description of what needs to be done and why.",
			},
			"acceptance_criteria": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "List of testable acceptance criteria for this specific issue.",
			},
			"labels": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Labels to apply. Always include 'agent:ready'. Add others like 'backend', 'frontend', 'infra' as appropriate.",
			},
		},
		Required: []string{"title", "description", "acceptance_criteria", "labels"},
	},
}

var toolFinishPlanning = anthropic.ToolParam{
	Name:        "finish_planning",
	Description: anthropic.String("Call this after all issues have been created to mark the planning session as complete."),
	InputSchema: anthropic.ToolInputSchemaParam{
		Properties: map[string]interface{}{
			"summary": map[string]interface{}{
				"type":        "string",
				"description": "A brief summary of what was planned and how many issues were created.",
			},
		},
		Required: []string{"summary"},
	},
}

var AllTools = []anthropic.ToolParam{toolCreateIssue, toolFinishPlanning}

type createIssueInput struct {
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
	Labels             []string `json:"labels"`
}

type finishPlanningInput struct {
	Summary string `json:"summary"`
}

type ToolResult struct {
	ToolUseID string
	Content   string // shown back to Claude as the tool result
}

func ExecuteTool(ctx context.Context, name string, rawInput json.RawMessage, sess *Session, creator IssueCreator) (ToolResult, error) {
	switch name {
	case "create_issue":
		return execCreateIssue(ctx, rawInput, sess, creator)
	case "finish_planning":
		return execFinishPlanning(rawInput, sess)
	default:
		return ToolResult{}, fmt.Errorf("unknown tool: %s", name)
	}
}

func execCreateIssue(ctx context.Context, raw json.RawMessage, sess *Session, creator IssueCreator) (ToolResult, error) {
	var input createIssueInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return ToolResult{}, fmt.Errorf("unmarshal create_issue input: %w", err)
	}

	body := buildIssueBody(input.Description, input.AcceptanceCriteria)

	issue, err := creator.CreateIssue(ctx, IssueInput{
		Title:  input.Title,
		Body:   body,
		Labels: input.Labels,
	})
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("error creating issue: %s", err)}, nil
		// Note: we return nil error so Claude sees the error as a tool result
		// and can handle it gracefully, rather than the whole request failing.
	}

	sess.Issues = append(sess.Issues, LinkedIssue{
		Number: issue.Number,
		Title:  issue.Title,
		URL:    issue.URL,
	})

	return ToolResult{
		Content: fmt.Sprintf("Created issue #%d: %s\n%s", issue.Number, issue.Title, issue.URL),
	}, nil
}

func execFinishPlanning(raw json.RawMessage, sess *Session) (ToolResult, error) {
	var input finishPlanningInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return ToolResult{}, fmt.Errorf("unmarshal finish_planning input: %w", err)
	}
	sess.Stage = StageDone
	return ToolResult{Content: "Planning session marked as complete."}, nil
}

func buildIssueBody(description string, ac []string) string {
	body := fmt.Sprintf("## Description\n\n%s\n\n## Acceptance Criteria\n", description)
	for _, c := range ac {
		body += fmt.Sprintf("- [ ] %s\n", c)
	}
	body += "\n---\n*Created by the Planner Agent*"
	return body
}
