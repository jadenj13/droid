Scaffold a brand-new droid agent end-to-end.

Steps:
1. Ask for the agent name, its responsibility, what triggers it (Slack message or HTTP webhook), and what tools it needs.
2. Read `internals/executor/agent.go` and `internals/executor/tools.go` as reference for the agentic loop pattern.
3. Create the following files, following existing conventions exactly:
   - `cmd/<name>/main.go` — read env vars, construct the agent, start the transport
   - `internals/<name>/agent.go` — agentic loop (call LLM → dispatch tools → loop)
   - `internals/<name>/tools.go` — `anthropic.ToolParam` slice for the agent's tools
   - `internals/<name>/webhook.go` (HTTP agents only) — signature verification + event dispatch
4. Add a service entry to `docker-compose.yml` (copy the executor block and adjust the binary name and port).
5. Add `run-<name>` and optionally `docker-service` support to the `Makefile`.
6. Run `make build` and fix any compilation errors.
7. Summarise what the new agent does and what env vars it needs.

Context: $ARGUMENTS
