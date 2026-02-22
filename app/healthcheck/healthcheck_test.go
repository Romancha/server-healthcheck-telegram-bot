package healthcheck

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// newTestBot creates a mock Telegram bot. If failAfterInit is true, getMe succeeds
// during bot creation but fails on subsequent calls (simulating Telegram going down).
func newTestBot(t *testing.T, failAfterInit bool) *tgbotapi.BotAPI {
	t.Helper()

	var getMeCalls atomic.Int32
	tgServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if strings.Contains(r.URL.Path, "getMe") {
			n := getMeCalls.Add(1)
			// First call is during bot init — always succeed.
			// Subsequent calls fail if failAfterInit is true.
			if failAfterInit && n > 1 {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"ok":          false,
					"description": "Unauthorized",
					"error_code":  401,
				})
				return
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"result": map[string]interface{}{
					"id":         123,
					"is_bot":     true,
					"first_name": "TestBot",
					"username":   "test_bot",
				},
			})
			return
		}

		// Default ok
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok": true,
			"result": map[string]interface{}{
				"id":         123,
				"is_bot":     true,
				"first_name": "TestBot",
				"username":   "test_bot",
			},
		})
	}))
	t.Cleanup(tgServer.Close)

	bot, err := tgbotapi.NewBotAPIWithAPIEndpoint("test-token", tgServer.URL+"/bot%s/%s")
	if err != nil {
		t.Fatalf("failed to create test bot: %v", err)
	}
	return bot
}

func TestHealthEndpoint_OK(t *testing.T) {
	bot := newTestBot(t, false)
	handler := newHealthHandler(bot)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", resp.Status)
	}
	if resp.Telegram != "" {
		t.Errorf("expected empty telegram field, got %q", resp.Telegram)
	}

	// Check Content-Type header
	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %q", ct)
	}
}

func TestHealthEndpoint_TelegramUnavailable(t *testing.T) {
	bot := newTestBot(t, true)
	handler := newHealthHandler(bot)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", rec.Code)
	}

	var resp response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Status != "error" {
		t.Errorf("expected status 'error', got %q", resp.Status)
	}
	if resp.Telegram == "" {
		t.Error("expected non-empty telegram error message")
	}
}

func TestHealthEndpoint_WrongPath_404(t *testing.T) {
	bot := newTestBot(t, false)
	handler := newHealthHandler(bot)

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404 for unknown path, got %d", rec.Code)
	}
}
