package github

import (
	"context"
	"fmt"

	"github.com/google/go-github/v60/github"
	"golang.org/x/oauth2"
)

type IssueInput struct {
	Title  string
	Body   string   // Markdown — will include description + AC
	Labels []string // e.g. ["agent:ready", "backend"]
}

type Issue struct {
	Number int
	Title  string
	URL    string
}

type Client struct {
	gh    *github.Client
	owner string
	repo  string
}

func NewClient(ctx context.Context, token, owner, repo string) *Client {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	httpClient := oauth2.NewClient(ctx, ts)
	return &Client{
		gh:    github.NewClient(httpClient),
		owner: owner,
		repo:  repo,
	}
}

func (c *Client) CreateIssue(ctx context.Context, input IssueInput) (Issue, error) {
	req := &github.IssueRequest{
		Title:  github.String(input.Title),
		Body:   github.String(input.Body),
		Labels: &input.Labels,
	}

	issue, _, err := c.gh.Issues.Create(ctx, c.owner, c.repo, req)
	if err != nil {
		return Issue{}, fmt.Errorf("github create issue: %w", err)
	}

	return Issue{
		Number: issue.GetNumber(),
		Title:  issue.GetTitle(),
		URL:    issue.GetHTMLURL(),
	}, nil
}

func (c *Client) GetIssue(ctx context.Context, number int) (Issue, error) {
	issue, _, err := c.gh.Issues.Get(ctx, c.owner, c.repo, number)
	if err != nil {
		return Issue{}, fmt.Errorf("github get issue: %w", err)
	}
	return Issue{
		Number: issue.GetNumber(),
		Title:  issue.GetTitle(),
		URL:    issue.GetHTMLURL(),
	}, nil
}

func (c *Client) AddLabel(ctx context.Context, number int, label string) error {
	_, _, err := c.gh.Issues.AddLabelsToIssue(ctx, c.owner, c.repo, number, []string{label})
	if err != nil {
		return fmt.Errorf("github add label: %w", err)
	}
	return nil
}
