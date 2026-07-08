package service

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"njk_go/internal/client/pgstore"
	"njk_go/internal/config"
	"njk_go/internal/dal/model"
	"njk_go/internal/napcat"
	"njk_go/internal/util/uface"
	"njk_go/internal/util/unapcat"
	"njk_go/internal/util/uslice"
	"njk_go/internal/util/utext"
	"njk_go/internal/util/utime"
)

func TestMatchCommandPrefersMoreSpecificPattern(t *testing.T) {
	service := NewService(config.Config{
		BotUserID:       "1558109748",
		BotNickname:     "你居垦",
		AllowedGroupIDs: map[string]struct{}{},
	}, nil, nil, nil, nil, nil)

	match := service.MatchCommand(".bbh 36 add 第一章\n内容")
	if match == nil {
		t.Fatal("expected pattern to match")
	}
	if match.Command.Key != commandBBHAdd {
		t.Fatalf("unexpected command key: %s", match.Command.Key)
	}
}

func TestFormatReportDropsTopicAndWordSections(t *testing.T) {
	report := formatReport(&pgstore.ReportStats{
		GroupName:    "测试群",
		MessageCount: 20,
		TopChattedDates: []pgstore.ReportDay{
			{Date: time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC), Count: 12},
		},
		LatestChatted: []pgstore.ReportNight{
			{FullTime: "2026-04-15 04:58:00", Sender: "你居垦"},
		},
		TopAttedUsers: []pgstore.ReportAtUser{
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
	}, nil, nil, nil, nil, nil)

	match := service.MatchCommand(".aic")
	if match == nil || match.Command.Key != commandAIC {
		t.Fatalf("expected .aic to match aic command, got=%v", match)
	}
}

func TestMatchCommandSupportsFaceWithoutSpace(t *testing.T) {
	service := NewService(config.Config{
		BotUserID:       "1558109748",
		BotNickname:     "你居垦",
		AllowedGroupIDs: map[string]struct{}{},
	}, nil, nil, nil, nil, nil)

	match := service.MatchCommand(".face12")
	if match == nil || match.Command.Key != commandFace {
		t.Fatalf("expected .face12 to match face command, got=%v", match)
	}
	if len(match.Groups) < 2 || match.Groups[1] != "12" {
		t.Fatalf("unexpected face match groups: %#v", match.Groups)
	}
}

func TestMatchCommandSupportsFaceIDSingle(t *testing.T) {
	service := NewService(config.Config{
		BotUserID:       "1558109748",
		BotNickname:     "你居垦",
		AllowedGroupIDs: map[string]struct{}{},
	}, nil, nil, nil, nil, nil)

	for _, input := range []string{".faceid12", ".faceid 12"} {
		match := service.MatchCommand(input)
		if match == nil || match.Command.Key != commandFaceID {
			t.Fatalf("expected %q to match faceid command, got=%v", input, match)
		}
		if len(match.Groups) < 2 || match.Groups[1] != "12" {
			t.Fatalf("unexpected faceid match groups for %q: %#v", input, match.Groups)
		}
	}
}

func TestMatchCommandSupportsFaceIDRange(t *testing.T) {
	service := NewService(config.Config{
		BotUserID:       "1558109748",
		BotNickname:     "你居垦",
		AllowedGroupIDs: map[string]struct{}{},
	}, nil, nil, nil, nil, nil)

	match := service.MatchCommand(".faceid 12-15")
	if match == nil || match.Command.Key != commandFaceID {
		t.Fatalf("expected .faceid 12-15 to match faceid command, got=%v", match)
	}
	if len(match.Groups) < 3 || match.Groups[1] != "12" || match.Groups[2] != "15" {
		t.Fatalf("unexpected faceid range match groups: %#v", match.Groups)
	}
}

