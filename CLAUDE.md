# Droid — Claude Code Guide

## Project overview

Droid is a multi-agent autonomous development platform. Three Claude-powered services collaborate via GitHub/GitLab issue and PR labels:

```
Slack message → Planner (creates issues) → Executor (writes code, opens PR) → Reviewer (approves or requests revision)
```

## Architecture

### Three services
| Service | Entry point | Transport | Port |
|---------|------------|-----------|------|
| `planner` | `cmd/planner/` | Slack Socket Mode | — |
| `executor` | `cmd/executor/` | HTTP webhooks | `:8080` |
| `reviewer` | `cmd/reviewer/` | HTTP webhooks | `:8081` |

### Shared internals (`internals/`)
- `llm/` — Anthropic SDK wrapper with exponential-backoff retry (max 4 retries, jitter up to 30s)
- `git/` — Factory pattern that resolves GitHub vs GitLab from repo URL; local git ops
- `slack/` — Socket Mode listener used by the planner

### Agentic loop pattern
All three agents follow the same skeleton:
1. Build a system prompt with tools defined as JSON schemas
2. Call the LLM via `llm.Client`
3. Extract tool calls from the response
4. Execute each tool (file I/O, shell, API calls)
5. Append results to the message history
6. Loop until `stop_reason == "end_turn"` or the iteration limit is reached

Limits: planner = 10 iterations, executor = 50 iterations, reviewer = single call (up to 5 revision rounds via webhook re-trigger).

### Label-driven workflow
| Label | Set by | Triggers |
|-------|--------|----------|
| `agent:ready` | Planner | Executor starts implementation |
| `agent:review` | Executor | Reviewer fetches PR diff and reviews |
| `agent:revision` | Reviewer | Executor re-runs on the same PR |
| `agent:approved` | Reviewer | Slack notification sent, cycle ends |

## Development commands

```sh
# Build all three binaries → ./bin/
make build

# Run individual services (each in its own terminal)
make run-planner
make run-executor
make run-reviewer

# Tests and linting
make test      # go test ./...
make lint      # go vet ./...
make clean     # remove ./bin/

# Docker
make docker-build
make docker-up
make docker-down
make docker-logs
make docker-service SERVICE=executor   # single service
```

## Environment

```sh
cp .env.example .env
```

Required variables: `ANTHROPIC_API_KEY`, `SLACK_BOT_TOKEN`, `SLACK_APP_TOKEN`, `SLACK_NOTIFY_CHANNEL`, `GITHUB_WEBHOOK_SECRET`, `GITLAB_WEBHOOK_SECRET`.
Optional: `GITHUB_TOKEN`, `GITLAB_TOKEN`, `EXECUTOR_ADDR`, `REVIEWER_ADDR`.

## Code conventions

- **Go 1.25** — use standard library where possible; avoid adding new dependencies
- **Logging** — `log/slog` with text handler; propagate errors with `fmt.Errorf("context: %w", err)`
- **Error handling** — return errors up the call stack; log at the service boundary (main or webhook handler), not deep in helpers
- **Tool definitions** — each agent owns its tools in a `tools.go` file as `anthropic.ToolParam` slices; keep tool names snake_case
- **No global state** — agents are stateless structs; the planner `SessionStore` is the only in-memory state
- **Provider abstraction** — always go through the `git.Factory` interface; never call GitHub or GitLab APIs directly from agent code

## Key files

| File | What it does |
|------|-------------|
| `internals/executor/agent.go` | Core executor agentic loop |
| `internals/executor/tools.go` | Tool definitions: `read_file`, `write_file`, `run_command`, `list_files`, `commit_changes`, `create_pr` |
| `internals/planner/agent.go` | Planner loop + interactive refinement |
| `internals/planner/session.go` | Per-thread session store |
| `internals/reviewer/agent.go` | Single-call review logic |
| `internals/reviewer/notifier.go` | Slack approval notification |
| `internals/git/resolver.go` | Parses repo URLs → GitHub or GitLab |
| `internals/llm/anthropic.go` | Anthropic API client with retry |

## Adding a new tool to an agent

1. Define the tool schema in the agent's `tools.go` as an `anthropic.ToolParam`
2. Add a handler case in the agent's tool-dispatch switch in `agent.go`
3. Update the agent's system prompt if the tool needs to be explained

## Adding a new agent

Follow the existing pattern:
- `cmd/<name>/main.go` — read env, construct agent, start transport (Slack or HTTP)
- `internals/<name>/agent.go` — agentic loop
- `internals/<name>/tools.go` — tool definitions
- `internals/<name>/webhook.go` (if HTTP) — validate signature, parse event, call worker
- Add a service to `docker-compose.yml` and a `run-<name>` target to the `Makefile`
