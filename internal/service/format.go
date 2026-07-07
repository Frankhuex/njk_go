package service

import (
	"time"
)

func formatDisplayTime(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

func formatDisplayDate(t time.Time) string {
	return t.Format("2006-01-02")
}
