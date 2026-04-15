package napcat

import "encoding/json"

// EventKind 表示当前原始入站 JSON 在 NapCat 协议中被识别出的事件大类。
type EventKind string

const (
	EventKindUnknown        EventKind = "unknown"
	EventKindGroupMessage   EventKind = "group_message"
	EventKindNotice         EventKind = "notice"
	EventKindActionResponse EventKind = "action_response"
)

// ParsedInbound 表示一条 NapCat 入站 JSON 经 envelope 判断并反序列化后的统一结果。
type ParsedInbound struct {
	Kind         EventKind
	Envelope     InboundEnvelope
	Raw          json.RawMessage
	GroupMessage *GroupMessageEvent
	Notice       *NoticeEvent
	Action       *ActionEnvelope
}

// ActionEnvelope 对应 NapCat action 调用返回时的外层回执结构。
type ActionEnvelope struct {
	Status      string          `json:"status"`
	Retcode     int             `json:"retcode"`
	Data        json.RawMessage `json:"data"`
	MessageText string          `json:"message,omitempty"`
	Wording     string          `json:"wording,omitempty"`
	Stream      string          `json:"stream,omitempty"`
	Echo        json.RawMessage `json:"echo,omitempty"`
}

// ActionMessageIDData 对应 NapCat 常见 action 成功回执里仅包含 message_id 的 data 结构。
type ActionMessageIDData struct {
	MessageID ID `json:"message_id"`
}

func (a ActionEnvelope) IsSuccess() bool {
	return a.Status == "ok" && a.Retcode == 0
}

func (a ActionEnvelope) DecodeData(v any) error {
	if len(a.Data) == 0 || string(a.Data) == "null" {
		return nil
	}
	return json.Unmarshal(a.Data, v)
}

func (a ActionEnvelope) MessageID() (ID, bool) {
	var data ActionMessageIDData
	if err := a.DecodeData(&data); err != nil {
		return "", false
	}
	if data.MessageID == "" {
		return "", false
	}
	return data.MessageID, true
}

func ParseInboundMessage(raw []byte) (*ParsedInbound, error) {
	var envelope InboundEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}

	parsed := &ParsedInbound{
		Kind:     EventKindUnknown,
		Envelope: envelope,
		Raw:      append(json.RawMessage(nil), raw...),
	}

	switch {
	case envelope.IsGroupMessage():
		var event GroupMessageEvent
		if err := json.Unmarshal(raw, &event); err != nil {
			return nil, err
		}
		parsed.Kind = EventKindGroupMessage
		parsed.GroupMessage = &event
	case envelope.IsNotice():
		var event NoticeEvent
		if err := json.Unmarshal(raw, &event); err != nil {
			return nil, err
		}
		parsed.Kind = EventKindNotice
		parsed.Notice = &event
	case envelope.IsActionResponse():
		var action ActionEnvelope
		if err := json.Unmarshal(raw, &action); err != nil {
			return nil, err
		}
		parsed.Kind = EventKindActionResponse
		parsed.Action = &action
	}

	return parsed, nil
}
