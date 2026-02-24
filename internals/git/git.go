package git

import "context"

type GitProvider interface {
	CreateIssue(ctx context.Context, input IssueInput) (Issue, error)
	GetIssue(ctx context.Context, number int) (Issue, error)
	AddLabel(ctx context.Context, number int, label string) error
	OpenPR(ctx context.Context, input PRInput) (string, error)
	GetPR(ctx context.Context, prNumber int) (PR, error)
	PostReview(ctx context.Context, prNumber int, review Review) error
	GetPRComments(ctx context.Context, prNumber int) ([]PRComment, error)
	RepoURL() string
}

type PRInput struct {
	Title       string
	Body        string
	Branch      string
	Base        string
	IssueNumber int
	Draft       bool
}

type IssueInput struct {
	Title  string
	Body   string   // Markdown
	Labels []string // e.g. ["agent:ready", "backend"]
}

type Issue struct {
	Number int
	Title  string
	Body   string
	URL    string
}

type PR struct {
	Number      int
	Title       string
	Description string // the PR body written by the executor
	URL         string
	Branch      string
	BaseBranch  string
	Diff        string // unified diff of all changes
	IssueURL    string // the originating issue URL parsed from the PR body
}

type Review struct {
	// Verdict is one of "approve", "request_changes", or "comment".
	Verdict  string
	Summary  string // overall review comment
	Comments []PRComment
}

type PRComment struct {
	Path string // file path
	Line int    // line number in the diff
	Body string // comment text
	// Side is "RIGHT" (new file) or "LEFT" (old file). Defaults to RIGHT.
	Side string
}

type Platform int

const (
	PlatformGitHub Platform = iota
	PlatformGitLab
)

func (p Platform) String() string {
	switch p {
	case PlatformGitHub:
		return "github"
	case PlatformGitLab:
		return "gitlab"
	default:
		return "unknown"
	}
}