func TestMatchCommandRejectsInvalidFaceID(t *testing.T) {
	service := NewService(config.Config{
		BotUserID:       "1558109748",
		BotNickname:     "你居垦",
		AllowedGroupIDs: map[string]struct{}{},
	}, nil, nil, nil, nil, nil)

	for _, input := range []string{".faceid abc", ".faceid 12-a", ".faceid"} {
		if match := service.MatchCommand(input); match != nil {
			t.Fatalf("expected invalid faceid command %q not to match, got=%v", input, match)
		}
	}
}

func TestMatchCommandSupportsGetFaceIDWithOptionalSpace(t *testing.T) {
	service := NewService(config.Config{
		BotUserID:       "1558109748",
		BotNickname:     "你居垦",
		AllowedGroupIDs: map[string]struct{}{},
	}, nil, nil, nil, nil, nil)

	for _, input := range []string{".getfaceid12", ".getfaceid 12"} {
		match := service.MatchCommand(input)
		if match == nil || match.Command.Key != commandGetFaceID {
			t.Fatalf("expected %q to match getfaceid command, got=%v", input, match)
		}
		if len(match.Groups) < 2 || match.Groups[1] != "12" {
			t.Fatalf("unexpected getfaceid match groups for %q: %#v", input, match.Groups)
		}
	}
}

func TestMatchCommandRejectsInvalidGetFaceID(t *testing.T) {
	service := NewService(config.Config{
		BotUserID:       "1558109748",
		BotNickname:     "你居垦",
		AllowedGroupIDs: map[string]struct{}{},
	}, nil, nil, nil, nil, nil)

	for _, input := range []string{".getfaceid abc", ".getfaceid"} {
		if match := service.MatchCommand(input); match != nil {
			t.Fatalf("expected invalid getfaceid command %q not to match, got=%v", input, match)
		}
	}
}

func TestMatchCommandSupportsAllFace(t *testing.T) {
	service := NewService(config.Config{
		BotUserID:       "1558109748",
		BotNickname:     "你居垦",
		AllowedGroupIDs: map[string]struct{}{},
	}, nil, nil, nil, nil, nil)

	match := service.MatchCommand(".allface")
	if match == nil || match.Command.Key != commandAllFace {
		t.Fatalf("expected .allface to match allface command, got=%v", match)
	}
}

func TestMatchCommandRejectsAllFaceWithArg(t *testing.T) {
	service := NewService(config.Config{
		BotUserID:       "1558109748",
		BotNickname:     "你居垦",
		AllowedGroupIDs: map[string]struct{}{},
	}, nil, nil, nil, nil, nil)

	if match := service.MatchCommand(".allface 1"); match != nil {
		t.Fatalf("expected .allface with arg not to match, got=%v", match)
	}
}

func TestMatchCommandSupportsJSONWithOptionalSpace(t *testing.T) {
	service := NewService(config.Config{
		BotUserID:       "1558109748",
		BotNickname:     "你居垦",
		AllowedGroupIDs: map[string]struct{}{},
	}, nil, nil, nil, nil, nil)

	for _, input := range []string{".json12", ".json 12"} {
		match := service.MatchCommand(input)
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
	}, nil, nil, nil, nil, nil)

	if match := service.MatchCommand(".json abc"); match != nil {
		t.Fatalf("expected invalid json command not to match, got=%v", match)
	}
}

func TestMatchCommandSupportsFileWithOptionalSpace(t *testing.T) {
	service := NewService(config.Config{
		BotUserID:       "1558109748",
		BotNickname:     "你居垦",
		AllowedGroupIDs: map[string]struct{}{},
	}, nil, nil, nil, nil, nil)

	for _, input := range []string{".file12", ".file 12"} {
		match := service.MatchCommand(input)
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
	}, nil, nil, nil, nil, nil)

	if match := service.MatchCommand(".file abc"); match != nil {
		t.Fatalf("expected invalid file command not to match, got=%v", match)
	}
}

