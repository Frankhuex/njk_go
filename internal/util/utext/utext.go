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

// EscapeCQAnimatedSummary repairs the bracketed animation label only inside
// complete CQ codes. Plain text and existing CQ escapes remain untouched.
func EscapeCQAnimatedSummary(text string) string {
	var result strings.Builder
	for {
		start := strings.Index(text, "[CQ:")
		if start < 0 {
			result.WriteString(text)
			return result.String()
		}
		result.WriteString(text[:start])
		text = text[start:]
		depth, end := 0, -1
		for i := 0; i < len(text); i++ {
			switch text[i] {
			case '[':
				depth++
			case ']':
				depth--
				if depth == 0 {
					end = i + 1
				}
			}
			if end >= 0 {
				break
			}
		}
		if end < 0 {
			result.WriteString(text)
			return result.String()
		}
		result.WriteString(strings.ReplaceAll(text[:end], "[动画表情]", "&#91;动画表情&#93;"))
		text = text[end:]
	}
}
