package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Repo struct {
	dir string // absolute path to the working tree
}

func Clone(ctx context.Context, repoURL, token string) (*Repo, error) {
	dir, err := os.MkdirTemp("", "agent-executor-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}

	authedURL, err := injectToken(repoURL, token)
	if err != nil {
		os.RemoveAll(dir)
		return nil, err
	}

	if _, err := run(ctx, "", "git", "clone", "--depth=1", authedURL, dir); err != nil {
		os.RemoveAll(dir)
		return nil, fmt.Errorf("git clone: %w", err)
	}

	if _, err := run(ctx, dir, "git", "config", "user.email", "agent@localhost"); err != nil {
		return nil, err
	}
	if _, err := run(ctx, dir, "git", "config", "user.name", "Executor Agent"); err != nil {
		return nil, err
	}

	return &Repo{dir: dir}, nil
}

func (r *Repo) Dir() string { return r.dir }

func (r *Repo) Cleanup() { os.RemoveAll(r.dir) }

func (r *Repo) CreateBranch(ctx context.Context, name string) error {
	_, err := run(ctx, r.dir, "git", "checkout", "-b", name)
	return err
}

func (r *Repo) CurrentBranch(ctx context.Context) (string, error) {
	out, err := run(ctx, r.dir, "git", "rev-parse", "--abbrev-ref", "HEAD")
	return strings.TrimSpace(out), err
}

func (r *Repo) Add(ctx context.Context) error {
	_, err := run(ctx, r.dir, "git", "add", "-A")
	return err
}

func (r *Repo) Commit(ctx context.Context, message string) (bool, error) {
	out, err := run(ctx, r.dir, "git", "status", "--porcelain")
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(out) == "" {
		return false, nil // nothing to commit
	}
	_, err = run(ctx, r.dir, "git", "commit", "-m", message)
	return err == nil, err
}

func (r *Repo) Push(ctx context.Context) error {
	branch, err := r.CurrentBranch(ctx)
	if err != nil {
		return err
	}
	_, err = run(ctx, r.dir, "git", "push", "origin", branch)
	return err
}

func (r *Repo) Diff(ctx context.Context) (string, error) {
	return run(ctx, r.dir, "git", "diff", "HEAD")
}

func BranchName(issueNumber int, title string) string {
	slug := strings.ToLower(title)
	replacer := strings.NewReplacer(" ", "-", "/", "-", "\\", "-", ":", "", ".", "")
	slug = replacer.Replace(slug)
	// Trim to a reasonable length.
	if len(slug) > 50 {
		slug = slug[:50]
	}
	slug = strings.Trim(slug, "-")
	return fmt.Sprintf("agent/issue-%d-%s", issueNumber, slug)
}

// injectToken rewrites an HTTPS URL to include the token as a credential.
// e.g. https://github.com/org/repo → https://x-token:TOKEN@github.com/org/repo
func injectToken(repoURL, token string) (string, error) {
	if token == "" {
		return repoURL, nil
	}
	if !strings.HasPrefix(repoURL, "https://") {
		return "", fmt.Errorf("token injection only supported for HTTPS URLs, got: %s", repoURL)
	}
	return strings.Replace(repoURL, "https://", fmt.Sprintf("https://x-token:%s@", token), 1), nil
}

func run(ctx context.Context, dir string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("run %q: %w\nstderr: %s", name+" "+strings.Join(args, " "), err, stderr.String())
	}
	return stdout.String(), nil
}

func (r *Repo) RunInDir(ctx context.Context, command string) (string, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = r.dir

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	_ = cmd.Run()
	out := buf.String()

	const maxBytes = 8000
	if len(out) > maxBytes {
		out = out[:maxBytes] + fmt.Sprintf("\n... (truncated, %d bytes total)", len(out))
	}
	return out, nil
}

func (r *Repo) ReadFile(relPath string) (string, error) {
	abs := filepath.Join(r.dir, relPath)
	b, err := os.ReadFile(abs)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", relPath, err)
	}
	return string(b), nil
}

func (r *Repo) WriteFile(relPath, content string) error {
	abs := filepath.Join(r.dir, relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(relPath), err)
	}
	return os.WriteFile(abs, []byte(content), 0644)
}

func (r *Repo) ListFiles(ctx context.Context, subdir string) (string, error) {
	target := filepath.Join(r.dir, subdir)
	out, err := run(ctx, r.dir, "find", target,
		"-not", "-path", "*/.git/*",
		"-not", "-path", "*/node_modules/*",
		"-not", "-path", "*/__pycache__/*",
	)
	if err != nil {
		return "", err
	}
	
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimPrefix(l, r.dir+"/")
	}
	const maxLines = 200
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		lines = append(lines, fmt.Sprintf("... (%d more files)", len(lines)-maxLines))
	}
	return strings.Join(lines, "\n"), nil
}