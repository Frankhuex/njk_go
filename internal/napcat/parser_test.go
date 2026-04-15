package napcat

import "testing"

func TestParseInboundMessageGroupMessage(t *testing.T) {
	raw := []byte(`{
		"time":1776271198,
		"self_id":1558109748,
		"post_type":"message",
		"message_type":"group",
		"sub_type":"normal",
		"message_id":123,
		"user_id":456,
		"group_id":789,
		"group_name":"test-group",
		"raw_message":"hello",
		"sender":{"user_id":456,"nickname":"alice"},
		"message":[{"type":"text","data":{"text":"hello"}}]
	}`)

	parsed, err := ParseInboundMessage(raw)
	if err != nil {
		t.Fatalf("ParseInboundMessage returned error: %v", err)
	}

	if parsed.Kind != EventKindGroupMessage {
		t.Fatalf("unexpected kind: %s", parsed.Kind)
	}
	if parsed.GroupMessage == nil {
		t.Fatal("group message should not be nil")
	}
	if parsed.GroupMessage.GroupID != "789" {
		t.Fatalf("unexpected group id: %s", parsed.GroupMessage.GroupID)
	}
	if len(parsed.GroupMessage.Message.Segments) != 1 {
		t.Fatalf("unexpected segment count: %d", len(parsed.GroupMessage.Message.Segments))
	}
}

func TestParseInboundMessageNotice(t *testing.T) {
	raw := []byte(`{
		"time":1776271198,
		"self_id":1558109748,
		"post_type":"notice",
		"notice_type":"notify",
		"sub_type":"poke",
		"group_id":789,
		"target_id":1558109748
	}`)

	parsed, err := ParseInboundMessage(raw)
	if err != nil {
		t.Fatalf("ParseInboundMessage returned error: %v", err)
	}

	if parsed.Kind != EventKindNotice {
		t.Fatalf("unexpected kind: %s", parsed.Kind)
	}
	if parsed.Notice == nil {
		t.Fatal("notice should not be nil")
	}
	if parsed.Notice.TargetID != "1558109748" {
		t.Fatalf("unexpected target id: %s", parsed.Notice.TargetID)
	}
}

func TestParseInboundMessageActionResponse(t *testing.T) {
	raw := []byte(`{
		"status":"ok",
		"retcode":0,
		"data":{"message_id":123456},
		"message":"",
		"wording":"",
		"stream":"normal-action"
	}`)

	parsed, err := ParseInboundMessage(raw)
	if err != nil {
		t.Fatalf("ParseInboundMessage returned error: %v", err)
	}

	if parsed.Kind != EventKindActionResponse {
		t.Fatalf("unexpected kind: %s", parsed.Kind)
	}
	if parsed.Action == nil {
		t.Fatal("action should not be nil")
	}
	if !parsed.Action.IsSuccess() {
		t.Fatal("action should be success")
	}
	messageID, ok := parsed.Action.MessageID()
	if !ok {
		t.Fatal("message id should exist")
	}
	if messageID != "123456" {
		t.Fatalf("unexpected message id: %s", messageID)
	}
}

func TestParseInboundMessageHeartbeatMetaEvent(t *testing.T) {
	raw := []byte(`{
		"time":1776271199,
		"self_id":1558109748,
		"post_type":"meta_event",
		"meta_event_type":"heartbeat",
		"status":{"online":true,"good":true},
		"interval":3000
	}`)

	parsed, err := ParseInboundMessage(raw)
	if err != nil {
		t.Fatalf("ParseInboundMessage returned error: %v", err)
	}

	if parsed.Kind != EventKindUnknown {
		t.Fatalf("unexpected kind: %s", parsed.Kind)
	}
	if parsed.Envelope.MetaEventType != "heartbeat" {
		t.Fatalf("unexpected meta event type: %s", parsed.Envelope.MetaEventType)
	}
}
