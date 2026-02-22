package checks

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// testBotMessages captures messages sent via the mock Telegram bot
type testBotMessages struct {
	mu   sync.Mutex
	msgs []string
}

func (m *testBotMessages) add(text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.msgs = append(m.msgs, text)
}

func (m *testBotMessages) all() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string{}, m.msgs...)
}

func (m *testBotMessages) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.msgs)
}

// newTestBot creates a mock Telegram bot backed by httptest server.
// Returns the bot instance and a testBotMessages that captures all sent messages.
func newTestBot(t *testing.T) (*tgbotapi.BotAPI, *testBotMessages) {
	t.Helper()
	sent := &testBotMessages{}

	tgServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if strings.Contains(r.URL.Path, "getMe") {
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

		// Capture sendMessage text
		if strings.Contains(r.URL.Path, "sendMessage") {
			r.ParseForm()
			text := r.FormValue("text")
			if text != "" {
				sent.add(text)
			}
		}

		// Default response for all API calls
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok": true,
			"result": map[string]interface{}{
				"message_id": 1,
				"chat":       map[string]interface{}{"id": 123, "type": "private"},
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

func TestFormatTimeAgo(t *testing.T) {
	t.Run("zero time returns never", func(t *testing.T) {
		got := FormatTimeAgo(time.Time{})
		if got != "never" {
			t.Errorf("FormatTimeAgo() = %q, want %q", got, "never")
		}
	})

	t.Run("seconds ago", func(t *testing.T) {
		got := FormatTimeAgo(time.Now().Add(-30 * time.Second))
		if !strings.HasSuffix(got, "seconds ago") {
			t.Errorf("FormatTimeAgo() = %q, want '* seconds ago'", got)
		}
	})

	t.Run("minutes ago", func(t *testing.T) {
		got := FormatTimeAgo(time.Now().Add(-5 * time.Minute))
		if !strings.HasSuffix(got, "minutes ago") {
			t.Errorf("FormatTimeAgo() = %q, want '* minutes ago'", got)
		}
	})

	t.Run("hours ago", func(t *testing.T) {
		got := FormatTimeAgo(time.Now().Add(-3 * time.Hour))
		if !strings.HasSuffix(got, "hours ago") {
			t.Errorf("FormatTimeAgo() = %q, want '* hours ago'", got)
		}
	})

	t.Run("days ago", func(t *testing.T) {
		got := FormatTimeAgo(time.Now().Add(-48 * time.Hour))
		if !strings.HasSuffix(got, "days ago") {
			t.Errorf("FormatTimeAgo() = %q, want '* days ago'", got)
		}
	})
}

func TestShouldSendSSLNotification(t *testing.T) {
	tests := []struct {
		name             string
		lastNotification time.Time
		want             bool
	}{
		{
			name:             "zero time (never notified) returns true",
			lastNotification: time.Time{},
			want:             true,
		},
		{
			name:             "notified 1 hour ago returns false",
			lastNotification: time.Now().Add(-1 * time.Hour),
			want:             false,
		},
		{
			name:             "notified 23 hours ago returns false",
			lastNotification: time.Now().Add(-23 * time.Hour),
			want:             false,
		},
		{
			name:             "notified 25 hours ago returns true",
			lastNotification: time.Now().Add(-25 * time.Hour),
			want:             true,
		},
		{
			name:             "notified 48 hours ago returns true",
			lastNotification: time.Now().Add(-48 * time.Hour),
			want:             true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldSendSSLNotification(tt.lastNotification)
			if got != tt.want {
				t.Errorf("shouldSendSSLNotification() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckServerStatus_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "Hello World")
	}))
	defer server.Close()

	result := checkServerStatus(ServerCheck{
		Name: "test",
		Url:  server.URL,
	})

	if !result.IsOk {
		t.Errorf("expected IsOk=true, got false. Error: %s", result.ErrorMessage)
	}
	if result.ResponseTime < 0 {
		t.Errorf("expected ResponseTime >= 0, got %d", result.ResponseTime)
	}
	if result.StatusCode != 200 {
		t.Errorf("expected StatusCode=200, got %d", result.StatusCode)
	}
}

func TestCheckServerStatus_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	result := checkServerStatus(ServerCheck{
		Name: "test",
		Url:  server.URL,
	})

	if result.IsOk {
		t.Error("expected IsOk=false for 500 response")
	}
	if result.StatusCode != 500 {
		t.Errorf("expected StatusCode=500, got %d", result.StatusCode)
	}
	if result.ErrorMessage == "" {
		t.Error("expected non-empty ErrorMessage for 500 response")
	}
}

func TestCheckServerStatus_Forbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	result := checkServerStatus(ServerCheck{
		Name: "test",
		Url:  server.URL,
	})

	if result.IsOk {
		t.Error("expected IsOk=false for 403 response")
	}
	if result.StatusCode != 403 {
		t.Errorf("expected StatusCode=403, got %d", result.StatusCode)
	}
}

func TestCheckServerStatus_ContentMatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "Service is healthy and running")
	}))
	defer server.Close()

	result := checkServerStatus(ServerCheck{
		Name:            "test",
		Url:             server.URL,
		ExpectedContent: "healthy",
	})

	if !result.IsOk {
		t.Errorf("expected IsOk=true when content matches, got false. Error: %s", result.ErrorMessage)
	}
	if !result.ContentMatched {
		t.Error("expected ContentMatched=true")
	}
}