func TestImageToFileSourceURLsFromRecordsSkipsBlankURLs(t *testing.T) {
	firstURL := " https://example.com/download?id=1 "
	blankURL := "   "
	secondURL := "https://example.com/download?id=2"

	urls := imageToFileSourceURLsFromRecords([]model.Image{
		{URL: nil},
		{URL: &firstURL},
		{URL: &blankURL},
		{URL: &secondURL},
	})

	if len(urls) != 2 {
		t.Fatalf("expected 2 urls, got=%d: %#v", len(urls), urls)
	}
	if urls[0] != "https://example.com/download?id=1" || urls[1] != "https://example.com/download?id=2" {
		t.Fatalf("unexpected urls: %#v", urls)
	}
}

func TestImageAndFileOutboundSegmentTypes(t *testing.T) {
	image := imageOutbound("123", []string{"https://example.com/a.png"})
	if image.ImageSegmentType != napcat.SegmentTypeImage {
		t.Fatalf("image outbound should use image segment, got=%s", image.ImageSegmentType)
	}

	file := fileOutbound("123", []string{"https://example.com/a.gif"})
	if file.ImageSegmentType != napcat.SegmentTypeFile {
		t.Fatalf("file outbound should use file segment, got=%s", file.ImageSegmentType)
	}
	if len(file.ImageURLs) != 1 || file.ImageURLs[0] != "https://example.com/a.gif" {
		t.Fatalf("file outbound should keep image urls, got=%#v", file.ImageURLs)
	}
	if file.ShouldSave {
		t.Fatal("file outbound should not be saved")
	}
}

func TestFormatRawJSONMessagesPreservesJSONTypes(t *testing.T) {
	result, err := formatRawJSONMessages([]pgstore.StoredMessage{
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
	}, nil, nil, nil, nil, nil)

	match := service.MatchCommand(".2 d 6")
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
	}, nil, nil, nil, nil, nil)

	match := service.MatchCommand(".2d6")
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
	parts := strings.Split(outbound.Message, "=")
	if len(parts) != 2 {
		t.Fatalf("unexpected dice output: %q", outbound.Message)
	}
	rolls := strings.Split(parts[0], "+")
	if len(rolls) != 2 {
		t.Fatalf("unexpected dice rolls: %q", outbound.Message)
	}
	left, err := strconv.Atoi(rolls[0])
	if err != nil || left < 1 || left > 6 {
		t.Fatalf("unexpected first roll: %q", outbound.Message)
	}
	right, err := strconv.Atoi(rolls[1])
	if err != nil || right < 1 || right > 6 {
		t.Fatalf("unexpected second roll: %q", outbound.Message)
	}
	total, err := strconv.Atoi(parts[1])
	if err != nil || total != left+right {
		t.Fatalf("unexpected dice total: %q", outbound.Message)
	}
}

func TestHandleDiceCommandRejectsCountOverTwenty(t *testing.T) {
	service := NewService(config.Config{
		BotUserID:       "1558109748",
		BotNickname:     "你居垦",
		AllowedGroupIDs: map[string]struct{}{},
	}, nil, nil, nil, nil, nil)

	match := service.MatchCommand(".21d6")
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
		napcat.NewFaceSegment("123"),
		napcat.NewReplySegment("456"),
		napcat.NewFaceSegment("789"),
	})
	if err != nil {
		t.Fatalf("marshal raw json: %v", err)
	}

	faceIDs, err := uface.ExtractFaceIDsFromRawJSON(string(rawJSONBytes))
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

func TestFaceIDsFromSegmentsDeduplicatesAndSkipsBlank(t *testing.T) {
	faceIDs := uface.FaceIDsFromSegments([]napcat.MessageSegment{
		napcat.NewTextSegment("hi"),
		napcat.NewFaceSegment("123"),
		napcat.NewFaceSegment("123"),
		napcat.NewFaceSegment(""),
		napcat.NewFaceSegment("456"),
	})
	if len(faceIDs) != 2 || faceIDs[0] != "123" || faceIDs[1] != "456" {
		t.Fatalf("unexpected face ids: %#v", faceIDs)
	}
}

