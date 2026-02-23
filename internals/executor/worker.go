package executor

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jadenj13/droid/internals/git"
)

type PROpener interface {
	OpenPR(ctx context.Context, input PRInput) (PRURL string, err error)
}

type PRInput struct {
	Title       string
	Body        string
	Branch      string // head branch
	Base        string // target branch, usually "main"
	IssueNumber int
	Draft       bool
}

type Worker struct {
	agent   *Agent
	factory git.Factory
	token   string // git clone token (same as the issue tracker token)
	log     *slog.Logger
}

func NewWorker(agent *Agent, factory git.Factory, token string, log *slog.Logger) *Worker {
	return &Worker{agent: agent, factory: factory, token: token, log: log}
}

func (w *Worker) HandleIssue(ctx context.Context, repoURL string, issue git.Issue) error {
	w.log.Info("handling issue", "issue", issue.Number, "title", issue.Title)

	tracker, _, err := w.factory.ProviderFor(ctx, repoURL)
	if err != nil {
		return fmt.Errorf("build tracker: %w", err)
	}

	full, err := tracker.GetIssue(ctx, issue.Number)
	if err != nil {
		return fmt.Errorf("fetch issue: %w", err)
	}
	issue = full

	result, err := w.agent.Run(ctx, issue, tracker, w.token)
	if err != nil {
		return fmt.Errorf("agent run: %w", err)
	}

	opener, ok := tracker.(PROpener)
	if !ok {
		return fmt.Errorf("tracker does not support opening PRs")
	}

	prURL, err := opener.OpenPR(ctx, PRInput{
		Title:       result.Title,
		Body:        buildPRBody(result, issue),
		Branch:      result.Branch,
		Base:        "main",
		IssueNumber: issue.Number,
		Draft:       false,
	})
	if err != nil {
		return fmt.Errorf("open PR: %w", err)
	}

	w.log.Info("PR opened", "url", prURL, "issue", issue.Number)

	if err := tracker.AddLabel(ctx, issue.Number, "agent:review"); err != nil {
		w.log.Warn("failed to add agent:review label", "err", err)
		// Non-fatal — the PR is open regardless.
	}

	return nil
}

func buildPRBody(result PRResult, issue git.Issue) string {
	var sb strings.Builder
	sb.WriteString(result.Summary)
	sb.WriteString("\n\n---\n")
	sb.WriteString(fmt.Sprintf("Closes %s\n", issue.URL))
	sb.WriteString("\n*Opened by the Executor Agent*")
	return sb.String()
}