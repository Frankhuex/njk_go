package bot

import (
	"bufio"
	"context"
	"encoding/json"
	"math/rand"
	"net"
	"strings"
	"testing"
	"time"

	"njk_go/internal/config"
	"njk_go/internal/napcat"
)

func TestMatchCommandPrefersMoreSpecificPattern(t *testing.T) {
	service := NewService(config.Config{
		BotUserID:       "1558109748",
		BotNickname:     "你居垦",
		AllowedGroupIDs: map[string]struct{}{},
	}, nil, nil, nil, nil)

	match := service.matchCommand(".bbh 36 add 第一章\n内容")
	if match == nil {
		t.Fatal("expected pattern to match")
	}
	if match.Command.Key != commandBBHAdd {
		t.Fatalf("unexpected command key: %s", match.Command.Key)
	}
}

func TestFormatReportDropsTopicAndWordSections(t *testing.T) {
	report := formatReport(&ReportStats{
		GroupName:    "测试群",
		MessageCount: 20,
		TopChattedDates: []ReportDay{
			{Date: time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC), Count: 12},
		},
		LatestChatted: []ReportNight{
			{FullTime: "2026-04-15 04:58:00", Sender: "你居垦"},
		},
		TopAttedUsers: []ReportAtUser{
			{Nickname: "甲", UserID: "1", Count: 3},
		},
		StartDate: time.Date(2026, 4, 10, 5, 0, 0, 0, time.Local),
		EndDate:   time.Date(2026, 4, 16, 1, 0, 0, 0, time.Local),
	}, 6, 5)

	if strings.Contains(report, "热门话题") || strings.Contains(report, "高频词汇") {
		t.Fatal("report should not contain removed topic or word sections")
	}
	if !strings.Contains(report, "熬夜之巅") {
		t.Fatal("report should contain 熬夜之巅 section")
	}
	if !strings.Contains(report, "被@最多") {
		t.Fatal("report should contain 被@最多 section")
	}
	if strings.Contains(report, "T00:00:00") || strings.Contains(report, "UTC") {
		t.Fatal("report should not contain raw time suffixes")
	}
}

func TestMatchCommandSupportsDotAIC(t *testing.T) {
	service := NewService(config.Config{
		BotUserID:       "1558109748",
		BotNickname:     "你居垦",
		AllowedGroupIDs: map[string]struct{}{},
	}, nil, nil, nil, nil)

	match := service.matchCommand(".aic")
	if match == nil || match.Command.Key != commandAIC {
		t.Fatalf("expected .aic to match aic command, got=%v", match)
	}
}

func TestMatchCommandSupportsFaceWithoutSpace(t *testing.T) {
	service := NewService(config.Config{
		BotUserID:       "1558109748",
		BotNickname:     "你居垦",
		AllowedGroupIDs: map[string]struct{}{},
	}, nil, nil, nil, nil)

	match := service.matchCommand(".face12")
	if match == nil || match.Command.Key != commandFace {
		t.Fatalf("expected .face12 to match face command, got=%v", match)
	}
	if len(match.Groups) < 2 || match.Groups[1] != "12" {
		t.Fatalf("unexpected face match groups: %#v", match.Groups)
	}
}

func TestMatchCommandSupportsJSONWithOptionalSpace(t *testing.T) {
	service := NewService(config.Config{
		BotUserID:       "1558109748",
		BotNickname:     "你居垦",
		AllowedGroupIDs: map[string]struct{}{},
	}, nil, nil, nil, nil)

	for _, input := range []string{".json12", ".json 12"} {
		match := service.matchCommand(input)
		if match == nil || match.Command.Key != commandJSON {
			t.Fatalf("expected %q to match json command, got=%v", input, match)
		}
		if len(match.Groups) < 2 || match.Groups[1] != "12" {
			t.Fatalf("unexpected json match groups for %q: %#v", input, match.Groups)
		}
	}
}

func TestMatchCommandRejectsInvalidJSONCount(t *testing.T) {
	service := NewService(config.Config{
		BotUserID:       "1558109748",
		BotNickname:     "你居垦",
		AllowedGroupIDs: map[string]struct{}{},
	}, nil, nil, nil, nil)

	if match := service.matchCommand(".json abc"); match != nil {
		t.Fatalf("expected invalid json command not to match, got=%v", match)
	}
}

