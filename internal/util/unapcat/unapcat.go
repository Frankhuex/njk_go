package unapcat

import (
	"strings"

	"njk_go/internal/napcat"
)

func MentionsUser(message napcat.MessagePayload, userID string) bool {
	for _, segment := range message.Segments {
		if segment.Type == napcat.SegmentTypeAt && strings.TrimSpace(segment.Data.QQ) == userID {
			return true
		}
	}
	return false
}

