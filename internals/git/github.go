package git

import (
	"context"
	"fmt"

	"github.com/google/go-github/v60/github"
	"golang.org/x/oauth2"
)

type GitHubTracker struct {
	gh   *github.Client
	info RepoInfo
}

func NewGitHubTracker(ctx context.Context, token string, info RepoInfo) (*GitHubTracker, error) {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	return &GitHubTracker{
		gh:   github.NewClient(oauth2.NewClient(ctx, ts)),
		info: info,
	}, nil
}

func (t *GitHubTracker) RepoURL() string { return t.info.RawURL }

func (t *GitHubTracker) CreateIssue(ctx context.Context, input IssueInput) (Issue, error) {
	req := &github.IssueRequest{
		Title:  github.String(input.Title),
		Body:   github.String(input.Body),
		Labels: &input.Labels,
	}
	issue, _, err := t.gh.Issues.Create(ctx, t.info.Owner, t.info.Repo, req)
	if err != nil {
		return Issue{}, fmt.Errorf("github create issue: %w", err)
	}
	return Issue{
		Number: issue.GetNumber(),
		Title:  issue.GetTitle(),
		URL:    issue.GetHTMLURL(),
	}, nil
}

func (t *GitHubTracker) GetIssue(ctx context.Context, number int) (Issue, error) {
	issue, _, err := t.gh.Issues.Get(ctx, t.info.Owner, t.info.Repo, number)
	if err != nil {
		return Issue{}, fmt.Errorf("github get issue: %w", err)
	}
	return Issue{
		Number: issue.GetNumber(),
		Title:  issue.GetTitle(),
		URL:    issue.GetHTMLURL(),
	}, nil
}

func (t *GitHubTracker) AddLabel(ctx context.Context, number int, label string) error {
	_, _, err := t.gh.Issues.AddLabelsToIssue(ctx, t.info.Owner, t.info.Repo, number, []string{label})
	if err != nil {
		return fmt.Errorf("github add label: %w", err)
	}
	return nil
}