func TestCheckServerStatus_ContentMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "Service is running")
	}))
	defer server.Close()

	result := checkServerStatus(ServerCheck{
		Name:            "test",
		Url:             server.URL,
		ExpectedContent: "healthy",
	})

	if result.IsOk {
		t.Error("expected IsOk=false when content does not match")
	}
	if result.ContentMatched {
		t.Error("expected ContentMatched=false")
	}
	if result.ErrorMessage == "" {
		t.Error("expected non-empty ErrorMessage for content mismatch")
	}
}

func TestCheckServerStatus_InvalidURL(t *testing.T) {
	result := checkServerStatus(ServerCheck{
		Name: "test",
		Url:  "http://invalid.server.that.does.not.exist.example:9999",
	})

	if result.IsOk {
		t.Error("expected IsOk=false for unreachable server")
	}
	if result.ErrorMessage == "" {
		t.Error("expected non-empty ErrorMessage for unreachable server")
	}
}

func TestCheckServerStatus_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Save and restore original timeout
	origTimeout := httpClient.Timeout
	ConfigureHttpClient(500 * time.Millisecond)
	defer ConfigureHttpClient(origTimeout)

	result := checkServerStatus(ServerCheck{
		Name: "test",
		Url:  server.URL,
	})

	if result.IsOk {
		t.Error("expected IsOk=false for timed out request")
	}
	if result.ErrorMessage == "" {
		t.Error("expected non-empty ErrorMessage for timeout")
	}
}

func TestConfigureHttpClient(t *testing.T) {
	origTimeout := httpClient.Timeout
	defer ConfigureHttpClient(origTimeout)

	ConfigureHttpClient(15 * time.Second)

	if httpClient.Timeout != 15*time.Second {
		t.Errorf("expected timeout 15s, got %v", httpClient.Timeout)
	}

	transport, ok := httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected Transport to be *http.Transport")
	}
	if transport.TLSHandshakeTimeout != 7500*time.Millisecond {
		t.Errorf("expected TLS timeout 7.5s, got %v", transport.TLSHandshakeTimeout)
	}
}

func TestSetGlobalSSLExpiryThreshold(t *testing.T) {
	orig := globalSSLExpiryThreshold
	defer func() { globalSSLExpiryThreshold = orig }()

	SetGlobalSSLExpiryThreshold(60)
	if globalSSLExpiryThreshold != 60 {
		t.Errorf("expected globalSSLExpiryThreshold=60, got %d", globalSSLExpiryThreshold)
	}
}

// setupPerformCheckTest sets up storage, resets global state, and returns cleanup function.
func setupPerformCheckTest(t *testing.T) func() {
	t.Helper()
	cleanup := setupTestStorage(t)
	InitStorage()
	ResetState()
	return func() {
		ResetState()
		cleanup()
	}
}

