package git

import "context"

type Tracker interface {
	CreateIssue(ctx context.Context, input IssueInput) (Issue, error)
	GetIssue(ctx context.Context, number int) (Issue, error)
	AddLabel(ctx context.Context, number int, label string) error
	RepoURL() string
}

type IssueInput struct {
	Title  string
	Body   string   // Markdown
	Labels []string // e.g. ["agent:ready", "backend"]
}

type Issue struct {
	Number int
	Title  string
	URL    string
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