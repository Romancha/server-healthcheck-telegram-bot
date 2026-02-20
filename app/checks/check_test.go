package checks

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFormatTimeAgo(t *testing.T) {
	tests := []struct {
		name string
		time time.Time
		want string
	}{
		{
			name: "zero time returns never",
			time: time.Time{},
			want: "never",
		},
		{
			name: "30 seconds ago",
			time: time.Now().Add(-30 * time.Second),
			want: "30 seconds ago",
		},
		{
			name: "5 minutes ago",
			time: time.Now().Add(-5 * time.Minute),
			want: "5 minutes ago",
		},
		{
			name: "3 hours ago",
			time: time.Now().Add(-3 * time.Hour),
			want: "3 hours ago",
		},
		{
			name: "2 days ago",
			time: time.Now().Add(-48 * time.Hour),
			want: "2 days ago",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatTimeAgo(tt.time)
			if got != tt.want {
				t.Errorf("FormatTimeAgo() = %q, want %q", got, tt.want)
			}
		})
	}
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
	if result.ResponseTime <= 0 {
		t.Errorf("expected ResponseTime > 0, got %d", result.ResponseTime)
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

func TestCheckServerStatus_Redirect(t *testing.T) {
	// Disable redirect following for this test
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMovedPermanently)
	}))
	defer server.Close()

	// Default httpClient follows redirects, so 301 will result in an error or a final response.
	// Let's test with a non-2xx that won't redirect
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server2.Close()

	result := checkServerStatus(ServerCheck{
		Name: "test",
		Url:  server2.URL,
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
		time.Sleep(3 * time.Second)
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
