package git

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

type RepoInfo struct {
	Platform Platform
	Host     string // e.g. "github.com" or "gitlab.mycompany.com"
	Owner    string
	Repo     string
	RawURL   string
}

func ParseRepoURL(rawURL string) (RepoInfo, error) {
	rawURL = strings.TrimSpace(rawURL)

	if strings.HasPrefix(rawURL, "git@") {
		rawURL = normaliseSSH(rawURL)
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return RepoInfo{}, fmt.Errorf("invalid URL %q: %w", rawURL, err)
	}

	host := strings.ToLower(u.Hostname())
	platform, err := detectPlatform(host)
	if err != nil {
		return RepoInfo{}, err
	}

	path := strings.TrimPrefix(u.Path, "/")
	path = strings.TrimSuffix(path, ".git")

	parts := strings.Split(path, "/")

	switch platform {
	case PlatformGitHub:
		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			return RepoInfo{}, fmt.Errorf("github URL must have owner and repo: %q", rawURL)
		}
		return RepoInfo{
			Platform: PlatformGitHub,
			Host:     host,
			Owner:    parts[0],
			Repo:     parts[1],
			RawURL:   rawURL,
		}, nil

	case PlatformGitLab:
		if len(parts) < 2 || parts[len(parts)-1] == "" {
			return RepoInfo{}, fmt.Errorf("gitlab URL must have at least namespace and repo: %q", rawURL)
		}
		repo := parts[len(parts)-1]
		owner := strings.Join(parts[:len(parts)-1], "/")
		return RepoInfo{
			Platform: PlatformGitLab,
			Host:     host,
			Owner:    owner,
			Repo:     repo,
			RawURL:   rawURL,
		}, nil
	}

	return RepoInfo{}, fmt.Errorf("unsupported platform for host %q", host)
}

func detectPlatform(host string) (Platform, error) {
	switch {
	case host == "github.com" || strings.HasSuffix(host, ".github.com"):
		return PlatformGitHub, nil
	case host == "gitlab.com" || strings.Contains(host, "gitlab"):
		return PlatformGitLab, nil
	default:
		return 0, fmt.Errorf(
			"cannot determine platform from host %q — expected a github.com or gitlab domain",
			host,
		)
	}
}

func normaliseSSH(s string) string {
	s = strings.TrimPrefix(s, "git@")
	s = strings.Replace(s, ":", "/", 1)
	return "https://" + s
}

type Factory struct {
	githubToken string
	gitlabToken string
	gitlabBaseURL string
}

type FactoryOption func(*Factory)

func WithGitLabBaseURL(baseURL string) FactoryOption {
	return func(f *Factory) { f.gitlabBaseURL = baseURL }
}

func NewFactory(githubToken, gitlabToken string, opts ...FactoryOption) *Factory {
	f := &Factory{
		githubToken:   githubToken,
		gitlabToken:   gitlabToken,
		gitlabBaseURL: "https://gitlab.com",
	}
	for _, o := range opts {
		o(f)
	}
	return f
}

func (f *Factory) TrackerFor(ctx context.Context, repoURL string) (Tracker, RepoInfo, error) {
	info, err := ParseRepoURL(repoURL)
	if err != nil {
		return nil, RepoInfo{}, err
	}

	switch info.Platform {
	case PlatformGitHub:
		if f.githubToken == "" {
			return nil, info, fmt.Errorf("no GitHub token configured")
		}
		t, err := NewGitHubTracker(ctx, f.githubToken, info)
		return t, info, err

	case PlatformGitLab:
		if f.gitlabToken == "" {
			return nil, info, fmt.Errorf("no GitLab token configured")
		}
		baseURL := f.gitlabBaseURL
		// For self-hosted: use the URL's scheme+host instead of the default.
		if info.Host != "gitlab.com" {
			parsed, _ := url.Parse(info.RawURL)
			baseURL = parsed.Scheme + "://" + parsed.Host
		}
		t, err := NewGitLabTracker(f.gitlabToken, baseURL, info)
		return t, info, err
	}

	return nil, info, fmt.Errorf("unsupported platform: %s", info.Platform)
}