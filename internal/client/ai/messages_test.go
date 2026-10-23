package ai

import "testing"

func TestChatMessagesOptionalSystemPrompt(t *testing.T) {
	for _, system := range []string{"", " ", "system instructions"} {
		parts := []chatContentPart{{Type: "text", Text: "context"}, {Type: "image_url", ImageURL: &chatImageURL{URL: "https://example.com/image.png"}}}
		messages := chatMessages(system, parts)
		want := 1
		if system == "system instructions" {
			want = 2
		}
		if len(messages) != want {
			t.Fatalf("system %q: got %d messages", system, len(messages))
		}
		user := messages[len(messages)-1]
		content, ok := user.Content.([]chatContentPart)
		if user.Role != "user" || !ok || len(content) != 2 || content[1].ImageURL.URL != parts[1].ImageURL.URL {
			t.Fatal("user content or image attachment changed")
		}
		if want == 2 && (messages[0].Role != "system" || messages[0].Content != system) {
			t.Fatal("system instructions changed")
		}
	}
}
