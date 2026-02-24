package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jadenj13/droid/internals/git"
	"github.com/jadenj13/droid/internals/llm"
	"github.com/jadenj13/droid/internals/reviewer"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	anthropicKey := mustEnv("ANTHROPIC_API_KEY")
	githubToken := os.Getenv("GITHUB_TOKEN")
	gitlabToken := os.Getenv("GITLAB_TOKEN")
	githubSecret := os.Getenv("GITHUB_WEBHOOK_SECRET")
	gitlabSecret := os.Getenv("GITLAB_WEBHOOK_SECRET")
	slackToken := mustEnv("SLACK_BOT_TOKEN")
	slackChannel := mustEnv("SLACK_NOTIFY_CHANNEL") // e.g. "C01234ABCDE" (channel ID)
	addr := envOr("REVIEWER_ADDR", ":8081")

	llmClient := llm.NewClient(anthropicKey, llm.WithMaxTokens(16000))
	factory := git.NewFactory(githubToken, gitlabToken)
	notifier := reviewer.NewSlackNotifier(slackToken, slackChannel)
	agent := reviewer.NewAgent(llmClient, log)
	worker := reviewer.NewWorker(agent, factory, notifier, log)
	webhook := reviewer.NewWebhookServer(worker, githubSecret, gitlabSecret, log)

	srv := &http.Server{
		Addr:         addr,
		Handler:      webhook.Handler(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Info("reviewer webhook listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	srv.Shutdown(shutCtx)
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		slog.Error("missing required env var", "key", key)
		os.Exit(1)
	}
	return v
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
