package providers

import (
	"testing"

	"github.com/brianmeier/estuary/internal/domain"
)

// Claude adapter tests

func TestClaudeTerminalAdapterProvider(t *testing.T) {
	a := &ClaudeTerminalAdapter{}
	if a.Provider() != domain.HabitatClaude {
		t.Errorf("Provider() = %q, want %q", a.Provider(), domain.HabitatClaude)
	}
}

func TestClaudeTerminalAdapterStartArgsNoArgs(t *testing.T) {
	a := &ClaudeTerminalAdapter{}
	cmd, args, _ := a.StartArgs(domain.Session{CurrentModel: "claude-sonnet-4-6"})
	if cmd != "claude" {
		t.Errorf("StartArgs() cmd = %q, want %q", cmd, "claude")
	}
	if !containsSeq(args, "--model", "claude-sonnet-4-6") {
		t.Errorf("StartArgs() args %v missing --model claude-sonnet-4-6", args)
	}
}

func TestClaudeTerminalAdapterResumeArgsWithID(t *testing.T) {
	a := &ClaudeTerminalAdapter{}
	_, args, _ := a.ResumeArgs(domain.Session{}, "sess-abc123")
	if !containsSeq(args, "--resume", "sess-abc123") {
		t.Errorf("ResumeArgs() args %v missing --resume sess-abc123", args)
	}
}

func TestClaudeTerminalAdapterResumeArgsFallback(t *testing.T) {
	a := &ClaudeTerminalAdapter{}
	session := domain.Session{CurrentModel: "claude-sonnet-4-6"}
	_, argsResume, _ := a.ResumeArgs(session, "")
	_, argsStart, _ := a.StartArgs(session)
	if len(argsResume) != len(argsStart) {
		t.Errorf("ResumeArgs() with empty nativeID should fall back to StartArgs: got %v, want %v", argsResume, argsStart)
	}
}

func TestClaudeTerminalAdapterHandoffArgsUseStartupContext(t *testing.T) {
	a := &ClaudeTerminalAdapter{}
	_, args, _ := a.HandoffArgs(domain.Session{CurrentModel: "claude-sonnet-4-6"}, "handoff context")
	if !containsSeq(args, "--append-system-prompt", "handoff context") {
		t.Errorf("HandoffArgs() args %v missing --append-system-prompt handoff context", args)
	}
}

func TestClaudeTerminalAdapterDoesNotInjectModelSlashCommand(t *testing.T) {
	a := &ClaudeTerminalAdapter{}
	if got := a.ModelSwitchInput("claude-opus-4-6"); got != "" {
		t.Errorf("ModelSwitchInput() = %q, want empty", got)
	}
}

// Codex adapter tests

func TestCodexTerminalAdapterProvider(t *testing.T) {
	a := &CodexTerminalAdapter{}
	if a.Provider() != domain.HabitatCodex {
		t.Errorf("Provider() = %q, want %q", a.Provider(), domain.HabitatCodex)
	}
}

func TestCodexTerminalAdapterStartArgs(t *testing.T) {
	a := &CodexTerminalAdapter{}
	session := domain.Session{
		CurrentModel:   "codex-model",
		CurrentHabitat: domain.HabitatCodex,
		FolderPath:     "/home/user/project",
	}
	cmd, args, _ := a.StartArgs(session)
	if cmd != "codex" {
		t.Errorf("StartArgs() cmd = %q, want %q", cmd, "codex")
	}
	if !containsArg(args, "--no-alt-screen") {
		t.Errorf("StartArgs() args %v missing --no-alt-screen", args)
	}
	if !containsSeq(args, "-C", "/home/user/project") {
		t.Errorf("StartArgs() args %v missing -C /home/user/project", args)
	}
	if !containsSeq(args, "--model", "codex-model") {
		t.Errorf("StartArgs() args %v missing --model codex-model", args)
	}
}

func TestCodexTerminalAdapterResumeArgsWithID(t *testing.T) {
	a := &CodexTerminalAdapter{}
	session := domain.Session{FolderPath: "/project"}
	_, _, env := a.ResumeArgs(session, "thread-xyz")
	found := false
	for _, e := range env {
		if e == "CODEX_THREAD_ID=thread-xyz" {
			found = true
		}
	}
	if !found {
		t.Errorf("ResumeArgs() env %v missing CODEX_THREAD_ID=thread-xyz", env)
	}
}

func TestCodexTerminalAdapterHandoffArgsUseStartupContext(t *testing.T) {
	a := &CodexTerminalAdapter{}
	session := domain.Session{FolderPath: "/project", CurrentModel: "gpt-5.4"}
	_, args, _ := a.HandoffArgs(session, "handoff context")
	if !containsSeq(args, "-c", `developer_instructions="handoff context"`) {
		t.Errorf("HandoffArgs() args %v missing developer_instructions config", args)
	}
}

// containsSeq checks whether args contains the subsequence [key, value].
func containsSeq(args []string, key, value string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == key && args[i+1] == value {
			return true
		}
	}
	return false
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}
