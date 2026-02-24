package git

import (
	"context"
	"fmt"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type GitLabProvider struct {
	gl      *gitlab.Client
	info    RepoInfo
	baseURL string
}

func NewGitLabProvider(token, baseURL string, info RepoInfo) (*GitLabProvider, error) {
	gl, err := gitlab.NewClient(token, gitlab.WithBaseURL(baseURL+"/api/v4"))
	if err != nil {
		return nil, fmt.Errorf("gitlab client: %w", err)
	}
	return &GitLabProvider{gl: gl, info: info, baseURL: baseURL}, nil
}

func (t *GitLabProvider) RepoURL() string { return t.info.RawURL }

func (t *GitLabProvider) pid() string {
	return t.info.Owner + "/" + t.info.Repo
}

func (t *GitLabProvider) CreateIssue(ctx context.Context, input IssueInput) (Issue, error) {
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

func (t *GitLabProvider) GetIssue(ctx context.Context, number int) (Issue, error) {
	issue, _, err := t.gl.Issues.GetIssue(t.pid(), int64(number), gitlab.WithContext(ctx))
	if err != nil {
		return Issue{}, fmt.Errorf("gitlab get issue: %w", err)
	}
	return Issue{
		Number: int(issue.IID),
		Title:  issue.Title,
		Body:   issue.Description,
		URL:    issue.WebURL,
	}, nil
}

func (t *GitLabProvider) AddLabel(ctx context.Context, number int, label string) error {
	opts := &gitlab.UpdateIssueOptions{
		AddLabels: (*gitlab.LabelOptions)(&[]string{label}),
	}
	_, _, err := t.gl.Issues.UpdateIssue(t.pid(), int64(number), opts, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("gitlab add label: %w", err)
	}
	return nil
}

func (t *GitLabProvider) OpenPR(ctx context.Context, input PRInput) (string, error) {
	mr, _, err := t.gl.MergeRequests.CreateMergeRequest(t.pid(), &gitlab.CreateMergeRequestOptions{
		Title:        gitlab.Ptr(input.Title),
		Description:  gitlab.Ptr(input.Body),
		SourceBranch: gitlab.Ptr(input.Branch),
		TargetBranch: gitlab.Ptr(input.Base),
	}, gitlab.WithContext(ctx))
	if err != nil {
		return "", fmt.Errorf("gitlab open MR: %w", err)
	}
	return mr.WebURL, nil
}

func (t *GitLabProvider) GetPR(ctx context.Context, prNumber int) (PR, error) {
	mr, _, err := t.gl.MergeRequests.GetMergeRequest(t.pid(), int64(prNumber), nil, gitlab.WithContext(ctx))
	if err != nil {
		return PR{}, fmt.Errorf("gitlab get MR: %w", err)
	}

	diff, err := t.getMRDiff(ctx, prNumber)
	if err != nil {
		return PR{}, err
	}

	return PR{
		Number:      int(mr.IID),
		Title:       mr.Title,
		Description: mr.Description,
		URL:         mr.WebURL,
		Branch:      mr.SourceBranch,
		BaseBranch:  mr.TargetBranch,
		Diff:        diff,
		IssueURL:    extractIssueURL(mr.Description),
	}, nil
}

func (t *GitLabProvider) getMRDiff(ctx context.Context, mrNumber int) (string, error) {
	diffs, _, err := t.gl.MergeRequests.ListMergeRequestDiffs(t.pid(), int64(mrNumber), nil, gitlab.WithContext(ctx))
	if err != nil {
		return "", fmt.Errorf("gitlab get MR diff: %w", err)
	}

	var sb strings.Builder
	for _, d := range diffs {
		sb.WriteString(fmt.Sprintf("--- %s\n+++ %s\n", d.OldPath, d.NewPath))
		sb.WriteString(d.Diff)
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

func (t *GitLabProvider) PostReview(ctx context.Context, prNumber int, review Review) error {
	_, _, err := t.gl.Notes.CreateMergeRequestNote(t.pid(), int64(prNumber), &gitlab.CreateMergeRequestNoteOptions{
		Body: gitlab.Ptr(review.Summary),
	}, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("gitlab post review note: %w", err)
	}

	for _, c := range review.Comments {
		side := "new" // GitLab uses "new"/"old" instead of "RIGHT"/"LEFT"
		if c.Side == "LEFT" {
			side = "old"
		}
		_, _, err := t.gl.Discussions.CreateMergeRequestDiscussion(t.pid(), int64(prNumber), &gitlab.CreateMergeRequestDiscussionOptions{
			Body: gitlab.Ptr(c.Body),
			Position: &gitlab.PositionOptions{
				PositionType: gitlab.Ptr("text"),
				NewPath:      gitlab.Ptr(c.Path),
				NewLine:      gitlab.Ptr(int64(c.Line)),
				LineRange: &gitlab.LineRangeOptions{
					Start: &gitlab.LinePositionOptions{
						Type: gitlab.Ptr(side),
					},
				},
			},
		}, gitlab.WithContext(ctx))
		if err != nil {
			// Non-fatal — line number mapping can fail if the diff shifts.
			// Log and continue rather than aborting the whole review.
			_ = err
		}
	}

	if review.Verdict == "approve" {
		_, _, err = t.gl.MergeRequestApprovals.ApproveMergeRequest(t.pid(), int64(prNumber), &gitlab.ApproveMergeRequestOptions{}, gitlab.WithContext(ctx))
		if err != nil {
			return fmt.Errorf("gitlab approve MR: %w", err)
		}
	}

	return nil
}

func (t *GitLabProvider) GetPRComments(ctx context.Context, prNumber int) ([]PRComment, error) {
	notes, _, err := t.gl.Notes.ListMergeRequestNotes(t.pid(), int64(prNumber), nil, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("gitlab get MR comments: %w", err)
	}
	out := make([]PRComment, 0, len(notes))
	for _, n := range notes {
		if n.System {
			continue // skip system notes like "added label"
		}
		out = append(out, PRComment{
			Body: n.Body,
		})
	}
	return out, nil
}
