package executor

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/jadenj13/droid/internals/git"
)

var toolReadFile = anthropic.ToolParam{
	Name:        "read_file",
	Description: anthropic.String("Read the contents of a file in the repository."),
	InputSchema: anthropic.ToolInputSchemaParam{
		Properties: map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the file relative to the repo root. E.g. 'internal/auth/handler.go'",
			},
		},
		Required: []string{"path"},
	},
}

var toolWriteFile = anthropic.ToolParam{
	Name:        "write_file",
	Description: anthropic.String("Write or overwrite a file in the repository. Creates intermediate directories as needed."),
	InputSchema: anthropic.ToolInputSchemaParam{
		Properties: map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the file relative to the repo root.",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "Full file content to write.",
			},
		},
		Required: []string{"path", "content"},
	},
}

var toolRunCommand = anthropic.ToolParam{
	Name:        "run_command",
	Description: anthropic.String("Run a shell command in the repository root. Use for building, testing, linting, and installing dependencies. Non-zero exit codes are returned as output, not errors."),
	InputSchema: anthropic.ToolInputSchemaParam{
		Properties: map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "Shell command to run. E.g. 'go test ./...' or 'npm run lint'",
			},
		},
		Required: []string{"command"},
	},
}

var toolListFiles = anthropic.ToolParam{
	Name:        "list_files",
	Description: anthropic.String("List files in the repository, optionally scoped to a subdirectory. Use this to understand the project structure before making changes."),
	InputSchema: anthropic.ToolInputSchemaParam{
		Properties: map[string]interface{}{
			"subdir": map[string]interface{}{
				"type":        "string",
				"description": "Subdirectory to list relative to repo root. Use '.' for the full repo.",
			},
		},
		Required: []string{"subdir"},
	},
}

var toolCommitChanges = anthropic.ToolParam{
	Name:        "commit_changes",
	Description: anthropic.String("Stage all changes and create a git commit. Call this after a coherent set of changes is complete — not after every file write."),
	InputSchema: anthropic.ToolInputSchemaParam{
		Properties: map[string]interface{}{
			"message": map[string]interface{}{
				"type":        "string",
				"description": "Commit message. Use the imperative mood. E.g. 'Add user authentication endpoint'",
			},
		},
		Required: []string{"message"},
	},
}

var toolSubmitWork = anthropic.ToolParam{
	Name:        "submit_work",
	Description: anthropic.String("Push the branch and open a pull/merge request. Call this only when all work is complete and tests pass."),
	InputSchema: anthropic.ToolInputSchemaParam{
		Properties: map[string]interface{}{
			"title": map[string]interface{}{
				"type":        "string",
				"description": "PR/MR title. Should reference the issue.",
			},
			"summary": map[string]interface{}{
				"type":        "string",
				"description": "Description of what was done and any relevant notes for the reviewer.",
			},
		},
		Required: []string{"title", "summary"},
	},
}

var AllTools = []anthropic.ToolParam{
	toolListFiles,
	toolReadFile,
	toolWriteFile,
	toolRunCommand,
	toolCommitChanges,
	toolSubmitWork,
}

type readFileInput struct {
	Path string `json:"path"`
}

type writeFileInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type runCommandInput struct {
	Command string `json:"command"`
}

type listFilesInput struct {
	Subdir string `json:"subdir"`
}

type commitChangesInput struct {
	Message string `json:"message"`
}

type submitWorkInput struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
}

type ToolResult struct {
	Content  string
	Done     bool   // true when submit_work is called — signals the loop to exit
	PRTitle  string // populated on submit_work
	PRSummary string
}

func ExecuteTool(ctx context.Context, name string, raw json.RawMessage, repo *git.Repo) (ToolResult, error) {
	switch name {
	case "read_file":
		return execReadFile(raw, repo)
	case "write_file":
		return execWriteFile(raw, repo)
	case "run_command":
		return execRunCommand(ctx, raw, repo)
	case "list_files":
		return execListFiles(ctx, raw, repo)
	case "commit_changes":
		return execCommitChanges(ctx, raw, repo)
	case "submit_work":
		return execSubmitWork(raw)
	default:
		return ToolResult{}, fmt.Errorf("unknown tool: %s", name)
	}
}

func execReadFile(raw json.RawMessage, repo *git.Repo) (ToolResult, error) {
	var in readFileInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return ToolResult{}, err
	}
	content, err := repo.ReadFile(in.Path)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("error: %s", err)}, nil
	}
	return ToolResult{Content: content}, nil
}

func execWriteFile(raw json.RawMessage, repo *git.Repo) (ToolResult, error) {
	var in writeFileInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return ToolResult{}, err
	}
	if err := repo.WriteFile(in.Path, in.Content); err != nil {
		return ToolResult{Content: fmt.Sprintf("error: %s", err)}, nil
	}
	return ToolResult{Content: fmt.Sprintf("wrote %s", in.Path)}, nil
}

func execRunCommand(ctx context.Context, raw json.RawMessage, repo *git.Repo) (ToolResult, error) {
	var in runCommandInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return ToolResult{}, err
	}
	out, err := repo.RunInDir(ctx, in.Command)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("error: %s", err)}, nil
	}
	return ToolResult{Content: out}, nil
}

func execListFiles(ctx context.Context, raw json.RawMessage, repo *git.Repo) (ToolResult, error) {
	var in listFilesInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return ToolResult{}, err
	}
	out, err := repo.ListFiles(ctx, in.Subdir)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("error: %s", err)}, nil
	}
	return ToolResult{Content: out}, nil
}

func execCommitChanges(ctx context.Context, raw json.RawMessage, repo *git.Repo) (ToolResult, error) {
	var in commitChangesInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return ToolResult{}, err
	}
	if err := repo.Add(ctx); err != nil {
		return ToolResult{Content: fmt.Sprintf("error staging: %s", err)}, nil
	}
	committed, err := repo.Commit(ctx, in.Message)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("error committing: %s", err)}, nil
	}
	if !committed {
		return ToolResult{Content: "nothing to commit — no changes detected"}, nil
	}
	return ToolResult{Content: fmt.Sprintf("committed: %s", in.Message)}, nil
}

func execSubmitWork(raw json.RawMessage) (ToolResult, error) {
	var in submitWorkInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return ToolResult{}, err
	}
	return ToolResult{
		Content:   "work submitted",
		Done:      true,
		PRTitle:   in.Title,
		PRSummary: in.Summary,
	}, nil
}
