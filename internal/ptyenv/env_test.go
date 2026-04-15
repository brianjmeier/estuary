package ptyenv

import "testing"

func TestBuildAddsTerminalDefaultsWhenMissing(t *testing.T) {
	got := Build([]string{"PATH=/bin"}, nil)

	assertEnv(t, got, "TERM", DefaultTerm)
	assertEnv(t, got, "COLORTERM", DefaultColorTerm)
	assertEnv(t, got, "PATH", "/bin")
}

func TestBuildNormalizesEmptyTerminalVars(t *testing.T) {
	got := Build([]string{"TERM=", "COLORTERM="}, nil)

	assertEnv(t, got, "TERM", DefaultTerm)
	assertEnv(t, got, "COLORTERM", DefaultColorTerm)
}

func TestBuildNormalizesExistingTerminalVars(t *testing.T) {
	got := Build([]string{"TERM=screen-256color", "COLORTERM=24bit"}, nil)

	assertEnv(t, got, "TERM", DefaultTerm)
	assertEnv(t, got, "COLORTERM", DefaultColorTerm)
}

func TestBuildPreservesAdapterEnvWithPrecedence(t *testing.T) {
	got := Build(
		[]string{"PATH=/bin", "CODEX_THREAD_ID=old"},
		[]string{"CODEX_THREAD_ID=new", "ESTUARY_FLAG=1"},
	)

	assertEnv(t, got, "PATH", "/bin")
	assertEnv(t, got, "CODEX_THREAD_ID", "new")
	assertEnv(t, got, "ESTUARY_FLAG", "1")
	assertEnv(t, got, "TERM", DefaultTerm)
	assertEnv(t, got, "COLORTERM", DefaultColorTerm)
}

func assertEnv(t *testing.T, env []string, key, want string) {
	t.Helper()
	prefix := key + "="
	for _, entry := range env {
		if len(entry) >= len(prefix) && entry[:len(prefix)] == prefix {
			if got := entry[len(prefix):]; got != want {
				t.Fatalf("%s = %q, want %q in %v", key, got, want, env)
			}
			return
		}
	}
	t.Fatalf("%s missing from %v", key, env)
}
