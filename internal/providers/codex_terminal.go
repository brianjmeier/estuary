package providers

import (
	"strconv"

	"github.com/brianmeier/estuary/internal/domain"
)

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

// HandoffArgs starts Codex with handoff context as developer instructions,
// leaving the interactive input available immediately.
func (a *CodexTerminalAdapter) HandoffArgs(session domain.Session, packetText string) (string, []string, []string) {
	cmd, args, env := a.StartArgs(session)
	if packetText != "" {
		args = append(args, "-c", "developer_instructions="+strconv.Quote(packetText))
	}
	return cmd, args, env
}

// ModelSwitchInput returns "" because Codex selects the model at startup via
// the --model flag and does not support runtime model switching.
func (a *CodexTerminalAdapter) ModelSwitchInput(_ string) string { return "" }
