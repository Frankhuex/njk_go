package ws

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"njk_go/internal/bot"
	"njk_go/internal/config"
	"njk_go/internal/napcat"
)

func TestHandleNoticeSendsGroupMessage(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	conn := &Conn{
		conn:   serverSide,
		reader: bufio.NewReader(serverSide),
	}

	service := bot.NewService(config.Config{
		BotUserID:       "1558109748",
		BotNickname:     "你居垦",
		AllowedGroupIDs: map[string]struct{}{},
	}, nil, nil, nil, nil)
	event := &napcat.NoticeEvent{
		SelfID:   "1558109748",
		TargetID: "1558109748",
		GroupID:  "123456789",
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		service.HandleNotice(context.Background(), conn, "test-client", event)
	}()

	payload, err := readFramePayload(clientSide)
	if err != nil {
		t.Fatalf("readFramePayload returned error: %v", err)
	}
	<-done

	var req napcat.SendGroupMsgRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}

	if req.Action != "send_group_msg" {
		t.Fatalf("unexpected action: %s", req.Action)
	}
	if req.Params.GroupID != "123456789" {
		t.Fatalf("unexpected group id: %s", req.Params.GroupID)
	}
	if req.Params.Message.StringValue() != "灰色中分已然绽放" {
		t.Fatalf("unexpected message: %s", req.Params.Message.StringValue())
	}
}

func TestHandleNoticeIgnoresOtherTarget(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	conn := &Conn{
		conn:   serverSide,
		reader: bufio.NewReader(serverSide),
	}

	service := bot.NewService(config.Config{
		BotUserID:       "1558109748",
		BotNickname:     "你居垦",
		AllowedGroupIDs: map[string]struct{}{},
	}, nil, nil, nil, nil)
	event := &napcat.NoticeEvent{
		SelfID:   "1558109748",
		TargetID: "42",
		GroupID:  "123456789",
	}

	service.HandleNotice(context.Background(), conn, "test-client", event)

	_ = clientSide.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	buf := make([]byte, 1)
	_, err := clientSide.Read(buf)
	if err == nil {
		t.Fatal("expected no websocket frame to be written")
	}

	netErr, ok := err.(net.Error)
	if !ok || !netErr.Timeout() {
		t.Fatalf("expected timeout error, got: %v", err)
	}
}

func TestConnWriteTextSerializesConcurrentWrites(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	conn := &Conn{
		conn:   serverSide,
		reader: bufio.NewReader(serverSide),
	}

	payloads := [][]byte{
		[]byte(`{"a":1}`),
		[]byte(`{"b":2}`),
	}

	var wg sync.WaitGroup
	for _, payload := range payloads {
		wg.Add(1)
		go func(payload []byte) {
			defer wg.Done()
			if err := conn.WriteText(payload); err != nil {
				t.Errorf("WriteText returned error: %v", err)
			}
		}(payload)
	}

	received := make([]string, 0, len(payloads))
	for range payloads {
		frame, err := readFramePayload(clientSide)
		if err != nil {
			t.Fatalf("readFramePayload returned error: %v", err)
		}
		received = append(received, string(frame))
	}

	wg.Wait()

	if len(received) != 2 {
		t.Fatalf("unexpected received count: %d", len(received))
	}

	firstOK := received[0] == string(payloads[0]) || received[0] == string(payloads[1])
	secondOK := received[1] == string(payloads[0]) || received[1] == string(payloads[1])
	if !firstOK || !secondOK || received[0] == received[1] {
		t.Fatalf("unexpected frames: %#v", received)
	}
}

func readFramePayload(r io.Reader) ([]byte, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}

	payloadLen := uint64(header[1] & 0x7F)
	if payloadLen == 126 {
		var ext uint16
		if err := binary.Read(r, binary.BigEndian, &ext); err != nil {
			return nil, err
		}
		payloadLen = uint64(ext)
	} else if payloadLen == 127 {
		var ext uint64
		if err := binary.Read(r, binary.BigEndian, &ext); err != nil {
			return nil, err
		}
		payloadLen = ext
	}

	payload := make([]byte, payloadLen)
	_, err := io.ReadFull(r, payload)
	return payload, err
}
