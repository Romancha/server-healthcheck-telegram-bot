package events

import "testing"

func TestIsSuper(t *testing.T) {
	tests := []struct {
		name       string
		superUsers SuperUser
		userName   string
		want       bool
	}{
		{
			name:       "exact match",
			superUsers: SuperUser{"admin"},
			userName:   "admin",
			want:       true,
		},
		{
			name:       "case insensitive match",
			superUsers: SuperUser{"Admin"},
			userName:   "admin",
			want:       true,
		},
		{
			name:       "case insensitive match reverse",
			superUsers: SuperUser{"admin"},
			userName:   "ADMIN",
			want:       true,
		},
		{
			name:       "match with slash prefix in superuser list",
			superUsers: SuperUser{"/admin"},
			userName:   "admin",
			want:       true,
		},
		{
			name:       "unknown user",
			superUsers: SuperUser{"admin"},
			userName:   "unknown",
			want:       false,
		},
		{
			name:       "empty superuser list",
			superUsers: SuperUser{},
			userName:   "admin",
			want:       false,
		},
		{
			name:       "empty username",
			superUsers: SuperUser{"admin"},
			userName:   "",
			want:       false,
		},
		{
			name:       "multiple superusers first match",
			superUsers: SuperUser{"admin", "moderator", "owner"},
			userName:   "admin",
			want:       true,
		},
		{
			name:       "multiple superusers last match",
			superUsers: SuperUser{"admin", "moderator", "owner"},
			userName:   "owner",
			want:       true,
		},
		{
			name:       "multiple superusers no match",
			superUsers: SuperUser{"admin", "moderator", "owner"},
			userName:   "hacker",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.superUsers.IsSuper(tt.userName)
			if got != tt.want {
				t.Errorf("IsSuper(%q) = %v, want %v", tt.userName, got, tt.want)
			}
		})
	}
}
