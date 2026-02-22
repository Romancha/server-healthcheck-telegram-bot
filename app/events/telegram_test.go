package events

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Romancha/server-healthcheck-telegram-bot/app/checks"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// testBotMessages captures messages sent via the mock Telegram bot.
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

func (m *testBotMessages) last() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.msgs) == 0 {
		return ""
	}
	return m.msgs[len(m.msgs)-1]
}

// newTestBot creates a mock Telegram bot and returns message capture.
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

		// Capture editMessageText
		if strings.Contains(r.URL.Path, "editMessageText") {
			r.ParseForm()
			text := r.FormValue("text")
			if text != "" {
				sent.add(text)
			}
		}

		// Default OK response with Message shape
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

// setupTestStorage redirects checks storage to a temp dir and initializes it.
func setupTestStorage(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	checks.SetStorageLocation(filepath.Join(tmpDir, "checks.json"))
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

// --- URL helper tests (kept from original) ---

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
			got := getFullServerUrl(tt.input)
			if got != tt.want {
				t.Errorf("getFullServerUrl(%q) = %q, want %q", tt.input, got, tt.want)
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
		wantUrl  string
		wantName string
	}{
		{
			name:     "url and name",
			message:  makeMessage("/add example.com myserver"),
			wantUrl:  "https://example.com",
			wantName: "myserver",
		},
		{
			name:     "url only uses url as name",
			message:  makeMessage("/add example.com"),
			wantUrl:  "https://example.com",
			wantName: "example.com",
		},
		{
			name:     "no arguments",
			message:  makeMessage("/add"),
			wantUrl:  "",
			wantName: "",
		},
		{
			name:     "url with https",
			message:  makeMessage("/add https://example.com myserver"),
			wantUrl:  "https://example.com",
			wantName: "myserver",
		},
		{
			name:     "url with http",
			message:  makeMessage("/add http://example.com myserver"),
			wantUrl:  "http://example.com",
			wantName: "myserver",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getServer(tt.message)
			if got.Url != tt.wantUrl {
				t.Errorf("getServer().Url = %q, want %q", got.Url, tt.wantUrl)
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
	bot, sent := newTestBot(t)
	superUsers := SuperUser{"admin"}

	update := makeCommandUpdate("add", "example.com myserver", "hacker")
	processUpdate(bot, update, superUsers)

	// Non-superuser should be ignored — no messages sent, no data saved
	if sent.count() != 0 {
		t.Errorf("expected 0 messages for non-superuser, got %d: %v", sent.count(), sent.all())
	}
	data := checks.ReadChecksData()
	if len(data.HealthChecks) != 0 {
		t.Errorf("expected 0 servers, non-superuser should not be able to add")
	}
}

func TestProcessUpdate_AddServer(t *testing.T) {
	setupTestStorage(t)
	bot, sent := newTestBot(t)
	superUsers := SuperUser{"admin"}

	update := makeCommandUpdate("add", "example.com myserver", "admin")
	processUpdate(bot, update, superUsers)

	// Server should be saved in storage
	data := checks.ReadChecksData()
	srv, ok := data.HealthChecks["myserver"]
	if !ok {
		t.Fatal("expected server 'myserver' to be added to storage")
	}
	if srv.Url != "https://example.com" {
		t.Errorf("expected Url='https://example.com', got %q", srv.Url)
	}

	// Confirmation message should be sent
	if sent.count() == 0 {
		t.Fatal("expected confirmation message")
	}
	if !strings.Contains(sent.last(), "added") {
		t.Errorf("expected 'added' in confirmation, got %q", sent.last())
	}
}

func TestProcessUpdate_AddServer_NoArgs(t *testing.T) {
	setupTestStorage(t)
	bot, sent := newTestBot(t)
	superUsers := SuperUser{"admin"}

	update := makeCommandUpdate("add", "", "admin")
	processUpdate(bot, update, superUsers)

	// Should get usage hint, not add anything
	data := checks.ReadChecksData()
	if len(data.HealthChecks) != 0 {
		t.Error("expected no servers to be added with empty args")
	}
	if sent.count() == 0 {
		t.Fatal("expected usage message")
	}
	if !strings.Contains(sent.last(), "Usage") {
		t.Errorf("expected usage message, got %q", sent.last())
	}
}

func TestProcessUpdate_AddDuplicateServer(t *testing.T) {
	setupTestStorage(t)
	bot, sent := newTestBot(t)
	superUsers := SuperUser{"admin"}

	// Add server first time
	update := makeCommandUpdate("add", "example.com myserver", "admin")
	processUpdate(bot, update, superUsers)

	// Try to add same server again
	processUpdate(bot, update, superUsers)

	// Should get "already exists" message
	lastMsg := sent.last()
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
			"myserver": {Name: "myserver", Url: "https://example.com"},
		},
	}
	if err := checks.SaveChecksData(data); err != nil {
		t.Fatalf("SaveChecksData: %v", err)
	}

	bot, sent := newTestBot(t)
	superUsers := SuperUser{"admin"}

	update := makeCommandUpdate("remove", "myserver", "admin")
	processUpdate(bot, update, superUsers)

	// Server should be removed
	got := checks.ReadChecksData()
	if _, ok := got.HealthChecks["myserver"]; ok {
		t.Error("expected server 'myserver' to be removed")
	}

	// Confirmation message
	if !strings.Contains(sent.last(), "removed") {
		t.Errorf("expected 'removed' in confirmation, got %q", sent.last())
	}
}

func TestProcessUpdate_RemoveNonExistentServer(t *testing.T) {
	setupTestStorage(t)
	bot, sent := newTestBot(t)
	superUsers := SuperUser{"admin"}

	update := makeCommandUpdate("remove", "ghost", "admin")
	processUpdate(bot, update, superUsers)

	if !strings.Contains(sent.last(), "not exists") {
		t.Errorf("expected 'not exists' message, got %q", sent.last())
	}
}

func TestProcessUpdate_RemoveAll(t *testing.T) {
	setupTestStorage(t)

	// Pre-seed servers
	data := checks.Data{
		HealthChecks: map[string]checks.ServerCheck{
			"s1": {Name: "s1", Url: "https://one.com"},
			"s2": {Name: "s2", Url: "https://two.com"},
		},
	}
	if err := checks.SaveChecksData(data); err != nil {
		t.Fatalf("SaveChecksData: %v", err)
	}

	bot, sent := newTestBot(t)
	superUsers := SuperUser{"admin"}

	update := makeCommandUpdate("removeall", "", "admin")
	processUpdate(bot, update, superUsers)

	// All servers should be gone
	got := checks.ReadChecksData()
	if len(got.HealthChecks) != 0 {
		t.Errorf("expected 0 servers after removeall, got %d", len(got.HealthChecks))
	}

	if !strings.Contains(sent.last(), "All servers removed") {
		t.Errorf("expected 'All servers removed', got %q", sent.last())
	}
}

func TestProcessUpdate_List_Empty(t *testing.T) {
	setupTestStorage(t)
	bot, sent := newTestBot(t)
	superUsers := SuperUser{"admin"}

	update := makeCommandUpdate("list", "", "admin")
	processUpdate(bot, update, superUsers)

	if !strings.Contains(sent.last(), "No servers") {
		t.Errorf("expected 'No servers' for empty list, got %q", sent.last())
	}
}

func TestProcessUpdate_List_WithServers(t *testing.T) {
	setupTestStorage(t)

	data := checks.Data{
		HealthChecks: map[string]checks.ServerCheck{
			"web": {Name: "web", Url: "https://web.com", IsOk: true},
		},
	}
	if err := checks.SaveChecksData(data); err != nil {
		t.Fatalf("SaveChecksData: %v", err)
	}

	bot, sent := newTestBot(t)
	superUsers := SuperUser{"admin"}

	update := makeCommandUpdate("list", "", "admin")
	processUpdate(bot, update, superUsers)

	// At least one message should contain the server info
	found := false
	for _, msg := range sent.all() {
		if strings.Contains(msg, "web") && strings.Contains(msg, "https://web.com") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected list to contain server info, got: %v", sent.all())
	}
}

func TestProcessUpdate_Stats_Empty(t *testing.T) {
	setupTestStorage(t)
	bot, sent := newTestBot(t)
	superUsers := SuperUser{"admin"}

	update := makeCommandUpdate("stats", "", "admin")
	processUpdate(bot, update, superUsers)

	if !strings.Contains(sent.last(), "No servers") {
		t.Errorf("expected 'No servers' for empty stats, got %q", sent.last())
	}
}

func TestProcessUpdate_Help(t *testing.T) {
	setupTestStorage(t)
	bot, sent := newTestBot(t)
	superUsers := SuperUser{"admin"}

	update := makeCommandUpdate("help", "", "admin")
	processUpdate(bot, update, superUsers)

	msg := sent.last()
	if !strings.Contains(msg, "/add") || !strings.Contains(msg, "/remove") {
		t.Errorf("expected help message with commands, got %q", msg)
	}
}

func TestProcessUpdate_SetResponseTime(t *testing.T) {
	setupTestStorage(t)

	data := checks.Data{
		HealthChecks: map[string]checks.ServerCheck{
			"api": {Name: "api", Url: "https://api.com"},
		},
	}
	if err := checks.SaveChecksData(data); err != nil {
		t.Fatalf("SaveChecksData: %v", err)
	}

	bot, sent := newTestBot(t)
	superUsers := SuperUser{"admin"}

	update := makeCommandUpdate("setresponsetime", "api 500", "admin")
	processUpdate(bot, update, superUsers)

	// Verify storage was updated
	got := checks.ReadChecksData()
	srv := got.HealthChecks["api"]
	if srv.ResponseTimeThreshold != 500 {
		t.Errorf("expected ResponseTimeThreshold=500, got %d", srv.ResponseTimeThreshold)
	}

	if !strings.Contains(sent.last(), "500ms") {
		t.Errorf("expected confirmation with threshold, got %q", sent.last())
	}
}

func TestProcessUpdate_SetContent(t *testing.T) {
	setupTestStorage(t)

	data := checks.Data{
		HealthChecks: map[string]checks.ServerCheck{
			"api": {Name: "api", Url: "https://api.com"},
		},
	}
	if err := checks.SaveChecksData(data); err != nil {
		t.Fatalf("SaveChecksData: %v", err)
	}

	bot, sent := newTestBot(t)
	superUsers := SuperUser{"admin"}

	update := makeCommandUpdate("setcontent", "api healthy ok", "admin")
	processUpdate(bot, update, superUsers)

	got := checks.ReadChecksData()
	srv := got.HealthChecks["api"]
	if srv.ExpectedContent != "healthy ok" {
		t.Errorf("expected ExpectedContent='healthy ok', got %q", srv.ExpectedContent)
	}

	if !strings.Contains(sent.last(), "healthy ok") {
		t.Errorf("expected confirmation with content, got %q", sent.last())
	}
}

func TestProcessUpdate_Details_NotFound(t *testing.T) {
	setupTestStorage(t)
	bot, sent := newTestBot(t)
	superUsers := SuperUser{"admin"}

	update := makeCommandUpdate("details", "ghost", "admin")
	processUpdate(bot, update, superUsers)

	if !strings.Contains(sent.last(), "not found") {
		t.Errorf("expected 'not found' for missing server, got %q", sent.last())
	}
}

func TestProcessUpdate_Details_Found(t *testing.T) {
	setupTestStorage(t)

	data := checks.Data{
		HealthChecks: map[string]checks.ServerCheck{
			"web": {
				Name:             "web",
				Url:              "https://web.com",
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

	bot, sent := newTestBot(t)
	superUsers := SuperUser{"admin"}

	update := makeCommandUpdate("details", "web", "admin")
	processUpdate(bot, update, superUsers)

	msg := sent.last()
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
			"web": {Name: "web", Url: "https://web.com"},
		},
	}
	if err := checks.SaveChecksData(data); err != nil {
		t.Fatalf("SaveChecksData: %v", err)
	}

	bot, sent := newTestBot(t)
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
	for _, msg := range sent.all() {
		if strings.Contains(msg, "removed") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'removed' in messages, got: %v", sent.all())
	}
}

func TestProcessUpdate_NilMessage_Ignored(t *testing.T) {
	bot, sent := newTestBot(t)
	superUsers := SuperUser{"admin"}

	// Update with no message and no callback — should be ignored
	update := tgbotapi.Update{}
	processUpdate(bot, update, superUsers)

	if sent.count() != 0 {
		t.Errorf("expected 0 messages for nil update, got %d", sent.count())
	}
}

func TestProcessUpdate_SuperUserCaseInsensitive(t *testing.T) {
	setupTestStorage(t)
	bot, sent := newTestBot(t)
	superUsers := SuperUser{"Admin"}

	// Send command as "ADMIN" (different case)
	update := makeCommandUpdate("help", "", "ADMIN")
	processUpdate(bot, update, superUsers)

	if sent.count() == 0 {
		t.Error("expected superuser check to be case-insensitive")
	}
}
