package service

import "testing"

func TestParseAIHistoryCount(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  int
	}{
		{"0", 0},
		{"100", 100},
		{"500", 500},
		{"501", 500},
		{"999999999999999999999999999999999999", 500},
	} {
		t.Run(tc.value, func(t *testing.T) {
			if got := parseAIHistoryCount(tc.value); got != tc.want {
				t.Fatalf("parseAIHistoryCount(%q) = %d, want %d", tc.value, got, tc.want)
			}
		})
	}
}
