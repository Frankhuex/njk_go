package napcathandler

import (
	"context"
	"encoding/json"
	"testing"

	"njk_go/internal/config"
	"njk_go/internal/napcat"
	"njk_go/internal/service"
)

func TestJSONOutputSendsExactPlainText(t *testing.T) {
	svc := service.NewService(config.Config{}, nil, nil, nil, nil, nil, nil)
	h := NewHandler(svc)
	writer := &sendRecorder{}
	message := `[{"type":"text","data":{"text":"line\n[CQ:face,id=500]"}}]`
	h.executeActions(context.Background(), writer, "test", []service.OutboundAction{{
		GroupID: "456", Message: message, PreserveText: true,
	}})
	if len(writer.payloads) != 1 {
		t.Fatalf("expected one request, got %d", len(writer.payloads))
	}
	var req napcat.SendGroupMsgRequest
	if err := json.Unmarshal(writer.payloads[0], &req); err != nil {
		t.Fatal(err)
	}
	if req.Params.Message.Text == nil || *req.Params.Message.Text != message {
		t.Fatalf("JSON output changed: %s", writer.payloads[0])
	}
	if req.Params.AutoEscape == nil || !*req.Params.AutoEscape {
		t.Fatalf("JSON output must be sent as plain text: %s", writer.payloads[0])
	}
}
