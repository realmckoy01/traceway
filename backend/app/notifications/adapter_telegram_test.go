package notifications

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTelegramTextWithinLimitIsUnchanged(t *testing.T) {
	msg := Message{
		Subject: "New error: RuntimeError",
		Body:    "A new error has been detected.",
		URL:     "https://x.example/issues/abc123",
	}
	text := telegramText(msg)
	want := msg.Subject + "\n\n" + msg.Body + "\n\n" + msg.URL
	if text != want {
		t.Errorf("telegramText() = %q, want %q", text, want)
	}
}

func TestTelegramTextTruncatesOversizedBody(t *testing.T) {
	msg := Message{
		Subject: "New error: PgError",
		Body:    "Stack trace:\n" + strings.Repeat("at some.deeply.nested.frame()\n", 500),
		URL:     "https://x.example/issues/abc123",
	}
	text := telegramText(msg)

	if utf8.RuneCountInString(text) > telegramMaxMessageLength {
		t.Errorf("text still exceeds Telegram's limit: %d runes", utf8.RuneCountInString(text))
	}
	if !strings.Contains(text, "[truncated]") {
		t.Error("oversized body should be marked as truncated")
	}
	if !strings.HasSuffix(text, msg.URL) {
		t.Errorf("truncation must never eat the URL: %q", text)
	}
}

func TestTelegramTextTruncationKeepsValidUTF8(t *testing.T) {
	msg := Message{
		Body: strings.Repeat("ж", 5000),
		URL:  "https://x.example/issues/abc123",
	}
	text := telegramText(msg)

	if !utf8.ValidString(text) {
		t.Errorf("truncation split a multi-byte rune: %q", text)
	}
	if utf8.RuneCountInString(text) > telegramMaxMessageLength {
		t.Errorf("text still exceeds Telegram's limit: %d runes", utf8.RuneCountInString(text))
	}
}

func TestTelegramTextWithoutURLStillTruncates(t *testing.T) {
	msg := Message{Body: strings.Repeat("x", 10000)}
	text := telegramText(msg)

	if utf8.RuneCountInString(text) > telegramMaxMessageLength {
		t.Errorf("text still exceeds Telegram's limit: %d runes", utf8.RuneCountInString(text))
	}
	if !strings.Contains(text, "[truncated]") {
		t.Error("oversized body should be marked as truncated")
	}
}

func TestTelegramTopicValidation(t *testing.T) {
	for _, topic := range []string{"", "  ", "123", " 123 ", "0", "-1", "abc", "1.5", "9223372036854775808"} {
		t.Run(topic, func(t *testing.T) {
			adapter := TelegramAdapter{BotToken: "123:secret", ChatId: "-100123", TopicId: topic}
			valid := strings.TrimSpace(topic) == "" || strings.TrimSpace(topic) == "123"
			if err := adapter.Validate(); (err == nil) != valid {
				t.Fatalf("Validate() = %v, valid = %v", err, valid)
			}
		})
	}
}

type telegramTestTransport func(*http.Request) (*http.Response, error)

func (f telegramTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestTelegramSendTopic(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	for _, topic := range []string{"", " 123 "} {
		t.Run(topic, func(t *testing.T) {
			http.DefaultTransport = telegramTestTransport(func(req *http.Request) (*http.Response, error) {
				if err := req.ParseForm(); err != nil {
					t.Fatal(err)
				}
				if got := req.Form.Get("message_thread_id"); got != strings.TrimSpace(topic) {
					t.Errorf("message_thread_id = %q, want %q", got, strings.TrimSpace(topic))
				}
				if _, present := req.Form["message_thread_id"]; present != (topic != "") {
					t.Errorf("message_thread_id presence = %v", present)
				}
				if req.Form.Get("chat_id") != "-100123" || req.Form.Get("text") != "test" {
					t.Errorf("unexpected form: %v", req.Form)
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"ok":true}`)), Header: make(http.Header)}, nil
			})
			adapter := TelegramAdapter{BotToken: "123:secret", ChatId: "-100123", TopicId: topic}
			if err := adapter.Send(context.Background(), Message{Body: "test"}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
