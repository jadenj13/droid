Investigate and diagnose a problem with a droid agent.

Steps:
1. Identify which agent is misbehaving: planner, executor, or reviewer.
2. Read the agent's `agent.go`, `tools.go`, and (if HTTP) `webhook.go`.
3. Check the agentic loop for the reported symptom:
   - **Stuck / infinite loop** — look at the iteration limit and the `stop_reason` handling; verify every tool call returns a result appended to messages
   - **Tool not called** — verify the tool is included in the `anthropic.ToolParam` slice passed to the LLM call; check the system prompt
   - **Webhook not firing** — check signature verification logic in `webhook.go` and ensure the correct label/event type is being matched
   - **LLM error / rate limit** — check `internals/llm/anthropic.go` retry logic; look for missing `ANTHROPIC_API_KEY`
   - **Git/API failure** — check `internals/git/` for the relevant provider; verify token scopes
4. Propose a minimal fix. Do not refactor surrounding code unless it is the direct cause of the bug.
5. Run `make build` and `make test` after the fix.

Context: $ARGUMENTS
