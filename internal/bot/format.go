package bot

import (
	"strings"
	"time"
)

func formatDisplayTime(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

func formatDisplayDate(t time.Time) string {
	return t.Format("2006-01-02")
}

func normalizeOutboundText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, `\r\n`, "\n")
	text = strings.ReplaceAll(text, `\n`, "\n")
	text = strings.ReplaceAll(text, `\t`, "\t")
	return text
}
