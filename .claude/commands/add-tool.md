Add a new tool to one of the droid agents (planner, executor, or reviewer).

Steps:
1. Ask which agent should receive the tool and get a description of what it should do.
2. Read that agent's `internals/<agent>/tools.go` and `internals/<agent>/agent.go`.
3. Add the tool schema as an `anthropic.ToolParam` in `tools.go` — follow the existing style exactly (snake_case name, JSON schema `input_schema`, description in plain English).
4. Add a handler case in the tool-dispatch switch inside `agent.go`. The case key must match the tool name defined in step 3.
5. If the tool requires a new helper (e.g., a new git operation), add it to the appropriate `internals/` package; do not inline complex logic in the switch case.
6. Update the agent's system prompt string if the new tool needs usage instructions.
7. Run `make build` and fix any compilation errors before finishing.

Context: $ARGUMENTS
