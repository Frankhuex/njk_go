package utext

import "strings"

func ContainsExact(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func NormalizeOutboundText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, `\r\n`, "\n")
	text = strings.ReplaceAll(text, `\n`, "\n")
	text = strings.ReplaceAll(text, `\t`, "\t")
	return text
}

// UnescapeCQText restores the plain-text portion of a OneBot string message.
// Decode ampersands last so a literal "&#91;" remains literal after one pass.
func UnescapeCQText(text string) string {
	text = strings.ReplaceAll(text, "&#91;", "[")
	text = strings.ReplaceAll(text, "&#93;", "]")
	return strings.ReplaceAll(text, "&amp;", "&")
}
