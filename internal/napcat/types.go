package napcat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
)

// ID 对应 NapCat 文档里大量出现的 string | number 标识字段，如 group_id、user_id、message_id。
type ID string

func (id *ID) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		*id = ""
		return nil
	}

	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		*id = ID(s)
		return nil
	}

	var num json.Number
	if err := json.Unmarshal(data, &num); err != nil {
		return fmt.Errorf("invalid id: %s", string(data))
	}
	*id = ID(num.String())
	return nil
}

func (id ID) String() string {
	return string(id)
}

func (id ID) Int64() (int64, error) {
	if id == "" {
		return 0, nil
	}
	return strconv.ParseInt(string(id), 10, 64)
}

// InboundEnvelope 对应 NapCat 反向 WS 上报与 action 回执的公共外层字段。
type InboundEnvelope struct {
	PostType      string          `json:"post_type,omitempty"`
	MetaEventType string          `json:"meta_event_type,omitempty"`
	MessageType   string          `json:"message_type,omitempty"`
	NoticeType    string          `json:"notice_type,omitempty"`
	SubType       string          `json:"sub_type,omitempty"`
	Status        json.RawMessage `json:"status,omitempty"`
	Retcode       int             `json:"retcode,omitempty"`
	Echo          json.RawMessage `json:"echo,omitempty"`
}

func (e InboundEnvelope) IsGroupMessage() bool {
	return e.PostType == "message" && e.MessageType == "group"
}

func (e InboundEnvelope) IsNotice() bool {
	return e.PostType == "notice"
}

func (e InboundEnvelope) IsActionResponse() bool {
	if len(e.Status) == 0 {
		return false
	}
	var status string
	return json.Unmarshal(e.Status, &status) == nil && status != ""
}

// ActionRequest 对应 NapCat WS 调用 API 时发送的通用请求体，形如 {"action":"...","params":{...}}。
type ActionRequest[T any] struct {
	Action string `json:"action"`
	Params T      `json:"params"`
	Echo   any    `json:"echo,omitempty"`
}

// ActionResponse 对应 NapCat API 调用后的通用返回体，复用 status、retcode、data 等字段。
type ActionResponse[T any] struct {
	Status      string `json:"status"`
	Retcode     int    `json:"retcode"`
	Data        T      `json:"data"`
	MessageText string `json:"message,omitempty"`
	Wording     string `json:"wording,omitempty"`
	Stream      string `json:"stream,omitempty"`
	Echo        any    `json:"echo,omitempty"`
}

// SendGroupMsgRequest 对应 NapCat 的 send_group_msg 请求体。
type SendGroupMsgRequest = ActionRequest[SendGroupMsgParams]

// GetMsgRequest 对应 NapCat 的 get_msg 请求体。
type GetMsgRequest = ActionRequest[GetMsgParams]

// SetMsgEmojiLikeRequest 对应 NapCat 的 set_msg_emoji_like 请求体。
type SetMsgEmojiLikeRequest = ActionRequest[SetMsgEmojiLikeParams]

// SendGroupMsgParams 对应 NapCat send_group_msg 的 params 部分。
type SendGroupMsgParams struct {
	GroupID    ID             `json:"group_id"`
	Message    MessagePayload `json:"message"`
	AutoEscape *bool          `json:"auto_escape,omitempty"`
}

// GetMsgParams 对应 NapCat get_msg 的 params 部分。
type GetMsgParams struct {
	MessageID ID `json:"message_id"`
}

// SetMsgEmojiLikeParams 对应 NapCat set_msg_emoji_like 的 params 部分。
type SetMsgEmojiLikeParams struct {
	MessageID ID     `json:"message_id"`
	EmojiID   string `json:"emoji_id"`
}

// SendMsgResponseData 对应 NapCat send_group_msg 成功响应中的 data 结构。
type SendMsgResponseData struct {
	MessageID ID     `json:"message_id"`
	ResID     string `json:"res_id,omitempty"`
	ForwardID string `json:"forward_id,omitempty"`
}

// GetMsgResponse 对应 NapCat get_msg 的完整返回体。
type GetMsgResponse = ActionResponse[*GetMsgData]

// SendMsgResponse 对应 NapCat send_group_msg 的完整返回体。
type SendMsgResponse = ActionResponse[*SendMsgResponseData]

// GetMsgData 对应 NapCat get_msg 返回体中的 data 结构。
type GetMsgData struct {
	Time           int64          `json:"time"`
	MessageType    string         `json:"message_type"`
	MessageID      ID             `json:"message_id"`
	RealID         ID             `json:"real_id"`
	MessageSeq     int64          `json:"message_seq"`
	Sender         Sender         `json:"sender"`
	Message        MessagePayload `json:"message"`
	RawMessage     string         `json:"raw_message"`
	Font           int64          `json:"font"`
	GroupID        ID             `json:"group_id,omitempty"`
	UserID         ID             `json:"user_id"`
	EmojiLikesList []string       `json:"emoji_likes_list,omitempty"`
}

// GroupMessageEvent 对应 NapCat 反向 WS 上报的群消息事件。
type GroupMessageEvent struct {
	Time        int64          `json:"time"`
	SelfID      ID             `json:"self_id"`
	PostType    string         `json:"post_type"`
	MessageType string         `json:"message_type"`
	SubType     string         `json:"sub_type,omitempty"`
	MessageID   ID             `json:"message_id"`
	UserID      ID             `json:"user_id"`
	GroupID     ID             `json:"group_id"`
	GroupName   string         `json:"group_name,omitempty"`
	RawMessage  string         `json:"raw_message"`
	Font        int64          `json:"font,omitempty"`
	Sender      Sender         `json:"sender"`
	Message     MessagePayload `json:"message"`
}