func TestPerformCheck_ServerUp_NoAlert(t *testing.T) {
	cleanup := setupPerformCheckTest(t)
	defer cleanup()

	// Create a healthy target server
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	// Seed storage with one server
	data := Data{
		HealthChecks: map[string]ServerCheck{
			"healthy": {Name: "healthy", Url: target.URL, IsOk: false},
		},
	}
	if err := SaveChecksData(data); err != nil {
		t.Fatalf("SaveChecksData: %v", err)
	}

	bot, sent := newTestBot(t)
	PerformCheck(bot, 123, 3)

	// No alert should be sent for a healthy server with no prior failure notification
	if sent.count() != 0 {
		t.Errorf("expected 0 messages for healthy server, got %d: %v", sent.count(), sent.all())
	}

	// Verify availability was updated
	got := ReadChecksData()
	srv := got.HealthChecks["healthy"]
	if !srv.IsOk {
		t.Error("expected server to be marked IsOk=true")
	}
	if srv.TotalChecks != 1 {
		t.Errorf("expected TotalChecks=1, got %d", srv.TotalChecks)
	}
	if srv.SuccessfulChecks != 1 {
		t.Errorf("expected SuccessfulChecks=1, got %d", srv.SuccessfulChecks)
	}
	if srv.Availability != 100 {
		t.Errorf("expected Availability=100, got %f", srv.Availability)
	}
}

func TestPerformCheck_ServerDown_BelowThreshold_NoAlert(t *testing.T) {
	cleanup := setupPerformCheckTest(t)
	defer cleanup()

	// Create a failing target server
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer target.Close()

	data := Data{
		HealthChecks: map[string]ServerCheck{
			"failing": {Name: "failing", Url: target.URL, IsOk: true},
		},
	}
	if err := SaveChecksData(data); err != nil {
		t.Fatalf("SaveChecksData: %v", err)
	}

	bot, sent := newTestBot(t)
	alertThreshold := 3

	// Run PerformCheck twice — below the threshold of 3
	PerformCheck(bot, 123, alertThreshold)
	PerformCheck(bot, 123, alertThreshold)

	// No "down" alert yet — only 2 failures, threshold is 3
	for _, msg := range sent.all() {
		if strings.Contains(msg, "is down") {
			t.Errorf("unexpected 'down' alert before reaching threshold: %s", msg)
		}
	}
}

func TestPerformCheck_ServerDown_ReachesThreshold_SendsAlert(t *testing.T) {
	cleanup := setupPerformCheckTest(t)
	defer cleanup()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer target.Close()

	data := Data{
		HealthChecks: map[string]ServerCheck{
			"failing": {Name: "failing", Url: target.URL, IsOk: true},
		},
	}
	if err := SaveChecksData(data); err != nil {
		t.Fatalf("SaveChecksData: %v", err)
	}

	bot, sent := newTestBot(t)
	alertThreshold := 3

	// Run PerformCheck 3 times to reach the threshold
	for i := 0; i < alertThreshold; i++ {
		PerformCheck(bot, 123, alertThreshold)
	}

	// Exactly one "down" alert should have been sent
	downAlerts := 0
	for _, msg := range sent.all() {
		if strings.Contains(msg, "is down") {
			downAlerts++
		}
	}
	if downAlerts != 1 {
		t.Errorf("expected 1 'down' alert, got %d. Messages: %v", downAlerts, sent.all())
	}

	// Verify availability: 0 successful out of 3 total
	got := ReadChecksData()
	srv := got.HealthChecks["failing"]
	if srv.TotalChecks != 3 {
		t.Errorf("expected TotalChecks=3, got %d", srv.TotalChecks)
	}
	if srv.SuccessfulChecks != 0 {
		t.Errorf("expected SuccessfulChecks=0, got %d", srv.SuccessfulChecks)
	}
	if srv.Availability != 0 {
		t.Errorf("expected Availability=0, got %f", srv.Availability)
	}
}

