package bot

import (
	"strings"
	"testing"
	"time"

	"njk_go/internal/config"
	"njk_go/internal/napcat"
)

func TestMatchIndexPrefersMoreSpecificPattern(t *testing.T) {
	service := NewService(config.Config{
		BotUserID:       "1558109748",
		BotNickname:     "你居垦",
		AllowedGroupIDs: map[string]struct{}{},
	}, nil, nil, nil, nil)

	pattern, index := service.matchIndex(".bbh 36 add 第一章\n内容")
	if pattern == nil {
		t.Fatal("expected pattern to match")
	}
	if index != bbhIndex+4 {
		t.Fatalf("unexpected index: %d", index)
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

func TestMatchIndexSupportsDotAIC(t *testing.T) {
	service := NewService(config.Config{
		BotUserID:       "1558109748",
		BotNickname:     "你居垦",
		AllowedGroupIDs: map[string]struct{}{},
	}, nil, nil, nil, nil)

	pattern, index := service.matchIndex(".aic")
	if pattern == nil || index != aicIndex {
		t.Fatalf("expected .aic to match aic index, got index=%d", index)
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
