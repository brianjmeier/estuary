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

func TestNativeSettingFallback(t *testing.T) {
	got := NativeSetting(`{"permission_mode":"default"}`, "permission_mode", "acceptEdits")
	if got != "default" {
		t.Fatalf("permission mode = %q", got)
	}
	got = NativeSetting(`{`, "permission_mode", "acceptEdits")
	if got != "acceptEdits" {
		t.Fatalf("fallback = %q", got)
	}
}
