package service

import (
	"strings"
	"testing"

	"njk_go/internal/client/pgstore"
	"njk_go/internal/client/searxng"
	"njk_go/internal/config"
)

func TestSearchCommandSyntax(t *testing.T) {
	svc := NewService(config.Config{}, nil, nil, nil, nil, nil, nil)
	for _, input := range []string{
		".搜索3 解释这些结果", ".搜索 3 解释这些结果", ".搜索3\t解释这些结果", ".搜索3\n解释这些结果", ".搜索3　解释这些结果",
	} {
		match := svc.MatchCommand(input)
		if match == nil || match.Key() != string(commandSearch) || match.Groups[1] != "3" || match.Groups[2] != "解释这些结果" {
			t.Errorf("unexpected match for %q: %#v", input, match)
		}
	}
	for _, input := range []string{".搜索3", ".搜索 3", ".搜索3　"} {
		match := svc.MatchCommand(input)
		if match == nil || match.Key() != string(commandSearch) || match.Groups[1] != "3" || match.Groups[2] != "" {
			t.Errorf("unexpected match for %q: %#v", input, match)
		}
	}
	for _, input := range []string{".搜索3解释这些结果", ".搜索 3解释这些结果", ".搜索"} {
		if match := svc.MatchCommand(input); match != nil {
			t.Errorf("unexpected match for %q: %#v", input, match)
		}
	}
}

func TestSearchAIPromptOmitsEmptyInstruction(t *testing.T) {
	prompt := searchAIPrompt("", "查询词", []searxng.Result{{Title: "标题", URL: "https://example.org", Snippet: "摘要"}})
	if strings.Contains(prompt, "用户提示词") {
		t.Errorf("empty instruction should omit 用户提示词 line: %s", prompt)
	}
	if !strings.Contains(prompt, "搜索词：查询词") {
		t.Errorf("unexpected prompt: %s", prompt)
	}
}

func TestSearchQueryAndAIPrompt(t *testing.T) {
	history := []pgstore.StoredMessage{{Text: "第一条"}, {Text: "  "}, {Text: "第二条 内容"}}
	query := searchQueryFromHistory(history)
	if query != "第一条 第二条 内容" {
		t.Fatalf("unexpected query: %q", query)
	}
	prompt := searchAIPrompt("按时间顺序整理", query, []searxng.Result{{Title: "标题", URL: "https://example.org", Snippet: "摘要"}})
	for _, want := range []string{"用户提示词：按时间顺序整理", "搜索词：第一条 第二条 内容", "[1] 标题", "链接：https://example.org", "摘要：摘要"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("AI prompt missing %q: %s", want, prompt)
		}
	}
}
