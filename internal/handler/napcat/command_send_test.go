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
