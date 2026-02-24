# droid

Automated software development through AI agents. Droid orchestrates three Claude-powered agents that plan features, write code, and review pull requests with minimal human involvement.

## How it works

Droid runs three independent services that hand work off to each other via issue/PR labels:

```
You (Slack)
    │
    ▼
┌─────────┐   creates issues    ┌──────────┐   opens PR     ┌──────────┐
│ Planner │ ─────────────────► │ Executor │ ─────────────► │ Reviewer │
│         │   label:agent:ready │          │ label:agent:   │          │
│  Slack  │                     │ Webhook  │       review   │ Webhook  │
└─────────┘                     └──────────┘                └──────────┘
                                      ▲                          │
                                      │    label:agent:revision  │
                                      └──────────────────────────┘
```

1. **You** describe a feature to the Planner in Slack
2. **Planner** breaks it down interactively and creates GitHub/GitLab issues labeled `agent:ready`
3. **Executor** picks up the issue, clones the repo, writes the code, and opens a PR
4. **Reviewer** reviews the PR diff against the original issue; if changes are needed it labels the PR `agent:revision` and the Executor iterates
5. When the Reviewer approves, it labels the issue `agent:approved` and notifies Slack

## Agents

### Planner
Listens for Slack mentions or DMs. Guides you through a planning session — brainstorming, writing a product spec, defining acceptance criteria — then creates structured issues on GitHub or GitLab. Each issue gets the `agent:ready` label to trigger the Executor.

### Executor
An HTTP server that receives webhooks when an issue is labeled `agent:ready` (or `agent:revision` for re-work). It clones the repository, runs an agentic loop with file read/write and shell execution tools, commits its changes, and opens a pull request. The loop runs up to 50 iterations before giving up.

### Reviewer
An HTTP server that receives webhooks when a PR is labeled `agent:review`. It fetches the PR diff and the original issue, then makes a single LLM call to produce a structured review with a verdict (`approve`, `request_changes`, or `comment`) and optional inline comments. Up to 5 revision rounds are allowed before the cycle stops.

## Prerequisites

- Go 1.23+
- An [Anthropic API key](https://console.anthropic.com/)
- A Slack app with **Socket Mode** enabled (for the Planner)
- A GitHub token and/or GitLab token with repo + issue permissions
- A publicly reachable URL for the Executor and Reviewer webhooks (e.g. via [ngrok](https://ngrok.com/) for local dev)

## Environment variables

Copy `.env.example` to `.env` and fill in your values:

```sh
cp .env.example .env
```

| Variable | Required by | Description |
|---|---|---|
| `ANTHROPIC_API_KEY` | all | Anthropic API key |
| `SLACK_BOT_TOKEN` | planner, reviewer | Bot token (`xoxb-...`) |
| `SLACK_APP_TOKEN` | planner | App-level token for Socket Mode (`xapp-...`) |
| `SLACK_NOTIFY_CHANNEL` | reviewer | Channel ID to post approval notifications |
| `GITHUB_TOKEN` | all | Personal access token with `repo` scope |
| `GITLAB_TOKEN` | all | Personal access token with `api` scope |
| `GITHUB_WEBHOOK_SECRET` | executor, reviewer | Secret used to verify GitHub webhook signatures |
| `GITLAB_WEBHOOK_SECRET` | executor, reviewer | Secret used to verify GitLab webhook signatures |
| `EXECUTOR_ADDR` | executor | Address to listen on (default `:8080`) |
| `REVIEWER_ADDR` | reviewer | Address to listen on (default `:8081`) |

`GITHUB_TOKEN` and `GITLAB_TOKEN` are both optional individually — you only need the one(s) matching your repos.

## Slack app setup

1. Go to [api.slack.com/apps](https://api.slack.com/apps) and create a new app **from scratch**
2. Under **Socket Mode**, enable it and generate an App-Level Token with the `connections:write` scope — this is your `SLACK_APP_TOKEN`
3. Under **OAuth & Permissions**, add these Bot Token Scopes:
   - `app_mentions:read`
   - `chat:write`
   - `im:history`
   - `im:read`
4. Install the app to your workspace and copy the Bot User OAuth Token — this is your `SLACK_BOT_TOKEN`
5. Under **Event Subscriptions**, enable events and subscribe to:
   - `app_mention`
   - `message.im`

## Webhook setup

The Executor listens on `/webhook/github` and `/webhook/gitlab`. The Reviewer does the same. Register each URL in your GitHub/GitLab repository settings.

**GitHub** (Settings → Webhooks → Add webhook):
- Executor: `https://your-host:8080/webhook/github`
- Reviewer: `https://your-host:8081/webhook/github`
- Content type: `application/json`
- Events: **Issues** and **Pull requests**
- Use the same secret for `GITHUB_WEBHOOK_SECRET`

**GitLab** (Settings → Webhooks):
- Executor: `https://your-host:8080/webhook/gitlab`
- Reviewer: `https://your-host:8081/webhook/gitlab`
- Triggers: **Issues events** and **Merge request events**
- Use the same secret for `GITLAB_WEBHOOK_SECRET`

## Running

Start each service in a separate terminal:

```sh
# Planner (Slack bot)
go run ./cmd/planner

# Executor (webhook server, default :8080)
go run ./cmd/executor

# Reviewer (webhook server, default :8081)
go run ./cmd/reviewer
```

Or build all three binaries:

```sh
go build -o bin/ ./cmd/...
./bin/planner &
./bin/executor &
./bin/reviewer &
```

Environment variables are read from the process environment. Use a tool like [direnv](https://direnv.net/) or `export $(cat .env | xargs)` to load your `.env` file.

## Issue labels

Droid uses labels to move work through the pipeline. Create these labels in your repository:

| Label | Set by | Meaning |
|---|---|---|
| `agent:ready` | Planner | Issue is ready for the Executor to implement |
| `agent:review` | Executor | PR is ready for the Reviewer |
| `agent:revision` | Reviewer | Executor should revise and push updates |
| `agent:approved` | Reviewer | PR has been approved |

## Repository structure

```
cmd/
  planner/    # Slack bot entry point
  executor/   # Webhook server entry point
  reviewer/   # Webhook server entry point
internals/
  git/        # GitHub & GitLab API clients, local git operations
  llm/        # Anthropic API client with retry logic
  planner/    # Planning agent, session management, tools
  executor/   # Execution agent, webhook handler, tools
  reviewer/   # Review agent, webhook handler, revision loop
  slack/      # Slack socket-mode handler
```
