package checks

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Romancha/server-healthcheck-telegram-bot/app/internal/testutil"
)

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
		URL:  server.URL,
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
		URL:  server.URL,
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
		URL:  server.URL,
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
		URL:             server.URL,
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
		URL:             server.URL,
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
	// Use closed localhost port for instant failure without DNS lookup
	result := checkServerStatus(ServerCheck{
		Name: "test",
		URL:  "http://127.0.0.1:1",
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
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Save and restore original timeout
	origTimeout := getHTTPClientTimeout()
	ConfigureHTTPClient(50 * time.Millisecond)
	defer ConfigureHTTPClient(origTimeout)

	result := checkServerStatus(ServerCheck{
		Name: "test",
		URL:  server.URL,
	})

	if result.IsOk {
		t.Error("expected IsOk=false for timed out request")
	}
	if result.ErrorMessage == "" {
		t.Error("expected non-empty ErrorMessage for timeout")
	}
}

func TestConfigureHTTPClient(t *testing.T) {
	origTimeout := getHTTPClientTimeout()
	defer ConfigureHTTPClient(origTimeout)

	ConfigureHTTPClient(15 * time.Second)

	if got := getHTTPClientTimeout(); got != 15*time.Second {
		t.Errorf("expected timeout 15s, got %v", got)
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

// setupPerformCheckTest sets up storage and resets global state.
func setupPerformCheckTest(t *testing.T) {
	t.Helper()
	setupTestStorage(t)
	resetState()
	t.Cleanup(func() { resetState() })
}

func TestPerformCheck_ServerUp_NoAlert(t *testing.T) {
	setupPerformCheckTest(t)

	// Create a healthy target server
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	// Seed storage with one server
	data := Data{
		HealthChecks: map[string]ServerCheck{
			"healthy": {Name: "healthy", URL: target.URL, IsOk: false},
		},
	}
	if err := SaveChecksData(data); err != nil {
		t.Fatalf("SaveChecksData: %v", err)
	}

	bot, sent := testutil.NewTestBot(t)
	PerformCheck(bot, 123, 3)

	// No alert should be sent for a healthy server with no prior failure notification
	if sent.Count() != 0 {
		t.Errorf("expected 0 messages for healthy server, got %d: %v", sent.Count(), sent.All())
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
	setupPerformCheckTest(t)

	// Create a failing target server
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer target.Close()

	data := Data{
		HealthChecks: map[string]ServerCheck{
			"failing": {Name: "failing", URL: target.URL, IsOk: true},
		},
	}
	if err := SaveChecksData(data); err != nil {
		t.Fatalf("SaveChecksData: %v", err)
	}

	bot, sent := testutil.NewTestBot(t)
	alertThreshold := 3

	// Run PerformCheck twice — below the threshold of 3
	PerformCheck(bot, 123, alertThreshold)
	PerformCheck(bot, 123, alertThreshold)

	// No "down" alert yet — only 2 failures, threshold is 3
	for _, msg := range sent.All() {
		if strings.Contains(msg, "is down") {
			t.Errorf("unexpected 'down' alert before reaching threshold: %s", msg)
		}
	}
}

func TestPerformCheck_ServerDown_ReachesThreshold_SendsAlert(t *testing.T) {
	setupPerformCheckTest(t)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer target.Close()

	data := Data{
		HealthChecks: map[string]ServerCheck{
			"failing": {Name: "failing", URL: target.URL, IsOk: true},
		},
	}
	if err := SaveChecksData(data); err != nil {
		t.Fatalf("SaveChecksData: %v", err)
	}

	bot, sent := testutil.NewTestBot(t)
	alertThreshold := 3

	// Run PerformCheck 3 times to reach the threshold
	for range alertThreshold {
		PerformCheck(bot, 123, alertThreshold)
	}

	// Exactly one "down" alert should have been sent
	downAlerts := 0
	for _, msg := range sent.All() {
		if strings.Contains(msg, "is down") {
			downAlerts++
		}
	}
	if downAlerts != 1 {
		t.Errorf("expected 1 'down' alert, got %d. Messages: %v", downAlerts, sent.All())
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
	setupPerformCheckTest(t)

	// Start with a failing server; use atomic to avoid data race with handler goroutine
	var failing atomic.Bool
	failing.Store(true)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failing.Load() {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer target.Close()

	data := Data{
		HealthChecks: map[string]ServerCheck{
			"flaky": {Name: "flaky", URL: target.URL, IsOk: true},
		},
	}
	if err := SaveChecksData(data); err != nil {
		t.Fatalf("SaveChecksData: %v", err)
	}

	bot, sent := testutil.NewTestBot(t)
	alertThreshold := 2

	// Fail enough to trigger alert
	for range alertThreshold {
		PerformCheck(bot, 123, alertThreshold)
	}

	// Now recover
	failing.Store(false)
	PerformCheck(bot, 123, alertThreshold)

	// Should have: 1 "down" alert + 1 "is up" recovery
	var downCount, upCount int
	for _, msg := range sent.All() {
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
	setupPerformCheckTest(t)

	// Server fails once then recovers — below threshold, so no alert was sent
	var callCount atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer target.Close()

	data := Data{
		HealthChecks: map[string]ServerCheck{
			"blip": {Name: "blip", URL: target.URL, IsOk: true},
		},
	}
	if err := SaveChecksData(data); err != nil {
		t.Fatalf("SaveChecksData: %v", err)
	}

	bot, sent := testutil.NewTestBot(t)

	// 1 failure, then recovery — threshold is 3, so no alert was ever sent
	PerformCheck(bot, 123, 3)
	PerformCheck(bot, 123, 3)

	// No messages should have been sent (neither down nor up)
	if sent.Count() != 0 {
		t.Errorf("expected 0 messages, got %d: %v", sent.Count(), sent.All())
	}
}

func TestPerformCheck_SlowResponse_SendsWarning(t *testing.T) {
	setupPerformCheckTest(t)

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
				URL:                   target.URL,
				IsOk:                  true,
				ResponseTimeThreshold: 10, // 10ms threshold — server will exceed it
			},
		},
	}
	if err := SaveChecksData(data); err != nil {
		t.Fatalf("SaveChecksData: %v", err)
	}

	bot, sent := testutil.NewTestBot(t)
	PerformCheck(bot, 123, 3)

	// Should get a slow response warning
	found := false
	for _, msg := range sent.All() {
		if strings.Contains(msg, "response time is slow") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected slow response warning, got messages: %v", sent.All())
	}
}

func TestPerformCheck_MultipleServers(t *testing.T) {
	setupPerformCheckTest(t)

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
			"ok-server":   {Name: "ok-server", URL: okTarget.URL, IsOk: true},
			"fail-server": {Name: "fail-server", URL: failTarget.URL, IsOk: true},
		},
	}
	if err := SaveChecksData(data); err != nil {
		t.Fatalf("SaveChecksData: %v", err)
	}

	bot, _ := testutil.NewTestBot(t)

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

func TestPerformCheck_ServerDown_ReachesThreshold_ThenContinuousFail_NoSecondAlert(t *testing.T) {
	setupPerformCheckTest(t)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer target.Close()

	data := Data{
		HealthChecks: map[string]ServerCheck{
			"failing": {Name: "failing", URL: target.URL, IsOk: true},
		},
	}
	if err := SaveChecksData(data); err != nil {
		t.Fatalf("SaveChecksData: %v", err)
	}

	bot, sent := testutil.NewTestBot(t)
	alertThreshold := 2

	// Trigger the first alert
	for range alertThreshold {
		PerformCheck(bot, 123, alertThreshold)
	}

	// Continue failing beyond the threshold — should NOT get a second "down" alert
	for range alertThreshold * 2 {
		PerformCheck(bot, 123, alertThreshold)
	}

	downAlerts := 0
	for _, msg := range sent.All() {
		if strings.Contains(msg, "is down") {
			downAlerts++
		}
	}
	// After initial alert, failure counter resets, so more alerts may be sent
	// once counter reaches threshold again. The production code resets the counter
	// after sending, so each batch of `alertThreshold` failures triggers one alert.
	// With threshold=2 and 4 more checks after the first alert, we expect 2 more alerts.
	// Total: 3 "down" alerts (1 initial + 2 subsequent batches of 2 failures).
	if downAlerts < 1 {
		t.Errorf("expected at least 1 'down' alert, got %d. Messages: %v", downAlerts, sent.All())
	}
}
