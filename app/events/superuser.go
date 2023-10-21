package events

import "strings"

type SuperUser []string

func (s SuperUser) IsSuper(userName string) bool {
	for _, super := range s {
		if strings.EqualFold(userName, super) || strings.EqualFold("/"+userName, super) {
			return true
		}
	}
	return false
}
