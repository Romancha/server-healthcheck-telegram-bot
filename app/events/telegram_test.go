package events

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Romancha/server-healthcheck-telegram-bot/app/checks"
	"github.com/Romancha/server-healthcheck-telegram-bot/app/internal/testutil"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// setupTestStorage redirects checks storage to a temp dir and initializes it.
func setupTestStorage(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	original := checks.SetStorageLocation(filepath.Join(tmpDir, "checks.json"))
	t.Cleanup(func() { checks.SetStorageLocation(original) })
	checks.InitStorage()
}

// makeCommandUpdate creates a tgbotapi.Update simulating a Telegram command message.
func makeCommandUpdate(command, args, username string) tgbotapi.Update {
	text := "/" + command
	if args != "" {
		text += " " + args
	}
	return tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{ID: 123, Type: "private"},
			From: &tgbotapi.User{UserName: username},
			Text: text,
			Entities: []tgbotapi.MessageEntity{
				{
					Type:   "bot_command",
					Offset: 0,
					Length: len("/" + command),
				},
			},
		},
	}
}

// --- URL helper tests ---

func TestGetFullServerUrl(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no protocol adds https",
			input: "example.com",
			want:  "https://example.com",
		},
		{
			name:  "http prefix stays",
			input: "http://example.com",
			want:  "http://example.com",
		},
		{
			name:  "https prefix stays",
			input: "https://example.com",
			want:  "https://example.com",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "url with path",
			input: "example.com/health",
			want:  "https://example.com/health",
		},
		{
			name:  "url with port",
			input: "example.com:8080",
			want:  "https://example.com:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getFullServerURL(tt.input)
			if got != tt.want {
				t.Errorf("getFullServerURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGetServer(t *testing.T) {
	makeMessage := func(text string) *tgbotapi.Message {
		return &tgbotapi.Message{
			Text: text,
			Entities: []tgbotapi.MessageEntity{
				{
					Type:   "bot_command",
					Offset: 0,
					Length: len("/add"),
				},
			},
		}
	}

	tests := []struct {
		name     string
		message  *tgbotapi.Message
		wantURL  string
		wantName string
	}{
		{
			name:     "url and name",
			message:  makeMessage("/add example.com myserver"),
			wantURL:  "https://example.com",
			wantName: "myserver",
		},
		{
			name:     "url only uses url as name",
			message:  makeMessage("/add example.com"),
			wantURL:  "https://example.com",
			wantName: "example.com",
		},
		{
			name:     "no arguments",
			message:  makeMessage("/add"),
			wantURL:  "",
			wantName: "",
		},
		{
			name:     "url with https",
			message:  makeMessage("/add https://example.com myserver"),
			wantURL:  "https://example.com",
			wantName: "myserver",
		},
		{
			name:     "url with http",
			message:  makeMessage("/add http://example.com myserver"),
			wantURL:  "http://example.com",
			wantName: "myserver",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getServer(tt.message)
			if got.URL != tt.wantURL {
				t.Errorf("getServer().Url = %q, want %q", got.URL, tt.wantURL)
			}
			if got.Name != tt.wantName {
				t.Errorf("getServer().Name = %q, want %q", got.Name, tt.wantName)
			}
		})
	}
}

// --- processUpdate command tests ---

func TestProcessUpdate_NonSuperUser_Ignored(t *testing.T) {
	setupTestStorage(t)
	bot, sent := testutil.NewTestBot(t)
	superUsers := SuperUser{"admin"}

	update := makeCommandUpdate("add", "example.com myserver", "hacker")
	processUpdate(bot, update, superUsers)

	// Non-superuser should be ignored — no messages sent, no data saved
	if sent.Count() != 0 {
		t.Errorf("expected 0 messages for non-superuser, got %d: %v", sent.Count(), sent.All())
	}
	data := checks.ReadChecksData()
	if len(data.HealthChecks) != 0 {
		t.Errorf("expected 0 servers, non-superuser should not be able to add")
	}
}

func TestProcessUpdate_AddServer(t *testing.T) {
	setupTestStorage(t)
	bot, sent := testutil.NewTestBot(t)
	superUsers := SuperUser{"admin"}

	update := makeCommandUpdate("add", "example.com myserver", "admin")
	processUpdate(bot, update, superUsers)

	// Server should be saved in storage
	data := checks.ReadChecksData()
	srv, ok := data.HealthChecks["myserver"]
	if !ok {
		t.Fatal("expected server 'myserver' to be added to storage")
	}
	if srv.URL != "https://example.com" {
		t.Errorf("expected Url='https://example.com', got %q", srv.URL)
	}

	// Confirmation message should be sent
	if sent.Count() == 0 {
		t.Fatal("expected confirmation message")
	}
	if !strings.Contains(sent.Last(), "added") {
		t.Errorf("expected 'added' in confirmation, got %q", sent.Last())
	}
}

func TestProcessUpdate_AddServer_NoArgs(t *testing.T) {
	setupTestStorage(t)
	bot, sent := testutil.NewTestBot(t)
	superUsers := SuperUser{"admin"}

	update := makeCommandUpdate("add", "", "admin")
	processUpdate(bot, update, superUsers)

	// Should get usage hint, not add anything
	data := checks.ReadChecksData()
	if len(data.HealthChecks) != 0 {
		t.Error("expected no servers to be added with empty args")
	}
	if sent.Count() == 0 {
		t.Fatal("expected usage message")
	}
	if !strings.Contains(sent.Last(), "Usage") {
		t.Errorf("expected usage message, got %q", sent.Last())
	}
}

func TestProcessUpdate_AddDuplicateServer(t *testing.T) {
	setupTestStorage(t)
	bot, sent := testutil.NewTestBot(t)
	superUsers := SuperUser{"admin"}

	// Add server first time
	update := makeCommandUpdate("add", "example.com myserver", "admin")
	processUpdate(bot, update, superUsers)

	// Try to add same server again
	processUpdate(bot, update, superUsers)

	// Should get "already exists" message
	lastMsg := sent.Last()
	if !strings.Contains(lastMsg, "already exists") {
		t.Errorf("expected 'already exists' message, got %q", lastMsg)
	}

	// Storage should still have exactly 1 server
	data := checks.ReadChecksData()
	if len(data.HealthChecks) != 1 {
		t.Errorf("expected 1 server, got %d", len(data.HealthChecks))
	}
}

func TestProcessUpdate_RemoveServer(t *testing.T) {
	setupTestStorage(t)

	// Pre-seed a server
	data := checks.Data{
		HealthChecks: map[string]checks.ServerCheck{
			"myserver": {Name: "myserver", URL: "https://example.com"},
		},
	}
	if err := checks.SaveChecksData(data); err != nil {
		t.Fatalf("SaveChecksData: %v", err)
	}

	bot, sent := testutil.NewTestBot(t)
	superUsers := SuperUser{"admin"}

	update := makeCommandUpdate("remove", "myserver", "admin")
	processUpdate(bot, update, superUsers)

	// Server should be removed
	got := checks.ReadChecksData()
	if _, ok := got.HealthChecks["myserver"]; ok {
		t.Error("expected server 'myserver' to be removed")
	}

	// Confirmation message
	if !strings.Contains(sent.Last(), "removed") {
		t.Errorf("expected 'removed' in confirmation, got %q", sent.Last())
	}
}

func TestProcessUpdate_RemoveNonExistentServer(t *testing.T) {
	setupTestStorage(t)
	bot, sent := testutil.NewTestBot(t)
	superUsers := SuperUser{"admin"}

	update := makeCommandUpdate("remove", "ghost", "admin")
	processUpdate(bot, update, superUsers)

	if !strings.Contains(sent.Last(), "not exists") {
		t.Errorf("expected 'not exists' message, got %q", sent.Last())
	}
}

func TestProcessUpdate_RemoveAll(t *testing.T) {
	setupTestStorage(t)

	// Pre-seed servers
	data := checks.Data{
		HealthChecks: map[string]checks.ServerCheck{
			"s1": {Name: "s1", URL: "https://one.com"},
			"s2": {Name: "s2", URL: "https://two.com"},
		},
	}
	if err := checks.SaveChecksData(data); err != nil {
		t.Fatalf("SaveChecksData: %v", err)
	}

	bot, sent := testutil.NewTestBot(t)
	superUsers := SuperUser{"admin"}

	update := makeCommandUpdate("removeall", "", "admin")
	processUpdate(bot, update, superUsers)

	// All servers should be gone
	got := checks.ReadChecksData()
	if len(got.HealthChecks) != 0 {
		t.Errorf("expected 0 servers after removeall, got %d", len(got.HealthChecks))
	}

	if !strings.Contains(sent.Last(), "All servers removed") {
		t.Errorf("expected 'All servers removed', got %q", sent.Last())
	}
}

func TestProcessUpdate_List_Empty(t *testing.T) {
	setupTestStorage(t)
	bot, sent := testutil.NewTestBot(t)
	superUsers := SuperUser{"admin"}

	update := makeCommandUpdate("list", "", "admin")
	processUpdate(bot, update, superUsers)

	if !strings.Contains(sent.Last(), "No servers") {
		t.Errorf("expected 'No servers' for empty list, got %q", sent.Last())
	}
}

func TestProcessUpdate_List_WithServers(t *testing.T) {
	setupTestStorage(t)

	data := checks.Data{
		HealthChecks: map[string]checks.ServerCheck{
			"web": {Name: "web", URL: "https://web.com", IsOk: true},
		},
	}
	if err := checks.SaveChecksData(data); err != nil {
		t.Fatalf("SaveChecksData: %v", err)
	}

	bot, sent := testutil.NewTestBot(t)
	superUsers := SuperUser{"admin"}

	update := makeCommandUpdate("list", "", "admin")
	processUpdate(bot, update, superUsers)

	// At least one message should contain the server info
	found := false
	for _, msg := range sent.All() {
		if strings.Contains(msg, "web") && strings.Contains(msg, "https://web.com") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected list to contain server info, got: %v", sent.All())
	}
}

func TestProcessUpdate_Stats_Empty(t *testing.T) {
	setupTestStorage(t)
	bot, sent := testutil.NewTestBot(t)
	superUsers := SuperUser{"admin"}

	update := makeCommandUpdate("stats", "", "admin")
	processUpdate(bot, update, superUsers)

	if !strings.Contains(sent.Last(), "No servers") {
		t.Errorf("expected 'No servers' for empty stats, got %q", sent.Last())
	}
}

func TestProcessUpdate_Stats_WithServers(t *testing.T) {
	setupTestStorage(t)

	data := checks.Data{
		HealthChecks: map[string]checks.ServerCheck{
			"api": {
				Name:             "api",
				URL:              "https://api.com",
				IsOk:             true,
				Availability:     99.5,
				TotalChecks:      200,
				SuccessfulChecks: 199,
				LastResponseTime: 42,
			},
		},
	}
	if err := checks.SaveChecksData(data); err != nil {
		t.Fatalf("SaveChecksData: %v", err)
	}

	bot, sent := testutil.NewTestBot(t)
	superUsers := SuperUser{"admin"}

	update := makeCommandUpdate("stats", "", "admin")
	processUpdate(bot, update, superUsers)

	msg := sent.Last()
	if !strings.Contains(msg, "api") {
		t.Errorf("expected stats to contain server name, got %q", msg)
	}
	if !strings.Contains(msg, "99.5") {
		t.Errorf("expected stats to contain availability, got %q", msg)
	}
	if !strings.Contains(msg, "42ms") {
		t.Errorf("expected stats to contain response time, got %q", msg)
	}
}

func TestProcessUpdate_Help(t *testing.T) {
	setupTestStorage(t)
	bot, sent := testutil.NewTestBot(t)
	superUsers := SuperUser{"admin"}

	update := makeCommandUpdate("help", "", "admin")
	processUpdate(bot, update, superUsers)

	msg := sent.Last()
	if !strings.Contains(msg, "/add") || !strings.Contains(msg, "/remove") {
		t.Errorf("expected help message with commands, got %q", msg)
	}
}

func TestProcessUpdate_SetResponseTime(t *testing.T) {
	setupTestStorage(t)

	data := checks.Data{
		HealthChecks: map[string]checks.ServerCheck{
			"api": {Name: "api", URL: "https://api.com"},
		},
	}
	if err := checks.SaveChecksData(data); err != nil {
		t.Fatalf("SaveChecksData: %v", err)
	}

	bot, sent := testutil.NewTestBot(t)
	superUsers := SuperUser{"admin"}

	update := makeCommandUpdate("setresponsetime", "api 500", "admin")
	processUpdate(bot, update, superUsers)

	// Verify storage was updated
	got := checks.ReadChecksData()
	srv := got.HealthChecks["api"]
	if srv.ResponseTimeThreshold != 500 {
		t.Errorf("expected ResponseTimeThreshold=500, got %d", srv.ResponseTimeThreshold)
	}

	if !strings.Contains(sent.Last(), "500ms") {
		t.Errorf("expected confirmation with threshold, got %q", sent.Last())
	}
}

func TestProcessUpdate_SetResponseTime_ServerNotFound(t *testing.T) {
	setupTestStorage(t)
	bot, sent := testutil.NewTestBot(t)
	superUsers := SuperUser{"admin"}

	update := makeCommandUpdate("setresponsetime", "ghost 500", "admin")
	processUpdate(bot, update, superUsers)

	if !strings.Contains(sent.Last(), "not found") {
		t.Errorf("expected 'not found', got %q", sent.Last())
	}
}

func TestProcessUpdate_SetContent(t *testing.T) {
	setupTestStorage(t)

	data := checks.Data{
		HealthChecks: map[string]checks.ServerCheck{
			"api": {Name: "api", URL: "https://api.com"},
		},
	}
	if err := checks.SaveChecksData(data); err != nil {
		t.Fatalf("SaveChecksData: %v", err)
	}

	bot, sent := testutil.NewTestBot(t)
	superUsers := SuperUser{"admin"}

	update := makeCommandUpdate("setcontent", "api healthy ok", "admin")
	processUpdate(bot, update, superUsers)

	got := checks.ReadChecksData()
	srv := got.HealthChecks["api"]
	if srv.ExpectedContent != "healthy ok" {
		t.Errorf("expected ExpectedContent='healthy ok', got %q", srv.ExpectedContent)
	}

	if !strings.Contains(sent.Last(), "healthy ok") {
		t.Errorf("expected confirmation with content, got %q", sent.Last())
	}
}

func TestProcessUpdate_Details_NotFound(t *testing.T) {
	setupTestStorage(t)
	bot, sent := testutil.NewTestBot(t)
	superUsers := SuperUser{"admin"}

	update := makeCommandUpdate("details", "ghost", "admin")
	processUpdate(bot, update, superUsers)

	if !strings.Contains(sent.Last(), "not found") {
		t.Errorf("expected 'not found' for missing server, got %q", sent.Last())
	}
}

func TestProcessUpdate_Details_Found(t *testing.T) {
	setupTestStorage(t)

	data := checks.Data{
		HealthChecks: map[string]checks.ServerCheck{
			"web": {
				Name:             "web",
				URL:              "https://web.com",
				IsOk:             true,
				TotalChecks:      100,
				SuccessfulChecks: 98,
				Availability:     98.0,
			},
		},
	}
	if err := checks.SaveChecksData(data); err != nil {
		t.Fatalf("SaveChecksData: %v", err)
	}

	bot, sent := testutil.NewTestBot(t)
	superUsers := SuperUser{"admin"}

	update := makeCommandUpdate("details", "web", "admin")
	processUpdate(bot, update, superUsers)

	msg := sent.Last()
	if !strings.Contains(msg, "web") {
		t.Errorf("expected details to contain server name, got %q", msg)
	}
	if !strings.Contains(msg, "https://web.com") {
		t.Errorf("expected details to contain URL, got %q", msg)
	}
	if !strings.Contains(msg, "98.0") {
		t.Errorf("expected details to contain availability, got %q", msg)
	}
}

func TestProcessUpdate_CallbackQuery_Remove(t *testing.T) {
	setupTestStorage(t)

	data := checks.Data{
		HealthChecks: map[string]checks.ServerCheck{
			"web": {Name: "web", URL: "https://web.com"},
		},
	}
	if err := checks.SaveChecksData(data); err != nil {
		t.Fatalf("SaveChecksData: %v", err)
	}

	bot, sent := testutil.NewTestBot(t)
	superUsers := SuperUser{"admin"}

	// Simulate callback query for remove action
	update := tgbotapi.Update{
		CallbackQuery: &tgbotapi.CallbackQuery{
			ID:   "123",
			Data: "remove:web",
			Message: &tgbotapi.Message{
				MessageID: 1,
				Chat:      &tgbotapi.Chat{ID: 123, Type: "private"},
			},
			From: &tgbotapi.User{UserName: "admin"},
		},
	}

	processUpdate(bot, update, superUsers)

	// Server should be removed
	got := checks.ReadChecksData()
	if _, ok := got.HealthChecks["web"]; ok {
		t.Error("expected server 'web' to be removed via callback")
	}

	// Should get edit message with "removed"
	found := false
	for _, msg := range sent.All() {
		if strings.Contains(msg, "removed") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'removed' in messages, got: %v", sent.All())
	}
}

func TestProcessUpdate_CallbackQuery_NonSuperUser_Ignored(t *testing.T) {
	setupTestStorage(t)

	data := checks.Data{
		HealthChecks: map[string]checks.ServerCheck{
			"web": {Name: "web", URL: "https://web.com"},
		},
	}
	if err := checks.SaveChecksData(data); err != nil {
		t.Fatalf("SaveChecksData: %v", err)
	}

	bot, _ := testutil.NewTestBot(t)
	superUsers := SuperUser{"admin"}

	// Non-superuser tries to remove via callback
	update := tgbotapi.Update{
		CallbackQuery: &tgbotapi.CallbackQuery{
			ID:   "456",
			Data: "remove:web",
			Message: &tgbotapi.Message{
				MessageID: 1,
				Chat:      &tgbotapi.Chat{ID: 123, Type: "private"},
			},
			From: &tgbotapi.User{UserName: "hacker"},
		},
	}

	processUpdate(bot, update, superUsers)

	// Server should NOT be removed
	got := checks.ReadChecksData()
	if _, ok := got.HealthChecks["web"]; !ok {
		t.Error("non-superuser should not be able to remove server via callback")
	}
}

func TestProcessUpdate_NilMessage_Ignored(t *testing.T) {
	bot, sent := testutil.NewTestBot(t)
	superUsers := SuperUser{"admin"}

	// Update with no message and no callback — should be ignored
	update := tgbotapi.Update{}
	processUpdate(bot, update, superUsers)

	if sent.Count() != 0 {
		t.Errorf("expected 0 messages for nil update, got %d", sent.Count())
	}
}

func TestProcessUpdate_SuperUserCaseInsensitive(t *testing.T) {
	setupTestStorage(t)
	bot, sent := testutil.NewTestBot(t)
	superUsers := SuperUser{"Admin"}

	// Send command as "ADMIN" (different case)
	update := makeCommandUpdate("help", "", "ADMIN")
	processUpdate(bot, update, superUsers)

	if sent.Count() == 0 {
		t.Error("expected superuser check to be case-insensitive")
	}
}

func TestProcessUpdate_Remove_NoArgs(t *testing.T) {
	setupTestStorage(t)
	bot, sent := testutil.NewTestBot(t)
	superUsers := SuperUser{"admin"}

	update := makeCommandUpdate("remove", "", "admin")
	processUpdate(bot, update, superUsers)

	if sent.Count() == 0 {
		t.Fatal("expected usage message")
	}
	if !strings.Contains(sent.Last(), "Usage") {
		t.Errorf("expected usage message, got %q", sent.Last())
	}
}

func TestProcessUpdate_SetSSLThreshold(t *testing.T) {
	setupTestStorage(t)

	data := checks.Data{
		HealthChecks: map[string]checks.ServerCheck{
			"api": {Name: "api", URL: "https://api.com"},
		},
	}
	if err := checks.SaveChecksData(data); err != nil {
		t.Fatalf("SaveChecksData: %v", err)
	}

	bot, sent := testutil.NewTestBot(t)
	superUsers := SuperUser{"admin"}

	update := makeCommandUpdate("setsslthreshold", "api 14", "admin")
	processUpdate(bot, update, superUsers)

	got := checks.ReadChecksData()
	srv := got.HealthChecks["api"]
	if srv.SSLExpiryThreshold != 14 {
		t.Errorf("expected SSLExpiryThreshold=14, got %d", srv.SSLExpiryThreshold)
	}

	if !strings.Contains(sent.Last(), "14 days") {
		t.Errorf("expected confirmation with threshold, got %q", sent.Last())
	}
}

func TestProcessUpdate_SetSSLThreshold_ServerNotFound(t *testing.T) {
	setupTestStorage(t)
	bot, sent := testutil.NewTestBot(t)
	superUsers := SuperUser{"admin"}

	update := makeCommandUpdate("setsslthreshold", "ghost 14", "admin")
	processUpdate(bot, update, superUsers)

	if !strings.Contains(sent.Last(), "not found") {
		t.Errorf("expected 'not found', got %q", sent.Last())
	}
}

func TestProcessUpdate_SetGlobalSSLThreshold(t *testing.T) {
	setupTestStorage(t)
	bot, sent := testutil.NewTestBot(t)
	superUsers := SuperUser{"admin"}

	update := makeCommandUpdate("setglobalsslthreshold", "60", "admin")
	processUpdate(bot, update, superUsers)

	if !strings.Contains(sent.Last(), "60 days") {
		t.Errorf("expected confirmation with '60 days', got %q", sent.Last())
	}
}

func TestProcessUpdate_SetGlobalSSLThreshold_NoArgs(t *testing.T) {
	setupTestStorage(t)
	bot, sent := testutil.NewTestBot(t)
	superUsers := SuperUser{"admin"}

	update := makeCommandUpdate("setglobalsslthreshold", "", "admin")
	processUpdate(bot, update, superUsers)

	if !strings.Contains(sent.Last(), "Usage") {
		t.Errorf("expected usage message, got %q", sent.Last())
	}
}

func TestProcessUpdate_UnknownCommand_Ignored(t *testing.T) {
	setupTestStorage(t)
	bot, sent := testutil.NewTestBot(t)
	superUsers := SuperUser{"admin"}

	update := makeCommandUpdate("nonexistent", "", "admin")
	processUpdate(bot, update, superUsers)

	if sent.Count() != 0 {
		t.Errorf("expected 0 messages for unknown command, got %d: %v", sent.Count(), sent.All())
	}
}
