package planner

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/jadenj13/droid/internals/git"
)

var toolSetRepo = anthropic.ToolParam{
	Name:        "set_repo",
	Description: anthropic.String("Validates and stores the repository URL for this planning session. Call this as soon as the user provides a repo URL, before creating any issues."),
	InputSchema: anthropic.ToolInputSchemaParam{
		Properties: map[string]interface{}{
			"repo_url": map[string]interface{}{
				"type":        "string",
				"description": "Full URL of the repository. E.g. https://github.com/myorg/myrepo or https://gitlab.mycompany.com/group/myrepo",
			},
		},
		Required: []string{"repo_url"},
	},
}

var toolCreateIssue = anthropic.ToolParam{
	Name:        "create_issue",
	Description: anthropic.String("Creates an issue in the configured repository for a discrete unit of work. Requires set_repo to have been called first. Call once per issue after the user approves the breakdown."),
	InputSchema: anthropic.ToolInputSchemaParam{
		Properties: map[string]interface{}{
			"title": map[string]interface{}{
				"type":        "string",
				"description": "Short, action-oriented issue title.",
			},
			"description": map[string]interface{}{
				"type":        "string",
				"description": "2-3 sentence description of what needs to be done and why.",
			},
			"acceptance_criteria": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Testable acceptance criteria for this issue.",
			},
			"labels": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Labels to apply. Always include 'agent:ready'.",
			},
		},
		Required: []string{"title", "description", "acceptance_criteria", "labels"},
	},
}

var toolFinishPlanning = anthropic.ToolParam{
	Name:        "finish_planning",
	Description: anthropic.String("Marks the planning session as complete after all issues have been created."),
	InputSchema: anthropic.ToolInputSchemaParam{
		Properties: map[string]interface{}{
			"summary": map[string]interface{}{
				"type":        "string",
				"description": "Brief summary of what was planned and how many issues were created.",
			},
		},
		Required: []string{"summary"},
	},
}

var AllTools = []anthropic.ToolParam{toolSetRepo, toolCreateIssue, toolFinishPlanning}

type setRepoInput struct {
	RepoURL string `json:"repo_url"`
}

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
	Content string
}
type ProviderFactory interface {
	ProviderFor(ctx context.Context, repoURL string) (git.GitProvider, git.RepoInfo, error)
}

func ExecuteTool(ctx context.Context, name string, raw json.RawMessage, sess *Session, factory ProviderFactory) (ToolResult, error) {
	switch name {
	case "set_repo":
		return execSetRepo(ctx, raw, sess, factory)
	case "create_issue":
		return execCreateIssue(ctx, raw, sess)
	case "finish_planning":
		return execFinishPlanning(raw, sess)
	default:
		return ToolResult{}, fmt.Errorf("unknown tool: %s", name)
	}
}

func execSetRepo(ctx context.Context, raw json.RawMessage, sess *Session, factory ProviderFactory) (ToolResult, error) {
	var input setRepoInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return ToolResult{}, fmt.Errorf("unmarshal set_repo: %w", err)
	}

	provider, info, err := factory.ProviderFor(ctx, input.RepoURL)
	if err != nil {
		// Return as a soft error so Claude can tell the user what went wrong.
		return ToolResult{Content: fmt.Sprintf("error: %s", err)}, nil
	}

	sess.Repo = &info
	sess.GitProvider = provider

	return ToolResult{
		Content: fmt.Sprintf("Repo configured: %s (%s) — owner: %q, repo: %q",
			info.RawURL, info.Platform, info.Owner, info.Repo),
	}, nil
}

func execCreateIssue(ctx context.Context, raw json.RawMessage, sess *Session) (ToolResult, error) {
	if sess.GitProvider == nil {
		return ToolResult{Content: "error: no repository configured — ask the user for a repo URL first"}, nil
	}

	var input createIssueInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return ToolResult{}, fmt.Errorf("unmarshal create_issue: %w", err)
	}

	issue, err := sess.GitProvider.CreateIssue(ctx, git.IssueInput{
		Title:  input.Title,
		Body:   buildIssueBody(input.Description, input.AcceptanceCriteria),
		Labels: input.Labels,
	})
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("error creating issue: %s", err)}, nil
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
		return ToolResult{}, fmt.Errorf("unmarshal finish_planning: %w", err)
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
