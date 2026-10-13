package notify

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type recordingSender struct{ got []Message }

func (r *recordingSender) Send(_ context.Context, m Message) error {
	r.got = append(r.got, m)
	return nil
}

func TestRouterPrefersTelegramWhenTheCustomerHasAChat(t *testing.T) {
	tg, fallback := &recordingSender{}, &recordingSender{}
	r := Router{Telegram: tg, Fallback: fallback}
	ctx := context.Background()
	_ = r.Send(ctx, Message{TelegramChatID: 42})
	_ = r.Send(ctx, Message{})
	if len(tg.got) != 1 || len(fallback.got) != 1 {
		t.Fatalf("telegram %d, fallback %d", len(tg.got), len(fallback.got))
	}
	noBot := Router{Fallback: fallback}
	_ = noBot.Send(ctx, Message{TelegramChatID: 42})
	if len(fallback.got) != 2 {
		t.Fatal("without a bot the fallback must be used")
	}
}

func TestTelegramSendsMessage(t *testing.T) {
	var gotPath string
	var gotBody struct {
		ChatID int64  `json:"chat_id"`
		Text   string `json:"text"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected request %s %s", r.Method, r.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Error(err)
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7}}`))
	}))
	defer srv.Close()

	tg := NewTelegram(srv.URL, "123:secret", srv.Client())
	if err := tg.Send(context.Background(), Message{TelegramChatID: -100500, Text: "Hi Anna"}); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/bot123:secret/sendMessage" {
		t.Fatalf("path %q", gotPath)
	}
	if gotBody.ChatID != -100500 || gotBody.Text != "Hi Anna" {
		t.Fatalf("body %+v", gotBody)
	}
}

func TestTelegramReportsAPIErrors(t *testing.T) {
	cases := map[string]struct {
		status int
		body   string
		want   string
	}{
		"rejected":     {http.StatusBadRequest, `{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`, "chat not found"},
		"rate limited": {http.StatusTooManyRequests, `{"ok":false,"error_code":429,"description":"Too Many Requests: retry after 5"}`, "429"},
		"not ok":       {http.StatusOK, `{"ok":false,"description":"odd"}`, "odd"},
		"server error": {http.StatusBadGateway, `<html>bad gateway</html>`, "502"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(c.status)
				_, _ = w.Write([]byte(c.body))
			}))
			defer srv.Close()
			err := NewTelegram(srv.URL, "123:secret", srv.Client()).Send(context.Background(), Message{TelegramChatID: 1, Text: "x"})
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("got %v, want an error mentioning %q", err, c.want)
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatalf("error leaks the bot token: %v", err)
			}
		})
	}
}

func TestTelegramErrorsDoNotLeakTheToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer srv.Close()
	client := &http.Client{Timeout: 20 * time.Millisecond}
	err := NewTelegram(srv.URL, "123:secret", client).Send(context.Background(), Message{TelegramChatID: 1, Text: "x"})
	if err == nil {
		t.Fatal("expected a timeout")
	}
	if strings.Contains(err.Error(), "secret") {
		t.Fatalf("error leaks the bot token: %v", err)
	}
	var netErr interface{ Timeout() bool }
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("the cause should stay inspectable: %v", err)
	}
}
