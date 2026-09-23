package napcathandler

import (
	"context"
	"encoding/json"
	"testing"

	"njk_go/internal/client/pgstore"
	"njk_go/internal/config"
	"njk_go/internal/napcat"
	"njk_go/internal/service"
)

type sendRecorder struct {
	payloads [][]byte
}

func (w *sendRecorder) WriteText(payload []byte) error {
	w.payloads = append(w.payloads, append([]byte(nil), payload...))
	return nil
}

func TestSendCommandSendsOneStringWithoutStorage(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"正文", "正文"}, {" 正文", " 正文"}, {" ", " "},
		{"正文  ", "正文  "}, {"第一行\n第二行\n", "第一行\n第二行\n"},
		{"你居垦 &#91;CQ:face,id=14&#93;", "你居垦 [CQ:face,id=14]"},
		{"&#91;x&#93; &amp; &#44;", "[x] & &#44;"},
		{"&amp;#91;CQ:face,id=14&amp;#93;", "&#91;CQ:face,id=14&#93;"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			// An uninitialized store makes any accidental database access fail.
			svc := service.NewService(config.Config{BotUserID: "123"}, &pgstore.Store{}, nil, nil, nil, nil, nil)
			h := NewHandler(svc)
			writer := &sendRecorder{}
			event := &napcat.GroupMessageEvent{
				GroupID: "456", UserID: "789", MessageID: "100",
				RawMessage: ".send " + tc.input,
				Message: napcat.NewSegmentMessage(napcat.MessageSegment{
					Type: napcat.SegmentTypeFace, Data: napcat.MessageSegmentData{ID: "14"},
				}),
			}
			h.HandleGroupMessage(context.Background(), writer, "test", event)
			if len(writer.payloads) != 1 {
				t.Fatalf("expected one request, got %d", len(writer.payloads))
			}
			var req struct {
				Action string `json:"action"`
				Params struct {
					GroupID napcat.ID `json:"group_id"`
					Message string    `json:"message"`
				} `json:"params"`
			}
			if err := json.Unmarshal(writer.payloads[0], &req); err != nil {
				t.Fatal(err)
			}
			if req.Action != "send_group_msg" || req.Params.GroupID != "456" || req.Params.Message != tc.want {
				t.Fatalf("unexpected request: %s", writer.payloads[0])
			}
			if err := svc.CompleteActionResult(context.Background(), "ok", 0, "101"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSendCommandRequiresSpaceAndBody(t *testing.T) {
	svc := service.NewService(config.Config{BotUserID: "123"}, nil, nil, nil, nil, nil, nil)
	for _, input := range []string{".send", ".send ", ".send正文", ".send\t正文", ".send\n正文"} {
		if match := svc.MatchCommand(input); match != nil {
			t.Errorf("unexpected match for %q: %s", input, match.Key())
		}
	}
	if match := svc.MatchCommand(".sendallface"); match == nil || match.Key() != "send_all_face" {
		t.Fatal(".sendallface must keep its existing handler")
	}
}

func TestSendJSONCommandSendsUnescapedSegmentWithoutStorage(t *testing.T) {
	svc := service.NewService(config.Config{BotUserID: "123"}, &pgstore.Store{}, nil, nil, nil, nil, nil)
	h := NewHandler(svc)
	writer := &sendRecorder{}
	event := &napcat.GroupMessageEvent{
		GroupID: "456", UserID: "789", MessageID: "100",
		RawMessage: `.sendjson {"type":"face","data":{"id":"500","raw":{"faceIndex":500,"faceText":"/秋秋赏月","faceType":2},"label":"&#91;月&#93; &amp; 星"}}`,
		Message: napcat.NewSegmentMessage(napcat.MessageSegment{
			Type: napcat.SegmentTypeFace, Data: napcat.MessageSegmentData{ID: "14"},
		}),
	}
	h.HandleGroupMessage(context.Background(), writer, "test", event)
	if len(writer.payloads) != 1 {
		t.Fatalf("expected one request, got %d", len(writer.payloads))
	}
	var req struct {
		Action string `json:"action"`
		Params struct {
			GroupID napcat.ID         `json:"group_id"`
			Message []json.RawMessage `json:"message"`
		} `json:"params"`
	}
	if err := json.Unmarshal(writer.payloads[0], &req); err != nil {
		t.Fatal(err)
	}
	if req.Action != "send_group_msg" || req.Params.GroupID != "456" || len(req.Params.Message) != 1 {
		t.Fatalf("unexpected request: %s", writer.payloads[0])
	}
	var data struct {
		ID    string `json:"id"`
		Label string `json:"label"`
		Raw   struct {
			FaceIndex int    `json:"faceIndex"`
			FaceText  string `json:"faceText"`
			FaceType  int    `json:"faceType"`
		} `json:"raw"`
	}
	var segment struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(req.Params.Message[0], &segment); err != nil || segment.Type != "face" {
		t.Fatalf("unexpected segment: %s, err=%v", req.Params.Message[0], err)
	}
	if err := json.Unmarshal(segment.Data, &data); err != nil {
		t.Fatal(err)
	}
	if data.ID != "500" || data.Label != "[月] & 星" || data.Raw.FaceIndex != 500 || data.Raw.FaceText != "/秋秋赏月" || data.Raw.FaceType != 2 {
		t.Fatalf("unexpected segment data: %+v", data)
	}
	if err := svc.CompleteActionResult(context.Background(), "ok", 0, "101"); err != nil {
		t.Fatal(err)
	}
}

func TestSendJSONCommandRejectsInvalidJSON(t *testing.T) {
	svc := service.NewService(config.Config{BotUserID: "123"}, &pgstore.Store{}, nil, nil, nil, nil, nil)
	h := NewHandler(svc)
	for _, input := range []string{
		`.sendjson {"type":"face","data":{"id":"500"}`,
		`.sendjson null`,
		`.sendjson []`,
	} {
		writer := &sendRecorder{}
		h.HandleGroupMessage(context.Background(), writer, "test", &napcat.GroupMessageEvent{
			GroupID: "456", UserID: "789", RawMessage: input,
		})
		if len(writer.payloads) != 1 {
			t.Fatalf("input %q: expected one error response, got %d", input, len(writer.payloads))
		}
		var req napcat.SendGroupMsgRequest
		if err := json.Unmarshal(writer.payloads[0], &req); err != nil {
			t.Fatal(err)
		}
		if !req.Params.Message.IsText() || req.Params.Message.Text == nil || (*req.Params.Message.Text)[:4] != "JSON" {
			t.Fatalf("input %q: unexpected response %s", input, writer.payloads[0])
		}
	}
	for _, input := range []string{".sendjson", ".sendjson ", ".sendjson正文"} {
		if match := svc.MatchCommand(input); match != nil {
			t.Errorf("unexpected match for %q: %s", input, match.Key())
		}
	}
}
