package claude

import "testing"

func TestExtractAssistantText(t *testing.T) {
	text := extractAssistantText(map[string]any{
		"content": []any{
			map[string]any{"type": "text", "text": "hello from claude"},
		},
	})
	if text != "hello from claude" {
		t.Fatalf("assistant text = %q", text)
	}
}

