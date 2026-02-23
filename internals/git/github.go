package git

import (
	"context"
	"fmt"

	"github.com/google/go-github/v60/github"
	"golang.org/x/oauth2"
)

type GitHubProvider struct {
	gh   *github.Client
	info RepoInfo
}

func NewGitHubProvider(ctx context.Context, token string, info RepoInfo) (*GitHubProvider, error) {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	return &GitHubProvider{
		gh:   github.NewClient(oauth2.NewClient(ctx, ts)),
		info: info,
	}, nil
}

func (t *GitHubProvider) RepoURL() string { return t.info.RawURL }

func (t *GitHubProvider) CreateIssue(ctx context.Context, input IssueInput) (Issue, error) {
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

func (t *GitHubProvider) GetIssue(ctx context.Context, number int) (Issue, error) {
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

func (t *GitHubProvider) AddLabel(ctx context.Context, number int, label string) error {
	_, _, err := t.gh.Issues.AddLabelsToIssue(ctx, t.info.Owner, t.info.Repo, number, []string{label})
	if err != nil {
		return fmt.Errorf("github add label: %w", err)
	}
	return nil
}

func (t *GitHubProvider) OpenPR(ctx context.Context, input PRInput) (string, error) {
	pr, _, err := t.gh.PullRequests.Create(ctx, t.info.Owner, t.info.Repo, &github.NewPullRequest{
		Title: github.String(input.Title),
		Body:  github.String(input.Body),
		Head:  github.String(input.Branch),
		Base:  github.String(input.Base),
		Draft: github.Bool(input.Draft),
	})
	if err != nil {
		return "", fmt.Errorf("github open PR: %w", err)
	}
	return pr.GetHTMLURL(), nil
}