package git

import (
	"context"
	"fmt"
	"strings"

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
		Body:   issue.GetBody(),
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

func (t *GitHubProvider) GetPRComments(ctx context.Context, prNumber int) ([]PRComment, error) {
	comments, _, err := t.gh.PullRequests.ListComments(ctx, t.info.Owner, t.info.Repo, prNumber, nil)
	if err != nil {
		return nil, fmt.Errorf("github get PR comments: %w", err)
	}
	out := make([]PRComment, 0, len(comments))
	for _, c := range comments {
		out = append(out, PRComment{
			Path: c.GetPath(),
			Line: c.GetLine(),
			Body: c.GetBody(),
		})
	}
	return out, nil
}

func (t *GitHubProvider) PostReview(ctx context.Context, prNumber int, review Review) error {
	event := verdictToGitHubEvent(review.Verdict)

	comments := make([]*github.DraftReviewComment, 0, len(review.Comments))
	for _, c := range review.Comments {
		side := c.Side
		if side == "" {
			side = "RIGHT"
		}
		comments = append(comments, &github.DraftReviewComment{
			Path: github.String(c.Path),
			Line: github.Int(c.Line),
			Body: github.String(c.Body),
			Side: github.String(side),
		})
	}

	_, _, err := t.gh.PullRequests.CreateReview(ctx, t.info.Owner, t.info.Repo, prNumber, &github.PullRequestReviewRequest{
		Event:    github.String(event),
		Body:     github.String(review.Summary),
		Comments: comments,
	})
	if err != nil {
		return fmt.Errorf("github post review: %w", err)
	}
	return nil
}

func (t *GitHubProvider) getPRDiff(ctx context.Context, prNumber int) (string, error) {
	opts := &github.ListOptions{}
	files, _, err := t.gh.PullRequests.ListFiles(ctx, t.info.Owner, t.info.Repo, prNumber, opts)
	if err != nil {
		return "", fmt.Errorf("github list PR files: %w", err)
	}

	var sb strings.Builder
	for _, f := range files {
		sb.WriteString(fmt.Sprintf("--- %s\n+++ %s\n", f.GetFilename(), f.GetFilename()))
		sb.WriteString(f.GetPatch())
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

func (t *GitHubProvider) GetPR(ctx context.Context, prNumber int) (PR, error) {
	pr, _, err := t.gh.PullRequests.Get(ctx, t.info.Owner, t.info.Repo, prNumber)
	if err != nil {
		return PR{}, fmt.Errorf("github get PR: %w", err)
	}

	// Fetch the unified diff by requesting the PR with the diff media type.
	diff, err := t.getPRDiff(ctx, prNumber)
	if err != nil {
		return PR{}, err
	}

	return PR{
		Number:      pr.GetNumber(),
		Title:       pr.GetTitle(),
		Description: pr.GetBody(),
		URL:         pr.GetHTMLURL(),
		Branch:      pr.GetHead().GetRef(),
		BaseBranch:  pr.GetBase().GetRef(),
		Diff:        diff,
		IssueURL:    extractIssueURL(pr.GetBody()),
	}, nil
}

func verdictToGitHubEvent(verdict string) string {
	switch verdict {
	case "approve":
		return "APPROVE"
	case "request_changes":
		return "REQUEST_CHANGES"
	default:
		return "COMMENT"
	}
}

func extractIssueURL(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Closes ") {
			return strings.TrimPrefix(line, "Closes ")
		}
	}
	return ""
}