func TestEmojiLikeFaceIDsUsesLikes(t *testing.T) {
	faceIDs := uface.EmojiLikeFaceIDs([]napcat.EmojiLike{
		{EmojiID: "66"},
		{EmojiID: "77"},
		{EmojiID: "66"},
		{EmojiID: ""},
	})
	if len(faceIDs) != 2 || faceIDs[0] != "66" || faceIDs[1] != "77" {
		t.Fatalf("unexpected emoji like face ids: %#v", faceIDs)
	}
}

func TestSortFaceIDsNumericOrder(t *testing.T) {
	faceIDs := []string{"10", "2", "1", "abc", "20"}
	uslice.SortIntStrings(faceIDs)
	want := []string{"1", "2", "10", "20", "abc"}
	if strings.Join(faceIDs, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected sorted face ids: %#v", faceIDs)
	}
}

func TestFormatGetFaceIDRowsGroupsBySource(t *testing.T) {
	rows := []pgstore.GetFaceIDMessageRow{
		{
			MessageID:        "1003",
			SegmentFaceIDs:   []string{"1", "2", "10"},
			EmojiLikeFaceIDs: []string{"5", "66"},
		},
		{
			MessageID:        "1002",
			SegmentFaceIDs:   []string{"14"},
			EmojiLikeFaceIDs: nil,
		},
		{
			MessageID:        "1001",
			SegmentFaceIDs:   nil,
			EmojiLikeFaceIDs: []string{"77", "77"},
		},
	}

	got := formatGetFaceIDRows(rows)
	want := strings.Join([]string{
		"发：1，2，10",
		"贴：5，66",
		"发：14",
		"贴：77，77",
	}, "\n")
	if got != want {
		t.Fatalf("unexpected formatted rows:\n%s", got)
	}
}

func TestFormatAllFaceIDsUsesFullWidthPunctuation(t *testing.T) {
	got := formatAllFaceIDs([]string{"1", "2", "10"}, []string{"2", "10"})
	want := "全部：1，2，10\n贴过的：2，10"
	if got != want {
		t.Fatalf("unexpected allface output: %q", got)
	}
}

func TestHandleFaceIDCommandBuildsSingleFaceSegment(t *testing.T) {
	service := NewService(config.Config{
		BotUserID:       "1558109748",
		BotNickname:     "你居垦",
		AllowedGroupIDs: map[string]struct{}{},
	}, nil, nil, nil, nil, nil)

	match := service.MatchCommand(".faceid 12")
	if match == nil {
		t.Fatal("expected faceid command to match")
	}
	outbound, err := service.handleFaceIDCommand(context.Background(), "123", "4456", *match)
	if err != nil {
		t.Fatalf("handle faceid command: %v", err)
	}
	if outbound == nil || !outbound.ShouldSave || len(outbound.Segments) != 1 {
		t.Fatalf("unexpected outbound: %#v", outbound)
	}
	if outbound.Segments[0].Type != napcat.SegmentTypeFace || outbound.Segments[0].Data.ID != "12" {
		t.Fatalf("unexpected face segment: %#v", outbound.Segments[0])
	}
}

func TestHandleFaceIDCommandBuildsRangeFaceSegments(t *testing.T) {
	service := NewService(config.Config{
		BotUserID:       "1558109748",
		BotNickname:     "你居垦",
		AllowedGroupIDs: map[string]struct{}{},
	}, nil, nil, nil, nil, nil)

	match := service.MatchCommand(".faceid 12-14")
	if match == nil {
		t.Fatal("expected faceid command to match")
	}
	outbound, err := service.handleFaceIDCommand(context.Background(), "123", "4456", *match)
	if err != nil {
		t.Fatalf("handle faceid command: %v", err)
	}
	if outbound == nil || len(outbound.Segments) != 3 {
		t.Fatalf("unexpected outbound: %#v", outbound)
	}
	for i, wantID := range []napcat.ID{"12", "13", "14"} {
		if outbound.Segments[i].Type != napcat.SegmentTypeFace || outbound.Segments[i].Data.ID != wantID {
			t.Fatalf("unexpected segment at %d: %#v", i, outbound.Segments[i])
		}
	}
}