func TestPerformCheck_ServerRecovers_SendsRecoveryMessage(t *testing.T) {
	cleanup := setupPerformCheckTest(t)
	defer cleanup()

	// Start with a failing server
	failing := true
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failing {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer target.Close()

	data := Data{
		HealthChecks: map[string]ServerCheck{
			"flaky": {Name: "flaky", Url: target.URL, IsOk: true},
		},
	}
	if err := SaveChecksData(data); err != nil {
		t.Fatalf("SaveChecksData: %v", err)
	}

	bot, sent := newTestBot(t)
	alertThreshold := 2

	// Fail enough to trigger alert
	for i := 0; i < alertThreshold; i++ {
		PerformCheck(bot, 123, alertThreshold)
	}

	// Now recover
	failing = false
	PerformCheck(bot, 123, alertThreshold)

	// Should have: 1 "down" alert + 1 "is up" recovery
	var downCount, upCount int
	for _, msg := range sent.all() {
		if strings.Contains(msg, "is down") {
			downCount++
		}
		if strings.Contains(msg, "is up") {
			upCount++
		}
	}
	if downCount != 1 {
		t.Errorf("expected 1 'down' alert, got %d", downCount)
	}
	if upCount != 1 {
		t.Errorf("expected 1 'up' recovery message, got %d", upCount)
	}
}

func TestPerformCheck_ServerRecovers_WithoutPriorAlert_NoRecoveryMessage(t *testing.T) {
	cleanup := setupPerformCheckTest(t)
	defer cleanup()

	// Server fails once then recovers — below threshold, so no alert was sent
	callCount := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer target.Close()

	data := Data{
		HealthChecks: map[string]ServerCheck{
			"blip": {Name: "blip", Url: target.URL, IsOk: true},
		},
	}
	if err := SaveChecksData(data); err != nil {
		t.Fatalf("SaveChecksData: %v", err)
	}

	bot, sent := newTestBot(t)

	// 1 failure, then recovery — threshold is 3, so no alert was ever sent
	PerformCheck(bot, 123, 3)
	PerformCheck(bot, 123, 3)

	// No messages should have been sent (neither down nor up)
	if sent.count() != 0 {
		t.Errorf("expected 0 messages, got %d: %v", sent.count(), sent.all())
	}
}

func TestPerformCheck_SlowResponse_SendsWarning(t *testing.T) {
	cleanup := setupPerformCheckTest(t)
	defer cleanup()

	// Server that responds slowly
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	data := Data{
		HealthChecks: map[string]ServerCheck{
			"slow": {
				Name:                  "slow",
				Url:                   target.URL,
				IsOk:                  true,
				ResponseTimeThreshold: 10, // 10ms threshold — server will exceed it
			},
		},
	}
	if err := SaveChecksData(data); err != nil {
		t.Fatalf("SaveChecksData: %v", err)
	}

	bot, sent := newTestBot(t)
	PerformCheck(bot, 123, 3)

	// Should get a slow response warning
	found := false
	for _, msg := range sent.all() {
		if strings.Contains(msg, "response time is slow") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected slow response warning, got messages: %v", sent.all())
	}
}

func TestPerformCheck_MultipleServers(t *testing.T) {
	cleanup := setupPerformCheckTest(t)
	defer cleanup()

	okTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer okTarget.Close()

	failTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer failTarget.Close()

	data := Data{
		HealthChecks: map[string]ServerCheck{
			"ok-server":   {Name: "ok-server", Url: okTarget.URL, IsOk: true},
			"fail-server": {Name: "fail-server", Url: failTarget.URL, IsOk: true},
		},
	}
	if err := SaveChecksData(data); err != nil {
		t.Fatalf("SaveChecksData: %v", err)
	}

	bot, _ := newTestBot(t)

	// Run a check — both servers should be checked
	PerformCheck(bot, 123, 3)

	got := ReadChecksData()
	okSrv := got.HealthChecks["ok-server"]
	failSrv := got.HealthChecks["fail-server"]

	if !okSrv.IsOk {
		t.Error("expected ok-server IsOk=true")
	}
	if failSrv.IsOk {
		t.Error("expected fail-server IsOk=false")
	}
	if okSrv.TotalChecks != 1 {
		t.Errorf("ok-server TotalChecks: expected 1, got %d", okSrv.TotalChecks)
	}
	if failSrv.TotalChecks != 1 {
		t.Errorf("fail-server TotalChecks: expected 1, got %d", failSrv.TotalChecks)
	}
}
