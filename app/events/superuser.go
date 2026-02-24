package events

import "strings"

// SuperUser is a list of Telegram usernames allowed to manage the bot.
type SuperUser []string

// IsSuper checks if the given username is in the superuser list (case-insensitive).
func (s SuperUser) IsSuper(userName string) bool {
	for _, super := range s {
		if strings.EqualFold(userName, super) || strings.EqualFold("/"+userName, super) {
			return true
		}
	}
	return false
}
