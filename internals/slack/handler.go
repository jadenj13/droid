package slack

import (
	"context"
	"log/slog"
	"strings"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

type Handler struct {
	client  *slack.Client
	socket  *socketmode.Client
	botID   string
	planner Planner
	log     *slog.Logger
}

type Planner interface {
	Handle(ctx context.Context, msg IncomingMessage) (string, error)
}

type IncomingMessage struct {
	ThreadTS  string // session ID — empty if this is the root message
	ChannelID string
	UserID    string
	Text      string
	IsDM      bool
}

func NewHandler(botToken, appToken string, planner Planner, log *slog.Logger) (*Handler, error) {
	api := slack.New(
		botToken,
		slack.OptionAppLevelToken(appToken),
	)

	socket := socketmode.New(
		api,
		socketmode.OptionLog(slog.NewLogLogger(log.Handler(), slog.LevelDebug)),
	)

	// Resolve the bot's own user ID so we can strip mentions from message text.
	authResp, err := api.AuthTest()
	if err != nil {
		return nil, err
	}

	return &Handler{
		client:  api,
		socket:  socket,
		botID:   authResp.UserID,
		planner: planner,
		log:     log,
	}, nil
}

func (h *Handler) Run(ctx context.Context) error {
	go func() {
		<-ctx.Done()
	}()

	for evt := range h.socket.Events {
		switch evt.Type {
		case socketmode.EventTypeEventsAPI:
			h.socket.Ack(*evt.Request)
			h.handleEventsAPI(ctx, evt)
		case socketmode.EventTypeConnecting:
			h.log.Info("Connecting to slack")
		case socketmode.EventTypeConnected:
			h.log.Info("Connected to slack")
		case socketmode.EventTypeConnectionError:
			h.log.Error("Slack connection error")
		}
	}
	return nil
}

func (h *Handler) handleEventsAPI(ctx context.Context, evt socketmode.Event) {
	payload, ok := evt.Data.(slackevents.EventsAPIEvent)
	if !ok {
		return
	}

	switch payload.Type {
	case slackevents.CallbackEvent:
		h.handleCallback(ctx, payload.InnerEvent)
	}
}

func (h *Handler) handleCallback(ctx context.Context, inner slackevents.EventsAPIInnerEvent) {
	switch ev := inner.Data.(type) {
	case *slackevents.AppMentionEvent:
		h.dispatch(ctx, IncomingMessage{
			ThreadTS:  threadTS(ev.ThreadTimeStamp, ev.TimeStamp),
			ChannelID: ev.Channel,
			UserID:    ev.User,
			Text:      h.stripMention(ev.Text),
			IsDM:      false,
		})

	case *slackevents.MessageEvent:
		// Ignore bot messages to avoid feedback loops.
		if ev.BotID != "" || ev.SubType == "bot_message" {
			return
		}
		h.dispatch(ctx, IncomingMessage{
			ThreadTS:  threadTS(ev.ThreadTimeStamp, ev.TimeStamp),
			ChannelID: ev.Channel,
			UserID:    ev.User,
			Text:      ev.Text,
			IsDM:      true,
		})
	}
}

func (h *Handler) dispatch(ctx context.Context, msg IncomingMessage) {
	h.log.Info("incoming message",
		"channel", msg.ChannelID,
		"thread", msg.ThreadTS,
		"user", msg.UserID,
		"dm", msg.IsDM,
	)

	reply, err := h.planner.Handle(ctx, msg)
	if err != nil {
		h.log.Error("planner error", "err", err)
		reply = "Sorry, something went wrong. Please try again."
	}

	h.postReply(msg.ChannelID, msg.ThreadTS, reply)
}

func (h *Handler) postReply(channelID, threadTS, text string) {
	_, _, err := h.client.PostMessage(
		channelID,
		slack.MsgOptionText(text, false),
		slack.MsgOptionTS(threadTS), // reply in thread
	)
	if err != nil {
		h.log.Error("failed to post message", "err", err)
	}
}

func (h *Handler) stripMention(text string) string {
	mention := "<@" + h.botID + ">"
	return strings.TrimSpace(strings.TrimPrefix(text, mention))
}

func threadTS(threadTS, msgTS string) string {
	if threadTS != "" {
		return threadTS
	}
	return msgTS
}
