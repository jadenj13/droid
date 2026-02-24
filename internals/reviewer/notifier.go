package reviewer

import (
	"context"
	"fmt"

	"github.com/slack-go/slack"
)

type SlackNotifier struct {
	client    *slack.Client
	channelID string // channel to post approval notifications to
}

func NewSlackNotifier(botToken, channelID string) *SlackNotifier {
	return &SlackNotifier{
		client:    slack.New(botToken),
		channelID: channelID,
	}
}

func (n *SlackNotifier) NotifyPRReady(ctx context.Context, msg PRReadyMessage) error {
	text := fmt.Sprintf(
		":white_check_mark: *PR ready for your review*\n"+
			"*<%s|%s>*\n"+
			"Issue: <%s|%s>\n"+
			"Repo: %s",
		msg.PRURL, msg.PRTitle,
		msg.IssueURL, msg.IssueTitle,
		msg.RepoURL,
	)

	_, _, err := n.client.PostMessageContext(ctx, n.channelID,
		slack.MsgOptionText(text, false),
	)
	if err != nil {
		return fmt.Errorf("slack notify: %w", err)
	}
	return nil
}
