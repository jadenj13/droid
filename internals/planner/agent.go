package planner

import (
	"context"
	"fmt"
	"log/slog"

	slackhandler "github.com/jadenj13/droid/internals/slack"
)

type Agent struct {
	sessions *SessionStore
	llm      LLM
	log      *slog.Logger
}

type LLM interface {
	Complete(ctx context.Context, system string, messages []Message) (string, error)
}

func NewAgent(sessions *SessionStore, llm LLM, log *slog.Logger) *Agent {
	return &Agent{sessions: sessions, llm: llm, log: log}
}

func (a *Agent) Handle(ctx context.Context, msg slackhandler.IncomingMessage) (string, error) {
	sess := a.sessions.GetOrCreate(msg.ThreadTS, msg.ChannelID)

	if err := a.sessions.AppendMessage(sess, "user", msg.Text); err != nil {
		return "", fmt.Errorf("append user message: %w", err)
	}

	system := systemPrompt(sess)

	reply, err := a.llm.Complete(ctx, system, sess.Messages)
	if err != nil {
		return "", fmt.Errorf("llm complete: %w", err)
	}

	if err := a.sessions.AppendMessage(sess, "assistant", reply); err != nil {
		return "", fmt.Errorf("append assistant message: %w", err)
	}

	a.log.Info("planner reply", "stage", sess.Stage, "thread", sess.ThreadTS)
	return reply, nil
}

func systemPrompt(sess *Session) string {
	base := `You are a technical project planning assistant embedded in Slack.
Your job is to help the user plan software projects and features by working through:
1. Understanding the problem and goals (brainstorm)
2. Writing a clear Product Requirements Document (PRD)
3. Defining acceptance criteria
4. Breaking the work into discrete GitHub issues

Guidelines:
- Ask clarifying questions before writing any documents.
- Be concise in Slack — use bullet points, avoid walls of text.
- When writing PRDs or acceptance criteria, be specific and testable.
- Only move to the next stage when the user confirms they're happy.
- When creating issues, make each one small enough for a single engineer to complete in a day or two.
`

	switch sess.Stage {
	case StageBrainstorm:
		base += `
Current stage: BRAINSTORM
Help the user articulate what they're building and why. Ask about:
- The problem being solved
- Who the users are
- What success looks like
- Any known constraints or dependencies
When you have enough context, suggest moving to writing the PRD.`

	case StagePRD:
		base += `
Current stage: PRD
Write a structured PRD with these sections:
- Overview
- Problem Statement  
- Goals & Non-goals
- User Stories
- Technical Approach (high level)
- Open Questions
Present it in full, then ask the user for feedback.`

	case StageCriteria:
		base += `
Current stage: ACCEPTANCE CRITERIA
Based on the PRD, write clear, testable acceptance criteria.
Format each as: "Given [context], when [action], then [outcome]".
Group them by feature area if there are many.`

	case StageIssues:
		base += `
Current stage: ISSUE BREAKDOWN
Break the work into GitHub issues. For each issue provide:
- A clear, action-oriented title
- A short description (2-3 sentences)
- Acceptance criteria (subset from the full AC list)
- Any dependencies on other issues
Keep issues small and independently deliverable.`

	case StageDone:
		base += `
Current stage: DONE
All issues have been created. Help the user review the plan or answer questions.
If they want to add more work, you can create additional issues.`
	}

	if sess.PRDDraft != "" {
		base += "\n\nCurrent PRD draft:\n" + sess.PRDDraft
	}

	return base
}