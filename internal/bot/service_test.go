package bot

import (
	"bufio"
	"context"
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
