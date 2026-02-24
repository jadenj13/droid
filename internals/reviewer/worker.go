package reviewer

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jadenj13/droid/internals/git"
)

const maxRevisionRounds = 5

type Notifier interface {
	NotifyPRReady(ctx context.Context, msg PRReadyMessage) error
}

type PRReadyMessage struct {
	PRURL      string
	PRTitle    string
	IssueURL   string
	IssueTitle string
	RepoURL    string
}

type Worker struct {
	agent    *Agent
	factory  ProviderFactory
	notifier Notifier
	log      *slog.Logger
}

type ProviderFactory interface {
	ProviderFor(ctx context.Context, repoURL string) (git.GitProvider, git.RepoInfo, error)
}

func NewWorker(agent *Agent, factory ProviderFactory, notifier Notifier, log *slog.Logger) *Worker {
	return &Worker{agent: agent, factory: factory, notifier: notifier, log: log}
}

func (w *Worker) HandlePR(ctx context.Context, repoURL string, prNumber int) error {
	provider, _, err := w.factory.ProviderFor(ctx, repoURL)
	if err != nil {
		return fmt.Errorf("build provider: %w", err)
	}

	return w.reviewLoop(ctx, provider, repoURL, prNumber, 0)
}

func (w *Worker) reviewLoop(ctx context.Context, provider git.GitProvider, repoURL string, prNumber, round int) error {
	if round >= maxRevisionRounds {
		return fmt.Errorf("exceeded %d revision rounds for PR #%d", maxRevisionRounds, prNumber)
	}

	pr, err := provider.GetPR(ctx, prNumber)
	if err != nil {
		return fmt.Errorf("get PR: %w", err)
	}

	var originalIssue git.Issue
	if pr.IssueURL != "" {
		issueNumber := parseIssueNumber(pr.IssueURL)
		if issueNumber > 0 {
			originalIssue, err = provider.GetIssue(ctx, issueNumber)
			if err != nil {
				w.log.Warn("could not fetch original issue", "url", pr.IssueURL, "err", err)
			}
		}
	}

	w.log.Info("reviewing PR", "pr", prNumber, "round", round)

	review, err := w.agent.Review(ctx, pr, originalIssue)
	if err != nil {
		return fmt.Errorf("agent review: %w", err)
	}

	if err := provider.PostReview(ctx, prNumber, review); err != nil {
		return fmt.Errorf("post review: %w", err)
	}

	w.log.Info("review posted", "pr", prNumber, "verdict", review.Verdict, "comments", len(review.Comments))

	switch review.Verdict {
	case "approve":
		if err := provider.AddLabel(ctx, originalIssue.Number, "agent:approved"); err != nil {
			w.log.Warn("failed to add agent:approved label", "err", err)
		}
		if err := w.notifier.NotifyPRReady(ctx, PRReadyMessage{
			PRURL:      pr.URL,
			PRTitle:    pr.Title,
			IssueURL:   originalIssue.URL,
			IssueTitle: originalIssue.Title,
			RepoURL:    repoURL,
		}); err != nil {
			w.log.Warn("failed to send Slack notification", "err", err)
		}

	case "request_changes":
		if err := provider.AddLabel(ctx, originalIssue.Number, "agent:revision"); err != nil {
			return fmt.Errorf("add revision label: %w", err)
		}
		w.log.Info("requested changes — executor will revise", "pr", prNumber, "round", round)
		// The executor webhook will fire when it sees "agent:revision" and push
		// an updated branch, which will re-trigger this reviewer via a new
		// "agent:review" label — so we don't recurse here directly.

	case "comment":
		w.log.Info("review posted as comment — no action required", "pr", prNumber)
	}

	return nil
}

// parseIssueNumber extracts the issue number from a URL like
// https://github.com/org/repo/issues/42
func parseIssueNumber(url string) int {
	parts := strings.Split(strings.TrimRight(url, "/"), "/")
	if len(parts) == 0 {
		return 0
	}
	var n int
	fmt.Sscanf(parts[len(parts)-1], "%d", &n)
	return n
}
