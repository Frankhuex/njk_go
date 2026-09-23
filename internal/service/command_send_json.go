package service

import (
	"bytes"
	"encoding/json"

	"njk_go/internal/util/utext"
)

func handleSendJSONCommand(groupID string, input string) *OutboundAction {
	segment := bytes.TrimSpace([]byte(utext.UnescapeCQText(input)))
	if len(segment) < 2 || segment[0] != '{' || !json.Valid(segment) {
		return simpleOutbound(groupID, "JSON格式错误：需要一个完整的JSON对象")
	}
	return &OutboundAction{GroupID: groupID, RawJSONSegments: []json.RawMessage{segment}}
}