func TestHandleFaceIDCommandRejectsInvalidRange(t *testing.T) {
	service := NewService(config.Config{
		BotUserID:       "1558109748",
		BotNickname:     "你居垦",
		AllowedGroupIDs: map[string]struct{}{},
	}, nil, nil, nil, nil, nil)

	for _, input := range []string{".faceid 0", ".faceid 3-1"} {
		match := service.MatchCommand(input)
		if match == nil {
			t.Fatalf("expected %q to match before handler validation", input)
		}
		outbound, err := service.handleFaceIDCommand(context.Background(), "123", "4456", *match)
		if err != nil {
			t.Fatalf("handle faceid command: %v", err)
		}
		if outbound == nil || outbound.Message != "参数错误" {
			t.Fatalf("unexpected outbound for %q: %#v", input, outbound)
		}
	}
}

func TestHandleFaceIDCommandRejectsLargeRange(t *testing.T) {
	service := NewService(config.Config{
		BotUserID:       "1558109748",
		BotNickname:     "你居垦",
		AllowedGroupIDs: map[string]struct{}{},
	}, nil, nil, nil, nil, nil)

	match := service.MatchCommand(".faceid 1-51")
	if match == nil {
		t.Fatal("expected faceid command to match")
	}
	outbound, err := service.handleFaceIDCommand(context.Background(), "123", "4456", *match)
	if err != nil {
		t.Fatalf("handle faceid command: %v", err)
	}
	if outbound == nil || outbound.Message != "太多啦，最多50个" {
		t.Fatalf("unexpected outbound: %#v", outbound)
	}
}

func TestExtractFaceIDsFromRawJSONRejectsNonSegmentJSON(t *testing.T) {
	rawJSONBytes, err := json.Marshal("hello")
	if err != nil {
		t.Fatalf("marshal raw json: %v", err)
	}

	_, err = uface.ExtractFaceIDsFromRawJSON(string(rawJSONBytes))
	if err == nil {
		t.Fatal("expected non-segment json to fail")
	}
}

func TestMentionsBotDetectsAtSegment(t *testing.T) {
	payload := napcat.NewSegmentMessage(
		napcat.NewAtSegment("1558109748", "你居垦"),
		napcat.NewTextSegment(" 在吗"),
	)

	if !unapcat.MentionsUser(payload, "1558109748") {
		t.Fatal("expected at segment to mention bot")
	}
}

func TestNormalizeOutboundTextConvertsEscapedNewlines(t *testing.T) {
	text := utext.NormalizeOutboundText("第一行\\n第二行\\n第三行")
	if strings.Contains(text, `\n`) {
		t.Fatal("expected escaped newline to be converted")
	}
	if !strings.Contains(text, "\n") {
		t.Fatal("expected actual newline to exist")
	}
}

func TestFormatDisplayTimeDoesNotShiftClock(t *testing.T) {
	input := time.Date(2026, 4, 16, 10, 30, 45, 0, time.FixedZone("CST", 8*3600))
	if got := utime.FormatDisplayTime(input); got != "2026-04-16 10:30:45" {
		t.Fatalf("unexpected formatted time: %s", got)
	}
}

func TestIsUserBannedUsesConfig(t *testing.T) {
	service := NewService(config.Config{
		BotUserID:       "1558109748",
		BotNickname:     "你居垦",
		AllowedGroupIDs: map[string]struct{}{},
		BannedUserIDs: map[string]struct{}{
			"3889001802": {},
		},
	}, nil, nil, nil, nil, nil)

	if !service.IsUserBanned("3889001802") {
		t.Fatal("expected banned user to be detected")
	}
	if service.IsUserBanned("123456") {
		t.Fatal("unexpected banned result for normal user")
	}
}

type stubOutboundWriter struct {
	conn *wsTestConn
}

func (s *stubOutboundWriter) WriteText(payload []byte) error {
	return s.conn.WriteText(payload)
}

type recordingOutboundWriter struct {
	payload []byte
}

func (r *recordingOutboundWriter) WriteText(payload []byte) error {
	r.payload = append(r.payload[:0], payload...)
	return nil
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
