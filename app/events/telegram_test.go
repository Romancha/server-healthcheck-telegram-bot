package events

import (
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

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
