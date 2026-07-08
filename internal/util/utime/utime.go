package utime

import "time"

func FormatDisplayTime(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

func FormatDisplayDate(t time.Time) string {
	return t.Format("2006-01-02")
}

func StartOfReport(dayNum int, now time.Time) time.Time {
	todayFive := time.Date(now.Year(), now.Month(), now.Day(), 5, 0, 0, 0, now.Location())
	return todayFive.AddDate(0, 0, -dayNum)
}