func TestMatchCommandSupportsFileWithOptionalSpace(t *testing.T) {
	service := NewService(config.Config{
		BotUserID:       "1558109748",
		BotNickname:     "你居垦",
		AllowedGroupIDs: map[string]struct{}{},
	}, nil, nil, nil, nil)

	for _, input := range []string{".file12", ".file 12"} {
		match := service.matchCommand(input)
		if match == nil || match.Command.Key != commandFile {
			t.Fatalf("expected %q to match file command, got=%v", input, match)
		}
		if len(match.Groups) < 2 || match.Groups[1] != "12" {
			t.Fatalf("unexpected file match groups for %q: %#v", input, match.Groups)
		}
	}
}

func TestMatchCommandRejectsInvalidFileCount(t *testing.T) {
	service := NewService(config.Config{
		BotUserID:       "1558109748",
		BotNickname:     "你居垦",
		AllowedGroupIDs: map[string]struct{}{},
	}, nil, nil, nil, nil)

	if match := service.matchCommand(".file abc"); match != nil {
		t.Fatalf("expected invalid file command not to match, got=%v", match)
	}
}

func TestImageToFileItemsFromMessagesUsesRawJSONFileNames(t *testing.T) {
	rawJSONBytes, err := json.Marshal([]napcat.MessageSegment{
		napcat.NewTextSegment("hi"),
		{
			Type: napcat.SegmentTypeImage,
			Data: napcat.MessageSegmentData{
				URL:  " https://example.com/download?id=1 ",
				File: "abc.png",
			},
		},
		{
			Type: napcat.SegmentTypeImage,
			Data: napcat.MessageSegmentData{
				URL:  "   ",
				File: "skip.jpg",
			},
		},
		{
			Type: napcat.SegmentTypeImage,
			Data: napcat.MessageSegmentData{
				URL:  "https://example.com/download?id=2",
				File: "anim.gif",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal raw json: %v", err)
	}

	files := imageToFileItemsFromMessages([]StoredMessage{
		{RawJSON: string(rawJSONBytes)},
		{RawJSON: `"bot reply"`},
	})

	if len(files) != 2 {
		t.Fatalf("expected 2 files, got=%d: %#v", len(files), files)
	}
	if files[0] != (outboundFile{URL: "https://example.com/download?id=1", FileName: "abc.png"}) {
		t.Fatalf("unexpected first file: %#v", files[0])
	}
	if files[1] != (outboundFile{URL: "https://example.com/download?id=2", FileName: "anim.gif"}) {
		t.Fatalf("unexpected second file: %#v", files[1])
	}
}

func TestImageAndFileOutboundSegmentTypes(t *testing.T) {
	image := imageOutbound("123", []string{"https://example.com/a.png"})
	if image.ImageSegmentType != napcat.SegmentTypeImage {
		t.Fatalf("image outbound should use image segment, got=%s", image.ImageSegmentType)
	}

	file := fileOutbound("123", []outboundFile{{URL: "https://example.com/a.gif", FileName: "a.gif"}})
	if file.ImageSegmentType != napcat.SegmentTypeFile {
		t.Fatalf("file outbound should use file segment, got=%s", file.ImageSegmentType)
	}
	if len(file.ImageFiles) != 1 || file.ImageFiles[0].FileName != "a.gif" {
		t.Fatalf("file outbound should keep file names, got=%#v", file.ImageFiles)
	}
	if file.ShouldSave {
		t.Fatal("file outbound should not be saved")
	}
}

func TestFormatRawJSONMessagesPreservesJSONTypes(t *testing.T) {
	result, err := formatRawJSONMessages([]StoredMessage{
		{RawJSON: `[{"type":"text","data":{"text":"hi"}}]`},
		{RawJSON: `"bot reply"`},
		{RawJSON: ``},
	})
	if err != nil {
		t.Fatalf("format raw json messages: %v", err)
	}
	parts := strings.Split(result, "\n\n")
	if len(parts) != 3 {
		t.Fatalf("expected 3 formatted raw json values, got=%d: %q", len(parts), result)
	}

	if !strings.Contains(parts[0], "\n    ") {
		t.Fatalf("expected formatted json with four-space indentation, got=%q", parts[0])
	}

	var segments []napcat.MessageSegment
	if err := json.Unmarshal([]byte(parts[0]), &segments); err != nil {
		t.Fatalf("first value should remain a segment array: %v", err)
	}
	if len(segments) != 1 || segments[0].Type != "text" || segments[0].Data.Text != "hi" {
		t.Fatalf("unexpected first value: %#v", segments)
	}

	var botReply string
	if err := json.Unmarshal([]byte(parts[1]), &botReply); err != nil {
		t.Fatalf("second value should remain a json string: %v", err)
	}
	if botReply != "bot reply" {
		t.Fatalf("unexpected second value: %q", botReply)
	}
	if parts[2] != "null" {
		t.Fatalf("expected empty raw json to become null, got=%s", parts[2])
	}
}

func TestMatchCommandSupportsDiceWithOptionalInnerSpaces(t *testing.T) {
	service := NewService(config.Config{
		BotUserID:       "1558109748",
		BotNickname:     "你居垦",
		AllowedGroupIDs: map[string]struct{}{},
	}, nil, nil, nil, nil)

	match := service.matchCommand(".2 d 6")
	if match == nil || match.Command.Key != commandDice {
		t.Fatalf("expected .2 d 6 to match dice command, got=%v", match)
	}
	if len(match.Groups) < 3 || match.Groups[1] != "2" || match.Groups[2] != "6" {
		t.Fatalf("unexpected dice match groups: %#v", match.Groups)
	}
}

func TestHandleDiceCommandReturnsCommaSeparatedRolls(t *testing.T) {
	service := NewService(config.Config{
		BotUserID:       "1558109748",
		BotNickname:     "你居垦",
		AllowedGroupIDs: map[string]struct{}{},
	}, nil, nil, nil, nil)
	service.rng = rand.New(rand.NewSource(1))

	match := service.matchCommand(".2d6")
	if match == nil {
		t.Fatal("expected dice command to match")
	}

	outbound, err := service.handleDiceCommand(context.Background(), "123", *match)
	if err != nil {
		t.Fatalf("handle dice command: %v", err)
	}
	if outbound == nil {
		t.Fatal("expected outbound response")
	}
	if outbound.ShouldSave {
		t.Fatal("dice command response should not be saved")
	}
	if outbound.Message != "6+4=10" {
		t.Fatalf("unexpected dice output: %q", outbound.Message)
	}
}

func TestHandleDiceCommandRejectsCountOverTwenty(t *testing.T) {
	service := NewService(config.Config{
		BotUserID:       "1558109748",
		BotNickname:     "你居垦",
		AllowedGroupIDs: map[string]struct{}{},
	}, nil, nil, nil, nil)

	match := service.matchCommand(".21d6")
	if match == nil {
		t.Fatal("expected dice command to match")
	}

	outbound, err := service.handleDiceCommand(context.Background(), "123", *match)
	if err != nil {
		t.Fatalf("handle dice command: %v", err)
	}
	if outbound == nil || outbound.Message != "太多啦，最多20次" {
		t.Fatalf("unexpected outbound: %#v", outbound)
	}
}

func TestExtractFaceIDsFromRawJSON(t *testing.T) {
	rawJSONBytes, err := json.Marshal([]napcat.MessageSegment{
		napcat.NewTextSegment("hi"),
		{
			Type: "face",
			Data: napcat.MessageSegmentData{ID: "123"},
		},
		napcat.NewReplySegment("456"),
		{
			Type: "face",
			Data: napcat.MessageSegmentData{ID: "789"},
		},
	})
	if err != nil {
		t.Fatalf("marshal raw json: %v", err)
	}

	faceIDs, err := extractFaceIDsFromRawJSON(string(rawJSONBytes))
	if err != nil {
		t.Fatalf("extract face ids: %v", err)
	}
	if len(faceIDs) != 2 {
		t.Fatalf("expected 2 face ids, got=%d", len(faceIDs))
	}
	if faceIDs[0] != "123" || faceIDs[1] != "789" {
		t.Fatalf("unexpected face ids: %#v", faceIDs)
	}
}

func TestExtractFaceIDsFromRawJSONRejectsNonSegmentJSON(t *testing.T) {
	rawJSONBytes, err := json.Marshal("hello")
	if err != nil {
		t.Fatalf("marshal raw json: %v", err)
	}

	_, err = extractFaceIDsFromRawJSON(string(rawJSONBytes))
	if err == nil {
		t.Fatal("expected non-segment json to fail")
	}
}

func TestMentionsBotDetectsAtSegment(t *testing.T) {
	payload := napcat.NewSegmentMessage(
		napcat.NewAtSegment("1558109748", "你居垦"),
		napcat.NewTextSegment(" 在吗"),
	)

	if !mentionsBot(payload, "1558109748") {
		t.Fatal("expected at segment to mention bot")
	}
}

func TestNormalizeOutboundTextConvertsEscapedNewlines(t *testing.T) {
	text := normalizeOutboundText("第一行\\n第二行\\n第三行")
	if strings.Contains(text, `\n`) {
		t.Fatal("expected escaped newline to be converted")
	}
	if !strings.Contains(text, "\n") {
		t.Fatal("expected actual newline to exist")
	}
}

func TestFormatDisplayTimeDoesNotShiftClock(t *testing.T) {
	input := time.Date(2026, 4, 16, 10, 30, 45, 0, time.FixedZone("CST", 8*3600))
	if got := formatDisplayTime(input); got != "2026-04-16 10:30:45" {
		t.Fatalf("unexpected formatted time: %s", got)
	}
}

func TestHandleGroupMessageIgnoresBannedUser(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	conn := &stubOutboundWriter{conn: &wsTestConn{
		conn:   serverSide,
		reader: bufio.NewReader(serverSide),
	}}

	service := NewService(config.Config{
		BotUserID:       "1558109748",
		BotNickname:     "你居垦",
		AllowedGroupIDs: map[string]struct{}{},
		BannedUserIDs: map[string]struct{}{
			"3889001802": {},
		},
	}, nil, nil, nil, nil)

	event := &napcat.GroupMessageEvent{
		Time:       time.Now().Unix(),
		UserID:     "3889001802",
		GroupID:    "123456789",
		RawMessage: ".help",
		Sender: napcat.Sender{
			UserID:   "3889001802",
			Nickname: "banned-user",
		},
		Message: napcat.NewTextMessage(".help"),
	}

	service.HandleGroupMessage(context.Background(), conn, "test-client", event)

	_ = clientSide.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	buf := make([]byte, 1)
	_, err := clientSide.Read(buf)
	if err == nil {
		t.Fatal("expected banned user message to be ignored without response")
	}
	netErr, ok := err.(net.Error)
	if !ok || !netErr.Timeout() {
		t.Fatalf("expected timeout error, got: %v", err)
	}
}

type stubOutboundWriter struct {
	conn *wsTestConn
}

func (s *stubOutboundWriter) WriteText(payload []byte) error {
	return s.conn.WriteText(payload)
}

type wsTestConn struct {
	conn   net.Conn
	reader *bufio.Reader
}

func (c *wsTestConn) WriteText(payload []byte) error {
	return writeWSFrame(c.conn, payload)
}

func writeWSFrame(conn net.Conn, payload []byte) error {
	header := []byte{0x81}
	switch length := len(payload); {
	case length < 126:
		header = append(header, byte(length))
	case length <= 65535:
		header = append(header, 126, byte(length>>8), byte(length))
	default:
		header = append(header, 127,
			byte(uint64(length)>>56),
			byte(uint64(length)>>48),
			byte(uint64(length)>>40),
			byte(uint64(length)>>32),
			byte(uint64(length)>>24),
			byte(uint64(length)>>16),
			byte(uint64(length)>>8),
			byte(uint64(length)),
		)
	}
	if _, err := conn.Write(header); err != nil {
		return err
	}
	_, err := conn.Write(payload)
	return err
}
