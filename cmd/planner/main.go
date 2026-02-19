package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jadenj13/droid/internals/llm"
	"github.com/jadenj13/droid/internals/planner"
	slackhandler "github.com/jadenj13/droid/internals/slack"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	botToken   := mustEnv("SLACK_BOT_TOKEN")
	appToken   := mustEnv("SLACK_APP_TOKEN")
	anthropicKey := mustEnv("ANTHROPIC_API_KEY")

	sessions := planner.NewSessionStore()

	llmClient := llm.NewClient(anthropicKey)
	// Optionally override model or token limit:
	// llm.NewClient(anthropicKey, llm.WithModel(anthropic.ModelClaude3OpusLatest))
	// llm.NewClient(anthropicKey, llm.WithMaxTokens(4096))

	agent := planner.NewAgent(sessions, llmClient, log)

	handler, err := slackhandler.NewHandler(botToken, appToken, agent, log)
	if err != nil {
		log.Error("failed to create slack handler", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Info("planner starting")
	if err := handler.Run(ctx); err != nil {
		log.Error("handler exited with error", "err", err)
		os.Exit(1)
	}
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		slog.Error("missing required environment variable", "key", key)
		os.Exit(1)
	}
	return v
}