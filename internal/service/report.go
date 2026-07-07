package service

import (
	"fmt"
	"strings"

	"njk_go/internal/client/pgstore"
	"njk_go/internal/util/utime"
)

func formatReport(stats *pgstore.ReportStats, dayNum int, limit int) string {
	if stats == nil || stats.MessageCount == 0 {
		groupName := "未知群聊"
		if stats != nil && stats.GroupName != "" {
			groupName = stats.GroupName
		}
		return fmt.Sprintf("📊 【%s】暂无消息统计数据。", groupName)
	}

	lines := []string{
		fmt.Sprintf("📊 【%s】全方位数据报告", stats.GroupName),
		fmt.Sprintf("⏳ 统计时段：%s 至 %s", stats.StartDate.Format("2006-01-02 15:04"), stats.EndDate.Format("2006-01-02 15:04")),
		strings.Repeat("-", 30),
		fmt.Sprintf("📈 概况：总计消息(%d条)，日均(%.2f条)", stats.MessageCount, averageDaily(stats.MessageCount, dayNum)),
		"",
	}

	if len(stats.TopChattedDates) > 0 {
		lines = append(lines, "🔥 【最活跃的日期】")
		for i, item := range stats.TopChattedDates {
			lines = append(lines, fmt.Sprintf(" %d. %s (%d条)", i+1, utime.FormatDisplayDate(item.Date), item.Count))
		}
		lines = append(lines, "")
	}

	if len(stats.LatestChatted) > 0 {
		lines = append(lines, "🌙 【熬夜之巅 (最晚发言)】")
		for i, item := range stats.LatestChatted {
			lines = append(lines, fmt.Sprintf(" %d. %s - %s", i+1, item.FullTime, item.Sender))
		}
		lines = append(lines, "   (注：按凌晨5点结算，越接近5:00排名越靠前)")
		lines = append(lines, "")
	}

	if len(stats.TopAttedUsers) > 0 {
		lines = append(lines, "📢 【社交核心 (被@最多)】")
		for i, item := range stats.TopAttedUsers {
			name := item.Nickname
			if name == "" {
				name = item.UserID
			}
			lines = append(lines, fmt.Sprintf(" %d. %s(%s) (%d次)", i+1, name, item.UserID, item.Count))
		}
		lines = append(lines, "")
	}

	if len(stats.TopChattedDates) == 0 && len(stats.LatestChatted) == 0 && len(stats.TopAttedUsers) == 0 {
		lines = append(lines, fmt.Sprintf("💡 最近 %d 天内暂无可展示的排行数据", limit), "")
	}

	lines = append(lines, strings.Repeat("-", 30), "💡 自动统计报告生成完毕")
	return strings.Join(lines, "\n")
}

func averageDaily(total int64, dayNum int) float64 {
	if dayNum <= 0 {
		return 0
	}
	return float64(total) / float64(dayNum)
}
