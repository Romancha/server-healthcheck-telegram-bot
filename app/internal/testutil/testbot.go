package testutil

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// BotMessages captures messages sent via the mock Telegram bot.
type BotMessages struct {
	mu   sync.Mutex
	msgs []string
}

// Add appends a message to the captured list.
func (m *BotMessages) Add(text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.msgs = append(m.msgs, text)
}

// All returns a copy of all captured messages.
func (m *BotMessages) All() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string{}, m.msgs...)
}

// Count returns the number of captured messages.
func (m *BotMessages) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.msgs)
}

// Last returns the last captured message, or empty string if none.
func (m *BotMessages) Last() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.msgs) == 0 {
		return ""
	}
	return m.msgs[len(m.msgs)-1]
}

// NewTestBot creates a mock Telegram bot backed by httptest server.
// Returns the bot instance and a BotMessages that captures all sent messages
// (sendMessage, editMessageText).
func NewTestBot(t *testing.T) (*tgbotapi.BotAPI, *BotMessages) {
	t.Helper()
	sent := &BotMessages{}

	tgServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if strings.Contains(r.URL.Path, "getMe") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": map[string]any{
					"id":         123,
					"is_bot":     true,
					"first_name": "TestBot",
					"username":   "test_bot",
				},
			})
			return
		}

		// Capture sendMessage text
		if strings.Contains(r.URL.Path, "sendMessage") {
			_ = r.ParseForm()
			text := r.FormValue("text")
			if text != "" {
				sent.Add(text)
			}
		}

		// Capture editMessageText
		if strings.Contains(r.URL.Path, "editMessageText") {
			_ = r.ParseForm()
			text := r.FormValue("text")
			if text != "" {
				sent.Add(text)
			}
		}

		// Default OK response with Message shape
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"result": map[string]any{
				"message_id": 1,
				"chat":       map[string]any{"id": 123, "type": "private"},
				"date":       0,
				"text":       "",
			},
		})
	}))
	t.Cleanup(tgServer.Close)

	bot, err := tgbotapi.NewBotAPIWithAPIEndpoint("test-token", tgServer.URL+"/bot%s/%s")
	if err != nil {
		t.Fatalf("failed to create test bot: %v", err)
	}

	return bot, sent
}
