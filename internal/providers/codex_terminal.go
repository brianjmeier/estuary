package providers

import "github.com/brianmeier/estuary/internal/domain"

// CodexTerminalAdapter spawns and configures interactive Codex sessions.
type CodexTerminalAdapter struct{}

func (a *CodexTerminalAdapter) Provider() domain.Habitat {
	return domain.HabitatCodex
}

// StartArgs returns the command and args for a fresh Codex interactive session.
func (a *CodexTerminalAdapter) StartArgs(session domain.Session) (string, []string, []string) {
	args := []string{"--no-alt-screen"}
	if session.FolderPath != "" {
		args = append(args, "-C", session.FolderPath)
	}
	if session.CurrentModel != "" {
		args = append(args, "--model", session.CurrentModel)
	}
	return "codex", args, nil
}

// ResumeArgs returns args for resuming a prior Codex session by its thread ID.
func (a *CodexTerminalAdapter) ResumeArgs(session domain.Session, nativeID string) (string, []string, []string) {
	if nativeID == "" {
		return a.StartArgs(session)
	}
	cmd, args, baseEnv := a.StartArgs(session)
	return cmd, args, append(baseEnv, "CODEX_THREAD_ID="+nativeID)
}

// HandoffArgs returns args for a fresh session. Handoff injection is handled
// at the application layer by writing initial input to the PTY after the process starts.
func (a *CodexTerminalAdapter) HandoffArgs(session domain.Session, _ string) (string, []string, []string) {
	return a.StartArgs(session)
}

// ModelSwitchInput returns "" because Codex selects the model at startup via
// the --model flag and does not support runtime model switching.
func (a *CodexTerminalAdapter) ModelSwitchInput(_ string) string { return "" }