// NoticeEvent 对应 NapCat 反向 WS 上报的 notice 事件。
type NoticeEvent struct {
	Time        int64  `json:"time"`
	SelfID      ID     `json:"self_id"`
	PostType    string `json:"post_type"`
	NoticeType  string `json:"notice_type,omitempty"`
	SubType     string `json:"sub_type,omitempty"`
	UserID      ID     `json:"user_id,omitempty"`
	GroupID     ID     `json:"group_id,omitempty"`
	OperatorID  ID     `json:"operator_id,omitempty"`
	TargetID    ID     `json:"target_id,omitempty"`
	MessageID   ID     `json:"message_id,omitempty"`
	RawInfo     any    `json:"raw_info,omitempty"`
	GroupName   string `json:"group_name,omitempty"`
	Nickname    string `json:"nickname,omitempty"`
	TargetUin   ID     `json:"target_uin,omitempty"`
	OperatorUin ID     `json:"operator_uin,omitempty"`
}

// Sender 对应 NapCat 消息事件或 get_msg 返回中的 sender 结构。
type Sender struct {
	UserID   ID     `json:"user_id,omitempty"`
	Nickname string `json:"nickname,omitempty"`
	Card     string `json:"card,omitempty"`
	Sex      string `json:"sex,omitempty"`
	Age      int    `json:"age,omitempty"`
	Area     string `json:"area,omitempty"`
	Level    string `json:"level,omitempty"`
	Role     string `json:"role,omitempty"`
	Title    string `json:"title,omitempty"`
}

// MessagePayload 对应 NapCat 文档里的 OB11MessageMixType，可为字符串、单个消息段或消息段数组。
type MessagePayload struct {
	Text     *string
	Segments []MessageSegment
}

func NewTextMessage(text string) MessagePayload {
	return MessagePayload{Text: &text}
}

func NewSegmentMessage(segments ...MessageSegment) MessagePayload {
	return MessagePayload{Segments: segments}
}

func (p *MessagePayload) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		*p = MessagePayload{}
		return nil
	}

	switch data[0] {
	case '"':
		var text string
		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}
		*p = MessagePayload{Text: &text}
		return nil
	case '[':
		var segments []MessageSegment
		if err := json.Unmarshal(data, &segments); err != nil {
			return err
		}
		*p = MessagePayload{Segments: segments}
		return nil
	case '{':
		var segment MessageSegment
		if err := json.Unmarshal(data, &segment); err != nil {
			return err
		}
		*p = MessagePayload{Segments: []MessageSegment{segment}}
		return nil
	default:
		return fmt.Errorf("unsupported message payload: %s", string(data))
	}
}

func (p MessagePayload) MarshalJSON() ([]byte, error) {
	if p.Text != nil {
		return json.Marshal(*p.Text)
	}
	if p.Segments == nil {
		return []byte("null"), nil
	}
	return json.Marshal(p.Segments)
}

func (p MessagePayload) IsText() bool {
	return p.Text != nil
}

func (p MessagePayload) StringValue() string {
	if p.Text == nil {
		return ""
	}
	return *p.Text
}

// MessageSegment 对应 NapCat 文档里的 OB11MessageData 单个消息段。
type MessageSegment struct {
	Type string             `json:"type"`
	Data MessageSegmentData `json:"data"`
}

// MessageSegmentData 对应 NapCat 各类消息段的 data 字段合集，覆盖当前项目会用到的主要字段。
type MessageSegmentData struct {
	Text           string          `json:"text,omitempty"`
	QQ             string          `json:"qq,omitempty"`
	Name           string          `json:"name,omitempty"`
	ID             ID              `json:"id,omitempty"`
	Seq            int64           `json:"seq,omitempty"`
	File           string          `json:"file,omitempty"`
	Path           string          `json:"path,omitempty"`
	URL            string          `json:"url,omitempty"`
	Thumb          string          `json:"thumb,omitempty"`
	Summary        string          `json:"summary,omitempty"`
	SubType        int64           `json:"sub_type,omitempty"`
	EmojiID        string          `json:"emoji_id,omitempty"`
	EmojiPackageID int64           `json:"emoji_package_id,omitempty"`
	Key            string          `json:"key,omitempty"`
	ResultID       string          `json:"resultId,omitempty"`
	ChainCount     int64           `json:"chainCount,omitempty"`
	Lat            json.Number     `json:"lat,omitempty"`
	Lon            json.Number     `json:"lon,omitempty"`
	Title          string          `json:"title,omitempty"`
	Content        *MessagePayload `json:"content,omitempty"`
	Data           json.RawMessage `json:"data,omitempty"`
}

func NewTextSegment(text string) MessageSegment {
	return MessageSegment{
		Type: "text",
		Data: MessageSegmentData{Text: text},
	}
}

func NewAtSegment(qq string, name string) MessageSegment {
	return MessageSegment{
		Type: "at",
		Data: MessageSegmentData{QQ: qq, Name: name},
	}
}

func NewReplySegment(id ID) MessageSegment {
	return MessageSegment{
		Type: "reply",
		Data: MessageSegmentData{ID: id},
	}
}
