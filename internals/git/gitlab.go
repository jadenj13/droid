package git

import (
	"context"
	"fmt"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type GitLabTracker struct {
	gl      *gitlab.Client
	info    RepoInfo
	baseURL string
}

func NewGitLabTracker(token, baseURL string, info RepoInfo) (*GitLabTracker, error) {
	gl, err := gitlab.NewClient(token, gitlab.WithBaseURL(baseURL+"/api/v4"))
	if err != nil {
		return nil, fmt.Errorf("gitlab client: %w", err)
	}
	return &GitLabTracker{gl: gl, info: info, baseURL: baseURL}, nil
}

func (t *GitLabTracker) RepoURL() string { return t.info.RawURL }

func (t *GitLabTracker) pid() string {
	return t.info.Owner + "/" + t.info.Repo
}

func (t *GitLabTracker) CreateIssue(ctx context.Context, input IssueInput) (Issue, error) {
	opts := &gitlab.CreateIssueOptions{
		Title:       gitlab.Ptr(input.Title),
		Description: gitlab.Ptr(input.Body),
		Labels:      (*gitlab.LabelOptions)(&input.Labels),
	}
	issue, _, err := t.gl.Issues.CreateIssue(t.pid(), opts, gitlab.WithContext(ctx))
	if err != nil {
		return Issue{}, fmt.Errorf("gitlab create issue: %w", err)
	}
	return Issue{
		Number: int(issue.IID), // IID is the project-scoped issue number
		Title:  issue.Title,
		URL:    issue.WebURL,
	}, nil
}

func (t *GitLabTracker) GetIssue(ctx context.Context, number int) (Issue, error) {
	issue, _, err := t.gl.Issues.GetIssue(t.pid(), int64(number), gitlab.WithContext(ctx))
	if err != nil {
		return Issue{}, fmt.Errorf("gitlab get issue: %w", err)
	}
	return Issue{
		Number: int(issue.IID),
		Title:  issue.Title,
		URL:    issue.WebURL,
	}, nil
}

func (t *GitLabTracker) AddLabel(ctx context.Context, number int, label string) error {
	opts := &gitlab.UpdateIssueOptions{
		AddLabels: (*gitlab.LabelOptions)(&[]string{label}),
	}
	_, _, err := t.gl.Issues.UpdateIssue(t.pid(), int64(number), opts, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("gitlab add label: %w", err)
	}
	return nil
}