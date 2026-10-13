package notify

import (
	"context"
	"log/slog"
)

// Message is one notification ready to deliver.
type Message struct {
	NotificationID int64
	BookingID      string
	Kind           Kind
	Phone          string
	Email          string
	TelegramChatID int64
	Text           string
}

type Sender interface {
	Send(ctx context.Context, m Message) error
}

// LogSender writes messages to the log. It is the default channel and
// stands in for SMS or email in the demo.
type LogSender struct {
	Log *slog.Logger
}

func (s LogSender) Send(ctx context.Context, m Message) error {
	s.Log.InfoContext(ctx, "notification delivered to log",
		"notification_id", m.NotificationID, "booking_id", m.BookingID, "kind", m.Kind, "text", m.Text)
	return nil
}

// Router sends through Telegram when the customer gave a chat id and a bot
// is configured, and through Fallback otherwise.
type Router struct {
	Telegram Sender
	Fallback Sender
}

func (r Router) Send(ctx context.Context, m Message) error {
	if m.TelegramChatID != 0 && r.Telegram != nil {
		return r.Telegram.Send(ctx, m)
	}
	return r.Fallback.Send(ctx, m)
}
