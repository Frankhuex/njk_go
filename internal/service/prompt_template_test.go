package service

import (
	"strings"
	"testing"
)

func TestUserPromptTemplate(t *testing.T) {
	context := "群友：100% [CQ:face,id=12] %s"
	for _, tc := range []struct{ name, template, want string }{
		{"passthrough", "", context},
		{"wrapped", "身份\n%s\n要求", "身份\n" + context + "\n要求"},
		{"literal percentages", "100% %s", "100% " + context},
		{"static", "固定输入", "固定输入"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command := compiledCommand{UserPromptTemplate: tc.template}
			if got := command.userPrompt(context); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNJKUserPromptPlacesContextBeforeReplyRequirements(t *testing.T) {
	for _, def := range commandDefs("123") {
		if def.Key != commandNJK {
			continue
		}
		if def.SystemPrompt != "" {
			t.Fatal("NJK instructions must be in user content")
		}
		if strings.Count(def.UserPromptTemplate, "%s") != 1 {
			t.Fatal("NJK must have one context slot")
		}
		command := compiledCommand{UserPromptTemplate: def.UserPromptTemplate}
		prompt := command.userPrompt("unique-chat-context")
		contextAt := strings.Index(prompt, "unique-chat-context")
		if contextAt <= strings.Index(prompt, "你以QQ群成员") || contextAt >= strings.Index(prompt, "程序已决定本轮需要回复") {
			t.Fatal("context must be between identity and reply requirements")
		}
		return
	}
	t.Fatal("NJK definition missing")
}
