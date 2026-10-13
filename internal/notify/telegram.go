package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// Telegram delivers messages with the Bot API sendMessage method.
type Telegram struct {
	baseURL string
	token   string
	client  *http.Client
}

// NewTelegram takes the API base URL (https://api.telegram.org in
// production) so tests can point it at a local server.
func NewTelegram(baseURL, token string, client *http.Client) *Telegram {
	return &Telegram{baseURL: baseURL, token: token, client: client}
}

type telegramReply struct {
	OK          bool   `json:"ok"`
	ErrorCode   int    `json:"error_code"`
	Description string `json:"description"`
}

func (t *Telegram) Send(ctx context.Context, m Message) error {
	body, err := json.Marshal(map[string]any{"chat_id": m.TelegramChatID, "text": m.Text})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+"/bot"+t.token+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return errors.New("telegram: build request: invalid base url")
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram: sendMessage: %w", withoutURL(err))
	}
	defer resp.Body.Close()

	var reply telegramReply
	decodeErr := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&reply)
	if resp.StatusCode != http.StatusOK || decodeErr != nil || !reply.OK {
		if reply.Description != "" {
			return fmt.Errorf("telegram: sendMessage: HTTP %d: %s", resp.StatusCode, reply.Description)
		}
		return fmt.Errorf("telegram: sendMessage: HTTP %d", resp.StatusCode)
	}
	return nil
}

// withoutURL drops the request URL from transport errors: it contains the
// bot token, and these errors end up in logs and in the outbox table.
func withoutURL(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}
